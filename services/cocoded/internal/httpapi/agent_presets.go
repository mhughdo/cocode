package httpapi

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/agentpreset"
	"github.com/hughdo/cocode/services/cocoded/internal/agents"
)

type AgentPresetResponse struct {
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

func listAgentPresetsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		presets := agentpreset.List()
		response := make([]AgentPresetResponse, 0, len(presets))
		for _, preset := range presets {
			response = append(response, agentPresetResponse(preset))
		}
		respondOK(c, response)
	}
}

func agentPresetResponse(preset agentpreset.Preset) AgentPresetResponse {
	return AgentPresetResponse{
		ID:             preset.ID,
		Name:           preset.Name,
		Description:    preset.Description,
		Role:           preset.Role,
		AdapterKind:    preset.AdapterKind,
		Command:        preset.Command,
		Args:           append([]string(nil), preset.Args...),
		CWDMode:        preset.CWDMode,
		EnvAllowlist:   append([]string(nil), preset.EnvAllowlist...),
		OutputMode:     preset.OutputMode,
		ModelLabel:     preset.ModelLabel,
		ReasoningLabel: preset.ReasoningLabel,
		Capabilities:   preset.Capabilities,
		Settings:       append(json.RawMessage(nil), preset.Settings...),
		Enabled:        preset.Enabled,
	}
}
