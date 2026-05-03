package agentrun

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestManagerCancelsSingleRunWithoutBlockingSession(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	manager := &Manager{Runner: runnerWithClock(env)}
	cancelTask := runnerTask(env, "agent_run_cancel_one")
	cancelCommand := writeFakeAgent(t, "#!/bin/sh\nexec /bin/sleep 2\n")
	resultCh := make(chan RunResult, 1)
	errorCh := make(chan error, 1)
	go func() {
		result, err := manager.Execute(context.Background(), RunParams{
			WorkspaceID: env.WorkspaceID,
			Config:      runnerConfig(cancelCommand),
			Task:        cancelTask,
		})
		resultCh <- result
		errorCh <- err
	}()

	waitForActiveRun(t, manager, cancelTask.RunID)
	if err := manager.Cancel(context.Background(), cancelTask.RunID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	cancelResult := readRunResult(t, resultCh, errorCh)
	if cancelResult.Run.Status != RunStatusCanceled ||
		cancelResult.Run.ErrorCode.String != "canceled" ||
		manager.Active(cancelTask.RunID) {
		t.Fatalf("canceled run = %+v active=%v", cancelResult.Run, manager.Active(cancelTask.RunID))
	}

	nextTask := runnerTask(env, "agent_run_after_cancel")
	nextResult, err := manager.Execute(context.Background(), RunParams{
		WorkspaceID: env.WorkspaceID,
		Config:      runnerConfig(writeFakeAgent(t, "#!/bin/sh\nprintf 'still running session\\n'\n")),
		Task:        nextTask,
	})
	if err != nil {
		t.Fatalf("Execute(next) error = %v", err)
	}
	if nextResult.Run.Status != RunStatusSucceeded {
		t.Fatalf("next run = %+v, want succeeded", nextResult.Run)
	}
}

func TestManagerRejectsDuplicateAndUnknownCancellation(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	manager := &Manager{Runner: runnerWithClock(env)}
	task := runnerTask(env, "agent_run_duplicate")
	resultCh := make(chan RunResult, 1)
	errorCh := make(chan error, 1)
	go func() {
		result, err := manager.Execute(context.Background(), RunParams{
			WorkspaceID: env.WorkspaceID,
			Config:      runnerConfig(writeFakeAgent(t, "#!/bin/sh\nexec /bin/sleep 2\n")),
			Task:        task,
		})
		resultCh <- result
		errorCh <- err
	}()
	waitForActiveRun(t, manager, task.RunID)

	_, duplicateErr := manager.Execute(context.Background(), RunParams{
		WorkspaceID: env.WorkspaceID,
		Config:      runnerConfig(writeFakeAgent(t, "#!/bin/sh\nexit 0\n")),
		Task:        task,
	})
	if !errors.Is(duplicateErr, ErrRunAlreadyActive) {
		t.Fatalf("duplicate Execute() error = %v, want ErrRunAlreadyActive", duplicateErr)
	}
	if err := manager.Cancel(context.Background(), "missing_run"); !errors.Is(err, ErrRunNotActive) {
		t.Fatalf("Cancel(missing) error = %v, want ErrRunNotActive", err)
	}

	if err := manager.Cancel(context.Background(), task.RunID); err != nil {
		t.Fatalf("Cancel(active) error = %v", err)
	}
	_ = readRunResult(t, resultCh, errorCh)
}

func TestManagerEnforcesGlobalConcurrencyLimit(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	driver := newBlockingDriver()
	manager := &Manager{
		Runner:        Runner{Queries: env.Queries, Artifacts: env.Artifacts, Driver: driver},
		MaxConcurrent: 1,
	}

	first := asyncManagerExecute(manager, env, runnerTask(env, "agent_run_global_first"))
	firstStarted := driver.waitStarted(t)
	if firstStarted != "agent_run_global_first" {
		t.Fatalf("first started = %s", firstStarted)
	}
	second := asyncManagerExecute(manager, env, runnerTask(env, "agent_run_global_second"))
	driver.assertNoStart(t, 50*time.Millisecond)

	driver.releaseRun(firstStarted)
	if result := first.read(t); result.Run.Status != RunStatusSucceeded {
		t.Fatalf("first result = %+v", result.Run)
	}
	secondStarted := driver.waitStarted(t)
	if secondStarted != "agent_run_global_second" {
		t.Fatalf("second started = %s", secondStarted)
	}
	driver.releaseRun(secondStarted)
	if result := second.read(t); result.Run.Status != RunStatusSucceeded {
		t.Fatalf("second result = %+v", result.Run)
	}
	if driver.maxConcurrent() != 1 {
		t.Fatalf("max driver concurrency = %d, want 1", driver.maxConcurrent())
	}
}

func TestManagerEnforcesPerSessionLimitWithoutBlockingOtherSessions(t *testing.T) {
	t.Parallel()

	env := setupOutputRecorder(t)
	driver := newBlockingDriver()
	manager := &Manager{
		Runner:                  Runner{Queries: env.Queries, Artifacts: env.Artifacts, Driver: driver},
		MaxConcurrentPerSession: 1,
	}

	firstTask := runnerTask(env, "agent_run_session_first")
	firstTask.ReviewSessionID = "review_session_1"
	first := asyncManagerExecute(manager, env, firstTask)
	if started := driver.waitStarted(t); started != firstTask.RunID {
		t.Fatalf("first started = %s", started)
	}

	secondTask := runnerTask(env, "agent_run_session_second")
	secondTask.ReviewSessionID = "review_session_1"
	second := asyncManagerExecute(manager, env, secondTask)
	driver.assertNoStart(t, 50*time.Millisecond)

	otherSessionTask := runnerTask(env, "agent_run_session_other")
	otherSessionTask.ReviewSessionID = "review_session_other"
	createReviewSessionForAgentRunTest(t, env, otherSessionTask.ReviewSessionID)
	other := asyncManagerExecute(manager, env, otherSessionTask)
	if started := driver.waitStarted(t); started != otherSessionTask.RunID {
		t.Fatalf("other session started = %s", started)
	}

	driver.releaseRun(firstTask.RunID)
	if result := first.read(t); result.Run.Status != RunStatusSucceeded {
		t.Fatalf("first result = %+v", result.Run)
	}
	if started := driver.waitStarted(t); started != secondTask.RunID {
		t.Fatalf("second started = %s", started)
	}
	driver.releaseRun(otherSessionTask.RunID)
	if result := other.read(t); result.Run.Status != RunStatusSucceeded {
		t.Fatalf("other result = %+v", result.Run)
	}
	driver.releaseRun(secondTask.RunID)
	if result := second.read(t); result.Run.Status != RunStatusSucceeded {
		t.Fatalf("second result = %+v", result.Run)
	}
	if driver.maxConcurrent() != 2 {
		t.Fatalf("max driver concurrency = %d, want 2", driver.maxConcurrent())
	}
}

func waitForActiveRun(t *testing.T, manager *Manager, runID string) {
	t.Helper()

	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for active run %s", runID)
		case <-tick.C:
			if manager.Active(runID) {
				return
			}
		}
	}
}

