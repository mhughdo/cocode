package contextbundle

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestBundleFromRowsMapsDBRowsAndRenderedArtifact(t *testing.T) {
	t.Parallel()

	queries := contextBundleTestQueries(t)
	store, err := artifact.New(filepath.Join(t.TempDir(), "artifacts"), queries)
	if err != nil {
		t.Fatalf("artifact.New() error = %v", err)
	}

	rendered := []byte("# Review context\n\nChanged auth route and tests.\n")
	renderedArtifact, err := store.Save(context.Background(), artifact.SaveParams{
		ID:              "artifact_context_rendered",
		WorkspaceID:     "workspace_1",
		ReviewSessionID: sql.NullString{String: "review_session_1", Valid: true},
		Kind:            "context_bundle",
		RelativePath:    "review_session_1/context/review.md",
		ContentType:     "text/markdown",
		MetadataJSON:    `{"scope":"review"}`,
		CreatedAt:       "2026-05-03T00:10:00Z",
	}, rendered)
	if err != nil {
		t.Fatalf("Save(rendered context) error = %v", err)
	}
	itemArtifact, err := store.Save(context.Background(), artifact.SaveParams{
		ID:              "artifact_context_item",
		WorkspaceID:     "workspace_1",
		ReviewSessionID: sql.NullString{String: "review_session_1", Valid: true},
		Kind:            "context_item",
		RelativePath:    "review_session_1/context/items/auth-route.patch",
		ContentType:     "text/x-diff",
		MetadataJSON:    `{"path":"apps/api/auth.ts"}`,
		CreatedAt:       "2026-05-03T00:10:01Z",
	}, []byte("@@ -1 +1 @@\n+requireAdmin()\n"))
	if err != nil {
		t.Fatalf("Save(context item) error = %v", err)
	}

	bundleRow, err := queries.CreateContextBundle(context.Background(), dbgen.CreateContextBundleParams{
		ID:              "bundle_1",
		ReviewSessionID: "review_session_1",
		Scope:           string(ScopeReview),
		TokenEstimate:   42,
		ItemCount:       1,
		ArtifactID:      sql.NullString{String: renderedArtifact.ID, Valid: true},
		PolicyJson:      `{"max_tokens":18000,"include_tests":true}`,
		CreatedAt:       "2026-05-03T00:11:00Z",
	})
	if err != nil {
		t.Fatalf("CreateContextBundle() error = %v", err)
	}
	if _, err := queries.CreateContextItem(context.Background(), dbgen.CreateContextItemParams{
		ID:                "item_1",
		ContextBundleID:   "bundle_1",
		Kind:              string(ItemChangedHunk),
		Path:              sql.NullString{String: "apps/api/auth.ts", Valid: true},
		StartLine:         sql.NullInt64{Int64: 12, Valid: true},
		EndLine:           sql.NullInt64{Int64: 18, Valid: true},
		Title:             sql.NullString{String: "Auth route diff", Valid: true},
		ContentArtifactID: sql.NullString{String: itemArtifact.ID, Valid: true},
		TokenEstimate:     37,
		MetadataJson:      `{"source":"diff","changed_file_id":"changed_file_1"}`,
	}); err != nil {
		t.Fatalf("CreateContextItem() error = %v", err)
	}

	itemRows, err := queries.ListContextItemsByBundle(context.Background(), "bundle_1")
	if err != nil {
		t.Fatalf("ListContextItemsByBundle() error = %v", err)
	}
	bundle, err := BundleFromRows(bundleRow, itemRows)
	if err != nil {
		t.Fatalf("BundleFromRows() error = %v", err)
	}
	if bundle.ID != "bundle_1" ||
		bundle.Scope != ScopeReview ||
		bundle.ArtifactID != renderedArtifact.ID ||
		string(bundle.Policy) != `{"max_tokens":18000,"include_tests":true}` ||
		len(bundle.Items) != 1 {
		t.Fatalf("bundle = %+v", bundle)
	}
	item := bundle.Items[0]
	if item.Kind != ItemChangedHunk ||
		item.Path != "apps/api/auth.ts" ||
		item.StartLine != 12 ||
		item.EndLine != 18 ||
		item.ContentArtifactID != itemArtifact.ID ||
		string(item.Metadata) != `{"source":"diff","changed_file_id":"changed_file_1"}` {
		t.Fatalf("item = %+v", item)
	}

	updated, err := queries.UpdateContextBundleArtifact(context.Background(), dbgen.UpdateContextBundleArtifactParams{
		ID:            "bundle_1",
		ArtifactID:    sql.NullString{String: renderedArtifact.ID, Valid: true},
		TokenEstimate: 64,
		ItemCount:     1,
	})
	if err != nil {
		t.Fatalf("UpdateContextBundleArtifact() error = %v", err)
	}
	if updated.TokenEstimate != 64 || updated.ArtifactID.String != renderedArtifact.ID {
		t.Fatalf("UpdateContextBundleArtifact() = %+v", updated)
	}

	content, artifactRow, err := store.Read(context.Background(), renderedArtifact.ID)
	if err != nil {
		t.Fatalf("Read(rendered context) error = %v", err)
	}
	if artifactRow.Kind != "context_bundle" || !bytes.Equal(content, rendered) {
		t.Fatalf("rendered artifact = %+v content %q", artifactRow, string(content))
	}

	if err := queries.DeleteContextBundle(context.Background(), "bundle_1"); err != nil {
		t.Fatalf("DeleteContextBundle() error = %v", err)
	}
	if _, err := queries.GetContextItem(context.Background(), "item_1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetContextItem(deleted bundle child) error = %v, want sql.ErrNoRows", err)
	}
}

