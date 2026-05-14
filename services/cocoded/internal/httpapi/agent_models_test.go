package httpapi

import "testing"

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
