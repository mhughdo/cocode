package chat

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	cocodedb "github.com/hughdo/cocode/services/cocoded/internal/db"
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

func TestLocalAnswerExplainsMatchingFinding(t *testing.T) {
	session := dbgen.ReviewSession{
		ID:     "review_session_1",
		Status: "completed",
	}
	answer := localAnswer(session, []dbgen.Finding{{
		ID:                     "finding_1",
		CanonicalClaim:         "Single-sided token prices panic in pickTokenPrice",
		Category:               "correctness",
		Severity:               "high",
		Confidence:             0.78,
		VerificationStatus:     "unverified",
		DecisionStatus:         "accepted",
		PrimaryPath:            sql.NullString{String: "internal/app/prices.go", Valid: true},
		PrimaryStartLine:       sql.NullInt64{Int64: 208, Valid: true},
		EvidenceSummary:        sql.NullString{String: "The code indexes prices[1] after only checking prices[0].", Valid: true},
		CounterEvidenceSummary: sql.NullString{String: "No guard was found on the current path.", Valid: true},
		SuggestedFix:           sql.NullString{String: "Require both token price entries before averaging.", Valid: true},
	}}, nil, "Explain the [high] Single-sided token prices panic in pickTokenPrice finding again for me")

	for _, want := range []string{
		"Single-sided token prices panic in pickTokenPrice",
		"internal/app/prices.go:208",
		"The code indexes prices[1]",
		"Require both token price entries",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("local answer missing %q:\n%s", want, answer)
		}
	}
	if strings.Contains(answer, "Current review status:") {
		t.Fatalf("finding question should not fall back to status-only answer:\n%s", answer)
	}
}

func TestSessionReviewerAgentConfigsUsesAssignmentRole(t *testing.T) {
	ctx := context.Background()
	database, err := cocodedb.Open(ctx, cocodedb.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	if err := cocodedb.Apply(ctx, database, cocodedb.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	queries := dbgen.New(database)
	if _, err := queries.CreateWorkspace(ctx, dbgen.CreateWorkspaceParams{
		ID:           "workspace_1",
		Name:         "Workspace",
		RootPath:     t.TempDir(),
		SettingsJson: "{}",
		CreatedAt:    "2026-05-03T00:00:00Z",
		UpdatedAt:    "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, err := queries.CreateRepository(ctx, dbgen.CreateRepositoryParams{
		ID:          "repo_1",
		WorkspaceID: "workspace_1",
		Name:        "repo",
		LocalPath:   t.TempDir(),
		CreatedAt:   "2026-05-03T00:00:00Z",
		UpdatedAt:   "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
	if _, err := queries.CreatePullRequestSnapshot(ctx, dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_1",
		RepositoryID: "repo_1",
		SourceType:   "branch_compare",
		MetadataJson: "{}",
		CreatedAt:    "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}
	session, err := queries.CreateReviewSession(ctx, dbgen.CreateReviewSessionParams{
		ID:                  "review_session_1",
		WorkspaceID:         "workspace_1",
		RepositoryID:        "repo_1",
		SnapshotID:          "snapshot_1",
		Title:               "Review",
		Status:              "completed",
		ReviewDepth:         "standard",
		RuntimeLimitSeconds: 60,
		ContextPolicyJson:   "{}",
		CreatedAt:           "2026-05-03T00:00:00Z",
		UpdatedAt:           "2026-05-03T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("CreateReviewSession() error = %v", err)
	}
	if _, err := queries.CreateAgentConfig(ctx, dbgen.CreateAgentConfigParams{
		ID:               "agent_config_orchestrator",
		Name:             "Orchestrator",
		Role:             "orchestrator",
		AdapterKind:      "cli_noninteractive",
		Command:          sql.NullString{String: "codex", Valid: true},
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       "json",
		CapabilitiesJson: `{"supports_json":true,"can_read":true,"output_modes":["json"]}`,
		SettingsJson:     "{}",
		Enabled:          1,
		CreatedAt:        "2026-05-03T00:00:00Z",
		UpdatedAt:        "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig() error = %v", err)
	}
	if _, err := queries.CreateReviewSessionAgent(ctx, dbgen.CreateReviewSessionAgentParams{
		ID:                   "review_session_agent_1",
		ReviewSessionID:      session.ID,
		AgentConfigID:        "agent_config_orchestrator",
		Role:                 "General Reviewer",
		RunOrder:             1,
		Enabled:              1,
		SettingsOverrideJson: "{}",
	}); err != nil {
		t.Fatalf("CreateReviewSessionAgent() error = %v", err)
	}

	configs, err := (Service{Queries: queries}).sessionReviewerAgentConfigs(ctx, session.ID)
	if err != nil {
		t.Fatalf("sessionReviewerAgentConfigs() error = %v", err)
	}
	if len(configs) != 1 || configs[0].ID != "agent_config_orchestrator" {
		t.Fatalf("sessionReviewerAgentConfigs() = %+v, want assigned reviewer config", configs)
	}
}
