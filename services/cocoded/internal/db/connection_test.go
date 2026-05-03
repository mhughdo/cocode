package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenCreatesDatabaseAndAppliesPragmas(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "cocoded.sqlite")
	database, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	var foreignKeys int
	if err := database.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("query foreign_keys pragma: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}

	var busyTimeout int
	if err := database.QueryRowContext(context.Background(), "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("query busy_timeout pragma: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busyTimeout)
	}

	var journalMode string
	if err := database.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode pragma: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
}

func TestOpenRequiresPath(t *testing.T) {
	t.Parallel()

	if _, err := Open(context.Background(), ""); err == nil {
		t.Fatal("Open() error = nil, want error")
	}
}
