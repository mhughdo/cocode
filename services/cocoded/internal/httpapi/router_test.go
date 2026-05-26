package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/app"
	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/contextbundle"
	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	evidencepkg "github.com/hughdo/cocode/services/cocoded/internal/evidence"
	"github.com/hughdo/cocode/services/cocoded/internal/orchestrator"
)

func TestHealthEndpoint(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var envelope Envelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	data, ok := envelope.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected health data object, got %T", envelope.Data)
	}

	if data["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", data["status"])
	}
	if data["service"] != "cocoded" {
		t.Fatalf("expected service cocoded, got %v", data["service"])
	}
	if data["version"] != "test-version" {
		t.Fatalf("expected version test-version, got %v", data["version"])
	}
}

func TestVersionEndpoint(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
}

func TestAuthenticatedRouteRejectsMissingToken(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestAuthenticatedRouteAcceptsToken(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
}

func TestAuthenticatedRouteRejectsDisallowedOrigin(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Origin", "https://example.com")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, response.Code)
	}
}

func TestAuthenticatedRouteAcceptsLoopbackOrigin(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Origin", "http://localhost:5173")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
}

func TestAuthenticatedRouteAcceptsPackagedElectronFileOrigin(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Origin", "file://")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
}

func TestPreflightAcceptsLoopbackOriginWithoutToken(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodOptions, "/api/session", nil)
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	request.Header.Set("Access-Control-Request-Headers", "X-Cocode-Token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-Cocode-Token") {
		t.Fatalf("Access-Control-Allow-Headers = %q", got)
	}
}

func TestAgentConfigEndpointCRUDAndHealth(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	command := writeFakeAgentConfigCommand(t, `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf 'fake-agent 1.0.0\n'
  exit 0
fi
exit 0
`)

	createRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/agents/configs", map[string]any{
		"name":            "Codex reviewer",
		"role":            "reviewer",
		"adapter_kind":    "cli_noninteractive",
		"command":         command,
		"args":            []string{"exec"},
		"cwd_mode":        "repo_root",
		"env_allowlist":   []string{"OPENAI_API_KEY", "OPENAI_API_KEY"},
		"output_mode":     "json",
		"model_label":     "gpt-5.5",
		"reasoning_label": "high",
		"capabilities": map[string]any{
			"supports_json": true,
			"can_read":      true,
			"can_write":     false,
			"can_cancel":    true,
			"output_modes":  []string{"json", "text"},
		},
		"settings": map[string]any{
			"prompt_delivery":         "stdin",
			"timeout_seconds":         600,
			"version_timeout_seconds": 15,
		},
		"enabled": true,
	})
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	created := decodeAgentConfigResponse(t, createResponse.Body.Bytes())
	if !strings.HasPrefix(created.ID, "agent_config_") ||
		created.Name != "Codex reviewer" ||
		created.Command != command ||
		len(created.Args) != 1 ||
		created.Args[0] != "exec" ||
		len(created.EnvAllowlist) != 1 ||
		created.EnvAllowlist[0] != "OPENAI_API_KEY" ||
		created.OutputMode != agents.OutputJSON ||
		!created.Capabilities.SupportsOutputMode(agents.OutputJSON) ||
		created.Capabilities.CanWrite ||
		!created.Enabled {
		t.Fatalf("created agent config = %+v", created)
	}

	stored, err := queries.GetAgentConfig(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetAgentConfig() error = %v", err)
	}
	if stored.CapabilitiesJson == "" || stored.SettingsJson == "" {
		t.Fatalf("stored agent config = %+v", stored)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/agents/configs", nil)
	listRequest.Header.Set("X-Cocode-Token", "test-token")
	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	list := decodeAgentConfigListResponse(t, listResponse.Body.Bytes())
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v", list)
	}

	healthRequest := httptest.NewRequest(http.MethodPost, "/api/agents/configs/"+created.ID+"/test", nil)
	healthRequest.Header.Set("X-Cocode-Token", "test-token")
	healthResponse := httptest.NewRecorder()
	router.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", healthResponse.Code, healthResponse.Body.String())
	}
	health := decodeAgentConfigHealthResponse(t, healthResponse.Body.Bytes())
	if health.AgentConfigID != created.ID ||
		health.Status != agents.HealthAvailable ||
		health.Metadata["version"] != "fake-agent 1.0.0" ||
		!health.Capabilities.SupportsOutputMode(agents.OutputJSON) {
		t.Fatalf("health = %+v", health)
	}

	patchRequest := newAuthenticatedJSONRequest(t, http.MethodPatch, "/api/agents/configs/"+created.ID, map[string]any{
		"name":            "Codex reviewer deep",
		"command":         "codex",
		"args":            []string{"exec", "--json"},
		"env_allowlist":   []string{"OPENAI_API_KEY"},
		"output_mode":     "jsonl",
		"reasoning_label": "xhigh",
		"capabilities": map[string]any{
			"supports_json":      true,
			"supports_streaming": true,
			"can_read":           true,
			"can_write":          false,
			"can_cancel":         true,
			"output_modes":       []string{"jsonl", "json"},
		},
		"settings": map[string]any{
			"timeout_seconds": 900,
		},
		"enabled": false,
	})
	patchResponse := httptest.NewRecorder()
	router.ServeHTTP(patchResponse, patchRequest)
	if patchResponse.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", patchResponse.Code, patchResponse.Body.String())
	}
	updated := decodeAgentConfigResponse(t, patchResponse.Body.Bytes())
	if updated.Name != "Codex reviewer deep" ||
		updated.ReasoningLabel != "xhigh" ||
		updated.OutputMode != agents.OutputJSONL ||
		!updated.Capabilities.SupportsStreaming ||
		updated.Enabled {
		t.Fatalf("updated agent config = %+v", updated)
	}

	disabledHealthRequest := httptest.NewRequest(http.MethodPost, "/api/agents/configs/"+created.ID+"/test", nil)
	disabledHealthRequest.Header.Set("X-Cocode-Token", "test-token")
	disabledHealthResponse := httptest.NewRecorder()
	router.ServeHTTP(disabledHealthResponse, disabledHealthRequest)
	if disabledHealthResponse.Code != http.StatusOK {
		t.Fatalf("disabled health status = %d, body = %s", disabledHealthResponse.Code, disabledHealthResponse.Body.String())
	}
	disabledHealth := decodeAgentConfigHealthResponse(t, disabledHealthResponse.Body.Bytes())
	if disabledHealth.Status != agents.HealthDegraded {
		t.Fatalf("disabled health = %+v", disabledHealth)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/agents/configs/"+created.ID, nil)
	deleteRequest.Header.Set("X-Cocode-Token", "test-token")
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if _, err := queries.GetAgentConfig(context.Background(), created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetAgentConfig(deleted) error = %v, want sql.ErrNoRows", err)
	}
}

func TestAgentPresetsEndpointIncludesBuiltInCLIs(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/agents/presets", nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("presets status = %d, body = %s", response.Code, response.Body.String())
	}
	presets := decodeAgentPresetListResponse(t, response.Body.Bytes())
	if len(presets) == 0 {
		t.Fatal("presets empty")
	}
	codex := findAgentPreset(t, presets, "codex-cli")
	if codex.ID == "" ||
		codex.Command != "codex" ||
		len(codex.Args) != 14 ||
		codex.Args[0] != "-a" ||
		codex.Args[1] != "never" ||
		codex.Args[2] != "exec" ||
		codex.Args[3] != "--json" ||
		codex.Args[4] != "--sandbox" ||
		codex.Args[5] != "workspace-write" ||
		codex.Args[6] != "--add-dir" ||
		codex.Args[7] != agents.CLIRuntimeBaseDir() ||
		codex.Args[8] != "--skip-git-repo-check" ||
		codex.Args[9] != "--ephemeral" ||
		codex.Args[10] != "--ignore-rules" ||
		codex.Args[11] != "--color" ||
		codex.Args[12] != "never" ||
		codex.Args[13] != "-" ||
		codex.Role != "orchestrator" ||
		codex.OutputMode != agents.OutputJSONL ||
		codex.ModelLabel != "default" ||
		!codex.Capabilities.CanCancel ||
		!codex.Capabilities.SupportsOutputMode(agents.OutputJSONL) ||
		!json.Valid(codex.Settings) {
		t.Fatalf("codex preset = %+v", codex)
	}
	codexApp := findAgentPreset(t, presets, "codex-app-server")
	if codexApp.Command != "codex" ||
		len(codexApp.Args) != 3 ||
		codexApp.Args[0] != "app-server" ||
		codexApp.Args[1] != "--listen" ||
		codexApp.Args[2] != "stdio://" ||
		codexApp.AdapterKind != agents.AdapterJSONRPCStdio ||
		codexApp.OutputMode != agents.OutputJSON ||
		codexApp.ModelLabel != "default" ||
		!codexApp.Capabilities.SupportsStreaming ||
		!codexApp.Capabilities.SupportsSessions ||
		!codexApp.Capabilities.SupportsOutputMode(agents.OutputJSON) ||
		!json.Valid(codexApp.Settings) {
		t.Fatalf("codex app-server preset = %+v", codexApp)
	}
	claude := findAgentPreset(t, presets, "claude-code-cli")
	if claude.Command != "claude" ||
		len(claude.Args) != 9 ||
		claude.Args[0] != "-p" ||
		claude.Args[1] != agents.PromptArgPlaceholder ||
		claude.Args[2] != "--output-format" ||
		claude.Args[3] != "stream-json" ||
		claude.Args[4] != "--verbose" ||
		claude.Args[5] != "--include-partial-messages" ||
		claude.Args[6] != "--permission-mode" ||
		claude.Args[7] != "plan" ||
		claude.Args[8] != "--no-session-persistence" ||
		claude.OutputMode != agents.OutputJSONL ||
		claude.ModelLabel != "claude" ||
		!claude.Capabilities.SupportsStreaming ||
		!claude.Capabilities.SupportsOutputMode(agents.OutputJSONL) ||
		!json.Valid(claude.Settings) {
		t.Fatalf("claude preset = %+v", claude)
	}
	gemini := findAgentPreset(t, presets, "gemini-cli")
	if gemini.Command != "gemini" ||
		len(gemini.Args) != 7 ||
		gemini.Args[0] != "-p" ||
		gemini.Args[1] != agents.PromptArgPlaceholder ||
		gemini.Args[2] != "--output-format" ||
		gemini.Args[3] != "json" ||
		gemini.Args[4] != "--approval-mode" ||
		gemini.Args[5] != "default" ||
		gemini.Args[6] != "--skip-trust" ||
		gemini.OutputMode != agents.OutputJSON ||
		gemini.ModelLabel != "gemini-3.1-pro-preview" ||
		!gemini.Capabilities.SupportsOutputMode(agents.OutputJSON) ||
		!json.Valid(gemini.Settings) {
		t.Fatalf("gemini preset = %+v", gemini)
	}
	geminiACP := findAgentPreset(t, presets, "gemini-acp")
	if geminiACP.Command != "gemini" ||
		len(geminiACP.Args) != 1 ||
		geminiACP.Args[0] != "--acp" ||
		geminiACP.AdapterKind != agents.AdapterACPStdio ||
		geminiACP.OutputMode != agents.OutputJSON ||
		geminiACP.ModelLabel != "gemini-acp" ||
		!geminiACP.Capabilities.SupportsStreaming ||
		!geminiACP.Capabilities.SupportsSessions ||
		!geminiACP.Capabilities.SupportsOutputMode(agents.OutputJSON) ||
		!json.Valid(geminiACP.Settings) {
		t.Fatalf("gemini acp preset = %+v", geminiACP)
	}
	opencode := findAgentPreset(t, presets, "opencode-cli")
	if opencode.Command != "opencode" ||
		len(opencode.Args) != 6 ||
		opencode.Args[0] != "run" ||
		opencode.Args[1] != "--pure" ||
		opencode.Args[2] != "--format" ||
		opencode.Args[3] != "json" ||
		opencode.Args[4] != "--thinking" ||
		opencode.Args[5] != agents.PromptArgPlaceholder ||
		opencode.OutputMode != agents.OutputJSONL ||
		opencode.ModelLabel != "opencode-go/kimi-k2.6" ||
		opencode.ReasoningLabel != "high" ||
		!opencode.Capabilities.SupportsOutputMode(agents.OutputJSONL) ||
		!json.Valid(opencode.Settings) {
		t.Fatalf("opencode preset = %+v", opencode)
	}
	opencodeACP := findAgentPreset(t, presets, "opencode-acp")
	if opencodeACP.Command != "opencode" ||
		len(opencodeACP.Args) != 1 ||
		opencodeACP.Args[0] != "acp" ||
		opencodeACP.AdapterKind != agents.AdapterACPStdio ||
		opencodeACP.OutputMode != agents.OutputJSON ||
		opencodeACP.ModelLabel != "opencode-acp" ||
		!opencodeACP.Capabilities.SupportsStreaming ||
		!opencodeACP.Capabilities.SupportsSessions ||
		!opencodeACP.Capabilities.SupportsOutputMode(agents.OutputJSON) ||
		!json.Valid(opencodeACP.Settings) {
		t.Fatalf("opencode acp preset = %+v", opencodeACP)
	}
	antigravity := findAgentPreset(t, presets, "antigravity-cli")
	if antigravity.Command != "agy" ||
		len(antigravity.Args) != 5 ||
		antigravity.Args[0] != "--print" ||
		antigravity.Args[1] != "--sandbox" ||
		antigravity.Args[2] != "--dangerously-skip-permissions" ||
		antigravity.Args[3] != "--print-timeout" ||
		antigravity.Args[4] != "30m0s" ||
		antigravity.OutputMode != agents.OutputText ||
		antigravity.ModelLabel != "gemini-3.5-flash" ||
		antigravity.ReasoningLabel != "high" ||
		antigravity.Capabilities.SupportsJSON ||
		!antigravity.Capabilities.SupportsOutputMode(agents.OutputText) ||
		!json.Valid(antigravity.Settings) {
		t.Fatalf("antigravity preset = %+v", antigravity)
	}
	kiro := findAgentPreset(t, presets, "kiro-cli")
	if kiro.Command != "kiro-cli" ||
		len(kiro.Args) != 6 ||
		kiro.Args[0] != "chat" ||
		kiro.Args[1] != "--no-interactive" ||
		kiro.Args[2] != "--trust-tools=read,grep,glob,code" ||
		kiro.Args[3] != "--wrap" ||
		kiro.Args[4] != "never" ||
		kiro.Args[5] != agents.PromptArgPlaceholder ||
		kiro.OutputMode != agents.OutputText ||
		kiro.ModelLabel != "auto" ||
		kiro.Capabilities.SupportsJSON ||
		!kiro.Capabilities.SupportsOutputMode(agents.OutputText) ||
		!json.Valid(kiro.Settings) {
		t.Fatalf("kiro preset = %+v", kiro)
	}
	custom := findAgentPreset(t, presets, "custom-cli")
	if custom.Command != "" ||
		custom.Enabled ||
		custom.OutputMode != agents.OutputText ||
		!custom.Capabilities.SupportsOutputMode(agents.OutputNDJSON) ||
		!json.Valid(custom.Settings) {
		t.Fatalf("custom preset = %+v", custom)
	}
}

func TestCustomCLIConfigCanBeSavedAndHealthChecked(t *testing.T) {
	router, _ := testRouterWithQueries(t)
	command := writeFakeAgentConfigCommand(t, `#!/bin/sh
if [ "$1" = "--ping" ]; then
  printf 'custom ok\n'
  exit 0
fi
printf 'unexpected args: %s\n' "$*" >&2
exit 2
`)

	createRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/agents/configs", map[string]any{
		"name":          "Custom reviewer",
		"role":          "custom_reviewer",
		"adapter_kind":  "cli_noninteractive",
		"command":       command,
		"args":          []string{"--run", agents.PromptArgPlaceholder},
		"cwd_mode":      "repo_root",
		"output_mode":   "text",
		"model_label":   "custom",
		"env_allowlist": []string{"CUSTOM_AGENT_TOKEN"},
		"capabilities": map[string]any{
			"supports_json": false,
			"can_read":      true,
			"can_cancel":    true,
			"output_modes":  []string{"text"},
		},
		"settings": map[string]any{
			"prompt_delivery":         "arg",
			"version_args":            []string{"--ping"},
			"version_timeout_seconds": 15,
		},
	})
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	created := decodeAgentConfigResponse(t, createResponse.Body.Bytes())
	if created.Command != command ||
		len(created.Args) != 2 ||
		created.Args[1] != agents.PromptArgPlaceholder ||
		created.EnvAllowlist[0] != "CUSTOM_AGENT_TOKEN" ||
		created.OutputMode != agents.OutputText ||
		created.Capabilities.SupportsJSON {
		t.Fatalf("created custom config = %+v", created)
	}

	healthRequest := httptest.NewRequest(http.MethodPost, "/api/agents/configs/"+created.ID+"/test", nil)
	healthRequest.Header.Set("X-Cocode-Token", "test-token")
	healthResponse := httptest.NewRecorder()
	router.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", healthResponse.Code, healthResponse.Body.String())
	}
	health := decodeAgentConfigHealthResponse(t, healthResponse.Body.Bytes())
	if health.Status != agents.HealthAvailable ||
		health.Metadata["version"] != "custom ok" {
		t.Fatalf("health = %+v", health)
	}
}

func TestProtocolAgentConfigCanBeSavedAndHealthChecked(t *testing.T) {
	router, _ := testRouterWithQueries(t)
	command := writeFakeAgentConfigCommand(t, `#!/bin/sh
if [ "$1" = "app-server" ] && [ "$2" = "--help" ]; then
  printf 'codex app-server ok\n'
  exit 0
fi
printf 'unexpected args: %s\n' "$*" >&2
exit 2
`)

	createRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/agents/configs", map[string]any{
		"name":         "Codex App Server",
		"role":         "primary_reviewer",
		"adapter_kind": "jsonrpc_stdio",
		"command":      command,
		"args":         []string{"app-server", "--listen", "stdio://"},
		"cwd_mode":     "repo_root",
		"output_mode":  "json",
		"capabilities": map[string]any{
			"supports_json":      true,
			"supports_streaming": true,
			"supports_sessions":  true,
			"can_read":           true,
			"can_cancel":         true,
			"output_modes":       []string{"json", "jsonl"},
		},
		"settings": map[string]any{
			"version_args":            []string{"app-server", "--help"},
			"version_timeout_seconds": 15,
			"smoke_prompt_enabled":    false,
		},
	})
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	created := decodeAgentConfigResponse(t, createResponse.Body.Bytes())
	if created.AdapterKind != agents.AdapterJSONRPCStdio ||
		created.Command != command ||
		created.OutputMode != agents.OutputJSON ||
		!created.Capabilities.SupportsSessions {
		t.Fatalf("created protocol config = %+v", created)
	}

	healthRequest := httptest.NewRequest(http.MethodPost, "/api/agents/configs/"+created.ID+"/test", nil)
	healthRequest.Header.Set("X-Cocode-Token", "test-token")
	healthResponse := httptest.NewRecorder()
	router.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", healthResponse.Code, healthResponse.Body.String())
	}
	health := decodeAgentConfigHealthResponse(t, healthResponse.Body.Bytes())
	if health.Status != agents.HealthAvailable ||
		health.Metadata["version"] != "codex app-server ok" {
		t.Fatalf("health = %+v", health)
	}
}

func TestAgentConfigHealthUsesEnvAllowlist(t *testing.T) {
	t.Setenv("COCODE_HEALTH_TOKEN", "visible")
	t.Setenv("COCODE_PARENT_SECRET", "hidden")
	router, _ := testRouterWithQueries(t)
	command := writeFakeAgentConfigCommand(t, `#!/bin/sh
if [ "$1" = "--check-env" ]; then
  printf 'token=%s secret=%s\n' "${COCODE_HEALTH_TOKEN-unset}" "${COCODE_PARENT_SECRET-unset}"
  if [ "$COCODE_HEALTH_TOKEN" = "visible" ] && [ "${COCODE_PARENT_SECRET-unset}" = "unset" ]; then
    exit 0
  fi
  exit 7
fi
exit 2
`)

	createRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/agents/configs", map[string]any{
		"name":          "Env health reviewer",
		"role":          "reviewer",
		"adapter_kind":  "cli_noninteractive",
		"command":       command,
		"output_mode":   "text",
		"env_allowlist": []string{"COCODE_HEALTH_TOKEN"},
		"settings": map[string]any{
			"version_args":            []string{"--check-env"},
			"version_timeout_seconds": 15,
		},
	})
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	created := decodeAgentConfigResponse(t, createResponse.Body.Bytes())

	healthRequest := httptest.NewRequest(http.MethodPost, "/api/agents/configs/"+created.ID+"/test", nil)
	healthRequest.Header.Set("X-Cocode-Token", "test-token")
	healthResponse := httptest.NewRecorder()
	router.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", healthResponse.Code, healthResponse.Body.String())
	}
	health := decodeAgentConfigHealthResponse(t, healthResponse.Body.Bytes())
	if health.Status != agents.HealthAvailable ||
		health.Metadata["version"] != "token=visible secret=unset" {
		t.Fatalf("health = %+v", health)
	}
}

func TestAgentConfigEndpointAllowsExplicitRiskyCommand(t *testing.T) {
	router, _ := testRouterWithQueries(t)

	createRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/agents/configs", map[string]any{
		"name":         "Explicit shell reviewer",
		"role":         "reviewer",
		"adapter_kind": "cli_noninteractive",
		"command":      "sh",
		"output_mode":  "text",
		"settings": map[string]any{
			"allow_risky_command": true,
			"skip_version":        true,
		},
	})
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	created := decodeAgentConfigResponse(t, createResponse.Body.Bytes())
	if created.Command != "sh" || !bytes.Contains(created.Settings, []byte("allow_risky_command")) {
		t.Fatalf("created explicit risky config = %+v", created)
	}
}

