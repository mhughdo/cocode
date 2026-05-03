-- name: CreateContextBundle :one
INSERT INTO context_bundles (
  id,
  review_session_id,
  agent_config_id,
  scope,
  token_estimate,
  item_count,
  artifact_id,
  policy_json,
  created_at
) VALUES (
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
RETURNING id, review_session_id, agent_config_id, scope, token_estimate, item_count, artifact_id, policy_json, created_at;

-- name: GetContextBundle :one
SELECT id, review_session_id, agent_config_id, scope, token_estimate, item_count, artifact_id, policy_json, created_at
FROM context_bundles
WHERE id = ?
LIMIT 1;

-- name: ListContextBundlesBySession :many
SELECT id, review_session_id, agent_config_id, scope, token_estimate, item_count, artifact_id, policy_json, created_at
FROM context_bundles
WHERE review_session_id = ?
ORDER BY created_at DESC, id ASC;

-- name: UpdateContextBundleArtifact :one
UPDATE context_bundles
SET
  artifact_id = ?,
  token_estimate = ?,
  item_count = ?
WHERE id = ?
RETURNING id, review_session_id, agent_config_id, scope, token_estimate, item_count, artifact_id, policy_json, created_at;

-- name: DeleteContextBundle :exec
DELETE FROM context_bundles
WHERE id = ?;

-- name: CreateContextItem :one
INSERT INTO context_items (
  id,
  context_bundle_id,
  kind,
  path,
  start_line,
  end_line,
  title,
  content_artifact_id,
  token_estimate,
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
  ?
)
RETURNING id, context_bundle_id, kind, path, start_line, end_line, title, content_artifact_id, token_estimate, metadata_json;

-- name: GetContextItem :one
SELECT id, context_bundle_id, kind, path, start_line, end_line, title, content_artifact_id, token_estimate, metadata_json
FROM context_items
WHERE id = ?
LIMIT 1;

-- name: ListContextItemsByBundle :many
SELECT id, context_bundle_id, kind, path, start_line, end_line, title, content_artifact_id, token_estimate, metadata_json
FROM context_items
WHERE context_bundle_id = ?
ORDER BY id ASC;

-- name: DeleteContextItem :exec
DELETE FROM context_items
WHERE id = ?;
