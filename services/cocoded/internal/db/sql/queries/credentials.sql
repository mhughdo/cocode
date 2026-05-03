-- name: UpsertCredentialRef :one
INSERT INTO credential_refs (
  id,
  kind,
  display_name,
  storage_provider,
  storage_key,
  metadata_json,
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
ON CONFLICT(id) DO UPDATE SET
  kind = excluded.kind,
  display_name = excluded.display_name,
  storage_provider = excluded.storage_provider,
  storage_key = excluded.storage_key,
  metadata_json = excluded.metadata_json,
  updated_at = excluded.updated_at
RETURNING id, kind, display_name, storage_provider, storage_key, metadata_json, created_at, updated_at;

-- name: GetCredentialRef :one
SELECT id, kind, display_name, storage_provider, storage_key, metadata_json, created_at, updated_at
FROM credential_refs
WHERE id = ?
LIMIT 1;

-- name: GetLatestCredentialRefByKind :one
SELECT id, kind, display_name, storage_provider, storage_key, metadata_json, created_at, updated_at
FROM credential_refs
WHERE kind = ?
ORDER BY updated_at DESC, display_name ASC
LIMIT 1;

-- name: DeleteCredentialRef :exec
DELETE FROM credential_refs
WHERE id = ?;
