package eventbus

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/eventlog"
)

func TestBusAppendsPersistsAndBroadcastsEvents(t *testing.T) {
	t.Parallel()

	database, queries := setupEventBusDB(t)
	store, err := eventlog.New(database)
	if err != nil {
		t.Fatalf("eventlog.New() error = %v", err)
	}
	bus, err := New(store)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	events, unsubscribe, err := bus.Subscribe("review_session_1")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	appended, err := bus.Append(context.Background(), eventlog.AppendParams{
		ID:              "event_1",
		ReviewSessionID: "review_session_1",
		Type:            "ReviewSessionStarted",
		PayloadJSON:     `{"phase":"starting"}`,
		CreatedAt:       "2026-05-03T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if appended.Sequence != 1 {
		t.Fatalf("sequence = %d, want 1", appended.Sequence)
	}
	select {
	case got := <-events:
		if got.ID != appended.ID || got.Sequence != appended.Sequence {
			t.Fatalf("broadcast event = %+v, want %+v", got, appended)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast event")
	}
	persisted, err := bus.ListByReviewSession(context.Background(), "review_session_1")
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	if len(persisted) != 1 || persisted[0].ID != "event_1" {
		t.Fatalf("persisted events = %+v", persisted)
	}

	unsubscribe()
	if _, err := bus.Append(context.Background(), eventlog.AppendParams{
		ID:              "event_2",
		ReviewSessionID: "review_session_1",
		Type:            "ReviewSessionCompleted",
		CreatedAt:       "2026-05-03T00:00:01Z",
	}); err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}
	if _, ok := <-events; ok {
		t.Fatal("subscriber channel should be closed after unsubscribe")
	}
	_ = queries
}

func setupEventBusDB(t *testing.T) (*sql.DB, *dbgen.Queries) {
	t.Helper()

	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	queries := dbgen.New(database)
	if _, err := queries.CreateWorkspace(context.Background(), dbgen.CreateWorkspaceParams{
		ID:           "workspace_1",
		Name:         "cocode",
		RootPath:     t.TempDir(),
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
		LocalPath:   t.TempDir(),
		CreatedAt:   "2026-05-03T00:00:00Z",
		UpdatedAt:   "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
	if _, err := queries.CreatePullRequestSnapshot(context.Background(), dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_1",
		RepositoryID: "repo_1",
		SourceType:   "branch_compare",
		MetadataJson: "{}",
		CreatedAt:    "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}
	if _, err := queries.CreateReviewSession(context.Background(), dbgen.CreateReviewSessionParams{
		ID:                  "review_session_1",
		WorkspaceID:         "workspace_1",
		RepositoryID:        "repo_1",
		SnapshotID:          "snapshot_1",
		Title:               "Review fixture",
		Status:              "running",
		ReviewDepth:         "standard",
		RuntimeLimitSeconds: 300,
		ContextPolicyJson:   "{}",
		CreatedAt:           "2026-05-03T00:00:00Z",
		UpdatedAt:           "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateReviewSession() error = %v", err)
	}
	return database, queries
}
