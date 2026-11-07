package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wreckx-in-scene/AuthForge/internal/digest"
	"github.com/wreckx-in-scene/AuthForge/internal/errs"
	"github.com/wreckx-in-scene/AuthForge/internal/id"
	"github.com/wreckx-in-scene/AuthForge/internal/pgerr"
)

type Account struct {
	ID    string
	Email string
	Name  string
}

type Organization struct {
	ID   string
	Name string
	Slug string
}

type Project struct {
	ID             string
	OrganizationID string
	Name           string
	Slug           string
}

type Environment struct {
	ID             string
	ProjectID      string
	OrganizationID string
	Name           string
}

type IssuedKey struct {
	ID            string
	EnvironmentID string
	KeyClass      string
	Prefix        string
}

type ResolvedKey struct {
	KeyID           string
	OrganizationID  string
	ProjectID       string
	EnvironmentID   string
	KeyClass        string
	EnvironmentName string
}

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) CreateAccount(ctx context.Context, email, normalized, name, passwordHash string) (Account, error) {
	account := Account{ID: uuid.NewString(), Email: email, Name: name}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tenant.accounts (id, email, normalized_email, password_hash, name)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, email, name
	`, account.ID, email, normalized, passwordHash, name).Scan(&account.ID, &account.Email, &account.Name)
	if pgerr.IsUnique(err) {
		return Account{}, errs.ErrConflict
	}
	if err != nil {
		return Account{}, err
	}
	return account, nil
}

func (s *Store) FindAccount(ctx context.Context, normalized string) (Account, string, error) {
	var account Account
	var hash string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, email, name, password_hash
		FROM tenant.accounts
		WHERE normalized_email = $1
	`, normalized).Scan(&account.ID, &account.Email, &account.Name, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, "", errs.ErrNotFound
	}
	if err != nil {
		return Account{}, "", err
	}
	return account, hash, nil
}

func (s *Store) RequestPasswordReset(ctx context.Context, normalized string) (string, time.Time, error) {
	var account Account
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, email, name
		FROM tenant.accounts
		WHERE normalized_email = $1
	`, normalized).Scan(&account.ID, &account.Email, &account.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, errs.ErrNotFound
	}
	if err != nil {
		return "", time.Time{}, err
	}
	resetToken, err := newPasswordResetToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().UTC().Add(30 * time.Minute)
	_, err = s.pool.Exec(ctx, `
		DELETE FROM tenant.password_reset_tokens
		WHERE account_id = $1
	`, account.ID)
	if err != nil {
		return "", time.Time{}, err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO tenant.password_reset_tokens (id, account_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, uuid.NewString(), account.ID, digest.SHA256(resetToken), expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	return resetToken, expiresAt, nil
}

func (s *Store) ResetPassword(ctx context.Context, resetToken, newPasswordHash string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var accountID string
	var expiresAt time.Time
	var usedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT account_id::text, expires_at, used_at
		FROM tenant.password_reset_tokens
		WHERE token_hash = $1
		FOR UPDATE
	`, digest.SHA256(resetToken)).Scan(&accountID, &expiresAt, &usedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errs.ErrUnauthenticated
	}
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if usedAt != nil || !now.Before(expiresAt) {
		if _, updateErr := tx.Exec(ctx, `
			UPDATE tenant.password_reset_tokens
			SET used_at = COALESCE(used_at, now())
			WHERE token_hash = $1
		`, digest.SHA256(resetToken)); updateErr != nil {
			return "", updateErr
		}
		return "", errs.ErrUnauthenticated
	}
	_, err = tx.Exec(ctx, `
		UPDATE tenant.accounts
		SET password_hash = $1
		WHERE id = $2
	`, newPasswordHash, accountID)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `
		UPDATE tenant.password_reset_tokens
		SET used_at = now()
		WHERE token_hash = $1
	`, digest.SHA256(resetToken))
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return accountID, nil
}

func newPasswordResetToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "af_pr_" + hex.EncodeToString(raw), nil
}

