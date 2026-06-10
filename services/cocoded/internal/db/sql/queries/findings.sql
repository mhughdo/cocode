-- name: CreateFindingCandidate :one
INSERT INTO finding_candidates (
  id,
  review_session_id,
  agent_run_id,
  raw_artifact_id,
  category,
  severity,
  confidence,
  claim,
  primary_path,
  primary_start_line,
  primary_end_line,
  locations_json,
  evidence_json,
  suggested_fix,
  draft_comment,
  fingerprint,
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
  ?,
  ?
)
ON CONFLICT(agent_run_id, fingerprint) WHERE fingerprint IS NOT NULL AND fingerprint <> ''
DO UPDATE SET
  id = finding_candidates.id
RETURNING id, review_session_id, agent_run_id, raw_artifact_id, category, severity, confidence, claim, primary_path, primary_start_line, primary_end_line, locations_json, evidence_json, suggested_fix, draft_comment, fingerprint, created_at;

-- name: GetFindingCandidate :one
SELECT id, review_session_id, agent_run_id, raw_artifact_id, category, severity, confidence, claim, primary_path, primary_start_line, primary_end_line, locations_json, evidence_json, suggested_fix, draft_comment, fingerprint, created_at
FROM finding_candidates
WHERE id = ?
LIMIT 1;

-- name: ListFindingCandidatesBySession :many
SELECT id, review_session_id, agent_run_id, raw_artifact_id, category, severity, confidence, claim, primary_path, primary_start_line, primary_end_line, locations_json, evidence_json, suggested_fix, draft_comment, fingerprint, created_at
FROM finding_candidates
WHERE review_session_id = ?
ORDER BY created_at ASC, id ASC;

-- name: DeleteFindingCandidate :exec
DELETE FROM finding_candidates
WHERE id = ?;

