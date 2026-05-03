package agents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultCommandStdoutLimit int64 = 1 << 20
	defaultCommandStderrLimit int64 = 1 << 20
)

type CommandOnceDriver struct{}

type CommandOnceConnection struct {
	config ConnectionConfig
	mu     sync.Mutex
	closed bool
}

func (d CommandOnceDriver) Open(ctx context.Context, config ConnectionConfig) (Connection, error) {
	ctx = commandContextOrBackground(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Kind != AdapterCLINonInteractive {
		return nil, errors.New("command once driver requires cli_noninteractive adapter kind")
	}
	if strings.TrimSpace(config.Command) == "" {
		return nil, errors.New("command once driver requires a command")
	}
	if strings.TrimSpace(config.WorkingDirectory) != "" {
		abs, err := filepath.Abs(config.WorkingDirectory)
		if err != nil {
			return nil, fmt.Errorf("resolve command working directory: %w", err)
		}
		stat, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("inspect command working directory: %w", err)
		}
		if !stat.IsDir() {
			return nil, errors.New("command working directory must be a directory")
		}
		config.WorkingDirectory = abs
	}
	if err := validateEnv(config.Env); err != nil {
		return nil, err
	}
	config.PromptDelivery = config.PromptDelivery.Normalize()
	return &CommandOnceConnection{config: config}, nil
}

func (c *CommandOnceConnection) SendTask(ctx context.Context, task AgentTask) (<-chan AgentEvent, error) {
	ctx = commandContextOrBackground(ctx)
	if c == nil {
		return nil, errors.New("command once connection is required")
	}
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil, errors.New("command once connection is closed")
	}
	if err := task.Validate(); err != nil {
		return nil, err
	}
	if task.Limits.MaxPromptBytes > 0 && int64(len(task.Prompt)) > task.Limits.MaxPromptBytes {
		return nil, errors.New("agent task prompt exceeds limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	events := make(chan AgentEvent, 8)
	go c.runTask(ctx, task, events)
	return events, nil
}

func (c *CommandOnceConnection) Close(context.Context) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *CommandOnceConnection) runTask(ctx context.Context, task AgentTask, events chan<- AgentEvent) {
	defer close(events)

	runCtx, cancel := commandContext(ctx, task.Limits)
	defer cancel()

	delivery, err := c.preparePrompt(task)
	if err != nil {
		events <- promptFailedEvent(task.RunID, err)
		return
	}
	defer delivery.cleanup()

	events <- AgentEvent{
		Type:    EventStarted,
		RunID:   task.RunID,
		At:      time.Now().UTC(),
		Message: "command started",
	}

	stdoutLimit := outputLimit(task.Limits.MaxStdoutBytes, defaultCommandStdoutLimit)
	stderrLimit := outputLimit(task.Limits.MaxStderrBytes, defaultCommandStderrLimit)
	stdout := &limitedOutput{limit: stdoutLimit}
	stderr := &limitedOutput{limit: stderrLimit}

	cmd := exec.CommandContext(runCtx, c.config.Command, delivery.args...)
	if c.config.WorkingDirectory != "" {
		cmd.Dir = c.config.WorkingDirectory
	}
	cmd.Env = envList(c.config.Env)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = delivery.stdin

	err = cmd.Run()
	emitCommandOutput(events, task.RunID, stdout, stderr)
	if err != nil {
		if runCtx.Err() != nil {
			events <- canceledEvent(task.RunID, runCtx.Err())
			return
		}
		events <- failedEvent(task.RunID, err, stderr.String())
		return
	}
	exitCode := 0
	events <- AgentEvent{
		Type:     EventCompleted,
		RunID:    task.RunID,
		At:       time.Now().UTC(),
		Message:  "command completed",
		ExitCode: &exitCode,
	}
}

type commandPromptDelivery struct {
	args    []string
	stdin   io.Reader
	cleanup func()
}

func (c *CommandOnceConnection) preparePrompt(task AgentTask) (commandPromptDelivery, error) {
	delivery := commandPromptDelivery{
		args:    append([]string(nil), c.config.Args...),
		cleanup: func() {},
	}
	if task.Prompt == "" {
		return delivery, nil
	}

	switch c.config.PromptDelivery.Normalize() {
	case PromptViaStdin:
		delivery.stdin = strings.NewReader(task.Prompt)
	case PromptViaArg:
		delivery.args = applyPromptArgument(delivery.args, PromptArgPlaceholder, task.Prompt)
	case PromptViaTempFile:
		path, cleanup, err := writePromptTempFile(task.Prompt)
		if err != nil {
			return commandPromptDelivery{}, err
		}
		delivery.args = applyPromptArgument(delivery.args, PromptFilePlaceholder, path)
		delivery.cleanup = cleanup
	default:
		return commandPromptDelivery{}, fmt.Errorf("unsupported prompt delivery %q", c.config.PromptDelivery)
	}
	return delivery, nil
}

