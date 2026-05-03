package agents

import "strings"

type AgentEgress string

const (
	AgentEgressLocal    AgentEgress = "local"
	AgentEgressExternal AgentEgress = "external"
)

type AgentVisibility struct {
	AgentConfigID string      `json:"agent_config_id,omitempty"`
	AdapterKind   AdapterKind `json:"adapter_kind,omitempty"`
	Provider      string      `json:"provider,omitempty"`
	Egress        AgentEgress `json:"egress"`
}

func VisibilityForConfig(config ConnectionConfig, capabilities AgentCapabilities) AgentVisibility {
	if capabilities.empty() {
		capabilities = DefaultCapabilities(config.Kind)
	}
	egress := AgentEgressExternal
	if config.Kind == AdapterLocalVerifier {
		egress = AgentEgressLocal
	}
	if value := metadataString(capabilities.Metadata, "egress"); value != "" {
		switch strings.ToLower(value) {
		case string(AgentEgressLocal):
			egress = AgentEgressLocal
		case string(AgentEgressExternal):
			egress = AgentEgressExternal
		}
	}
	provider := metadataString(capabilities.Metadata, "provider")
	if provider == "" {
		provider = string(config.Kind)
	}
	return AgentVisibility{
		AgentConfigID: strings.TrimSpace(config.AdapterID),
		AdapterKind:   config.Kind,
		Provider:      provider,
		Egress:        egress,
	}
}

func (v AgentVisibility) IsExternal() bool {
	return v.Egress == AgentEgressExternal
}

func (v AgentVisibility) Metadata() map[string]any {
	out := map[string]any{
		"egress": string(v.Egress),
	}
	if strings.TrimSpace(v.AgentConfigID) != "" {
		out["agent_config_id"] = strings.TrimSpace(v.AgentConfigID)
	}
	if strings.TrimSpace(string(v.AdapterKind)) != "" {
		out["adapter_kind"] = string(v.AdapterKind)
	}
	if strings.TrimSpace(v.Provider) != "" {
		out["provider"] = strings.TrimSpace(v.Provider)
	}
	return out
}

func metadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}
