package contextbundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hughdo/cocode/services/cocoded/internal/projectrules"
)

const (
	defaultProjectRuleMaxContentBytes int64 = 16 * 1024
	defaultProjectRuleMaxItems              = 24
)

type ProjectRuleOptions struct {
	BundleID        string
	RepoRoot        string
	MaxFileBytes    int64
	MaxContentBytes int64
	MaxItems        int
}

func BuildProjectRuleContextItems(options ProjectRuleOptions) ([]Item, error) {
	options = normalizeProjectRuleOptions(options)
	if strings.TrimSpace(options.BundleID) == "" {
		return nil, errors.New("context bundle id is required")
	}
	root, err := safeRepoRoot(options.RepoRoot)
	if err != nil {
		return nil, err
	}

	candidates, err := projectrules.DiscoverWithOptions(root, projectrules.Options{MaxFileBytes: options.MaxFileBytes})
	if err != nil {
		return nil, err
	}
	prioritizeProjectRuleCandidates(candidates)

	items := make([]Item, 0, min(len(candidates), options.MaxItems))
	for _, candidate := range candidates {
		if len(items) >= options.MaxItems {
			break
		}
		item, ok, err := projectRuleContextItem(options, candidate)
		if err != nil {
			return nil, err
		}
		if ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func normalizeProjectRuleOptions(options ProjectRuleOptions) ProjectRuleOptions {
	if options.MaxFileBytes <= 0 {
		options.MaxFileBytes = projectrules.DefaultMaxFileBytes
	}
	if options.MaxContentBytes <= 0 {
		options.MaxContentBytes = defaultProjectRuleMaxContentBytes
	}
	if options.MaxItems <= 0 {
		options.MaxItems = defaultProjectRuleMaxItems
	}
	return options
}

func prioritizeProjectRuleCandidates(candidates []projectrules.Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		leftRank := projectRulePriority(candidates[i])
		rightRank := projectRulePriority(candidates[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if candidates[i].Kind != candidates[j].Kind {
			return candidates[i].Kind < candidates[j].Kind
		}
		return candidates[i].Path < candidates[j].Path
	})
}

func projectRulePriority(candidate projectrules.Candidate) int {
	switch candidate.Kind {
	case "codeowners":
		return 0
	case "readme":
		return 10
	case "package_manifest", "go_module", "workspace_config", "python_config", "rust_config":
		return 20
	case "lint_config", "typescript_config", "build_config", "style_config", "ui_config", "editor_config":
		return 30
	default:
		return 100
	}
}

func projectRuleContextItem(options ProjectRuleOptions, candidate projectrules.Candidate) (Item, bool, error) {
	path, ok := cleanSearchMatchPath(candidate.Path)
	if !ok {
		return Item{}, false, nil
	}
	content, truncated := boundedProjectRuleContent(candidate.Content, options.MaxContentBytes)
	if strings.TrimSpace(content) == "" {
		return Item{}, false, nil
	}
	lineCount := countContentLines(content)
	metadata, err := projectRuleMetadata(candidate, truncated, int64(len(content)))
	if err != nil {
		return Item{}, false, err
	}
	item := Item{
		ID:              stableFileContextItemID(options.BundleID, path, ItemProjectRule, 1, lineCount),
		ContextBundleID: options.BundleID,
		Kind:            ItemProjectRule,
		Path:            path,
		StartLine:       1,
		EndLine:         lineCount,
		Title:           candidate.Title,
		Content:         content,
		TokenEstimate:   estimateTokens(content),
		Metadata:        metadata,
	}
	if err := item.Validate(); err != nil {
		return Item{}, false, err
	}
	return item, true, nil
}

func boundedProjectRuleContent(content []byte, maxBytes int64) (string, bool) {
	if maxBytes <= 0 {
		return "", true
	}
	if int64(len(content)) <= maxBytes {
		return string(content), false
	}
	cut := content[:maxBytes]
	for len(cut) > 0 && !utf8.Valid(cut) {
		cut = cut[:len(cut)-1]
	}
	return string(cut), true
}

func projectRuleMetadata(candidate projectrules.Candidate, truncated bool, contentBytes int64) (json.RawMessage, error) {
	payload := make(map[string]any, len(candidate.Metadata)+5)
	for key, value := range candidate.Metadata {
		payload[key] = value
	}
	payload["context_source"] = "project_rule_context"
	payload["rule_kind"] = candidate.Kind
	payload["size_bytes"] = candidate.SizeBytes
	payload["content_bytes"] = contentBytes
	payload["truncated"] = truncated
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode project rule metadata: %w", err)
	}
	return data, nil
}
