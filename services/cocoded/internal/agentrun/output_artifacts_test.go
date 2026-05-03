package agentrun

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestOutputRecorderSavesCommandOutputsAndLinksRun(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	command := writeFakeAgent(t, "#!/bin/sh\nprintf 'hello stdout\\n'\nprintf 'warn stderr\\n' >&2\n")
	connection := openCommandConnection(t, command)
	events := runCommandTask(t, connection, env.Task)

	result, err := env.Recorder.SaveRawOutputs(context.Background(), OutputArtifactParams{
		WorkspaceID: env.WorkspaceID,
		Task:        env.Task,
		Events:      events,
	})
	if err != nil {
		t.Fatalf("SaveRawOutputs() error = %v", err)
	}
	if result.Stdout.ID == "" || result.Stderr.ID == "" {
		t.Fatalf("result = %+v, want stdout and stderr artifacts", result)
	}
	if result.Run.StdoutArtifactID.String != result.Stdout.ID ||
		result.Run.StderrArtifactID.String != result.Stderr.ID ||
		result.Run.Status != "running" ||
		result.Run.MetadataJson != `{"phase":"executing"}` {
		t.Fatalf("updated run = %+v", result.Run)
	}

	stdoutContent, stdoutArtifact, err := env.Artifacts.Read(context.Background(), result.Stdout.ID)
	if err != nil {
		t.Fatalf("Read(stdout) error = %v", err)
	}
	if string(stdoutContent) != "hello stdout\n" {
		t.Fatalf("stdout content = %q", string(stdoutContent))
	}
	assertOutputArtifact(t, stdoutArtifact, env, "stdout", false, int64(len(stdoutContent)), 1<<20)

	stderrContent, stderrArtifact, err := env.Artifacts.Read(context.Background(), result.Stderr.ID)
	if err != nil {
		t.Fatalf("Read(stderr) error = %v", err)
	}
	if string(stderrContent) != "warn stderr\n" {
		t.Fatalf("stderr content = %q", string(stderrContent))
	}
	assertOutputArtifact(t, stderrArtifact, env, "stderr", false, int64(len(stderrContent)), 1<<20)
}

func TestOutputRecorderPersistsTruncatedOutputFromFailedCommand(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	command := writeFakeAgent(t, "#!/bin/sh\nprintf 'abcdef'\nprintf 'wxyzq' >&2\nexit 7\n")
	limits := agents.TaskLimits{MaxStdoutBytes: 3, MaxStderrBytes: 4}
	connection := openCommandConnection(t, command)
	task := env.Task
	task.Limits = limits
	events := runCommandTask(t, connection, task)
	terminal := events[len(events)-1]
	if terminal.Type != agents.EventFailed || terminal.ExitCode == nil || *terminal.ExitCode != 7 {
		t.Fatalf("terminal event = %+v, want failed exit 7", terminal)
	}

	result, err := env.Recorder.SaveRawOutputs(context.Background(), OutputArtifactParams{
		WorkspaceID: env.WorkspaceID,
		Task:        task,
		Events:      events,
	})
	if err != nil {
		t.Fatalf("SaveRawOutputs() error = %v", err)
	}

	stdoutContent, stdoutArtifact, err := env.Artifacts.Read(context.Background(), result.Stdout.ID)
	if err != nil {
		t.Fatalf("Read(stdout) error = %v", err)
	}
	if string(stdoutContent) != "abc" {
		t.Fatalf("stdout content = %q", string(stdoutContent))
	}
	assertOutputArtifact(t, stdoutArtifact, env, "stdout", true, 3, 3)

	stderrContent, stderrArtifact, err := env.Artifacts.Read(context.Background(), result.Stderr.ID)
	if err != nil {
		t.Fatalf("Read(stderr) error = %v", err)
	}
	if string(stderrContent) != "wxyz" {
		t.Fatalf("stderr content = %q", string(stderrContent))
	}
	assertOutputArtifact(t, stderrArtifact, env, "stderr", true, 4, 4)
}

