package contextbundle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/security"
)

func (s Service) recipientVisibility(ctx context.Context, agentConfigID string) (agents.AgentVisibility, error) {
	agentConfigID = strings.TrimSpace(agentConfigID)
	if agentConfigID == "" {
		return agents.AgentVisibility{Provider: "cocode", Egress: agents.AgentEgressLocal}, nil
	}
	agentConfig, err := s.Queries.GetAgentConfig(ctx, agentConfigID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agents.AgentVisibility{}, ErrAgentConfigNotFound
		}
		return agents.AgentVisibility{}, fmt.Errorf("read agent config: %w", err)
	}
	capabilities, err := agents.DecodeCapabilitiesJSON(agentConfig.CapabilitiesJson, agents.AdapterKind(agentConfig.AdapterKind))
	if err != nil {
		return agents.AgentVisibility{}, fmt.Errorf("decode agent capabilities: %w", err)
	}
	return agents.VisibilityForConfig(agents.ConnectionConfig{
		AdapterID: agentConfig.ID,
		Kind:      agents.AdapterKind(agentConfig.AdapterKind),
	}, capabilities), nil
}

type VisibilityReport struct {
	Recipient         agents.AgentVisibility `json:"recipient"`
	SentItemCount     int                    `json:"sent_item_count"`
	SentItemByKind    map[ItemKind]int       `json:"sent_item_by_kind,omitempty"`
	LocalOnlyEnforced bool                   `json:"local_only_enforced"`
	LocalOnlyPaths    []string               `json:"local_only_paths,omitempty"`
	Omitted           []VisibilityOmission   `json:"omitted,omitempty"`
}

type VisibilityOmission struct {
	Path   string   `json:"path,omitempty"`
	ItemID string   `json:"item_id,omitempty"`
	Kind   ItemKind `json:"kind,omitempty"`
	Reason string   `json:"reason"`
}

type localOnlyMatcher struct {
	paths map[string]struct{}
}

func newLocalOnlyMatcher(paths []string) localOnlyMatcher {
	matcher := localOnlyMatcher{paths: make(map[string]struct{}, len(paths))}
	for _, path := range paths {
		clean, err := security.CleanRelativePath(path)
		if err != nil || clean == "." {
			continue
		}
		matcher.paths[clean] = struct{}{}
	}
	return matcher
}

func (m localOnlyMatcher) empty() bool {
	return len(m.paths) == 0
}

func (m localOnlyMatcher) match(path string) (string, bool) {
	clean, err := security.CleanRelativePath(path)
	if err != nil || clean == "." {
		return "", false
	}
	for {
		if _, ok := m.paths[clean]; ok {
			return clean, true
		}
		parent := parentRelativePath(clean)
		if parent == "" || parent == clean {
			return "", false
		}
		clean = parent
	}
}

func visibilityReport(visibility agents.AgentVisibility, policy ReviewContextPolicy, bundle Bundle, omitted []VisibilityOmission) VisibilityReport {
	counts := make(map[ItemKind]int, len(bundle.Items))
	for _, item := range bundle.Items {
		counts[item.Kind]++
	}
	return VisibilityReport{
		Recipient:         visibility,
		SentItemCount:     len(bundle.Items),
		SentItemByKind:    counts,
		LocalOnlyEnforced: visibility.IsExternal() && len(policy.LocalOnlyPaths) > 0,
		LocalOnlyPaths:    append([]string(nil), policy.LocalOnlyPaths...),
		Omitted:           append([]VisibilityOmission(nil), omitted...),
	}
}

func applyLocalOnlyChangedFileFilter(files []dbgen.ChangedFile, matcher localOnlyMatcher) ([]dbgen.ChangedFile, []VisibilityOmission) {
	if matcher.empty() {
		return files, nil
	}
	out := make([]dbgen.ChangedFile, 0, len(files))
	omitted := make([]VisibilityOmission, 0)
	seen := map[string]struct{}{}
	for _, file := range files {
		if matched, ok := changedFileLocalOnlyPath(file, matcher); ok {
			if _, exists := seen[matched]; !exists {
				seen[matched] = struct{}{}
				omitted = append(omitted, VisibilityOmission{
					Path:   matched,
					Reason: "local_only_path",
				})
			}
			continue
		}
		out = append(out, file)
	}
	return out, omitted
}

func applyLocalOnlyItemFilter(items []Item, matcher localOnlyMatcher) ([]Item, []VisibilityOmission) {
	if matcher.empty() {
		return items, nil
	}
	out := make([]Item, 0, len(items))
	omitted := make([]VisibilityOmission, 0)
	for _, item := range items {
		if path, ok := matcher.match(item.Path); ok {
			omitted = append(omitted, VisibilityOmission{
				Path:   path,
				ItemID: item.ID,
				Kind:   item.Kind,
				Reason: "local_only_path",
			})
			continue
		}
		out = append(out, item)
	}
	return out, omitted
}

