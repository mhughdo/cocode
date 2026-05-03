package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

type Migration struct {
	Version int
	Name    string
	SQL     string
}

func Apply(ctx context.Context, database *sql.DB, migrations []Migration) error {
	if database == nil {
		return errors.New("db is required")
	}
	if err := validateMigrations(migrations); err != nil {
		return err
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TEXT NOT NULL
)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, tx)
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if applied[migration.Version] {
			continue
		}
		if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
			return fmt.Errorf("apply migration %d %s: %w", migration.Version, migration.Name, err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)",
			migration.Version,
			migration.Name,
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record migration %d %s: %w", migration.Version, migration.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

func appliedVersions(ctx context.Context, tx *sql.Tx) (map[int]bool, error) {
	rows, err := tx.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("load applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}
	return applied, nil
}

func validateMigrations(migrations []Migration) error {
	seen := make(map[int]bool, len(migrations))
	versions := make([]int, 0, len(migrations))
	for _, migration := range migrations {
		if migration.Version <= 0 {
			return fmt.Errorf("migration %q has invalid version %d", migration.Name, migration.Version)
		}
		if migration.Name == "" {
			return fmt.Errorf("migration %d has empty name", migration.Version)
		}
		if migration.SQL == "" {
			return fmt.Errorf("migration %d %s has empty sql", migration.Version, migration.Name)
		}
		if seen[migration.Version] {
			return fmt.Errorf("duplicate migration version %d", migration.Version)
		}
		seen[migration.Version] = true
		versions = append(versions, migration.Version)
	}

	if !sort.IntsAreSorted(versions) {
		return errors.New("migrations must be sorted by version")
	}
	return nil
}
