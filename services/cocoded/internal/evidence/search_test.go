package evidence

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRipgrepSearcherFindsMatchesAndLimitsOutput(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg is not installed")
	}
	root := t.TempDir()
	writeEvidenceRepoFile(t, root, "src/auth.go", "package src\n\nfunc RequireAdmin() bool { return true }\n")
	writeEvidenceRepoFile(t, root, "src/routes.go", "package src\n\nfunc route() { _ = RequireAdmin() }\n")

	matches, err := (RipgrepSearcher{}).Search(context.Background(), SearchOptions{
		RepoRoot: root,
		Query:    "RequireAdmin",
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(matches) != 1 ||
		matches[0].Path == "" ||
		matches[0].Line <= 0 ||
		!strings.Contains(matches[0].Text, "RequireAdmin") {
		t.Fatalf("matches = %+v", matches)
	}

	none, err := (RipgrepSearcher{}).Search(context.Background(), SearchOptions{
		RepoRoot: root,
		Query:    "DefinitelyMissing",
	})
	if err != nil {
		t.Fatalf("Search(no hits) error = %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("Search(no hits) = %+v, want empty", none)
	}
}

func TestRipgrepSearcherFallsBackWhenDefaultCommandIsMissing(t *testing.T) {
	originalLookPath := lookPath
	lookPath = func(command string) (string, error) {
		if command == "rg" {
			return "", exec.ErrNotFound
		}
		return originalLookPath(command)
	}
	t.Cleanup(func() {
		lookPath = originalLookPath
	})

	root := t.TempDir()
	writeEvidenceRepoFile(t, root, "src/auth.go", "package src\n\nfunc RequireAdmin() bool { return true }\n")
	writeEvidenceRepoFile(t, root, "src/routes.go", "package src\n\nfunc route() { _ = RequireAdmin() }\n")

	matches, err := (RipgrepSearcher{}).Search(context.Background(), SearchOptions{
		RepoRoot:    root,
		Query:       "RequireAdmin",
		Paths:       []string{"src"},
		ExcludePath: []string{"src/auth.go"},
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(matches) != 1 ||
		matches[0].Path != "src/routes.go" ||
		matches[0].Line != 3 ||
		!strings.Contains(matches[0].Text, "RequireAdmin") {
		t.Fatalf("matches = %+v", matches)
	}
}

func TestRipgrepSearcherRejectsUnsafePathsAndSymlinkEscapes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeEvidenceRepoFile(t, root, "src/auth.go", "package src\n")
	if _, err := (RipgrepSearcher{}).Search(context.Background(), SearchOptions{
		RepoRoot: root,
		Query:    "package",
		Paths:    []string{"../outside.go"},
	}); err == nil || !strings.Contains(err.Error(), "escapes repo root") {
		t.Fatalf("Search(unsafe path) error = %v", err)
	}

	outside := t.TempDir()
	writeEvidenceRepoFile(t, outside, "secret.go", "package secret\n")
	if err := os.Symlink(filepath.Join(outside, "secret.go"), filepath.Join(root, "link.go")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, _, err := safeRepoFilePath(root, "link.go"); err == nil || !strings.Contains(err.Error(), "escapes repo root") {
		t.Fatalf("safeRepoFilePath(symlink) error = %v", err)
	}
}

func TestRipgrepSearcherHonorsTimeout(t *testing.T) {
	t.Parallel()

	command := filepath.Join(t.TempDir(), "slow-search")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nsleep 2\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(slow-search) error = %v", err)
	}
	root := t.TempDir()
	writeEvidenceRepoFile(t, root, "src/auth.go", "package src\n")
	_, err := (RipgrepSearcher{Command: command}).Search(context.Background(), SearchOptions{
		RepoRoot: root,
		Query:    "package",
		Timeout:  10 * time.Millisecond,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Search(timeout) error = %v, want deadline exceeded", err)
	}
}

func TestParseRipgrepJSONRejectsMalformedOutput(t *testing.T) {
	t.Parallel()

	if _, err := parseRipgrepJSON([]byte("{not-json}\n"), 10); err == nil {
		t.Fatal("parseRipgrepJSON(malformed) error = nil")
	}
}

func TestLimitedSearchBufferEnforcesSharedLimit(t *testing.T) {
	t.Parallel()

	remaining := int64(4)
	exceeded := false
	buffer := &limitedSearchBuffer{remaining: &remaining, exceeded: &exceeded, mu: &sync.Mutex{}}
	if _, err := buffer.Write([]byte("abcdef")); !errors.Is(err, errSearchOutputLimit) {
		t.Fatalf("Write() error = %v, want output limit", err)
	}
	if !exceeded || buffer.String() != "abcd" {
		t.Fatalf("buffer = %q exceeded=%v", buffer.String(), exceeded)
	}
}

func writeEvidenceRepoFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
