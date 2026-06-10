package reviewprompt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/hughdo/cocode/services/cocoded/internal/agentoutput"
)

const (
	DefaultTemplateSource = "services/cocoded/internal/reviewprompt/templates/review-agent.md"
	RoleOverlaySource     = "services/cocoded/internal/reviewprompt/templates/reviewer-roles.json"
	OutputSchemaVersion   = "review-agent-output/v1"
	PromptVersion         = "review-agent/v1"
)

type RenderInput struct {
	TemplateOverride string
	SessionID        string
	ReviewDepth      string
	Focus            string
	Role             string
	LocalScoutText   string
	ContextText      string
}

type RenderedPrompt struct {
	Text                string
	Version             string
	Hash                string
	TemplateSource      string
	TemplateHash        string
	RoleOverlayID       string
	RoleOverlayHash     string
	RoleOverlayFallback bool
	OutputSchema        string
	OutputSchemaHash    string
	PromptOverride      bool
	MaxFindings         int
}

type RoleOverlay struct {
	ID                   string   `json:"id"`
	Label                string   `json:"label"`
	ShortLabel           string   `json:"short_label"`
	Objective            string   `json:"objective"`
	FocusAreas           []string `json:"focus_areas"`
	Boundaries           []string `json:"boundaries"`
	DoNotFlag            []string `json:"do_not_flag"`
	EvidenceRequirements []string `json:"evidence_requirements"`
	AllowedCategories    []string `json:"allowed_categories"`
	MaxFindings          int      `json:"max_findings"`
}

var (
	roleOnce sync.Once
	roleMap  map[string]RoleOverlay
	roleErr  error
)

func DefaultTemplate() string {
	return strings.TrimSpace(defaultTemplate)
}

