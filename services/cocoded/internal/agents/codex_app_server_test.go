package agents

import (
	"context"
	"errors"
	"testing"
)

func TestCodexAppServerAdapterIsDisabled(t *testing.T) {
	t.Parallel()

	adapter := CodexAppServerAdapter{AdapterID: "agent_codex_app_server"}
	if adapter.ID() != "agent_codex_app_server" || adapter.Kind() != AdapterJSONRPCStdio {
		t.Fatalf("adapter identity = %s/%s", adapter.ID(), adapter.Kind())
	}
	health, err := adapter.HealthCheck(context.Background())
	if !errors.Is(err, ErrCodexAppServerDisabled) ||
		health.Status != HealthUnknown ||
		health.Metadata["future_adapter"] != true {
		t.Fatalf("HealthCheck() = %+v, %v", health, err)
	}
	capabilities, err := adapter.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	if !capabilities.SupportsSessions ||
		!capabilities.SupportsStreaming ||
		capabilities.Metadata["protocol"] != "codex_app_server" {
		t.Fatalf("capabilities = %+v", capabilities)
	}
	if _, err := adapter.RunTask(context.Background(), baseCommandTask()); !errors.Is(err, ErrCodexAppServerDisabled) {
		t.Fatalf("RunTask() error = %v, want disabled", err)
	}
	if err := adapter.Cancel(context.Background(), "run_1"); !errors.Is(err, ErrCodexAppServerDisabled) {
		t.Fatalf("Cancel() error = %v, want disabled", err)
	}
}

func TestMapCodexAppServerEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inputType string
		wantType  EventType
		wantCode  string
	}{
		{name: "started", inputType: "turn.started", wantType: EventStarted},
		{name: "completed", inputType: "turn.completed", wantType: EventCompleted},
		{name: "failed", inputType: "turn.failed", wantType: EventFailed, wantCode: "app_server_failed"},
		{name: "canceled", inputType: "turn.canceled", wantType: EventCanceled, wantCode: "canceled"},
		{name: "progress fallback", inputType: "unknown.notification", wantType: EventProgress},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := MapCodexAppServerEvent(CodexAppServerEvent{
				Type:    tt.inputType,
				RunID:   "run_1",
				Message: "message",
				Payload: []byte(`{"ok":true}`),
				Error:   "error",
			})
			if got.Type != tt.wantType ||
				got.RunID != "run_1" ||
				got.Message != "message" ||
				got.ErrorCode != tt.wantCode ||
				string(got.Payload) != `{"ok":true}` {
				t.Fatalf("mapped event = %+v", got)
			}
		})
	}
}
