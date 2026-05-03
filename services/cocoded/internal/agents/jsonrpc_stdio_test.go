package agents

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestJSONRPCStdioDriverIsDisabledForFutureAdapters(t *testing.T) {
	t.Parallel()

	for _, kind := range []AdapterKind{AdapterJSONRPCStdio, AdapterACPStdio} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()

			_, err := (JSONRPCStdioDriver{}).Open(context.Background(), ConnectionConfig{
				AdapterID: "agent_1",
				Kind:      kind,
				Command:   "future-agent",
			})
			if !errors.Is(err, ErrJSONRPCStdioDisabled) {
				t.Fatalf("Open() error = %v, want ErrJSONRPCStdioDisabled", err)
			}
		})
	}
}

func TestJSONRPCStdioDriverRejectsWrongKindAndMissingCommand(t *testing.T) {
	t.Parallel()

	_, err := (JSONRPCStdioDriver{}).Open(context.Background(), ConnectionConfig{
		AdapterID: "agent_1",
		Kind:      AdapterCLINonInteractive,
		Command:   "codex",
	})
	if err == nil || !strings.Contains(err.Error(), "requires jsonrpc_stdio or acp_stdio") {
		t.Fatalf("Open(wrong kind) error = %v", err)
	}

	_, err = (JSONRPCStdioDriver{}).Open(context.Background(), ConnectionConfig{
		AdapterID: "agent_1",
		Kind:      AdapterJSONRPCStdio,
	})
	if err == nil || !strings.Contains(err.Error(), "requires a command") {
		t.Fatalf("Open(missing command) error = %v", err)
	}
}

func TestJSONRPCStdioConnectionEmitsUnsupportedFailure(t *testing.T) {
	t.Parallel()

	connection := &JSONRPCStdioConnection{config: ConnectionConfig{
		AdapterID: "agent_1",
		Kind:      AdapterJSONRPCStdio,
		Command:   "future-agent",
	}}
	events, err := connection.SendTask(context.Background(), baseCommandTask())
	if err != nil {
		t.Fatalf("SendTask() error = %v", err)
	}
	event := <-events
	if event.Type != EventFailed ||
		event.ErrorCode != "unsupported" ||
		event.Error != ErrJSONRPCStdioDisabled.Error() {
		t.Fatalf("event = %+v", event)
	}
	if _, ok := <-events; ok {
		t.Fatal("events channel should be closed")
	}
}