func TestAgentConfigEndpointRejectsInvalidInputs(t *testing.T) {
	router, _ := testRouterWithQueries(t)

	tests := []struct {
		name string
		body map[string]any
	}{
		{
			name: "invalid adapter kind",
			body: map[string]any{
				"name":         "Shell reviewer",
				"role":         "reviewer",
				"adapter_kind": "shell",
				"output_mode":  "text",
			},
		},
		{
			name: "missing cli command",
			body: map[string]any{
				"name":         "Codex reviewer",
				"role":         "reviewer",
				"adapter_kind": "cli_noninteractive",
				"output_mode":  "text",
			},
		},
		{
			name: "missing protocol command",
			body: map[string]any{
				"name":         "ACP reviewer",
				"role":         "reviewer",
				"adapter_kind": "acp_stdio",
				"output_mode":  "json",
			},
		},
		{
			name: "invalid output mode",
			body: map[string]any{
				"name":         "Codex reviewer",
				"role":         "reviewer",
				"adapter_kind": "cli_noninteractive",
				"command":      "codex",
				"output_mode":  "yaml",
			},
		},
		{
			name: "command with inline args",
			body: map[string]any{
				"name":         "Codex reviewer",
				"role":         "reviewer",
				"adapter_kind": "cli_noninteractive",
				"command":      "codex --json",
				"output_mode":  "text",
			},
		},
		{
			name: "risky command without explicit setup",
			body: map[string]any{
				"name":         "Shell reviewer",
				"role":         "reviewer",
				"adapter_kind": "cli_noninteractive",
				"command":      "sh",
				"output_mode":  "text",
			},
		},
		{
			name: "invalid env allowlist name",
			body: map[string]any{
				"name":          "Codex reviewer",
				"role":          "reviewer",
				"adapter_kind":  "cli_noninteractive",
				"command":       "codex",
				"output_mode":   "text",
				"env_allowlist": []string{"1BAD"},
			},
		},
		{
			name: "invalid risky command setting",
			body: map[string]any{
				"name":         "Codex reviewer",
				"role":         "reviewer",
				"adapter_kind": "cli_noninteractive",
				"command":      "codex",
				"output_mode":  "text",
				"settings": map[string]any{
					"allow_risky_command": "yes",
				},
			},
		},
		{
			name: "capability mismatch",
			body: map[string]any{
				"name":         "Text reviewer",
				"role":         "reviewer",
				"adapter_kind": "cli_noninteractive",
				"command":      "reviewer",
				"output_mode":  "json",
				"capabilities": map[string]any{
					"supports_json": false,
				},
			},
		},
		{
			name: "settings must be object",
			body: map[string]any{
				"name":         "Codex reviewer",
				"role":         "reviewer",
				"adapter_kind": "cli_noninteractive",
				"command":      "codex",
				"output_mode":  "text",
				"settings":     []string{"bad"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/agents/configs", tt.body)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusBadRequest, response.Body.String())
			}
		})
	}
}

func TestAgentConfigHealthReportsMissingCommand(t *testing.T) {
	router, _ := testRouterWithQueries(t)

	createRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/agents/configs", map[string]any{
		"name":         "Missing reviewer",
		"role":         "reviewer",
		"adapter_kind": "cli_noninteractive",
		"command":      filepath.Join(t.TempDir(), "missing-agent"),
		"output_mode":  "text",
		"settings": map[string]any{
			"skip_version": true,
		},
	})
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	created := decodeAgentConfigResponse(t, createResponse.Body.Bytes())

	healthRequest := httptest.NewRequest(http.MethodPost, "/api/agents/configs/"+created.ID+"/test", nil)
	healthRequest.Header.Set("X-Cocode-Token", "test-token")
	healthResponse := httptest.NewRecorder()
	router.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", healthResponse.Code, healthResponse.Body.String())
	}
	health := decodeAgentConfigHealthResponse(t, healthResponse.Body.Bytes())
	if health.Status != agents.HealthUnavailable ||
		!strings.Contains(health.Message, "not installed") {
		t.Fatalf("health = %+v", health)
	}
}

func TestAgentConfigEndpointRejectsAdapterKindChangeAndMissingHealthConfig(t *testing.T) {
	router, _ := testRouterWithQueries(t)

	createRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/agents/configs", map[string]any{
		"name":         "Local verifier",
		"role":         "verifier",
		"adapter_kind": "local_verifier",
		"output_mode":  "json",
	})
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	created := decodeAgentConfigResponse(t, createResponse.Body.Bytes())

	patchRequest := newAuthenticatedJSONRequest(t, http.MethodPatch, "/api/agents/configs/"+created.ID, map[string]any{
		"adapter_kind": "cli_noninteractive",
	})
	patchResponse := httptest.NewRecorder()
	router.ServeHTTP(patchResponse, patchRequest)
	if patchResponse.Code != http.StatusBadRequest {
		t.Fatalf("patch status = %d, want %d, body = %s", patchResponse.Code, http.StatusBadRequest, patchResponse.Body.String())
	}

	healthRequest := httptest.NewRequest(http.MethodPost, "/api/agents/configs/missing/test", nil)
	healthRequest.Header.Set("X-Cocode-Token", "test-token")
	healthResponse := httptest.NewRecorder()
	router.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusNotFound {
		t.Fatalf("missing health status = %d, body = %s", healthResponse.Code, healthResponse.Body.String())
	}
}

func TestChangedFilesEndpointReturnsSnapshotFiles(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPISnapshot(t, queries)

	request := httptest.NewRequest(http.MethodGet, "/api/pr-snapshots/snapshot_1/changed-files", nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}

	var envelope struct {
		Data  []ChangedFileResponse `json:"data"`
		Error any                   `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	if len(envelope.Data) != 2 {
		t.Fatalf("changed files len = %d, want 2", len(envelope.Data))
	}
	generated := envelope.Data[0]
	if generated.Path != "generated/api.pb.go" ||
		!generated.IsGenerated ||
		!generated.IsExcluded ||
		generated.IsBinary ||
		string(generated.LineRanges) != `[[1,4]]` {
		t.Fatalf("generated response = %+v line ranges %s", generated, string(generated.LineRanges))
	}
	renamed := envelope.Data[1]
	if renamed.Path != "src/new.go" ||
		renamed.OldPath != "src/old.go" ||
		renamed.Status != "renamed" ||
		renamed.PatchArtifactID != "artifact_patch" ||
		renamed.IsBinary {
		t.Fatalf("renamed response = %+v", renamed)
	}
}

func TestChangedFilesEndpointRejectsMissingSnapshot(t *testing.T) {
	router, _ := testRouterWithQueries(t)

	request := httptest.NewRequest(http.MethodGet, "/api/pr-snapshots/missing/changed-files", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, response.Code, response.Body.String())
	}
}

func TestChangedFilePatchEndpointReturnsPatchArtifact(t *testing.T) {
	artifactDir := filepath.Join(t.TempDir(), "artifacts")
	router, queries := testRouterWithConfigAndQueries(t, app.Config{
		ArtifactDir: artifactDir,
	})
	createHTTPAPISnapshot(t, queries)
	store, err := artifact.New(artifactDir, queries)
	if err != nil {
		t.Fatalf("artifact.New() error = %v", err)
	}
	if _, err := store.Save(context.Background(), artifact.SaveParams{
		ID:           "artifact_patch",
		WorkspaceID:  "workspace_1",
		Kind:         "patch",
		RelativePath: "snapshots/snapshot_1/patches/src-new.patch",
		ContentType:  "text/x-diff",
		MetadataJSON: `{"path":"src/new.go"}`,
		CreatedAt:    "2026-05-03T00:05:00Z",
	}, []byte("diff --git a/src/old.go b/src/new.go\n@@ -1,2 +1,3 @@\n-old\n+new\n+added\n")); err != nil {
		t.Fatalf("Save(patch) error = %v", err)
	}

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/pr-snapshots/snapshot_1/changed-files/file_2/patch",
		nil,
	)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}

	var envelope struct {
		Data  ChangedFilePatchResponse `json:"data"`
		Error any                      `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	if envelope.Data.ChangedFileID != "file_2" ||
		envelope.Data.ArtifactID != "artifact_patch" ||
		envelope.Data.ContentType != "text/x-diff" ||
		!strings.Contains(envelope.Data.Content, "+added") ||
		envelope.Data.ContentTruncated {
		t.Fatalf("patch response = %+v", envelope.Data)
	}
}

func TestChangedFilePatchEndpointRejectsFileFromOtherSnapshot(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPISnapshot(t, queries)
	if _, err := queries.CreatePullRequestSnapshot(context.Background(), dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_2",
		RepositoryID: "repo_1",
		SourceType:   "branch_compare",
		MetadataJson: "{}",
		CreatedAt:    "2026-05-03T00:06:00Z",
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot(snapshot_2) error = %v", err)
	}

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/pr-snapshots/snapshot_2/changed-files/file_2/patch",
		nil,
	)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, response.Code, response.Body.String())
	}
}

func TestSnapshotEndpointReturnsSnapshot(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPISnapshot(t, queries)

	request := httptest.NewRequest(http.MethodGet, "/api/pr-snapshots/snapshot_1", nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	snapshot := decodeSnapshotResponse(t, response.Body.Bytes())
	if snapshot.ID != "snapshot_1" || snapshot.SourceType != "branch_compare" || snapshot.ChangedFileCount != 2 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestCreateGitHubSnapshotEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ghp_test" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.URL.Path == "/repos/openai/codex/pulls/123" && r.Header.Get("Accept") == "application/vnd.github+json":
			_, _ = w.Write([]byte(`{
				"title": "Add snapshot route",
				"html_url": "https://github.com/openai/codex/pull/123",
				"user": {"login": "octocat"},
				"base": {"ref": "main", "sha": "base-sha"},
				"head": {"ref": "feature/snapshot", "sha": "head-sha"}
			}`))
		case r.URL.Path == "/repos/openai/codex/pulls/123/files":
			_, _ = w.Write([]byte(`[{
				"sha": "file-sha",
				"filename": "api/routes.go",
				"status": "modified",
				"additions": 1,
				"deletions": 1,
				"changes": 2,
				"patch": "@@ -1 +1 @@\n-old\n+new\n"
			}]`))
		case r.URL.Path == "/repos/openai/codex/pulls/123" && r.Header.Get("Accept") == "application/vnd.github.diff":
			_, _ = w.Write([]byte("diff --git a/api/routes.go b/api/routes.go\n@@ -1 +1 @@\n-old\n+new\n"))
		case r.URL.Path == "/repos/openai/codex/issues/123/comments":
			_, _ = w.Write([]byte(`[{
				"id": 10,
				"body": "Please avoid duplicating the route helper.",
				"html_url": "https://github.com/openai/codex/pull/123#issuecomment-10",
				"created_at": "2026-05-03T10:00:00Z",
				"updated_at": "2026-05-03T10:00:00Z",
				"user": {"login": "reviewer-a"}
			}]`))
		case r.URL.Path == "/repos/openai/codex/pulls/123/comments":
			_, _ = w.Write([]byte(`[{
				"id": 20,
				"pull_request_review_id": 99,
				"body": "This branch already handles the edge case.",
				"html_url": "https://github.com/openai/codex/pull/123#discussion_r20",
				"path": "api/routes.go",
				"line": 12,
				"created_at": "2026-05-03T10:01:00Z",
				"updated_at": "2026-05-03T10:01:00Z",
				"user": {"login": "reviewer-b"}
			}]`))
		case r.URL.Path == "/repos/openai/codex/pulls/123/reviews":
			_, _ = w.Write([]byte(`[{
				"id": 99,
				"body": "One duplicate-avoidance note.",
				"state": "COMMENTED",
				"html_url": "https://github.com/openai/codex/pull/123#pullrequestreview-99",
				"commit_id": "head-sha",
				"submitted_at": "2026-05-03T10:02:00Z",
				"user": {"login": "reviewer-b"}
			}]`))
		default:
			t.Fatalf("unexpected GitHub request path=%s accept=%s", r.URL.Path, r.Header.Get("Accept"))
		}
	}))
	defer server.Close()

	router, queries := testRouterWithConfigAndQueries(t, app.Config{GitHubAPIBaseURL: server.URL})
	createHTTPAPIWorkspaceAndRepository(t, queries, "/tmp/cocode")

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/pr-snapshots/from-github-url", map[string]any{
		"workspace_id":  "workspace_1",
		"repository_id": "repo_1",
		"url":           "https://github.com/openai/codex/pull/123",
		"github_token":  "ghp_test",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	snapshot := decodeSnapshotResponse(t, response.Body.Bytes())
	if snapshot.SourceType != "github_pr" ||
		snapshot.Provider != "github" ||
		snapshot.Owner != "openai" ||
		snapshot.Repo != "codex" ||
		snapshot.PRNumber != 123 ||
		snapshot.ChangedFileCount != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if snapshot.PreviousCommentsArtifactID == "" {
		t.Fatalf("PreviousCommentsArtifactID is empty: %+v", snapshot)
	}
	commentsArtifact, err := queries.GetArtifact(context.Background(), snapshot.PreviousCommentsArtifactID)
	if err != nil {
		t.Fatalf("GetArtifact(previous comments) error = %v", err)
	}
	if commentsArtifact.Kind != "github_previous_comments" || commentsArtifact.ContentType != "application/json" {
		t.Fatalf("previous comments artifact = %+v", commentsArtifact)
	}
	var metadata struct {
		PreviousComments struct {
			ArtifactID         string `json:"artifact_id"`
			CommentCount       int    `json:"comment_count"`
			IssueCommentCount  int    `json:"issue_comment_count"`
			ReviewCommentCount int    `json:"review_comment_count"`
			ReviewCount        int    `json:"review_count"`
		} `json:"previous_comments"`
	}
	if err := json.Unmarshal(snapshot.Metadata, &metadata); err != nil {
		t.Fatalf("decode snapshot metadata: %v", err)
	}
	if metadata.PreviousComments.ArtifactID != snapshot.PreviousCommentsArtifactID ||
		metadata.PreviousComments.CommentCount != 3 ||
		metadata.PreviousComments.IssueCommentCount != 1 ||
		metadata.PreviousComments.ReviewCommentCount != 1 ||
		metadata.PreviousComments.ReviewCount != 1 {
		t.Fatalf("previous comments metadata = %+v", metadata.PreviousComments)
	}
	file, err := queries.GetChangedFileByPath(context.Background(), dbgen.GetChangedFileByPathParams{
		SnapshotID: snapshot.ID,
		Path:       "api/routes.go",
	})
	if err != nil {
		t.Fatalf("GetChangedFileByPath() error = %v", err)
	}
	if file.Additions != 1 || file.Deletions != 1 || file.PatchArtifactID.String == "" {
		t.Fatalf("changed file = %+v", file)
	}
}

func TestCreateGitHubSnapshotEndpointKeepsSnapshotWhenPreviousCommentsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ghp_test" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.URL.Path == "/repos/openai/codex/pulls/123" && r.Header.Get("Accept") == "application/vnd.github+json":
			_, _ = w.Write([]byte(`{
				"title": "Add snapshot route",
				"html_url": "https://github.com/openai/codex/pull/123",
				"user": {"login": "octocat"},
				"base": {"ref": "main", "sha": "base-sha"},
				"head": {"ref": "feature/snapshot", "sha": "head-sha"}
			}`))
		case r.URL.Path == "/repos/openai/codex/pulls/123/files":
			_, _ = w.Write([]byte(`[{
				"sha": "file-sha",
				"filename": "api/routes.go",
				"status": "modified",
				"additions": 1,
				"deletions": 1,
				"changes": 2,
				"patch": "@@ -1 +1 @@\n-old\n+new\n"
			}]`))
		case r.URL.Path == "/repos/openai/codex/pulls/123" && r.Header.Get("Accept") == "application/vnd.github.diff":
			_, _ = w.Write([]byte("diff --git a/api/routes.go b/api/routes.go\n@@ -1 +1 @@\n-old\n+new\n"))
		case r.URL.Path == "/repos/openai/codex/issues/123/comments":
			w.WriteHeader(http.StatusGone)
		default:
			t.Fatalf("unexpected GitHub request path=%s accept=%s", r.URL.Path, r.Header.Get("Accept"))
		}
	}))
	defer server.Close()

	router, queries := testRouterWithConfigAndQueries(t, app.Config{GitHubAPIBaseURL: server.URL})
	createHTTPAPIWorkspaceAndRepository(t, queries, "/tmp/cocode")

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/pr-snapshots/from-github-url", map[string]any{
		"workspace_id":  "workspace_1",
		"repository_id": "repo_1",
		"url":           "https://github.com/openai/codex/pull/123",
		"github_token":  "ghp_test",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	snapshot := decodeSnapshotResponse(t, response.Body.Bytes())
	if snapshot.PreviousCommentsArtifactID != "" || snapshot.ChangedFileCount != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	var metadata struct {
		PreviousCommentsFetchError string `json:"previous_comments_fetch_error"`
	}
	if err := json.Unmarshal(snapshot.Metadata, &metadata); err != nil {
		t.Fatalf("decode snapshot metadata: %v", err)
	}
	if metadata.PreviousCommentsFetchError == "" {
		t.Fatalf("previous comments fetch error is empty in metadata %s", string(snapshot.Metadata))
	}
	artifacts, err := queries.ListArtifactsByWorkspace(context.Background(), "workspace_1")
	if err != nil {
		t.Fatalf("ListArtifactsByWorkspace() error = %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("workspace artifacts len = %d, want diff and patch: %+v", len(artifacts), artifacts)
	}
}

func TestGitHubCredentialEndpointsStoreOnlyReference(t *testing.T) {
	const token = "ghp_secret_for_settings"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Fatalf("path = %q, want /user", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("X-OAuth-Scopes", "repo, read:user")
		_, _ = w.Write([]byte(`{"login":"octocat"}`))
	}))
	defer server.Close()

	router, queries := testRouterWithConfigAndQueries(t, app.Config{GitHubAPIBaseURL: server.URL})
	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/credentials/github", map[string]any{
		"display_name": "Work GitHub",
		"storage_key":  "github:default",
		"token":        token,
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", response.Code, response.Body.String())
	}
	saved := decodeGitHubCredentialStatusResponse(t, response.Body.Bytes())
	if !saved.Configured ||
		saved.Credential == nil ||
		saved.Credential.DisplayName != "Work GitHub" ||
		saved.Credential.StorageProvider != "electron_safe_storage" ||
		strings.Contains(string(saved.Credential.Metadata), token) {
		t.Fatalf("saved credential = %+v", saved)
	}

	ref, err := queries.GetLatestCredentialRefByKind(context.Background(), "github")
	if err != nil {
		t.Fatalf("GetLatestCredentialRefByKind() error = %v", err)
	}
	if strings.Contains(ref.DisplayName, token) ||
		strings.Contains(ref.MetadataJson, token) ||
		strings.Contains(ref.StorageKey, token) {
		t.Fatalf("credential ref leaked token: %+v", ref)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/credentials/github", nil)
	getRequest.Header.Set("X-Cocode-Token", "test-token")
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
	loaded := decodeGitHubCredentialStatusResponse(t, getResponse.Body.Bytes())
	if !loaded.Configured || loaded.Credential == nil || loaded.Credential.ID != saved.Credential.ID {
		t.Fatalf("loaded credential = %+v", loaded)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/credentials/github", nil)
	deleteRequest.Header.Set("X-Cocode-Token", "test-token")
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	deleted := decodeDeleteGitHubCredentialResponse(t, deleteResponse.Body.Bytes())
	if !deleted.Deleted || deleted.StorageKey != "github:default" {
		t.Fatalf("deleted = %+v", deleted)
	}
}

func TestGitHubCredentialEndpointRejectsInvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	router, queries := testRouterWithConfigAndQueries(t, app.Config{GitHubAPIBaseURL: server.URL})
	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/credentials/github", map[string]any{
		"storage_key": "github:default",
		"token":       "bad-token",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, err := queries.GetLatestCredentialRefByKind(context.Background(), "github"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("credential ref error = %v, want sql.ErrNoRows", err)
	}
}

