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

	"github.com/wreckx-in-scene/AuthForge/internal/database"
	"github.com/wreckx-in-scene/AuthForge/internal/digest"
	"github.com/wreckx-in-scene/AuthForge/internal/errs"
	"github.com/wreckx-in-scene/AuthForge/internal/id"
	"github.com/wreckx-in-scene/AuthForge/internal/pgerr"
)

type User struct {
	ID            string
	EnvironmentID string
	ProjectID     string
	Email         string
	EmailVerified bool
	Status        string
}

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) CreateUser(ctx context.Context, environmentID, projectID, email, normalized, passwordHash string) (User, error) {
	if err := id.Parse(projectID); err != nil {
		return User{}, err
	}
	user := User{
		ID:            uuid.NewString(),
		EnvironmentID: environmentID,
		ProjectID:     projectID,
		Email:         email,
		Status:        "active",
	}
	err := database.InEnvironment(ctx, s.pool, environmentID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO identity.users (id, environment_id, project_id, email, normalized_email)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING email_verified, status
		`, user.ID, environmentID, projectID, email, normalized).Scan(&user.EmailVerified, &user.Status)
		if pgerr.IsUnique(err) {
			return errs.ErrConflict
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO identity.credentials (environment_id, user_id, password_hash)
			VALUES ($1, $2, $3)
		`, environmentID, user.ID, passwordHash)
		return err
	})
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) FindByEmail(ctx context.Context, environmentID, normalized string) (User, string, error) {
	var user User
	var hash string
	err := database.InEnvironment(ctx, s.pool, environmentID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT u.id::text, u.environment_id::text, u.project_id::text, u.email, u.email_verified, u.status, c.password_hash
			FROM identity.users u
			JOIN identity.credentials c
			  ON c.environment_id = u.environment_id AND c.user_id = u.id
			WHERE u.environment_id = $1 AND u.normalized_email = $2
		`, environmentID, normalized).Scan(
			&user.ID, &user.EnvironmentID, &user.ProjectID, &user.Email, &user.EmailVerified, &user.Status, &hash,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return errs.ErrNotFound
		}
		return err
	})
	if err != nil {
		return User{}, "", err
	}
	return user, hash, nil
}

func (s *Store) GetUser(ctx context.Context, environmentID, userID string) (User, error) {
	if err := id.Parse(userID); err != nil {
		return User{}, err
	}
	var user User
	err := database.InEnvironment(ctx, s.pool, environmentID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id::text, environment_id::text, project_id::text, email, email_verified, status
			FROM identity.users
			WHERE environment_id = $1 AND id = $2
		`, environmentID, userID).Scan(
			&user.ID, &user.EnvironmentID, &user.ProjectID, &user.Email, &user.EmailVerified, &user.Status,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return errs.ErrNotFound
		}
		return err
	})
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) CreateGoogleNonce(ctx context.Context, environmentID string) (string, error) {
	if err := id.Parse(environmentID); err != nil {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	nonce := "af_gn_" + hex.EncodeToString(raw)
	err := database.InEnvironment(ctx, s.pool, environmentID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			DELETE FROM identity.google_auth_nonces
			WHERE environment_id = $1 AND (expires_at <= now() OR consumed_at IS NOT NULL)
		`, environmentID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO identity.google_auth_nonces (id, environment_id, nonce_hash, expires_at)
			VALUES ($1, $2, $3, $4)
		`, uuid.NewString(), environmentID, digest.SHA256(nonce), time.Now().UTC().Add(5*time.Minute))
		return err
	})
	if err != nil {
		return "", err
	}
	return nonce, nil
}

