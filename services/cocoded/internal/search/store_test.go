package search

import (
	"context"
	"database/sql"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestStoreSyncFindingSearchAndUpdates(t *testing.T) {
	t.Parallel()

	database, queries := searchTestDatabase(t)
	store, err := New(database)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	finding := createSearchFinding(t, queries)
	if err := store.SyncFinding(context.Background(), finding); err != nil {
		t.Fatalf("SyncFinding() error = %v", err)
	}

	ids, err := store.SearchFindings(context.Background(), "monotonic", 10)
	if err != nil {
		t.Fatalf("SearchFindings(monotonic) error = %v", err)
	}
	if len(ids) != 1 || ids[0] != "finding_1" {
		t.Fatalf("SearchFindings(monotonic) = %+v, want [finding_1]", ids)
	}

	updated, err := queries.UpdateFinding(context.Background(), dbgen.UpdateFindingParams{
		ID:                     "finding_1",
		CanonicalClaim:         "Artifact hashes persist",
		Category:               "correctness",
		Severity:               "medium",
		Confidence:             0.91,
		PrimaryPath:            sql.NullString{String: "services/cocoded/internal/artifact/store.go", Valid: true},
		PrimaryStartLine:       sql.NullInt64{Int64: 20, Valid: true},
		PrimaryEndLine:         sql.NullInt64{Int64: 40, Valid: true},
		EvidenceSummary:        sql.NullString{String: "sha256 size metadata is stored", Valid: true},
		CounterEvidenceSummary: sql.NullString{String: "none", Valid: true},
		SuggestedFix:           sql.NullString{String: "keep hashing content", Valid: true},
		DraftComment:           sql.NullString{String: "Artifact metadata should retain hashes.", Valid: true},
		MergedFromCount:        1,
		UpdatedAt:              "2026-05-03T00:11:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateFinding() error = %v", err)
	}
	if err := store.SyncFinding(context.Background(), updated); err != nil {
		t.Fatalf("SyncFinding(updated) error = %v", err)
	}

	ids, err = store.SearchFindings(context.Background(), "monotonic", 10)
	if err != nil {
		t.Fatalf("SearchFindings(old term) error = %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("SearchFindings(old term) = %+v, want empty", ids)
	}

	ids, err = store.SearchFindings(context.Background(), "sha256", 10)
	if err != nil {
		t.Fatalf("SearchFindings(sha256) error = %v", err)
	}
	if len(ids) != 1 || ids[0] != "finding_1" {
		t.Fatalf("SearchFindings(sha256) = %+v, want [finding_1]", ids)
	}

	if err := store.DeleteFinding(context.Background(), "finding_1"); err != nil {
		t.Fatalf("DeleteFinding() error = %v", err)
	}
	ids, err = store.SearchFindings(context.Background(), "sha256", 10)
	if err != nil {
		t.Fatalf("SearchFindings(after delete) error = %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("SearchFindings(after delete) = %+v, want empty", ids)
	}
}

func TestStoreSyncEvidenceSearchAndDeletes(t *testing.T) {
	t.Parallel()

	database, queries := searchTestDatabase(t)
	store, err := New(database)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	finding := createSearchFinding(t, queries)
	item, err := queries.CreateEvidenceItem(context.Background(), dbgen.CreateEvidenceItemParams{
		ID:           "evidence_item_1",
		FindingID:    finding.ID,
		Kind:         "supporting",
		Title:        "Router lifecycle evidence",
		Summary:      "The router path supports review lifecycle storage.",
		Path:         sql.NullString{String: "services/cocoded/internal/httpapi/router.go", Valid: true},
		StartLine:    sql.NullInt64{Int64: 1, Valid: true},
		EndLine:      sql.NullInt64{Int64: 25, Valid: true},
		Confidence:   0.8,
		MetadataJson: "{}",
		CreatedAt:    "2026-05-03T00:12:00Z",
	})
	if err != nil {
		t.Fatalf("CreateEvidenceItem() error = %v", err)
	}

	if err := store.SyncEvidenceItem(context.Background(), item); err != nil {
		t.Fatalf("SyncEvidenceItem() error = %v", err)
	}
	ids, err := store.SearchEvidence(context.Background(), "router", 10)
	if err != nil {
		t.Fatalf("SearchEvidence(router) error = %v", err)
	}
	if len(ids) != 1 || ids[0] != "evidence_item_1" {
		t.Fatalf("SearchEvidence(router) = %+v, want [evidence_item_1]", ids)
	}

	updated, err := queries.UpdateEvidenceItem(context.Background(), dbgen.UpdateEvidenceItemParams{
		ID:           "evidence_item_1",
		Kind:         "supporting",
		Title:        "Artifact metadata evidence",
		Summary:      "Hash metadata is stored for artifact content.",
		Path:         sql.NullString{String: "services/cocoded/internal/artifact/store.go", Valid: true},
		StartLine:    sql.NullInt64{Int64: 1, Valid: true},
		EndLine:      sql.NullInt64{Int64: 40, Valid: true},
		Confidence:   0.9,
		MetadataJson: "{}",
	})
	if err != nil {
		t.Fatalf("UpdateEvidenceItem() error = %v", err)
	}
	if err := store.SyncEvidenceItem(context.Background(), updated); err != nil {
		t.Fatalf("SyncEvidenceItem(updated) error = %v", err)
	}

	ids, err = store.SearchEvidence(context.Background(), "router", 10)
	if err != nil {
		t.Fatalf("SearchEvidence(old term) error = %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("SearchEvidence(old term) = %+v, want empty", ids)
	}
	ids, err = store.SearchEvidence(context.Background(), "hash", 10)
	if err != nil {
		t.Fatalf("SearchEvidence(hash) error = %v", err)
	}
	if len(ids) != 1 || ids[0] != "evidence_item_1" {
		t.Fatalf("SearchEvidence(hash) = %+v, want [evidence_item_1]", ids)
	}

	if err := store.DeleteEvidenceItem(context.Background(), "evidence_item_1"); err != nil {
		t.Fatalf("DeleteEvidenceItem() error = %v", err)
	}
	ids, err = store.SearchEvidence(context.Background(), "hash", 10)
	if err != nil {
		t.Fatalf("SearchEvidence(after delete) error = %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("SearchEvidence(after delete) = %+v, want empty", ids)
	}
}

func TestBuildFTSQuery(t *testing.T) {
	t.Parallel()

	got := buildFTSQuery(`  review "quoted" lifecycle  `)
	want := `"review" """quoted""" "lifecycle"`
	if got != want {
		t.Fatalf("buildFTSQuery() = %q, want %q", got, want)
	}
	if got := buildFTSQuery("   "); got != "" {
		t.Fatalf("buildFTSQuery(blank) = %q, want empty", got)
	}
}

func searchTestDatabase(t *testing.T) (*sql.DB, *dbgen.Queries) {
	t.Helper()

	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
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
		ID:          "repo_1",
		WorkspaceID: "workspace_1",
		Name:        "cocode",
		LocalPath:   "/tmp/cocode",
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

	return database, queries
}

func createSearchFinding(t *testing.T, queries *dbgen.Queries) dbgen.Finding {
	t.Helper()

	finding, err := queries.CreateFinding(context.Background(), dbgen.CreateFindingParams{
		ID:                     "finding_1",
		ReviewSessionID:        "review_session_1",
		CanonicalClaim:         "Event log uses monotonic sequence numbers",
		Category:               "correctness",
		Severity:               "high",
		Confidence:             0.88,
		VerificationStatus:     "verified",
		DecisionStatus:         "accepted",
		PrimaryPath:            sql.NullString{String: "services/cocoded/internal/eventlog/store.go", Valid: true},
		PrimaryStartLine:       sql.NullInt64{Int64: 1, Valid: true},
		PrimaryEndLine:         sql.NullInt64{Int64: 80, Valid: true},
		EvidenceSummary:        sql.NullString{String: "session events are appended in order", Valid: true},
		CounterEvidenceSummary: sql.NullString{String: "none", Valid: true},
		SuggestedFix:           sql.NullString{String: "keep sequence assignment transactional", Valid: true},
		DraftComment:           sql.NullString{String: "The event store should keep ordering stable.", Valid: true},
		Fingerprint:            "finding-fingerprint-1",
		MergedFromCount:        1,
		FirstSeenAt:            "2026-05-03T00:10:00Z",
		UpdatedAt:              "2026-05-03T00:10:00Z",
	})
	if err != nil {
		t.Fatalf("CreateFinding() error = %v", err)
	}
	return finding
}
