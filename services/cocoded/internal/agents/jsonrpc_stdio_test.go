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

func TestCodexAppServerJSONRPCConnectionStreamsReviewOutput(t *testing.T) {
	t.Parallel()

	command := writeFakeCommand(t, `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"id":1,"result":{"userAgent":"fake-codex","codexHome":"/tmp/cocode","platformFamily":"unix","platformOs":"macos"}}'
      ;;
    *'"method":"initialized"'*)
      ;;
    *'"method":"thread/start"'*)
      printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread_1"},"model":"fake-model"}}'
      ;;
    *'"method":"turn/start"'*)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn_1","status":"inProgress"}}}'
      printf '%s\n' '{"method":"item/agentMessage/delta","params":{"threadId":"thread_1","turnId":"turn_1","itemId":"item_1","delta":"{\"findings\":[]}"}}'
      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thread_1","turn":{"id":"turn_1","status":"completed"}}}'
      ;;
  esac
done
`)
	connection := openJSONRPCStdio(t, ConnectionConfig{
		AdapterID:        "agent_1",
		Kind:             AdapterJSONRPCStdio,
		Command:          command,
		WorkingDirectory: t.TempDir(),
		Metadata: map[string]any{
			"model_label":     "fake-model",
			"reasoning_label": "high",
		},
	})
	defer func() {
		if err := connection.Close(context.Background()); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}()

	events, err := connection.SendTask(context.Background(), baseCommandTask())
	if err != nil {
		t.Fatalf("SendTask() error = %v", err)
	}
	got := collectCommandEvents(t, events)

	if got[0].Type != EventStarted {
		t.Fatalf("first event = %+v, want started", got[0])
	}
	if stdout := outputText(got, "stdout"); stdout != `{"findings":[]}` {
		t.Fatalf("stdout = %q, want findings JSON", stdout)
	}
	if terminal := got[len(got)-1]; terminal.Type != EventCompleted {
		t.Fatalf("terminal event = %+v, want completed", terminal)
	}
}

func TestACPJSONRPCConnectionStreamsReviewOutput(t *testing.T) {
	t.Parallel()

	command := writeFakeCommand(t, `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"id":1,"result":{"protocolVersion":1,"agentInfo":{"name":"fake-acp","version":"0.0.0"},"capabilities":{}}}'
      ;;
    *'"method":"session/new"'*)
      printf '%s\n' '{"id":2,"result":{"sessionId":"session_1"}}'
      ;;
    *'"method":"session/prompt"'*)
      printf '%s\n' '{"method":"session/update","params":{"sessionId":"session_1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"{\"findings\":[]}"}}}}'
      printf '%s\n' '{"id":3,"result":{"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1}}}'
      ;;
  esac
done
`)
	connection := openJSONRPCStdio(t, ConnectionConfig{
		AdapterID:        "agent_1",
		Kind:             AdapterACPStdio,
		Command:          command,
		WorkingDirectory: t.TempDir(),
	})
	defer func() {
		if err := connection.Close(context.Background()); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}()

	events, err := connection.SendTask(context.Background(), baseCommandTask())
	if err != nil {
		t.Fatalf("SendTask() error = %v", err)
	}
	got := collectCommandEvents(t, events)

	if got[0].Type != EventStarted {
		t.Fatalf("first event = %+v, want started", got[0])
	}
	if stdout := outputText(got, "stdout"); stdout != `{"findings":[]}` {
		t.Fatalf("stdout = %q, want findings JSON", stdout)
	}
	if terminal := got[len(got)-1]; terminal.Type != EventCompleted {
		t.Fatalf("terminal event = %+v, want completed", terminal)
	}
}

func openJSONRPCStdio(t *testing.T, config ConnectionConfig) Connection {
	t.Helper()

	connection, err := (JSONRPCStdioDriver{Enabled: true}).Open(context.Background(), config)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return connection
}
