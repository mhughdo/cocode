package chat

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hughdo/cocode/services/cocoded/internal/agentoutput"
	"github.com/hughdo/cocode/services/cocoded/internal/agentrun"
	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/contextbundle"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/eventlog"
	"github.com/hughdo/cocode/services/cocoded/internal/reviewprompt"
)

const (
	AuthorUser         = "user"
	AuthorCocode       = "cocode"
	AuthorOrchestrator = "orchestrator"
	AuthorAgent        = "agent"
	AuthorSystem       = "system"
	AuthorVerifier     = "verifier"

	MessageStatusCompleted = "completed"
	MessageStatusFailed    = "failed"

	TurnStatusCreated      = "created"
	TurnStatusRouting      = "routing"
	TurnStatusContextBuild = "context_building"
	TurnStatusRunning      = "running"
	TurnStatusSynthesizing = "synthesizing"
	TurnStatusCompleted    = "completed"
	TurnStatusFailed       = "failed"
	TurnStatusCancelReq    = "cancel_requested"
	TurnStatusCanceled     = "canceled"

	AudienceOrchestrator = "orchestrator"
	AudienceAllAgents    = "all_agents"
	AudienceSelected     = "selected_agent"

	defaultThreadTitleBytes = 96
	defaultChatFanoutLimit  = 4
)

var (
	ErrServiceNotConfigured  = errors.New("chat service is not configured")
	ErrReviewSessionNotFound = errors.New("review session was not found")
	ErrThreadNotFound        = errors.New("chat thread was not found")
	ErrInvalidMessage        = errors.New("chat message is invalid")
	ErrInvalidTurn           = errors.New("chat turn is invalid")
	ErrAgentConfigNotFound   = errors.New("chat agent config was not found")
	ErrInvalidAgentConfig    = errors.New("chat agent config is invalid")
)

type EventLog interface {
	Append(ctx context.Context, params eventlog.AppendParams) (dbgen.Event, error)
}

type Service struct {
	Database       *sql.DB
	Queries        *dbgen.Queries
	ContextBuilder *contextbundle.Service
	Artifacts      *artifact.Store
	AgentManager   *agentrun.Manager
	Events         EventLog
	Now            func() time.Time
	NewID          func(prefix string) string
}

type Thread struct {
	ID              string `json:"id"`
	ReviewSessionID string `json:"review_session_id"`
	Title           string `json:"title"`
	Status          string `json:"status"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type Message struct {
	ID                string          `json:"id"`
	ThreadID          string          `json:"thread_id"`
	ParentMessageID   string          `json:"parent_message_id,omitempty"`
	AuthorType        string          `json:"author_type"`
	AuthorDisplayName string          `json:"author_display_name"`
	AgentConfigID     string          `json:"agent_config_id,omitempty"`
	AgentRunID        string          `json:"agent_run_id,omitempty"`
	ContextBundleID   string          `json:"context_bundle_id,omitempty"`
	ArtifactID        string          `json:"artifact_id,omitempty"`
	Body              string          `json:"body"`
	Status            string          `json:"status"`
	Metadata          json.RawMessage `json:"metadata"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
}

type Turn struct {
	ID                     string `json:"id"`
	ThreadID               string `json:"thread_id"`
	UserMessageID          string `json:"user_message_id"`
	Mode                   string `json:"mode"`
	Audience               string `json:"audience"`
	ResponderAgentConfigID string `json:"responder_agent_config_id,omitempty"`
	Status                 string `json:"status"`
	ErrorCode              string `json:"error_code,omitempty"`
	ErrorMessage           string `json:"error_message,omitempty"`
	StartedAt              string `json:"started_at,omitempty"`
	CompletedAt            string `json:"completed_at,omitempty"`
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
}

type ThreadView struct {
	Session  dbgen.ReviewSession `json:"-"`
	Thread   Thread              `json:"thread"`
	Messages []Message           `json:"messages"`
}

type AskParams struct {
	ReviewSessionID        string          `json:"review_session_id"`
	Body                   string          `json:"body"`
	Mode                   string          `json:"mode"`
	Audience               string          `json:"audience"`
	ResponderAgentConfigID string          `json:"responder_agent_config_id"`
	ContextRefs            json.RawMessage `json:"context_refs"`
	IncludeEvidence        bool            `json:"include_evidence"`
	IncludeRecentMessages  bool            `json:"include_recent_messages"`
}

type AskResult struct {
	Thread      Thread    `json:"thread"`
	Messages    []Message `json:"messages"`
	Turn        Turn      `json:"turn"`
	AgentRunIDs []string  `json:"agent_run_ids,omitempty"`
}

type appendMessageParams struct {
	ThreadID          string
	ParentMessageID   string
	AuthorType        string
	AuthorDisplayName string
	AgentConfigID     string
	AgentRunID        string
	ContextBundleID   string
	ArtifactID        string
	Body              string
	Status            string
	MetadataJSON      json.RawMessage
}

type runtimeSettings struct {
	PromptDelivery    agents.PromptDelivery `json:"prompt_delivery"`
	AllowRiskyCommand bool                  `json:"allow_risky_command"`
	TimeoutSeconds    int64                 `json:"timeout_seconds"`
	MaxStdoutBytes    int64                 `json:"max_stdout_bytes"`
	MaxStderrBytes    int64                 `json:"max_stderr_bytes"`
	MaxPromptBytes    int64                 `json:"max_prompt_bytes"`
}

type chatPromptContext struct {
	Bundle                contextbundle.Bundle
	Findings              []dbgen.Finding
	RecentMessages        []Message
	ContextRefs           json.RawMessage
	IncludeEvidence       bool
	IncludeRecentMessages bool
}

var allowedTurnTransitions = map[string]map[string]bool{
	TurnStatusCreated: {
		TurnStatusRouting:   true,
		TurnStatusCancelReq: true,
		TurnStatusFailed:    true,
	},
	TurnStatusRouting: {
		TurnStatusContextBuild: true,
		TurnStatusRunning:      true,
		TurnStatusSynthesizing: true,
		TurnStatusCancelReq:    true,
		TurnStatusCompleted:    true,
		TurnStatusFailed:       true,
		TurnStatusCanceled:     true,
	},
	TurnStatusContextBuild: {
		TurnStatusRunning:   true,
		TurnStatusCancelReq: true,
		TurnStatusCompleted: true,
		TurnStatusFailed:    true,
		TurnStatusCanceled:  true,
	},
	TurnStatusRunning: {
		TurnStatusSynthesizing: true,
		TurnStatusCancelReq:    true,
		TurnStatusCompleted:    true,
		TurnStatusFailed:       true,
		TurnStatusCanceled:     true,
	},
	TurnStatusSynthesizing: {
		TurnStatusCancelReq: true,
		TurnStatusCompleted: true,
		TurnStatusFailed:    true,
		TurnStatusCanceled:  true,
	},
	TurnStatusCancelReq: {
		TurnStatusCompleted: true,
		TurnStatusFailed:    true,
		TurnStatusCanceled:  true,
	},
}

