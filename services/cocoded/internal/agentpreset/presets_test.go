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

func TestListIncludesKnownPresets(t *testing.T) {
	t.Parallel()

	presets := List()
	ids := make(map[string]struct{}, len(presets))
	for _, preset := range presets {
		ids[preset.ID] = struct{}{}
	}
	for _, id := range []string{"codex-cli", "claude-code-cli"} {
		if _, ok := ids[id]; !ok {
			t.Fatalf("preset %q missing from %+v", id, presets)
		}
	}
}

type presetSettings struct {
	PromptDelivery     agents.PromptDelivery `json:"prompt_delivery"`
	TimeoutSeconds     int64                 `json:"timeout_seconds"`
	VersionArgs        []string              `json:"version_args"`
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
