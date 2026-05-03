-- name: CreateReviewSession :one
INSERT INTO review_sessions (
  id,
  workspace_id,
  repository_id,
  snapshot_id,
  title,
  status,
  review_depth,
  focus_prompt,
  preset,
  runtime_limit_seconds,
  context_policy_json,
  started_at,
  completed_at,
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
  ?
)
RETURNING id, workspace_id, repository_id, snapshot_id, title, status, review_depth, focus_prompt, preset, runtime_limit_seconds, context_policy_json, started_at, completed_at, created_at, updated_at;

-- name: GetReviewSession :one
SELECT id, workspace_id, repository_id, snapshot_id, title, status, review_depth, focus_prompt, preset, runtime_limit_seconds, context_policy_json, started_at, completed_at, created_at, updated_at
FROM review_sessions
WHERE id = ?
LIMIT 1;

-- name: ListReviewSessionsByWorkspace :many
SELECT id, workspace_id, repository_id, snapshot_id, title, status, review_depth, focus_prompt, preset, runtime_limit_seconds, context_policy_json, started_at, completed_at, created_at, updated_at
FROM review_sessions
WHERE workspace_id = ?
ORDER BY created_at DESC;

-- name: UpdateReviewSession :one
UPDATE review_sessions
SET
  title = ?,
  review_depth = ?,
  focus_prompt = ?,
  preset = ?,
  runtime_limit_seconds = ?,
  context_policy_json = ?,
  updated_at = ?
WHERE id = ?
RETURNING id, workspace_id, repository_id, snapshot_id, title, status, review_depth, focus_prompt, preset, runtime_limit_seconds, context_policy_json, started_at, completed_at, created_at, updated_at;

-- name: UpdateReviewSessionStatus :one
UPDATE review_sessions
SET
  status = ?,
  started_at = ?,
  completed_at = ?,
  updated_at = ?
WHERE id = ?
RETURNING id, workspace_id, repository_id, snapshot_id, title, status, review_depth, focus_prompt, preset, runtime_limit_seconds, context_policy_json, started_at, completed_at, created_at, updated_at;

-- name: UpdateReviewSessionStatusIfCurrent :one
UPDATE review_sessions
SET
  status = ?,
  started_at = ?,
  completed_at = ?,
  updated_at = ?
WHERE id = ? AND status = ?
RETURNING id, workspace_id, repository_id, snapshot_id, title, status, review_depth, focus_prompt, preset, runtime_limit_seconds, context_policy_json, started_at, completed_at, created_at, updated_at;

-- name: DeleteReviewSession :exec
DELETE FROM review_sessions
WHERE id = ?;
