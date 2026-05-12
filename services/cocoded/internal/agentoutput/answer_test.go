package agentoutput

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractAnswerHandlesCLIResultWrapper(t *testing.T) {
	t.Parallel()

	parsed := ParseAuto([]byte(`{
		"type": "result",
		"subtype": "success",
		"result": "## Findings\n\n1. Missing auth guard\n2. Test accepts insecure behavior",
		"usage": {"input_tokens": 100, "output_tokens": 20}
	}`))

	answer := ExtractAnswer(parsed)
	if answer.Content != "## Findings\n\n1. Missing auth guard\n2. Test accepts insecure behavior" {
		t.Fatalf("Content = %q", answer.Content)
	}
	if string(answer.EvidenceRefs) != "[]" {
		t.Fatalf("EvidenceRefs = %s, want []", answer.EvidenceRefs)
	}
}

func TestExtractAnswerHandlesNestedResultObject(t *testing.T) {
	t.Parallel()

	refs := `[{"path":"src/server.js","start_line":10}]`
	parsed := ParseAuto([]byte(`{
		"result": {
			"answer": "The missing admin check is still reachable.",
			"evidence_refs": ` + refs + `
		}
	}`))

	answer := ExtractAnswer(parsed)
	if answer.Content != "The missing admin check is still reachable." {
		t.Fatalf("Content = %q", answer.Content)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(answer.EvidenceRefs, &decoded); err != nil {
		t.Fatalf("Unmarshal(EvidenceRefs) error = %v", err)
	}
	if len(decoded) != 1 || decoded[0]["path"] != "src/server.js" {
		t.Fatalf("EvidenceRefs = %+v", decoded)
	}
}

func TestExtractAnswerHandlesFencedJSONResultString(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(map[string]any{
		"type":    "result",
		"subtype": "success",
		"result":  "```json\n{\"answer\":\"The finding is supported by src/auth.ts:2.\",\"evidence_refs\":[{\"path\":\"src/auth.ts\",\"start_line\":2}]}\n```",
	})
	if err != nil {
		t.Fatalf("Marshal payload error = %v", err)
	}
	parsed := ParseAuto(payload)

	answer := ExtractAnswer(parsed)
	if answer.Content != "The finding is supported by src/auth.ts:2." {
		t.Fatalf("Content = %q", answer.Content)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(answer.EvidenceRefs, &decoded); err != nil {
		t.Fatalf("Unmarshal(EvidenceRefs) error = %v", err)
	}
	if len(decoded) != 1 || decoded[0]["path"] != "src/auth.ts" {
		t.Fatalf("EvidenceRefs = %+v", decoded)
	}
}

func TestExtractAnswerPrefersLastStructuredAnswer(t *testing.T) {
	t.Parallel()

	parsed := ParseAuto([]byte(strings.Join([]string{
		`{"message":"starting"}`,
		`{"answer":"final answer"}`,
		"",
	}, "\n")))

	answer := ExtractAnswer(parsed)
	if answer.Content != "final answer" {
		t.Fatalf("Content = %q", answer.Content)
	}
}

func TestExtractAnswerHandlesNestedCLIEventWrappers(t *testing.T) {
	t.Parallel()

	parsed := ParseAuto([]byte(strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread_1"}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"I am reading the diff."}}`,
		`{"type":"text","part":{"type":"text","text":"## Findings\n\n- Missing admin guard"}}`,
		"",
	}, "\n")))

	answer := ExtractAnswer(parsed)
	if answer.Content != "## Findings\n\n- Missing admin guard" {
		t.Fatalf("Content = %q", answer.Content)
	}
}

func TestExtractAnswerDoesNotExposeUnrecognizedStructuredJSON(t *testing.T) {
	t.Parallel()

	parsed := ParseAuto([]byte(strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread_1"}`,
		`{"type":"turn.started"}`,
		"",
	}, "\n")))

	answer := ExtractAnswer(parsed)
	if answer.Content != "" {
		t.Fatalf("Content = %q, want empty answer instead of raw JSON", answer.Content)
	}
}

func TestExtractAnswerCapturesReasoningBlocks(t *testing.T) {
	t.Parallel()

	parsed := ParseAuto([]byte(strings.Join([]string{
		`{"type":"reasoning","part":{"type":"reasoning","text":"I compared the changed route with its tests."}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"The guard was removed on the changed path."}}}`,
		`{"part":{"thought":true,"text":"Gemini thought summary: focus on auth checks."}}`,
		`{"type":"text","part":{"type":"text","text":"The route is missing an admin check."}}`,
		"",
	}, "\n")))

	answer := ExtractAnswer(parsed)
	if answer.Content != "The route is missing an admin check." {
		t.Fatalf("Content = %q", answer.Content)
	}
	for _, want := range []string{
		"I compared the changed route with its tests.",
		"The guard was removed on the changed path.",
		"Gemini thought summary: focus on auth checks.",
	} {
		if !strings.Contains(answer.ReasoningSummary, want) {
			t.Fatalf("ReasoningSummary = %q, missing %q", answer.ReasoningSummary, want)
		}
	}
}

func TestExtractAnswerFormatsStructuredFindings(t *testing.T) {
	t.Parallel()

	parsed := ParseAuto([]byte(`{
		"findings": [
			{
				"claim": "Missing admin guard in cancelSubscription",
				"severity": "critical",
				"category": "security",
				"primary_path": "src/server.js",
				"primary_start_line": 10,
				"primary_end_line": 11,
				"evidence": [
					{"title":"Changed code","summary":"The route no longer calls requireAdmin(request.user)."}
				],
				"suggested_fix": "Restore the authorization check before calling the database."
			}
		]
	}`))

	answer := ExtractAnswer(parsed)
	if !strings.Contains(answer.Content, "Findings (1)") {
		t.Fatalf("Content = %q", answer.Content)
	}
	for _, want := range []string{
		"Missing admin guard in cancelSubscription",
		"- **Severity:** blocker",
		"- **Category:** security",
		"- **Location:** `src/server.js:10-11`",
		"- **Evidence:** The route no longer calls requireAdmin(request.user).",
		"- **Suggested fix:** Restore the authorization check before calling the database.",
	} {
		if !strings.Contains(answer.Content, want) {
			t.Fatalf("Content = %q, missing %q", answer.Content, want)
		}
	}
}
