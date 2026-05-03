-- name: CreatePublishDraft :one
INSERT INTO publish_drafts (
  id,
  review_session_id,
  provider,
  status,
  review_event,
  body,
  comments_json,
  artifact_id,
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
  ?
)
RETURNING id, review_session_id, provider, status, review_event, body, comments_json, artifact_id, created_at, updated_at;

-- name: GetPublishDraft :one
SELECT id, review_session_id, provider, status, review_event, body, comments_json, artifact_id, created_at, updated_at
FROM publish_drafts
WHERE id = ?
LIMIT 1;

-- name: ListPublishDraftsBySession :many
SELECT id, review_session_id, provider, status, review_event, body, comments_json, artifact_id, created_at, updated_at
FROM publish_drafts
WHERE review_session_id = ?
ORDER BY created_at DESC, id ASC;
