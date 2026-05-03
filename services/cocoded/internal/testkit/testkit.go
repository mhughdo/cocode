package testkit

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/app"
	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/httpapi"
)

const DefaultAuthToken = "test-token"

type DBFixture struct {
	DB      *sql.DB
	Queries *dbgen.Queries
}

type RouterFixture struct {
	DBFixture
	Handler http.Handler
	Config  app.Config
}

type GitRepoFixture struct {
	Path string
}

func OpenDB(t testing.TB) DBFixture {
	t.Helper()

	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Fatalf("db.Close() error = %v", err)
		}
	})
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("db.Apply() error = %v", err)
	}
	return DBFixture{
		DB:      database,
		Queries: dbgen.New(database),
	}
}

func NewHTTPRouter(t testing.TB, config app.Config) RouterFixture {
	t.Helper()

	fixture := OpenDB(t)
	if config.Addr == "" {
		config.Addr = "127.0.0.1:0"
	}
	if config.AuthToken == "" {
		config.AuthToken = DefaultAuthToken
	}
	if config.DataDir == "" {
		config.DataDir = filepath.Join(t.TempDir(), "data")
	}
	if config.DBPath == "" {
		config.DBPath = filepath.Join(config.DataDir, "cocoded.sqlite")
	}
	if config.ArtifactDir == "" {
		config.ArtifactDir = filepath.Join(t.TempDir(), "artifacts")
	}
	if config.Version == "" {
		config.Version = "test-version"
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))
	return RouterFixture{
		DBFixture: fixture,
		Handler:   httpapi.NewRouter(config, logger, fixture.DB),
		Config:    config,
	}
}

func FakeAgentPath(t testing.TB, name string) string {
	t.Helper()

	if name == "" || strings.Contains(name, "/") || strings.Contains(name, string(os.PathSeparator)) {
		t.Fatalf("fake agent name %q is invalid", name)
	}
	path := filepath.Join(repoRoot(t), "testdata", "fake-agents", name)
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("fake agent %q is unavailable: %v", name, err)
	}
	if stat.IsDir() {
		t.Fatalf("fake agent %q is a directory", name)
	}
	return path
}

func WriteAgent(t testing.TB, script string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-agent")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake agent: %v", err)
	}
	return path
}

func InitGitRepo(t testing.TB) GitRepoFixture {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	path := t.TempDir()
	RunGit(t, path, "init")
	RunGit(t, path, "config", "user.email", "cocode@example.com")
	RunGit(t, path, "config", "user.name", "cocode")
	RunGit(t, path, "config", "commit.gpgsign", "false")
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve git repo path: %v", err)
	}
	return GitRepoFixture{Path: canonical}
}

func (r GitRepoFixture) WriteFile(t testing.TB, relativePath string, content string) {
	t.Helper()
	WriteRepoFile(t, r.Path, relativePath, content)
}

func (r GitRepoFixture) RunGit(t testing.TB, args ...string) {
	t.Helper()
	RunGit(t, r.Path, args...)
}

func WriteRepoFile(t testing.TB, repoPath string, relativePath string, content string) {
	t.Helper()

	clean := filepath.Clean(filepath.FromSlash(relativePath))
	if clean == "." || clean == "" || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || clean == ".." {
		t.Fatalf("repo file path %q is invalid", relativePath)
	}
	path := filepath.Join(repoPath, clean)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create repo file directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write repo file %s: %v", relativePath, err)
	}
}

func RunGit(t testing.TB, cwd string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, string(output))
	}
}

func repoRoot(t testing.TB) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "pnpm-workspace.yaml")); err != nil {
		t.Fatalf("resolve repository root from %s: %v", file, err)
	}
	return root
}
