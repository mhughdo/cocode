package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/app"
	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
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
			"prompt_delivery": "stdin",
			"timeout_seconds": 600,
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
		len(codex.Args) != 3 ||
		codex.Args[0] != "exec" ||
		codex.Args[1] != "--json" ||
		codex.Args[2] != "-" ||
		codex.OutputMode != agents.OutputJSONL ||
		codex.ModelLabel != "gpt-5.3-codex" ||
		!codex.Capabilities.CanCancel ||
		!codex.Capabilities.SupportsOutputMode(agents.OutputJSONL) ||
		!json.Valid(codex.Settings) {
		t.Fatalf("codex preset = %+v", codex)
	}
	claude := findAgentPreset(t, presets, "claude-code-cli")
	if claude.Command != "claude" ||
		len(claude.Args) != 4 ||
		claude.Args[0] != "-p" ||
		claude.Args[1] != agents.PromptArgPlaceholder ||
		claude.Args[2] != "--output-format" ||
		claude.Args[3] != "json" ||
		claude.OutputMode != agents.OutputJSON ||
		claude.ModelLabel != "claude" ||
		!claude.Capabilities.SupportsOutputMode(agents.OutputJSON) ||
		!json.Valid(claude.Settings) {
		t.Fatalf("claude preset = %+v", claude)
	}
	gemini := findAgentPreset(t, presets, "gemini-cli")
	if gemini.Command != "gemini" ||
		len(gemini.Args) != 4 ||
		gemini.Args[0] != "--model" ||
		gemini.Args[1] != "pro" ||
		gemini.Args[2] != "--output-format" ||
		gemini.Args[3] != "json" ||
		gemini.OutputMode != agents.OutputJSON ||
		gemini.ModelLabel != "pro" ||
		!gemini.Capabilities.SupportsOutputMode(agents.OutputJSON) ||
		!json.Valid(gemini.Settings) {
		t.Fatalf("gemini preset = %+v", gemini)
	}
	opencode := findAgentPreset(t, presets, "opencode-cli")
	if opencode.Command != "opencode" ||
		len(opencode.Args) != 4 ||
		opencode.Args[0] != "run" ||
		opencode.Args[1] != "--format" ||
		opencode.Args[2] != "json" ||
		opencode.Args[3] != agents.PromptArgPlaceholder ||
		opencode.OutputMode != agents.OutputJSONL ||
		opencode.ModelLabel != "opencode" ||
		!opencode.Capabilities.SupportsOutputMode(agents.OutputJSONL) ||
		!json.Valid(opencode.Settings) {
		t.Fatalf("opencode preset = %+v", opencode)
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
			"prompt_delivery": "arg",
			"version_args":    []string{"--ping"},
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
		"workspace_id":          "workspace_1",
		"snapshot_id":           "snapshot_1",
		"title":                 "Review auth changes",
		"review_depth":          "deep",
		"preset":                "security_sensitive",
		"focus_prompt":          "Focus auth guard behavior.",
		"agent_config_ids":      []string{"agent_config_codex", "agent_config_verifier"},
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
		len(created.Agents) != 2 ||
		created.Agents[0].AgentConfigID != "agent_config_codex" ||
		created.Agents[0].RunOrder != 1 ||
		created.Agents[1].AgentConfigID != "agent_config_verifier" ||
		created.Agents[1].Role != "verifier" {
		t.Fatalf("created session = %+v", created)
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
	if got.ID != created.ID || len(got.Agents) != 2 {
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

func TestReviewSessionCreateRejectsInvalidInputs(t *testing.T) {
	router, queries := testRouterWithQueries(t)
	createHTTPAPISnapshot(t, queries)
	createHTTPAPIAgentConfig(t, queries, "agent_config_disabled", "reviewer", 0)

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
	if err := os.WriteFile(filepath.Join(repoPath, "src", "new.go"), []byte("package src\n\nfunc RequireAdmin() bool { return true }\n"), 0o644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}
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
		detail.EvidenceItems[0].CodeSnippet == "" ||
		detail.EvidenceItems[0].LineWindow == nil ||
		detail.EvidenceItems[0].LineWindow.StartLine != 84 ||
		len(detail.Decisions) != 1 ||
		detail.Decisions[0].Decision != "accepted" {
		t.Fatalf("detail = %+v", detail)
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
	if err := os.WriteFile(filepath.Join(repoPath, "src", "new.go"), []byte("package src\n\nfunc RequireAdmin() bool { return true }\n"), 0o644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}
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

	createHTTPAPISnapshotAt(t, queries, "/tmp/cocode")
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
			t.Fatalf("review session ended as %s, want %s: %+v", session.Status, status, session)
		}
		time.Sleep(20 * time.Millisecond)
	}
	session, err := queries.GetReviewSession(context.Background(), id)
	if err != nil {
		t.Fatalf("GetReviewSession(%s) after timeout error = %v", id, err)
	}
	t.Fatalf("review session status = %s after timeout, want %s", session.Status, status)
	return dbgen.ReviewSession{}
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

func fakeJSONAgentPath(t *testing.T) string {
	t.Helper()

	path, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "testdata", "fake-agents", "json-agent.sh"))
	if err != nil {
		t.Fatalf("resolve fake agent path: %v", err)
	}
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
