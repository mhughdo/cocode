package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateEphemeralWorktreeCleansUpDisposableFiles(t *testing.T) {
	t.Parallel()

	repoPath := initGitRepoWithCommit(t)
	worktree, err := DefaultRunner().CreateEphemeralWorktree(context.Background(), repoPath, "")
	if err != nil {
		t.Fatalf("CreateEphemeralWorktree() error = %v", err)
	}
	if worktree.Path == "" || worktree.Path == repoPath {
		t.Fatalf("worktree path = %q, source = %q", worktree.Path, repoPath)
	}
	if worktree.SourceRoot != repoPath {
		t.Fatalf("SourceRoot = %q, want %q", worktree.SourceRoot, repoPath)
	}
	if content, err := os.ReadFile(filepath.Join(worktree.Path, "README.md")); err != nil || string(content) != "hello\n" {
		t.Fatalf("worktree README = %q, err = %v", string(content), err)
	}
	if err := os.WriteFile(filepath.Join(worktree.Path, "agent-output.txt"), []byte("disposable\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(disposable) error = %v", err)
	}
	parent := filepath.Dir(worktree.Path)

	if err := worktree.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, err := os.Stat(parent); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temp parent stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(repoPath, "agent-output.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source disposable stat error = %v, want not exist", err)
	}
}

func TestCreateEphemeralWorktreeRequiresHEAD(t *testing.T) {
	t.Parallel()

	repoPath := initGitRepo(t)
	_, err := DefaultRunner().CreateEphemeralWorktree(context.Background(), repoPath, "")
	if err == nil || !strings.Contains(err.Error(), "HEAD") {
		t.Fatalf("CreateEphemeralWorktree() error = %v, want HEAD error", err)
	}
}

func initGitRepoWithCommit(t *testing.T) string {
	t.Helper()

	repoPath := initGitRepo(t)
	runTestGit(t, repoPath, "config", "user.email", "cocode@example.com")
	runTestGit(t, repoPath, "config", "user.name", "Cocode Test")
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README) error = %v", err)
	}
	runTestGit(t, repoPath, "add", "README.md")
	runTestGit(t, repoPath, "commit", "-m", "initial")
	return repoPath
}
