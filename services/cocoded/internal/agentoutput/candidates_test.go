package agentoutput

import (
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
)

func TestExtractCandidatesFromReviewAgentOutput(t *testing.T) {
	t.Parallel()

	output := runFakeAgent(t, "json-agent.sh", "review this fixture")
	parsed := ParseAuto(output)
	result := ExtractCandidates(parsed)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("Diagnostics = %+v", result.Diagnostics)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("Candidates = %+v", result.Candidates)
	}
	candidate := result.Candidates[0]
	if candidate.SchemaVersion != CandidateSchemaVersion ||
		candidate.Category != "security" ||
		candidate.Severity != "high" ||
		candidate.Confidence != 0.91 ||
		candidate.PrimaryPath != "apps/api/src/routes/repositories.ts" ||
		candidate.PrimaryStartLine != 87 ||
		candidate.PrimaryEndLine != 112 ||
		candidate.Evidence[0].Kind != "unknown" ||
		candidate.SuggestedFix == "" ||
		candidate.DraftComment == "" {
		t.Fatalf("candidate = %+v", candidate)
	}
}

func TestExtractCandidatesFromDelimitedJSONOutput(t *testing.T) {
	t.Parallel()

	raw := []byte(`starting review
{"event":"finding","finding":{"claim":"request body is trusted before validation","category":"correctness","severity":"medium","confidence":0.7,"locations":[{"path":"apps/api/src/routes/repositories.ts","start_line":21,"end_line":29,"side":"RIGHT"}],"evidence":[{"title":"handler reads unvalidated body","summary":"the finding event cites the changed handler"}]}}
{"type":"finding","candidate":{"schema_version":"finding-candidate/v1","claim":"workspace role cache can be stale","category":"reliability","severity":"low","confidence":0.55,"locations":[{"path":"apps/api/src/auth/roles.ts","start_line":44,"end_line":48,"side":"RIGHT"}],"evidence":[{"title":"cached role has no expiry","summary":"the candidate already contains normalized evidence","kind":"related_code"}],"fingerprint":"reliability:roles:44"}}
{"event":"done","count":2}
`)
	parsed := Parse(raw, agents.OutputNDJSON)
	result := ExtractCandidates(parsed)
	if len(result.Candidates) != 2 {
		t.Fatalf("Candidates = %+v Diagnostics = %+v", result.Candidates, result.Diagnostics)
	}
	if result.Candidates[0].PrimaryPath != "apps/api/src/routes/repositories.ts" ||
		result.Candidates[0].Evidence[0].Kind != "unknown" ||
		result.Candidates[1].Fingerprint != "reliability:roles:44" ||
		result.Candidates[1].Evidence[0].Kind != "related_code" {
		t.Fatalf("Candidates = %+v", result.Candidates)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "invalid_json_line" {
		t.Fatalf("Diagnostics = %+v", result.Diagnostics)
	}
}

func TestExtractCandidatesFromRealCLIWrappers(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"type":"item.completed","item":{"type":"agent_message","text":"{\"findings\":[{\"claim\":\"codex wrapper finding\",\"category\":\"security\",\"severity\":\"high\",\"confidence\":0.8,\"locations\":[{\"path\":\"src/auth.ts\",\"start_line\":2,\"end_line\":2,\"side\":\"RIGHT\"}],\"evidence\":[{\"title\":\"codex text wrapper\",\"summary\":\"codex wrapped final response\"}]}]}"}}
{"type":"text","part":{"type":"text","text":"{\"findings\":[{\"claim\":\"opencode wrapper finding\",\"category\":\"correctness\",\"severity\":\"medium\",\"confidence\":0.7,\"locations\":[{\"path\":\"src/auth.ts\",\"start_line\":3,\"end_line\":3,\"side\":\"RIGHT\"}],\"evidence\":[{\"title\":\"opencode text wrapper\",\"summary\":\"opencode wrapped text event\"}]}]}"}}
{"response":"{\"findings\":[{\"claim\":\"gemini wrapper finding\",\"category\":\"reliability\",\"severity\":\"low\",\"confidence\":0.6,\"locations\":[{\"path\":\"src/auth.ts\",\"start_line\":4,\"end_line\":4,\"side\":\"RIGHT\"}],\"evidence\":[{\"title\":\"gemini response wrapper\",\"summary\":\"gemini wrapped response field\"}]}]}"}
{"result":"{\"findings\":[{\"claim\":\"claude wrapper finding\",\"category\":\"maintainability\",\"severity\":\"nit\",\"confidence\":0.5,\"locations\":[{\"path\":\"src/auth.ts\",\"start_line\":5,\"end_line\":5,\"side\":\"RIGHT\"}],\"evidence\":[{\"title\":\"claude result wrapper\",\"summary\":\"claude wrapped result field\"}]}]}"}
`)
	parsed := Parse(raw, agents.OutputJSONL)
	result := ExtractCandidates(parsed)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("Diagnostics = %+v", result.Diagnostics)
	}
	if len(result.Candidates) != 4 {
		t.Fatalf("Candidates = %+v", result.Candidates)
	}
	claims := []string{
		result.Candidates[0].Claim,
		result.Candidates[1].Claim,
		result.Candidates[2].Claim,
		result.Candidates[3].Claim,
	}
	for _, want := range []string{
		"codex wrapper finding",
		"opencode wrapper finding",
		"gemini wrapper finding",
		"claude wrapper finding",
	} {
		if !containsString(claims, want) {
			t.Fatalf("claims = %+v, missing %q", claims, want)
		}
	}
}

func TestExtractCandidatesIgnoresPlainTextRealCLIWrappers(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"type":"item.completed","item":{"type":"agent_message","text":"No findings."}}
{"type":"text","part":{"type":"text","text":"No findings."}}
{"response":"No findings."}
{"result":"No findings."}
`)
	parsed := Parse(raw, agents.OutputJSONL)
	result := ExtractCandidates(parsed)
	if len(result.Candidates) != 0 || len(result.Diagnostics) != 0 {
		t.Fatalf("Candidates = %+v Diagnostics = %+v", result.Candidates, result.Diagnostics)
	}
}

