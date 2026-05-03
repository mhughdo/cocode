-- name: CreateGitHubPublication :one
INSERT INTO github_publications (
  id,
  review_session_id,
  publish_draft_id,
  github_review_id,
  github_comment_ids_json,
  status,
  error_message,
  created_at
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
RETURNING id, review_session_id, publish_draft_id, github_review_id, github_comment_ids_json, status, error_message, created_at;

-- name: GetGitHubPublication :one
SELECT id, review_session_id, publish_draft_id, github_review_id, github_comment_ids_json, status, error_message, created_at
FROM github_publications
WHERE id = ?
LIMIT 1;

-- name: ListGitHubPublicationsBySession :many
SELECT id, review_session_id, publish_draft_id, github_review_id, github_comment_ids_json, status, error_message, created_at
FROM github_publications
WHERE review_session_id = ?
ORDER BY created_at DESC, id ASC;