func RoleOverlays() ([]RoleOverlay, error) {
	roles, err := loadRoleMap()
	if err != nil {
		return nil, err
	}
	result := make([]RoleOverlay, 0, len(roles))
	for _, role := range roles {
		result = append(result, role)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func RenderReviewPrompt(input RenderInput) (RenderedPrompt, error) {
	template := DefaultTemplate()
	templateSource := DefaultTemplateSource
	promptOverride := false
	if override := strings.TrimSpace(input.TemplateOverride); override != "" {
		template = override
		templateSource = "service.prompt_template_override"
		promptOverride = true
	}
	role, fallback, err := ResolveRole(input.Role)
	if err != nil {
		return RenderedPrompt{}, err
	}
	maxFindings := role.MaxFindings
	if maxFindings <= 0 {
		maxFindings = 8
	}

	var builder strings.Builder
	builder.WriteString(renderTemplateWithUntrustedBoundary(template))
	builder.WriteString("\n\n")
	builder.WriteString(renderRoleOverlay(role, fallback))
	builder.WriteString("\n\n")
	builder.WriteString(renderOutputContract(maxFindings))
	builder.WriteString("\n\n")
	builder.WriteString(renderRuntimeRules())
	builder.WriteString("\n\n")
	builder.WriteString(renderSession(input))
	if scout := strings.TrimSpace(input.LocalScoutText); scout != "" {
		builder.WriteString("\n\n")
		builder.WriteString(scout)
	}
	if contextText := strings.TrimSpace(input.ContextText); contextText != "" {
		builder.WriteString("\n")
		builder.WriteString(contextText)
	}

	text := builder.String()
	roleHash := hashString(canonicalRoleJSON(role))
	return RenderedPrompt{
		Text:                text,
		Version:             PromptVersion,
		Hash:                hashString(text),
		TemplateSource:      templateSource,
		TemplateHash:        hashString(template),
		RoleOverlayID:       role.ID,
		RoleOverlayHash:     roleHash,
		RoleOverlayFallback: fallback,
		OutputSchema:        OutputSchemaVersion,
		OutputSchemaHash:    outputSchemaHash(),
		PromptOverride:      promptOverride,
		MaxFindings:         maxFindings,
	}, nil
}

func ResolveRole(roleID string) (RoleOverlay, bool, error) {
	roles, err := loadRoleMap()
	if err != nil {
		return RoleOverlay{}, false, err
	}
	normalized := normalizeRoleID(roleID)
	if role, ok := roles[normalized]; ok {
		return role, false, nil
	}
	if role, ok := roles["general-reviewer"]; ok {
		return role, normalized != "" && normalized != "general-reviewer", nil
	}
	return RoleOverlay{}, false, fmt.Errorf("review prompt roles are missing general-reviewer")
}

func (p RenderedPrompt) MetadataMap(promptArtifactID string) map[string]any {
	metadata := map[string]any{
		"prompt_version":         p.Version,
		"prompt_hash":            p.Hash,
		"prompt_template_source": p.TemplateSource,
		"prompt_template_hash":   p.TemplateHash,
		"role_overlay_id":        p.RoleOverlayID,
		"role_overlay_hash":      p.RoleOverlayHash,
		"role_overlay_fallback":  p.RoleOverlayFallback,
		"output_schema":          p.OutputSchema,
		"output_schema_hash":     p.OutputSchemaHash,
		"prompt_override":        p.PromptOverride,
		"max_findings":           p.MaxFindings,
	}
	if strings.TrimSpace(promptArtifactID) != "" {
		metadata["prompt_artifact_id"] = promptArtifactID
	}
	return metadata
}

func renderRoleOverlay(role RoleOverlay, fallback bool) string {
	var builder strings.Builder
	builder.WriteString("# Reviewer Role Overlay\n\n")
	builder.WriteString("Role ID: `")
	builder.WriteString(role.ID)
	builder.WriteString("`\n")
	builder.WriteString("Role name: ")
	builder.WriteString(role.Label)
	builder.WriteString("\n")
	if fallback {
		builder.WriteString("Role fallback: requested role was unknown, so use the general reviewer overlay.\n")
	}
	builder.WriteString("\nObjective: ")
	builder.WriteString(role.Objective)
	builder.WriteString("\n\nFocus areas:\n")
	writeBulletList(&builder, role.FocusAreas)
	builder.WriteString("\nBoundaries:\n")
	writeBulletList(&builder, role.Boundaries)
	builder.WriteString("\nDo not flag:\n")
	writeBulletList(&builder, role.DoNotFlag)
	builder.WriteString("\nEvidence requirements:\n")
	writeBulletList(&builder, role.EvidenceRequirements)
	if len(role.AllowedCategories) > 0 {
		builder.WriteString("\nPreferred categories for this role: ")
		builder.WriteString(inlineCodeList(role.AllowedCategories))
		builder.WriteString(". Use another valid category only when it is a better fit.\n")
	}
	return strings.TrimSpace(builder.String())
}

func renderOutputContract(maxFindings int) string {
	var builder strings.Builder
	builder.WriteString("# Output Contract\n\n")
	builder.WriteString("Output exactly one JSON object and nothing else. Do not wrap it in Markdown fences, logs, or commentary.\n")
	builder.WriteString("Return `{\"summary\":\"...\",\"findings\":[]}` when no concrete finding is anchored to a changed line.\n")
	builder.WriteString(fmt.Sprintf("Return at most %d findings. Prefer fewer, stronger findings over broad coverage.\n\n", maxFindings))
	builder.WriteString("Required top-level fields: `schema_version`, `summary`, `findings`.\n")
	builder.WriteString("Use `schema_version: \"review-agent-output/v1\"`.\n")
	builder.WriteString("Each finding must include `claim`, `category`, `severity`, `confidence`, `locations`, `evidence`, and `counter_evidence_request`. Include `suggested_fix` and `draft_comment` when useful.\n\n")
	builder.WriteString("Valid categories: ")
	builder.WriteString(inlineCodeList(agentoutput.KnownCategories()))
	builder.WriteString(".\n")
	builder.WriteString("Valid severities: ")
	builder.WriteString(inlineCodeList(agentoutput.KnownSeverities()))
	builder.WriteString(".\n")
	builder.WriteString("Valid location sides: ")
	builder.WriteString(inlineCodeList(agentoutput.KnownSides()))
	builder.WriteString(".\n")
	builder.WriteString("Valid evidence kinds: ")
	builder.WriteString(inlineCodeList(agentoutput.KnownEvidenceKinds()))
	builder.WriteString(".\n")
	builder.WriteString("Confidence is a number from 0.0 to 1.0: use 0.95+ only when code evidence is direct, 0.70-0.94 for strong but incomplete local support, and below 0.70 when the user must verify an assumption.\n\n")
	builder.WriteString("Severity rubric:\n")
	builder.WriteString("- `blocker`: likely data loss, security break, runtime panic, or deploy-stopping regression on a normal path.\n")
	builder.WriteString("- `high`: serious user-visible or production-impacting regression with clear reachability.\n")
	builder.WriteString("- `medium`: concrete defect or missed important behavior that is limited by scope, input, or rollout path.\n")
	builder.WriteString("- `low`: minor but real defect, missing targeted test for risky code, or edge-case behavior with low blast radius.\n")
	builder.WriteString("- `nit`: tiny issue only worth mentioning because it is directly changed and easy to fix.\n\n")
	builder.WriteString("Stop conditions:\n")
	builder.WriteString("- Do not emit a finding without a changed-code location.\n")
	builder.WriteString("- Do not emit a finding when related code or tests directly disprove the claim.\n")
	builder.WriteString("- Do not emit broad style, maintainability, or test coverage comments unless they are tied to a concrete failure.\n\n")
	builder.WriteString("Worked example:\n")
	builder.WriteString("```json\n")
	builder.WriteString(`{"schema_version":"review-agent-output/v1","summary":"One verified issue.","findings":[{"claim":"Handler dereferences req.User before authentication middleware can populate it on the new route","category":"security","severity":"high","confidence":0.91,"locations":[{"path":"internal/http/routes.go","start_line":42,"end_line":42,"side":"RIGHT"}],"evidence":[{"title":"New route bypasses auth middleware","summary":"The changed route registers the handler on the public router, while the handler assumes req.User is non-nil.","kind":"changed_code","path":"internal/http/routes.go","start_line":42,"end_line":42}],"counter_evidence_request":"A cited middleware registration that wraps this exact route before the handler runs would disprove the claim.","suggested_fix":"Register the route under the authenticated router or add an explicit guard in the handler."}]}`)
	builder.WriteString("\n```\n")
	return strings.TrimSpace(builder.String())
}

func renderRuntimeRules() string {
	var builder strings.Builder
	builder.WriteString("# Runtime Rules\n\n")
	builder.WriteString("- Review mode is read-only: do not edit, create, delete, move, or publish files.\n")
	builder.WriteString("- Report suggested fixes in the JSON output instead of applying them.\n")
	builder.WriteString("- When using Go tools such as `gopls`, resolve the executable through PATH first, for example with `command -v gopls`; do not hard-code stale GOPATH binaries.\n")
	builder.WriteString("- ")
	builder.WriteString(UntrustedContextInstruction())
	return builder.String()
}

func renderSession(input RenderInput) string {
	var builder strings.Builder
	builder.WriteString("# Session\n\n")
	builder.WriteString("Review session ID: ")
	builder.WriteString(strings.TrimSpace(input.SessionID))
	builder.WriteByte('\n')
	builder.WriteString("Review depth: ")
	builder.WriteString(strings.TrimSpace(input.ReviewDepth))
	builder.WriteByte('\n')
	if focus := strings.TrimSpace(input.Focus); focus != "" {
		builder.WriteString("Additional reviewer context:\n")
		builder.WriteString(focus)
		builder.WriteByte('\n')
	}
	return strings.TrimSpace(builder.String())
}

func loadRoleMap() (map[string]RoleOverlay, error) {
	roleOnce.Do(func() {
		var roles []RoleOverlay
		if err := json.Unmarshal([]byte(roleOverlaysJSON), &roles); err != nil {
			roleErr = fmt.Errorf("decode review role overlays: %w", err)
			return
		}
		roleMap = make(map[string]RoleOverlay, len(roles))
		for _, role := range roles {
			role.ID = normalizeRoleID(role.ID)
			if role.ID == "" {
				roleErr = fmt.Errorf("review role overlay has empty id")
				return
			}
			if _, exists := roleMap[role.ID]; exists {
				roleErr = fmt.Errorf("duplicate review role overlay id %q", role.ID)
				return
			}
			roleMap[role.ID] = role
		}
	})
	return roleMap, roleErr
}

func outputSchemaHash() string {
	var builder strings.Builder
	builder.WriteString(OutputSchemaVersion)
	builder.WriteString("\ncategory:")
	builder.WriteString(strings.Join(agentoutput.KnownCategories(), ","))
	builder.WriteString("\nseverity:")
	builder.WriteString(strings.Join(agentoutput.KnownSeverities(), ","))
	builder.WriteString("\nside:")
	builder.WriteString(strings.Join(agentoutput.KnownSides(), ","))
	builder.WriteString("\nevidence_kind:")
	builder.WriteString(strings.Join(agentoutput.KnownEvidenceKinds(), ","))
	return hashString(builder.String())
}

func canonicalRoleJSON(role RoleOverlay) string {
	data, err := json.Marshal(role)
	if err != nil {
		return role.ID
	}
	return string(data)
}

func writeBulletList(builder *strings.Builder, values []string) {
	if len(values) == 0 {
		builder.WriteString("- None.\n")
		return
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		builder.WriteString("- ")
		builder.WriteString(value)
		builder.WriteByte('\n')
	}
}

func inlineCodeList(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parts = append(parts, "`"+value+"`")
	}
	return strings.Join(parts, ", ")
}

func normalizeRoleID(roleID string) string {
	roleID = strings.TrimSpace(strings.ToLower(roleID))
	roleID = strings.ReplaceAll(roleID, "_", "-")
	return roleID
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