func (s Service) EnsureSessionThread(ctx context.Context, reviewSessionID string) (ThreadView, error) {
	if s.Database == nil || s.Queries == nil {
		return ThreadView{}, ErrServiceNotConfigured
	}
	session, err := s.session(ctx, reviewSessionID)
	if err != nil {
		return ThreadView{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	thread, err := s.upsertThread(ctx, session, now)
	if err != nil {
		return ThreadView{}, err
	}
	messages, err := s.listMessages(ctx, thread.ID)
	if err != nil {
		return ThreadView{}, err
	}
	if len(messages) == 0 {
		if err := s.seedInitialMessages(ctx, session, thread); err != nil {
			return ThreadView{}, err
		}
		messages, err = s.listMessages(ctx, thread.ID)
		if err != nil {
			return ThreadView{}, err
		}
	}
	if err := s.syncReviewProgressMessages(ctx, session, thread); err != nil {
		return ThreadView{}, err
	}
	messages, err = s.listMessages(ctx, thread.ID)
	if err != nil {
		return ThreadView{}, err
	}
	return ThreadView{Session: session, Thread: thread, Messages: messages}, nil
}

func (s Service) LoadThread(ctx context.Context, threadID string) (ThreadView, error) {
	if s.Database == nil || s.Queries == nil {
		return ThreadView{}, ErrServiceNotConfigured
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ThreadView{}, fmt.Errorf("%w: thread id is required", ErrThreadNotFound)
	}
	thread, err := s.threadByID(ctx, threadID)
	if err != nil {
		return ThreadView{}, err
	}
	session, err := s.session(ctx, thread.ReviewSessionID)
	if err != nil {
		return ThreadView{}, err
	}
	if err := s.syncReviewProgressMessages(ctx, session, thread); err != nil {
		return ThreadView{}, err
	}
	messages, err := s.listMessages(ctx, thread.ID)
	if err != nil {
		return ThreadView{}, err
	}
	return ThreadView{Session: session, Thread: thread, Messages: messages}, nil
}

func (s Service) Ask(ctx context.Context, params AskParams) (AskResult, error) {
	result, err := s.CreateTurn(ctx, params)
	if err != nil {
		return AskResult{}, err
	}
	return s.runTurn(ctx, result.Turn.ID, params)
}

func (s Service) CreateTurn(ctx context.Context, params AskParams) (AskResult, error) {
	view, err := s.EnsureSessionThread(ctx, params.ReviewSessionID)
	if err != nil {
		return AskResult{}, err
	}
	body := strings.TrimSpace(params.Body)
	if body == "" {
		return AskResult{}, fmt.Errorf("%w: body is required", ErrInvalidMessage)
	}
	if !params.IncludeEvidence {
		params.IncludeEvidence = true
	}
	if !params.IncludeRecentMessages {
		params.IncludeRecentMessages = true
	}
	now := s.now().Format(time.RFC3339Nano)
	userMessage, err := s.appendMessage(ctx, appendMessageParams{
		ThreadID:          view.Thread.ID,
		AuthorType:        AuthorUser,
		AuthorDisplayName: "You",
		Body:              body,
		Status:            MessageStatusCompleted,
		MetadataJSON:      normalizedJSON(params.ContextRefs, "[]"),
	})
	if err != nil {
		return AskResult{}, err
	}
	turn, err := s.createTurn(ctx, view.Thread.ID, userMessage.ID, params, now)
	if err != nil {
		return AskResult{}, err
	}
	messages, err := s.listMessages(ctx, view.Thread.ID)
	if err != nil {
		return AskResult{}, err
	}
	audience := normalizeAudience(params.Audience)
	s.emit(ctx, view.Session.ID, "ChatTurnCreated", map[string]any{
		"thread_id":       view.Thread.ID,
		"chat_turn_id":    turn.ID,
		"user_message_id": userMessage.ID,
		"audience":        audience,
		"mode":            turn.Mode,
	})
	return AskResult{Thread: view.Thread, Messages: messages, Turn: turn}, nil
}

func (s Service) RunTurn(ctx context.Context, turnID string, params AskParams) {
	if _, err := s.runTurn(ctx, turnID, params); err != nil {
		log.Printf("chat turn %s failed: %v", turnID, err)
	}
}

func (s Service) CancelTurn(ctx context.Context, turnID string) (Turn, error) {
	turn, err := s.turnByID(ctx, strings.TrimSpace(turnID))
	if err != nil {
		return Turn{}, err
	}
	if turnStatusTerminal(turn.Status) {
		return turn, nil
	}
	if turn.Status != TurnStatusCancelReq {
		turn, err = s.updateTurn(ctx, turn, TurnStatusCancelReq, "", "")
		if err != nil {
			return Turn{}, err
		}
	}
	thread, err := s.threadByID(ctx, turn.ThreadID)
	if err != nil {
		return Turn{}, err
	}
	runIDs, err := s.cancelableRunIDsForTurn(ctx, thread.ReviewSessionID, turn)
	if err != nil {
		return Turn{}, err
	}
	for _, runID := range runIDs {
		if s.AgentManager == nil {
			continue
		}
		if err := s.AgentManager.Cancel(ctx, runID); err != nil && !errors.Is(err, agentrun.ErrRunNotActive) {
			return Turn{}, fmt.Errorf("cancel chat agent run %s: %w", runID, err)
		}
	}
	s.emit(ctx, thread.ReviewSessionID, "ChatTurnCancelRequested", map[string]any{
		"thread_id":       turn.ThreadID,
		"chat_turn_id":    turn.ID,
		"user_message_id": turn.UserMessageID,
		"agent_run_ids":   runIDs,
	})
	return turn, nil
}

func (s Service) runTurn(ctx context.Context, turnID string, params AskParams) (AskResult, error) {
	turn, err := s.turnByID(ctx, strings.TrimSpace(turnID))
	if err != nil {
		return AskResult{}, err
	}
	thread, err := s.threadByID(ctx, turn.ThreadID)
	if err != nil {
		return AskResult{}, err
	}
	session, err := s.session(ctx, thread.ReviewSessionID)
	if err != nil {
		return AskResult{}, err
	}
	userMessage, err := s.messageByID(ctx, turn.UserMessageID)
	if err != nil {
		return AskResult{}, err
	}
	params = paramsForTurn(params, turn, userMessage)
	body := strings.TrimSpace(userMessage.Body)
	if body == "" {
		return AskResult{}, fmt.Errorf("%w: user message is empty", ErrInvalidMessage)
	}
	if turnStatusTerminal(turn.Status) {
		messages, err := s.listMessages(ctx, thread.ID)
		if err != nil {
			return AskResult{}, err
		}
		return AskResult{Thread: thread, Messages: messages, Turn: turn}, nil
	}
	if turn.Status == TurnStatusCancelReq {
		return s.cancelTurnResult(ctx, session, thread, turn, normalizeAudience(params.Audience), nil)
	}
	turn, err = s.updateTurn(ctx, turn, TurnStatusRouting, "", "")
	if err != nil {
		return AskResult{}, err
	}

	agentRuns := []string{}
	audience := normalizeAudience(params.Audience)
	if s.turnCancelRequested(ctx, turn.ID) {
		return s.cancelTurnResult(ctx, session, thread, turn, audience, agentRuns)
	}
	switch audience {
	case AudienceSelected:
		turn, _ = s.updateTurn(ctx, turn, TurnStatusContextBuild, "", "")
		if s.turnCancelRequested(ctx, turn.ID) {
			return s.cancelTurnResult(ctx, session, thread, turn, audience, agentRuns)
		}
		agentRunID, runErr := s.answerWithAgent(ctx, session, thread, userMessage, params, body)
		if agentRunID != "" {
			agentRuns = append(agentRuns, agentRunID)
			_ = s.linkTurnAgentRun(ctx, turn.ID, agentRunID, "chat")
		}
		if runErr != nil {
			if s.turnCancelRequested(ctx, turn.ID) {
				return s.cancelTurnResult(ctx, session, thread, turn, audience, agentRuns)
			}
			turn, _ = s.updateTurn(ctx, turn, TurnStatusFailed, "agent_run_failed", runErr.Error())
			messages, _ := s.listMessages(ctx, thread.ID)
			s.emit(ctx, session.ID, "ChatTurnFailed", map[string]any{
				"thread_id":    thread.ID,
				"chat_turn_id": turn.ID,
				"audience":     audience,
				"error":        runErr.Error(),
			})
			return AskResult{Thread: thread, Messages: messages, Turn: turn, AgentRunIDs: agentRuns}, nil
		}
	case AudienceOrchestrator:
		if responderID := strings.TrimSpace(params.ResponderAgentConfigID); responderID != "" {
			if config, err := s.agentConfig(ctx, responderID); err == nil {
				turn, _ = s.updateTurn(ctx, turn, TurnStatusSynthesizing, "", "")
				if s.turnCancelRequested(ctx, turn.ID) {
					return s.cancelTurnResult(ctx, session, thread, turn, audience, agentRuns)
				}
				synthesisRunID, synthesisErr := s.answerWithSynthesisAgentConfig(ctx, session, thread, userMessage, config, params, body, nil, nil, nil)
				if synthesisRunID != "" {
					agentRuns = append(agentRuns, synthesisRunID)
					_ = s.linkTurnAgentRun(ctx, turn.ID, synthesisRunID, "chat")
				}
				if synthesisErr == nil {
					break
				}
				if s.turnCancelRequested(ctx, turn.ID) {
					return s.cancelTurnResult(ctx, session, thread, turn, audience, agentRuns)
				}
				findings, _ := s.Queries.ListFindingsBySession(ctx, session.ID)
				if _, err := s.appendMessage(ctx, appendMessageParams{
					ThreadID:          thread.ID,
					AuthorType:        AuthorOrchestrator,
					AuthorDisplayName: "Orchestrator",
					Body:              orchestratorSynthesisMessage(session, findings, nil, nil, body),
					Status:            MessageStatusCompleted,
					MetadataJSON:      agentSynthesisMetadata(agentRuns, nil),
				}); err != nil {
					turn, _ = s.updateTurn(ctx, turn, TurnStatusFailed, "orchestrator_answer_failed", err.Error())
					return AskResult{}, err
				}
				break
			}
		}
		turn, _ = s.updateTurn(ctx, turn, TurnStatusRunning, "", "")
		if s.turnCancelRequested(ctx, turn.ID) {
			return s.cancelTurnResult(ctx, session, thread, turn, audience, agentRuns)
		}
		if _, err := s.appendLocalAnswer(ctx, session, thread, body); err != nil {
			turn, _ = s.updateTurn(ctx, turn, TurnStatusFailed, "local_answer_failed", err.Error())
			return AskResult{}, err
		}
	case AudienceAllAgents:
		turn, _ = s.updateTurn(ctx, turn, TurnStatusContextBuild, "", "")
		if s.turnCancelRequested(ctx, turn.ID) {
			return s.cancelTurnResult(ctx, session, thread, turn, audience, agentRuns)
		}
		ids, runErr := s.answerWithAllAgents(ctx, session, thread, userMessage, params, body)
		agentRuns = append(agentRuns, ids...)
		for _, id := range ids {
			_ = s.linkTurnAgentRun(ctx, turn.ID, id, "chat")
		}
		if runErr != nil {
			if s.turnCancelRequested(ctx, turn.ID) {
				return s.cancelTurnResult(ctx, session, thread, turn, audience, agentRuns)
			}
			turn, _ = s.updateTurn(ctx, turn, TurnStatusFailed, "agent_run_failed", runErr.Error())
			messages, _ := s.listMessages(ctx, thread.ID)
			s.emit(ctx, session.ID, "ChatTurnFailed", map[string]any{
				"thread_id":    thread.ID,
				"chat_turn_id": turn.ID,
				"audience":     audience,
				"error":        runErr.Error(),
			})
			return AskResult{Thread: thread, Messages: messages, Turn: turn, AgentRunIDs: agentRuns}, nil
		}
	default:
		turn, _ = s.updateTurn(ctx, turn, TurnStatusRunning, "", "")
		if s.turnCancelRequested(ctx, turn.ID) {
			return s.cancelTurnResult(ctx, session, thread, turn, audience, agentRuns)
		}
		if _, err := s.appendLocalAnswer(ctx, session, thread, body); err != nil {
			turn, _ = s.updateTurn(ctx, turn, TurnStatusFailed, "local_answer_failed", err.Error())
			return AskResult{}, err
		}
	}

	if s.turnCancelRequested(ctx, turn.ID) {
		return s.cancelTurnResult(ctx, session, thread, turn, audience, agentRuns)
	}
	turn, err = s.updateTurn(ctx, turn, TurnStatusCompleted, "", "")
	if err != nil {
		return AskResult{}, err
	}
	messages, err := s.listMessages(ctx, thread.ID)
	if err != nil {
		return AskResult{}, err
	}
	s.emit(ctx, session.ID, "ChatTurnCompleted", map[string]any{
		"thread_id":     thread.ID,
		"chat_turn_id":  turn.ID,
		"message_count": len(messages),
		"audience":      audience,
	})
	return AskResult{Thread: thread, Messages: messages, Turn: turn, AgentRunIDs: agentRuns}, nil
}

func paramsForTurn(params AskParams, turn Turn, userMessage Message) AskParams {
	params.Mode = turn.Mode
	params.Audience = turn.Audience
	params.ResponderAgentConfigID = turn.ResponderAgentConfigID
	if len(strings.TrimSpace(string(params.ContextRefs))) == 0 {
		params.ContextRefs = normalizedJSON(userMessage.Metadata, "[]")
	}
	if !params.IncludeEvidence {
		params.IncludeEvidence = true
	}
	if !params.IncludeRecentMessages {
		params.IncludeRecentMessages = true
	}
	return params
}

func (s Service) seedInitialMessages(ctx context.Context, session dbgen.ReviewSession, thread Thread) error {
	request := strings.TrimSpace(nullableStringValue(session.FocusPrompt))
	if request == "" {
		request = fmt.Sprintf("Run a %s multi-agent review for %s.", session.ReviewDepth, session.Title)
	}
	if _, err := s.appendMessage(ctx, appendMessageParams{
		ThreadID:          thread.ID,
		AuthorType:        AuthorUser,
		AuthorDisplayName: "You",
		Body:              request,
		Status:            MessageStatusCompleted,
		MetadataJSON:      json.RawMessage(`{"seeded":true}`),
	}); err != nil {
		return err
	}
	agentCount, _ := s.enabledSessionAgentCount(ctx, session.ID)
	body := fmt.Sprintf(
		"Understood. I'll coordinate a %s review with %d configured reviewer%s, keep provenance for every answer, and surface verified findings in the Findings tab.",
		session.ReviewDepth,
		agentCount,
		plural(agentCount),
	)
	if _, err := s.appendMessage(ctx, appendMessageParams{
		ThreadID:          thread.ID,
		AuthorType:        AuthorOrchestrator,
		AuthorDisplayName: "Orchestrator",
		Body:              body,
		Status:            MessageStatusCompleted,
		MetadataJSON:      json.RawMessage(`{"seeded":true}`),
	}); err != nil {
		return err
	}
	_, err := s.appendMessage(ctx, appendMessageParams{
		ThreadID:          thread.ID,
		AuthorType:        AuthorSystem,
		AuthorDisplayName: "System",
		Body:              fmt.Sprintf("Review status is %s. Activity and agent outputs will appear here as they are persisted.", session.Status),
		Status:            MessageStatusCompleted,
		MetadataJSON:      json.RawMessage(`{"seeded":true}`),
	})
	return err
}

func (s Service) syncReviewProgressMessages(ctx context.Context, session dbgen.ReviewSession, thread Thread) error {
	if s.Queries == nil {
		return ErrServiceNotConfigured
	}
	messages, err := s.listMessages(ctx, thread.ID)
	if err != nil {
		return err
	}
	messages, err = s.removeHiddenReviewAgentRunMessages(ctx, messages)
	if err != nil {
		return err
	}
	seenEvents := map[string]bool{}
	seenRuns := map[string]bool{}
	hasFindingsDigest := false
	lastTerminalProgressMessages := map[string]string{}
	for _, message := range messages {
		metadata := messageMetadata(message.Metadata)
		if eventID, ok := metadata["progress_event_id"].(string); ok && eventID != "" {
			seenEvents[eventID] = true
			eventType, _ := metadata["progress_event_type"].(string)
			if terminalReviewProgressEvent(eventType) && message.ID == messages[len(messages)-1].ID {
				lastTerminalProgressMessages[eventID] = message.ID
			}
		}
		if runID, ok := metadata["review_agent_run_id"].(string); ok && runID != "" {
			seenRuns[runID] = true
		}
		if strings.TrimSpace(message.AgentRunID) != "" {
			seenRuns[message.AgentRunID] = true
		}
		if value, ok := metadata["review_findings_digest"].(bool); ok && value {
			hasFindingsDigest = true
		}
	}

	events, err := s.Queries.ListEventsByReviewSession(ctx, sql.NullString{String: session.ID, Valid: true})
	if err != nil {
		return fmt.Errorf("list review events for chat: %w", err)
	}
	appendProgress := func(event dbgen.Event, progress progressMessage) error {
		metadata, err := json.Marshal(map[string]any{
			"answer_source":             "review_progress",
			"progress_event_created_at": event.CreatedAt,
			"progress_event_id":         event.ID,
			"progress_event_sequence":   event.Sequence,
			"progress_event_type":       event.Type,
		})
		if err != nil {
			return fmt.Errorf("encode progress metadata: %w", err)
		}
		if _, err := s.appendMessage(ctx, appendMessageParams{
			ThreadID:          thread.ID,
			AuthorType:        progress.authorType,
			AuthorDisplayName: progress.displayName,
			Body:              progress.body,
			Status:            progress.status,
			MetadataJSON:      metadata,
		}); err != nil {
			return err
		}
		seenEvents[event.ID] = true
		return nil
	}
	terminalEvents := make([]dbgen.Event, 0, 1)
	terminalProgress := map[string]progressMessage{}
	for _, event := range events {
		if seenEvents[event.ID] {
			continue
		}
		progress, ok := reviewProgressMessage(event)
		if !ok {
			continue
		}
		if terminalReviewProgressEvent(event.Type) {
			terminalEvents = append(terminalEvents, event)
			terminalProgress[event.ID] = progress
			continue
		}
		if err := appendProgress(event, progress); err != nil {
			return err
		}
	}

	runs, err := s.Queries.ListAgentRunsBySession(ctx, session.ID)
	if err != nil {
		return fmt.Errorf("list agent runs for chat: %w", err)
	}
	for _, run := range runs {
		if seenRuns[run.ID] || shouldHideReviewAgentRunFromChat(run) || !terminalAgentRunStatus(run.Status) {
			continue
		}
		config, err := s.Queries.GetAgentConfig(ctx, run.AgentConfigID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return fmt.Errorf("read agent config for chat progress: %w", err)
		}
		body, status, answer := s.reviewAgentRunMessage(ctx, run, config)
		messageMetadata := map[string]any{
			"agent_status":                  run.Status,
			"answer_source":                 "review_agent_run",
			"review_agent_run_completed_at": nullableStringValue(run.CompletedAt),
			"review_agent_run_id":           run.ID,
			"review_agent_run_started_at":   nullableStringValue(run.StartedAt),
		}
		if modelLabel, reasoningLabel := agentRunModelLabels(run); modelLabel != "" || reasoningLabel != "" {
			if modelLabel != "" {
				messageMetadata["model_label"] = modelLabel
			}
			if reasoningLabel != "" {
				messageMetadata["reasoning_label"] = reasoningLabel
			}
		}
		if reasoning := strings.TrimSpace(answer.ReasoningSummary); reasoning != "" {
			messageMetadata["reasoning_summary"] = truncateEventPreview(reasoning)
			messageMetadata["reasoning_disclaimer"] = "Provider-returned reasoning or thinking summary, not private hidden chain-of-thought."
		}
		if strings.TrimSpace(answer.Content) != "" {
			messageMetadata["has_model_output"] = true
		}
		metadata, err := json.Marshal(messageMetadata)
		if err != nil {
			return fmt.Errorf("encode agent run metadata: %w", err)
		}
		if _, err := s.appendMessage(ctx, appendMessageParams{
			ThreadID:          thread.ID,
			AuthorType:        AuthorAgent,
			AuthorDisplayName: config.Name,
			AgentConfigID:     config.ID,
			AgentRunID:        run.ID,
			ContextBundleID:   nullableStringValue(run.ContextBundleID),
			ArtifactID:        nullableStringValue(run.StdoutArtifactID),
			Body:              body,
			Status:            status,
			MetadataJSON:      metadata,
		}); err != nil {
			return err
		}
		seenRuns[run.ID] = true
	}

	if !hasFindingsDigest {
		findings, err := s.Queries.ListFindingsBySession(ctx, session.ID)
		if err != nil {
			return fmt.Errorf("list findings for chat digest: %w", err)
		}
		if len(findings) > 0 {
			metadata, err := json.Marshal(map[string]any{
				"answer_source":          "review_findings",
				"review_findings_digest": true,
				"review_findings_count":  len(findings),
				"review_session_status":  session.Status,
			})
			if err != nil {
				return fmt.Errorf("encode findings digest metadata: %w", err)
			}
			digestMessage, err := s.appendMessage(ctx, appendMessageParams{
				ThreadID:          thread.ID,
				AuthorType:        AuthorCocode,
				AuthorDisplayName: "cocode",
				Body:              findingsDigestMessage(findings),
				Status:            MessageStatusCompleted,
				MetadataJSON:      metadata,
			})
			if err != nil {
				return err
			}
			for _, messageID := range lastTerminalProgressMessages {
				if err := s.moveMessageAfter(ctx, messageID, digestMessage.CreatedAt); err != nil {
					return err
				}
			}
		}
	}
	for _, event := range terminalEvents {
		if seenEvents[event.ID] {
			continue
		}
		if err := appendProgress(event, terminalProgress[event.ID]); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) appendLocalAnswer(ctx context.Context, session dbgen.ReviewSession, thread Thread, question string) (Message, error) {
	findings, _ := s.Queries.ListFindingsBySession(ctx, session.ID)
	events, _ := s.Queries.ListEventsByReviewSession(ctx, sql.NullString{String: session.ID, Valid: true})
	body := localAnswer(session, findings, events, question)
	return s.appendMessage(ctx, appendMessageParams{
		ThreadID:          thread.ID,
		AuthorType:        AuthorCocode,
		AuthorDisplayName: "cocode",
		Body:              body,
		Status:            MessageStatusCompleted,
		MetadataJSON:      json.RawMessage(`{"answer_source":"local_state"}`),
	})
}

func (s Service) answerWithAllAgents(ctx context.Context, session dbgen.ReviewSession, thread Thread, userMessage Message, params AskParams, question string) ([]string, error) {
	configs, err := s.sessionReviewerAgentConfigs(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		_, err := s.appendLocalAnswer(ctx, session, thread, question)
		return nil, err
	}
	sharedContext, err := s.buildSharedChatContext(ctx, session, configs)
	if err != nil {
		return nil, err
	}
	runIDs := make([]string, 0, len(configs))
	failures := []string{}
	type agentResult struct {
		runID string
		err   error
	}
	results := make([]agentResult, len(configs))
	limit := minPositive(defaultChatFanoutLimit, len(configs))
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for index, config := range configs {
		index, config := index, config
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[index].err = ctx.Err()
				return
			}
			runID, err := s.answerWithAgentConfigWithBundle(ctx, session, thread, userMessage, config, params, question, &sharedContext)
			results[index] = agentResult{runID: runID, err: err}
		}()
	}
	wg.Wait()
	for index, result := range results {
		if result.runID != "" {
			runIDs = append(runIDs, result.runID)
		}
		if result.err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", configs[index].Name, result.err))
		}
	}
	answers, _ := s.agentMessagesForRuns(ctx, thread.ID, runIDs)
	findings, _ := s.Queries.ListFindingsBySession(ctx, session.ID)
	if responderID := strings.TrimSpace(params.ResponderAgentConfigID); responderID != "" {
		config, err := s.agentConfig(ctx, responderID)
		if err == nil {
			synthesisRunID, synthesisErr := s.answerWithSynthesisAgentConfig(ctx, session, thread, userMessage, config, params, question, answers, failures, &sharedContext)
			if synthesisRunID != "" {
				runIDs = append(runIDs, synthesisRunID)
			}
			if synthesisErr == nil {
				return runIDs, nil
			}
			failures = append(failures, fmt.Sprintf("%s synthesis: %v", config.Name, synthesisErr))
		}
	}
	synthesis := orchestratorSynthesisMessage(session, findings, answers, failures, question)
	if _, err := s.appendMessage(ctx, appendMessageParams{
		ThreadID:          thread.ID,
		AuthorType:        AuthorOrchestrator,
		AuthorDisplayName: "Orchestrator",
		Body:              synthesis,
		Status:            MessageStatusCompleted,
		MetadataJSON:      agentSynthesisMetadata(runIDs, failures),
	}); err != nil {
		return runIDs, err
	}
	return runIDs, nil
}

