package githubpr

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
)

type Reference struct {
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Number       int64  `json:"number"`
	CanonicalURL string `json:"canonical_url"`
}

func ParseURL(_ context.Context, raw string) (Reference, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Reference{}, apperror.InvalidRequest("GitHub PR URL is required")
	}
	if strings.HasPrefix(raw, "github.com/") || strings.HasPrefix(raw, "www.github.com/") {
		raw = "https://" + raw
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return Reference{}, apperror.InvalidRequest("GitHub PR URL is invalid")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return Reference{}, apperror.InvalidRequest("GitHub PR URL must use http or https")
	}

	host := strings.ToLower(parsed.Hostname())
	host = strings.TrimPrefix(host, "www.")
	if host != "github.com" {
		return Reference{}, apperror.InvalidRequest("GitHub PR URL must be on github.com")
	}

	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return Reference{}, apperror.InvalidRequest("GitHub PR URL must look like https://github.com/{owner}/{repo}/pull/{number}")
	}

	owner, err := url.PathUnescape(parts[0])
	if err != nil || !validPathSegment(owner) {
		return Reference{}, apperror.InvalidRequest("GitHub PR owner is invalid")
	}
	repo, err := url.PathUnescape(parts[1])
	if err != nil || !validPathSegment(repo) {
		return Reference{}, apperror.InvalidRequest("GitHub PR repository is invalid")
	}

	numberText, err := url.PathUnescape(parts[3])
	if err != nil {
		return Reference{}, apperror.InvalidRequest("GitHub PR number is invalid")
	}
	number, err := strconv.ParseInt(numberText, 10, 64)
	if err != nil || number <= 0 {
		return Reference{}, apperror.InvalidRequest("GitHub PR number is invalid")
	}

	return Reference{
		Owner:        owner,
		Repo:         repo,
		Number:       number,
		CanonicalURL: "https://github.com/" + owner + "/" + repo + "/pull/" + strconv.FormatInt(number, 10),
	}, nil
}

func validPathSegment(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." {
		return false
	}
	if strings.Contains(value, "/") || strings.Contains(value, "\\") {
		return false
	}
	return true
}
