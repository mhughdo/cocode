-- name: CreateCopyPacket :one
INSERT INTO copy_packets (
  id,
  review_session_id,
  finding_id,
  format,
  content_artifact_id,
  finding_count,
  token_estimate,
  copied_at,
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
RETURNING id, review_session_id, finding_id, format, content_artifact_id, finding_count, token_estimate, copied_at, created_at;

-- name: GetCopyPacket :one
SELECT id, review_session_id, finding_id, format, content_artifact_id, finding_count, token_estimate, copied_at, created_at
FROM copy_packets
WHERE id = ?
LIMIT 1;

-- name: ListCopyPacketsBySession :many
SELECT id, review_session_id, finding_id, format, content_artifact_id, finding_count, token_estimate, copied_at, created_at
FROM copy_packets
WHERE review_session_id = ?
ORDER BY created_at DESC, id ASC;

-- name: MarkCopyPacketCopied :one
UPDATE copy_packets
SET copied_at = ?
WHERE id = ?
RETURNING id, review_session_id, finding_id, format, content_artifact_id, finding_count, token_estimate, copied_at, created_at;
