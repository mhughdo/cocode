package githubpr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
)

const defaultAPIBaseURL = "https://api.github.com"

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

func (c Client) FetchMetadata(ctx context.Context, ref Reference) (Metadata, error) {
	if strings.TrimSpace(c.Token) == "" {
		return Metadata{}, apperror.InvalidRequest("GitHub token is required")
	}
	if ref.Owner == "" || ref.Repo == "" || ref.Number <= 0 {
		return Metadata{}, apperror.InvalidRequest("GitHub pull request reference is invalid")
	}

	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultAPIBaseURL
	}
	endpoint := baseURL + "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + "/pulls/" + strconv.FormatInt(ref.Number, 10)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Metadata{}, apperror.Internal("failed to build GitHub pull request request")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.Token))
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Metadata{}, apperror.Internal("failed to fetch GitHub pull request metadata")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return Metadata{}, apperror.Unauthorized("GitHub token was rejected")
	case http.StatusNotFound:
		return Metadata{}, apperror.NotFound("GitHub pull request was not found")
	default:
		return Metadata{}, apperror.Internal("GitHub pull request metadata request failed")
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
