package githubpr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
)

func TestFetchMetadataGetsPullRequestFields(t *testing.T) {
	t.Parallel()

	const token = "ghp_token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/openai/codex/pulls/123" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Fatalf("Accept = %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("X-GitHub-Api-Version") != gitHubAPIVersion {
			t.Fatalf("X-GitHub-Api-Version = %q", r.Header.Get("X-GitHub-Api-Version"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"title": "Add repository settings panel",
			"html_url": "https://github.com/openai/codex/pull/123",
			"user": {"login": "octocat"},
			"base": {"ref": "main", "sha": "base-sha"},
			"head": {"ref": "feature/settings", "sha": "head-sha"}
		}`))
	}))
	defer server.Close()

	metadata, err := (Client{
		BaseURL: server.URL,
		Token:   token,
		Client:  server.Client(),
	}).FetchMetadata(context.Background(), Reference{
		Owner:        "openai",
		Repo:         "codex",
		Number:       123,
		CanonicalURL: "https://github.com/openai/codex/pull/123",
	})
	if err != nil {
		t.Fatalf("FetchMetadata() error = %v", err)
	}
	if metadata.Title != "Add repository settings panel" ||
		metadata.Author != "octocat" ||
		metadata.BaseRef != "main" ||
		metadata.HeadRef != "feature/settings" ||
		metadata.BaseSHA != "base-sha" ||
		metadata.HeadSHA != "head-sha" ||
		metadata.URL != "https://github.com/openai/codex/pull/123" {
		t.Fatalf("FetchMetadata() = %+v", metadata)
	}
}

func TestFetchMetadataRequiresToken(t *testing.T) {
	t.Parallel()

	_, err := (Client{}).FetchMetadata(context.Background(), Reference{Owner: "openai", Repo: "codex", Number: 123})
	assertClientAppError(t, err, apperror.CodeInvalidRequest)
}

func TestFetchChangedFilesFollowsPagination(t *testing.T) {
	t.Parallel()

	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.String())
		if r.URL.Path != "/repos/openai/codex/pulls/123/files" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Fatalf("Accept = %q", r.Header.Get("Accept"))
		}

		page := r.URL.Query().Get("page")
		switch page {
		case "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/openai/codex/pulls/123/files?per_page=100&page=2>; rel="next"`, serverURLPlaceholder))
			_, _ = w.Write([]byte(`[{
				"sha": "sha-1",
				"filename": "apps/api/routes.ts",
				"status": "modified",
				"additions": 12,
				"deletions": 3,
				"changes": 15,
				"patch": "@@ -1,2 +1,3 @@"
			}]`))
		case "2":
			_, _ = w.Write([]byte(`[{
				"sha": "sha-2",
				"filename": "apps/api/new name.ts",
				"previous_filename": "apps/api/old name.ts",
				"status": "renamed",
				"additions": 4,
				"deletions": 1,
				"changes": 5
			}]`))
		default:
			t.Fatalf("page = %q", page)
		}
	}))
	defer server.Close()

	files, err := (Client{
		BaseURL: server.URL,
		Token:   "token",
		Client:  server.Client(),
	}).FetchChangedFiles(context.Background(), Reference{Owner: "openai", Repo: "codex", Number: 123})
	if err != nil {
		t.Fatalf("FetchChangedFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2: %+v", len(files), files)
	}
	if files[0].Filename != "apps/api/routes.ts" || files[0].Patch == "" {
		t.Fatalf("first file = %+v", files[0])
	}
	if files[1].PreviousFilename != "apps/api/old name.ts" || files[1].Patch != "" {
		t.Fatalf("second file = %+v", files[1])
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %v, want 2 pages", requests)
	}
}

func TestFetchDiffUsesDiffMediaType(t *testing.T) {
	t.Parallel()

	const diff = "diff --git a/a.go b/a.go\n@@ -1 +1 @@\n-old\n+new\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/openai/codex/pulls/123" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/vnd.github.diff" {
			t.Fatalf("Accept = %q", r.Header.Get("Accept"))
		}
		_, _ = w.Write([]byte(diff))
	}))
	defer server.Close()

	content, err := (Client{
		BaseURL: server.URL,
		Token:   "token",
		Client:  server.Client(),
	}).FetchDiff(context.Background(), Reference{Owner: "openai", Repo: "codex", Number: 123})
	if err != nil {
		t.Fatalf("FetchDiff() error = %v", err)
	}
	if string(content) != diff {
		t.Fatalf("FetchDiff() = %q", string(content))
	}
}

func TestFetchMetadataMapsAuthAndNotFoundErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		code   apperror.Code
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, code: apperror.CodeUnauthorized},
		{name: "forbidden", status: http.StatusForbidden, code: apperror.CodeUnauthorized},
		{name: "not found", status: http.StatusNotFound, code: apperror.CodeNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			_, err := (Client{
				BaseURL: server.URL,
				Token:   "token",
				Client:  server.Client(),
			}).FetchMetadata(context.Background(), Reference{Owner: "openai", Repo: "codex", Number: 123})
			assertClientAppError(t, err, tt.code)
		})
	}
}

func TestFetchMetadataRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{`))
	}))
	defer server.Close()

	_, err := (Client{
		BaseURL: server.URL,
		Token:   "token",
		Client:  server.Client(),
	}).FetchMetadata(context.Background(), Reference{Owner: "openai", Repo: "codex", Number: 123})
	assertClientAppError(t, err, apperror.CodeInternal)
}

const serverURLPlaceholder = "https://api.github.test"

func assertClientAppError(t *testing.T, err error, code apperror.Code) {
	t.Helper()

	if err == nil {
		t.Fatal("error = nil, want *apperror.Error")
	}
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %T, want *apperror.Error", err)
	}
	if appErr.Code != code {
		t.Fatalf("Code = %q, want %q", appErr.Code, code)
	}
}
