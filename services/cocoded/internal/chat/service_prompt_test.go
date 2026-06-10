package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/agentrun"
	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/contextbundle"
	cocodedb "github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/eventlog"
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

func TestPromptThreadHistoryCompactsOlderMessages(t *testing.T) {
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
	createChatSessionFixture(t, queries, "review_session_compact")
	service := Service{Database: database, Queries: queries}
	view, err := service.EnsureSessionThread(ctx, "review_session_compact")
	if err != nil {
		t.Fatalf("EnsureSessionThread() error = %v", err)
	}
	for index := 0; index < defaultChatRecentLimit+4; index++ {
		author := AuthorUser
		name := "You"
		if index%2 == 1 {
			author = AuthorAgent
			name = "Reviewer"
		}
		if _, err := service.appendMessage(ctx, appendMessageParams{
			ThreadID:          view.Thread.ID,
			AuthorType:        author,
			AuthorDisplayName: name,
			Body:              "message body that should be summarized " + string(rune('A'+index)),
			Status:            MessageStatusCompleted,
			MetadataJSON:      []byte(`{"answer_source":"test"}`),
		}); err != nil {
			t.Fatalf("appendMessage(%d) error = %v", index, err)
		}
	}

	history, err := service.promptThreadHistory(ctx, view.Thread.ID, defaultChatRecentLimit)
	if err != nil {
		t.Fatalf("promptThreadHistory() error = %v", err)
	}
	if len(history.Recent) != defaultChatRecentLimit {
		t.Fatalf("recent len = %d, want %d", len(history.Recent), defaultChatRecentLimit)
	}
	for _, want := range []string{"Compacted", "Participants:", "Earlier turns"} {
		if !strings.Contains(history.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, history.Summary)
		}
	}
	prompt := chatPrompt(
		dbgen.ReviewSession{ID: "review_session_compact", Title: "Review"},
		view.Thread,
		Message{ID: "message_current"},
		dbgen.AgentConfig{Name: "Codex CLI"},
		chatPromptContext{
			ThreadSummary:         history.Summary,
			RecentMessages:        history.Recent,
			IncludeRecentMessages: true,
		},
		"what happened?",
	)
	if !strings.Contains(prompt, "# Earlier centralized chat summary") {
		t.Fatalf("prompt missing compact summary section:\n%s", prompt)
	}
}

func TestLoadThreadDoesNotSyncReviewProgress(t *testing.T) {
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
	createChatSessionFixture(t, queries, "review_session_readonly")
	service := Service{Database: database, Queries: queries}
	view, err := service.EnsureSessionThread(ctx, "review_session_readonly")
	if err != nil {
		t.Fatalf("EnsureSessionThread() error = %v", err)
	}
	initialCount := len(view.Messages)
	if _, err := queries.CreateEvent(ctx, dbgen.CreateEventParams{
		ID:              "event_readonly_queued",
		ReviewSessionID: nullableString("review_session_readonly"),
		Type:            "ReviewSessionQueued",
		Level:           "info",
		Sequence:        1,
		PayloadJson:     `{"status":"queued"}`,
		CreatedAt:       "2026-05-03T00:08:00Z",
	}); err != nil {
		t.Fatalf("CreateEvent() error = %v", err)
	}

	loaded, err := service.LoadThread(ctx, view.Thread.ID)
	if err != nil {
		t.Fatalf("LoadThread() error = %v", err)
	}
	if len(loaded.Messages) != initialCount {
		t.Fatalf("LoadThread changed message count: got %d want %d", len(loaded.Messages), initialCount)
	}
	for _, message := range loaded.Messages {
		if strings.Contains(message.Body, "Review queued") {
			t.Fatalf("LoadThread synced progress unexpectedly: %+v", loaded.Messages)
		}
	}
	ensured, err := service.EnsureSessionThread(ctx, "review_session_readonly")
	if err != nil {
		t.Fatalf("EnsureSessionThread(reload) error = %v", err)
	}
	if len(ensured.Messages) <= initialCount {
		t.Fatalf("EnsureSessionThread should sync progress after event: before=%d after=%d", initialCount, len(ensured.Messages))
	}
}

