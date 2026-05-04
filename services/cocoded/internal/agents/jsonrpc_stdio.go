package agents

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ErrJSONRPCStdioDisabled = errors.New("json-rpc stdio connections are disabled")

const (
	defaultJSONRPCFrameLimit  int64 = 16 << 20
	defaultJSONRPCStderrLimit int64 = 1 << 20
)

type JSONRPCStdioDriver struct {
	Enabled bool
}

type JSONRPCStdioConnection struct {
	config        ConnectionConfig
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	maxFrameBytes int64
	stderr        *limitedOutput

	writeMu sync.Mutex

	mu            sync.Mutex
	closed        bool
	nextID        int64
	pending       map[string]chan jsonRPCResult
	notifications chan jsonRPCNotification
	done          chan error
	waitDone      chan error
	closeOnce     sync.Once
}

type jsonRPCEnvelope struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *jsonRPCError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == 0 {
		return e.Message
	}
	return fmt.Sprintf("json-rpc error %d: %s", e.Code, e.Message)
}

type jsonRPCResult struct {
	result json.RawMessage
	err    error
}

type jsonRPCNotification struct {
	Method string
	Params json.RawMessage
}

func (d JSONRPCStdioDriver) Open(ctx context.Context, config ConnectionConfig) (Connection, error) {
	ctx = commandContextOrBackground(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Kind != AdapterJSONRPCStdio && config.Kind != AdapterACPStdio {
		return nil, errors.New("json-rpc stdio driver requires jsonrpc_stdio or acp_stdio adapter kind")
	}
	if strings.TrimSpace(config.Command) == "" {
		return nil, errors.New("json-rpc stdio driver requires a command")
	}
	if !d.Enabled {
		return nil, ErrJSONRPCStdioDisabled
	}
	if err := ValidateCommandSafety(config.Command, config.CommandSafety); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.WorkingDirectory) != "" {
		abs, err := filepath.Abs(config.WorkingDirectory)
		if err != nil {
			return nil, fmt.Errorf("resolve json-rpc working directory: %w", err)
		}
		stat, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("inspect json-rpc working directory: %w", err)
		}
		if !stat.IsDir() {
			return nil, errors.New("json-rpc working directory must be a directory")
		}
		config.WorkingDirectory = abs
	}
	if err := validateEnv(config.Env); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, config.Command, config.Args...)
	if config.WorkingDirectory != "" {
		cmd.Dir = config.WorkingDirectory
	}
	cmd.Env = envList(config.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open json-rpc stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open json-rpc stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("open json-rpc stderr: %w", err)
	}

	connection := &JSONRPCStdioConnection{
		config:        config,
		cmd:           cmd,
		stdin:         stdin,
		maxFrameBytes: metadataInt64Default(config.Metadata, "max_frame_bytes", defaultJSONRPCFrameLimit),
		stderr:        &limitedOutput{limit: metadataInt64Default(config.Metadata, "max_stderr_bytes", defaultJSONRPCStderrLimit)},
		pending:       map[string]chan jsonRPCResult{},
		notifications: make(chan jsonRPCNotification, 128),
		done:          make(chan error, 1),
		waitDone:      make(chan error, 1),
	}
	if connection.maxFrameBytes <= 0 {
		connection.maxFrameBytes = defaultJSONRPCFrameLimit
	}
	if connection.stderr.limit <= 0 {
		connection.stderr.limit = defaultJSONRPCStderrLimit
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start json-rpc command: %w", err)
	}
	go connection.readStdout(stdout)
	go connection.readStderr(stderr)
	go func() {
		connection.waitDone <- cmd.Wait()
	}()
	return connection, nil
}

