-- name: CreateReviewRule :one
INSERT INTO review_rules (
  id,
  workspace_id,
  scope,
  rule_type,
  content,
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
  ?
)
RETURNING id, workspace_id, scope, rule_type, content, enabled, created_at, updated_at;

-- name: GetReviewRule :one
SELECT id, workspace_id, scope, rule_type, content, enabled, created_at, updated_at
FROM review_rules
WHERE id = ?
LIMIT 1;

-- name: ListReviewRulesByWorkspace :many
SELECT id, workspace_id, scope, rule_type, content, enabled, created_at, updated_at
FROM review_rules
WHERE workspace_id = ?
ORDER BY enabled DESC, scope ASC, rule_type ASC, updated_at DESC, id ASC;

-- name: ListEnabledReviewRulesByWorkspace :many
SELECT id, workspace_id, scope, rule_type, content, enabled, created_at, updated_at
FROM review_rules
WHERE workspace_id = ? AND enabled = 1
ORDER BY scope ASC, rule_type ASC, updated_at DESC, id ASC;

-- name: UpdateReviewRule :one
UPDATE review_rules
SET
  scope = ?,
  rule_type = ?,
  content = ?,
  enabled = ?,
  updated_at = ?
WHERE id = ?
RETURNING id, workspace_id, scope, rule_type, content, enabled, created_at, updated_at;

-- name: SetReviewRuleEnabled :one
UPDATE review_rules
SET enabled = ?, updated_at = ?
WHERE id = ?
RETURNING id, workspace_id, scope, rule_type, content, enabled, created_at, updated_at;

-- name: DeleteReviewRule :exec
DELETE FROM review_rules
WHERE id = ?;
