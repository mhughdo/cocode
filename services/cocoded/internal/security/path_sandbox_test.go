package security

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanRelativePathRejectsTraversalAndAbsolutePaths(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"../secret.txt", "safe/../../secret.txt", "/tmp/secret.txt", "safe/.."} {
		if _, err := CleanRelativePath(value); !errors.Is(err, ErrPathEscapesRoot) {
			t.Fatalf("CleanRelativePath(%q) error = %v, want ErrPathEscapesRoot", value, err)
		}
	}
	clean, err := CleanRelativePath("./safe\\nested.txt")
	if err != nil {
		t.Fatalf("CleanRelativePath() error = %v", err)
	}
	if clean != "safe/nested.txt" {
		t.Fatalf("clean = %q", clean)
	}
}

func TestResolveExistingWithinRootRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, _, err := ResolveExistingWithinRoot(root, "link.txt"); !errors.Is(err, ErrPathEscapesRoot) {
		t.Fatalf("ResolveExistingWithinRoot() error = %v, want ErrPathEscapesRoot", err)
	}
}

func TestResolveWriteWithinRootRejectsSymlinkParentEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "out")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, _, err := ResolveWriteWithinRoot(root, "out/artifact.txt"); !errors.Is(err, ErrPathEscapesRoot) {
		t.Fatalf("ResolveWriteWithinRoot() error = %v, want ErrPathEscapesRoot", err)
	}
}

func TestJoinWithinRootReturnsCleanPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target, clean, err := JoinWithinRoot(root, "nested/file.txt")
	if err != nil {
		t.Fatalf("JoinWithinRoot() error = %v", err)
	}
	resolvedRoot, err := ResolveRoot(root)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if clean != "nested/file.txt" || target != filepath.Join(resolvedRoot, "nested", "file.txt") {
		t.Fatalf("target = %q, clean = %q", target, clean)
	}
}
