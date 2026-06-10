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
		"chat_threads",
		"chat_messages",
		"chat_message_context_refs",
		"chat_turns",
		"chat_turn_agent_runs",
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
	if count != len(Migrations) {
		t.Fatalf("schema_migrations count = %d, want %d", count, len(Migrations))
	}
}

func TestApplyDropsReviewSessionAgentConfigUniqueness(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	const oldReviewSessionAgentsSQL = `
CREATE TABLE review_sessions (
  id TEXT PRIMARY KEY
);

CREATE TABLE agent_configs (
  id TEXT PRIMARY KEY,
  name TEXT,
  role TEXT,
  adapter_kind TEXT,
  command TEXT,
  args_json TEXT NOT NULL DEFAULT '[]',
  output_mode TEXT,
  updated_at TEXT
);

CREATE TABLE review_session_agents (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  agent_config_id TEXT NOT NULL REFERENCES agent_configs(id) ON DELETE RESTRICT,
  role TEXT NOT NULL,
  run_order INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  settings_override_json TEXT NOT NULL DEFAULT '{}',
  UNIQUE(review_session_id, agent_config_id)
)`
	if err := Apply(context.Background(), database, []Migration{{
		Version: 1,
		Name:    "schema_v1",
		SQL:     oldReviewSessionAgentsSQL,
	}}); err != nil {
		t.Fatalf("Apply(old schema) error = %v", err)
	}
	if err := Apply(context.Background(), database, Migrations); err != nil {
		t.Fatalf("Apply(current migrations) error = %v", err)
	}

	rows, err := database.QueryContext(context.Background(), "PRAGMA index_list(review_session_agents)")
	if err != nil {
		t.Fatalf("index_list(review_session_agents) error = %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan index_list row: %v", err)
		}
		if unique == 1 && origin != "pk" {
			t.Fatalf("review_session_agents still has non-pk unique index %q", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate index_list rows: %v", err)
	}
}

func TestApplyAllowsEvidenceMapRouteNodeKinds(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	const oldEvidenceMapSQL = `
CREATE TABLE evidence_graphs (
  id TEXT PRIMARY KEY
);

CREATE TABLE evidence_items (
  id TEXT PRIMARY KEY
);

CREATE TABLE call_paths (
  id TEXT PRIMARY KEY
);

CREATE TABLE evidence_nodes (
  id TEXT PRIMARY KEY,
  evidence_graph_id TEXT NOT NULL REFERENCES evidence_graphs(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK(kind IN ('changed_code','related_code','middleware','guard','handler','test','config','counter_evidence','missing_guard','unknown')),
  label TEXT NOT NULL,
  path TEXT,
  symbol TEXT,
  start_line INTEGER,
  end_line INTEGER,
  evidence_item_id TEXT REFERENCES evidence_items(id) ON DELETE SET NULL,
  confidence REAL NOT NULL DEFAULT 0.5,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE evidence_edges (
  id TEXT PRIMARY KEY,
  evidence_graph_id TEXT NOT NULL REFERENCES evidence_graphs(id) ON DELETE CASCADE,
  source_node_id TEXT NOT NULL REFERENCES evidence_nodes(id) ON DELETE CASCADE,
  target_node_id TEXT NOT NULL REFERENCES evidence_nodes(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK(kind IN ('calls','mounts','protects','tests','supports','contradicts','missing_guard','imports','reads','writes','unknown')),
  status TEXT NOT NULL DEFAULT 'observed',
  label TEXT,
  confidence REAL NOT NULL DEFAULT 0.5,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE call_path_steps (
  id TEXT PRIMARY KEY,
  call_path_id TEXT NOT NULL REFERENCES call_paths(id) ON DELETE CASCADE,
  step_index INTEGER NOT NULL,
  node_id TEXT REFERENCES evidence_nodes(id) ON DELETE SET NULL,
  path TEXT,
  start_line INTEGER,
  end_line INTEGER,
  label TEXT NOT NULL,
  UNIQUE(call_path_id, step_index)
);

INSERT INTO evidence_graphs(id) VALUES ('graph_1');
INSERT INTO call_paths(id) VALUES ('call_path_1');
INSERT INTO evidence_nodes(id, evidence_graph_id, kind, label, metadata_json)
VALUES
  ('node_changed', 'graph_1', 'changed_code', 'Changed code', '{}'),
  ('node_related', 'graph_1', 'related_code', 'Related code', '{}');
INSERT INTO evidence_edges(id, evidence_graph_id, source_node_id, target_node_id, kind, metadata_json)
VALUES ('edge_1', 'graph_1', 'node_changed', 'node_related', 'calls', '{}');
INSERT INTO call_path_steps(id, call_path_id, step_index, node_id, label)
VALUES ('step_1', 'call_path_1', 0, 'node_changed', 'Changed code');
`
	legacyMigrations := []Migration{
		{Version: 1, Name: "schema_v1", SQL: oldEvidenceMapSQL},
		{Version: 2, Name: "noop_2", SQL: "SELECT 1;"},
		{Version: 3, Name: "noop_3", SQL: "SELECT 1;"},
		{Version: 4, Name: "noop_4", SQL: "SELECT 1;"},
		{Version: 5, Name: "noop_5", SQL: "SELECT 1;"},
		{Version: 6, Name: "noop_6", SQL: "SELECT 1;"},
	}
	if err := Apply(context.Background(), database, legacyMigrations); err != nil {
		t.Fatalf("Apply(legacy schema) error = %v", err)
	}
	if err := Apply(context.Background(), database, Migrations); err != nil {
		t.Fatalf("Apply(current migrations) error = %v", err)
	}

	if _, err := database.ExecContext(
		context.Background(),
		"INSERT INTO evidence_nodes(id, evidence_graph_id, kind, label, metadata_json) VALUES (?, ?, ?, ?, '{}'), (?, ?, ?, ?, '{}')",
		"node_route",
		"graph_1",
		"route",
		"Route",
		"node_entrypoint",
		"graph_1",
		"entrypoint",
		"Entrypoint",
	); err != nil {
		t.Fatalf("insert route node kinds: %v", err)
	}

	var edgeCount int
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM evidence_edges WHERE id = 'edge_1'",
	).Scan(&edgeCount); err != nil {
		t.Fatalf("count migrated edges: %v", err)
	}
	if edgeCount != 1 {
		t.Fatalf("migrated edge count = %d, want 1", edgeCount)
	}
}

