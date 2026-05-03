CREATE TABLE workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  root_path TEXT NOT NULL UNIQUE,
  default_repo_id TEXT,
  settings_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE repositories (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  owner TEXT,
  remote_url TEXT,
  local_path TEXT NOT NULL,
  default_branch TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(workspace_id, local_path)
);

CREATE TABLE pull_request_snapshots (
  id TEXT PRIMARY KEY,
  repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
  source_type TEXT NOT NULL CHECK(source_type IN ('github_pr','branch_compare','commit_compare','local_changes')),
  provider TEXT,
  owner TEXT,
  repo TEXT,
  pr_number INTEGER,
  pr_title TEXT,
  pr_url TEXT,
  base_ref TEXT,
  head_ref TEXT,
  base_sha TEXT,
  head_sha TEXT,
  diff_artifact_id TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE changed_files (
  id TEXT PRIMARY KEY,
  snapshot_id TEXT NOT NULL REFERENCES pull_request_snapshots(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  old_path TEXT,
  status TEXT NOT NULL,
  additions INTEGER NOT NULL DEFAULT 0,
  deletions INTEGER NOT NULL DEFAULT 0,
  is_binary INTEGER NOT NULL DEFAULT 0,
  is_generated INTEGER NOT NULL DEFAULT 0,
  is_excluded INTEGER NOT NULL DEFAULT 0,
  line_ranges_json TEXT NOT NULL DEFAULT '[]',
  patch_artifact_id TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX idx_changed_files_snapshot ON changed_files(snapshot_id);

CREATE TABLE review_sessions (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
  snapshot_id TEXT NOT NULL REFERENCES pull_request_snapshots(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  review_depth TEXT NOT NULL DEFAULT 'standard',
  focus_prompt TEXT,
  preset TEXT,
  runtime_limit_seconds INTEGER NOT NULL DEFAULT 1800,
  context_policy_json TEXT NOT NULL DEFAULT '{}',
  started_at TEXT,
  completed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE agent_configs (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  role TEXT NOT NULL,
  adapter_kind TEXT NOT NULL CHECK(adapter_kind IN ('cli_noninteractive','jsonrpc_stdio','acp_stdio','mcp','a2a','provider_api','local_verifier')),
  command TEXT,
  args_json TEXT NOT NULL DEFAULT '[]',
  cwd_mode TEXT NOT NULL DEFAULT 'repo_root',
  env_allowlist_json TEXT NOT NULL DEFAULT '[]',
  output_mode TEXT NOT NULL DEFAULT 'text',
  model_label TEXT,
  reasoning_label TEXT,
  capabilities_json TEXT NOT NULL DEFAULT '{}',
  settings_json TEXT NOT NULL DEFAULT '{}',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
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
);

CREATE TABLE artifacts (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  review_session_id TEXT REFERENCES review_sessions(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  relative_path TEXT NOT NULL,
  content_type TEXT NOT NULL DEFAULT 'text/plain',
  size_bytes INTEGER NOT NULL DEFAULT 0,
  sha256 TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE context_bundles (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  agent_config_id TEXT REFERENCES agent_configs(id) ON DELETE SET NULL,
  scope TEXT NOT NULL CHECK(scope IN ('review','finding','evidence_map','follow_up')),
  token_estimate INTEGER NOT NULL DEFAULT 0,
  item_count INTEGER NOT NULL DEFAULT 0,
  artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  policy_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE agent_runs (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  agent_config_id TEXT NOT NULL REFERENCES agent_configs(id) ON DELETE RESTRICT,
  context_bundle_id TEXT REFERENCES context_bundles(id) ON DELETE SET NULL,
  status TEXT NOT NULL,
  role TEXT NOT NULL,
  started_at TEXT,
  completed_at TEXT,
  duration_ms INTEGER,
  exit_code INTEGER,
  stdout_artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  stderr_artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  parsed_output_artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  error_code TEXT,
  error_message TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_review_sessions_workspace ON review_sessions(workspace_id, created_at DESC);
CREATE INDEX idx_agent_runs_session ON agent_runs(review_session_id, status);
