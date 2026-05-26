package contextbundle

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ReviewDepth string

const (
	ReviewDepthQuick    ReviewDepth = "quick"
	ReviewDepthStandard ReviewDepth = "standard"
	ReviewDepthDeep     ReviewDepth = "deep"
)

const (
	defaultBudgetMaxTokens int64 = 18_000
	defaultBudgetMaxItems        = 200
)

type BudgetOptions struct {
	Depth     ReviewDepth
	MaxTokens int64
	MaxItems  int
}

type BudgetResult struct {
	Items         []Item        `json:"items"`
	Dropped       []DroppedItem `json:"dropped"`
	TokenEstimate int64         `json:"token_estimate"`
	ItemCount     int64         `json:"item_count"`
}

type DroppedItem struct {
	ItemID        string   `json:"item_id"`
	Kind          ItemKind `json:"kind"`
	Path          string   `json:"path,omitempty"`
	Title         string   `json:"title,omitempty"`
	TokenEstimate int64    `json:"token_estimate"`
	Reason        string   `json:"reason"`
}

func BudgetContextItems(options BudgetOptions, items []Item) (BudgetResult, error) {
	options = normalizeBudgetOptions(options)
	if !options.Depth.Valid() {
		return BudgetResult{}, fmt.Errorf("review depth %q is invalid", options.Depth)
	}
	if options.MaxTokens <= 0 {
		return BudgetResult{}, errors.New("context token budget must be positive")
	}
	if options.MaxItems <= 0 {
		return BudgetResult{}, errors.New("context item budget must be positive")
	}

	candidates := make([]budgetCandidate, 0, len(items))
	dropped := make([]DroppedItem, 0)
	for index, item := range items {
		item = ApplyItemTokenEstimate(item)
		if err := item.Validate(); err != nil {
			return BudgetResult{}, err
		}
		if !depthAllowsKind(options.Depth, item.Kind) {
			dropped = append(dropped, droppedContextItem(item, "excluded_by_depth"))
			continue
		}
		candidates = append(candidates, budgetCandidate{
			item:     item,
			index:    index,
			priority: budgetKindPriority(item.Kind),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		return candidates[i].index < candidates[j].index
	})

	selectedCandidates := make([]budgetCandidate, 0, min(len(candidates), options.MaxItems))
	var tokenEstimate int64
	for _, candidate := range candidates {
		item := candidate.item
		if len(selectedCandidates) >= options.MaxItems {
			dropped = append(dropped, droppedContextItem(item, "item_limit_exceeded"))
			continue
		}
		if tokenEstimate+item.TokenEstimate > options.MaxTokens {
			dropped = append(dropped, droppedContextItem(item, "token_budget_exceeded"))
			continue
		}
		selectedCandidates = append(selectedCandidates, candidate)
		tokenEstimate += item.TokenEstimate
	}
	sort.SliceStable(selectedCandidates, func(i, j int) bool {
		return selectedCandidates[i].index < selectedCandidates[j].index
	})
	selected := make([]Item, 0, len(selectedCandidates))
	for _, candidate := range selectedCandidates {
		selected = append(selected, candidate.item)
	}
	return BudgetResult{
		Items:         selected,
		Dropped:       dropped,
		TokenEstimate: tokenEstimate,
		ItemCount:     int64(len(selected)),
	}, nil
}

func BudgetBundle(bundle Bundle, options BudgetOptions) (Bundle, []DroppedItem, error) {
	result, err := BudgetContextItems(options, bundle.Items)
	if err != nil {
		return Bundle{}, nil, err
	}
	bundle.Items = result.Items
	bundle.ItemCount = result.ItemCount
	bundle.TokenEstimate = result.TokenEstimate
	if err := bundle.Validate(); err != nil {
		return Bundle{}, nil, err
	}
	return bundle, result.Dropped, nil
}

func (d ReviewDepth) Valid() bool {
	switch d {
	case ReviewDepthQuick, ReviewDepthStandard, ReviewDepthDeep:
		return true
	default:
		return false
	}
}

func normalizeBudgetOptions(options BudgetOptions) BudgetOptions {
	if strings.TrimSpace(string(options.Depth)) == "" {
		options.Depth = ReviewDepthStandard
	}
	if options.MaxTokens <= 0 {
		options.MaxTokens = defaultBudgetMaxTokens
	}
	if options.MaxItems <= 0 {
		options.MaxItems = defaultBudgetMaxItems
	}
	return options
}

func depthAllowsKind(depth ReviewDepth, kind ItemKind) bool {
	switch depth {
	case ReviewDepthQuick:
		return kind == ItemPromptMaterial || kind == ItemFocusFile || kind == ItemChangedHunk || kind == ItemEvidence
	case ReviewDepthStandard:
		switch kind {
		case ItemPromptMaterial, ItemFocusFile, ItemChangedHunk, ItemFullFile, ItemFileSlice, ItemRelatedCode, ItemRelatedTest, ItemEvidence:
			return true
		default:
			return false
		}
	case ReviewDepthDeep:
		return true
	default:
		return false
	}
}

func budgetKindPriority(kind ItemKind) int {
	switch kind {
	case ItemPromptMaterial:
		return -10
	case ItemFocusFile:
		return -8
	case ItemEvidence:
		return -5
	case ItemChangedHunk:
		return 0
	case ItemFullFile, ItemFileSlice:
		return 10
	case ItemRelatedTest:
		return 20
	case ItemRelatedCode:
		return 30
	case ItemProjectRule:
		return 40
	case ItemPriorComment:
		return 50
	case ItemPriorDecision:
		return 60
	case ItemRedactionNote:
		return 80
	default:
		return 100
	}
}

type budgetCandidate struct {
	item     Item
	index    int
	priority int
}

func droppedContextItem(item Item, reason string) DroppedItem {
	return DroppedItem{
		ItemID:        item.ID,
		Kind:          item.Kind,
		Path:          item.Path,
		Title:         item.Title,
		TokenEstimate: item.TokenEstimate,
		Reason:        reason,
	}
}
