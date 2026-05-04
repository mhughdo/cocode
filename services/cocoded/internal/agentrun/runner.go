package agentrun

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const (
	RunStatusQueued        = "queued"
	RunStatusRunning       = "running"
	RunStatusSucceeded     = "succeeded"
	RunStatusFailed        = "failed"
	RunStatusTimedOut      = "timed_out"
	RunStatusCanceled      = "canceled"
	RunStatusOutputInvalid = "output_invalid"
)

type Runner struct {
	Queries   *dbgen.Queries
	Artifacts *artifact.Store
	Driver    agents.ConnectionDriver
	Now       func() time.Time
	NewRunID  func() string
}

type RunParams struct {
	WorkspaceID   string
	Config        agents.ConnectionConfig
	Capabilities  agents.AgentCapabilities
	Permissions   agents.PermissionPolicy
	Task          agents.AgentTask
	TimeoutPolicy TimeoutPolicy
	Metadata      map[string]any
}

type RunResult struct {
	Run             dbgen.AgentRun
	Events          []agents.AgentEvent
	OutputArtifacts OutputArtifactResult
}

type runOutcome struct {
	status       string
	exitCode     sql.NullInt64
	errorCode    sql.NullString
	errorMessage sql.NullString
}

func (r Runner) Execute(ctx context.Context, params RunParams) (RunResult, error) {
	ctx = contextOrBackground(ctx)
	if err := ctx.Err(); err != nil {
		return RunResult{}, err
	}
	if r.Queries == nil {
		return RunResult{}, errors.New("agent run queries are required")
	}
	if r.Artifacts == nil {
		return RunResult{}, errors.New("artifact store is required")
	}
	if strings.TrimSpace(params.WorkspaceID) == "" {
		return RunResult{}, errors.New("workspace id is required")
	}

	task := params.Task
	if strings.TrimSpace(task.RunID) == "" {
		task.RunID = r.newRunID()
	}
	if strings.TrimSpace(task.ID) == "" {
		task.ID = task.RunID
	}
	if err := task.Validate(); err != nil {
		return RunResult{}, err
	}

	config := params.Config
	if strings.TrimSpace(config.AdapterID) == "" {
		config.AdapterID = task.AgentConfigID
	}
	startedAt := r.now()
	task, timeoutMetadata, preflightOutcome, err := params.TimeoutPolicy.Apply(startedAt, task)
	if err != nil {
		return RunResult{}, err
	}
	permissions := params.Permissions.Evaluate(agents.RequiredPermissionsForRun(config, params.Capabilities))
	visibility := agents.VisibilityForConfig(config, params.Capabilities)
	metadataJSON, err := runMetadataJSON(mergeRunMetadata(params.Metadata, map[string]any{
		"timeout_policy":    timeoutMetadata,
		"permission_policy": permissions.Metadata(),
		"agent_visibility":  visibility.Metadata(),
	}), task)
	if err != nil {
		return RunResult{}, err
	}

	persistCtx := context.WithoutCancel(ctx)
	run, err := r.Queries.CreateAgentRun(persistCtx, dbgen.CreateAgentRunParams{
		ID:              task.RunID,
		ReviewSessionID: task.ReviewSessionID,
		AgentConfigID:   task.AgentConfigID,
		ContextBundleID: nullableRunString(task.ContextBundleID),
		Status:          RunStatusQueued,
		Role:            task.Role,
		MetadataJson:    metadataJSON,
	})
	if err != nil {
		return RunResult{}, fmt.Errorf("create agent run: %w", err)
	}

	result := RunResult{
		Run: run,
		Events: []agents.AgentEvent{{
			Type:    agents.EventQueued,
			RunID:   task.RunID,
			At:      startedAt,
			Message: "agent run queued",
		}},
	}
	run, err = r.updateRun(persistCtx, run, runUpdate{
		status:    RunStatusRunning,
		startedAt: nullableRunString(startedAt.Format(time.RFC3339Nano)),
	})
	if err != nil {
		return result, fmt.Errorf("mark agent run running: %w", err)
	}
	result.Run = run
	if preflightOutcome != nil {
		completedAt := r.now()
		result.Events = append(result.Events, syntheticTerminalEvent(run.ID, completedAt, *preflightOutcome))
		finished, err := r.finishRun(persistCtx, run, startedAt, completedAt, *preflightOutcome)
		result.Run = finished
		return result, err
	}
	if denied, ok := permissions.FirstDenied(); ok {
		return r.finishWithError(persistCtx, result, run, startedAt, "permission_denied", permissionDeniedError(denied))
	}

	connection, err := r.driver(config.Kind).Open(ctx, config)
	if err != nil {
		return r.finishWithError(persistCtx, result, run, startedAt, "open_error", err)
	}
	defer func() {
		_ = connection.Close(context.Background())
	}()

	eventStream, err := connection.SendTask(ctx, task)
	if err != nil {
		return r.finishWithError(persistCtx, result, run, startedAt, "send_error", err)
	}
	for event := range eventStream {
		result.Events = append(result.Events, event)
	}

	completedAt := r.now()
	recorder := OutputRecorder{Artifacts: r.Artifacts, Queries: r.Queries, Now: r.Now}
	outputs, err := recorder.SaveRawOutputs(persistCtx, OutputArtifactParams{
		WorkspaceID: params.WorkspaceID,
		Task:        task,
		Events:      result.Events,
	})
	if err != nil {
		failed, updateErr := r.finishRun(persistCtx, run, startedAt, completedAt, runOutcome{
			status:       RunStatusFailed,
			errorCode:    nullableRunString("artifact_error"),
			errorMessage: nullableRunString(err.Error()),
		})
		result.Run = failed
		if updateErr != nil {
			return result, fmt.Errorf("save raw outputs: %w; update failed run: %w", err, updateErr)
		}
		return result, fmt.Errorf("save raw outputs: %w", err)
	}
	result.OutputArtifacts = outputs
	run = outputs.Run

	finished, err := r.finishRun(persistCtx, run, startedAt, completedAt, outcomeFromEvents(result.Events, ctx.Err()))
	if err != nil {
		return result, err
	}
	result.Run = finished
	return result, nil
}

