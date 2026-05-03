package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type AdapterKind string

const (
	AdapterCLINonInteractive AdapterKind = "cli_noninteractive"
	AdapterJSONRPCStdio      AdapterKind = "jsonrpc_stdio"
	AdapterACPStdio          AdapterKind = "acp_stdio"
	AdapterMCP               AdapterKind = "mcp"
	AdapterA2A               AdapterKind = "a2a"
	AdapterProviderAPI       AdapterKind = "provider_api"
	AdapterLocalVerifier     AdapterKind = "local_verifier"
)

type EventType string

const (
	EventQueued    EventType = "queued"
	EventStarted   EventType = "started"
	EventProgress  EventType = "progress"
	EventOutput    EventType = "output"
	EventArtifact  EventType = "artifact"
	EventCompleted EventType = "completed"
	EventFailed    EventType = "failed"
	EventCanceled  EventType = "canceled"
)

type HealthStatus string

const (
	HealthUnknown     HealthStatus = "unknown"
	HealthAvailable   HealthStatus = "available"
	HealthUnavailable HealthStatus = "unavailable"
	HealthDegraded    HealthStatus = "degraded"
)

type OutputMode string

type PromptDelivery string

const (
	OutputText   OutputMode = "text"
	OutputJSON   OutputMode = "json"
	OutputJSONL  OutputMode = "jsonl"
	OutputNDJSON OutputMode = "ndjson"
)

const (
	PromptViaStdin    PromptDelivery = "stdin"
	PromptViaArg      PromptDelivery = "arg"
	PromptViaTempFile PromptDelivery = "temp_file"
)

const (
	PromptArgPlaceholder  = "{{prompt}}"
	PromptFilePlaceholder = "{{prompt_file}}"
)

type AgentAdapter interface {
	ID() string
	Kind() AdapterKind
	HealthCheck(ctx context.Context) (AgentHealth, error)
	Capabilities(ctx context.Context) (AgentCapabilities, error)
	RunTask(ctx context.Context, task AgentTask) (<-chan AgentEvent, error)
	Cancel(ctx context.Context, runID string) error
}

type ConnectionDriver interface {
	Open(ctx context.Context, config ConnectionConfig) (Connection, error)
}

type Connection interface {
	SendTask(ctx context.Context, task AgentTask) (<-chan AgentEvent, error)
	Close(ctx context.Context) error
}

type ConnectionConfig struct {
	AdapterID        string            `json:"adapter_id"`
	Kind             AdapterKind       `json:"kind"`
	Command          string            `json:"command,omitempty"`
	Args             []string          `json:"args,omitempty"`
	PromptDelivery   PromptDelivery    `json:"prompt_delivery,omitempty"`
	WorkingDirectory string            `json:"working_directory,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	Metadata         map[string]any    `json:"metadata,omitempty"`
}

type AgentTask struct {
	ID               string         `json:"id"`
	RunID            string         `json:"run_id"`
	ReviewSessionID  string         `json:"review_session_id"`
	AgentConfigID    string         `json:"agent_config_id"`
	ContextBundleID  string         `json:"context_bundle_id,omitempty"`
	Role             string         `json:"role"`
	Prompt           string         `json:"prompt,omitempty"`
	ContextArtifacts []ArtifactRef  `json:"context_artifacts,omitempty"`
	InputArtifacts   []ArtifactRef  `json:"input_artifacts,omitempty"`
	RepositoryRoot   string         `json:"repository_root,omitempty"`
	WorkspaceRoot    string         `json:"workspace_root,omitempty"`
	Limits           TaskLimits     `json:"limits,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type ArtifactRef struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	RelativePath string `json:"relative_path,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	SizeBytes    int64  `json:"size_bytes,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
}

type TaskLimits struct {
	Timeout        time.Duration `json:"-"`
	TimeoutSeconds int64         `json:"timeout_seconds,omitempty"`
	MaxStdoutBytes int64         `json:"max_stdout_bytes,omitempty"`
	MaxStderrBytes int64         `json:"max_stderr_bytes,omitempty"`
	MaxPromptBytes int64         `json:"max_prompt_bytes,omitempty"`
}

type AgentEvent struct {
	Type       EventType       `json:"type"`
	RunID      string          `json:"run_id"`
	At         time.Time       `json:"at"`
	Message    string          `json:"message,omitempty"`
	Stream     string          `json:"stream,omitempty"`
	Text       string          `json:"text,omitempty"`
	ArtifactID string          `json:"artifact_id,omitempty"`
	ExitCode   *int            `json:"exit_code,omitempty"`
	ErrorCode  string          `json:"error_code,omitempty"`
	Error      string          `json:"error,omitempty"`
	Truncated  bool            `json:"truncated,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Metadata   map[string]any  `json:"metadata,omitempty"`
}

