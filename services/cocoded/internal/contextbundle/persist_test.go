package contextbundle

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestRenderBundleIncludesItemsAndTotals(t *testing.T) {
	t.Parallel()

	rendered := RenderBundle(Bundle{
		ID:              "bundle_1",
		ReviewSessionID: "review_session_1",
		Scope:           ScopeReview,
		Policy:          []byte(`{"max_tokens":12000}`),
		Items: []Item{
			{
				ID:              "item_hunk",
				ContextBundleID: "bundle_1",
				Kind:            ItemChangedHunk,
				Path:            "apps/api/auth.ts",
				StartLine:       12,
				EndLine:         18,
				Title:           "Auth route diff",
				Content:         "@@ -12,6 +12,7 @@\n+requireAdmin()\n",
				Metadata:        []byte(`{"source":"diff"}`),
			},
			{
				ID:              "item_rule",
				ContextBundleID: "bundle_1",
				Kind:            ItemProjectRule,
				Path:            "README.md",
				Title:           "Project conventions",
				Content:         "Run focused tests before broad suites.",
				Metadata:        []byte(`{"source":"project_rules"}`),
			},
		},
	})

	for _, want := range []string{
		"# Context Bundle",
		"Bundle ID: bundle_1",
		"Scope: review",
		"Token estimate: ",
		"Item count: 2",
		"## 01. changed_hunk - Auth route diff",
		"Path: apps/api/auth.ts:12-18",
		"Item ID: item_hunk",
		"requireAdmin()",
		"## 02. project_rule - Project conventions",
		"Run focused tests before broad suites.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("RenderBundle() missing %q in:\n%s", want, rendered)
		}
	}
}