func (r Runner) finishWithError(ctx context.Context, result RunResult, run dbgen.AgentRun, startedAt time.Time, code string, err error) (RunResult, error) {
	completedAt := r.now()
	outcome := outcomeFromError(code, err)
	result.Events = append(result.Events, syntheticTerminalEvent(run.ID, completedAt, outcome))
	finished, updateErr := r.finishRun(ctx, run, startedAt, completedAt, outcome)
	result.Run = finished
	if updateErr != nil {
		return result, updateErr
	}
	return result, nil
}

type runUpdate struct {
	status       string
	startedAt    sql.NullString
	completedAt  sql.NullString
	durationMs   sql.NullInt64
	exitCode     sql.NullInt64
	errorCode    sql.NullString
	errorMessage sql.NullString
}

func (r Runner) finishRun(ctx context.Context, run dbgen.AgentRun, startedAt time.Time, completedAt time.Time, outcome runOutcome) (dbgen.AgentRun, error) {
	return r.updateRun(ctx, run, runUpdate{
		status:       outcome.status,
		startedAt:    run.StartedAt,
		completedAt:  nullableRunString(completedAt.Format(time.RFC3339Nano)),
		durationMs:   nullableRunInt64(durationMillis(startedAt, completedAt)),
		exitCode:     outcome.exitCode,
		errorCode:    outcome.errorCode,
		errorMessage: outcome.errorMessage,
	})
}

func (r Runner) updateRun(ctx context.Context, run dbgen.AgentRun, update runUpdate) (dbgen.AgentRun, error) {
	if update.startedAt.Valid {
		run.StartedAt = update.startedAt
	}
	if update.completedAt.Valid {
		run.CompletedAt = update.completedAt
	}
	if update.durationMs.Valid {
		run.DurationMs = update.durationMs
	}
	run.Status = update.status
	run.ExitCode = update.exitCode
	run.ErrorCode = update.errorCode
	run.ErrorMessage = update.errorMessage
	return r.Queries.UpdateAgentRunStatus(ctx, dbgen.UpdateAgentRunStatusParams{
		ID:                     run.ID,
		Status:                 run.Status,
		StartedAt:              run.StartedAt,
		CompletedAt:            run.CompletedAt,
		DurationMs:             run.DurationMs,
		ExitCode:               run.ExitCode,
		StdoutArtifactID:       run.StdoutArtifactID,
		StderrArtifactID:       run.StderrArtifactID,
		ParsedOutputArtifactID: run.ParsedOutputArtifactID,
		ErrorCode:              run.ErrorCode,
		ErrorMessage:           run.ErrorMessage,
		MetadataJson:           run.MetadataJson,
	})
}

