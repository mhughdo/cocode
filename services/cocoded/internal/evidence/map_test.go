package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestRebuildEvidenceMapPersistsGraphNodesEdgesAndCallPath(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	finding := createEvidenceFinding(t, env.Queries, testFindingParams("finding_map", "Settings mutation lacks admin guard", "security", "high", 0.91))
	createMapEvidenceItem(t, env, "evidence_map_primary", finding.ID, KindSupporting, "Changed route", "Primary location is changed.", "src/handler.go", 4, 4, 0.92, `{"producer":"local_verifier","source":"primary_location"}`)
	createMapEvidenceItem(t, env, "evidence_map_counter", finding.ID, KindCounter, "Admin guard wraps handler", "RequireAdmin is mounted before this handler, directly refuting reachability by members.", "src/auth.go", 3, 3, 0.62, `{"producer":"local_verifier","source":"direct_contradiction","rule":"auth_guard"}`)
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
	if view.Nodes[0].CodeSnippet == "" || view.Nodes[0].LineWindow == nil {
		t.Fatalf("expected node source preview, nodes = %+v", view.Nodes)
	}
	if view.Nodes[0].FileContent == "" ||
		view.Nodes[0].FileLineCount == 0 ||
		view.Nodes[0].FileTruncated {
		t.Fatalf("expected node full-file source, nodes = %+v", view.Nodes)
	}
	if len(view.Panel.Evidence) == 0 || view.Panel.Evidence[0].CodeSnippet == "" || view.Panel.Evidence[0].LineWindow == nil {
		t.Fatalf("expected panel evidence source preview, panel = %+v", view.Panel.Evidence)
	}
	if len(view.Panel.Evidence) == 0 ||
		view.Panel.Evidence[0].FileContent == "" ||
		view.Panel.Evidence[0].FileLineCount == 0 ||
		view.Panel.Evidence[0].FileTruncated {
		t.Fatalf("expected panel evidence full-file source, panel = %+v", view.Panel.Evidence)
	}

	loaded, err := env.Service.LoadEvidenceMap(context.Background(), finding)
	if err != nil {
		t.Fatalf("LoadEvidenceMap() error = %v", err)
	}
	if loaded.Graph.ID != view.Graph.ID || len(loaded.Nodes) != len(view.Nodes) || len(loaded.Edges) != len(view.Edges) {
		t.Fatalf("loaded = %+v, original = %+v", loaded, view)
	}
}

func TestLoadOrRebuildEvidenceMapRebuildsStaleGraphAfterFindingUpdate(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	finding := createEvidenceFinding(t, env.Queries, testFindingParams("finding_stale_map", "Settings mutation lacks admin guard", "security", "high", 0.91))
	createMapEvidenceItem(t, env, "evidence_stale_primary", finding.ID, KindSupporting, "Changed route", "Primary location is changed.", "src/handler.go", 4, 4, 0.92, `{"producer":"orchestrator_curator","source":"primary_location"}`)
	if _, err := env.Service.RebuildEvidenceMap(context.Background(), finding); err != nil {
		t.Fatalf("RebuildEvidenceMap() error = %v", err)
	}
	updated, err := env.Queries.UpdateFindingVerificationEvidence(context.Background(), dbgen.UpdateFindingVerificationEvidenceParams{
		VerificationStatus:     StatusVerified,
		EvidenceSummary:        nullableTestString("Orchestrator refined this finding after the first map was built."),
		CounterEvidenceSummary: nullableTestString("No direct contradiction was verified."),
		UpdatedAt:              "2026-05-03T00:20:00Z",
		ID:                     finding.ID,
	})
	if err != nil {
		t.Fatalf("UpdateFindingVerificationEvidence() error = %v", err)
	}

	view, rebuilt, err := env.Service.LoadOrRebuildEvidenceMap(context.Background(), updated)
	if err != nil {
		t.Fatalf("LoadOrRebuildEvidenceMap() error = %v", err)
	}
	if !rebuilt || !strings.Contains(view.Panel.EvidenceSummary, "Orchestrator refined") {
		t.Fatalf("rebuilt=%v panel=%+v", rebuilt, view.Panel)
	}
}

