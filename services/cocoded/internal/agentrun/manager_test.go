package agentrun

import (
	"context"
	"errors"
	"testing"
	"time"
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