func readRunResult(t *testing.T, resultCh <-chan RunResult, errorCh <-chan error) RunResult {
	t.Helper()

	timeout := time.After(5 * time.Second)
	select {
	case err := <-errorCh:
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	case <-timeout:
		t.Fatal("timed out waiting for run error")
	}
	select {
	case result := <-resultCh:
		return result
	case <-timeout:
		t.Fatal("timed out waiting for run result")
		return RunResult{}
	}
}

type asyncRun struct {
	resultCh <-chan RunResult
	errorCh  <-chan error
}

func asyncManagerExecute(manager *Manager, env outputRecorderEnv, task agents.AgentTask) asyncRun {
	resultCh := make(chan RunResult, 1)
	errorCh := make(chan error, 1)
	go func() {
		result, err := manager.Execute(context.Background(), RunParams{
			WorkspaceID: env.WorkspaceID,
			Config:      runnerConfig("fake-agent"),
			Task:        task,
		})
		resultCh <- result
		errorCh <- err
	}()
	return asyncRun{resultCh: resultCh, errorCh: errorCh}
}

func (r asyncRun) read(t *testing.T) RunResult {
	t.Helper()
	return readRunResult(t, r.resultCh, r.errorCh)
}

func createReviewSessionForAgentRunTest(t *testing.T, env outputRecorderEnv, id string) {
	t.Helper()

	if _, err := env.Queries.CreateReviewSession(context.Background(), dbgen.CreateReviewSessionParams{
		ID:                  id,
		WorkspaceID:         env.WorkspaceID,
		RepositoryID:        "repo_1",
		SnapshotID:          "snapshot_1",
		Title:               "Other session",
		Status:              "running",
		ReviewDepth:         "standard",
		RuntimeLimitSeconds: 1800,
		ContextPolicyJson:   "{}",
		CreatedAt:           "2026-05-03T00:00:00Z",
		UpdatedAt:           "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateReviewSession(%s) error = %v", id, err)
	}
}

type blockingDriver struct {
	started chan string

	mu       sync.Mutex
	current  int
	max      int
	releases map[string]chan struct{}
}

func newBlockingDriver() *blockingDriver {
	return &blockingDriver{
		started:  make(chan string, 8),
		releases: map[string]chan struct{}{},
	}
}

func (d *blockingDriver) Open(context.Context, agents.ConnectionConfig) (agents.Connection, error) {
	return blockingConnection{driver: d}, nil
}

func (d *blockingDriver) waitStarted(t *testing.T) string {
	t.Helper()

	select {
	case runID := <-d.started:
		return runID
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for driver start")
		return ""
	}
}

func (d *blockingDriver) assertNoStart(t *testing.T, duration time.Duration) {
	t.Helper()

	select {
	case runID := <-d.started:
		t.Fatalf("unexpected run start %s", runID)
	case <-time.After(duration):
	}
}

func (d *blockingDriver) releaseRun(runID string) {
	d.mu.Lock()
	release := d.releases[runID]
	d.mu.Unlock()
	if release != nil {
		close(release)
	}
}

func (d *blockingDriver) maxConcurrent() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.max
}

func (d *blockingDriver) start(runID string) <-chan struct{} {
	d.mu.Lock()
	d.current++
	if d.current > d.max {
		d.max = d.current
	}
	release := make(chan struct{})
	d.releases[runID] = release
	d.mu.Unlock()
	d.started <- runID
	return release
}

func (d *blockingDriver) finish(runID string) {
	d.mu.Lock()
	d.current--
	delete(d.releases, runID)
	d.mu.Unlock()
}

type blockingConnection struct {
	driver *blockingDriver
}

func (c blockingConnection) SendTask(ctx context.Context, task agents.AgentTask) (<-chan agents.AgentEvent, error) {
	events := make(chan agents.AgentEvent, 2)
	go func() {
		defer close(events)
		release := c.driver.start(task.RunID)
		defer c.driver.finish(task.RunID)
		select {
		case <-release:
			exitCode := 0
			events <- agents.AgentEvent{Type: agents.EventCompleted, RunID: task.RunID, ExitCode: &exitCode}
		case <-ctx.Done():
			events <- agents.AgentEvent{Type: agents.EventCanceled, RunID: task.RunID, ErrorCode: "canceled", Error: ctx.Err().Error()}
		}
	}()
	return events, nil
}

func (blockingConnection) Close(context.Context) error {
	return nil
}
