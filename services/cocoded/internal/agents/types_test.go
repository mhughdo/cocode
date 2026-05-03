package agents

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAdapterInterfacesSupportTaskLifecycle(t *testing.T) {
	t.Parallel()

	task := AgentTask{
		ID:              "task_1",
		RunID:           "run_1",
		ReviewSessionID: "session_1",
		AgentConfigID:   "agent_1",
		Role:            "primary_reviewer",
		ContextArtifacts: []ArtifactRef{
			{
				ID:           "artifact_context",
				Kind:         "context_bundle",
				RelativePath: "snapshots/context.md",
				ContentType:  "text/markdown",
				SizeBytes:    1 << 20,
				SHA256:       "sha",
			},
		},
		Limits: TaskLimits{
			Timeout:        30 * time.Second,
			TimeoutSeconds: 30,
			MaxStdoutBytes: 2 << 20,
			MaxStderrBytes: 1 << 20,
			MaxPromptBytes: 512 << 10,
		},
	}
	if err := task.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	adapter := fakeAdapter{id: "agent_1", kind: AdapterCLINonInteractive}
	if adapter.ID() != "agent_1" || adapter.Kind() != AdapterCLINonInteractive {
		t.Fatalf("adapter identity = %s/%s", adapter.ID(), adapter.Kind())
	}
	health, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if health.Status != HealthAvailable || health.CheckedAt.IsZero() {
		t.Fatalf("health = %+v", health)
	}
	capabilities, err := adapter.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	if !capabilities.SupportsJSON || !capabilities.CanRead || capabilities.CanWrite {
		t.Fatalf("capabilities = %+v", capabilities)
	}

	events, err := adapter.RunTask(context.Background(), task)
	if err != nil {
		t.Fatalf("RunTask() error = %v", err)
	}
	var got []AgentEvent
	for event := range events {
		got = append(got, event)
	}
	if len(got) != 2 || got[0].Type != EventStarted || got[1].Type != EventCompleted || !got[1].Type.Terminal() {
		t.Fatalf("events = %+v", got)
	}
	if got[1].RunID != task.RunID {
		t.Fatalf("completed run id = %q, want %q", got[1].RunID, task.RunID)
	}

	if err := adapter.Cancel(context.Background(), task.RunID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
}

func TestConnectionConfigValidate(t *testing.T) {
	t.Parallel()

	valid := ConnectionConfig{
		AdapterID:        "agent_1",
		Kind:             AdapterCLINonInteractive,
		Command:          "codex",
		Args:             []string{"exec", "--json"},
		PromptDelivery:   PromptViaStdin,
		WorkingDirectory: "/repo",
		Env:              map[string]string{"HOME": "/tmp/cocode-home"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate(valid) error = %v", err)
	}
	driver := fakeDriver{}
	connection, err := driver.Open(context.Background(), valid)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	events, err := connection.SendTask(context.Background(), AgentTask{
		ID:              "task_1",
		RunID:           "run_1",
		ReviewSessionID: "session_1",
		AgentConfigID:   "agent_1",
		Role:            "reviewer",
		Prompt:          "Review this change.",
	})
	if err != nil {
		t.Fatalf("SendTask() error = %v", err)
	}
	if event := <-events; event.Type != EventCompleted || event.RunID != "run_1" {
		t.Fatalf("event = %+v", event)
	}
	if err := connection.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	tests := []struct {
		name   string
		config ConnectionConfig
	}{
		{name: "missing adapter", config: ConnectionConfig{Kind: AdapterCLINonInteractive}},
		{name: "invalid kind", config: ConnectionConfig{AdapterID: "agent_1", Kind: AdapterKind("shell")}},
		{name: "invalid prompt delivery", config: ConnectionConfig{AdapterID: "agent_1", Kind: AdapterCLINonInteractive, PromptDelivery: PromptDelivery("pipe")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.config.Validate(); err == nil {
				t.Fatalf("Validate(%+v) error = nil, want error", tt.config)
			}
		})
	}
}

func TestAgentTaskValidate(t *testing.T) {
	t.Parallel()

	valid := AgentTask{
		ID:              "task_1",
		RunID:           "run_1",
		ReviewSessionID: "session_1",
		AgentConfigID:   "agent_1",
		Role:            "verifier",
		Prompt:          "Review this change.",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate(valid) error = %v", err)
	}

	tests := []struct {
		name string
		task AgentTask
	}{
		{name: "missing id", task: AgentTask{RunID: "run_1", ReviewSessionID: "session_1", AgentConfigID: "agent_1", Role: "reviewer", Prompt: "prompt"}},
		{name: "missing run", task: AgentTask{ID: "task_1", ReviewSessionID: "session_1", AgentConfigID: "agent_1", Role: "reviewer", Prompt: "prompt"}},
		{name: "missing session", task: AgentTask{ID: "task_1", RunID: "run_1", AgentConfigID: "agent_1", Role: "reviewer", Prompt: "prompt"}},
		{name: "missing config", task: AgentTask{ID: "task_1", RunID: "run_1", ReviewSessionID: "session_1", Role: "reviewer", Prompt: "prompt"}},
		{name: "missing role", task: AgentTask{ID: "task_1", RunID: "run_1", ReviewSessionID: "session_1", AgentConfigID: "agent_1", Prompt: "prompt"}},
		{name: "missing input", task: AgentTask{ID: "task_1", RunID: "run_1", ReviewSessionID: "session_1", AgentConfigID: "agent_1", Role: "reviewer"}},
		{name: "negative timeout", task: AgentTask{ID: "task_1", RunID: "run_1", ReviewSessionID: "session_1", AgentConfigID: "agent_1", Role: "reviewer", Prompt: "prompt", Limits: TaskLimits{TimeoutSeconds: -1}}},
		{name: "negative byte limit", task: AgentTask{ID: "task_1", RunID: "run_1", ReviewSessionID: "session_1", AgentConfigID: "agent_1", Role: "reviewer", Prompt: "prompt", Limits: TaskLimits{MaxStdoutBytes: -1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.task.Validate(); err == nil {
				t.Fatalf("Validate(%+v) error = nil, want error", tt.task)
			}
		})
	}
}

func TestDecodeCapabilitiesJSONAppliesDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	defaults, err := DecodeCapabilitiesJSON("{}", AdapterCLINonInteractive)
	if err != nil {
		t.Fatalf("DecodeCapabilitiesJSON(defaults) error = %v", err)
	}
	if !defaults.SupportsJSON ||
		defaults.SupportsStreaming ||
		defaults.SupportsSessions ||
		!defaults.CanRead ||
		defaults.CanWrite ||
		!defaults.CanCancel ||
		len(defaults.OutputModes) != 4 {
		t.Fatalf("default CLI capabilities = %+v", defaults)
	}

	capabilities, err := DecodeCapabilitiesJSON(`{
		"supports_json": false,
		"supports_streaming": true,
		"supports_sessions": true,
		"can_read": false,
		"can_write": true,
		"can_cancel": false,
		"output_modes": ["text"],
		"metadata": {"provider": "custom"}
	}`, AdapterCLINonInteractive)
	if err != nil {
		t.Fatalf("DecodeCapabilitiesJSON(overrides) error = %v", err)
	}
	if capabilities.SupportsJSON ||
		!capabilities.SupportsStreaming ||
		!capabilities.SupportsSessions ||
		capabilities.CanRead ||
		!capabilities.CanWrite ||
		capabilities.CanCancel ||
		len(capabilities.OutputModes) != 1 ||
		capabilities.OutputModes[0] != OutputText ||
		capabilities.Metadata["provider"] != "custom" {
		t.Fatalf("overridden capabilities = %+v", capabilities)
	}

	textOnly, err := DecodeCapabilitiesJSON(`{"supports_json":false}`, AdapterCLINonInteractive)
	if err != nil {
		t.Fatalf("DecodeCapabilitiesJSON(text only) error = %v", err)
	}
	if textOnly.SupportsJSON || len(textOnly.OutputModes) != 1 || textOnly.OutputModes[0] != OutputText {
		t.Fatalf("text-only capabilities = %+v", textOnly)
	}
}

func TestCapabilitiesNormalizeAndEncode(t *testing.T) {
	t.Parallel()

	capabilities := AgentCapabilities{
		OutputModes: []OutputMode{OutputJSON, OutputText, OutputJSON},
	}
	normalized := capabilities.Normalize()
	if !normalized.SupportsJSON {
		t.Fatalf("SupportsJSON = false for structured output: %+v", normalized)
	}
	if len(normalized.OutputModes) != 2 || normalized.OutputModes[0] != OutputJSON || normalized.OutputModes[1] != OutputText {
		t.Fatalf("OutputModes = %+v, want unique modes in input order", normalized.OutputModes)
	}
	if normalized.Metadata == nil {
		t.Fatal("Metadata = nil, want empty map")
	}
	if !normalized.SupportsOutputMode(OutputJSON) || normalized.SupportsOutputMode(OutputNDJSON) || normalized.SupportsOutputMode(OutputMode("yaml")) {
		t.Fatalf("SupportsOutputMode checks failed for %+v", normalized.OutputModes)
	}

	encoded, err := capabilities.EncodeJSON()
	if err != nil {
		t.Fatalf("EncodeJSON() error = %v", err)
	}
	decoded, err := DecodeCapabilitiesJSON(encoded, AdapterKind(""))
	if err != nil {
		t.Fatalf("DecodeCapabilitiesJSON(encoded) error = %v", err)
	}
	if !decoded.SupportsJSON || len(decoded.OutputModes) != 2 {
		t.Fatalf("decoded encoded capabilities = %+v", decoded)
	}
}

func TestDecodeCapabilitiesJSONRejectsInvalidData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{name: "invalid json", raw: `{`},
		{name: "invalid output mode", raw: `{"output_modes":["yaml"]}`},
		{name: "conflicting json mode", raw: `{"supports_json":false,"output_modes":["json"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeCapabilitiesJSON(tt.raw, AdapterCLINonInteractive); err == nil {
				t.Fatalf("DecodeCapabilitiesJSON(%q) error = nil, want error", tt.raw)
			}
		})
	}

	if _, err := (AgentCapabilities{OutputModes: []OutputMode{OutputMode("yaml")}}).EncodeJSON(); err == nil {
		t.Fatal("EncodeJSON(invalid output mode) error = nil, want error")
	}
}

func TestAdapterKindValidAndEventTerminal(t *testing.T) {
	t.Parallel()

	for _, kind := range []AdapterKind{
		AdapterCLINonInteractive,
		AdapterJSONRPCStdio,
		AdapterACPStdio,
		AdapterMCP,
		AdapterA2A,
		AdapterProviderAPI,
		AdapterLocalVerifier,
	} {
		if !kind.Valid() {
			t.Fatalf("Valid(%q) = false", kind)
		}
	}
	if AdapterKind("shell").Valid() {
		t.Fatal(`Valid("shell") = true, want false`)
	}
	for _, eventType := range []EventType{EventCompleted, EventFailed, EventCanceled} {
		if !eventType.Terminal() {
			t.Fatalf("Terminal(%q) = false", eventType)
		}
	}
	if EventProgress.Terminal() {
		t.Fatal("Terminal(progress) = true, want false")
	}
}

func TestAdapterPropagatesTaskValidation(t *testing.T) {
	t.Parallel()

	adapter := fakeAdapter{id: "agent_1", kind: AdapterLocalVerifier}
	_, err := adapter.RunTask(context.Background(), AgentTask{})
	if err == nil {
		t.Fatal("RunTask(invalid) error = nil, want error")
	}
}

type fakeAdapter struct {
	id   string
	kind AdapterKind
}

func (a fakeAdapter) ID() string {
	return a.id
}

func (a fakeAdapter) Kind() AdapterKind {
	return a.kind
}

func (a fakeAdapter) HealthCheck(context.Context) (AgentHealth, error) {
	return AgentHealth{
		Status:    HealthAvailable,
		CheckedAt: time.Now().UTC(),
	}, nil
}

func (a fakeAdapter) Capabilities(context.Context) (AgentCapabilities, error) {
	return AgentCapabilities{
		SupportsJSON: true,
		CanRead:      true,
		CanWrite:     false,
		OutputModes:  []OutputMode{OutputJSON, OutputText},
	}, nil
}

func (a fakeAdapter) RunTask(ctx context.Context, task AgentTask) (<-chan AgentEvent, error) {
	if err := task.Validate(); err != nil {
		return nil, err
	}
	events := make(chan AgentEvent, 2)
	now := time.Now().UTC()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	events <- AgentEvent{Type: EventStarted, RunID: task.RunID, At: now}
	events <- AgentEvent{Type: EventCompleted, RunID: task.RunID, At: now.Add(time.Second)}
	close(events)
	return events, nil
}

func (a fakeAdapter) Cancel(_ context.Context, runID string) error {
	if runID == "" {
		return errors.New("run id is required")
	}
	return nil
}

type fakeDriver struct{}

func (d fakeDriver) Open(_ context.Context, config ConnectionConfig) (Connection, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return fakeConnection{}, nil
}

type fakeConnection struct{}

func (c fakeConnection) SendTask(_ context.Context, task AgentTask) (<-chan AgentEvent, error) {
	if err := task.Validate(); err != nil {
		return nil, err
	}
	events := make(chan AgentEvent, 1)
	events <- AgentEvent{Type: EventCompleted, RunID: task.RunID, At: time.Now().UTC()}
	close(events)
	return events, nil
}

func (c fakeConnection) Close(context.Context) error {
	return nil
}

var _ AgentAdapter = fakeAdapter{}
var _ ConnectionDriver = fakeDriver{}
var _ Connection = fakeConnection{}