func TestExtractCandidatesSkipsInvalidFinding(t *testing.T) {
	t.Parallel()

	parsed := Parse([]byte(`{
		"summary": "mixed output",
		"findings": [
			{
				"claim": "valid finding survives",
				"category": "security",
				"severity": "high",
				"confidence": 0.82,
				"locations": [{"path": "a.go", "start_line": 10, "end_line": 11, "side": "RIGHT"}],
				"evidence": [{"title": "guard is absent", "summary": "the route has no guard"}]
			},
			{
				"claim": "",
				"category": "security",
				"severity": "critical",
				"confidence": 1.2,
				"locations": [{"path": "b.go", "start_line": 20, "end_line": 19, "side": "RIGHT"}],
				"evidence": [{"title": "bad range", "summary": "range is inverted"}]
			}
		]
	}`), agents.OutputJSON)

	result := ExtractCandidates(parsed)
	if len(result.Candidates) != 1 || result.Candidates[0].Claim != "valid finding survives" {
		t.Fatalf("Candidates = %+v", result.Candidates)
	}
	if len(result.Diagnostics) != 3 {
		t.Fatalf("Diagnostics = %+v", result.Diagnostics)
	}
	assertDiagnosticCode(t, result.Diagnostics, "missing_claim")
	assertDiagnosticCode(t, result.Diagnostics, "invalid_confidence")
	assertDiagnosticCode(t, result.Diagnostics, "invalid_location_range")
}

func TestExtractCandidatesNormalizesUnknownLabels(t *testing.T) {
	t.Parallel()

	parsed := Parse([]byte(`{
		"summary": "label mapping",
		"findings": [
			{
				"claim": "repository update reads stale role cache",
				"category": "bug",
				"severity": "critical",
				"confidence": 0.8,
				"locations": [{"path": "a.go", "start_line": 10, "end_line": 11, "side": "right"}],
				"evidence": [{"title": "stale role", "summary": "the cache is stale", "kind": "RELATED_CODE"}]
			}
		]
	}`), agents.OutputJSON)
	result := ExtractCandidates(parsed)
	if len(result.Candidates) != 1 || len(result.Diagnostics) != 0 {
		t.Fatalf("Candidates = %+v Diagnostics = %+v", result.Candidates, result.Diagnostics)
	}
	candidate := result.Candidates[0]
	if candidate.Category != "correctness" ||
		candidate.Severity != "blocker" ||
		candidate.Locations[0].Side != "RIGHT" ||
		candidate.Evidence[0].Kind != "related_code" {
		t.Fatalf("candidate = %+v", candidate)
	}
}

