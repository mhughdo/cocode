package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/app"
	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestHealthEndpoint(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var envelope Envelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	data, ok := envelope.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected health data object, got %T", envelope.Data)
	}

	if data["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", data["status"])
	}
	if data["service"] != "cocoded" {
		t.Fatalf("expected service cocoded, got %v", data["service"])
	}
	if data["version"] != "test-version" {
		t.Fatalf("expected version test-version, got %v", data["version"])
	}
}

func TestVersionEndpoint(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
}

func TestAuthenticatedRouteRejectsMissingToken(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestAuthenticatedRouteAcceptsToken(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
}

func TestChangedFilesEndpointReturnsSnapshotFiles(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPISnapshot(t, queries)

	request := httptest.NewRequest(http.MethodGet, "/api/pr-snapshots/snapshot_1/changed-files", nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}

	var envelope struct {
		Data  []ChangedFileResponse `json:"data"`
		Error any                   `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	if len(envelope.Data) != 2 {
		t.Fatalf("changed files len = %d, want 2", len(envelope.Data))
	}
	generated := envelope.Data[0]
	if generated.Path != "generated/api.pb.go" ||
		!generated.IsGenerated ||
		!generated.IsExcluded ||
		generated.IsBinary ||
		string(generated.LineRanges) != `[[1,4]]` {
		t.Fatalf("generated response = %+v line ranges %s", generated, string(generated.LineRanges))
	}
	renamed := envelope.Data[1]
	if renamed.Path != "src/new.go" ||
		renamed.OldPath != "src/old.go" ||
		renamed.Status != "renamed" ||
		renamed.PatchArtifactID != "artifact_patch" ||
		renamed.IsBinary {
		t.Fatalf("renamed response = %+v", renamed)
	}
}

func TestChangedFilesEndpointRejectsMissingSnapshot(t *testing.T) {
	router, _ := testRouterWithQueries(t)

	request := httptest.NewRequest(http.MethodGet, "/api/pr-snapshots/missing/changed-files", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, response.Code, response.Body.String())
	}
}

func testRouter(t *testing.T) http.Handler {
	router, _ := testRouterWithQueries(t)
	return router
}

func testRouterWithQueries(t *testing.T) (http.Handler, *dbgen.Queries) {
	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		if t != nil {
			t.Fatalf("Open() error = %v", err)
		}
		panic(err)
	}
	if t != nil {
		t.Cleanup(func() {
			_ = database.Close()
		})
	}
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		if t != nil {
			t.Fatalf("Apply() error = %v", err)
		}
		panic(err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))
	return NewRouter(app.Config{
		Addr:      "127.0.0.1:0",
		AuthToken: "test-token",
		DataDir:   "/tmp/cocode-test",
		Version:   "test-version",
	}, logger, database), dbgen.New(database)
}

func createHTTPAPISnapshot(t *testing.T, queries *dbgen.Queries) {
	t.Helper()

	if _, err := queries.CreateWorkspace(context.Background(), dbgen.CreateWorkspaceParams{
		ID:           "workspace_1",
		Name:         "cocode",
		RootPath:     "/tmp/cocode",
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
		LocalPath:   "/tmp/cocode",
		CreatedAt:   "2026-05-03T00:01:00Z",
		UpdatedAt:   "2026-05-03T00:01:00Z",
	}); err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
	if _, err := queries.CreatePullRequestSnapshot(context.Background(), dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_1",
		RepositoryID: "repo_1",
		SourceType:   "branch_compare",
		BaseRef:      nullableString("main"),
		HeadRef:      nullableString("feature"),
		BaseSha:      nullableString("base-sha"),
		HeadSha:      nullableString("head-sha"),
		MetadataJson: "{}",
		CreatedAt:    "2026-05-03T00:02:00Z",
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}
	if _, err := queries.CreateChangedFile(context.Background(), dbgen.CreateChangedFileParams{
		ID:              "file_2",
		SnapshotID:      "snapshot_1",
		Path:            "src/new.go",
		OldPath:         nullableString("src/old.go"),
		Status:          "renamed",
		Additions:       2,
		Deletions:       1,
		LineRangesJson:  `[[8,9]]`,
		PatchArtifactID: nullableString("artifact_patch"),
		CreatedAt:       "2026-05-03T00:03:00Z",
	}); err != nil {
		t.Fatalf("CreateChangedFile(renamed) error = %v", err)
	}
	if _, err := queries.CreateChangedFile(context.Background(), dbgen.CreateChangedFileParams{
		ID:             "file_1",
		SnapshotID:     "snapshot_1",
		Path:           "generated/api.pb.go",
		Status:         "modified",
		Additions:      4,
		IsGenerated:    1,
		IsExcluded:     1,
		LineRangesJson: `[[1,4]]`,
		CreatedAt:      "2026-05-03T00:04:00Z",
	}); err != nil {
		t.Fatalf("CreateChangedFile(generated) error = %v", err)
	}
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}
