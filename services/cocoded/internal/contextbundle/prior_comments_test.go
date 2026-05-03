package contextbundle

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/githubpr"
)

func TestBuildPriorCommentContextItemsFromJSON(t *testing.T) {
	t.Parallel()

	comments := githubpr.PreviousComments{
		IssueCommentCount:  1,
		ReviewCommentCount: 1,
		ReviewCount:        1,
		Comments: []githubpr.PreviousComment{
			{
				Source:    "issue_comment",
				ID:        10,
				Author:    "reviewer-a",
				Body:      "Please avoid publishing the same auth finding twice.",
				HTMLURL:   "https://github.test/pull/1#issuecomment-10",
				CreatedAt: "2026-01-02T10:00:00Z",
			},
			{
				Source:      "review_comment",
				ID:          11,
				ReviewID:    99,
				Author:      "reviewer-b",
				Body:        "This nil check has already been discussed.",
				DiffHunk:    "@@ -40,2 +40,2 @@\n return user.name\n",
				HTMLURL:     "https://github.test/pull/1#discussion_r11",
				Path:        "src/auth.ts",
				Line:        42,
				StartLine:   40,
				Side:        "RIGHT",
				UpdatedAt:   "2026-01-03T10:00:00Z",
				InReplyToID: 7,
			},
			{
				Source:      "review",
				ID:          12,
				State:       "APPROVED",
				SubmittedAt: "2026-01-04T10:00:00Z",
			},
		},
	}
	raw, err := json.Marshal(comments)
	if err != nil {
		t.Fatalf("Marshal(comments) error = %v", err)
	}

	items, err := BuildPriorCommentContextItemsFromJSON(PriorCommentOptions{
		BundleID:                   "bundle_1",
		PreviousCommentsArtifactID: "artifact_comments",
		MaxItems:                   10,
		MaxCommentBytes:            4096,
	}, raw)
	if err != nil {
		t.Fatalf("BuildPriorCommentContextItemsFromJSON() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2: %+v", len(items), items)
	}
	if items[0].Path != "src/auth.ts" ||
		items[0].StartLine != 40 ||
		items[0].EndLine != 42 ||
		!strings.Contains(items[0].Content, "already been discussed") ||
		!strings.Contains(items[0].Content, "Diff hunk") {
		t.Fatalf("inline prior comment item = %+v", items[0])
	}
	assertPriorCommentMetadata(t, items[0].Metadata, "review_comment", int64(11), false)

	if items[1].Path != "" ||
		!strings.Contains(items[1].Content, "same auth finding twice") {
		t.Fatalf("issue prior comment item = %+v", items[1])
	}
	assertPriorCommentMetadata(t, items[1].Metadata, "issue_comment", int64(10), false)
}

func TestBuildPriorCommentContextItemsLimitsAndTruncates(t *testing.T) {
	t.Parallel()

	comments := githubpr.PreviousComments{
		Comments: []githubpr.PreviousComment{
			{
				Source:    "issue_comment",
				ID:        1,
				Body:      "older comment",
				CreatedAt: "2026-01-01T10:00:00Z",
			},
			{
				Source:    "issue_comment",
				ID:        2,
				Body:      strings.Repeat("newer duplicate note ", 20),
				CreatedAt: "2026-01-02T10:00:00Z",
			},
		},
	}

	items, err := BuildPriorCommentContextItems(PriorCommentOptions{
		BundleID:        "bundle_1",
		MaxItems:        1,
		MaxCommentBytes: 64,
	}, comments)
	if err != nil {
		t.Fatalf("BuildPriorCommentContextItems() error = %v", err)
	}
	if len(items) != 1 || !strings.Contains(items[0].Content, "#2") {
		t.Fatalf("items = %+v", items)
	}
	if len(items[0].Content) > 64 {
		t.Fatalf("content len = %d, want <= 64", len(items[0].Content))
	}
	assertPriorCommentMetadata(t, items[0].Metadata, "issue_comment", int64(2), true)
}

func TestBuildPriorCommentContextItemsHandlesMissingAndInvalidInputs(t *testing.T) {
	t.Parallel()

	items, err := BuildPriorCommentContextItemsFromJSON(PriorCommentOptions{BundleID: "bundle_1"}, []byte("  "))
	if err != nil || len(items) != 0 {
		t.Fatalf("empty JSON items = %+v, err = %v", items, err)
	}
	if _, err := BuildPriorCommentContextItemsFromJSON(PriorCommentOptions{BundleID: "bundle_1"}, []byte("{")); err == nil {
		t.Fatal("BuildPriorCommentContextItemsFromJSON(invalid) error = nil, want error")
	}
	if _, err := BuildPriorCommentContextItems(PriorCommentOptions{}, githubpr.PreviousComments{
		Comments: []githubpr.PreviousComment{{ID: 1, Body: "body"}},
	}); err == nil {
		t.Fatal("BuildPriorCommentContextItems(empty bundle) error = nil, want error")
	}
}

func assertPriorCommentMetadata(t *testing.T, raw json.RawMessage, source string, commentID int64, truncated bool) {
	t.Helper()

	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatalf("Unmarshal(metadata) error = %v", err)
	}
	if metadata["source"] != "previous_pr_comment" ||
		metadata["comment_source"] != source ||
		int64(metadata["comment_id"].(float64)) != commentID ||
		metadata["truncated"] != truncated {
		t.Fatalf("metadata = %+v", metadata)
	}
}
