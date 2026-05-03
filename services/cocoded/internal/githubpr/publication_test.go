package githubpr

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestRecordPublicationStoresGitHubIDsAndMarksFindingsPublished(t *testing.T) {
	database, queries := publicationTestDB(t)
	createPublicationFixture(t, queries)

	result, err := (PublicationService{
		Database: database,
		Queries:  queries,
		Now:      func() time.Time { return time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC) },
		NewID:    sequenceID(),
	}).RecordPublication(context.Background(), RecordPublicationParams{
		PublishDraftID:     "publish_draft_1",
		GitHubReviewID:     "7001",
		GitHubCommentIDs:   []int64{101, 102},
		Status:             "published",
		UpdateFindingState: true,
	})
	if err != nil {
		t.Fatalf("RecordPublication() error = %v", err)
	}
	if result.Publication.GithubReviewID.String != "7001" ||
		result.Publication.GithubCommentIdsJson != `[101,102]` ||
		len(result.Decisions) != 2 ||
		len(result.FindingIDs) != 2 {
		t.Fatalf("result = %+v", result)
	}
	for _, findingID := range []string{"finding_auth", "finding_budget"} {
		finding, err := queries.GetFinding(context.Background(), findingID)
		if err != nil {
			t.Fatalf("GetFinding(%s) error = %v", findingID, err)
		}
		if finding.DecisionStatus != "published" {
			t.Fatalf("finding %s = %+v", findingID, finding)
		}
	}
	decisions, err := queries.ListHumanDecisionsByFinding(context.Background(), "finding_auth")
	if err != nil {
		t.Fatalf("ListHumanDecisionsByFinding() error = %v", err)
	}
	if len(decisions) != 1 ||
		decisions[0].Decision != "published" ||
		decisions[0].Reason.String != "github_review" {
		t.Fatalf("decisions = %+v", decisions)
	}
}

func TestRecordPublicationDoesNotPublishFindingsForFailedStatus(t *testing.T) {
	database, queries := publicationTestDB(t)
	createPublicationFixture(t, queries)

	result, err := (PublicationService{Database: database, Queries: queries}).RecordPublication(context.Background(), RecordPublicationParams{
		PublishDraftID:     "publish_draft_1",
		Status:             "failed",
		ErrorMessage:       "GitHub rejected review",
		UpdateFindingState: true,
	})
	if err != nil {
		t.Fatalf("RecordPublication() error = %v", err)
	}
	if result.Publication.Status != "failed" || len(result.Decisions) != 0 {
		t.Fatalf("result = %+v", result)
	}
	finding, err := queries.GetFinding(context.Background(), "finding_auth")
	if err != nil {
		t.Fatalf("GetFinding() error = %v", err)
	}
	if finding.DecisionStatus != "accepted" {
		t.Fatalf("finding = %+v", finding)
	}
}

func TestRecordPublicationRequiresAcceptedFindingsBeforePublishing(t *testing.T) {
	database, queries := publicationTestDB(t)
	createPublicationFixture(t, queries)
	if _, err := queries.UpdateFindingDecisionStatus(context.Background(), dbgen.UpdateFindingDecisionStatusParams{
		ID:             "finding_budget",
		DecisionStatus: "undecided",
		UpdatedAt:      "2026-05-03T00:10:00Z",
	}); err != nil {
		t.Fatalf("UpdateFindingDecisionStatus() error = %v", err)
	}

	_, err := (PublicationService{
		Database: database,
		Queries:  queries,
		Now:      func() time.Time { return time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC) },
		NewID:    sequenceID(),
	}).RecordPublication(context.Background(), RecordPublicationParams{
		PublishDraftID:     "publish_draft_1",
		GitHubReviewID:     "7001",
		GitHubCommentIDs:   []int64{101, 102},
		Status:             "published",
		UpdateFindingState: true,
	})
	if !errors.Is(err, ErrPublicationInvalid) {
		t.Fatalf("RecordPublication() error = %v, want ErrPublicationInvalid", err)
	}
	for findingID, want := range map[string]string{
		"finding_auth":   "accepted",
		"finding_budget": "undecided",
	} {
		finding, err := queries.GetFinding(context.Background(), findingID)
		if err != nil {
			t.Fatalf("GetFinding(%s) error = %v", findingID, err)
		}
		if finding.DecisionStatus != want {
			t.Fatalf("finding %s = %+v", findingID, finding)
		}
	}
}

