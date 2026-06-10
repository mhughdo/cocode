package orchestrator

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/hughdo/cocode/services/cocoded/internal/agentoutput"
	"github.com/hughdo/cocode/services/cocoded/internal/agentrun"
	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/contextbundle"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/eventlog"
	"github.com/hughdo/cocode/services/cocoded/internal/evidence"
	"github.com/hughdo/cocode/services/cocoded/internal/findingengine"
	"github.com/hughdo/cocode/services/cocoded/internal/reviewprompt"
)

const (
	StatusDraft     = "draft"
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusPaused    = "paused"
	StatusCanceling = "canceling"
	StatusCanceled  = "canceled"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

const (
	PhaseBuildContext     = "build_review_context"
	PhaseScoutRisk        = "risk_scout"
	PhaseRunAgents        = "run_review_agents"
	PhaseNormalizeOutputs = "normalize_outputs"
	PhaseDeduplicate      = "deduplicate_findings"
	PhaseVerifyFindings   = "verify_findings"
	PhaseBuildEvidence    = "build_evidence_maps"
	PhaseDraftComments    = "draft_comments"
)

var (
	ErrServiceNotConfigured      = errors.New("review workflow service is not configured")
	ErrReviewSessionNotFound     = errors.New("review session was not found")
	ErrAgentRunNotFound          = errors.New("agent run was not found")
	ErrInvalidStatusTransition   = errors.New("invalid review session status transition")
	ErrNoEnabledReviewAgents     = errors.New("review session has no enabled agents")
	ErrInvalidAgentConfiguration = errors.New("agent configuration is invalid")
	ErrReviewSessionCanceled     = errors.New("review session was canceled")
)

type Service struct {
	Queries          *dbgen.Queries
	ContextBuilder   *contextbundle.Service
	Artifacts        *artifact.Store
	Events           EventLog
	AgentManager     *agentrun.Manager
	Evidence         *evidence.Service
	DedupeHook       DedupeHook
	EnableDedupeHook bool
	PromptTemplate   string
	Background       context.Context
	Now              func() time.Time
	NewEventID       func() string
	NewArtifactID    func() string

	mu             sync.Mutex
	activeSessions map[string]struct{}
}

type EventLog interface {
	Append(ctx context.Context, params eventlog.AppendParams) (dbgen.Event, error)
	ListByReviewSession(ctx context.Context, reviewSessionID string) ([]dbgen.Event, error)
}

type DedupeHook interface {
	RefineDedupe(ctx context.Context, input findingengine.DedupeInput) (findingengine.DedupeResult, error)
}

type StartResult struct {
	Session dbgen.ReviewSession
}

type ReconcileResult struct {
	SessionsPaused      int `json:"sessions_paused"`
	SessionsCanceled    int `json:"sessions_canceled"`
	AgentRunsCanceled   int `json:"agent_runs_canceled"`
	AgentRunEvents      int `json:"agent_run_events"`
	SessionEvents       int `json:"session_events"`
	InterruptedSessions int `json:"interrupted_sessions"`
	InterruptedRuns     int `json:"interrupted_runs"`
}

type Checkpoint struct {
	ReviewSessionID string   `json:"review_session_id"`
	Status          string   `json:"status"`
	Phase           string   `json:"phase,omitempty"`
	PhaseStatus     string   `json:"phase_status,omitempty"`
	LastSequence    int64    `json:"last_sequence,omitempty"`
	CompletedPhases []string `json:"completed_phases,omitempty"`
	StartedAt       string   `json:"started_at,omitempty"`
	CompletedAt     string   `json:"completed_at,omitempty"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
}

type RunSummary struct {
	ReviewSessionID     string         `json:"review_session_id"`
	Status              string         `json:"status"`
	Phase               string         `json:"phase,omitempty"`
	PhaseStatus         string         `json:"phase_status,omitempty"`
	ProgressPercent     int            `json:"progress_percent"`
	ChangedFilesTotal   int            `json:"changed_files_total"`
	ChangedFilesScanned int            `json:"changed_files_scanned"`
	AgentRunsTotal      int            `json:"agent_runs_total"`
	ActiveAgents        int            `json:"active_agents"`
	AgentStatusCounts   map[string]int `json:"agent_status_counts"`
	AgentRuns           []AgentRun     `json:"agent_runs,omitempty"`
	FindingCounts       FindingCounts  `json:"finding_counts"`
	UpdatedAt           string         `json:"updated_at,omitempty"`
}

type AgentRun struct {
	ID                   string `json:"id"`
	ReviewSessionID      string `json:"review_session_id"`
	AgentConfigID        string `json:"agent_config_id"`
	ReviewSessionAgentID string `json:"review_session_agent_id,omitempty"`
	ContextBundleID      string `json:"context_bundle_id,omitempty"`
	Status               string `json:"status"`
	Role                 string `json:"role"`
	ModelLabel           string `json:"model_label,omitempty"`
	ReasoningLabel       string `json:"reasoning_label,omitempty"`
	StartedAt            string `json:"started_at,omitempty"`
	CompletedAt          string `json:"completed_at,omitempty"`
	ErrorCode            string `json:"error_code,omitempty"`
	ErrorMessage         string `json:"error_message,omitempty"`
}

type FindingCounts struct {
	Candidates           int            `json:"candidates"`
	Findings             int            `json:"findings"`
	BySeverity           map[string]int `json:"by_severity"`
	ByVerificationStatus map[string]int `json:"by_verification_status"`
	ByDecisionStatus     map[string]int `json:"by_decision_status"`
}

type runtimeSettings struct {
	PromptDelivery    agents.PromptDelivery `json:"prompt_delivery"`
	AllowRiskyCommand bool                  `json:"allow_risky_command"`
	TimeoutSeconds    int64                 `json:"timeout_seconds"`
	MaxStdoutBytes    int64                 `json:"max_stdout_bytes"`
	MaxStderrBytes    int64                 `json:"max_stderr_bytes"`
	MaxPromptBytes    int64                 `json:"max_prompt_bytes"`
	ReviewTimeoutSecs int64                 `json:"review_timeout_seconds"`
}

type sessionAgentSettingsOverride struct {
	ModelLabel     string `json:"model_label"`
	ReasoningLabel string `json:"reasoning_label"`
}

type runContext struct {
	Session      dbgen.ReviewSession
	Repository   dbgen.Repository
	Workspace    dbgen.Workspace
	SessionAgent dbgen.ReviewSessionAgent
	AgentConfig  dbgen.AgentConfig
	Bundle       contextbundle.Bundle
	BundleText   string
	Scout        localReviewScout
}

type localReviewScout struct {
	SchemaVersion string             `json:"schema_version"`
	OverallRisk   string             `json:"overall_risk"`
	RiskScore     int                `json:"risk_score"`
	Profiles      []string           `json:"profiles,omitempty"`
	Summary       string             `json:"summary"`
	Leads         []localReviewLead  `json:"leads,omitempty"`
	IgnoredAreas  []localIgnoredArea `json:"ignored_areas,omitempty"`
	GeneratedAt   string             `json:"generated_at"`
}

type localReviewLead struct {
	Path              string   `json:"path"`
	Status            string   `json:"status"`
	StartLine         int64    `json:"start_line,omitempty"`
	EndLine           int64    `json:"end_line,omitempty"`
	Additions         int64    `json:"additions"`
	Deletions         int64    `json:"deletions"`
	RiskScore         int      `json:"risk_score"`
	SeverityHint      string   `json:"severity_hint"`
	SuggestedReviewer string   `json:"suggested_reviewer"`
	Reason            string   `json:"reason"`
	Signals           []string `json:"signals,omitempty"`
}

type localIgnoredArea struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

func (s *Service) Start(ctx context.Context, reviewSessionID string) (StartResult, error) {
	if err := s.validate(); err != nil {
		return StartResult{}, err
	}
	session, err := s.Transition(ctx, reviewSessionID, StatusQueued)
	if err != nil {
		return StartResult{}, err
	}
	if err := s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: session.ID,
		Type:            "ReviewSessionQueued",
		Payload: map[string]any{
			"status": session.Status,
		},
	}); err != nil {
		return StartResult{}, err
	}

	go func() {
		_ = s.Run(s.background(), session.ID)
	}()
	return StartResult{Session: session}, nil
}

func (s *Service) Run(ctx context.Context, reviewSessionID string) error {
	if err := s.validate(); err != nil {
		return err
	}
	ctx = contextOrBackground(ctx)
	if !s.registerActiveSession(reviewSessionID) {
		return fmt.Errorf("%w: review session is already running", ErrInvalidStatusTransition)
	}
	defer s.unregisterActiveSession(reviewSessionID)
	err := s.run(ctx, reviewSessionID)
	return s.handleRunError(ctx, reviewSessionID, err)
}

func (s *Service) handleRunError(ctx context.Context, reviewSessionID string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrReviewSessionCanceled) {
		return nil
	}
	failCtx := context.WithoutCancel(ctx)
	_, _ = s.Transition(failCtx, reviewSessionID, StatusFailed)
	_ = s.appendEvent(failCtx, appendEventParams{
		ReviewSessionID: reviewSessionID,
		Type:            "ReviewSessionFailed",
		Level:           "error",
		Payload: map[string]any{
			"error": err.Error(),
		},
	})
	return err
}

func (s *Service) ReconcileLocalSessions(ctx context.Context) (ReconcileResult, error) {
	if err := s.validate(); err != nil {
		return ReconcileResult{}, err
	}
	ctx = contextOrBackground(ctx)
	result := ReconcileResult{}

	runs, err := s.Queries.ListInterruptedAgentRuns(ctx)
	if err != nil {
		return result, fmt.Errorf("list interrupted agent runs: %w", err)
	}
	result.InterruptedRuns = len(runs)
	for _, run := range runs {
		updated, err := s.cancelInterruptedAgentRun(ctx, run)
		if err != nil {
			return result, err
		}
		result.AgentRunsCanceled++
		if err := s.appendEvent(ctx, appendEventParams{
			ReviewSessionID: updated.ReviewSessionID,
			AgentRunID:      nullableEventString(updated.ID),
			Type:            "AgentRunCanceled",
			Level:           "warn",
			Payload: map[string]any{
				"agent_run_id":    updated.ID,
				"agent_config_id": updated.AgentConfigID,
				"previous_status": run.Status,
				"status":          updated.Status,
				"error_code":      "app_restarted",
				"reason":          "backend restarted before the local agent run reached a terminal state",
			},
		}); err != nil {
			return result, err
		}
		result.AgentRunEvents++
	}

	sessions, err := s.Queries.ListInterruptedReviewSessions(ctx)
	if err != nil {
		return result, fmt.Errorf("list interrupted review sessions: %w", err)
	}
	result.InterruptedSessions = len(sessions)
	for _, session := range sessions {
		switch session.Status {
		case StatusCanceling:
			updated, err := s.reconcileSessionStatus(ctx, session, StatusCanceled)
			if err != nil {
				return result, err
			}
			result.SessionsCanceled++
			if err := s.appendEvent(ctx, appendEventParams{
				ReviewSessionID: updated.ID,
				Type:            "ReviewSessionCanceled",
				Level:           "warn",
				Payload: map[string]any{
					"previous_status": session.Status,
					"status":          updated.Status,
					"reason":          "backend restarted while cancellation was pending",
				},
			}); err != nil {
				return result, err
			}
			result.SessionEvents++
		case StatusQueued, StatusRunning:
			updated, err := s.reconcileSessionStatus(ctx, session, StatusPaused)
			if err != nil {
				return result, err
			}
			result.SessionsPaused++
			if err := s.appendEvent(ctx, appendEventParams{
				ReviewSessionID: updated.ID,
				Type:            "ReviewSessionReconciled",
				Level:           "warn",
				Payload: map[string]any{
					"previous_status": session.Status,
					"status":          updated.Status,
					"resume_from":     lastCompletedPhaseName(s.loadCheckpointBestEffort(ctx, updated.ID).CompletedPhases),
					"reason":          "backend restarted before the local review session reached a terminal state",
				},
			}); err != nil {
				return result, err
			}
			result.SessionEvents++
		}
	}
	return result, nil
}

func (s *Service) Cancel(ctx context.Context, reviewSessionID string) (dbgen.ReviewSession, error) {
	if err := s.validate(); err != nil {
		return dbgen.ReviewSession{}, err
	}
	ctx = contextOrBackground(ctx)
	reviewSessionID = strings.TrimSpace(reviewSessionID)
	if reviewSessionID == "" {
		return dbgen.ReviewSession{}, fmt.Errorf("%w: review session id is required", ErrInvalidStatusTransition)
	}
	current, err := s.Queries.GetReviewSession(ctx, reviewSessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.ReviewSession{}, ErrReviewSessionNotFound
		}
		return dbgen.ReviewSession{}, fmt.Errorf("read review session: %w", err)
	}
	if current.Status == StatusCanceling || current.Status == StatusCanceled {
		return current, nil
	}
	nextStatus := StatusCanceling
	if current.Status == StatusQueued {
		nextStatus = StatusCanceled
	}
	updated, err := s.Transition(ctx, reviewSessionID, nextStatus)
	if err != nil {
		return dbgen.ReviewSession{}, err
	}
	if err := s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: reviewSessionID,
		Type:            "ReviewSessionCancelRequested",
		Payload: map[string]any{
			"previous_status": current.Status,
			"status":          updated.Status,
		},
	}); err != nil {
		return dbgen.ReviewSession{}, err
	}
	runs, err := s.Queries.ListAgentRunsBySession(ctx, reviewSessionID)
	if err != nil {
		return dbgen.ReviewSession{}, fmt.Errorf("list agent runs: %w", err)
	}
	canceledRuns := 0
	for _, run := range runs {
		if run.Status != agentrun.RunStatusQueued && run.Status != agentrun.RunStatusRunning {
			continue
		}
		if err := s.AgentManager.Cancel(ctx, run.ID); err != nil && !errors.Is(err, agentrun.ErrRunNotActive) {
			return dbgen.ReviewSession{}, fmt.Errorf("cancel agent run %s: %w", run.ID, err)
		}
		canceledRuns++
	}
	if updated.Status == StatusCanceled {
		if err := s.appendEvent(ctx, appendEventParams{
			ReviewSessionID: reviewSessionID,
			Type:            "ReviewSessionCanceled",
			Payload: map[string]any{
				"canceled_agent_runs": canceledRuns,
				"status":              updated.Status,
			},
		}); err != nil {
			return dbgen.ReviewSession{}, err
		}
	}
	return updated, nil
}

func (s *Service) CancelAgentRun(ctx context.Context, reviewSessionID string, agentRunID string) (dbgen.AgentRun, error) {
	if err := s.validate(); err != nil {
		return dbgen.AgentRun{}, err
	}
	ctx = contextOrBackground(ctx)
	reviewSessionID = strings.TrimSpace(reviewSessionID)
	agentRunID = strings.TrimSpace(agentRunID)
	if reviewSessionID == "" || agentRunID == "" {
		return dbgen.AgentRun{}, fmt.Errorf("%w: review session id and agent run id are required", ErrInvalidStatusTransition)
	}
	if _, err := s.Queries.GetReviewSession(ctx, reviewSessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.AgentRun{}, ErrReviewSessionNotFound
		}
		return dbgen.AgentRun{}, fmt.Errorf("read review session: %w", err)
	}
	run, err := s.Queries.GetAgentRun(ctx, agentRunID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.AgentRun{}, ErrAgentRunNotFound
		}
		return dbgen.AgentRun{}, fmt.Errorf("read agent run: %w", err)
	}
	if run.ReviewSessionID != reviewSessionID {
		return dbgen.AgentRun{}, ErrAgentRunNotFound
	}
	if run.Status != agentrun.RunStatusQueued && run.Status != agentrun.RunStatusRunning {
		return run, nil
	}
	if err := s.AgentManager.Cancel(ctx, run.ID); err != nil {
		if errors.Is(err, agentrun.ErrRunNotActive) {
			return run, nil
		}
		return dbgen.AgentRun{}, fmt.Errorf("cancel agent run %s: %w", run.ID, err)
	}
	if err := s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: reviewSessionID,
		AgentRunID:      nullableEventString(run.ID),
		Type:            "AgentRunCancelRequested",
		Payload: map[string]any{
			"agent_run_id":    run.ID,
			"agent_config_id": run.AgentConfigID,
			"status":          run.Status,
		},
	}); err != nil {
		return dbgen.AgentRun{}, err
	}
	return run, nil
}

func (s *Service) Pause(ctx context.Context, reviewSessionID string) (dbgen.ReviewSession, error) {
	if err := s.validate(); err != nil {
		return dbgen.ReviewSession{}, err
	}
	session, err := s.Transition(ctx, reviewSessionID, StatusPaused)
	if err != nil {
		return dbgen.ReviewSession{}, err
	}
	if err := s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: session.ID,
		Type:            "ReviewSessionPaused",
		Payload: map[string]any{
			"status": session.Status,
			"scope":  "phase_boundary",
		},
	}); err != nil {
		return dbgen.ReviewSession{}, err
	}
	return session, nil
}

func (s *Service) Resume(ctx context.Context, reviewSessionID string) (dbgen.ReviewSession, error) {
	if err := s.validate(); err != nil {
		return dbgen.ReviewSession{}, err
	}
	ctx = contextOrBackground(ctx)
	session, err := s.Transition(ctx, reviewSessionID, StatusRunning)
	if err != nil {
		return dbgen.ReviewSession{}, err
	}
	if err := s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: session.ID,
		Type:            "ReviewSessionResumed",
		Payload: map[string]any{
			"status": session.Status,
		},
	}); err != nil {
		return dbgen.ReviewSession{}, err
	}
	if s.registerActiveSession(session.ID) {
		go func() {
			defer s.unregisterActiveSession(session.ID)
			bg := s.background()
			_ = s.handleRunError(bg, session.ID, s.runWorkflow(bg, session))
		}()
	}
	return session, nil
}

func (s *Service) Transition(ctx context.Context, reviewSessionID string, next string) (dbgen.ReviewSession, error) {
	if err := s.validateQueries(); err != nil {
		return dbgen.ReviewSession{}, err
	}
	ctx = contextOrBackground(ctx)
	reviewSessionID = strings.TrimSpace(reviewSessionID)
	next = strings.TrimSpace(next)
	if reviewSessionID == "" || next == "" {
		return dbgen.ReviewSession{}, fmt.Errorf("%w: review session id and next status are required", ErrInvalidStatusTransition)
	}
	current, err := s.Queries.GetReviewSession(ctx, reviewSessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.ReviewSession{}, ErrReviewSessionNotFound
		}
		return dbgen.ReviewSession{}, fmt.Errorf("read review session: %w", err)
	}
	if !CanTransition(current.Status, next) {
		return dbgen.ReviewSession{}, fmt.Errorf("%w: %s -> %s", ErrInvalidStatusTransition, current.Status, next)
	}

	now := s.now().Format(time.RFC3339Nano)
	startedAt := current.StartedAt
	completedAt := current.CompletedAt
	if next == StatusRunning && !startedAt.Valid {
		startedAt = nullableString(now)
	}
	if terminalStatus(next) {
		completedAt = nullableString(now)
	}
	updated, err := s.Queries.UpdateReviewSessionStatusIfCurrent(ctx, dbgen.UpdateReviewSessionStatusIfCurrentParams{
		Status:      next,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
		UpdatedAt:   now,
		ID:          current.ID,
		Status_2:    current.Status,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.ReviewSession{}, fmt.Errorf("%w: %s -> %s", ErrInvalidStatusTransition, current.Status, next)
		}
		return dbgen.ReviewSession{}, fmt.Errorf("update review session status: %w", err)
	}
	return updated, nil
}

func CanTransition(current string, next string) bool {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" || next == "" || current == next {
		return false
	}
	switch current {
	case StatusDraft:
		return next == StatusQueued
	case StatusQueued:
		return next == StatusRunning || next == StatusCanceled || next == StatusFailed
	case StatusRunning:
		return next == StatusPaused || next == StatusCanceling || next == StatusCompleted || next == StatusFailed
	case StatusPaused:
		return next == StatusRunning || next == StatusCanceling
	case StatusCanceling:
		return next == StatusCanceled || next == StatusFailed
	default:
		return false
	}
}

func (s *Service) cancelInterruptedAgentRun(ctx context.Context, run dbgen.AgentRun) (dbgen.AgentRun, error) {
	if run.Status != agentrun.RunStatusQueued && run.Status != agentrun.RunStatusRunning {
		return run, nil
	}
	now := s.now().Format(time.RFC3339Nano)
	completedAt := nullableString(now)
	durationMs := run.DurationMs
	if run.StartedAt.Valid {
		if startedAt, err := time.Parse(time.RFC3339Nano, run.StartedAt.String); err == nil {
			durationMs = sql.NullInt64{Int64: maxInt64(0, s.now().Sub(startedAt).Milliseconds()), Valid: true}
		}
	}
	updated, err := s.Queries.UpdateAgentRunStatus(ctx, dbgen.UpdateAgentRunStatusParams{
		ID:                     run.ID,
		Status:                 agentrun.RunStatusCanceled,
		StartedAt:              run.StartedAt,
		CompletedAt:            completedAt,
		DurationMs:             durationMs,
		ExitCode:               run.ExitCode,
		StdoutArtifactID:       run.StdoutArtifactID,
		StderrArtifactID:       run.StderrArtifactID,
		ParsedOutputArtifactID: run.ParsedOutputArtifactID,
		ErrorCode:              nullableString("app_restarted"),
		ErrorMessage:           nullableString("backend restarted before the local agent run reached a terminal state"),
		MetadataJson:           run.MetadataJson,
	})
	if err != nil {
		return dbgen.AgentRun{}, fmt.Errorf("cancel interrupted agent run %s: %w", run.ID, err)
	}
	return updated, nil
}

func (s *Service) reconcileSessionStatus(ctx context.Context, current dbgen.ReviewSession, next string) (dbgen.ReviewSession, error) {
	now := s.now().Format(time.RFC3339Nano)
	completedAt := current.CompletedAt
	if terminalStatus(next) {
		completedAt = nullableString(now)
	}
	updated, err := s.Queries.UpdateReviewSessionStatusIfCurrent(ctx, dbgen.UpdateReviewSessionStatusIfCurrentParams{
		Status:      next,
		StartedAt:   current.StartedAt,
		CompletedAt: completedAt,
		UpdatedAt:   now,
		ID:          current.ID,
		Status_2:    current.Status,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.ReviewSession{}, fmt.Errorf("%w: %s -> %s", ErrInvalidStatusTransition, current.Status, next)
		}
		return dbgen.ReviewSession{}, fmt.Errorf("reconcile review session %s: %w", current.ID, err)
	}
	return updated, nil
}

func (s *Service) loadCheckpointBestEffort(ctx context.Context, reviewSessionID string) Checkpoint {
	checkpoint, err := s.LoadCheckpoint(ctx, reviewSessionID)
	if err != nil {
		return Checkpoint{}
	}
	return checkpoint
}

func (s *Service) LoadCheckpoint(ctx context.Context, reviewSessionID string) (Checkpoint, error) {
	if err := s.validate(); err != nil {
		return Checkpoint{}, err
	}
	ctx = contextOrBackground(ctx)
	session, err := s.Queries.GetReviewSession(ctx, strings.TrimSpace(reviewSessionID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Checkpoint{}, ErrReviewSessionNotFound
		}
		return Checkpoint{}, fmt.Errorf("read review session: %w", err)
	}
	events, err := s.Events.ListByReviewSession(ctx, session.ID)
	if err != nil {
		return Checkpoint{}, err
	}
	checkpoint := Checkpoint{
		ReviewSessionID: session.ID,
		Status:          session.Status,
		StartedAt:       nullableValue(session.StartedAt),
		CompletedAt:     nullableValue(session.CompletedAt),
		UpdatedAt:       session.UpdatedAt,
	}
	completed := map[string]bool{}
	for _, event := range events {
		checkpoint.LastSequence = event.Sequence
		var payload map[string]any
		_ = json.Unmarshal([]byte(event.PayloadJson), &payload)
		phase, _ := payload["phase"].(string)
		switch event.Type {
		case "WorkflowPhaseStarted":
			if phase != "" {
				checkpoint.Phase = phase
				checkpoint.PhaseStatus = "running"
			}
		case "WorkflowPhaseCompleted":
			if phase != "" {
				checkpoint.Phase = phase
				checkpoint.PhaseStatus = "completed"
				completed[phase] = true
			}
		case "WorkflowPhaseFailed":
			if phase != "" {
				checkpoint.Phase = phase
				checkpoint.PhaseStatus = "failed"
			}
		}
	}
	for _, phase := range workflowPhases() {
		if completed[phase] {
			checkpoint.CompletedPhases = append(checkpoint.CompletedPhases, phase)
		}
	}
	return checkpoint, nil
}

func (s *Service) Summary(ctx context.Context, reviewSessionID string) (RunSummary, error) {
	if err := s.validate(); err != nil {
		return RunSummary{}, err
	}
	ctx = contextOrBackground(ctx)
	session, err := s.Queries.GetReviewSession(ctx, strings.TrimSpace(reviewSessionID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RunSummary{}, ErrReviewSessionNotFound
		}
		return RunSummary{}, fmt.Errorf("read review session: %w", err)
	}
	checkpoint, err := s.LoadCheckpoint(ctx, session.ID)
	if err != nil {
		return RunSummary{}, err
	}
	changedFiles, err := s.Queries.ListChangedFilesBySnapshot(ctx, session.SnapshotID)
	if err != nil {
		return RunSummary{}, fmt.Errorf("list changed files: %w", err)
	}
	bundles, err := s.Queries.ListContextBundlesBySession(ctx, session.ID)
	if err != nil {
		return RunSummary{}, fmt.Errorf("list context bundles: %w", err)
	}
	runs, err := s.Queries.ListAgentRunsBySession(ctx, session.ID)
	if err != nil {
		return RunSummary{}, fmt.Errorf("list agent runs: %w", err)
	}
	candidates, err := s.Queries.ListFindingCandidatesBySession(ctx, session.ID)
	if err != nil {
		return RunSummary{}, fmt.Errorf("list finding candidates: %w", err)
	}
	findings, err := s.Queries.ListFindingsBySession(ctx, session.ID)
	if err != nil {
		return RunSummary{}, fmt.Errorf("list findings: %w", err)
	}

	agentStatusCounts := map[string]int{}
	activeAgents := 0
	agentRuns := make([]AgentRun, 0, len(runs))
	for _, run := range runs {
		agentStatusCounts[run.Status]++
		if run.Status == agentrun.RunStatusQueued || run.Status == agentrun.RunStatusRunning {
			activeAgents++
		}
		agentRuns = append(agentRuns, agentRunSummary(run))
	}
	findingCounts := FindingCounts{
		Candidates:           len(candidates),
		Findings:             len(findings),
		BySeverity:           map[string]int{},
		ByVerificationStatus: map[string]int{},
		ByDecisionStatus:     map[string]int{},
	}
	for _, finding := range findings {
		findingCounts.BySeverity[finding.Severity]++
		findingCounts.ByVerificationStatus[finding.VerificationStatus]++
		findingCounts.ByDecisionStatus[finding.DecisionStatus]++
	}
	filesScanned := 0
	if len(bundles) > 0 || phaseCompleted(checkpoint.CompletedPhases, PhaseBuildContext) {
		filesScanned = len(changedFiles)
	}
	return RunSummary{
		ReviewSessionID:     session.ID,
		Status:              session.Status,
		Phase:               checkpoint.Phase,
		PhaseStatus:         checkpoint.PhaseStatus,
		ProgressPercent:     progressPercent(session.Status, checkpoint.CompletedPhases),
		ChangedFilesTotal:   len(changedFiles),
		ChangedFilesScanned: filesScanned,
		AgentRunsTotal:      len(runs),
		ActiveAgents:        activeAgents,
		AgentStatusCounts:   agentStatusCounts,
		AgentRuns:           agentRuns,
		FindingCounts:       findingCounts,
		UpdatedAt:           session.UpdatedAt,
	}, nil
}

func (s *Service) run(ctx context.Context, reviewSessionID string) error {
	session, err := s.Transition(ctx, reviewSessionID, StatusRunning)
	if err != nil {
		if canceled, checkErr := s.cancellationRequested(ctx, reviewSessionID); checkErr == nil && canceled {
			return ErrReviewSessionCanceled
		}
		return err
	}
	if err := s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: session.ID,
		Type:            "ReviewSessionStarted",
		Payload:         map[string]any{"status": session.Status},
	}); err != nil {
		return err
	}
	return s.runWorkflow(ctx, session)
}

func (s *Service) runWorkflow(ctx context.Context, session dbgen.ReviewSession) error {
	repository, err := s.Queries.GetRepository(ctx, session.RepositoryID)
	if err != nil {
		return fmt.Errorf("read repository: %w", err)
	}
	workspace, err := s.Queries.GetWorkspace(ctx, session.WorkspaceID)
	if err != nil {
		return fmt.Errorf("read workspace: %w", err)
	}
	sessionAgents, err := s.enabledSessionAgents(ctx, session.ID)
	if err != nil {
		return err
	}
	if len(sessionAgents) == 0 {
		return ErrNoEnabledReviewAgents
	}

	checkpoint, err := s.LoadCheckpoint(ctx, session.ID)
	if err != nil {
		return err
	}
	runContexts := make([]runContext, 0, len(sessionAgents))
	if phaseCompleted(checkpoint.CompletedPhases, PhaseBuildContext) {
		runContexts, err = s.loadRunContextsFromPersistedBundles(ctx, session, repository, workspace, sessionAgents)
		if err != nil {
			return err
		}
	} else if err := s.withPhase(ctx, session.ID, PhaseBuildContext, func() error {
		for _, sessionAgent := range sessionAgents {
			agentConfig, err := s.Queries.GetAgentConfig(ctx, sessionAgent.AgentConfigID)
			if err != nil {
				return fmt.Errorf("read agent config %s: %w", sessionAgent.AgentConfigID, err)
			}
			if agentConfig.Enabled == 0 {
				return fmt.Errorf("%w: agent config %s is disabled", ErrInvalidAgentConfiguration, agentConfig.ID)
			}
			built, err := s.ContextBuilder.BuildReviewContext(ctx, contextbundle.BuildReviewContextParams{
				ReviewSessionID: session.ID,
				AgentConfigID:   agentConfig.ID,
				Persist:         true,
			})
			if err != nil {
				return fmt.Errorf("build context for agent %s: %w", agentConfig.ID, err)
			}
			if err := s.appendEvent(ctx, appendEventParams{
				ReviewSessionID: session.ID,
				Type:            "ContextBundleCreated",
				ArtifactID:      nullableEventString(built.Bundle.ArtifactID),
				Payload: map[string]any{
					"phase":             PhaseBuildContext,
					"agent_config_id":   agentConfig.ID,
					"context_bundle_id": built.Bundle.ID,
					"item_count":        built.Bundle.ItemCount,
					"token_estimate":    built.Bundle.TokenEstimate,
					"warnings":          built.Warnings,
				},
			}); err != nil {
				return err
			}
			runContexts = append(runContexts, runContext{
				Session:      session,
				Repository:   repository,
				Workspace:    workspace,
				SessionAgent: sessionAgent,
				AgentConfig:  agentConfig,
				Bundle:       built.Bundle,
				BundleText:   contextbundle.RenderBundle(built.Bundle),
			})
		}
		return nil
	}); err != nil {
		return err
	}
	if canceled, err := s.cancellationRequested(ctx, session.ID); err != nil {
		return err
	} else if canceled {
		return s.completeCanceled(ctx, session.ID)
	}
	if err := s.waitWhilePaused(ctx, session.ID); err != nil {
		return err
	}

	var scout localReviewScout
	if phaseCompleted(checkpoint.CompletedPhases, PhaseScoutRisk) {
		scout, err = s.buildLocalReviewScout(ctx, session, repository)
		if err != nil {
			return err
		}
	} else if err := s.withPhase(ctx, session.ID, PhaseScoutRisk, func() error {
		var buildErr error
		scout, buildErr = s.buildLocalReviewScout(ctx, session, repository)
		if buildErr != nil {
			return buildErr
		}
		return s.recordLocalReviewScout(ctx, session, scout)
	}); err != nil {
		return err
	}
	for i := range runContexts {
		runContexts[i].Scout = scout
	}
	if canceled, err := s.cancellationRequested(ctx, session.ID); err != nil {
		return err
	} else if canceled {
		return s.completeCanceled(ctx, session.ID)
	}
	if err := s.waitWhilePaused(ctx, session.ID); err != nil {
		return err
	}

	failedRuns := 0
	succeededRuns := 0
	runResults := []agentrun.RunResult{}
	if phaseCompleted(checkpoint.CompletedPhases, PhaseRunAgents) {
		runResults, err = s.loadReviewAgentRunResults(ctx, session.ID)
		if err != nil {
			return err
		}
		for _, result := range runResults {
			if result.Run.Status == agentrun.RunStatusSucceeded {
				succeededRuns++
			} else {
				failedRuns++
			}
		}
	} else if err := s.withPhase(ctx, session.ID, PhaseRunAgents, func() error {
		results, err := s.runAgents(ctx, runContexts)
		if err != nil {
			return err
		}
		runResults = results
		for _, result := range results {
			if result.Run.Status == agentrun.RunStatusSucceeded {
				succeededRuns++
			} else {
				failedRuns++
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if canceled, err := s.cancellationRequested(ctx, session.ID); err != nil {
		return err
	} else if canceled {
		return s.completeCanceled(ctx, session.ID)
	}
	if err := s.waitWhilePaused(ctx, session.ID); err != nil {
		return err
	}
	if failedRuns > 0 {
		if err := s.appendEvent(ctx, appendEventParams{
			ReviewSessionID: session.ID,
			Type:            "ReviewSessionPartialFailure",
			Level:           "warn",
			Payload: map[string]any{
				"failed_agent_runs":    failedRuns,
				"succeeded_agent_runs": succeededRuns,
			},
		}); err != nil {
			return err
		}
	}
	if succeededRuns == 0 {
		return fmt.Errorf("all review agent runs failed")
	}

	for _, phase := range []string{
		PhaseNormalizeOutputs,
		PhaseDeduplicate,
		PhaseVerifyFindings,
		PhaseBuildEvidence,
		PhaseDraftComments,
	} {
		if canceled, err := s.cancellationRequested(ctx, session.ID); err != nil {
			return err
		} else if canceled {
			return s.completeCanceled(ctx, session.ID)
		}
		if err := s.waitWhilePaused(ctx, session.ID); err != nil {
			return err
		}
		if phaseCompleted(checkpoint.CompletedPhases, phase) {
			continue
		}
		runPhase := func() error { return nil }
		switch phase {
		case PhaseNormalizeOutputs:
			runPhase = func() error {
				return s.normalizeAgentOutputs(ctx, session, runResults)
			}
		case PhaseDeduplicate:
			runPhase = func() error {
				return s.deduplicateFindings(ctx, session)
			}
		case PhaseVerifyFindings:
			runPhase = func() error {
				return s.verifyFindings(ctx, session, repository)
			}
		case PhaseBuildEvidence:
			runPhase = func() error {
				return s.buildEvidenceMaps(ctx, session)
			}
		}
		if err := s.withPhase(ctx, session.ID, phase, runPhase); err != nil {
			return err
		}
	}
	if canceled, err := s.cancellationRequested(ctx, session.ID); err != nil {
		return err
	} else if canceled {
		return s.completeCanceled(ctx, session.ID)
	}
	if err := s.waitWhilePaused(ctx, session.ID); err != nil {
		return err
	}

	completed, err := s.Transition(ctx, session.ID, StatusCompleted)
	if err != nil {
		return err
	}
	return s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: session.ID,
		Type:            "ReviewSessionCompleted",
		Payload: map[string]any{
			"status": completed.Status,
		},
	})
}

func (s *Service) waitWhilePaused(ctx context.Context, reviewSessionID string) error {
	for {
		session, err := s.Queries.GetReviewSession(ctx, reviewSessionID)
		if err != nil {
			return fmt.Errorf("read review session pause state: %w", err)
		}
		switch session.Status {
		case StatusPaused:
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		case StatusCanceling, StatusCanceled:
			return s.completeCanceled(ctx, reviewSessionID)
		default:
			return nil
		}
	}
}

func (s *Service) cancellationRequested(ctx context.Context, reviewSessionID string) (bool, error) {
	session, err := s.Queries.GetReviewSession(ctx, reviewSessionID)
	if err != nil {
		return false, fmt.Errorf("read review session cancellation state: %w", err)
	}
	return session.Status == StatusCanceling || session.Status == StatusCanceled, nil
}

func (s *Service) completeCanceled(ctx context.Context, reviewSessionID string) error {
	session, err := s.Queries.GetReviewSession(ctx, reviewSessionID)
	if err != nil {
		return fmt.Errorf("read review session before cancel completion: %w", err)
	}
	if session.Status == StatusCanceled {
		return ErrReviewSessionCanceled
	}
	canceled, err := s.Transition(ctx, reviewSessionID, StatusCanceled)
	if err != nil {
		return err
	}
	if err := s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: reviewSessionID,
		Type:            "ReviewSessionCanceled",
		Payload: map[string]any{
			"status": canceled.Status,
		},
	}); err != nil {
		return err
	}
	return ErrReviewSessionCanceled
}

func (s *Service) runAgents(ctx context.Context, items []runContext) ([]agentrun.RunResult, error) {
	type runAgentResult struct {
		index  int
		result agentrun.RunResult
		err    error
	}

	results := make([]agentrun.RunResult, len(items))
	resultCh := make(chan runAgentResult, len(items))
	for index, item := range items {
		go func(index int, item runContext) {
			result, err := s.runAgent(ctx, item)
			resultCh <- runAgentResult{index: index, result: result, err: err}
		}(index, item)
	}

	var errs []error
	for range items {
		item := <-resultCh
		results[item.index] = item.result
		if item.err != nil {
			errs = append(errs, item.err)
		}
	}
	return results, errors.Join(errs...)
}

func (s *Service) runAgent(ctx context.Context, item runContext) (agentrun.RunResult, error) {
	config, limits, err := s.connectionConfig(item)
	if err != nil {
		return agentrun.RunResult{}, err
	}
	capabilities, err := agentCapabilities(item.AgentConfig)
	if err != nil {
		return agentrun.RunResult{}, err
	}
	taskID := s.newID("agent_task_")
	runID := s.newID("agent_run_")
	taskRole := firstNonEmptyString(item.SessionAgent.Role, item.AgentConfig.Role)
	renderedPrompt, err := s.renderReviewPrompt(item)
	if err != nil {
		return agentrun.RunResult{}, err
	}
	promptArtifactID, err := s.persistRenderedPrompt(ctx, item, runID, renderedPrompt)
	if err != nil {
		return agentrun.RunResult{}, err
	}
	promptMetadata := renderedPrompt.MetadataMap(promptArtifactID)
	task := agents.AgentTask{
		ID:               taskID,
		RunID:            runID,
		ReviewSessionID:  item.Session.ID,
		AgentConfigID:    item.AgentConfig.ID,
		ContextBundleID:  item.Bundle.ID,
		Role:             taskRole,
		Prompt:           renderedPrompt.Text,
		ContextArtifacts: contextArtifactRefs(item.Bundle),
		RepositoryRoot:   item.Repository.LocalPath,
		WorkspaceRoot:    item.Workspace.RootPath,
		Limits:           limits,
		Metadata: map[string]any{
			"review_session_agent_id": item.SessionAgent.ID,
			"context_bundle_id":       item.Bundle.ID,
		},
	}
	for key, value := range promptMetadata {
		task.Metadata[key] = value
	}
	reviewDeadline := time.Time{}
	if item.Session.RuntimeLimitSeconds > 0 && item.Session.StartedAt.Valid {
		if startedAt, err := time.Parse(time.RFC3339Nano, item.Session.StartedAt.String); err == nil {
			reviewDeadline = startedAt.Add(time.Duration(item.Session.RuntimeLimitSeconds) * time.Second)
		}
	}
	runMetadata := map[string]any{
		"phase":                   PhaseRunAgents,
		"review_session_agent_id": item.SessionAgent.ID,
		"context_bundle_id":       item.Bundle.ID,
		"output_mode":             string(agents.OutputMode(item.AgentConfig.OutputMode)),
	}
	for key, value := range promptMetadata {
		runMetadata[key] = value
	}
	copyStringMetadata(runMetadata, config.Metadata, "model_label")
	copyStringMetadata(runMetadata, config.Metadata, "reasoning_label")
	result, err := s.AgentManager.Execute(ctx, agentrun.RunParams{
		WorkspaceID:  item.Workspace.ID,
		Config:       config,
		Capabilities: capabilities,
		Permissions:  agents.ReviewModePermissionPolicy(),
		Task:         task,
		TimeoutPolicy: agentrun.TimeoutPolicy{
			AgentTimeoutSeconds:  limits.TimeoutSeconds,
			ReviewDeadline:       reviewDeadline,
			ReviewTimeoutSeconds: maxInt64(0, item.Session.RuntimeLimitSeconds),
		},
		Metadata:  runMetadata,
		EventSink: s.agentRunEventSink(item.Session.ID),
	})
	if err != nil {
		return result, err
	}
	if err := s.parseAgentOutputForPhase(ctx, item, &result, PhaseNormalizeOutputs); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) parseAgentOutputForPhase(ctx context.Context, item runContext, result *agentrun.RunResult, phase string) error {
	if !result.Run.StdoutArtifactID.Valid {
		return nil
	}
	content, _, err := s.Artifacts.Read(ctx, result.Run.StdoutArtifactID.String)
	if err != nil {
		return fmt.Errorf("read stdout artifact for parsing: %w", err)
	}
	outputMode := agents.OutputMode(item.AgentConfig.OutputMode)
	parsed := agentoutput.Parse(content, outputMode)
	encoded, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return fmt.Errorf("encode parsed output: %w", err)
	}
	createdAt := s.now().Format(time.RFC3339Nano)
	metadata, err := json.Marshal(map[string]any{
		"agent_run_id":            result.Run.ID,
		"agent_config_id":         item.AgentConfig.ID,
		"mode":                    string(outputMode),
		"structured":              parsed.Structured,
		"diagnostics":             len(parsed.Diagnostics),
		"source_trust":            "untrusted_agent_output",
		"side_effects_allowed":    false,
		"requires_human_decision": true,
	})
	if err != nil {
		return fmt.Errorf("encode parsed output metadata: %w", err)
	}
	artifactRow, err := s.Artifacts.Save(ctx, artifact.SaveParams{
		ID:              s.artifactID(),
		WorkspaceID:     item.Workspace.ID,
		ReviewSessionID: nullableEventString(item.Session.ID),
		Kind:            "parsed_output",
		RelativePath:    fmt.Sprintf("review-sessions/%s/agent-runs/%s/parsed-output.json", item.Session.ID, result.Run.ID),
		ContentType:     "application/json",
		MetadataJSON:    string(metadata),
		CreatedAt:       createdAt,
	}, encoded)
	if err != nil {
		return fmt.Errorf("save parsed output artifact: %w", err)
	}
	result.Run.ParsedOutputArtifactID = nullableString(artifactRow.ID)
	result.Run, err = s.Queries.UpdateAgentRunStatus(ctx, dbgen.UpdateAgentRunStatusParams{
		ID:                     result.Run.ID,
		Status:                 result.Run.Status,
		StartedAt:              result.Run.StartedAt,
		CompletedAt:            result.Run.CompletedAt,
		DurationMs:             result.Run.DurationMs,
		ExitCode:               result.Run.ExitCode,
		StdoutArtifactID:       result.Run.StdoutArtifactID,
		StderrArtifactID:       result.Run.StderrArtifactID,
		ParsedOutputArtifactID: result.Run.ParsedOutputArtifactID,
		ErrorCode:              result.Run.ErrorCode,
		ErrorMessage:           result.Run.ErrorMessage,
		MetadataJson:           result.Run.MetadataJson,
	})
	if err != nil {
		return fmt.Errorf("attach parsed output artifact: %w", err)
	}
	return s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: item.Session.ID,
		AgentRunID:      nullableEventString(result.Run.ID),
		Type:            "AgentOutputParsed",
		ArtifactID:      nullableEventString(artifactRow.ID),
		Payload: map[string]any{
			"phase":                 phase,
			"agent_config_id":       item.AgentConfig.ID,
			"agent_run_id":          result.Run.ID,
			"parsed_artifact_id":    artifactRow.ID,
			"structured":            parsed.Structured,
			"document_count":        len(parsed.Documents),
			"diagnostic_count":      len(parsed.Diagnostics),
			"parsed_output_mode":    string(parsed.Mode),
			"requested_output_mode": string(outputMode),
			"source_trust":          "untrusted_agent_output",
			"side_effects_allowed":  false,
		},
	})
}

func (s *Service) normalizeAgentOutputs(ctx context.Context, session dbgen.ReviewSession, results []agentrun.RunResult) error {
	changedFiles, err := s.Queries.ListChangedFilesBySnapshot(ctx, session.SnapshotID)
	if err != nil {
		return fmt.Errorf("list changed files for candidate normalization: %w", err)
	}
	totalCandidates := 0
	totalDiagnostics := 0
	for _, result := range results {
		run := result.Run
		if run.Status != agentrun.RunStatusSucceeded || !run.ParsedOutputArtifactID.Valid {
			continue
		}
		content, _, err := s.Artifacts.Read(ctx, run.ParsedOutputArtifactID.String)
		if err != nil {
			return fmt.Errorf("read parsed output artifact %s: %w", run.ParsedOutputArtifactID.String, err)
		}
		var parsed agentoutput.ParsedOutput
		if err := json.Unmarshal(content, &parsed); err != nil {
			return fmt.Errorf("decode parsed output artifact %s: %w", run.ParsedOutputArtifactID.String, err)
		}
		extracted := agentoutput.ExtractCandidates(parsed)
		totalDiagnostics += len(extracted.Diagnostics)
		if len(extracted.Diagnostics) > 0 {
			if err := s.appendEvent(ctx, appendEventParams{
				ReviewSessionID: session.ID,
				AgentRunID:      nullableEventString(run.ID),
				Type:            "FindingNormalizationDiagnostics",
				Level:           "warn",
				ArtifactID:      run.ParsedOutputArtifactID,
				Payload: map[string]any{
					"phase":            PhaseNormalizeOutputs,
					"agent_run_id":     run.ID,
					"diagnostic_count": len(extracted.Diagnostics),
					"diagnostics":      extracted.Diagnostics,
				},
			}); err != nil {
				return err
			}
		}
		for _, candidate := range extracted.Candidates {
			created, err := s.createFindingCandidate(ctx, session, run, findingengine.NormalizeCandidate(candidate, changedFiles))
			if err != nil {
				return err
			}
			totalCandidates++
			if err := s.appendEvent(ctx, appendEventParams{
				ReviewSessionID: session.ID,
				AgentRunID:      nullableEventString(run.ID),
				Type:            "FindingCandidateCreated",
				ArtifactID:      run.StdoutArtifactID,
				Payload: map[string]any{
					"phase":                PhaseNormalizeOutputs,
					"agent_run_id":         run.ID,
					"finding_candidate_id": created.ID,
					"claim":                created.Claim,
					"category":             created.Category,
					"severity":             created.Severity,
					"confidence":           created.Confidence,
					"raw_artifact_id":      nullableValue(created.RawArtifactID),
					"candidate_trust":      "unverified_agent_claim",
					"source_trust":         "untrusted_agent_output",
					"side_effects_allowed": false,
				},
			}); err != nil {
				return err
			}
		}
	}
	return s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: session.ID,
		Type:            "FindingNormalized",
		Payload: map[string]any{
			"phase":                PhaseNormalizeOutputs,
			"candidate_count":      totalCandidates,
			"diagnostic_count":     totalDiagnostics,
			"agent_runs_scanned":   len(results),
			"source_trust":         "untrusted_agent_output",
			"side_effects_allowed": false,
		},
	})
}

func (s *Service) createFindingCandidate(ctx context.Context, session dbgen.ReviewSession, run dbgen.AgentRun, candidate agentoutput.Candidate) (dbgen.FindingCandidate, error) {
	locationsJSON, err := json.Marshal(candidate.Locations)
	if err != nil {
		return dbgen.FindingCandidate{}, fmt.Errorf("encode candidate locations: %w", err)
	}
	evidenceJSON, err := json.Marshal(candidate.Evidence)
	if err != nil {
		return dbgen.FindingCandidate{}, fmt.Errorf("encode candidate evidence: %w", err)
	}
	created, err := s.Queries.CreateFindingCandidate(ctx, dbgen.CreateFindingCandidateParams{
		ID:               s.newID("finding_candidate_"),
		ReviewSessionID:  session.ID,
		AgentRunID:       run.ID,
		RawArtifactID:    run.StdoutArtifactID,
		Category:         candidate.Category,
		Severity:         candidate.Severity,
		Confidence:       candidate.Confidence,
		Claim:            candidate.Claim,
		PrimaryPath:      nullableString(candidate.PrimaryPath),
		PrimaryStartLine: nullablePositiveInt64(candidate.PrimaryStartLine),
		PrimaryEndLine:   nullablePositiveInt64(candidate.PrimaryEndLine),
		LocationsJson:    string(locationsJSON),
		EvidenceJson:     string(evidenceJSON),
		SuggestedFix:     nullableString(candidate.SuggestedFix),
		DraftComment:     nullableString(candidate.DraftComment),
		Fingerprint:      nullableString(candidate.Fingerprint),
		CreatedAt:        s.now().Format(time.RFC3339Nano),
	})
	if err != nil {
		return dbgen.FindingCandidate{}, fmt.Errorf("create finding candidate for run %s: %w", run.ID, err)
	}
	return created, nil
}

func (s *Service) deduplicateFindings(ctx context.Context, session dbgen.ReviewSession) error {
	candidates, err := s.Queries.ListFindingCandidatesBySession(ctx, session.ID)
	if err != nil {
		return fmt.Errorf("list finding candidates for dedupe: %w", err)
	}
	deterministicClusters := findingengine.Deduplicate(candidates)
	curation, err := s.refineDedupeClusters(ctx, session, candidates, deterministicClusters)
	if err != nil {
		return err
	}
	clusters := curation.Clusters
	snapshot, err := s.Queries.GetPullRequestSnapshot(ctx, session.SnapshotID)
	if err != nil {
		return fmt.Errorf("read snapshot for findings: %w", err)
	}
	repository, err := s.Queries.GetRepository(ctx, session.RepositoryID)
	if err != nil {
		return fmt.Errorf("read repository for findings: %w", err)
	}
	changedFiles, err := s.Queries.ListChangedFilesBySnapshot(ctx, session.SnapshotID)
	if err != nil {
		return fmt.Errorf("read changed files for findings: %w", err)
	}
	for _, cluster := range clusters {
		representative := findingengine.Representative(cluster)
		if representative.ID == "" || !representative.Fingerprint.Valid {
			continue
		}
		curated, hasCuration := curation.Curated[clusterKey(cluster)]
		representativePrimaryPath := representative.PrimaryPath
		representativePrimaryStartLine := representative.PrimaryStartLine
		representativePrimaryEndLine := representative.PrimaryEndLine
		canonicalClaim := representative.Claim
		category := representative.Category
		severity := representative.Severity
		confidence := representative.Confidence
		consensusConfidence, consensusSourceAgents := findingengine.ConsensusConfidence(cluster)
		verificationStatus := evidence.StatusUnverified
		primaryPath := representative.PrimaryPath
		primaryStartLine := representative.PrimaryStartLine
		primaryEndLine := representative.PrimaryEndLine
		evidenceSummary := findingengine.EvidenceSummary(representative)
		counterEvidenceSummary := sql.NullString{}
		suggestedFix := representative.SuggestedFix
		draftComment := representative.DraftComment
		curatedLocationOverride := false
		curatorRequestedStatus := ""
		if hasCuration {
			if strings.TrimSpace(curated.CanonicalClaim) != "" {
				canonicalClaim = strings.TrimSpace(curated.CanonicalClaim)
			}
			if strings.TrimSpace(curated.Category) != "" {
				category = curated.Category
			}
			if strings.TrimSpace(curated.Severity) != "" {
				severity = curated.Severity
			}
			if curated.Confidence > 0 {
				confidence = curated.Confidence
			}
			if strings.TrimSpace(curated.VerificationStatus) != "" {
				verificationStatus = curated.VerificationStatus
				curatorRequestedStatus = curated.VerificationStatus
			}
			if strings.TrimSpace(curated.PrimaryPath) != "" {
				primaryPath = nullableString(curated.PrimaryPath)
				curatedLocationOverride = true
			}
			if curated.PrimaryStartLine > 0 {
				primaryStartLine = nullablePositiveInt64(curated.PrimaryStartLine)
				curatedLocationOverride = true
			}
			if curated.PrimaryEndLine > 0 {
				primaryEndLine = nullablePositiveInt64(curated.PrimaryEndLine)
				curatedLocationOverride = true
			}
			if strings.TrimSpace(curated.EvidenceSummary) != "" {
				evidenceSummary = nullableString(curated.EvidenceSummary)
			}
			if strings.TrimSpace(curated.CounterEvidenceSummary) != "" {
				counterEvidenceSummary = nullableString(curated.CounterEvidenceSummary)
			}
			if strings.TrimSpace(curated.SuggestedFix) != "" {
				suggestedFix = nullableString(curated.SuggestedFix)
			}
			if strings.TrimSpace(curated.DraftComment) != "" {
				draftComment = nullableString(curated.DraftComment)
			}
		}
		if consensusConfidence > confidence {
			confidence = consensusConfidence
		}
		primaryStartLine, primaryEndLine = refinePrimaryLocationFromCode(
			repository.LocalPath,
			primaryPath,
			primaryStartLine,
			primaryEndLine,
			canonicalClaim,
			evidenceSummary,
			suggestedFix,
		)
		anchorValidation := validatePrimaryChangedCodeAnchor(repository.LocalPath, changedFiles, primaryPath, primaryStartLine, primaryEndLine)
		anchorSource := "representative"
		curatedPrimaryRejected := false
		curatedStatusDowngraded := false
		if hasCuration {
			anchorSource = "curator"
		}
		if hasCuration && curatedLocationOverride && !anchorValidation.Valid {
			curatedPrimaryRejected = true
			fallbackStartLine, fallbackEndLine := refinePrimaryLocationFromCode(
				repository.LocalPath,
				representativePrimaryPath,
				representativePrimaryStartLine,
				representativePrimaryEndLine,
				representative.Claim,
				findingengine.EvidenceSummary(representative),
				representative.SuggestedFix,
			)
			fallbackValidation := validatePrimaryChangedCodeAnchor(repository.LocalPath, changedFiles, representativePrimaryPath, fallbackStartLine, fallbackEndLine)
			if fallbackValidation.Valid {
				primaryPath = representativePrimaryPath
				primaryStartLine = fallbackStartLine
				primaryEndLine = fallbackEndLine
				anchorValidation = fallbackValidation
				anchorSource = "representative_fallback"
			}
		}
		if verificationStatus == evidence.StatusVerified && !anchorValidation.Valid {
			verificationStatus = evidence.StatusNeedsHuman
			curatedStatusDowngraded = true
		}
		if verificationStatus == evidence.StatusVerified && curatedPrimaryRejected && anchorValidation.Valid {
			verificationStatus = evidence.StatusLocallySupported
			curatedStatusDowngraded = true
		}
		now := s.now().Format(time.RFC3339Nano)
		finding, err := s.Queries.CreateFinding(ctx, dbgen.CreateFindingParams{
			ID:                     s.newID("finding_"),
			ReviewSessionID:        session.ID,
			CanonicalClaim:         canonicalClaim,
			Category:               category,
			Severity:               severity,
			Confidence:             confidence,
			VerificationStatus:     verificationStatus,
			DecisionStatus:         "undecided",
			PrimaryPath:            primaryPath,
			PrimaryStartLine:       primaryStartLine,
			PrimaryEndLine:         primaryEndLine,
			EvidenceSummary:        evidenceSummary,
			CounterEvidenceSummary: counterEvidenceSummary,
			SuggestedFix:           suggestedFix,
			DraftComment:           draftComment,
			Fingerprint:            representative.Fingerprint.String,
			MergedFromCount:        int64(len(cluster.Candidates)),
			IntroducedInSha:        snapshot.HeadSha,
			FirstSeenAt:            now,
			UpdatedAt:              now,
		})
		if err != nil {
			return fmt.Errorf("create canonical finding: %w", err)
		}
		for _, candidate := range cluster.Candidates {
			if err := s.Queries.LinkFindingCandidate(ctx, dbgen.LinkFindingCandidateParams{
				FindingID:          finding.ID,
				FindingCandidateID: candidate.ID,
				Relation:           candidateLinkRelation(representative, candidate),
			}); err != nil {
				return fmt.Errorf("link candidate %s to finding %s: %w", candidate.ID, finding.ID, err)
			}
		}
		curatedEvidenceItems := 0
		if hasCuration {
			curatedEvidenceItems, err = s.createCuratedEvidenceItems(ctx, finding, curated, repository.LocalPath)
			if err != nil {
				return err
			}
		}
		if err := s.appendEvent(ctx, appendEventParams{
			ReviewSessionID: session.ID,
			Type:            "FindingMerged",
			Payload: map[string]any{
				"phase":                     PhaseDeduplicate,
				"finding_id":                finding.ID,
				"fingerprint":               finding.Fingerprint,
				"candidate_count":           len(cluster.Candidates),
				"canonical_claim":           finding.CanonicalClaim,
				"merged_from_count":         finding.MergedFromCount,
				"curated":                   hasCuration,
				"curation_refiner":          curation.Refiner,
				"curator_agent_config_id":   curation.AgentConfigID,
				"curator_agent_run_id":      curation.AgentRunID,
				"curated_evidence_items":    curatedEvidenceItems,
				"curator_requested_status":  curatorRequestedStatus,
				"consensus_confidence":      consensusConfidence,
				"consensus_source_agents":   consensusSourceAgents,
				"primary_anchor_source":     anchorSource,
				"primary_anchor_valid":      anchorValidation.Valid,
				"primary_anchor_reason":     anchorValidation.Reason,
				"curated_location_override": curatedLocationOverride,
				"curated_primary_rejected":  curatedPrimaryRejected,
				"curated_status_downgraded": curatedStatusDowngraded,
			},
		}); err != nil {
			return err
		}
	}
	return s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: session.ID,
		Type:            "FindingDeduplicated",
		Payload: map[string]any{
			"phase":                   PhaseDeduplicate,
			"candidate_count":         len(candidates),
			"finding_count":           len(clusters),
			"refiner":                 curation.Refiner,
			"curator_agent_config_id": curation.AgentConfigID,
			"curator_agent_run_id":    curation.AgentRunID,
		},
	})
}

func (s *Service) refineDedupeClusters(ctx context.Context, session dbgen.ReviewSession, candidates []dbgen.FindingCandidate, deterministicClusters []findingengine.Cluster) (dedupeCurationResult, error) {
	fallback := defaultDedupeCuration(deterministicClusters)
	if len(candidates) == 0 {
		return fallback, nil
	}
	if s.DedupeHook == nil || !s.EnableDedupeHook {
		curation, attempted, err := s.runOrchestratorFindingCuration(ctx, session, candidates, deterministicClusters)
		if !attempted {
			return fallback, nil
		}
		if err != nil {
			if eventErr := s.appendEvent(ctx, appendEventParams{
				ReviewSessionID: session.ID,
				Type:            "FindingCurationFailed",
				Level:           "warn",
				Payload: map[string]any{
					"phase":                       PhaseDeduplicate,
					"candidate_count":             len(candidates),
					"deterministic_cluster_count": len(deterministicClusters),
					"error":                       err.Error(),
					"fallback":                    "deterministic_dedupe",
				},
			}); eventErr != nil {
				return dedupeCurationResult{}, eventErr
			}
			return fallback, nil
		}
		if err := s.appendEvent(ctx, appendEventParams{
			ReviewSessionID: session.ID,
			AgentRunID:      nullableEventString(curation.AgentRunID),
			Type:            "FindingDedupeRefined",
			Payload: map[string]any{
				"phase":                       PhaseDeduplicate,
				"candidate_count":             len(candidates),
				"deterministic_cluster_count": len(deterministicClusters),
				"refined_cluster_count":       len(curation.Clusters),
				"refiner":                     curation.Refiner,
				"agent_config_id":             curation.AgentConfigID,
				"agent_run_id":                curation.AgentRunID,
				"curated_findings":            len(curation.Curated),
			},
		}); err != nil {
			return dedupeCurationResult{}, err
		}
		return curation, nil
	}
	result, err := s.DedupeHook.RefineDedupe(ctx, findingengine.DedupeInput{
		ReviewSessionID:       session.ID,
		Candidates:            candidates,
		DeterministicClusters: deterministicClusters,
	})
	if err != nil {
		return dedupeCurationResult{}, fmt.Errorf("refine dedupe clusters: %w", err)
	}
	if err := findingengine.ValidateDedupeResult(candidates, result.Clusters); err != nil {
		return dedupeCurationResult{}, fmt.Errorf("refine dedupe clusters: %w", err)
	}
	if err := s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: session.ID,
		Type:            "FindingDedupeRefined",
		Payload: map[string]any{
			"phase":                       PhaseDeduplicate,
			"candidate_count":             len(candidates),
			"deterministic_cluster_count": len(deterministicClusters),
			"refined_cluster_count":       len(result.Clusters),
			"refiner":                     "dedupe_hook",
		},
	}); err != nil {
		return dedupeCurationResult{}, err
	}
	return dedupeCurationResult{
		Clusters: result.Clusters,
		Curated:  map[string]curatedFinding{},
		Refiner:  "dedupe_hook",
	}, nil
}

func candidateLinkRelation(representative dbgen.FindingCandidate, candidate dbgen.FindingCandidate) string {
	if representative.ID == candidate.ID {
		return "primary"
	}
	if representative.Fingerprint.Valid && candidate.Fingerprint.Valid && representative.Fingerprint.String == candidate.Fingerprint.String {
		return "exact_duplicate"
	}
	return "overlap_duplicate"
}

func (s *Service) verifyFindings(ctx context.Context, session dbgen.ReviewSession, repository dbgen.Repository) error {
	verifier := s.Evidence
	if verifier == nil {
		verifier = &evidence.Service{Queries: s.Queries}
	}
	summary, err := verifier.VerifySession(ctx, session, repository)
	if err != nil {
		return err
	}
	if err := s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: session.ID,
		Type:            "FindingVerificationCompleted",
		Payload: map[string]any{
			"phase":                  PhaseVerifyFindings,
			"finding_count":          summary.Findings,
			"evidence_items_created": summary.EvidenceItemsCreated,
			"by_verification_status": summary.ByVerificationStatus,
			"supporting_evidence":    summary.SupportingEvidence,
			"counter_evidence":       summary.CounterEvidence,
			"missing_evidence":       summary.MissingEvidence,
		},
	}); err != nil {
		return err
	}
	agentSummary, err := s.runVerifierAgents(ctx, session, repository)
	if err != nil {
		return err
	}
	return s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: session.ID,
		Type:            "VerifierAgentVerificationCompleted",
		Payload: map[string]any{
			"phase":                   PhaseVerifyFindings,
			"configured":              agentSummary.Configured,
			"findings_eligible":       agentSummary.FindingsEligible,
			"findings_attempted":      agentSummary.FindingsAttempted,
			"runs_started":            agentSummary.RunsStarted,
			"runs_succeeded":          agentSummary.RunsSucceeded,
			"runs_failed":             agentSummary.RunsFailed,
			"evidence_items_created":  agentSummary.EvidenceItemsCreated,
			"status_updates":          agentSummary.StatusUpdates,
			"by_verification_status":  agentSummary.ByVerificationStatus,
			"context_bundle_failures": agentSummary.ContextBundleFailures,
			"apply_failures":          agentSummary.ApplyFailures,
			"skipped":                 agentSummary.Skipped,
			"skip_reason":             agentSummary.SkipReason,
		},
	})
}

func (s *Service) buildEvidenceMaps(ctx context.Context, session dbgen.ReviewSession) error {
	builder := s.Evidence
	if builder == nil {
		builder = &evidence.Service{Queries: s.Queries}
	}
	summary, err := builder.BuildSessionEvidenceMaps(ctx, session)
	if err != nil {
		return err
	}
	return s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: session.ID,
		Type:            "EvidenceMapBuildCompleted",
		Payload: map[string]any{
			"phase":         PhaseBuildEvidence,
			"finding_count": summary.Findings,
			"ready":         summary.Ready,
			"partial":       summary.Partial,
			"nodes":         summary.Nodes,
			"edges":         summary.Edges,
			"by_status":     summary.ByStatus,
		},
	})
}

func (s *Service) connectionConfig(item runContext) (agents.ConnectionConfig, agents.TaskLimits, error) {
	args, err := agents.DecodeStringArray(item.AgentConfig.ArgsJson, "agent args")
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, fmt.Errorf("%w: %v", ErrInvalidAgentConfiguration, err)
	}
	envNames, err := agents.DecodeStringArray(item.AgentConfig.EnvAllowlistJson, "agent env_allowlist")
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, fmt.Errorf("%w: %v", ErrInvalidAgentConfiguration, err)
	}
	env, err := agents.ResolveAllowedEnvironment(envNames)
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, fmt.Errorf("%w: agent env_allowlist is invalid: %v", ErrInvalidAgentConfiguration, err)
	}
	settings, err := decodeRuntimeSettings(item.AgentConfig.SettingsJson)
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, err
	}
	selection, err := decodeSessionAgentSettingsOverride(item.SessionAgent.SettingsOverrideJson)
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, err
	}
	workingDirectory, err := workingDirectoryForAgent(item.AgentConfig.CwdMode, item.Repository, item.Workspace)
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, err
	}
	modelLabel := selectedAgentModelLabel(item, selection)
	reasoningLabel := selectedAgentReasoningLabel(item, selection)
	command := nullableValue(item.AgentConfig.Command)
	kind := agents.AdapterKind(item.AgentConfig.AdapterKind)
	args = agents.SanitizeCommandArgs(command, args)
	args = agents.CommandArgsWithModelSelection(kind, command, args, modelLabel, reasoningLabel)
	config := agents.ConnectionConfig{
		AdapterID:        item.AgentConfig.ID,
		Kind:             kind,
		Command:          command,
		Args:             args,
		PromptDelivery:   settings.PromptDelivery,
		CommandSafety:    agents.CommandSafetyOptions{AllowRiskyCommand: settings.AllowRiskyCommand},
		WorkingDirectory: workingDirectory,
		Env:              env,
		Metadata: map[string]any{
			"output_mode":     string(item.AgentConfig.OutputMode),
			"model_label":     modelLabel,
			"reasoning_label": reasoningLabel,
		},
	}
	limits := agents.TaskLimits{
		TimeoutSeconds: settings.TimeoutSeconds,
		MaxStdoutBytes: settings.MaxStdoutBytes,
		MaxStderrBytes: settings.MaxStderrBytes,
		MaxPromptBytes: settings.MaxPromptBytes,
	}
	return config, limits, nil
}

func agentCapabilities(config dbgen.AgentConfig) (agents.AgentCapabilities, error) {
	capabilities, err := agents.DecodeCapabilitiesJSON(config.CapabilitiesJson, agents.AdapterKind(config.AdapterKind))
	if err != nil {
		return agents.AgentCapabilities{}, fmt.Errorf("%w: agent capabilities are invalid", ErrInvalidAgentConfiguration)
	}
	return capabilities, nil
}

func (s *Service) buildLocalReviewScout(ctx context.Context, session dbgen.ReviewSession, repository dbgen.Repository) (localReviewScout, error) {
	files, err := s.Queries.ListChangedFilesBySnapshot(ctx, session.SnapshotID)
	if err != nil {
		return localReviewScout{}, fmt.Errorf("list changed files for local scout: %w", err)
	}
	return assessLocalReviewScout(session, repository, files, s.now()), nil
}

func (s *Service) recordLocalReviewScout(ctx context.Context, session dbgen.ReviewSession, scout localReviewScout) error {
	artifactID := sql.NullString{}
	if s.Artifacts != nil {
		content, err := json.MarshalIndent(scout, "", "  ")
		if err != nil {
			return fmt.Errorf("encode local scout artifact: %w", err)
		}
		metadata, err := json.Marshal(map[string]any{
			"review_session_id": session.ID,
			"phase":             PhaseScoutRisk,
			"source":            "local_scout",
		})
		if err != nil {
			return fmt.Errorf("encode local scout artifact metadata: %w", err)
		}
		saved, err := s.Artifacts.Save(ctx, artifact.SaveParams{
			ID:              s.artifactID(),
			WorkspaceID:     session.WorkspaceID,
			ReviewSessionID: nullableString(session.ID),
			Kind:            "review_scout",
			RelativePath:    filepath.ToSlash(filepath.Join("review-scout", session.ID+".json")),
			ContentType:     "application/json",
			MetadataJSON:    string(metadata),
			CreatedAt:       s.now().Format(time.RFC3339Nano),
		}, content)
		if err != nil {
			return fmt.Errorf("save local scout artifact: %w", err)
		}
		artifactID = nullableEventString(saved.ID)
	}
	return s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: session.ID,
		Type:            "ReviewScoutCompleted",
		ArtifactID:      artifactID,
		Payload: map[string]any{
			"phase":       PhaseScoutRisk,
			"risk_tier":   scout.OverallRisk,
			"risk_score":  scout.RiskScore,
			"profiles":    scout.Profiles,
			"lead_count":  len(scout.Leads),
			"summary":     scout.Summary,
			"artifact_id": nullableValue(artifactID),
		},
	})
}

func assessLocalReviewScout(session dbgen.ReviewSession, repository dbgen.Repository, files []dbgen.ChangedFile, now time.Time) localReviewScout {
	profileSet := map[string]bool{}
	leads := make([]localReviewLead, 0, len(files))
	ignored := make([]localIgnoredArea, 0)
	maxScore := 0
	totalRisk := 0
	focus := strings.ToLower(session.FocusPrompt.String)

	for _, file := range files {
		path := strings.TrimSpace(file.Path)
		if path == "" {
			continue
		}
		lowerPath := strings.ToLower(path)
		if file.IsExcluded != 0 {
			ignored = append(ignored, localIgnoredArea{Path: path, Reason: "excluded by context visibility policy"})
			continue
		}
		if file.IsBinary != 0 {
			ignored = append(ignored, localIgnoredArea{Path: path, Reason: "binary file"})
			continue
		}
		if file.IsGenerated != 0 {
			ignored = append(ignored, localIgnoredArea{Path: path, Reason: "generated file"})
			continue
		}

		score := 1
		signals := []string{}
		suggestedReviewer := "correctness_reviewer"
		addSignal := func(profile string, signal string, weight int) {
			if profile != "" {
				profileSet[profile] = true
			}
			signals = append(signals, signal)
			score += weight
		}

		switch {
		case containsAny(lowerPath, "auth", "permission", "rbac", "jwt", "token", "secret", "crypto", "oauth", "acl", "security"):
			addSignal("security", "security-sensitive path", 4)
			suggestedReviewer = "security_reviewer"
		case containsAny(lowerPath, "payment", "billing", "price", "quote", "trade", "wallet", "reward", "settlement"):
			addSignal("business_logic", "money or quote path", 4)
			suggestedReviewer = "correctness_reviewer"
		}
		if containsAny(lowerPath, "migration", "schema", ".sql", "database", "/db/", "models", "repository") {
			addSignal("data_integrity", "data or schema path", 3)
			if suggestedReviewer == "correctness_reviewer" {
				suggestedReviewer = "data_reviewer"
			}
		}
		if containsAny(lowerPath, "worker", "queue", "lock", "mutex", "goroutine", "async", "thread", "scheduler", "concurrent") {
			addSignal("reliability", "async or concurrency path", 3)
			if suggestedReviewer == "correctness_reviewer" {
				suggestedReviewer = "reliability_reviewer"
			}
		}
		if containsAny(lowerPath, "api", "handler", "router", "controller", "client", "proto", "contract", "webhook") {
			addSignal("api_contract", "API boundary path", 2)
		}
		if containsAny(lowerPath, "config", ".github/workflows", "helm", "values.yaml", "dockerfile", "terraform") {
			addSignal("release", "configuration or release path", 2)
			if suggestedReviewer == "correctness_reviewer" {
				suggestedReviewer = "release_reviewer"
			}
		}
		if strings.EqualFold(file.Status, "deleted") {
			addSignal("correctness", "deleted file", 2)
		}
		churn := file.Additions + file.Deletions
		switch {
		case churn >= 300:
			addSignal("complexity", "large diff", 3)
		case churn >= 80:
			addSignal("complexity", "medium-sized diff", 1)
		}
		if isTestPath(lowerPath) {
			score--
			profileSet["tests"] = true
			signals = append(signals, "test path")
			if suggestedReviewer == "correctness_reviewer" {
				suggestedReviewer = "test_reviewer"
			}
		}
		if isDocumentationPath(lowerPath) {
			score -= 2
			signals = append(signals, "documentation-only-looking path")
		}
		if focus != "" && focusMatchesPath(focus, lowerPath) {
			addSignal("focus", "matches user focus prompt", 2)
		}
		if score < 0 {
			score = 0
		}
		totalRisk += score
		if score > maxScore {
			maxScore = score
		}
		if score < 3 && len(signals) == 0 {
			continue
		}
		startLine, endLine := firstChangedLineRange(file.LineRangesJson)
		leads = append(leads, localReviewLead{
			Path:              path,
			Status:            file.Status,
			StartLine:         startLine,
			EndLine:           endLine,
			Additions:         file.Additions,
			Deletions:         file.Deletions,
			RiskScore:         score,
			SeverityHint:      severityHintForScore(score),
			SuggestedReviewer: suggestedReviewer,
			Reason:            scoutReason(path, signals, score),
			Signals:           dedupeStrings(signals),
		})
	}

	sort.SliceStable(leads, func(i, j int) bool {
		if leads[i].RiskScore != leads[j].RiskScore {
			return leads[i].RiskScore > leads[j].RiskScore
		}
		return leads[i].Path < leads[j].Path
	})
	if len(leads) > 8 {
		leads = leads[:8]
	}
	profiles := mapKeys(profileSet)
	riskScore := maxScore + minInt(totalRisk/6, 5) + minInt(len(files)/10, 3)
	tier := "lite"
	switch {
	case riskScore >= 9 || profileSet["security"] || profileSet["business_logic"]:
		tier = "full"
	case riskScore >= 5 || len(leads) >= 3:
		tier = "standard"
	}
	return localReviewScout{
		SchemaVersion: "local_review_scout.v1",
		OverallRisk:   tier,
		RiskScore:     riskScore,
		Profiles:      profiles,
		Summary:       scoutSummary(repository, tier, leads, len(files)),
		Leads:         leads,
		IgnoredAreas:  ignored,
		GeneratedAt:   now.UTC().Format(time.RFC3339Nano),
	}
}

func renderLocalScoutPrompt(scout localReviewScout) string {
	if strings.TrimSpace(scout.SchemaVersion) == "" {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("# Local Scout\n\n")
	builder.WriteString("These deterministic local signals are not findings. Use them only to prioritize investigation, then verify or discard each lead from code evidence.\n\n")
	builder.WriteString("Risk tier: ")
	builder.WriteString(scout.OverallRisk)
	builder.WriteString(" (score ")
	builder.WriteString(fmt.Sprintf("%d", scout.RiskScore))
	builder.WriteString(")\n")
	if len(scout.Profiles) > 0 {
		builder.WriteString("Profiles: ")
		builder.WriteString(strings.Join(scout.Profiles, ", "))
		builder.WriteByte('\n')
	}
	if strings.TrimSpace(scout.Summary) != "" {
		builder.WriteString("Summary: ")
		builder.WriteString(scout.Summary)
		builder.WriteByte('\n')
	}
	builder.WriteByte('\n')
	if len(scout.Leads) == 0 {
		builder.WriteString("- No high-risk local leads were found; still review the changed lines normally.\n\n")
		return builder.String()
	}
	builder.WriteString("Investigation leads:\n")
	for _, lead := range scout.Leads {
		builder.WriteString("- ")
		builder.WriteString(lead.Path)
		if lead.StartLine > 0 {
			builder.WriteString(fmt.Sprintf(":L%d", lead.StartLine))
			if lead.EndLine > lead.StartLine {
				builder.WriteString(fmt.Sprintf("-L%d", lead.EndLine))
			}
		}
		builder.WriteString(" - ")
		builder.WriteString(lead.SeverityHint)
		builder.WriteString(", ")
		builder.WriteString(lead.SuggestedReviewer)
		builder.WriteString(": ")
		builder.WriteString(lead.Reason)
		if len(lead.Signals) > 0 {
			builder.WriteString(" Signals: ")
			builder.WriteString(strings.Join(lead.Signals, "; "))
		}
		builder.WriteByte('\n')
	}
	builder.WriteByte('\n')
	return builder.String()
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func isTestPath(path string) bool {
	return strings.Contains(path, "_test.") ||
		strings.Contains(path, ".test.") ||
		strings.Contains(path, ".spec.") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/")
}

func isDocumentationPath(path string) bool {
	return strings.HasSuffix(path, ".md") ||
		strings.HasSuffix(path, ".mdx") ||
		strings.HasSuffix(path, ".txt") ||
		strings.HasPrefix(path, "docs/")
}

func focusMatchesPath(focus string, path string) bool {
	for _, token := range strings.FieldsFunc(focus, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '/')
	}) {
		token = strings.TrimSpace(strings.ToLower(token))
		if len(token) >= 3 && strings.Contains(path, token) {
			return true
		}
	}
	return false
}

func firstChangedLineRange(raw string) (int64, int64) {
	var ranges [][]int64
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &ranges); err != nil || len(ranges) == 0 || len(ranges[0]) == 0 {
		return 0, 0
	}
	start := ranges[0][0]
	end := start
	if len(ranges[0]) > 1 {
		end = ranges[0][1]
	}
	if end < start {
		end = start
	}
	return start, end
}

func severityHintForScore(score int) string {
	switch {
	case score >= 8:
		return "high"
	case score >= 5:
		return "medium"
	default:
		return "low"
	}
}

func scoutReason(path string, signals []string, score int) string {
	if len(signals) == 0 {
		return fmt.Sprintf("%s changed with local risk score %d", path, score)
	}
	return fmt.Sprintf("Local scout ranked this changed file at %d because %s.", score, strings.Join(dedupeStrings(signals), ", "))
}

func scoutSummary(repository dbgen.Repository, tier string, leads []localReviewLead, changedFileCount int) string {
	repoName := strings.TrimSpace(repository.Name)
	if repoName == "" {
		repoName = "repository"
	}
	if len(leads) == 0 {
		return fmt.Sprintf("%s has %d changed file(s); no high-risk local lead exceeded the scout threshold.", repoName, changedFileCount)
	}
	return fmt.Sprintf("%s local scout classified this review as %s with %d investigation lead(s). Top lead: %s.", repoName, tier, len(leads), leads[0].Path)
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func mapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key, ok := range values {
		if ok && strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func (s *Service) reviewPrompt(item runContext) string {
	rendered, err := s.renderReviewPrompt(item)
	if err != nil {
		return ""
	}
	return rendered.Text
}

func (s *Service) renderReviewPrompt(item runContext) (reviewprompt.RenderedPrompt, error) {
	contextText := item.BundleText
	if strings.TrimSpace(contextText) == "" {
		contextText = contextbundle.RenderBundle(item.Bundle)
	}
	return reviewprompt.RenderReviewPrompt(reviewprompt.RenderInput{
		TemplateOverride: s.PromptTemplate,
		SessionID:        item.Session.ID,
		ReviewDepth:      item.Session.ReviewDepth,
		Focus:            item.Session.FocusPrompt.String,
		Role:             firstNonEmptyString(item.SessionAgent.Role, item.AgentConfig.Role),
		LocalScoutText:   renderLocalScoutPrompt(item.Scout),
		ContextText:      contextText,
	})
}

func (s *Service) persistRenderedPrompt(ctx context.Context, item runContext, runID string, rendered reviewprompt.RenderedPrompt) (string, error) {
	if s.Artifacts == nil {
		return "", nil
	}
	metadata, err := json.Marshal(rendered.MetadataMap(""))
	if err != nil {
		return "", fmt.Errorf("encode rendered prompt metadata: %w", err)
	}
	saved, err := s.Artifacts.Save(ctx, artifact.SaveParams{
		ID:              s.artifactID(),
		WorkspaceID:     item.Workspace.ID,
		ReviewSessionID: nullableString(item.Session.ID),
		Kind:            "rendered_prompt",
		RelativePath: filepath.ToSlash(filepath.Join(
			"review-prompts",
			safePromptArtifactSegment(item.Session.ID),
			safePromptArtifactSegment(runID)+".md",
		)),
		ContentType:  "text/markdown",
		MetadataJSON: string(metadata),
		CreatedAt:    s.now().Format(time.RFC3339Nano),
	}, []byte(rendered.Text))
	if err != nil {
		return "", fmt.Errorf("save rendered prompt artifact: %w", err)
	}
	return saved.ID, nil
}

func (s *Service) withPhase(ctx context.Context, reviewSessionID string, phase string, run func() error) error {
	if err := s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: reviewSessionID,
		Type:            "WorkflowPhaseStarted",
		Payload:         map[string]any{"phase": phase},
	}); err != nil {
		return err
	}
	if err := run(); err != nil {
		_ = s.appendEvent(context.WithoutCancel(ctx), appendEventParams{
			ReviewSessionID: reviewSessionID,
			Type:            "WorkflowPhaseFailed",
			Level:           "error",
			Payload: map[string]any{
				"phase": phase,
				"error": err.Error(),
			},
		})
		return err
	}
	return s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: reviewSessionID,
		Type:            "WorkflowPhaseCompleted",
		Payload:         map[string]any{"phase": phase},
	})
}

func (s *Service) enabledSessionAgents(ctx context.Context, reviewSessionID string) ([]dbgen.ReviewSessionAgent, error) {
	agents, err := s.Queries.ListReviewSessionAgents(ctx, reviewSessionID)
	if err != nil {
		return nil, fmt.Errorf("list review session agents: %w", err)
	}
	enabled := make([]dbgen.ReviewSessionAgent, 0, len(agents))
	for _, agent := range agents {
		if agent.Enabled == 0 {
			continue
		}
		enabled = append(enabled, agent)
	}
	return enabled, nil
}

func (s *Service) loadRunContextsFromPersistedBundles(ctx context.Context, session dbgen.ReviewSession, repository dbgen.Repository, workspace dbgen.Workspace, sessionAgents []dbgen.ReviewSessionAgent) ([]runContext, error) {
	rows, err := s.Queries.ListContextBundlesBySession(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("list persisted context bundles: %w", err)
	}
	bundleByAgentConfig := map[string]dbgen.ContextBundle{}
	for _, row := range rows {
		if row.Scope != string(contextbundle.ScopeReview) || !row.AgentConfigID.Valid {
			continue
		}
		if _, exists := bundleByAgentConfig[row.AgentConfigID.String]; exists {
			continue
		}
		bundleByAgentConfig[row.AgentConfigID.String] = row
	}
	runContexts := make([]runContext, 0, len(sessionAgents))
	for _, sessionAgent := range sessionAgents {
		agentConfig, err := s.Queries.GetAgentConfig(ctx, sessionAgent.AgentConfigID)
		if err != nil {
			return nil, fmt.Errorf("read agent config %s: %w", sessionAgent.AgentConfigID, err)
		}
		row, ok := bundleByAgentConfig[agentConfig.ID]
		if !ok {
			return nil, fmt.Errorf("resume review session %s: persisted context bundle for agent %s was not found", session.ID, agentConfig.ID)
		}
		itemRows, err := s.Queries.ListContextItemsByBundle(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("list context bundle items %s: %w", row.ID, err)
		}
		bundle, err := contextbundle.BundleFromRows(row, itemRows)
		if err != nil {
			return nil, fmt.Errorf("load context bundle %s: %w", row.ID, err)
		}
		if !row.ArtifactID.Valid {
			return nil, fmt.Errorf("resume review session %s: context bundle %s has no rendered artifact", session.ID, row.ID)
		}
		rendered, _, err := s.Artifacts.Read(ctx, row.ArtifactID.String)
		if err != nil {
			return nil, fmt.Errorf("read context bundle artifact %s: %w", row.ArtifactID.String, err)
		}
		runContexts = append(runContexts, runContext{
			Session:      session,
			Repository:   repository,
			Workspace:    workspace,
			SessionAgent: sessionAgent,
			AgentConfig:  agentConfig,
			Bundle:       bundle,
			BundleText:   string(rendered),
		})
	}
	return runContexts, nil
}

func (s *Service) loadReviewAgentRunResults(ctx context.Context, reviewSessionID string) ([]agentrun.RunResult, error) {
	runs, err := s.Queries.ListAgentRunsBySession(ctx, reviewSessionID)
	if err != nil {
		return nil, fmt.Errorf("list persisted agent runs: %w", err)
	}
	results := make([]agentrun.RunResult, 0, len(runs))
	for _, run := range runs {
		if !agentRunMetadataPhase(run.MetadataJson, PhaseRunAgents) {
			continue
		}
		switch run.Status {
		case agentrun.RunStatusSucceeded, agentrun.RunStatusFailed, agentrun.RunStatusTimedOut, agentrun.RunStatusCanceled, agentrun.RunStatusOutputInvalid:
			results = append(results, agentrun.RunResult{Run: run})
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("resume review session %s: completed agent phase has no persisted terminal review agent runs", reviewSessionID)
	}
	return results, nil
}

func agentRunMetadataPhase(raw string, phase string) bool {
	var metadata map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &metadata); err != nil {
		return false
	}
	value, _ := metadata["phase"].(string)
	return strings.TrimSpace(value) == phase
}

type appendEventParams struct {
	ReviewSessionID string
	AgentRunID      sql.NullString
	Type            string
	Level           string
	Payload         map[string]any
	ArtifactID      sql.NullString
	CreatedAt       time.Time
}

func (s *Service) appendEvent(ctx context.Context, params appendEventParams) error {
	payload := params.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode event payload: %w", err)
	}
	createdAt := params.CreatedAt
	if createdAt.IsZero() {
		createdAt = s.now()
	}
	_, err = s.Events.Append(ctx, eventlog.AppendParams{
		ID:              s.eventID(),
		ReviewSessionID: strings.TrimSpace(params.ReviewSessionID),
		AgentRunID:      params.AgentRunID,
		Type:            strings.TrimSpace(params.Type),
		Level:           strings.TrimSpace(params.Level),
		PayloadJSON:     string(payloadJSON),
		ArtifactID:      params.ArtifactID,
		CreatedAt:       createdAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("append event %s: %w", params.Type, err)
	}
	return nil
}

func (s *Service) appendAgentRunEvents(ctx context.Context, reviewSessionID string, events []agents.AgentEvent) error {
	for _, event := range events {
		if err := s.appendAgentRunEvent(ctx, reviewSessionID, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) agentRunEventSink(reviewSessionID string) func(context.Context, agents.AgentEvent) {
	if s.Events == nil {
		return nil
	}
	return func(ctx context.Context, event agents.AgentEvent) {
		if err := s.appendAgentRunEvent(ctx, reviewSessionID, event); err != nil {
			log.Printf("orchestrator: append agent run event failed review_session_id=%s agent_run_id=%s event=%s: %v", reviewSessionID, event.RunID, event.Type, err)
		}
	}
}

func (s *Service) appendAgentRunEvent(ctx context.Context, reviewSessionID string, event agents.AgentEvent) error {
	eventType := workflowEventType(event.Type)
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
	return s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: reviewSessionID,
		AgentRunID:      nullableEventString(event.RunID),
		Type:            eventType,
		Level:           level,
		Payload:         payload,
		ArtifactID:      nullableEventString(event.ArtifactID),
		CreatedAt:       event.At,
	})
}

func truncateEventPreview(value string) string {
	value = strings.TrimSpace(value)
	const limit = 12 * 1024
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n..."
}

func workflowEventType(eventType agents.EventType) string {
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

func workflowPhases() []string {
	return []string{
		PhaseBuildContext,
		PhaseScoutRisk,
		PhaseRunAgents,
		PhaseNormalizeOutputs,
		PhaseDeduplicate,
		PhaseVerifyFindings,
		PhaseBuildEvidence,
		PhaseDraftComments,
	}
}

func progressPercent(status string, completedPhases []string) int {
	switch status {
	case StatusCompleted:
		return 100
	case StatusDraft, StatusQueued:
		return 0
	}
	total := len(workflowPhases())
	if total == 0 {
		return 0
	}
	completed := 0
	for _, phase := range workflowPhases() {
		if phaseCompleted(completedPhases, phase) {
			completed++
		}
	}
	percent := completed * 100 / total
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

func phaseCompleted(completedPhases []string, phase string) bool {
	for _, completed := range completedPhases {
		if completed == phase {
			return true
		}
	}
	return false
}

func lastCompletedPhaseName(completedPhases []string) string {
	last := ""
	for _, phase := range workflowPhases() {
		if phaseCompleted(completedPhases, phase) {
			last = phase
		}
	}
	return last
}

func terminalStatus(status string) bool {
	switch status {
	case StatusCanceled, StatusCompleted, StatusFailed:
		return true
	default:
		return false
	}
}

func contextArtifactRefs(bundle contextbundle.Bundle) []agents.ArtifactRef {
	if strings.TrimSpace(bundle.ArtifactID) == "" {
		return nil
	}
	return []agents.ArtifactRef{{
		ID:          bundle.ArtifactID,
		Kind:        "context_bundle",
		ContentType: "text/markdown",
		SizeBytes:   0,
	}}
}

func decodeRuntimeSettings(raw string) (runtimeSettings, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	var settings runtimeSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return runtimeSettings{}, fmt.Errorf("%w: settings must be a JSON object", ErrInvalidAgentConfiguration)
	}
	if settings.PromptDelivery != "" && !settings.PromptDelivery.Valid() {
		return runtimeSettings{}, fmt.Errorf("%w: prompt_delivery is invalid", ErrInvalidAgentConfiguration)
	}
	if settings.TimeoutSeconds < 0 ||
		settings.MaxStdoutBytes < 0 ||
		settings.MaxStderrBytes < 0 ||
		settings.MaxPromptBytes < 0 ||
		settings.ReviewTimeoutSecs < 0 {
		return runtimeSettings{}, fmt.Errorf("%w: runtime limits cannot be negative", ErrInvalidAgentConfiguration)
	}
	return settings, nil
}

func decodeSessionAgentSettingsOverride(raw string) (sessionAgentSettingsOverride, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	var settings sessionAgentSettingsOverride
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return sessionAgentSettingsOverride{}, fmt.Errorf("%w: session agent settings must be a JSON object", ErrInvalidAgentConfiguration)
	}
	settings.ModelLabel = strings.TrimSpace(settings.ModelLabel)
	settings.ReasoningLabel = strings.TrimSpace(settings.ReasoningLabel)
	return settings, nil
}

func selectedAgentModelLabel(item runContext, override sessionAgentSettingsOverride) string {
	if override.ModelLabel != "" {
		return override.ModelLabel
	}
	return strings.TrimSpace(nullableValue(item.AgentConfig.ModelLabel))
}

func selectedAgentReasoningLabel(item runContext, override sessionAgentSettingsOverride) string {
	if override.ReasoningLabel != "" {
		return override.ReasoningLabel
	}
	return strings.TrimSpace(nullableValue(item.AgentConfig.ReasoningLabel))
}

func workingDirectoryForAgent(cwdMode string, repository dbgen.Repository, workspace dbgen.Workspace) (string, error) {
	switch strings.TrimSpace(cwdMode) {
	case "", "repo_root":
		if strings.TrimSpace(repository.LocalPath) == "" {
			return "", fmt.Errorf("%w: repository local path is not configured", ErrInvalidAgentConfiguration)
		}
		return repository.LocalPath, nil
	case "workspace_root":
		if strings.TrimSpace(workspace.RootPath) == "" {
			return "", fmt.Errorf("%w: workspace root path is not configured", ErrInvalidAgentConfiguration)
		}
		return workspace.RootPath, nil
	default:
		return "", fmt.Errorf("%w: cwd_mode %q is unsupported", ErrInvalidAgentConfiguration, cwdMode)
	}
}

func (s *Service) validate() error {
	if err := s.validateQueries(); err != nil {
		return err
	}
	if s.ContextBuilder == nil {
		return fmt.Errorf("%w: context builder is required", ErrServiceNotConfigured)
	}
	if s.Artifacts == nil {
		return fmt.Errorf("%w: artifact store is required", ErrServiceNotConfigured)
	}
	if s.Events == nil {
		return fmt.Errorf("%w: event store is required", ErrServiceNotConfigured)
	}
	if s.AgentManager == nil {
		return fmt.Errorf("%w: agent manager is required", ErrServiceNotConfigured)
	}
	return nil
}

func (s *Service) validateQueries() error {
	if s == nil || s.Queries == nil {
		return fmt.Errorf("%w: queries are required", ErrServiceNotConfigured)
	}
	return nil
}

func (s *Service) registerActiveSession(reviewSessionID string) bool {
	reviewSessionID = strings.TrimSpace(reviewSessionID)
	if reviewSessionID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeSessions == nil {
		s.activeSessions = map[string]struct{}{}
	}
	if _, exists := s.activeSessions[reviewSessionID]; exists {
		return false
	}
	s.activeSessions[reviewSessionID] = struct{}{}
	return true
}

func (s *Service) unregisterActiveSession(reviewSessionID string) {
	reviewSessionID = strings.TrimSpace(reviewSessionID)
	if reviewSessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.activeSessions, reviewSessionID)
}

func (s *Service) background() context.Context {
	if s.Background != nil {
		return s.Background
	}
	return context.Background()
}

func (s *Service) promptTemplate() string {
	if strings.TrimSpace(s.PromptTemplate) != "" {
		return s.PromptTemplate
	}
	return reviewprompt.DefaultTemplate()
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) eventID() string {
	if s.NewEventID != nil {
		if id := strings.TrimSpace(s.NewEventID()); id != "" {
			return id
		}
	}
	return s.newID("event_")
}

func (s *Service) artifactID() string {
	if s.NewArtifactID != nil {
		if id := strings.TrimSpace(s.NewArtifactID()); id != "" {
			return id
		}
	}
	return s.newID("artifact_")
}

func safePromptArtifactSegment(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsNumber(char) || char == '.' || char == '_' || char == '-' {
			builder.WriteRune(char)
			continue
		}
		builder.WriteByte('-')
	}
	cleaned := strings.Trim(builder.String(), ".-_")
	if cleaned == "" {
		return "unknown"
	}
	return cleaned
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Service) newID(prefix string) string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + "unavailable"
	}
	return prefix + hex.EncodeToString(bytes[:])
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func agentRunSummary(run dbgen.AgentRun) AgentRun {
	metadata := decodeAgentRunDisplayMetadata(run.MetadataJson)
	return AgentRun{
		ID:                   run.ID,
		ReviewSessionID:      run.ReviewSessionID,
		AgentConfigID:        run.AgentConfigID,
		ReviewSessionAgentID: metadata.ReviewSessionAgentID,
		ContextBundleID:      nullableValue(run.ContextBundleID),
		Status:               run.Status,
		Role:                 run.Role,
		ModelLabel:           metadata.ModelLabel,
		ReasoningLabel:       metadata.ReasoningLabel,
		StartedAt:            nullableValue(run.StartedAt),
		CompletedAt:          nullableValue(run.CompletedAt),
		ErrorCode:            nullableValue(run.ErrorCode),
		ErrorMessage:         nullableValue(run.ErrorMessage),
	}
}

type agentRunDisplayMetadata struct {
	ReviewSessionAgentID string `json:"review_session_agent_id"`
	ModelLabel           string `json:"model_label"`
	ReasoningLabel       string `json:"reasoning_label"`
}

func decodeAgentRunDisplayMetadata(raw string) agentRunDisplayMetadata {
	var metadata agentRunDisplayMetadata
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &metadata); err != nil {
		return agentRunDisplayMetadata{}
	}
	metadata.ReviewSessionAgentID = strings.TrimSpace(metadata.ReviewSessionAgentID)
	metadata.ModelLabel = strings.TrimSpace(metadata.ModelLabel)
	metadata.ReasoningLabel = strings.TrimSpace(metadata.ReasoningLabel)
	return metadata
}

func copyStringMetadata(target map[string]any, source map[string]any, key string) {
	if target == nil || source == nil {
		return
	}
	value, ok := source[key].(string)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return
	}
	target[key] = value
}

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullablePositiveInt64(value int64) sql.NullInt64 {
	if value < 1 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: value, Valid: true}
}

func nullableEventString(value string) sql.NullString {
	return nullableString(value)
}

func nullableValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullableInt64Value(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func validatePrimaryChangedCodeAnchor(repoRoot string, changedFiles []dbgen.ChangedFile, path sql.NullString, startLine sql.NullInt64, endLine sql.NullInt64) evidence.ChangedCodeAnchorValidation {
	if !path.Valid || strings.TrimSpace(path.String) == "" {
		return evidence.ChangedCodeAnchorValidation{
			Reason:  "missing_location",
			Summary: "The primary location is missing, so the finding cannot be deterministically anchored to changed code.",
		}
	}
	start := nullableInt64Value(startLine)
	end := nullableInt64Value(endLine)
	if end < start {
		end = start
	}
	return evidence.ValidateChangedCodeAnchor(repoRoot, changedFiles, path.String, start, end, 2, 16*1024)
}

func refinePrimaryLocationFromCode(repoRoot string, path sql.NullString, startLine sql.NullInt64, endLine sql.NullInt64, textParts ...any) (sql.NullInt64, sql.NullInt64) {
	if !path.Valid || !startLine.Valid {
		return startLine, endLine
	}
	end := startLine.Int64
	if endLine.Valid && endLine.Int64 >= startLine.Int64 {
		end = endLine.Int64
	}
	refinedStart, refinedEnd := refineCodeLocationRange(repoRoot, path.String, startLine.Int64, end, textParts...)
	if refinedStart < 1 {
		return startLine, endLine
	}
	return nullablePositiveInt64(refinedStart), nullablePositiveInt64(refinedEnd)
}

func refineCodeLocationRange(repoRoot string, path string, startLine int64, endLine int64, textParts ...any) (int64, int64) {
	repoRoot = strings.TrimSpace(repoRoot)
	path = strings.TrimSpace(path)
	if repoRoot == "" || path == "" || startLine < 1 {
		return 0, 0
	}
	if endLine < startLine {
		endLine = startLine
	}
	absPath := filepath.Join(repoRoot, filepath.FromSlash(path))
	source, err := os.ReadFile(absPath)
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(string(source), "\n")
	if len(lines) == 0 {
		return 0, 0
	}
	tokens := locationSignalTokens(textParts...)
	if len(tokens) == 0 {
		return 0, 0
	}
	windowStart := maxInt64(1, startLine-6)
	windowEnd := endLine + 6
	if windowEnd > int64(len(lines)) {
		windowEnd = int64(len(lines))
	}
	bestLine := int64(0)
	bestScore := 0
	for lineNo := windowStart; lineNo <= windowEnd; lineNo++ {
		text := strings.TrimSpace(lines[lineNo-1])
		if text == "" || strings.HasPrefix(text, "//") {
			continue
		}
		score := locationLineScore(text, tokens)
		if score > bestScore || score == bestScore && bestLine > 0 && absInt64(lineNo-startLine) < absInt64(bestLine-startLine) {
			bestScore = score
			bestLine = lineNo
		}
	}
	if bestLine == 0 || bestScore < 3 {
		return 0, 0
	}
	refinedStart := bestLine
	refinedEnd := bestLine
	if previous := bestLine - 1; previous >= windowStart && locationControlLine(lines[previous-1]) && locationLineScore(lines[previous-1], tokens) > 0 {
		refinedStart = previous
	}
	if next := bestLine + 1; next <= windowEnd && locationLineScore(lines[next-1], tokens) >= bestScore && !locationControlLine(lines[bestLine-1]) {
		refinedEnd = next
	}
	return refinedStart, refinedEnd
}

func locationSignalTokens(parts ...any) map[string]int {
	raw := strings.Builder{}
	for _, part := range parts {
		switch value := part.(type) {
		case string:
			raw.WriteByte(' ')
			raw.WriteString(value)
		case sql.NullString:
			if value.Valid {
				raw.WriteByte(' ')
				raw.WriteString(value.String)
			}
		}
	}
	tokens := map[string]int{}
	for _, token := range strings.FieldsFunc(raw.String(), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '[' && r != ']'
	}) {
		rawToken := strings.Trim(token, "[]`.,:;(){}")
		token = strings.ToLower(rawToken)
		if !usefulLocationToken(token) {
			continue
		}
		weight := 1
		if strings.Contains(token, "[") || strings.Contains(token, "]") {
			weight = 5
		} else if strings.Contains(token, "_") || hasMixedCaseSignal(rawToken) {
			weight = 3
		}
		tokens[token] = maxInt(tokens[token], weight)
		if base := strings.Split(token, "[")[0]; base != token && usefulLocationToken(base) {
			tokens[base] = maxInt(tokens[base], 2)
		}
	}
	return tokens
}

func usefulLocationToken(token string) bool {
	if len(token) < 3 {
		return false
	}
	switch token {
	case "the", "and", "for", "with", "when", "then", "line", "lines", "code", "claim", "finding", "issue", "path", "file", "nil", "null", "true", "false", "return", "expected", "observed", "changed", "without", "causing", "runtime", "panic", "guard", "check", "before", "after":
		return false
	default:
		return true
	}
}

func hasMixedCaseSignal(token string) bool {
	hasLower := false
	hasUpper := false
	for _, r := range token {
		if unicode.IsLower(r) {
			hasLower = true
		}
		if unicode.IsUpper(r) {
			hasUpper = true
		}
	}
	return hasLower && hasUpper
}

func locationLineScore(line string, tokens map[string]int) int {
	line = strings.ToLower(line)
	score := 0
	for token, weight := range tokens {
		if strings.Contains(line, token) {
			score += weight
		}
	}
	return score
}

func locationControlLine(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "if ") ||
		strings.HasPrefix(line, "for ") ||
		strings.HasPrefix(line, "switch ") ||
		strings.HasPrefix(line, "case ")
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func maxInt(a int, b int) int {
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

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
