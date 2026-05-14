package agentpreset

import (
	"encoding/json"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
)

func TestCodexCLIPreset(t *testing.T) {
	t.Parallel()

	preset := CodexCLI()
	if preset.ID != "codex-cli" ||
		preset.Command != "codex" ||
		preset.Role != "orchestrator" ||
		preset.AdapterKind != agents.AdapterCLINonInteractive ||
		preset.OutputMode != agents.OutputJSONL ||
		preset.ModelLabel != "default" ||
		!preset.Capabilities.CanCancel ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputJSONL) {
		t.Fatalf("preset = %+v", preset)
	}
	if len(preset.Args) != 14 ||
		preset.Args[0] != "-a" ||
		preset.Args[1] != "never" ||
		preset.Args[2] != "exec" ||
		preset.Args[3] != "--json" ||
		preset.Args[4] != "--sandbox" ||
		preset.Args[5] != "workspace-write" ||
		preset.Args[6] != "--add-dir" ||
		preset.Args[7] != agents.CLIRuntimeBaseDir() ||
		preset.Args[8] != "--skip-git-repo-check" ||
		preset.Args[9] != "--ephemeral" ||
		preset.Args[10] != "--ignore-rules" ||
		preset.Args[11] != "--color" ||
		preset.Args[12] != "never" ||
		preset.Args[13] != "-" {
		t.Fatalf("preset args = %+v", preset.Args)
	}
	if !containsString(preset.EnvAllowlist, "PATH") ||
		!containsString(preset.EnvAllowlist, "HOME") {
		t.Fatalf("env allowlist = %+v", preset.EnvAllowlist)
	}
	settings := decodePresetSettings(t, preset)
	if settings.PromptDelivery != agents.PromptViaStdin ||
		settings.TimeoutSeconds != 1800 ||
		len(settings.VersionArgs) != 1 ||
		settings.VersionArgs[0] != "--version" ||
		settings.SmokePromptEnabled {
		t.Fatalf("settings = %+v", settings)
	}
}

func TestCodexAppServerPreset(t *testing.T) {
	t.Parallel()

	preset := CodexAppServer()
	if preset.ID != "codex-app-server" ||
		preset.Command != "codex" ||
		preset.AdapterKind != agents.AdapterJSONRPCStdio ||
		preset.OutputMode != agents.OutputJSON ||
		preset.ModelLabel != "default" ||
		!preset.Capabilities.SupportsStreaming ||
		!preset.Capabilities.SupportsSessions ||
		!preset.Capabilities.CanCancel ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputJSON) {
		t.Fatalf("preset = %+v", preset)
	}
	if len(preset.Args) != 3 ||
		preset.Args[0] != "app-server" ||
		preset.Args[1] != "--listen" ||
		preset.Args[2] != "stdio://" {
		t.Fatalf("preset args = %+v", preset.Args)
	}
	settings := decodePresetSettings(t, preset)
	if settings.PromptDelivery != agents.PromptViaStdin ||
		settings.TimeoutSeconds != 1800 ||
		len(settings.VersionArgs) != 2 ||
		settings.VersionArgs[0] != "app-server" ||
		settings.VersionArgs[1] != "--help" ||
		settings.Protocol != "codex_app_server" ||
		settings.SmokePromptEnabled {
		t.Fatalf("settings = %+v", settings)
	}
}

