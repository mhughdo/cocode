-- name: InsertFindingSearch :exec
INSERT INTO finding_search (
  finding_id,
  claim,
  evidence_summary,
  suggested_fix,
  draft_comment
) VALUES (
  ?,
  ?,
  ?,
  ?,
  ?
);

-- name: DeleteFindingSearch :exec
DELETE FROM finding_search
WHERE finding_id = ?;

-- name: SearchFindings :many
SELECT finding_id
FROM finding_search
WHERE claim MATCH ?
  OR evidence_summary MATCH ?
  OR suggested_fix MATCH ?
  OR draft_comment MATCH ?
LIMIT ?;

-- name: InsertEvidenceSearch :exec
INSERT INTO evidence_search (
  evidence_item_id,
  title,
  summary,
  path
) VALUES (
  ?,
  ?,
  ?,
  ?
);

-- name: DeleteEvidenceSearch :exec
DELETE FROM evidence_search
WHERE evidence_item_id = ?;

-- name: SearchEvidence :many
SELECT evidence_item_id
FROM evidence_search
WHERE title MATCH ?
  OR summary MATCH ?
  OR path MATCH ?
LIMIT ?;