func (s *Store) AuthenticateGoogle(ctx context.Context, environmentID, projectID, subject, email, normalizedEmail, nonce string) (User, error) {
	if err := id.Parse(environmentID, projectID); err != nil {
		return User{}, err
	}
	if subject == "" || email == "" || nonce == "" {
		return User{}, errs.ErrInvalid
	}
	var user User
	err := database.InEnvironment(ctx, s.pool, environmentID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE identity.google_auth_nonces
			SET consumed_at = now()
			WHERE environment_id = $1 AND nonce_hash = $2
			  AND consumed_at IS NULL AND expires_at > now()
		`, environmentID, digest.SHA256(nonce))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errs.ErrUnauthenticated
		}
		err = tx.QueryRow(ctx, `
			SELECT u.id::text, u.environment_id::text, u.project_id::text, u.email, u.email_verified, u.status
			FROM identity.external_identities x
			JOIN identity.users u
			  ON u.environment_id = x.environment_id AND u.id = x.user_id
			WHERE x.environment_id = $1 AND x.provider = 'google' AND x.provider_subject = $2
		`, environmentID, subject).Scan(
			&user.ID, &user.EnvironmentID, &user.ProjectID, &user.Email, &user.EmailVerified, &user.Status,
		)
		if err == nil {
			if user.ProjectID != projectID {
				return errs.ErrForbidden
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		user = User{ID: uuid.NewString(), EnvironmentID: environmentID, ProjectID: projectID, Email: email}
		err = tx.QueryRow(ctx, `
			INSERT INTO identity.users (id, environment_id, project_id, email, normalized_email, email_verified)
			VALUES ($1, $2, $3, $4, $5, TRUE)
			ON CONFLICT (environment_id, normalized_email)
			DO UPDATE SET email_verified = TRUE
			RETURNING id::text, environment_id::text, project_id::text, email, email_verified, status
		`, user.ID, environmentID, projectID, email, normalizedEmail).Scan(
			&user.ID, &user.EnvironmentID, &user.ProjectID, &user.Email, &user.EmailVerified, &user.Status,
		)
		if err != nil {
			return err
		}
		if user.ProjectID != projectID {
			return errs.ErrForbidden
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO identity.external_identities (environment_id, provider, provider_subject, user_id)
			VALUES ($1, 'google', $2, $3)
			ON CONFLICT DO NOTHING
		`, environmentID, subject, user.ID); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			SELECT u.id::text, u.environment_id::text, u.project_id::text, u.email, u.email_verified, u.status
			FROM identity.external_identities x
			JOIN identity.users u
			  ON u.environment_id = x.environment_id AND u.id = x.user_id
			WHERE x.environment_id = $1 AND x.provider = 'google' AND x.provider_subject = $2
		`, environmentID, subject).Scan(
			&user.ID, &user.EnvironmentID, &user.ProjectID, &user.Email, &user.EmailVerified, &user.Status,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return errs.ErrConflict
		}
		if err != nil {
			return err
		}
		if user.ProjectID != projectID {
			return errs.ErrForbidden
		}
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) CreateEmailVerificationToken(ctx context.Context, environmentID, normalizedEmail string) (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	verificationToken := "af_ev_" + hex.EncodeToString(raw)
	var email string
	err := database.InEnvironment(ctx, s.pool, environmentID, func(tx pgx.Tx) error {
		var userID string
		var verified bool
		var currentEmail string
		err := tx.QueryRow(ctx, `
			SELECT id::text, email, email_verified
			FROM identity.users
			WHERE environment_id = $1 AND normalized_email = $2
			FOR UPDATE
		`, environmentID, normalizedEmail).Scan(&userID, &currentEmail, &verified)
		if errors.Is(err, pgx.ErrNoRows) {
			return errs.ErrNotFound
		}
		if err != nil {
			return err
		}
		if verified {
			return nil
		}
		email = currentEmail
		if _, err := tx.Exec(ctx, `
			DELETE FROM identity.email_verification_tokens
			WHERE environment_id = $1 AND user_id = $2
		`, environmentID, userID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO identity.email_verification_tokens (id, environment_id, user_id, token_hash, expires_at)
			VALUES ($1, $2, $3, $4, $5)
		`, uuid.NewString(), environmentID, userID, digest.SHA256(verificationToken), time.Now().UTC().Add(24*time.Hour))
		return err
	})
	if err != nil {
		return "", "", err
	}
	if email == "" {
		return "", "", nil
	}
	return email, verificationToken, nil
}

func (s *Store) VerifyEmail(ctx context.Context, environmentID, verificationToken string) error {
	if err := id.Parse(environmentID); err != nil {
		return err
	}
	return database.InEnvironment(ctx, s.pool, environmentID, func(tx pgx.Tx) error {
		var userID string
		var expiresAt time.Time
		var usedAt *time.Time
		err := tx.QueryRow(ctx, `
			SELECT user_id::text, expires_at, used_at
			FROM identity.email_verification_tokens
			WHERE environment_id = $1 AND token_hash = $2
			FOR UPDATE
		`, environmentID, digest.SHA256(verificationToken)).Scan(&userID, &expiresAt, &usedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return errs.ErrUnauthenticated
		}
		if err != nil {
			return err
		}
		if usedAt != nil {
			return errs.ErrUnauthenticated
		}
		if !time.Now().UTC().Before(expiresAt) {
			return errs.ErrUnauthenticated
		}
		tag, err := tx.Exec(ctx, `
			UPDATE identity.users SET email_verified = TRUE
			WHERE environment_id = $1 AND id = $2
		`, environmentID, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errs.ErrNotFound
		}
		_, err = tx.Exec(ctx, `
			UPDATE identity.email_verification_tokens SET used_at = now()
			WHERE environment_id = $1 AND user_id = $2 AND used_at IS NULL
		`, environmentID, userID)
		return err
	})
}
