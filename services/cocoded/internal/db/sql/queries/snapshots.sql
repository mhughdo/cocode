-- name: CreatePullRequestSnapshot :one
INSERT INTO pull_request_snapshots (
  id,
  repository_id,
  source_type,
  provider,
  owner,
  repo,
  pr_number,
  pr_title,
  pr_url,
  base_ref,
  head_ref,
  base_sha,
  head_sha,
  diff_artifact_id,
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
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?
)
RETURNING id, repository_id, source_type, provider, owner, repo, pr_number, pr_title, pr_url, base_ref, head_ref, base_sha, head_sha, diff_artifact_id, metadata_json, created_at;

-- name: GetPullRequestSnapshot :one
SELECT id, repository_id, source_type, provider, owner, repo, pr_number, pr_title, pr_url, base_ref, head_ref, base_sha, head_sha, diff_artifact_id, metadata_json, created_at
FROM pull_request_snapshots
WHERE id = ?
LIMIT 1;

-- name: ListPullRequestSnapshotsByRepository :many
SELECT id, repository_id, source_type, provider, owner, repo, pr_number, pr_title, pr_url, base_ref, head_ref, base_sha, head_sha, diff_artifact_id, metadata_json, created_at
FROM pull_request_snapshots
WHERE repository_id = ?
ORDER BY created_at DESC;

-- name: DeletePullRequestSnapshot :exec
DELETE FROM pull_request_snapshots
WHERE id = ?;

-- name: CreateChangedFile :one
INSERT INTO changed_files (
  id,
  snapshot_id,
  path,
  old_path,
  status,
  additions,
  deletions,
  is_binary,
  is_generated,
  is_excluded,
  line_ranges_json,
  patch_artifact_id,
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
  ?,
  ?,
  ?,
  ?
)
RETURNING id, snapshot_id, path, old_path, status, additions, deletions, is_binary, is_generated, is_excluded, line_ranges_json, patch_artifact_id, created_at;

-- name: GetChangedFile :one
SELECT id, snapshot_id, path, old_path, status, additions, deletions, is_binary, is_generated, is_excluded, line_ranges_json, patch_artifact_id, created_at
FROM changed_files
WHERE id = ?
LIMIT 1;

-- name: GetChangedFileByPath :one
SELECT id, snapshot_id, path, old_path, status, additions, deletions, is_binary, is_generated, is_excluded, line_ranges_json, patch_artifact_id, created_at
FROM changed_files
WHERE snapshot_id = ? AND path = ?
LIMIT 1;

-- name: ListChangedFilesBySnapshot :many
SELECT id, snapshot_id, path, old_path, status, additions, deletions, is_binary, is_generated, is_excluded, line_ranges_json, patch_artifact_id, created_at
FROM changed_files
WHERE snapshot_id = ?
ORDER BY path ASC;

-- name: UpdateChangedFileExclusion :one
UPDATE changed_files
SET is_excluded = ?
WHERE id = ?
RETURNING id, snapshot_id, path, old_path, status, additions, deletions, is_binary, is_generated, is_excluded, line_ranges_json, patch_artifact_id, created_at;
