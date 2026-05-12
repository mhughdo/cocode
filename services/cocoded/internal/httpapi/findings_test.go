package httpapi

import (
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestUserVisibleFindingsFiltersMachineEventClaims(t *testing.T) {
	rows := []dbgen.Finding{
		{
			ID:             "finding_auth",
			CanonicalClaim: "Missing authorization check",
		},
		{
			ID:             "finding_hook",
			CanonicalClaim: `{"type":"system","subtype":"hook_started","hook_name":"SessionStart:startup","session_id":"session_1"}`,
		},
		{
			ID:             "finding_turn",
			CanonicalClaim: `{"type":"turn.started"}`,
		},
	}

	filtered := userVisibleFindings(rows)
	if len(filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1", len(filtered))
	}
	if filtered[0].ID != "finding_auth" {
		t.Fatalf("filtered[0].ID = %q, want finding_auth", filtered[0].ID)
	}
}

func TestFilterFindingsSupportsAgentFileAndQuery(t *testing.T) {
	rows := []dbgen.Finding{
		{
			ID:             "finding_auth",
			CanonicalClaim: "Missing authorization check",
			Severity:       "high",
			DecisionStatus: "undecided",
			PrimaryPath:    nullableString("src/server.js"),
		},
		{
			ID:             "finding_perf",
			CanonicalClaim: "Slow query plan",
			Severity:       "medium",
			DecisionStatus: "undecided",
			PrimaryPath:    nullableString("src/db.js"),
		},
	}
	sources := map[string][]FindingSourceAgentResponse{
		"finding_auth": {
			{
				AgentConfigID: "agent_codex",
				Name:          "Codex CLI",
				ModelLabel:    "GPT-5.5",
			},
		},
		"finding_perf": {
			{
				AgentConfigID: "agent_gemini",
				Name:          "Gemini CLI",
				ModelLabel:    "Gemini 3.1 Pro",
			},
		},
	}

	filtered := filterFindings(rows, sources, "needs_triage", "high", "agent_codex", "src/server.js", "authorization")
	if len(filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1", len(filtered))
	}
	if filtered[0].ID != "finding_auth" {
		t.Fatalf("filtered[0].ID = %q, want finding_auth", filtered[0].ID)
	}
}
