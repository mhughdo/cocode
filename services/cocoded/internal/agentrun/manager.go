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
	Runner                  Runner
	MaxConcurrent           int
	MaxConcurrentPerSession int

	mu           sync.Mutex
	active       map[string]context.CancelFunc
	globalSlots  chan struct{}
	sessionSlots map[string]chan struct{}
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
	release, err := m.acquire(runCtx, params.Task.ReviewSessionID)
	if err != nil {
		cancel()
		return RunResult{}, err
	}
	defer release()
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

func (m *Manager) acquire(ctx context.Context, reviewSessionID string) (func(), error) {
	releases := make([]func(), 0, 2)
	if global := m.globalLimiter(); global != nil {
		if err := acquireSlot(ctx, global); err != nil {
			return func() {}, err
		}
		releases = append(releases, func() { <-global })
	}
	if session := m.sessionLimiter(reviewSessionID); session != nil {
		if err := acquireSlot(ctx, session); err != nil {
			releaseAll(releases)
			return func() {}, err
		}
		releases = append(releases, func() { <-session })
	}
	return func() {
		releaseAll(releases)
	}, nil
}

func (m *Manager) globalLimiter() chan struct{} {
	if m.MaxConcurrent <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.globalSlots == nil {
		m.globalSlots = make(chan struct{}, m.MaxConcurrent)
	}
	return m.globalSlots
}

func (m *Manager) sessionLimiter(reviewSessionID string) chan struct{} {
	if m.MaxConcurrentPerSession <= 0 {
		return nil
	}
	reviewSessionID = strings.TrimSpace(reviewSessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessionSlots == nil {
		m.sessionSlots = map[string]chan struct{}{}
	}
	if m.sessionSlots[reviewSessionID] == nil {
		m.sessionSlots[reviewSessionID] = make(chan struct{}, m.MaxConcurrentPerSession)
	}
	return m.sessionSlots[reviewSessionID]
}

func acquireSlot(ctx context.Context, slots chan struct{}) error {
	select {
	case slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseAll(releases []func()) {
	for index := len(releases) - 1; index >= 0; index-- {
		releases[index]()
	}
}
