package gitrepo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
)

func TestCollectorCompareBranchesBuildsDiffSnapshot(t *testing.T) {
	t.Parallel()

	repoPath := initGitRepo(t)
	configureTestGitIdentity(t, repoPath)
	runTestGit(t, repoPath, "checkout", "-B", "main")
	writeRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc main() {}\n")
	writeRepoFile(t, repoPath, "docs/old.md", "keep\nold\n")
	runTestGit(t, repoPath, "add", ".")
	runTestGit(t, repoPath, "commit", "-m", "initial")

	runTestGit(t, repoPath, "checkout", "-b", "feature/review")
	writeRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc main() {\n\tprintln(\"cocode\")\n}\n")
	runTestGit(t, repoPath, "mv", "docs/old.md", "docs/new.md")
	writeRepoFile(t, repoPath, "docs/new.md", "keep\nold\nnew\n")
	runTestGit(t, repoPath, "add", ".")
	runTestGit(t, repoPath, "commit", "-m", "feature changes")

	snapshot, err := NewCollector(DefaultRunner()).CompareBranches(context.Background(), repoPath, "main", "feature/review")
	if err != nil {
		t.Fatalf("CompareBranches() error = %v", err)
	}
	if snapshot.SourceType != SourceBranchCompare ||
		snapshot.BaseRef != "main" ||
		snapshot.HeadRef != "feature/review" ||
		snapshot.BaseSHA == "" ||
		snapshot.HeadSHA == "" ||
		snapshot.BaseSHA == snapshot.HeadSHA {
		t.Fatalf("snapshot identity = %+v", snapshot)
	}
	if len(snapshot.Diff) == 0 || !strings.Contains(string(snapshot.Diff), "diff --git a/app/main.go b/app/main.go") {
		t.Fatalf("Diff = %q", string(snapshot.Diff))
	}
	if got, ok := snapshot.Metadata["has_uncommitted_changes"].(bool); !ok || got {
		t.Fatalf("has_uncommitted_changes metadata = %#v", snapshot.Metadata["has_uncommitted_changes"])
	}

	modified := diffFileByPath(t, snapshot.Files, "app/main.go")
	if modified.Status != "modified" || modified.Additions != 3 || modified.Deletions != 1 {
		t.Fatalf("modified file = %+v", modified)
	}
	if modified.LineRangesJSON == "[]" || !strings.Contains(modified.Patch, "println(\"cocode\")") {
		t.Fatalf("modified ranges/patch = %+v", modified)
	}

	renamed := diffFileByPath(t, snapshot.Files, "docs/new.md")
	if renamed.Status != "renamed" || renamed.OldPath != "docs/old.md" || renamed.Additions != 1 {
		t.Fatalf("renamed file = %+v", renamed)
	}
}

func TestCollectorLocalChangesIncludesTrackedAndUntrackedFiles(t *testing.T) {
	t.Parallel()

	repoPath := initGitRepo(t)
	configureTestGitIdentity(t, repoPath)
	runTestGit(t, repoPath, "checkout", "-B", "main")
	writeRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc main() {}\n")
	runTestGit(t, repoPath, "add", ".")
	runTestGit(t, repoPath, "commit", "-m", "initial")

	writeRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc main() {\n\tprintln(\"local\")\n}\n")
	writeRepoFile(t, repoPath, "notes/todo.md", "one\ntwo\n")
	writeRepoBytes(t, repoPath, "assets/logo.bin", []byte{0x00, 0x01, 0x02})

	snapshot, err := NewCollector(DefaultRunner()).LocalChanges(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("LocalChanges() error = %v", err)
	}
	if snapshot.SourceType != SourceLocalChanges ||
		snapshot.BaseRef != "HEAD" ||
		snapshot.HeadRef != "WORKTREE" ||
		snapshot.BaseSHA == "" ||
		snapshot.HeadSHA != snapshot.BaseSHA {
		t.Fatalf("snapshot identity = %+v", snapshot)
	}
	if got, ok := snapshot.Metadata["untracked_file_count"].(int); !ok || got != 2 {
		t.Fatalf("untracked_file_count = %#v, want 2", snapshot.Metadata["untracked_file_count"])
	}

	tracked := diffFileByPath(t, snapshot.Files, "app/main.go")
	if tracked.Status != "modified" || tracked.Additions != 3 || tracked.Deletions != 1 {
		t.Fatalf("tracked file = %+v", tracked)
	}
	untrackedText := diffFileByPath(t, snapshot.Files, "notes/todo.md")
	if untrackedText.Status != "added" ||
		untrackedText.Additions != 2 ||
		untrackedText.LineRangesJSON != "[[1,2]]" ||
		!strings.Contains(untrackedText.Patch, "diff --git a/notes/todo.md b/notes/todo.md") {
		t.Fatalf("untracked text file = %+v", untrackedText)
	}
	untrackedBinary := diffFileByPath(t, snapshot.Files, "assets/logo.bin")
	if untrackedBinary.Status != "added" ||
		!untrackedBinary.IsBinary ||
		untrackedBinary.LineRangesJSON != "[]" ||
		!strings.Contains(untrackedBinary.Patch, "Binary files /dev/null and b/assets/logo.bin differ") {
		t.Fatalf("untracked binary file = %+v", untrackedBinary)
	}
	if !strings.Contains(string(snapshot.Diff), "diff --git a/notes/todo.md b/notes/todo.md") ||
		!strings.Contains(string(snapshot.Diff), "diff --git a/assets/logo.bin b/assets/logo.bin") {
		t.Fatalf("Diff missing untracked patches:\n%s", string(snapshot.Diff))
	}
}

func TestCollectorCompareBranchesRejectsUnsafeRef(t *testing.T) {
	t.Parallel()

	repoPath := initGitRepo(t)
	_, err := NewCollector(DefaultRunner()).CompareBranches(context.Background(), repoPath, "-bad", "HEAD")
	assertAppError(t, err, apperror.CodeInvalidRequest, "base ref is invalid")
}

func configureTestGitIdentity(t *testing.T, repoPath string) {
	t.Helper()

	runTestGit(t, repoPath, "config", "user.email", "cocode@example.com")
	runTestGit(t, repoPath, "config", "user.name", "cocode")
	runTestGit(t, repoPath, "config", "commit.gpgsign", "false")
}

func writeRepoFile(t *testing.T, repoPath string, relativePath string, content string) {
	t.Helper()
	writeRepoBytes(t, repoPath, relativePath, []byte(content))
}

func writeRepoBytes(t *testing.T, repoPath string, relativePath string, content []byte) {
	t.Helper()

	path := filepath.Join(repoPath, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", relativePath, err)
	}
}

func diffFileByPath(t *testing.T, files []DiffFile, path string) DiffFile {
	t.Helper()

	for _, file := range files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("diff file %q not found in %+v", path, files)
	return DiffFile{}
}
