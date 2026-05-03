package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestSnapshotEndpointReturnsSnapshot(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPISnapshot(t, queries)

	request := httptest.NewRequest(http.MethodGet, "/api/pr-snapshots/snapshot_1", nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	snapshot := decodeSnapshotResponse(t, response.Body.Bytes())
	if snapshot.ID != "snapshot_1" || snapshot.SourceType != "branch_compare" || snapshot.ChangedFileCount != 2 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestCreateGitHubSnapshotEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ghp_test" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.URL.Path == "/repos/openai/codex/pulls/123" && r.Header.Get("Accept") == "application/vnd.github+json":
			_, _ = w.Write([]byte(`{
				"title": "Add snapshot route",
				"html_url": "https://github.com/openai/codex/pull/123",
				"user": {"login": "octocat"},
				"base": {"ref": "main", "sha": "base-sha"},
				"head": {"ref": "feature/snapshot", "sha": "head-sha"}
			}`))
		case r.URL.Path == "/repos/openai/codex/pulls/123/files":
			_, _ = w.Write([]byte(`[{
				"sha": "file-sha",
				"filename": "api/routes.go",
				"status": "modified",
				"additions": 1,
				"deletions": 1,
				"changes": 2,
				"patch": "@@ -1 +1 @@\n-old\n+new\n"
			}]`))
		case r.URL.Path == "/repos/openai/codex/pulls/123" && r.Header.Get("Accept") == "application/vnd.github.diff":
			_, _ = w.Write([]byte("diff --git a/api/routes.go b/api/routes.go\n@@ -1 +1 @@\n-old\n+new\n"))
		case r.URL.Path == "/repos/openai/codex/issues/123/comments":
			_, _ = w.Write([]byte(`[{
				"id": 10,
				"body": "Please avoid duplicating the route helper.",
				"html_url": "https://github.com/openai/codex/pull/123#issuecomment-10",
				"created_at": "2026-05-03T10:00:00Z",
				"updated_at": "2026-05-03T10:00:00Z",
				"user": {"login": "reviewer-a"}
			}]`))
		case r.URL.Path == "/repos/openai/codex/pulls/123/comments":
			_, _ = w.Write([]byte(`[{
				"id": 20,
				"pull_request_review_id": 99,
				"body": "This branch already handles the edge case.",
				"html_url": "https://github.com/openai/codex/pull/123#discussion_r20",
				"path": "api/routes.go",
				"line": 12,
				"created_at": "2026-05-03T10:01:00Z",
				"updated_at": "2026-05-03T10:01:00Z",
				"user": {"login": "reviewer-b"}
			}]`))
		case r.URL.Path == "/repos/openai/codex/pulls/123/reviews":
			_, _ = w.Write([]byte(`[{
				"id": 99,
				"body": "One duplicate-avoidance note.",
				"state": "COMMENTED",
				"html_url": "https://github.com/openai/codex/pull/123#pullrequestreview-99",
				"commit_id": "head-sha",
				"submitted_at": "2026-05-03T10:02:00Z",
				"user": {"login": "reviewer-b"}
			}]`))
		default:
			t.Fatalf("unexpected GitHub request path=%s accept=%s", r.URL.Path, r.Header.Get("Accept"))
		}
	}))
	defer server.Close()

	router, queries := testRouterWithConfigAndQueries(t, app.Config{GitHubAPIBaseURL: server.URL})
	createHTTPAPIWorkspaceAndRepository(t, queries, "/tmp/cocode")

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/pr-snapshots/from-github-url", map[string]any{
		"workspace_id":  "workspace_1",
		"repository_id": "repo_1",
		"url":           "https://github.com/openai/codex/pull/123",
		"github_token":  "ghp_test",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	snapshot := decodeSnapshotResponse(t, response.Body.Bytes())
	if snapshot.SourceType != "github_pr" ||
		snapshot.Provider != "github" ||
		snapshot.Owner != "openai" ||
		snapshot.Repo != "codex" ||
		snapshot.PRNumber != 123 ||
		snapshot.ChangedFileCount != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if snapshot.PreviousCommentsArtifactID == "" {
		t.Fatalf("PreviousCommentsArtifactID is empty: %+v", snapshot)
	}
	commentsArtifact, err := queries.GetArtifact(context.Background(), snapshot.PreviousCommentsArtifactID)
	if err != nil {
		t.Fatalf("GetArtifact(previous comments) error = %v", err)
	}
	if commentsArtifact.Kind != "github_previous_comments" || commentsArtifact.ContentType != "application/json" {
		t.Fatalf("previous comments artifact = %+v", commentsArtifact)
	}
	var metadata struct {
		PreviousComments struct {
			ArtifactID         string `json:"artifact_id"`
			CommentCount       int    `json:"comment_count"`
			IssueCommentCount  int    `json:"issue_comment_count"`
			ReviewCommentCount int    `json:"review_comment_count"`
			ReviewCount        int    `json:"review_count"`
		} `json:"previous_comments"`
	}
	if err := json.Unmarshal(snapshot.Metadata, &metadata); err != nil {
		t.Fatalf("decode snapshot metadata: %v", err)
	}
	if metadata.PreviousComments.ArtifactID != snapshot.PreviousCommentsArtifactID ||
		metadata.PreviousComments.CommentCount != 3 ||
		metadata.PreviousComments.IssueCommentCount != 1 ||
		metadata.PreviousComments.ReviewCommentCount != 1 ||
		metadata.PreviousComments.ReviewCount != 1 {
		t.Fatalf("previous comments metadata = %+v", metadata.PreviousComments)
	}
	file, err := queries.GetChangedFileByPath(context.Background(), dbgen.GetChangedFileByPathParams{
		SnapshotID: snapshot.ID,
		Path:       "api/routes.go",
	})
	if err != nil {
		t.Fatalf("GetChangedFileByPath() error = %v", err)
	}
	if file.Additions != 1 || file.Deletions != 1 || file.PatchArtifactID.String == "" {
		t.Fatalf("changed file = %+v", file)
	}
}