type AgentHealth struct {
	Status    HealthStatus   `json:"status"`
	Message   string         `json:"message,omitempty"`
	CheckedAt time.Time      `json:"checked_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type AgentCapabilities struct {
	SupportsJSON      bool           `json:"supports_json"`
	SupportsStreaming bool           `json:"supports_streaming"`
	SupportsSessions  bool           `json:"supports_sessions"`
	CanRead           bool           `json:"can_read"`
	CanWrite          bool           `json:"can_write"`
	CanCancel         bool           `json:"can_cancel"`
	OutputModes       []OutputMode   `json:"output_modes,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type capabilityJSON struct {
	SupportsJSON      *bool          `json:"supports_json"`
	SupportsStreaming *bool          `json:"supports_streaming"`
	SupportsSessions  *bool          `json:"supports_sessions"`
	CanRead           *bool          `json:"can_read"`
	CanWrite          *bool          `json:"can_write"`
	CanCancel         *bool          `json:"can_cancel"`
	OutputModes       []OutputMode   `json:"output_modes"`
	Metadata          map[string]any `json:"metadata"`
}

func (k AdapterKind) Valid() bool {
	switch k {
	case AdapterCLINonInteractive, AdapterJSONRPCStdio, AdapterACPStdio, AdapterMCP, AdapterA2A, AdapterProviderAPI, AdapterLocalVerifier:
		return true
	default:
		return false
	}
}

func (e EventType) Terminal() bool {
	switch e {
	case EventCompleted, EventFailed, EventCanceled:
		return true
	default:
		return false
	}
}

func (m OutputMode) Valid() bool {
	switch m {
	case OutputText, OutputJSON, OutputJSONL, OutputNDJSON:
		return true
	default:
		return false
	}
}

func (d PromptDelivery) Valid() bool {
	switch d {
	case PromptViaStdin, PromptViaArg, PromptViaTempFile:
		return true
	default:
		return false
	}
}

func (d PromptDelivery) Normalize() PromptDelivery {
	if d == "" {
		return PromptViaStdin
	}
	return d
}

func DefaultCapabilities(kind AdapterKind) AgentCapabilities {
	switch kind {
	case AdapterCLINonInteractive:
		return AgentCapabilities{
			SupportsJSON: true,
			CanRead:      true,
			CanCancel:    true,
			OutputModes:  []OutputMode{OutputText, OutputJSON, OutputJSONL, OutputNDJSON},
		}
	case AdapterLocalVerifier:
		return AgentCapabilities{
			SupportsJSON: true,
			CanRead:      true,
			CanCancel:    true,
			OutputModes:  []OutputMode{OutputText, OutputJSON},
		}
	case AdapterJSONRPCStdio, AdapterACPStdio:
		return AgentCapabilities{
			SupportsJSON:      true,
			SupportsStreaming: true,
			SupportsSessions:  true,
			CanRead:           true,
			CanCancel:         true,
			OutputModes:       []OutputMode{OutputJSON, OutputJSONL, OutputNDJSON},
		}
	case AdapterMCP, AdapterA2A, AdapterProviderAPI:
		return AgentCapabilities{
			SupportsJSON:      true,
			SupportsStreaming: true,
			SupportsSessions:  true,
			CanRead:           true,
			CanCancel:         true,
			OutputModes:       []OutputMode{OutputJSON},
		}
	default:
		return AgentCapabilities{
			OutputModes: []OutputMode{OutputText},
		}
	}
}

func DecodeCapabilitiesJSON(raw string, kind AdapterKind) (AgentCapabilities, error) {
	capabilities := DefaultCapabilities(kind)
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return capabilities, nil
	}

	var decoded capabilityJSON
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return AgentCapabilities{}, fmt.Errorf("decode agent capabilities: %w", err)
	}
	if decoded.SupportsJSON != nil {
		capabilities.SupportsJSON = *decoded.SupportsJSON
	}
	if decoded.SupportsStreaming != nil {
		capabilities.SupportsStreaming = *decoded.SupportsStreaming
	}
	if decoded.SupportsSessions != nil {
		capabilities.SupportsSessions = *decoded.SupportsSessions
	}
	if decoded.CanRead != nil {
		capabilities.CanRead = *decoded.CanRead
	}
	if decoded.CanWrite != nil {
		capabilities.CanWrite = *decoded.CanWrite
	}
	if decoded.CanCancel != nil {
		capabilities.CanCancel = *decoded.CanCancel
	}
	if decoded.OutputModes != nil {
		capabilities.OutputModes = decoded.OutputModes
	}
	if decoded.SupportsJSON != nil && !*decoded.SupportsJSON {
		if decoded.OutputModes != nil && hasStructuredOutput(decoded.OutputModes) {
			return AgentCapabilities{}, errors.New("agent capabilities cannot disable supports_json while declaring structured output modes")
		}
		capabilities.OutputModes = textOnlyOutputModes(capabilities.OutputModes)
	}
	if decoded.Metadata != nil {
		capabilities.Metadata = decoded.Metadata
	}
	if err := capabilities.Validate(); err != nil {
		return AgentCapabilities{}, err
	}
	return capabilities.Normalize(), nil
}