func TestExtractCandidatesIgnoresPublishLikeFields(t *testing.T) {
	t.Parallel()

	parsed := Parse([]byte(`{
		"summary": "malicious output",
		"publish": true,
		"review_event": "APPROVE",
		"decision_status": "accepted",
		"side_effects_allowed": true,
		"findings": [
			{
				"claim": "repository update reads stale role cache",
				"category": "reliability",
				"severity": "medium",
				"confidence": 0.8,
				"decision_status": "published",
				"publish": true,
				"review_event": "REQUEST_CHANGES",
				"side_effects_allowed": true,
				"locations": [{"path": "a.go", "start_line": 10, "end_line": 11, "side": "RIGHT"}],
				"evidence": [{"title": "stale role", "summary": "the cache is stale"}],
				"suggested_fix": "Inspect the role cache."
			}
		]
	}`), agents.OutputJSON)

	result := ExtractCandidates(parsed)
	if len(result.Candidates) != 1 || len(result.Diagnostics) != 0 {
		t.Fatalf("Candidates = %+v Diagnostics = %+v", result.Candidates, result.Diagnostics)
	}
	candidate := result.Candidates[0]
	if candidate.Claim != "repository update reads stale role cache" ||
		candidate.SuggestedFix != "Inspect the role cache." ||
		candidate.PrimaryPath != "a.go" {
		t.Fatalf("candidate = %+v", candidate)
	}
}

func TestExtractCandidatesNormalizesTextOutput(t *testing.T) {
	t.Parallel()

	output := runFakeAgent(t, "text-agent.sh", "review this fixture")
	parsed := Parse(output, agents.OutputText)
	result := ExtractCandidates(parsed)
	if len(result.Candidates) != 1 {
		t.Fatalf("Candidates = %+v Diagnostics = %+v", result.Candidates, result.Diagnostics)
	}
	candidate := result.Candidates[0]
	if candidate.Claim != "Repository settings can be changed without proving workspace admin permission." ||
		candidate.Category != "security" ||
		candidate.Severity != "medium" ||
		candidate.Confidence != 0.62 ||
		candidate.PrimaryPath != "apps/api/src/routes/repositories.ts" ||
		candidate.PrimaryStartLine != 87 ||
		candidate.SuggestedFix == "" ||
		candidate.DraftComment == "" {
		t.Fatalf("candidate = %+v", candidate)
	}
	assertDiagnosticCode(t, result.Diagnostics, "text_output_normalized")
}

func TestExtractCandidatesRepairsMalformedStructuredOutput(t *testing.T) {
	t.Parallel()

	output := runFakeAgent(t, "malformed-agent.sh", "review this fixture")
	parsed := ParseAuto(output)
	if parsed.Structured {
		t.Fatalf("parsed = %+v, want initial text fallback before repair", parsed)
	}
	result := ExtractCandidates(parsed)
	if len(result.Candidates) != 1 {
		t.Fatalf("Candidates = %+v Diagnostics = %+v", result.Candidates, result.Diagnostics)
	}
	candidate := result.Candidates[0]
	if candidate.Claim != "A route appears to mutate state without validation." ||
		candidate.Category != "correctness" ||
		candidate.Severity != "medium" ||
		candidate.PrimaryPath != "apps/api/src/routes/repositories.ts" {
		t.Fatalf("candidate = %+v", candidate)
	}
	assertDiagnosticCode(t, result.Diagnostics, "invalid_json")
	assertDiagnosticCode(t, result.Diagnostics, "repaired_json")
}

func assertDiagnosticCode(t *testing.T, diagnostics []Diagnostic, code string) {
	t.Helper()

	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	t.Fatalf("diagnostic %q not found in %+v", code, diagnostics)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