func publicationTestDB(t *testing.T) (*sql.DB, *dbgen.Queries) {
	t.Helper()
	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	return database, dbgen.New(database)
}

func createPublicationFixture(t *testing.T, queries *dbgen.Queries) {
	t.Helper()
	ctx := context.Background()
	if _, err := queries.CreateWorkspace(ctx, dbgen.CreateWorkspaceParams{
		ID:           "workspace_1",
		Name:         "cocode",
		RootPath:     "/tmp/cocode",
		SettingsJson: "{}",
		CreatedAt:    "2026-05-03T00:00:00Z",
		UpdatedAt:    "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, err := queries.CreateRepository(ctx, dbgen.CreateRepositoryParams{
		ID:          "repo_1",
		WorkspaceID: "workspace_1",
		Name:        "cocode",
		LocalPath:   "/tmp/cocode",
		CreatedAt:   "2026-05-03T00:00:00Z",
		UpdatedAt:   "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
	if _, err := queries.CreatePullRequestSnapshot(ctx, dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_1",
		RepositoryID: "repo_1",
		SourceType:   "github_pr",
		MetadataJson: "{}",
		CreatedAt:    "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}
	if _, err := queries.CreateReviewSession(ctx, dbgen.CreateReviewSessionParams{
		ID:                "review_session_1",
		WorkspaceID:       "workspace_1",
		RepositoryID:      "repo_1",
		SnapshotID:        "snapshot_1",
		Title:             "Review",
		Status:            "completed",
		ReviewDepth:       "standard",
		ContextPolicyJson: "{}",
		CreatedAt:         "2026-05-03T00:00:00Z",
		UpdatedAt:         "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateReviewSession() error = %v", err)
	}
	for _, finding := range []struct {
		id       string
		claim    string
		decision string
	}{
		{"finding_auth", "Auth guard is missing.", "accepted"},
		{"finding_budget", "Diff preview is too large.", "accepted"},
	} {
		if _, err := queries.CreateFinding(ctx, dbgen.CreateFindingParams{
			ID:                 finding.id,
			ReviewSessionID:    "review_session_1",
			CanonicalClaim:     finding.claim,
			Category:           "security",
			Severity:           "high",
			Confidence:         0.9,
			VerificationStatus: "verified",
			DecisionStatus:     finding.decision,
			PrimaryPath:        sql.NullString{String: "app/auth.go", Valid: true},
			PrimaryStartLine:   sql.NullInt64{Int64: 12, Valid: true},
			PrimaryEndLine:     sql.NullInt64{Int64: 12, Valid: true},
			Fingerprint:        finding.id,
			FirstSeenAt:        "2026-05-03T00:00:00Z",
			UpdatedAt:          "2026-05-03T00:00:00Z",
		}); err != nil {
			t.Fatalf("CreateFinding(%s) error = %v", finding.id, err)
		}
	}
	comments, err := json.Marshal([]ReviewCommentDraft{
		{FindingID: "finding_auth", Path: "app/auth.go", Body: "Fix auth.", Line: 12, Side: SideRight},
		{FindingID: "finding_budget", Path: "app/ui.go", Body: "Fix UI.", Line: 30, Side: SideRight},
	})
	if err != nil {
		t.Fatalf("marshal comments: %v", err)
	}
	if _, err := queries.CreatePublishDraft(ctx, dbgen.CreatePublishDraftParams{
		ID:              "publish_draft_1",
		ReviewSessionID: "review_session_1",
		Provider:        "github",
		Status:          "draft",
		ReviewEvent:     sql.NullString{String: "COMMENT", Valid: true},
		Body:            sql.NullString{String: "Body", Valid: true},
		CommentsJson:    string(comments),
		CreatedAt:       "2026-05-03T00:00:00Z",
		UpdatedAt:       "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreatePublishDraft() error = %v", err)
	}
}

func sequenceID() func(string) string {
	next := 0
	return func(prefix string) string {
		next++
		return prefix + string(rune('a'+next))
	}
}