func (s Service) buildSharedChatContext(ctx context.Context, session dbgen.ReviewSession, configs []dbgen.AgentConfig) (contextbundle.BuildReviewContextResult, error) {
	if s.ContextBuilder == nil {
		return contextbundle.BuildReviewContextResult{}, ErrServiceNotConfigured
	}
	return s.ContextBuilder.BuildReviewContext(ctx, contextbundle.BuildReviewContextParams{
		ReviewSessionID: session.ID,
		AgentConfigID:   sharedContextRecipientAgentConfigID(configs),
		PolicyOverride:  json.RawMessage(session.ContextPolicyJson),
		Persist:         true,
	})
}

func sharedContextRecipientAgentConfigID(configs []dbgen.AgentConfig) string {
	fallback := ""
	for _, config := range configs {
		if fallback == "" {
			fallback = strings.TrimSpace(config.ID)
		}
		capabilities, err := agentCapabilities(config)
		if err != nil {
			continue
		}
		visibility := agents.VisibilityForConfig(agents.ConnectionConfig{
			AdapterID: config.ID,
			Kind:      agents.AdapterKind(config.AdapterKind),
		}, capabilities)
		if visibility.IsExternal() {
			return strings.TrimSpace(config.ID)
		}
	}
	return fallback
}

func (s Service) answerWithAgent(ctx context.Context, session dbgen.ReviewSession, thread Thread, userMessage Message, params AskParams, question string) (string, error) {
	configID := strings.TrimSpace(params.ResponderAgentConfigID)
	if configID == "" {
		return "", fmt.Errorf("%w: responder_agent_config_id is required", ErrInvalidTurn)
	}
	config, err := s.agentConfig(ctx, configID)
	if err != nil {
		return "", err
	}
	return s.answerWithAgentConfig(ctx, session, thread, userMessage, config, params, question)
}

func (s Service) answerWithSynthesisAgentConfig(ctx context.Context, session dbgen.ReviewSession, thread Thread, userMessage Message, config dbgen.AgentConfig, params AskParams, question string, answers []Message, failures []string, sharedContext *contextbundle.BuildReviewContextResult) (string, error) {
	if s.ContextBuilder == nil || s.AgentManager == nil || s.Artifacts == nil {
		return "", ErrServiceNotConfigured
	}
	if err := validateAgentConfig(config); err != nil {
		return "", err
	}
	repository, err := s.Queries.GetRepository(ctx, session.RepositoryID)
	if err != nil {
		return "", fmt.Errorf("read repository: %w", err)
	}
	workspace, err := s.Queries.GetWorkspace(ctx, session.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("read workspace: %w", err)
	}
	built := contextbundle.BuildReviewContextResult{}
	if sharedContext != nil {
		built = *sharedContext
	} else {
		var err error
		built, err = s.ContextBuilder.BuildReviewContext(ctx, contextbundle.BuildReviewContextParams{
			ReviewSessionID: session.ID,
			AgentConfigID:   config.ID,
			PolicyOverride:  json.RawMessage(session.ContextPolicyJson),
			Persist:         true,
		})
		if err != nil {
			return "", fmt.Errorf("build synthesis context: %w", err)
		}
	}
	promptContext := chatPromptContext{
		Bundle:                built.Bundle,
		ContextRefs:           normalizedJSON(params.ContextRefs, "[]"),
		IncludeEvidence:       true,
		IncludeRecentMessages: true,
	}
	promptContext.Findings, _ = s.Queries.ListFindingsBySession(ctx, session.ID)
	promptContext.Findings = userVisibleChatFindings(promptContext.Findings)
	promptContext.RecentMessages, _ = s.recentThreadMessages(ctx, thread.ID, 16)
	connection, limits, err := connectionConfig(config, repository, workspace)
	if err != nil {
		return "", err
	}
	capabilities, err := agentCapabilities(config)
	if err != nil {
		return "", err
	}
	task := agents.AgentTask{
		ID:               s.newID("agent_task_"),
		RunID:            s.newID("agent_run_"),
		ReviewSessionID:  session.ID,
		AgentConfigID:    config.ID,
		ContextBundleID:  built.Bundle.ID,
		Role:             "chat_synthesis",
		Prompt:           orchestratorAgentSynthesisPrompt(session, thread, userMessage, config, promptContext, answers, failures, question),
		ContextArtifacts: s.contextArtifactRefs(ctx, built.Bundle),
		RepositoryRoot:   repository.LocalPath,
		WorkspaceRoot:    workspace.RootPath,
		Limits:           limits,
		Metadata: map[string]any{
			"thread_id":         thread.ID,
			"user_message_id":   userMessage.ID,
			"context_bundle_id": built.Bundle.ID,
			"reviewer_count":    len(answers),
			"failure_count":     len(failures),
		},
	}
	result, err := s.AgentManager.Execute(ctx, agentrun.RunParams{
		WorkspaceID:  workspace.ID,
		Config:       connection,
		Capabilities: capabilities,
		Permissions:  agents.ReviewModePermissionPolicy(),
		Task:         task,
		TimeoutPolicy: agentrun.TimeoutPolicy{
			AgentTimeoutSeconds:  limits.TimeoutSeconds,
			ReviewTimeoutSeconds: maxInt64(0, session.RuntimeLimitSeconds),
		},
		Metadata: map[string]any{
			"phase":             "chat_synthesis",
			"thread_id":         thread.ID,
			"user_message_id":   userMessage.ID,
			"context_bundle_id": built.Bundle.ID,
			"output_mode":       config.OutputMode,
		},
		EventSink: s.agentRunEventSink(session.ID),
	})
	if err != nil {
		s.appendAgentFailure(ctx, thread, config, result.Run, built.Bundle.ID, err)
		return result.Run.ID, err
	}
	if result.Run.Status != agentrun.RunStatusSucceeded {
		runErr := fmt.Errorf("agent run %s finished with status %s", result.Run.ID, result.Run.Status)
		s.appendAgentFailure(ctx, thread, config, result.Run, built.Bundle.ID, runErr)
		return result.Run.ID, runErr
	}
	answer := s.answerFromRun(ctx, result.Run)
	if strings.TrimSpace(answer.Content) == "" {
		answer.Content = orchestratorSynthesisMessage(session, promptContext.Findings, answers, failures, question)
	}
	_, err = s.appendMessage(ctx, appendMessageParams{
		ThreadID:          thread.ID,
		AuthorType:        AuthorOrchestrator,
		AuthorDisplayName: "Orchestrator",
		AgentConfigID:     config.ID,
		AgentRunID:        result.Run.ID,
		ContextBundleID:   built.Bundle.ID,
		ArtifactID:        nullableStringValue(result.Run.StdoutArtifactID),
		Body:              answer.Content,
		Status:            MessageStatusCompleted,
		MetadataJSON:      agentSynthesisRunMetadata(answer, answers, failures),
	})
	return result.Run.ID, err
}

func (s Service) answerWithAgentConfig(ctx context.Context, session dbgen.ReviewSession, thread Thread, userMessage Message, config dbgen.AgentConfig, params AskParams, question string) (string, error) {
	return s.answerWithAgentConfigWithBundle(ctx, session, thread, userMessage, config, params, question, nil)
}

