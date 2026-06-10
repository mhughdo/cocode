package agents

import (
	"context"
	"errors"
	"time"
)

var ErrCodexAppServerDisabled = errors.New("codex app server adapter is disabled for the MVP")

type CodexAppServerAdapter struct {
	AdapterID string
	Enabled   bool
	Driver    ConnectionDriver
}

type CodexAppServerEvent struct {
	Type      string
	RunID     string
	ThreadID  string
	TurnID    string
	Message   string
	Payload   []byte
	Error     string
	AdapterID string
}

func (a CodexAppServerAdapter) ID() string {
	if a.AdapterID != "" {
		return a.AdapterID
	}
	return "codex-app-server"
}

func (a CodexAppServerAdapter) Kind() AdapterKind {
	return AdapterJSONRPCStdio
}

func (a CodexAppServerAdapter) HealthCheck(context.Context) (AgentHealth, error) {
	return AgentHealth{
		Status:    HealthUnknown,
		Message:   ErrCodexAppServerDisabled.Error(),
		CheckedAt: time.Now().UTC(),
		Metadata: map[string]any{
			"future_adapter": true,
			"transport":      string(AdapterJSONRPCStdio),
		},
	}, ErrCodexAppServerDisabled
}

func (a CodexAppServerAdapter) Capabilities(context.Context) (AgentCapabilities, error) {
	capabilities := DefaultCapabilities(AdapterJSONRPCStdio)
	capabilities.Metadata = map[string]any{
		"future_adapter": true,
		"protocol":       "codex_app_server",
	}
	return capabilities, nil
}

func (a CodexAppServerAdapter) RunTask(context.Context, AgentTask) (<-chan AgentEvent, error) {
	if !a.Enabled {
		return nil, ErrCodexAppServerDisabled
	}
	return nil, ErrCodexAppServerDisabled
}

func (a CodexAppServerAdapter) Cancel(context.Context, string) error {
	return ErrCodexAppServerDisabled
}

func MapCodexAppServerEvent(event CodexAppServerEvent) AgentEvent {
	mapped := AgentEvent{
		Type:    EventProgress,
		RunID:   event.RunID,
		At:      time.Now().UTC(),
		Message: event.Message,
		Error:   event.Error,
	}
	if len(event.Payload) > 0 {
		mapped.Payload = append([]byte(nil), event.Payload...)
	}
	mapped.Metadata = ExternalSessionEventMetadata(ExternalSessionMetadata{
		AdapterID: event.AdapterID,
		Protocol:  "codex_app_server",
		ThreadID:  event.ThreadID,
		TurnID:    event.TurnID,
		Source:    event.Type,
	}, nil)
	switch event.Type {
	case "session.started", "turn.started":
		mapped.Type = EventStarted
	case "turn.completed":
		mapped.Type = EventCompleted
	case "turn.failed":
		mapped.Type = EventFailed
		mapped.ErrorCode = "app_server_failed"
	case "turn.canceled":
		mapped.Type = EventCanceled
		mapped.ErrorCode = "canceled"
	}
	return mapped
}

var _ AgentAdapter = CodexAppServerAdapter{}
