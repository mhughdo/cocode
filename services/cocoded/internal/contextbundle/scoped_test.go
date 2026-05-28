package contextbundle

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestServiceBuildFindingContextScopesEvidenceAndChangedFiles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repoPath := t.TempDir()
	writeRepoFile(t, repoPath, "app/auth.go", "package app\n\nconst apiKey = \"sk-abcdefghijklmnopqrstuvwxyz\"\n\nfunc UpdateSettings() {}\n")
	writeRepoFile(t, repoPath, "app/auth_test.go", "package app\n\nfunc TestUpdateSettings(t *testing.T) {}\n")
	writeRepoFile(t, repoPath, "app/other.go", "package app\n\nfunc Other() {}\n")
	queries, store := contextBuilderTestStore(t, repoPath)
	createScopedPatch(t, store, queries, "artifact_patch_auth", "file_auth", "app/auth.go", "@@ -1,3 +1,5 @@\n package app\n+const apiKey = \"sk-abcdefghijklmnopqrstuvwxyz\"\n+func UpdateSettings() {}\n")
	createScopedPatch(t, store, queries, "artifact_patch_other", "file_other", "app/other.go", "@@ -1,2 +1,3 @@\n package app\n+func Other() {}\n")
	createScopedFinding(t, queries, "finding_auth")
	createScopedEvidence(t, queries, "evidence_primary", "finding_auth", "supporting", "Primary code", "Changed code is missing an admin guard.", "app/auth.go", 4, 0, 0.9, `{"producer":"local_verifier","source":"primary_location","code_snippet":"4: const apiKey = \"sk-abcdefghijklmnopqrstuvwxyz\""}`)
	createScopedEvidence(t, queries, "evidence_counter", "finding_auth", "counter", "Existing guard", "RequireAdmin appears in middleware.", "app/middleware.go", 12, 12, 0.6, `{"producer":"local_verifier","source":"counter_evidence_search"}`)

	service := Service{
		Queries:   queries,
		Artifacts: store,
		Searcher:  fakeReviewContextSearcher{matches: []CodeSearchMatch{{Path: "app/auth_test.go", Line: 3, Text: "func TestUpdateSettings(t *testing.T) {}"}}},
		Now: func() time.Time {
			return time.Date(2026, 5, 3, 1, 0, 0, 0, time.UTC)
		},
	}
	result, err := service.BuildFindingContext(ctx, BuildFindingContextParams{
		ReviewSessionID: "review_session_1",
		FindingID:       "finding_auth",
		Persist:         true,
		PolicyOverride:  json.RawMessage(`{"max_tokens":10000,"max_items":40}`),
	})
	if err != nil {
		t.Fatalf("BuildFindingContext() error = %v", err)
	}
	if !result.Persisted ||
		result.Bundle.Scope != ScopeFinding ||
		result.Bundle.ArtifactID == "" ||
		result.Artifact.Kind != "context_bundle" ||
		result.RedactionReport.RedactionCount == 0 {
		t.Fatalf("result = %+v", result)
	}
	kinds := itemKinds(result.Bundle.Items)
	for _, want := range []ItemKind{ItemPromptMaterial, ItemEvidence, ItemChangedHunk, ItemFullFile, ItemRelatedCode, ItemRelatedTest} {
		if !slices.Contains(kinds, want) {
			t.Fatalf("item kinds = %+v, missing %s", kinds, want)
		}
	}
	for _, item := range result.Bundle.Items {
		if item.Path == "app/other.go" {
			t.Fatalf("unrelated changed file leaked into finding context: %+v", item)
		}
		if item.Kind == ItemEvidence && item.StartLine != 0 && item.EndLine == 0 {
			t.Fatalf("evidence item line range was not normalized: %+v", item)
		}
	}
	rendered, _, err := store.Read(ctx, result.Artifact.ID)
	if err != nil {
		t.Fatalf("Read(context artifact) error = %v", err)
	}
	if strings.Contains(string(rendered), "sk-abcdefghijklmnopqrstuvwxyz") ||
		!strings.Contains(string(rendered), "[REDACTED]") ||
		!strings.Contains(string(rendered), "Finding ID: finding_auth") {
		t.Fatalf("rendered context = %s", string(rendered))
	}
}

