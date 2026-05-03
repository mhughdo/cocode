package agents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCommandOnceDriverRunsFakeCLIWithoutShellOrAmbientEnv(t *testing.T) {
	t.Setenv("COCODE_PARENT_SECRET", "should-not-leak")
	command := writeFakeCommand(t, `#!/bin/sh
input=$(/bin/cat)
printf 'arg=%s prompt=%s explicit=%s secret=%s\n' "$1" "$input" "$COCODE_EXPLICIT" "${COCODE_PARENT_SECRET-unset}"
printf 'warn\n' >&2
`)
	connection := openCommandOnce(t, ConnectionConfig{
		AdapterID:        "agent_1",
		Kind:             AdapterCLINonInteractive,
		Command:          command,
		Args:             []string{"literal; exit 9"},
		WorkingDirectory: t.TempDir(),
		Env:              map[string]string{"COCODE_EXPLICIT": "visible"},
	})

	events, err := connection.SendTask(context.Background(), baseCommandTask())
	if err != nil {
		t.Fatalf("SendTask() error = %v", err)
	}
	got := collectCommandEvents(t, events)

	if got[0].Type != EventStarted {
		t.Fatalf("first event = %+v, want started", got[0])
	}
	stdout := outputText(got, "stdout")
	if !strings.Contains(stdout, "arg=literal; exit 9") ||
		!strings.Contains(stdout, "prompt=review this diff") ||
		!strings.Contains(stdout, "explicit=visible") ||
		!strings.Contains(stdout, "secret=unset") {
		t.Fatalf("stdout = %q", stdout)
	}
	if stderr := outputText(got, "stderr"); stderr != "warn\n" {
		t.Fatalf("stderr = %q, want warn", stderr)
	}
	terminal := got[len(got)-1]
	if terminal.Type != EventCompleted || terminal.ExitCode == nil || *terminal.ExitCode != 0 {
		t.Fatalf("terminal event = %+v, want completed exit 0", terminal)
	}
}

func TestCommandOnceDriverDeliversPromptViaArg(t *testing.T) {
	t.Parallel()

	command := writeFakeCommand(t, `#!/bin/sh
input=$(/bin/cat)
printf 'flag=%s prompt=%s stdin=%s count=%s\n' "$1" "$2" "$input" "$#"
`)
	connection := openCommandOnce(t, ConnectionConfig{
		AdapterID:        "agent_1",
		Kind:             AdapterCLINonInteractive,
		Command:          command,
		Args:             []string{"--prompt", PromptArgPlaceholder},
		PromptDelivery:   PromptViaArg,
		WorkingDirectory: t.TempDir(),
	})

	events, err := connection.SendTask(context.Background(), baseCommandTask())
	if err != nil {
		t.Fatalf("SendTask() error = %v", err)
	}
	got := collectCommandEvents(t, events)

	stdout := outputText(got, "stdout")
	if !strings.Contains(stdout, "flag=--prompt") ||
		!strings.Contains(stdout, "prompt=review this diff") ||
		!strings.Contains(stdout, "stdin=") ||
		!strings.Contains(stdout, "count=2") {
		t.Fatalf("stdout = %q", stdout)
	}
	if terminal := got[len(got)-1]; terminal.Type != EventCompleted {
		t.Fatalf("terminal event = %+v, want completed", terminal)
	}
}