func (s Service) answerWithAgentConfigWithBundle(ctx context.Context, session dbgen.ReviewSession, thread Thread, userMessage Message, config dbgen.AgentConfig, params AskParams, question string, sharedContext *contextbundle.BuildReviewContextResult) (string, error) {
	if s.ContextBuilder == nil || s.AgentManager == nil || s.Artifacts == nil {
		return "", ErrServiceNotConfigured
	}
	if err := validateAgentConfig(config); err != nil {
		return "", err
	}
	repository, err := s.Queries.GetRepository(ctx, session.RepositoryID)
	if err != nil {
		return "", fmt.Errorf("read repository: %w", err)
	}
	workspace, err := s.Queries.GetWorkspace(ctx, session.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("read workspace: %w", err)
	}
	built := contextbundle.BuildReviewContextResult{}
	if sharedContext != nil {
		built = *sharedContext
	} else {
		var err error
		built, err = s.ContextBuilder.BuildReviewContext(ctx, contextbundle.BuildReviewContextParams{
			ReviewSessionID: session.ID,
			AgentConfigID:   config.ID,
			PolicyOverride:  json.RawMessage(session.ContextPolicyJson),
			Persist:         true,
		})
		if err != nil {
			return "", fmt.Errorf("build chat context: %w", err)
		}
	}
	promptContext := chatPromptContext{
		Bundle:                built.Bundle,
		ContextRefs:           normalizedJSON(params.ContextRefs, "[]"),
		IncludeEvidence:       params.IncludeEvidence,
		IncludeRecentMessages: params.IncludeRecentMessages,
	}
	promptContext.Findings, _ = s.Queries.ListFindingsBySession(ctx, session.ID)
	promptContext.Findings = userVisibleChatFindings(promptContext.Findings)
	if params.IncludeRecentMessages {
		promptContext.RecentMessages, _ = s.recentThreadMessages(ctx, thread.ID, 12)
	}
	connection, limits, err := connectionConfig(config, repository, workspace)
	if err != nil {
		return "", err
	}
	capabilities, err := agentCapabilities(config)
	if err != nil {
		return "", err
	}
	task := agents.AgentTask{
		ID:               s.newID("agent_task_"),
		RunID:            s.newID("agent_run_"),
		ReviewSessionID:  session.ID,
		AgentConfigID:    config.ID,
		ContextBundleID:  built.Bundle.ID,
		Role:             "chat",
		Prompt:           chatPrompt(session, thread, userMessage, config, promptContext, question),
		ContextArtifacts: s.contextArtifactRefs(ctx, built.Bundle),
		RepositoryRoot:   repository.LocalPath,
		WorkspaceRoot:    workspace.RootPath,
		Limits:           limits,
		Metadata: map[string]any{
			"thread_id":         thread.ID,
			"user_message_id":   userMessage.ID,
			"context_bundle_id": built.Bundle.ID,
		},
	}
	result, err := s.AgentManager.Execute(ctx, agentrun.RunParams{
		WorkspaceID:  workspace.ID,
		Config:       connection,
		Capabilities: capabilities,
		Permissions:  agents.ReviewModePermissionPolicy(),
		Task:         task,
		TimeoutPolicy: agentrun.TimeoutPolicy{
			AgentTimeoutSeconds:  limits.TimeoutSeconds,
			ReviewTimeoutSeconds: maxInt64(0, session.RuntimeLimitSeconds),
		},
		Metadata: map[string]any{
			"phase":             "chat_turn",
			"thread_id":         thread.ID,
			"user_message_id":   userMessage.ID,
			"context_bundle_id": built.Bundle.ID,
			"output_mode":       config.OutputMode,
		},
		EventSink: s.agentRunEventSink(session.ID),
	})
	if err != nil {
		s.appendAgentFailure(ctx, thread, config, result.Run, built.Bundle.ID, err)
		return result.Run.ID, err
	}
	if result.Run.Status != agentrun.RunStatusSucceeded {
		runErr := fmt.Errorf("agent run %s finished with status %s", result.Run.ID, result.Run.Status)
		s.appendAgentFailure(ctx, thread, config, result.Run, built.Bundle.ID, runErr)
		return result.Run.ID, runErr
	}
	answer := s.answerFromRun(ctx, result.Run)
	if strings.TrimSpace(answer.Content) == "" {
		answer.Content = "The agent completed but did not return text."
	}
	_, err = s.appendMessage(ctx, appendMessageParams{
		ThreadID:          thread.ID,
		AuthorType:        AuthorAgent,
		AuthorDisplayName: config.Name,
		AgentConfigID:     config.ID,
		AgentRunID:        result.Run.ID,
		ContextBundleID:   built.Bundle.ID,
		ArtifactID:        nullableStringValue(result.Run.StdoutArtifactID),
		Body:              answer.Content,
		Status:            MessageStatusCompleted,
		MetadataJSON:      agentAnswerMetadata(answer),
	})
	return result.Run.ID, err
}

func (s Service) appendAgentFailure(ctx context.Context, thread Thread, config dbgen.AgentConfig, run dbgen.AgentRun, contextBundleID string, err error) {
	body, artifactID := s.agentFailureMessage(ctx, run, config, err)
	_, _ = s.appendMessage(ctx, appendMessageParams{
		ThreadID:          thread.ID,
		AuthorType:        AuthorAgent,
		AuthorDisplayName: config.Name,
		AgentConfigID:     config.ID,
		AgentRunID:        run.ID,
		ContextBundleID:   contextBundleID,
		ArtifactID:        artifactID,
		Body:              body,
		Status:            MessageStatusFailed,
		MetadataJSON:      json.RawMessage(`{"answer_source":"agent","failed":true}`),
	})
}

func (s Service) appendMessage(ctx context.Context, params appendMessageParams) (Message, error) {
	if s.Database == nil {
		return Message{}, ErrServiceNotConfigured
	}
	threadID := strings.TrimSpace(params.ThreadID)
	if threadID == "" {
		return Message{}, fmt.Errorf("%w: thread id is required", ErrInvalidMessage)
	}
	authorType, err := normalizeAuthor(params.AuthorType)
	if err != nil {
		return Message{}, err
	}
	displayName := strings.TrimSpace(params.AuthorDisplayName)
	if displayName == "" {
		displayName = defaultDisplayName(authorType)
	}
	body := strings.TrimSpace(params.Body)
	if body == "" {
		return Message{}, fmt.Errorf("%w: body is required", ErrInvalidMessage)
	}
	status := strings.TrimSpace(params.Status)
	if status == "" {
		status = MessageStatusCompleted
	}
	metadata := normalizedJSON(params.MetadataJSON, "{}")
	now := s.now().Format(time.RFC3339Nano)
	id := s.newID("chat_message_")
	_, err = s.Database.ExecContext(ctx, `
INSERT INTO chat_messages (
  id, thread_id, parent_message_id, author_type, author_display_name,
  agent_config_id, agent_run_id, context_bundle_id, artifact_id,
  body, status, metadata_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id,
		threadID,
		nullableString(params.ParentMessageID),
		authorType,
		displayName,
		nullableString(params.AgentConfigID),
		nullableString(params.AgentRunID),
		nullableString(params.ContextBundleID),
		nullableString(params.ArtifactID),
		body,
		status,
		string(metadata),
		now,
		now,
	)
	if err != nil {
		return Message{}, fmt.Errorf("create chat message: %w", err)
	}
	if _, err := s.Database.ExecContext(ctx, "UPDATE chat_threads SET updated_at = ? WHERE id = ?", now, threadID); err != nil {
		return Message{}, fmt.Errorf("touch chat thread: %w", err)
	}
	message, err := s.messageByID(ctx, id)
	if err != nil {
		return Message{}, err
	}
	s.emit(ctx, "", "ChatMessageCreated", map[string]any{
		"thread_id":  threadID,
		"message_id": message.ID,
		"author":     message.AuthorType,
	})
	return message, nil
}

func (s Service) createTurn(ctx context.Context, threadID string, userMessageID string, params AskParams, now string) (Turn, error) {
	id := s.newID("chat_turn_")
	mode := strings.TrimSpace(params.Mode)
	if mode == "" {
		mode = "follow_up"
	}
	audience := normalizeAudience(params.Audience)
	_, err := s.Database.ExecContext(ctx, `
INSERT INTO chat_turns (
  id, thread_id, user_message_id, mode, audience, responder_agent_config_id,
  status, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id,
		threadID,
		userMessageID,
		mode,
		audience,
		nullableString(params.ResponderAgentConfigID),
		TurnStatusCreated,
		now,
		now,
	)
	if err != nil {
		return Turn{}, fmt.Errorf("create chat turn: %w", err)
	}
	return s.turnByID(ctx, id)
}

func (s Service) updateTurn(ctx context.Context, turn Turn, status string, code string, message string) (Turn, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		return Turn{}, fmt.Errorf("%w: turn status is required", ErrInvalidTurn)
	}
	if !validTurnTransition(turn.Status, status) {
		return Turn{}, fmt.Errorf("%w: cannot transition chat turn from %s to %s", ErrInvalidTurn, turn.Status, status)
	}
	now := s.now().Format(time.RFC3339Nano)
	startedAt := nullableString(turn.StartedAt)
	completedAt := nullableString(turn.CompletedAt)
	if turn.StartedAt == "" && turnStatusStarted(status) {
		startedAt = nullableString(now)
	}
	if turnStatusTerminal(status) {
		completedAt = nullableString(now)
	}
	_, err := s.Database.ExecContext(ctx, `
UPDATE chat_turns
SET status = ?, error_code = ?, error_message = ?, started_at = ?, completed_at = ?, updated_at = ?
WHERE id = ?`,
		status,
		nullableString(code),
		nullableString(message),
		startedAt,
		completedAt,
		now,
		turn.ID,
	)
	if err != nil {
		return Turn{}, fmt.Errorf("update chat turn: %w", err)
	}
	return s.turnByID(ctx, turn.ID)
}

func validTurnTransition(from string, to string) bool {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" {
		return false
	}
	if from == to {
		return true
	}
	if turnStatusTerminal(from) {
		return false
	}
	return allowedTurnTransitions[from][to]
}

func turnStatusStarted(status string) bool {
	return strings.TrimSpace(status) != TurnStatusCreated
}

func turnStatusTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case TurnStatusCompleted, TurnStatusFailed, TurnStatusCanceled:
		return true
	default:
		return false
	}
}

func (s Service) upsertThread(ctx context.Context, session dbgen.ReviewSession, now string) (Thread, error) {
	id := s.newID("chat_thread_")
	title := defaultThreadTitle(session)
	_, err := s.Database.ExecContext(ctx, `
INSERT INTO chat_threads (id, review_session_id, title, status, created_at, updated_at)
VALUES (?, ?, ?, 'active', ?, ?)
ON CONFLICT(review_session_id) DO UPDATE SET updated_at = chat_threads.updated_at`,
		id,
		session.ID,
		title,
		now,
		now,
	)
	if err != nil {
		return Thread{}, fmt.Errorf("upsert chat thread: %w", err)
	}
	return s.threadBySessionID(ctx, session.ID)
}

func (s Service) threadBySessionID(ctx context.Context, sessionID string) (Thread, error) {
	row := s.Database.QueryRowContext(ctx, `
SELECT id, review_session_id, title, status, created_at, updated_at
FROM chat_threads
WHERE review_session_id = ?
LIMIT 1`, sessionID)
	return scanThread(row)
}

func (s Service) threadByID(ctx context.Context, threadID string) (Thread, error) {
	row := s.Database.QueryRowContext(ctx, `
SELECT id, review_session_id, title, status, created_at, updated_at
FROM chat_threads
WHERE id = ?
LIMIT 1`, threadID)
	return scanThread(row)
}

func (s Service) listMessages(ctx context.Context, threadID string) ([]Message, error) {
	rows, err := s.Database.QueryContext(ctx, `
SELECT id, thread_id, parent_message_id, author_type, author_display_name,
  agent_config_id, agent_run_id, context_bundle_id, artifact_id,
  body, status, metadata_json, created_at, updated_at
FROM chat_messages
WHERE thread_id = ?
ORDER BY created_at ASC, rowid ASC`, threadID)
	if err != nil {
		return nil, fmt.Errorf("list chat messages: %w", err)
	}
	defer rows.Close()
	messages := []Message{}
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat messages: %w", err)
	}
	return messages, nil
}

func (s Service) recentThreadMessages(ctx context.Context, threadID string, limit int) ([]Message, error) {
	messages, err := s.listMessages(ctx, threadID)
	if err != nil {
		return nil, err
	}
	return promptVisibleChatMessages(messages, limit), nil
}

func promptVisibleChatMessages(messages []Message, limit int) []Message {
	filtered := make([]Message, 0, len(messages))
	for _, message := range messages {
		if promptVisibleChatMessage(message) {
			filtered = append(filtered, message)
		}
	}
	if limit <= 0 || len(filtered) <= limit {
		return filtered
	}
	return filtered[len(filtered)-limit:]
}

func promptVisibleChatMessage(message Message) bool {
	if message.Status != MessageStatusCompleted {
		return false
	}
	switch message.AuthorType {
	case AuthorUser, AuthorCocode, AuthorOrchestrator, AuthorAgent, AuthorVerifier:
	default:
		return false
	}
	metadata := messageMetadata(message.Metadata)
	if source, _ := metadata["answer_source"].(string); source == "review_progress" {
		return false
	}
	if failed, _ := metadata["failed"].(bool); failed {
		return false
	}
	return !transientSystemDiagnosticText(message.Body)
}