func outcomeFromEvents(events []agents.AgentEvent, ctxErr error) runOutcome {
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if !event.Type.Terminal() {
			continue
		}
		switch event.Type {
		case agents.EventCompleted:
			exitCode := int64(0)
			if event.ExitCode != nil {
				exitCode = int64(*event.ExitCode)
			}
			return runOutcome{status: RunStatusSucceeded, exitCode: nullableRunInt64(exitCode)}
		case agents.EventFailed:
			return runOutcome{
				status:       RunStatusFailed,
				exitCode:     nullableEventExitCode(event.ExitCode),
				errorCode:    nullableRunString(firstNonEmpty(event.ErrorCode, "failed")),
				errorMessage: nullableRunString(firstNonEmpty(event.Error, event.Message, "agent command failed")),
			}
		case agents.EventCanceled:
			status := RunStatusCanceled
			if event.ErrorCode == "timeout" {
				status = RunStatusTimedOut
			}
			return runOutcome{
				status:       status,
				exitCode:     nullableEventExitCode(event.ExitCode),
				errorCode:    nullableRunString(firstNonEmpty(event.ErrorCode, "canceled")),
				errorMessage: nullableRunString(firstNonEmpty(event.Error, event.Message, "agent command canceled")),
			}
		}
	}
	return outcomeFromError("missing_terminal", ctxErr)
}

func outcomeFromError(code string, err error) runOutcome {
	status := RunStatusFailed
	if errors.Is(err, context.DeadlineExceeded) {
		status = RunStatusTimedOut
		code = "timeout"
	} else if errors.Is(err, context.Canceled) {
		status = RunStatusCanceled
		code = "canceled"
	}
	message := "agent command ended without terminal event"
	if err != nil {
		message = err.Error()
	}
	return runOutcome{
		status:       status,
		errorCode:    nullableRunString(code),
		errorMessage: nullableRunString(message),
	}
}

func syntheticTerminalEvent(runID string, at time.Time, outcome runOutcome) agents.AgentEvent {
	eventType := agents.EventFailed
	message := "agent run failed"
	if outcome.status == RunStatusTimedOut || outcome.status == RunStatusCanceled {
		eventType = agents.EventCanceled
		message = "agent run canceled"
	}
	return agents.AgentEvent{
		Type:      eventType,
		RunID:     runID,
		At:        at,
		Message:   message,
		ErrorCode: outcome.errorCode.String,
		Error:     outcome.errorMessage.String,
	}
}

func runMetadataJSON(metadata map[string]any, task agents.AgentTask) (string, error) {
	value := make(map[string]any, len(metadata)+2)
	for key, item := range metadata {
		key = strings.TrimSpace(key)
		if key != "" {
			value[key] = item
		}
	}
	if _, exists := value["task_id"]; !exists && strings.TrimSpace(task.ID) != "" {
		value["task_id"] = task.ID
	}
	if _, exists := value["agent_config_id"]; !exists {
		value["agent_config_id"] = task.AgentConfigID
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode agent run metadata: %w", err)
	}
	return string(data), nil
}

func permissionDeniedError(result agents.PermissionResult) error {
	if result.Reason != "" {
		return fmt.Errorf("permission denied for %s action (%s risk): %s", result.Action, result.Risk, result.Reason)
	}
	return fmt.Errorf("permission denied for %s action (%s risk)", result.Action, result.Risk)
}

func mergeRunMetadata(base map[string]any, extra map[string]any) map[string]any {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	merged := make(map[string]any, len(base)+len(extra))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		if value != nil {
			merged[key] = value
		}
	}
	return merged
}

func (r Runner) driver(kind agents.AdapterKind) agents.ConnectionDriver {
	if r.Driver != nil {
		return r.Driver
	}
	switch kind {
	case agents.AdapterJSONRPCStdio, agents.AdapterACPStdio:
		return agents.JSONRPCStdioDriver{Enabled: true}
	}
	return agents.CommandOnceDriver{}
}

func (r Runner) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func (r Runner) newRunID() string {
	if r.NewRunID != nil {
		if id := strings.TrimSpace(r.NewRunID()); id != "" {
			return id
		}
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "agent_run_unavailable"
	}
	return "agent_run_" + hex.EncodeToString(bytes[:])
}

func durationMillis(startedAt time.Time, completedAt time.Time) int64 {
	if completedAt.Before(startedAt) {
		return 0
	}
	return completedAt.Sub(startedAt).Milliseconds()
}

func nullableRunString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullableRunInt64(value int64) sql.NullInt64 {
	return sql.NullInt64{Int64: value, Valid: true}
}

func nullableEventExitCode(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return nullableRunInt64(int64(*value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