func TestCommandOnceDriverDeliversPromptViaTempFileAndCleansUp(t *testing.T) {
	t.Parallel()

	command := writeFakeCommand(t, `#!/bin/sh
path="${1#--prompt-file=}"
if [ ! -f "$path" ]; then
  printf 'missing prompt file\n' >&2
  exit 6
fi
printf 'prompt='
/bin/cat "$path"
printf '\npath=%s\n' "$path"
`)
	connection := openCommandOnce(t, ConnectionConfig{
		AdapterID:        "agent_1",
		Kind:             AdapterCLINonInteractive,
		Command:          command,
		Args:             []string{"--prompt-file=" + PromptFilePlaceholder},
		PromptDelivery:   PromptViaTempFile,
		WorkingDirectory: t.TempDir(),
	})

	events, err := connection.SendTask(context.Background(), baseCommandTask())
	if err != nil {
		t.Fatalf("SendTask() error = %v", err)
	}
	got := collectCommandEvents(t, events)

	stdout := outputText(got, "stdout")
	if !strings.Contains(stdout, "prompt=review this diff") {
		t.Fatalf("stdout = %q", stdout)
	}
	path := promptPathFromOutput(t, stdout)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prompt temp file stat error = %v, want os.ErrNotExist", err)
	}
	if terminal := got[len(got)-1]; terminal.Type != EventCompleted {
		t.Fatalf("terminal event = %+v, want completed", terminal)
	}
}

func TestCommandOnceDriverCleansPromptTempFileAfterFailure(t *testing.T) {
	t.Parallel()

	command := writeFakeCommand(t, "#!/bin/sh\nprintf 'path=%s\\n' \"$1\"\nexit 7\n")
	connection := openCommandOnce(t, ConnectionConfig{
		AdapterID:        "agent_1",
		Kind:             AdapterCLINonInteractive,
		Command:          command,
		PromptDelivery:   PromptViaTempFile,
		WorkingDirectory: t.TempDir(),
	})

	events, err := connection.SendTask(context.Background(), baseCommandTask())
	if err != nil {
		t.Fatalf("SendTask() error = %v", err)
	}
	got := collectCommandEvents(t, events)

	path := promptPathFromOutput(t, outputText(got, "stdout"))
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prompt temp file stat error = %v, want os.ErrNotExist", err)
	}
	terminal := got[len(got)-1]
	if terminal.Type != EventFailed || terminal.ExitCode == nil || *terminal.ExitCode != 7 {
		t.Fatalf("terminal event = %+v, want failed exit 7", terminal)
	}
}

func TestCommandOnceDriverCleansPromptTempFileAfterCancellation(t *testing.T) {
	t.Parallel()

	markerPath := filepath.Join(t.TempDir(), "prompt-path")
	command := writeFakeCommand(t, "#!/bin/sh\nprintf '%s\\n' \"$2\" > \"$1\"\nexec /bin/sleep 2\n")
	connection := openCommandOnce(t, ConnectionConfig{
		AdapterID:        "agent_1",
		Kind:             AdapterCLINonInteractive,
		Command:          command,
		Args:             []string{markerPath},
		PromptDelivery:   PromptViaTempFile,
		WorkingDirectory: t.TempDir(),
	})
	ctx, cancel := context.WithCancel(context.Background())

	events, err := connection.SendTask(ctx, baseCommandTask())
	if err != nil {
		t.Fatalf("SendTask() error = %v", err)
	}
	path := waitForPromptPathMarker(t, markerPath)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("prompt temp file should exist before cancellation: %v", err)
	}
	cancel()
	got := collectCommandEvents(t, events)

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prompt temp file stat error = %v, want os.ErrNotExist", err)
	}
	terminal := got[len(got)-1]
	if terminal.Type != EventCanceled || terminal.ErrorCode != "canceled" {
		t.Fatalf("terminal event = %+v, want canceled", terminal)
	}
}

func TestCommandOnceDriverReportsExitFailure(t *testing.T) {
	t.Parallel()

	command := writeFakeCommand(t, "#!/bin/sh\nprintf 'fatal from fake\\n' >&2\nexit 7\n")
	connection := openCommandOnce(t, ConnectionConfig{
		AdapterID:        "agent_1",
		Kind:             AdapterCLINonInteractive,
		Command:          command,
		WorkingDirectory: t.TempDir(),
	})

	events, err := connection.SendTask(context.Background(), baseCommandTask())
	if err != nil {
		t.Fatalf("SendTask() error = %v", err)
	}
	got := collectCommandEvents(t, events)

	terminal := got[len(got)-1]
	if terminal.Type != EventFailed ||
		terminal.ErrorCode != "exit_error" ||
		terminal.ExitCode == nil ||
		*terminal.ExitCode != 7 ||
		!strings.Contains(terminal.Error, "fatal from fake") {
		t.Fatalf("terminal event = %+v, want failed exit 7 with stderr", terminal)
	}
}