func (c AgentCapabilities) EncodeJSON() (string, error) {
	normalized := c.Normalize()
	if err := normalized.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode agent capabilities: %w", err)
	}
	return string(data), nil
}

func (c AgentCapabilities) Normalize() AgentCapabilities {
	c.OutputModes = normalizeOutputModes(c.OutputModes)
	if len(c.OutputModes) == 0 {
		c.OutputModes = []OutputMode{OutputText}
	}
	if hasStructuredOutput(c.OutputModes) {
		c.SupportsJSON = true
	}
	if c.Metadata == nil {
		c.Metadata = map[string]any{}
	}
	return c
}

func (c AgentCapabilities) Validate() error {
	for _, mode := range c.OutputModes {
		if !mode.Valid() {
			return fmt.Errorf("agent capability output mode %q is invalid", mode)
		}
	}
	return nil
}

func (c AgentCapabilities) SupportsOutputMode(mode OutputMode) bool {
	if !mode.Valid() {
		return false
	}
	for _, candidate := range c.Normalize().OutputModes {
		if candidate == mode {
			return true
		}
	}
	return false
}

func (t AgentTask) Validate() error {
	if strings.TrimSpace(t.ID) == "" {
		return errors.New("agent task id is required")
	}
	if strings.TrimSpace(t.RunID) == "" {
		return errors.New("agent task run id is required")
	}
	if strings.TrimSpace(t.ReviewSessionID) == "" {
		return errors.New("agent task review session id is required")
	}
	if strings.TrimSpace(t.AgentConfigID) == "" {
		return errors.New("agent task config id is required")
	}
	if strings.TrimSpace(t.Role) == "" {
		return errors.New("agent task role is required")
	}
	if strings.TrimSpace(t.Prompt) == "" && len(t.ContextArtifacts) == 0 && len(t.InputArtifacts) == 0 {
		return errors.New("agent task requires prompt or input artifacts")
	}
	if t.Limits.Timeout < 0 || t.Limits.TimeoutSeconds < 0 {
		return errors.New("agent task timeout cannot be negative")
	}
	if t.Limits.MaxStdoutBytes < 0 || t.Limits.MaxStderrBytes < 0 || t.Limits.MaxPromptBytes < 0 {
		return errors.New("agent task byte limits cannot be negative")
	}
	return nil
}

func normalizeOutputModes(modes []OutputMode) []OutputMode {
	if len(modes) == 0 {
		return nil
	}
	seen := map[OutputMode]struct{}{}
	normalized := make([]OutputMode, 0, len(modes))
	for _, mode := range modes {
		if _, exists := seen[mode]; exists {
			continue
		}
		seen[mode] = struct{}{}
		normalized = append(normalized, mode)
	}
	return normalized
}

func hasStructuredOutput(modes []OutputMode) bool {
	for _, mode := range modes {
		if mode == OutputJSON || mode == OutputJSONL || mode == OutputNDJSON {
			return true
		}
	}
	return false
}

func textOnlyOutputModes(modes []OutputMode) []OutputMode {
	kept := make([]OutputMode, 0, len(modes))
	for _, mode := range modes {
		if mode == OutputText {
			kept = append(kept, mode)
		}
	}
	if len(kept) == 0 {
		return []OutputMode{OutputText}
	}
	return kept
}

func (c ConnectionConfig) Validate() error {
	if strings.TrimSpace(c.AdapterID) == "" {
		return errors.New("connection adapter id is required")
	}
	if !c.Kind.Valid() {
		return errors.New("connection adapter kind is invalid")
	}
	if c.PromptDelivery != "" && !c.PromptDelivery.Valid() {
		return errors.New("connection prompt delivery is invalid")
	}
	return nil
}
