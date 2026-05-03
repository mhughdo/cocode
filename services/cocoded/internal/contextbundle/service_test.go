package contextbundle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestServiceBuildReviewContextPreviewsAndPersists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repoPath := t.TempDir()
	writeRepoFile(t, repoPath, "app/main.go", "package main\n\nconst apiKey = \"sk-abcdefghijklmnopqrstuvwxyz\"\n\nfunc RequireAdmin() {}\n")
	writeRepoFile(t, repoPath, "app/main_test.go", "package main\n\nfunc TestRequireAdmin(t *testing.T) {\n\tRequireAdmin()\n}\n")
	writeRepoFile(t, repoPath, "CODEOWNERS", "* @team/review\n")

	queries, store := contextBuilderTestStore(t, repoPath)
	patch, err := store.Save(ctx, artifact.SaveParams{
		ID:           "artifact_patch_main",
		WorkspaceID:  "workspace_1",
		Kind:         "patch",
		RelativePath: "snapshots/snapshot_1/patches/main.patch",
		ContentType:  "text/x-diff",
		MetadataJSON: `{"path":"app/main.go"}`,
		CreatedAt:    "2026-05-03T00:04:00Z",
	}, []byte("@@ -1,3 +1,5 @@\n package main\n+const apiKey = \"sk-abcdefghijklmnopqrstuvwxyz\"\n+func RequireAdmin() {}\n"))
	if err != nil {
		t.Fatalf("Save(patch) error = %v", err)
	}
	if _, err := queries.CreateChangedFile(ctx, dbgen.CreateChangedFileParams{
		ID:              "file_main",
		SnapshotID:      "snapshot_1",
		Path:            "app/main.go",
		Status:          "modified",
		Additions:       2,
		LineRangesJson:  `[[3,5]]`,
		PatchArtifactID: sql.NullString{String: patch.ID, Valid: true},
		CreatedAt:       "2026-05-03T00:05:00Z",
	}); err != nil {
		t.Fatalf("CreateChangedFile() error = %v", err)
	}
	if _, err := queries.CreateReviewRule(ctx, dbgen.CreateReviewRuleParams{
		ID:          "rule_1",
		WorkspaceID: "workspace_1",
		Scope:       "go",
		RuleType:    "preference",
		Content:     "Prefer table-driven tests for branchy logic.",
		Enabled:     1,
		CreatedAt:   "2026-05-03T00:06:00Z",
		UpdatedAt:   "2026-05-03T00:06:00Z",
	}); err != nil {
		t.Fatalf("CreateReviewRule() error = %v", err)
	}

	service := Service{
		Queries:   queries,
		Artifacts: store,
		Searcher: fakeReviewContextSearcher{matches: []CodeSearchMatch{
			{Path: "internal/auth/guard.go", Line: 7, Text: "RequireAdmin()"},
		}},
		Now: func() time.Time {
			return time.Date(2026, 5, 3, 0, 7, 0, 0, time.UTC)
		},
	}
	result, err := service.BuildReviewContext(ctx, BuildReviewContextParams{
		ReviewSessionID: "review_session_1",
		Persist:         true,
		PolicyOverride:  json.RawMessage(`{"max_tokens":50000,"max_items":80}`),
	})
	if err != nil {
		t.Fatalf("BuildReviewContext() error = %v", err)
	}

	if !result.Persisted ||
		result.Bundle.ID == "" ||
		result.Bundle.ArtifactID == "" ||
		result.Artifact.Kind != "context_bundle" ||
		result.RedactionReport.RedactionCount == 0 ||
		result.RedactionReportArtifact.Kind != "context_redaction_report" ||
		result.Bundle.ItemCount != int64(len(result.Bundle.Items)) ||
		result.Bundle.TokenEstimate <= 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Dropped) != 0 {
		t.Fatalf("dropped = %+v", result.Dropped)
	}

	kinds := itemKinds(result.Bundle.Items)
	for _, want := range []ItemKind{ItemPromptMaterial, ItemChangedHunk, ItemFullFile, ItemRelatedCode, ItemRelatedTest, ItemProjectRule, ItemPriorDecision} {
		if !slices.Contains(kinds, want) {
			t.Fatalf("item kinds = %+v, missing %s", kinds, want)
		}
	}
	rendered, _, err := store.Read(ctx, result.Artifact.ID)
	if err != nil {
		t.Fatalf("Read(rendered context) error = %v", err)
	}
	if strings.Contains(string(rendered), "sk-abcdefghijklmnopqrstuvwxyz") ||
		!strings.Contains(string(rendered), "[REDACTED]") ||
		!strings.Contains(string(rendered), "Review session: Deep review") {
		t.Fatalf("rendered context = %s", string(rendered))
	}

	rows, err := queries.ListContextBundlesBySession(ctx, "review_session_1")
	if err != nil {
		t.Fatalf("ListContextBundlesBySession() error = %v", err)
	}
	if len(rows) != 1 || rows[0].ArtifactID.String != result.Artifact.ID {
		t.Fatalf("context bundle rows = %+v", rows)
	}
}

