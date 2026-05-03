package githubfake

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type Server struct {
	URL string

	server *httptest.Server
	mu     sync.Mutex

	User           User
	PullRequest    PullRequest
	Diff           string
	Files          []ChangedFile
	IssueComments  []IssueComment
	ReviewComments []ReviewComment
	Reviews        []PullReview

	nextReviewID int64
	ReviewWrites []ReviewWrite
}

type User struct {
	Login  string
	Scopes []string
}

type PullRequest struct {
	Owner   string
	Repo    string
	Number  int64
	Title   string
	HTMLURL string
	Author  string
	BaseRef string
	BaseSHA string
	HeadRef string
	HeadSHA string
}

type ChangedFile struct {
	SHA              string `json:"sha,omitempty"`
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename,omitempty"`
	Status           string `json:"status"`
	Additions        int64  `json:"additions"`
	Deletions        int64  `json:"deletions"`
	Changes          int64  `json:"changes"`
	Patch            string `json:"patch,omitempty"`
	BlobURL          string `json:"blob_url,omitempty"`
	RawURL           string `json:"raw_url,omitempty"`
}

type IssueComment struct {
	ID                int64   `json:"id"`
	Body              string  `json:"body"`
	HTMLURL           string  `json:"html_url"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	AuthorAssociation string  `json:"author_association"`
	User              UserRef `json:"user"`
}

type ReviewComment struct {
	ID                  int64   `json:"id"`
	PullRequestReviewID int64   `json:"pull_request_review_id"`
	Body                string  `json:"body"`
	HTMLURL             string  `json:"html_url"`
	Path                string  `json:"path"`
	DiffHunk            string  `json:"diff_hunk"`
	CommitID            string  `json:"commit_id"`
	OriginalCommitID    string  `json:"original_commit_id"`
	Line                int64   `json:"line"`
	OriginalLine        int64   `json:"original_line"`
	StartLine           int64   `json:"start_line,omitempty"`
	OriginalStartLine   int64   `json:"original_start_line,omitempty"`
	Side                string  `json:"side"`
	StartSide           string  `json:"start_side,omitempty"`
	InReplyToID         int64   `json:"in_reply_to_id,omitempty"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
	AuthorAssociation   string  `json:"author_association"`
	User                UserRef `json:"user"`
}

type PullReview struct {
	ID                int64   `json:"id"`
	Body              string  `json:"body"`
	State             string  `json:"state"`
	HTMLURL           string  `json:"html_url"`
	CommitID          string  `json:"commit_id"`
	SubmittedAt       string  `json:"submitted_at"`
	AuthorAssociation string  `json:"author_association"`
	User              UserRef `json:"user"`
}

type UserRef struct {
	Login string `json:"login"`
}

type ReviewWrite struct {
	Owner    string
	Repo     string
	Number   int64
	ReviewID int64
	Action   string
	Payload  map[string]any
}

func New(t testing.TB) *Server {
	t.Helper()

	fake := &Server{
		User: User{
			Login:  "octocat",
			Scopes: []string{"repo", "read:user"},
		},
		PullRequest: PullRequest{
			Owner:   "octo-org",
			Repo:    "hello-world",
			Number:  42,
			Title:   "Tighten repository auth",
			HTMLURL: "https://github.com/octo-org/hello-world/pull/42",
			Author:  "mona",
			BaseRef: "main",
			BaseSHA: "base-sha",
			HeadRef: "feature",
			HeadSHA: "head-sha",
		},
		Diff:         defaultDiff,
		nextReviewID: 1000,
	}
	fake.Files = []ChangedFile{{
		SHA:       "file-sha",
		Filename:  "apps/api/src/routes/repositories.ts",
		Status:    "modified",
		Additions: 2,
		Deletions: 1,
		Changes:   3,
		Patch:     "@@ -87,3 +87,4 @@\n-old\n+new\n+guard\n",
	}}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	fake.URL = fake.server.URL
	t.Cleanup(fake.Close)
	return fake
}

func (s *Server) Close() {
	if s.server != nil {
		s.server.Close()
	}
}

func (s *Server) ReviewWritesSnapshot() []ReviewWrite {
	s.mu.Lock()
	defer s.mu.Unlock()

	writes := make([]ReviewWrite, len(s.ReviewWrites))
	copy(writes, s.ReviewWrites)
	return writes
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "bad credentials"})
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/user" {
		s.handleUser(w)
		return
	}

	owner, repo, collection, number, tail, ok := parseRepoPath(r.URL.Path)
	if !ok || owner != s.PullRequest.Owner || repo != s.PullRequest.Repo || number != s.PullRequest.Number {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
		return
	}

	switch {
	case r.Method == http.MethodGet && collection == "pulls" && len(tail) == 0:
		if strings.Contains(r.Header.Get("Accept"), "application/vnd.github.diff") {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(s.Diff))
			return
		}
		writeJSON(w, http.StatusOK, s.pullRequestBody())
	case r.Method == http.MethodGet && collection == "pulls" && len(tail) == 1 && tail[0] == "files":
		writePaged(w, r, s.Files)
	case r.Method == http.MethodGet && collection == "issues" && len(tail) == 1 && tail[0] == "comments":
		writePaged(w, r, s.IssueComments)
	case r.Method == http.MethodGet && collection == "pulls" && len(tail) == 1 && tail[0] == "comments":
		writePaged(w, r, s.ReviewComments)
	case r.Method == http.MethodGet && collection == "pulls" && len(tail) == 1 && tail[0] == "reviews":
		writePaged(w, r, s.Reviews)
	case r.Method == http.MethodPost && collection == "pulls" && len(tail) == 1 && tail[0] == "reviews":
		s.handleReviewCreate(w, r, owner, repo, number)
	case r.Method == http.MethodPost && collection == "pulls" && len(tail) == 3 && tail[0] == "reviews" && tail[2] == "events":
		reviewID, err := strconv.ParseInt(tail[1], 10, 64)
		if err != nil || reviewID <= 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
			return
		}
		s.handleReviewSubmit(w, r, owner, repo, number, reviewID)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	}
}

