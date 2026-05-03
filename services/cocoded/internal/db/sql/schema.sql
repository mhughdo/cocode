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