func transientSystemDiagnosticText(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	for _, marker := range []string{
		"context canceled",
		"context cancelled",
		"context deadline exceeded",
		"operation was canceled",
		"operation was cancelled",
		"request canceled",
		"request cancelled",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func (s Service) agentMessagesForRuns(ctx context.Context, threadID string, runIDs []string) ([]Message, error) {
	if len(runIDs) == 0 {
		return nil, nil
	}
	messages, err := s.listMessages(ctx, threadID)
	if err != nil {
		return nil, err
	}
	runSet := make(map[string]struct{}, len(runIDs))
	for _, runID := range runIDs {
		if strings.TrimSpace(runID) != "" {
			runSet[runID] = struct{}{}
		}
	}
	matches := make([]Message, 0, len(runSet))
	for _, message := range messages {
		if _, ok := runSet[message.AgentRunID]; ok && promptVisibleChatMessage(message) {
			matches = append(matches, message)
		}
	}
	return matches, nil
}

func (s Service) messageByID(ctx context.Context, id string) (Message, error) {
	row := s.Database.QueryRowContext(ctx, `
	SELECT id, thread_id, parent_message_id, author_type, author_display_name,
	  agent_config_id, agent_run_id, context_bundle_id, artifact_id,
	  body, status, metadata_json, created_at, updated_at
FROM chat_messages
WHERE id = ?
	LIMIT 1`, id)
	return scanMessage(row)
}

func (s Service) moveMessageAfter(ctx context.Context, id string, after string) error {
	timestamp := timestampAfter(after)
	if _, err := s.Database.ExecContext(ctx, `
UPDATE chat_messages
SET created_at = ?, updated_at = ?
WHERE id = ?`, timestamp, timestamp, id); err != nil {
		return fmt.Errorf("move chat message: %w", err)
	}
	return nil
}

func (s Service) turnByID(ctx context.Context, id string) (Turn, error) {
	row := s.Database.QueryRowContext(ctx, `
	SELECT id, thread_id, user_message_id, mode, audience, responder_agent_config_id,
  status, error_code, error_message, started_at, completed_at, created_at, updated_at
FROM chat_turns
WHERE id = ?
LIMIT 1`, id)
	return scanTurn(row)
}

func (s Service) linkTurnAgentRun(ctx context.Context, turnID string, agentRunID string, role string) error {
	if strings.TrimSpace(agentRunID) == "" {
		return nil
	}
	_, err := s.Database.ExecContext(ctx, `
INSERT OR IGNORE INTO chat_turn_agent_runs (chat_turn_id, agent_run_id, role)
VALUES (?, ?, ?)`, turnID, agentRunID, role)
	return err
}

func (s Service) cancelableRunIDsForTurn(ctx context.Context, reviewSessionID string, turn Turn) ([]string, error) {
	seen := map[string]bool{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id != "" {
			seen[id] = true
		}
	}
	rows, err := s.Database.QueryContext(ctx, `
SELECT agent_run_id
FROM chat_turn_agent_runs
WHERE chat_turn_id = ?`, turn.ID)
	if err != nil {
		return nil, fmt.Errorf("list linked chat agent runs: %w", err)
	}
	for rows.Next() {
		var runID string
		if err := rows.Scan(&runID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan linked chat agent run: %w", err)
		}
		add(runID)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close linked chat agent runs: %w", err)
	}
	runs, err := s.Queries.ListAgentRunsBySession(ctx, reviewSessionID)
	if err != nil {
		return nil, fmt.Errorf("list session agent runs for chat cancel: %w", err)
	}
	for _, run := range runs {
		if run.Status != agentrun.RunStatusRunning {
			continue
		}
		role := strings.TrimSpace(strings.ToLower(run.Role))
		if role != "chat" && role != "chat_synthesis" {
			continue
		}
		metadata := eventPayload(run.MetadataJson)
		if stringValue(metadata["thread_id"]) == turn.ThreadID &&
			stringValue(metadata["user_message_id"]) == turn.UserMessageID {
			add(run.ID)
		}
	}
	return sortedMapKeys(seen), nil
}

func (s Service) turnCancelRequested(ctx context.Context, turnID string) bool {
	turn, err := s.turnByID(ctx, turnID)
	return err == nil && turn.Status == TurnStatusCancelReq
}

func (s Service) cancelTurnResult(ctx context.Context, session dbgen.ReviewSession, thread Thread, turn Turn, audience string, agentRuns []string) (AskResult, error) {
	latest, err := s.turnByID(ctx, turn.ID)
	if err == nil {
		turn = latest
	}
	if !turnStatusTerminal(turn.Status) {
		if turn.Status != TurnStatusCancelReq {
			turn, err = s.updateTurn(ctx, turn, TurnStatusCancelReq, "", "")
			if err != nil {
				return AskResult{}, err
			}
		}
		turn, err = s.updateTurn(ctx, turn, TurnStatusCanceled, "", "")
		if err != nil {
			return AskResult{}, err
		}
	}
	messages, err := s.listMessages(ctx, thread.ID)
	if err != nil {
		return AskResult{}, err
	}
	s.emit(ctx, session.ID, "ChatTurnCanceled", map[string]any{
		"thread_id":     thread.ID,
		"chat_turn_id":  turn.ID,
		"message_count": len(messages),
		"audience":      audience,
	})
	return AskResult{Thread: thread, Messages: messages, Turn: turn, AgentRunIDs: agentRuns}, nil
}

func (s Service) session(ctx context.Context, reviewSessionID string) (dbgen.ReviewSession, error) {
	reviewSessionID = strings.TrimSpace(reviewSessionID)
	if reviewSessionID == "" {
		return dbgen.ReviewSession{}, fmt.Errorf("%w: review session id is required", ErrReviewSessionNotFound)
	}
	session, err := s.Queries.GetReviewSession(ctx, reviewSessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.ReviewSession{}, ErrReviewSessionNotFound
		}
		return dbgen.ReviewSession{}, fmt.Errorf("read review session: %w", err)
	}
	return session, nil
}

func (s Service) sessionAgentConfigs(ctx context.Context, reviewSessionID string) ([]dbgen.AgentConfig, error) {
	assignments, err := s.Queries.ListReviewSessionAgents(ctx, reviewSessionID)
	if err != nil {
		return nil, fmt.Errorf("list session agents: %w", err)
	}
	configs := make([]dbgen.AgentConfig, 0, len(assignments))
	for _, assignment := range assignments {
		if assignment.Enabled == 0 {
			continue
		}
		config, err := s.Queries.GetAgentConfig(ctx, assignment.AgentConfigID)
		if err != nil {
			continue
		}
		if config.Enabled == 0 {
			continue
		}
		configs = append(configs, config)
	}
	return configs, nil
}

func (s Service) sessionReviewerAgentConfigs(ctx context.Context, reviewSessionID string) ([]dbgen.AgentConfig, error) {
	assignments, err := s.Queries.ListReviewSessionAgents(ctx, reviewSessionID)
	if err != nil {
		return nil, fmt.Errorf("list session agents: %w", err)
	}
	configs := make([]dbgen.AgentConfig, 0, len(assignments))
	for _, assignment := range assignments {
		if assignment.Enabled == 0 {
			continue
		}
		assignmentRole := strings.ToLower(strings.TrimSpace(assignment.Role))
		if assignmentRole == AuthorOrchestrator || strings.Contains(assignmentRole, "orchestrator") {
			continue
		}
		config, err := s.Queries.GetAgentConfig(ctx, assignment.AgentConfigID)
		if err != nil {
			continue
		}
		if config.Enabled == 0 {
			continue
		}
		configs = append(configs, config)
	}
	return configs, nil
}

func (s Service) enabledSessionAgentCount(ctx context.Context, reviewSessionID string) (int, error) {
	configs, err := s.sessionReviewerAgentConfigs(ctx, reviewSessionID)
	return len(configs), err
}

func (s Service) agentConfig(ctx context.Context, id string) (dbgen.AgentConfig, error) {
	config, err := s.Queries.GetAgentConfig(ctx, strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.AgentConfig{}, ErrAgentConfigNotFound
		}
		return dbgen.AgentConfig{}, fmt.Errorf("read agent config: %w", err)
	}
	if config.Enabled == 0 {
		return dbgen.AgentConfig{}, fmt.Errorf("%w: config is disabled", ErrInvalidAgentConfig)
	}
	return config, nil
}

func (s Service) contextArtifactRefs(ctx context.Context, bundle contextbundle.Bundle) []agents.ArtifactRef {
	if s.Queries == nil || bundle.ArtifactID == "" {
		return nil
	}
	row, err := s.Queries.GetArtifact(ctx, bundle.ArtifactID)
	if err != nil {
		return nil
	}
	return []agents.ArtifactRef{{
		ID:           row.ID,
		Kind:         row.Kind,
		RelativePath: row.RelativePath,
		ContentType:  row.ContentType,
		SizeBytes:    row.SizeBytes,
		SHA256:       nullableStringValue(row.Sha256),
	}}
}

func (s Service) answerFromRun(ctx context.Context, run dbgen.AgentRun) agentoutput.Answer {
	if s.Artifacts == nil || !run.StdoutArtifactID.Valid {
		return agentoutput.Answer{EvidenceRefs: json.RawMessage("[]")}
	}
	content, _, err := s.Artifacts.Read(ctx, run.StdoutArtifactID.String)
	if err != nil {
		return agentoutput.Answer{EvidenceRefs: json.RawMessage("[]")}
	}
	raw := strings.TrimSpace(string(content))
	if raw == "" {
		return agentoutput.Answer{EvidenceRefs: json.RawMessage("[]")}
	}
	parsed := agentoutput.ParseAuto(content)
	answer := agentoutput.ExtractAnswer(parsed)
	if strings.TrimSpace(answer.Content) != "" {
		answer.Content = strings.TrimSpace(answer.Content)
		return answer
	}
	if parsed.Structured {
		return answer
	}
	answer.Content = raw
	return answer
}

func agentAnswerMetadata(answer agentoutput.Answer) json.RawMessage {
	metadata := map[string]any{"answer_source": "agent"}
	if reasoning := strings.TrimSpace(answer.ReasoningSummary); reasoning != "" {
		metadata["reasoning_summary"] = truncateEventPreview(reasoning)
		metadata["reasoning_disclaimer"] = "Provider-returned reasoning or thinking summary, not private hidden chain-of-thought."
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return json.RawMessage(`{"answer_source":"agent"}`)
	}
	return encoded
}

func agentSynthesisMetadata(runIDs []string, failures []string) json.RawMessage {
	metadata := map[string]any{
		"answer_source":      "agent_synthesis",
		"agent_run_ids":      runIDs,
		"failed_agent_count": len(failures),
	}
	if len(failures) > 0 {
		metadata["failures"] = failures
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return json.RawMessage(`{"answer_source":"agent_synthesis"}`)
	}
	return encoded
}

func agentSynthesisRunMetadata(answer agentoutput.Answer, answers []Message, failures []string) json.RawMessage {
	metadata := map[string]any{
		"answer_source":        "agent_synthesis",
		"answer_source_detail": "orchestrator_agent",
		"reviewer_answer_ids":  messageIDs(answers),
		"reviewer_count":       len(answers),
		"failed_agent_count":   len(failures),
	}
	if len(failures) > 0 {
		metadata["failures"] = failures
	}
	if reasoning := strings.TrimSpace(answer.ReasoningSummary); reasoning != "" {
		metadata["reasoning_summary"] = truncateEventPreview(reasoning)
		metadata["reasoning_disclaimer"] = "Provider-returned reasoning or thinking summary, not private hidden chain-of-thought."
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return json.RawMessage(`{"answer_source":"agent_synthesis"}`)
	}
	return encoded
}

func messageIDs(messages []Message) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		if strings.TrimSpace(message.ID) != "" {
			ids = append(ids, message.ID)
		}
	}
	return ids
}

func orchestratorSynthesisMessage(session dbgen.ReviewSession, findings []dbgen.Finding, answers []Message, failures []string, question string) string {
	var builder strings.Builder
	builder.WriteString("## Orchestrator synthesis\n\n")
	if len(answers) == 0 && len(failures) == 0 {
		builder.WriteString("I answered directly from the current review state and recent centralized chat.")
	} else {
		builder.WriteString(fmt.Sprintf("I asked %d reviewer%s and reviewed %d persisted answer%s before responding.", len(answers)+len(failures), plural(len(answers)+len(failures)), len(answers), plural(len(answers))))
		if len(failures) > 0 {
			builder.WriteString(fmt.Sprintf(" %d reviewer%s failed, so I treated those as gaps instead of blocking the turn.", len(failures), plural(len(failures))))
		}
	}
	builder.WriteString("\n\n")

	if len(findings) > 0 {
		builder.WriteString("### Current answer\n\n")
		if looksLikeFindingsQuestion(question) {
			builder.WriteString("Here are the normalized findings I have in the review state:\n\n")
		} else {
			builder.WriteString("Based on the current review state, the strongest items are:\n\n")
		}
		for index := 0; index < minInt(len(findings), 6); index++ {
			finding := findings[index]
			builder.WriteString(fmt.Sprintf("%d. **%s** - %s at `%s` (%.0f%% confidence).\n",
				index+1,
				fallbackLabel(finding.Severity, "finding"),
				fallbackLabel(finding.CanonicalClaim, "Untitled finding"),
				findingPromptLocation(finding),
				finding.Confidence*100,
			))
		}
		if len(findings) > 6 {
			builder.WriteString(fmt.Sprintf("\n%d more finding%s are available in the Findings tab.\n", len(findings)-6, plural(len(findings)-6)))
		}
	} else {
		builder.WriteString("### Current answer\n\nNo normalized findings are available yet. I can still summarize the reviewer responses below, but the Findings tab has not received structured findings for this session.\n")
	}

	if len(answers) > 0 {
		builder.WriteString("\n### Reviewer signals\n\n")
		for _, answer := range answers {
			body := summarizeAgentAnswer(answer.Body)
			if body == "" {
				continue
			}
			name := fallbackLabel(answer.AuthorDisplayName, "Reviewer")
			builder.WriteString(fmt.Sprintf("- **%s**: %s\n", name, body))
		}
	}
	if len(failures) > 0 {
		builder.WriteString("\n### Gaps\n\n")
		for _, failure := range failures {
			builder.WriteString("- ")
			builder.WriteString(truncatePromptText(failure, 300))
			builder.WriteByte('\n')
		}
	}
	builder.WriteString("\nThe session remains `")
	builder.WriteString(session.Status)
	builder.WriteString("`; I used the durable findings, recent chat, and fresh reviewer outputs for this synthesis.")
	return builder.String()
}