func TestClaudeCodeCLIPreset(t *testing.T) {
	t.Parallel()

	preset := ClaudeCodeCLI()
	if preset.ID != "claude-code-cli" ||
		preset.Command != "claude" ||
		preset.AdapterKind != agents.AdapterCLINonInteractive ||
		preset.OutputMode != agents.OutputJSONL ||
		preset.ModelLabel != "claude" ||
		!preset.Capabilities.SupportsStreaming ||
		!preset.Capabilities.CanCancel ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputJSONL) {
		t.Fatalf("preset = %+v", preset)
	}
	if len(preset.Args) != 9 ||
		preset.Args[0] != "-p" ||
		preset.Args[1] != agents.PromptArgPlaceholder ||
		preset.Args[2] != "--output-format" ||
		preset.Args[3] != "stream-json" ||
		preset.Args[4] != "--verbose" ||
		preset.Args[5] != "--include-partial-messages" ||
		preset.Args[6] != "--permission-mode" ||
		preset.Args[7] != "plan" ||
		preset.Args[8] != "--no-session-persistence" {
		t.Fatalf("preset args = %+v", preset.Args)
	}
	if !containsString(preset.EnvAllowlist, "PATH") ||
		!containsString(preset.EnvAllowlist, "HOME") {
		t.Fatalf("env allowlist = %+v", preset.EnvAllowlist)
	}
	settings := decodePresetSettings(t, preset)
	if settings.PromptDelivery != agents.PromptViaArg ||
		settings.TimeoutSeconds != 1800 ||
		len(settings.VersionArgs) != 1 ||
		settings.VersionArgs[0] != "--version" ||
		settings.SmokePromptEnabled {
		t.Fatalf("settings = %+v", settings)
	}
}

func TestGeminiCLIPreset(t *testing.T) {
	t.Parallel()

	preset := GeminiCLI()
	if preset.ID != "gemini-cli" ||
		preset.Command != "gemini" ||
		preset.AdapterKind != agents.AdapterCLINonInteractive ||
		preset.OutputMode != agents.OutputJSON ||
		preset.ModelLabel != "gemini-3.1-pro-preview" ||
		!preset.Capabilities.CanCancel ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputJSON) {
		t.Fatalf("preset = %+v", preset)
	}
	if len(preset.Args) != 7 ||
		preset.Args[0] != "-p" ||
		preset.Args[1] != agents.PromptArgPlaceholder ||
		preset.Args[2] != "--output-format" ||
		preset.Args[3] != "json" ||
		preset.Args[4] != "--approval-mode" ||
		preset.Args[5] != "default" ||
		preset.Args[6] != "--skip-trust" {
		t.Fatalf("preset args = %+v", preset.Args)
	}
	settings := decodePresetSettings(t, preset)
	if settings.PromptDelivery != agents.PromptViaArg ||
		settings.TimeoutSeconds != 1800 ||
		len(settings.VersionArgs) != 1 ||
		settings.VersionArgs[0] != "--version" ||
		settings.SmokePromptEnabled {
		t.Fatalf("settings = %+v", settings)
	}
	if !containsString(preset.EnvAllowlist, "PATH") ||
		!containsString(preset.EnvAllowlist, "HOME") ||
		!containsString(preset.EnvAllowlist, "GEMINI_API_KEY") ||
		!containsString(preset.EnvAllowlist, "GOOGLE_APPLICATION_CREDENTIALS") {
		t.Fatalf("env allowlist = %+v", preset.EnvAllowlist)
	}
}

func TestGeminiACPPreset(t *testing.T) {
	t.Parallel()

	preset := GeminiACP()
	if preset.ID != "gemini-acp" ||
		preset.Command != "gemini" ||
		preset.AdapterKind != agents.AdapterACPStdio ||
		preset.OutputMode != agents.OutputJSON ||
		preset.ModelLabel != "gemini-acp" ||
		!preset.Capabilities.SupportsStreaming ||
		!preset.Capabilities.SupportsSessions ||
		!preset.Capabilities.CanCancel ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputJSON) {
		t.Fatalf("preset = %+v", preset)
	}
	if len(preset.Args) != 1 || preset.Args[0] != "--acp" {
		t.Fatalf("preset args = %+v", preset.Args)
	}
	settings := decodePresetSettings(t, preset)
	if settings.PromptDelivery != agents.PromptViaStdin ||
		settings.TimeoutSeconds != 1800 ||
		len(settings.VersionArgs) != 1 ||
		settings.VersionArgs[0] != "--help" ||
		settings.Protocol != "acp" ||
		settings.SmokePromptEnabled {
		t.Fatalf("settings = %+v", settings)
	}
	if !containsString(preset.EnvAllowlist, "PATH") ||
		!containsString(preset.EnvAllowlist, "HOME") ||
		!containsString(preset.EnvAllowlist, "GEMINI_API_KEY") ||
		!containsString(preset.EnvAllowlist, "GOOGLE_APPLICATION_CREDENTIALS") {
		t.Fatalf("env allowlist = %+v", preset.EnvAllowlist)
	}
}