func changedFileLocalOnlyPath(file dbgen.ChangedFile, matcher localOnlyMatcher) (string, bool) {
	if path, ok := matcher.match(file.Path); ok {
		return path, true
	}
	if file.OldPath.Valid {
		if path, ok := matcher.match(file.OldPath.String); ok {
			return path, true
		}
	}
	return "", false
}

func appendVisibilityWarning(warnings []string, report VisibilityReport) []string {
	if !report.LocalOnlyEnforced || len(report.Omitted) == 0 {
		return warnings
	}
	return appendWarning(warnings, fmt.Sprintf("local-only context omitted %d item(s) for external agent %s", len(report.Omitted), strings.TrimSpace(report.Recipient.AgentConfigID)))
}

func visibilityArtifactMetadata(base map[string]any, report VisibilityReport) map[string]any {
	if base == nil {
		base = map[string]any{}
	}
	base["visibility"] = reportMetadata(report)
	return base
}

func reportMetadata(report VisibilityReport) map[string]any {
	omitted := make([]map[string]any, 0, len(report.Omitted))
	for _, item := range report.Omitted {
		entry := map[string]any{
			"reason": item.Reason,
		}
		if item.Path != "" {
			entry["path"] = item.Path
		}
		if item.ItemID != "" {
			entry["item_id"] = item.ItemID
		}
		if item.Kind != "" {
			entry["kind"] = string(item.Kind)
		}
		omitted = append(omitted, entry)
	}
	counts := map[string]int{}
	for kind, count := range report.SentItemByKind {
		counts[string(kind)] = count
	}
	return map[string]any{
		"recipient":           report.Recipient.Metadata(),
		"sent_item_count":     report.SentItemCount,
		"sent_item_by_kind":   counts,
		"local_only_enforced": report.LocalOnlyEnforced,
		"local_only_paths":    append([]string(nil), report.LocalOnlyPaths...),
		"omitted":             omitted,
	}
}

func VisibilityReportFromArtifactMetadata(raw json.RawMessage) (VisibilityReport, bool) {
	var metadata struct {
		Visibility struct {
			Recipient struct {
				AgentConfigID string             `json:"agent_config_id"`
				AdapterKind   agents.AdapterKind `json:"adapter_kind"`
				Provider      string             `json:"provider"`
				Egress        agents.AgentEgress `json:"egress"`
			} `json:"recipient"`
			SentItemCount     int            `json:"sent_item_count"`
			SentItemByKind    map[string]int `json:"sent_item_by_kind"`
			LocalOnlyEnforced bool           `json:"local_only_enforced"`
			LocalOnlyPaths    []string       `json:"local_only_paths"`
			Omitted           []struct {
				Path   string `json:"path"`
				ItemID string `json:"item_id"`
				Kind   string `json:"kind"`
				Reason string `json:"reason"`
			} `json:"omitted"`
		} `json:"visibility"`
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return VisibilityReport{}, false
	}
	if metadata.Visibility.Recipient.Egress == "" && metadata.Visibility.SentItemCount == 0 && len(metadata.Visibility.Omitted) == 0 {
		return VisibilityReport{}, false
	}
	report := VisibilityReport{
		Recipient: agents.AgentVisibility{
			AgentConfigID: metadata.Visibility.Recipient.AgentConfigID,
			AdapterKind:   metadata.Visibility.Recipient.AdapterKind,
			Provider:      metadata.Visibility.Recipient.Provider,
			Egress:        metadata.Visibility.Recipient.Egress,
		},
		SentItemCount:     metadata.Visibility.SentItemCount,
		SentItemByKind:    map[ItemKind]int{},
		LocalOnlyEnforced: metadata.Visibility.LocalOnlyEnforced,
		LocalOnlyPaths:    append([]string(nil), metadata.Visibility.LocalOnlyPaths...),
	}
	for kind, count := range metadata.Visibility.SentItemByKind {
		report.SentItemByKind[ItemKind(kind)] = count
	}
	for _, item := range metadata.Visibility.Omitted {
		report.Omitted = append(report.Omitted, VisibilityOmission{
			Path:   item.Path,
			ItemID: item.ItemID,
			Kind:   ItemKind(item.Kind),
			Reason: item.Reason,
		})
	}
	return report, true
}

func parentRelativePath(path string) string {
	index := strings.LastIndex(path, "/")
	if index <= 0 {
		return ""
	}
	return path[:index]
}