func TestCreateGitHubSnapshotEndpointKeepsSnapshotWhenPreviousCommentsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ghp_test" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.URL.Path == "/repos/openai/codex/pulls/123" && r.Header.Get("Accept") == "application/vnd.github+json":
			_, _ = w.Write([]byte(`{
				"title": "Add snapshot route",
				"html_url": "https://github.com/openai/codex/pull/123",
				"user": {"login": "octocat"},
				"base": {"ref": "main", "sha": "base-sha"},
				"head": {"ref": "feature/snapshot", "sha": "head-sha"}
			}`))
		case r.URL.Path == "/repos/openai/codex/pulls/123/files":
			_, _ = w.Write([]byte(`[{
				"sha": "file-sha",
				"filename": "api/routes.go",
				"status": "modified",
				"additions": 1,
				"deletions": 1,
				"changes": 2,
				"patch": "@@ -1 +1 @@\n-old\n+new\n"
			}]`))
		case r.URL.Path == "/repos/openai/codex/pulls/123" && r.Header.Get("Accept") == "application/vnd.github.diff":
			_, _ = w.Write([]byte("diff --git a/api/routes.go b/api/routes.go\n@@ -1 +1 @@\n-old\n+new\n"))
		case r.URL.Path == "/repos/openai/codex/issues/123/comments":
			w.WriteHeader(http.StatusGone)
		default:
			t.Fatalf("unexpected GitHub request path=%s accept=%s", r.URL.Path, r.Header.Get("Accept"))
		}
	}))
	defer server.Close()

	router, queries := testRouterWithConfigAndQueries(t, app.Config{GitHubAPIBaseURL: server.URL})
	createHTTPAPIWorkspaceAndRepository(t, queries, "/tmp/cocode")

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/pr-snapshots/from-github-url", map[string]any{
		"workspace_id":  "workspace_1",
		"repository_id": "repo_1",
		"url":           "https://github.com/openai/codex/pull/123",
		"github_token":  "ghp_test",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	snapshot := decodeSnapshotResponse(t, response.Body.Bytes())
	if snapshot.PreviousCommentsArtifactID != "" || snapshot.ChangedFileCount != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	var metadata struct {
		PreviousCommentsFetchError string `json:"previous_comments_fetch_error"`
	}
	if err := json.Unmarshal(snapshot.Metadata, &metadata); err != nil {
		t.Fatalf("decode snapshot metadata: %v", err)
	}
	if metadata.PreviousCommentsFetchError == "" {
		t.Fatalf("previous comments fetch error is empty in metadata %s", string(snapshot.Metadata))
	}
	artifacts, err := queries.ListArtifactsByWorkspace(context.Background(), "workspace_1")
	if err != nil {
		t.Fatalf("ListArtifactsByWorkspace() error = %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("workspace artifacts len = %d, want diff and patch: %+v", len(artifacts), artifacts)
	}
}

