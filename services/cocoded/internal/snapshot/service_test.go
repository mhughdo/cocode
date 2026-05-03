package snapshot

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/githubpr"
	"github.com/hughdo/cocode/services/cocoded/internal/gitrepo"
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

func TestCreateGitHubSnapshotStoresPreviousCommentsArtifact(t *testing.T) {
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

	result, err := service.CreateGitHubSnapshot(ctx, GitHubSnapshotParams{
		WorkspaceID:  "workspace_1",
		RepositoryID: "repo_1",
		Metadata: githubpr.Metadata{
			Owner:   "openai",
			Repo:    "codex",
			Number:  123,
			BaseSHA: "base-sha",
			HeadSHA: "head-sha",
		},
		Diff: []byte("diff --git a/api.go b/api.go\n"),
		PreviousComments: &githubpr.PreviousComments{
			IssueCommentCount:  1,
			ReviewCommentCount: 1,
			ReviewCount:        1,
			Comments: []githubpr.PreviousComment{
				{
					Source:    "issue_comment",
					ID:        10,
					Author:    "reviewer-a",
					Body:      "Please add a test.",
					CreatedAt: "2026-05-03T10:00:00Z",
				},
				{
					Source:      "review_comment",
					ID:          20,
					ReviewID:    99,
					Author:      "reviewer-b",
					Path:        "api.go",
					Line:        12,
					InReplyToID: 19,
					CreatedAt:   "2026-05-03T10:01:00Z",
				},
				{
					Source:      "review",
					ID:          99,
					Author:      "reviewer-b",
					State:       "COMMENTED",
					SubmittedAt: "2026-05-03T10:02:00Z",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateGitHubSnapshot(previous comments) error = %v", err)
	}

	if result.PreviousCommentsArtifact.ID == "" ||
		result.PreviousCommentsArtifact.Kind != "github_previous_comments" ||
		result.PreviousCommentsArtifact.ContentType != "application/json" {
		t.Fatalf("previous comments artifact = %+v", result.PreviousCommentsArtifact)
	}
	if !strings.HasSuffix(result.PreviousCommentsArtifact.RelativePath, "/github-previous-comments.json") {
		t.Fatalf("previous comments relative path = %q", result.PreviousCommentsArtifact.RelativePath)
	}

	var metadata struct {
		Source             string `json:"source"`
		Owner              string `json:"owner"`
		Repo               string `json:"repo"`
		PRNumber           int64  `json:"pr_number"`
		SnapshotID         string `json:"snapshot_id"`
		CommentCount       int    `json:"comment_count"`
		IssueCommentCount  int    `json:"issue_comment_count"`
		ReviewCommentCount int    `json:"review_comment_count"`
		ReviewCount        int    `json:"review_count"`
	}
	if err := json.Unmarshal([]byte(result.PreviousCommentsArtifact.MetadataJson), &metadata); err != nil {
		t.Fatalf("decode previous comments metadata: %v", err)
	}
	if metadata.Source != "github" ||
		metadata.Owner != "openai" ||
		metadata.Repo != "codex" ||
		metadata.PRNumber != 123 ||
		metadata.SnapshotID != result.Snapshot.ID ||
		metadata.CommentCount != 3 ||
		metadata.IssueCommentCount != 1 ||
		metadata.ReviewCommentCount != 1 ||
		metadata.ReviewCount != 1 {
		t.Fatalf("previous comments metadata = %+v", metadata)
	}
	var snapshotMetadata struct {
		PreviousComments struct {
			ArtifactID         string `json:"artifact_id"`
			CommentCount       int    `json:"comment_count"`
			IssueCommentCount  int    `json:"issue_comment_count"`
			ReviewCommentCount int    `json:"review_comment_count"`
			ReviewCount        int    `json:"review_count"`
		} `json:"previous_comments"`
	}
	if err := json.Unmarshal([]byte(result.Snapshot.MetadataJson), &snapshotMetadata); err != nil {
		t.Fatalf("decode snapshot metadata: %v", err)
	}
	if snapshotMetadata.PreviousComments.ArtifactID != result.PreviousCommentsArtifact.ID ||
		snapshotMetadata.PreviousComments.CommentCount != 3 ||
		snapshotMetadata.PreviousComments.IssueCommentCount != 1 ||
		snapshotMetadata.PreviousComments.ReviewCommentCount != 1 ||
		snapshotMetadata.PreviousComments.ReviewCount != 1 {
		t.Fatalf("snapshot previous comments metadata = %+v", snapshotMetadata.PreviousComments)
	}

	content := readArtifact(t, artifactRoot, "workspace_1", result.PreviousCommentsArtifact.RelativePath)
	var stored githubpr.PreviousComments
	if err := json.Unmarshal(content, &stored); err != nil {
		t.Fatalf("decode previous comments content: %v", err)
	}
	if len(stored.Comments) != 3 || stored.Comments[1].Path != "api.go" || stored.Comments[1].InReplyToID != 19 {
		t.Fatalf("stored previous comments = %+v", stored)
	}

	artifacts, err := queries.ListArtifactsByWorkspace(ctx, "workspace_1")
	if err != nil {
		t.Fatalf("ListArtifactsByWorkspace() error = %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("workspace artifacts len = %d, want 2: %+v", len(artifacts), artifacts)
	}
}

func TestCreateGitSnapshotStoresBranchCompareOutput(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repoPath := initSnapshotGitRepo(t)
	runSnapshotGit(t, repoPath, "checkout", "-B", "main")
	writeSnapshotRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc main() {}\n")
	runSnapshotGit(t, repoPath, "add", ".")
	runSnapshotGit(t, repoPath, "commit", "-m", "initial")
	runSnapshotGit(t, repoPath, "checkout", "-b", "feature/review")
	writeSnapshotRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc main() {\n\tprintln(\"review\")\n}\n")
	runSnapshotGit(t, repoPath, "add", ".")
	runSnapshotGit(t, repoPath, "commit", "-m", "feature")

	collected, err := gitrepo.NewCollector(gitrepo.DefaultRunner()).CompareBranches(ctx, repoPath, "main", "feature/review")
	if err != nil {
		t.Fatalf("CompareBranches() error = %v", err)
	}

	database, queries := openSnapshotTestDB(t)
	artifactRoot := filepath.Join(t.TempDir(), "artifacts")
	createWorkspaceAndRepositoryAt(t, queries, repoPath)
	service, err := New(database, artifactRoot)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service.now = fixedSnapshotTime

	result, err := service.CreateGitSnapshot(ctx, GitSnapshotParams{
		WorkspaceID:  "workspace_1",
		RepositoryID: "repo_1",
		SourceType:   collected.SourceType,
		BaseRef:      collected.BaseRef,
		HeadRef:      collected.HeadRef,
		BaseSHA:      collected.BaseSHA,
		HeadSHA:      collected.HeadSHA,
		Metadata:     collected.Metadata,
		Files:        changedInputsFromGit(collected.Files),
		Diff:         collected.Diff,
	})
	if err != nil {
		t.Fatalf("CreateGitSnapshot() error = %v", err)
	}

	if result.Snapshot.SourceType != "branch_compare" ||
		result.Snapshot.Provider.Valid ||
		result.Snapshot.BaseRef.String != "main" ||
		result.Snapshot.HeadRef.String != "feature/review" {
		t.Fatalf("snapshot = %+v", result.Snapshot)
	}
	changed := changedFileByPath(t, result.ChangedFiles, "app/main.go")
	if changed.LineRangesJson == "[]" ||
		changed.PatchArtifactID.String == "" ||
		changed.Additions != 3 ||
		changed.Deletions != 1 {
		t.Fatalf("changed file = %+v", changed)
	}
	patchContent := readArtifact(t, artifactRoot, "workspace_1", result.PatchArtifacts[0].RelativePath)
	if !strings.Contains(string(patchContent), "println(\"review\")") {
		t.Fatalf("patch artifact content = %q", string(patchContent))
	}
}

func TestCreateGitSnapshotPersistsBinaryFlag(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, queries := openSnapshotTestDB(t)
	createWorkspaceAndRepository(t, queries)
	service, err := New(database, filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service.now = fixedSnapshotTime

	result, err := service.CreateGitSnapshot(ctx, GitSnapshotParams{
		WorkspaceID:  "workspace_1",
		RepositoryID: "repo_1",
		SourceType:   "local_changes",
		BaseRef:      "HEAD",
		HeadRef:      "WORKTREE",
		BaseSHA:      "head-sha",
		HeadSHA:      "head-sha",
		Files: []ChangedFileInput{
			{
				Path:           "assets/logo.bin",
				Status:         "added",
				IsBinary:       true,
				LineRangesJSON: "[]",
				Patch:          "diff --git a/assets/logo.bin b/assets/logo.bin\nBinary files /dev/null and b/assets/logo.bin differ\n",
			},
			{
				Path:           "services/cocoded/internal/db/dbgen/snapshots.sql.go",
				Status:         "modified",
				LineRangesJSON: "[]",
				Patch:          "diff --git a/services/cocoded/internal/db/dbgen/snapshots.sql.go b/services/cocoded/internal/db/dbgen/snapshots.sql.go\n+// Code generated by sqlc. DO NOT EDIT.\n",
			},
		},
		Diff: []byte("diff --git a/assets/logo.bin b/assets/logo.bin\n"),
	})
	if err != nil {
		t.Fatalf("CreateGitSnapshot(binary) error = %v", err)
	}
	binary := changedFileByPath(t, result.ChangedFiles, "assets/logo.bin")
	if binary.IsBinary != 1 || binary.IsGenerated != 0 || binary.IsExcluded != 1 {
		t.Fatalf("binary changed file = %+v", binary)
	}
	generated := changedFileByPath(t, result.ChangedFiles, "services/cocoded/internal/db/dbgen/snapshots.sql.go")
	if generated.IsBinary != 0 || generated.IsGenerated != 1 || generated.IsExcluded != 1 {
		t.Fatalf("generated changed file = %+v", generated)
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

func TestCreateGitSnapshotRejectsUnsafeChangedPathsBeforeWritingArtifacts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, queries := openSnapshotTestDB(t)
	artifactRoot := filepath.Join(t.TempDir(), "artifacts")
	createWorkspaceAndRepository(t, queries)
	service, err := New(database, artifactRoot)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, file := range []ChangedFileInput{
		{Path: "../escape.go", Status: "modified"},
		{Path: "safe.go", OldPath: "/tmp/old.go", Status: "renamed"},
	} {
		_, err = service.CreateGitSnapshot(ctx, GitSnapshotParams{
			WorkspaceID:  "workspace_1",
			RepositoryID: "repo_1",
			SourceType:   "local_changes",
			BaseRef:      "HEAD",
			HeadRef:      "WORKTREE",
			BaseSHA:      "head-sha",
			HeadSHA:      "head-sha",
			Files:        []ChangedFileInput{file},
			Diff:         []byte("diff"),
		})
		if err == nil {
			t.Fatalf("CreateGitSnapshot(%+v) error = nil, want unsafe path error", file)
		}
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
	createWorkspaceAndRepositoryAt(t, queries, filepath.Join(t.TempDir(), "repo"))
}

func createWorkspaceAndRepositoryAt(t *testing.T, queries *dbgen.Queries, repoPath string) {
	t.Helper()

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

func changedInputsFromGit(files []gitrepo.DiffFile) []ChangedFileInput {
	inputs := make([]ChangedFileInput, 0, len(files))
	for _, file := range files {
		inputs = append(inputs, ChangedFileInput{
			Path:           file.Path,
			OldPath:        file.OldPath,
			Status:         file.Status,
			Additions:      file.Additions,
			Deletions:      file.Deletions,
			IsBinary:       file.IsBinary,
			LineRangesJSON: file.LineRangesJSON,
			Patch:          file.Patch,
		})
	}
	return inputs
}

func initSnapshotGitRepo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	repoPath := t.TempDir()
	runSnapshotGit(t, repoPath, "init")
	runSnapshotGit(t, repoPath, "config", "user.email", "cocode@example.com")
	runSnapshotGit(t, repoPath, "config", "user.name", "cocode")
	runSnapshotGit(t, repoPath, "config", "commit.gpgsign", "false")
	canonical, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	return canonical
}

func runSnapshotGit(t *testing.T, cwd string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, string(output))
	}
}

func writeSnapshotRepoFile(t *testing.T, repoPath string, relativePath string, content string) {
	t.Helper()

	path := filepath.Join(repoPath, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", relativePath, err)
	}
}