func writePromptTempFile(prompt string) (string, func(), error) {
	file, err := os.CreateTemp("", "cocode-prompt-*.txt")
	if err != nil {
		return "", nil, fmt.Errorf("create prompt temp file: %w", err)
	}
	path := file.Name()
	cleanup := func() {
		_ = os.Remove(path)
	}
	if _, err := file.WriteString(prompt); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, fmt.Errorf("write prompt temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close prompt temp file: %w", err)
	}
	return path, cleanup, nil
}

func applyPromptArgument(args []string, placeholder string, value string) []string {
	out := append([]string(nil), args...)
	replaced := false
	for index, arg := range out {
		if strings.Contains(arg, placeholder) {
			out[index] = strings.ReplaceAll(arg, placeholder, value)
			replaced = true
		}
	}
	if !replaced {
		out = append(out, value)
	}
	return out
}

func commandContextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func commandContext(parent context.Context, limits TaskLimits) (context.Context, context.CancelFunc) {
	timeout := limits.Timeout
	if timeout <= 0 && limits.TimeoutSeconds > 0 {
		timeout = time.Duration(limits.TimeoutSeconds) * time.Second
	}
	if timeout > 0 {
		return context.WithTimeout(parent, timeout)
	}
	return context.WithCancel(parent)
}

func emitCommandOutput(events chan<- AgentEvent, runID string, stdout *limitedOutput, stderr *limitedOutput) {
	if text := stdout.String(); text != "" || stdout.Truncated() {
		events <- AgentEvent{
			Type:      EventOutput,
			RunID:     runID,
			At:        time.Now().UTC(),
			Stream:    "stdout",
			Text:      text,
			Truncated: stdout.Truncated(),
		}
	}
	if text := stderr.String(); text != "" || stderr.Truncated() {
		events <- AgentEvent{
			Type:      EventOutput,
			RunID:     runID,
			At:        time.Now().UTC(),
			Stream:    "stderr",
			Text:      text,
			Truncated: stderr.Truncated(),
		}
	}
}

func failedEvent(runID string, err error, stderr string) AgentEvent {
	event := AgentEvent{
		Type:      EventFailed,
		RunID:     runID,
		At:        time.Now().UTC(),
		Message:   "command failed",
		ErrorCode: "exit_error",
		Error:     strings.TrimSpace(err.Error()),
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		event.ExitCode = &code
		if strings.TrimSpace(stderr) != "" {
			event.Error = truncateMessage(stderr)
		}
	}
	return event
}

func promptFailedEvent(runID string, err error) AgentEvent {
	return AgentEvent{
		Type:      EventFailed,
		RunID:     runID,
		At:        time.Now().UTC(),
		Message:   "prepare prompt failed",
		ErrorCode: "prompt_error",
		Error:     err.Error(),
	}
}

func canceledEvent(runID string, err error) AgentEvent {
	code := "canceled"
	message := "command canceled"
	if errors.Is(err, context.DeadlineExceeded) {
		code = "timeout"
		message = "command timed out"
	}
	return AgentEvent{
		Type:      EventCanceled,
		RunID:     runID,
		At:        time.Now().UTC(),
		Message:   message,
		ErrorCode: code,
		Error:     err.Error(),
	}
}

type limitedOutput struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int64
	truncated bool
}

func (o *limitedOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.limit <= 0 {
		o.truncated = true
		return len(p), nil
	}
	remaining := o.limit - int64(o.buf.Len())
	if remaining <= 0 {
		o.truncated = true
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		_, _ = o.buf.Write(p[:int(remaining)])
		o.truncated = true
		return len(p), nil
	}
	_, _ = o.buf.Write(p)
	return len(p), nil
}

func (o *limitedOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

func (o *limitedOutput) Truncated() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.truncated
}

func outputLimit(value int64, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func envList(env map[string]string) []string {
	if len(env) == 0 {
		return []string{}
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+env[key])
	}
	return out
}

func validateEnv(env map[string]string) error {
	for key := range env {
		if strings.TrimSpace(key) == "" || strings.Contains(key, "=") {
			return fmt.Errorf("invalid environment variable name %q", key)
		}
	}
	return nil
}

func truncateMessage(value string) string {
	value = strings.TrimSpace(value)
	const limit = 300
	if len(value) > limit {
		return value[:limit] + "..."
	}
	return value
}

var _ ConnectionDriver = CommandOnceDriver{}
var _ Connection = (*CommandOnceConnection)(nil)
