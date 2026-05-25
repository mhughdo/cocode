-- name: CreateAgentConfig :one
INSERT INTO agent_configs (
  id,
  name,
  role,
  adapter_kind,
  command,
  args_json,
  cwd_mode,
  env_allowlist_json,
  output_mode,
  model_label,
  reasoning_label,
  capabilities_json,
  settings_json,
  enabled,
  created_at,
  updated_at
) VALUES (
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?
)
RETURNING id, name, role, adapter_kind, command, args_json, cwd_mode, env_allowlist_json, output_mode, model_label, reasoning_label, capabilities_json, settings_json, enabled, created_at, updated_at;

-- name: GetAgentConfig :one
SELECT id, name, role, adapter_kind, command, args_json, cwd_mode, env_allowlist_json, output_mode, model_label, reasoning_label, capabilities_json, settings_json, enabled, created_at, updated_at
FROM agent_configs
WHERE id = ?
LIMIT 1;

-- name: ListAgentConfigs :many
SELECT id, name, role, adapter_kind, command, args_json, cwd_mode, env_allowlist_json, output_mode, model_label, reasoning_label, capabilities_json, settings_json, enabled, created_at, updated_at
FROM agent_configs
ORDER BY enabled DESC, name ASC;

-- name: UpdateAgentConfig :one
UPDATE agent_configs
SET
  name = ?,
  role = ?,
  command = ?,
  args_json = ?,
  cwd_mode = ?,
  env_allowlist_json = ?,
  output_mode = ?,
  model_label = ?,
  reasoning_label = ?,
  capabilities_json = ?,
  settings_json = ?,
  enabled = ?,
  updated_at = ?
WHERE id = ?
RETURNING id, name, role, adapter_kind, command, args_json, cwd_mode, env_allowlist_json, output_mode, model_label, reasoning_label, capabilities_json, settings_json, enabled, created_at, updated_at;

-- name: DeleteAgentConfig :exec
DELETE FROM agent_configs
WHERE id = ?;

-- name: CreateReviewSessionAgent :one
INSERT INTO review_session_agents (
  id,
  review_session_id,
  agent_config_id,
  role,
  run_order,
  enabled,
  settings_override_json
) VALUES (
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?
)
RETURNING id, review_session_id, agent_config_id, role, run_order, enabled, settings_override_json;

-- name: ListReviewSessionAgents :many
SELECT id, review_session_id, agent_config_id, role, run_order, enabled, settings_override_json
FROM review_session_agents
WHERE review_session_id = ?
ORDER BY run_order ASC, role ASC;

-- name: UpdateReviewSessionAgentEnabled :one
UPDATE review_session_agents
SET enabled = ?
WHERE id = ?
RETURNING id, review_session_id, agent_config_id, role, run_order, enabled, settings_override_json;

-- name: CreateAgentRun :one
INSERT INTO agent_runs (
  id,
  review_session_id,
  agent_config_id,
  context_bundle_id,
  status,
  role,
  started_at,
  completed_at,
  duration_ms,
  exit_code,
  stdout_artifact_id,
  stderr_artifact_id,
  parsed_output_artifact_id,
  error_code,
  error_message,
  metadata_json
) VALUES (
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?
)
RETURNING id, review_session_id, agent_config_id, context_bundle_id, status, role, started_at, completed_at, duration_ms, exit_code, stdout_artifact_id, stderr_artifact_id, parsed_output_artifact_id, error_code, error_message, metadata_json;

-- name: GetAgentRun :one
SELECT id, review_session_id, agent_config_id, context_bundle_id, status, role, started_at, completed_at, duration_ms, exit_code, stdout_artifact_id, stderr_artifact_id, parsed_output_artifact_id, error_code, error_message, metadata_json
FROM agent_runs
WHERE id = ?
LIMIT 1;

-- name: ListAgentRunsBySession :many
SELECT id, review_session_id, agent_config_id, context_bundle_id, status, role, started_at, completed_at, duration_ms, exit_code, stdout_artifact_id, stderr_artifact_id, parsed_output_artifact_id, error_code, error_message, metadata_json
FROM agent_runs
WHERE review_session_id = ?
ORDER BY started_at ASC, id ASC;

-- name: ListInterruptedAgentRuns :many
SELECT id, review_session_id, agent_config_id, context_bundle_id, status, role, started_at, completed_at, duration_ms, exit_code, stdout_artifact_id, stderr_artifact_id, parsed_output_artifact_id, error_code, error_message, metadata_json
FROM agent_runs
WHERE status IN ('queued', 'running')
ORDER BY review_session_id ASC, started_at ASC, id ASC;

-- name: UpdateAgentRunStatus :one
UPDATE agent_runs
SET
  status = ?,
  started_at = ?,
  completed_at = ?,
  duration_ms = ?,
  exit_code = ?,
  stdout_artifact_id = ?,
  stderr_artifact_id = ?,
  parsed_output_artifact_id = ?,
  error_code = ?,
  error_message = ?,
  metadata_json = ?
WHERE id = ?
RETURNING id, review_session_id, agent_config_id, context_bundle_id, status, role, started_at, completed_at, duration_ms, exit_code, stdout_artifact_id, stderr_artifact_id, parsed_output_artifact_id, error_code, error_message, metadata_json;