func TestApplyRepairsStaleClaudeToolsArgs(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	if err := Apply(context.Background(), database, Migrations[:3]); err != nil {
		t.Fatalf("Apply(initial migrations) error = %v", err)
	}
	const staleArgs = `["-p","{{prompt}}","--output-format","json","--permission-mode","plan","--no-session-persistence","--tools"]`
	const validToolsArgs = `["--tools","Read,Glob","-p","{{prompt}}"]`
	if _, err := database.ExecContext(context.Background(), `
INSERT INTO agent_configs (
  id, name, role, adapter_kind, command, args_json, cwd_mode,
  env_allowlist_json, output_mode, capabilities_json, settings_json,
  enabled, created_at, updated_at
) VALUES
  ('agent_config_stale_claude', 'Claude Code CLI', 'primary_reviewer', 'cli_noninteractive', 'claude', ?, 'repo_root', '[]', 'json', '{}', '{}', 1, '2026-05-10T00:00:00Z', '2026-05-10T00:00:00Z'),
  ('agent_config_valid_claude', 'Claude With Tools', 'primary_reviewer', 'cli_noninteractive', 'claude', ?, 'repo_root', '[]', 'json', '{}', '{}', 1, '2026-05-10T00:00:00Z', '2026-05-10T00:00:00Z')
`, staleArgs, validToolsArgs); err != nil {
		t.Fatalf("insert stale configs: %v", err)
	}
	if err := Apply(context.Background(), database, Migrations); err != nil {
		t.Fatalf("Apply(repair migration) error = %v", err)
	}

	const repairedArgs = `["-p","{{prompt}}","--output-format","stream-json","--verbose","--include-partial-messages","--permission-mode","plan","--no-session-persistence"]`
	var staleConfigArgs string
	if err := database.QueryRowContext(context.Background(), "SELECT args_json FROM agent_configs WHERE id = 'agent_config_stale_claude'").Scan(&staleConfigArgs); err != nil {
		t.Fatalf("read repaired stale config: %v", err)
	}
	if staleConfigArgs != repairedArgs {
		t.Fatalf("stale config args = %s, want %s", staleConfigArgs, repairedArgs)
	}
	var validConfigArgs string
	if err := database.QueryRowContext(context.Background(), "SELECT args_json FROM agent_configs WHERE id = 'agent_config_valid_claude'").Scan(&validConfigArgs); err != nil {
		t.Fatalf("read valid config: %v", err)
	}
	if validConfigArgs != validToolsArgs {
		t.Fatalf("valid config args = %s, want unchanged %s", validConfigArgs, validToolsArgs)
	}
}

