package agentrun

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
)

func TestRunnerPersistsSuccessfulCommandRun(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	task := runnerTask(env, "agent_run_success")
	command := writeFakeAgent(t, "#!/bin/sh\nprintf 'review ok\\n'\nprintf 'note\\n' >&2\n")
	result, err := runnerWithClock(env).Execute(context.Background(), RunParams{
		WorkspaceID: env.WorkspaceID,
		Config:      runnerConfig(command),
		Task:        task,
		Metadata:    map[string]any{"phase": "review"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Run.Status != RunStatusSucceeded ||
		!result.Run.StartedAt.Valid ||
		!result.Run.CompletedAt.Valid ||
		!result.Run.DurationMs.Valid ||
		result.Run.DurationMs.Int64 != 2000 ||
		!result.Run.ExitCode.Valid ||
		result.Run.ExitCode.Int64 != 0 ||
		!result.Run.StdoutArtifactID.Valid ||
		!result.Run.StderrArtifactID.Valid ||
		result.Run.ErrorCode.Valid {
		t.Fatalf("run = %+v", result.Run)
	}
	if len(result.Events) < 3 || result.Events[0].Type != agents.EventQueued || result.Events[len(result.Events)-1].Type != agents.EventCompleted {
		t.Fatalf("events = %+v", result.Events)
	}
	stdout, _, err := env.Artifacts.Read(context.Background(), result.Run.StdoutArtifactID.String)
	if err != nil {
		t.Fatalf("Read(stdout) error = %v", err)
	}
	if string(stdout) != "review ok\n" {
		t.Fatalf("stdout = %q", string(stdout))
	}
	stderr, _, err := env.Artifacts.Read(context.Background(), result.Run.StderrArtifactID.String)
	if err != nil {
		t.Fatalf("Read(stderr) error = %v", err)
	}
	if string(stderr) != "note\n" {
		t.Fatalf("stderr = %q", string(stderr))
	}
	assertRunMetadata(t, result.Run.MetadataJson, map[string]any{
		"agent_config_id": task.AgentConfigID,
		"phase":           "review",
		"task_id":         task.ID,
	})
}

func TestRunnerPersistsFailedCommandRun(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	task := runnerTask(env, "agent_run_failed")
	command := writeFakeAgent(t, "#!/bin/sh\nprintf 'agent failed\\n' >&2\nexit 7\n")
	result, err := runnerWithClock(env).Execute(context.Background(), RunParams{
		WorkspaceID: env.WorkspaceID,
		Config:      runnerConfig(command),
		Task:        task,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Run.Status != RunStatusFailed ||
		!result.Run.ExitCode.Valid ||
		result.Run.ExitCode.Int64 != 7 ||
		result.Run.ErrorCode.String != "exit_error" ||
		result.Run.ErrorMessage.String != "agent failed" ||
		!result.Run.StderrArtifactID.Valid ||
		result.Run.StdoutArtifactID.Valid {
		t.Fatalf("run = %+v", result.Run)
	}
	stderr, _, err := env.Artifacts.Read(context.Background(), result.Run.StderrArtifactID.String)
	if err != nil {
		t.Fatalf("Read(stderr) error = %v", err)
	}
	if string(stderr) != "agent failed\n" {
		t.Fatalf("stderr = %q", string(stderr))
	}
}

func TestRunnerPersistsCanceledCommandRunAfterParentCancellation(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	task := runnerTask(env, "agent_run_canceled")
	command := writeFakeAgent(t, "#!/bin/sh\n/bin/sleep 1\n")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	result, err := runnerWithClock(env).Execute(ctx, RunParams{
		WorkspaceID: env.WorkspaceID,
		Config:      runnerConfig(command),
		Task:        task,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Run.Status != RunStatusCanceled ||
		result.Run.ErrorCode.String != "canceled" ||
		!strings.Contains(result.Run.ErrorMessage.String, "canceled") ||
		result.Run.DurationMs.Int64 != 2000 {
		t.Fatalf("run = %+v", result.Run)
	}
}

func TestRunnerPersistsTimedOutCommandRun(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	task := runnerTask(env, "agent_run_timeout")
	task.Limits.Timeout = 20 * time.Millisecond
	command := writeFakeAgent(t, "#!/bin/sh\n/bin/sleep 1\n")
	result, err := runnerWithClock(env).Execute(context.Background(), RunParams{
		WorkspaceID: env.WorkspaceID,
		Config:      runnerConfig(command),
		Task:        task,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Run.Status != RunStatusTimedOut ||
		result.Run.ErrorCode.String != "timeout" ||
		!strings.Contains(result.Run.ErrorMessage.String, "deadline") ||
		result.Run.DurationMs.Int64 != 2000 {
		t.Fatalf("run = %+v", result.Run)
	}
}

func TestRunnerGeneratesRunIDAndPersistsOpenError(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	task := runnerTask(env, "")
	task.RunID = ""
	task.ID = ""
	result, err := runnerWithClock(env).Execute(context.Background(), RunParams{
		WorkspaceID: env.WorkspaceID,
		Config: agents.ConnectionConfig{
			AdapterID:        task.AgentConfigID,
			Kind:             agents.AdapterCLINonInteractive,
			Command:          writeFakeAgent(t, "#!/bin/sh\nexit 0\n"),
			WorkingDirectory: "/path/that/does/not/exist",
		},
		Task: task,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Run.ID != "agent_run_generated" ||
		result.Run.Status != RunStatusFailed ||
		result.Run.ErrorCode.String != "open_error" ||
		!strings.Contains(result.Run.ErrorMessage.String, "working directory") {
		t.Fatalf("run = %+v", result.Run)
	}
}

func runnerTask(env outputRecorderEnv, runID string) agents.AgentTask {
	task := env.Task
	task.ID = runID + "_task"
	task.RunID = runID
	return task
}

func runnerConfig(command string) agents.ConnectionConfig {
	return agents.ConnectionConfig{
		AdapterID: "agent_config_1",
		Kind:      agents.AdapterCLINonInteractive,
		Command:   command,
	}
}

func runnerWithClock(env outputRecorderEnv) Runner {
	start := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	calls := 0
	return Runner{
		Queries:   env.Queries,
		Artifacts: env.Artifacts,
		NewRunID: func() string {
			return "agent_run_generated"
		},
		Now: func() time.Time {
			calls++
			if calls == 1 {
				return start
			}
			return start.Add(2 * time.Second)
		},
	}
}

func assertRunMetadata(t *testing.T, raw string, want map[string]any) {
	t.Helper()

	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal(metadata) error = %v", err)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("metadata[%s] = %v, want %v; metadata = %+v", key, got[key], value, got)
		}
	}
}