func (c *JSONRPCStdioConnection) SendTask(ctx context.Context, task AgentTask) (<-chan AgentEvent, error) {
	ctx = commandContextOrBackground(ctx)
	if c == nil {
		return nil, errors.New("json-rpc stdio connection is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := task.Validate(); err != nil {
		return nil, err
	}
	if task.Limits.MaxPromptBytes > 0 && int64(len(task.Prompt)) > task.Limits.MaxPromptBytes {
		return nil, errors.New("agent task prompt exceeds limit")
	}
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil, errors.New("json-rpc stdio connection is closed")
	}

	events := make(chan AgentEvent, 32)
	go func() {
		defer close(events)
		switch c.config.Kind {
		case AdapterJSONRPCStdio:
			c.runCodexAppServerTask(ctx, task, events)
		case AdapterACPStdio:
			c.runACPTask(ctx, task, events)
		default:
			events <- failedProtocolEvent(task.RunID, "unsupported_adapter", fmt.Errorf("unsupported json-rpc adapter kind %q", c.config.Kind))
		}
		c.emitStderr(task.RunID, events)
	}()
	return events, nil
}

func (c *JSONRPCStdioConnection) Close(ctx context.Context) error {
	if c == nil {
		return nil
	}
	ctx = commandContextOrBackground(ctx)
	var closeErr error
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		pending := c.pending
		c.pending = map[string]chan jsonRPCResult{}
		c.mu.Unlock()
		for _, ch := range pending {
			ch <- jsonRPCResult{err: errors.New("json-rpc connection closed")}
		}
		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		if c.cmd != nil && c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		select {
		case err := <-c.waitDone:
			closeErr = err
		case <-ctx.Done():
			closeErr = ctx.Err()
		case <-time.After(2 * time.Second):
			closeErr = errors.New("timed out waiting for json-rpc command to exit")
		}
	})
	if closeErr != nil && strings.Contains(closeErr.Error(), "signal: killed") {
		return nil
	}
	return closeErr
}

func (c *JSONRPCStdioConnection) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	ctx = commandContextOrBackground(ctx)
	method = strings.TrimSpace(method)
	if method == "" {
		return nil, errors.New("json-rpc method is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id := atomic.AddInt64(&c.nextID, 1)
	key := strconv.FormatInt(id, 10)
	responseCh := make(chan jsonRPCResult, 1)

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("json-rpc stdio connection is closed")
	}
	c.pending[key] = responseCh
	c.mu.Unlock()

	if err := c.writeJSON(jsonRPCEnvelope{
		JSONRPC: "2.0",
		ID:      json.RawMessage(key),
		Method:  method,
		Params:  mustRawMessage(params),
	}); err != nil {
		c.removePending(key)
		return nil, err
	}

	select {
	case result := <-responseCh:
		return result.result, result.err
	case err := <-c.done:
		c.removePending(key)
		return nil, err
	case <-ctx.Done():
		c.removePending(key)
		return nil, ctx.Err()
	}
}

func (c *JSONRPCStdioConnection) notify(method string, params any) error {
	method = strings.TrimSpace(method)
	if method == "" {
		return errors.New("json-rpc notification method is required")
	}
	return c.writeJSON(jsonRPCEnvelope{
		JSONRPC: "2.0",
		Method:  method,
		Params:  mustRawMessage(params),
	})
}

func (c *JSONRPCStdioConnection) writeJSON(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode json-rpc message: %w", err)
	}
	data = append(data, '\n')

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.stdin == nil {
		return errors.New("json-rpc stdin is closed")
	}
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("write json-rpc message: %w", err)
	}
	return nil
}

func (c *JSONRPCStdioConnection) readStdout(stdout io.Reader) {
	err := c.readMessages(stdout)
	c.failPending(err)
}