func TestCreateLocalCompareSnapshotEndpoint(t *testing.T) {
	repoPath := initHTTPAPIGitRepo(t)
	runHTTPAPIGit(t, repoPath, "checkout", "-B", "main")
	writeHTTPAPIRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc main() {}\n")
	runHTTPAPIGit(t, repoPath, "add", ".")
	runHTTPAPIGit(t, repoPath, "commit", "-m", "initial")
	runHTTPAPIGit(t, repoPath, "checkout", "-b", "feature/api")
	writeHTTPAPIRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc main() {\n\tprintln(\"api\")\n}\n")
	runHTTPAPIGit(t, repoPath, "add", ".")
	runHTTPAPIGit(t, repoPath, "commit", "-m", "feature")

	router, queries := testRouterWithQueries(t)
	createHTTPAPIWorkspaceAndRepository(t, queries, repoPath)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/pr-snapshots/from-local-compare", map[string]any{
		"workspace_id":  "workspace_1",
		"repository_id": "repo_1",
		"base_ref":      "main",
		"head_ref":      "feature/api",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	snapshot := decodeSnapshotResponse(t, response.Body.Bytes())
	if snapshot.SourceType != "branch_compare" ||
		snapshot.BaseRef != "main" ||
		snapshot.HeadRef != "feature/api" ||
		snapshot.ChangedFileCount != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	files, err := queries.ListChangedFilesBySnapshot(context.Background(), snapshot.ID)
	if err != nil {
		t.Fatalf("ListChangedFilesBySnapshot() error = %v", err)
	}
	if len(files) != 1 || files[0].Path != "app/main.go" || files[0].LineRangesJson == "[]" {
		t.Fatalf("changed files = %+v", files)
	}
}

func TestCreateLocalChangesSnapshotEndpointIncludesUntrackedBinary(t *testing.T) {
	repoPath := initHTTPAPIGitRepo(t)
	runHTTPAPIGit(t, repoPath, "checkout", "-B", "main")
	writeHTTPAPIRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc main() {}\n")
	runHTTPAPIGit(t, repoPath, "add", ".")
	runHTTPAPIGit(t, repoPath, "commit", "-m", "initial")
	writeHTTPAPIRepoFile(t, repoPath, "app/main.go", "package main\n\nfunc main() {\n\tprintln(\"local\")\n}\n")
	writeHTTPAPIRepoBytes(t, repoPath, "assets/logo.bin", []byte{0x00, 0x01})

	router, queries := testRouterWithQueries(t)
	createHTTPAPIWorkspaceAndRepository(t, queries, repoPath)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/pr-snapshots/from-local-changes", map[string]any{
		"workspace_id":  "workspace_1",
		"repository_id": "repo_1",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	snapshot := decodeSnapshotResponse(t, response.Body.Bytes())
	if snapshot.SourceType != "local_changes" || snapshot.ChangedFileCount != 2 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	binary, err := queries.GetChangedFileByPath(context.Background(), dbgen.GetChangedFileByPathParams{
		SnapshotID: snapshot.ID,
		Path:       "assets/logo.bin",
	})
	if err != nil {
		t.Fatalf("GetChangedFileByPath(binary) error = %v", err)
	}
	if binary.IsBinary != 1 || binary.IsExcluded != 1 {
		t.Fatalf("binary changed file = %+v", binary)
	}
}

func TestReviewSessionEndpointCreateGetList(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPISnapshot(t, queries)
	createHTTPAPIAgentConfig(t, queries, "agent_config_codex", "reviewer", 1)
	createHTTPAPIAgentConfig(t, queries, "agent_config_verifier", "verifier", 1)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions", map[string]any{
		"workspace_id":     "workspace_1",
		"snapshot_id":      "snapshot_1",
		"title":            "Review auth changes",
		"review_depth":     "deep",
		"preset":           "security_sensitive",
		"focus_prompt":     "Focus auth guard behavior.",
		"agent_config_ids": []string{"agent_config_codex", "agent_config_codex", "agent_config_verifier"},
		"agent_selections": []map[string]any{
			{
				"agent_config_id": "agent_config_codex",
				"role":            "Security Reviewer",
				"model_label":     "gpt-5.5",
				"reasoning_label": "high",
			},
			{
				"agent_config_id": "agent_config_codex",
				"role":            "Performance Reviewer",
				"model_label":     "gpt-5.4",
				"reasoning_label": "medium",
			},
			{
				"agent_config_id": "agent_config_verifier",
				"role":            "Evidence Verifier",
			},
		},
		"runtime_limit_seconds": 900,
		"context_policy": map[string]any{
			"include_related_tests": false,
			"max_tokens":            4096,
		},
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	created := decodeReviewSessionResponse(t, response.Body.Bytes())
	if !strings.HasPrefix(created.ID, "review_session_") ||
		created.Status != "draft" ||
		created.Title != "Review auth changes" ||
		created.ReviewDepth != "deep" ||
		created.Preset != "security_sensitive" ||
		created.FocusPrompt != "Focus auth guard behavior." ||
		created.RuntimeLimitSeconds != 900 ||
		len(created.Agents) != 3 ||
		created.Agents[0].AgentConfigID != "agent_config_codex" ||
		created.Agents[0].Role != "Security Reviewer" ||
		created.Agents[0].RunOrder != 1 ||
		created.Agents[1].AgentConfigID != "agent_config_codex" ||
		created.Agents[1].Role != "Performance Reviewer" ||
		created.Agents[1].RunOrder != 2 ||
		created.Agents[2].AgentConfigID != "agent_config_verifier" ||
		created.Agents[2].Role != "Evidence Verifier" {
		t.Fatalf("created session = %+v", created)
	}
	var override reviewSessionAgentSettingsOverride
	if err := json.Unmarshal(created.Agents[0].SettingsOverride, &override); err != nil {
		t.Fatalf("decode agent settings override: %v", err)
	}
	if override.ModelLabel != "gpt-5.5" || override.ReasoningLabel != "high" {
		t.Fatalf("agent settings override = %+v", override)
	}
	if err := json.Unmarshal(created.Agents[1].SettingsOverride, &override); err != nil {
		t.Fatalf("decode duplicate agent settings override: %v", err)
	}
	if override.ModelLabel != "gpt-5.4" || override.ReasoningLabel != "medium" {
		t.Fatalf("duplicate agent settings override = %+v", override)
	}
	var policy struct {
		IncludeRelatedTests bool  `json:"include_related_tests"`
		MaxTokens           int64 `json:"max_tokens"`
	}
	if err := json.Unmarshal(created.ContextPolicy, &policy); err != nil {
		t.Fatalf("decode context policy: %v", err)
	}
	if policy.IncludeRelatedTests || policy.MaxTokens != 4096 {
		t.Fatalf("context policy = %s", string(created.ContextPolicy))
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions/"+created.ID, nil)
	getRequest.Header.Set("X-Cocode-Token", "test-token")
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
	got := decodeReviewSessionResponse(t, getResponse.Body.Bytes())
	if got.ID != created.ID || len(got.Agents) != 3 {
		t.Fatalf("got session = %+v", got)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions?workspace_id=workspace_1", nil)
	listRequest.Header.Set("X-Cocode-Token", "test-token")
	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	list := decodeReviewSessionListResponse(t, listResponse.Body.Bytes())
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v", list)
	}
}

func TestDeleteReviewSessionEndpointDeletesSessionAndOrphanSnapshot(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPISnapshot(t, queries)
	createHTTPAPIAgentConfig(t, queries, "agent_config_delete", "primary_reviewer", 1)
	session := createHTTPAPIReviewSessionRow(t, queries, "review_session_delete", []string{"agent_config_delete"})

	request := httptest.NewRequest(http.MethodDelete, "/api/review-sessions/"+session.ID, nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data  DeleteReviewSessionResponse `json:"data"`
		Error any                         `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if !envelope.Data.Deleted || !envelope.Data.SnapshotDeleted || envelope.Data.ID != session.ID || envelope.Data.SnapshotID != "snapshot_1" {
		t.Fatalf("delete response = %+v", envelope.Data)
	}
	if _, err := queries.GetReviewSession(context.Background(), session.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetReviewSession() error = %v, want sql.ErrNoRows", err)
	}
	if _, err := queries.GetPullRequestSnapshot(context.Background(), "snapshot_1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetPullRequestSnapshot() error = %v, want sql.ErrNoRows", err)
	}
	if _, err := queries.GetChangedFileByPath(context.Background(), dbgen.GetChangedFileByPathParams{
		SnapshotID: "snapshot_1",
		Path:       "src/new.go",
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetChangedFileByPath() error = %v, want sql.ErrNoRows", err)
	}
}

func TestReviewSessionChatThreadEndpointSeedsAndAnswers(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPISnapshot(t, queries)
	createHTTPAPIAgentConfig(t, queries, "agent_config_chat", "primary_reviewer", 1)
	session := createHTTPAPIReviewSessionRow(t, queries, "review_session_chat", []string{"agent_config_chat"})

	getRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions/"+session.ID+"/chat-thread", nil)
	getRequest.Header.Set("X-Cocode-Token", "test-token")
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("chat thread status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
	seeded := decodeChatThreadViewResponse(t, getResponse.Body.Bytes())
	if seeded.Thread.ReviewSessionID != session.ID || seeded.Thread.ID == "" {
		t.Fatalf("seeded thread = %+v", seeded.Thread)
	}
	if len(seeded.Messages) != 3 {
		t.Fatalf("seeded message count = %d, messages = %+v", len(seeded.Messages), seeded.Messages)
	}
	if seeded.Messages[1].AuthorType != "orchestrator" || !strings.Contains(seeded.Messages[1].Body, "coordinate") {
		t.Fatalf("orchestrator seed message = %+v", seeded.Messages[1])
	}

	askRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions/"+session.ID+"/chat-turns", map[string]any{
		"body":     "What is the current review status?",
		"audience": "orchestrator",
	})
	askResponse := httptest.NewRecorder()
	router.ServeHTTP(askResponse, askRequest)
	if askResponse.Code != http.StatusOK {
		t.Fatalf("chat turn status = %d, body = %s", askResponse.Code, askResponse.Body.String())
	}
	answered := decodeChatTurnResponse(t, askResponse.Body.Bytes())
	if answered.Turn.Status != "completed" || answered.Turn.Audience != "orchestrator" {
		t.Fatalf("chat turn = %+v", answered.Turn)
	}
	if len(answered.Messages) < 5 {
		t.Fatalf("answered message count = %d, messages = %+v", len(answered.Messages), answered.Messages)
	}
	last := answered.Messages[len(answered.Messages)-1]
	if last.AuthorType != "cocode" || !strings.Contains(last.Body, "Current review status: draft.") {
		t.Fatalf("local answer = %+v", last)
	}
}

func TestReviewSessionChatThreadEndpointUsesOrchestratorResponder(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)
	command := fakeJSONAgentPath(t)
	createHTTPAPIAgentConfigWithCommand(t, queries, "agent_config_orchestrator", "orchestrator", 1, command, agents.OutputJSON, `{"prompt_delivery":"stdin","timeout_seconds":30}`)
	if _, err := queries.CreateReviewSessionAgent(context.Background(), dbgen.CreateReviewSessionAgentParams{
		ID:                   "review_session_agent_orchestrator",
		ReviewSessionID:      "review_session_findings",
		AgentConfigID:        "agent_config_orchestrator",
		Role:                 "orchestrator",
		RunOrder:             2,
		Enabled:              1,
		SettingsOverrideJson: "{}",
	}); err != nil {
		t.Fatalf("CreateReviewSessionAgent(orchestrator) error = %v", err)
	}

	askRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions/review_session_findings/chat-turns", map[string]any{
		"body":                      "Explain the finding again for me.",
		"audience":                  "orchestrator",
		"responder_agent_config_id": "agent_config_orchestrator",
	})
	askResponse := httptest.NewRecorder()
	router.ServeHTTP(askResponse, askRequest)
	if askResponse.Code != http.StatusOK {
		t.Fatalf("chat turn status = %d, body = %s", askResponse.Code, askResponse.Body.String())
	}
	answered := decodeChatTurnResponse(t, askResponse.Body.Bytes())
	if answered.Turn.Status != "completed" || answered.Turn.Audience != "orchestrator" {
		t.Fatalf("chat turn = %+v", answered.Turn)
	}
	if len(answered.Messages) == 0 {
		t.Fatalf("answered messages = %+v", answered.Messages)
	}
	last := answered.Messages[len(answered.Messages)-1]
	if last.AuthorType != "orchestrator" || strings.Contains(last.Body, "Current review status:") || !strings.Contains(last.Body, "Found one deterministic fixture issue.") {
		t.Fatalf("orchestrator answer = %+v", last)
	}
}

func TestReviewSessionChatThreadEndpointSyncsWorkflowProgress(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPISnapshot(t, queries)
	createHTTPAPIAgentConfig(t, queries, "agent_config_chat", "primary_reviewer", 1)
	session := createHTTPAPIReviewSessionRow(t, queries, "review_session_chat_progress", []string{"agent_config_chat"})
	ctx := context.Background()
	if _, err := queries.CreateEvent(ctx, dbgen.CreateEventParams{
		ID:              "event_review_queued",
		ReviewSessionID: nullableString(session.ID),
		Type:            "ReviewSessionQueued",
		Level:           "info",
		Sequence:        1,
		PayloadJson:     `{"status":"queued"}`,
		CreatedAt:       "2026-05-03T00:08:00Z",
	}); err != nil {
		t.Fatalf("CreateEvent(queued) error = %v", err)
	}
	if _, err := queries.CreateEvent(ctx, dbgen.CreateEventParams{
		ID:              "event_phase_started",
		ReviewSessionID: nullableString(session.ID),
		Type:            "WorkflowPhaseStarted",
		Level:           "info",
		Sequence:        2,
		PayloadJson:     `{"phase":"run_agents"}`,
		CreatedAt:       "2026-05-03T00:09:00Z",
	}); err != nil {
		t.Fatalf("CreateEvent(phase) error = %v", err)
	}
	if _, err := queries.CreateEvent(ctx, dbgen.CreateEventParams{
		ID:              "event_review_completed",
		ReviewSessionID: nullableString(session.ID),
		Type:            "ReviewSessionCompleted",
		Level:           "info",
		Sequence:        4,
		PayloadJson:     `{"status":"completed"}`,
		CreatedAt:       "2026-05-03T00:12:00Z",
	}); err != nil {
		t.Fatalf("CreateEvent(completed) error = %v", err)
	}
	if _, err := queries.CreateEvent(ctx, dbgen.CreateEventParams{
		ID:              "event_verify_started",
		ReviewSessionID: nullableString(session.ID),
		Type:            "WorkflowPhaseStarted",
		Level:           "info",
		Sequence:        3,
		PayloadJson:     `{"phase":"verify_findings"}`,
		CreatedAt:       "2026-05-03T00:11:00Z",
	}); err != nil {
		t.Fatalf("CreateEvent(verify phase) error = %v", err)
	}
	if _, err := queries.CreateAgentRun(ctx, dbgen.CreateAgentRunParams{
		ID:              "agent_run_chat_progress",
		ReviewSessionID: session.ID,
		AgentConfigID:   "agent_config_chat",
		Status:          "succeeded",
		Role:            "primary_reviewer",
		MetadataJson:    "{}",
	}); err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if _, err := queries.CreateFinding(ctx, dbgen.CreateFindingParams{
		ID:                 "finding_chat_progress",
		ReviewSessionID:    session.ID,
		CanonicalClaim:     "Missing authorization check on invoice export.",
		Category:           "security",
		Severity:           "high",
		Confidence:         0.91,
		VerificationStatus: "verified",
		DecisionStatus:     "needs_triage",
		PrimaryPath:        nullableString("src/export.go"),
		Fingerprint:        "finding-chat-progress",
		FirstSeenAt:        "2026-05-03T00:10:00Z",
		UpdatedAt:          "2026-05-03T00:10:00Z",
	}); err != nil {
		t.Fatalf("CreateFinding() error = %v", err)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions/"+session.ID+"/chat-thread", nil)
	getRequest.Header.Set("X-Cocode-Token", "test-token")
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("chat thread status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
	view := decodeChatThreadViewResponse(t, getResponse.Body.Bytes())
	bodies := make([]string, 0, len(view.Messages))
	for _, message := range view.Messages {
		bodies = append(bodies, message.Body)
	}
	joined := strings.Join(bodies, "\n")
	for _, want := range []string{
		"Review queued.",
		"Agent review started.",
		"Orchestrator is re-checking each finding against code evidence and counter-evidence.",
		"agent_config_chat did not emit answer text.",
		"Early findings are in.",
		"Missing authorization check on invoice export.",
		"Review completed.",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("chat messages missing %q:\n%s", want, joined)
		}
	}
	if got := view.Messages[len(view.Messages)-1].Body; !strings.Contains(got, "Review completed.") {
		t.Fatalf("last progress message = %q, want review completion last", got)
	}

	reloadRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions/"+session.ID+"/chat-thread", nil)
	reloadRequest.Header.Set("X-Cocode-Token", "test-token")
	reloadResponse := httptest.NewRecorder()
	router.ServeHTTP(reloadResponse, reloadRequest)
	if reloadResponse.Code != http.StatusOK {
		t.Fatalf("reload status = %d, body = %s", reloadResponse.Code, reloadResponse.Body.String())
	}
	reloaded := decodeChatThreadViewResponse(t, reloadResponse.Body.Bytes())
	if len(reloaded.Messages) != len(view.Messages) {
		t.Fatalf("message count changed on idempotent reload: %d -> %d", len(view.Messages), len(reloaded.Messages))
	}
}

func TestReviewSessionCreateRejectsInvalidInputs(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPISnapshot(t, queries)
	createHTTPAPIAgentConfig(t, queries, "agent_config_disabled", "reviewer", 0)
	if _, err := queries.CreateAgentConfig(context.Background(), dbgen.CreateAgentConfigParams{
		ID:               "agent_config_writer",
		Name:             "Write-capable reviewer",
		Role:             "reviewer",
		AdapterKind:      "cli_noninteractive",
		Command:          nullableString("codex"),
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       "json",
		CapabilitiesJson: `{"supports_json":true,"can_read":true,"can_write":true,"output_modes":["json"]}`,
		SettingsJson:     "{}",
		Enabled:          1,
		CreatedAt:        "2026-05-03T00:06:00Z",
		UpdatedAt:        "2026-05-03T00:06:00Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig(writer) error = %v", err)
	}

	tests := []struct {
		name string
		body map[string]any
		code int
	}{
		{
			name: "missing agent configs",
			body: map[string]any{
				"snapshot_id": "snapshot_1",
			},
			code: http.StatusBadRequest,
		},
		{
			name: "disabled agent",
			body: map[string]any{
				"snapshot_id":      "snapshot_1",
				"agent_config_ids": []string{"agent_config_disabled"},
			},
			code: http.StatusBadRequest,
		},
		{
			name: "write capable agent",
			body: map[string]any{
				"snapshot_id":      "snapshot_1",
				"agent_config_ids": []string{"agent_config_writer"},
			},
			code: http.StatusBadRequest,
		},
		{
			name: "missing snapshot",
			body: map[string]any{
				"snapshot_id":      "missing_snapshot",
				"agent_config_ids": []string{"agent_config_disabled"},
			},
			code: http.StatusNotFound,
		},
		{
			name: "invalid policy",
			body: map[string]any{
				"snapshot_id":      "snapshot_1",
				"agent_config_ids": []string{"agent_config_disabled"},
				"context_policy":   map[string]any{"max_tokens": 0},
			},
			code: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions", tt.body)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.code {
				t.Fatalf("status = %d, want %d, body = %s", response.Code, tt.code, response.Body.String())
			}
		})
	}
}

func TestStartReviewSessionEndpointRunsWorkflow(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	repoPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoPath, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	writeHTTPAPIDefaultRepo(t, repoPath)
	createHTTPAPISnapshotAt(t, queries, repoPath)
	fakeAgent := fakeJSONAgentPath(t)
	createHTTPAPIAgentConfigWithCommand(t, queries, "agent_config_fake", "primary_reviewer", 1, fakeAgent, agents.OutputJSON, `{"prompt_delivery":"stdin","timeout_seconds":30}`)

	createRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions", map[string]any{
		"workspace_id":          "workspace_1",
		"snapshot_id":           "snapshot_1",
		"title":                 "Review fake workflow",
		"agent_config_ids":      []string{"agent_config_fake"},
		"runtime_limit_seconds": 60,
		"context_policy": map[string]any{
			"include_prompt_material":     true,
			"include_changed_code":        true,
			"include_related_call_sites":  false,
			"include_related_tests":       false,
			"include_project_conventions": false,
			"include_prior_comments":      false,
			"include_prior_decisions":     false,
			"max_tokens":                  4096,
			"max_items":                   20,
		},
	})
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	created := decodeReviewSessionResponse(t, createResponse.Body.Bytes())

	startRequest := httptest.NewRequest(http.MethodPost, "/api/review-sessions/"+created.ID+"/start", nil)
	startRequest.Header.Set("X-Cocode-Token", "test-token")
	startResponse := httptest.NewRecorder()
	router.ServeHTTP(startResponse, startRequest)
	if startResponse.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", startResponse.Code, startResponse.Body.String())
	}
	started := decodeReviewSessionResponse(t, startResponse.Body.Bytes())
	if started.Status != "queued" || started.StartedAt != "" || started.CompletedAt != "" {
		t.Fatalf("started response = %+v", started)
	}

	completed := waitForHTTPAPIReviewSessionStatus(t, queries, created.ID, "completed")
	if completed.StartedAt.String == "" || completed.CompletedAt.String == "" {
		t.Fatalf("completed session timestamps = %+v", completed)
	}
	runs, err := queries.ListAgentRunsBySession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "succeeded" || !runs[0].ParsedOutputArtifactID.Valid {
		t.Fatalf("agent runs = %+v", runs)
	}

	checkpointRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions/"+created.ID+"/checkpoint", nil)
	checkpointRequest.Header.Set("X-Cocode-Token", "test-token")
	checkpointResponse := httptest.NewRecorder()
	router.ServeHTTP(checkpointResponse, checkpointRequest)
	if checkpointResponse.Code != http.StatusOK {
		t.Fatalf("checkpoint status = %d, body = %s", checkpointResponse.Code, checkpointResponse.Body.String())
	}
	checkpoint := decodeReviewCheckpointResponse(t, checkpointResponse.Body.Bytes())
	if checkpoint.Status != "completed" || checkpoint.Phase != "draft_comments" || checkpoint.PhaseStatus != "completed" {
		t.Fatalf("checkpoint = %+v", checkpoint)
	}
	summaryRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions/"+created.ID+"/summary", nil)
	summaryRequest.Header.Set("X-Cocode-Token", "test-token")
	summaryResponse := httptest.NewRecorder()
	router.ServeHTTP(summaryResponse, summaryRequest)
	if summaryResponse.Code != http.StatusOK {
		t.Fatalf("summary status = %d, body = %s", summaryResponse.Code, summaryResponse.Body.String())
	}
	summary := decodeReviewSummaryResponse(t, summaryResponse.Body.Bytes())
	if summary.Status != "completed" ||
		summary.ProgressPercent != 100 ||
		summary.AgentStatusCounts["succeeded"] != 1 ||
		summary.ChangedFilesTotal != 2 ||
		summary.ActiveAgents != 0 {
		t.Fatalf("summary = %+v", summary)
	}

	duplicateStart := httptest.NewRequest(http.MethodPost, "/api/review-sessions/"+created.ID+"/start", nil)
	duplicateStart.Header.Set("X-Cocode-Token", "test-token")
	duplicateResponse := httptest.NewRecorder()
	router.ServeHTTP(duplicateResponse, duplicateStart)
	if duplicateResponse.Code != http.StatusBadRequest {
		t.Fatalf("duplicate start status = %d, body = %s", duplicateResponse.Code, duplicateResponse.Body.String())
	}
}

func TestRouterStartupReconcilesInterruptedReviewWorkflow(t *testing.T) {
	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	queries := dbgen.New(database)
	repoPath := t.TempDir()
	writeHTTPAPIDefaultRepo(t, repoPath)
	createHTTPAPISnapshotAt(t, queries, repoPath)
	createHTTPAPIAgentConfig(t, queries, "agent_config_interrupted", "primary_reviewer", 1)
	session := createHTTPAPIReviewSessionRow(t, queries, "review_session_startup_reconcile", []string{"agent_config_interrupted"})
	if _, err := queries.UpdateReviewSessionStatus(context.Background(), dbgen.UpdateReviewSessionStatusParams{
		ID:        session.ID,
		Status:    "running",
		StartedAt: nullableString("2026-05-03T00:08:00Z"),
		UpdatedAt: "2026-05-03T00:08:00Z",
	}); err != nil {
		t.Fatalf("UpdateReviewSessionStatus() error = %v", err)
	}
	if _, err := queries.CreateAgentRun(context.Background(), dbgen.CreateAgentRunParams{
		ID:              "agent_run_startup_reconcile",
		ReviewSessionID: session.ID,
		AgentConfigID:   "agent_config_interrupted",
		Status:          "running",
		Role:            "primary_reviewer",
		StartedAt:       nullableString("2026-05-03T00:08:01Z"),
		MetadataJson:    `{"phase":"run_review_agents"}`,
	}); err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	router := NewRouter(app.Config{
		Addr:        "127.0.0.1:0",
		AuthToken:   "test-token",
		DataDir:     t.TempDir(),
		ArtifactDir: filepath.Join(t.TempDir(), "artifacts"),
		Version:     "test-version",
	}, logger, database)

	reconciled, err := queries.GetReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetReviewSession() error = %v", err)
	}
	if reconciled.Status != "paused" || reconciled.CompletedAt.Valid {
		t.Fatalf("reconciled session = %+v", reconciled)
	}
	run, err := queries.GetAgentRun(context.Background(), "agent_run_startup_reconcile")
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if run.Status != "canceled" || run.ErrorCode.String != "app_restarted" {
		t.Fatalf("reconciled run = %+v", run)
	}

	summaryRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions/"+session.ID+"/summary", nil)
	summaryRequest.Header.Set("X-Cocode-Token", "test-token")
	summaryResponse := httptest.NewRecorder()
	router.ServeHTTP(summaryResponse, summaryRequest)
	if summaryResponse.Code != http.StatusOK {
		t.Fatalf("summary status = %d, body = %s", summaryResponse.Code, summaryResponse.Body.String())
	}
	summary := decodeReviewSummaryResponse(t, summaryResponse.Body.Bytes())
	if summary.Status != "paused" || summary.ActiveAgents != 0 || summary.AgentStatusCounts["canceled"] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestFindingListEndpointReturnsCountsAndFilters(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	listRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions/review_session_findings/findings", nil)
	listRequest.Header.Set("X-Cocode-Token", "test-token")
	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	list := decodeFindingListResponse(t, listResponse.Body.Bytes())
	if len(list.Items) != 3 ||
		list.Stats.Total != 3 ||
		list.Stats.NeedsTriage != 1 ||
		list.Stats.ByDecision["accepted"] != 1 ||
		list.Items[0].ID != "finding_auth" ||
		list.Items[0].Severity != "high" {
		t.Fatalf("list = %+v", list)
	}
	if len(list.Items[0].SourceAgents) != 1 ||
		list.Items[0].SourceAgents[0].AgentConfigID != "agent_config_findings" ||
		list.Items[0].SourceAgents[0].Name != "agent_config_findings" {
		t.Fatalf("source agents = %+v", list.Items[0].SourceAgents)
	}
	if len(list.Filters.Agents) != 1 ||
		list.Filters.Agents[0].ID != "agent_config_findings" ||
		list.Filters.Agents[0].Count != 3 {
		t.Fatalf("agent filters = %+v", list.Filters.Agents)
	}
	if len(list.Filters.Files) < 2 ||
		list.Filters.Files[0].ID != "apps/desktop/src/renderer/src/app/App.tsx" ||
		list.Filters.Files[0].Count != 2 {
		t.Fatalf("file filters = %+v", list.Filters.Files)
	}

	acceptedRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions/review_session_findings/findings?status=accepted", nil)
	acceptedRequest.Header.Set("X-Cocode-Token", "test-token")
	acceptedResponse := httptest.NewRecorder()
	router.ServeHTTP(acceptedResponse, acceptedRequest)
	if acceptedResponse.Code != http.StatusOK {
		t.Fatalf("accepted status = %d, body = %s", acceptedResponse.Code, acceptedResponse.Body.String())
	}
	accepted := decodeFindingListResponse(t, acceptedResponse.Body.Bytes())
	if len(accepted.Items) != 1 || accepted.Items[0].ID != "finding_auth" {
		t.Fatalf("accepted = %+v", accepted)
	}

	searchRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions/review_session_findings/findings?status=needs_triage&q=budget", nil)
	searchRequest.Header.Set("X-Cocode-Token", "test-token")
	searchResponse := httptest.NewRecorder()
	router.ServeHTTP(searchResponse, searchRequest)
	if searchResponse.Code != http.StatusOK {
		t.Fatalf("search status = %d, body = %s", searchResponse.Code, searchResponse.Body.String())
	}
	search := decodeFindingListResponse(t, searchResponse.Body.Bytes())
	if len(search.Items) != 1 || search.Items[0].ID != "finding_budget" {
		t.Fatalf("search = %+v", search)
	}

	agentRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions/review_session_findings/findings?agent=agent_config_findings&file=apps/desktop/src/renderer/src/app/App.tsx", nil)
	agentRequest.Header.Set("X-Cocode-Token", "test-token")
	agentResponse := httptest.NewRecorder()
	router.ServeHTTP(agentResponse, agentRequest)
	if agentResponse.Code != http.StatusOK {
		t.Fatalf("agent/file status = %d, body = %s", agentResponse.Code, agentResponse.Body.String())
	}
	agentFiltered := decodeFindingListResponse(t, agentResponse.Body.Bytes())
	if len(agentFiltered.Items) != 2 ||
		agentFiltered.Items[0].ID != "finding_budget" ||
		agentFiltered.Items[1].ID != "finding_theme" {
		t.Fatalf("agent/file filtered = %+v", agentFiltered.Items)
	}
}

func TestFindingDetailEndpointReturnsProvenanceAndEvidence(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := httptest.NewRequest(http.MethodGet, "/api/findings/finding_auth", nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", response.Code, response.Body.String())
	}
	detail := decodeFindingDetailResponse(t, response.Body.Bytes())
	if detail.Finding.ID != "finding_auth" ||
		detail.Finding.DraftComment == "" ||
		len(detail.Candidates) != 2 ||
		len(detail.EvidenceItems) != 1 ||
		detail.EvidenceItems[0].Kind != "supporting" ||
		len(detail.EvidenceGroups.Supporting) != 1 ||
		detail.EvidenceItems[0].CodeSnippet == "" ||
		detail.EvidenceItems[0].LineWindow == nil ||
		detail.EvidenceItems[0].LineWindow.StartLine != 84 ||
		len(detail.Decisions) != 1 ||
		detail.Decisions[0].Decision != "accepted" {
		t.Fatalf("detail = %+v", detail)
	}
}

func TestFindingDetailEndpointHydratesEvidenceSnippetFromRepository(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	repoPath := t.TempDir()
	writeHTTPAPIRepoFile(t, repoPath, "src/server.js", strings.Join([]string{
		"export function createBillingRouter({ db }) {",
		"  return {",
		"    cancelSubscription(request) {",
		"      return db.cancelSubscription(request.params.subscriptionId);",
		"    },",
		"  };",
		"}",
	}, "\n"))
	createHTTPAPISnapshotAt(t, queries, repoPath)
	createHTTPAPIAgentConfig(t, queries, "agent_config_hydrate", "primary_reviewer", 1)
	createHTTPAPIReviewSessionRow(t, queries, "review_session_hydrate", []string{"agent_config_hydrate"})
	if _, err := queries.CreateFinding(context.Background(), dbgen.CreateFindingParams{
		ID:                 "finding_hydrate",
		ReviewSessionID:    "review_session_hydrate",
		CanonicalClaim:     "Subscription cancellation bypasses authorization.",
		Category:           "security",
		Severity:           "high",
		Confidence:         0.91,
		VerificationStatus: "verified",
		DecisionStatus:     "needs_triage",
		PrimaryPath:        nullableString("src/server.js"),
		PrimaryStartLine:   sql.NullInt64{Int64: 4, Valid: true},
		PrimaryEndLine:     sql.NullInt64{Int64: 4, Valid: true},
		EvidenceSummary:    nullableString("The changed route calls the database directly."),
		SuggestedFix:       nullableString("Require admin authorization before calling the database."),
		Fingerprint:        "hydrate-snippet",
		FirstSeenAt:        "2026-05-03T00:20:00Z",
		UpdatedAt:          "2026-05-03T00:20:00Z",
	}); err != nil {
		t.Fatalf("CreateFinding(hydrate) error = %v", err)
	}
	if _, err := queries.CreateEvidenceItem(context.Background(), dbgen.CreateEvidenceItemParams{
		ID:           "evidence_hydrate",
		FindingID:    "finding_hydrate",
		Kind:         "supporting",
		Title:        "Changed route calls cancelSubscription",
		Summary:      "The changed route reaches the database without a guard.",
		Path:         nullableString("src/server.js"),
		StartLine:    sql.NullInt64{Int64: 4, Valid: true},
		EndLine:      sql.NullInt64{Int64: 4, Valid: true},
		Confidence:   0.9,
		MetadataJson: `{"producer":"local_verifier"}`,
		CreatedAt:    "2026-05-03T00:21:00Z",
	}); err != nil {
		t.Fatalf("CreateEvidenceItem(hydrate) error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/findings/finding_hydrate", nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", response.Code, response.Body.String())
	}
	detail := decodeFindingDetailResponse(t, response.Body.Bytes())
	if len(detail.EvidenceItems) != 1 {
		t.Fatalf("evidence items = %+v, want one", detail.EvidenceItems)
	}
	item := detail.EvidenceItems[0]
	if !strings.Contains(item.CodeSnippet, "cancelSubscription") ||
		item.LineWindow == nil ||
		item.LineWindow.StartLine != 1 ||
		item.LineWindow.EndLine < 4 {
		t.Fatalf("hydrated evidence = %+v", item)
	}
}

func TestFindingDetailEndpointSynthesizesPrimarySnippetFromFindingAnchor(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	repoPath := t.TempDir()
	writeHTTPAPIRepoFile(t, repoPath, "src/server.js", strings.Join([]string{
		"export function createBillingRouter({ db }) {",
		"  return {",
		"    cancelSubscription(request) {",
		"      return db.cancelSubscription(request.params.subscriptionId);",
		"    },",
		"  };",
		"}",
	}, "\n"))
	createHTTPAPISnapshotAt(t, queries, repoPath)
	createHTTPAPIAgentConfig(t, queries, "agent_config_primary_snippet", "primary_reviewer", 1)
	createHTTPAPIReviewSessionRow(t, queries, "review_session_primary_snippet", []string{"agent_config_primary_snippet"})
	if _, err := queries.CreateFinding(context.Background(), dbgen.CreateFindingParams{
		ID:                 "finding_primary_snippet",
		ReviewSessionID:    "review_session_primary_snippet",
		CanonicalClaim:     "Subscription cancellation bypasses authorization.",
		Category:           "security",
		Severity:           "high",
		Confidence:         0.91,
		VerificationStatus: "verified",
		DecisionStatus:     "needs_triage",
		PrimaryPath:        nullableString("src/server.js"),
		PrimaryStartLine:   sql.NullInt64{Int64: 4, Valid: true},
		PrimaryEndLine:     sql.NullInt64{Int64: 4, Valid: true},
		EvidenceSummary:    nullableString("The changed route calls the database directly."),
		SuggestedFix:       nullableString("Require admin authorization before calling the database."),
		Fingerprint:        "primary-snippet",
		FirstSeenAt:        "2026-05-03T00:20:00Z",
		UpdatedAt:          "2026-05-03T00:20:00Z",
	}); err != nil {
		t.Fatalf("CreateFinding(primary snippet) error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/findings/finding_primary_snippet", nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", response.Code, response.Body.String())
	}
	detail := decodeFindingDetailResponse(t, response.Body.Bytes())
	if len(detail.EvidenceItems) != 1 {
		t.Fatalf("evidence items = %+v, want synthesized primary item", detail.EvidenceItems)
	}
	item := detail.EvidenceItems[0]
	if item.ID != "finding_primary_snippet:primary-code" ||
		item.Kind != "supporting" ||
		item.Path != "src/server.js" ||
		item.Summary != "The changed route calls the database directly." ||
		!strings.Contains(item.CodeSnippet, "cancelSubscription") ||
		item.LineWindow == nil ||
		item.LineWindow.StartLine != 1 ||
		item.LineWindow.EndLine < 4 ||
		len(detail.EvidenceGroups.Supporting) != 1 {
		t.Fatalf("synthesized evidence = %+v, groups = %+v", item, detail.EvidenceGroups)
	}
}

func TestFindingDetailEndpointClampsStalePrimarySnippetLine(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	repoPath := t.TempDir()
	writeHTTPAPIRepoFile(t, repoPath, "src/server.js", strings.Join([]string{
		"first line",
		"second line",
		"last line",
	}, "\n"))
	createHTTPAPISnapshotAt(t, queries, repoPath)
	createHTTPAPIAgentConfig(t, queries, "agent_config_stale_primary", "primary_reviewer", 1)
	createHTTPAPIReviewSessionRow(t, queries, "review_session_stale_primary", []string{"agent_config_stale_primary"})
	if _, err := queries.CreateFinding(context.Background(), dbgen.CreateFindingParams{
		ID:                 "finding_stale_primary",
		ReviewSessionID:    "review_session_stale_primary",
		CanonicalClaim:     "Reported line moved beyond the current file.",
		Category:           "correctness",
		Severity:           "medium",
		Confidence:         0.7,
		VerificationStatus: "needs_human",
		DecisionStatus:     "needs_triage",
		PrimaryPath:        nullableString("src/server.js"),
		PrimaryStartLine:   sql.NullInt64{Int64: 99, Valid: true},
		PrimaryEndLine:     sql.NullInt64{Int64: 99, Valid: true},
		Fingerprint:        "stale-primary-snippet",
		FirstSeenAt:        "2026-05-03T00:20:00Z",
		UpdatedAt:          "2026-05-03T00:20:00Z",
	}); err != nil {
		t.Fatalf("CreateFinding(stale primary) error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/findings/finding_stale_primary", nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", response.Code, response.Body.String())
	}
	detail := decodeFindingDetailResponse(t, response.Body.Bytes())
	if len(detail.EvidenceItems) != 1 {
		t.Fatalf("evidence items = %+v, want synthesized primary item", detail.EvidenceItems)
	}
	item := detail.EvidenceItems[0]
	if !strings.Contains(item.CodeSnippet, "3: last line") ||
		item.LineWindow == nil ||
		item.LineWindow.StartLine != 1 ||
		item.LineWindow.EndLine != 3 {
		t.Fatalf("stale primary evidence = %+v", item)
	}
}

func TestFindingEvidenceEndpointGroupsItems(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)
	if _, err := queries.CreateEvidenceItem(context.Background(), dbgen.CreateEvidenceItemParams{
		ID:           "evidence_auth_test",
		FindingID:    "finding_auth",
		Kind:         "test",
		Title:        "Admin route test exists",
		Summary:      "A local verifier found a related test path.",
		Path:         nullableString("apps/api/src/routes/repositories.test.ts"),
		StartLine:    sql.NullInt64{Int64: 19, Valid: true},
		EndLine:      sql.NullInt64{Int64: 19, Valid: true},
		Confidence:   0.6,
		MetadataJson: `{"producer":"local_verifier","code_snippet":"19: expect(adminOnly).toBe(true)"}`,
		CreatedAt:    "2026-05-03T00:15:00Z",
	}); err != nil {
		t.Fatalf("CreateEvidenceItem(test) error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/findings/finding_auth/evidence", nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("evidence status = %d, body = %s", response.Code, response.Body.String())
	}
	evidence := decodeFindingEvidenceResponse(t, response.Body.Bytes())
	if evidence.Finding.ID != "finding_auth" ||
		len(evidence.Items) != 2 ||
		len(evidence.Groups.Supporting) != 1 ||
		len(evidence.Groups.Test) != 1 ||
		evidence.Counts["supporting"] != 1 ||
		evidence.Counts["test"] != 1 ||
		evidence.Groups.Test[0].CodeSnippet == "" {
		t.Fatalf("evidence = %+v", evidence)
	}
}

func TestFindingEvidenceMapEndpointBuildsAndRebuilds(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := httptest.NewRequest(http.MethodGet, "/api/findings/finding_auth/evidence-map", nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("evidence map status = %d, body = %s", response.Code, response.Body.String())
	}
	view := decodeFindingEvidenceMapResponse(t, response.Body.Bytes())
	if view.Finding.ID != "finding_auth" ||
		view.Graph.Status != evidencepkg.GraphStatusReady ||
		len(view.Nodes) < 2 ||
		len(view.Edges) == 0 ||
		len(view.CallPath) < 2 ||
		len(view.Hierarchy) == 0 ||
		view.Panel.EvidenceCounts[evidencepkg.KindSupporting] != 1 {
		t.Fatalf("view = %+v", view)
	}
	if !hasEvidenceMapEdge(view.Edges, evidencepkg.EdgeMissingGuard, evidencepkg.EdgeStatusMissing) {
		t.Fatalf("edges = %+v", view.Edges)
	}

	rebuildRequest := httptest.NewRequest(http.MethodPost, "/api/findings/finding_auth/evidence-map/rebuild", nil)
	rebuildRequest.Header.Set("X-Cocode-Token", "test-token")
	rebuildResponse := httptest.NewRecorder()
	router.ServeHTTP(rebuildResponse, rebuildRequest)
	if rebuildResponse.Code != http.StatusOK {
		t.Fatalf("evidence map rebuild status = %d, body = %s", rebuildResponse.Code, rebuildResponse.Body.String())
	}
	rebuilt := decodeFindingEvidenceMapResponse(t, rebuildResponse.Body.Bytes())
	if rebuilt.Graph.ID != view.Graph.ID || rebuilt.Graph.UpdatedAt == "" {
		t.Fatalf("rebuilt = %+v, original = %+v", rebuilt, view)
	}
	events, err := queries.ListEventsByReviewSession(context.Background(), nullableString("review_session_findings"))
	if err != nil {
		t.Fatalf("ListEventsByReviewSession() error = %v", err)
	}
	if len(events) != 1 || events[0].Type != "EvidenceMapRebuilt" {
		t.Fatalf("events = %+v", events)
	}
}

func TestFindingThreadEndpointCreatesAndReloadsThread(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := httptest.NewRequest(http.MethodGet, "/api/findings/finding_auth/thread", nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("thread status = %d, body = %s", response.Code, response.Body.String())
	}
	first := decodeFindingThreadViewResponse(t, response.Body.Bytes())
	if first.Finding.ID != "finding_auth" ||
		first.Thread.FindingID != "finding_auth" ||
		first.Thread.ReviewSessionID != "review_session_findings" ||
		first.Thread.Title == "" ||
		len(first.Messages) != 0 {
		t.Fatalf("thread view = %+v", first)
	}
	stored, err := queries.GetFindingThreadByFinding(context.Background(), "finding_auth")
	if err != nil {
		t.Fatalf("GetFindingThreadByFinding() error = %v", err)
	}
	if stored.ID != first.Thread.ID {
		t.Fatalf("stored thread = %+v, response = %+v", stored, first.Thread)
	}
	if _, err := queries.CreateFindingThreadMessage(context.Background(), dbgen.CreateFindingThreadMessageParams{
		ID:               "finding_thread_message_user",
		ThreadID:         stored.ID,
		Role:             "user",
		Content:          "Can you re-check the guard path?",
		EvidenceRefsJson: `[{"evidence_item_id":"evidence_auth_guard"}]`,
		CreatedAt:        "2026-05-03T00:17:00Z",
	}); err != nil {
		t.Fatalf("CreateFindingThreadMessage(user) error = %v", err)
	}
	if _, err := queries.CreateArtifact(context.Background(), dbgen.CreateArtifactParams{
		ID:              "artifact_thread_answer",
		WorkspaceID:     "workspace_1",
		ReviewSessionID: nullableString("review_session_findings"),
		Kind:            "followup_answer",
		RelativePath:    "followup/answer.md",
		ContentType:     "text/markdown",
		SizeBytes:       54,
		MetadataJson:    "{}",
		CreatedAt:       "2026-05-03T00:17:30Z",
	}); err != nil {
		t.Fatalf("CreateArtifact(thread answer) error = %v", err)
	}
	if _, err := queries.CreateFindingThreadMessage(context.Background(), dbgen.CreateFindingThreadMessageParams{
		ID:               "finding_thread_message_assistant",
		ThreadID:         stored.ID,
		Role:             "assistant",
		AgentConfigID:    nullableString("agent_config_findings"),
		Content:          "The scoped evidence still supports the auth finding.",
		EvidenceRefsJson: `[]`,
		ArtifactID:       nullableString("artifact_thread_answer"),
		CreatedAt:        "2026-05-03T00:18:00Z",
	}); err != nil {
		t.Fatalf("CreateFindingThreadMessage(assistant) error = %v", err)
	}

	reloadRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions/review_session_findings/findings/finding_auth/thread", nil)
	reloadRequest.Header.Set("X-Cocode-Token", "test-token")
	reloadResponse := httptest.NewRecorder()
	router.ServeHTTP(reloadResponse, reloadRequest)
	if reloadResponse.Code != http.StatusOK {
		t.Fatalf("thread reload status = %d, body = %s", reloadResponse.Code, reloadResponse.Body.String())
	}
	reloaded := decodeFindingThreadViewResponse(t, reloadResponse.Body.Bytes())
	if reloaded.Thread.ID != first.Thread.ID ||
		len(reloaded.Messages) != 2 ||
		reloaded.Messages[0].Role != "user" ||
		string(reloaded.Messages[0].EvidenceRefs) != `[{"evidence_item_id":"evidence_auth_guard"}]` ||
		reloaded.Messages[1].AgentConfigID != "agent_config_findings" ||
		reloaded.Messages[1].ArtifactID != "artifact_thread_answer" {
		t.Fatalf("reloaded thread = %+v", reloaded)
	}
}

func TestFindingQuestionEndpointRunsAgentAndPersistsMessages(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)
	if err := os.MkdirAll("/tmp/cocode", 0o755); err != nil {
		t.Fatalf("mkdir repo root: %v", err)
	}
	command := writeFakeAgentConfigCommand(t, `#!/bin/sh
cat >/dev/null
printf '{"answer":"The scoped evidence still supports the auth finding.","evidence_refs":[{"evidence_item_id":"evidence_auth_guard","path":"apps/api/src/routes/repositories.ts","start_line":87}]}\n'
`)
	createHTTPAPIAgentConfigWithCommand(t, queries, "agent_config_followup", "verifier", 1, command, agents.OutputJSON, `{"prompt_delivery":"stdin","timeout_seconds":30}`)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/findings/finding_auth/question", map[string]any{
		"question":        "Can you re-check the guard evidence?",
		"agent_config_id": "agent_config_followup",
		"context_policy": map[string]any{
			"max_tokens": 4000,
			"max_items":  24,
		},
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("question status = %d, body = %s", response.Code, response.Body.String())
	}
	answer := decodeAskFindingQuestionResponse(t, response.Body.Bytes())
	if answer.Thread.Finding.ID != "finding_auth" ||
		len(answer.Thread.Messages) != 2 ||
		answer.UserMessage.Role != "user" ||
		answer.AssistantMessage.Role != "assistant" ||
		answer.AssistantMessage.AgentConfigID != "agent_config_followup" ||
		!strings.Contains(answer.AssistantMessage.Content, "supports the auth finding") ||
		string(answer.AssistantMessage.EvidenceRefs) == "[]" ||
		answer.AssistantMessage.ArtifactID == "" ||
		answer.AgentRunID == "" ||
		answer.ContextBundleID == "" {
		t.Fatalf("answer = %+v", answer)
	}
	runs, err := queries.ListAgentRunsBySession(context.Background(), "review_session_findings")
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	foundRun := false
	for _, run := range runs {
		if run.ID == answer.AgentRunID &&
			run.Role == "follow_up" &&
			run.Status == "succeeded" &&
			run.ContextBundleID.Valid &&
			run.StdoutArtifactID.Valid {
			foundRun = true
			break
		}
	}
	if !foundRun {
		t.Fatalf("agent runs = %+v, want follow-up run %s", runs, answer.AgentRunID)
	}

	reloadRequest := httptest.NewRequest(http.MethodGet, "/api/findings/finding_auth/thread", nil)
	reloadRequest.Header.Set("X-Cocode-Token", "test-token")
	reloadResponse := httptest.NewRecorder()
	router.ServeHTTP(reloadResponse, reloadRequest)
	if reloadResponse.Code != http.StatusOK {
		t.Fatalf("thread reload status = %d, body = %s", reloadResponse.Code, reloadResponse.Body.String())
	}
	reloaded := decodeFindingThreadViewResponse(t, reloadResponse.Body.Bytes())
	if len(reloaded.Messages) != 2 ||
		reloaded.Messages[0].Content != "Can you re-check the guard evidence?" ||
		reloaded.Messages[1].ArtifactID != answer.AssistantMessage.ArtifactID {
		t.Fatalf("reloaded = %+v", reloaded)
	}
}

func TestEvidenceMapQuestionEndpointRunsVerifierWithGraphContext(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)
	if err := os.MkdirAll("/tmp/cocode", 0o755); err != nil {
		t.Fatalf("mkdir repo root: %v", err)
	}
	command := writeFakeAgentConfigCommand(t, `#!/bin/sh
cat >/dev/null
printf '{"answer":"The graph path still shows the missing guard edge.","evidence_refs":[{"node_id":"graph-node","path":"apps/api/src/routes/repositories.ts","start_line":87}]}\n'
`)
	createHTTPAPIAgentConfigWithCommand(t, queries, "agent_config_graph_verifier", "verifier", 1, command, agents.OutputJSON, `{"prompt_delivery":"stdin","timeout_seconds":30}`)

	mapRequest := httptest.NewRequest(http.MethodGet, "/api/findings/finding_auth/evidence-map", nil)
	mapRequest.Header.Set("X-Cocode-Token", "test-token")
	mapResponse := httptest.NewRecorder()
	router.ServeHTTP(mapResponse, mapRequest)
	if mapResponse.Code != http.StatusOK {
		t.Fatalf("evidence map status = %d, body = %s", mapResponse.Code, mapResponse.Body.String())
	}
	graph := decodeFindingEvidenceMapResponse(t, mapResponse.Body.Bytes())
	if len(graph.Nodes) == 0 {
		t.Fatalf("graph nodes = %+v", graph.Nodes)
	}

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/findings/finding_auth/evidence-map/question", map[string]any{
		"question":        "Does this graph path prove the missing guard?",
		"agent_config_id": "agent_config_graph_verifier",
		"graph_refs": []map[string]any{{
			"node_id": graph.Nodes[0].ID,
		}},
		"context_policy": map[string]any{
			"max_tokens": 5000,
			"max_items":  30,
		},
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("evidence map question status = %d, body = %s", response.Code, response.Body.String())
	}
	answer := decodeAskFindingQuestionResponse(t, response.Body.Bytes())
	if answer.Thread.Finding.ID != "finding_auth" ||
		answer.UserMessage.Role != "user" ||
		!strings.Contains(string(answer.UserMessage.EvidenceRefs), graph.Nodes[0].ID) ||
		answer.AssistantMessage.AgentConfigID != "agent_config_graph_verifier" ||
		!strings.Contains(answer.AssistantMessage.Content, "missing guard edge") ||
		answer.AgentRunID == "" ||
		answer.ContextBundleID == "" {
		t.Fatalf("answer = %+v", answer)
	}
	bundle, err := queries.GetContextBundle(context.Background(), answer.ContextBundleID)
	if err != nil {
		t.Fatalf("GetContextBundle() error = %v", err)
	}
	if bundle.Scope != string(contextbundle.ScopeEvidenceMap) {
		t.Fatalf("bundle scope = %q", bundle.Scope)
	}
	runs, err := queries.ListAgentRunsBySession(context.Background(), "review_session_findings")
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	foundRun := false
	for _, run := range runs {
		if run.ID == answer.AgentRunID && run.Role == "verifier" && run.ContextBundleID.Valid && run.ContextBundleID.String == answer.ContextBundleID {
			foundRun = true
			break
		}
	}
	if !foundRun {
		t.Fatalf("agent runs = %+v, want verifier run %s", runs, answer.AgentRunID)
	}
}

func TestEvidenceMapQuestionRejectsUnknownGraphRef(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	mapRequest := httptest.NewRequest(http.MethodGet, "/api/findings/finding_auth/evidence-map", nil)
	mapRequest.Header.Set("X-Cocode-Token", "test-token")
	mapResponse := httptest.NewRecorder()
	router.ServeHTTP(mapResponse, mapRequest)
	if mapResponse.Code != http.StatusOK {
		t.Fatalf("evidence map status = %d, body = %s", mapResponse.Code, mapResponse.Body.String())
	}
	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/findings/finding_auth/evidence-map/question", map[string]any{
		"question":        "Can you inspect this node?",
		"agent_config_id": "agent_config_findings",
		"graph_refs": []map[string]any{{
			"node_id": "node_missing",
		}},
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestFindingQuickActionEndpointAcceptsAndAppendsThreadMessage(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/findings/finding_budget/thread/actions", map[string]any{
		"action": "accept",
		"reason": "the preview overflow is a real UI regression",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("quick action status = %d, body = %s", response.Code, response.Body.String())
	}
	action := decodeFindingQuickActionResponse(t, response.Body.Bytes())
	if action.Action != "accept" ||
		action.Finding.DecisionStatus != "accepted" ||
		action.Decision == nil ||
		action.Decision.Decision != "accepted" ||
		action.Message == nil ||
		action.Message.Role != "system" ||
		!strings.Contains(action.Message.Content, "Accepted finding") ||
		len(action.Thread.Messages) != 1 {
		t.Fatalf("action = %+v", action)
	}
	stored, err := queries.GetFinding(context.Background(), "finding_budget")
	if err != nil {
		t.Fatalf("GetFinding() error = %v", err)
	}
	if stored.DecisionStatus != "accepted" {
		t.Fatalf("stored finding = %+v", stored)
	}
	decisions, err := queries.ListHumanDecisionsByFinding(context.Background(), "finding_budget")
	if err != nil {
		t.Fatalf("ListHumanDecisionsByFinding() error = %v", err)
	}
	if len(decisions) != 1 ||
		decisions[0].Decision != "accepted" ||
		!strings.Contains(decisions[0].MetadataJson, `"follow_up_quick_action"`) {
		t.Fatalf("decisions = %+v", decisions)
	}
}

func TestFindingQuickActionEndpointMarksCopied(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/findings/finding_auth/thread/actions", map[string]any{
		"action": "copy",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("copy quick action status = %d, body = %s", response.Code, response.Body.String())
	}
	action := decodeFindingQuickActionResponse(t, response.Body.Bytes())
	if action.Finding.DecisionStatus != "copied" ||
		action.Decision == nil ||
		action.Decision.Decision != "copied" ||
		action.Message == nil ||
		!strings.Contains(action.Message.Content, "copied") {
		t.Fatalf("action = %+v", action)
	}
}

func TestFindingQuickActionEndpointAsksCounterEvidence(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)
	if err := os.MkdirAll("/tmp/cocode", 0o755); err != nil {
		t.Fatalf("mkdir repo root: %v", err)
	}
	command := writeFakeAgentConfigCommand(t, `#!/bin/sh
cat >/dev/null
printf '{"answer":"I found no counter-evidence in the scoped bundle.","evidence_refs":[{"evidence_item_id":"evidence_auth_guard"}]}\n'
`)
	createHTTPAPIAgentConfigWithCommand(t, queries, "agent_config_counter", "verifier", 1, command, agents.OutputJSON, `{"prompt_delivery":"stdin","timeout_seconds":30}`)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/findings/finding_auth/thread/actions", map[string]any{
		"action":          "ask_counter_evidence",
		"agent_config_id": "agent_config_counter",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("counter-evidence status = %d, body = %s", response.Code, response.Body.String())
	}
	action := decodeFindingQuickActionResponse(t, response.Body.Bytes())
	if action.Action != "ask_counter_evidence" ||
		action.Message == nil ||
		action.Message.Role != "user" ||
		!strings.Contains(action.Message.Content, "counter-evidence") ||
		action.AssistantMessage == nil ||
		!strings.Contains(action.AssistantMessage.Content, "no counter-evidence") ||
		string(action.AssistantMessage.EvidenceRefs) == "[]" ||
		action.AgentRunID == "" ||
		action.ContextBundleID == "" ||
		len(action.Thread.Messages) != 2 {
		t.Fatalf("action = %+v", action)
	}
}

func TestFindingQuickActionEndpointRequiresDismissalReason(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/findings/finding_budget/thread/actions", map[string]any{
		"action": "dismiss",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("dismiss quick action status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestFindingContextPreviewEndpointBuildsScopedBundle(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/findings/finding_auth/context-bundles/preview", map[string]any{
		"persist": true,
		"context_policy": map[string]any{
			"max_tokens": 4000,
			"max_items":  20,
		},
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("finding context status = %d, body = %s", response.Code, response.Body.String())
	}
	preview := decodeBuildReviewContextResponse(t, response.Body.Bytes())
	if !preview.Persisted ||
		preview.Bundle.Scope != contextbundle.ScopeFinding ||
		preview.Bundle.ArtifactID == "" ||
		preview.ArtifactID == "" ||
		preview.Bundle.ItemCount == 0 ||
		!hasContextBundleItemKind(preview.Bundle.Items, contextbundle.ItemEvidence) {
		t.Fatalf("preview = %+v", preview)
	}
}

func TestEvidenceMapContextPreviewEndpointIncludesGraph(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)
	if _, err := queries.CreateEvidenceGraph(context.Background(), dbgen.CreateEvidenceGraphParams{
		ID:              "graph_auth_context",
		FindingID:       "finding_auth",
		ReviewSessionID: "review_session_findings",
		Status:          "ready",
		LayoutJson:      `{"direction":"LR"}`,
		Summary:         nullableString("Auth evidence map context."),
		CreatedAt:       "2026-05-03T00:16:00Z",
		UpdatedAt:       "2026-05-03T00:16:00Z",
	}); err != nil {
		t.Fatalf("CreateEvidenceGraph() error = %v", err)
	}
	if _, err := queries.CreateEvidenceNode(context.Background(), dbgen.CreateEvidenceNodeParams{
		ID:              "node_auth_context_route",
		EvidenceGraphID: "graph_auth_context",
		Kind:            "changed_code",
		Label:           "Repository settings route",
		Path:            nullableString("apps/api/src/routes/repositories.ts"),
		StartLine:       sql.NullInt64{Int64: 87, Valid: true},
		EndLine:         sql.NullInt64{Int64: 112, Valid: true},
		EvidenceItemID:  nullableString("evidence_auth_guard"),
		Confidence:      0.9,
		MetadataJson:    `{}`,
	}); err != nil {
		t.Fatalf("CreateEvidenceNode() error = %v", err)
	}

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/findings/finding_auth/evidence-map/context-bundles/preview", map[string]any{
		"context_policy": map[string]any{
			"max_tokens": 5000,
			"max_items":  30,
		},
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("evidence map context status = %d, body = %s", response.Code, response.Body.String())
	}
	preview := decodeBuildReviewContextResponse(t, response.Body.Bytes())
	if preview.Bundle.Scope != contextbundle.ScopeEvidenceMap ||
		!hasContextBundleItemTitle(preview.Bundle.Items, "Evidence Map graph") {
		t.Fatalf("preview = %+v", preview)
	}
}

func TestFindingDecisionEndpointUpdatesStatusAndAppendsDecision(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := newAuthenticatedJSONRequest(t, http.MethodPatch, "/api/findings/finding_budget/decision", map[string]any{
		"decision": "accepted",
		"reason":   "valid UI risk",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("decision status = %d, body = %s", response.Code, response.Body.String())
	}
	detail := decodeFindingDetailResponse(t, response.Body.Bytes())
	if detail.Finding.DecisionStatus != "accepted" ||
		len(detail.Decisions) != 1 ||
		detail.Decisions[0].Decision != "accepted" {
		t.Fatalf("detail = %+v", detail)
	}
	stored, err := queries.GetFinding(context.Background(), "finding_budget")
	if err != nil {
		t.Fatalf("GetFinding() error = %v", err)
	}
	if stored.DecisionStatus != "accepted" {
		t.Fatalf("stored finding = %+v", stored)
	}
}

func TestFindingDecisionEndpointRequiresDismissalReason(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := newAuthenticatedJSONRequest(t, http.MethodPatch, "/api/findings/finding_budget/decision", map[string]any{
		"decision": "dismissed",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("dismiss status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestFindingDecisionEndpointCreatesReviewRuleMemory(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	body := map[string]any{
		"decision":               "dismissed",
		"reason":                 "generated fixture churn",
		"rule_memory_suggestion": "Do not flag generated fixture churn in renderer snapshots.",
	}
	request := newAuthenticatedJSONRequest(t, http.MethodPatch, "/api/findings/finding_budget/decision", body)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("dismiss status = %d, body = %s", response.Code, response.Body.String())
	}
	detail := decodeFindingDetailResponse(t, response.Body.Bytes())
	if detail.Finding.DecisionStatus != "dismissed" {
		t.Fatalf("detail finding = %+v", detail.Finding)
	}
	if len(detail.Decisions) == 0 {
		t.Fatalf("expected stored human decision")
	}
	var metadata map[string]string
	if err := json.Unmarshal(detail.Decisions[0].Metadata, &metadata); err != nil {
		t.Fatalf("decode decision metadata: %v", err)
	}
	if metadata["review_rule_id"] == "" {
		t.Fatalf("metadata missing review_rule_id: %+v", metadata)
	}

	rules, err := queries.ListReviewRulesByWorkspace(context.Background(), "workspace_1")
	if err != nil {
		t.Fatalf("ListReviewRulesByWorkspace() error = %v", err)
	}
	if len(rules) != 1 ||
		rules[0].Scope != "workspace" ||
		rules[0].RuleType != "dismissal" ||
		rules[0].Enabled != 1 ||
		!strings.Contains(rules[0].Content, "generated fixture churn") {
		t.Fatalf("rules = %+v", rules)
	}

	secondRequest := newAuthenticatedJSONRequest(t, http.MethodPatch, "/api/findings/finding_budget/decision", body)
	secondResponse := httptest.NewRecorder()
	router.ServeHTTP(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusOK {
		t.Fatalf("second dismiss status = %d, body = %s", secondResponse.Code, secondResponse.Body.String())
	}
	rules, err = queries.ListReviewRulesByWorkspace(context.Background(), "workspace_1")
	if err != nil {
		t.Fatalf("ListReviewRulesByWorkspace(second) error = %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("duplicate dismissal created rules = %+v", rules)
	}
}

func TestReviewRuleEndpointCRUDAndDedup(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIWorkspaceAndRepository(t, queries, "/tmp/cocode")

	createRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/workspaces/workspace_1/review-rules", map[string]any{
		"scope":     "workspace",
		"rule_type": "dismissal",
		"content":   "Do not flag generated file formatting noise.",
		"enabled":   true,
	})
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create rule status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	created := decodeReviewRuleResponse(t, createResponse.Body.Bytes())
	if created.ID == "" ||
		created.WorkspaceID != "workspace_1" ||
		created.Scope != "workspace" ||
		created.RuleType != "dismissal" ||
		!created.Enabled {
		t.Fatalf("created rule = %+v", created)
	}

	duplicateRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/workspaces/workspace_1/review-rules", map[string]any{
		"content": " do not flag GENERATED file formatting noise. ",
	})
	duplicateResponse := httptest.NewRecorder()
	router.ServeHTTP(duplicateResponse, duplicateRequest)
	if duplicateResponse.Code != http.StatusOK {
		t.Fatalf("duplicate rule status = %d, body = %s", duplicateResponse.Code, duplicateResponse.Body.String())
	}
	duplicate := decodeReviewRuleResponse(t, duplicateResponse.Body.Bytes())
	if duplicate.ID != created.ID {
		t.Fatalf("duplicate ID = %q, want %q", duplicate.ID, created.ID)
	}

	disableRequest := newAuthenticatedJSONRequest(t, http.MethodPatch, "/api/review-rules/"+created.ID+"/enabled", map[string]any{
		"enabled": false,
	})
	disableResponse := httptest.NewRecorder()
	router.ServeHTTP(disableResponse, disableRequest)
	if disableResponse.Code != http.StatusOK {
		t.Fatalf("disable rule status = %d, body = %s", disableResponse.Code, disableResponse.Body.String())
	}
	disabled := decodeReviewRuleResponse(t, disableResponse.Body.Bytes())
	if disabled.Enabled {
		t.Fatalf("disabled rule = %+v", disabled)
	}

	updateRequest := newAuthenticatedJSONRequest(t, http.MethodPatch, "/api/review-rules/"+created.ID, map[string]any{
		"scope":     "repository",
		"rule_type": "review_guidance",
		"content":   "Prefer deterministic renderer snapshots for UI-only churn.",
		"enabled":   true,
	})
	updateResponse := httptest.NewRecorder()
	router.ServeHTTP(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update rule status = %d, body = %s", updateResponse.Code, updateResponse.Body.String())
	}
	updated := decodeReviewRuleResponse(t, updateResponse.Body.Bytes())
	if updated.Scope != "repository" ||
		updated.RuleType != "review_guidance" ||
		!updated.Enabled ||
		!strings.Contains(updated.Content, "deterministic") {
		t.Fatalf("updated rule = %+v", updated)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/workspaces/workspace_1/review-rules", nil)
	listRequest.Header.Set("X-Cocode-Token", "test-token")
	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list rule status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	list := decodeReviewRuleListResponse(t, listResponse.Body.Bytes())
	if len(list.Items) != 1 || list.Items[0].ID != created.ID {
		t.Fatalf("list = %+v", list)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/review-rules/"+created.ID, nil)
	deleteRequest.Header.Set("X-Cocode-Token", "test-token")
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete rule status = %d, body = %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if _, err := queries.GetReviewRule(context.Background(), created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetReviewRule(deleted) error = %v, want sql.ErrNoRows", err)
	}
}

func TestSettingsExportImportEndpointRedactsAndValidates(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIWorkspaceAndRepository(t, queries, "/tmp/cocode")
	createHTTPAPIAgentConfigWithCommand(t, queries, "agent_config_secret", "primary_reviewer", 1, "codex", agents.OutputJSON, `{
		"timeout_seconds": 900,
		"credential_refs": {"OPENAI_API_KEY": "credential:openai"},
		"api_key": "raw-secret"
	}`)
	if _, err := queries.CreateReviewRule(context.Background(), dbgen.CreateReviewRuleParams{
		ID:          "review_rule_secret",
		WorkspaceID: "workspace_1",
		Scope:       "workspace",
		RuleType:    "dismissal",
		Content:     "Do not flag generated lockfile churn.",
		Enabled:     1,
		CreatedAt:   "2026-05-03T00:16:00Z",
		UpdatedAt:   "2026-05-03T00:16:00Z",
	}); err != nil {
		t.Fatalf("CreateReviewRule() error = %v", err)
	}

	exportRequest := httptest.NewRequest(http.MethodGet, "/api/workspaces/workspace_1/settings-export", nil)
	exportRequest.Header.Set("X-Cocode-Token", "test-token")
	exportResponse := httptest.NewRecorder()
	router.ServeHTTP(exportResponse, exportRequest)
	if exportResponse.Code != http.StatusOK {
		t.Fatalf("export status = %d, body = %s", exportResponse.Code, exportResponse.Body.String())
	}
	exported := decodeSettingsExportResponse(t, exportResponse.Body.Bytes())
	if exported.Schema != settingsExportSchema ||
		len(exported.AgentPresets) == 0 ||
		len(exported.AgentConfigs) != 1 ||
		len(exported.ReviewRules) != 1 {
		t.Fatalf("exported = %+v", exported)
	}
	exportedBytes, err := json.Marshal(exported)
	if err != nil {
		t.Fatalf("Marshal(exported) error = %v", err)
	}
	if strings.Contains(string(exportedBytes), "raw-secret") ||
		strings.Contains(string(exportedBytes), "credential_refs") {
		t.Fatalf("export leaked secret material: %s", string(exportedBytes))
	}

	importPayload := SettingsExportPayload{
		Schema: settingsExportSchema,
		WorkspaceSettings: map[string]any{
			"theme":   "light",
			"api_key": "must-not-import",
		},
		AgentConfigs: []SettingsAgentConfigExport{
			{
				Name:         "Imported reviewer",
				Role:         "primary_reviewer",
				AdapterKind:  agents.AdapterCLINonInteractive,
				Command:      "codex",
				Args:         []string{"exec", "--json", "-"},
				CWDMode:      "repo_root",
				EnvAllowlist: []string{"PATH", "OPENAI_API_KEY"},
				OutputMode:   agents.OutputJSON,
				Capabilities: map[string]any{
					"supports_json": true,
					"can_read":      true,
					"output_modes":  []any{"json"},
				},
				Settings: map[string]any{
					"timeout_seconds": 600,
					"credential_refs": map[string]any{
						"OPENAI_API_KEY": "credential:openai",
					},
				},
				Enabled: true,
			},
		},
		ReviewRules: []SettingsReviewRuleExport{
			{
				Scope:    "workspace",
				RuleType: "dismissal",
				Content:  "Do not flag generated fixture churn.",
				Enabled:  true,
			},
		},
	}
	importRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/workspaces/workspace_1/settings-import", map[string]any{
		"payload":          importPayload,
		"collision_policy": "skip",
	})
	importResponse := httptest.NewRecorder()
	router.ServeHTTP(importResponse, importRequest)
	if importResponse.Code != http.StatusOK {
		t.Fatalf("import status = %d, body = %s", importResponse.Code, importResponse.Body.String())
	}
	imported := decodeSettingsImportResponse(t, importResponse.Body.Bytes())
	if imported.WorkspaceSettings.Created != 1 ||
		imported.WorkspaceSettings.Redacted != 1 ||
		imported.AgentConfigs.Created != 1 ||
		imported.AgentConfigs.Redacted != 1 ||
		imported.ReviewRules.Created != 1 {
		t.Fatalf("imported = %+v", imported)
	}

	workspace, err := queries.GetWorkspace(context.Background(), "workspace_1")
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if !strings.Contains(workspace.SettingsJson, `"theme":"light"`) ||
		strings.Contains(workspace.SettingsJson, "api_key") {
		t.Fatalf("workspace settings = %s", workspace.SettingsJson)
	}
	configs, err := queries.ListAgentConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListAgentConfigs() error = %v", err)
	}
	importedConfig := dbgen.AgentConfig{}
	for _, config := range configs {
		if config.Name == "Imported reviewer" {
			importedConfig = config
			break
		}
	}
	if importedConfig.ID == "" ||
		!strings.Contains(importedConfig.SettingsJson, "timeout_seconds") ||
		strings.Contains(importedConfig.SettingsJson, "credential_refs") {
		t.Fatalf("imported config = %+v", importedConfig)
	}
	rules, err := queries.ListReviewRulesByWorkspace(context.Background(), "workspace_1")
	if err != nil {
		t.Fatalf("ListReviewRulesByWorkspace() error = %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("rules = %+v", rules)
	}

	invalidRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/workspaces/workspace_1/settings-import", map[string]any{
		"payload": map[string]any{"schema": "wrong"},
	})
	invalidResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidResponse, invalidRequest)
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid import status = %d, body = %s", invalidResponse.Code, invalidResponse.Body.String())
	}
}

func TestReviewSessionCopyPacketEndpointRendersAcceptedFindings(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions/review_session_findings/export/copy-packet", map[string]any{
		"format": "markdown",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("copy packet status = %d, body = %s", response.Code, response.Body.String())
	}
	packet := decodeCreateCopyPacketResponse(t, response.Body.Bytes())
	if packet.CopyPacketID == "" ||
		packet.ContentArtifactID == "" ||
		packet.Format != "markdown" ||
		packet.FindingCount != 1 ||
		packet.TokenEstimate == 0 ||
		!strings.Contains(packet.Content, "Repository settings updates miss the workspace admin guard") ||
		strings.Contains(packet.Content, "Renderer preview can load") {
		t.Fatalf("packet = %+v", packet)
	}
	stored, err := queries.GetCopyPacket(context.Background(), packet.CopyPacketID)
	if err != nil {
		t.Fatalf("GetCopyPacket() error = %v", err)
	}
	if stored.ContentArtifactID != packet.ContentArtifactID ||
		!stored.FindingID.Valid ||
		stored.FindingID.String != "finding_auth" {
		t.Fatalf("stored packet = %+v", stored)
	}
}

func TestReviewSessionCopyPacketEndpointPreservesSelectedFindingOrder(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions/review_session_findings/export/copy-packet", map[string]any{
		"format":      "compact",
		"finding_ids": []string{"finding_budget", "finding_auth", "finding_budget"},
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("selected copy packet status = %d, body = %s", response.Code, response.Body.String())
	}
	packet := decodeCreateCopyPacketResponse(t, response.Body.Bytes())
	budgetIndex := strings.Index(packet.Content, "Renderer preview can load")
	authIndex := strings.Index(packet.Content, "Repository settings updates miss")
	if packet.Format != "compact" ||
		packet.FindingCount != 2 ||
		budgetIndex < 0 ||
		authIndex < 0 ||
		budgetIndex > authIndex {
		t.Fatalf("packet = %+v", packet)
	}
	stored, err := queries.GetCopyPacket(context.Background(), packet.CopyPacketID)
	if err != nil {
		t.Fatalf("GetCopyPacket() error = %v", err)
	}
	if stored.FindingID.Valid {
		t.Fatalf("selected packet should not store one finding id: %+v", stored)
	}
}

func TestFindingCopyPacketEndpointRendersSingleJSON(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/findings/finding_budget/export/copy-packet", map[string]any{
		"format":                   "json",
		"include_code_snippets":    true,
		"include_evidence":         true,
		"include_counter_evidence": true,
		"target_agent":             "codex_cli",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("finding copy packet status = %d, body = %s", response.Code, response.Body.String())
	}
	packet := decodeCreateCopyPacketResponse(t, response.Body.Bytes())
	var payload struct {
		Findings []struct {
			ID    string `json:"id"`
			Claim string `json:"claim"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(packet.Content), &payload); err != nil {
		t.Fatalf("packet JSON parse error = %v\n%s", err, packet.Content)
	}
	if packet.Format != "json" ||
		packet.FindingCount != 1 ||
		len(payload.Findings) != 1 ||
		payload.Findings[0].ID != "finding_budget" ||
		!strings.Contains(payload.Findings[0].Claim, "Renderer preview") {
		t.Fatalf("packet = %+v payload = %+v", packet, payload)
	}
	stored, err := queries.GetCopyPacket(context.Background(), packet.CopyPacketID)
	if err != nil {
		t.Fatalf("GetCopyPacket() error = %v", err)
	}
	if !stored.FindingID.Valid || stored.FindingID.String != "finding_budget" {
		t.Fatalf("stored packet = %+v", stored)
	}
}

func TestCopyPacketCopiedEndpointMarksPacketAndFindings(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	createRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions/review_session_findings/export/copy-packet", map[string]any{
		"format":      "markdown",
		"finding_ids": []string{"finding_budget", "finding_auth"},
	})
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("copy packet status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	packet := decodeCreateCopyPacketResponse(t, createResponse.Body.Bytes())

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/copy-packets/"+packet.CopyPacketID+"/copied", map[string]any{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("mark copied status = %d, body = %s", response.Code, response.Body.String())
	}
	copied := decodeMarkCopyPacketCopiedResponse(t, response.Body.Bytes())
	if copied.CopyPacketID != packet.CopyPacketID ||
		copied.CopiedAt == "" ||
		len(copied.FindingIDs) != 2 ||
		len(copied.Decisions) != 2 {
		t.Fatalf("copied = %+v", copied)
	}
	stored, err := queries.GetCopyPacket(context.Background(), packet.CopyPacketID)
	if err != nil {
		t.Fatalf("GetCopyPacket() error = %v", err)
	}
	if !stored.CopiedAt.Valid {
		t.Fatalf("stored packet = %+v", stored)
	}
	for _, findingID := range []string{"finding_budget", "finding_auth"} {
		finding, err := queries.GetFinding(context.Background(), findingID)
		if err != nil {
			t.Fatalf("GetFinding(%s) error = %v", findingID, err)
		}
		if finding.DecisionStatus != "copied" {
			t.Fatalf("finding %s = %+v", findingID, finding)
		}
	}
}

