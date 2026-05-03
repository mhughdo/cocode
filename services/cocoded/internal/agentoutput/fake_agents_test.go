package agentoutput

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type fixtureReviewOutput struct {
	Summary  string `json:"summary"`
	Findings []struct {
		Claim      string  `json:"claim"`
		Category   string  `json:"category"`
		Severity   string  `json:"severity"`
		Confidence float64 `json:"confidence"`
		Locations  []struct {
			Path      string `json:"path"`
			StartLine int64  `json:"start_line"`
			EndLine   int64  `json:"end_line"`
			Side      string `json:"side"`
		} `json:"locations"`
		Evidence []struct {
			Title     string `json:"title"`
			Summary   string `json:"summary"`
			Path      string `json:"path"`
			StartLine int64  `json:"start_line"`
			EndLine   int64  `json:"end_line"`
		} `json:"evidence"`
		SuggestedFix string `json:"suggested_fix"`
		DraftComment string `json:"draft_comment"`
	} `json:"findings"`
}

func TestFakeJSONAgentEmitsValidFindingJSON(t *testing.T) {
	t.Parallel()

	output := runFakeAgent(t, "json-agent.sh", "review this fixture")
	parsed := ParseAuto(output)
	if !parsed.Structured || len(parsed.Documents) != 1 {
		t.Fatalf("parsed = %+v", parsed)
	}

	var review fixtureReviewOutput
	if err := json.Unmarshal(parsed.Documents[0], &review); err != nil {
		t.Fatalf("Unmarshal(fake output) error = %v", err)
	}
	if !strings.Contains(review.Summary, "fixture") ||
		len(review.Findings) != 1 ||
		review.Findings[0].Category != "security" ||
		review.Findings[0].Severity != "high" ||
		review.Findings[0].Confidence <= 0 ||
		len(review.Findings[0].Locations) != 1 ||
		review.Findings[0].Locations[0].Path == "" ||
		len(review.Findings[0].Evidence) != 1 ||
		review.Findings[0].SuggestedFix == "" ||
		review.Findings[0].DraftComment == "" {
		t.Fatalf("review = %+v", review)
	}
}

func runFakeAgent(t *testing.T, name string, stdin string) []byte {
	t.Helper()

	path := fakeAgentPath(t, name)
	cmd := exec.Command(path)
	cmd.Stdin = strings.NewReader(stdin)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s error = %v", name, err)
	}
	return output
}

func fakeAgentPath(t *testing.T, name string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "testdata", "fake-agents", name)
}