func TestServiceBuildReviewContextReturnsTypedErrors(t *testing.T) {
	t.Parallel()

	queries, store := contextBuilderTestStore(t, t.TempDir())
	service := Service{Queries: queries, Artifacts: store}
	if _, err := service.BuildReviewContext(context.Background(), BuildReviewContextParams{
		ReviewSessionID: "missing_session",
	}); !errors.Is(err, ErrReviewSessionNotFound) {
		t.Fatalf("BuildReviewContext(missing session) error = %v", err)
	}
	if _, err := service.BuildReviewContext(context.Background(), BuildReviewContextParams{
		ReviewSessionID: "review_session_1",
		PolicyOverride:  json.RawMessage(`{"max_tokens":0}`),
	}); !errors.Is(err, ErrInvalidReviewContextPolicy) {
		t.Fatalf("BuildReviewContext(invalid policy) error = %v", err)
	}
}

func TestServiceBuildReviewContextEnforcesLocalOnlyPathsForExternalAgent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repoPath := t.TempDir()
	writeRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc Public() {}\n")
	writeRepoFile(t, repoPath, "config/local.yaml", "token: local-only-token\n")

	queries, store := contextBuilderTestStore(t, repoPath)
	createContextBundleTestAgentConfig(t, queries, "agent_config_external", agents.AdapterCLINonInteractive, `{"provider":"openai","egress":"external"}`)
	createContextBundlePatch(t, store, queries, "artifact_patch_public", "file_public", "app/main.go", "@@ -1,2 +1,3 @@\n package main\n+func Public() {}\n")
	createContextBundlePatch(t, store, queries, "artifact_patch_local", "file_local", "config/local.yaml", "@@ -1 +1 @@\n-token: old\n+token: local-only-token\n")

	service := Service{
		Queries:   queries,
		Artifacts: store,
		Now: func() time.Time {
			return time.Date(2026, 5, 3, 0, 8, 0, 0, time.UTC)
		},
	}
	result, err := service.BuildReviewContext(ctx, BuildReviewContextParams{
		ReviewSessionID: "review_session_1",
		AgentConfigID:   "agent_config_external",
		Persist:         true,
		PolicyOverride: json.RawMessage(`{
			"local_only_paths":["config"],
			"include_related_call_sites":false,
			"include_related_tests":false,
			"include_project_conventions":false,
			"include_prior_comments":false,
			"include_prior_decisions":false,
			"max_tokens":50000,
			"max_items":80
		}`),
	})
	if err != nil {
		t.Fatalf("BuildReviewContext() error = %v", err)
	}
	if !result.VisibilityReport.LocalOnlyEnforced ||
		result.VisibilityReport.Recipient.Egress != agents.AgentEgressExternal ||
		len(result.VisibilityReport.Omitted) == 0 {
		t.Fatalf("visibility report = %+v", result.VisibilityReport)
	}
	for _, item := range result.Bundle.Items {
		if strings.HasPrefix(item.Path, "config/") ||
			strings.Contains(item.Content, "local-only-token") ||
			strings.Contains(string(item.Metadata), "config/local.yaml") {
			t.Fatalf("local-only item leaked into bundle: %+v", item)
		}
	}
	rendered, _, err := store.Read(ctx, result.Artifact.ID)
	if err != nil {
		t.Fatalf("Read(context artifact) error = %v", err)
	}
	if strings.Contains(string(rendered), "config/local.yaml") ||
		strings.Contains(string(rendered), "local-only-token") ||
		!strings.Contains(string(rendered), "Local-only paths configured: 1") {
		t.Fatalf("rendered context = %s", string(rendered))
	}
	report, ok := VisibilityReportFromArtifactMetadata(json.RawMessage(result.Artifact.MetadataJson))
	if !ok ||
		!report.LocalOnlyEnforced ||
		report.Recipient.Provider != "openai" ||
		report.SentItemCount != len(result.Bundle.Items) {
		t.Fatalf("artifact visibility report = %+v ok=%v", report, ok)
	}
}