func TestOpenCodeCLIPreset(t *testing.T) {
	t.Parallel()

	preset := OpenCodeCLI()
	if preset.ID != "opencode-cli" ||
		preset.Command != "opencode" ||
		preset.AdapterKind != agents.AdapterCLINonInteractive ||
		preset.OutputMode != agents.OutputJSONL ||
		preset.ModelLabel != "opencode-go/kimi-k2.6" ||
		preset.ReasoningLabel != "high" ||
		!preset.Capabilities.CanCancel ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputJSONL) ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputJSON) {
		t.Fatalf("preset = %+v", preset)
	}
	if len(preset.Args) != 6 ||
		preset.Args[0] != "run" ||
		preset.Args[1] != "--pure" ||
		preset.Args[2] != "--format" ||
		preset.Args[3] != "json" ||
		preset.Args[4] != "--thinking" ||
		preset.Args[5] != agents.PromptArgPlaceholder {
		t.Fatalf("preset args = %+v", preset.Args)
	}
	settings := decodePresetSettings(t, preset)
	if settings.PromptDelivery != agents.PromptViaArg ||
		settings.TimeoutSeconds != 1800 ||
		len(settings.VersionArgs) != 1 ||
		settings.VersionArgs[0] != "--version" ||
		settings.SmokePromptEnabled {
		t.Fatalf("settings = %+v", settings)
	}
	if !containsString(preset.EnvAllowlist, "PATH") ||
		!containsString(preset.EnvAllowlist, "HOME") ||
		!containsString(preset.EnvAllowlist, "OPENAI_API_KEY") ||
		!containsString(preset.EnvAllowlist, "ANTHROPIC_API_KEY") ||
		!containsString(preset.EnvAllowlist, "XAI_API_KEY") {
		t.Fatalf("env allowlist = %+v", preset.EnvAllowlist)
	}
}

func TestOpenCodeACPPreset(t *testing.T) {
	t.Parallel()

	preset := OpenCodeACP()
	if preset.ID != "opencode-acp" ||
		preset.Command != "opencode" ||
		preset.AdapterKind != agents.AdapterACPStdio ||
		preset.OutputMode != agents.OutputJSON ||
		preset.ModelLabel != "opencode-acp" ||
		!preset.Capabilities.SupportsStreaming ||
		!preset.Capabilities.SupportsSessions ||
		!preset.Capabilities.CanCancel ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputJSON) {
		t.Fatalf("preset = %+v", preset)
	}
	if len(preset.Args) != 1 || preset.Args[0] != "acp" {
		t.Fatalf("preset args = %+v", preset.Args)
	}
	settings := decodePresetSettings(t, preset)
	if settings.PromptDelivery != agents.PromptViaStdin ||
		settings.TimeoutSeconds != 1800 ||
		len(settings.VersionArgs) != 2 ||
		settings.VersionArgs[0] != "acp" ||
		settings.VersionArgs[1] != "--help" ||
		settings.Protocol != "acp" ||
		settings.SmokePromptEnabled {
		t.Fatalf("settings = %+v", settings)
	}
	if !containsString(preset.EnvAllowlist, "PATH") ||
		!containsString(preset.EnvAllowlist, "HOME") ||
		!containsString(preset.EnvAllowlist, "OPENAI_API_KEY") ||
		!containsString(preset.EnvAllowlist, "OPENROUTER_API_KEY") ||
		!containsString(preset.EnvAllowlist, "XAI_API_KEY") {
		t.Fatalf("env allowlist = %+v", preset.EnvAllowlist)
	}
}

