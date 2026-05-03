package githubpr

import (
	"context"
	"errors"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
)

func TestParseURLValidShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		owner     string
		repo      string
		number    int64
		canonical string
	}{
		{
			name:      "standard https url",
			raw:       "https://github.com/openai/codex/pull/123",
			owner:     "openai",
			repo:      "codex",
			number:    123,
			canonical: "https://github.com/openai/codex/pull/123",
		},
		{
			name:      "url with files suffix and query",
			raw:       "https://github.com/hughdo/cocode/pull/42/files?diff=split#discussion_r1",
			owner:     "hughdo",
			repo:      "cocode",
			number:    42,
			canonical: "https://github.com/hughdo/cocode/pull/42",
		},
		{
			name:      "host without scheme",
			raw:       "github.com/org-name/repo.name/pull/7",
			owner:     "org-name",
			repo:      "repo.name",
			number:    7,
			canonical: "https://github.com/org-name/repo.name/pull/7",
		},
		{
			name:      "www host",
			raw:       "https://www.github.com/owner/repo_name/pull/9",
			owner:     "owner",
			repo:      "repo_name",
			number:    9,
			canonical: "https://github.com/owner/repo_name/pull/9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ref, err := ParseURL(context.Background(), tt.raw)
			if err != nil {
				t.Fatalf("ParseURL() error = %v", err)
			}
			if ref.Owner != tt.owner || ref.Repo != tt.repo || ref.Number != tt.number || ref.CanonicalURL != tt.canonical {
				t.Fatalf("ParseURL() = %+v", ref)
			}
		})
	}
}

func TestParseURLInvalidShapes(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"not a url",
		"ssh://github.com/openai/codex/pull/1",
		"https://gitlab.com/openai/codex/pull/1",
		"https://github.com/openai/codex/issues/1",
		"https://github.com/openai/codex/pull/",
		"https://github.com/openai/codex/pull/abc",
		"https://github.com/openai/codex/pull/0",
		"https://github.com/openai/../pull/1",
	}

	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			_, err := ParseURL(context.Background(), raw)
			if err == nil {
				t.Fatal("ParseURL() error = nil, want error")
			}
			var appErr *apperror.Error
			if !errors.As(err, &appErr) {
				t.Fatalf("ParseURL() error = %T, want *apperror.Error", err)
			}
			if appErr.Code != apperror.CodeInvalidRequest {
				t.Fatalf("Code = %q, want %q", appErr.Code, apperror.CodeInvalidRequest)
			}
		})
	}
}
