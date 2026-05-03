package githubpr

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
)

const defaultAPIBaseURL = "https://api.github.com"
const gitHubAPIVersion = "2026-03-10"

type Client struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

type Metadata struct {
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	Number  int64  `json:"number"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	URL     string `json:"url"`
	BaseRef string `json:"base_ref"`
	HeadRef string `json:"head_ref"`
	BaseSHA string `json:"base_sha"`
	HeadSHA string `json:"head_sha"`
}

type ChangedFile struct {
	SHA              string `json:"sha"`
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

type PreviousComments struct {
	Comments           []PreviousComment `json:"comments"`
	IssueCommentCount  int               `json:"issue_comment_count"`
	ReviewCommentCount int               `json:"review_comment_count"`
	ReviewCount        int               `json:"review_count"`
}

type PreviousComment struct {
	Source            string `json:"source"`
	ID                int64  `json:"id"`
	ReviewID          int64  `json:"review_id,omitempty"`
	Author            string `json:"author,omitempty"`
	AuthorAssociation string `json:"author_association,omitempty"`
	Body              string `json:"body,omitempty"`
	State             string `json:"state,omitempty"`
	HTMLURL           string `json:"html_url,omitempty"`
	Path              string `json:"path,omitempty"`
	DiffHunk          string `json:"diff_hunk,omitempty"`
	CommitID          string `json:"commit_id,omitempty"`
	OriginalCommitID  string `json:"original_commit_id,omitempty"`
	Line              int64  `json:"line,omitempty"`
	OriginalLine      int64  `json:"original_line,omitempty"`
	StartLine         int64  `json:"start_line,omitempty"`
	OriginalStartLine int64  `json:"original_start_line,omitempty"`
	Side              string `json:"side,omitempty"`
	StartSide         string `json:"start_side,omitempty"`
	InReplyToID       int64  `json:"in_reply_to_id,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
	SubmittedAt       string `json:"submitted_at,omitempty"`
}

type SubmitReviewParams struct {
	CommitID string
	Body     string
	Event    string
	Comments []ReviewCommentDraft
}

type PublishedReview struct {
	ID          int64  `json:"id"`
	State       string `json:"state"`
	HTMLURL     string `json:"html_url"`
	CommitID    string `json:"commit_id"`
	SubmittedAt string `json:"submitted_at"`
}

