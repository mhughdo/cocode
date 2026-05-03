package agentrun

import (
	"errors"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
)

type TimeoutPolicy struct {
	AgentTimeoutSeconds  int64
	AgentTimeout         time.Duration
	ReviewTimeoutSeconds int64
	ReviewTimeout        time.Duration
	ReviewDeadline       time.Time
}

type timeoutCandidate struct {
	source  string
	timeout time.Duration
	expired bool
}

func (p TimeoutPolicy) Apply(now time.Time, task agents.AgentTask) (agents.AgentTask, map[string]any, *runOutcome, error) {
	if err := p.validate(task); err != nil {
		return task, nil, nil, err
	}

	candidates := p.candidates(now, task)
	if len(candidates) == 0 {
		return task, nil, nil, nil
	}

	selected := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.expired || (!selected.expired && candidate.timeout < selected.timeout) {
			selected = candidate
		}
	}

	metadata := map[string]any{
		"effective_timeout_source": selected.source,
	}
	if selected.expired {
		metadata["effective_timeout_ms"] = int64(0)
		outcome := runOutcome{
			status:       RunStatusTimedOut,
			errorCode:    nullableRunString("timeout"),
			errorMessage: nullableRunString("review runtime limit exceeded before agent start"),
		}
		return task, metadata, &outcome, nil
	}

	task.Limits.Timeout = selected.timeout
	task.Limits.TimeoutSeconds = durationSecondsCeil(selected.timeout)
	metadata["effective_timeout_ms"] = selected.timeout.Milliseconds()
	metadata["effective_timeout_seconds"] = task.Limits.TimeoutSeconds
	return task, metadata, nil, nil
}

func (p TimeoutPolicy) validate(task agents.AgentTask) error {
	if p.AgentTimeoutSeconds < 0 ||
		p.AgentTimeout < 0 ||
		p.ReviewTimeoutSeconds < 0 ||
		p.ReviewTimeout < 0 ||
		task.Limits.Timeout < 0 ||
		task.Limits.TimeoutSeconds < 0 {
		return errors.New("timeout limits cannot be negative")
	}
	return nil
}

func (p TimeoutPolicy) candidates(now time.Time, task agents.AgentTask) []timeoutCandidate {
	candidates := make([]timeoutCandidate, 0, 5)
	if task.Limits.Timeout > 0 {
		candidates = append(candidates, timeoutCandidate{source: "task", timeout: task.Limits.Timeout})
	}
	if task.Limits.TimeoutSeconds > 0 {
		candidates = append(candidates, timeoutCandidate{source: "task_seconds", timeout: time.Duration(task.Limits.TimeoutSeconds) * time.Second})
	}
	if p.AgentTimeout > 0 {
		candidates = append(candidates, timeoutCandidate{source: "agent", timeout: p.AgentTimeout})
	}
	if p.AgentTimeoutSeconds > 0 {
		candidates = append(candidates, timeoutCandidate{source: "agent_seconds", timeout: time.Duration(p.AgentTimeoutSeconds) * time.Second})
	}
	if p.ReviewTimeout > 0 {
		candidates = append(candidates, timeoutCandidate{source: "review", timeout: p.ReviewTimeout})
	}
	if p.ReviewTimeoutSeconds > 0 {
		candidates = append(candidates, timeoutCandidate{source: "review_seconds", timeout: time.Duration(p.ReviewTimeoutSeconds) * time.Second})
	}
	if !p.ReviewDeadline.IsZero() {
		remaining := p.ReviewDeadline.Sub(now)
		if remaining <= 0 {
			candidates = append(candidates, timeoutCandidate{source: "review_deadline", expired: true})
		} else {
			candidates = append(candidates, timeoutCandidate{source: "review_deadline", timeout: remaining})
		}
	}
	return candidates
}

func durationSecondsCeil(value time.Duration) int64 {
	if value <= 0 {
		return 0
	}
	seconds := value / time.Second
	if value%time.Second != 0 {
		seconds++
	}
	return int64(seconds)
}
