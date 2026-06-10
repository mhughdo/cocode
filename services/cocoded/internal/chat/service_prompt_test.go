package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	cocodedb "github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/reviewprompt"
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
	if !strings.Contains(prompt, reviewprompt.UntrustedContextInstruction()) {
		t.Fatalf("prompt missing shared untrusted-context instruction: %s", prompt)
	}
}

func TestRenderContextRefsFormatsStructuredRefs(t *testing.T) {
	raw := json.RawMessage(`[
		{"ref_type":"finding","ref_id":"finding_1","label":"Nil guard panic"},
		{"path":"internal/app/server.go","label":"Server handler"}
	]`)
	rendered := renderContextRefs(raw)
	for _, want := range []string{
		"finding: `finding_1` (Nil guard panic)",
		"file: `internal/app/server.go` (Server handler)",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered refs missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "```json") {
		t.Fatalf("structured refs should render as a concise list:\n%s", rendered)
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

func TestPromptVisibleChatMessagesDropsSystemAndTransientFailures(t *testing.T) {
	messages := []Message{
		{
			ID:                "system_progress",
			AuthorType:        AuthorSystem,
			AuthorDisplayName: "System",
			Body:              "Workflow phase failed: context canceled",
			Status:            MessageStatusFailed,
		},
		{
			ID:                "review_progress",
			AuthorType:        AuthorOrchestrator,
			AuthorDisplayName: "Orchestrator",
			Body:              "Review started. I will coordinate reviewers.",
			Status:            MessageStatusCompleted,
			Metadata:          json.RawMessage(`{"answer_source":"review_progress"}`),
		},
		{
			ID:                "agent_failure",
			AuthorType:        AuthorAgent,
			AuthorDisplayName: "Codex CLI",
			Body:              "Codex CLI could not complete its review.\n\n```text\ncontext canceled\n```",
			Status:            MessageStatusFailed,
		},
		{
			ID:                "orchestrator_answer_with_transient_detail",
			AuthorType:        AuthorOrchestrator,
			AuthorDisplayName: "Orchestrator",
			Body:              "Reviewer coverage: Codex CLI previously failed with context canceled.",
			Status:            MessageStatusCompleted,
		},
		{
			ID:                "user_question",
			AuthorType:        AuthorUser,
			AuthorDisplayName: "You",
			Body:              "Can you explain finding 1 again?",
			Status:            MessageStatusCompleted,
		},
		{
			ID:                "agent_answer",
			AuthorType:        AuthorAgent,
			AuthorDisplayName: "Codex CLI",
			Body:              "Finding 1 is anchored at `internal/app/server.go:42`.",
			Status:            MessageStatusCompleted,
		},
	}

	filtered := promptVisibleChatMessages(messages, 10)
	if got, want := len(filtered), 2; got != want {
		t.Fatalf("filtered len = %d, want %d: %+v", got, want, filtered)
	}
	if filtered[0].ID != "user_question" || filtered[1].ID != "agent_answer" {
		t.Fatalf("filtered IDs = [%s, %s], want user_question and agent_answer", filtered[0].ID, filtered[1].ID)
	}
	rendered := renderChatMessages(filtered)
	if strings.Contains(rendered, "context canceled") || strings.Contains(rendered, "Review started") {
		t.Fatalf("rendered prompt context leaked system diagnostics:\n%s", rendered)
	}
}

func TestChatTurnStateMachine(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		want bool
	}{
		{name: "created starts routing", from: TurnStatusCreated, to: TurnStatusRouting, want: true},
		{name: "routing builds context", from: TurnStatusRouting, to: TurnStatusContextBuild, want: true},
		{name: "context can complete", from: TurnStatusContextBuild, to: TurnStatusCompleted, want: true},
		{name: "running can synthesize", from: TurnStatusRunning, to: TurnStatusSynthesizing, want: true},
		{name: "cancel request can cancel", from: TurnStatusCancelReq, to: TurnStatusCanceled, want: true},
		{name: "completed is terminal", from: TurnStatusCompleted, to: TurnStatusRunning, want: false},
		{name: "created cannot complete directly", from: TurnStatusCreated, to: TurnStatusCompleted, want: false},
		{name: "unknown status is invalid", from: "unknown", to: TurnStatusRunning, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validTurnTransition(tt.from, tt.to); got != tt.want {
				t.Fatalf("validTurnTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestCancelTurnMarksRequestAndRunExitsCanceled(t *testing.T) {
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
	createChatSessionFixture(t, queries, "review_session_cancel")
	service := Service{Database: database, Queries: queries}

	created, err := service.CreateTurn(ctx, AskParams{
		ReviewSessionID: "review_session_cancel",
		Body:            "Please stop this follow-up.",
		Audience:        AudienceOrchestrator,
	})
	if err != nil {
		t.Fatalf("CreateTurn() error = %v", err)
	}
	canceled, err := service.CancelTurn(ctx, created.Turn.ID)
	if err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}
	if canceled.Status != TurnStatusCancelReq {
		t.Fatalf("canceled status = %s, want %s", canceled.Status, TurnStatusCancelReq)
	}
	completed, err := service.runTurn(ctx, created.Turn.ID, AskParams{})
	if err != nil {
		t.Fatalf("runTurn() error = %v", err)
	}
	if completed.Turn.Status != TurnStatusCanceled {
		t.Fatalf("completed status = %s, want %s", completed.Turn.Status, TurnStatusCanceled)
	}
	if len(completed.Messages) != len(created.Messages) {
		t.Fatalf("runTurn appended messages after cancellation: before=%d after=%d", len(created.Messages), len(completed.Messages))
	}
}

func TestCreateTurnPersistsContextRefs(t *testing.T) {
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
	createChatSessionFixture(t, queries, "review_session_context_refs")
	service := Service{Database: database, Queries: queries}

	created, err := service.CreateTurn(ctx, AskParams{
		ReviewSessionID: "review_session_context_refs",
		Body:            "Explain this finding using the selected file.",
		Audience:        AudienceOrchestrator,
		ContextRefs: json.RawMessage(`[
			{"ref_type":"finding","ref_id":"finding_123","label":"Unsafe nil dereference"},
			{"path":"internal/app/server.go","label":"Server handler"},
			{"ref_type":"unknown","ref_id":"ignored"}
		]`),
	})
	if err != nil {
		t.Fatalf("CreateTurn() error = %v", err)
	}

	refs, err := service.listMessageContextRefs(ctx, created.Turn.UserMessageID)
	if err != nil {
		t.Fatalf("listMessageContextRefs() error = %v", err)
	}
	if got, want := len(refs), 2; got != want {
		t.Fatalf("refs len = %d, want %d: %+v", got, want, refs)
	}
	if refs[0].RefType != "finding" || refs[0].RefID != "finding_123" || refs[0].Label != "Unsafe nil dereference" {
		t.Fatalf("first ref = %+v", refs[0])
	}
	if refs[1].RefType != "file" || refs[1].RefID != "internal/app/server.go" {
		t.Fatalf("second ref = %+v", refs[1])
	}
	refsJSON, ok := service.messageContextRefsJSON(ctx, created.Turn.UserMessageID)
	if !ok {
		t.Fatalf("messageContextRefsJSON() did not return persisted refs")
	}
	if rendered := renderContextRefs(refsJSON); !strings.Contains(rendered, "finding: `finding_123`") || !strings.Contains(rendered, "file: `internal/app/server.go`") {
		t.Fatalf("persisted refs did not render cleanly:\n%s", rendered)
	}
}

func TestSharedContextRecipientPrefersExternalVisibility(t *testing.T) {
	configs := []dbgen.AgentConfig{
		{
			ID:               "agent_config_local",
			AdapterKind:      string(agents.AdapterLocalVerifier),
			CapabilitiesJson: `{"metadata":{"egress":"local"}}`,
		},
		{
			ID:               "agent_config_external",
			AdapterKind:      string(agents.AdapterCLINonInteractive),
			CapabilitiesJson: `{"metadata":{"egress":"external"}}`,
		},
	}
	if got := sharedContextRecipientAgentConfigID(configs); got != "agent_config_external" {
		t.Fatalf("sharedContextRecipientAgentConfigID() = %q, want external config", got)
	}
	if got := sharedContextRecipientAgentConfigID(configs[:1]); got != "agent_config_local" {
		t.Fatalf("sharedContextRecipientAgentConfigID(local) = %q, want local config", got)
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

func TestShouldHideReviewAgentRunFromChatSkipsInternalWorkflowRuns(t *testing.T) {
	tests := []struct {
		role string
		want bool
	}{
		{role: "chat", want: false},
		{role: "chat_synthesis", want: false},
		{role: "orchestrator", want: true},
		{role: "Orchestrator", want: true},
		{role: "verifier", want: true},
		{role: "finding_verifier", want: true},
		{role: "primary_reviewer", want: false},
		{role: "General Reviewer", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			got := shouldHideReviewAgentRunFromChat(dbgen.AgentRun{Role: tt.role})
			if got != tt.want {
				t.Fatalf("shouldHideReviewAgentRunFromChat(%q) = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestRemoveHiddenReviewAgentRunMessagesDeletesPersistedInternalCards(t *testing.T) {
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
	now := "2026-05-03T00:00:00Z"
	if _, err := queries.CreateWorkspace(ctx, dbgen.CreateWorkspaceParams{
		ID:           "workspace_cleanup",
		Name:         "Workspace",
		RootPath:     t.TempDir(),
		SettingsJson: "{}",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, err := queries.CreateRepository(ctx, dbgen.CreateRepositoryParams{
		ID:          "repo_cleanup",
		WorkspaceID: "workspace_cleanup",
		Name:        "repo",
		LocalPath:   t.TempDir(),
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
	if _, err := queries.CreatePullRequestSnapshot(ctx, dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_cleanup",
		RepositoryID: "repo_cleanup",
		SourceType:   "branch_compare",
		MetadataJson: "{}",
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}
	if _, err := queries.CreateReviewSession(ctx, dbgen.CreateReviewSessionParams{
		ID:                  "review_session_cleanup",
		WorkspaceID:         "workspace_cleanup",
		RepositoryID:        "repo_cleanup",
		SnapshotID:          "snapshot_cleanup",
		Title:               "Review",
		Status:              "completed",
		ReviewDepth:         "standard",
		RuntimeLimitSeconds: 60,
		ContextPolicyJson:   "{}",
		CreatedAt:           now,
		UpdatedAt:           now,
	}); err != nil {
		t.Fatalf("CreateReviewSession() error = %v", err)
	}
	if _, err := queries.CreateAgentConfig(ctx, dbgen.CreateAgentConfigParams{
		ID:               "agent_config_cleanup",
		Name:             "Codex CLI",
		Role:             "general_reviewer",
		AdapterKind:      "cli_noninteractive",
		Command:          sql.NullString{String: "codex", Valid: true},
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       "json",
		CapabilitiesJson: `{"supports_json":true}`,
		SettingsJson:     "{}",
		Enabled:          1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("CreateAgentConfig() error = %v", err)
	}
	for _, run := range []struct {
		id   string
		role string
	}{
		{id: "agent_run_reviewer", role: "General Reviewer"},
		{id: "agent_run_chat", role: "chat"},
		{id: "agent_run_verifier", role: "verifier"},
	} {
		if _, err := queries.CreateAgentRun(ctx, dbgen.CreateAgentRunParams{
			ID:              run.id,
			ReviewSessionID: "review_session_cleanup",
			AgentConfigID:   "agent_config_cleanup",
			Status:          "succeeded",
			Role:            run.role,
			StartedAt:       sql.NullString{String: now, Valid: true},
			CompletedAt:     sql.NullString{String: now, Valid: true},
			MetadataJson:    "{}",
		}); err != nil {
			t.Fatalf("CreateAgentRun(%s) error = %v", run.id, err)
		}
	}
	if _, err := database.ExecContext(ctx, `
INSERT INTO chat_threads (id, review_session_id, title, status, created_at, updated_at)
VALUES ('thread_cleanup', 'review_session_cleanup', 'Review', 'active', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert chat thread error = %v", err)
	}
	service := Service{Database: database, Queries: queries}
	if _, err := service.appendMessage(ctx, appendMessageParams{
		ThreadID:          "thread_cleanup",
		AuthorType:        AuthorAgent,
		AuthorDisplayName: "Codex CLI",
		AgentRunID:        "agent_run_reviewer",
		Body:              "reviewer output",
		Status:            MessageStatusCompleted,
		MetadataJSON:      []byte(`{"answer_source":"review_agent_run","review_agent_run_id":"agent_run_reviewer"}`),
	}); err != nil {
		t.Fatalf("append reviewer message error = %v", err)
	}
	if _, err := service.appendMessage(ctx, appendMessageParams{
		ThreadID:          "thread_cleanup",
		AuthorType:        AuthorAgent,
		AuthorDisplayName: "Codex CLI",
		AgentRunID:        "agent_run_chat",
		Body:              "chat answer",
		Status:            MessageStatusCompleted,
		MetadataJSON:      []byte(`{"answer_source":"review_agent_run","review_agent_run_id":"agent_run_chat"}`),
	}); err != nil {
		t.Fatalf("append chat message error = %v", err)
	}
	if _, err := service.appendMessage(ctx, appendMessageParams{
		ThreadID:          "thread_cleanup",
		AuthorType:        AuthorAgent,
		AuthorDisplayName: "Codex CLI",
		AgentRunID:        "agent_run_verifier",
		Body:              "verifier output",
		Status:            MessageStatusCompleted,
		MetadataJSON:      []byte(`{"answer_source":"review_agent_run","review_agent_run_id":"agent_run_verifier"}`),
	}); err != nil {
		t.Fatalf("append verifier message error = %v", err)
	}

	messages, err := service.listMessages(ctx, "thread_cleanup")
	if err != nil {
		t.Fatalf("listMessages() error = %v", err)
	}
	visible, err := service.removeHiddenReviewAgentRunMessages(ctx, messages)
	if err != nil {
		t.Fatalf("removeHiddenReviewAgentRunMessages() error = %v", err)
	}
	if len(visible) != 2 {
		t.Fatalf("visible messages = %+v", visible)
	}
	visibleByRun := map[string]bool{}
	for _, message := range visible {
		visibleByRun[message.AgentRunID] = true
	}
	if !visibleByRun["agent_run_reviewer"] || !visibleByRun["agent_run_chat"] || visibleByRun["agent_run_verifier"] {
		t.Fatalf("visible messages = %+v", visible)
	}
	persisted, err := service.listMessages(ctx, "thread_cleanup")
	if err != nil {
		t.Fatalf("listMessages(after cleanup) error = %v", err)
	}
	if len(persisted) != 2 {
		t.Fatalf("persisted messages = %+v", persisted)
	}
	persistedByRun := map[string]bool{}
	for _, message := range persisted {
		persistedByRun[message.AgentRunID] = true
	}
	if !persistedByRun["agent_run_reviewer"] || !persistedByRun["agent_run_chat"] || persistedByRun["agent_run_verifier"] {
		t.Fatalf("persisted messages = %+v", persisted)
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

func createChatSessionFixture(t *testing.T, queries *dbgen.Queries, sessionID string) {
	t.Helper()

	ctx := context.Background()
	now := "2026-05-03T00:00:00Z"
	if _, err := queries.CreateWorkspace(ctx, dbgen.CreateWorkspaceParams{
		ID:           "workspace_" + sessionID,
		Name:         "Workspace",
		RootPath:     t.TempDir(),
		SettingsJson: "{}",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, err := queries.CreateRepository(ctx, dbgen.CreateRepositoryParams{
		ID:          "repo_" + sessionID,
		WorkspaceID: "workspace_" + sessionID,
		Name:        "repo",
		LocalPath:   t.TempDir(),
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
	if _, err := queries.CreatePullRequestSnapshot(ctx, dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_" + sessionID,
		RepositoryID: "repo_" + sessionID,
		SourceType:   "branch_compare",
		MetadataJson: "{}",
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}
	if _, err := queries.CreateReviewSession(ctx, dbgen.CreateReviewSessionParams{
		ID:                  sessionID,
		WorkspaceID:         "workspace_" + sessionID,
		RepositoryID:        "repo_" + sessionID,
		SnapshotID:          "snapshot_" + sessionID,
		Title:               "Cancel test",
		Status:              "running",
		ReviewDepth:         "standard",
		RuntimeLimitSeconds: 0,
		ContextPolicyJson:   "{}",
		CreatedAt:           now,
		UpdatedAt:           now,
	}); err != nil {
		t.Fatalf("CreateReviewSession() error = %v", err)
	}
}
