package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/app"
)

func TestHealthEndpoint(t *testing.T) {
	router := testRouter()

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
	router := testRouter()

	request := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
}

func TestAuthenticatedRouteRejectsMissingToken(t *testing.T) {
	router := testRouter()

	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestAuthenticatedRouteAcceptsToken(t *testing.T) {
	router := testRouter()

	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
}

func testRouter() http.Handler {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))
	return NewRouter(app.Config{
		Addr:      "127.0.0.1:0",
		AuthToken: "test-token",
		DataDir:   "/tmp/cocode-test",
		Version:   "test-version",
	}, logger)
}