func TestSourcePreviewFromMetadataUsesNestedAgentSnippet(t *testing.T) {
	t.Parallel()

	snippet, window := sourcePreviewFromMetadata(json.RawMessage(`{
		"producer":"orchestrator_curator",
		"agent_metadata":{
			"code_snippet":"if prices[1] != nil {\n  return *prices[1]\n}",
			"line_window":{"start_line":211,"end_line":213}
		}
	}`), 211, 211)
	if !strings.Contains(snippet, "prices[1]") ||
		window == nil ||
		window.StartLine != 211 ||
		window.EndLine != 213 {
		t.Fatalf("snippet=%q window=%+v", snippet, window)
	}
}

func TestReadSourceFileReturnsBoundedFullFile(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeEvidenceRepoFile(t, repoRoot, "src/server.js", "first\nsecond\nthird\n")
	content, lineCount, truncated, err := ReadSourceFile(repoRoot, "src/server.js", 1024)
	if err != nil {
		t.Fatalf("ReadSourceFile() error = %v", err)
	}
	if content != "first\nsecond\nthird" || lineCount != 3 || truncated {
		t.Fatalf("content=%q lineCount=%d truncated=%v", content, lineCount, truncated)
	}
}

func TestRebuildEvidenceMapOmitsProjectMetadataEvidence(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	finding := createEvidenceFinding(t, env.Queries, testFindingParams("finding_metadata_map", "Invoice export lacks admin guard", "security", "high", 0.9))
	createMapEvidenceItem(t, env, "evidence_metadata_primary", finding.ID, KindSupporting, "Changed export", "Primary location is changed.", "src/server.js", 19, 19, 0.9, `{"producer":"local_verifier","source":"primary_location"}`)
	createMapEvidenceItem(t, env, "evidence_metadata_manifest", finding.ID, KindSearch, "Package test script", "Manifest mentions a test script.", "package.json", 1, 1, 0.6, `{"producer":"local_verifier","source":"related_context","rule":"auth_guard"}`)
	createMapEvidenceItem(t, env, "evidence_metadata_test", finding.ID, KindTest, "Authorization test", "A related test mentions admin access.", "test/server.test.js", 7, 7, 0.6, `{"producer":"local_verifier","source":"counter_evidence_search","rule":"auth_guard"}`)

	view, err := env.Service.RebuildEvidenceMap(context.Background(), finding)
	if err != nil {
		t.Fatalf("RebuildEvidenceMap() error = %v", err)
	}
	if hasMapNode(view.Nodes, NodeCounterEvidence, "package.json") {
		t.Fatalf("project metadata leaked into evidence map nodes: %+v", view.Nodes)
	}
	for _, item := range view.Panel.Evidence {
		if item.Path == "package.json" {
			t.Fatalf("project metadata leaked into evidence panel: %+v", view.Panel.Evidence)
		}
	}
	if !hasMapNode(view.Nodes, NodeTest, "test/server.test.js") ||
		view.Panel.EvidenceCounts[KindTest] != 1 ||
		view.Panel.EvidenceCounts[KindCounter] != 0 {
		t.Fatalf("useful evidence was not preserved: nodes=%+v panel=%+v", view.Nodes, view.Panel)
	}
}