func TestCommandOnceDriverReportsTimeout(t *testing.T) {
	t.Parallel()

	command := writeFakeCommand(t, "#!/bin/sh\nexec /bin/sleep 2\n")
	connection := openCommandOnce(t, ConnectionConfig{
		AdapterID:        "agent_1",
		Kind:             AdapterCLINonInteractive,
		Command:          command,
		WorkingDirectory: t.TempDir(),
	})
	task := baseCommandTask()
	task.Limits.Timeout = 10 * time.Millisecond

	events, err := connection.SendTask(context.Background(), task)
	if err != nil {
		t.Fatalf("SendTask() error = %v", err)
	}
	got := collectCommandEvents(t, events)

	terminal := got[len(got)-1]
	if terminal.Type != EventCanceled || terminal.ErrorCode != "timeout" {
		t.Fatalf("terminal event = %+v, want timeout cancel", terminal)
	}
}

func TestCommandOnceDriverReportsParentCancellation(t *testing.T) {
	t.Parallel()

	command := writeFakeCommand(t, "#!/bin/sh\nexec /bin/sleep 2\n")
	connection := openCommandOnce(t, ConnectionConfig{
		AdapterID:        "agent_1",
		Kind:             AdapterCLINonInteractive,
		Command:          command,
		WorkingDirectory: t.TempDir(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	events, err := connection.SendTask(ctx, baseCommandTask())
	if err != nil {
		t.Fatalf("SendTask() error = %v", err)
	}

	first := nextCommandEvent(t, events)
	if first.Type != EventStarted {
		t.Fatalf("first event = %+v, want started", first)
	}
	cancel()
	got := append([]AgentEvent{first}, collectCommandEvents(t, events)...)

	terminal := got[len(got)-1]
	if terminal.Type != EventCanceled || terminal.ErrorCode != "canceled" {
		t.Fatalf("terminal event = %+v, want canceled", terminal)
	}
}

func TestCommandOnceDriverTruncatesOutput(t *testing.T) {
	t.Parallel()

	command := writeFakeCommand(t, "#!/bin/sh\nprintf 'abcdef'\n")
	connection := openCommandOnce(t, ConnectionConfig{
		AdapterID:        "agent_1",
		Kind:             AdapterCLINonInteractive,
		Command:          command,
		WorkingDirectory: t.TempDir(),
	})
	task := baseCommandTask()
	task.Limits.MaxStdoutBytes = 3

	events, err := connection.SendTask(context.Background(), task)
	if err != nil {
		t.Fatalf("SendTask() error = %v", err)
	}
	got := collectCommandEvents(t, events)

	output := outputEvent(got, "stdout")
	if output.Text != "abc" || !output.Truncated {
		t.Fatalf("stdout output = %+v, want truncated abc", output)
	}
	terminal := got[len(got)-1]
	if terminal.Type != EventCompleted {
		t.Fatalf("terminal event = %+v, want completed", terminal)
	}
}

func TestCommandOnceDriverValidation(t *testing.T) {
	t.Parallel()

	command := writeFakeCommand(t, "#!/bin/sh\nexit 0\n")
	tests := []struct {
		name   string
		config ConnectionConfig
	}{
		{
			name: "missing command",
			config: ConnectionConfig{
				AdapterID: "agent_1",
				Kind:      AdapterCLINonInteractive,
			},
		},
		{
			name: "wrong kind",
			config: ConnectionConfig{
				AdapterID: "agent_1",
				Kind:      AdapterJSONRPCStdio,
				Command:   command,
			},
		},
		{
			name: "missing working directory",
			config: ConnectionConfig{
				AdapterID:        "agent_1",
				Kind:             AdapterCLINonInteractive,
				Command:          command,
				WorkingDirectory: filepath.Join(t.TempDir(), "missing"),
			},
		},
		{
			name: "invalid env name",
			config: ConnectionConfig{
				AdapterID: "agent_1",
				Kind:      AdapterCLINonInteractive,
				Command:   command,
				Env:       map[string]string{"BAD=KEY": "value"},
			},
		},
		{
			name: "invalid prompt delivery",
			config: ConnectionConfig{
				AdapterID:      "agent_1",
				Kind:           AdapterCLINonInteractive,
				Command:        command,
				PromptDelivery: PromptDelivery("pipe"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := (CommandOnceDriver{}).Open(context.Background(), tt.config); err == nil {
				t.Fatalf("Open(%+v) error = nil, want error", tt.config)
			}
		})
	}
}

func TestCommandOnceConnectionRejectsClosedAndOversizedPrompt(t *testing.T) {
	t.Parallel()

	command := writeFakeCommand(t, "#!/bin/sh\nexit 0\n")
	connection := openCommandOnce(t, ConnectionConfig{
		AdapterID:        "agent_1",
		Kind:             AdapterCLINonInteractive,
		Command:          command,
		WorkingDirectory: t.TempDir(),
	})
	task := baseCommandTask()
	task.Limits.MaxPromptBytes = 3
	if _, err := connection.SendTask(context.Background(), task); err == nil {
		t.Fatal("SendTask() error = nil, want oversized prompt error")
	}
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := connection.SendTask(canceledCtx, baseCommandTask()); !errors.Is(err, context.Canceled) {
		t.Fatalf("SendTask() error = %v, want context canceled", err)
	}
	if err := connection.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := connection.SendTask(context.Background(), baseCommandTask()); err == nil {
		t.Fatal("SendTask() after Close error = nil, want error")
	}
}

func openCommandOnce(t *testing.T, config ConnectionConfig) Connection {
	t.Helper()

	connection, err := (CommandOnceDriver{}).Open(context.Background(), config)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return connection
}

func baseCommandTask() AgentTask {
	return AgentTask{
		ID:              "task_1",
		RunID:           "run_1",
		ReviewSessionID: "session_1",
		AgentConfigID:   "agent_1",
		Role:            "reviewer",
		Prompt:          "review this diff",
	}
}

func writeFakeCommand(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-agent")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func collectCommandEvents(t *testing.T, events <-chan AgentEvent) []AgentEvent {
	t.Helper()

	timeout := time.After(3 * time.Second)
	var got []AgentEvent
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return got
			}
			got = append(got, event)
		case <-timeout:
			t.Fatal("timed out waiting for command events")
		}
	}
}

func nextCommandEvent(t *testing.T, events <-chan AgentEvent) AgentEvent {
	t.Helper()

	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("events channel closed before next event")
		}
		return event
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for command event")
	}
	return AgentEvent{}
}

func outputText(events []AgentEvent, stream string) string {
	return outputEvent(events, stream).Text
}

func outputEvent(events []AgentEvent, stream string) AgentEvent {
	for _, event := range events {
		if event.Type == EventOutput && event.Stream == stream {
			return event
		}
	}
	return AgentEvent{}
}

func promptPathFromOutput(t *testing.T, output string) string {
	t.Helper()

	for _, line := range strings.Split(output, "\n") {
		path, ok := strings.CutPrefix(line, "path=")
		if ok && path != "" {
			return path
		}
	}
	t.Fatalf("output %q did not include prompt temp path", output)
	return ""
}

func waitForPromptPathMarker(t *testing.T, markerPath string) string {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		marker, err := os.ReadFile(markerPath)
		if err == nil {
			path := strings.TrimSpace(string(marker))
			if path != "" {
				return path
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("prompt path marker %q was not written", markerPath)
	return ""
}
