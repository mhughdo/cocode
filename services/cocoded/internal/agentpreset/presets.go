package agentpreset

import (
	"encoding/json"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
)

var baseCLIEnvAllowlist = []string{
	"PATH",
	"HOME",
	"USER",
	"LOGNAME",
	"SHELL",
	"TMPDIR",
	"TERM",
	"COLORTERM",
	"LANG",
	"NO_COLOR",
}

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
	return []Preset{CodexCLI(), CodexAppServer(), ClaudeCodeCLI(), GeminiCLI(), GeminiACP(), OpenCodeCLI(), OpenCodeACP(), AntigravityCLI(), KiroCLI(), CustomCLI()}
}

func CodexCLI() Preset {
	settings := json.RawMessage(`{"prompt_delivery":"stdin","timeout_seconds":1800,"version_args":["--version"],"smoke_prompt_enabled":false}`)
	return Preset{
		ID:             "codex-cli",
		Name:           "Codex CLI",
		Description:    "Runs Codex non-interactively as the default review orchestrator and captures JSONL events.",
		Role:           "orchestrator",
		AdapterKind:    agents.AdapterCLINonInteractive,
		Command:        "codex",
		Args:           []string{"-a", "never", "exec", "--json", "--sandbox", "read-only", "--skip-git-repo-check", "--ephemeral", "--ignore-rules", "--color", "never", "-"},
		CWDMode:        "repo_root",
		EnvAllowlist:   append([]string{}, baseCLIEnvAllowlist...),
		OutputMode:     agents.OutputJSONL,
		ModelLabel:     "default",
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

func CodexAppServer() Preset {
	settings := json.RawMessage(`{"prompt_delivery":"stdin","timeout_seconds":1800,"version_args":["app-server","--help"],"smoke_prompt_enabled":false,"protocol":"codex_app_server"}`)
	return Preset{
		ID:             "codex-app-server",
		Name:           "Codex App Server",
		Description:    "Runs the Codex app-server JSON-RPC stdio protocol with streaming review output.",
		Role:           "primary_reviewer",
		AdapterKind:    agents.AdapterJSONRPCStdio,
		Command:        "codex",
		Args:           []string{"app-server", "--listen", "stdio://"},
		CWDMode:        "repo_root",
		EnvAllowlist:   append([]string{}, baseCLIEnvAllowlist...),
		OutputMode:     agents.OutputJSON,
		ModelLabel:     "default",
		ReasoningLabel: "high",
		Capabilities: agents.AgentCapabilities{
			SupportsJSON:      true,
			SupportsStreaming: true,
			SupportsSessions:  true,
			CanRead:           true,
			CanCancel:         true,
			OutputModes:       []agents.OutputMode{agents.OutputJSON, agents.OutputJSONL, agents.OutputNDJSON, agents.OutputText},
			Metadata:          map[string]any{"provider": "openai", "egress": string(agents.AgentEgressExternal), "protocol": "codex_app_server"},
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
		Description:    "Runs Claude Code in non-interactive print mode and captures stream JSON, including visible thinking deltas when the model returns them.",
		Role:           "primary_reviewer",
		AdapterKind:    agents.AdapterCLINonInteractive,
		Command:        "claude",
		Args:           []string{"-p", agents.PromptArgPlaceholder, "--output-format", "stream-json", "--verbose", "--include-partial-messages", "--permission-mode", "plan", "--no-session-persistence"},
		CWDMode:        "repo_root",
		EnvAllowlist:   append(baseCLIEnvAllowlist, "ANTHROPIC_API_KEY"),
		OutputMode:     agents.OutputJSONL,
		ModelLabel:     "claude",
		ReasoningLabel: "",
		Capabilities: agents.AgentCapabilities{
			SupportsJSON:      true,
			SupportsStreaming: true,
			CanRead:           true,
			CanCancel:         true,
			OutputModes:       []agents.OutputMode{agents.OutputJSON, agents.OutputJSONL, agents.OutputText},
			Metadata:          map[string]any{"provider": "anthropic", "egress": string(agents.AgentEgressExternal)},
		},
		Settings: settings,
		Enabled:  true,
	}
}

func GeminiCLI() Preset {
	settings := json.RawMessage(`{"prompt_delivery":"arg","timeout_seconds":1800,"version_args":["--version"],"smoke_prompt_enabled":false}`)
	return Preset{
		ID:             "gemini-cli",
		Name:           "Gemini CLI",
		Description:    "Runs Gemini CLI in headless mode with JSON output using Gemini 3.1 Pro Preview unless a model is selected.",
		Role:           "primary_reviewer",
		AdapterKind:    agents.AdapterCLINonInteractive,
		Command:        "gemini",
		Args:           []string{"-p", agents.PromptArgPlaceholder, "--output-format", "json", "--approval-mode", "default", "--skip-trust"},
		CWDMode:        "repo_root",
		EnvAllowlist:   append(baseCLIEnvAllowlist, "GEMINI_API_KEY", "GOOGLE_API_KEY", "GOOGLE_GENAI_USE_VERTEXAI", "GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_CLOUD_PROJECT", "GOOGLE_CLOUD_LOCATION"),
		OutputMode:     agents.OutputJSON,
		ModelLabel:     "gemini-3.1-pro-preview",
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

func GeminiACP() Preset {
	settings := json.RawMessage(`{"prompt_delivery":"stdin","timeout_seconds":1800,"version_args":["--help"],"smoke_prompt_enabled":false,"protocol":"acp"}`)
	return Preset{
		ID:             "gemini-acp",
		Name:           "Gemini ACP",
		Description:    "Runs Gemini through the Agent Client Protocol stdio flow with streaming review output.",
		Role:           "primary_reviewer",
		AdapterKind:    agents.AdapterACPStdio,
		Command:        "gemini",
		Args:           []string{"--acp"},
		CWDMode:        "repo_root",
		EnvAllowlist:   append(baseCLIEnvAllowlist, "GEMINI_API_KEY", "GOOGLE_API_KEY", "GOOGLE_GENAI_USE_VERTEXAI", "GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_CLOUD_PROJECT", "GOOGLE_CLOUD_LOCATION"),
		OutputMode:     agents.OutputJSON,
		ModelLabel:     "gemini-acp",
		ReasoningLabel: "",
		Capabilities: agents.AgentCapabilities{
			SupportsJSON:      true,
			SupportsStreaming: true,
			SupportsSessions:  true,
			CanRead:           true,
			CanCancel:         true,
			OutputModes:       []agents.OutputMode{agents.OutputJSON, agents.OutputJSONL, agents.OutputNDJSON, agents.OutputText},
			Metadata:          map[string]any{"provider": "google", "egress": string(agents.AgentEgressExternal), "protocol": "acp"},
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
		Description:    "Runs OpenCode in non-interactive run mode with opencode-go/kimi-k2.6 and captures JSON event output, including provider-returned thinking blocks when available.",
		Role:           "primary_reviewer",
		AdapterKind:    agents.AdapterCLINonInteractive,
		Command:        "opencode",
		Args:           []string{"run", "--pure", "--format", "json", "--thinking", agents.PromptArgPlaceholder},
		CWDMode:        "repo_root",
		EnvAllowlist:   append(baseCLIEnvAllowlist, "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENROUTER_API_KEY", "XAI_API_KEY"),
		OutputMode:     agents.OutputJSONL,
		ModelLabel:     "opencode-go/kimi-k2.6",
		ReasoningLabel: "high",
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

func OpenCodeACP() Preset {
	settings := json.RawMessage(`{"prompt_delivery":"stdin","timeout_seconds":1800,"version_args":["acp","--help"],"smoke_prompt_enabled":false,"protocol":"acp"}`)
	return Preset{
		ID:             "opencode-acp",
		Name:           "OpenCode ACP",
		Description:    "Runs OpenCode through the Agent Client Protocol stdio flow for model-backed review agents.",
		Role:           "primary_reviewer",
		AdapterKind:    agents.AdapterACPStdio,
		Command:        "opencode",
		Args:           []string{"acp"},
		CWDMode:        "repo_root",
		EnvAllowlist:   append(baseCLIEnvAllowlist, "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENROUTER_API_KEY", "XAI_API_KEY"),
		OutputMode:     agents.OutputJSON,
		ModelLabel:     "opencode-acp",
		ReasoningLabel: "",
		Capabilities: agents.AgentCapabilities{
			SupportsJSON:      true,
			SupportsStreaming: true,
			SupportsSessions:  true,
			CanRead:           true,
			CanCancel:         true,
			OutputModes:       []agents.OutputMode{agents.OutputJSON, agents.OutputJSONL, agents.OutputNDJSON, agents.OutputText},
			Metadata:          map[string]any{"provider": "opencode", "egress": string(agents.AgentEgressExternal), "protocol": "acp"},
		},
		Settings: settings,
		Enabled:  true,
	}
}

func AntigravityCLI() Preset {
	settings := json.RawMessage(`{"prompt_delivery":"stdin","timeout_seconds":1800,"version_args":["--version"],"smoke_prompt_enabled":false}`)
	return Preset{
		ID:             "antigravity-cli",
		Name:           "Antigravity CLI",
		Description:    "Runs Antigravity CLI in non-interactive print mode with sandboxed terminal access and captures the text response.",
		Role:           "primary_reviewer",
		AdapterKind:    agents.AdapterCLINonInteractive,
		Command:        "agy",
		Args:           []string{"--print", "--sandbox", "--print-timeout", "30m0s"},
		CWDMode:        "repo_root",
		EnvAllowlist:   append(append([]string{}, baseCLIEnvAllowlist...), "GEMINI_API_KEY", "GOOGLE_API_KEY", "GOOGLE_GENAI_USE_VERTEXAI", "GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_CLOUD_PROJECT", "GOOGLE_CLOUD_LOCATION"),
		OutputMode:     agents.OutputText,
		ModelLabel:     "gemini-3.5-flash",
		ReasoningLabel: "high",
		Capabilities: agents.AgentCapabilities{
			CanRead:     true,
			CanCancel:   true,
			OutputModes: []agents.OutputMode{agents.OutputText},
			Metadata:    map[string]any{"provider": "antigravity", "upstream_provider": "google", "egress": string(agents.AgentEgressExternal)},
		},
		Settings: settings,
		Enabled:  true,
	}
}

func KiroCLI() Preset {
	settings := json.RawMessage(`{"prompt_delivery":"arg","timeout_seconds":1800,"version_args":["--version"],"smoke_prompt_enabled":false}`)
	return Preset{
		ID:             "kiro-cli",
		Name:           "Kiro CLI",
		Description:    "Runs Kiro CLI headlessly with read-only review tools trusted up front and captures the non-interactive text response.",
		Role:           "primary_reviewer",
		AdapterKind:    agents.AdapterCLINonInteractive,
		Command:        "kiro-cli",
		Args:           []string{"chat", "--no-interactive", "--trust-tools=read,grep,glob,code", "--wrap", "never", agents.PromptArgPlaceholder},
		CWDMode:        "repo_root",
		EnvAllowlist:   append(baseCLIEnvAllowlist, "KIRO_API_KEY", "KIRO_HOME"),
		OutputMode:     agents.OutputText,
		ModelLabel:     "auto",
		ReasoningLabel: "",
		Capabilities: agents.AgentCapabilities{
			CanRead:     true,
			CanCancel:   true,
			OutputModes: []agents.OutputMode{agents.OutputText},
			Metadata:    map[string]any{"provider": "kiro", "egress": string(agents.AgentEgressExternal)},
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
		EnvAllowlist:   append([]string{}, baseCLIEnvAllowlist...),
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
