package agents

import (
	"strings"
	"testing"
)

func TestEnforceReviewModeRuntimeNormalizesCodexReadOnlyArgs(t *testing.T) {
	t.Parallel()

	config, err := EnforceReviewModeRuntime(ConnectionConfig{
		Kind:    AdapterCLINonInteractive,
		Command: "codex",
		Args: []string{
			"-a", "never", "exec", "--json", "--sandbox", "workspace-write",
			"--add-dir", "/tmp/cocode-agent-runtime", "--skip-git-repo-check", "-",
		},
	}, AgentCapabilities{CanRead: true})
	if err != nil {
		t.Fatalf("EnforceReviewModeRuntime() error = %v", err)
	}
	if reviewModeArgsContain(config.Args, "workspace-write", "--add-dir", "/tmp/cocode-agent-runtime") {
		t.Fatalf("unsafe codex args remain: %+v", config.Args)
	}
	if !containsAdjacentArgs(config.Args, "--sandbox", "read-only") {
		t.Fatalf("read-only sandbox missing from args: %+v", config.Args)
	}
}

func TestEnforceReviewModeRuntimeRejectsUnsafeReviewConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		config       ConnectionConfig
		capabilities AgentCapabilities
		want         string
	}{
		{
			name:         "write capable",
			config:       ConnectionConfig{Kind: AdapterCLINonInteractive, Command: "safe-agent"},
			capabilities: AgentCapabilities{CanRead: true, CanWrite: true},
			want:         "write-capable",
		},
		{
			name:         "risky command override",
			config:       ConnectionConfig{Kind: AdapterCLINonInteractive, Command: "safe-agent", CommandSafety: CommandSafetyOptions{AllowRiskyCommand: true}},
			capabilities: AgentCapabilities{CanRead: true},
			want:         "allow_risky_command",
		},
		{
			name:         "permission bypass flag",
			config:       ConnectionConfig{Kind: AdapterCLINonInteractive, Command: "agy", Args: []string{"--print", "--dangerously-skip-permissions"}},
			capabilities: AgentCapabilities{CanRead: true},
			want:         "dangerously-skip-permissions",
		},
		{
			name:         "non codex workspace write",
			config:       ConnectionConfig{Kind: AdapterCLINonInteractive, Command: "other-agent", Args: []string{"--sandbox", "workspace-write"}},
			capabilities: AgentCapabilities{CanRead: true},
			want:         "workspace-write",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := EnforceReviewModeRuntime(tt.config, tt.capabilities)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("EnforceReviewModeRuntime() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func containsAdjacentArgs(args []string, left string, right string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == left && args[index+1] == right {
			return true
		}
	}
	return false
}