func TestReconcileInterruptedTurnsMarksNonTerminalTurns(t *testing.T) {
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
	createChatSessionFixture(t, queries, "review_session_reconcile")
	service := Service{Database: database, Queries: queries}
	first, err := service.CreateTurn(ctx, AskParams{
		ReviewSessionID: "review_session_reconcile",
		Body:            "This worker never started.",
		Audience:        AudienceOrchestrator,
	})
	if err != nil {
		t.Fatalf("CreateTurn(first) error = %v", err)
	}
	second, err := service.CreateTurn(ctx, AskParams{
		ReviewSessionID: "review_session_reconcile",
		Body:            "Please cancel this.",
		Audience:        AudienceOrchestrator,
	})
	if err != nil {
		t.Fatalf("CreateTurn(second) error = %v", err)
	}
	if _, err := service.CancelTurn(ctx, second.Turn.ID); err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}

	count, err := service.ReconcileInterruptedTurns(ctx)
	if err != nil {
		t.Fatalf("ReconcileInterruptedTurns() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("reconciled count = %d, want 2", count)
	}
	reconciledFirst, err := service.turnByID(ctx, first.Turn.ID)
	if err != nil {
		t.Fatalf("turnByID(first) error = %v", err)
	}
	if reconciledFirst.Status != TurnStatusFailed || reconciledFirst.ErrorCode != "interrupted" {
		t.Fatalf("first turn = %+v", reconciledFirst)
	}
	reconciledSecond, err := service.turnByID(ctx, second.Turn.ID)
	if err != nil {
		t.Fatalf("turnByID(second) error = %v", err)
	}
	if reconciledSecond.Status != TurnStatusCanceled {
		t.Fatalf("second turn status = %s, want canceled", reconciledSecond.Status)
	}
}

func TestChatMessageAndTurnEventsCarryDurablePayloads(t *testing.T) {
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
	createChatSessionFixture(t, queries, "review_session_events")
	recorder := &recordingEventLog{}
	service := Service{Database: database, Queries: queries, Events: recorder}
	view, err := service.EnsureSessionThread(ctx, "review_session_events")
	if err != nil {
		t.Fatalf("EnsureSessionThread() error = %v", err)
	}
	message, err := service.appendMessage(ctx, appendMessageParams{
		ThreadID:          view.Thread.ID,
		AuthorType:        AuthorAgent,
		AuthorDisplayName: "Codex CLI",
		Body:              "streaming answer",
		Status:            MessageStatusStreaming,
		MetadataJSON:      []byte(`{"answer_source":"agent"}`),
	})
	if err != nil {
		t.Fatalf("appendMessage() error = %v", err)
	}
	updated, err := service.updateMessage(ctx, message.ID, updateMessageParams{
		Body:         "final answer",
		Status:       MessageStatusCompleted,
		MetadataJSON: []byte(`{"answer_source":"agent","done":true}`),
	})
	if err != nil {
		t.Fatalf("updateMessage() error = %v", err)
	}
	created, err := service.CreateTurn(ctx, AskParams{
		ReviewSessionID: "review_session_events",
		Body:            "question",
		Audience:        AudienceOrchestrator,
	})
	if err != nil {
		t.Fatalf("CreateTurn() error = %v", err)
	}
	if _, err := service.updateTurn(ctx, created.Turn, TurnStatusRouting, "", ""); err != nil {
		t.Fatalf("updateTurn() error = %v", err)
	}

	createdEvent := recorder.lastMessageEvent("ChatMessageCreated", message.ID)
	if createdEvent.Type == "" {
		t.Fatalf("missing ChatMessageCreated event: %+v", recorder.events)
	}
	createdPayload := decodeEventPayload(t, createdEvent.PayloadJson)
	if createdPayload["message_id"] != message.ID {
		t.Fatalf("created payload message_id = %v, want %s", createdPayload["message_id"], message.ID)
	}
	messagePayload, ok := createdPayload["message"].(map[string]any)
	if !ok {
		t.Fatalf("created payload missing message object: %+v", createdPayload)
	}
	if messagePayload["status"] != MessageStatusStreaming || messagePayload["body"] != "streaming answer" {
		t.Fatalf("created message payload = %+v", messagePayload)
	}
	updatedEvent := recorder.lastMessageEvent("ChatMessageUpdated", message.ID)
	if updatedEvent.Type == "" {
		t.Fatalf("missing ChatMessageUpdated event: %+v", recorder.events)
	}
	updatedPayload := decodeEventPayload(t, updatedEvent.PayloadJson)
	updatedMessage := updatedPayload["message"].(map[string]any)
	if updatedMessage["status"] != MessageStatusCompleted || updatedMessage["body"] != updated.Body {
		t.Fatalf("updated message payload = %+v", updatedMessage)
	}
	turnEvent := recorder.lastEventOfType("ChatTurnStatusChanged")
	if turnEvent.Type == "" {
		t.Fatalf("missing ChatTurnStatusChanged event: %+v", recorder.events)
	}
	turnPayload := decodeEventPayload(t, turnEvent.PayloadJson)
	if turnPayload["chat_turn_id"] != created.Turn.ID || turnPayload["status"] != TurnStatusRouting {
		t.Fatalf("turn payload = %+v", turnPayload)
	}
}

