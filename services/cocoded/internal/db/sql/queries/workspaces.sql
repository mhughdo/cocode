-- name: CreateWorkspace :one
INSERT INTO workspaces (
  id,
  name,
  root_path,
  default_repo_id,
  settings_json,
  created_at,
  updated_at
) VALUES (
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?
)
RETURNING id, name, root_path, default_repo_id, settings_json, created_at, updated_at;

-- name: GetWorkspace :one
SELECT id, name, root_path, default_repo_id, settings_json, created_at, updated_at
FROM workspaces
WHERE id = ?
LIMIT 1;

-- name: GetWorkspaceByRootPath :one
SELECT id, name, root_path, default_repo_id, settings_json, created_at, updated_at
FROM workspaces
WHERE root_path = ?
LIMIT 1;

-- name: ListWorkspaces :many
SELECT id, name, root_path, default_repo_id, settings_json, created_at, updated_at
FROM workspaces
ORDER BY updated_at DESC, name ASC;

-- name: UpdateWorkspace :one
UPDATE workspaces
SET
  name = ?,
  default_repo_id = ?,
  settings_json = ?,
  updated_at = ?
WHERE id = ?
RETURNING id, name, root_path, default_repo_id, settings_json, created_at, updated_at;

-- name: DeleteWorkspace :exec
DELETE FROM workspaces
WHERE id = ?;
