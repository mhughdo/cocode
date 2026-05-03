package githubpr

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
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

	files := []ChangedFile{}
	baseEndpoint := pullEndpoint(c.baseURL(), ref) + "/files"
	for page := int64(1); ; page++ {
		endpoint := baseEndpoint + "?per_page=100&page=" + strconv.FormatInt(page, 10)
		resp, err := c.get(ctx, endpoint, "application/vnd.github+json")
		if err != nil {
			return nil, err
		}

		var pageFiles []ChangedFile
		decodeErr := error(nil)
		if statusErr := mapGitHubStatus(resp.StatusCode, "GitHub pull request files"); statusErr == nil {
			decodeErr = json.NewDecoder(resp.Body).Decode(&pageFiles)
		} else {
			decodeErr = statusErr
		}
		_ = resp.Body.Close()
		if decodeErr != nil {
			if _, ok := decodeErr.(*apperror.Error); ok {
				return nil, decodeErr
			}
			return nil, apperror.Internal("failed to decode GitHub pull request files")
		}
		files = append(files, pageFiles...)
		if !hasNextPage(resp.Header.Get("Link")) {
			break
		}
	}
	return files, nil
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

func pullEndpoint(baseURL string, ref Reference) string {
	return baseURL + "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + "/pulls/" + strconv.FormatInt(ref.Number, 10)
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