func TestOutputRecorderSkipsEmptyStreams(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	result, err := env.Recorder.SaveRawOutputs(context.Background(), OutputArtifactParams{
		WorkspaceID: env.WorkspaceID,
		Task:        env.Task,
		Events: []agents.AgentEvent{
			{Type: agents.EventCompleted, RunID: env.Task.RunID},
		},
	})
	if err != nil {
		t.Fatalf("SaveRawOutputs() error = %v", err)
	}
	if result.Stdout.ID != "" || result.Stderr.ID != "" {
		t.Fatalf("result = %+v, want no artifacts", result)
	}
	if result.Run.StdoutArtifactID.Valid || result.Run.StderrArtifactID.Valid {
		t.Fatalf("run artifact ids = stdout %v stderr %v, want empty", result.Run.StdoutArtifactID, result.Run.StderrArtifactID)
	}
	artifacts, err := env.Queries.ListArtifactsByWorkspace(context.Background(), env.WorkspaceID)
	if err != nil {
		t.Fatalf("ListArtifactsByWorkspace() error = %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("artifacts = %+v, want empty", artifacts)
	}
}

type outputRecorderEnv struct {
	WorkspaceID string
	Task        agents.AgentTask
	Queries     *dbgen.Queries
	Artifacts   *artifact.Store
	Recorder    OutputRecorder
}

func setupOutputRecorder(t *testing.T) outputRecorderEnv {
	t.Helper()

	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	queries := dbgen.New(database)
	createdAt := "2026-05-03T00:00:00Z"
	workspaceID := "workspace_1"
	task := agents.AgentTask{
		ID:              "task_1",
		RunID:           "agent_run_1",
		ReviewSessionID: "review_session_1",
		AgentConfigID:   "agent_config_1",
		Role:            "reviewer",
		Prompt:          "review this diff",
	}

	if _, err := queries.CreateWorkspace(context.Background(), dbgen.CreateWorkspaceParams{
		ID:           workspaceID,
		Name:         "cocode",
		RootPath:     "/tmp/cocode",
		SettingsJson: "{}",
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
	}); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, err := queries.CreateRepository(context.Background(), dbgen.CreateRepositoryParams{
		ID:            "repo_1",
		WorkspaceID:   workspaceID,
		Name:          "cocode",
		LocalPath:     "/tmp/cocode",
		DefaultBranch: sql.NullString{String: "main", Valid: true},
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}); err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
	if _, err := queries.CreatePullRequestSnapshot(context.Background(), dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_1",
		RepositoryID: "repo_1",
		SourceType:   "local_changes",
		MetadataJson: "{}",
		CreatedAt:    createdAt,
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}
	if _, err := queries.CreateReviewSession(context.Background(), dbgen.CreateReviewSessionParams{
		ID:                  task.ReviewSessionID,
		WorkspaceID:         workspaceID,
		RepositoryID:        "repo_1",
		SnapshotID:          "snapshot_1",
		Title:               "Review cocode",
		Status:              "running",
		ReviewDepth:         "standard",
		RuntimeLimitSeconds: 1800,
		ContextPolicyJson:   "{}",
		CreatedAt:           createdAt,
		UpdatedAt:           createdAt,
	}); err != nil {
		t.Fatalf("CreateReviewSession() error = %v", err)
	}
	capabilities, err := agents.DefaultCapabilities(agents.AdapterCLINonInteractive).EncodeJSON()
	if err != nil {
		t.Fatalf("EncodeJSON() error = %v", err)
	}
	if _, err := queries.CreateAgentConfig(context.Background(), dbgen.CreateAgentConfigParams{
		ID:               task.AgentConfigID,
		Name:             "Codex reviewer",
		Role:             "reviewer",
		AdapterKind:      string(agents.AdapterCLINonInteractive),
		Command:          sql.NullString{String: "fake-agent", Valid: true},
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       string(agents.OutputText),
		CapabilitiesJson: capabilities,
		SettingsJson:     "{}",
		Enabled:          1,
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt,
	}); err != nil {
		t.Fatalf("CreateAgentConfig() error = %v", err)
	}
	if _, err := queries.CreateAgentRun(context.Background(), dbgen.CreateAgentRunParams{
		ID:              task.RunID,
		ReviewSessionID: task.ReviewSessionID,
		AgentConfigID:   task.AgentConfigID,
		Status:          "running",
		Role:            task.Role,
		StartedAt:       sql.NullString{String: createdAt, Valid: true},
		MetadataJson:    `{"phase":"executing"}`,
	}); err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	artifacts, err := artifact.New(filepath.Join(t.TempDir(), "artifacts"), queries)
	if err != nil {
		t.Fatalf("artifact.New() error = %v", err)
	}
	recorder := OutputRecorder{
		Artifacts: artifacts,
		Queries:   queries,
		Now: func() time.Time {
			return time.Date(2026, 5, 3, 0, 5, 0, 0, time.UTC)
		},
	}
	return outputRecorderEnv{
		WorkspaceID: workspaceID,
		Task:        task,
		Queries:     queries,
		Artifacts:   artifacts,
		Recorder:    recorder,
	}
}

func openCommandConnection(t *testing.T, command string) agents.Connection {
	t.Helper()

	connection, err := (agents.CommandOnceDriver{}).Open(context.Background(), agents.ConnectionConfig{
		AdapterID:        "agent_config_1",
		Kind:             agents.AdapterCLINonInteractive,
		Command:          command,
		WorkingDirectory: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return connection
}

func runCommandTask(t *testing.T, connection agents.Connection, task agents.AgentTask) []agents.AgentEvent {
	t.Helper()

	events, err := connection.SendTask(context.Background(), task)
	if err != nil {
		t.Fatalf("SendTask() error = %v", err)
	}
	return collectEvents(t, events)
}

func writeFakeAgent(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-agent")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func collectEvents(t *testing.T, events <-chan agents.AgentEvent) []agents.AgentEvent {
	t.Helper()

	timeout := time.After(10 * time.Second)
	var got []agents.AgentEvent
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return got
			}
			got = append(got, event)
		case <-timeout:
			t.Fatal("timed out waiting for command events")
		}
	}
}

