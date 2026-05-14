//go:build realcli

package agentoutput

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
)

func TestRealCLIReviewOutputsParseIntoTrustedCandidates(t *testing.T) {
	outputDir := os.Getenv("COCODE_REAL_CLI_OUTPUT_DIR")
	if outputDir == "" {
		t.Skip("set COCODE_REAL_CLI_OUTPUT_DIR to a directory containing real CLI smoke outputs")
	}

	tests := []struct {
		name string
		file string
		mode agents.OutputMode
	}{
		{name: "codex gpt-5.5 high", file: "codex-gpt-5.5-high.jsonl", mode: agents.OutputJSONL},
		{name: "claude sonnet 4.6 xhigh", file: "claude-sonnet-4.6-xhigh.jsonl", mode: agents.OutputJSONL},
		{name: "gemini 3.1 pro preview", file: "gemini-3.1-pro-preview.json", mode: agents.OutputJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(outputDir, tt.file))
			if err != nil {
				t.Fatalf("read real CLI output: %v", err)
			}
			parsed := Parse(raw, tt.mode)
			result := ExtractCandidates(parsed)
			if len(result.Diagnostics) != 0 {
				t.Fatalf("Diagnostics = %+v", result.Diagnostics)
			}
			if len(result.Candidates) == 0 {
				t.Fatalf("no candidates parsed from %s", tt.file)
			}

			hasServerFinding := false
			for _, candidate := range result.Candidates {
				if len(candidate.Locations) == 0 {
					t.Fatalf("candidate missing locations: %+v", candidate)
				}
				if len(candidate.Evidence) == 0 {
					t.Fatalf("candidate missing evidence: %+v", candidate)
				}
				if candidate.PrimaryPath == "src/server.js" && candidate.PrimaryStartLine > 0 {
					hasServerFinding = true
				}
				for _, item := range candidate.Evidence {
					if item.Kind == "counter_evidence" {
						t.Fatalf("real CLI output treated related context as counter-evidence: %+v", candidate)
					}
				}
			}
			if !hasServerFinding {
				t.Fatalf("expected at least one src/server.js finding, got %+v", result.Candidates)
			}
		})
	}
}
