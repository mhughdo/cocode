package agentrun

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

func TestRunnerStreamsCommandEventsBeforeRunCompletes(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	task := runnerTask(env, "agent_run_streaming")
	command := writeFakeAgent(t, "#!/bin/sh\nprintf 'partial\\n'\n/bin/sleep 0.2\nprintf 'done\\n'\n")
	outputSeen := make(chan agents.AgentEvent, 1)
	resultCh := make(chan RunResult, 1)
	errCh := make(chan error, 1)

	go func() {
		result, err := runnerWithClock(env).Execute(context.Background(), RunParams{
			WorkspaceID: env.WorkspaceID,
			Config:      runnerConfig(command),
			Task:        task,
			EventSink: func(_ context.Context, event agents.AgentEvent) {
				if event.Type == agents.EventOutput && strings.Contains(event.Text, "partial") {
					select {
					case outputSeen <- event:
					default:
					}
				}
			},
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	select {
	case event := <-outputSeen:
		if event.RunID != task.RunID || event.Stream != "stdout" {
			t.Fatalf("streamed event = %+v", event)
		}
		run, err := env.Queries.GetAgentRun(context.Background(), task.RunID)
		if err != nil {
			t.Fatalf("GetAgentRun() error = %v", err)
		}
		if run.Status != RunStatusRunning {
			t.Fatalf("run status after streamed output = %s, want running", run.Status)
		}
	case err := <-errCh:
		t.Fatalf("Execute() error before stream = %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for streamed output")
	}

	select {
	case result := <-resultCh:
		if result.Run.Status != RunStatusSucceeded {
			t.Fatalf("run = %+v", result.Run)
		}
		stdout, _, err := env.Artifacts.Read(context.Background(), result.Run.StdoutArtifactID.String)
		if err != nil {
			t.Fatalf("Read(stdout) error = %v", err)
		}
		if string(stdout) != "partial\ndone\n" {
			t.Fatalf("stdout = %q", string(stdout))
		}
	case err := <-errCh:
		t.Fatalf("Execute() error = %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for completed run")
	}
}

func TestRunnerUsesProtocolDriverForJSONRPCAdapters(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	task := runnerTask(env, "agent_run_jsonrpc")
	command := writeFakeAgent(t, `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"id":1,"result":{"userAgent":"fake-codex","codexHome":"/tmp/cocode","platformFamily":"unix","platformOs":"macos"}}'
      ;;
    *'"method":"initialized"'*)
      ;;
    *'"method":"thread/start"'*)
      printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread_1"}}}'
      ;;
    *'"method":"turn/start"'*)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn_1","status":"inProgress"}}}'
      printf '%s\n' '{"method":"item/agentMessage/delta","params":{"threadId":"thread_1","turnId":"turn_1","itemId":"item_1","delta":"{\"findings\":[]}"}}'
      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thread_1","turn":{"id":"turn_1","status":"completed"}}}'
      ;;
  esac
done
`)
	config := runnerConfig(command)
	config.Kind = agents.AdapterJSONRPCStdio
	config.WorkingDirectory = t.TempDir()
	config.Metadata = map[string]any{
		"model_label":     "fake-model",
		"reasoning_label": "high",
	}

	result, err := runnerWithClock(env).Execute(context.Background(), RunParams{
		WorkspaceID: env.WorkspaceID,
		Config:      config,
		Task:        task,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Run.Status != RunStatusSucceeded ||
		!result.Run.ExitCode.Valid ||
		result.Run.ExitCode.Int64 != 0 ||
		!result.Run.StdoutArtifactID.Valid {
		t.Fatalf("run = %+v", result.Run)
	}
	stdout, _, err := env.Artifacts.Read(context.Background(), result.Run.StdoutArtifactID.String)
	if err != nil {
		t.Fatalf("Read(stdout) error = %v", err)
	}
	if string(stdout) != `{"findings":[]}` {
		t.Fatalf("stdout = %q", string(stdout))
	}
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
	driver := &scriptedDriver{
		events: []agents.AgentEvent{
			{
				Type:     agents.EventOutput,
				RunID:    task.RunID,
				Stream:   "stdout",
				Text:     "partial before timeout\n",
				Metadata: map[string]any{"limit_bytes": int64(1 << 20)},
			},
			{
				Type:      agents.EventCanceled,
				RunID:     task.RunID,
				ErrorCode: "timeout",
				Error:     context.DeadlineExceeded.Error(),
			},
		},
	}
	runner := runnerWithClock(env)
	runner.Driver = driver
	result, err := runner.Execute(context.Background(), RunParams{
		WorkspaceID: env.WorkspaceID,
		Config:      runnerConfig("fake-agent"),
		Task:        task,
		TimeoutPolicy: TimeoutPolicy{
			AgentTimeout:  time.Second,
			ReviewTimeout: 200 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Run.Status != RunStatusTimedOut ||
		result.Run.ErrorCode.String != "timeout" ||
		!strings.Contains(result.Run.ErrorMessage.String, "deadline") ||
		result.Run.DurationMs.Int64 != 2000 ||
		!result.Run.StdoutArtifactID.Valid {
		t.Fatalf("run = %+v", result.Run)
	}
	stdout, _, err := env.Artifacts.Read(context.Background(), result.Run.StdoutArtifactID.String)
	if err != nil {
		t.Fatalf("Read(stdout) error = %v", err)
	}
	if string(stdout) != "partial before timeout\n" {
		t.Fatalf("stdout = %q", string(stdout))
	}
	if driver.task.Limits.Timeout != 200*time.Millisecond {
		t.Fatalf("driver task timeout = %s, want 200ms", driver.task.Limits.Timeout)
	}
	assertRunMetadata(t, result.Run.MetadataJson, map[string]any{
		"agent_config_id": task.AgentConfigID,
		"task_id":         task.ID,
	})
	assertRunTimeoutMetadata(t, result.Run.MetadataJson, "review")
}

func TestRunnerMarksExpiredReviewLimitWithoutLaunchingCommand(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	task := runnerTask(env, "agent_run_review_expired")
	markerPath := filepath.Join(t.TempDir(), "started")
	command := writeFakeAgent(t, "#!/bin/sh\nprintf started > \"$1\"\n")
	start := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	result, err := runnerWithClockAt(env, start).Execute(context.Background(), RunParams{
		WorkspaceID: env.WorkspaceID,
		Config:      runnerConfigWithArgs(command, []string{markerPath}),
		Task:        task,
		TimeoutPolicy: TimeoutPolicy{
			ReviewDeadline: start.Add(-time.Millisecond),
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Run.Status != RunStatusTimedOut ||
		result.Run.ErrorCode.String != "timeout" ||
		!strings.Contains(result.Run.ErrorMessage.String, "runtime limit exceeded") ||
		result.Run.StdoutArtifactID.Valid ||
		result.Run.StderrArtifactID.Valid {
		t.Fatalf("run = %+v", result.Run)
	}
	if _, err := os.Stat(markerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("marker stat error = %v, want not executed", err)
	}
	assertRunTimeoutMetadata(t, result.Run.MetadataJson, "review_deadline")
}

func TestRunnerDeniesReviewModeWriteCapability(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	task := runnerTask(env, "agent_run_permission_denied")
	driver := &scriptedDriver{
		events: []agents.AgentEvent{{
			Type:  agents.EventCompleted,
			RunID: task.RunID,
		}},
	}
	runner := runnerWithClock(env)
	runner.Driver = driver

	result, err := runner.Execute(context.Background(), RunParams{
		WorkspaceID: env.WorkspaceID,
		Config:      runnerConfig("fake-agent"),
		Capabilities: agents.AgentCapabilities{
			CanRead:  true,
			CanWrite: true,
		},
		Permissions: agents.ReviewModePermissionPolicy(),
		Task:        task,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if driver.opened {
		t.Fatal("driver opened despite denied write permission")
	}
	if result.Run.Status != RunStatusFailed ||
		result.Run.ErrorCode.String != "permission_denied" ||
		!strings.Contains(result.Run.ErrorMessage.String, "write-capable") {
		t.Fatalf("run = %+v", result.Run)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(result.Run.MetadataJson), &metadata); err != nil {
		t.Fatalf("Unmarshal(metadata) error = %v", err)
	}
	if metadata["permission_policy"] == nil {
		t.Fatalf("permission policy metadata missing: %+v", metadata)
	}
}

func TestRunnerDeniesUnsafeReviewModeRuntimeBeforeOpen(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	task := runnerTask(env, "agent_run_runtime_denied")
	driver := &scriptedDriver{
		events: []agents.AgentEvent{{
			Type:  agents.EventCompleted,
			RunID: task.RunID,
		}},
	}
	runner := runnerWithClock(env)
	runner.Driver = driver

	config := runnerConfig("agy")
	config.Args = []string{"--print", "--dangerously-skip-permissions"}
	result, err := runner.Execute(context.Background(), RunParams{
		WorkspaceID:  env.WorkspaceID,
		Config:       config,
		Capabilities: agents.AgentCapabilities{CanRead: true},
		Permissions:  agents.ReviewModePermissionPolicy(),
		Task:         task,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if driver.opened {
		t.Fatal("driver opened despite unsafe review runtime")
	}
	if result.Run.Status != RunStatusFailed ||
		result.Run.ErrorCode.String != "permission_denied" ||
		!strings.Contains(result.Run.ErrorMessage.String, "dangerously-skip-permissions") {
		t.Fatalf("run = %+v", result.Run)
	}
}

func TestTimeoutPolicyRejectsNegativeLimits(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	_, err := runnerWithClock(env).Execute(context.Background(), RunParams{
		WorkspaceID: env.WorkspaceID,
		Config:      runnerConfig(writeFakeAgent(t, "#!/bin/sh\nexit 0\n")),
		Task:        runnerTask(env, "agent_run_bad_timeout"),
		TimeoutPolicy: TimeoutPolicy{
			AgentTimeoutSeconds: -1,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("Execute() error = %v, want negative timeout error", err)
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
	return runnerConfigWithArgs(command, nil)
}

func runnerConfigWithArgs(command string, args []string) agents.ConnectionConfig {
	return agents.ConnectionConfig{
		AdapterID: "agent_config_1",
		Kind:      agents.AdapterCLINonInteractive,
		Command:   command,
		Args:      args,
	}
}

func runnerWithClock(env outputRecorderEnv) Runner {
	return runnerWithClockAt(env, time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC))
}

func runnerWithClockAt(env outputRecorderEnv, start time.Time) Runner {
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

func assertRunTimeoutMetadata(t *testing.T, raw string, source string) {
	t.Helper()

	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal(metadata) error = %v", err)
	}
	policy, ok := got["timeout_policy"].(map[string]any)
	if !ok {
		t.Fatalf("timeout_policy metadata missing in %+v", got)
	}
	if policy["effective_timeout_source"] != source {
		t.Fatalf("timeout source = %v, want %s; policy = %+v", policy["effective_timeout_source"], source, policy)
	}
}

type scriptedDriver struct {
	task   agents.AgentTask
	events []agents.AgentEvent
	opened bool
}

func (d *scriptedDriver) Open(context.Context, agents.ConnectionConfig) (agents.Connection, error) {
	d.opened = true
	return scriptedConnection{driver: d}, nil
}

type scriptedConnection struct {
	driver *scriptedDriver
}

func (c scriptedConnection) SendTask(_ context.Context, task agents.AgentTask) (<-chan agents.AgentEvent, error) {
	c.driver.task = task
	events := make(chan agents.AgentEvent, len(c.driver.events))
	for _, event := range c.driver.events {
		events <- event
	}
	close(events)
	return events, nil
}

func (scriptedConnection) Close(context.Context) error {
	return nil
}
