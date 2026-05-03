package testkit

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/app"
)

func TestOpenDBAppliesMigrations(t *testing.T) {
	t.Parallel()

	fixture := OpenDB(t)
	if fixture.DB == nil || fixture.Queries == nil {
		t.Fatalf("fixture = %+v", fixture)
	}
	var migrationCount int
	if err := fixture.DB.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("count schema migrations: %v", err)
	}
	if migrationCount == 0 {
		t.Fatal("migrationCount = 0, want applied migrations")
	}
}

func TestNewHTTPRouterServesHealthWithTestConfig(t *testing.T) {
	t.Parallel()

	fixture := NewHTTPRouter(t, appConfigForTest(t))
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()
	fixture.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"service":"cocoded"`) {
		t.Fatalf("health body = %s", response.Body.String())
	}
}

func TestFakeAgentHelpersResolveAndWriteExecutableAgents(t *testing.T) {
	t.Parallel()

	path := FakeAgentPath(t, "json-agent.sh")
	if !strings.HasSuffix(path, filepath.Join("testdata", "fake-agents", "json-agent.sh")) {
		t.Fatalf("fake agent path = %s", path)
	}
	custom := WriteAgent(t, "#!/bin/sh\nprintf ok\n")
	info, err := os.Stat(custom)
	if err != nil {
		t.Fatalf("stat custom agent: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("custom agent mode = %v, want executable", info.Mode())
	}
}

func TestGitRepoFixtureWritesAndCommitsFiles(t *testing.T) {
	t.Parallel()

	repo := InitGitRepo(t)
	repo.WriteFile(t, "src/app.go", "package src\n")
	repo.RunGit(t, "add", ".")
	repo.RunGit(t, "commit", "-m", "initial")

	if _, err := os.Stat(filepath.Join(repo.Path, "src", "app.go")); err != nil {
		t.Fatalf("stat repo file: %v", err)
	}
}

func appConfigForTest(t testing.TB) app.Config {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "data")
	return app.Config{
		Addr:        "127.0.0.1:0",
		AuthToken:   DefaultAuthToken,
		DataDir:     dataDir,
		DBPath:      filepath.Join(dataDir, "cocoded.sqlite"),
		ArtifactDir: filepath.Join(t.TempDir(), "artifacts"),
		Version:     "test-version",
	}
}
