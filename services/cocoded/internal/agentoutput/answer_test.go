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

func TestExtractAnswerPrefersStructuredFindingsOverProgressText(t *testing.T) {
	t.Parallel()

	findingsPayload, err := json.Marshal(`{"findings":[{"claim":"reward matching can collide without wallet key","severity":"high","category":"correctness","locations":[{"path":"internal/rewards.go","start_line":42,"end_line":45,"side":"RIGHT"}],"evidence":[{"kind":"changed_code","title":"Changed reward lookup","summary":"The new lookup only keys by chain ID and token ID."}],"suggested_fix":"Include wallet address in the reward matching key."}]}`)
	if err != nil {
		t.Fatalf("Marshal findings payload error = %v", err)
	}
	parsed := ParseAuto([]byte(strings.Join([]string{
		`{"type":"item.completed","item":{"type":"agent_message","text":"The main behavior question I'm checking is whether reward matching has enough keys."}}`,
		`{"type":"text","part":{"type":"text","text":` + string(findingsPayload) + `}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"I am double-checking the position model."}}`,
		"",
	}, "\n")))

	answer := ExtractAnswer(parsed)
	for _, want := range []string{
		"Findings (1)",
		"reward matching can collide without wallet key",
		"internal/rewards.go:42-45",
	} {
		if !strings.Contains(answer.Content, want) {
			t.Fatalf("Content = %q, missing %q", answer.Content, want)
		}
	}
	if strings.Contains(answer.Content, "main behavior question") {
		t.Fatalf("Content = %q, still preferred progress text", answer.Content)
	}
}

func TestExtractAnswerPrefersStructuredFindingsOverNonPrefixProgressText(t *testing.T) {
	t.Parallel()

	findingsPayload, err := json.Marshal(`{"findings":[{"claim":"pickTokenPrice dereferences prices[1] without checking it","severity":"high","category":"correctness","locations":[{"path":"internal/kem_rewards.go","start_line":208,"end_line":208,"side":"RIGHT"}]}]}`)
	if err != nil {
		t.Fatalf("Marshal findings payload error = %v", err)
	}
	parsed := ParseAuto([]byte(strings.Join([]string{
		`{"type":"item.completed","item":{"type":"agent_message","text":"The risky line is real in the checkout; I am checking the producer path now."}}`,
		`{"type":"text","part":{"type":"text","text":` + string(findingsPayload) + `}}`,
		"",
	}, "\n")))

	answer := ExtractAnswer(parsed)
	if !strings.Contains(answer.Content, "Findings (1)") ||
		!strings.Contains(answer.Content, "pickTokenPrice dereferences prices[1]") ||
		strings.Contains(answer.Content, "risky line is real") {
		t.Fatalf("Content = %q", answer.Content)
	}
}

func TestExtractAnswerKeepsExplicitSummaryOverStructuredFindings(t *testing.T) {
	t.Parallel()

	parsed := ParseAuto([]byte(`{
		"summary": "Found one deterministic fixture issue.",
		"findings": [
			{
				"claim": "Repository settings updates can run without an admin permission check.",
				"severity": "high",
				"category": "security",
				"primary_path": "apps/api/src/routes/repositories.ts",
				"primary_start_line": 87,
				"evidence": [
					{"summary": "The route updates repository settings without requiring workspace admin privileges."}
				]
			}
		]
	}`))

	answer := ExtractAnswer(parsed)
	if answer.Content != "Found one deterministic fixture issue." {
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

func TestExtractAnswerFormatsStructuredFindingsFromProviderStreams(t *testing.T) {
	t.Parallel()

	codexPayload, err := json.Marshal(`{"findings":[{"claim":"cancelSubscription lacks admin authorization","severity":"high","category":"security","locations":[{"path":"src/server.js","start_line":10,"end_line":11,"side":"RIGHT"}],"evidence":[{"kind":"code","title":"Changed route","summary":"The route calls db.cancelSubscription without requireAdmin.","path":"src/server.js","start_line":10,"end_line":11}]}]}`)
	if err != nil {
		t.Fatalf("Marshal codex payload error = %v", err)
	}
	geminiPayload, err := json.Marshal("```json\n" + `{"findings":[{"claim":"exportInvoices lacks admin authorization","severity":"critical","category":"security","locations":[{"path":"src/server.js","start_line":17,"end_line":18,"side":"RIGHT"}],"evidence":[{"kind":"diff","title":"Changed export route","summary":"The route calls db.exportInvoices without requireAdmin.","path":"src/server.js","start_line":17,"end_line":18}]}]}` + "\n```")
	if err != nil {
		t.Fatalf("Marshal gemini payload error = %v", err)
	}
	claudePayload, err := json.Marshal("```json\n" + `{"findings":[{"claim":"deleteAccount lacks owner validation","severity":"high","category":"security","locations":[{"path":"src/server.js","start_line":25,"end_line":26,"side":"RIGHT"}],"evidence":[{"kind":"diff_hunk","title":"Changed delete route","summary":"The route deletes an account without checking ownership.","path":"src/server.js","start_line":25,"end_line":26}]}]}` + "\n```")
	if err != nil {
		t.Fatalf("Marshal claude payload error = %v", err)
	}

	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{
			name: "codex agent message text",
			raw:  []byte(`{"type":"item.completed","item":{"type":"agent_message","text":` + string(codexPayload) + `}}`),
			want: "cancelSubscription lacks admin authorization",
		},
		{
			name: "gemini response fenced json",
			raw:  []byte(`{"response":` + string(geminiPayload) + `}`),
			want: "exportInvoices lacks admin authorization",
		},
		{
			name: "claude stream message content",
			raw: []byte(strings.Join([]string{
				`{"type":"system","subtype":"init"}`,
				`{"type":"assistant","message":{"type":"message","role":"assistant","content":[{"type":"text","text":` + string(claudePayload) + `}]}}`,
				"",
			}, "\n")),
			want: "deleteAccount lacks owner validation",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			answer := ExtractAnswer(ParseAuto(test.raw))
			if !strings.Contains(answer.Content, "Findings (1)") || !strings.Contains(answer.Content, test.want) {
				t.Fatalf("Content = %q", answer.Content)
			}
		})
	}
}
