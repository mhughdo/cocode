package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestKiroModelOptionsUsesCLIModelCatalog(t *testing.T) {
	t.Parallel()

	models := kiroModelOptions(`{
  "models": [
    {"model_name": "auto", "model_id": "auto"},
    {"model_name": "claude-sonnet-4.5", "model_id": "claude-sonnet-4.5"},
    {"model_name": "qwen3-coder-next", "model_id": "qwen3-coder-next"}
  ],
  "default_model": "auto"
}`)
	if len(models) != 3 {
		t.Fatalf("kiroModelOptions() length = %d, want 3: %+v", len(models), models)
	}
	if models[0].ID != "auto" ||
		models[0].Label != "Auto" ||
		models[0].Provider != "kiro" ||
		models[0].ProviderLabel != "Kiro" ||
		models[0].Source != "cli" ||
		!models[0].Default {
		t.Fatalf("auto model = %+v", models[0])
	}
	if models[1].ID != "claude-sonnet-4.5" || models[1].Label != "Claude Sonnet 4.5" || models[1].Default {
		t.Fatalf("sonnet model = %+v", models[1])
	}
}

func TestKiroModelOptionsFallsBackToAutoDefault(t *testing.T) {
	t.Parallel()

	models := kiroModelOptions(`{
  "models": [
    {"model_name": "auto", "model_id": "auto"},
    {"model_name": "glm-5", "model_id": "glm-5"}
  ],
  "default_model": ""
}`)
	if len(models) != 2 || !models[0].Default || models[1].Default {
		t.Fatalf("kiroModelOptions() = %+v, want auto as fallback default", models)
	}
}

func TestDiscoverAntigravityModelsUsesKnownGeminiFlashDefault(t *testing.T) {
	binDir := t.TempDir()
	agyPath := filepath.Join(binDir, "agy")
	if err := os.WriteFile(agyPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(agy) error = %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	catalog := discoverAntigravityModels(context.Background())
	if !catalog.Available ||
		catalog.Provider != "antigravity" ||
		catalog.ProviderLabel != "Antigravity" ||
		catalog.Command != "agy" ||
		catalog.Source != "cli-known" ||
		len(catalog.Models) != 1 {
		t.Fatalf("catalog = %+v", catalog)
	}
	model := catalog.Models[0]
	if model.ID != "gemini-3.5-flash" ||
		model.Label != "Gemini 3.5 Flash" ||
		model.Provider != "google" ||
		!model.Default ||
		len(model.ReasoningEfforts) != 3 ||
		!model.ReasoningEfforts[2].Default {
		t.Fatalf("model = %+v", model)
	}
}
