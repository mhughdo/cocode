package agentrun

import (
	"context"
	"errors"
	"strings"
	"sync"
)

var (
	ErrRunAlreadyActive = errors.New("agent run is already active")
	ErrRunNotActive     = errors.New("agent run is not active")
)

type Manager struct {
	Runner Runner

	mu     sync.Mutex
	active map[string]context.CancelFunc
}

func (m *Manager) Execute(ctx context.Context, params RunParams) (RunResult, error) {
	if m == nil {
		return RunResult{}, errors.New("agent run manager is required")
	}
	ctx = contextOrBackground(ctx)
	if strings.TrimSpace(params.Task.RunID) == "" {
		params.Task.RunID = m.Runner.newRunID()
	}
	if strings.TrimSpace(params.Task.ID) == "" {
		params.Task.ID = params.Task.RunID
	}
	runID := strings.TrimSpace(params.Task.RunID)
	if runID == "" {
		return RunResult{}, errors.New("agent run id is required")
	}

	runCtx, cancel := context.WithCancel(ctx)
	if err := m.register(runID, cancel); err != nil {
		cancel()
		return RunResult{}, err
	}
	defer m.unregister(runID)
	return m.Runner.Execute(runCtx, params)
}

func (m *Manager) Cancel(ctx context.Context, runID string) error {
	if m == nil {
		return errors.New("agent run manager is required")
	}
	ctx = contextOrBackground(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return errors.New("agent run id is required")
	}

	m.mu.Lock()
	cancel := m.active[runID]
	m.mu.Unlock()
	if cancel == nil {
		return ErrRunNotActive
	}
	cancel()
	return nil
}

func (m *Manager) Active(runID string) bool {
	if m == nil {
		return false
	}
	runID = strings.TrimSpace(runID)
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active[runID] != nil
}

func (m *Manager) register(runID string, cancel context.CancelFunc) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		m.active = map[string]context.CancelFunc{}
	}
	if m.active[runID] != nil {
		return ErrRunAlreadyActive
	}
	m.active[runID] = cancel
	return nil
}

func (m *Manager) unregister(runID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.active, runID)
}
