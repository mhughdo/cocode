-- name: CreateArtifact :one
INSERT INTO artifacts (
  id,
  workspace_id,
  review_session_id,
  kind,
  relative_path,
  content_type,
  size_bytes,
  sha256,
  metadata_json,
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
  ?,
  ?
)
RETURNING id, workspace_id, review_session_id, kind, relative_path, content_type, size_bytes, sha256, metadata_json, created_at;

-- name: GetArtifact :one
SELECT id, workspace_id, review_session_id, kind, relative_path, content_type, size_bytes, sha256, metadata_json, created_at
FROM artifacts
WHERE id = ?
LIMIT 1;

-- name: ListArtifactsByWorkspace :many
SELECT id, workspace_id, review_session_id, kind, relative_path, content_type, size_bytes, sha256, metadata_json, created_at
FROM artifacts
WHERE workspace_id = ?
ORDER BY created_at DESC, id ASC;

-- name: ListArtifactsByReviewSession :many
SELECT id, workspace_id, review_session_id, kind, relative_path, content_type, size_bytes, sha256, metadata_json, created_at
FROM artifacts
WHERE review_session_id = ?
ORDER BY created_at DESC, id ASC;

-- name: DeleteArtifact :exec
DELETE FROM artifacts
WHERE id = ?;