func TestServiceBuildEvidenceMapContextIncludesGraph(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repoPath := t.TempDir()
	writeRepoFile(t, repoPath, "app/auth.go", "package app\n\nfunc UpdateSettings() {}\n")
	queries, store := contextBuilderTestStore(t, repoPath)
	createScopedPatch(t, store, queries, "artifact_patch_auth", "file_auth", "app/auth.go", "@@ -1,2 +1,3 @@\n package app\n+func UpdateSettings() {}\n")
	createScopedFinding(t, queries, "finding_auth")
	createScopedEvidence(t, queries, "evidence_primary", "finding_auth", "supporting", "Primary code", "Changed code is missing an admin guard.", "app/auth.go", 3, 3, 0.9, `{"producer":"local_verifier","source":"primary_location"}`)
	createScopedEvidenceGraph(t, queries)

	service := Service{
		Queries:   queries,
		Artifacts: store,
		Now: func() time.Time {
			return time.Date(2026, 5, 3, 1, 5, 0, 0, time.UTC)
		},
	}
	result, err := service.BuildEvidenceMapContext(ctx, BuildEvidenceMapContextParams{
		FindingID:      "finding_auth",
		PolicyOverride: json.RawMessage(`{"max_tokens":10000,"max_items":60}`),
	})
	if err != nil {
		t.Fatalf("BuildEvidenceMapContext() error = %v", err)
	}
	if result.Bundle.Scope != ScopeEvidenceMap ||
		!hasContextItemTitle(result.Bundle.Items, "Evidence Map graph") ||
		result.Bundle.TokenEstimate <= 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestFindingContextSymbolsIncludeEnclosingGoFunction(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	writeRepoFile(t, repoPath, "app/auth.go", `package app

type Service struct{}

func (s *Service) UpdateSettings() {
	if s == nil {
		return
	}
}
`)
	finding := dbgen.Finding{
		CanonicalClaim:   "Settings mutation lacks admin guard",
		PrimaryPath:      sql.NullString{String: "app/auth.go", Valid: true},
		PrimaryStartLine: sql.NullInt64{Int64: 6, Valid: true},
	}

	symbols := findingContextSymbols(repoPath, finding, nil)
	for _, want := range []string{"Service.UpdateSettings", "UpdateSettings", "Service"} {
		if !slices.Contains(symbols, want) {
			t.Fatalf("symbols = %+v, missing %q", symbols, want)
		}
	}
}

func TestFindingContextSymbolsIncludeHeuristicLanguageSymbol(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	writeRepoFile(t, repoPath, "src/prices.ts", `export class RewardFetcher {
  pickTokenPrice(prices: number[]) {
    return prices[0]
  }
}
`)
	finding := dbgen.Finding{
		CanonicalClaim:   "Price picker trusts the first price",
		PrimaryPath:      sql.NullString{String: "src/prices.ts", Valid: true},
		PrimaryStartLine: sql.NullInt64{Int64: 3, Valid: true},
	}

	symbols := findingContextSymbols(repoPath, finding, nil)
	for _, want := range []string{"RewardFetcher.pickTokenPrice", "pickTokenPrice", "RewardFetcher"} {
		if !slices.Contains(symbols, want) {
			t.Fatalf("symbols = %+v, missing %q", symbols, want)
		}
	}
}

func createScopedPatch(t *testing.T, store *artifact.Store, queries *dbgen.Queries, artifactID string, fileID string, path string, patchContent string) {
	t.Helper()

	patch, err := store.Save(context.Background(), artifact.SaveParams{
		ID:           artifactID,
		WorkspaceID:  "workspace_1",
		Kind:         "patch",
		RelativePath: "snapshots/snapshot_1/patches/" + fileID + ".patch",
		ContentType:  "text/x-diff",
		MetadataJSON: `{"path":"` + path + `"}`,
		CreatedAt:    "2026-05-03T00:04:00Z",
	}, []byte(patchContent))
	if err != nil {
		t.Fatalf("Save(patch %s) error = %v", artifactID, err)
	}
	if _, err := queries.CreateChangedFile(context.Background(), dbgen.CreateChangedFileParams{
		ID:              fileID,
		SnapshotID:      "snapshot_1",
		Path:            path,
		Status:          "modified",
		Additions:       1,
		LineRangesJson:  `[[3,4]]`,
		PatchArtifactID: sql.NullString{String: patch.ID, Valid: true},
		CreatedAt:       "2026-05-03T00:05:00Z",
	}); err != nil {
		t.Fatalf("CreateChangedFile(%s) error = %v", fileID, err)
	}
}

func createScopedFinding(t *testing.T, queries *dbgen.Queries, id string) {
	t.Helper()

	if _, err := queries.CreateFinding(context.Background(), dbgen.CreateFindingParams{
		ID:                 id,
		ReviewSessionID:    "review_session_1",
		CanonicalClaim:     "Settings mutation lacks admin guard",
		Category:           "security",
		Severity:           "high",
		Confidence:         0.92,
		VerificationStatus: "verified",
		DecisionStatus:     "undecided",
		PrimaryPath:        sql.NullString{String: "app/auth.go", Valid: true},
		PrimaryStartLine:   sql.NullInt64{Int64: 4, Valid: true},
		EvidenceSummary:    sql.NullString{String: "Primary code and local verifier evidence support the claim.", Valid: true},
		Fingerprint:        "fp_" + id,
		MergedFromCount:    1,
		FirstSeenAt:        "2026-05-03T00:10:00Z",
		UpdatedAt:          "2026-05-03T00:10:00Z",
	}); err != nil {
		t.Fatalf("CreateFinding(%s) error = %v", id, err)
	}
}

func createScopedEvidence(t *testing.T, queries *dbgen.Queries, id string, findingID string, kind string, title string, summary string, path string, startLine int64, endLine int64, confidence float64, metadata string) {
	t.Helper()

	if _, err := queries.CreateEvidenceItem(context.Background(), dbgen.CreateEvidenceItemParams{
		ID:           id,
		FindingID:    findingID,
		Kind:         kind,
		Title:        title,
		Summary:      summary,
		Path:         sql.NullString{String: path, Valid: path != ""},
		StartLine:    sql.NullInt64{Int64: startLine, Valid: startLine > 0},
		EndLine:      sql.NullInt64{Int64: endLine, Valid: endLine > 0},
		Confidence:   confidence,
		MetadataJson: metadata,
		CreatedAt:    "2026-05-03T00:15:00Z",
	}); err != nil {
		t.Fatalf("CreateEvidenceItem(%s) error = %v", id, err)
	}
}

func createScopedEvidenceGraph(t *testing.T, queries *dbgen.Queries) {
	t.Helper()

	if _, err := queries.CreateEvidenceGraph(context.Background(), dbgen.CreateEvidenceGraphParams{
		ID:              "graph_auth",
		FindingID:       "finding_auth",
		ReviewSessionID: "review_session_1",
		Status:          "ready",
		LayoutJson:      `{"direction":"LR"}`,
		Summary:         sql.NullString{String: "Auth graph includes route and missing guard.", Valid: true},
		CreatedAt:       "2026-05-03T00:16:00Z",
		UpdatedAt:       "2026-05-03T00:16:00Z",
	}); err != nil {
		t.Fatalf("CreateEvidenceGraph() error = %v", err)
	}
	if _, err := queries.CreateEvidenceNode(context.Background(), dbgen.CreateEvidenceNodeParams{
		ID:              "node_route",
		EvidenceGraphID: "graph_auth",
		Kind:            "changed_code",
		Label:           "UpdateSettings route",
		Path:            sql.NullString{String: "app/auth.go", Valid: true},
		StartLine:       sql.NullInt64{Int64: 4, Valid: true},
		EndLine:         sql.NullInt64{Int64: 4, Valid: true},
		EvidenceItemID:  sql.NullString{String: "evidence_primary", Valid: true},
		Confidence:      0.9,
		MetadataJson:    `{}`,
	}); err != nil {
		t.Fatalf("CreateEvidenceNode(route) error = %v", err)
	}
	if _, err := queries.CreateEvidenceNode(context.Background(), dbgen.CreateEvidenceNodeParams{
		ID:              "node_missing_guard",
		EvidenceGraphID: "graph_auth",
		Kind:            "missing_guard",
		Label:           "Admin guard not mounted",
		Confidence:      0.8,
		MetadataJson:    `{}`,
	}); err != nil {
		t.Fatalf("CreateEvidenceNode(missing) error = %v", err)
	}
	if _, err := queries.CreateEvidenceEdge(context.Background(), dbgen.CreateEvidenceEdgeParams{
		ID:              "edge_missing_guard",
		EvidenceGraphID: "graph_auth",
		SourceNodeID:    "node_route",
		TargetNodeID:    "node_missing_guard",
		Kind:            "missing_guard",
		Status:          "missing",
		Label:           sql.NullString{String: "guard missing", Valid: true},
		Confidence:      0.8,
		MetadataJson:    `{}`,
	}); err != nil {
		t.Fatalf("CreateEvidenceEdge() error = %v", err)
	}
}

func hasContextItemTitle(items []Item, title string) bool {
	for _, item := range items {
		if item.Title == title {
			return true
		}
	}
	return false
}
