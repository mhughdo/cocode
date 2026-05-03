package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/agentpreset"
	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const settingsExportSchema = "cocode.settings_export.v1"

type SettingsExportPayload struct {
	Schema            string                      `json:"schema"`
	ExportedAt        string                      `json:"exported_at"`
	Sections          []string                    `json:"sections"`
	Workspace         SettingsWorkspaceExport     `json:"workspace"`
	WorkspaceSettings map[string]any              `json:"workspace_settings"`
	AgentPresets      []AgentPresetResponse       `json:"agent_presets"`
	AgentConfigs      []SettingsAgentConfigExport `json:"agent_configs"`
	ReviewRules       []SettingsReviewRuleExport  `json:"review_rules"`
}

type SettingsWorkspaceExport struct {
	Name string `json:"name"`
}

type SettingsAgentConfigExport struct {
	Name           string             `json:"name"`
	Role           string             `json:"role"`
	AdapterKind    agents.AdapterKind `json:"adapter_kind"`
	Command        string             `json:"command"`
	Args           []string           `json:"args"`
	CWDMode        string             `json:"cwd_mode"`
	EnvAllowlist   []string           `json:"env_allowlist"`
	OutputMode     agents.OutputMode  `json:"output_mode"`
	ModelLabel     string             `json:"model_label,omitempty"`
	ReasoningLabel string             `json:"reasoning_label,omitempty"`
	Capabilities   map[string]any     `json:"capabilities"`
	Settings       map[string]any     `json:"settings"`
	Enabled        bool               `json:"enabled"`
}

type SettingsReviewRuleExport struct {
	Scope    string `json:"scope"`
	RuleType string `json:"rule_type"`
	Content  string `json:"content"`
	Enabled  bool   `json:"enabled"`
}

type SettingsImportRequest struct {
	Payload         SettingsExportPayload `json:"payload"`
	CollisionPolicy string                `json:"collision_policy"`
}

type SettingsImportResponse struct {
	Schema            string               `json:"schema"`
	ImportedAt        string               `json:"imported_at"`
	CollisionPolicy   string               `json:"collision_policy"`
	WorkspaceSettings SettingsImportReport `json:"workspace_settings"`
	AgentConfigs      SettingsImportReport `json:"agent_configs"`
	ReviewRules       SettingsImportReport `json:"review_rules"`
	Warnings          []string             `json:"warnings,omitempty"`
}

type SettingsImportReport struct {
	Created    int      `json:"created"`
	Updated    int      `json:"updated"`
	Skipped    int      `json:"skipped"`
	Collisions []string `json:"collisions,omitempty"`
	Redacted   int      `json:"redacted,omitempty"`
}

func exportWorkspaceSettingsHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		workspace, appErr := getWorkspace(c.Request.Context(), services.queries, c.Param("id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		payload, appErr := buildSettingsExport(c.Request.Context(), services.queries, workspace)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, payload)
	}
}

func importWorkspaceSettingsHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request SettingsImportRequest
		if !bindJSON(c, &request) {
			return
		}
		workspace, appErr := getWorkspace(c.Request.Context(), services.queries, c.Param("id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		if services.database == nil {
			respondError(c, apperror.Internal("database is not configured"))
			return
		}
		policy, appErr := normalizeSettingsCollisionPolicy(request.CollisionPolicy)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		if request.Payload.Schema != settingsExportSchema {
			respondError(c, apperror.InvalidRequest("settings export schema is invalid"))
			return
		}

		tx, err := services.database.BeginTx(c.Request.Context(), nil)
		if err != nil {
			respondError(c, apperror.Internal("failed to begin settings import"))
			return
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback()
			}
		}()
		txQueries := services.queries.WithTx(tx)
		response, appErr := importSettingsPayload(c.Request.Context(), txQueries, workspace, request.Payload, policy)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		if err := tx.Commit(); err != nil {
			respondError(c, apperror.Internal("failed to commit settings import"))
			return
		}
		committed = true
		respondOK(c, response)
	}
}

