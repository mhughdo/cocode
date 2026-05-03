package orchestrator

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/agentoutput"
	"github.com/hughdo/cocode/services/cocoded/internal/agentrun"
	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/contextbundle"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/eventlog"
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
	PhaseRunAgents        = "run_review_agents"
	PhaseNormalizeOutputs = "normalize_outputs"
	PhaseDeduplicate      = "deduplicate_findings"
	PhaseVerifyFindings   = "verify_findings"
	PhaseBuildEvidence    = "build_evidence_maps"
	PhaseDraftComments    = "draft_comments"
)

const defaultPromptTemplate = `# Role

You are a code review agent inside cocode.

# Task

Review the provided diff and bounded repository context. Return evidence-backed findings only.

# Rules

- Prefer correctness, security, reliability, data integrity, tests, and API compatibility findings.
- Cite files and lines whenever possible.
- Do not suggest broad style changes unless they hide a concrete defect.`

var (
	ErrServiceNotConfigured      = errors.New("review workflow service is not configured")
	ErrReviewSessionNotFound     = errors.New("review session was not found")
	ErrInvalidStatusTransition   = errors.New("invalid review session status transition")
	ErrNoEnabledReviewAgents     = errors.New("review session has no enabled agents")
	ErrInvalidAgentConfiguration = errors.New("agent configuration is invalid")
	ErrReviewSessionCanceled     = errors.New("review session was canceled")
)

type Service struct {
	Queries        *dbgen.Queries
	ContextBuilder *contextbundle.Service
	Artifacts      *artifact.Store
	Events         EventLog
	AgentManager   *agentrun.Manager
	PromptTemplate string
	Background     context.Context
	Now            func() time.Time
	NewEventID     func() string
	NewArtifactID  func() string
}

type EventLog interface {
	Append(ctx context.Context, params eventlog.AppendParams) (dbgen.Event, error)
	ListByReviewSession(ctx context.Context, reviewSessionID string) ([]dbgen.Event, error)
}

type StartResult struct {
	Session dbgen.ReviewSession
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

type runtimeSettings struct {
	PromptDelivery    agents.PromptDelivery `json:"prompt_delivery"`
	TimeoutSeconds    int64                 `json:"timeout_seconds"`
	MaxStdoutBytes    int64                 `json:"max_stdout_bytes"`
	MaxStderrBytes    int64                 `json:"max_stderr_bytes"`
	MaxPromptBytes    int64                 `json:"max_prompt_bytes"`
	ReviewTimeoutSecs int64                 `json:"review_timeout_seconds"`
}

type runContext struct {
	Session      dbgen.ReviewSession
	Repository   dbgen.Repository
	Workspace    dbgen.Workspace
	SessionAgent dbgen.ReviewSessionAgent
	AgentConfig  dbgen.AgentConfig
	Bundle       contextbundle.Bundle
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
	err := s.run(ctx, reviewSessionID)
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

	runContexts := make([]runContext, 0, len(sessionAgents))
	if err := s.withPhase(ctx, session.ID, PhaseBuildContext, func() error {
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

	failedRuns := 0
	succeededRuns := 0
	if err := s.withPhase(ctx, session.ID, PhaseRunAgents, func() error {
		results, err := s.runAgents(ctx, runContexts)
		if err != nil {
			return err
		}
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
		if err := s.withPhase(ctx, session.ID, phase, func() error { return nil }); err != nil {
			return err
		}
	}
	if canceled, err := s.cancellationRequested(ctx, session.ID); err != nil {
		return err
	} else if canceled {
		return s.completeCanceled(ctx, session.ID)
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
	prompt := s.reviewPrompt(item)
	task := agents.AgentTask{
		ID:               s.newID("agent_task_"),
		RunID:            s.newID("agent_run_"),
		ReviewSessionID:  item.Session.ID,
		AgentConfigID:    item.AgentConfig.ID,
		ContextBundleID:  item.Bundle.ID,
		Role:             item.SessionAgent.Role,
		Prompt:           prompt,
		ContextArtifacts: contextArtifactRefs(item.Bundle),
		RepositoryRoot:   item.Repository.LocalPath,
		WorkspaceRoot:    item.Workspace.RootPath,
		Limits:           limits,
		Metadata: map[string]any{
			"review_session_agent_id": item.SessionAgent.ID,
			"context_bundle_id":       item.Bundle.ID,
		},
	}
	reviewDeadline := time.Time{}
	if item.Session.RuntimeLimitSeconds > 0 && item.Session.StartedAt.Valid {
		if startedAt, err := time.Parse(time.RFC3339Nano, item.Session.StartedAt.String); err == nil {
			reviewDeadline = startedAt.Add(time.Duration(item.Session.RuntimeLimitSeconds) * time.Second)
		}
	}
	result, err := s.AgentManager.Execute(ctx, agentrun.RunParams{
		WorkspaceID: item.Workspace.ID,
		Config:      config,
		Task:        task,
		TimeoutPolicy: agentrun.TimeoutPolicy{
			AgentTimeoutSeconds:  limits.TimeoutSeconds,
			ReviewDeadline:       reviewDeadline,
			ReviewTimeoutSeconds: maxInt64(0, item.Session.RuntimeLimitSeconds),
		},
		Metadata: map[string]any{
			"phase":                   PhaseRunAgents,
			"review_session_agent_id": item.SessionAgent.ID,
			"context_bundle_id":       item.Bundle.ID,
			"output_mode":             string(agents.OutputMode(item.AgentConfig.OutputMode)),
		},
	})
	if err != nil {
		return result, err
	}
	if err := s.appendAgentRunEvents(ctx, item.Session.ID, result.Events); err != nil {
		return result, err
	}
	if err := s.parseAgentOutput(ctx, item, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) parseAgentOutput(ctx context.Context, item runContext, result *agentrun.RunResult) error {
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
		"agent_run_id":    result.Run.ID,
		"agent_config_id": item.AgentConfig.ID,
		"mode":            string(outputMode),
		"structured":      parsed.Structured,
		"diagnostics":     len(parsed.Diagnostics),
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
			"phase":                 PhaseNormalizeOutputs,
			"agent_config_id":       item.AgentConfig.ID,
			"agent_run_id":          result.Run.ID,
			"parsed_artifact_id":    artifactRow.ID,
			"structured":            parsed.Structured,
			"document_count":        len(parsed.Documents),
			"diagnostic_count":      len(parsed.Diagnostics),
			"parsed_output_mode":    string(parsed.Mode),
			"requested_output_mode": string(outputMode),
		},
	})
}

func (s *Service) connectionConfig(item runContext) (agents.ConnectionConfig, agents.TaskLimits, error) {
	args, err := decodeStringArray(item.AgentConfig.ArgsJson, "agent args")
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, err
	}
	envNames, err := decodeStringArray(item.AgentConfig.EnvAllowlistJson, "agent env_allowlist")
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, err
	}
	settings, err := decodeRuntimeSettings(item.AgentConfig.SettingsJson)
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, err
	}
	workingDirectory, err := workingDirectoryForAgent(item.AgentConfig.CwdMode, item.Repository, item.Workspace)
	if err != nil {
		return agents.ConnectionConfig{}, agents.TaskLimits{}, err
	}
	return agents.ConnectionConfig{
			AdapterID:        item.AgentConfig.ID,
			Kind:             agents.AdapterKind(item.AgentConfig.AdapterKind),
			Command:          nullableValue(item.AgentConfig.Command),
			Args:             args,
			PromptDelivery:   settings.PromptDelivery,
			WorkingDirectory: workingDirectory,
			Env:              allowedEnvironment(envNames),
			Metadata: map[string]any{
				"output_mode": string(item.AgentConfig.OutputMode),
			},
		}, agents.TaskLimits{
			TimeoutSeconds: settings.TimeoutSeconds,
			MaxStdoutBytes: settings.MaxStdoutBytes,
			MaxStderrBytes: settings.MaxStderrBytes,
			MaxPromptBytes: settings.MaxPromptBytes,
		}, nil
}

