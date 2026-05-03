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
	var settings struct {
		PromptDelivery     agents.PromptDelivery `json:"prompt_delivery"`
		TimeoutSeconds     int64                 `json:"timeout_seconds"`
		VersionArgs        []string              `json:"version_args"`
		SmokePromptEnabled bool                  `json:"smoke_prompt_enabled"`
	}
	if err := json.Unmarshal(preset.Settings, &settings); err != nil {
		t.Fatalf("Unmarshal(settings) error = %v", err)
	}
	if settings.PromptDelivery != agents.PromptViaStdin ||
		settings.TimeoutSeconds != 1800 ||
		len(settings.VersionArgs) != 1 ||
		settings.VersionArgs[0] != "--version" ||
		settings.SmokePromptEnabled {
		t.Fatalf("settings = %+v", settings)
	}
}
