package contextbundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const (
	defaultPriorDecisionMaxBytes int64 = 4 * 1024
	defaultPriorDecisionMaxItems       = 40
)

type PriorDecisionOptions struct {
	BundleID         string
	WorkspaceID      string
	MaxDecisionBytes int64
	MaxItems         int
}

func BuildPriorDecisionContextItems(options PriorDecisionOptions, rules []dbgen.ReviewRule) ([]Item, error) {
	options = normalizePriorDecisionOptions(options)
	if strings.TrimSpace(options.BundleID) == "" {
		return nil, errors.New("context bundle id is required")
	}

	ordered := make([]dbgen.ReviewRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Enabled == 0 || strings.TrimSpace(rule.Content) == "" {
			continue
		}
		if strings.TrimSpace(options.WorkspaceID) != "" && rule.WorkspaceID != options.WorkspaceID {
			continue
		}
		ordered = append(ordered, rule)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		leftRank := priorDecisionRuleTypePriority(ordered[i].RuleType)
		rightRank := priorDecisionRuleTypePriority(ordered[j].RuleType)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if ordered[i].Scope != ordered[j].Scope {
			return ordered[i].Scope < ordered[j].Scope
		}
		if ordered[i].UpdatedAt != ordered[j].UpdatedAt {
			return ordered[i].UpdatedAt > ordered[j].UpdatedAt
		}
		return ordered[i].ID < ordered[j].ID
	})

	items := make([]Item, 0, min(len(ordered), options.MaxItems))
	for _, rule := range ordered {
		if len(items) >= options.MaxItems {
			break
		}
		item, ok, err := priorDecisionContextItem(options, rule)
		if err != nil {
			return nil, err
		}
		if ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func normalizePriorDecisionOptions(options PriorDecisionOptions) PriorDecisionOptions {
	if options.MaxDecisionBytes <= 0 {
		options.MaxDecisionBytes = defaultPriorDecisionMaxBytes
	}
	if options.MaxItems <= 0 {
		options.MaxItems = defaultPriorDecisionMaxItems
	}
	return options
}

func priorDecisionContextItem(options PriorDecisionOptions, rule dbgen.ReviewRule) (Item, bool, error) {
	content, truncated := boundedPriorDecisionContent(renderPriorDecision(rule), options.MaxDecisionBytes)
	if strings.TrimSpace(content) == "" {
		return Item{}, false, nil
	}
	lineCount := countContentLines(content)
	metadata, err := priorDecisionMetadata(rule, truncated, int64(len(content)))
	if err != nil {
		return Item{}, false, err
	}
	item := Item{
		ID:              stableContextItemID(options.BundleID, "prior_decision\x00"+rule.ID, 0),
		ContextBundleID: options.BundleID,
		Kind:            ItemPriorDecision,
		Title:           priorDecisionTitle(rule),
		Content:         content,
		StartLine:       1,
		EndLine:         lineCount,
		TokenEstimate:   estimateTokens(content),
		Metadata:        metadata,
	}
	if err := item.Validate(); err != nil {
		return Item{}, false, err
	}
	return item, true, nil
}

func renderPriorDecision(rule dbgen.ReviewRule) string {
	var builder strings.Builder
	builder.WriteString("Guidance:\n")
	builder.WriteString(strings.TrimSpace(rule.Content))
	builder.WriteString("\n\n")
	builder.WriteString("Rule type: ")
	builder.WriteString(strings.TrimSpace(rule.RuleType))
	builder.WriteByte('\n')
	builder.WriteString("Scope: ")
	builder.WriteString(strings.TrimSpace(rule.Scope))
	builder.WriteByte('\n')
	if strings.TrimSpace(rule.UpdatedAt) != "" {
		builder.WriteString("Updated at: ")
		builder.WriteString(strings.TrimSpace(rule.UpdatedAt))
		builder.WriteByte('\n')
	}
	return builder.String()
}

func boundedPriorDecisionContent(content string, maxBytes int64) (string, bool) {
	if maxBytes <= 0 {
		return "", true
	}
	if int64(len(content)) <= maxBytes {
		return content, false
	}
	cut := []byte(content)
	cut = cut[:maxBytes]
	for len(cut) > 0 && !utf8.Valid(cut) {
		cut = cut[:len(cut)-1]
	}
	return string(cut), true
}

func priorDecisionTitle(rule dbgen.ReviewRule) string {
	ruleType := strings.TrimSpace(rule.RuleType)
	if ruleType == "" {
		ruleType = "review"
	}
	scope := strings.TrimSpace(rule.Scope)
	if scope == "" {
		return fmt.Sprintf("Prior %s rule", ruleType)
	}
	return fmt.Sprintf("Prior %s rule (%s)", ruleType, scope)
}

func priorDecisionRuleTypePriority(ruleType string) int {
	switch strings.ToLower(strings.TrimSpace(ruleType)) {
	case "dismissal", "false_positive":
		return 0
	case "accepted", "preference":
		return 10
	default:
		return 100
	}
}

func priorDecisionMetadata(rule dbgen.ReviewRule, truncated bool, contentBytes int64) (json.RawMessage, error) {
	payload := map[string]any{
		"source":        "review_rule",
		"rule_id":       rule.ID,
		"workspace_id":  rule.WorkspaceID,
		"scope":         rule.Scope,
		"rule_type":     rule.RuleType,
		"enabled":       rule.Enabled != 0,
		"created_at":    rule.CreatedAt,
		"updated_at":    rule.UpdatedAt,
		"truncated":     truncated,
		"content_bytes": contentBytes,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode prior decision metadata: %w", err)
	}
	return data, nil
}
