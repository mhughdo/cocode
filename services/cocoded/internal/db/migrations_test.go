package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestApplyRunsSchemaV1Idempotently(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	if err := Apply(context.Background(), database, Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if err := Apply(context.Background(), database, Migrations); err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}

	for _, table := range []string{
		"workspaces",
		"repositories",
		"pull_request_snapshots",
		"changed_files",
		"review_sessions",
		"agent_configs",
		"review_session_agents",
		"artifacts",
		"events",
		"context_bundles",
		"context_items",
		"agent_runs",
		"finding_candidates",
		"findings",
		"finding_candidate_links",
		"evidence_items",
		"evidence_graphs",
		"evidence_nodes",
		"evidence_edges",
		"call_paths",
		"call_path_steps",
		"finding_threads",
		"finding_thread_messages",
		"human_decisions",
		"copy_packets",
		"publish_drafts",
		"github_publications",
		"credential_refs",
		"review_rules",
		"finding_search",
		"evidence_search",
	} {
		assertTableExists(t, database, table)
	}

	var count int
	if err := database.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("schema_migrations count = %d, want 1", count)
	}
}

func TestApplyEnforcesForeignKeys(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	if err := Apply(context.Background(), database, Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	_, err = database.ExecContext(context.Background(), `
INSERT INTO repositories(id, workspace_id, name, local_path, created_at, updated_at)
VALUES ('repo_1', 'missing_workspace', 'repo', '/tmp/repo', '2026-05-03T00:00:00Z', '2026-05-03T00:00:00Z')`)
	if err == nil {
		t.Fatal("insert with missing workspace error = nil, want foreign key error")
	}
}

func TestApplyRejectsInvalidMigrations(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	err = Apply(context.Background(), database, []Migration{
		{Version: 2, Name: "two", SQL: "CREATE TABLE two(id TEXT PRIMARY KEY)"},
		{Version: 1, Name: "one", SQL: "CREATE TABLE one(id TEXT PRIMARY KEY)"},
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want invalid migration error")
	}
}

func assertTableExists(t *testing.T, database *sql.DB, table string) {
	t.Helper()

	var name string
	err := database.QueryRowContext(
		context.Background(),
		"SELECT name FROM sqlite_master WHERE type IN ('table', 'virtual table') AND name = ?",
		table,
	).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("table %q does not exist", table)
	}
	if err != nil {
		t.Fatalf("lookup table %q: %v", table, err)
	}
}
