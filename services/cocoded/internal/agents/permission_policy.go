package agents

import (
	"fmt"
	"strings"
)

type PermissionAction string

const (
	PermissionRead    PermissionAction = "read"
	PermissionSearch  PermissionAction = "search"
	PermissionTest    PermissionAction = "test"
	PermissionShell   PermissionAction = "shell"
	PermissionWrite   PermissionAction = "write"
	PermissionPublish PermissionAction = "publish"
)

type PermissionRisk string

const (
	PermissionRiskLow      PermissionRisk = "low"
	PermissionRiskMedium   PermissionRisk = "medium"
	PermissionRiskHigh     PermissionRisk = "high"
	PermissionRiskCritical PermissionRisk = "critical"
)

type PermissionDecision string

const (
	PermissionApproved         PermissionDecision = "approved"
	PermissionDenied           PermissionDecision = "denied"
	PermissionRequiresApproval PermissionDecision = "requires_approval"
)

type PermissionMode string

const (
	PermissionModeUnrestricted PermissionMode = "unrestricted"
	PermissionModeReview       PermissionMode = "review"
)

type PermissionPolicy struct {
	Mode            PermissionMode     `json:"mode,omitempty"`
	ApprovedActions []PermissionAction `json:"approved_actions,omitempty"`
	DeniedActions   []PermissionAction `json:"denied_actions,omitempty"`
}

type PermissionResult struct {
	Action   PermissionAction   `json:"action"`
	Risk     PermissionRisk     `json:"risk"`
	Decision PermissionDecision `json:"decision"`
	Reason   string             `json:"reason,omitempty"`
}

type PermissionEvaluation struct {
	Mode    PermissionMode     `json:"mode"`
	Results []PermissionResult `json:"results"`
}

func ReviewModePermissionPolicy() PermissionPolicy {
	return PermissionPolicy{
		Mode:            PermissionModeReview,
		ApprovedActions: []PermissionAction{PermissionRead, PermissionSearch, PermissionTest, PermissionShell},
		DeniedActions:   []PermissionAction{PermissionWrite, PermissionPublish},
	}
}

func PermissionRiskForAction(action PermissionAction) PermissionRisk {
	switch action {
	case PermissionRead, PermissionSearch:
		return PermissionRiskLow
	case PermissionTest:
		return PermissionRiskMedium
	case PermissionShell, PermissionWrite:
		return PermissionRiskHigh
	case PermissionPublish:
		return PermissionRiskCritical
	default:
		return PermissionRiskHigh
	}
}

func RequiredPermissionsForRun(config ConnectionConfig, capabilities AgentCapabilities) []PermissionAction {
	if capabilities.empty() {
		capabilities = DefaultCapabilities(config.Kind)
	}
	actions := make([]PermissionAction, 0, 3)
	if capabilities.CanRead {
		actions = append(actions, PermissionRead)
	}
	if config.Kind == AdapterCLINonInteractive || config.Kind == AdapterJSONRPCStdio || config.Kind == AdapterACPStdio {
		actions = append(actions, PermissionShell)
	}
	if capabilities.CanWrite {
		actions = append(actions, PermissionWrite)
	}
	return normalizePermissionActions(actions)
}

func ValidateReviewModePermissions(config ConnectionConfig, capabilities AgentCapabilities) error {
	evaluation := ReviewModePermissionPolicy().Evaluate(RequiredPermissionsForRun(config, capabilities))
	if denied, ok := evaluation.FirstDenied(); ok {
		return fmt.Errorf("review mode denies %s permission (%s risk): %s", denied.Action, denied.Risk, denied.Reason)
	}
	return nil
}

func (p PermissionPolicy) Evaluate(actions []PermissionAction) PermissionEvaluation {
	p = p.normalize()
	actions = normalizePermissionActions(actions)
	results := make([]PermissionResult, 0, len(actions))
	for _, action := range actions {
		results = append(results, p.decide(action))
	}
	return PermissionEvaluation{Mode: p.Mode, Results: results}
}

func (e PermissionEvaluation) FirstDenied() (PermissionResult, bool) {
	for _, result := range e.Results {
		if result.Decision != PermissionApproved {
			return result, true
		}
	}
	return PermissionResult{}, false
}

func (e PermissionEvaluation) Metadata() map[string]any {
	results := make([]map[string]any, 0, len(e.Results))
	for _, result := range e.Results {
		item := map[string]any{
			"action":   string(result.Action),
			"risk":     string(result.Risk),
			"decision": string(result.Decision),
		}
		if strings.TrimSpace(result.Reason) != "" {
			item["reason"] = result.Reason
		}
		results = append(results, item)
	}
	return map[string]any{
		"mode":    string(e.Mode),
		"results": results,
	}
}

func (p PermissionPolicy) normalize() PermissionPolicy {
	if p.Mode == "" {
		if len(p.ApprovedActions) == 0 && len(p.DeniedActions) == 0 {
			p.Mode = PermissionModeUnrestricted
		} else {
			p.Mode = "custom"
		}
	}
	p.ApprovedActions = normalizePermissionActions(p.ApprovedActions)
	p.DeniedActions = normalizePermissionActions(p.DeniedActions)
	return p
}

func (p PermissionPolicy) decide(action PermissionAction) PermissionResult {
	result := PermissionResult{
		Action: action,
		Risk:   PermissionRiskForAction(action),
	}
	if containsPermissionAction(p.DeniedActions, action) {
		result.Decision = PermissionDenied
		result.Reason = fmt.Sprintf("%s mode denies %s permission", p.Mode, action)
		return result
	}
	if containsPermissionAction(p.ApprovedActions, action) || p.Mode == PermissionModeUnrestricted {
		result.Decision = PermissionApproved
		return result
	}
	result.Decision = PermissionRequiresApproval
	result.Reason = fmt.Sprintf("%s permission requires approval", action)
	return result
}

func normalizePermissionActions(actions []PermissionAction) []PermissionAction {
	normalized := make([]PermissionAction, 0, len(actions))
	seen := map[PermissionAction]struct{}{}
	for _, action := range actions {
		action = PermissionAction(strings.TrimSpace(string(action)))
		if action == "" {
			continue
		}
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		normalized = append(normalized, action)
	}
	return normalized
}

func containsPermissionAction(actions []PermissionAction, target PermissionAction) bool {
	for _, action := range actions {
		if action == target {
			return true
		}
	}
	return false
}

func (c AgentCapabilities) empty() bool {
	return !c.SupportsJSON &&
		!c.SupportsStreaming &&
		!c.SupportsSessions &&
		!c.CanRead &&
		!c.CanWrite &&
		!c.CanCancel &&
		len(c.OutputModes) == 0 &&
		len(c.Metadata) == 0
}
