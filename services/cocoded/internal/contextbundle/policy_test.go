package contextbundle

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReviewContextPolicyDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	policy, err := DecodeReviewContextPolicy(json.RawMessage(`{
		"include_related_call_sites": false,
		"local_only_paths": [" config/local.yaml ", "config/local.yaml", ""],
		"max_tokens": 2048,
		"max_items": 17
	}`))
	if err != nil {
		t.Fatalf("DecodeReviewContextPolicy() error = %v", err)
	}
	if !policy.IncludeChangedCode ||
		policy.IncludeRelatedCallSites ||
		!policy.IncludeRelatedTests ||
		!policy.RedactSecrets ||
		policy.MaxTokens != 2048 ||
		policy.MaxItems != 17 {
		t.Fatalf("policy = %+v", policy)
	}
	if len(policy.LocalOnlyPaths) != 1 || policy.LocalOnlyPaths[0] != "config/local.yaml" {
		t.Fatalf("LocalOnlyPaths = %+v", policy.LocalOnlyPaths)
	}

	override, err := ApplyReviewContextPolicy(policy, json.RawMessage(`{"include_related_tests":false}`))
	if err != nil {
		t.Fatalf("ApplyReviewContextPolicy() error = %v", err)
	}
	if override.IncludeRelatedTests || override.IncludeChangedCode != policy.IncludeChangedCode {
		t.Fatalf("override = %+v", override)
	}
}

func TestReviewContextPolicyRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"unknown":true}`,
		`{"max_tokens":0}`,
		`{"local_only_paths":["../secrets.env"]}`,
		`{"include_changed_code":true} {"include_related_tests":false}`,
	} {
		if _, err := DecodeReviewContextPolicy(json.RawMessage(raw)); err == nil || !strings.Contains(err.Error(), "policy") && !strings.Contains(err.Error(), "max_") && !strings.Contains(err.Error(), "multiple") {
			t.Fatalf("DecodeReviewContextPolicy(%s) error = %v", raw, err)
		}
	}
}
