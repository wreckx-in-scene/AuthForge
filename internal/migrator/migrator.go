package migrator

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/wreckx-in-scene/AuthForge/db"
)

func Up(databaseURL string) error {
	steps := []struct {
		table string
		files fs.FS
		dir   string
	}{
		{table: "tenant_schema_migrations", files: db.Tenant, dir: "migrations/tenant"},
		{table: "identity_schema_migrations", files: db.Identity, dir: "migrations/identity"},
		{table: "session_schema_migrations", files: db.Session, dir: "migrations/session"},
	}
	for _, step := range steps {
		if err := apply(databaseURL, step.table, step.files, step.dir); err != nil {
			return fmt.Errorf("%s: %w", step.table, err)
		}
	}
	return nil
}

func apply(databaseURL, table string, files fs.FS, dir string) error {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	driver, err := pgxmigrate.WithInstance(sqlDB, &pgxmigrate.Config{
		MigrationsTable:       table,
		SchemaName:            "public",
		MultiStatementEnabled: true,
		MultiStatementMaxSize: 10 * 1 << 20,
	})
	if err != nil {
		sqlDB.Close()
		return err
	}
	src, err := iofs.New(files, dir)
	if err != nil {
		sqlDB.Close()
		return err
	}
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		sqlDB.Close()
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}