func TestCreateLocalCompareSnapshotEndpoint(t *testing.T) {
	repoPath := initHTTPAPIGitRepo(t)
	runHTTPAPIGit(t, repoPath, "checkout", "-B", "main")
	writeHTTPAPIRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc main() {}\n")
	runHTTPAPIGit(t, repoPath, "add", ".")
	runHTTPAPIGit(t, repoPath, "commit", "-m", "initial")
	runHTTPAPIGit(t, repoPath, "checkout", "-b", "feature/api")
	writeHTTPAPIRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc main() {\n\tprintln(\"api\")\n}\n")
	runHTTPAPIGit(t, repoPath, "add", ".")
	runHTTPAPIGit(t, repoPath, "commit", "-m", "feature")

	router, queries := testRouterWithQueries(t)
	createHTTPAPIWorkspaceAndRepository(t, queries, repoPath)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/pr-snapshots/from-local-compare", map[string]any{
		"workspace_id":  "workspace_1",
		"repository_id": "repo_1",
		"base_ref":      "main",
		"head_ref":      "feature/api",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	snapshot := decodeSnapshotResponse(t, response.Body.Bytes())
	if snapshot.SourceType != "branch_compare" ||
		snapshot.BaseRef != "main" ||
		snapshot.HeadRef != "feature/api" ||
		snapshot.ChangedFileCount != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	files, err := queries.ListChangedFilesBySnapshot(context.Background(), snapshot.ID)
	if err != nil {
		t.Fatalf("ListChangedFilesBySnapshot() error = %v", err)
	}
	if len(files) != 1 || files[0].Path != "app/main.go" || files[0].LineRangesJson == "[]" {
		t.Fatalf("changed files = %+v", files)
	}
}

func TestCreateLocalChangesSnapshotEndpointIncludesUntrackedBinary(t *testing.T) {
	repoPath := initHTTPAPIGitRepo(t)
	runHTTPAPIGit(t, repoPath, "checkout", "-B", "main")
	writeHTTPAPIRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc main() {}\n")
	runHTTPAPIGit(t, repoPath, "add", ".")
	runHTTPAPIGit(t, repoPath, "commit", "-m", "initial")
	writeHTTPAPIRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc main() {\n\tprintln(\"local\")\n}\n")
	writeHTTPAPIRepoBytes(t, repoPath, "assets/logo.bin", []byte{0x00, 0x01})

	router, queries := testRouterWithQueries(t)
	createHTTPAPIWorkspaceAndRepository(t, queries, repoPath)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/pr-snapshots/from-local-changes", map[string]any{
		"workspace_id":  "workspace_1",
		"repository_id": "repo_1",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	snapshot := decodeSnapshotResponse(t, response.Body.Bytes())
	if snapshot.SourceType != "local_changes" || snapshot.ChangedFileCount != 2 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	binary, err := queries.GetChangedFileByPath(context.Background(), dbgen.GetChangedFileByPathParams{
		SnapshotID: snapshot.ID,
		Path:       "assets/logo.bin",
	})
	if err != nil {
		t.Fatalf("GetChangedFileByPath(binary) error = %v", err)
	}
	if binary.IsBinary != 1 || binary.IsExcluded != 1 {
		t.Fatalf("binary changed file = %+v", binary)
	}
}

func TestCreateSnapshotEndpointRejectsInvalidJSON(t *testing.T) {
	router, _ := testRouterWithQueries(t)

	request := httptest.NewRequest(http.MethodPost, "/api/pr-snapshots/from-local-changes", strings.NewReader("{"))
	request.Header.Set("X-Cocode-Token", "test-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, response.Code, response.Body.String())
	}
}

func testRouter(t *testing.T) http.Handler {
	router, _ := testRouterWithQueries(t)
	return router
}

func testRouterWithQueries(t *testing.T) (http.Handler, *dbgen.Queries) {
	return testRouterWithConfigAndQueries(t, app.Config{})
}

func testRouterWithConfigAndQueries(t *testing.T, config app.Config) (http.Handler, *dbgen.Queries) {
	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))
	if config.Addr == "" {
		config.Addr = "127.0.0.1:0"
	}
	if config.AuthToken == "" {
		config.AuthToken = "test-token"
	}
	if config.DataDir == "" {
		config.DataDir = "/tmp/cocode-test"
	}
	if config.ArtifactDir == "" {
		config.ArtifactDir = filepath.Join(t.TempDir(), "artifacts")
	}
	if config.Version == "" {
		config.Version = "test-version"
	}
	return NewRouter(config, logger, database), dbgen.New(database)
}

func createHTTPAPISnapshot(t *testing.T, queries *dbgen.Queries) {
	t.Helper()

	createHTTPAPIWorkspaceAndRepository(t, queries, "/tmp/cocode")
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

func createHTTPAPIWorkspaceAndRepository(t *testing.T, queries *dbgen.Queries, repoPath string) {
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
		CreatedAt:   "2026-05-03T00:01:00Z",
		UpdatedAt:   "2026-05-03T00:01:00Z",
	}); err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

func newAuthenticatedJSONRequest(t *testing.T, method string, path string, body any) *http.Request {
	t.Helper()

	content, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(content))
	request.Header.Set("X-Cocode-Token", "test-token")
	request.Header.Set("Content-Type", "application/json")
	return request
}

func decodeSnapshotResponse(t *testing.T, content []byte) SnapshotResponse {
	t.Helper()

	var envelope struct {
		Data  SnapshotResponse `json:"data"`
		Error any              `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func initHTTPAPIGitRepo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	repoPath := t.TempDir()
	runHTTPAPIGit(t, repoPath, "init")
	runHTTPAPIGit(t, repoPath, "config", "user.email", "cocode@example.com")
	runHTTPAPIGit(t, repoPath, "config", "user.name", "cocode")
	runHTTPAPIGit(t, repoPath, "config", "commit.gpgsign", "false")
	canonical, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	return canonical
}

func runHTTPAPIGit(t *testing.T, cwd string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, string(output))
	}
}

func writeHTTPAPIRepoFile(t *testing.T, repoPath string, relativePath string, content string) {
	t.Helper()
	writeHTTPAPIRepoBytes(t, repoPath, relativePath, []byte(content))
}

func writeHTTPAPIRepoBytes(t *testing.T, repoPath string, relativePath string, content []byte) {
	t.Helper()

	path := filepath.Join(repoPath, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", relativePath, err)
	}
}
