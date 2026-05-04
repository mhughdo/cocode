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
		preset.AdapterKind != agents.AdapterCLINonInteractive ||
		preset.OutputMode != agents.OutputJSONL ||
		preset.ModelLabel != "default" ||
		!preset.Capabilities.CanCancel ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputJSONL) {
		t.Fatalf("preset = %+v", preset)
	}
	if len(preset.Args) != 10 ||
		preset.Args[0] != "exec" ||
		preset.Args[1] != "--json" ||
		preset.Args[2] != "--sandbox" ||
		preset.Args[3] != "read-only" ||
		preset.Args[4] != "--skip-git-repo-check" ||
		preset.Args[5] != "--ephemeral" ||
		preset.Args[6] != "--ignore-rules" ||
		preset.Args[7] != "--color" ||
		preset.Args[8] != "never" ||
		preset.Args[9] != "-" {
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
		preset.OutputMode != agents.OutputJSON ||
		preset.ModelLabel != "claude" ||
		!preset.Capabilities.CanCancel ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputJSON) {
		t.Fatalf("preset = %+v", preset)
	}
	if len(preset.Args) != 9 ||
		preset.Args[0] != "-p" ||
		preset.Args[1] != agents.PromptArgPlaceholder ||
		preset.Args[2] != "--output-format" ||
		preset.Args[3] != "json" ||
		preset.Args[4] != "--permission-mode" ||
		preset.Args[5] != "plan" ||
		preset.Args[6] != "--no-session-persistence" ||
		preset.Args[7] != "--tools" ||
		preset.Args[8] != "" {
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
		preset.ModelLabel != "default" ||
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
		preset.Args[5] != "plan" ||
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
		preset.ModelLabel != "opencode" ||
		!preset.Capabilities.CanCancel ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputJSONL) ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputJSON) {
		t.Fatalf("preset = %+v", preset)
	}
	if len(preset.Args) != 4 ||
		preset.Args[0] != "run" ||
		preset.Args[1] != "--format" ||
		preset.Args[2] != "json" ||
		preset.Args[3] != agents.PromptArgPlaceholder {
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
	for _, id := range []string{"codex-cli", "codex-app-server", "claude-code-cli", "gemini-cli", "gemini-acp", "opencode-cli", "opencode-acp", "custom-cli"} {
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
