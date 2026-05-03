package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestEvidenceGraphQueriesCRUD(t *testing.T) {
	t.Parallel()

	queries := seededReviewQueries(t)
	createFindingForEvidenceTest(t, queries)

	evidenceItem, err := queries.CreateEvidenceItem(context.Background(), dbgen.CreateEvidenceItemParams{
		ID:           "evidence_item_1",
		FindingID:    "finding_1",
		Kind:         "supporting",
		Title:        "Status lifecycle test",
		Summary:      "The test exercises review session status updates.",
		Path:         nullableString("services/cocoded/internal/db/review_agent_queries_test.go"),
		StartLine:    nullableInt64(10),
		EndLine:      nullableInt64(30),
		Confidence:   0.9,
		MetadataJson: `{"source":"unit-test"}`,
		CreatedAt:    "2026-05-03T00:15:00Z",
	})
	if err != nil {
		t.Fatalf("CreateEvidenceItem() error = %v", err)
	}
	if evidenceItem.Kind != "supporting" || evidenceItem.FindingID != "finding_1" {
		t.Fatalf("CreateEvidenceItem() = %+v", evidenceItem)
	}

	updatedEvidenceItem, err := queries.UpdateEvidenceItem(context.Background(), dbgen.UpdateEvidenceItemParams{
		ID:           "evidence_item_1",
		Kind:         "test",
		Title:        "Review lifecycle test",
		Summary:      "The test covers create, update, and completion status.",
		Path:         nullableString("services/cocoded/internal/db/review_agent_queries_test.go"),
		StartLine:    nullableInt64(12),
		EndLine:      nullableInt64(32),
		Confidence:   0.95,
		MetadataJson: `{"source":"updated"}`,
	})
	if err != nil {
		t.Fatalf("UpdateEvidenceItem() error = %v", err)
	}
	if updatedEvidenceItem.Kind != "test" || updatedEvidenceItem.Confidence != 0.95 {
		t.Fatalf("UpdateEvidenceItem() = %+v", updatedEvidenceItem)
	}

	items, err := queries.ListEvidenceItemsByFinding(context.Background(), "finding_1")
	if err != nil {
		t.Fatalf("ListEvidenceItemsByFinding() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "evidence_item_1" {
		t.Fatalf("ListEvidenceItemsByFinding() = %+v", items)
	}

	graph, err := queries.CreateEvidenceGraph(context.Background(), dbgen.CreateEvidenceGraphParams{
		ID:              "evidence_graph_1",
		FindingID:       "finding_1",
		ReviewSessionID: "review_session_1",
		Status:          "ready",
		LayoutJson:      "{}",
		Summary:         nullableString("Lifecycle storage path"),
		CreatedAt:       "2026-05-03T00:16:00Z",
		UpdatedAt:       "2026-05-03T00:16:00Z",
	})
	if err != nil {
		t.Fatalf("CreateEvidenceGraph() error = %v", err)
	}
	if graph.FindingID != "finding_1" {
		t.Fatalf("CreateEvidenceGraph() = %+v", graph)
	}

	byFinding, err := queries.GetEvidenceGraphByFinding(context.Background(), "finding_1")
	if err != nil {
		t.Fatalf("GetEvidenceGraphByFinding() error = %v", err)
	}
	if byFinding.ID != graph.ID {
		t.Fatalf("GetEvidenceGraphByFinding() ID = %q, want %q", byFinding.ID, graph.ID)
	}

	updatedGraph, err := queries.UpdateEvidenceGraph(context.Background(), dbgen.UpdateEvidenceGraphParams{
		ID:         "evidence_graph_1",
		Status:     "ready",
		LayoutJson: `{"direction":"horizontal"}`,
		Summary:    nullableString("Updated lifecycle storage path"),
		UpdatedAt:  "2026-05-03T00:17:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateEvidenceGraph() error = %v", err)
	}
	if updatedGraph.LayoutJson != `{"direction":"horizontal"}` {
		t.Fatalf("UpdateEvidenceGraph() = %+v", updatedGraph)
	}

	changedNode, err := queries.CreateEvidenceNode(context.Background(), dbgen.CreateEvidenceNodeParams{
		ID:              "node_changed",
		EvidenceGraphID: "evidence_graph_1",
		Kind:            "changed_code",
		Label:           "review session status query",
		Path:            nullableString("services/cocoded/internal/db/sql/queries/review_sessions.sql"),
		Symbol:          nullableString("UpdateReviewSessionStatus"),
		StartLine:       nullableInt64(42),
		EndLine:         nullableInt64(52),
		EvidenceItemID:  nullableString("evidence_item_1"),
		Confidence:      0.9,
		MetadataJson:    "{}",
	})
	if err != nil {
		t.Fatalf("CreateEvidenceNode(changed) error = %v", err)
	}
	handlerNode, err := queries.CreateEvidenceNode(context.Background(), dbgen.CreateEvidenceNodeParams{
		ID:              "node_handler",
		EvidenceGraphID: "evidence_graph_1",
		Kind:            "handler",
		Label:           "review session API",
		Path:            nullableString("services/cocoded/internal/httpapi/router.go"),
		Symbol:          nullableString("NewRouter"),
		StartLine:       nullableInt64(1),
		EndLine:         nullableInt64(20),
		Confidence:      0.7,
		MetadataJson:    "{}",
	})
	if err != nil {
		t.Fatalf("CreateEvidenceNode(handler) error = %v", err)
	}

	updatedNode, err := queries.UpdateEvidenceNode(context.Background(), dbgen.UpdateEvidenceNodeParams{
		ID:             "node_changed",
		Kind:           "changed_code",
		Label:          "typed review session status query",
		Path:           nullableString("services/cocoded/internal/db/sql/queries/review_sessions.sql"),
		Symbol:         nullableString("UpdateReviewSessionStatus"),
		StartLine:      nullableInt64(42),
		EndLine:        nullableInt64(52),
		EvidenceItemID: nullableString("evidence_item_1"),
		Confidence:     0.95,
		MetadataJson:   `{"updated":true}`,
	})
	if err != nil {
		t.Fatalf("UpdateEvidenceNode() error = %v", err)
	}
	if updatedNode.Label != "typed review session status query" || updatedNode.Confidence != 0.95 {
		t.Fatalf("UpdateEvidenceNode() = %+v", updatedNode)
	}

	nodes, err := queries.ListEvidenceNodesByGraph(context.Background(), "evidence_graph_1")
	if err != nil {
		t.Fatalf("ListEvidenceNodesByGraph() error = %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("ListEvidenceNodesByGraph() len = %d, want 2", len(nodes))
	}

	edge, err := queries.CreateEvidenceEdge(context.Background(), dbgen.CreateEvidenceEdgeParams{
		ID:              "edge_1",
		EvidenceGraphID: "evidence_graph_1",
		SourceNodeID:    changedNode.ID,
		TargetNodeID:    handlerNode.ID,
		Kind:            "supports",
		Status:          "observed",
		Label:           nullableString("covered by"),
		Confidence:      0.8,
		MetadataJson:    "{}",
	})
	if err != nil {
		t.Fatalf("CreateEvidenceEdge() error = %v", err)
	}
	if edge.Kind != "supports" {
		t.Fatalf("CreateEvidenceEdge() = %+v", edge)
	}

	updatedEdge, err := queries.UpdateEvidenceEdge(context.Background(), dbgen.UpdateEvidenceEdgeParams{
		ID:           "edge_1",
		Kind:         "tests",
		Status:       "observed",
		Label:        nullableString("tested by"),
		Confidence:   0.88,
		MetadataJson: `{"updated":true}`,
	})
	if err != nil {
		t.Fatalf("UpdateEvidenceEdge() error = %v", err)
	}
	if updatedEdge.Kind != "tests" || updatedEdge.Confidence != 0.88 {
		t.Fatalf("UpdateEvidenceEdge() = %+v", updatedEdge)
	}

	edges, err := queries.ListEvidenceEdgesByGraph(context.Background(), "evidence_graph_1")
	if err != nil {
		t.Fatalf("ListEvidenceEdgesByGraph() error = %v", err)
	}
	if len(edges) != 1 || edges[0].ID != "edge_1" {
		t.Fatalf("ListEvidenceEdgesByGraph() = %+v", edges)
	}

	callPath, err := queries.CreateCallPath(context.Background(), dbgen.CreateCallPathParams{
		ID:              "call_path_1",
		EvidenceGraphID: "evidence_graph_1",
		Label:           nullableString("status update path"),
		Confidence:      0.75,
		CreatedAt:       "2026-05-03T00:18:00Z",
	})
	if err != nil {
		t.Fatalf("CreateCallPath() error = %v", err)
	}
	if callPath.EvidenceGraphID != "evidence_graph_1" {
		t.Fatalf("CreateCallPath() = %+v", callPath)
	}

	updatedCallPath, err := queries.UpdateCallPath(context.Background(), dbgen.UpdateCallPathParams{
		ID:         "call_path_1",
		Label:      nullableString("updated status path"),
		Confidence: 0.8,
	})
	if err != nil {
		t.Fatalf("UpdateCallPath() error = %v", err)
	}
	if updatedCallPath.Label.String != "updated status path" || updatedCallPath.Confidence != 0.8 {
		t.Fatalf("UpdateCallPath() = %+v", updatedCallPath)
	}

	if _, err := queries.CreateCallPathStep(context.Background(), dbgen.CreateCallPathStepParams{
		ID:         "call_path_step_2",
		CallPathID: "call_path_1",
		StepIndex:  2,
		NodeID:     nullableString("node_handler"),
		Path:       nullableString("services/cocoded/internal/httpapi/router.go"),
		StartLine:  nullableInt64(1),
		EndLine:    nullableInt64(20),
		Label:      "HTTP layer observes session state",
	}); err != nil {
		t.Fatalf("CreateCallPathStep(2) error = %v", err)
	}
	if _, err := queries.CreateCallPathStep(context.Background(), dbgen.CreateCallPathStepParams{
		ID:         "call_path_step_1",
		CallPathID: "call_path_1",
		StepIndex:  1,
		NodeID:     nullableString("node_changed"),
		Path:       nullableString("services/cocoded/internal/db/sql/queries/review_sessions.sql"),
		StartLine:  nullableInt64(42),
		EndLine:    nullableInt64(52),
		Label:      "DB updates session status",
	}); err != nil {
		t.Fatalf("CreateCallPathStep(1) error = %v", err)
	}

	steps, err := queries.ListCallPathStepsByCallPath(context.Background(), "call_path_1")
	if err != nil {
		t.Fatalf("ListCallPathStepsByCallPath() error = %v", err)
	}
	if len(steps) != 2 || steps[0].StepIndex != 1 || steps[1].StepIndex != 2 {
		t.Fatalf("ListCallPathStepsByCallPath() = %+v", steps)
	}

	if err := queries.DeleteEvidenceEdge(context.Background(), "edge_1"); err != nil {
		t.Fatalf("DeleteEvidenceEdge() error = %v", err)
	}
	edges, err = queries.ListEvidenceEdgesByGraph(context.Background(), "evidence_graph_1")
	if err != nil {
		t.Fatalf("ListEvidenceEdgesByGraph(after delete) error = %v", err)
	}
	if len(edges) != 0 {
		t.Fatalf("ListEvidenceEdgesByGraph(after delete) len = %d, want 0", len(edges))
	}

	if err := queries.DeleteCallPath(context.Background(), "call_path_1"); err != nil {
		t.Fatalf("DeleteCallPath() error = %v", err)
	}
	paths, err := queries.ListCallPathsByGraph(context.Background(), "evidence_graph_1")
	if err != nil {
		t.Fatalf("ListCallPathsByGraph(after delete) error = %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("ListCallPathsByGraph(after delete) len = %d, want 0", len(paths))
	}

	if err := queries.DeleteEvidenceGraph(context.Background(), "evidence_graph_1"); err != nil {
		t.Fatalf("DeleteEvidenceGraph() error = %v", err)
	}
	if _, err := queries.GetEvidenceGraph(context.Background(), "evidence_graph_1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetEvidenceGraph(deleted) error = %v, want sql.ErrNoRows", err)
	}
	if _, err := queries.GetEvidenceNode(context.Background(), "node_changed"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetEvidenceNode(deleted graph child) error = %v, want sql.ErrNoRows", err)
	}

	if err := queries.DeleteEvidenceItem(context.Background(), "evidence_item_1"); err != nil {
		t.Fatalf("DeleteEvidenceItem() error = %v", err)
	}
	if _, err := queries.GetEvidenceItem(context.Background(), "evidence_item_1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetEvidenceItem(deleted) error = %v, want sql.ErrNoRows", err)
	}
}

func createFindingForEvidenceTest(t *testing.T, queries *dbgen.Queries) {
	t.Helper()

	createReviewSessionForTest(t, queries, "review_session_1", "Review cocode")
	createAgentRunForFindingTest(t, queries)
	if _, err := queries.CreateFinding(context.Background(), dbgen.CreateFindingParams{
		ID:                     "finding_1",
		ReviewSessionID:        "review_session_1",
		CanonicalClaim:         "Session status updates persist",
		Category:               "correctness",
		Severity:               "high",
		Confidence:             0.9,
		VerificationStatus:     "verified",
		DecisionStatus:         "accepted",
		PrimaryPath:            nullableString("services/cocoded/internal/db/review_agent_queries_test.go"),
		PrimaryStartLine:       nullableInt64(10),
		PrimaryEndLine:         nullableInt64(20),
		EvidenceSummary:        nullableString("status updates are persisted"),
		CounterEvidenceSummary: nullableString("none"),
		SuggestedFix:           nullableString("keep lifecycle tests"),
		DraftComment:           nullableString("Lifecycle storage is covered."),
		Fingerprint:            "finding-fingerprint-1",
		MergedFromCount:        1,
		FirstSeenAt:            "2026-05-03T00:10:00Z",
		UpdatedAt:              "2026-05-03T00:10:00Z",
	}); err != nil {
		t.Fatalf("CreateFinding() error = %v", err)
	}
}
