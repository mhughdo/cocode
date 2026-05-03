package snapshot

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/githubpr"
)

func TestCreateGitHubSnapshotStoresRowsAndArtifacts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, queries := openSnapshotTestDB(t)
	artifactRoot := filepath.Join(t.TempDir(), "artifacts")
	createWorkspaceAndRepository(t, queries)

	service, err := New(database, artifactRoot)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service.now = fixedSnapshotTime

	diff := []byte("diff --git a/api.go b/api.go\n@@ -1 +1 @@\n-old\n+new\n")
	result, err := service.CreateGitHubSnapshot(ctx, GitHubSnapshotParams{
		WorkspaceID:  "workspace_1",
		RepositoryID: "repo_1",
		Metadata: githubpr.Metadata{
			Owner:   "openai",
			Repo:    "codex",
			Number:  123,
			Title:   "Add API route",
			Author:  "octocat",
			URL:     "https://github.com/openai/codex/pull/123",
			BaseRef: "main",
			HeadRef: "feature/api",
			BaseSHA: "base-sha",
			HeadSHA: "head-sha",
		},
		Files: []githubpr.ChangedFile{
			{
				SHA:       "sha-1",
				Filename:  "apps/api/routes.go",
				Status:    "modified",
				Additions: 12,
				Deletions: 3,
				Changes:   15,
				Patch:     "@@ -1 +1 @@\n-old\n+new\n",
			},
			{
				SHA:              "sha-2",
				Filename:         "apps/api/new name.go",
				PreviousFilename: "apps/api/old name.go",
				Status:           "renamed",
				Additions:        4,
				Deletions:        1,
				Changes:          5,
			},
		},
		Diff: diff,
	})
	if err != nil {
		t.Fatalf("CreateGitHubSnapshot() error = %v", err)
	}

	if result.Snapshot.SourceType != "github_pr" || result.Snapshot.DiffArtifactID.String != result.DiffArtifact.ID {
		t.Fatalf("snapshot = %+v, diff artifact = %+v", result.Snapshot, result.DiffArtifact)
	}
	if !result.Snapshot.Provider.Valid || result.Snapshot.Provider.String != "github" {
		t.Fatalf("Provider = %+v", result.Snapshot.Provider)
	}
	if result.Snapshot.PrNumber.Int64 != 123 ||
		result.Snapshot.PrTitle.String != "Add API route" ||
		result.Snapshot.BaseSha.String != "base-sha" ||
		result.Snapshot.HeadSha.String != "head-sha" {
		t.Fatalf("snapshot metadata fields = %+v", result.Snapshot)
	}

	diffHash := sha256.Sum256(diff)
	wantSHA := hex.EncodeToString(diffHash[:])
	if result.DiffArtifact.Kind != "diff" ||
		result.DiffArtifact.ContentType != "text/x-diff" ||
		result.DiffArtifact.SizeBytes != int64(len(diff)) ||
		!result.DiffArtifact.Sha256.Valid ||
		result.DiffArtifact.Sha256.String != wantSHA {
		t.Fatalf("diff artifact = %+v, want sha %s size %d", result.DiffArtifact, wantSHA, len(diff))
	}
	diffContent := readArtifact(t, artifactRoot, "workspace_1", result.DiffArtifact.RelativePath)
	if string(diffContent) != string(diff) {
		t.Fatalf("diff artifact content = %q", string(diffContent))
	}

	if len(result.ChangedFiles) != 2 {
		t.Fatalf("changed files len = %d, want 2", len(result.ChangedFiles))
	}
	modified := changedFileByPath(t, result.ChangedFiles, "apps/api/routes.go")
	if modified.PatchArtifactID.String == "" ||
		modified.LineRangesJson != "[]" ||
		modified.IsBinary != 0 ||
		modified.Additions != 12 ||
		modified.Deletions != 3 {
		t.Fatalf("modified changed file = %+v", modified)
	}
	renamed := changedFileByPath(t, result.ChangedFiles, "apps/api/new name.go")
	if !renamed.OldPath.Valid ||
		renamed.OldPath.String != "apps/api/old name.go" ||
		renamed.PatchArtifactID.Valid ||
		renamed.IsBinary != 0 {
		t.Fatalf("renamed changed file = %+v", renamed)
	}

	if len(result.PatchArtifacts) != 1 {
		t.Fatalf("patch artifacts len = %d, want 1", len(result.PatchArtifacts))
	}
	patchContent := readArtifact(t, artifactRoot, "workspace_1", result.PatchArtifacts[0].RelativePath)
	if string(patchContent) != "@@ -1 +1 @@\n-old\n+new\n" {
		t.Fatalf("patch artifact content = %q", string(patchContent))
	}

	storedFiles, err := queries.ListChangedFilesBySnapshot(ctx, result.Snapshot.ID)
	if err != nil {
		t.Fatalf("ListChangedFilesBySnapshot() error = %v", err)
	}
	if len(storedFiles) != 2 || storedFiles[0].Path != "apps/api/new name.go" || storedFiles[1].Path != "apps/api/routes.go" {
		t.Fatalf("stored files = %+v", storedFiles)
	}

	var snapshotMetadata struct {
		FileCount  int    `json:"file_count"`
		DiffSHA256 string `json:"diff_sha256"`
	}
	if err := json.Unmarshal([]byte(result.Snapshot.MetadataJson), &snapshotMetadata); err != nil {
		t.Fatalf("decode snapshot metadata: %v", err)
	}
	if snapshotMetadata.FileCount != 2 || snapshotMetadata.DiffSHA256 != wantSHA {
		t.Fatalf("snapshot metadata = %+v", snapshotMetadata)
	}
}

