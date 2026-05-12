package app

import "testing"

func TestLoadConfigUsesMultiAgentConcurrencyDefaults(t *testing.T) {
	t.Setenv("COCODED_AUTH_TOKEN", "test-token")
	t.Setenv("COCODED_AGENT_RUN_MAX_CONCURRENT", "")
	t.Setenv("COCODED_AGENT_RUN_MAX_CONCURRENT_PER_SESSION", "")

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.AgentRunMaxConcurrent < 4 {
		t.Fatalf("AgentRunMaxConcurrent = %d, want at least 4", config.AgentRunMaxConcurrent)
	}
	if config.AgentRunMaxConcurrentPerSession < 4 {
		t.Fatalf("AgentRunMaxConcurrentPerSession = %d, want at least 4", config.AgentRunMaxConcurrentPerSession)
	}
}

func TestLoadConfigReadsAgentConcurrencyOverrides(t *testing.T) {
	t.Setenv("COCODED_AUTH_TOKEN", "test-token")
	t.Setenv("COCODED_AGENT_RUN_MAX_CONCURRENT", "12")
	t.Setenv("COCODED_AGENT_RUN_MAX_CONCURRENT_PER_SESSION", "5")

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.AgentRunMaxConcurrent != 12 {
		t.Fatalf("AgentRunMaxConcurrent = %d, want 12", config.AgentRunMaxConcurrent)
	}
	if config.AgentRunMaxConcurrentPerSession != 5 {
		t.Fatalf("AgentRunMaxConcurrentPerSession = %d, want 5", config.AgentRunMaxConcurrentPerSession)
	}
}