func (s *Server) handleUser(w http.ResponseWriter) {
	w.Header().Set("X-OAuth-Scopes", strings.Join(s.User.Scopes, ", "))
	writeJSON(w, http.StatusOK, map[string]string{"login": s.User.Login})
}

func (s *Server) pullRequestBody() map[string]any {
	pr := s.PullRequest
	return map[string]any{
		"title":    pr.Title,
		"html_url": pr.HTMLURL,
		"user":     map[string]string{"login": pr.Author},
		"base":     map[string]string{"ref": pr.BaseRef, "sha": pr.BaseSHA},
		"head":     map[string]string{"ref": pr.HeadRef, "sha": pr.HeadSHA},
	}
}

func (s *Server) handleReviewCreate(w http.ResponseWriter, r *http.Request, owner string, repo string, number int64) {
	payload, ok := decodePayload(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	s.nextReviewID++
	reviewID := s.nextReviewID
	event, _ := payload["event"].(string)
	state := strings.ToLower(strings.TrimSpace(event))
	if state == "" {
		state = "pending"
	}
	write := ReviewWrite{Owner: owner, Repo: repo, Number: number, ReviewID: reviewID, Action: "create", Payload: payload}
	s.ReviewWrites = append(s.ReviewWrites, write)
	s.mu.Unlock()

	status := http.StatusCreated
	writeJSON(w, status, map[string]any{
		"id":           reviewID,
		"state":        state,
		"html_url":     fmt.Sprintf("https://github.com/%s/%s/pull/%d#pullrequestreview-%d", owner, repo, number, reviewID),
		"commit_id":    stringValue(payload["commit_id"], s.PullRequest.HeadSHA),
		"submitted_at": "2026-05-04T00:00:00Z",
	})
}

func (s *Server) handleReviewSubmit(w http.ResponseWriter, r *http.Request, owner string, repo string, number int64, reviewID int64) {
	payload, ok := decodePayload(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	s.ReviewWrites = append(s.ReviewWrites, ReviewWrite{Owner: owner, Repo: repo, Number: number, ReviewID: reviewID, Action: "submit", Payload: payload})
	s.mu.Unlock()

	state := strings.ToLower(stringValue(payload["event"], "comment"))
	writeJSON(w, http.StatusOK, map[string]any{
		"id":           reviewID,
		"state":        state,
		"html_url":     fmt.Sprintf("https://github.com/%s/%s/pull/%d#pullrequestreview-%d", owner, repo, number, reviewID),
		"commit_id":    s.PullRequest.HeadSHA,
		"submitted_at": "2026-05-04T00:01:00Z",
	})
}

func parseRepoPath(path string) (string, string, string, int64, []string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 5 || parts[0] != "repos" {
		return "", "", "", 0, nil, false
	}
	collection := parts[3]
	if collection != "pulls" && collection != "issues" {
		return "", "", "", 0, nil, false
	}
	number, err := strconv.ParseInt(parts[4], 10, 64)
	if err != nil || number <= 0 {
		return "", "", "", 0, nil, false
	}
	return parts[1], parts[2], collection, number, parts[5:], true
}

func writePaged[T any](w http.ResponseWriter, r *http.Request, items []T) {
	perPage := queryInt(r, "per_page", 100)
	page := queryInt(r, "page", 1)
	if perPage <= 0 {
		perPage = 100
	}
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * perPage
	if start >= len(items) {
		writeJSON(w, http.StatusOK, []T{})
		return
	}
	end := start + perPage
	if end > len(items) {
		end = len(items)
	}
	if end < len(items) {
		w.Header().Set("Link", fmt.Sprintf("<%s?per_page=%d&page=%d>; rel=\"next\"", r.URL.Path, perPage, page+1))
	}
	writeJSON(w, http.StatusOK, items[start:end])
}

func queryInt(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return fallback
	}
	return value
}

func decodePayload(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	defer r.Body.Close()
	payload := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": "invalid json"})
		return nil, false
	}
	return payload, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func stringValue(value any, fallback string) string {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return fallback
	}
	return text
}

const defaultDiff = `diff --git a/apps/api/src/routes/repositories.ts b/apps/api/src/routes/repositories.ts
--- a/apps/api/src/routes/repositories.ts
+++ b/apps/api/src/routes/repositories.ts
@@ -87,3 +87,4 @@ function updateRepositorySettings() {
-  return updateSettings()
+  requireWorkspaceAdmin()
+  return updateSettings()
 }
`
