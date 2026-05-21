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
	"strings"
	"sync"
	"time"
)

const (
	defaultCommandStdoutLimit       int64 = 1 << 20
	defaultCommandStderrLimit       int64 = 1 << 20
	defaultCommandRetryAttempts           = 3
	defaultCommandRetryInitialDelay       = 3 * time.Second
	defaultCommandRetryMaxDelay           = 15 * time.Second
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
	if err := ValidateCommandSafety(config.Command, config.CommandSafety); err != nil {
		return nil, err
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
	config.Env = NormalizeCLIEnvironment(config.Command, config.Env)
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

	events := make(chan AgentEvent, 32)
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

	env, cleanupEnv, err := PrepareCommandRuntimeEnvironment(c.config.Command, c.config.Env)
	if err != nil {
		events <- failedEvent(task.RunID, err, "")
		return
	}
	defer cleanupEnv()

	command, err := ResolveCommandExecutableWithEnv(c.config.Command, env)
	if err != nil {
		events <- failedEvent(task.RunID, err, "")
		return
	}

	retryPolicy := commandRetryPolicyFromMetadata(c.config.Metadata)
	for attempt := 1; attempt <= retryPolicy.maxAttempts; attempt++ {
		delivery, err := c.preparePrompt(task)
		if err != nil {
			events <- promptFailedEvent(task.RunID, err)
			return
		}
		completed, terminal := c.runCommandAttempt(runCtx, task, command, env, delivery, attempt, events)
		delivery.cleanup()
		if completed {
			return
		}
		if terminal.Type == EventCanceled || !shouldRetryCommandFailure(terminal, attempt, retryPolicy.maxAttempts) {
			events <- terminal
			return
		}
		delay := retryPolicy.delay(attempt)
		events <- commandRetryEvent(task.RunID, terminal, attempt+1, retryPolicy.maxAttempts, delay)
		if !sleepCommandRetry(runCtx, delay) {
			events <- canceledEvent(task.RunID, runCtx.Err())
			return
		}
	}
}

func (c *CommandOnceConnection) runCommandAttempt(ctx context.Context, task AgentTask, command string, env map[string]string, delivery commandPromptDelivery, attempt int, events chan<- AgentEvent) (bool, AgentEvent) {
	events <- AgentEvent{
		Type:    EventStarted,
		RunID:   task.RunID,
		At:      time.Now().UTC(),
		Message: "command started",
		Metadata: map[string]any{
			"attempt": attempt,
		},
	}

	stdoutLimit := outputLimit(task.Limits.MaxStdoutBytes, defaultCommandStdoutLimit)
	stderrLimit := outputLimit(task.Limits.MaxStderrBytes, defaultCommandStderrLimit)
	stdout := newStreamingOutput(ctx, task.RunID, "stdout", stdoutLimit, events)
	stderr := newStreamingOutput(ctx, task.RunID, "stderr", stderrLimit, events)
	cmd := exec.CommandContext(ctx, command, delivery.args...)
	if c.config.WorkingDirectory != "" {
		cmd.Dir = c.config.WorkingDirectory
	}
	cmd.Env = envList(env)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = delivery.stdin
	configureCommandProcess(cmd)

	err := cmd.Run()
	stdout.Flush()
	stderr.Flush()
	if err != nil {
		if ctx.Err() != nil {
			return false, canceledEvent(task.RunID, ctx.Err())
		}
		return false, failedEvent(task.RunID, err, strings.Join([]string{stdout.String(), stderr.String()}, "\n"))
	}
	exitCode := 0
	events <- AgentEvent{
		Type:     EventCompleted,
		RunID:    task.RunID,
		At:       time.Now().UTC(),
		Message:  "command completed",
		ExitCode: &exitCode,
		Metadata: map[string]any{
			"attempt": attempt,
		},
	}
	return true, AgentEvent{}
}

type commandRetryPolicy struct {
	maxAttempts  int
	initialDelay time.Duration
	maxDelay     time.Duration
}

func commandRetryPolicyFromMetadata(metadata map[string]any) commandRetryPolicy {
	maxAttempts := int(metadataInt64Default(metadata, "retry_max_attempts", defaultCommandRetryAttempts))
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	initialDelayMs := metadataInt64Default(metadata, "retry_initial_delay_ms", int64(defaultCommandRetryInitialDelay/time.Millisecond))
	if initialDelayMs < 0 {
		initialDelayMs = 0
	}
	maxDelayMs := metadataInt64Default(metadata, "retry_max_delay_ms", int64(defaultCommandRetryMaxDelay/time.Millisecond))
	if maxDelayMs < initialDelayMs {
		maxDelayMs = initialDelayMs
	}
	return commandRetryPolicy{
		maxAttempts:  maxAttempts,
		initialDelay: time.Duration(initialDelayMs) * time.Millisecond,
		maxDelay:     time.Duration(maxDelayMs) * time.Millisecond,
	}
}

func (p commandRetryPolicy) delay(attempt int) time.Duration {
	if attempt <= 0 || p.initialDelay <= 0 {
		return 0
	}
	delay := p.initialDelay
	for index := 1; index < attempt; index++ {
		delay *= 2
		if delay >= p.maxDelay {
			return p.maxDelay
		}
	}
	if delay > p.maxDelay {
		return p.maxDelay
	}
	return delay
}

func shouldRetryCommandFailure(event AgentEvent, attempt int, maxAttempts int) bool {
	if attempt >= maxAttempts || event.Type != EventFailed {
		return false
	}
	text := strings.ToLower(strings.Join([]string{event.ErrorCode, event.Error, event.Message}, "\n"))
	return containsTransientCLIError(text)
}

func containsTransientCLIError(text string) bool {
	text = strings.ToLower(text)
	return strings.Contains(text, "429") ||
		strings.Contains(text, "too many requests") ||
		strings.Contains(text, "rate limit") ||
		strings.Contains(text, "rate_limit") ||
		strings.Contains(text, "resource_exhausted") ||
		strings.Contains(text, "quota exceeded") ||
		strings.Contains(text, "temporarily unavailable") ||
		strings.Contains(text, "service unavailable") ||
		strings.Contains(text, "server overloaded") ||
		strings.Contains(text, "status 503") ||
		strings.Contains(text, "status code 503") ||
		strings.Contains(text, "status 502") ||
		strings.Contains(text, "status code 502") ||
		strings.Contains(text, "status 504") ||
		strings.Contains(text, "status code 504")
}

func commandRetryEvent(runID string, cause AgentEvent, nextAttempt int, maxAttempts int, delay time.Duration) AgentEvent {
	return AgentEvent{
		Type:    EventProgress,
		RunID:   runID,
		At:      time.Now().UTC(),
		Message: fmt.Sprintf("transient CLI error; retrying attempt %d of %d", nextAttempt, maxAttempts),
		Error:   strings.TrimSpace(cause.Error),
		Metadata: map[string]any{
			"attempt":        nextAttempt,
			"max_attempts":   maxAttempts,
			"delay_ms":       delay.Milliseconds(),
			"retryable":      true,
			"retry_cause":    cause.ErrorCode,
			"retry_cause_at": cause.At.Format(time.RFC3339Nano),
		},
	}
}

func sleepCommandRetry(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
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

func outputEventMetadata(text string, output *limitedOutput) map[string]any {
	return map[string]any{
		"captured_bytes": int64(len([]byte(text))),
		"limit_bytes":    output.Limit(),
		"truncated":      output.Truncated(),
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
			event.Error = strings.TrimSpace(stderr)
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
	o.append(p)
	return len(p), nil
}

func (o *limitedOutput) append(p []byte) ([]byte, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()

	wasTruncated := o.truncated
	if o.limit <= 0 {
		o.truncated = true
		return nil, !wasTruncated
	}
	remaining := o.limit - int64(o.buf.Len())
	if remaining <= 0 {
		o.truncated = true
		return nil, !wasTruncated
	}
	if int64(len(p)) > remaining {
		stored := append([]byte(nil), p[:int(remaining)]...)
		_, _ = o.buf.Write(stored)
		o.truncated = true
		return stored, !wasTruncated
	}
	stored := append([]byte(nil), p...)
	_, _ = o.buf.Write(stored)
	return stored, false
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

func (o *limitedOutput) Limit() int64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.limit
}

type streamingOutput struct {
	ctx     context.Context
	runID   string
	stream  string
	output  *limitedOutput
	events  chan<- AgentEvent
	mu      sync.Mutex
	pending []byte
}

func newStreamingOutput(ctx context.Context, runID string, stream string, limit int64, events chan<- AgentEvent) *streamingOutput {
	return &streamingOutput{
		ctx:    ctx,
		runID:  runID,
		stream: stream,
		output: &limitedOutput{limit: limit},
		events: events,
	}
}

func (o *streamingOutput) Write(p []byte) (int, error) {
	stored, truncated := o.output.append(p)
	o.emitStored(stored, truncated, false)
	return len(p), nil
}

func (o *streamingOutput) String() string {
	return o.output.String()
}

func (o *streamingOutput) Flush() {
	o.emitStored(nil, false, true)
}

func (o *streamingOutput) emitStored(stored []byte, truncated bool, flush bool) {
	events := o.drainEvents(stored, truncated, flush)
	for _, event := range events {
		select {
		case o.events <- event:
		case <-o.ctx.Done():
			return
		}
	}
}

func (o *streamingOutput) drainEvents(stored []byte, truncated bool, flush bool) []AgentEvent {
	o.mu.Lock()
	defer o.mu.Unlock()

	if len(stored) > 0 {
		o.pending = append(o.pending, stored...)
	}
	events := []AgentEvent{}
	for {
		index := bytes.IndexByte(o.pending, '\n')
		if index < 0 {
			break
		}
		text := string(o.pending[:index+1])
		o.pending = append([]byte(nil), o.pending[index+1:]...)
		events = append(events, o.outputEvent(text, false))
	}
	if truncated || (flush && len(o.pending) > 0) {
		text := string(o.pending)
		o.pending = nil
		events = append(events, o.outputEvent(text, truncated))
	} else if truncated {
		events = append(events, o.outputEvent("", true))
	}
	return events
}

func (o *streamingOutput) outputEvent(text string, truncated bool) AgentEvent {
	return AgentEvent{
		Type:      EventOutput,
		RunID:     o.runID,
		At:        time.Now().UTC(),
		Stream:    o.stream,
		Text:      text,
		Truncated: truncated,
		Metadata:  outputEventMetadata(text, o.output),
	}
}

func outputLimit(value int64, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func truncateMessage(value string) string {
	value = strings.TrimSpace(value)
	const limit = 4096
	if len(value) > limit {
		return value[:limit] + "..."
	}
	return value
}

var _ ConnectionDriver = CommandOnceDriver{}
var _ Connection = (*CommandOnceConnection)(nil)
