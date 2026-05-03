package contextbundle

import "testing"

func TestEstimateContentTokensKnownSamples(t *testing.T) {
	t.Parallel()

	tests := map[string]int64{
		"":          0,
		"a":         1,
		"abcd":      1,
		"abcde":     2,
		"abcdefgh":  2,
		"abcdefghi": 3,
	}
	for content, want := range tests {
		if got := EstimateContentTokens(content); got != want {
			t.Fatalf("EstimateContentTokens(%q) = %d, want %d", content, got, want)
		}
	}
}

func TestEstimateBundleTokensAndApplyEstimates(t *testing.T) {
	t.Parallel()

	bundle := Bundle{
		ID:              "bundle_1",
		ReviewSessionID: "review_session_1",
		Scope:           ScopeReview,
		Policy:          []byte("{}"),
		Items: []Item{
			{
				ID:              "item_1",
				ContextBundleID: "bundle_1",
				Kind:            ItemChangedHunk,
				Content:         "abcde",
				Metadata:        []byte("{}"),
			},
			{
				ID:              "item_2",
				ContextBundleID: "bundle_1",
				Kind:            ItemProjectRule,
				Title:           "fallback title",
				Metadata:        []byte("{}"),
			},
		},
	}

	withEstimates := ApplyBundleTokenEstimates(bundle)
	if withEstimates.ItemCount != 2 {
		t.Fatalf("ItemCount = %d, want 2", withEstimates.ItemCount)
	}
	if withEstimates.Items[0].TokenEstimate != 2 ||
		withEstimates.Items[1].TokenEstimate != EstimateContentTokens("fallback title") {
		t.Fatalf("items = %+v", withEstimates.Items)
	}
	if withEstimates.TokenEstimate != EstimateBundleTokens(withEstimates.Items) {
		t.Fatalf("TokenEstimate = %d, want %d", withEstimates.TokenEstimate, EstimateBundleTokens(withEstimates.Items))
	}
	if err := withEstimates.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
