package agents

import (
	"context"
	"errors"
	"testing"
)

func TestACPAdapterIsDisabled(t *testing.T) {
	t.Parallel()

	adapter := ACPAdapter{AdapterID: "agent_acp"}
	if adapter.ID() != "agent_acp" || adapter.Kind() != AdapterACPStdio {
		t.Fatalf("adapter identity = %s/%s", adapter.ID(), adapter.Kind())
	}
	health, err := adapter.HealthCheck(context.Background())
	if !errors.Is(err, ErrACPAdapterDisabled) ||
		health.Status != HealthUnknown ||
		health.Metadata["future_adapter"] != true ||
		health.Metadata["transport"] != string(AdapterACPStdio) {
		t.Fatalf("HealthCheck() = %+v, %v", health, err)
	}
	capabilities, err := adapter.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	if !capabilities.SupportsSessions ||
		!capabilities.SupportsStreaming ||
		capabilities.Metadata["protocol"] != "acp" {
		t.Fatalf("capabilities = %+v", capabilities)
	}
	if _, err := adapter.RunTask(context.Background(), baseCommandTask()); !errors.Is(err, ErrACPAdapterDisabled) {
		t.Fatalf("RunTask() error = %v, want disabled", err)
	}
	if err := adapter.Cancel(context.Background(), "run_1"); !errors.Is(err, ErrACPAdapterDisabled) {
		t.Fatalf("Cancel() error = %v, want disabled", err)
	}
}

func TestACPAdapterEnabledStillReportsUnimplementedRuntime(t *testing.T) {
	t.Parallel()

	adapter := ACPAdapter{Enabled: true}
	if _, err := adapter.RunTask(context.Background(), baseCommandTask()); !errors.Is(err, ErrACPAdapterNotImplemented) {
		t.Fatalf("RunTask(enabled) error = %v, want not implemented", err)
	}
	if err := adapter.Cancel(context.Background(), "run_1"); !errors.Is(err, ErrACPAdapterNotImplemented) {
		t.Fatalf("Cancel(enabled) error = %v, want not implemented", err)
	}
}

func TestMapACPEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inputType string
		errorCode string
		wantType  EventType
		wantCode  string
	}{
		{name: "started", inputType: "run.started", wantType: EventStarted},
		{name: "output", inputType: "content.delta", wantType: EventOutput},
		{name: "completed", inputType: "run.completed", wantType: EventCompleted},
		{name: "failed default code", inputType: "run.failed", wantType: EventFailed, wantCode: "acp_failed"},
		{name: "failed explicit code", inputType: "task.failed", errorCode: "tool_error", wantType: EventFailed, wantCode: "tool_error"},
		{name: "canceled", inputType: "task.canceled", wantType: EventCanceled, wantCode: "canceled"},
		{name: "progress fallback", inputType: "unknown.notification", wantType: EventProgress},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := MapACPEvent(ACPEvent{
				Type:      tt.inputType,
				RunID:     "run_1",
				AdapterID: "agent_acp",
				SessionID: "session_1",
				Message:   "message",
				Text:      "delta",
				Payload:   []byte(`{"ok":true}`),
				Error:     "error",
				ErrorCode: tt.errorCode,
			})
			if got.Type != tt.wantType ||
				got.RunID != "run_1" ||
				got.Message != "message" ||
				got.Text != "delta" ||
				got.ErrorCode != tt.wantCode ||
				string(got.Payload) != `{"ok":true}` ||
				got.Metadata["acp_session_id"] != "session_1" ||
				got.Metadata[ExternalSessionMetadataKey] == nil {
				t.Fatalf("mapped event = %+v", got)
			}
			session, ok := ExtractExternalSessionMetadata("", got.Metadata)
			if !ok ||
				session.AdapterID != "agent_acp" ||
				session.Protocol != "acp" ||
				session.SessionID != "session_1" {
				t.Fatalf("external session = %+v, ok = %v", session, ok)
			}
		})
	}
}
