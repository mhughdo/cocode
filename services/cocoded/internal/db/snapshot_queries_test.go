package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestSnapshotQueriesCRUD(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	if err := Apply(context.Background(), database, Migrations); err != nil {
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

	repository, err := queries.CreateRepository(context.Background(), dbgen.CreateRepositoryParams{
		ID:            "repo_1",
		WorkspaceID:   "workspace_1",
		Name:          "cocode",
		Owner:         nullableString("hughdo"),
		RemoteUrl:     nullableString("git@github.com:hughdo/cocode.git"),
		LocalPath:     "/tmp/cocode",
		DefaultBranch: nullableString("main"),
		CreatedAt:     "2026-05-03T00:01:00Z",
		UpdatedAt:     "2026-05-03T00:01:00Z",
	})
	if err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
	if repository.WorkspaceID != "workspace_1" {
		t.Fatalf("CreateRepository() WorkspaceID = %q, want workspace_1", repository.WorkspaceID)
	}

	byPath, err := queries.GetRepositoryByLocalPath(context.Background(), dbgen.GetRepositoryByLocalPathParams{
		WorkspaceID: "workspace_1",
		LocalPath:   "/tmp/cocode",
	})
	if err != nil {
		t.Fatalf("GetRepositoryByLocalPath() error = %v", err)
	}
	if byPath.ID != repository.ID {
		t.Fatalf("GetRepositoryByLocalPath() ID = %q, want %q", byPath.ID, repository.ID)
	}

	snapshot, err := queries.CreatePullRequestSnapshot(context.Background(), dbgen.CreatePullRequestSnapshotParams{
		ID:             "snapshot_1",
		RepositoryID:   "repo_1",
		SourceType:     "github_pr",
		Provider:       nullableString("github"),
		Owner:          nullableString("hughdo"),
		Repo:           nullableString("cocode"),
		PrNumber:       nullableInt64(42),
		PrTitle:        nullableString("Add review cockpit"),
		PrUrl:          nullableString("https://github.com/hughdo/cocode/pull/42"),
		BaseRef:        nullableString("main"),
		HeadRef:        nullableString("feature/review-cockpit"),
		BaseSha:        nullableString("base-sha"),
		HeadSha:        nullableString("head-sha"),
		DiffArtifactID: nullableString("artifact_diff"),
		MetadataJson:   `{"source":"test"}`,
		CreatedAt:      "2026-05-03T00:02:00Z",
	})
	if err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}
	if snapshot.RepositoryID != "repo_1" || snapshot.PrNumber.Int64 != 42 {
		t.Fatalf("CreatePullRequestSnapshot() = %+v", snapshot)
	}

	gotSnapshot, err := queries.GetPullRequestSnapshot(context.Background(), "snapshot_1")
	if err != nil {
		t.Fatalf("GetPullRequestSnapshot() error = %v", err)
	}
	if gotSnapshot.ID != snapshot.ID {
		t.Fatalf("GetPullRequestSnapshot() ID = %q, want %q", gotSnapshot.ID, snapshot.ID)
	}

	if _, err := queries.CreateChangedFile(context.Background(), dbgen.CreateChangedFileParams{
		ID:             "changed_file_2",
		SnapshotID:     "snapshot_1",
		Path:           "services/cocoded/internal/db/z.go",
		Status:         "modified",
		Additions:      4,
		Deletions:      2,
		LineRangesJson: `[[10,20]]`,
		CreatedAt:      "2026-05-03T00:03:00Z",
	}); err != nil {
		t.Fatalf("CreateChangedFile(z.go) error = %v", err)
	}
	changed, err := queries.CreateChangedFile(context.Background(), dbgen.CreateChangedFileParams{
		ID:              "changed_file_1",
		SnapshotID:      "snapshot_1",
		Path:            "services/cocoded/internal/db/a.go",
		OldPath:         nullableString("services/cocoded/internal/db/old_a.go"),
		Status:          "renamed",
		Additions:       8,
		Deletions:       1,
		IsGenerated:     0,
		LineRangesJson:  `[[1,12]]`,
		PatchArtifactID: nullableString("artifact_patch"),
		CreatedAt:       "2026-05-03T00:04:00Z",
	})
	if err != nil {
		t.Fatalf("CreateChangedFile(a.go) error = %v", err)
	}
	if changed.OldPath.String != "services/cocoded/internal/db/old_a.go" || !changed.OldPath.Valid {
		t.Fatalf("CreateChangedFile() OldPath = %+v", changed.OldPath)
	}

	byChangedPath, err := queries.GetChangedFileByPath(context.Background(), dbgen.GetChangedFileByPathParams{
		SnapshotID: "snapshot_1",
		Path:       "services/cocoded/internal/db/a.go",
	})
	if err != nil {
		t.Fatalf("GetChangedFileByPath() error = %v", err)
	}
	if byChangedPath.ID != changed.ID {
		t.Fatalf("GetChangedFileByPath() ID = %q, want %q", byChangedPath.ID, changed.ID)
	}

	files, err := queries.ListChangedFilesBySnapshot(context.Background(), "snapshot_1")
	if err != nil {
		t.Fatalf("ListChangedFilesBySnapshot() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("ListChangedFilesBySnapshot() len = %d, want 2", len(files))
	}
	if files[0].Path != "services/cocoded/internal/db/a.go" || files[1].Path != "services/cocoded/internal/db/z.go" {
		t.Fatalf("ListChangedFilesBySnapshot() order = [%s, %s]", files[0].Path, files[1].Path)
	}

	excluded, err := queries.UpdateChangedFileExclusion(context.Background(), dbgen.UpdateChangedFileExclusionParams{
		ID:         "changed_file_1",
		IsExcluded: 1,
	})
	if err != nil {
		t.Fatalf("UpdateChangedFileExclusion() error = %v", err)
	}
	if excluded.IsExcluded != 1 {
		t.Fatalf("UpdateChangedFileExclusion() IsExcluded = %d, want 1", excluded.IsExcluded)
	}

	if err := queries.DeletePullRequestSnapshot(context.Background(), "snapshot_1"); err != nil {
		t.Fatalf("DeletePullRequestSnapshot() error = %v", err)
	}
	if _, err := queries.GetChangedFile(context.Background(), "changed_file_1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetChangedFile(deleted snapshot child) error = %v, want sql.ErrNoRows", err)
	}
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

func nullableInt64(value int64) sql.NullInt64 {
	return sql.NullInt64{Int64: value, Valid: true}
}
