package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestReviewRuleQueriesLifecycle(t *testing.T) {
	t.Parallel()

	queries := seededReviewQueries(t)
	first, err := queries.CreateReviewRule(context.Background(), dbgen.CreateReviewRuleParams{
		ID:          "review_rule_1",
		WorkspaceID: "workspace_1",
		Scope:       "workspace",
		RuleType:    "dismissal",
		Content:     "Do not report generated file formatting noise.",
		Enabled:     1,
		CreatedAt:   "2026-05-03T00:01:00Z",
		UpdatedAt:   "2026-05-03T00:01:00Z",
	})
	if err != nil {
		t.Fatalf("CreateReviewRule() error = %v", err)
	}
	if first.Scope != "workspace" || first.Enabled != 1 {
		t.Fatalf("CreateReviewRule() = %+v", first)
	}

	second, err := queries.CreateReviewRule(context.Background(), dbgen.CreateReviewRuleParams{
		ID:          "review_rule_2",
		WorkspaceID: "workspace_1",
		Scope:       "repository",
		RuleType:    "false_positive",
		Content:     "Theme persistence is handled by the provider.",
		Enabled:     0,
		CreatedAt:   "2026-05-03T00:02:00Z",
		UpdatedAt:   "2026-05-03T00:02:00Z",
	})
	if err != nil {
		t.Fatalf("CreateReviewRule(disabled) error = %v", err)
	}

	got, err := queries.GetReviewRule(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("GetReviewRule() error = %v", err)
	}
	if got.ID != first.ID {
		t.Fatalf("GetReviewRule() ID = %q, want %q", got.ID, first.ID)
	}

	rules, err := queries.ListReviewRulesByWorkspace(context.Background(), "workspace_1")
	if err != nil {
		t.Fatalf("ListReviewRulesByWorkspace() error = %v", err)
	}
	if len(rules) != 2 || rules[0].ID != first.ID || rules[1].ID != second.ID {
		t.Fatalf("ListReviewRulesByWorkspace() = %+v", rules)
	}

	enabled, err := queries.ListEnabledReviewRulesByWorkspace(context.Background(), "workspace_1")
	if err != nil {
		t.Fatalf("ListEnabledReviewRulesByWorkspace() error = %v", err)
	}
	if len(enabled) != 1 || enabled[0].ID != first.ID {
		t.Fatalf("ListEnabledReviewRulesByWorkspace() = %+v", enabled)
	}

	updated, err := queries.UpdateReviewRule(context.Background(), dbgen.UpdateReviewRuleParams{
		ID:        second.ID,
		Scope:     "repository",
		RuleType:  "dismissal",
		Content:   "Theme persistence is provider-owned; do not flag App.tsx.",
		Enabled:   1,
		UpdatedAt: "2026-05-03T00:03:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateReviewRule() error = %v", err)
	}
	if updated.Enabled != 1 || updated.RuleType != "dismissal" {
		t.Fatalf("UpdateReviewRule() = %+v", updated)
	}

	disabled, err := queries.SetReviewRuleEnabled(context.Background(), dbgen.SetReviewRuleEnabledParams{
		ID:        first.ID,
		Enabled:   0,
		UpdatedAt: "2026-05-03T00:04:00Z",
	})
	if err != nil {
		t.Fatalf("SetReviewRuleEnabled() error = %v", err)
	}
	if disabled.Enabled != 0 {
		t.Fatalf("SetReviewRuleEnabled() = %+v", disabled)
	}

	if err := queries.DeleteReviewRule(context.Background(), first.ID); err != nil {
		t.Fatalf("DeleteReviewRule() error = %v", err)
	}
	if _, err := queries.GetReviewRule(context.Background(), first.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetReviewRule(deleted) error = %v, want sql.ErrNoRows", err)
	}
}
