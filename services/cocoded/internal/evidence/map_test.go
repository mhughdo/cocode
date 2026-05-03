package evidence

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestRebuildEvidenceMapPersistsGraphNodesEdgesAndCallPath(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	finding := createEvidenceFinding(t, env.Queries, testFindingParams("finding_map", "Settings mutation lacks admin guard", "security", "high", 0.91))
	createMapEvidenceItem(t, env, "evidence_map_primary", finding.ID, KindSupporting, "Changed route", "Primary location is changed.", "src/handler.go", 4, 4, 0.92, `{"producer":"local_verifier","source":"primary_location"}`)
	createMapEvidenceItem(t, env, "evidence_map_counter", finding.ID, KindCounter, "Admin guard exists", "RequireAdmin exists in auth middleware.", "src/auth.go", 3, 3, 0.62, `{"producer":"local_verifier","source":"counter_evidence_search","rule":"auth_guard"}`)
	createMapEvidenceItem(t, env, "evidence_map_test", finding.ID, KindTest, "Admin route test", "A related route test mentions admin access.", "src/handler_test.go", 9, 9, 0.58, `{"producer":"local_verifier","source":"counter_evidence_search","rule":"auth_guard"}`)

	view, err := env.Service.RebuildEvidenceMap(context.Background(), finding)
	if err != nil {
		t.Fatalf("RebuildEvidenceMap() error = %v", err)
	}
	if view.Graph.Status != GraphStatusReady ||
		view.Graph.FindingID != finding.ID ||
		len(view.Nodes) != 4 ||
		len(view.Edges) != 3 ||
		len(view.CallPath) < 2 ||
		view.Panel.EvidenceCounts[KindCounter] != 1 ||
		view.Panel.EvidenceCounts[KindTest] != 1 {
		t.Fatalf("view = %+v", view)
	}
	if !hasMapNode(view.Nodes, NodeChangedCode, "src/handler.go") ||
		!hasMapNode(view.Nodes, NodeCounterEvidence, "src/auth.go") ||
		!hasMapNode(view.Nodes, NodeTest, "src/handler_test.go") {
		t.Fatalf("nodes = %+v", view.Nodes)
	}
	if !hasMapEdge(view.Edges, EdgeContradicts, EdgeStatusObserved) ||
		!hasMapEdge(view.Edges, EdgeTests, EdgeStatusObserved) ||
		!hasMapEdge(view.Edges, EdgeMissingGuard, EdgeStatusMissing) {
		t.Fatalf("edges = %+v", view.Edges)
	}
	if len(view.Hierarchy) != 3 || view.Hierarchy[0].Path != "src/auth.go" {
		t.Fatalf("hierarchy = %+v", view.Hierarchy)
	}

	loaded, err := env.Service.LoadEvidenceMap(context.Background(), finding)
	if err != nil {
		t.Fatalf("LoadEvidenceMap() error = %v", err)
	}
	if loaded.Graph.ID != view.Graph.ID || len(loaded.Nodes) != len(view.Nodes) || len(loaded.Edges) != len(view.Edges) {
		t.Fatalf("loaded = %+v, original = %+v", loaded, view)
	}
}

func TestRebuildEvidenceMapReturnsPartialForSparseFinding(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	finding := createEvidenceFinding(t, env.Queries, testFindingParams("finding_sparse_map", "A claim without enough evidence", "correctness", "medium", 0.44))

	view, err := env.Service.RebuildEvidenceMap(context.Background(), finding)
	if err != nil {
		t.Fatalf("RebuildEvidenceMap() error = %v", err)
	}
	if view.Graph.Status != GraphStatusPartial ||
		len(view.Nodes) != 1 ||
		view.Nodes[0].Kind != NodeUnknown ||
		len(view.Edges) != 0 ||
		view.CallPathUnavailableReason == "" ||
		len(view.MissingReasons) == 0 {
		t.Fatalf("partial view = %+v", view)
	}
	if _, err := env.Queries.GetEvidenceGraphByFinding(context.Background(), finding.ID); err != nil {
		t.Fatalf("GetEvidenceGraphByFinding() error = %v", err)
	}
}

func TestRebuildEvidenceMapBoundsEvidenceNodeCount(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	finding := createEvidenceFinding(t, env.Queries, testFindingParams("finding_large_map", "Large diff has many repeated findings", "reliability", "medium", 0.7))
	for i := 0; i < defaultEvidenceMapItemLimit+5; i++ {
		createMapEvidenceItem(t, env, "evidence_large_"+stringID(i), finding.ID, KindSearch, "Related code", "Related repeated evidence.", "src/handler.go", int64(4+i), int64(4+i), 0.5, `{"producer":"agent"}`)
	}

	view, err := env.Service.RebuildEvidenceMap(context.Background(), finding)
	if err != nil {
		t.Fatalf("RebuildEvidenceMap() error = %v", err)
	}
	if len(view.Nodes) != defaultEvidenceMapItemLimit+1 ||
		!containsMissingReason(view.MissingReasons, "omitted from graph") ||
		view.Graph.Status != GraphStatusPartial {
		t.Fatalf("bounded view nodes=%d status=%s missing=%+v", len(view.Nodes), view.Graph.Status, view.MissingReasons)
	}
}

func testFindingParams(id string, claim string, category string, severity string, confidence float64) dbgen.CreateFindingParams {
	params := dbgen.CreateFindingParams{
		ID:                 id,
		ReviewSessionID:    "session_1",
		CanonicalClaim:     claim,
		Category:           category,
		Severity:           severity,
		Confidence:         confidence,
		VerificationStatus: StatusUnverified,
		DecisionStatus:     "undecided",
		Fingerprint:        "fp_" + id,
		MergedFromCount:    1,
		FirstSeenAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:          "2026-05-03T00:04:00Z",
	}
	if !strings.Contains(id, "sparse") {
		params.PrimaryPath = nullableTestString("src/handler.go")
		params.PrimaryStartLine = nullableTestInt64(4)
		params.PrimaryEndLine = nullableTestInt64(4)
	}
	return params
}

func createMapEvidenceItem(t *testing.T, env evidenceEnv, id string, findingID string, kind string, title string, summary string, path string, startLine int64, endLine int64, confidence float64, metadata string) {
	t.Helper()

	if _, err := env.Queries.CreateEvidenceItem(context.Background(), dbgen.CreateEvidenceItemParams{
		ID:           id,
		FindingID:    findingID,
		Kind:         kind,
		Title:        title,
		Summary:      summary,
		Path:         nullableTestString(path),
		StartLine:    nullableTestInt64(startLine),
		EndLine:      nullableTestInt64(endLine),
		Confidence:   confidence,
		MetadataJson: metadata,
		CreatedAt:    "2026-05-03T00:15:00Z",
	}); err != nil {
		t.Fatalf("CreateEvidenceItem(%s) error = %v", id, err)
	}
}

func hasMapNode(nodes []NodeView, kind string, path string) bool {
	for _, node := range nodes {
		if node.Kind == kind && node.Path == path && node.DeepLink != nil {
			return true
		}
	}
	return false
}

func hasMapEdge(edges []EdgeView, kind string, status string) bool {
	for _, edge := range edges {
		if edge.Kind == kind && edge.Status == status {
			return true
		}
	}
	return false
}

func containsMissingReason(reasons []string, text string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, text) {
			return true
		}
	}
	return false
}

func stringID(value int) string {
	return fmt.Sprintf("%03d", value)
}
