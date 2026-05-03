package githubpr

import (
	"context"
	"errors"
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
