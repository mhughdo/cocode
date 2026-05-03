package agents

import (
	"strings"
	"testing"
)

func TestValidateCommandSafety(t *testing.T) {
	t.Parallel()

	safeCommand := writeFakeCommand(t, "#!/bin/sh\nexit 0\n")
	tests := []struct {
		name    string
		command string
		options CommandSafetyOptions
		wantErr bool
	}{
		{name: "safe executable path", command: safeCommand},
		{name: "safe command name", command: "codex"},
		{name: "inline args rejected", command: "codex --json", wantErr: true},
		{name: "shell metachar rejected", command: "codex; rm -rf /", wantErr: true},
		{name: "shell interpreter rejected by default", command: "sh", wantErr: true},
		{name: "destructive command rejected by default", command: "rm", wantErr: true},
		{name: "explicit risky shell allowed", command: "sh", options: CommandSafetyOptions{AllowRiskyCommand: true}},
		{name: "explicit risky destructive command allowed", command: "rm", options: CommandSafetyOptions{AllowRiskyCommand: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateCommandSafety(tt.command, tt.options)
			if tt.wantErr && err == nil {
				t.Fatal("ValidateCommandSafety() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateCommandSafety() error = %v", err)
			}
		})
	}
}

func TestNormalizeEnvAllowlist(t *testing.T) {
	t.Parallel()

	got, err := NormalizeEnvAllowlist([]string{" OPENAI_API_KEY ", "OPENAI_API_KEY", "_COCODE_TOKEN"})
	if err != nil {
		t.Fatalf("NormalizeEnvAllowlist() error = %v", err)
	}
	if len(got) != 2 || got[0] != "OPENAI_API_KEY" || got[1] != "_COCODE_TOKEN" {
		t.Fatalf("NormalizeEnvAllowlist() = %+v", got)
	}

	for _, values := range [][]string{
		{""},
		{"1BAD"},
		{"BAD-NAME"},
		{"BAD=NAME"},
	} {
		if _, err := NormalizeEnvAllowlist(values); err == nil || !strings.Contains(err.Error(), "environment variable") {
			t.Fatalf("NormalizeEnvAllowlist(%+v) error = %v, want env name error", values, err)
		}
	}
}
