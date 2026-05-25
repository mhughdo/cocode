package eventlog

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestStoreAppendAssignsMonotonicSessionSequence(t *testing.T) {
	t.Parallel()

	database, queries := eventTestDatabase(t)
	store, err := New(database)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	first, err := store.Append(context.Background(), AppendParams{
		ID:              "event_1",
		ReviewSessionID: "review_session_1",
		Type:            "session.created",
		PayloadJSON:     `{"status":"draft"}`,
		CreatedAt:       "2026-05-03T00:21:00Z",
	})
	if err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	if first.Sequence != 1 || first.Level != "info" {
		t.Fatalf("Append(first) = %+v", first)
	}

	second, err := store.Append(context.Background(), AppendParams{
		ID:              "event_2",
		ReviewSessionID: "review_session_1",
		Type:            "agent.started",
		Level:           "debug",
		PayloadJSON:     `{"agent":"codex"}`,
		CreatedAt:       "2026-05-03T00:22:00Z",
	})
	if err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}
	if second.Sequence != 2 || second.Level != "debug" {
		t.Fatalf("Append(second) = %+v", second)
	}

	events, err := store.ListByReviewSession(context.Background(), "review_session_1")
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("ListByReviewSession() len = %d, want 2", len(events))
	}
	if events[0].ID != "event_1" || events[1].ID != "event_2" {
		t.Fatalf("ListByReviewSession() order = [%s, %s]", events[0].ID, events[1].ID)
	}

	_, err = queries.CreateEvent(context.Background(), dbgen.CreateEventParams{
		ID:              "event_duplicate",
		ReviewSessionID: sql.NullString{String: "review_session_1", Valid: true},
		Type:            "duplicate",
		Level:           "info",
		Sequence:        2,
		PayloadJson:     "{}",
		CreatedAt:       "2026-05-03T00:23:00Z",
	})
	if err == nil {
		t.Fatal("CreateEvent(duplicate sequence) error = nil, want unique constraint error")
	}
}

func TestStoreAppendValidatesRequiredFields(t *testing.T) {
	t.Parallel()

	database, _ := eventTestDatabase(t)
	store, err := New(database)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, params := range []AppendParams{
		{ReviewSessionID: "review_session_1", Type: "missing.id"},
		{ID: "event_missing_session", Type: "missing.session"},
		{ID: "event_missing_type", ReviewSessionID: "review_session_1"},
	} {
		if _, err := store.Append(context.Background(), params); err == nil {
			t.Fatalf("Append(%+v) error = nil, want validation error", params)
		}
	}
}

func TestRetryableAppendConflictDetection(t *testing.T) {
	t.Parallel()

	retryable := errors.New("create event: constraint failed: UNIQUE constraint failed: events.review_session_id, events.sequence")
	if !isRetryableAppendConflict(retryable) {
		t.Fatalf("isRetryableAppendConflict(%q) = false, want true", retryable)
	}
	if isRetryableAppendConflict(errors.New("create event: UNIQUE constraint failed: events.id")) {
		t.Fatal("isRetryableAppendConflict(primary key) = true, want false")
	}
}

func eventTestDatabase(t *testing.T) (*sql.DB, *dbgen.Queries) {
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