func buildSettingsExport(ctx context.Context, queries *dbgen.Queries, workspace dbgen.Workspace) (SettingsExportPayload, *apperror.Error) {
	workspaceSettings, redacted, appErr := sanitizedObjectJSON(workspace.SettingsJson)
	if appErr != nil {
		return SettingsExportPayload{}, appErr
	}
	if redacted > 0 {
		// The count is intentionally not surfaced in the export body; the absence of
		// sensitive keys is the portable contract.
		_ = redacted
	}

	agentConfigs, err := queries.ListAgentConfigs(ctx)
	if err != nil {
		return SettingsExportPayload{}, apperror.Internal("failed to list agent configs")
	}
	exportedConfigs := make([]SettingsAgentConfigExport, 0, len(agentConfigs))
	for _, row := range agentConfigs {
		exported, appErr := settingsAgentConfigExport(row)
		if appErr != nil {
			return SettingsExportPayload{}, appErr
		}
		exportedConfigs = append(exportedConfigs, exported)
	}

	rules, err := queries.ListReviewRulesByWorkspace(ctx, workspace.ID)
	if err != nil {
		return SettingsExportPayload{}, apperror.Internal("failed to list review rules")
	}
	exportedRules := make([]SettingsReviewRuleExport, 0, len(rules))
	for _, rule := range rules {
		exportedRules = append(exportedRules, SettingsReviewRuleExport{
			Scope:    rule.Scope,
			RuleType: rule.RuleType,
			Content:  rule.Content,
			Enabled:  rule.Enabled != 0,
		})
	}

	presets := agentpreset.List()
	exportedPresets := make([]AgentPresetResponse, 0, len(presets))
	for _, preset := range presets {
		item := agentPresetResponse(preset)
		settings, _, appErr := sanitizedObjectJSON(string(item.Settings))
		if appErr != nil {
			return SettingsExportPayload{}, appErr
		}
		item.Settings = mustJSONRaw(settings)
		exportedPresets = append(exportedPresets, item)
	}

	return SettingsExportPayload{
		Schema:     settingsExportSchema,
		ExportedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Sections: []string{
			"workspace_settings",
			"agent_presets",
			"agent_configs",
			"review_rules",
		},
		Workspace: SettingsWorkspaceExport{
			Name: workspace.Name,
		},
		WorkspaceSettings: workspaceSettings,
		AgentPresets:      exportedPresets,
		AgentConfigs:      exportedConfigs,
		ReviewRules:       exportedRules,
	}, nil
}

func settingsAgentConfigExport(row dbgen.AgentConfig) (SettingsAgentConfigExport, *apperror.Error) {
	args, err := decodeStringArray(row.ArgsJson)
	if err != nil {
		return SettingsAgentConfigExport{}, apperror.Internal("stored agent args are invalid")
	}
	envAllowlist, err := decodeStringArray(row.EnvAllowlistJson)
	if err != nil {
		return SettingsAgentConfigExport{}, apperror.Internal("stored agent env allowlist is invalid")
	}
	capabilities, _, appErr := sanitizedObjectJSON(row.CapabilitiesJson)
	if appErr != nil {
		return SettingsAgentConfigExport{}, appErr
	}
	settings, _, appErr := sanitizedObjectJSON(row.SettingsJson)
	if appErr != nil {
		return SettingsAgentConfigExport{}, appErr
	}
	return SettingsAgentConfigExport{
		Name:           row.Name,
		Role:           row.Role,
		AdapterKind:    agents.AdapterKind(row.AdapterKind),
		Command:        nullableResponseString(row.Command),
		Args:           args,
		CWDMode:        row.CwdMode,
		EnvAllowlist:   envAllowlist,
		OutputMode:     agents.OutputMode(row.OutputMode),
		ModelLabel:     nullableResponseString(row.ModelLabel),
		ReasoningLabel: nullableResponseString(row.ReasoningLabel),
		Capabilities:   capabilities,
		Settings:       settings,
		Enabled:        row.Enabled != 0,
	}, nil
}

