package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
)

func TestWorkspaceEndpointsOpenAndListRepository(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	repoPath := initHTTPAPIGitRepo(t)
	runHTTPAPIGit(t, repoPath, "remote", "add", "origin", "git@github.com:hughdo/cocode.git")
	selectedPath := filepath.Join(repoPath, "apps", "desktop")
	if err := os.MkdirAll(selectedPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	openRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/workspaces/open-repository", map[string]any{
		"path": selectedPath,
	})
	openResponse := httptest.NewRecorder()
	router.ServeHTTP(openResponse, openRequest)
	if openResponse.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openResponse.Code, openResponse.Body.String())
	}
	opened := decodeHTTPAPIData[OpenRepositoryResponse](t, openResponse.Body.Bytes())
	if opened.Workspace.RootPath != repoPath {
		t.Fatalf("workspace root_path = %q, want %q", opened.Workspace.RootPath, repoPath)
	}
	if opened.Workspace.DefaultRepoID == nil || *opened.Workspace.DefaultRepoID != opened.Repository.ID {
		t.Fatalf("default_repo_id = %+v, repository ID = %q", opened.Workspace.DefaultRepoID, opened.Repository.ID)
	}
	if opened.Workspace.Settings["source"] != "git_repo_validation" {
		t.Fatalf("workspace settings = %+v", opened.Workspace.Settings)
	}
	if opened.Repository.LocalPath != repoPath {
		t.Fatalf("repository local_path = %q, want %q", opened.Repository.LocalPath, repoPath)
	}
	if opened.Repository.Owner == nil || *opened.Repository.Owner != "hughdo" {
		t.Fatalf("repository owner = %+v", opened.Repository.Owner)
	}
	if opened.Repository.RemoteURL == nil || *opened.Repository.RemoteURL != "git@github.com:hughdo/cocode.git" {
		t.Fatalf("repository remote_url = %+v", opened.Repository.RemoteURL)
	}
	if len(opened.Repositories) != 1 || opened.Repositories[0].ID != opened.Repository.ID {
		t.Fatalf("repositories = %+v", opened.Repositories)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/workspaces", nil)
	listRequest.Header.Set("X-Cocode-Token", "test-token")
	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list workspaces status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	workspaces := decodeHTTPAPIData[[]WorkspaceResponse](t, listResponse.Body.Bytes())
	if len(workspaces) != 1 || workspaces[0].ID != opened.Workspace.ID {
		t.Fatalf("workspaces = %+v", workspaces)
	}

	repositoriesRequest := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+opened.Workspace.ID+"/repositories", nil)
	repositoriesRequest.Header.Set("Authorization", "Bearer test-token")
	repositoriesResponse := httptest.NewRecorder()
	router.ServeHTTP(repositoriesResponse, repositoriesRequest)
	if repositoriesResponse.Code != http.StatusOK {
		t.Fatalf("list repositories status = %d, body = %s", repositoriesResponse.Code, repositoriesResponse.Body.String())
	}
	repositories := decodeHTTPAPIData[[]RepositoryResponse](t, repositoriesResponse.Body.Bytes())
	if len(repositories) != 1 || repositories[0].ID != opened.Repository.ID {
		t.Fatalf("repositories = %+v", repositories)
	}

	secondRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/workspaces/open-repository", map[string]any{
		"path": repoPath,
	})
	secondResponse := httptest.NewRecorder()
	router.ServeHTTP(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusOK {
		t.Fatalf("second open status = %d, body = %s", secondResponse.Code, secondResponse.Body.String())
	}
	second := decodeHTTPAPIData[OpenRepositoryResponse](t, secondResponse.Body.Bytes())
	if second.Workspace.ID != opened.Workspace.ID || second.Repository.ID != opened.Repository.ID {
		t.Fatalf("second open = %+v, first = %+v", second, opened)
	}
	storedWorkspaces, err := queries.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces() after second open error = %v", err)
	}
	if len(storedWorkspaces) != 1 {
		t.Fatalf("workspace count after second open = %d, want 1", len(storedWorkspaces))
	}
	storedRepositories, err := queries.ListRepositoriesByWorkspace(context.Background(), opened.Workspace.ID)
	if err != nil {
		t.Fatalf("ListRepositoriesByWorkspace() after second open error = %v", err)
	}
	if len(storedRepositories) != 1 {
		t.Fatalf("repository count after second open = %d, want 1", len(storedRepositories))
	}
}