func contextBuilderTestStore(t *testing.T, repoPath string) (*dbgen.Queries, *artifact.Store) {
	t.Helper()

	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	queries := dbgen.New(database)
	if _, err := queries.CreateWorkspace(context.Background(), dbgen.CreateWorkspaceParams{
		ID:           "workspace_1",
		Name:         "cocode",
		RootPath:     repoPath,
		SettingsJson: "{}",
		CreatedAt:    "2026-05-03T00:00:00Z",
		UpdatedAt:    "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, err := queries.CreateRepository(context.Background(), dbgen.CreateRepositoryParams{
		ID:          "repo_1",
		WorkspaceID: "workspace_1",
		Name:        "cocode",
		LocalPath:   repoPath,
		CreatedAt:   "2026-05-03T00:01:00Z",
		UpdatedAt:   "2026-05-03T00:01:00Z",
	}); err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
	if _, err := queries.CreatePullRequestSnapshot(context.Background(), dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_1",
		RepositoryID: "repo_1",
		SourceType:   "local_changes",
		MetadataJson: "{}",
		CreatedAt:    "2026-05-03T00:02:00Z",
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}
	if _, err := queries.CreateReviewSession(context.Background(), dbgen.CreateReviewSessionParams{
		ID:                  "review_session_1",
		WorkspaceID:         "workspace_1",
		RepositoryID:        "repo_1",
		SnapshotID:          "snapshot_1",
		Title:               "Deep review",
		Status:              "draft",
		ReviewDepth:         "deep",
		FocusPrompt:         sql.NullString{String: "Focus auth and data integrity.", Valid: true},
		RuntimeLimitSeconds: 1800,
		ContextPolicyJson:   "{}",
		CreatedAt:           "2026-05-03T00:03:00Z",
		UpdatedAt:           "2026-05-03T00:03:00Z",
	}); err != nil {
		t.Fatalf("CreateReviewSession() error = %v", err)
	}
	store, err := artifact.New(filepath.Join(t.TempDir(), "artifacts"), queries)
	if err != nil {
		t.Fatalf("artifact.New() error = %v", err)
	}
	return queries, store
}

func createContextBundleTestAgentConfig(t *testing.T, queries *dbgen.Queries, id string, kind agents.AdapterKind, metadata string) {
	t.Helper()

	if _, err := queries.CreateAgentConfig(context.Background(), dbgen.CreateAgentConfigParams{
		ID:               id,
		Name:             id,
		Role:             "primary_reviewer",
		AdapterKind:      string(kind),
		Command:          sql.NullString{String: "fake-agent", Valid: true},
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       string(agents.OutputJSON),
		CapabilitiesJson: `{"supports_json":true,"can_read":true,"output_modes":["json"],"metadata":` + metadata + `}`,
		SettingsJson:     "{}",
		Enabled:          1,
		CreatedAt:        "2026-05-03T00:06:30Z",
		UpdatedAt:        "2026-05-03T00:06:30Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig(%s) error = %v", id, err)
	}
}

func createContextBundlePatch(t *testing.T, store *artifact.Store, queries *dbgen.Queries, artifactID string, fileID string, path string, patchContent string) {
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
		LineRangesJson:  `[[1,3]]`,
		PatchArtifactID: sql.NullString{String: patch.ID, Valid: true},
		CreatedAt:       "2026-05-03T00:05:00Z",
	}); err != nil {
		t.Fatalf("CreateChangedFile(%s) error = %v", fileID, err)
	}
}

type fakeReviewContextSearcher struct {
	matches []CodeSearchMatch
}

func (s fakeReviewContextSearcher) Search(_ context.Context, _ string, _ string, _ int) ([]CodeSearchMatch, error) {
	return append([]CodeSearchMatch(nil), s.matches...), nil
}

func itemKinds(items []Item) []ItemKind {
	kinds := make([]ItemKind, 0, len(items))
	for _, item := range items {
		kinds = append(kinds, item.Kind)
	}
	return kinds
}
