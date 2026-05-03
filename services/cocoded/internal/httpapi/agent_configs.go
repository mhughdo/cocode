package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

type AgentConfigResponse struct {
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	Role           string                   `json:"role"`
	AdapterKind    agents.AdapterKind       `json:"adapter_kind"`
	Command        string                   `json:"command,omitempty"`
	Args           []string                 `json:"args"`
	CWDMode        string                   `json:"cwd_mode"`
	EnvAllowlist   []string                 `json:"env_allowlist"`
	OutputMode     agents.OutputMode        `json:"output_mode"`
	ModelLabel     string                   `json:"model_label,omitempty"`
	ReasoningLabel string                   `json:"reasoning_label,omitempty"`
	Capabilities   agents.AgentCapabilities `json:"capabilities"`
	Settings       json.RawMessage          `json:"settings"`
	Enabled        bool                     `json:"enabled"`
	CreatedAt      string                   `json:"created_at"`
	UpdatedAt      string                   `json:"updated_at"`
}

type CreateAgentConfigRequest struct {
	Name           string             `json:"name"`
	Role           string             `json:"role"`
	AdapterKind    agents.AdapterKind `json:"adapter_kind"`
	Command        string             `json:"command"`
	Args           []string           `json:"args"`
	CWDMode        string             `json:"cwd_mode"`
	EnvAllowlist   []string           `json:"env_allowlist"`
	OutputMode     agents.OutputMode  `json:"output_mode"`
	ModelLabel     string             `json:"model_label"`
	ReasoningLabel string             `json:"reasoning_label"`
	Capabilities   json.RawMessage    `json:"capabilities"`
	Settings       json.RawMessage    `json:"settings"`
	Enabled        *bool              `json:"enabled"`
}

type UpdateAgentConfigRequest struct {
	Name           *string             `json:"name"`
	Role           *string             `json:"role"`
	AdapterKind    *agents.AdapterKind `json:"adapter_kind"`
	Command        *string             `json:"command"`
	Args           []string            `json:"args"`
	CWDMode        *string             `json:"cwd_mode"`
	EnvAllowlist   []string            `json:"env_allowlist"`
	OutputMode     *agents.OutputMode  `json:"output_mode"`
	ModelLabel     *string             `json:"model_label"`
	ReasoningLabel *string             `json:"reasoning_label"`
	Capabilities   json.RawMessage     `json:"capabilities"`
	Settings       json.RawMessage     `json:"settings"`
	Enabled        *bool               `json:"enabled"`
}

type AgentConfigHealthResponse struct {
	AgentConfigID string                   `json:"agent_config_id"`
	Status        agents.HealthStatus      `json:"status"`
	Message       string                   `json:"message,omitempty"`
	Capabilities  agents.AgentCapabilities `json:"capabilities"`
	CheckedAt     string                   `json:"checked_at"`
	Metadata      map[string]any           `json:"metadata,omitempty"`
}

func listAgentConfigsHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := queries.ListAgentConfigs(c.Request.Context())
		if err != nil {
			respondError(c, apperror.Internal("failed to list agent configs"))
			return
		}
		response := make([]AgentConfigResponse, 0, len(rows))
		for _, row := range rows {
			item, appErr := agentConfigResponse(row)
			if appErr != nil {
				respondError(c, appErr)
				return
			}
			response = append(response, item)
		}
		respondOK(c, response)
	}
}

func createAgentConfigHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request CreateAgentConfigRequest
		if !bindJSON(c, &request) {
			return
		}
		params, appErr := createAgentConfigParams(request)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		row, err := queries.CreateAgentConfig(c.Request.Context(), params)
		if err != nil {
			respondAppError(c, err)
			return
		}
		response, appErr := agentConfigResponse(row)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, response)
	}
}

func updateAgentConfigHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			respondError(c, apperror.InvalidRequest("agent config id is required"))
			return
		}
		var request UpdateAgentConfigRequest
		if !bindJSON(c, &request) {
			return
		}
		existing, appErr := getAgentConfig(c.Request.Context(), queries, id)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		params, appErr := updateAgentConfigParams(existing, request)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		row, err := queries.UpdateAgentConfig(c.Request.Context(), params)
		if err != nil {
			respondAppError(c, err)
			return
		}
		response, appErr := agentConfigResponse(row)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, response)
	}
}

func testAgentConfigHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			respondError(c, apperror.InvalidRequest("agent config id is required"))
			return
		}
		row, appErr := getAgentConfig(c.Request.Context(), queries, id)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		capabilities, err := agents.DecodeCapabilitiesJSON(row.CapabilitiesJson, agents.AdapterKind(row.AdapterKind))
		if err != nil {
			respondError(c, apperror.Internal("stored agent capabilities are invalid"))
			return
		}

		kind := agents.AdapterKind(row.AdapterKind)
		health := agents.AgentHealth{
			Status:    agents.HealthUnknown,
			Message:   "runtime health checks are not implemented for this adapter kind",
			CheckedAt: time.Now().UTC(),
			Metadata:  map[string]any{},
		}
		if row.Enabled == 0 {
			health.Status = agents.HealthDegraded
			health.Message = "agent config is disabled"
		} else if kind == agents.AdapterCLINonInteractive {
			args, err := decodeStringArray(row.ArgsJson)
			if err != nil {
				respondError(c, apperror.Internal("stored agent args are invalid"))
				return
			}
			settings, appErr := decodeCommandHealthSettings(row.SettingsJson)
			if appErr != nil {
				respondError(c, appErr)
				return
			}
			health = agents.CheckCommandHealth(c.Request.Context(), agents.ConnectionConfig{
				AdapterID:      row.ID,
				Kind:           kind,
				Command:        nullableResponseString(row.Command),
				Args:           args,
				PromptDelivery: settings.PromptDelivery,
			}, settings)
		}

		respondOK(c, AgentConfigHealthResponse{
			AgentConfigID: row.ID,
			Status:        health.Status,
			Message:       health.Message,
			Capabilities:  capabilities,
			CheckedAt:     health.CheckedAt.Format(time.RFC3339Nano),
			Metadata:      health.Metadata,
		})
	}
}

func deleteAgentConfigHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			respondError(c, apperror.InvalidRequest("agent config id is required"))
			return
		}
		if _, appErr := getAgentConfig(c.Request.Context(), queries, id); appErr != nil {
			respondError(c, appErr)
			return
		}
		if err := queries.DeleteAgentConfig(c.Request.Context(), id); err != nil {
			respondAppError(c, err)
			return
		}
		respondOK(c, gin.H{"deleted": true})
	}
}

