package contextbundle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type ReviewContextPolicy struct {
	IncludePromptMaterial     bool     `json:"include_prompt_material"`
	IncludeChangedCode        bool     `json:"include_changed_code"`
	IncludeRelatedCallSites   bool     `json:"include_related_call_sites"`
	IncludeRelatedTests       bool     `json:"include_related_tests"`
	IncludeProjectConventions bool     `json:"include_project_conventions"`
	IncludePriorComments      bool     `json:"include_prior_comments"`
	IncludePriorDecisions     bool     `json:"include_prior_decisions"`
	RedactSecrets             bool     `json:"redact_secrets"`
	LocalOnlyPaths            []string `json:"local_only_paths,omitempty"`
	MaxTokens                 int64    `json:"max_tokens"`
	MaxItems                  int      `json:"max_items"`
}

type reviewContextPolicyPatch struct {
	IncludePromptMaterial     *bool    `json:"include_prompt_material"`
	IncludeChangedCode        *bool    `json:"include_changed_code"`
	IncludeRelatedCallSites   *bool    `json:"include_related_call_sites"`
	IncludeRelatedTests       *bool    `json:"include_related_tests"`
	IncludeProjectConventions *bool    `json:"include_project_conventions"`
	IncludePriorComments      *bool    `json:"include_prior_comments"`
	IncludePriorDecisions     *bool    `json:"include_prior_decisions"`
	RedactSecrets             *bool    `json:"redact_secrets"`
	LocalOnlyPaths            []string `json:"local_only_paths"`
	MaxTokens                 *int64   `json:"max_tokens"`
	MaxItems                  *int     `json:"max_items"`
}

func DefaultReviewContextPolicy() ReviewContextPolicy {
	return ReviewContextPolicy{
		IncludePromptMaterial:     true,
		IncludeChangedCode:        true,
		IncludeRelatedCallSites:   true,
		IncludeRelatedTests:       true,
		IncludeProjectConventions: true,
		IncludePriorComments:      true,
		IncludePriorDecisions:     true,
		RedactSecrets:             true,
		MaxTokens:                 defaultBudgetMaxTokens,
		MaxItems:                  defaultBudgetMaxItems,
	}
}

func DecodeReviewContextPolicy(raw json.RawMessage) (ReviewContextPolicy, error) {
	return ApplyReviewContextPolicy(DefaultReviewContextPolicy(), raw)
}

func ApplyReviewContextPolicy(base ReviewContextPolicy, raw json.RawMessage) (ReviewContextPolicy, error) {
	if isEmptyPolicy(raw) {
		return base.withCleanLocalOnlyPaths(), nil
	}

	var patch reviewContextPolicyPatch
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&patch); err != nil {
		return ReviewContextPolicy{}, fmt.Errorf("decode review context policy: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return ReviewContextPolicy{}, errors.New("review context policy contains multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return ReviewContextPolicy{}, errors.New("review context policy contains multiple JSON values")
	}
	if patch.IncludePromptMaterial != nil {
		base.IncludePromptMaterial = *patch.IncludePromptMaterial
	}
	if patch.IncludeChangedCode != nil {
		base.IncludeChangedCode = *patch.IncludeChangedCode
	}
	if patch.IncludeRelatedCallSites != nil {
		base.IncludeRelatedCallSites = *patch.IncludeRelatedCallSites
	}
	if patch.IncludeRelatedTests != nil {
		base.IncludeRelatedTests = *patch.IncludeRelatedTests
	}
	if patch.IncludeProjectConventions != nil {
		base.IncludeProjectConventions = *patch.IncludeProjectConventions
	}
	if patch.IncludePriorComments != nil {
		base.IncludePriorComments = *patch.IncludePriorComments
	}
	if patch.IncludePriorDecisions != nil {
		base.IncludePriorDecisions = *patch.IncludePriorDecisions
	}
	if patch.RedactSecrets != nil {
		base.RedactSecrets = *patch.RedactSecrets
	}
	if patch.LocalOnlyPaths != nil {
		base.LocalOnlyPaths = append([]string(nil), patch.LocalOnlyPaths...)
	}
	if patch.MaxTokens != nil {
		if *patch.MaxTokens <= 0 {
			return ReviewContextPolicy{}, errors.New("review context policy max_tokens must be positive")
		}
		base.MaxTokens = *patch.MaxTokens
	}
	if patch.MaxItems != nil {
		if *patch.MaxItems <= 0 {
			return ReviewContextPolicy{}, errors.New("review context policy max_items must be positive")
		}
		base.MaxItems = *patch.MaxItems
	}
	if base.MaxTokens <= 0 {
		base.MaxTokens = defaultBudgetMaxTokens
	}
	if base.MaxItems <= 0 {
		base.MaxItems = defaultBudgetMaxItems
	}
	return base.withCleanLocalOnlyPaths(), nil
}

func (p ReviewContextPolicy) JSON() json.RawMessage {
	data, err := json.Marshal(p.withCleanLocalOnlyPaths())
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}

func (p ReviewContextPolicy) withCleanLocalOnlyPaths() ReviewContextPolicy {
	if len(p.LocalOnlyPaths) == 0 {
		return p
	}
	seen := map[string]struct{}{}
	paths := make([]string, 0, len(p.LocalOnlyPaths))
	for _, path := range p.LocalOnlyPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	p.LocalOnlyPaths = paths
	return p
}

func isEmptyPolicy(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return value == "" || value == "null"
}
