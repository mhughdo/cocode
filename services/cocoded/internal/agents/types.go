package agents

import (
	"context"
	"encoding/json"
	"errors"
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
	OutputModes       []string       `json:"output_modes,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
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

func (c ConnectionConfig) Validate() error {
	if strings.TrimSpace(c.AdapterID) == "" {
		return errors.New("connection adapter id is required")
	}
	if !c.Kind.Valid() {
		return errors.New("connection adapter kind is invalid")
	}
	return nil
}