func TestKiroCLIPreset(t *testing.T) {
	t.Parallel()

	preset := KiroCLI()
	if preset.ID != "kiro-cli" ||
		preset.Command != "kiro-cli" ||
		preset.AdapterKind != agents.AdapterCLINonInteractive ||
		preset.OutputMode != agents.OutputText ||
		preset.ModelLabel != "auto" ||
		preset.Capabilities.SupportsJSON ||
		!preset.Capabilities.CanCancel ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputText) {
		t.Fatalf("preset = %+v", preset)
	}
	if len(preset.Args) != 6 ||
		preset.Args[0] != "chat" ||
		preset.Args[1] != "--no-interactive" ||
		preset.Args[2] != "--trust-tools=read,grep,glob,code" ||
		preset.Args[3] != "--wrap" ||
		preset.Args[4] != "never" ||
		preset.Args[5] != agents.PromptArgPlaceholder {
		t.Fatalf("preset args = %+v", preset.Args)
	}
	settings := decodePresetSettings(t, preset)
	if settings.PromptDelivery != agents.PromptViaArg ||
		settings.TimeoutSeconds != 1800 ||
		len(settings.VersionArgs) != 1 ||
		settings.VersionArgs[0] != "--version" ||
		settings.SmokePromptEnabled {
		t.Fatalf("settings = %+v", settings)
	}
	if !containsString(preset.EnvAllowlist, "PATH") ||
		!containsString(preset.EnvAllowlist, "HOME") ||
		!containsString(preset.EnvAllowlist, "KIRO_API_KEY") ||
		!containsString(preset.EnvAllowlist, "KIRO_HOME") {
		t.Fatalf("env allowlist = %+v", preset.EnvAllowlist)
	}
}

func TestCustomCLIPreset(t *testing.T) {
	t.Parallel()

	preset := CustomCLI()
	if preset.ID != "custom-cli" ||
		preset.Command != "" ||
		preset.Role != "custom_reviewer" ||
		preset.AdapterKind != agents.AdapterCLINonInteractive ||
		preset.OutputMode != agents.OutputText ||
		preset.ModelLabel != "custom" ||
		preset.Enabled ||
		!preset.Capabilities.CanCancel ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputNDJSON) {
		t.Fatalf("preset = %+v", preset)
	}
	settings := decodePresetSettings(t, preset)
	if settings.PromptDelivery != agents.PromptViaStdin ||
		settings.TimeoutSeconds != 1800 ||
		!settings.SkipVersion ||
		settings.SmokePromptEnabled {
		t.Fatalf("settings = %+v", settings)
	}
	if !containsString(preset.EnvAllowlist, "PATH") ||
		!containsString(preset.EnvAllowlist, "HOME") {
		t.Fatalf("env allowlist = %+v", preset.EnvAllowlist)
	}
}

func TestListIncludesKnownPresets(t *testing.T) {
	t.Parallel()

	presets := List()
	ids := make(map[string]struct{}, len(presets))
	for _, preset := range presets {
		ids[preset.ID] = struct{}{}
	}
	for _, id := range []string{"codex-cli", "codex-app-server", "claude-code-cli", "gemini-cli", "gemini-acp", "opencode-cli", "opencode-acp", "kiro-cli", "custom-cli"} {
		if _, ok := ids[id]; !ok {
			t.Fatalf("preset %q missing from %+v", id, presets)
		}
	}
}

type presetSettings struct {
	PromptDelivery     agents.PromptDelivery `json:"prompt_delivery"`
	TimeoutSeconds     int64                 `json:"timeout_seconds"`
	VersionArgs        []string              `json:"version_args"`
	SkipVersion        bool                  `json:"skip_version"`
	SmokePromptEnabled bool                  `json:"smoke_prompt_enabled"`
	Protocol           string                `json:"protocol"`
}

func decodePresetSettings(t *testing.T, preset Preset) presetSettings {
	t.Helper()

	var settings presetSettings
	if err := json.Unmarshal(preset.Settings, &settings); err != nil {
		t.Fatalf("Unmarshal(settings) error = %v", err)
	}
	return settings
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