func orchestratorAgentSynthesisPrompt(session dbgen.ReviewSession, thread Thread, userMessage Message, config dbgen.AgentConfig, promptContext chatPromptContext, answers []Message, failures []string, question string) string {
	var builder strings.Builder
	builder.WriteString(chatPrompt(session, thread, userMessage, config, promptContext, question))
	builder.WriteString("\n\n# Orchestration task\n\n")
	builder.WriteString("You are the orchestrator. Synthesize the reviewer responses below with the current normalized findings and recent centralized chat. Do not simply repeat each agent. Resolve disagreements, call out failed or missing reviewers as coverage gaps, and answer the user with a clear Markdown response.\n\n")
	if len(answers) > 0 {
		builder.WriteString("# Reviewer responses for this turn\n\n")
		for _, answer := range answers {
			name := fallbackLabel(answer.AuthorDisplayName, "Reviewer")
			builder.WriteString("## ")
			builder.WriteString(name)
			builder.WriteString("\n\n")
			builder.WriteString(truncatePromptText(answer.Body, 12*1024))
			builder.WriteString("\n\n")
		}
	}
	if len(failures) > 0 {
		builder.WriteString("# Reviewer gaps\n\n")
		for _, failure := range failures {
			builder.WriteString("- ")
			builder.WriteString(truncatePromptText(failure, 800))
			builder.WriteByte('\n')
		}
		builder.WriteByte('\n')
	}
	builder.WriteString("# Required response\n\n")
	builder.WriteString("Return Markdown only. Start with the direct answer. Include a short `Reviewer coverage` section if any reviewer failed or disagreed. If the user asks for findings, use the normalized finding list first and then mention reviewer deltas.\n")
	return builder.String()
}

func looksLikeFindingsQuestion(question string) bool {
	lower := strings.ToLower(question)
	return strings.Contains(lower, "finding") || strings.Contains(lower, "issue") || strings.Contains(lower, "again")
}

func summarizeAgentAnswer(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.TrimPrefix(body, "```markdown")
	body = strings.TrimPrefix(body, "```")
	body = strings.TrimSuffix(body, "```")
	body = strings.TrimSpace(body)
	lines := strings.Split(body, "\n")
	summary := make([]string, 0, 2)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		summary = append(summary, line)
		if len(summary) == 2 {
			break
		}
	}
	return truncatePromptText(strings.Join(summary, " "), 320)
}

type progressMessage struct {
	authorType  string
	displayName string
	body        string
	status      string
}

func reviewProgressMessage(event dbgen.Event) (progressMessage, bool) {
	payload := eventPayload(event.PayloadJson)
	phaseLabel := humanWorkflowPhase(stringValue(payload["phase"]))
	errorMessage := stringValue(payload["error"])
	switch event.Type {
	case "ReviewSessionQueued":
		return progressMessage{
			authorType:  AuthorSystem,
			displayName: "System",
			body:        "Review queued. cocode is preparing context and assigning the selected reviewers.",
			status:      MessageStatusCompleted,
		}, true
	case "ReviewSessionStarted":
		return progressMessage{
			authorType:  AuthorOrchestrator,
			displayName: "Orchestrator",
			body:        "Review started. I’ll build the context bundle, run reviewers in parallel, normalize their outputs, and surface verified findings as they land.",
			status:      MessageStatusCompleted,
		}, true
	case "WorkflowPhaseStarted":
		if message, ok := orchestratorPhaseStartMessage(stringValue(payload["phase"])); ok {
			return progressMessage{
				authorType:  AuthorOrchestrator,
				displayName: "Orchestrator",
				body:        message,
				status:      MessageStatusCompleted,
			}, true
		}
		return progressMessage{
			authorType:  AuthorSystem,
			displayName: "System",
			body:        fmt.Sprintf("%s started.", phaseLabel),
			status:      MessageStatusCompleted,
		}, phaseLabel != ""
	case "WorkflowPhaseCompleted":
		return progressMessage{
			authorType:  AuthorSystem,
			displayName: "System",
			body:        fmt.Sprintf("%s completed.", phaseLabel),
			status:      MessageStatusCompleted,
		}, phaseLabel != ""
	case "WorkflowPhaseFailed":
		if errorMessage == "" {
			errorMessage = "unknown error"
		}
		return progressMessage{
			authorType:  AuthorSystem,
			displayName: "System",
			body:        fmt.Sprintf("%s failed: %s", phaseLabel, errorMessage),
			status:      MessageStatusFailed,
		}, phaseLabel != ""
	case "ReviewSessionPartialFailure":
		return progressMessage{
			authorType:  AuthorSystem,
			displayName: "System",
			body:        "Some reviewers failed, but cocode will continue with the successful outputs and keep the failures visible.",
			status:      MessageStatusCompleted,
		}, true
	case "ReviewSessionCompleted":
		return progressMessage{
			authorType:  AuthorCocode,
			displayName: "cocode",
			body:        "Review completed. Findings, evidence, and publish-ready comments are ready to inspect.",
			status:      MessageStatusCompleted,
		}, true
	case "ReviewSessionFailed":
		if errorMessage == "" {
			errorMessage = "the review workflow failed"
		}
		return progressMessage{
			authorType:  AuthorSystem,
			displayName: "System",
			body:        "Review failed: " + errorMessage,
			status:      MessageStatusFailed,
		}, true
	case "ReviewSessionCanceled":
		return progressMessage{
			authorType:  AuthorSystem,
			displayName: "System",
			body:        "Review canceled.",
			status:      MessageStatusCompleted,
		}, true
	default:
		return progressMessage{}, false
	}
}

func (s Service) reviewAgentRunMessage(ctx context.Context, run dbgen.AgentRun, config dbgen.AgentConfig) (string, string, agentoutput.Answer) {
	label := strings.TrimSpace(config.Name)
	if label == "" {
		label = "Reviewer"
	}
	answer := agentoutput.Answer{EvidenceRefs: json.RawMessage("[]")}
	if run.Status != agentrun.RunStatusSucceeded {
		body, _ := s.agentFailureMessage(ctx, run, config, nil)
		return body, MessageStatusFailed, answer
	}
	answer = s.answerFromRun(ctx, run)
	if content := strings.TrimSpace(answer.Content); content != "" {
		return content, MessageStatusCompleted, answer
	}
	if reasoning := strings.TrimSpace(answer.ReasoningSummary); reasoning != "" {
		return reasoning, MessageStatusCompleted, answer
	}
	return fmt.Sprintf("%s did not emit answer text. The run trace below has the captured reasoning, tool calls, and diagnostics.", label), MessageStatusCompleted, answer
}

func (s Service) agentFailureMessage(ctx context.Context, run dbgen.AgentRun, config dbgen.AgentConfig, runErr error) (string, string) {
	label := strings.TrimSpace(config.Name)
	if label == "" {
		label = "Reviewer"
	}
	detail := strings.TrimSpace(nullableStringValue(run.ErrorMessage))
	if detail == "" && runErr != nil {
		detail = strings.TrimSpace(runErr.Error())
	}
	if stderr, ok := s.readRunArtifactText(ctx, run.StderrArtifactID); ok {
		detail = stderr
	}
	if stdout, ok := s.readRunArtifactText(ctx, run.StdoutArtifactID); ok && (detail == "" || detail == run.Status || strings.EqualFold(detail, "exit status 1")) {
		detail = stdout
	}
	if detail == "" {
		detail = strings.TrimSpace(run.Status)
	}
	if detail == "" {
		detail = "unknown error"
	}
	artifactID := nullableStringValue(run.StderrArtifactID)
	if artifactID == "" {
		artifactID = nullableStringValue(run.StdoutArtifactID)
	}
	if summary, ok := providerFailureSummary(detail); ok {
		return fmt.Sprintf("%s could not complete its review. %s\n\n```text\n%s\n```", label, summary, detail), artifactID
	}
	return fmt.Sprintf("%s could not complete its review.\n\n```text\n%s\n```", label, detail), artifactID
}

func providerFailureSummary(detail string) (string, bool) {
	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "429"),
		strings.Contains(lower, "rate limit"),
		strings.Contains(lower, "ratelimit"),
		strings.Contains(lower, "resource_exhausted"),
		strings.Contains(lower, "quota"),
		strings.Contains(lower, "capacity"),
		strings.Contains(lower, "too many requests"):
		return "The provider reported capacity or rate limiting. cocode kept the full diagnostic and continued with other available agents.", true
	case strings.Contains(lower, "modelnotfound"),
		strings.Contains(lower, "requested entity was not found"),
		strings.Contains(lower, "model was not found"):
		return "The selected model was rejected by the provider. Check the model preset for this agent; the full provider diagnostic is below.", true
	default:
		return "", false
	}
}

func (s Service) readRunArtifactText(ctx context.Context, artifactID sql.NullString) (string, bool) {
	if s.Artifacts == nil || !artifactID.Valid {
		return "", false
	}
	content, _, err := s.Artifacts.Read(ctx, artifactID.String)
	if err != nil {
		return "", false
	}
	text := strings.TrimSpace(string(content))
	return text, text != ""
}

func (s Service) removeHiddenReviewAgentRunMessages(ctx context.Context, messages []Message) ([]Message, error) {
	if s.Database == nil || s.Queries == nil || len(messages) == 0 {
		return messages, nil
	}
	visible := messages[:0]
	for _, message := range messages {
		runID := strings.TrimSpace(message.AgentRunID)
		if runID == "" {
			metadata := messageMetadata(message.Metadata)
			if metadataRunID, ok := metadata["review_agent_run_id"].(string); ok {
				runID = strings.TrimSpace(metadataRunID)
			}
		}
		if runID == "" {
			visible = append(visible, message)
			continue
		}
		run, err := s.Queries.GetAgentRun(ctx, runID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				visible = append(visible, message)
				continue
			}
			return nil, fmt.Errorf("read agent run for chat cleanup: %w", err)
		}
		if !shouldHideReviewAgentRunFromChat(run) {
			visible = append(visible, message)
			continue
		}
		if _, err := s.Database.ExecContext(ctx, `DELETE FROM chat_messages WHERE id = ?`, message.ID); err != nil {
			return nil, fmt.Errorf("delete hidden internal agent message: %w", err)
		}
	}
	return visible, nil
}

func findingsDigestMessage(findings []dbgen.Finding) string {
	lines := []string{"Early findings are in. Here are the top items so far:"}
	limit := minInt(len(findings), 3)
	for index := 0; index < limit; index++ {
		finding := findings[index]
		severity := strings.TrimSpace(finding.Severity)
		if severity == "" {
			severity = "finding"
		}
		lines = append(lines, fmt.Sprintf("%d. [%s] %s", index+1, severity, finding.CanonicalClaim))
	}
	lines = append(lines, "Full details are in the Findings tab.")
	return strings.Join(lines, "\n")
}

func terminalAgentRunStatus(status string) bool {
	switch status {
	case agentrun.RunStatusSucceeded, agentrun.RunStatusFailed, agentrun.RunStatusCanceled, agentrun.RunStatusTimedOut, agentrun.RunStatusOutputInvalid:
		return true
	default:
		return false
	}
}

func shouldHideReviewAgentRunFromChat(run dbgen.AgentRun) bool {
	role := strings.ToLower(strings.TrimSpace(run.Role))
	return role == AuthorOrchestrator ||
		role == AuthorVerifier ||
		strings.Contains(role, "orchestrator") ||
		strings.Contains(role, "verifier")
}

func terminalReviewProgressEvent(eventType string) bool {
	switch eventType {
	case "ReviewSessionCompleted", "ReviewSessionFailed", "ReviewSessionCanceled":
		return true
	default:
		return false
	}
}

func eventPayload(raw string) map[string]any {
	var payload map[string]any
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return map[string]any{}
	}
	return payload
}

