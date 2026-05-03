package goldenrepo

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	AuthBug             = "go-api-auth-bug"
	WebhookValidation   = "webhook-validation-bug"
	GeneratedFilesNoise = "generated-files-noise"
)

func Path(t testing.TB, name string) string {
	t.Helper()

	if name == "" || strings.Contains(name, "/") || strings.Contains(name, string(os.PathSeparator)) {
		t.Fatalf("golden repo name %q is invalid", name)
	}
	root := repoRoot(t)
	path := filepath.Join(root, "testdata", "repos", name)
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("golden repo %q is unavailable: %v", name, err)
	}
	if !stat.IsDir() {
		t.Fatalf("golden repo %q is not a directory", name)
	}
	return path
}

func repoRoot(t testing.TB) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "pnpm-workspace.yaml")); err != nil {
		t.Fatalf("resolve repository root from %s: %v", file, err)
	}
	return root
}
