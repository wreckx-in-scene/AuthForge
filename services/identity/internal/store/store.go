package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wreckx-in-scene/AuthForge/internal/database"
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