func importSettingsPayload(ctx context.Context, queries *dbgen.Queries, workspace dbgen.Workspace, payload SettingsExportPayload, policy string) (SettingsImportResponse, *apperror.Error) {
	response := SettingsImportResponse{
		Schema:          settingsExportSchema,
		ImportedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		CollisionPolicy: policy,
	}

	workspaceReport, appErr := importWorkspaceSettings(ctx, queries, workspace, payload.WorkspaceSettings, policy)
	if appErr != nil {
		return SettingsImportResponse{}, appErr
	}
	response.WorkspaceSettings = workspaceReport

	agentReport, appErr := importAgentConfigs(ctx, queries, payload.AgentConfigs, policy)
	if appErr != nil {
		return SettingsImportResponse{}, appErr
	}
	response.AgentConfigs = agentReport

	ruleReport, warnings, appErr := importReviewRules(ctx, queries, workspace.ID, payload.ReviewRules, policy)
	if appErr != nil {
		return SettingsImportResponse{}, appErr
	}
	response.ReviewRules = ruleReport
	response.Warnings = warnings
	return response, nil
}

func importWorkspaceSettings(ctx context.Context, queries *dbgen.Queries, workspace dbgen.Workspace, imported map[string]any, policy string) (SettingsImportReport, *apperror.Error) {
	report := SettingsImportReport{}
	if len(imported) == 0 {
		return report, nil
	}
	existing, _, appErr := sanitizedObjectJSON(workspace.SettingsJson)
	if appErr != nil {
		return SettingsImportReport{}, appErr
	}
	safeImported, redacted := sanitizeObject(imported)
	report.Redacted += redacted

	next := cloneJSONObject(existing)
	keys := sortedMapKeys(safeImported)
	for _, key := range keys {
		if _, exists := next[key]; exists {
			report.Collisions = append(report.Collisions, "workspace_settings."+key)
			switch policy {
			case "skip":
				report.Skipped++
				continue
			case "fail":
				return SettingsImportReport{}, apperror.InvalidRequest("settings import collided on workspace setting " + key)
			case "rename":
				key = nextImportedKey(next, key)
				report.Created++
			case "replace":
				report.Updated++
			}
		} else {
			report.Created++
		}
		next[key] = safeImported[key]
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return SettingsImportReport{}, apperror.InvalidRequest("workspace settings must be a JSON object")
	}
	if _, err := queries.UpdateWorkspace(ctx, dbgen.UpdateWorkspaceParams{
		ID:            workspace.ID,
		Name:          workspace.Name,
		DefaultRepoID: workspace.DefaultRepoID,
		SettingsJson:  string(encoded),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return SettingsImportReport{}, apperror.Internal("failed to update workspace settings")
	}
	return report, nil
}

func importAgentConfigs(ctx context.Context, queries *dbgen.Queries, configs []SettingsAgentConfigExport, policy string) (SettingsImportReport, *apperror.Error) {
	report := SettingsImportReport{}
	if len(configs) == 0 {
		return report, nil
	}
	existingRows, err := queries.ListAgentConfigs(ctx)
	if err != nil {
		return SettingsImportReport{}, apperror.Internal("failed to list agent configs")
	}
	existingByKey := make(map[string]dbgen.AgentConfig, len(existingRows))
	existingNames := make(map[string]struct{}, len(existingRows))
	for _, row := range existingRows {
		existingByKey[agentConfigCollisionKey(row.Name, row.Role, agents.AdapterKind(row.AdapterKind))] = row
		existingNames[strings.ToLower(row.Name)] = struct{}{}
	}

	for _, item := range configs {
		settings, redacted := sanitizeObject(item.Settings)
		report.Redacted += redacted
		capabilities, capRedacted := sanitizeObject(item.Capabilities)
		report.Redacted += capRedacted
		settingsRaw := mustJSONRaw(settings)
		capabilitiesRaw := mustJSONRaw(capabilities)
		enabled := item.Enabled
		createRequest := CreateAgentConfigRequest{
			Name:           item.Name,
			Role:           item.Role,
			AdapterKind:    item.AdapterKind,
			Command:        item.Command,
			Args:           append([]string(nil), item.Args...),
			CWDMode:        item.CWDMode,
			EnvAllowlist:   append([]string(nil), item.EnvAllowlist...),
			OutputMode:     item.OutputMode,
			ModelLabel:     item.ModelLabel,
			ReasoningLabel: item.ReasoningLabel,
			Capabilities:   capabilitiesRaw,
			Settings:       settingsRaw,
			Enabled:        &enabled,
		}
		key := agentConfigCollisionKey(createRequest.Name, createRequest.Role, createRequest.AdapterKind)
		if existing, ok := existingByKey[key]; ok {
			report.Collisions = append(report.Collisions, "agent_configs."+createRequest.Name)
			switch policy {
			case "skip":
				report.Skipped++
				continue
			case "fail":
				return SettingsImportReport{}, apperror.InvalidRequest("settings import collided on agent config " + createRequest.Name)
			case "rename":
				createRequest.Name = uniqueImportedName(existingNames, createRequest.Name)
			case "replace":
				updateRequest := updateAgentConfigRequestFromExport(createRequest)
				params, appErr := updateAgentConfigParams(existing, updateRequest)
				if appErr != nil {
					return SettingsImportReport{}, appErr
				}
				updated, err := queries.UpdateAgentConfig(ctx, params)
				if err != nil {
					return SettingsImportReport{}, apperror.Internal("failed to update agent config")
				}
				existingByKey[key] = updated
				report.Updated++
				continue
			}
		}
		params, appErr := createAgentConfigParams(createRequest)
		if appErr != nil {
			return SettingsImportReport{}, appErr
		}
		created, err := queries.CreateAgentConfig(ctx, params)
		if err != nil {
			return SettingsImportReport{}, apperror.Internal("failed to create agent config")
		}
		existingByKey[agentConfigCollisionKey(created.Name, created.Role, agents.AdapterKind(created.AdapterKind))] = created
		existingNames[strings.ToLower(created.Name)] = struct{}{}
		report.Created++
	}
	return report, nil
}

func importReviewRules(ctx context.Context, queries *dbgen.Queries, workspaceID string, rules []SettingsReviewRuleExport, policy string) (SettingsImportReport, []string, *apperror.Error) {
	report := SettingsImportReport{}
	warnings := []string{}
	if len(rules) == 0 {
		return report, warnings, nil
	}
	existing, err := queries.ListReviewRulesByWorkspace(ctx, workspaceID)
	if err != nil {
		return SettingsImportReport{}, nil, apperror.Internal("failed to list review rules")
	}
	existingByKey := make(map[string]dbgen.ReviewRule, len(existing))
	for _, row := range existing {
		existingByKey[reviewRuleCollisionKey(row.Scope, row.RuleType, row.Content)] = row
	}
	for _, item := range rules {
		write, appErr := normalizeReviewRuleWrite(reviewRuleWrite{
			WorkspaceID: workspaceID,
			Scope:       item.Scope,
			RuleType:    item.RuleType,
			Content:     item.Content,
			Enabled:     item.Enabled,
		})
		if appErr != nil {
			return SettingsImportReport{}, nil, appErr
		}
		key := reviewRuleCollisionKey(write.Scope, write.RuleType, write.Content)
		if existingRule, ok := existingByKey[key]; ok {
			report.Collisions = append(report.Collisions, "review_rules."+write.Scope+"."+write.RuleType)
			switch policy {
			case "skip", "rename":
				if policy == "rename" {
					warnings = append(warnings, "review_rules collisions are deduped; rename was treated as skip")
				}
				report.Skipped++
				continue
			case "fail":
				return SettingsImportReport{}, nil, apperror.InvalidRequest("settings import collided on review rule")
			case "replace":
				updated, err := queries.UpdateReviewRule(ctx, dbgen.UpdateReviewRuleParams{
					ID:        existingRule.ID,
					Scope:     write.Scope,
					RuleType:  write.RuleType,
					Content:   write.Content,
					Enabled:   boolInt64(write.Enabled),
					UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
				})
				if err != nil {
					return SettingsImportReport{}, nil, apperror.Internal("failed to update review rule")
				}
				existingByKey[key] = updated
				report.Updated++
				continue
			}
		}
		created, appErr := createOrReuseReviewRule(ctx, queries, write)
		if appErr != nil {
			return SettingsImportReport{}, nil, appErr
		}
		existingByKey[key] = created
		report.Created++
	}
	return report, warnings, nil
}

func updateAgentConfigRequestFromExport(request CreateAgentConfigRequest) UpdateAgentConfigRequest {
	return UpdateAgentConfigRequest{
		Name:           stringPointer(request.Name),
		Role:           stringPointer(request.Role),
		AdapterKind:    adapterKindPointer(request.AdapterKind),
		Command:        stringPointer(request.Command),
		Args:           append([]string(nil), request.Args...),
		CWDMode:        stringPointer(request.CWDMode),
		EnvAllowlist:   append([]string(nil), request.EnvAllowlist...),
		OutputMode:     outputModePointer(request.OutputMode),
		ModelLabel:     stringPointer(request.ModelLabel),
		ReasoningLabel: stringPointer(request.ReasoningLabel),
		Capabilities:   append(json.RawMessage(nil), request.Capabilities...),
		Settings:       append(json.RawMessage(nil), request.Settings...),
		Enabled:        request.Enabled,
	}
}

func normalizeSettingsCollisionPolicy(value string) (string, *apperror.Error) {
	policy := strings.ToLower(strings.TrimSpace(value))
	if policy == "" {
		return "skip", nil
	}
	switch policy {
	case "skip", "replace", "rename", "fail":
		return policy, nil
	default:
		return "", apperror.InvalidRequest("settings import collision_policy is invalid")
	}
}

func sanitizedObjectJSON(raw string) (map[string]any, int, *apperror.Error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return map[string]any{}, 0, nil
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, 0, apperror.Internal("stored settings JSON is invalid")
	}
	safe, redacted := sanitizeObject(value)
	return safe, redacted, nil
}

