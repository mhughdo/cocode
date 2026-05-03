package githubpr

import (
	"context"
	"encoding/json"
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

func TestFetchPreviousCommentsCollectsReviewContext(t *testing.T) {
	t.Parallel()

	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.String())
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Fatalf("Accept = %q", r.Header.Get("Accept"))
		}

		switch r.URL.Path {
		case "/repos/openai/codex/issues/123/comments":
			switch r.URL.Query().Get("page") {
			case "1":
				w.Header().Set("Link", `<https://api.github.test/repos/openai/codex/issues/123/comments?per_page=100&page=2>; rel="next"`)
				_, _ = w.Write([]byte(`[{
					"id": 10,
					"body": "Please add a test for this.",
					"html_url": "https://github.com/openai/codex/pull/123#issuecomment-10",
					"created_at": "2026-05-03T10:00:00Z",
					"updated_at": "2026-05-03T10:00:10Z",
					"author_association": "MEMBER",
					"user": {"login": "reviewer-a"}
				}]`))
			case "2":
				_, _ = w.Write([]byte(`[{
					"id": 11,
					"body": "Looks good after the update.",
					"html_url": "https://github.com/openai/codex/pull/123#issuecomment-11",
					"created_at": "2026-05-03T10:03:00Z",
					"updated_at": "2026-05-03T10:03:00Z",
					"author_association": "COLLABORATOR",
					"user": {"login": "reviewer-b"}
				}]`))
			default:
				t.Fatalf("issue comment page = %q", r.URL.Query().Get("page"))
			}
		case "/repos/openai/codex/pulls/123/comments":
			_, _ = w.Write([]byte(`[{
				"id": 20,
				"pull_request_review_id": 99,
				"body": "This duplicates an existing helper.",
				"html_url": "https://github.com/openai/codex/pull/123#discussion_r20",
				"path": "api/routes.go",
				"diff_hunk": "@@ -8,6 +8,7 @@",
				"commit_id": "head-sha",
				"original_commit_id": "old-head-sha",
				"line": 42,
				"original_line": 40,
				"start_line": 39,
				"original_start_line": 38,
				"side": "RIGHT",
				"start_side": "RIGHT",
				"in_reply_to_id": 19,
				"created_at": "2026-05-03T10:01:00Z",
				"updated_at": "2026-05-03T10:01:30Z",
				"author_association": "MEMBER",
				"user": {"login": "reviewer-a"}
			}]`))
		case "/repos/openai/codex/pulls/123/reviews":
			_, _ = w.Write([]byte(`[{
				"id": 99,
				"body": "Leaving a couple of comments.",
				"state": "COMMENTED",
				"html_url": "https://github.com/openai/codex/pull/123#pullrequestreview-99",
				"commit_id": "head-sha",
				"submitted_at": "2026-05-03T10:02:00Z",
				"author_association": "MEMBER",
				"user": {"login": "reviewer-a"}
			}, {
				"id": 100,
				"body": "Draft feedback should not be context yet.",
				"state": "PENDING",
				"html_url": "https://github.com/openai/codex/pull/123#pullrequestreview-100",
				"commit_id": "head-sha",
				"user": {"login": "reviewer-a"}
			}]`))
		default:
			t.Fatalf("unexpected path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	comments, err := (Client{
		BaseURL: server.URL,
		Token:   "token",
		Client:  server.Client(),
	}).FetchPreviousComments(context.Background(), Reference{Owner: "openai", Repo: "codex", Number: 123})
	if err != nil {
		t.Fatalf("FetchPreviousComments() error = %v", err)
	}
	if comments.IssueCommentCount != 2 || comments.ReviewCommentCount != 1 || comments.ReviewCount != 1 || len(comments.Comments) != 4 {
		t.Fatalf("comments counts = %+v", comments)
	}
	gotSources := []string{
		comments.Comments[0].Source,
		comments.Comments[1].Source,
		comments.Comments[2].Source,
		comments.Comments[3].Source,
	}
	wantSources := []string{"issue_comment", "review_comment", "review", "issue_comment"}
	for index, want := range wantSources {
		if gotSources[index] != want {
			t.Fatalf("sources = %v, want %v", gotSources, wantSources)
		}
	}
	inline := comments.Comments[1]
	if inline.ID != 20 ||
		inline.ReviewID != 99 ||
		inline.Path != "api/routes.go" ||
		inline.Line != 42 ||
		inline.OriginalLine != 40 ||
		inline.InReplyToID != 19 ||
		inline.Author != "reviewer-a" {
		t.Fatalf("inline review comment = %+v", inline)
	}
	review := comments.Comments[2]
	if review.ID != 99 || review.State != "COMMENTED" || review.CommitID != "head-sha" || review.SubmittedAt == "" {
		t.Fatalf("review = %+v", review)
	}
	if len(requests) != 4 {
		t.Fatalf("requests = %v, want 4 paged requests", requests)
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

func TestSubmitReviewSendsReviewPayload(t *testing.T) {
	t.Parallel()

	const token = "ghp_token"
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/openai/codex/pulls/123/reviews" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"id": 7001,
			"state": "COMMENTED",
			"html_url": "https://github.com/openai/codex/pull/123#pullrequestreview-7001",
			"commit_id": "head-sha",
			"submitted_at": "2026-05-03T12:00:00Z"
		}`))
	}))
	defer server.Close()

	review, err := (Client{
		BaseURL: server.URL,
		Token:   token,
		Client:  server.Client(),
	}).SubmitReview(context.Background(), Reference{Owner: "openai", Repo: "codex", Number: 123}, SubmitReviewParams{
		CommitID: "head-sha",
		Body:     "Please address the selected findings.",
		Event:    "COMMENT",
		Comments: []ReviewCommentDraft{{
			FindingID: "finding_auth",
			Path:      "app/auth.go",
			Body:      "Please require admin permissions.",
			Line:      12,
			Side:      SideRight,
			Position:  3,
		}},
	})
	if err != nil {
		t.Fatalf("SubmitReview() error = %v", err)
	}
	if review.ID != 7001 || review.CommitID != "head-sha" || review.HTMLURL == "" {
		t.Fatalf("review = %+v", review)
	}
	comments, ok := payload["comments"].([]any)
	if payload["event"] != "COMMENT" ||
		payload["commit_id"] != "head-sha" ||
		payload["body"] != "Please address the selected findings." ||
		!ok ||
		len(comments) != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	comment := comments[0].(map[string]any)
	if comment["path"] != "app/auth.go" ||
		comment["body"] != "Please require admin permissions." ||
		comment["line"].(float64) != 12 ||
		comment["side"] != SideRight {
		t.Fatalf("comment payload = %+v", comment)
	}
	if _, hasPosition := comment["position"]; hasPosition {
		t.Fatalf("payload should use line/side instead of deprecated position: %+v", comment)
	}
}

func TestSubmitReviewRejectsUnanchoredComments(t *testing.T) {
	t.Parallel()

	_, err := (Client{Token: "token"}).SubmitReview(context.Background(), Reference{Owner: "openai", Repo: "codex", Number: 123}, SubmitReviewParams{
		Body:  "Body",
		Event: "COMMENT",
		Comments: []ReviewCommentDraft{{
			FindingID:  "finding_missing",
			Body:       "Cannot anchor",
			Unanchored: true,
		}},
	})
	assertClientAppError(t, err, apperror.CodeInvalidRequest)
}

func TestSubmitSummaryReviewSendsBodyWithoutComments(t *testing.T) {
	t.Parallel()

	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":7002,"state":"COMMENTED","html_url":"https://github.com/openai/codex/pull/123#pullrequestreview-7002"}`))
	}))
	defer server.Close()

	review, err := (Client{
		BaseURL: server.URL,
		Token:   "token",
		Client:  server.Client(),
	}).SubmitSummaryReview(context.Background(), Reference{Owner: "openai", Repo: "codex", Number: 123}, "head-sha", "Summary body only.", "COMMENT")
	if err != nil {
		t.Fatalf("SubmitSummaryReview() error = %v", err)
	}
	if review.ID != 7002 {
		t.Fatalf("review = %+v", review)
	}
	if payload["body"] != "Summary body only." ||
		payload["event"] != "COMMENT" ||
		payload["commit_id"] != "head-sha" {
		t.Fatalf("payload = %+v", payload)
	}
	if _, ok := payload["comments"]; ok {
		t.Fatalf("summary-only payload should omit comments: %+v", payload)
	}
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
