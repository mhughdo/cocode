package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const (
	DriverName     = "sqlite"
	MemoryDatabase = ":memory:"
)

var pragmaStatements = []string{
	"PRAGMA foreign_keys = ON",
	"PRAGMA journal_mode = WAL",
	"PRAGMA synchronous = NORMAL",
	"PRAGMA busy_timeout = 5000",
}

func Open(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		return nil, errors.New("db path is required")
	}
	if path != MemoryDatabase {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}

	database, err := sql.Open(DriverName, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	database.SetMaxOpenConns(1)

	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("ping sqlite db: %w", err)
	}
	for _, statement := range pragmaStatements {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			_ = database.Close()
			return nil, fmt.Errorf("apply sqlite pragma %q: %w", statement, err)
		}
	}

	return database, nil
}