func TestBundleFromRowsRejectsInvalidRows(t *testing.T) {
	t.Parallel()

	validBundle := dbgen.ContextBundle{
		ID:              "bundle_1",
		ReviewSessionID: "review_session_1",
		Scope:           string(ScopeReview),
		PolicyJson:      "{}",
	}
	validItem := dbgen.ContextItem{
		ID:              "item_1",
		ContextBundleID: "bundle_1",
		Kind:            string(ItemChangedHunk),
		MetadataJson:    "{}",
	}

	if _, err := BundleFromRows(dbgen.ContextBundle{
		ID:              "bundle_1",
		ReviewSessionID: "review_session_1",
		Scope:           "bad_scope",
		PolicyJson:      "{}",
	}, nil); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("BundleFromRows(invalid scope) error = %v", err)
	}
	if _, err := BundleFromRows(dbgen.ContextBundle{
		ID:              "bundle_1",
		ReviewSessionID: "review_session_1",
		Scope:           string(ScopeReview),
		PolicyJson:      "{",
	}, nil); err == nil || !strings.Contains(err.Error(), "policy") {
		t.Fatalf("BundleFromRows(invalid policy) error = %v", err)
	}
	invalidLineItem := validItem
	invalidLineItem.StartLine = sql.NullInt64{Int64: 20, Valid: true}
	invalidLineItem.EndLine = sql.NullInt64{Int64: 10, Valid: true}
	if _, err := BundleFromRows(validBundle, []dbgen.ContextItem{invalidLineItem}); err == nil || !strings.Contains(err.Error(), "line range") {
		t.Fatalf("BundleFromRows(invalid line range) error = %v", err)
	}
	mismatchedItem := validItem
	mismatchedItem.ContextBundleID = "bundle_2"
	if _, err := BundleFromRows(validBundle, []dbgen.ContextItem{mismatchedItem}); err == nil || !strings.Contains(err.Error(), "belongs to bundle") {
		t.Fatalf("BundleFromRows(mismatched item) error = %v", err)
	}
}

func contextBundleTestQueries(t *testing.T) *dbgen.Queries {
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
		RootPath:     "/tmp/cocode",
		SettingsJson: "{}",
		CreatedAt:    "2026-05-03T00:00:00Z",
		UpdatedAt:    "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, err := queries.CreateRepository(context.Background(), dbgen.CreateRepositoryParams{
		ID:            "repo_1",
		WorkspaceID:   "workspace_1",
		Name:          "cocode",
		LocalPath:     "/tmp/cocode",
		DefaultBranch: sql.NullString{String: "main", Valid: true},
		CreatedAt:     "2026-05-03T00:01:00Z",
		UpdatedAt:     "2026-05-03T00:01:00Z",
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
		Title:               "Review cocode",
		Status:              "draft",
		ReviewDepth:         "standard",
		RuntimeLimitSeconds: 1800,
		ContextPolicyJson:   "{}",
		CreatedAt:           "2026-05-03T00:03:00Z",
		UpdatedAt:           "2026-05-03T00:03:00Z",
	}); err != nil {
		t.Fatalf("CreateReviewSession() error = %v", err)
	}
	return queries
}
