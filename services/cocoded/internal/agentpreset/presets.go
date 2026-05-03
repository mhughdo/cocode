package agentpreset

import (
	"encoding/json"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
)

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
	return []Preset{CodexCLI(), ClaudeCodeCLI()}
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
		EnvAllowlist:   []string{},
		OutputMode:     agents.OutputJSONL,
		ModelLabel:     "gpt-5.3-codex",
		ReasoningLabel: "high",
		Capabilities: agents.AgentCapabilities{
			SupportsJSON:      true,
			SupportsStreaming: true,
			CanRead:           true,
			CanCancel:         true,
			OutputModes:       []agents.OutputMode{agents.OutputJSONL, agents.OutputNDJSON, agents.OutputText},
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
		EnvAllowlist:   []string{"ANTHROPIC_API_KEY"},
		OutputMode:     agents.OutputJSON,
		ModelLabel:     "claude",
		ReasoningLabel: "",
		Capabilities: agents.AgentCapabilities{
			SupportsJSON: true,
			CanRead:      true,
			CanCancel:    true,
			OutputModes:  []agents.OutputMode{agents.OutputJSON, agents.OutputJSONL, agents.OutputText},
		},
		Settings: settings,
		Enabled:  true,
	}
}
