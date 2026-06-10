package reviewprompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/agentoutput"
)

func TestEmbeddedPromptAssetsMatchPackageFiles(t *testing.T) {
	t.Parallel()

	assertEmbeddedAssetMatchesPackageFile(t, DefaultTemplate(), filepath.Join("packages", "prompts", "review-agent.md"))
	assertEmbeddedAssetMatchesPackageFile(t, strings.TrimSpace(roleOverlaysJSON), filepath.Join("packages", "prompts", "reviewer-roles.json"))
}

func TestRenderReviewPromptIncludesContractEnumsAndRoleOverlay(t *testing.T) {
	t.Parallel()

	rendered, err := RenderReviewPrompt(RenderInput{
		SessionID:      "review_session_1",
		ReviewDepth:    "standard",
		Role:           "security",
		LocalScoutText: "# Local Scout\n\nRisk: high\n",
		ContextText:    "# Context Bundle\n\nUNTRUSTED_CONTEXT_DATA\n",
	})
	if err != nil {
		t.Fatalf("RenderReviewPrompt() error = %v", err)
	}
	if rendered.RoleOverlayID != "security" ||
		rendered.RoleOverlayFallback ||
		rendered.Hash == "" ||
		rendered.TemplateHash == "" ||
		rendered.OutputSchemaHash == "" {
		t.Fatalf("rendered metadata = %+v", rendered)
	}
	for _, value := range append(append(agentoutput.KnownCategories(), agentoutput.KnownSeverities()...), agentoutput.KnownEvidenceKinds()...) {
		if !strings.Contains(rendered.Text, "`"+value+"`") {
			t.Fatalf("prompt missing enum value %q:\n%s", value, rendered.Text)
		}
	}
	for _, want := range []string{
		"Role ID: `security`",
		"Output exactly one JSON object and nothing else",
		"Severity rubric",
		"Return at most 5 findings",
		"UNTRUSTED_CONTEXT_DATA",
	} {
		if !strings.Contains(rendered.Text, want) {
			t.Fatalf("prompt missing %q:\n%s", want, rendered.Text)
		}
	}
}

func TestRenderReviewPromptHashIsStableAndRoleSensitive(t *testing.T) {
	t.Parallel()

	input := RenderInput{SessionID: "review_session_1", ReviewDepth: "standard", Role: "security", ContextText: "# Context"}
	first, err := RenderReviewPrompt(input)
	if err != nil {
		t.Fatalf("RenderReviewPrompt(first) error = %v", err)
	}
	second, err := RenderReviewPrompt(input)
	if err != nil {
		t.Fatalf("RenderReviewPrompt(second) error = %v", err)
	}
	if first.Hash != second.Hash || first.RoleOverlayHash != second.RoleOverlayHash {
		t.Fatalf("hashes are unstable: first=%+v second=%+v", first, second)
	}
	input.Role = "go-performance"
	performance, err := RenderReviewPrompt(input)
	if err != nil {
		t.Fatalf("RenderReviewPrompt(performance) error = %v", err)
	}
	if performance.Hash == first.Hash ||
		performance.RoleOverlayHash == first.RoleOverlayHash ||
		performance.RoleOverlayID != "go-performance" {
		t.Fatalf("role change did not affect prompt: security=%+v performance=%+v", first, performance)
	}
}

func TestRenderReviewPromptTracksTemplateOverride(t *testing.T) {
	t.Parallel()

	rendered, err := RenderReviewPrompt(RenderInput{
		TemplateOverride: "# Custom Review Contract\n\nUse this test-only template.",
		Role:             "general-reviewer",
		ContextText:      "# Context",
	})
	if err != nil {
		t.Fatalf("RenderReviewPrompt() error = %v", err)
	}
	if !rendered.PromptOverride ||
		rendered.TemplateSource != "service.prompt_template_override" ||
		rendered.TemplateHash == "" ||
		!strings.Contains(rendered.Text, "# Custom Review Contract") ||
		strings.Contains(rendered.Text, "# Core Task") {
		t.Fatalf("rendered override = %+v\n%s", rendered, rendered.Text)
	}
}

func TestRenderReviewPromptFallsBackUnknownRole(t *testing.T) {
	t.Parallel()

	rendered, err := RenderReviewPrompt(RenderInput{Role: "unknown-specialist", ContextText: "# Context"})
	if err != nil {
		t.Fatalf("RenderReviewPrompt() error = %v", err)
	}
	if rendered.RoleOverlayID != "general-reviewer" || !rendered.RoleOverlayFallback {
		t.Fatalf("rendered metadata = %+v", rendered)
	}
	if !strings.Contains(rendered.Text, "Role fallback: requested role was unknown") {
		t.Fatalf("prompt missing fallback note:\n%s", rendered.Text)
	}
}

func assertEmbeddedAssetMatchesPackageFile(t *testing.T, embedded string, packagePath string) {
	t.Helper()

	content, err := os.ReadFile(filepath.Join("..", "..", "..", "..", packagePath))
	if err != nil {
		t.Fatalf("read %s: %v", packagePath, err)
	}
	if strings.TrimSpace(string(content)) != strings.TrimSpace(embedded) {
		t.Fatalf("embedded asset does not match %s", packagePath)
	}
}