func TestGitHubPreviewEndpointCreatesDraftWithWarnings(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions/review_session_findings/github/preview", map[string]any{
		"finding_ids":  []string{"finding_auth"},
		"review_event": "COMMENT",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("github preview status = %d, body = %s", response.Code, response.Body.String())
	}
	preview := decodeGitHubPreviewResponse(t, response.Body.Bytes())
	if preview.PublishDraftID == "" ||
		preview.ArtifactID == "" ||
		preview.ReviewEvent != "COMMENT" ||
		len(preview.Comments) != 1 ||
		len(preview.Warnings) == 0 ||
		!preview.Checklist.HasSelectedFindings ||
		!preview.Checklist.HasUnanchoredComments ||
		preview.Checklist.CanPublishInline ||
		!preview.Checklist.CanPublishSummaryOnly ||
		!strings.Contains(preview.Body, "Repository settings updates") {
		t.Fatalf("preview = %+v", preview)
	}
	draft, err := queries.GetPublishDraft(context.Background(), preview.PublishDraftID)
	if err != nil {
		t.Fatalf("GetPublishDraft() error = %v", err)
	}
	if !draft.ArtifactID.Valid ||
		draft.ArtifactID.String != preview.ArtifactID ||
		draft.CommentsJson == "" ||
		!strings.Contains(nullableValue(draft.Body), "Repository settings updates") {
		t.Fatalf("draft = %+v", draft)
	}
}