func createAgentConfigParams(request CreateAgentConfigRequest) (dbgen.CreateAgentConfigParams, *apperror.Error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	normalized, appErr := normalizeAgentConfigInput(agentConfigInput{
		Name:           request.Name,
		Role:           request.Role,
		AdapterKind:    request.AdapterKind,
		Command:        request.Command,
		Args:           request.Args,
		CWDMode:        request.CWDMode,
		EnvAllowlist:   request.EnvAllowlist,
		OutputMode:     request.OutputMode,
		ModelLabel:     request.ModelLabel,
		ReasoningLabel: request.ReasoningLabel,
		Capabilities:   request.Capabilities,
		Settings:       request.Settings,
		Enabled:        enabled,
	})
	if appErr != nil {
		return dbgen.CreateAgentConfigParams{}, appErr
	}
	return dbgen.CreateAgentConfigParams{
		ID:               "agent_config_" + newRequestID(),
		Name:             normalized.Name,
		Role:             normalized.Role,
		AdapterKind:      string(normalized.AdapterKind),
		Command:          nullableSQLString(normalized.Command),
		ArgsJson:         normalized.ArgsJSON,
		CwdMode:          normalized.CWDMode,
		EnvAllowlistJson: normalized.EnvAllowlistJSON,
		OutputMode:       string(normalized.OutputMode),
		ModelLabel:       nullableSQLString(normalized.ModelLabel),
		ReasoningLabel:   nullableSQLString(normalized.ReasoningLabel),
		CapabilitiesJson: normalized.CapabilitiesJSON,
		SettingsJson:     normalized.SettingsJSON,
		Enabled:          boolInt64(normalized.Enabled),
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func updateAgentConfigParams(existing dbgen.AgentConfig, request UpdateAgentConfigRequest) (dbgen.UpdateAgentConfigParams, *apperror.Error) {
	kind := agents.AdapterKind(existing.AdapterKind)
	if request.AdapterKind != nil && *request.AdapterKind != kind {
		return dbgen.UpdateAgentConfigParams{}, apperror.InvalidRequest("agent adapter kind cannot be changed")
	}
	input := agentConfigInput{
		Name:             existing.Name,
		Role:             existing.Role,
		AdapterKind:      kind,
		Command:          nullableResponseString(existing.Command),
		CWDMode:          existing.CwdMode,
		OutputMode:       agents.OutputMode(existing.OutputMode),
		ModelLabel:       nullableResponseString(existing.ModelLabel),
		ReasoningLabel:   nullableResponseString(existing.ReasoningLabel),
		CapabilitiesJSON: existing.CapabilitiesJson,
		SettingsJSON:     existing.SettingsJson,
		Enabled:          existing.Enabled != 0,
	}
	if request.Name != nil {
		input.Name = *request.Name
	}
	if request.Role != nil {
		input.Role = *request.Role
	}
	if request.Command != nil {
		input.Command = *request.Command
	}
	if request.Args != nil {
		input.Args = request.Args
	} else {
		input.ArgsJSON = existing.ArgsJson
	}
	if request.CWDMode != nil {
		input.CWDMode = *request.CWDMode
	}
	if request.EnvAllowlist != nil {
		input.EnvAllowlist = request.EnvAllowlist
	} else {
		input.EnvAllowlistJSON = existing.EnvAllowlistJson
	}
	if request.OutputMode != nil {
		input.OutputMode = *request.OutputMode
	}
	if request.ModelLabel != nil {
		input.ModelLabel = *request.ModelLabel
	}
	if request.ReasoningLabel != nil {
		input.ReasoningLabel = *request.ReasoningLabel
	}
	if request.Capabilities != nil {
		input.Capabilities = request.Capabilities
		input.CapabilitiesJSON = ""
	}
	if request.Settings != nil {
		input.Settings = request.Settings
		input.SettingsJSON = ""
	}
	if request.Enabled != nil {
		input.Enabled = *request.Enabled
	}

	normalized, appErr := normalizeAgentConfigInput(input)
	if appErr != nil {
		return dbgen.UpdateAgentConfigParams{}, appErr
	}
	return dbgen.UpdateAgentConfigParams{
		ID:               existing.ID,
		Name:             normalized.Name,
		Role:             normalized.Role,
		Command:          nullableSQLString(normalized.Command),
		ArgsJson:         normalized.ArgsJSON,
		CwdMode:          normalized.CWDMode,
		EnvAllowlistJson: normalized.EnvAllowlistJSON,
		OutputMode:       string(normalized.OutputMode),
		ModelLabel:       nullableSQLString(normalized.ModelLabel),
		ReasoningLabel:   nullableSQLString(normalized.ReasoningLabel),
		CapabilitiesJson: normalized.CapabilitiesJSON,
		SettingsJson:     normalized.SettingsJSON,
		Enabled:          boolInt64(normalized.Enabled),
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

type agentConfigInput struct {
	Name             string
	Role             string
	AdapterKind      agents.AdapterKind
	Command          string
	Args             []string
	ArgsJSON         string
	CWDMode          string
	EnvAllowlist     []string
	EnvAllowlistJSON string
	OutputMode       agents.OutputMode
	ModelLabel       string
	ReasoningLabel   string
	Capabilities     json.RawMessage
	CapabilitiesJSON string
	Settings         json.RawMessage
	SettingsJSON     string
	Enabled          bool
}

type normalizedAgentConfigInput struct {
	Name             string
	Role             string
	AdapterKind      agents.AdapterKind
	Command          string
	ArgsJSON         string
	CWDMode          string
	EnvAllowlistJSON string
	OutputMode       agents.OutputMode
	ModelLabel       string
	ReasoningLabel   string
	CapabilitiesJSON string
	SettingsJSON     string
	Enabled          bool
}

func normalizeAgentConfigInput(input agentConfigInput) (normalizedAgentConfigInput, *apperror.Error) {
	kind := input.AdapterKind
	if !kind.Valid() {
		return normalizedAgentConfigInput{}, apperror.InvalidRequest("agent adapter kind is invalid")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return normalizedAgentConfigInput{}, apperror.InvalidRequest("agent name is required")
	}
	role := strings.TrimSpace(input.Role)
	if role == "" {
		return normalizedAgentConfigInput{}, apperror.InvalidRequest("agent role is required")
	}
	command := strings.TrimSpace(input.Command)
	if kind == agents.AdapterCLINonInteractive && command == "" {
		return normalizedAgentConfigInput{}, apperror.InvalidRequest("command is required for CLI agents")
	}
	cwdMode := strings.TrimSpace(input.CWDMode)
	if cwdMode == "" {
		cwdMode = "repo_root"
	}
	outputMode := input.OutputMode
	if outputMode == "" {
		outputMode = agents.OutputText
	}
	if !outputMode.Valid() {
		return normalizedAgentConfigInput{}, apperror.InvalidRequest("agent output mode is invalid")
	}

	argsJSON, appErr := normalizeStringArrayJSON(input.Args, input.ArgsJSON, "args")
	if appErr != nil {
		return normalizedAgentConfigInput{}, appErr
	}
	envAllowlistJSON, appErr := normalizeEnvAllowlistJSON(input.EnvAllowlist, input.EnvAllowlistJSON)
	if appErr != nil {
		return normalizedAgentConfigInput{}, appErr
	}
	capabilitiesJSON, appErr := normalizeCapabilitiesJSON(input.Capabilities, input.CapabilitiesJSON, kind, outputMode)
	if appErr != nil {
		return normalizedAgentConfigInput{}, appErr
	}
	settingsJSON, appErr := normalizeObjectJSON(input.Settings, input.SettingsJSON, "settings")
	if appErr != nil {
		return normalizedAgentConfigInput{}, appErr
	}

	return normalizedAgentConfigInput{
		Name:             name,
		Role:             role,
		AdapterKind:      kind,
		Command:          command,
		ArgsJSON:         argsJSON,
		CWDMode:          cwdMode,
		EnvAllowlistJSON: envAllowlistJSON,
		OutputMode:       outputMode,
		ModelLabel:       strings.TrimSpace(input.ModelLabel),
		ReasoningLabel:   strings.TrimSpace(input.ReasoningLabel),
		CapabilitiesJSON: capabilitiesJSON,
		SettingsJSON:     settingsJSON,
		Enabled:          input.Enabled,
	}, nil
}

func normalizeStringArrayJSON(values []string, existingJSON string, field string) (string, *apperror.Error) {
	if existingJSON != "" && values == nil {
		if _, err := decodeStringArray(existingJSON); err != nil {
			return "", apperror.InvalidRequest(field + " must be a JSON string array")
		}
		return existingJSON, nil
	}
	if values == nil {
		values = []string{}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "", apperror.InvalidRequest(field + " must be a JSON string array")
	}
	return string(data), nil
}

func normalizeEnvAllowlistJSON(values []string, existingJSON string) (string, *apperror.Error) {
	if existingJSON != "" && values == nil {
		values, err := decodeStringArray(existingJSON)
		if err != nil {
			return "", apperror.InvalidRequest("env_allowlist must be a JSON string array")
		}
		return encodeEnvAllowlist(values)
	}
	return encodeEnvAllowlist(values)
}

func encodeEnvAllowlist(values []string) (string, *apperror.Error) {
	if values == nil {
		values = []string{}
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return "", apperror.InvalidRequest("env_allowlist cannot contain empty names")
		}
		if slices.Contains(normalized, value) {
			continue
		}
		normalized = append(normalized, value)
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", apperror.InvalidRequest("env_allowlist must be a JSON string array")
	}
	return string(data), nil
}

func normalizeCapabilitiesJSON(raw json.RawMessage, existingJSON string, kind agents.AdapterKind, outputMode agents.OutputMode) (string, *apperror.Error) {
	if len(raw) == 0 && existingJSON != "" {
		capabilities, err := agents.DecodeCapabilitiesJSON(existingJSON, kind)
		if err != nil {
			return "", apperror.InvalidRequest("agent capabilities are invalid")
		}
		if !capabilities.SupportsOutputMode(outputMode) {
			return "", apperror.InvalidRequest("agent output mode is not supported by capabilities")
		}
		encoded, err := capabilities.EncodeJSON()
		if err != nil {
			return "", apperror.InvalidRequest("agent capabilities are invalid")
		}
		return encoded, nil
	}
	capabilities, err := agents.DecodeCapabilitiesJSON(string(raw), kind)
	if err != nil {
		return "", apperror.InvalidRequest("agent capabilities are invalid")
	}
	if !capabilities.SupportsOutputMode(outputMode) {
		return "", apperror.InvalidRequest("agent output mode is not supported by capabilities")
	}
	encoded, err := capabilities.EncodeJSON()
	if err != nil {
		return "", apperror.InvalidRequest("agent capabilities are invalid")
	}
	return encoded, nil
}

func normalizeObjectJSON(raw json.RawMessage, existingJSON string, field string) (string, *apperror.Error) {
	if len(raw) == 0 && existingJSON != "" {
		raw = json.RawMessage(existingJSON)
	}
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return "{}", nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", apperror.InvalidRequest(field + " must be a JSON object")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", apperror.InvalidRequest(field + " must be a JSON object")
	}
	return string(data), nil
}

func decodeCommandHealthSettings(raw string) (agents.CommandHealthSettings, *apperror.Error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return agents.CommandHealthSettings{}, nil
	}
	var settings agents.CommandHealthSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return agents.CommandHealthSettings{}, apperror.Internal("stored agent settings are invalid")
	}
	return settings, nil
}

