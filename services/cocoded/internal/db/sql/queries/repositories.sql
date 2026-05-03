-- name: CreateRepository :one
INSERT INTO repositories (
  id,
  workspace_id,
  name,
  owner,
  remote_url,
  local_path,
  default_branch,
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
  ?
)
RETURNING id, workspace_id, name, owner, remote_url, local_path, default_branch, created_at, updated_at;

-- name: GetRepository :one
SELECT id, workspace_id, name, owner, remote_url, local_path, default_branch, created_at, updated_at
FROM repositories
WHERE id = ?
LIMIT 1;

-- name: GetRepositoryByLocalPath :one
SELECT id, workspace_id, name, owner, remote_url, local_path, default_branch, created_at, updated_at
FROM repositories
WHERE workspace_id = ? AND local_path = ?
LIMIT 1;

-- name: ListRepositoriesByWorkspace :many
SELECT id, workspace_id, name, owner, remote_url, local_path, default_branch, created_at, updated_at
FROM repositories
WHERE workspace_id = ?
ORDER BY updated_at DESC, name ASC;

-- name: UpdateRepository :one
UPDATE repositories
SET
  name = ?,
  owner = ?,
  remote_url = ?,
  default_branch = ?,
  updated_at = ?
WHERE id = ?
RETURNING id, workspace_id, name, owner, remote_url, local_path, default_branch, created_at, updated_at;

-- name: DeleteRepository :exec
DELETE FROM repositories
WHERE id = ?;