func (s *Service) reviewPrompt(item runContext) string {
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(s.promptTemplate()))
	builder.WriteString("\n\n# Output Contract\n\n")
	builder.WriteString("Return a JSON object with a `findings` array. Use an empty array when there are no concrete defects.\n\n")
	builder.WriteString("# Session\n\n")
	builder.WriteString("Review session ID: ")
	builder.WriteString(item.Session.ID)
	builder.WriteByte('\n')
	builder.WriteString("Review depth: ")
	builder.WriteString(item.Session.ReviewDepth)
	builder.WriteByte('\n')
	if strings.TrimSpace(item.Session.FocusPrompt.String) != "" {
		builder.WriteString("Focus: ")
		builder.WriteString(strings.TrimSpace(item.Session.FocusPrompt.String))
		builder.WriteByte('\n')
	}
	builder.WriteString("\n")
	builder.WriteString(contextbundle.RenderBundle(item.Bundle))
	return builder.String()
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
		if agent.Enabled != 0 {
			enabled = append(enabled, agent)
		}
	}
	return enabled, nil
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
		eventType := workflowEventType(event.Type)
		if eventType == "" {
			continue
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
		if err := s.appendEvent(ctx, appendEventParams{
			ReviewSessionID: reviewSessionID,
			AgentRunID:      nullableEventString(event.RunID),
			Type:            eventType,
			Level:           level,
			Payload:         payload,
			ArtifactID:      nullableEventString(event.ArtifactID),
			CreatedAt:       event.At,
		}); err != nil {
			return err
		}
	}
	return nil
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
		PhaseRunAgents,
		PhaseNormalizeOutputs,
		PhaseDeduplicate,
		PhaseVerifyFindings,
		PhaseBuildEvidence,
		PhaseDraftComments,
	}
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

func decodeStringArray(raw string, field string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "[]"
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("%w: %s must be a JSON string array", ErrInvalidAgentConfiguration, field)
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("%w: %s cannot contain empty values", ErrInvalidAgentConfiguration, field)
		}
	}
	return values, nil
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

func allowedEnvironment(names []string) map[string]string {
	env := map[string]string{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if value, ok := os.LookupEnv(name); ok {
			env[name] = value
		}
	}
	return env
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
	return defaultPromptTemplate
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

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
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

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
