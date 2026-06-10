package contextbundle

import (
	"strings"
	"testing"
)

func TestBudgetContextItemsAppliesDepthRules(t *testing.T) {
	t.Parallel()

	items := []Item{
		testBudgetItem("item_hunk", ItemChangedHunk, "src/auth.ts", "changed hunk body"),
		testBudgetItem("item_file", ItemFullFile, "src/auth.ts", "full file body"),
		testBudgetItem("item_test", ItemRelatedTest, "src/auth.test.ts", "related test body"),
		testBudgetItem("item_rule", ItemProjectRule, "CODEOWNERS", "project rule body"),
		testBudgetItem("item_comment", ItemPriorComment, "", "prior comment body"),
	}

	quick, err := BudgetContextItems(BudgetOptions{Depth: ReviewDepthQuick, MaxTokens: 1_000, MaxItems: 10}, items)
	if err != nil {
		t.Fatalf("BudgetContextItems(quick) error = %v", err)
	}
	if len(quick.Items) != 1 || quick.Items[0].ID != "item_hunk" {
		t.Fatalf("quick items = %+v", quick.Items)
	}
	if len(quick.Dropped) != 4 || quick.Dropped[0].Reason != "excluded_by_depth" {
		t.Fatalf("quick dropped = %+v", quick.Dropped)
	}

	standard, err := BudgetContextItems(BudgetOptions{Depth: ReviewDepthStandard, MaxTokens: 1_000, MaxItems: 10}, items)
	if err != nil {
		t.Fatalf("BudgetContextItems(standard) error = %v", err)
	}
	if got := itemIDs(standard.Items); strings.Join(got, ",") != "item_hunk,item_file,item_test,item_rule" {
		t.Fatalf("standard item IDs = %+v", got)
	}
	if len(standard.Dropped) != 1 ||
		standard.Dropped[0].ItemID != "item_comment" {
		t.Fatalf("standard dropped = %+v", standard.Dropped)
	}
}

func TestBudgetContextItemsCapsTokensAndItemsDeterministically(t *testing.T) {
	t.Parallel()

	items := []Item{
		testBudgetItem("item_rule", ItemProjectRule, "CODEOWNERS", "rule body"),
		testBudgetItem("item_hunk", ItemChangedHunk, "src/auth.ts", strings.Repeat("a", 20)),
		testBudgetItem("item_test", ItemRelatedTest, "src/auth.test.ts", strings.Repeat("b", 20)),
		testBudgetItem("item_code", ItemRelatedCode, "src/callsite.ts", strings.Repeat("c", 20)),
	}

	result, err := BudgetContextItems(BudgetOptions{Depth: ReviewDepthDeep, MaxTokens: 10, MaxItems: 2}, items)
	if err != nil {
		t.Fatalf("BudgetContextItems() error = %v", err)
	}
	if got := itemIDs(result.Items); strings.Join(got, ",") != "item_hunk,item_test" {
		t.Fatalf("selected item IDs = %+v", got)
	}
	if result.TokenEstimate != 10 || result.ItemCount != 2 {
		t.Fatalf("result totals = %+v", result)
	}
	if len(result.Dropped) != 2 {
		t.Fatalf("dropped len = %d, want 2: %+v", len(result.Dropped), result.Dropped)
	}
	if result.Dropped[0].ItemID != "item_code" || result.Dropped[0].Reason != "item_limit_exceeded" {
		t.Fatalf("first dropped = %+v", result.Dropped[0])
	}
	if result.Dropped[1].ItemID != "item_rule" || result.Dropped[1].Reason != "item_limit_exceeded" {
		t.Fatalf("second dropped = %+v", result.Dropped[1])
	}
}

func TestBudgetContextItemsRecordsTokenBudgetDrops(t *testing.T) {
	t.Parallel()

	items := []Item{
		testBudgetItem("item_hunk", ItemChangedHunk, "src/auth.ts", strings.Repeat("a", 20)),
		testBudgetItem("item_test", ItemRelatedTest, "src/auth.test.ts", strings.Repeat("b", 20)),
	}

	result, err := BudgetContextItems(BudgetOptions{Depth: ReviewDepthStandard, MaxTokens: 5, MaxItems: 10}, items)
	if err != nil {
		t.Fatalf("BudgetContextItems() error = %v", err)
	}
	if got := itemIDs(result.Items); strings.Join(got, ",") != "item_hunk" {
		t.Fatalf("selected item IDs = %+v", got)
	}
	if len(result.Dropped) != 1 ||
		result.Dropped[0].ItemID != "item_test" ||
		result.Dropped[0].Reason != "token_budget_exceeded" {
		t.Fatalf("dropped = %+v", result.Dropped)
	}
}

func TestBudgetBundleUpdatesTotalsAndRejectsInvalidDepth(t *testing.T) {
	t.Parallel()

	bundle := Bundle{
		ID:              "bundle_1",
		ReviewSessionID: "review_session_1",
		Scope:           ScopeReview,
		Policy:          []byte("{}"),
		Items: []Item{
			testBudgetItem("item_hunk", ItemChangedHunk, "src/auth.ts", "changed hunk body"),
			testBudgetItem("item_rule", ItemProjectRule, "CODEOWNERS", "project rule body"),
		},
	}

	budgeted, dropped, err := BudgetBundle(bundle, BudgetOptions{Depth: ReviewDepthQuick, MaxTokens: 1_000, MaxItems: 10})
	if err != nil {
		t.Fatalf("BudgetBundle() error = %v", err)
	}
	if budgeted.ItemCount != 1 ||
		budgeted.TokenEstimate != budgeted.Items[0].TokenEstimate ||
		len(dropped) != 1 ||
		dropped[0].ItemID != "item_rule" {
		t.Fatalf("budgeted = %+v dropped = %+v", budgeted, dropped)
	}
	if _, _, err := BudgetBundle(bundle, BudgetOptions{Depth: "wide", MaxTokens: 100, MaxItems: 10}); err == nil || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("BudgetBundle(invalid depth) error = %v", err)
	}
}

func testBudgetItem(id string, kind ItemKind, path string, content string) Item {
	return Item{
		ID:              id,
		ContextBundleID: "bundle_1",
		Kind:            kind,
		Path:            path,
		StartLine:       1,
		EndLine:         1,
		Title:           id,
		Content:         content,
		Metadata:        []byte("{}"),
	}
}

func itemIDs(items []Item) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}