func TestListRepositoryBranches(t *testing.T) {
	router, _ := testRouterWithQueries(t)
	repoPath := initHTTPAPIGitRepo(t)
	runHTTPAPIGit(t, repoPath, "checkout", "-B", "main")
	writeHTTPAPIRepoFile(t, repoPath, "app/main.go", "package main\n")
	runHTTPAPIGit(t, repoPath, "add", ".")
	runHTTPAPIGit(t, repoPath, "commit", "-m", "initial")
	runHTTPAPIGit(t, repoPath, "checkout", "-b", "feature/review")

	openRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/workspaces/open-repository", map[string]any{
		"path": repoPath,
	})
	openResponse := httptest.NewRecorder()
	router.ServeHTTP(openResponse, openRequest)
	if openResponse.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openResponse.Code, openResponse.Body.String())
	}
	opened := decodeHTTPAPIData[OpenRepositoryResponse](t, openResponse.Body.Bytes())

	branchesRequest := httptest.NewRequest(http.MethodGet, "/api/repositories/"+opened.Repository.ID+"/branches?workspace_id="+opened.Workspace.ID, nil)
	branchesRequest.Header.Set("X-Cocode-Token", "test-token")
	branchesResponse := httptest.NewRecorder()
	router.ServeHTTP(branchesResponse, branchesRequest)
	if branchesResponse.Code != http.StatusOK {
		t.Fatalf("branches status = %d, body = %s", branchesResponse.Code, branchesResponse.Body.String())
	}
	branches := decodeHTTPAPIData[[]RepositoryBranchResponse](t, branchesResponse.Body.Bytes())
	if len(branches) < 2 {
		t.Fatalf("branches = %+v, want at least main and feature/review", branches)
	}
	if branches[0].Name != "feature/review" || !branches[0].Current {
		t.Fatalf("current branch = %+v, want feature/review first", branches[0])
	}
	if !repositoryBranchNames(branches)["main"] {
		t.Fatalf("branches = %+v, want main", branches)
	}
}

func TestOpenRepositoryRejectsNonGitPathWithoutPersistence(t *testing.T) {
	router, queries := testRouterWithQueries(t)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/workspaces/open-repository", map[string]any{
		"path": t.TempDir(),
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("open non-git status = %d, body = %s", response.Code, response.Body.String())
	}
	appErr := decodeHTTPAPIError(t, response.Body.Bytes())
	if appErr.Code != apperror.CodeInvalidRequest {
		t.Fatalf("error code = %q, want %q", appErr.Code, apperror.CodeInvalidRequest)
	}

	workspaces, err := queries.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces() error = %v", err)
	}
	if len(workspaces) != 0 {
		t.Fatalf("workspaces after rejected open = %+v", workspaces)
	}
}

func TestListWorkspaceRepositoriesReturnsNotFoundForMissingWorkspace(t *testing.T) {
	router, _ := testRouterWithQueries(t)

	request := httptest.NewRequest(http.MethodGet, "/api/workspaces/workspace_missing/repositories", nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func repositoryBranchNames(branches []RepositoryBranchResponse) map[string]bool {
	names := make(map[string]bool, len(branches))
	for _, branch := range branches {
		names[branch.Name] = true
	}
	return names
}

func decodeHTTPAPIData[T any](t *testing.T, content []byte) T {
	t.Helper()

	var envelope struct {
		Data  T   `json:"data"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeHTTPAPIError(t *testing.T, content []byte) apperror.Error {
	t.Helper()

	var envelope struct {
		Data  any            `json:"data"`
		Error apperror.Error `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return envelope.Error
}