func TestApplyPromotesDefaultCodexCLIToOrchestrator(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	if err := Apply(context.Background(), database, Migrations[:7]); err != nil {
		t.Fatalf("Apply(initial migrations) error = %v", err)
	}
	const codexCLIArgs = `["-a","never","exec","--json","--sandbox","workspace-write","--add-dir","/tmp/cocode-agent-runtime","--skip-git-repo-check","--ephemeral","--ignore-rules","--color","never","-"]`
	if _, err := database.ExecContext(context.Background(), `
INSERT INTO agent_configs (
  id, name, role, adapter_kind, command, args_json, cwd_mode,
  env_allowlist_json, output_mode, capabilities_json, settings_json,
  enabled, created_at, updated_at
) VALUES
  ('agent_config_codex_cli', 'Codex CLI', 'primary_reviewer', 'cli_noninteractive', 'codex', ?, 'repo_root', '[]', 'jsonl', '{}', '{}', 1, '2026-05-10T00:00:00Z', '2026-05-10T00:00:00Z'),
  ('agent_config_codex_app', 'Codex App Server', 'primary_reviewer', 'jsonrpc_stdio', 'codex', '["app-server","--listen","stdio://"]', 'repo_root', '[]', 'json', '{}', '{}', 1, '2026-05-10T00:00:00Z', '2026-05-10T00:00:00Z')
`, codexCLIArgs); err != nil {
		t.Fatalf("insert codex configs: %v", err)
	}
	if err := Apply(context.Background(), database, Migrations); err != nil {
		t.Fatalf("Apply(codex role migration) error = %v", err)
	}

	var codexCLIRole string
	if err := database.QueryRowContext(context.Background(), "SELECT role FROM agent_configs WHERE id = 'agent_config_codex_cli'").Scan(&codexCLIRole); err != nil {
		t.Fatalf("read codex cli role: %v", err)
	}
	if codexCLIRole != "orchestrator" {
		t.Fatalf("codex cli role = %q, want orchestrator", codexCLIRole)
	}
	var codexCLIUpdatedArgs string
	if err := database.QueryRowContext(context.Background(), "SELECT args_json FROM agent_configs WHERE id = 'agent_config_codex_cli'").Scan(&codexCLIUpdatedArgs); err != nil {
		t.Fatalf("read codex cli args: %v", err)
	}
	const codexCLIReadOnlyArgs = `["-a","never","exec","--json","--sandbox","read-only","--skip-git-repo-check","--ephemeral","--ignore-rules","--color","never","-"]`
	if codexCLIUpdatedArgs != codexCLIReadOnlyArgs {
		t.Fatalf("codex cli args = %s, want %s", codexCLIUpdatedArgs, codexCLIReadOnlyArgs)
	}
	var codexAppRole string
	if err := database.QueryRowContext(context.Background(), "SELECT role FROM agent_configs WHERE id = 'agent_config_codex_app'").Scan(&codexAppRole); err != nil {
		t.Fatalf("read codex app role: %v", err)
	}
	if codexAppRole != "primary_reviewer" {
		t.Fatalf("codex app role = %q, want primary_reviewer", codexAppRole)
	}
}

func TestApplyDeduplicatesFindingCandidatesBeforeUniqueIndex(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	const legacyFindingCandidatesSQL = `
CREATE TABLE finding_candidates (
  id TEXT PRIMARY KEY,
  agent_run_id TEXT NOT NULL,
  fingerprint TEXT
);
`
	legacyMigrations := []Migration{
		{Version: 1, Name: "schema_v1", SQL: legacyFindingCandidatesSQL},
		{Version: 2, Name: "noop_2", SQL: "SELECT 1;"},
		{Version: 3, Name: "noop_3", SQL: "SELECT 1;"},
		{Version: 4, Name: "noop_4", SQL: "SELECT 1;"},
		{Version: 5, Name: "noop_5", SQL: "SELECT 1;"},
		{Version: 6, Name: "noop_6", SQL: "SELECT 1;"},
		{Version: 7, Name: "noop_7", SQL: "SELECT 1;"},
		{Version: 8, Name: "noop_8", SQL: "SELECT 1;"},
		{Version: 9, Name: "noop_9", SQL: "SELECT 1;"},
	}
	if err := Apply(context.Background(), database, legacyMigrations); err != nil {
		t.Fatalf("Apply(legacy schema) error = %v", err)
	}
	if _, err := database.ExecContext(context.Background(), `
INSERT INTO finding_candidates(id, agent_run_id, fingerprint)
VALUES
  ('candidate_1', 'run_1', 'fp_1'),
  ('candidate_2', 'run_1', 'fp_1'),
  ('candidate_3', 'run_2', 'fp_1'),
  ('candidate_4', 'run_1', NULL)
`); err != nil {
		t.Fatalf("insert duplicate candidates: %v", err)
	}
	if err := Apply(context.Background(), database, Migrations); err != nil {
		t.Fatalf("Apply(current migrations) error = %v", err)
	}

	var count int
	if err := database.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM finding_candidates").Scan(&count); err != nil {
		t.Fatalf("count candidates: %v", err)
	}
	if count != 3 {
		t.Fatalf("candidate count = %d, want 3", count)
	}
	if _, err := database.ExecContext(
		context.Background(),
		"INSERT INTO finding_candidates(id, agent_run_id, fingerprint) VALUES ('candidate_5', 'run_1', 'fp_1')",
	); err == nil {
		t.Fatal("duplicate candidate insert error = nil, want unique constraint error")
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