func TestChatAgentRunEventSinkEmitsFullMessageDelta(t *testing.T) {
	ctx := context.Background()
	recorder := &recordingEventLog{}
	service := Service{
		Events: recorder,
		NewID: func(prefix string) string {
			return prefix + "1"
		},
		Now: func() time.Time {
			return time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
		},
	}
	longDelta := strings.Repeat("full-delta-", 2048)
	err := service.appendAgentRunEvent(ctx, "review_session_delta", "chat_message_delta", agents.AgentEvent{
		Type:   agents.EventOutput,
		RunID:  "agent_run_delta",
		Stream: "stdout",
		Text:   longDelta,
	})
	if err != nil {
		t.Fatalf("appendAgentRunEvent() error = %v", err)
	}
	deltaEvent := recorder.lastEventOfType("ChatMessageDelta")
	if deltaEvent.Type == "" {
		t.Fatalf("missing ChatMessageDelta event: %+v", recorder.events)
	}
	payload := decodeEventPayload(t, deltaEvent.PayloadJson)
	if payload["message_id"] != "chat_message_delta" || payload["agent_run_id"] != "agent_run_delta" {
		t.Fatalf("delta payload IDs = %+v", payload)
	}
	if payload["text_delta"] != longDelta {
		t.Fatalf("delta payload was truncated: got %d bytes want %d", len(stringValue(payload["text_delta"])), len(longDelta))
	}
	outputEvent := recorder.lastEventOfType("AgentRunOutput")
	if outputEvent.Type == "" {
		t.Fatalf("missing AgentRunOutput event: %+v", recorder.events)
	}
	outputPayload := decodeEventPayload(t, outputEvent.PayloadJson)
	if len(stringValue(outputPayload["text_preview"])) >= len(longDelta) {
		t.Fatalf("runtime preview should remain bounded")
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

func TestBuildOrReuseChatContextUsesPersistedBundle(t *testing.T) {
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
	createChatSessionFixture(t, queries, "review_session_cache")
	session, err := queries.GetReviewSession(ctx, "review_session_cache")
	if err != nil {
		t.Fatalf("GetReviewSession() error = %v", err)
	}
	if _, err := queries.CreateAgentConfig(ctx, dbgen.CreateAgentConfigParams{
		ID:               "agent_config_cache",
		Name:             "Cache Agent",
		Role:             "reviewer",
		AdapterKind:      string(agents.AdapterCLINonInteractive),
		Command:          sql.NullString{String: "codex", Valid: true},
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       "json",
		CapabilitiesJson: `{"supports_json":true,"can_read":true,"output_modes":["json"],"metadata":{"egress":"external"}}`,
		SettingsJson:     "{}",
		Enabled:          1,
		CreatedAt:        "2026-05-03T00:00:00Z",
		UpdatedAt:        "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig() error = %v", err)
	}
	store, err := artifact.New(t.TempDir(), queries)
	if err != nil {
		t.Fatalf("artifact.New() error = %v", err)
	}
	itemArtifact, err := store.Save(ctx, artifact.SaveParams{
		ID:              "artifact_cached_item",
		WorkspaceID:     session.WorkspaceID,
		ReviewSessionID: sql.NullString{String: session.ID, Valid: true},
		Kind:            "context_item",
		RelativePath:    "context/cache-item.txt",
		ContentType:     "text/plain",
		MetadataJSON:    "{}",
		CreatedAt:       "2026-05-03T00:00:00Z",
	}, []byte("cached file content"))
	if err != nil {
		t.Fatalf("Save(item) error = %v", err)
	}
	bundleArtifact, err := store.Save(ctx, artifact.SaveParams{
		ID:              "artifact_cached_bundle",
		WorkspaceID:     session.WorkspaceID,
		ReviewSessionID: sql.NullString{String: session.ID, Valid: true},
		Kind:            "context_bundle",
		RelativePath:    "context/cache-bundle.md",
		ContentType:     "text/markdown",
		MetadataJSON:    "{}",
		CreatedAt:       "2026-05-03T00:00:00Z",
	}, []byte("# cached bundle"))
	if err != nil {
		t.Fatalf("Save(bundle) error = %v", err)
	}
	_, policyJSON, err := resolvedChatContextPolicy(session)
	if err != nil {
		t.Fatalf("resolvedChatContextPolicy() error = %v", err)
	}
	if _, err := queries.CreateContextBundle(ctx, dbgen.CreateContextBundleParams{
		ID:              "bundle_cached_review",
		ReviewSessionID: session.ID,
		AgentConfigID:   sql.NullString{String: "agent_config_cache", Valid: true},
		Scope:           string(contextbundle.ScopeReview),
		TokenEstimate:   12,
		ItemCount:       2,
		ArtifactID:      sql.NullString{String: bundleArtifact.ID, Valid: true},
		PolicyJson:      policyJSON,
		CreatedAt:       "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateContextBundle() error = %v", err)
	}
	if _, err := queries.CreateContextItem(ctx, dbgen.CreateContextItemParams{
		ID:              "context_item_prompt",
		ContextBundleID: "bundle_cached_review",
		Kind:            string(contextbundle.ItemPromptMaterial),
		Title:           sql.NullString{String: "Prompt material", Valid: true},
		TokenEstimate:   2,
		MetadataJson:    `{"snapshot_id":"snapshot_review_session_cache"}`,
	}); err != nil {
		t.Fatalf("CreateContextItem(prompt) error = %v", err)
	}
	if _, err := queries.CreateContextItem(ctx, dbgen.CreateContextItemParams{
		ID:                "context_item_file",
		ContextBundleID:   "bundle_cached_review",
		Kind:              string(contextbundle.ItemFullFile),
		Path:              sql.NullString{String: "app/main.go", Valid: true},
		Title:             sql.NullString{String: "Full file", Valid: true},
		ContentArtifactID: sql.NullString{String: itemArtifact.ID, Valid: true},
		TokenEstimate:     10,
		MetadataJson:      `{}`,
	}); err != nil {
		t.Fatalf("CreateContextItem(file) error = %v", err)
	}

	service := Service{Database: database, Queries: queries, Artifacts: store}
	result, err := service.buildOrReuseChatContext(ctx, session, "agent_config_cache")
	if err != nil {
		t.Fatalf("buildOrReuseChatContext() error = %v", err)
	}
	if result.Bundle.ID != "bundle_cached_review" {
		t.Fatalf("bundle ID = %q, want cached bundle", result.Bundle.ID)
	}
	rendered := contextbundle.RenderBundle(result.Bundle)
	if !strings.Contains(rendered, "cached file content") {
		t.Fatalf("cached bundle was not hydrated with item content:\n%s", rendered)
	}
}

func TestValidateAgentConfigAllowsSessionCapableProtocolAdapters(t *testing.T) {
	protocolConfig := dbgen.AgentConfig{
		ID:               "agent_config_protocol",
		AdapterKind:      string(agents.AdapterJSONRPCStdio),
		CapabilitiesJson: `{"supports_json":true,"supports_streaming":true,"supports_sessions":true,"can_read":true,"can_write":false,"output_modes":["json"]}`,
	}
	if err := validateAgentConfig(protocolConfig); err != nil {
		t.Fatalf("validateAgentConfig(protocol) error = %v", err)
	}

	withoutSessions := protocolConfig
	withoutSessions.CapabilitiesJson = `{"supports_json":true,"supports_streaming":true,"supports_sessions":false,"can_read":true,"can_write":false,"output_modes":["json"]}`
	if err := validateAgentConfig(withoutSessions); err == nil || !strings.Contains(err.Error(), "unsupported for centralized chat") {
		t.Fatalf("validateAgentConfig(without sessions) error = %v, want unsupported", err)
	}

	localVerifier := protocolConfig
	localVerifier.AdapterKind = string(agents.AdapterLocalVerifier)
	localVerifier.CapabilitiesJson = `{"supports_json":true,"can_read":true,"can_write":false,"output_modes":["json"]}`
	if err := validateAgentConfig(localVerifier); err == nil || !strings.Contains(err.Error(), "unsupported for centralized chat") {
		t.Fatalf("validateAgentConfig(local verifier) error = %v, want unsupported", err)
	}
}

func TestChatExternalSessionRoutingUsesLatestReusableRun(t *testing.T) {
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
	createChatSessionFixture(t, queries, "review_session_external_session")
	now := "2026-05-03T00:00:00Z"
	if _, err := queries.CreateAgentConfig(ctx, dbgen.CreateAgentConfigParams{
		ID:               "agent_config_protocol",
		Name:             "Codex App Server",
		Role:             "reviewer",
		AdapterKind:      string(agents.AdapterJSONRPCStdio),
		Command:          sql.NullString{String: "codex", Valid: true},
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       "json",
		CapabilitiesJson: `{"supports_json":true,"supports_streaming":true,"supports_sessions":true,"can_read":true,"can_write":false,"output_modes":["json"]}`,
		SettingsJson:     "{}",
		Enabled:          1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("CreateAgentConfig() error = %v", err)
	}
	for _, run := range []struct {
		id      string
		started string
		thread  string
	}{
		{id: "agent_run_old", started: "2026-05-03T00:00:01Z", thread: "thread_old"},
		{id: "agent_run_new", started: "2026-05-03T00:00:02Z", thread: "thread_new"},
	} {
		metadata, err := json.Marshal(map[string]any{
			agents.ExternalSessionMetadataKey: map[string]any{
				"adapter_id": "agent_config_protocol",
				"protocol":   "codex_app_server",
				"thread_id":  run.thread,
				"source":     "thread/start",
			},
		})
		if err != nil {
			t.Fatalf("Marshal(metadata) error = %v", err)
		}
		if _, err := queries.CreateAgentRun(ctx, dbgen.CreateAgentRunParams{
			ID:              run.id,
			ReviewSessionID: "review_session_external_session",
			AgentConfigID:   "agent_config_protocol",
			Status:          agentrun.RunStatusSucceeded,
			Role:            "reviewer",
			StartedAt:       sql.NullString{String: run.started, Valid: true},
			CompletedAt:     sql.NullString{String: run.started, Valid: true},
			MetadataJson:    string(metadata),
		}); err != nil {
			t.Fatalf("CreateAgentRun(%s) error = %v", run.id, err)
		}
	}

	service := Service{
		Database: database,
		Queries:  queries,
		Now: func() time.Time {
			return time.Date(2026, 5, 3, 0, 0, 3, 0, time.UTC)
		},
	}
	metadata := map[string]any{}
	service.addReusableExternalSession(ctx, "review_session_external_session", "agent_config_protocol", agents.AgentCapabilities{
		SupportsSessions: true,
	}, chatFollowupStrategy{Mode: "resume_session", AllowSessionReuse: true}, metadata)
	session, ok := agents.ExtractExternalSessionMetadata("agent_config_protocol", metadata)
	if !ok || session.ThreadID != "thread_new" || session.Source != "agent_run:agent_run_new" {
		t.Fatalf("external session = %+v, ok = %v, metadata = %+v", session, ok, metadata)
	}

	compact := classifyChatFollowup(AskParams{
		ContextRefs: json.RawMessage(`[{"ref_type":"finding","ref_id":"finding_1"}]`),
	}, "Explain this finding")
	if compact.AllowSessionReuse || compact.Mode != "fresh_compact_finding" {
		t.Fatalf("compact strategy = %+v, want no session reuse", compact)
	}
	metadata = map[string]any{}
	service.addReusableExternalSession(ctx, "review_session_external_session", "agent_config_protocol", agents.AgentCapabilities{
		SupportsSessions: true,
	}, compact, metadata)
	if _, ok := metadata[agents.ExternalSessionMetadataKey]; ok {
		t.Fatalf("compact explain turn should not attach external session: %+v", metadata)
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

type recordingEventLog struct {
	events []dbgen.Event
}

func (r *recordingEventLog) Append(_ context.Context, params eventlog.AppendParams) (dbgen.Event, error) {
	event := dbgen.Event{
		ID:              params.ID,
		ReviewSessionID: sql.NullString{String: params.ReviewSessionID, Valid: params.ReviewSessionID != ""},
		AgentRunID:      params.AgentRunID,
		Type:            params.Type,
		Level:           params.Level,
		Sequence:        int64(len(r.events) + 1),
		PayloadJson:     params.PayloadJSON,
		ArtifactID:      params.ArtifactID,
		CreatedAt:       params.CreatedAt,
	}
	r.events = append(r.events, event)
	return event, nil
}

func (r *recordingEventLog) lastEventOfType(eventType string) dbgen.Event {
	for index := len(r.events) - 1; index >= 0; index-- {
		if r.events[index].Type == eventType {
			return r.events[index]
		}
	}
	return dbgen.Event{}
}

func (r *recordingEventLog) lastMessageEvent(eventType string, messageID string) dbgen.Event {
	for index := len(r.events) - 1; index >= 0; index-- {
		event := r.events[index]
		if event.Type != eventType {
			continue
		}
		var payload map[string]any
		if json.Unmarshal([]byte(event.PayloadJson), &payload) != nil {
			continue
		}
		if payload["message_id"] == messageID {
			return event
		}
	}
	return dbgen.Event{}
}

func decodeEventPayload(t *testing.T, raw string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode event payload %q: %v", raw, err)
	}
	return payload
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
