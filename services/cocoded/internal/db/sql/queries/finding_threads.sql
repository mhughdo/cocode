-- name: UpsertFindingThreadByFinding :one
INSERT INTO finding_threads (
  id,
  finding_id,
  review_session_id,
  title,
  created_at,
  updated_at
) VALUES (
  ?,
  ?,
  ?,
  ?,
  ?,
  ?
)
ON CONFLICT(finding_id) DO UPDATE SET
  updated_at = finding_threads.updated_at
RETURNING id, finding_id, review_session_id, title, created_at, updated_at;

-- name: GetFindingThread :one
SELECT id, finding_id, review_session_id, title, created_at, updated_at
FROM finding_threads
WHERE id = ?
LIMIT 1;

-- name: GetFindingThreadByFinding :one
SELECT id, finding_id, review_session_id, title, created_at, updated_at
FROM finding_threads
WHERE finding_id = ?
LIMIT 1;

-- name: ListFindingThreadsBySession :many
SELECT id, finding_id, review_session_id, title, created_at, updated_at
FROM finding_threads
WHERE review_session_id = ?
ORDER BY updated_at DESC, id ASC;

-- name: UpdateFindingThread :one
UPDATE finding_threads
SET title = ?, updated_at = ?
WHERE id = ?
RETURNING id, finding_id, review_session_id, title, created_at, updated_at;

-- name: CreateFindingThreadMessage :one
INSERT INTO finding_thread_messages (
  id,
  thread_id,
  role,
  agent_config_id,
  content,
  evidence_refs_json,
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
  ?
)
RETURNING id, thread_id, role, agent_config_id, content, evidence_refs_json, artifact_id, created_at;

-- name: ListFindingThreadMessages :many
SELECT id, thread_id, role, agent_config_id, content, evidence_refs_json, artifact_id, created_at
FROM finding_thread_messages
WHERE thread_id = ?
ORDER BY created_at ASC, id ASC;

-- name: GetFindingThreadMessage :one
SELECT id, thread_id, role, agent_config_id, content, evidence_refs_json, artifact_id, created_at
FROM finding_thread_messages
WHERE id = ?
LIMIT 1;
