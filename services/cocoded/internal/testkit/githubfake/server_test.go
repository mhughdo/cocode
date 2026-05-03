package githubfake

import (
	"context"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/githubauth"
	"github.com/hughdo/cocode/services/cocoded/internal/githubpr"
)

func TestServerSupportsGitHubClientIngestionFlow(t *testing.T) {
	t.Parallel()

	server := New(t)
	server.Files = append(server.Files, manyChangedFiles(125)...)
	server.IssueComments = []IssueComment{{
		ID:                11,
		Body:              "Please check auth.",
		HTMLURL:           "https://github.com/octo-org/hello-world/pull/42#issuecomment-11",
		CreatedAt:         "2026-05-03T00:00:00Z",
		UpdatedAt:         "2026-05-03T00:00:00Z",
		AuthorAssociation: "MEMBER",
		User:              UserRef{Login: "reviewer"},
	}}
	server.ReviewComments = []ReviewComment{{
		ID:                  21,
		PullRequestReviewID: 31,
		Body:                "Line comment.",
		HTMLURL:             "https://github.com/octo-org/hello-world/pull/42#discussion_r21",
		Path:                "apps/api/src/routes/repositories.ts",
		Line:                88,
		Side:                "RIGHT",
		CreatedAt:           "2026-05-03T00:01:00Z",
		UpdatedAt:           "2026-05-03T00:01:00Z",
		AuthorAssociation:   "MEMBER",
		User:                UserRef{Login: "reviewer"},
	}}
	server.Reviews = []PullReview{
		{
			ID:                41,
			Body:              "Submitted review.",
			State:             "COMMENTED",
			HTMLURL:           "https://github.com/octo-org/hello-world/pull/42#pullrequestreview-41",
			CommitID:          "head-sha",
			SubmittedAt:       "2026-05-03T00:02:00Z",
			AuthorAssociation: "MEMBER",
			User:              UserRef{Login: "reviewer"},
		},
		{
			ID:    42,
			State: "PENDING",
			User:  UserRef{Login: "reviewer"},
		},
	}

	ref := githubpr.Reference{
		Owner:        server.PullRequest.Owner,
		Repo:         server.PullRequest.Repo,
		Number:       server.PullRequest.Number,
		CanonicalURL: server.PullRequest.HTMLURL,
	}
	client := githubpr.Client{BaseURL: server.URL, Token: "ghp_test"}

	metadata, err := client.FetchMetadata(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchMetadata() error = %v", err)
	}
	if metadata.Title != server.PullRequest.Title || metadata.BaseSHA != server.PullRequest.BaseSHA || metadata.HeadSHA != server.PullRequest.HeadSHA {
		t.Fatalf("metadata = %+v", metadata)
	}

	files, err := client.FetchChangedFiles(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchChangedFiles() error = %v", err)
	}
	if len(files) != 126 {
		t.Fatalf("files len = %d, want 126", len(files))
	}

	diff, err := client.FetchDiff(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchDiff() error = %v", err)
	}
	if !strings.Contains(string(diff), "requireWorkspaceAdmin") {
		t.Fatalf("diff = %s", string(diff))
	}

	comments, err := client.FetchPreviousComments(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchPreviousComments() error = %v", err)
	}
	if comments.IssueCommentCount != 1 || comments.ReviewCommentCount != 1 || comments.ReviewCount != 1 || len(comments.Comments) != 3 {
		t.Fatalf("comments = %+v", comments)
	}
}

func TestServerSupportsValidationAndReviewWrites(t *testing.T) {
	t.Parallel()

	server := New(t)
	validator := githubauth.HTTPTokenValidator{BaseURL: server.URL}
	result, err := validator.Validate(context.Background(), "ghp_test")
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Login != "octocat" || len(result.Scopes) != 2 {
		t.Fatalf("validation result = %+v", result)
	}

	ref := githubpr.Reference{
		Owner:  server.PullRequest.Owner,
		Repo:   server.PullRequest.Repo,
		Number: server.PullRequest.Number,
	}
	client := githubpr.Client{BaseURL: server.URL, Token: "ghp_test"}
	published, err := client.SubmitReview(context.Background(), ref, githubpr.SubmitReviewParams{
		CommitID: "head-sha",
		Body:     "Review body",
		Event:    "COMMENT",
		Comments: []githubpr.ReviewCommentDraft{{
			Path: "apps/api/src/routes/repositories.ts",
			Line: 88,
			Side: "RIGHT",
			Body: "Please add a guard.",
		}},
	})
	if err != nil {
		t.Fatalf("SubmitReview() error = %v", err)
	}
	pending, err := client.CreatePendingReview(context.Background(), ref, githubpr.SubmitReviewParams{Body: "Pending body"})
	if err != nil {
		t.Fatalf("CreatePendingReview() error = %v", err)
	}
	submitted, err := client.SubmitPendingReview(context.Background(), ref, pending.ID, "Submit pending", "REQUEST_CHANGES")
	if err != nil {
		t.Fatalf("SubmitPendingReview() error = %v", err)
	}

	writes := server.ReviewWritesSnapshot()
	if published.ID == 0 || pending.ID == 0 || submitted.State != "request_changes" || len(writes) != 3 {
		t.Fatalf("published=%+v pending=%+v submitted=%+v writes=%+v", published, pending, submitted, writes)
	}
	if writes[0].Action != "create" || writes[0].Payload["event"] != "COMMENT" || writes[2].Action != "submit" {
		t.Fatalf("writes = %+v", writes)
	}
}

func manyChangedFiles(count int) []ChangedFile {
	files := make([]ChangedFile, count)
	for i := range files {
		files[i] = ChangedFile{
			Filename:  "apps/api/src/generated/file.go",
			Status:    "modified",
			Additions: 1,
			Deletions: 1,
			Changes:   2,
		}
	}
	return files
}
