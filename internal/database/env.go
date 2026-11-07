package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wreckx-in-scene/AuthForge/internal/errs"
	"github.com/wreckx-in-scene/AuthForge/internal/id"
)

func InEnvironment(ctx context.Context, pool *pgxpool.Pool, environmentID string, fn func(pgx.Tx) error) error {
	if err := id.Parse(environmentID); err != nil {
		return errs.ErrInvalid
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT set_config('authforge.environment_id', $1, true)`, environmentID); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