func sanitizeObject(value map[string]any) (map[string]any, int) {
	safe := make(map[string]any, len(value))
	redacted := 0
	for _, key := range sortedMapKeys(value) {
		if isSensitiveSettingsKey(key) {
			redacted++
			continue
		}
		next, removed := sanitizeValue(value[key])
		redacted += removed
		if next != nil {
			safe[key] = next
		}
	}
	return safe, redacted
}

func sanitizeValue(value any) (any, int) {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeObject(typed)
	case []any:
		items := make([]any, 0, len(typed))
		redacted := 0
		for _, item := range typed {
			next, removed := sanitizeValue(item)
			redacted += removed
			if next != nil {
				items = append(items, next)
			}
		}
		return items, redacted
	default:
		return value, 0
	}
}

func isSensitiveSettingsKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, marker := range []string{"secret", "token", "password", "private_key", "api_key", "storage_key", "credential"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func mustJSONRaw(value map[string]any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(data)
}

func cloneJSONObject(value map[string]any) map[string]any {
	next := make(map[string]any, len(value))
	for key, item := range value {
		next[key] = item
	}
	return next
}

func sortedMapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func nextImportedKey(existing map[string]any, key string) string {
	base := key + "_imported"
	if _, ok := existing[base]; !ok {
		return base
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s_%d", base, index)
		if _, ok := existing[candidate]; !ok {
			return candidate
		}
	}
}

func uniqueImportedName(existing map[string]struct{}, name string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "Imported agent"
	}
	candidate := base + " (imported)"
	if _, ok := existing[strings.ToLower(candidate)]; !ok {
		existing[strings.ToLower(candidate)] = struct{}{}
		return candidate
	}
	for index := 2; ; index++ {
		candidate = fmt.Sprintf("%s (imported %d)", base, index)
		if _, ok := existing[strings.ToLower(candidate)]; !ok {
			existing[strings.ToLower(candidate)] = struct{}{}
			return candidate
		}
	}
}

func agentConfigCollisionKey(name string, role string, kind agents.AdapterKind) string {
	return strings.ToLower(strings.Join([]string{
		strings.TrimSpace(name),
		strings.TrimSpace(role),
		strings.TrimSpace(string(kind)),
	}, "\x00"))
}

func reviewRuleCollisionKey(scope string, ruleType string, content string) string {
	return strings.ToLower(strings.Join([]string{
		strings.TrimSpace(scope),
		strings.TrimSpace(ruleType),
		normalizedRuleContent(content),
	}, "\x00"))
}

func stringPointer(value string) *string {
	next := value
	return &next
}

func adapterKindPointer(value agents.AdapterKind) *agents.AdapterKind {
	next := value
	return &next
}

func outputModePointer(value agents.OutputMode) *agents.OutputMode {
	next := value
	return &next
}
