package contextbundle

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestBuildPriorDecisionContextItemsFiltersAndOrdersRules(t *testing.T) {
	t.Parallel()

	rules := []dbgen.ReviewRule{
		{
			ID:          "rule_preference",
			WorkspaceID: "workspace_1",
			Scope:       "workspace",
			RuleType:    "preference",
			Content:     "Prefer direct tests over broad snapshots.",
			Enabled:     1,
			UpdatedAt:   "2026-01-02T10:00:00Z",
			CreatedAt:   "2026-01-01T10:00:00Z",
		},
		{
			ID:          "rule_disabled",
			WorkspaceID: "workspace_1",
			Scope:       "workspace",
			RuleType:    "dismissal",
			Content:     "Disabled rule should not appear.",
			Enabled:     0,
			UpdatedAt:   "2026-01-05T10:00:00Z",
		},
		{
			ID:          "rule_other_workspace",
			WorkspaceID: "workspace_2",
			Scope:       "workspace",
			RuleType:    "dismissal",
			Content:     "Other workspace should not appear.",
			Enabled:     1,
			UpdatedAt:   "2026-01-05T10:00:00Z",
		},
		{
			ID:          "rule_dismissal",
			WorkspaceID: "workspace_1",
			Scope:       "repository",
			RuleType:    "dismissal",
			Content:     "Theme persistence is handled by the provider; do not flag App.tsx.",
			Enabled:     1,
			UpdatedAt:   "2026-01-03T10:00:00Z",
			CreatedAt:   "2026-01-01T10:00:00Z",
		},
	}

	items, err := BuildPriorDecisionContextItems(PriorDecisionOptions{
		BundleID:         "bundle_1",
		WorkspaceID:      "workspace_1",
		MaxItems:         10,
		MaxDecisionBytes: 4096,
	}, rules)
	if err != nil {
		t.Fatalf("BuildPriorDecisionContextItems() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2: %+v", len(items), items)
	}
	if !strings.Contains(items[0].Content, "Theme persistence") ||
		!strings.Contains(items[1].Content, "Prefer direct tests") {
		t.Fatalf("items = %+v", items)
	}
	assertPriorDecisionMetadata(t, items[0].Metadata, "rule_dismissal", "dismissal", false)
	assertPriorDecisionMetadata(t, items[1].Metadata, "rule_preference", "preference", false)
}

func TestBuildPriorDecisionContextItemsLimitsAndTruncates(t *testing.T) {
	t.Parallel()

	items, err := BuildPriorDecisionContextItems(PriorDecisionOptions{
		BundleID:         "bundle_1",
		WorkspaceID:      "workspace_1",
		MaxItems:         1,
		MaxDecisionBytes: 80,
	}, []dbgen.ReviewRule{
		{
			ID:          "rule_old",
			WorkspaceID: "workspace_1",
			Scope:       "workspace",
			RuleType:    "dismissal",
			Content:     "old",
			Enabled:     1,
			UpdatedAt:   "2026-01-01T10:00:00Z",
		},
		{
			ID:          "rule_new",
			WorkspaceID: "workspace_1",
			Scope:       "workspace",
			RuleType:    "dismissal",
			Content:     strings.Repeat("new dismissal guidance ", 20),
			Enabled:     1,
			UpdatedAt:   "2026-01-02T10:00:00Z",
		},
	})
	if err != nil {
		t.Fatalf("BuildPriorDecisionContextItems() error = %v", err)
	}
	if len(items) != 1 || !strings.Contains(items[0].Content, "new dismissal") {
		t.Fatalf("items = %+v", items)
	}
	if len(items[0].Content) > 80 {
		t.Fatalf("content len = %d, want <= 80", len(items[0].Content))
	}
	assertPriorDecisionMetadata(t, items[0].Metadata, "rule_new", "dismissal", true)
}

func TestBuildPriorDecisionContextItemsValidatesBundle(t *testing.T) {
	t.Parallel()

	_, err := BuildPriorDecisionContextItems(PriorDecisionOptions{}, []dbgen.ReviewRule{
		{ID: "rule_1", WorkspaceID: "workspace_1", Content: "body", Enabled: 1},
	})
	if err == nil || !strings.Contains(err.Error(), "bundle") {
		t.Fatalf("BuildPriorDecisionContextItems() error = %v, want bundle error", err)
	}
}

func assertPriorDecisionMetadata(t *testing.T, raw json.RawMessage, ruleID string, ruleType string, truncated bool) {
	t.Helper()

	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatalf("Unmarshal(metadata) error = %v", err)
	}
	if metadata["source"] != "review_rule" ||
		metadata["rule_id"] != ruleID ||
		metadata["rule_type"] != ruleType ||
		metadata["truncated"] != truncated {
		t.Fatalf("metadata = %+v", metadata)
	}
}
