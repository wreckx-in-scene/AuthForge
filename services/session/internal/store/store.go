package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wreckx-in-scene/AuthForge/internal/database"
	"github.com/wreckx-in-scene/AuthForge/internal/digest"
	"github.com/wreckx-in-scene/AuthForge/internal/errs"
	"github.com/wreckx-in-scene/AuthForge/internal/id"
	"github.com/wreckx-in-scene/AuthForge/internal/token"
)

type Issued struct {
	SessionID     string
	EnvironmentID string
	ProjectID     string
	UserID        string
	RefreshToken  string
	ExpiresAt     time.Time
}

type Store struct {
	pool *pgxpool.Pool
	ttl  time.Duration
}

func New(pool *pgxpool.Pool, ttl time.Duration) *Store {
	return &Store{pool: pool, ttl: ttl}
}

func (s *Store) Create(ctx context.Context, environmentID, projectID, userID string) (Issued, error) {
	if err := id.Parse(environmentID, projectID, userID); err != nil {
		return Issued{}, err
	}
	issued := Issued{
		SessionID:     uuid.NewString(),
		EnvironmentID: environmentID,
		ProjectID:     projectID,
		UserID:        userID,
		ExpiresAt:     time.Now().UTC().Add(s.ttl),
	}
	err := database.InEnvironment(ctx, s.pool, environmentID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO session.sessions (id, environment_id, project_id, user_id, status, expires_at)
			VALUES ($1, $2, $3, $4, 'active', $5)
		`, issued.SessionID, environmentID, projectID, userID, issued.ExpiresAt); err != nil {
			return err
		}
		plain, err := insertRefresh(ctx, tx, environmentID, issued.SessionID, "", issued.ExpiresAt)
		if err != nil {
			return err
		}
		issued.RefreshToken = plain
		return nil
	})
	if err != nil {
		return Issued{}, err
	}
	return issued, nil
}

func (s *Store) Rotate(ctx context.Context, refreshToken string) (Issued, error) {
	if refreshToken == "" {
		return Issued{}, errs.ErrUnauthenticated
	}
	sum := digest.SHA256(refreshToken)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Issued{}, err
	}
	defer tx.Rollback(ctx)

	var environmentID *string
	if err := tx.QueryRow(ctx, `SELECT session.peek_refresh_environment($1)::text`, sum).Scan(&environmentID); err != nil {
		return Issued{}, err
	}
	if environmentID == nil || *environmentID == "" {
		return Issued{}, errs.ErrUnauthenticated
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('authforge.environment_id', $1, true)`, *environmentID); err != nil {
		return Issued{}, err
	}

	var (
		tokenID, sessionID, userID, projectID, sessionStatus string
		usedAt, revokedAt                                    *time.Time
		tokenExp, sessionExp                                 time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT rt.id::text, rt.session_id::text, rt.used_at, rt.revoked_at, rt.expires_at,
		       s.user_id::text, s.project_id::text, s.status, s.expires_at
		FROM session.refresh_tokens rt
		JOIN session.sessions s ON s.id = rt.session_id AND s.environment_id = rt.environment_id
		WHERE rt.token_hash = $1
		FOR UPDATE OF rt, s
	`, sum).Scan(&tokenID, &sessionID, &usedAt, &revokedAt, &tokenExp, &userID, &projectID, &sessionStatus, &sessionExp)
	if errors.Is(err, pgx.ErrNoRows) {
		return Issued{}, errs.ErrUnauthenticated
	}
	if err != nil {
		return Issued{}, err
	}

	now := time.Now().UTC()
	switch decide(usedAt != nil, revokedAt != nil, sessionStatus, tokenExp, sessionExp, now) {
	case decisionReuse:
		if err := revokeFamily(ctx, tx, sessionID); err != nil {
			return Issued{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Issued{}, err
		}
		return Issued{}, errs.ErrReuse
	case decisionReject:
		return Issued{}, errs.ErrUnauthenticated
	}

	expires := now.Add(s.ttl)
	if _, err := tx.Exec(ctx, `UPDATE session.refresh_tokens SET used_at = $2 WHERE id = $1`, tokenID, now); err != nil {
		return Issued{}, err
	}
	plain, err := insertRefresh(ctx, tx, *environmentID, sessionID, tokenID, expires)
	if err != nil {
		return Issued{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE session.sessions SET expires_at = $2 WHERE id = $1`, sessionID, expires); err != nil {
		return Issued{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Issued{}, err
	}
	return Issued{
		SessionID:     sessionID,
		EnvironmentID: *environmentID,
		ProjectID:     projectID,
		UserID:        userID,
		RefreshToken:  plain,
		ExpiresAt:     expires,
	}, nil
}

func (s *Store) Revoke(ctx context.Context, refreshToken, sessionID, environmentID string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	if refreshToken != "" {
		sum := digest.SHA256(refreshToken)
		var peeked *string
		var foundSession string
		if err := tx.QueryRow(ctx, `SELECT session.peek_refresh_environment($1)::text`, sum).Scan(&peeked); err != nil {
			return "", err
		}
		if peeked == nil || *peeked == "" {
			return "", errs.ErrUnauthenticated
		}
		if environmentID != "" && environmentID != *peeked {
			return "", errs.ErrUnauthenticated
		}
		environmentID = *peeked
		if _, err := tx.Exec(ctx, `SELECT set_config('authforge.environment_id', $1, true)`, environmentID); err != nil {
			return "", err
		}
		err = tx.QueryRow(ctx, `
			SELECT session_id::text FROM session.refresh_tokens WHERE token_hash = $1
		`, sum).Scan(&foundSession)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errs.ErrUnauthenticated
		}
		if err != nil {
			return "", err
		}
		sessionID = foundSession
	} else {
		if err := id.Parse(sessionID, environmentID); err != nil {
			return "", errs.ErrInvalid
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('authforge.environment_id', $1, true)`, environmentID); err != nil {
			return "", err
		}
	}

	var status string
	err = tx.QueryRow(ctx, `
		SELECT status FROM session.sessions WHERE id = $1 AND environment_id = $2
	`, sessionID, environmentID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errs.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if err := revokeFamily(ctx, tx, sessionID); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return sessionID, nil
}

func insertRefresh(ctx context.Context, tx pgx.Tx, environmentID, sessionID, parentID string, expires time.Time) (string, error) {
	plain, sum, err := token.NewRefresh()
	if err != nil {
		return "", err
	}
	var parent any
	if parentID != "" {
		parent = parentID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO session.refresh_tokens (id, session_id, environment_id, token_hash, parent_id, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, uuid.NewString(), sessionID, environmentID, sum, parent, expires)
	if err != nil {
		return "", err
	}
	return plain, nil
}

func revokeFamily(ctx context.Context, tx pgx.Tx, sessionID string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE session.sessions
		SET status = 'revoked', revoked_at = COALESCE(revoked_at, now())
		WHERE id = $1
	`, sessionID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE session.refresh_tokens
		SET revoked_at = COALESCE(revoked_at, now())
		WHERE session_id = $1
	`, sessionID)
	return err
}
