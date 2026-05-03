package securitysmoke

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/app"
	"github.com/hughdo/cocode/services/cocoded/internal/contextbundle"
	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/httpapi"
	"github.com/hughdo/cocode/services/cocoded/internal/security"
)

func TestLocalAuthAndOriginSmoke(t *testing.T) {
	t.Parallel()

	handler := securitySmokeRouter(t)

	missingToken := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	missingTokenResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingTokenResponse, missingToken)
	if missingTokenResponse.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, body = %s", missingTokenResponse.Code, missingTokenResponse.Body.String())
	}

	disallowedOrigin := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	disallowedOrigin.Header.Set("Authorization", "Bearer smoke-token")
	disallowedOrigin.Header.Set("Origin", "https://example.com")
	disallowedOriginResponse := httptest.NewRecorder()
	handler.ServeHTTP(disallowedOriginResponse, disallowedOrigin)
	if disallowedOriginResponse.Code != http.StatusForbidden {
		t.Fatalf("disallowed origin status = %d, body = %s", disallowedOriginResponse.Code, disallowedOriginResponse.Body.String())
	}

	preflight := httptest.NewRequest(http.MethodOptions, "/api/session", nil)
	preflight.Header.Set("Origin", "http://127.0.0.1:5173")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodGet)
	preflightResponse := httptest.NewRecorder()
	handler.ServeHTTP(preflightResponse, preflight)
	if preflightResponse.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, body = %s", preflightResponse.Code, preflightResponse.Body.String())
	}

	allowedOrigin := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	allowedOrigin.Header.Set("Authorization", "Bearer smoke-token")
	allowedOrigin.Header.Set("Origin", "http://localhost:5173")
	allowedOriginResponse := httptest.NewRecorder()
	handler.ServeHTTP(allowedOriginResponse, allowedOrigin)
	if allowedOriginResponse.Code != http.StatusOK ||
		allowedOriginResponse.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("allowed origin status = %d headers = %+v body = %s", allowedOriginResponse.Code, allowedOriginResponse.Header(), allowedOriginResponse.Body.String())
	}
}

func TestPathSandboxSmoke(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "safe.txt"), []byte("safe"), 0o600); err != nil {
		t.Fatalf("write safe file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if _, err := security.CleanRelativePath("../secret.txt"); !errors.Is(err, security.ErrPathEscapesRoot) {
		t.Fatalf("CleanRelativePath traversal error = %v, want ErrPathEscapesRoot", err)
	}
	if _, clean, err := security.ResolveExistingWithinRoot(root, "safe.txt"); err != nil || clean != "safe.txt" {
		t.Fatalf("ResolveExistingWithinRoot(safe) clean = %q error = %v", clean, err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatalf("symlink file escape: %v", err)
	}
	if _, _, err := security.ResolveExistingWithinRoot(root, "link.txt"); !errors.Is(err, security.ErrPathEscapesRoot) {
		t.Fatalf("ResolveExistingWithinRoot(link) error = %v, want ErrPathEscapesRoot", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "out")); err != nil {
		t.Fatalf("symlink dir escape: %v", err)
	}
	if _, _, err := security.ResolveWriteWithinRoot(root, "out/artifact.txt"); !errors.Is(err, security.ErrPathEscapesRoot) {
		t.Fatalf("ResolveWriteWithinRoot(link parent) error = %v, want ErrPathEscapesRoot", err)
	}
}

func TestEnvAllowlistSmoke(t *testing.T) {
	t.Setenv("COCODE_ALLOWED_SECRET", "allowed-secret-value")
	t.Setenv("COCODE_DENIED_SECRET", "denied-secret-value")

	normalized, err := agents.NormalizeEnvAllowlist([]string{" COCODE_ALLOWED_SECRET ", "COCODE_ALLOWED_SECRET"})
	if err != nil {
		t.Fatalf("NormalizeEnvAllowlist() error = %v", err)
	}
	if len(normalized) != 1 || normalized[0] != "COCODE_ALLOWED_SECRET" {
		t.Fatalf("normalized allowlist = %+v", normalized)
	}
	env, err := agents.ResolveAllowedEnvironment(normalized)
	if err != nil {
		t.Fatalf("ResolveAllowedEnvironment() error = %v", err)
	}
	if env["COCODE_ALLOWED_SECRET"] != "allowed-secret-value" {
		t.Fatalf("allowed env = %+v", env)
	}
	if _, leaked := env["COCODE_DENIED_SECRET"]; leaked {
		t.Fatalf("denied env leaked = %+v", env)
	}
	if _, err := agents.NormalizeEnvAllowlist([]string{"BAD=VALUE"}); err == nil {
		t.Fatalf("NormalizeEnvAllowlist(BAD=VALUE) error = nil, want error")
	}
}

func TestSecretRedactionSmoke(t *testing.T) {
	t.Parallel()

	rawSecret := "sk-testsecret12345678901234567890"
	envSecret := "env-secret-value-123456"
	bundle := contextbundle.Bundle{
		ID:              "bundle_smoke",
		ReviewSessionID: "review_session_smoke",
		Scope:           contextbundle.ScopeReview,
		Items: []contextbundle.Item{{
			ID:              "item_secret",
			ContextBundleID: "bundle_smoke",
			Kind:            contextbundle.ItemFullFile,
			Path:            ".env",
			Title:           "OPENAI_API_KEY=" + rawSecret,
			Content:         "OPENAI_API_KEY=" + rawSecret + "\nFROM_ENV=" + envSecret,
			Metadata:        json.RawMessage(`{"token":"` + envSecret + `"}`),
		}},
	}

	redacted, report, err := contextbundle.RedactBundle(bundle, contextbundle.RedactionOptions{
		EnvValues: map[string]string{"FROM_ENV": envSecret},
	})
	if err != nil {
		t.Fatalf("RedactBundle() error = %v", err)
	}
	if report.RedactionCount < 3 || len(report.Items) != 1 {
		t.Fatalf("report = %+v", report)
	}
	item := redacted.Items[0]
	for _, secret := range []string{rawSecret, envSecret} {
		if strings.Contains(item.Title, secret) || strings.Contains(item.Content, secret) || strings.Contains(string(item.Metadata), secret) {
			t.Fatalf("secret %q leaked in item %+v metadata %s", secret, item, string(item.Metadata))
		}
	}
	if !strings.Contains(item.Content, "[REDACTED]") || !json.Valid(item.Metadata) {
		t.Fatalf("redacted item = %+v metadata %s", item, string(item.Metadata))
	}
}

func securitySmokeRouter(t *testing.T) http.Handler {
	t.Helper()

	database := securitySmokeDB(t)
	dataDir := t.TempDir()
	return httpapi.NewRouter(app.Config{
		AuthToken:   "smoke-token",
		DataDir:     dataDir,
		ArtifactDir: filepath.Join(dataDir, "artifacts"),
		Version:     "security-smoke",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), database)
}

func securitySmokeDB(t *testing.T) *sql.DB {
	t.Helper()

	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	return database
}