func TestGitHubPreviewEndpointRejectsUnpublishableAcceptedFinding(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)
	if _, err := queries.UpdateFindingDecisionStatus(context.Background(), dbgen.UpdateFindingDecisionStatusParams{
		ID:             "finding_budget",
		DecisionStatus: "accepted",
		UpdatedAt:      "2026-05-03T00:20:00Z",
	}); err != nil {
		t.Fatalf("UpdateFindingDecisionStatus() error = %v", err)
	}

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions/review_session_findings/github/preview", map[string]any{
		"finding_ids": []string{"finding_budget"},
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "finding is not publishable") {
		t.Fatalf("github preview status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestReviewSessionAuditLogEndpointCombinesReviewActions(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)
	if _, err := queries.CreateEvent(context.Background(), dbgen.CreateEventParams{
		ID:              "event_audit_1",
		ReviewSessionID: nullableString("review_session_findings"),
		AgentRunID:      nullableString("agent_run_findings"),
		Type:            "FindingMerged",
		Level:           "info",
		Sequence:        1,
		PayloadJson:     `{"finding_id":"finding_auth"}`,
		CreatedAt:       "2026-05-03T00:16:00Z",
	}); err != nil {
		t.Fatalf("CreateEvent() error = %v", err)
	}

	previewRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions/review_session_findings/github/preview", map[string]any{
		"finding_ids":  []string{"finding_auth"},
		"review_event": "COMMENT",
	})
	previewResponse := httptest.NewRecorder()
	router.ServeHTTP(previewResponse, previewRequest)
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("github preview status = %d, body = %s", previewResponse.Code, previewResponse.Body.String())
	}
	preview := decodeGitHubPreviewResponse(t, previewResponse.Body.Bytes())
	if _, err := queries.CreateGitHubPublication(context.Background(), dbgen.CreateGitHubPublicationParams{
		ID:                   "github_publication_audit",
		ReviewSessionID:      "review_session_findings",
		PublishDraftID:       nullableString(preview.PublishDraftID),
		GithubReviewID:       nullableString("12345"),
		GithubCommentIdsJson: `["100","101"]`,
		Status:               "submitted",
		CreatedAt:            "2026-05-03T00:18:00Z",
	}); err != nil {
		t.Fatalf("CreateGitHubPublication() error = %v", err)
	}

	copyRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions/review_session_findings/export/copy-packet", map[string]any{
		"format":      "markdown",
		"finding_ids": []string{"finding_auth"},
	})
	copyResponse := httptest.NewRecorder()
	router.ServeHTTP(copyResponse, copyRequest)
	if copyResponse.Code != http.StatusOK {
		t.Fatalf("copy packet status = %d, body = %s", copyResponse.Code, copyResponse.Body.String())
	}
	packet := decodeCreateCopyPacketResponse(t, copyResponse.Body.Bytes())

	copiedRequest := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/copy-packets/"+packet.CopyPacketID+"/copied", map[string]any{})
	copiedResponse := httptest.NewRecorder()
	router.ServeHTTP(copiedResponse, copiedRequest)
	if copiedResponse.Code != http.StatusOK {
		t.Fatalf("mark copied status = %d, body = %s", copiedResponse.Code, copiedResponse.Body.String())
	}

	request := httptest.NewRequest(http.MethodGet, "/api/review-sessions/review_session_findings/audit-log", nil)
	request.Header.Set("X-Cocode-Token", "test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("audit status = %d, body = %s", response.Code, response.Body.String())
	}
	audit := decodeAuditLogResponse(t, response.Body.Bytes())
	kinds := map[string]AuditLogEntryResponse{}
	for _, entry := range audit.Entries {
		kinds[entry.Kind] = entry
		if entry.ReviewSessionID != "review_session_findings" {
			t.Fatalf("entry missing review session: %+v", entry)
		}
	}
	for _, kind := range []string{"event", "decision", "copy_packet", "copy_packet_copied", "publish_draft", "github_publication"} {
		if _, ok := kinds[kind]; !ok {
			t.Fatalf("audit entries missing %s: %+v", kind, audit.Entries)
		}
	}
	var publishMetadata map[string]any
	if err := json.Unmarshal(kinds["publish_draft"].Metadata, &publishMetadata); err != nil {
		t.Fatalf("decode publish metadata: %v", err)
	}
	if publishMetadata["comment_count"] != float64(1) || publishMetadata["comments_json"] != nil {
		t.Fatalf("publish metadata = %+v", publishMetadata)
	}
	var publicationMetadata map[string]any
	if err := json.Unmarshal(kinds["github_publication"].Metadata, &publicationMetadata); err != nil {
		t.Fatalf("decode publication metadata: %v", err)
	}
	if publicationMetadata["github_comment_ids"] != float64(2) {
		t.Fatalf("publication metadata = %+v", publicationMetadata)
	}
}

