package gitrepo

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db"
)

func TestValidateResolvesRepositoryRoot(t *testing.T) {
	t.Parallel()

	repoPath := initGitRepo(t)
	subdir := filepath.Join(repoPath, "apps", "desktop")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	info, err := Validate(context.Background(), subdir)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if info.SelectedPath != subdir {
		t.Fatalf("SelectedPath = %q, want %q", info.SelectedPath, subdir)
	}
	if info.RootPath != repoPath {
		t.Fatalf("RootPath = %q, want %q", info.RootPath, repoPath)
	}
	if info.Name != filepath.Base(repoPath) {
		t.Fatalf("Name = %q, want %q", info.Name, filepath.Base(repoPath))
	}
	if info.Owner != "hughdo" {
		t.Fatalf("Owner = %q, want hughdo", info.Owner)
	}
	if info.RemoteURL != "git@github.com:hughdo/cocode.git" {
		t.Fatalf("RemoteURL = %q", info.RemoteURL)
	}
	if info.DefaultBranch == "" {
		t.Fatal("DefaultBranch is empty")
	}
}

func TestOpenCreatesWorkspaceAndRepositoryIdempotently(t *testing.T) {
	t.Parallel()

	repoPath := initGitRepo(t)
	subdir := filepath.Join(repoPath, "packages", "ui")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	service, err := New(database)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service.now = func() time.Time {
		return time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	}

	first, err := service.Open(context.Background(), subdir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if first.Workspace.RootPath != repoPath {
		t.Fatalf("Workspace.RootPath = %q, want %q", first.Workspace.RootPath, repoPath)
	}
	if !first.Workspace.DefaultRepoID.Valid || first.Workspace.DefaultRepoID.String != first.Repository.ID {
		t.Fatalf("DefaultRepoID = %+v, repository ID %q", first.Workspace.DefaultRepoID, first.Repository.ID)
	}
	if first.Repository.LocalPath != repoPath {
		t.Fatalf("Repository.LocalPath = %q, want %q", first.Repository.LocalPath, repoPath)
	}
	if !first.Repository.RemoteUrl.Valid || first.Repository.RemoteUrl.String != "git@github.com:hughdo/cocode.git" {
		t.Fatalf("Repository.RemoteUrl = %+v", first.Repository.RemoteUrl)
	}

	second, err := service.Open(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	if second.Workspace.ID != first.Workspace.ID {
		t.Fatalf("second workspace ID = %q, want %q", second.Workspace.ID, first.Workspace.ID)
	}
	if second.Repository.ID != first.Repository.ID {
		t.Fatalf("second repository ID = %q, want %q", second.Repository.ID, first.Repository.ID)
	}
	assertCount(t, database, 1, `SELECT COUNT(*) FROM workspaces WHERE root_path = ?`, repoPath)
	assertCount(t, database, 1, `SELECT COUNT(*) FROM repositories WHERE local_path = ?`, repoPath)
}

func TestValidateReturnsTypedInvalidRequest(t *testing.T) {
	t.Parallel()

	_, err := Validate(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("Validate(non-git dir) error = nil, want error")
	}
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("Validate(non-git dir) error = %T, want *apperror.Error", err)
	}
	if appErr.Code != apperror.CodeInvalidRequest {
		t.Fatalf("Code = %q, want %q", appErr.Code, apperror.CodeInvalidRequest)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	repoPath := t.TempDir()
	runTestGit(t, repoPath, "init")
	runTestGit(t, repoPath, "remote", "add", "origin", "git@github.com:hughdo/cocode.git")
	canonical, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	return canonical
}

func runTestGit(t *testing.T, cwd string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, string(output))
	}
}

func assertCount(t *testing.T, database *sql.DB, want int, query string, args ...any) {
	t.Helper()

	var got int
	if err := database.QueryRowContext(context.Background(), query, args...).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("count query %q = %d, want %d", query, got, want)
	}
}
