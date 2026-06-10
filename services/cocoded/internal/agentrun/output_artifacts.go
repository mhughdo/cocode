package agentrun

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const rawOutputArtifactKind = "raw_output"

type OutputRecorder struct {
	Artifacts *artifact.Store
	Queries   *dbgen.Queries
	Now       func() time.Time
}

type OutputArtifactParams struct {
	WorkspaceID string
	Task        agents.AgentTask
	Events      []agents.AgentEvent
}

type OutputArtifactResult struct {
	Stdout dbgen.Artifact
	Stderr dbgen.Artifact
	Run    dbgen.AgentRun
}

type streamCapture struct {
	Stream    string
	Content   bytes.Buffer
	Truncated bool
	Limit     int64
}

func (r OutputRecorder) SaveRawOutputs(ctx context.Context, params OutputArtifactParams) (OutputArtifactResult, error) {
	if r.Artifacts == nil {
		return OutputArtifactResult{}, errors.New("artifact store is required")
	}
	if r.Queries == nil {
		return OutputArtifactResult{}, errors.New("agent run queries are required")
	}
	if strings.TrimSpace(params.WorkspaceID) == "" {
		return OutputArtifactResult{}, errors.New("workspace id is required")
	}
	if strings.TrimSpace(params.Task.RunID) == "" {
		return OutputArtifactResult{}, errors.New("agent task run id is required")
	}
	if strings.TrimSpace(params.Task.ReviewSessionID) == "" {
		return OutputArtifactResult{}, errors.New("agent task review session id is required")
	}
	if strings.TrimSpace(params.Task.AgentConfigID) == "" {
		return OutputArtifactResult{}, errors.New("agent task config id is required")
	}

	run, err := r.Queries.GetAgentRun(ctx, params.Task.RunID)
	if err != nil {
		return OutputArtifactResult{}, fmt.Errorf("get agent run: %w", err)
	}
	if run.ReviewSessionID != params.Task.ReviewSessionID || run.AgentConfigID != params.Task.AgentConfigID {
		return OutputArtifactResult{}, errors.New("agent task does not match stored run")
	}

	captures := captureStreamOutputs(params.Task.RunID, params.Events)
	result := OutputArtifactResult{Run: run}
	if stdout, ok := captures["stdout"]; ok && stdout.ShouldStore() {
		artifact, err := r.saveStream(ctx, params, *stdout)
		if err != nil {
			return OutputArtifactResult{}, err
		}
		result.Stdout = artifact
		run.StdoutArtifactID = sql.NullString{String: artifact.ID, Valid: true}
	}
	if stderr, ok := captures["stderr"]; ok && stderr.ShouldStore() {
		artifact, err := r.saveStream(ctx, params, *stderr)
		if err != nil {
			return OutputArtifactResult{}, err
		}
		result.Stderr = artifact
		run.StderrArtifactID = sql.NullString{String: artifact.ID, Valid: true}
	}
	if result.Stdout.ID == "" && result.Stderr.ID == "" {
		return result, nil
	}
	run.MetadataJson = outputRunMetadataJSON(run.MetadataJson, captures)

	updated, err := r.Queries.UpdateAgentRunStatus(ctx, dbgen.UpdateAgentRunStatusParams{
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
	if err != nil {
		return OutputArtifactResult{}, fmt.Errorf("update agent run output artifacts: %w", err)
	}
	result.Run = updated
	return result, nil
}

func outputRunMetadataJSON(raw string, captures map[string]*streamCapture) string {
	metadata := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &metadata)
	}
	var outputBytes int64
	for stream, capture := range captures {
		if capture == nil {
			continue
		}
		keyPrefix := stream + "_"
		metadata[keyPrefix+"bytes"] = int64(capture.Content.Len())
		metadata[keyPrefix+"truncated"] = capture.Truncated
		outputBytes += int64(capture.Content.Len())
	}
	if outputBytes > 0 {
		metadata["output_bytes"] = outputBytes
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return raw
	}
	return string(encoded)
}

func (r OutputRecorder) saveStream(ctx context.Context, params OutputArtifactParams, capture streamCapture) (dbgen.Artifact, error) {
	metadata := map[string]any{
		"agent_config_id": params.Task.AgentConfigID,
		"captured_bytes":  int64(capture.Content.Len()),
		"run_id":          params.Task.RunID,
		"stream":          capture.Stream,
		"truncated":       capture.Truncated,
	}
	if strings.TrimSpace(params.Task.ID) != "" {
		metadata["task_id"] = params.Task.ID
	}
	if capture.Limit > 0 {
		metadata["limit_bytes"] = capture.Limit
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return dbgen.Artifact{}, fmt.Errorf("encode output artifact metadata: %w", err)
	}

	saved, err := r.Artifacts.Save(ctx, artifact.SaveParams{
		ID:              outputArtifactID(params.Task.RunID, capture.Stream),
		WorkspaceID:     params.WorkspaceID,
		ReviewSessionID: sql.NullString{String: params.Task.ReviewSessionID, Valid: true},
		Kind:            rawOutputArtifactKind,
		RelativePath:    outputArtifactPath(params.Task.ReviewSessionID, params.Task.RunID, capture.Stream),
		ContentType:     "text/plain",
		MetadataJSON:    string(metadataJSON),
		CreatedAt:       r.now().Format(time.RFC3339Nano),
	}, capture.Content.Bytes())
	if err != nil {
		return dbgen.Artifact{}, fmt.Errorf("save %s output artifact: %w", capture.Stream, err)
	}
	return saved, nil
}

func (r OutputRecorder) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func captureStreamOutputs(runID string, events []agents.AgentEvent) map[string]*streamCapture {
	captures := map[string]*streamCapture{}
	for _, event := range events {
		if event.Type != agents.EventOutput || event.RunID != runID || !knownOutputStream(event.Stream) {
			continue
		}
		capture := captures[event.Stream]
		if capture == nil {
			capture = &streamCapture{Stream: event.Stream}
			captures[event.Stream] = capture
		}
		capture.Content.WriteString(event.Text)
		capture.Truncated = capture.Truncated || event.Truncated
		if limit := metadataInt64(event.Metadata, "limit_bytes"); limit > capture.Limit {
			capture.Limit = limit
		}
	}
	return captures
}

func knownOutputStream(stream string) bool {
	return stream == "stdout" || stream == "stderr"
}

func (c streamCapture) ShouldStore() bool {
	return c.Content.Len() > 0 || c.Truncated
}

func metadataInt64(metadata map[string]any, key string) int64 {
	if metadata == nil {
		return 0
	}
	switch value := metadata[key].(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case json.Number:
		out, _ := value.Int64()
		return out
	default:
		return 0
	}
}

func outputArtifactID(runID string, stream string) string {
	digest := sha256.Sum256([]byte(runID + "\x00" + stream))
	return "artifact_agent_output_" + hex.EncodeToString(digest[:])[:24]
}

func outputArtifactPath(reviewSessionID string, runID string, stream string) string {
	return path.Join(
		"review_sessions",
		safeArtifactSegment(reviewSessionID),
		"agent_runs",
		safeArtifactSegment(runID),
		stream+".txt",
	)
}

var unsafeArtifactSegment = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func safeArtifactSegment(value string) string {
	value = strings.TrimSpace(value)
	value = unsafeArtifactSegment.ReplaceAllString(value, "-")
	value = strings.Trim(value, ".-")
	if value == "" {
		return "unknown"
	}
	if len(value) > 80 {
		return value[:80]
	}
	return value
}
