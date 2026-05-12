package githubpr

import (
	"context"
	"strings"
	"testing"
)

func TestGHClientFetchesMetadataAndChangedFiles(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/api/routes.go b/api/routes.go\n",
		"index 1111111..2222222 100644\n",
		"--- a/api/routes.go\n",
		"+++ b/api/routes.go\n",
		"@@ -1 +1 @@\n",
		"-old\n",
		"+new\n",
		"diff --git a/docs/new.md b/docs/new.md\n",
		"new file mode 100644\n",
		"--- /dev/null\n",
		"+++ b/docs/new.md\n",
		"@@ -0,0 +1,2 @@\n",
		"+hello\n",
		"+world\n",
	}, "")
	runner := fakeGHRunner{
		responses: map[string][]byte{
			fakeGHKey("gh", "pr", "view", "123", "--repo", "openai/codex", "--json", "title,url,author,baseRefName,headRefName,baseRefOid,headRefOid"): []byte(`{
				"title": "Add gh snapshot",
				"url": "https://github.com/openai/codex/pull/123",
				"author": {"login": "octocat"},
				"baseRefName": "main",
				"headRefName": "feature/gh",
				"baseRefOid": "base-sha",
				"headRefOid": "head-sha"
			}`),
			fakeGHKey("gh", "pr", "diff", "123", "--repo", "openai/codex", "--patch"): []byte(diff),
		},
	}
	client := GHClient{Runner: &runner}
	ref := Reference{Owner: "openai", Repo: "codex", Number: 123}

	metadata, err := client.FetchMetadata(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchMetadata() error = %v", err)
	}
	if metadata.Title != "Add gh snapshot" || metadata.Author != "octocat" || metadata.BaseSHA != "base-sha" || metadata.HeadSHA != "head-sha" {
		t.Fatalf("metadata = %+v", metadata)
	}

	files, err := client.FetchChangedFiles(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchChangedFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files len = %d, want 2: %+v", len(files), files)
	}
	if files[0].Filename != "api/routes.go" || files[0].Status != "modified" || files[0].Additions != 1 || files[0].Deletions != 1 || files[0].Patch == "" {
		t.Fatalf("first file = %+v", files[0])
	}
	if files[1].Filename != "docs/new.md" || files[1].Status != "added" || files[1].Additions != 2 || files[1].Deletions != 0 {
		t.Fatalf("second file = %+v", files[1])
	}
}

func TestGHClientFetchPreviousComments(t *testing.T) {
	runner := fakeGHRunner{
		responses: map[string][]byte{
			fakeGHKey("gh", "api", "--paginate", "--slurp", "repos/openai/codex/issues/123/comments"): []byte(`[[{
				"id": 10,
				"body": "Looks good overall.",
				"html_url": "https://github.com/openai/codex/pull/123#issuecomment-10",
				"created_at": "2026-05-03T10:00:00Z",
				"updated_at": "2026-05-03T10:00:00Z",
				"user": {"login": "reviewer-a"}
			}]]`),
			fakeGHKey("gh", "api", "--paginate", "--slurp", "repos/openai/codex/pulls/123/comments"): []byte(`[[{
				"id": 20,
				"pull_request_review_id": 99,
				"body": "Please keep this anchored.",
				"html_url": "https://github.com/openai/codex/pull/123#discussion_r20",
				"path": "api/routes.go",
				"line": 12,
				"created_at": "2026-05-03T10:01:00Z",
				"updated_at": "2026-05-03T10:01:00Z",
				"user": {"login": "reviewer-b"}
			}]]`),
			fakeGHKey("gh", "api", "--paginate", "--slurp", "repos/openai/codex/pulls/123/reviews"): []byte(`[[{
				"id": 99,
				"body": "Ready after the route tweak.",
				"state": "COMMENTED",
				"html_url": "https://github.com/openai/codex/pull/123#pullrequestreview-99",
				"commit_id": "head-sha",
				"submitted_at": "2026-05-03T10:02:00Z",
				"user": {"login": "reviewer-b"}
			}]]`),
		},
	}
	comments, err := (GHClient{Runner: &runner}).FetchPreviousComments(context.Background(), Reference{Owner: "openai", Repo: "codex", Number: 123})
	if err != nil {
		t.Fatalf("FetchPreviousComments() error = %v", err)
	}
	if comments.IssueCommentCount != 1 || comments.ReviewCommentCount != 1 || comments.ReviewCount != 1 || len(comments.Comments) != 3 {
		t.Fatalf("comments = %+v", comments)
	}
	if comments.Comments[1].Path != "api/routes.go" || comments.Comments[1].ReviewID != 99 {
		t.Fatalf("review comment = %+v", comments.Comments[1])
	}
}

func TestDecodeGHAPIListAcceptsPlainAndSlurpedArrays(t *testing.T) {
	plain, err := decodeGHAPIList[issueComment]([]byte(`[{"id":1},{"id":2}]`))
	if err != nil {
		t.Fatalf("decode plain array error = %v", err)
	}
	slurped, err := decodeGHAPIList[issueComment]([]byte(`[[{"id":1}],[{"id":2}]]`))
	if err != nil {
		t.Fatalf("decode slurped array error = %v", err)
	}
	if len(plain) != 2 || len(slurped) != 2 || plain[0].ID != slurped[0].ID || plain[1].ID != slurped[1].ID {
		t.Fatalf("plain = %+v slurped = %+v", plain, slurped)
	}
}

type fakeGHRunner struct {
	responses map[string][]byte
}

func (f *fakeGHRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := fakeGHKey(name, args...)
	if response, ok := f.responses[key]; ok {
		return response, nil
	}
	return nil, &fakeGHError{key: key}
}

type fakeGHError struct {
	key string
}

func (e *fakeGHError) Error() string {
	return "unexpected gh command: " + e.key
}

func fakeGHKey(name string, args ...string) string {
	return name + "\x00" + strings.Join(args, "\x00")
}