func TestRebuildEvidenceMapUsesCuratedRelationshipDirection(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	finding := createEvidenceFinding(t, env.Queries, testFindingParams("finding_relationship_map", "pickTokenPrice can panic", "reliability", "high", 0.9))
	createMapEvidenceItem(t, env, "evidence_relationship_primary", finding.ID, KindSupporting, "Unsafe average branch", "The changed line dereferences prices[1].", "internal/kem_rewards.go", 208, 208, 0.95, `{"producer":"orchestrator_curator","source":"primary_location"}`)
	createMapEvidenceItem(t, env, "evidence_relationship_entry", finding.ID, KindStaticAnalysis, "Reward token info calls price picker", "fetchRewardTokenInfo passes price slots into pickTokenPrice.", "internal/fetcher.go", 373, 389, 0.94, `{"producer":"orchestrator_curator","relationship":"caller"}`)
	createMapEvidenceItem(t, env, "evidence_relationship_downstream", finding.ID, KindStaticAnalysis, "Reward conversion consumes the result", "Reward conversion uses the selected price downstream.", "internal/rewards.go", 228, 237, 0.82, `{"producer":"orchestrator_curator","relationship":"downstream"}`)

	view, err := env.Service.RebuildEvidenceMap(context.Background(), finding)
	if err != nil {
		t.Fatalf("RebuildEvidenceMap() error = %v", err)
	}
	if !hasMapNode(view.Nodes, NodeHandler, "internal/fetcher.go") ||
		!hasMapNode(view.Nodes, NodeRelatedCode, "internal/rewards.go") ||
		!hasMapEdgeLabel(view.Edges, "caller") ||
		!hasMapEdgeLabel(view.Edges, "downstream") {
		t.Fatalf("view nodes=%+v edges=%+v", view.Nodes, view.Edges)
	}
	if !strings.Contains(view.Panel.ConnectionSummary, "related check") {
		t.Fatalf("connection summary = %q", view.Panel.ConnectionSummary)
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

func TestParseGoplsCallHierarchy(t *testing.T) {
	t.Parallel()

	output := `caller[0]: ranges 10:3-11 in /repo/internal/router.go from/to function BuildRouter in /repo/internal/router.go:24:6-17
identifier: function cancelSubscription in /repo/internal/handlers.go:42:6-24
callee[0]: ranges 44:8-17 in /repo/internal/handlers.go from/to function requireAdmin in /repo/internal/auth.go:11:6-18
callee[1]: ranges 45:8-31 in /repo/internal/handlers.go from/to function CancelSubscription in /repo/internal/db.go:72:6-24`

	identifier, entries := parseGoplsCallHierarchy(output, "/repo")
	if identifier == nil ||
		identifier.symbol != "cancelSubscription" ||
		identifier.path != "internal/handlers.go" ||
		identifier.line != 42 {
		t.Fatalf("identifier = %+v", identifier)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].direction != "caller" ||
		entries[0].symbol != "BuildRouter" ||
		entries[0].path != "internal/router.go" ||
		entries[0].line != 24 {
		t.Fatalf("caller = %+v", entries[0])
	}
	if entries[1].direction != "callee" ||
		entries[1].symbol != "requireAdmin" ||
		entries[1].path != "internal/auth.go" ||
		entries[1].line != 11 {
		t.Fatalf("callee = %+v", entries[1])
	}
}

func TestParseGoplsCallHierarchyDropsExternalLibraryEntries(t *testing.T) {
	t.Parallel()

	output := `identifier: function pickTokenPrice in /repo/internal/kem_rewards.go:203:6-20
callee[0]: ranges 204:12-19 in /repo/internal/kem_rewards.go from/to function ToLower in /opt/homebrew/Cellar/go/libexec/src/strings/strings.go:727:6-12
caller[0]: ranges 12:4-18 in /repo/internal/fetcher.go from/to function fetchRewardTokenInfo in /repo/internal/fetcher.go:10:6-20`

	_, entries := parseGoplsCallHierarchy(output, "/repo")
	if len(entries) != 1 ||
		entries[0].symbol != "fetchRewardTokenInfo" ||
		entries[0].path != "internal/fetcher.go" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestGoCallHierarchyTargetFindsEnclosingFunction(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	path := filepath.Join(repoRoot, "internal", "app", "aggregatedposition", "fetcher", "kyberdata")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	source := []byte(`package kyberdata

func pickTokenPrice(prices *[2]*float64) float64 {
	if prices == nil {
		return 0
	}
	if prices[0] != nil && *prices[0] > 0 {
		return (float64(*prices[0]) + float64(*prices[1])) / float64(2)
	}
	return 0
}
`)
	filePath := filepath.Join(path, "kem_rewards.go")
	if err := os.WriteFile(filePath, source, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	line, column := callHierarchyTarget(
		repoRoot,
		"internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go",
		7,
	)
	if line != 3 || column <= 0 {
		t.Fatalf("callHierarchyTarget() = %d:%d, want 3:>0", line, column)
	}
}

func TestSelectGoplsHierarchyEntriesKeepsMoreCallerContext(t *testing.T) {
	t.Parallel()

	entries := []goplsHierarchyEntry{
		{direction: "caller", symbol: "callerOne", path: "internal/caller_one.go", line: 11},
		{direction: "caller", symbol: "callerTwo", path: "internal/caller_two.go", line: 12},
		{direction: "caller", symbol: "callerThree", path: "internal/caller_three.go", line: 13},
		{direction: "caller", symbol: "callerFour", path: "internal/caller_four.go", line: 14},
		{direction: "callee", symbol: "calleeOne", path: "internal/callee_one.go", line: 20},
		{direction: "caller", symbol: "callerOne", path: "internal/caller_one.go", line: 11},
	}

	selected := selectGoplsHierarchyEntries(entries, 4)
	if len(selected) != 4 {
		t.Fatalf("selected len = %d, want 4: %+v", len(selected), selected)
	}
	callers := 0
	callees := 0
	for _, entry := range selected {
		switch entry.direction {
		case "caller":
			callers++
		case "callee":
			callees++
		}
	}
	if callers != 3 || callees != 1 {
		t.Fatalf("selected = %+v, want caller-heavy context with callee coverage", selected)
	}
}

func TestRebuildEvidenceMapWithContextFallsBackToLocalGoCallScan(t *testing.T) {
	env := setupEvidenceEnv(t)
	repoRoot := t.TempDir()
	sourceDir := filepath.Join(repoRoot, "internal", "app", "aggregatedposition", "fetcher", "kyberdata")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(sourceDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "kem_rewards.go"), []byte(`package kyberdata

func pickTokenPrice(prices *[2]*float64) float64 {
	if prices == nil {
		return 0
	}
	if prices[0] != nil && *prices[0] > 0 {
		return (float64(*prices[0]) + float64(*prices[1])) / float64(2)
	}
	return 0
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile(kem_rewards.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "fetcher.go"), []byte(`package kyberdata

func fetchRewardTokenInfo(prices *[2]*float64) float64 {
	return pickTokenPrice(prices)
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile(fetcher.go) error = %v", err)
	}

	finding := createEvidenceFinding(t, env.Queries, testFindingParams("finding_local_scan_map", "pickTokenPrice dereferences prices[1]", "correctness", "high", 0.85))
	finding.PrimaryPath = nullableTestString("internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go")
	finding.PrimaryStartLine = nullableTestInt64(7)
	finding.PrimaryEndLine = nullableTestInt64(8)
	createMapEvidenceItem(t, env, "evidence_local_scan_primary", finding.ID, KindSupporting, "Changed average", "The changed line dereferences prices[1].", "internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go", 7, 8, 0.85, `{"producer":"local_verifier","source":"primary_location"}`)

	view, err := env.Service.RebuildEvidenceMapWithContext(context.Background(), finding, MapContext{RepositoryLocalPath: repoRoot})
	if err != nil {
		t.Fatalf("RebuildEvidenceMapWithContext() error = %v", err)
	}
	if !hasMapNode(view.Nodes, NodeHandler, "internal/app/aggregatedposition/fetcher/kyberdata/fetcher.go") ||
		len(view.CallPaths) == 0 ||
		!strings.Contains(view.Panel.ConnectionSummary, "local Go AST call-site analysis") {
		t.Fatalf("view = %+v", view)
	}
	var layout struct {
		CallHierarchy struct {
			Available bool   `json:"available"`
			Source    string `json:"source"`
			Reason    string `json:"reason"`
		} `json:"call_hierarchy"`
	}
	if err := json.Unmarshal(view.Graph.Layout, &layout); err != nil {
		t.Fatalf("Unmarshal(layout) error = %v", err)
	}
	if !layout.CallHierarchy.Available ||
		layout.CallHierarchy.Source != "go_ast_call_scan" ||
		!strings.Contains(layout.CallHierarchy.Reason, "local Go AST call-site analysis") {
		t.Fatalf("layout = %+v", layout)
	}
}

func TestRebuildEvidenceMapWithContextUsesHeuristicCallScanForTypeScript(t *testing.T) {
	env := setupEvidenceEnv(t)
	repoRoot := t.TempDir()
	sourceDir := filepath.Join(repoRoot, "src")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(sourceDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "prices.ts"), []byte(`export function pickTokenPrice(prices: number[]): number {
  if (!prices.length) {
    return 0
  }
  return prices[0]
}

export class RewardFetcher {
  fetchRewardTokenInfo(prices: number[]) {
    return pickTokenPrice(prices)
  }
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile(prices.ts) error = %v", err)
	}

	finding := createEvidenceFinding(t, env.Queries, testFindingParams("finding_ts_map", "pickTokenPrice trusts the first price", "correctness", "medium", 0.8))
	finding.PrimaryPath = nullableTestString("src/prices.ts")
	finding.PrimaryStartLine = nullableTestInt64(4)
	finding.PrimaryEndLine = nullableTestInt64(5)
	createMapEvidenceItem(t, env, "evidence_ts_primary", finding.ID, KindSupporting, "Changed return", "The changed line returns prices[0].", "src/prices.ts", 4, 5, 0.8, `{"producer":"local_verifier","source":"primary_location"}`)

	view, err := env.Service.RebuildEvidenceMapWithContext(context.Background(), finding, MapContext{RepositoryLocalPath: repoRoot})
	if err != nil {
		t.Fatalf("RebuildEvidenceMapWithContext() error = %v", err)
	}
	if !hasMapNode(view.Nodes, NodeHandler, "src/prices.ts") ||
		len(view.CallPaths) == 0 ||
		!strings.Contains(view.Panel.ConnectionSummary, "bundled heuristic call-site scan") {
		t.Fatalf("view = %+v", view)
	}
	var layout struct {
		CallHierarchy struct {
			Available bool   `json:"available"`
			Source    string `json:"source"`
			Reason    string `json:"reason"`
		} `json:"call_hierarchy"`
	}
	if err := json.Unmarshal(view.Graph.Layout, &layout); err != nil {
		t.Fatalf("Unmarshal(layout) error = %v", err)
	}
	if !layout.CallHierarchy.Available ||
		layout.CallHierarchy.Source != "heuristic_call_scan" ||
		!strings.Contains(layout.CallHierarchy.Reason, "TypeScript LSP call hierarchy is not configured") {
		t.Fatalf("layout = %+v", layout)
	}
}

func TestRebuildEvidenceMapWithContextUsesGoplsCallHierarchyTarget(t *testing.T) {
	env := setupEvidenceEnv(t)
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/repo\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	sourceDir := filepath.Join(repoRoot, "internal", "app", "aggregatedposition", "fetcher", "kyberdata")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(sourceDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "kem_rewards.go"), []byte(`package kyberdata

func pickTokenPrice(prices *[2]*float64) float64 {
	if prices == nil {
		return 0
	}
	if prices[0] != nil && *prices[0] > 0 {
		return (float64(*prices[0]) + float64(*prices[1])) / float64(2)
	}
	return 0
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile(kem_rewards.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "fetcher.go"), []byte(`package kyberdata

func fetchRewardTokenInfo(prices *[2]*float64) float64 {
	return pickTokenPrice(prices)
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile(fetcher.go) error = %v", err)
	}
	originalLookPathGopls := lookPathGopls
	originalRunGoplsCallHierarchy := runGoplsCallHierarchy
	var seenTarget string
	var seenDir string
	lookPathGopls = func(name string) (string, error) {
		if name == "gopls" {
			return "/test/bin/gopls", nil
		}
		return originalLookPathGopls(name)
	}
	runGoplsCallHierarchy = func(_ context.Context, goplsPath string, dir string, target string) ([]byte, error) {
		if goplsPath != "/test/bin/gopls" {
			t.Fatalf("gopls path = %q", goplsPath)
		}
		seenTarget = target
		seenDir = dir
		return []byte(fmt.Sprintf(`caller[0]: ranges 4:9-23 in %[1]s/internal/app/aggregatedposition/fetcher/kyberdata/fetcher.go from/to function fetchRewardTokenInfo in %[1]s/internal/app/aggregatedposition/fetcher/kyberdata/fetcher.go:3:6-26
identifier: function pickTokenPrice in %[1]s/internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go:3:6-20
`, repoRoot)), nil
	}
	t.Cleanup(func() {
		lookPathGopls = originalLookPathGopls
		runGoplsCallHierarchy = originalRunGoplsCallHierarchy
	})

	finding := createEvidenceFinding(t, env.Queries, testFindingParams("finding_gopls_map", "pickTokenPrice dereferences prices[1]", "correctness", "high", 0.85))
	finding.PrimaryPath = nullableTestString("internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go")
	finding.PrimaryStartLine = nullableTestInt64(7)
	finding.PrimaryEndLine = nullableTestInt64(8)
	createMapEvidenceItem(t, env, "evidence_gopls_primary", finding.ID, KindSupporting, "Changed average", "The changed line dereferences prices[1].", "internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go", 7, 8, 0.85, `{"producer":"local_verifier","source":"primary_location"}`)

	view, err := env.Service.RebuildEvidenceMapWithContext(context.Background(), finding, MapContext{RepositoryLocalPath: repoRoot})
	if err != nil {
		t.Fatalf("RebuildEvidenceMapWithContext() error = %v", err)
	}
	if !strings.Contains(seenTarget, "kem_rewards.go:3:6") || seenDir != repoRoot {
		t.Fatalf("gopls target = %q dir = %q, want enclosing function identifier at module root", seenTarget, seenDir)
	}
	if !hasMapNode(view.Nodes, NodeHandler, "internal/app/aggregatedposition/fetcher/kyberdata/fetcher.go") ||
		len(view.CallPaths) == 0 ||
		!strings.Contains(view.Panel.ConnectionSummary, "gopls call hierarchy") {
		t.Fatalf("view = %+v", view)
	}
	var layout struct {
		CallHierarchy struct {
			Attempted bool   `json:"attempted"`
			Available bool   `json:"available"`
			Target    string `json:"target"`
		} `json:"call_hierarchy"`
	}
	if err := json.Unmarshal(view.Graph.Layout, &layout); err != nil {
		t.Fatalf("Unmarshal(layout) error = %v", err)
	}
	if !layout.CallHierarchy.Attempted || !layout.CallHierarchy.Available || !strings.Contains(layout.CallHierarchy.Target, "kem_rewards.go:3:6") {
		t.Fatalf("layout = %+v", layout)
	}
}

func TestFindGoModuleRootSupportsNestedModules(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	moduleRoot := filepath.Join(repoRoot, "services", "api")
	if err := os.MkdirAll(filepath.Join(moduleRoot, "internal"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte("module example.com/api\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}

	root, ok := findGoModuleRoot(repoRoot, "services/api/internal/handler.go")
	if !ok || root != moduleRoot {
		t.Fatalf("findGoModuleRoot() = %q, %v; want %q, true", root, ok, moduleRoot)
	}
	if root, ok := findGoModuleRoot(repoRoot, "../outside/handler.go"); ok || root != "" {
		t.Fatalf("findGoModuleRoot(outside) = %q, %v; want empty false", root, ok)
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

func hasMapEdgeLabel(edges []EdgeView, label string) bool {
	for _, edge := range edges {
		if edge.Label == label {
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