func (c *JSONRPCStdioConnection) readMessages(stdout io.Reader) error {
	defer close(c.notifications)
	reader := bufio.NewReader(stdout)
	for {
		frame, err := readJSONRPCFrame(reader, c.maxFrameBytes)
		if len(bytes.TrimSpace(frame)) > 0 {
			if dispatchErr := c.dispatchFrame(frame); dispatchErr != nil {
				return dispatchErr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func (c *JSONRPCStdioConnection) dispatchFrame(frame []byte) error {
	var envelope jsonRPCEnvelope
	if err := json.Unmarshal(frame, &envelope); err != nil {
		return fmt.Errorf("decode json-rpc frame: %w", err)
	}
	if len(envelope.ID) > 0 && (envelope.Result != nil || envelope.Error != nil) {
		key := string(bytes.TrimSpace(envelope.ID))
		c.mu.Lock()
		ch := c.pending[key]
		delete(c.pending, key)
		c.mu.Unlock()
		if ch != nil {
			if envelope.Error != nil {
				ch <- jsonRPCResult{err: envelope.Error}
			} else {
				ch <- jsonRPCResult{result: cloneRaw(envelope.Result)}
			}
		}
		return nil
	}
	if envelope.Method != "" && len(envelope.ID) > 0 {
		return c.rejectServerRequest(envelope)
	}
	if envelope.Method != "" {
		notification := jsonRPCNotification{Method: envelope.Method, Params: cloneRaw(envelope.Params)}
		select {
		case c.notifications <- notification:
		default:
			return errors.New("json-rpc notification buffer is full")
		}
	}
	return nil
}

func (c *JSONRPCStdioConnection) rejectServerRequest(envelope jsonRPCEnvelope) error {
	response := struct {
		JSONRPC string          `json:"jsonrpc,omitempty"`
		ID      json.RawMessage `json:"id"`
		Error   jsonRPCError    `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      envelope.ID,
		Error: jsonRPCError{
			Code:    -32601,
			Message: fmt.Sprintf("method %q is not supported by cocode", envelope.Method),
		},
	}
	return c.writeJSON(response)
}

func (c *JSONRPCStdioConnection) readStderr(stderr io.Reader) {
	_, _ = io.Copy(c.stderr, stderr)
}

func (c *JSONRPCStdioConnection) removePending(key string) {
	c.mu.Lock()
	delete(c.pending, key)
	c.mu.Unlock()
}

func (c *JSONRPCStdioConnection) failPending(err error) {
	if err == nil {
		err = io.EOF
	}
	c.mu.Lock()
	pending := c.pending
	c.pending = map[string]chan jsonRPCResult{}
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- jsonRPCResult{err: err}
	}
	select {
	case c.done <- err:
	default:
	}
}

func (c *JSONRPCStdioConnection) emitStderr(runID string, events chan<- AgentEvent) {
	if c.stderr == nil {
		return
	}
	text := c.stderr.String()
	if text == "" && !c.stderr.Truncated() {
		return
	}
	events <- AgentEvent{
		Type:      EventOutput,
		RunID:     runID,
		At:        time.Now().UTC(),
		Stream:    "stderr",
		Text:      text,
		Truncated: c.stderr.Truncated(),
		Metadata:  outputEventMetadata(text, c.stderr),
	}
}

func (c *JSONRPCStdioConnection) runCodexAppServerTask(ctx context.Context, task AgentTask, events chan<- AgentEvent) {
	if _, err := c.request(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "cocode",
			"version": "0.0.0",
		},
		"capabilities": map[string]any{
			"experimentalApi":           true,
			"optOutNotificationMethods": []string{},
		},
	}); err != nil {
		events <- failedProtocolEvent(task.RunID, "codex_initialize_failed", err)
		return
	}
	_ = c.notify("initialized", nil)

	threadParams := map[string]any{
		"cwd":                    firstNonEmptyString(task.RepositoryRoot, c.config.WorkingDirectory),
		"approvalPolicy":         "never",
		"sandbox":                "read-only",
		"serviceName":            "cocode",
		"developerInstructions":  "You are running as a read-only code review agent inside cocode. Do not modify files; return findings in the requested output contract.",
		"ephemeral":              true,
		"experimentalRawEvents":  false,
		"persistExtendedHistory": false,
	}
	if model := metadataString(c.config.Metadata, "model_label"); model != "" {
		threadParams["model"] = model
	}
	threadResult, err := c.request(ctx, "thread/start", threadParams)
	if err != nil {
		events <- failedProtocolEvent(task.RunID, "codex_thread_start_failed", err)
		return
	}
	threadID := rawStringAt(threadResult, "thread", "id")
	if threadID == "" {
		events <- failedProtocolEvent(task.RunID, "codex_thread_start_invalid", errors.New("thread/start response did not include thread.id"))
		return
	}

	events <- AgentEvent{
		Type:    EventStarted,
		RunID:   task.RunID,
		At:      time.Now().UTC(),
		Message: "codex app-server turn started",
		Metadata: map[string]any{
			"protocol":  "codex_app_server",
			"thread_id": threadID,
		},
	}

	turnParams := map[string]any{
		"threadId": threadID,
		"input": []map[string]any{{
			"type":          "text",
			"text":          task.Prompt,
			"text_elements": []any{},
		}},
	}
	if cwd := firstNonEmptyString(task.RepositoryRoot, c.config.WorkingDirectory); cwd != "" {
		turnParams["cwd"] = cwd
	}
	if model := metadataString(c.config.Metadata, "model_label"); model != "" {
		turnParams["model"] = model
	}
	if effort := metadataString(c.config.Metadata, "reasoning_label"); effort != "" {
		turnParams["effort"] = effort
	}

	resultCh := make(chan jsonRPCResult, 1)
	go func() {
		result, err := c.request(ctx, "turn/start", turnParams)
		resultCh <- jsonRPCResult{result: result, err: err}
	}()

	turnID := ""
	for {
		select {
		case result := <-resultCh:
			if result.err != nil {
				events <- failedProtocolEvent(task.RunID, "codex_turn_start_failed", result.err)
				return
			}
			turnID = firstNonEmptyString(turnID, rawStringAt(result.result, "turn", "id"))
			if status := rawStringAt(result.result, "turn", "status"); status == "completed" {
				events <- completedProtocolEvent(task.RunID, "codex app-server turn completed")
				return
			}
			if status := rawStringAt(result.result, "turn", "status"); status == "failed" {
				events <- failedProtocolEvent(task.RunID, "codex_turn_failed", errors.New(firstNonEmptyString(rawStringAt(result.result, "turn", "error", "message"), "codex turn failed")))
				return
			}
		case notification, ok := <-c.notifications:
			if !ok {
				events <- failedProtocolEvent(task.RunID, "codex_connection_closed", errors.New("codex app-server closed before turn completed"))
				return
			}
			if terminal := mapCodexNotification(task.RunID, turnID, notification, events); terminal {
				return
			}
		case <-ctx.Done():
			events <- canceledEvent(task.RunID, ctx.Err())
			return
		}
	}
}

func (c *JSONRPCStdioConnection) runACPTask(ctx context.Context, task AgentTask, events chan<- AgentEvent) {
	if _, err := c.request(ctx, "initialize", map[string]any{
		"protocolVersion": 1,
		"clientInfo": map[string]any{
			"name":    "cocode",
			"version": "0.0.0",
		},
		"clientCapabilities": map[string]any{
			"auth":     map[string]any{"terminal": false},
			"fs":       map[string]any{"readTextFile": false, "writeTextFile": false},
			"terminal": false,
		},
	}); err != nil {
		events <- failedProtocolEvent(task.RunID, "acp_initialize_failed", err)
		return
	}

	sessionResult, err := c.request(ctx, "session/new", map[string]any{
		"cwd":        firstNonEmptyString(task.RepositoryRoot, c.config.WorkingDirectory),
		"mcpServers": []any{},
	})
	if err != nil {
		events <- failedProtocolEvent(task.RunID, "acp_session_new_failed", err)
		return
	}
	sessionID := rawStringAt(sessionResult, "sessionId")
	if sessionID == "" {
		events <- failedProtocolEvent(task.RunID, "acp_session_new_invalid", errors.New("session/new response did not include sessionId"))
		return
	}

	events <- AgentEvent{
		Type:    EventStarted,
		RunID:   task.RunID,
		At:      time.Now().UTC(),
		Message: "acp session prompt started",
		Metadata: map[string]any{
			"protocol":       "acp",
			"acp_session_id": sessionID,
		},
	}

	resultCh := make(chan jsonRPCResult, 1)
	go func() {
		result, err := c.request(ctx, "session/prompt", map[string]any{
			"sessionId": sessionID,
			"prompt": []map[string]any{{
				"type": "text",
				"text": task.Prompt,
			}},
		})
		resultCh <- jsonRPCResult{result: result, err: err}
	}()

	for {
		select {
		case result := <-resultCh:
			if result.err != nil {
				events <- failedProtocolEvent(task.RunID, "acp_prompt_failed", result.err)
				return
			}
			stopReason := rawStringAt(result.result, "stopReason")
			if stopReason == "" || stopReason == "end_turn" || stopReason == "max_tokens" {
				events <- completedProtocolEvent(task.RunID, "acp session prompt completed")
				return
			}
			if stopReason == "cancelled" {
				events <- AgentEvent{
					Type:      EventCanceled,
					RunID:     task.RunID,
					At:        time.Now().UTC(),
					Message:   "acp session prompt canceled",
					ErrorCode: "canceled",
				}
				return
			}
			events <- failedProtocolEvent(task.RunID, "acp_prompt_stopped", fmt.Errorf("acp prompt stopped with reason %q", stopReason))
			return
		case notification, ok := <-c.notifications:
			if !ok {
				events <- failedProtocolEvent(task.RunID, "acp_connection_closed", errors.New("acp connection closed before prompt completed"))
				return
			}
			mapACPNotification(task.RunID, sessionID, notification, events)
		case <-ctx.Done():
			events <- canceledEvent(task.RunID, ctx.Err())
			return
		}
	}
}

func mapCodexNotification(runID string, turnID string, notification jsonRPCNotification, events chan<- AgentEvent) bool {
	switch notification.Method {
	case "item/agentMessage/delta":
		if delta := rawStringAt(notification.Params, "delta"); delta != "" {
			events <- AgentEvent{
				Type:   EventOutput,
				RunID:  runID,
				At:     time.Now().UTC(),
				Stream: "stdout",
				Text:   delta,
				Metadata: map[string]any{
					"protocol": "codex_app_server",
					"method":   notification.Method,
					"turn_id":  rawStringAt(notification.Params, "turnId"),
					"item_id":  rawStringAt(notification.Params, "itemId"),
				},
			}
		}
	case "turn/started":
		events <- AgentEvent{
			Type:    EventProgress,
			RunID:   runID,
			At:      time.Now().UTC(),
			Message: "codex turn running",
			Payload: cloneRaw(notification.Params),
			Metadata: map[string]any{
				"protocol": "codex_app_server",
				"method":   notification.Method,
			},
		}
	case "turn/completed":
		if turnID != "" && rawStringAt(notification.Params, "turn", "id") != "" && rawStringAt(notification.Params, "turn", "id") != turnID {
			return false
		}
		status := rawStringAt(notification.Params, "turn", "status")
		if status == "failed" {
			events <- failedProtocolEvent(runID, "codex_turn_failed", errors.New(firstNonEmptyString(rawStringAt(notification.Params, "turn", "error", "message"), "codex turn failed")))
			return true
		}
		events <- completedProtocolEvent(runID, "codex app-server turn completed")
		return true
	case "error":
		message := firstNonEmptyString(rawStringAt(notification.Params, "error", "message"), "codex app-server error")
		events <- failedProtocolEvent(runID, "codex_error", errors.New(message))
		return true
	case "turn/diff/updated", "turn/plan/updated", "item/started", "item/completed", "rawResponseItem/completed":
		events <- AgentEvent{
			Type:    EventProgress,
			RunID:   runID,
			At:      time.Now().UTC(),
			Message: notification.Method,
			Payload: cloneRaw(notification.Params),
			Metadata: map[string]any{
				"protocol": "codex_app_server",
				"method":   notification.Method,
			},
		}
	}
	return false
}

func mapACPNotification(runID string, sessionID string, notification jsonRPCNotification, events chan<- AgentEvent) {
	if notification.Method != "session/update" {
		events <- AgentEvent{
			Type:    EventProgress,
			RunID:   runID,
			At:      time.Now().UTC(),
			Message: notification.Method,
			Payload: cloneRaw(notification.Params),
			Metadata: map[string]any{
				"protocol": "acp",
				"method":   notification.Method,
			},
		}
		return
	}
	if session := rawStringAt(notification.Params, "sessionId"); sessionID != "" && session != "" && session != sessionID {
		return
	}
	update := rawMapAt(notification.Params, "update")
	updateType, _ := update["sessionUpdate"].(string)
	switch updateType {
	case "agent_message_chunk":
		text := contentBlockText(update["content"])
		if text == "" {
			return
		}
		events <- AgentEvent{
			Type:   EventOutput,
			RunID:  runID,
			At:     time.Now().UTC(),
			Stream: "stdout",
			Text:   text,
			Metadata: map[string]any{
				"protocol":       "acp",
				"method":         notification.Method,
				"session_update": updateType,
				"acp_session_id": sessionID,
			},
		}
	case "agent_thought_chunk", "plan", "tool_call", "tool_call_update", "available_commands_update", "current_mode_update", "config_option_update", "session_info_update", "usage_update":
		events <- AgentEvent{
			Type:    EventProgress,
			RunID:   runID,
			At:      time.Now().UTC(),
			Message: "acp " + updateType,
			Payload: cloneRaw(notification.Params),
			Metadata: map[string]any{
				"protocol":       "acp",
				"method":         notification.Method,
				"session_update": updateType,
				"acp_session_id": sessionID,
			},
		}
	default:
		events <- AgentEvent{
			Type:    EventProgress,
			RunID:   runID,
			At:      time.Now().UTC(),
			Message: "acp session/update",
			Payload: cloneRaw(notification.Params),
			Metadata: map[string]any{
				"protocol":       "acp",
				"method":         notification.Method,
				"session_update": updateType,
				"acp_session_id": sessionID,
			},
		}
	}
}

func readJSONRPCFrame(reader *bufio.Reader, maxBytes int64) ([]byte, error) {
	var frame []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(part) > 0 {
			frame = append(frame, part...)
			if maxBytes > 0 && int64(len(frame)) > maxBytes {
				return frame, fmt.Errorf("json-rpc frame exceeds limit of %d bytes", maxBytes)
			}
		}
		if err == nil {
			return frame, nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return frame, err
	}
}

func failedProtocolEvent(runID string, code string, err error) AgentEvent {
	return AgentEvent{
		Type:      EventFailed,
		RunID:     runID,
		At:        time.Now().UTC(),
		Message:   "json-rpc agent failed",
		ErrorCode: code,
		Error:     err.Error(),
	}
}

func completedProtocolEvent(runID string, message string) AgentEvent {
	exitCode := 0
	return AgentEvent{
		Type:     EventCompleted,
		RunID:    runID,
		At:       time.Now().UTC(),
		Message:  message,
		ExitCode: &exitCode,
	}
}

func mustRawMessage(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	if raw, ok := value.(json.RawMessage); ok {
		return raw
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func rawStringAt(raw json.RawMessage, path ...string) string {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	for _, segment := range path {
		obj, ok := value.(map[string]any)
		if !ok {
			return ""
		}
		value = obj[segment]
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func rawMapAt(raw json.RawMessage, path ...string) map[string]any {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return nil
	}
	for _, segment := range path {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = obj[segment]
	}
	obj, _ := value.(map[string]any)
	return obj
}

func contentBlockText(value any) string {
	obj, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if obj["type"] != "text" {
		return ""
	}
	text, _ := obj["text"].(string)
	return text
}

func metadataInt64Default(metadata map[string]any, key string, fallback int64) int64 {
	if metadata == nil {
		return fallback
	}
	switch value := metadata[key].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return parsed
		}
	}
	return fallback
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var _ ConnectionDriver = JSONRPCStdioDriver{}
var _ Connection = (*JSONRPCStdioConnection)(nil)
