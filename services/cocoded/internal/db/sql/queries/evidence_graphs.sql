-- name: CreateEvidenceItem :one
INSERT INTO evidence_items (
  id,
  finding_id,
  kind,
  title,
  summary,
  path,
  start_line,
  end_line,
  artifact_id,
  confidence,
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
  ?
)
RETURNING id, finding_id, kind, title, summary, path, start_line, end_line, artifact_id, confidence, metadata_json, created_at;

-- name: GetEvidenceItem :one
SELECT id, finding_id, kind, title, summary, path, start_line, end_line, artifact_id, confidence, metadata_json, created_at
FROM evidence_items
WHERE id = ?
LIMIT 1;

-- name: ListEvidenceItemsByFinding :many
SELECT id, finding_id, kind, title, summary, path, start_line, end_line, artifact_id, confidence, metadata_json, created_at
FROM evidence_items
WHERE finding_id = ?
ORDER BY created_at ASC, id ASC;

-- name: UpdateEvidenceItem :one
UPDATE evidence_items
SET
  kind = ?,
  title = ?,
  summary = ?,
  path = ?,
  start_line = ?,
  end_line = ?,
  artifact_id = ?,
  confidence = ?,
  metadata_json = ?
WHERE id = ?
RETURNING id, finding_id, kind, title, summary, path, start_line, end_line, artifact_id, confidence, metadata_json, created_at;

-- name: DeleteEvidenceItem :exec
DELETE FROM evidence_items
WHERE id = ?;

-- name: CreateEvidenceGraph :one
INSERT INTO evidence_graphs (
  id,
  finding_id,
  review_session_id,
  status,
  layout_json,
  summary,
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
RETURNING id, finding_id, review_session_id, status, layout_json, summary, created_at, updated_at;

-- name: GetEvidenceGraph :one
SELECT id, finding_id, review_session_id, status, layout_json, summary, created_at, updated_at
FROM evidence_graphs
WHERE id = ?
LIMIT 1;

-- name: GetEvidenceGraphByFinding :one
SELECT id, finding_id, review_session_id, status, layout_json, summary, created_at, updated_at
FROM evidence_graphs
WHERE finding_id = ?
LIMIT 1;

-- name: UpdateEvidenceGraph :one
UPDATE evidence_graphs
SET
  status = ?,
  layout_json = ?,
  summary = ?,
  updated_at = ?
WHERE id = ?
RETURNING id, finding_id, review_session_id, status, layout_json, summary, created_at, updated_at;

-- name: DeleteEvidenceGraph :exec
DELETE FROM evidence_graphs
WHERE id = ?;

-- name: CreateEvidenceNode :one
INSERT INTO evidence_nodes (
  id,
  evidence_graph_id,
  kind,
  label,
  path,
  symbol,
  start_line,
  end_line,
  evidence_item_id,
  confidence,
  metadata_json
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
  ?
)
RETURNING id, evidence_graph_id, kind, label, path, symbol, start_line, end_line, evidence_item_id, confidence, metadata_json;

-- name: GetEvidenceNode :one
SELECT id, evidence_graph_id, kind, label, path, symbol, start_line, end_line, evidence_item_id, confidence, metadata_json
FROM evidence_nodes
WHERE id = ?
LIMIT 1;

-- name: ListEvidenceNodesByGraph :many
SELECT id, evidence_graph_id, kind, label, path, symbol, start_line, end_line, evidence_item_id, confidence, metadata_json
FROM evidence_nodes
WHERE evidence_graph_id = ?
ORDER BY kind ASC, label ASC;

-- name: UpdateEvidenceNode :one
UPDATE evidence_nodes
SET
  kind = ?,
  label = ?,
  path = ?,
  symbol = ?,
  start_line = ?,
  end_line = ?,
  evidence_item_id = ?,
  confidence = ?,
  metadata_json = ?
WHERE id = ?
RETURNING id, evidence_graph_id, kind, label, path, symbol, start_line, end_line, evidence_item_id, confidence, metadata_json;

-- name: DeleteEvidenceNode :exec
DELETE FROM evidence_nodes
WHERE id = ?;

-- name: CreateEvidenceEdge :one
INSERT INTO evidence_edges (
  id,
  evidence_graph_id,
  source_node_id,
  target_node_id,
  kind,
  status,
  label,
  confidence,
  metadata_json
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
RETURNING id, evidence_graph_id, source_node_id, target_node_id, kind, status, label, confidence, metadata_json;

-- name: ListEvidenceEdgesByGraph :many
SELECT id, evidence_graph_id, source_node_id, target_node_id, kind, status, label, confidence, metadata_json
FROM evidence_edges
WHERE evidence_graph_id = ?
ORDER BY kind ASC, id ASC;

-- name: UpdateEvidenceEdge :one
UPDATE evidence_edges
SET
  kind = ?,
  status = ?,
  label = ?,
  confidence = ?,
  metadata_json = ?
WHERE id = ?
RETURNING id, evidence_graph_id, source_node_id, target_node_id, kind, status, label, confidence, metadata_json;

-- name: DeleteEvidenceEdge :exec
DELETE FROM evidence_edges
WHERE id = ?;

-- name: CreateCallPath :one
INSERT INTO call_paths (
  id,
  evidence_graph_id,
  label,
  confidence,
  created_at
) VALUES (
  ?,
  ?,
  ?,
  ?,
  ?
)
RETURNING id, evidence_graph_id, label, confidence, created_at;

-- name: ListCallPathsByGraph :many
SELECT id, evidence_graph_id, label, confidence, created_at
FROM call_paths
WHERE evidence_graph_id = ?
ORDER BY created_at ASC, id ASC;

-- name: UpdateCallPath :one
UPDATE call_paths
SET
  label = ?,
  confidence = ?
WHERE id = ?
RETURNING id, evidence_graph_id, label, confidence, created_at;

-- name: DeleteCallPath :exec
DELETE FROM call_paths
WHERE id = ?;

-- name: CreateCallPathStep :one
INSERT INTO call_path_steps (
  id,
  call_path_id,
  step_index,
  node_id,
  path,
  start_line,
  end_line,
  label
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
RETURNING id, call_path_id, step_index, node_id, path, start_line, end_line, label;

-- name: ListCallPathStepsByCallPath :many
SELECT id, call_path_id, step_index, node_id, path, start_line, end_line, label
FROM call_path_steps
WHERE call_path_id = ?
ORDER BY step_index ASC;
