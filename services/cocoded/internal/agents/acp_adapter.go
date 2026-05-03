package agents

import (
	"context"
	"errors"
	"time"
)

var (
	ErrACPAdapterDisabled       = errors.New("acp adapter is disabled for the MVP")
	ErrACPAdapterNotImplemented = errors.New("acp adapter runtime is not implemented")
)

type ACPAdapter struct {
	AdapterID string
	Enabled   bool
	Driver    ConnectionDriver
}

type ACPEvent struct {
	Type      string
	RunID     string
	SessionID string
	Message   string
	Text      string
	Payload   []byte
	Error     string
	ErrorCode string
}

func (a ACPAdapter) ID() string {
	if a.AdapterID != "" {
		return a.AdapterID
	}
	return "acp-stdio"
}

func (a ACPAdapter) Kind() AdapterKind {
	return AdapterACPStdio
}

func (a ACPAdapter) HealthCheck(context.Context) (AgentHealth, error) {
	return AgentHealth{
		Status:    HealthUnknown,
		Message:   ErrACPAdapterDisabled.Error(),
		CheckedAt: time.Now().UTC(),
		Metadata: map[string]any{
			"future_adapter": true,
			"transport":      string(AdapterACPStdio),
		},
	}, ErrACPAdapterDisabled
}

func (a ACPAdapter) Capabilities(context.Context) (AgentCapabilities, error) {
	capabilities := DefaultCapabilities(AdapterACPStdio)
	capabilities.Metadata = map[string]any{
		"future_adapter": true,
		"protocol":       "acp",
	}
	return capabilities, nil
}

func (a ACPAdapter) RunTask(context.Context, AgentTask) (<-chan AgentEvent, error) {
	if !a.Enabled {
		return nil, ErrACPAdapterDisabled
	}
	return nil, ErrACPAdapterNotImplemented
}

func (a ACPAdapter) Cancel(context.Context, string) error {
	if !a.Enabled {
		return ErrACPAdapterDisabled
	}
	return ErrACPAdapterNotImplemented
}

func MapACPEvent(event ACPEvent) AgentEvent {
	mapped := AgentEvent{
		Type:    EventProgress,
		RunID:   event.RunID,
		At:      time.Now().UTC(),
		Message: event.Message,
		Text:    event.Text,
		Error:   event.Error,
	}
	if len(event.Payload) > 0 {
		mapped.Payload = append([]byte(nil), event.Payload...)
	}
	if event.SessionID != "" {
		mapped.Metadata = map[string]any{"acp_session_id": event.SessionID}
	}

	switch event.Type {
	case "session.started", "run.started", "task.started":
		mapped.Type = EventStarted
	case "content.delta", "message.delta", "output.delta":
		mapped.Type = EventOutput
	case "run.completed", "task.completed":
		mapped.Type = EventCompleted
	case "run.failed", "task.failed":
		mapped.Type = EventFailed
		mapped.ErrorCode = event.ErrorCode
		if mapped.ErrorCode == "" {
			mapped.ErrorCode = "acp_failed"
		}
	case "run.canceled", "task.canceled":
		mapped.Type = EventCanceled
		mapped.ErrorCode = event.ErrorCode
		if mapped.ErrorCode == "" {
			mapped.ErrorCode = "canceled"
		}
	}
	return mapped
}

var _ AgentAdapter = ACPAdapter{}
