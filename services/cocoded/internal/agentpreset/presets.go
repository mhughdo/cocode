package agentpreset

import (
	"encoding/json"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
)

var pathEnvAllowlist = []string{"PATH"}

type Preset struct {
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	Description    string                   `json:"description"`
	Role           string                   `json:"role"`
	AdapterKind    agents.AdapterKind       `json:"adapter_kind"`
	Command        string                   `json:"command"`
	Args           []string                 `json:"args"`
	CWDMode        string                   `json:"cwd_mode"`
	EnvAllowlist   []string                 `json:"env_allowlist"`
	OutputMode     agents.OutputMode        `json:"output_mode"`
	ModelLabel     string                   `json:"model_label"`
	ReasoningLabel string                   `json:"reasoning_label"`
	Capabilities   agents.AgentCapabilities `json:"capabilities"`
	Settings       json.RawMessage          `json:"settings"`
	Enabled        bool                     `json:"enabled"`
}

func List() []Preset {
	return []Preset{CodexCLI(), ClaudeCodeCLI(), GeminiCLI(), OpenCodeCLI(), CustomCLI()}
}

func CodexCLI() Preset {
	settings := json.RawMessage(`{"prompt_delivery":"stdin","timeout_seconds":1800,"version_args":["--version"],"smoke_prompt_enabled":false}`)
	return Preset{
		ID:             "codex-cli",
		Name:           "Codex CLI",
		Description:    "Runs Codex non-interactively against the selected repository and captures JSONL events.",
		Role:           "primary_reviewer",
		AdapterKind:    agents.AdapterCLINonInteractive,
		Command:        "codex",
		Args:           []string{"exec", "--json", "-"},
		CWDMode:        "repo_root",
		EnvAllowlist:   pathEnvAllowlist,
		OutputMode:     agents.OutputJSONL,
		ModelLabel:     "gpt-5.3-codex",
		ReasoningLabel: "high",
		Capabilities: agents.AgentCapabilities{
			SupportsJSON:      true,
			SupportsStreaming: true,
			CanRead:           true,
			CanCancel:         true,
			OutputModes:       []agents.OutputMode{agents.OutputJSONL, agents.OutputNDJSON, agents.OutputText},
			Metadata:          map[string]any{"provider": "openai", "egress": string(agents.AgentEgressExternal)},
		},
		Settings: settings,
		Enabled:  true,
	}
}

func ClaudeCodeCLI() Preset {
	settings := json.RawMessage(`{"prompt_delivery":"arg","timeout_seconds":1800,"version_args":["--version"],"smoke_prompt_enabled":false}`)
	return Preset{
		ID:             "claude-code-cli",
		Name:           "Claude Code CLI",
		Description:    "Runs Claude Code in non-interactive print mode and captures a JSON result payload.",
		Role:           "primary_reviewer",
		AdapterKind:    agents.AdapterCLINonInteractive,
		Command:        "claude",
		Args:           []string{"-p", agents.PromptArgPlaceholder, "--output-format", "json"},
		CWDMode:        "repo_root",
		EnvAllowlist:   append(pathEnvAllowlist, "ANTHROPIC_API_KEY"),
		OutputMode:     agents.OutputJSON,
		ModelLabel:     "claude",
		ReasoningLabel: "",
		Capabilities: agents.AgentCapabilities{
			SupportsJSON: true,
			CanRead:      true,
			CanCancel:    true,
			OutputModes:  []agents.OutputMode{agents.OutputJSON, agents.OutputJSONL, agents.OutputText},
			Metadata:     map[string]any{"provider": "anthropic", "egress": string(agents.AgentEgressExternal)},
		},
		Settings: settings,
		Enabled:  true,
	}
}

func GeminiCLI() Preset {
	settings := json.RawMessage(`{"prompt_delivery":"stdin","timeout_seconds":1800,"version_args":["--version"],"smoke_prompt_enabled":false}`)
	return Preset{
		ID:             "gemini-cli",
		Name:           "Gemini CLI",
		Description:    "Runs Gemini CLI in headless mode with JSON output using the Pro model alias.",
		Role:           "primary_reviewer",
		AdapterKind:    agents.AdapterCLINonInteractive,
		Command:        "gemini",
		Args:           []string{"--model", "pro", "--output-format", "json"},
		CWDMode:        "repo_root",
		EnvAllowlist:   append(pathEnvAllowlist, "GEMINI_API_KEY", "GOOGLE_API_KEY", "GOOGLE_GENAI_USE_VERTEXAI", "GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_CLOUD_PROJECT", "GOOGLE_CLOUD_LOCATION"),
		OutputMode:     agents.OutputJSON,
		ModelLabel:     "pro",
		ReasoningLabel: "",
		Capabilities: agents.AgentCapabilities{
			SupportsJSON: true,
			CanRead:      true,
			CanCancel:    true,
			OutputModes:  []agents.OutputMode{agents.OutputJSON, agents.OutputJSONL, agents.OutputText},
			Metadata:     map[string]any{"provider": "google", "egress": string(agents.AgentEgressExternal)},
		},
		Settings: settings,
		Enabled:  true,
	}
}

func OpenCodeCLI() Preset {
	settings := json.RawMessage(`{"prompt_delivery":"arg","timeout_seconds":1800,"version_args":["--version"],"smoke_prompt_enabled":false}`)
	return Preset{
		ID:             "opencode-cli",
		Name:           "OpenCode CLI",
		Description:    "Runs OpenCode in non-interactive run mode and captures raw JSON event output.",
		Role:           "primary_reviewer",
		AdapterKind:    agents.AdapterCLINonInteractive,
		Command:        "opencode",
		Args:           []string{"run", "--format", "json", agents.PromptArgPlaceholder},
		CWDMode:        "repo_root",
		EnvAllowlist:   append(pathEnvAllowlist, "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENROUTER_API_KEY"),
		OutputMode:     agents.OutputJSONL,
		ModelLabel:     "opencode",
		ReasoningLabel: "",
		Capabilities: agents.AgentCapabilities{
			SupportsJSON:      true,
			SupportsStreaming: true,
			CanRead:           true,
			CanCancel:         true,
			OutputModes:       []agents.OutputMode{agents.OutputJSONL, agents.OutputNDJSON, agents.OutputJSON, agents.OutputText},
			Metadata:          map[string]any{"provider": "opencode", "egress": string(agents.AgentEgressExternal)},
		},
		Settings: settings,
		Enabled:  true,
	}
}

func CustomCLI() Preset {
	settings := json.RawMessage(`{"prompt_delivery":"stdin","timeout_seconds":1800,"skip_version":true,"smoke_prompt_enabled":false}`)
	return Preset{
		ID:             "custom-cli",
		Name:           "Custom CLI",
		Description:    "Template for a user-provided non-interactive CLI command, arguments, output mode, and health settings.",
		Role:           "custom_reviewer",
		AdapterKind:    agents.AdapterCLINonInteractive,
		Command:        "",
		Args:           []string{},
		CWDMode:        "repo_root",
		EnvAllowlist:   pathEnvAllowlist,
		OutputMode:     agents.OutputText,
		ModelLabel:     "custom",
		ReasoningLabel: "",
		Capabilities: agents.AgentCapabilities{
			SupportsJSON: true,
			CanRead:      true,
			CanCancel:    true,
			OutputModes:  []agents.OutputMode{agents.OutputText, agents.OutputJSON, agents.OutputJSONL, agents.OutputNDJSON},
			Metadata:     map[string]any{"provider": "custom", "egress": string(agents.AgentEgressExternal)},
		},
		Settings: settings,
		Enabled:  false,
	}
}