func (s *Store) CreateOrganization(ctx context.Context, accountID, name, slug string) (Organization, error) {
	if err := id.Parse(accountID); err != nil {
		return Organization{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Organization{}, err
	}
	defer tx.Rollback(ctx)

	var exists int
	err = tx.QueryRow(ctx, `SELECT 1 FROM tenant.accounts WHERE id = $1`, accountID).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, errs.ErrNotFound
	}
	if err != nil {
		return Organization{}, err
	}

	org := Organization{ID: uuid.NewString(), Name: name, Slug: slug}
	err = tx.QueryRow(ctx, `
		INSERT INTO tenant.organizations (id, name, slug)
		VALUES ($1, $2, $3)
		RETURNING id::text, name, slug
	`, org.ID, name, slug).Scan(&org.ID, &org.Name, &org.Slug)
	if pgerr.IsUnique(err) {
		return Organization{}, errs.ErrConflict
	}
	if err != nil {
		return Organization{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tenant.organization_members (organization_id, account_id, role)
		VALUES ($1, $2, 'owner')
	`, org.ID, accountID); err != nil {
		return Organization{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Organization{}, err
	}
	return org, nil
}

func (s *Store) CreateProject(ctx context.Context, accountID, organizationID, name, slug string) (Project, error) {
	if err := id.Parse(accountID, organizationID); err != nil {
		return Project{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireMember(ctx, tx, organizationID, accountID); err != nil {
		return Project{}, err
	}
	project := Project{ID: uuid.NewString(), OrganizationID: organizationID, Name: name, Slug: slug}
	err = tx.QueryRow(ctx, `
		INSERT INTO tenant.projects (id, organization_id, name, slug)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, organization_id::text, name, slug
	`, project.ID, organizationID, name, slug).Scan(&project.ID, &project.OrganizationID, &project.Name, &project.Slug)
	if pgerr.IsUnique(err) {
		return Project{}, errs.ErrConflict
	}
	if err != nil {
		return Project{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Project{}, err
	}
	return project, nil
}

func (s *Store) CreateEnvironment(ctx context.Context, accountID, projectID, name string) (Environment, error) {
	if err := id.Parse(accountID, projectID); err != nil {
		return Environment{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Environment{}, err
	}
	defer tx.Rollback(ctx)

	var organizationID string
	err = tx.QueryRow(ctx, `SELECT organization_id::text FROM tenant.projects WHERE id = $1`, projectID).Scan(&organizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Environment{}, errs.ErrNotFound
	}
	if err != nil {
		return Environment{}, err
	}
	if err := requireMember(ctx, tx, organizationID, accountID); err != nil {
		return Environment{}, err
	}

	env := Environment{ID: uuid.NewString(), ProjectID: projectID, OrganizationID: organizationID, Name: name}
	err = tx.QueryRow(ctx, `
		INSERT INTO tenant.environments (id, project_id, organization_id, name)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, project_id::text, organization_id::text, name
	`, env.ID, projectID, organizationID, name).Scan(&env.ID, &env.ProjectID, &env.OrganizationID, &env.Name)
	if pgerr.IsUnique(err) {
		return Environment{}, errs.ErrConflict
	}
	if err != nil {
		return Environment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Environment{}, err
	}
	return env, nil
}

func (s *Store) ResolveEnvironmentName(ctx context.Context, accountID, environmentID string) (string, error) {
	if err := id.Parse(accountID, environmentID); err != nil {
		return "", err
	}
	var name, organizationID string
	err := s.pool.QueryRow(ctx, `
		SELECT name, organization_id::text
		FROM tenant.environments
		WHERE id = $1
	`, environmentID).Scan(&name, &organizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errs.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	var member int
	err = s.pool.QueryRow(ctx, `
		SELECT 1 FROM tenant.organization_members
		WHERE organization_id = $1 AND account_id = $2
	`, organizationID, accountID).Scan(&member)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errs.ErrForbidden
	}
	if err != nil {
		return "", err
	}
	return name, nil
}

func (s *Store) InsertAPIKey(ctx context.Context, accountID, environmentID, class, prefix string, keyHash []byte) (IssuedKey, error) {
	if err := id.Parse(accountID, environmentID); err != nil {
		return IssuedKey{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return IssuedKey{}, err
	}
	defer tx.Rollback(ctx)

	var projectID, organizationID string
	err = tx.QueryRow(ctx, `
		SELECT project_id::text, organization_id::text
		FROM tenant.environments
		WHERE id = $1
	`, environmentID).Scan(&projectID, &organizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return IssuedKey{}, errs.ErrNotFound
	}
	if err != nil {
		return IssuedKey{}, err
	}
	if err := requireMember(ctx, tx, organizationID, accountID); err != nil {
		return IssuedKey{}, err
	}

	issued := IssuedKey{ID: uuid.NewString(), EnvironmentID: environmentID, KeyClass: class, Prefix: prefix}
	_, err = tx.Exec(ctx, `
		INSERT INTO tenant.api_keys (id, environment_id, project_id, organization_id, prefix, key_hash, key_class)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, issued.ID, environmentID, projectID, organizationID, prefix, keyHash, class)
	if err != nil {
		return IssuedKey{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return IssuedKey{}, err
	}
	return issued, nil
}

func (s *Store) ResolveAPIKey(ctx context.Context, keyHash []byte) (ResolvedKey, error) {
	var resolved ResolvedKey
	err := s.pool.QueryRow(ctx, `
		SELECT k.id::text, k.organization_id::text, k.project_id::text, k.environment_id::text, k.key_class, e.name
		FROM tenant.api_keys k
		JOIN tenant.environments e ON e.id = k.environment_id AND e.project_id = k.project_id
		WHERE k.key_hash = $1 AND k.status = 'active'
	`, keyHash).Scan(
		&resolved.KeyID,
		&resolved.OrganizationID,
		&resolved.ProjectID,
		&resolved.EnvironmentID,
		&resolved.KeyClass,
		&resolved.EnvironmentName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ResolvedKey{}, errs.ErrNotFound
	}
	if err != nil {
		return ResolvedKey{}, err
	}
	return resolved, nil
}

func requireMember(ctx context.Context, tx pgx.Tx, organizationID, accountID string) error {
	var exists int
	err := tx.QueryRow(ctx, `SELECT 1 FROM tenant.organizations WHERE id = $1`, organizationID).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return errs.ErrNotFound
	}
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `
		SELECT 1 FROM tenant.organization_members
		WHERE organization_id = $1 AND account_id = $2
	`, organizationID, accountID).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return errs.ErrForbidden
	}
	return err
}