func messageMetadata(raw json.RawMessage) map[string]any {
	var metadata map[string]any
	if json.Unmarshal(raw, &metadata) != nil {
		return map[string]any{}
	}
	return metadata
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func sortedMapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key, ok := range values {
		if ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func minPositive(left int, right int) int {
	switch {
	case left <= 0:
		return right
	case right <= 0:
		return left
	case left < right:
		return left
	default:
		return right
	}
}

func orchestratorPhaseStartMessage(phase string) (string, bool) {
	switch strings.TrimSpace(phase) {
	case "risk_scout":
		return "Orchestrator is risk-tiering the diff and preparing local scout leads for the reviewers.", true
	case "normalize_outputs":
		return "Orchestrator is reading reviewer outputs and extracting candidate findings.", true
	case "deduplicate", "deduplicate_findings":
		return "Orchestrator is re-checking and deduplicating findings across reviewer outputs.", true
	case "verify_findings":
		return "Orchestrator is re-checking each finding against code evidence and counter-evidence.", true
	case "build_evidence", "build_evidence_maps":
		return "Orchestrator is enriching findings with evidence maps and source context.", true
	default:
		return "", false
	}
}

func humanWorkflowPhase(phase string) string {
	switch strings.TrimSpace(phase) {
	case "build_context", "build_review_context":
		return "Context build"
	case "risk_scout":
		return "Risk scout"
	case "run_agents", "run_review_agents":
		return "Agent review"
	case "normalize_outputs":
		return "Finding normalization"
	case "deduplicate", "deduplicate_findings":
		return "Finding deduplication"
	case "verify_findings":
		return "Finding verification"
	case "build_evidence", "build_evidence_maps":
		return "Evidence map build"
	case "draft_comments":
		return "Publish draft preparation"
	default:
		if phase == "" {
			return ""
		}
		return strings.ReplaceAll(phase, "_", " ")
	}
}

func (s Service) emit(ctx context.Context, reviewSessionID string, eventType string, payload map[string]any) {
	if s.Events == nil || strings.TrimSpace(reviewSessionID) == "" {
		return
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		payloadJSON = []byte("{}")
	}
	_, _ = s.Events.Append(ctx, eventlog.AppendParams{
		ID:              s.newID("event_"),
		ReviewSessionID: reviewSessionID,
		Type:            eventType,
		Level:           "info",
		PayloadJSON:     string(payloadJSON),
		CreatedAt:       s.now().Format(time.RFC3339Nano),
	})
}

func (s Service) agentRunEventSink(reviewSessionID string) func(context.Context, agents.AgentEvent) {
	if s.Events == nil || strings.TrimSpace(reviewSessionID) == "" {
		return nil
	}
	return func(ctx context.Context, event agents.AgentEvent) {
		if err := s.appendAgentRunEvent(ctx, reviewSessionID, event); err != nil {
			log.Printf("chat: append agent run event failed review_session_id=%s agent_run_id=%s event=%s: %v", reviewSessionID, event.RunID, event.Type, err)
		}
	}
}

func (s Service) appendAgentRunEvent(ctx context.Context, reviewSessionID string, event agents.AgentEvent) error {
	if s.Events == nil {
		return nil
	}
	eventType := chatAgentRunEventType(event.Type)
	if eventType == "" {
		return nil
	}
	payload := map[string]any{
		"agent_run_id": event.RunID,
		"agent_event":  string(event.Type),
		"message":      event.Message,
	}
	level := "info"
	if event.Stream != "" {
		payload["stream"] = event.Stream
		payload["text_bytes"] = len(event.Text)
		payload["truncated"] = event.Truncated
		if preview := truncateEventPreview(event.Text); preview != "" {
			payload["text_preview"] = preview
		}
	}
	if event.ExitCode != nil {
		payload["exit_code"] = *event.ExitCode
	}
	if event.ErrorCode != "" {
		payload["error_code"] = event.ErrorCode
	}
	if event.Error != "" {
		payload["error"] = event.Error
		level = "error"
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode agent event payload: %w", err)
	}
	createdAt := event.At
	if createdAt.IsZero() {
		createdAt = s.now()
	}
	_, err = s.Events.Append(ctx, eventlog.AppendParams{
		ID:              s.newID("event_"),
		ReviewSessionID: strings.TrimSpace(reviewSessionID),
		AgentRunID:      nullableString(event.RunID),
		Type:            eventType,
		Level:           level,
		PayloadJSON:     string(payloadJSON),
		ArtifactID:      nullableString(event.ArtifactID),
		CreatedAt:       createdAt.UTC().Format(time.RFC3339Nano),
	})
	return err
}

func truncateEventPreview(value string) string {
	value = strings.TrimSpace(value)
	const limit = 12 * 1024
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n..."
}

func chatAgentRunEventType(eventType agents.EventType) string {
	switch eventType {
	case agents.EventQueued:
		return "AgentRunQueued"
	case agents.EventStarted:
		return "AgentRunStarted"
	case agents.EventProgress:
		return "AgentRunProgress"
	case agents.EventOutput:
		return "AgentRunOutput"
	case agents.EventArtifact:
		return "AgentRunArtifact"
	case agents.EventCompleted:
		return "AgentRunCompleted"
	case agents.EventFailed:
		return "AgentRunFailed"
	case agents.EventCanceled:
		return "AgentRunCanceled"
	default:
		return ""
	}
}

func scanThread(scanner interface {
	Scan(dest ...any) error
}) (Thread, error) {
	var thread Thread
	if err := scanner.Scan(&thread.ID, &thread.ReviewSessionID, &thread.Title, &thread.Status, &thread.CreatedAt, &thread.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Thread{}, ErrThreadNotFound
		}
		return Thread{}, fmt.Errorf("scan chat thread: %w", err)
	}
	return thread, nil
}

func scanMessage(scanner interface {
	Scan(dest ...any) error
}) (Message, error) {
	var message Message
	var parent, agentConfigID, agentRunID, contextBundleID, artifactID sql.NullString
	var metadata string
	if err := scanner.Scan(
		&message.ID,
		&message.ThreadID,
		&parent,
		&message.AuthorType,
		&message.AuthorDisplayName,
		&agentConfigID,
		&agentRunID,
		&contextBundleID,
		&artifactID,
		&message.Body,
		&message.Status,
		&metadata,
		&message.CreatedAt,
		&message.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Message{}, ErrInvalidMessage
		}
		return Message{}, fmt.Errorf("scan chat message: %w", err)
	}
	message.ParentMessageID = nullableStringValue(parent)
	message.AgentConfigID = nullableStringValue(agentConfigID)
	message.AgentRunID = nullableStringValue(agentRunID)
	message.ContextBundleID = nullableStringValue(contextBundleID)
	message.ArtifactID = nullableStringValue(artifactID)
	message.Metadata = normalizedJSON(json.RawMessage(metadata), "{}")
	return message, nil
}

func scanTurn(scanner interface {
	Scan(dest ...any) error
}) (Turn, error) {
	var turn Turn
	var responder, code, message, startedAt, completedAt sql.NullString
	if err := scanner.Scan(
		&turn.ID,
		&turn.ThreadID,
		&turn.UserMessageID,
		&turn.Mode,
		&turn.Audience,
		&responder,
		&turn.Status,
		&code,
		&message,
		&startedAt,
		&completedAt,
		&turn.CreatedAt,
		&turn.UpdatedAt,
	); err != nil {
		return Turn{}, fmt.Errorf("scan chat turn: %w", err)
	}
	turn.ResponderAgentConfigID = nullableStringValue(responder)
	turn.ErrorCode = nullableStringValue(code)
	turn.ErrorMessage = nullableStringValue(message)
	turn.StartedAt = nullableStringValue(startedAt)
	turn.CompletedAt = nullableStringValue(completedAt)
	return turn, nil
}

func localAnswer(session dbgen.ReviewSession, findings []dbgen.Finding, events []dbgen.Event, question string) string {
	findings = userVisibleChatFindings(findings)
	if finding, ok := bestLocalFindingMatch(question, findings); ok {
		return localFindingAnswer(session, finding)
	}

	verified := 0
	accepted := 0
	dismissed := 0
	for _, finding := range findings {
		if finding.VerificationStatus == "verified" {
			verified++
		}
		switch finding.DecisionStatus {
		case "accepted":
			accepted++
		case "dismissed":
			dismissed++
		}
	}
	lines := []string{
		fmt.Sprintf("Current review status: %s.", session.Status),
		fmt.Sprintf("Findings: %d total, %d verified, %d accepted, %d dismissed.", len(findings), verified, accepted, dismissed),
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		lines = append(lines, fmt.Sprintf("Latest activity: %s.", last.Type))
	}
	if strings.Contains(strings.ToLower(question), "top") && len(findings) > 0 {
		lines = append(lines, "Top finding: "+findings[0].CanonicalClaim)
	}
	return strings.Join(lines, "\n")
}

func bestLocalFindingMatch(question string, findings []dbgen.Finding) (dbgen.Finding, bool) {
	if len(findings) == 0 || !looksLikeFindingsQuestion(question) {
		return dbgen.Finding{}, false
	}
	questionText := normalizeFindingMatchText(question)
	questionTokens := findingMatchTokens(questionText)
	bestIndex := -1
	bestScore := 0
	for index, finding := range findings {
		claim := normalizeFindingMatchText(finding.CanonicalClaim)
		location := normalizeFindingMatchText(findingPromptLocation(finding))
		score := 0
		if claim != "" && strings.Contains(questionText, claim) {
			score += 100
		}
		if location != "" && strings.Contains(questionText, location) {
			score += 50
		}
		findingTokens := findingMatchTokens(strings.Join([]string{
			finding.CanonicalClaim,
			finding.Severity,
			finding.Category,
			findingPromptLocation(finding),
		}, " "))
		for token := range questionTokens {
			if _, ok := findingTokens[token]; ok {
				score += 3
			}
		}
		if score > bestScore {
			bestScore = score
			bestIndex = index
		}
	}
	if bestIndex < 0 || bestScore < 9 {
		return dbgen.Finding{}, false
	}
	return findings[bestIndex], true
}

func localFindingAnswer(session dbgen.ReviewSession, finding dbgen.Finding) string {
	title := fallbackLabel(finding.CanonicalClaim, "Untitled finding")
	lines := []string{
		"## " + title,
		"",
		fmt.Sprintf("This is a **%s** finding in review session `%s` with **%.0f%% confidence**.", fallbackLabel(finding.Severity, "unknown severity"), session.ID, finding.Confidence*100),
		fmt.Sprintf("Status: `%s`; verification: `%s`; session: `%s`.", fallbackLabel(finding.DecisionStatus, "needs_triage"), fallbackLabel(finding.VerificationStatus, "unverified"), fallbackLabel(session.Status, "unknown")),
	}
	if location := findingPromptLocation(finding); location != "No primary location" {
		lines = append(lines, fmt.Sprintf("Location: `%s`.", location))
	}
	if evidence := strings.TrimSpace(nullableStringValue(finding.EvidenceSummary)); evidence != "" {
		lines = append(lines, "", "### Why it was flagged", truncatePromptText(evidence, 1200))
	}
	if counter := strings.TrimSpace(nullableStringValue(finding.CounterEvidenceSummary)); counter != "" {
		lines = append(lines, "", "### Verification checks", truncatePromptText(counter, 900))
	}
	if fix := strings.TrimSpace(nullableStringValue(finding.SuggestedFix)); fix != "" {
		lines = append(lines, "", "### Suggested fix", truncatePromptText(fix, 900))
	}
	return strings.Join(lines, "\n")
}

func normalizeFindingMatchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	builder.Grow(len(value))
	lastSpace := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			builder.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(builder.String())
}

func findingMatchTokens(value string) map[string]struct{} {
	tokens := map[string]struct{}{}
	for _, token := range strings.Fields(normalizeFindingMatchText(value)) {
		if len(token) < 3 || localFindingStopWords[token] {
			continue
		}
		tokens[token] = struct{}{}
	}
	return tokens
}

var localFindingStopWords = map[string]bool{
	"again": true,
	"and":   true,
	"for":   true,
	"from":  true,
	"high":  true,
	"low":   true,
	"me":    true,
	"the":   true,
	"this":  true,
	"with":  true,
}

