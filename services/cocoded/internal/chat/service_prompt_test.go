package chat

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestChatPromptProvidesFindingsContext(t *testing.T) {
	session := dbgen.ReviewSession{ID: "review_session_1", Title: "Auth review"}
	thread := Thread{ID: "thread_1"}
	userMessage := Message{ID: "message_1"}
	config := dbgen.AgentConfig{Name: "Gemini CLI"}
	context := chatPromptContext{
		Findings: []dbgen.Finding{{
			CanonicalClaim:   "Missing authorization check",
			Severity:         "high",
			DecisionStatus:   "undecided",
			Confidence:       0.9,
			PrimaryPath:      sql.NullString{String: "src/server.js", Valid: true},
			PrimaryStartLine: sql.NullInt64{Int64: 10, Valid: true},
			EvidenceSummary:  sql.NullString{String: "Route calls DB without requireAdmin.", Valid: true},
			SuggestedFix:     sql.NullString{String: "Restore requireAdmin before the DB call.", Valid: true},
		}},
		IncludeRecentMessages: true,
		RecentMessages: []Message{{
			AuthorType:        AuthorUser,
			AuthorDisplayName: "You",
			Body:              "Give me all findings again",
			Status:            MessageStatusCompleted,
		}},
	}

	prompt := chatPrompt(session, thread, userMessage, config, context, "Give me all findings again")
	if !strings.Contains(prompt, "# Current findings") {
		t.Fatalf("prompt missing findings section")
	}
	if !strings.Contains(prompt, "Missing authorization check") {
		t.Fatalf("prompt missing normalized finding: %s", prompt)
	}
	if strings.Contains(prompt, "Please provide the previous findings") {
		t.Fatalf("prompt should not ask the user for prior findings")
	}
}

func TestUserVisibleChatFindingsFiltersMachineEvents(t *testing.T) {
	findings := []dbgen.Finding{
		{ID: "finding_real", CanonicalClaim: "Missing auth check"},
		{ID: "finding_hook", CanonicalClaim: `{"type":"system","subtype":"hook_started","hook_name":"SessionStart:startup"}`},
	}

	filtered := userVisibleChatFindings(findings)
	if len(filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1", len(filtered))
	}
	if filtered[0].ID != "finding_real" {
		t.Fatalf("filtered[0].ID = %q, want finding_real", filtered[0].ID)
	}
}