func TestGitHubPreviewEndpointRejectsUnacceptedFinding(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions/review_session_findings/github/preview", map[string]any{
		"finding_ids": []string{"finding_budget"},
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "accepted before GitHub preview") {
		t.Fatalf("github preview status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestGitHubPreviewEndpointRejectsAlreadyPublishedFinding(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)
	if _, err := queries.UpdateFindingDecisionStatus(context.Background(), dbgen.UpdateFindingDecisionStatusParams{
		ID:             "finding_auth",
		DecisionStatus: "published",
		UpdatedAt:      "2026-05-03T00:20:00Z",
	}); err != nil {
		t.Fatalf("UpdateFindingDecisionStatus() error = %v", err)
	}

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions/review_session_findings/github/preview", map[string]any{
		"finding_ids": []string{"finding_auth"},
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("github preview status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestFindingDraftCommentEndpointPersistsEdit(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPIFindingFixture(t, queries)

	request := newAuthenticatedJSONRequest(t, http.MethodPatch, "/api/findings/finding_budget/draft-comment", map[string]any{
		"draft_comment": "Please clamp the preview to keep the review list scannable.",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("draft status = %d, body = %s", response.Code, response.Body.String())
	}
	finding := decodeFindingResponse(t, response.Body.Bytes())
	if finding.DraftComment != "Please clamp the preview to keep the review list scannable." {
		t.Fatalf("finding = %+v", finding)
	}
	decisions, err := queries.ListHumanDecisionsByFinding(context.Background(), "finding_budget")
	if err != nil {
		t.Fatalf("ListHumanDecisionsByFinding() error = %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "edited" {
		t.Fatalf("decisions = %+v", decisions)
	}
}

func TestReviewSessionEventsEndpointStreamsLiveWorkflowEvents(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	repoPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoPath, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	writeHTTPAPIDefaultRepo(t, repoPath)
	createHTTPAPISnapshotAt(t, queries, repoPath)
	createHTTPAPIAgentConfigWithCommand(t, queries, "agent_config_fake", "primary_reviewer", 1, fakeJSONAgentPath(t), agents.OutputJSON, `{"prompt_delivery":"stdin","timeout_seconds":30}`)
	session := createHTTPAPIReviewSessionRow(t, queries, "review_session_sse", []string{"agent_config_fake"})

	server := httptest.NewServer(router)
	defer server.Close()

	sseCtx, cancelSSE := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelSSE()
	sseRequest, err := http.NewRequestWithContext(sseCtx, http.MethodGet, server.URL+"/api/review-sessions/"+session.ID+"/events", nil)
	if err != nil {
		t.Fatalf("NewRequest(events) error = %v", err)
	}
	sseRequest.Header.Set("X-Cocode-Token", "test-token")
	responseCh := make(chan struct {
		response *http.Response
		err      error
	}, 1)
	go func() {
		response, err := server.Client().Do(sseRequest)
		responseCh <- struct {
			response *http.Response
			err      error
		}{response: response, err: err}
	}()

	startRequest, err := http.NewRequest(http.MethodPost, server.URL+"/api/review-sessions/"+session.ID+"/start", nil)
	if err != nil {
		t.Fatalf("NewRequest(start) error = %v", err)
	}
	startRequest.Header.Set("X-Cocode-Token", "test-token")
	startResponse, err := server.Client().Do(startRequest)
	if err != nil {
		t.Fatalf("POST start error = %v", err)
	}
	_ = startResponse.Body.Close()
	if startResponse.StatusCode != http.StatusOK {
		t.Fatalf("start status = %d", startResponse.StatusCode)
	}

	result := <-responseCh
	if result.err != nil {
		t.Fatalf("GET events error = %v", result.err)
	}
	defer result.response.Body.Close()
	if result.response.StatusCode != http.StatusOK {
		t.Fatalf("events status = %d", result.response.StatusCode)
	}
	scanner := bufio.NewScanner(result.response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id: ") && strings.TrimPrefix(line, "id: ") == "0" {
			t.Fatalf("SSE id should be a positive sequence, got %q", line)
		}
		if strings.HasPrefix(line, "event: ") && line != "event: review.event" {
			t.Fatalf("unexpected SSE event line %q", line)
		}
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"type":"ReviewSessionCompleted"`) {
			cancelSSE()
			break
		}
	}
	if err := scanner.Err(); err != nil && sseCtx.Err() == nil {
		t.Fatalf("scan SSE stream: %v", err)
	}
	waitForHTTPAPIReviewSessionStatus(t, queries, session.ID, "completed")

	replayCtx, cancelReplay := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelReplay()
	replayRequest, err := http.NewRequestWithContext(replayCtx, http.MethodGet, server.URL+"/api/review-sessions/"+session.ID+"/events", nil)
	if err != nil {
		t.Fatalf("NewRequest(replay) error = %v", err)
	}
	replayRequest.Header.Set("X-Cocode-Token", "test-token")
	replayRequest.Header.Set("Last-Event-ID", "1")
	replayResponse, err := server.Client().Do(replayRequest)
	if err != nil {
		t.Fatalf("GET replay events error = %v", err)
	}
	defer replayResponse.Body.Close()
	replayScanner := bufio.NewScanner(replayResponse.Body)
	for replayScanner.Scan() {
		line := replayScanner.Text()
		if strings.HasPrefix(line, "id: ") {
			if line == "id: 1" {
				t.Fatalf("replayed event did not honor Last-Event-ID: %q", line)
			}
			cancelReplay()
			return
		}
	}
	if err := replayScanner.Err(); err != nil && replayCtx.Err() == nil {
		t.Fatalf("scan replay SSE stream: %v", err)
	}
	t.Fatal("replay stream did not emit an event after Last-Event-ID")
}

func TestCancelReviewSessionEndpointStopsRunningWorkflow(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	repoPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoPath, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "src", "new.go"), []byte("package src\n\nfunc RequireAdmin() bool { return true }\n"), 0o644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}
	createHTTPAPISnapshotAt(t, queries, repoPath)
	createHTTPAPIAgentConfigWithCommand(t, queries, "agent_config_slow", "primary_reviewer", 1, writeSlowHTTPAPIAgent(t), agents.OutputJSON, `{"prompt_delivery":"stdin","timeout_seconds":30}`)
	session := createHTTPAPIReviewSessionRow(t, queries, "review_session_cancel", []string{"agent_config_slow"})

	startRequest := httptest.NewRequest(http.MethodPost, "/api/review-sessions/"+session.ID+"/start", nil)
	startRequest.Header.Set("X-Cocode-Token", "test-token")
	startResponse := httptest.NewRecorder()
	router.ServeHTTP(startResponse, startRequest)
	if startResponse.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", startResponse.Code, startResponse.Body.String())
	}
	waitForHTTPAPIAgentRunStatus(t, queries, session.ID, "running")

	cancelRequest := httptest.NewRequest(http.MethodPost, "/api/review-sessions/"+session.ID+"/cancel", nil)
	cancelRequest.Header.Set("X-Cocode-Token", "test-token")
	cancelResponse := httptest.NewRecorder()
	router.ServeHTTP(cancelResponse, cancelRequest)
	if cancelResponse.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, body = %s", cancelResponse.Code, cancelResponse.Body.String())
	}
	canceled := waitForHTTPAPIReviewSessionStatus(t, queries, session.ID, "canceled")
	if !canceled.CompletedAt.Valid {
		t.Fatalf("canceled session missing completed_at: %+v", canceled)
	}
	runs, err := queries.ListAgentRunsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "canceled" {
		t.Fatalf("agent runs = %+v", runs)
	}
	events, err := queries.ListEventsByReviewSession(context.Background(), nullableString(session.ID))
	if err != nil {
		t.Fatalf("ListEventsByReviewSession() error = %v", err)
	}
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.Type] = true
	}
	for _, typ := range []string{"ReviewSessionCancelRequested", "AgentRunCanceled", "ReviewSessionCanceled"} {
		if !seen[typ] {
			t.Fatalf("events missing %s: %+v", typ, events)
		}
	}
}

func TestCancelAgentRunEndpointStopsOneRunningAgent(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	repoPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoPath, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "src", "new.go"), []byte("package src\n\nfunc RequireAdmin() bool { return true }\n"), 0o644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}
	createHTTPAPISnapshotAt(t, queries, repoPath)
	createHTTPAPIAgentConfigWithCommand(t, queries, "agent_config_slow_a", "primary_reviewer", 1, writeSlowHTTPAPIAgent(t), agents.OutputJSON, `{"prompt_delivery":"stdin","timeout_seconds":30}`)
	createHTTPAPIAgentConfigWithCommand(t, queries, "agent_config_slow_b", "secondary_reviewer", 1, writeSlowHTTPAPIAgent(t), agents.OutputJSON, `{"prompt_delivery":"stdin","timeout_seconds":30}`)
	session := createHTTPAPIReviewSessionRow(t, queries, "review_session_cancel_one", []string{"agent_config_slow_a", "agent_config_slow_b"})

	startRequest := httptest.NewRequest(http.MethodPost, "/api/review-sessions/"+session.ID+"/start", nil)
	startRequest.Header.Set("X-Cocode-Token", "test-token")
	startResponse := httptest.NewRecorder()
	router.ServeHTTP(startResponse, startRequest)
	if startResponse.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", startResponse.Code, startResponse.Body.String())
	}
	running := waitForHTTPAPIAgentRunCount(t, queries, session.ID, "running", 2)
	targetRun := running[0]

	cancelRequest := httptest.NewRequest(http.MethodPost, "/api/review-sessions/"+session.ID+"/agent-runs/"+targetRun.ID+"/cancel", nil)
	cancelRequest.Header.Set("X-Cocode-Token", "test-token")
	cancelResponse := httptest.NewRecorder()
	router.ServeHTTP(cancelResponse, cancelRequest)
	if cancelResponse.Code != http.StatusOK {
		t.Fatalf("cancel agent status = %d, body = %s", cancelResponse.Code, cancelResponse.Body.String())
	}
	canceledRun := decodeAgentRunResponse(t, cancelResponse.Body.Bytes())
	if canceledRun.ID != targetRun.ID || canceledRun.ReviewSessionID != session.ID {
		t.Fatalf("cancel response = %+v", canceledRun)
	}
	waitForHTTPAPIAgentRunIDStatus(t, queries, targetRun.ID, "canceled")

	runs, err := queries.ListAgentRunsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	statusByID := map[string]string{}
	for _, run := range runs {
		statusByID[run.ID] = run.Status
	}
	if statusByID[targetRun.ID] != "canceled" {
		t.Fatalf("runs = %+v", runs)
	}
	otherActive := false
	for _, run := range running[1:] {
		if statusByID[run.ID] == "running" {
			otherActive = true
		}
	}
	if !otherActive {
		t.Fatalf("canceling one run stopped all runs: %+v", runs)
	}
	events, err := queries.ListEventsByReviewSession(context.Background(), nullableString(session.ID))
	if err != nil {
		t.Fatalf("ListEventsByReviewSession() error = %v", err)
	}
	seenRequest := false
	for _, event := range events {
		if event.Type == "AgentRunCancelRequested" && event.AgentRunID.Valid && event.AgentRunID.String == targetRun.ID {
			seenRequest = true
			break
		}
	}
	if !seenRequest {
		t.Fatalf("missing AgentRunCancelRequested for %s: %+v", targetRun.ID, events)
	}

	cancelSessionRequest := httptest.NewRequest(http.MethodPost, "/api/review-sessions/"+session.ID+"/cancel", nil)
	cancelSessionRequest.Header.Set("X-Cocode-Token", "test-token")
	cancelSessionResponse := httptest.NewRecorder()
	router.ServeHTTP(cancelSessionResponse, cancelSessionRequest)
	if cancelSessionResponse.Code != http.StatusOK {
		t.Fatalf("cleanup cancel session status = %d, body = %s", cancelSessionResponse.Code, cancelSessionResponse.Body.String())
	}
}

func TestPauseResumeReviewSessionEndpoint(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	repoPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoPath, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "src", "new.go"), []byte("package src\n\nfunc RequireAdmin() bool { return true }\n"), 0o644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}
	createHTTPAPISnapshotAt(t, queries, repoPath)
	createHTTPAPIAgentConfigWithCommand(t, queries, "agent_config_pause", "primary_reviewer", 1, writeSleepHTTPAPIAgent(t, "1"), agents.OutputJSON, `{"prompt_delivery":"stdin","timeout_seconds":30}`)
	session := createHTTPAPIReviewSessionRow(t, queries, "review_session_pause", []string{"agent_config_pause"})

	startRequest := httptest.NewRequest(http.MethodPost, "/api/review-sessions/"+session.ID+"/start", nil)
	startRequest.Header.Set("X-Cocode-Token", "test-token")
	startResponse := httptest.NewRecorder()
	router.ServeHTTP(startResponse, startRequest)
	if startResponse.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", startResponse.Code, startResponse.Body.String())
	}
	waitForHTTPAPIAgentRunStatus(t, queries, session.ID, "running")

	pauseRequest := httptest.NewRequest(http.MethodPost, "/api/review-sessions/"+session.ID+"/pause", nil)
	pauseRequest.Header.Set("X-Cocode-Token", "test-token")
	pauseResponse := httptest.NewRecorder()
	router.ServeHTTP(pauseResponse, pauseRequest)
	if pauseResponse.Code != http.StatusOK {
		t.Fatalf("pause status = %d, body = %s", pauseResponse.Code, pauseResponse.Body.String())
	}
	paused := decodeReviewSessionResponse(t, pauseResponse.Body.Bytes())
	if paused.Status != "paused" {
		t.Fatalf("paused response = %+v", paused)
	}

	resumeRequest := httptest.NewRequest(http.MethodPost, "/api/review-sessions/"+session.ID+"/resume", nil)
	resumeRequest.Header.Set("X-Cocode-Token", "test-token")
	resumeResponse := httptest.NewRecorder()
	router.ServeHTTP(resumeResponse, resumeRequest)
	if resumeResponse.Code != http.StatusOK {
		t.Fatalf("resume status = %d, body = %s", resumeResponse.Code, resumeResponse.Body.String())
	}
	resumed := decodeReviewSessionResponse(t, resumeResponse.Body.Bytes())
	if resumed.Status != "running" {
		t.Fatalf("resumed response = %+v", resumed)
	}
	waitForHTTPAPIReviewSessionStatus(t, queries, session.ID, "completed")
	events, err := queries.ListEventsByReviewSession(context.Background(), nullableString(session.ID))
	if err != nil {
		t.Fatalf("ListEventsByReviewSession() error = %v", err)
	}
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.Type] = true
	}
	for _, typ := range []string{"ReviewSessionPaused", "ReviewSessionResumed", "ReviewSessionCompleted"} {
		if !seen[typ] {
			t.Fatalf("events missing %s: %+v", typ, events)
		}
	}
}

func TestBuildReviewContextPreviewEndpointPersistsBundle(t *testing.T) {
	repoPath := t.TempDir()
	writeHTTPAPIRepoFile(t, repoPath, "app/main.go", "package main\n\nconst apiKey = \"sk-abcdefghijklmnopqrstuvwxyz\"\n\nfunc RequireAdmin() {}\n")
	writeHTTPAPIRepoFile(t, repoPath, "app/main_test.go", "package main\n\nfunc TestRequireAdmin(t *testing.T) {\n\tRequireAdmin()\n}\n")

	artifactDir := filepath.Join(t.TempDir(), "artifacts")
	router, queries := testRouterWithConfigAndQueries(t, app.Config{ArtifactDir: artifactDir})
	createHTTPAPIWorkspaceAndRepository(t, queries, repoPath)
	if _, err := queries.CreatePullRequestSnapshot(context.Background(), dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_1",
		RepositoryID: "repo_1",
		SourceType:   "local_changes",
		MetadataJson: "{}",
		CreatedAt:    "2026-05-03T00:02:00Z",
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}
	if _, err := queries.CreateReviewSession(context.Background(), dbgen.CreateReviewSessionParams{
		ID:                  "review_session_1",
		WorkspaceID:         "workspace_1",
		RepositoryID:        "repo_1",
		SnapshotID:          "snapshot_1",
		Title:               "Review auth",
		Status:              "draft",
		ReviewDepth:         "standard",
		FocusPrompt:         nullableString("Focus auth guard behavior."),
		RuntimeLimitSeconds: 1800,
		ContextPolicyJson:   `{"include_related_call_sites":false,"include_project_conventions":false}`,
		CreatedAt:           "2026-05-03T00:03:00Z",
		UpdatedAt:           "2026-05-03T00:03:00Z",
	}); err != nil {
		t.Fatalf("CreateReviewSession() error = %v", err)
	}
	store, err := artifact.New(artifactDir, queries)
	if err != nil {
		t.Fatalf("artifact.New() error = %v", err)
	}
	patch, err := store.Save(context.Background(), artifact.SaveParams{
		ID:           "artifact_patch_main",
		WorkspaceID:  "workspace_1",
		Kind:         "patch",
		RelativePath: "snapshots/snapshot_1/patches/main.patch",
		ContentType:  "text/x-diff",
		MetadataJSON: `{"path":"app/main.go"}`,
		CreatedAt:    "2026-05-03T00:04:00Z",
	}, []byte("@@ -1,3 +1,5 @@\n package main\n+const apiKey = \"sk-abcdefghijklmnopqrstuvwxyz\"\n+func RequireAdmin() {}\n"))
	if err != nil {
		t.Fatalf("Save(patch) error = %v", err)
	}
	if _, err := queries.CreateChangedFile(context.Background(), dbgen.CreateChangedFileParams{
		ID:              "file_main",
		SnapshotID:      "snapshot_1",
		Path:            "app/main.go",
		Status:          "modified",
		Additions:       2,
		LineRangesJson:  `[[3,5]]`,
		PatchArtifactID: sql.NullString{String: patch.ID, Valid: true},
		CreatedAt:       "2026-05-03T00:05:00Z",
	}); err != nil {
		t.Fatalf("CreateChangedFile() error = %v", err)
	}

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions/review_session_1/context-bundles/preview", map[string]any{
		"persist": true,
		"context_policy": map[string]any{
			"include_prior_comments":  false,
			"include_prior_decisions": false,
			"max_tokens":              50000,
			"max_items":               50,
		},
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	preview := decodeBuildReviewContextResponse(t, response.Body.Bytes())
	if !preview.Persisted ||
		preview.ArtifactID == "" ||
		preview.Bundle.ArtifactID != preview.ArtifactID ||
		preview.Bundle.ItemCount != int64(len(preview.Bundle.Items)) ||
		preview.Bundle.TokenEstimate <= 0 ||
		preview.RedactionReport.RedactionCount == 0 {
		t.Fatalf("preview = %+v", preview)
	}
	kinds := map[string]bool{}
	for _, item := range preview.Bundle.Items {
		kinds[string(item.Kind)] = true
	}
	for _, want := range []string{"prompt_material", "changed_hunk", "full_file", "related_test"} {
		if !kinds[want] {
			t.Fatalf("preview item kinds = %+v, missing %s", kinds, want)
		}
	}
	rendered, _, err := store.Read(context.Background(), preview.ArtifactID)
	if err != nil {
		t.Fatalf("Read(context artifact) error = %v", err)
	}
	if strings.Contains(string(rendered), "sk-abcdefghijklmnopqrstuvwxyz") || !strings.Contains(string(rendered), "[REDACTED]") {
		t.Fatalf("rendered context = %s", string(rendered))
	}
	rows, err := queries.ListContextItemsByBundle(context.Background(), preview.Bundle.ID)
	if err != nil {
		t.Fatalf("ListContextItemsByBundle() error = %v", err)
	}
	if len(rows) != len(preview.Bundle.Items) {
		t.Fatalf("context item rows len = %d, want %d", len(rows), len(preview.Bundle.Items))
	}

	if _, err := queries.CreateAgentConfig(context.Background(), dbgen.CreateAgentConfigParams{
		ID:               "agent_config_1",
		Name:             "Codex reviewer",
		Role:             "reviewer",
		AdapterKind:      "cli_noninteractive",
		Command:          nullableString("codex"),
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       "json",
		CapabilitiesJson: "{}",
		SettingsJson:     "{}",
		Enabled:          1,
		CreatedAt:        "2026-05-03T00:06:00Z",
		UpdatedAt:        "2026-05-03T00:06:00Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig() error = %v", err)
	}
	if _, err := queries.CreateAgentRun(context.Background(), dbgen.CreateAgentRunParams{
		ID:              "agent_run_1",
		ReviewSessionID: "review_session_1",
		AgentConfigID:   "agent_config_1",
		ContextBundleID: sql.NullString{String: preview.Bundle.ID, Valid: true},
		Status:          "completed",
		Role:            "reviewer",
		MetadataJson:    "{}",
	}); err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	debugRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions/review_session_1/context-bundles", nil)
	debugRequest.Header.Set("X-Cocode-Token", "test-token")
	debugResponse := httptest.NewRecorder()
	router.ServeHTTP(debugResponse, debugRequest)
	if debugResponse.Code != http.StatusOK {
		t.Fatalf("debug status = %d, body = %s", debugResponse.Code, debugResponse.Body.String())
	}
	debug := decodeContextBundleDebugResponse(t, debugResponse.Body.Bytes())
	if len(debug.Bundles) != 1 ||
		debug.Bundles[0].Bundle.ID != preview.Bundle.ID ||
		debug.Bundles[0].Artifact == nil ||
		!strings.Contains(debug.Bundles[0].Artifact.Content, "[REDACTED]") ||
		len(debug.Bundles[0].AgentRunIDs) != 1 ||
		debug.Bundles[0].AgentRunIDs[0] != "agent_run_1" {
		t.Fatalf("debug response = %+v", debug)
	}
}

func TestBuildReviewContextPreviewEndpointMapsMissingSession(t *testing.T) {
	router := testRouter(t)

	request := newAuthenticatedJSONRequest(t, http.MethodPost, "/api/review-sessions/missing/context-bundles/preview", map[string]any{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing session status = %d, body = %s", response.Code, response.Body.String())
	}

	debugRequest := httptest.NewRequest(http.MethodGet, "/api/review-sessions/missing/context-bundles", nil)
	debugRequest.Header.Set("X-Cocode-Token", "test-token")
	debugResponse := httptest.NewRecorder()
	router.ServeHTTP(debugResponse, debugRequest)
	if debugResponse.Code != http.StatusNotFound {
		t.Fatalf("missing debug status = %d, body = %s", debugResponse.Code, debugResponse.Body.String())
	}
}

func TestCreateSnapshotEndpointRejectsInvalidJSON(t *testing.T) {
	router, _ := testRouterWithQueries(t)

	request := httptest.NewRequest(http.MethodPost, "/api/pr-snapshots/from-local-changes", strings.NewReader("{"))
	request.Header.Set("X-Cocode-Token", "test-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, response.Code, response.Body.String())
	}
}

func testRouter(t *testing.T) http.Handler {
	router, _ := testRouterWithQueries(t)
	return router
}

func testRouterWithQueries(t *testing.T) (http.Handler, *dbgen.Queries) {
	return testRouterWithConfigAndQueries(t, app.Config{})
}

func testRouterWithConfigAndQueries(t *testing.T, config app.Config) (http.Handler, *dbgen.Queries) {
	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))
	if config.Addr == "" {
		config.Addr = "127.0.0.1:0"
	}
	if config.AuthToken == "" {
		config.AuthToken = "test-token"
	}
	if config.DataDir == "" {
		config.DataDir = "/tmp/cocode-test"
	}
	if config.ArtifactDir == "" {
		config.ArtifactDir = filepath.Join(t.TempDir(), "artifacts")
	}
	if config.Version == "" {
		config.Version = "test-version"
	}
	return NewRouter(config, logger, database), dbgen.New(database)
}

func createHTTPAPISnapshot(t *testing.T, queries *dbgen.Queries) {
	t.Helper()

	repoPath := t.TempDir()
	writeHTTPAPIDefaultRepo(t, repoPath)
	createHTTPAPISnapshotAt(t, queries, repoPath)
}

func createHTTPAPISnapshotAt(t *testing.T, queries *dbgen.Queries, repoPath string) {
	t.Helper()

	createHTTPAPIWorkspaceAndRepository(t, queries, repoPath)
	if _, err := queries.CreatePullRequestSnapshot(context.Background(), dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_1",
		RepositoryID: "repo_1",
		SourceType:   "branch_compare",
		BaseRef:      nullableString("main"),
		HeadRef:      nullableString("feature"),
		BaseSha:      nullableString("base-sha"),
		HeadSha:      nullableString("head-sha"),
		MetadataJson: "{}",
		CreatedAt:    "2026-05-03T00:02:00Z",
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}
	if _, err := queries.CreateChangedFile(context.Background(), dbgen.CreateChangedFileParams{
		ID:              "file_2",
		SnapshotID:      "snapshot_1",
		Path:            "src/new.go",
		OldPath:         nullableString("src/old.go"),
		Status:          "renamed",
		Additions:       2,
		Deletions:       1,
		LineRangesJson:  `[[8,9]]`,
		PatchArtifactID: nullableString("artifact_patch"),
		CreatedAt:       "2026-05-03T00:03:00Z",
	}); err != nil {
		t.Fatalf("CreateChangedFile(renamed) error = %v", err)
	}
	if _, err := queries.CreateChangedFile(context.Background(), dbgen.CreateChangedFileParams{
		ID:             "file_1",
		SnapshotID:     "snapshot_1",
		Path:           "generated/api.pb.go",
		Status:         "modified",
		Additions:      4,
		IsGenerated:    1,
		IsExcluded:     1,
		LineRangesJson: `[[1,4]]`,
		CreatedAt:      "2026-05-03T00:04:00Z",
	}); err != nil {
		t.Fatalf("CreateChangedFile(generated) error = %v", err)
	}
}

func createHTTPAPIWorkspaceAndRepository(t *testing.T, queries *dbgen.Queries, repoPath string) {
	t.Helper()

	if _, err := queries.CreateWorkspace(context.Background(), dbgen.CreateWorkspaceParams{
		ID:           "workspace_1",
		Name:         "cocode",
		RootPath:     repoPath,
		SettingsJson: "{}",
		CreatedAt:    "2026-05-03T00:00:00Z",
		UpdatedAt:    "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, err := queries.CreateRepository(context.Background(), dbgen.CreateRepositoryParams{
		ID:          "repo_1",
		WorkspaceID: "workspace_1",
		Name:        "cocode",
		LocalPath:   repoPath,
		CreatedAt:   "2026-05-03T00:01:00Z",
		UpdatedAt:   "2026-05-03T00:01:00Z",
	}); err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
}

func createHTTPAPIAgentConfig(t *testing.T, queries *dbgen.Queries, id string, role string, enabled int64) {
	t.Helper()

	createHTTPAPIAgentConfigWithCommand(t, queries, id, role, enabled, "codex", agents.OutputJSON, "{}")
}

func createHTTPAPIAgentConfigWithCommand(t *testing.T, queries *dbgen.Queries, id string, role string, enabled int64, command string, outputMode agents.OutputMode, settingsJSON string) {
	t.Helper()

	if settingsJSON == "" {
		settingsJSON = "{}"
	}
	if _, err := queries.CreateAgentConfig(context.Background(), dbgen.CreateAgentConfigParams{
		ID:               id,
		Name:             id,
		Role:             role,
		AdapterKind:      "cli_noninteractive",
		Command:          nullableString(command),
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       string(outputMode),
		CapabilitiesJson: `{"supports_json":true,"can_read":true,"output_modes":["json"]}`,
		SettingsJson:     settingsJSON,
		Enabled:          enabled,
		CreatedAt:        "2026-05-03T00:06:00Z",
		UpdatedAt:        "2026-05-03T00:06:00Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig(%s) error = %v", id, err)
	}
}

func createHTTPAPIReviewSessionRow(t *testing.T, queries *dbgen.Queries, id string, agentConfigIDs []string) dbgen.ReviewSession {
	t.Helper()

	session, err := queries.CreateReviewSession(context.Background(), dbgen.CreateReviewSessionParams{
		ID:                  id,
		WorkspaceID:         "workspace_1",
		RepositoryID:        "repo_1",
		SnapshotID:          "snapshot_1",
		Title:               "Review SSE fixture",
		Status:              "draft",
		ReviewDepth:         "standard",
		RuntimeLimitSeconds: 60,
		ContextPolicyJson: `{
			"include_prompt_material": true,
			"include_changed_code": true,
			"include_related_call_sites": false,
			"include_related_tests": false,
			"include_project_conventions": false,
			"include_prior_comments": false,
			"include_prior_decisions": false,
			"redact_secrets": true,
			"max_tokens": 4096,
			"max_items": 20
		}`,
		CreatedAt: "2026-05-03T00:07:00Z",
		UpdatedAt: "2026-05-03T00:07:00Z",
	})
	if err != nil {
		t.Fatalf("CreateReviewSession() error = %v", err)
	}
	for index, agentConfigID := range agentConfigIDs {
		if _, err := queries.CreateReviewSessionAgent(context.Background(), dbgen.CreateReviewSessionAgentParams{
			ID:                   "review_session_agent_" + id + "_" + agentConfigID,
			ReviewSessionID:      id,
			AgentConfigID:        agentConfigID,
			Role:                 "primary_reviewer",
			RunOrder:             int64(index + 1),
			Enabled:              1,
			SettingsOverrideJson: "{}",
		}); err != nil {
			t.Fatalf("CreateReviewSessionAgent(%s) error = %v", agentConfigID, err)
		}
	}
	return session
}

func createHTTPAPIFindingFixture(t *testing.T, queries *dbgen.Queries) {
	t.Helper()

	createHTTPAPISnapshot(t, queries)
	createHTTPAPIAgentConfig(t, queries, "agent_config_findings", "primary_reviewer", 1)
	createHTTPAPIReviewSessionRow(t, queries, "review_session_findings", []string{"agent_config_findings"})
	if _, err := queries.CreateAgentRun(context.Background(), dbgen.CreateAgentRunParams{
		ID:              "agent_run_findings",
		ReviewSessionID: "review_session_findings",
		AgentConfigID:   "agent_config_findings",
		Status:          "succeeded",
		Role:            "primary_reviewer",
		StartedAt:       nullableString("2026-05-03T00:07:00Z"),
		CompletedAt:     nullableString("2026-05-03T00:08:00Z"),
		DurationMs:      sql.NullInt64{Int64: 1000, Valid: true},
		ExitCode:        sql.NullInt64{Int64: 0, Valid: true},
		MetadataJson:    "{}",
	}); err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	createHTTPAPIFindingCandidate(t, queries, "candidate_auth_1", "Repository settings updates miss admin guard", "security", "high", 0.91, "apps/api/src/routes/repositories.ts", "auth-guard-missing")
	createHTTPAPIFindingCandidate(t, queries, "candidate_auth_2", "The update route does not enforce admin permissions", "security", "high", 0.88, "apps/api/src/routes/repositories.ts", "auth-guard-missing")
	createHTTPAPIFindingCandidate(t, queries, "candidate_budget", "Renderer preview can load the full diff payload", "reliability", "medium", 0.72, "apps/desktop/src/renderer/src/app/App.tsx", "renderer-budget")
	createHTTPAPIFindingCandidate(t, queries, "candidate_theme", "Theme selection might not persist", "maintainability", "low", 0.38, "apps/desktop/src/renderer/src/app/App.tsx", "theme-persistence")

	createHTTPAPIFinding(t, queries, "finding_auth", "Repository settings updates miss the workspace admin guard.", "security", "high", 0.92, "verified", "accepted", "apps/api/src/routes/repositories.ts", "auth-guard-missing", 2, "This mutation appears reachable to workspace members.")
	createHTTPAPIFinding(t, queries, "finding_budget", "Renderer preview can load the full diff payload without a display budget.", "reliability", "medium", 0.74, "unverified", "undecided", "apps/desktop/src/renderer/src/app/App.tsx", "renderer-budget", 1, "Large diffs may make the board hard to scan.")
	createHTTPAPIFinding(t, queries, "finding_theme", "Theme selection might not persist after app restart.", "maintainability", "low", 0.38, "likely_false_positive", "dismissed", "apps/desktop/src/renderer/src/app/App.tsx", "theme-persistence", 1, "")

	for _, link := range []struct {
		findingID   string
		candidateID string
		relation    string
	}{
		{"finding_auth", "candidate_auth_1", "primary"},
		{"finding_auth", "candidate_auth_2", "exact_duplicate"},
		{"finding_budget", "candidate_budget", "primary"},
		{"finding_theme", "candidate_theme", "primary"},
	} {
		if err := queries.LinkFindingCandidate(context.Background(), dbgen.LinkFindingCandidateParams{
			FindingID:          link.findingID,
			FindingCandidateID: link.candidateID,
			Relation:           link.relation,
		}); err != nil {
			t.Fatalf("LinkFindingCandidate(%s, %s) error = %v", link.findingID, link.candidateID, err)
		}
	}
	if _, err := queries.CreateEvidenceItem(context.Background(), dbgen.CreateEvidenceItemParams{
		ID:           "evidence_auth_guard",
		FindingID:    "finding_auth",
		Kind:         "supporting",
		Title:        "Mutation route reaches updateSettings",
		Summary:      "The route reaches repositoryService.updateSettings after member authentication only.",
		Path:         nullableString("apps/api/src/routes/repositories.ts"),
		StartLine:    sql.NullInt64{Int64: 87, Valid: true},
		EndLine:      sql.NullInt64{Int64: 112, Valid: true},
		Confidence:   0.9,
		MetadataJson: `{"producer":"local_verifier","code_snippet":"87: router.patch('/settings', updateSettings)","line_window":{"start_line":84,"end_line":115}}`,
		CreatedAt:    "2026-05-03T00:14:00Z",
	}); err != nil {
		t.Fatalf("CreateEvidenceItem() error = %v", err)
	}
	if _, err := queries.CreateHumanDecision(context.Background(), dbgen.CreateHumanDecisionParams{
		ID:              "decision_auth_accepted",
		FindingID:       "finding_auth",
		ReviewSessionID: "review_session_findings",
		Decision:        "accepted",
		Reason:          nullableString("valid security issue"),
		MetadataJson:    "{}",
		CreatedAt:       "2026-05-03T00:15:00Z",
	}); err != nil {
		t.Fatalf("CreateHumanDecision() error = %v", err)
	}
}

func createHTTPAPIFindingCandidate(t *testing.T, queries *dbgen.Queries, id string, claim string, category string, severity string, confidence float64, path string, fingerprint string) {
	t.Helper()

	if _, err := queries.CreateFindingCandidate(context.Background(), dbgen.CreateFindingCandidateParams{
		ID:               id,
		ReviewSessionID:  "review_session_findings",
		AgentRunID:       "agent_run_findings",
		Category:         category,
		Severity:         severity,
		Confidence:       confidence,
		Claim:            claim,
		PrimaryPath:      nullableString(path),
		PrimaryStartLine: sql.NullInt64{Int64: 87, Valid: true},
		PrimaryEndLine:   sql.NullInt64{Int64: 112, Valid: true},
		LocationsJson:    `[{"path":"` + path + `","start_line":87,"end_line":112,"side":"RIGHT","changed_file_id":"file_2","valid":true}]`,
		EvidenceJson:     `[{"title":"supporting evidence","summary":"candidate evidence","kind":"changed_code"}]`,
		SuggestedFix:     nullableString("Apply the focused fix."),
		DraftComment:     nullableString("Please address this finding."),
		Fingerprint:      nullableString(fingerprint),
		CreatedAt:        "2026-05-03T00:09:00Z",
	}); err != nil {
		t.Fatalf("CreateFindingCandidate(%s) error = %v", id, err)
	}
}

func createHTTPAPIFinding(t *testing.T, queries *dbgen.Queries, id string, claim string, category string, severity string, confidence float64, verification string, decision string, path string, fingerprint string, merged int64, draftComment string) {
	t.Helper()

	if _, err := queries.CreateFinding(context.Background(), dbgen.CreateFindingParams{
		ID:                 id,
		ReviewSessionID:    "review_session_findings",
		CanonicalClaim:     claim,
		Category:           category,
		Severity:           severity,
		Confidence:         confidence,
		VerificationStatus: verification,
		DecisionStatus:     decision,
		PrimaryPath:        nullableString(path),
		PrimaryStartLine:   sql.NullInt64{Int64: 87, Valid: true},
		PrimaryEndLine:     sql.NullInt64{Int64: 112, Valid: true},
		EvidenceSummary:    nullableString("Evidence summary for " + id),
		SuggestedFix:       nullableString("Suggested fix for " + id),
		DraftComment:       nullableString(draftComment),
		Fingerprint:        fingerprint,
		MergedFromCount:    merged,
		IntroducedInSha:    nullableString("head-sha"),
		FirstSeenAt:        "2026-05-03T00:10:00Z",
		UpdatedAt:          "2026-05-03T00:11:00Z",
	}); err != nil {
		t.Fatalf("CreateFinding(%s) error = %v", id, err)
	}
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

func waitForHTTPAPIReviewSessionStatus(t *testing.T, queries *dbgen.Queries, id string, status string) dbgen.ReviewSession {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		session, err := queries.GetReviewSession(context.Background(), id)
		if err != nil {
			t.Fatalf("GetReviewSession(%s) error = %v", id, err)
		}
		if session.Status == status {
			return session
		}
		if session.Status == "failed" || session.Status == "canceled" {
			t.Fatalf(
				"review session ended as %s, want %s: %+v\n%s",
				session.Status,
				status,
				session,
				httpAPIReviewSessionDiagnostics(t, queries, id),
			)
		}
		time.Sleep(20 * time.Millisecond)
	}
	session, err := queries.GetReviewSession(context.Background(), id)
	if err != nil {
		t.Fatalf("GetReviewSession(%s) after timeout error = %v", id, err)
	}
	t.Fatalf(
		"review session status = %s after timeout, want %s: %+v\n%s",
		session.Status,
		status,
		session,
		httpAPIReviewSessionDiagnostics(t, queries, id),
	)
	return dbgen.ReviewSession{}
}

func httpAPIReviewSessionDiagnostics(t *testing.T, queries *dbgen.Queries, reviewSessionID string) string {
	t.Helper()

	var builder strings.Builder
	runs, err := queries.ListAgentRunsBySession(context.Background(), reviewSessionID)
	if err != nil {
		fmt.Fprintf(&builder, "agent runs: error=%v\n", err)
	} else {
		fmt.Fprintf(&builder, "agent runs: count=%d\n", len(runs))
		for _, run := range runs {
			fmt.Fprintf(
				&builder,
				"- id=%s role=%s status=%s exit=%s error_code=%s error_message=%s stdout=%s stderr=%s parsed=%s metadata=%s\n",
				run.ID,
				run.Role,
				run.Status,
				nullInt64Diagnostic(run.ExitCode),
				nullStringDiagnostic(run.ErrorCode),
				nullStringDiagnostic(run.ErrorMessage),
				nullStringDiagnostic(run.StdoutArtifactID),
				nullStringDiagnostic(run.StderrArtifactID),
				nullStringDiagnostic(run.ParsedOutputArtifactID),
				run.MetadataJson,
			)
		}
	}

	events, err := queries.ListEventsByReviewSession(context.Background(), nullableString(reviewSessionID))
	if err != nil {
		fmt.Fprintf(&builder, "events: error=%v\n", err)
	} else {
		fmt.Fprintf(&builder, "events: count=%d\n", len(events))
		for _, event := range events {
			fmt.Fprintf(
				&builder,
				"- seq=%d type=%s level=%s agent_run=%s artifact=%s payload=%s\n",
				event.Sequence,
				event.Type,
				event.Level,
				nullStringDiagnostic(event.AgentRunID),
				nullStringDiagnostic(event.ArtifactID),
				event.PayloadJson,
			)
		}
	}

	return builder.String()
}

func nullStringDiagnostic(value sql.NullString) string {
	if !value.Valid {
		return "<null>"
	}
	return value.String
}

func nullInt64Diagnostic(value sql.NullInt64) string {
	if !value.Valid {
		return "<null>"
	}
	return fmt.Sprintf("%d", value.Int64)
}

func waitForHTTPAPIAgentRunStatus(t *testing.T, queries *dbgen.Queries, reviewSessionID string, status string) dbgen.AgentRun {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := queries.ListAgentRunsBySession(context.Background(), reviewSessionID)
		if err != nil {
			t.Fatalf("ListAgentRunsBySession(%s) error = %v", reviewSessionID, err)
		}
		for _, run := range runs {
			if run.Status == status {
				return run
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	runs, err := queries.ListAgentRunsBySession(context.Background(), reviewSessionID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession(%s) after timeout error = %v", reviewSessionID, err)
	}
	t.Fatalf("no agent run reached status %s after timeout: %+v", status, runs)
	return dbgen.AgentRun{}
}

func waitForHTTPAPIAgentRunIDStatus(t *testing.T, queries *dbgen.Queries, runID string, status string) dbgen.AgentRun {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		run, err := queries.GetAgentRun(context.Background(), runID)
		if err != nil {
			t.Fatalf("GetAgentRun(%s) error = %v", runID, err)
		}
		if run.Status == status {
			return run
		}
		time.Sleep(20 * time.Millisecond)
	}
	run, err := queries.GetAgentRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetAgentRun(%s) after timeout error = %v", runID, err)
	}
	t.Fatalf("agent run %s status = %s after timeout, want %s", runID, run.Status, status)
	return dbgen.AgentRun{}
}

func waitForHTTPAPIAgentRunCount(t *testing.T, queries *dbgen.Queries, reviewSessionID string, status string, count int) []dbgen.AgentRun {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := queries.ListAgentRunsBySession(context.Background(), reviewSessionID)
		if err != nil {
			t.Fatalf("ListAgentRunsBySession(%s) error = %v", reviewSessionID, err)
		}
		matching := make([]dbgen.AgentRun, 0, len(runs))
		for _, run := range runs {
			if run.Status == status {
				matching = append(matching, run)
			}
		}
		if len(matching) >= count {
			return matching
		}
		time.Sleep(20 * time.Millisecond)
	}
	runs, err := queries.ListAgentRunsBySession(context.Background(), reviewSessionID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession(%s) after timeout error = %v", reviewSessionID, err)
	}
	t.Fatalf("agent runs with status %s = %+v after timeout, want %d", status, runs, count)
	return nil
}

func fakeJSONAgentPath(t *testing.T) string {
	t.Helper()

	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fake agent path: runtime caller unavailable")
	}
	path := filepath.Join(filepath.Dir(source), "..", "..", "..", "..", "testdata", "fake-agents", "json-agent.sh")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod fake agent: %v", err)
	}
	return path
}

func writeSlowHTTPAPIAgent(t *testing.T) string {
	t.Helper()

	return writeSleepHTTPAPIAgent(t, "10")
}

func writeSleepHTTPAPIAgent(t *testing.T, seconds string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "sleep-agent.sh")
	content := "#!/bin/sh\nset -eu\n/bin/cat >/dev/null || true\n/bin/sleep " + seconds + "\nprintf '{\"findings\":[]}'\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write sleep agent: %v", err)
	}
	return path
}

func newAuthenticatedJSONRequest(t *testing.T, method string, path string, body any) *http.Request {
	t.Helper()

	content, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(content))
	request.Header.Set("X-Cocode-Token", "test-token")
	request.Header.Set("Content-Type", "application/json")
	return request
}

func decodeSnapshotResponse(t *testing.T, content []byte) SnapshotResponse {
	t.Helper()

	var envelope struct {
		Data  SnapshotResponse `json:"data"`
		Error any              `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeBuildReviewContextResponse(t *testing.T, content []byte) BuildReviewContextResponse {
	t.Helper()

	var envelope struct {
		Data  BuildReviewContextResponse `json:"data"`
		Error any                        `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeReviewSessionResponse(t *testing.T, content []byte) ReviewSessionResponse {
	t.Helper()

	var envelope struct {
		Data  ReviewSessionResponse `json:"data"`
		Error any                   `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeReviewSessionListResponse(t *testing.T, content []byte) []ReviewSessionResponse {
	t.Helper()

	var envelope struct {
		Data  []ReviewSessionResponse `json:"data"`
		Error any                     `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

type chatThreadViewTestResponse struct {
	Thread struct {
		ID              string `json:"id"`
		ReviewSessionID string `json:"review_session_id"`
	} `json:"thread"`
	Messages []chatMessageTestResponse `json:"messages"`
}

type chatTurnTestResponse struct {
	Status   string `json:"status"`
	Audience string `json:"audience"`
}

type chatTurnEnvelopeTestResponse struct {
	Thread struct {
		ID              string `json:"id"`
		ReviewSessionID string `json:"review_session_id"`
	} `json:"thread"`
	Messages []chatMessageTestResponse `json:"messages"`
	Turn     chatTurnTestResponse      `json:"turn"`
}

type chatMessageTestResponse struct {
	AuthorType string `json:"author_type"`
	Body       string `json:"body"`
}

func decodeChatThreadViewResponse(t *testing.T, content []byte) chatThreadViewTestResponse {
	t.Helper()

	var envelope struct {
		Data  chatThreadViewTestResponse `json:"data"`
		Error any                        `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeChatTurnResponse(t *testing.T, content []byte) chatTurnEnvelopeTestResponse {
	t.Helper()

	var envelope struct {
		Data  chatTurnEnvelopeTestResponse `json:"data"`
		Error any                          `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

type reviewCheckpointTestResponse struct {
	Status      string `json:"status"`
	Phase       string `json:"phase"`
	PhaseStatus string `json:"phase_status"`
}

type reviewSummaryTestResponse struct {
	Status            string         `json:"status"`
	ProgressPercent   int            `json:"progress_percent"`
	ChangedFilesTotal int            `json:"changed_files_total"`
	ActiveAgents      int            `json:"active_agents"`
	AgentStatusCounts map[string]int `json:"agent_status_counts"`
}

func decodeReviewCheckpointResponse(t *testing.T, content []byte) reviewCheckpointTestResponse {
	t.Helper()

	var envelope struct {
		Data  reviewCheckpointTestResponse `json:"data"`
		Error any                          `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeReviewSummaryResponse(t *testing.T, content []byte) reviewSummaryTestResponse {
	t.Helper()

	var envelope struct {
		Data  reviewSummaryTestResponse `json:"data"`
		Error any                       `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeAgentRunResponse(t *testing.T, content []byte) orchestrator.AgentRun {
	t.Helper()

	var envelope struct {
		Data  orchestrator.AgentRun `json:"data"`
		Error any                   `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeFindingListResponse(t *testing.T, content []byte) FindingListResponse {
	t.Helper()

	var envelope struct {
		Data  FindingListResponse `json:"data"`
		Error any                 `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeFindingDetailResponse(t *testing.T, content []byte) FindingDetailResponse {
	t.Helper()

	var envelope struct {
		Data  FindingDetailResponse `json:"data"`
		Error any                   `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeReviewRuleResponse(t *testing.T, content []byte) ReviewRuleResponse {
	t.Helper()

	var envelope struct {
		Data  ReviewRuleResponse `json:"data"`
		Error any                `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeReviewRuleListResponse(t *testing.T, content []byte) ReviewRuleListResponse {
	t.Helper()

	var envelope struct {
		Data  ReviewRuleListResponse `json:"data"`
		Error any                    `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeSettingsExportResponse(t *testing.T, content []byte) SettingsExportPayload {
	t.Helper()

	var envelope struct {
		Data  SettingsExportPayload `json:"data"`
		Error any                   `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeSettingsImportResponse(t *testing.T, content []byte) SettingsImportResponse {
	t.Helper()

	var envelope struct {
		Data  SettingsImportResponse `json:"data"`
		Error any                    `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeFindingEvidenceResponse(t *testing.T, content []byte) FindingEvidenceResponse {
	t.Helper()

	var envelope struct {
		Data  FindingEvidenceResponse `json:"data"`
		Error any                     `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeFindingEvidenceMapResponse(t *testing.T, content []byte) FindingEvidenceMapResponse {
	t.Helper()

	var envelope struct {
		Data  FindingEvidenceMapResponse `json:"data"`
		Error any                        `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeFindingThreadViewResponse(t *testing.T, content []byte) FindingThreadViewResponse {
	t.Helper()

	var envelope struct {
		Data  FindingThreadViewResponse `json:"data"`
		Error any                       `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeAskFindingQuestionResponse(t *testing.T, content []byte) AskFindingQuestionResponse {
	t.Helper()

	var envelope struct {
		Data  AskFindingQuestionResponse `json:"data"`
		Error any                        `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeFindingQuickActionResponse(t *testing.T, content []byte) FindingQuickActionResponse {
	t.Helper()

	var envelope struct {
		Data  FindingQuickActionResponse `json:"data"`
		Error any                        `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeCreateCopyPacketResponse(t *testing.T, content []byte) CreateCopyPacketResponse {
	t.Helper()

	var envelope struct {
		Data  CreateCopyPacketResponse `json:"data"`
		Error any                      `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeMarkCopyPacketCopiedResponse(t *testing.T, content []byte) MarkCopyPacketCopiedResponse {
	t.Helper()

	var envelope struct {
		Data  MarkCopyPacketCopiedResponse `json:"data"`
		Error any                          `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeAuditLogResponse(t *testing.T, content []byte) AuditLogResponse {
	t.Helper()

	var envelope struct {
		Data  AuditLogResponse `json:"data"`
		Error any              `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeGitHubPreviewResponse(t *testing.T, content []byte) GitHubPreviewResponse {
	t.Helper()

	var envelope struct {
		Data  GitHubPreviewResponse `json:"data"`
		Error any                   `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeGitHubCredentialStatusResponse(t *testing.T, content []byte) GitHubCredentialStatusResponse {
	t.Helper()

	var envelope struct {
		Data  GitHubCredentialStatusResponse `json:"data"`
		Error any                            `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeDeleteGitHubCredentialResponse(t *testing.T, content []byte) DeleteGitHubCredentialResponse {
	t.Helper()

	var envelope struct {
		Data  DeleteGitHubCredentialResponse `json:"data"`
		Error any                            `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeFindingResponse(t *testing.T, content []byte) FindingResponse {
	t.Helper()

	var envelope struct {
		Data  FindingResponse `json:"data"`
		Error any             `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func hasEvidenceMapEdge(edges []evidencepkg.EdgeView, kind string, status string) bool {
	for _, edge := range edges {
		if edge.Kind == kind && edge.Status == status {
			return true
		}
	}
	return false
}

func hasContextBundleItemKind(items []contextbundle.Item, kind contextbundle.ItemKind) bool {
	for _, item := range items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func hasContextBundleItemTitle(items []contextbundle.Item, title string) bool {
	for _, item := range items {
		if item.Title == title {
			return true
		}
	}
	return false
}

func decodeContextBundleDebugResponse(t *testing.T, content []byte) ContextBundleDebugResponse {
	t.Helper()

	var envelope struct {
		Data  ContextBundleDebugResponse `json:"data"`
		Error any                        `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeAgentConfigResponse(t *testing.T, content []byte) AgentConfigResponse {
	t.Helper()

	var envelope struct {
		Data  AgentConfigResponse `json:"data"`
		Error any                 `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeAgentConfigListResponse(t *testing.T, content []byte) []AgentConfigResponse {
	t.Helper()

	var envelope struct {
		Data  []AgentConfigResponse `json:"data"`
		Error any                   `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeAgentConfigHealthResponse(t *testing.T, content []byte) AgentConfigHealthResponse {
	t.Helper()

	var envelope struct {
		Data  AgentConfigHealthResponse `json:"data"`
		Error any                       `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("response error = %+v", envelope.Error)
	}
	return envelope.Data
}

func decodeAgentPresetListResponse(t *testing.T, content []byte) []AgentPresetResponse {
	t.Helper()

	var envelope struct {
		Data  []AgentPresetResponse `json:"data"`
		Error any                   `json:"error"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("Unmarshal(agent preset list) error = %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("agent preset list error = %+v", envelope.Error)
	}
	return envelope.Data
}

func findAgentPreset(t *testing.T, presets []AgentPresetResponse, id string) AgentPresetResponse {
	t.Helper()

	for _, preset := range presets {
		if preset.ID == id {
			return preset
		}
	}
	t.Fatalf("preset %q missing from %+v", id, presets)
	return AgentPresetResponse{}
}

func writeFakeAgentConfigCommand(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-agent")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func initHTTPAPIGitRepo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	repoPath := t.TempDir()
	runHTTPAPIGit(t, repoPath, "init")
	runHTTPAPIGit(t, repoPath, "config", "user.email", "cocode@example.com")
	runHTTPAPIGit(t, repoPath, "config", "user.name", "cocode")
	runHTTPAPIGit(t, repoPath, "config", "commit.gpgsign", "false")
	canonical, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	return canonical
}

func runHTTPAPIGit(t *testing.T, cwd string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, string(output))
	}
}

func writeHTTPAPIRepoFile(t *testing.T, repoPath string, relativePath string, content string) {
	t.Helper()
	writeHTTPAPIRepoBytes(t, repoPath, relativePath, []byte(content))
}

func writeHTTPAPIDefaultRepo(t *testing.T, repoPath string) {
	t.Helper()

	writeHTTPAPIRepoFile(t, repoPath, "src/new.go", "package src\n\nfunc RequireAdmin() bool { return true }\n")
	writeHTTPAPIRepoFile(t, repoPath, "apps/api/src/routes/repositories.ts", numberedHTTPAPIFile(130, "export const repositoriesRoute = true;"))
	writeHTTPAPIRepoFile(t, repoPath, "apps/desktop/src/renderer/src/app/App.tsx", numberedHTTPAPIFile(130, "export function App() { return null; }"))
}

func numberedHTTPAPIFile(lines int, marker string) string {
	var builder strings.Builder
	for line := 1; line <= lines; line++ {
		if line == 87 {
			builder.WriteString(marker)
		} else {
			builder.WriteString("// fixture line")
		}
		builder.WriteByte('\n')
	}
	return builder.String()
}

func writeHTTPAPIRepoBytes(t *testing.T, repoPath string, relativePath string, content []byte) {
	t.Helper()

	path := filepath.Join(repoPath, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", relativePath, err)
	}
}