func (c Client) FetchMetadata(ctx context.Context, ref Reference) (Metadata, error) {
	if ref.Owner == "" || ref.Repo == "" || ref.Number <= 0 {
		return Metadata{}, apperror.InvalidRequest("GitHub pull request reference is invalid")
	}

	resp, err := c.get(ctx, pullEndpoint(c.baseURL(), ref), "application/vnd.github+json")
	if err != nil {
		return Metadata{}, err
	}
	defer resp.Body.Close()

	if err := mapGitHubStatus(resp.StatusCode, "GitHub pull request"); err != nil {
		return Metadata{}, err
	}
	var body struct {
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
		User    struct {
			Login string `json:"login"`
		} `json:"user"`
		Base struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"base"`
		Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Metadata{}, apperror.Internal("failed to decode GitHub pull request metadata")
	}
	if body.HTMLURL == "" {
		body.HTMLURL = ref.CanonicalURL
	}

	return Metadata{
		Owner:   ref.Owner,
		Repo:    ref.Repo,
		Number:  ref.Number,
		Title:   body.Title,
		Author:  body.User.Login,
		URL:     body.HTMLURL,
		BaseRef: body.Base.Ref,
		HeadRef: body.Head.Ref,
		BaseSHA: body.Base.SHA,
		HeadSHA: body.Head.SHA,
	}, nil
}

func (c Client) FetchChangedFiles(ctx context.Context, ref Reference) ([]ChangedFile, error) {
	if ref.Owner == "" || ref.Repo == "" || ref.Number <= 0 {
		return nil, apperror.InvalidRequest("GitHub pull request reference is invalid")
	}

	baseEndpoint := pullEndpoint(c.baseURL(), ref) + "/files"
	return fetchPaged[ChangedFile](ctx, c, baseEndpoint, "GitHub pull request files")
}

func (c Client) FetchDiff(ctx context.Context, ref Reference) ([]byte, error) {
	if ref.Owner == "" || ref.Repo == "" || ref.Number <= 0 {
		return nil, apperror.InvalidRequest("GitHub pull request reference is invalid")
	}

	resp, err := c.get(ctx, pullEndpoint(c.baseURL(), ref), "application/vnd.github.diff")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := mapGitHubStatus(resp.StatusCode, "GitHub pull request diff"); err != nil {
		return nil, err
	}
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperror.Internal("failed to read GitHub pull request diff")
	}
	return content, nil
}

func (c Client) FetchPreviousComments(ctx context.Context, ref Reference) (PreviousComments, error) {
	if ref.Owner == "" || ref.Repo == "" || ref.Number <= 0 {
		return PreviousComments{}, apperror.InvalidRequest("GitHub pull request reference is invalid")
	}

	baseURL := c.baseURL()
	issueComments, err := fetchPaged[issueComment](ctx, c, issueCommentsEndpoint(baseURL, ref), "GitHub pull request issue comments")
	if err != nil {
		return PreviousComments{}, err
	}
	reviewComments, err := fetchPaged[reviewComment](ctx, c, pullEndpoint(baseURL, ref)+"/comments", "GitHub pull request review comments")
	if err != nil {
		return PreviousComments{}, err
	}
	reviews, err := fetchPaged[pullReview](ctx, c, pullEndpoint(baseURL, ref)+"/reviews", "GitHub pull request reviews")
	if err != nil {
		return PreviousComments{}, err
	}

	result := PreviousComments{
		IssueCommentCount:  len(issueComments),
		ReviewCommentCount: len(reviewComments),
		Comments:           make([]PreviousComment, 0, len(issueComments)+len(reviewComments)+len(reviews)),
	}
	for _, comment := range issueComments {
		result.Comments = append(result.Comments, PreviousComment{
			Source:            "issue_comment",
			ID:                comment.ID,
			Author:            comment.User.Login,
			AuthorAssociation: comment.AuthorAssociation,
			Body:              comment.Body,
			HTMLURL:           comment.HTMLURL,
			CreatedAt:         comment.CreatedAt,
			UpdatedAt:         comment.UpdatedAt,
		})
	}
	for _, comment := range reviewComments {
		result.Comments = append(result.Comments, PreviousComment{
			Source:            "review_comment",
			ID:                comment.ID,
			ReviewID:          comment.PullRequestReviewID,
			Author:            comment.User.Login,
			AuthorAssociation: comment.AuthorAssociation,
			Body:              comment.Body,
			HTMLURL:           comment.HTMLURL,
			Path:              comment.Path,
			DiffHunk:          comment.DiffHunk,
			CommitID:          comment.CommitID,
			OriginalCommitID:  comment.OriginalCommitID,
			Line:              comment.Line,
			OriginalLine:      comment.OriginalLine,
			StartLine:         comment.StartLine,
			OriginalStartLine: comment.OriginalStartLine,
			Side:              comment.Side,
			StartSide:         comment.StartSide,
			InReplyToID:       comment.InReplyToID,
			CreatedAt:         comment.CreatedAt,
			UpdatedAt:         comment.UpdatedAt,
		})
	}
	for _, review := range reviews {
		if strings.EqualFold(review.State, "PENDING") || strings.TrimSpace(review.SubmittedAt) == "" {
			continue
		}
		result.ReviewCount++
		result.Comments = append(result.Comments, PreviousComment{
			Source:            "review",
			ID:                review.ID,
			Author:            review.User.Login,
			AuthorAssociation: review.AuthorAssociation,
			Body:              review.Body,
			State:             review.State,
			HTMLURL:           review.HTMLURL,
			CommitID:          review.CommitID,
			SubmittedAt:       review.SubmittedAt,
		})
	}
	sort.SliceStable(result.Comments, func(i, j int) bool {
		left := commentTime(result.Comments[i])
		right := commentTime(result.Comments[j])
		if left != right {
			return left < right
		}
		if result.Comments[i].Source != result.Comments[j].Source {
			return result.Comments[i].Source < result.Comments[j].Source
		}
		return result.Comments[i].ID < result.Comments[j].ID
	})
	return result, nil
}

func (c Client) SubmitReview(ctx context.Context, ref Reference, params SubmitReviewParams) (PublishedReview, error) {
	if ref.Owner == "" || ref.Repo == "" || ref.Number <= 0 {
		return PublishedReview{}, apperror.InvalidRequest("GitHub pull request reference is invalid")
	}
	payload, err := submitReviewPayload(params)
	if err != nil {
		return PublishedReview{}, err
	}
	resp, err := c.postJSON(ctx, pullEndpoint(c.baseURL(), ref)+"/reviews", payload)
	if err != nil {
		return PublishedReview{}, err
	}
	defer resp.Body.Close()
	if err := mapGitHubWriteStatus(resp.StatusCode, "GitHub pull request review"); err != nil {
		return PublishedReview{}, err
	}
	var body PublishedReview
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return PublishedReview{}, apperror.Internal("failed to decode GitHub pull request review")
	}
	return body, nil
}

func (c Client) SubmitSummaryReview(ctx context.Context, ref Reference, commitID string, body string, event string) (PublishedReview, error) {
	return c.SubmitReview(ctx, ref, SubmitReviewParams{
		CommitID: commitID,
		Body:     body,
		Event:    event,
	})
}

func submitReviewPayload(params SubmitReviewParams) (map[string]any, error) {
	event := strings.ToUpper(strings.TrimSpace(params.Event))
	switch event {
	case "COMMENT", "REQUEST_CHANGES", "APPROVE":
	default:
		return nil, apperror.InvalidRequest("GitHub review event is invalid")
	}
	payload := map[string]any{
		"body":  strings.TrimSpace(params.Body),
		"event": event,
	}
	if strings.TrimSpace(params.CommitID) != "" {
		payload["commit_id"] = strings.TrimSpace(params.CommitID)
	}
	comments := make([]map[string]any, 0, len(params.Comments))
	for _, comment := range params.Comments {
		if comment.Unanchored {
			return nil, apperror.InvalidRequest("GitHub review contains unanchored comments")
		}
		if strings.TrimSpace(comment.Path) == "" || strings.TrimSpace(comment.Body) == "" || comment.Line <= 0 || strings.TrimSpace(comment.Side) == "" {
			return nil, apperror.InvalidRequest("GitHub review comment is invalid")
		}
		comments = append(comments, map[string]any{
			"path": strings.TrimSpace(comment.Path),
			"body": strings.TrimSpace(comment.Body),
			"line": comment.Line,
			"side": strings.ToUpper(strings.TrimSpace(comment.Side)),
		})
	}
	if len(comments) > 0 {
		payload["comments"] = comments
	}
	return payload, nil
}

func (c Client) baseURL() string {
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultAPIBaseURL
	}
	return baseURL
}

func (c Client) get(ctx context.Context, endpoint string, accept string) (*http.Response, error) {
	if strings.TrimSpace(c.Token) == "" {
		return nil, apperror.InvalidRequest("GitHub token is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, apperror.Internal("failed to build GitHub request")
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.Token))
	req.Header.Set("X-GitHub-Api-Version", gitHubAPIVersion)

	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, apperror.Internal("failed to call GitHub")
	}
	return resp, nil
}

func (c Client) postJSON(ctx context.Context, endpoint string, payload any) (*http.Response, error) {
	if strings.TrimSpace(c.Token) == "" {
		return nil, apperror.InvalidRequest("GitHub token is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, apperror.Internal("failed to encode GitHub request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, apperror.Internal("failed to build GitHub request")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.Token))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", gitHubAPIVersion)

	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, apperror.Internal("failed to call GitHub")
	}
	return resp, nil
}

func pullEndpoint(baseURL string, ref Reference) string {
	return baseURL + "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + "/pulls/" + strconv.FormatInt(ref.Number, 10)
}

func issueCommentsEndpoint(baseURL string, ref Reference) string {
	return baseURL + "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + "/issues/" + strconv.FormatInt(ref.Number, 10) + "/comments"
}

func fetchPaged[T any](ctx context.Context, c Client, baseEndpoint string, resource string) ([]T, error) {
	items := []T{}
	for page := int64(1); ; page++ {
		endpoint := baseEndpoint + "?per_page=100&page=" + strconv.FormatInt(page, 10)
		resp, err := c.get(ctx, endpoint, "application/vnd.github+json")
		if err != nil {
			return nil, err
		}

		var pageItems []T
		decodeErr := error(nil)
		if statusErr := mapGitHubStatus(resp.StatusCode, resource); statusErr == nil {
			decodeErr = json.NewDecoder(resp.Body).Decode(&pageItems)
		} else {
			decodeErr = statusErr
		}
		_ = resp.Body.Close()
		if decodeErr != nil {
			if _, ok := decodeErr.(*apperror.Error); ok {
				return nil, decodeErr
			}
			return nil, apperror.Internal("failed to decode " + strings.ToLower(resource))
		}
		items = append(items, pageItems...)
		if !hasNextPage(resp.Header.Get("Link")) {
			break
		}
	}
	return items, nil
}

type userSummary struct {
	Login string `json:"login"`
}

type issueComment struct {
	ID                int64       `json:"id"`
	Body              string      `json:"body"`
	HTMLURL           string      `json:"html_url"`
	CreatedAt         string      `json:"created_at"`
	UpdatedAt         string      `json:"updated_at"`
	AuthorAssociation string      `json:"author_association"`
	User              userSummary `json:"user"`
}

type reviewComment struct {
	ID                  int64       `json:"id"`
	PullRequestReviewID int64       `json:"pull_request_review_id"`
	Body                string      `json:"body"`
	HTMLURL             string      `json:"html_url"`
	Path                string      `json:"path"`
	DiffHunk            string      `json:"diff_hunk"`
	CommitID            string      `json:"commit_id"`
	OriginalCommitID    string      `json:"original_commit_id"`
	Line                int64       `json:"line"`
	OriginalLine        int64       `json:"original_line"`
	StartLine           int64       `json:"start_line"`
	OriginalStartLine   int64       `json:"original_start_line"`
	Side                string      `json:"side"`
	StartSide           string      `json:"start_side"`
	InReplyToID         int64       `json:"in_reply_to_id"`
	CreatedAt           string      `json:"created_at"`
	UpdatedAt           string      `json:"updated_at"`
	AuthorAssociation   string      `json:"author_association"`
	User                userSummary `json:"user"`
}

type pullReview struct {
	ID                int64       `json:"id"`
	Body              string      `json:"body"`
	State             string      `json:"state"`
	HTMLURL           string      `json:"html_url"`
	CommitID          string      `json:"commit_id"`
	SubmittedAt       string      `json:"submitted_at"`
	AuthorAssociation string      `json:"author_association"`
	User              userSummary `json:"user"`
}

func commentTime(comment PreviousComment) string {
	switch {
	case comment.CreatedAt != "":
		return comment.CreatedAt
	case comment.SubmittedAt != "":
		return comment.SubmittedAt
	case comment.UpdatedAt != "":
		return comment.UpdatedAt
	default:
		return ""
	}
}

func mapGitHubStatus(statusCode int, resource string) error {
	switch statusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return apperror.Unauthorized("GitHub token was rejected")
	case http.StatusNotFound:
		return apperror.NotFound(resource + " was not found")
	case http.StatusUnprocessableEntity:
		return apperror.InvalidRequest(resource + " request was rejected by GitHub")
	default:
		return apperror.Internal(resource + " request failed")
	}
}

func mapGitHubWriteStatus(statusCode int, resource string) error {
	switch statusCode {
	case http.StatusOK, http.StatusCreated:
		return nil
	default:
		return mapGitHubStatus(statusCode, resource)
	}
}

func hasNextPage(linkHeader string) bool {
	for _, part := range strings.Split(linkHeader, ",") {
		sections := strings.Split(part, ";")
		if len(sections) < 2 {
			continue
		}
		for _, section := range sections[1:] {
			if strings.TrimSpace(section) == `rel="next"` {
				return true
			}
		}
	}
	return false
}