func agentConfigResponse(row dbgen.AgentConfig) (AgentConfigResponse, *apperror.Error) {
	capabilities, err := agents.DecodeCapabilitiesJSON(row.CapabilitiesJson, agents.AdapterKind(row.AdapterKind))
	if err != nil {
		return AgentConfigResponse{}, apperror.Internal("stored agent capabilities are invalid")
	}
	args, err := decodeStringArray(row.ArgsJson)
	if err != nil {
		return AgentConfigResponse{}, apperror.Internal("stored agent args are invalid")
	}
	envAllowlist, err := decodeStringArray(row.EnvAllowlistJson)
	if err != nil {
		return AgentConfigResponse{}, apperror.Internal("stored agent env allowlist is invalid")
	}
	outputMode := agents.OutputMode(row.OutputMode)
	if !outputMode.Valid() {
		return AgentConfigResponse{}, apperror.Internal("stored agent output mode is invalid")
	}
	settings := json.RawMessage(row.SettingsJson)
	if len(settings) == 0 || !json.Valid(settings) {
		settings = json.RawMessage("{}")
	}
	return AgentConfigResponse{
		ID:             row.ID,
		Name:           row.Name,
		Role:           row.Role,
		AdapterKind:    agents.AdapterKind(row.AdapterKind),
		Command:        nullableResponseString(row.Command),
		Args:           args,
		CWDMode:        row.CwdMode,
		EnvAllowlist:   envAllowlist,
		OutputMode:     outputMode,
		ModelLabel:     nullableResponseString(row.ModelLabel),
		ReasoningLabel: nullableResponseString(row.ReasoningLabel),
		Capabilities:   capabilities,
		Settings:       settings,
		Enabled:        row.Enabled != 0,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}, nil
}

func getAgentConfig(ctx context.Context, queries *dbgen.Queries, id string) (dbgen.AgentConfig, *apperror.Error) {
	row, err := queries.GetAgentConfig(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.AgentConfig{}, apperror.NotFound("agent config was not found")
		}
		return dbgen.AgentConfig{}, apperror.Internal("failed to read agent config")
	}
	return row, nil
}

func decodeStringArray(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []string{}
	}
	return values, nil
}

func nullableSQLString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func boolInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
