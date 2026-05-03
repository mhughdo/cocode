package agents

import "testing"

func TestVisibilityForConfigDefaultsAndMetadataOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		config       ConnectionConfig
		capabilities AgentCapabilities
		wantProvider string
		wantEgress   AgentEgress
	}{
		{
			name:         "cli defaults to external",
			config:       ConnectionConfig{AdapterID: "agent_config_cli", Kind: AdapterCLINonInteractive},
			capabilities: AgentCapabilities{},
			wantProvider: string(AdapterCLINonInteractive),
			wantEgress:   AgentEgressExternal,
		},
		{
			name:         "local verifier defaults to local",
			config:       ConnectionConfig{AdapterID: "agent_config_local", Kind: AdapterLocalVerifier},
			capabilities: AgentCapabilities{},
			wantProvider: string(AdapterLocalVerifier),
			wantEgress:   AgentEgressLocal,
		},
		{
			name:   "metadata overrides provider and egress",
			config: ConnectionConfig{AdapterID: "agent_config_openai", Kind: AdapterCLINonInteractive},
			capabilities: AgentCapabilities{
				Metadata: map[string]any{"provider": "openai", "egress": "local"},
			},
			wantProvider: "openai",
			wantEgress:   AgentEgressLocal,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := VisibilityForConfig(tt.config, tt.capabilities)
			if got.AgentConfigID != tt.config.AdapterID ||
				got.AdapterKind != tt.config.Kind ||
				got.Provider != tt.wantProvider ||
				got.Egress != tt.wantEgress {
				t.Fatalf("VisibilityForConfig() = %+v", got)
			}
		})
	}
}
