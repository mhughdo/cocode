-- name: NextEventSequence :one
SELECT COALESCE(MAX(sequence), 0) + 1
FROM events
WHERE review_session_id = ?;

-- name: CreateEvent :one
INSERT INTO events (
  id,
  review_session_id,
  agent_run_id,
  type,
  level,
  sequence,
  payload_json,
  artifact_id,
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
RETURNING id, review_session_id, agent_run_id, type, level, sequence, payload_json, artifact_id, created_at;

-- name: GetEvent :one
SELECT id, review_session_id, agent_run_id, type, level, sequence, payload_json, artifact_id, created_at
FROM events
WHERE id = ?
LIMIT 1;

-- name: ListEventsByReviewSession :many
SELECT id, review_session_id, agent_run_id, type, level, sequence, payload_json, artifact_id, created_at
FROM events
WHERE review_session_id = ?
ORDER BY sequence ASC;