func assertOutputArtifact(t *testing.T, artifact dbgen.Artifact, env outputRecorderEnv, stream string, truncated bool, capturedBytes int64, limitBytes int64) {
	t.Helper()

	if artifact.WorkspaceID != env.WorkspaceID ||
		artifact.ReviewSessionID.String != env.Task.ReviewSessionID ||
		artifact.Kind != rawOutputArtifactKind ||
		artifact.ContentType != "text/plain" ||
		!strings.HasSuffix(artifact.RelativePath, "/"+stream+".txt") {
		t.Fatalf("artifact = %+v", artifact)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(artifact.MetadataJson), &metadata); err != nil {
		t.Fatalf("Unmarshal(metadata) error = %v", err)
	}
	if metadata["run_id"] != env.Task.RunID ||
		metadata["agent_config_id"] != env.Task.AgentConfigID ||
		metadata["task_id"] != env.Task.ID ||
		metadata["stream"] != stream ||
		metadata["truncated"] != truncated ||
		int64(metadata["captured_bytes"].(float64)) != capturedBytes ||
		int64(metadata["limit_bytes"].(float64)) != limitBytes {
		t.Fatalf("metadata = %+v", metadata)
	}
	if _, err := env.Queries.GetArtifact(context.Background(), artifact.ID); err != nil {
		t.Fatalf("GetArtifact() error = %v", err)
	}
}

func TestOutputRecorderValidation(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	_, err := (OutputRecorder{}).SaveRawOutputs(context.Background(), OutputArtifactParams{
		WorkspaceID: env.WorkspaceID,
		Task:        env.Task,
	})
	if err == nil {
		t.Fatal("SaveRawOutputs() error = nil, want missing dependencies error")
	}
	_, err = env.Recorder.SaveRawOutputs(context.Background(), OutputArtifactParams{
		WorkspaceID: "",
		Task:        env.Task,
	})
	if err == nil {
		t.Fatal("SaveRawOutputs() error = nil, want missing workspace error")
	}
	missingRunTask := env.Task
	missingRunTask.RunID = "missing_run"
	_, err = env.Recorder.SaveRawOutputs(context.Background(), OutputArtifactParams{
		WorkspaceID: env.WorkspaceID,
		Task:        missingRunTask,
	})
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("SaveRawOutputs() error = %v, want sql.ErrNoRows", err)
	}
	mismatchedTask := env.Task
	mismatchedTask.AgentConfigID = "agent_config_other"
	_, err = env.Recorder.SaveRawOutputs(context.Background(), OutputArtifactParams{
		WorkspaceID: env.WorkspaceID,
		Task:        mismatchedTask,
	})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("SaveRawOutputs() error = %v, want run mismatch", err)
	}
}
