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

CREATE TABLE chat_threads (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('active','archived')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(review_session_id)
);

CREATE TABLE chat_messages (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
  parent_message_id TEXT REFERENCES chat_messages(id) ON DELETE SET NULL,
  author_type TEXT NOT NULL CHECK(author_type IN ('user','cocode','orchestrator','agent','system','verifier')),
  author_display_name TEXT NOT NULL,
  agent_config_id TEXT REFERENCES agent_configs(id) ON DELETE SET NULL,
  agent_run_id TEXT REFERENCES agent_runs(id) ON DELETE SET NULL,
  context_bundle_id TEXT REFERENCES context_bundles(id) ON DELETE SET NULL,
  artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  body TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('pending','streaming','completed','failed','canceled')),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE chat_message_context_refs (
  id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
  ref_type TEXT NOT NULL CHECK(ref_type IN ('review_session','finding','evidence_map','artifact','file','publish_draft','copy_packet','agent_run','context_bundle')),
  ref_id TEXT NOT NULL,
  label TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE chat_turns (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
  user_message_id TEXT NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
  mode TEXT NOT NULL,
  audience TEXT NOT NULL,
  responder_agent_config_id TEXT REFERENCES agent_configs(id) ON DELETE SET NULL,
  status TEXT NOT NULL CHECK(status IN ('created','routing','context_building','running','synthesizing','completed','failed','cancel_requested','canceled')),
  error_code TEXT,
  error_message TEXT,
  started_at TEXT,
  completed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE chat_turn_agent_runs (
  chat_turn_id TEXT NOT NULL REFERENCES chat_turns(id) ON DELETE CASCADE,
  agent_run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  PRIMARY KEY(chat_turn_id, agent_run_id)
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
  settings_override_json TEXT NOT NULL DEFAULT '{}'
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

CREATE TABLE events (
  id TEXT PRIMARY KEY,
  review_session_id TEXT REFERENCES review_sessions(id) ON DELETE CASCADE,
  agent_run_id TEXT,
  type TEXT NOT NULL,
  level TEXT NOT NULL DEFAULT 'info',
  sequence INTEGER NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  UNIQUE(review_session_id, sequence)
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

CREATE TABLE context_items (
  id TEXT PRIMARY KEY,
  context_bundle_id TEXT NOT NULL REFERENCES context_bundles(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  path TEXT,
  start_line INTEGER,
  end_line INTEGER,
  title TEXT,
  content_artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  token_estimate INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}'
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
CREATE INDEX idx_chat_threads_session ON chat_threads(review_session_id, updated_at DESC);
CREATE INDEX idx_chat_messages_thread_created ON chat_messages(thread_id, created_at, id);
CREATE INDEX idx_chat_message_context_refs_message ON chat_message_context_refs(message_id);
CREATE INDEX idx_chat_message_context_refs_ref ON chat_message_context_refs(ref_type, ref_id);
CREATE INDEX idx_chat_turns_thread_created ON chat_turns(thread_id, created_at);
CREATE INDEX idx_agent_runs_session ON agent_runs(review_session_id, status);
CREATE INDEX idx_events_session_sequence ON events(review_session_id, sequence);
CREATE INDEX idx_context_bundles_session ON context_bundles(review_session_id, scope, created_at DESC);
CREATE INDEX idx_context_items_bundle ON context_items(context_bundle_id, kind);

CREATE TABLE finding_candidates (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  agent_run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
  raw_artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  category TEXT NOT NULL,
  severity TEXT NOT NULL,
  confidence REAL NOT NULL DEFAULT 0.5,
  claim TEXT NOT NULL,
  primary_path TEXT,
  primary_start_line INTEGER,
  primary_end_line INTEGER,
  locations_json TEXT NOT NULL DEFAULT '[]',
  evidence_json TEXT NOT NULL DEFAULT '[]',
  suggested_fix TEXT,
  draft_comment TEXT,
  fingerprint TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE findings (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  canonical_claim TEXT NOT NULL,
  category TEXT NOT NULL,
  severity TEXT NOT NULL,
  confidence REAL NOT NULL DEFAULT 0.5,
  verification_status TEXT NOT NULL DEFAULT 'unverified',
  decision_status TEXT NOT NULL DEFAULT 'undecided',
  primary_path TEXT,
  primary_start_line INTEGER,
  primary_end_line INTEGER,
  evidence_summary TEXT,
  counter_evidence_summary TEXT,
  suggested_fix TEXT,
  draft_comment TEXT,
  fingerprint TEXT NOT NULL,
  merged_from_count INTEGER NOT NULL DEFAULT 1,
  introduced_in_sha TEXT,
  first_seen_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(review_session_id, fingerprint)
);

CREATE TABLE finding_candidate_links (
  finding_id TEXT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
  finding_candidate_id TEXT NOT NULL REFERENCES finding_candidates(id) ON DELETE CASCADE,
  relation TEXT NOT NULL DEFAULT 'merged',
  PRIMARY KEY(finding_id, finding_candidate_id)
);

CREATE TABLE finding_threads (
  id TEXT PRIMARY KEY,
  finding_id TEXT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(finding_id)
);

CREATE TABLE finding_thread_messages (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL REFERENCES finding_threads(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK(role IN ('user','assistant','system','agent')),
  agent_config_id TEXT REFERENCES agent_configs(id) ON DELETE SET NULL,
  content TEXT NOT NULL,
  evidence_refs_json TEXT NOT NULL DEFAULT '[]',
  artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX idx_finding_threads_session ON finding_threads(review_session_id, updated_at DESC);
CREATE INDEX idx_finding_thread_messages_thread ON finding_thread_messages(thread_id, created_at ASC);

CREATE TABLE human_decisions (
  id TEXT PRIMARY KEY,
  finding_id TEXT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  decision TEXT NOT NULL CHECK(decision IN ('accepted','dismissed','deferred','copied','published','edited')),
  reason TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE INDEX idx_candidates_session ON finding_candidates(review_session_id);
CREATE INDEX idx_candidates_fingerprint ON finding_candidates(review_session_id, fingerprint);
CREATE INDEX idx_findings_session_status ON findings(review_session_id, decision_status, verification_status);
CREATE INDEX idx_findings_path ON findings(review_session_id, primary_path);
CREATE INDEX idx_decisions_finding ON human_decisions(finding_id, created_at DESC);

CREATE TABLE copy_packets (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  finding_id TEXT REFERENCES findings(id) ON DELETE CASCADE,
  format TEXT NOT NULL CHECK(format IN ('markdown','xmlish','json','compact','github_summary')),
  content_artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
  finding_count INTEGER NOT NULL,
  token_estimate INTEGER NOT NULL DEFAULT 0,
  copied_at TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX idx_copy_packets_session ON copy_packets(review_session_id, created_at DESC);

CREATE TABLE evidence_items (
  id TEXT PRIMARY KEY,
  finding_id TEXT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK(kind IN ('supporting','counter','neutral','missing','test','search','agent','static_analysis')),
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  path TEXT,
  start_line INTEGER,
  end_line INTEGER,
  artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  confidence REAL NOT NULL DEFAULT 0.5,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE evidence_graphs (
  id TEXT PRIMARY KEY,
  finding_id TEXT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'ready',
  layout_json TEXT NOT NULL DEFAULT '{}',
  summary TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(finding_id)
);

CREATE TABLE evidence_nodes (
  id TEXT PRIMARY KEY,
  evidence_graph_id TEXT NOT NULL REFERENCES evidence_graphs(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK(kind IN ('changed_code','entrypoint','route','related_code','middleware','guard','handler','test','config','counter_evidence','missing_guard','unknown')),
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

CREATE TABLE call_paths (
  id TEXT PRIMARY KEY,
  evidence_graph_id TEXT NOT NULL REFERENCES evidence_graphs(id) ON DELETE CASCADE,
  label TEXT,
  confidence REAL NOT NULL DEFAULT 0.5,
  created_at TEXT NOT NULL
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

CREATE INDEX idx_evidence_finding ON evidence_items(finding_id, kind);
CREATE INDEX idx_evidence_nodes_graph ON evidence_nodes(evidence_graph_id, kind);
CREATE INDEX idx_evidence_edges_graph ON evidence_edges(evidence_graph_id, kind);

CREATE TABLE publish_drafts (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  provider TEXT NOT NULL DEFAULT 'github',
  status TEXT NOT NULL DEFAULT 'draft',
  review_event TEXT CHECK(review_event IN ('COMMENT','REQUEST_CHANGES','APPROVE')),
  body TEXT,
  comments_json TEXT NOT NULL DEFAULT '[]',
  artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE github_publications (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  publish_draft_id TEXT REFERENCES publish_drafts(id) ON DELETE SET NULL,
  github_review_id TEXT,
  github_comment_ids_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL,
  error_message TEXT,
  created_at TEXT NOT NULL
);

CREATE VIRTUAL TABLE finding_search USING fts5(
  finding_id UNINDEXED,
  claim,
  evidence_summary,
  suggested_fix,
  draft_comment
);

CREATE VIRTUAL TABLE evidence_search USING fts5(
  evidence_item_id UNINDEXED,
  title,
  summary,
  path
);

CREATE TABLE credential_refs (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  display_name TEXT NOT NULL,
  storage_provider TEXT NOT NULL,
  storage_key TEXT NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE review_rules (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  scope TEXT NOT NULL,
  rule_type TEXT NOT NULL,
  content TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