func chatPrompt(session dbgen.ReviewSession, thread Thread, userMessage Message, config dbgen.AgentConfig, promptContext chatPromptContext, question string) string {
	var builder strings.Builder
	builder.WriteString("# Role\n\n")
	builder.WriteString("You are ")
	builder.WriteString(config.Name)
	builder.WriteString(", a code review agent responding inside cocode's centralized review chat.\n\n")
	builder.WriteString("# Output\n\n")
	builder.WriteString("Return a concise Markdown answer. Cite concrete files, lines, findings, or evidence when available. Use tables or fenced code blocks when they make the answer clearer. The sections below are the authoritative current review context for this turn, including normalized findings, recent chat, and the context bundle when available; do not ask the user to provide prior findings unless every provided section is empty.\n\n")
	builder.WriteString("# Rules\n\n")
	builder.WriteString("- Answer only the user's question.\n")
	builder.WriteString("- ")
	builder.WriteString(reviewprompt.UntrustedContextInstruction())
	builder.WriteByte('\n')
	builder.WriteString("- Do not modify files or run write actions.\n\n")
	builder.WriteString("# Review\n\n")
	builder.WriteString("Review session: ")
	builder.WriteString(session.ID)
	builder.WriteByte('\n')
	builder.WriteString("Title: ")
	builder.WriteString(session.Title)
	builder.WriteByte('\n')
	builder.WriteString("Thread: ")
	builder.WriteString(thread.ID)
	builder.WriteByte('\n')
	builder.WriteString("User message: ")
	builder.WriteString(userMessage.ID)
	builder.WriteString("\n\n")
	if len(promptContext.Findings) > 0 {
		builder.WriteString("# Current findings\n\n")
		builder.WriteString(renderChatFindings(promptContext.Findings))
		builder.WriteString("\n\n")
	} else {
		builder.WriteString("# Current findings\n\nNo normalized findings have been stored yet. If the user asks for findings, explain that no findings are currently available and use the review context bundle to answer what can be inferred.\n\n")
	}
	if promptContext.IncludeRecentMessages && len(promptContext.RecentMessages) > 0 {
		builder.WriteString("# Recent centralized chat\n\n")
		builder.WriteString(renderChatMessages(promptContext.RecentMessages))
		builder.WriteString("\n\n")
	}
	if refs := strings.TrimSpace(string(promptContext.ContextRefs)); refs != "" && refs != "[]" && refs != "{}" {
		builder.WriteString("# User-selected context references\n\n```json\n")
		builder.WriteString(truncatePromptText(refs, 8*1024))
		builder.WriteString("\n```\n\n")
	}
	if promptContext.IncludeEvidence {
		builder.WriteString("# Review context bundle\n\n")
		builder.WriteString(truncatePromptText(contextbundle.RenderBundle(promptContext.Bundle), 64*1024))
		builder.WriteString("\n\n")
	}
	builder.WriteString("# User question\n\n")
	builder.WriteString(question)
	return builder.String()
}

func renderChatFindings(findings []dbgen.Finding) string {
	if len(findings) == 0 {
		return "No findings have been normalized yet."
	}
	limit := minInt(len(findings), 12)
	lines := make([]string, 0, limit*6)
	for index := 0; index < limit; index++ {
		finding := findings[index]
		title := strings.TrimSpace(finding.CanonicalClaim)
		if title == "" {
			title = "Untitled finding"
		}
		lines = append(lines,
			fmt.Sprintf("%d. **%s**", index+1, title),
			fmt.Sprintf("   - Severity: %s; status: %s; confidence: %.0f%%", fallbackLabel(finding.Severity, "unknown"), fallbackLabel(finding.DecisionStatus, "needs_triage"), finding.Confidence*100),
			fmt.Sprintf("   - Location: %s", findingPromptLocation(finding)),
		)
		if evidence := strings.TrimSpace(nullableStringValue(finding.EvidenceSummary)); evidence != "" {
			lines = append(lines, "   - Evidence: "+truncatePromptText(evidence, 700))
		}
		if fix := strings.TrimSpace(nullableStringValue(finding.SuggestedFix)); fix != "" {
			lines = append(lines, "   - Suggested fix: "+truncatePromptText(fix, 500))
		}
	}
	if len(findings) > limit {
		lines = append(lines, fmt.Sprintf("... %d additional finding(s) omitted from the prompt.", len(findings)-limit))
	}
	return strings.Join(lines, "\n")
}

func renderChatMessages(messages []Message) string {
	if len(messages) == 0 {
		return "No prior chat messages are available."
	}
	lines := make([]string, 0, len(messages)*2)
	for _, message := range messages {
		body := strings.TrimSpace(message.Body)
		if body == "" {
			continue
		}
		label := strings.TrimSpace(message.AuthorDisplayName)
		if label == "" {
			label = defaultDisplayName(message.AuthorType)
		}
		lines = append(lines,
			fmt.Sprintf("- %s (%s, %s):", label, message.AuthorType, message.Status),
			indentPromptBlock(truncatePromptText(body, 1600), "  "),
		)
	}
	if len(lines) == 0 {
		return "No prior chat messages are available."
	}
	return strings.Join(lines, "\n")
}

func userVisibleChatFindings(findings []dbgen.Finding) []dbgen.Finding {
	filtered := findings[:0]
	for _, finding := range findings {
		if chatFindingLooksLikeMachineEvent(finding) {
			continue
		}
		filtered = append(filtered, finding)
	}
	return filtered
}

func chatFindingLooksLikeMachineEvent(finding dbgen.Finding) bool {
	claim := strings.TrimSpace(finding.CanonicalClaim)
	if claim == "" || !strings.HasPrefix(claim, "{") {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(claim), &payload); err != nil {
		return strings.Contains(claim, `"hook_started"`) && strings.Contains(claim, `"hook_name"`)
	}
	eventType := strings.ToLower(strings.TrimSpace(stringValue(payload["type"])))
	subtype := strings.ToLower(strings.TrimSpace(stringValue(payload["subtype"])))
	hookName := strings.ToLower(strings.TrimSpace(stringValue(payload["hook_name"])))
	return eventType == "system" ||
		eventType == "thread.started" ||
		eventType == "turn.started" ||
		strings.Contains(subtype, "hook") ||
		strings.Contains(hookName, "sessionstart")
}

func findingPromptLocation(finding dbgen.Finding) string {
	path := strings.TrimSpace(nullableStringValue(finding.PrimaryPath))
	if path == "" {
		return "No primary location"
	}
	start := finding.PrimaryStartLine.Int64
	end := finding.PrimaryEndLine.Int64
	if start <= 0 {
		return path
	}
	if end > start {
		return fmt.Sprintf("%s:%d-%d", path, start, end)
	}
	return fmt.Sprintf("%s:%d", path, start)
}

func indentPromptBlock(value string, prefix string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	for index, line := range lines {
		lines[index] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func fallbackLabel(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func truncatePromptText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	truncated := value[:limit]
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated + "\n...[truncated]"
}

func connectionConfig(config dbgen.AgentConfig, repository dbgen.Repository, workspace dbgen.Workspace) (agents.ConnectionConfig, agents.TaskLimits, error) {
	args, err := agents.DecodeStringArray(config.ArgsJson, "agent args")
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, fmt.Errorf("%w: %v", ErrInvalidAgentConfig, err)
	}
	envNames, err := agents.DecodeStringArray(config.EnvAllowlistJson, "agent env_allowlist")
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, fmt.Errorf("%w: %v", ErrInvalidAgentConfig, err)
	}
	env, err := agents.ResolveAllowedEnvironment(envNames)
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, fmt.Errorf("%w: agent env_allowlist is invalid: %v", ErrInvalidAgentConfig, err)
	}
	settings, err := decodeRuntimeSettings(config.SettingsJson)
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, err
	}
	workingDirectory, err := workingDirectoryForAgent(config.CwdMode, repository, workspace)
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, err
	}
	command := nullableStringValue(config.Command)
	kind := agents.AdapterKind(config.AdapterKind)
	modelLabel := strings.TrimSpace(nullableStringValue(config.ModelLabel))
	reasoningLabel := strings.TrimSpace(nullableStringValue(config.ReasoningLabel))
	args = agents.SanitizeCommandArgs(command, args)
	args = agents.CommandArgsWithModelSelection(kind, command, args, modelLabel, reasoningLabel)
	return agents.ConnectionConfig{
			AdapterID:        config.ID,
			Kind:             kind,
			Command:          command,
			Args:             args,
			PromptDelivery:   settings.PromptDelivery,
			CommandSafety:    agents.CommandSafetyOptions{AllowRiskyCommand: settings.AllowRiskyCommand},
			WorkingDirectory: workingDirectory,
			Env:              env,
			Metadata: map[string]any{
				"output_mode":     config.OutputMode,
				"model_label":     modelLabel,
				"reasoning_label": reasoningLabel,
			},
		}, agents.TaskLimits{
			TimeoutSeconds: settings.TimeoutSeconds,
			MaxStdoutBytes: settings.MaxStdoutBytes,
			MaxStderrBytes: settings.MaxStderrBytes,
			MaxPromptBytes: settings.MaxPromptBytes,
		}, nil
}

func validateAgentConfig(config dbgen.AgentConfig) error {
	capabilities, err := agentCapabilities(config)
	if err != nil {
		return err
	}
	if err := agents.ValidateReviewModePermissions(agents.ConnectionConfig{Kind: agents.AdapterKind(config.AdapterKind)}, capabilities); err != nil {
		return fmt.Errorf("%w: agent config %s cannot be used for review mode: %v", ErrInvalidAgentConfig, config.ID, err)
	}
	if agents.AdapterKind(config.AdapterKind) != agents.AdapterCLINonInteractive {
		return fmt.Errorf("%w: adapter %q is unsupported for centralized chat", ErrInvalidAgentConfig, config.AdapterKind)
	}
	return nil
}

func agentCapabilities(config dbgen.AgentConfig) (agents.AgentCapabilities, error) {
	capabilities, err := agents.DecodeCapabilitiesJSON(config.CapabilitiesJson, agents.AdapterKind(config.AdapterKind))
	if err != nil {
		return agents.AgentCapabilities{}, fmt.Errorf("%w: agent capabilities are invalid", ErrInvalidAgentConfig)
	}
	return capabilities, nil
}

func decodeRuntimeSettings(raw string) (runtimeSettings, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	var settings runtimeSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return runtimeSettings{}, fmt.Errorf("%w: settings must be a JSON object", ErrInvalidAgentConfig)
	}
	if settings.PromptDelivery != "" && !settings.PromptDelivery.Valid() {
		return runtimeSettings{}, fmt.Errorf("%w: prompt_delivery %q is invalid", ErrInvalidAgentConfig, settings.PromptDelivery)
	}
	if settings.TimeoutSeconds < 0 || settings.MaxStdoutBytes < 0 || settings.MaxStderrBytes < 0 || settings.MaxPromptBytes < 0 {
		return runtimeSettings{}, fmt.Errorf("%w: runtime limits cannot be negative", ErrInvalidAgentConfig)
	}
	return settings, nil
}

func workingDirectoryForAgent(cwdMode string, repository dbgen.Repository, workspace dbgen.Workspace) (string, error) {
	switch strings.TrimSpace(cwdMode) {
	case "", "repo_root":
		if strings.TrimSpace(repository.LocalPath) == "" {
			return "", fmt.Errorf("%w: repository local path is not configured", ErrInvalidAgentConfig)
		}
		return repository.LocalPath, nil
	case "workspace_root":
		if strings.TrimSpace(workspace.RootPath) == "" {
			return "", fmt.Errorf("%w: workspace root path is not configured", ErrInvalidAgentConfig)
		}
		return workspace.RootPath, nil
	default:
		return "", fmt.Errorf("%w: cwd_mode %q is unsupported", ErrInvalidAgentConfig, cwdMode)
	}
}

func normalizeAuthor(author string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(author)) {
	case AuthorUser:
		return AuthorUser, nil
	case AuthorCocode:
		return AuthorCocode, nil
	case AuthorOrchestrator:
		return AuthorOrchestrator, nil
	case AuthorAgent:
		return AuthorAgent, nil
	case AuthorSystem:
		return AuthorSystem, nil
	case AuthorVerifier:
		return AuthorVerifier, nil
	default:
		return "", fmt.Errorf("%w: author_type is invalid", ErrInvalidMessage)
	}
}

func normalizeAudience(audience string) string {
	switch strings.ToLower(strings.TrimSpace(audience)) {
	case AudienceAllAgents:
		return AudienceAllAgents
	case AudienceSelected:
		return AudienceSelected
	default:
		return AudienceOrchestrator
	}
}

func defaultDisplayName(author string) string {
	switch author {
	case AuthorUser:
		return "You"
	case AuthorOrchestrator:
		return "Orchestrator"
	case AuthorSystem:
		return "System"
	case AuthorAgent:
		return "Agent"
	default:
		return "cocode"
	}
}

func defaultThreadTitle(session dbgen.ReviewSession) string {
	title := strings.TrimSpace(session.Title)
	if title == "" {
		title = session.ID
	}
	if len(title) <= defaultThreadTitleBytes {
		return title
	}
	for len(title) > defaultThreadTitleBytes {
		_, size := utf8.DecodeLastRuneInString(title)
		if size <= 0 {
			break
		}
		title = title[:len(title)-size]
	}
	return strings.TrimSpace(title) + "..."
}

func normalizedJSON(raw json.RawMessage, fallback string) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return json.RawMessage(fallback)
	}
	if !json.Valid([]byte(trimmed)) {
		return json.RawMessage(fallback)
	}
	return json.RawMessage(trimmed)
}

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func nullableStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func timestampAfter(value string) string {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	return parsed.Add(time.Nanosecond).UTC().Format(time.RFC3339Nano)
}

func agentRunModelLabels(run dbgen.AgentRun) (string, string) {
	var metadata struct {
		ModelLabel     string `json:"model_label"`
		ReasoningLabel string `json:"reasoning_label"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(run.MetadataJson)), &metadata); err != nil {
		return "", ""
	}
	return strings.TrimSpace(metadata.ModelLabel), strings.TrimSpace(metadata.ReasoningLabel)
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) newID(prefix string) string {
	if s.NewID != nil {
		return s.NewID(prefix)
	}
	return prefix + randomHex(8)
}

func randomHex(bytes int) string {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", data)
}
