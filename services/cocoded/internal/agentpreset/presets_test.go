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
		preset.ModelLabel != "gpt-5.3-codex" ||
		!preset.Capabilities.CanCancel ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputJSONL) {
		t.Fatalf("preset = %+v", preset)
	}
	if len(preset.Args) != 3 ||
		preset.Args[0] != "exec" ||
		preset.Args[1] != "--json" ||
		preset.Args[2] != "-" {
		t.Fatalf("preset args = %+v", preset.Args)
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
	if len(preset.Args) != 4 ||
		preset.Args[0] != "-p" ||
		preset.Args[1] != agents.PromptArgPlaceholder ||
		preset.Args[2] != "--output-format" ||
		preset.Args[3] != "json" {
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
}

func TestGeminiCLIPreset(t *testing.T) {
	t.Parallel()

	preset := GeminiCLI()
	if preset.ID != "gemini-cli" ||
		preset.Command != "gemini" ||
		preset.AdapterKind != agents.AdapterCLINonInteractive ||
		preset.OutputMode != agents.OutputJSON ||
		preset.ModelLabel != "pro" ||
		!preset.Capabilities.CanCancel ||
		!preset.Capabilities.SupportsOutputMode(agents.OutputJSON) {
		t.Fatalf("preset = %+v", preset)
	}
	if len(preset.Args) != 4 ||
		preset.Args[0] != "--model" ||
		preset.Args[1] != "pro" ||
		preset.Args[2] != "--output-format" ||
		preset.Args[3] != "json" {
		t.Fatalf("preset args = %+v", preset.Args)
	}
	settings := decodePresetSettings(t, preset)
	if settings.PromptDelivery != agents.PromptViaStdin ||
		settings.TimeoutSeconds != 1800 ||
		len(settings.VersionArgs) != 1 ||
		settings.VersionArgs[0] != "--version" ||
		settings.SmokePromptEnabled {
		t.Fatalf("settings = %+v", settings)
	}
	if !containsString(preset.EnvAllowlist, "GEMINI_API_KEY") ||
		!containsString(preset.EnvAllowlist, "GOOGLE_APPLICATION_CREDENTIALS") {
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
}

func TestListIncludesKnownPresets(t *testing.T) {
	t.Parallel()

	presets := List()
	ids := make(map[string]struct{}, len(presets))
	for _, preset := range presets {
		ids[preset.ID] = struct{}{}
	}
	for _, id := range []string{"codex-cli", "claude-code-cli", "gemini-cli", "custom-cli"} {
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