func TestCreateGitHubSnapshotAcceptsEmptyDiff(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, queries := openSnapshotTestDB(t)
	createWorkspaceAndRepository(t, queries)

	service, err := New(database, filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service.now = fixedSnapshotTime

	result, err := service.CreateGitHubSnapshot(ctx, GitHubSnapshotParams{
		WorkspaceID:  "workspace_1",
		RepositoryID: "repo_1",
		Metadata:     githubpr.Metadata{Owner: "openai", Repo: "codex", Number: 123},
		Diff:         []byte{},
	})
	if err != nil {
		t.Fatalf("CreateGitHubSnapshot(empty diff) error = %v", err)
	}
	if result.DiffArtifact.SizeBytes != 0 {
		t.Fatalf("SizeBytes = %d, want 0", result.DiffArtifact.SizeBytes)
	}
}

func TestCreateGitHubSnapshotRejectsDuplicatePathsBeforeWritingArtifacts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, queries := openSnapshotTestDB(t)
	artifactRoot := filepath.Join(t.TempDir(), "artifacts")
	createWorkspaceAndRepository(t, queries)

	service, err := New(database, artifactRoot)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = service.CreateGitHubSnapshot(ctx, GitHubSnapshotParams{
		WorkspaceID:  "workspace_1",
		RepositoryID: "repo_1",
		Metadata:     githubpr.Metadata{Owner: "openai", Repo: "codex", Number: 123},
		Files: []githubpr.ChangedFile{
			{Filename: "dup.go", Patch: "patch one"},
			{Filename: "dup.go", Patch: "patch two"},
		},
		Diff: []byte("diff"),
	})
	if err == nil {
		t.Fatal("CreateGitHubSnapshot(duplicate paths) error = nil, want error")
	}
	if _, statErr := os.Stat(filepath.Join(artifactRoot, "workspace_1")); !os.IsNotExist(statErr) {
		t.Fatalf("artifact workspace stat error = %v, want not exist", statErr)
	}
}

func openSnapshotTestDB(t *testing.T) (*sql.DB, *dbgen.Queries) {
	t.Helper()

	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	return database, dbgen.New(database)
}

func createWorkspaceAndRepository(t *testing.T, queries *dbgen.Queries) {
	t.Helper()

	if _, err := queries.CreateWorkspace(context.Background(), dbgen.CreateWorkspaceParams{
		ID:           "workspace_1",
		Name:         "cocode",
		RootPath:     filepath.Join(t.TempDir(), "repo"),
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
		LocalPath:   filepath.Join(t.TempDir(), "repo"),
		CreatedAt:   "2026-05-03T00:00:00Z",
		UpdatedAt:   "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
}

func fixedSnapshotTime() time.Time {
	return time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
}

func readArtifact(t *testing.T, root string, workspaceID string, relativePath string) []byte {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(root, workspaceID, filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", relativePath, err)
	}
	return content
}

func changedFileByPath(t *testing.T, files []dbgen.ChangedFile, path string) dbgen.ChangedFile {
	t.Helper()

	for _, file := range files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("changed file %q not found in %+v", path, files)
	return dbgen.ChangedFile{}
}