func TestPersisterPersistsRenderedBundleArtifactAndItems(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queries := contextBundleTestQueries(t)
	store, err := artifact.New(filepath.Join(t.TempDir(), "artifacts"), queries)
	if err != nil {
		t.Fatalf("artifact.New() error = %v", err)
	}

	result, err := Persister{Queries: queries, Artifacts: store}.PersistRenderedBundle(ctx, PersistParams{
		WorkspaceID: "workspace_1",
		ArtifactID:  "artifact_context_bundle_1",
		CreatedAt:   "2026-05-03T00:20:00Z",
		Bundle: Bundle{
			ID:              "bundle_persist",
			ReviewSessionID: "review_session_1",
			Scope:           ScopeReview,
			Policy:          []byte(`{"max_tokens":12000,"include_related_tests":true}`),
			Items: []Item{
				{
					ID:              "item_persist_hunk",
					ContextBundleID: "bundle_persist",
					Kind:            ItemChangedHunk,
					Path:            "apps/api/auth.ts",
					StartLine:       12,
					EndLine:         18,
					Title:           "Auth route diff",
					Content:         "@@ -12,6 +12,7 @@\n+requireAdmin()\n",
					Metadata:        []byte(`{"source":"diff","changed_file_id":"changed_file_1"}`),
				},
				{
					ID:              "item_persist_test",
					ContextBundleID: "bundle_persist",
					Kind:            ItemRelatedTest,
					Path:            "apps/api/auth.test.ts",
					StartLine:       1,
					EndLine:         42,
					Title:           "Related auth tests",
					Content:         "describe('auth route', () => {})",
					Metadata:        []byte(`{"source":"related_tests"}`),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("PersistRenderedBundle() error = %v", err)
	}

	if result.Bundle.ID != "bundle_persist" ||
		result.Bundle.ArtifactID != result.Artifact.ID ||
		result.Bundle.ItemCount != 2 ||
		result.Bundle.TokenEstimate <= 0 ||
		len(result.Items) != 2 {
		t.Fatalf("persisted result = %+v", result)
	}
	if result.Artifact.Kind != "context_bundle" ||
		result.Artifact.ContentType != "text/markdown" ||
		result.Artifact.RelativePath != "context/bundle_persist/context.md" ||
		result.Artifact.ReviewSessionID.String != "review_session_1" {
		t.Fatalf("artifact = %+v", result.Artifact)
	}

	content, artifactRow, err := store.Read(ctx, result.Artifact.ID)
	if err != nil {
		t.Fatalf("Read(context artifact) error = %v", err)
	}
	if artifactRow.SizeBytes == 0 ||
		!strings.Contains(string(content), "## 01. changed_hunk - Auth route diff") ||
		!strings.Contains(string(content), "describe('auth route'") {
		t.Fatalf("artifact content = %q row = %+v", string(content), artifactRow)
	}

	itemRows, err := queries.ListContextItemsByBundle(ctx, result.Bundle.ID)
	if err != nil {
		t.Fatalf("ListContextItemsByBundle() error = %v", err)
	}
	if len(itemRows) != 2 ||
		itemRows[0].ContextBundleID != result.Bundle.ID ||
		itemRows[0].TokenEstimate <= 0 ||
		itemRows[0].Path.String != "apps/api/auth.ts" {
		t.Fatalf("context item rows = %+v", itemRows)
	}

	if _, err := queries.CreateAgentConfig(ctx, dbgen.CreateAgentConfigParams{
		ID:               "agent_config_1",
		Name:             "Codex reviewer",
		Role:             "reviewer",
		AdapterKind:      "cli_noninteractive",
		Command:          sql.NullString{String: "codex", Valid: true},
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       "json",
		ModelLabel:       sql.NullString{String: "latest", Valid: true},
		ReasoningLabel:   sql.NullString{String: "medium", Valid: true},
		CapabilitiesJson: `{"can_read":true}`,
		SettingsJson:     "{}",
		Enabled:          1,
		CreatedAt:        "2026-05-03T00:21:00Z",
		UpdatedAt:        "2026-05-03T00:21:00Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig() error = %v", err)
	}
	run, err := queries.CreateAgentRun(ctx, dbgen.CreateAgentRunParams{
		ID:              "agent_run_1",
		ReviewSessionID: "review_session_1",
		AgentConfigID:   "agent_config_1",
		ContextBundleID: sql.NullString{String: result.Bundle.ID, Valid: true},
		Status:          "queued",
		Role:            "reviewer",
		MetadataJson:    "{}",
	})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if !run.ContextBundleID.Valid || run.ContextBundleID.String != result.Bundle.ID {
		t.Fatalf("agent run context bundle id = %+v, want %q", run.ContextBundleID, result.Bundle.ID)
	}
}

func TestPersisterValidatesInputs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queries := contextBundleTestQueries(t)
	store, err := artifact.New(filepath.Join(t.TempDir(), "artifacts"), queries)
	if err != nil {
		t.Fatalf("artifact.New() error = %v", err)
	}
	validBundle := Bundle{
		ID:              "bundle_valid",
		ReviewSessionID: "review_session_1",
		Scope:           ScopeReview,
		Policy:          []byte(`{}`),
	}

	tests := []struct {
		name      string
		persister Persister
		params    PersistParams
		want      string
	}{
		{
			name:      "missing queries",
			persister: Persister{Artifacts: store},
			params:    PersistParams{WorkspaceID: "workspace_1", Bundle: validBundle},
			want:      "queries",
		},
		{
			name:      "missing artifact store",
			persister: Persister{Queries: queries},
			params:    PersistParams{WorkspaceID: "workspace_1", Bundle: validBundle},
			want:      "artifact store",
		},
		{
			name:      "missing workspace",
			persister: Persister{Queries: queries, Artifacts: store},
			params:    PersistParams{Bundle: validBundle},
			want:      "workspace id",
		},
		{
			name:      "invalid bundle",
			persister: Persister{Queries: queries, Artifacts: store},
			params: PersistParams{WorkspaceID: "workspace_1", Bundle: Bundle{
				ID:              "bundle_invalid",
				ReviewSessionID: "review_session_1",
				Scope:           "bad_scope",
				Policy:          []byte(`{}`),
			}},
			want: "scope",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := tt.persister.PersistRenderedBundle(ctx, tt.params); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("PersistRenderedBundle() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