-- name: CreateFinding :one
INSERT INTO findings (
  id,
  review_session_id,
  canonical_claim,
  category,
  severity,
  confidence,
  verification_status,
  decision_status,
  primary_path,
  primary_start_line,
  primary_end_line,
  evidence_summary,
  counter_evidence_summary,
  suggested_fix,
  draft_comment,
  fingerprint,
  merged_from_count,
  introduced_in_sha,
  first_seen_at,
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
ON CONFLICT(review_session_id, fingerprint)
DO UPDATE SET
  canonical_claim = excluded.canonical_claim,
  category = excluded.category,
  severity = excluded.severity,
  confidence = excluded.confidence,
  verification_status = excluded.verification_status,
  primary_path = excluded.primary_path,
  primary_start_line = excluded.primary_start_line,
  primary_end_line = excluded.primary_end_line,
  evidence_summary = excluded.evidence_summary,
  counter_evidence_summary = excluded.counter_evidence_summary,
  suggested_fix = excluded.suggested_fix,
  draft_comment = excluded.draft_comment,
  merged_from_count = excluded.merged_from_count,
  introduced_in_sha = excluded.introduced_in_sha,
  updated_at = excluded.updated_at
RETURNING id, review_session_id, canonical_claim, category, severity, confidence, verification_status, decision_status, primary_path, primary_start_line, primary_end_line, evidence_summary, counter_evidence_summary, suggested_fix, draft_comment, fingerprint, merged_from_count, introduced_in_sha, first_seen_at, updated_at;

-- name: GetFinding :one
SELECT id, review_session_id, canonical_claim, category, severity, confidence, verification_status, decision_status, primary_path, primary_start_line, primary_end_line, evidence_summary, counter_evidence_summary, suggested_fix, draft_comment, fingerprint, merged_from_count, introduced_in_sha, first_seen_at, updated_at
FROM findings
WHERE id = ?
LIMIT 1;

-- name: ListFindingsBySession :many
SELECT id, review_session_id, canonical_claim, category, severity, confidence, verification_status, decision_status, primary_path, primary_start_line, primary_end_line, evidence_summary, counter_evidence_summary, suggested_fix, draft_comment, fingerprint, merged_from_count, introduced_in_sha, first_seen_at, updated_at
FROM findings
WHERE review_session_id = ?
ORDER BY updated_at DESC, id ASC;

-- name: UpdateFinding :one
UPDATE findings
SET
  canonical_claim = ?,
  category = ?,
  severity = ?,
  confidence = ?,
  primary_path = ?,
  primary_start_line = ?,
  primary_end_line = ?,
  evidence_summary = ?,
  counter_evidence_summary = ?,
  suggested_fix = ?,
  draft_comment = ?,
  merged_from_count = ?,
  introduced_in_sha = ?,
  updated_at = ?
WHERE id = ?
RETURNING id, review_session_id, canonical_claim, category, severity, confidence, verification_status, decision_status, primary_path, primary_start_line, primary_end_line, evidence_summary, counter_evidence_summary, suggested_fix, draft_comment, fingerprint, merged_from_count, introduced_in_sha, first_seen_at, updated_at;

-- name: UpdateFindingVerificationStatus :one
UPDATE findings
SET verification_status = ?, updated_at = ?
WHERE id = ?
RETURNING id, review_session_id, canonical_claim, category, severity, confidence, verification_status, decision_status, primary_path, primary_start_line, primary_end_line, evidence_summary, counter_evidence_summary, suggested_fix, draft_comment, fingerprint, merged_from_count, introduced_in_sha, first_seen_at, updated_at;

-- name: UpdateFindingVerificationEvidence :one
UPDATE findings
SET
  verification_status = ?,
  evidence_summary = ?,
  counter_evidence_summary = ?,
  updated_at = ?
WHERE id = ?
RETURNING id, review_session_id, canonical_claim, category, severity, confidence, verification_status, decision_status, primary_path, primary_start_line, primary_end_line, evidence_summary, counter_evidence_summary, suggested_fix, draft_comment, fingerprint, merged_from_count, introduced_in_sha, first_seen_at, updated_at;

-- name: UpdateFindingDecisionStatus :one
UPDATE findings
SET decision_status = ?, updated_at = ?
WHERE id = ?
RETURNING id, review_session_id, canonical_claim, category, severity, confidence, verification_status, decision_status, primary_path, primary_start_line, primary_end_line, evidence_summary, counter_evidence_summary, suggested_fix, draft_comment, fingerprint, merged_from_count, introduced_in_sha, first_seen_at, updated_at;

-- name: UpdateFindingDraftComment :one
UPDATE findings
SET draft_comment = ?, updated_at = ?
WHERE id = ?
RETURNING id, review_session_id, canonical_claim, category, severity, confidence, verification_status, decision_status, primary_path, primary_start_line, primary_end_line, evidence_summary, counter_evidence_summary, suggested_fix, draft_comment, fingerprint, merged_from_count, introduced_in_sha, first_seen_at, updated_at;

-- name: DeleteFinding :exec
DELETE FROM findings
WHERE id = ?;

-- name: LinkFindingCandidate :exec
INSERT INTO finding_candidate_links (
  finding_id,
  finding_candidate_id,
  relation
) VALUES (
  ?,
  ?,
  ?
)
ON CONFLICT(finding_id, finding_candidate_id)
DO UPDATE SET relation = excluded.relation;

-- name: ListFindingCandidateLinks :many
SELECT finding_id, finding_candidate_id, relation
FROM finding_candidate_links
WHERE finding_id = ?
ORDER BY finding_candidate_id ASC;

-- name: CreateHumanDecision :one
INSERT INTO human_decisions (
  id,
  finding_id,
  review_session_id,
  decision,
  reason,
  metadata_json,
  created_at
) VALUES (
  ?,
  ?,
  ?,
  ?,
  ?,
  ?,
  ?
)
RETURNING id, finding_id, review_session_id, decision, reason, metadata_json, created_at;

-- name: ListHumanDecisionsByFinding :many
SELECT id, finding_id, review_session_id, decision, reason, metadata_json, created_at
FROM human_decisions
WHERE finding_id = ?
ORDER BY created_at DESC, id ASC;

-- name: ListHumanDecisionsBySession :many
SELECT id, finding_id, review_session_id, decision, reason, metadata_json, created_at
FROM human_decisions
WHERE review_session_id = ?
ORDER BY created_at DESC, id ASC;

-- name: ListDismissedHumanDecisionsByRepository :many
SELECT
  hd.id,
  hd.finding_id,
  hd.review_session_id,
  hd.decision,
  hd.reason,
  hd.metadata_json,
  hd.created_at,
  f.fingerprint AS finding_fingerprint
FROM human_decisions hd
JOIN findings f ON f.id = hd.finding_id
JOIN review_sessions rs ON rs.id = f.review_session_id
WHERE rs.repository_id = ?
  AND hd.decision = 'dismissed'
  AND f.fingerprint IS NOT NULL
  AND f.fingerprint <> ''
ORDER BY hd.created_at DESC, hd.id DESC;
