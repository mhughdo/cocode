package githubauth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db"
)

func TestSaveReferenceValidatesAndStoresOnlyReference(t *testing.T) {
	t.Parallel()

	const token = "ghp_secret_token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Fatalf("path = %q, want /user", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("Authorization header = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("X-OAuth-Scopes", "repo, read:user")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"login":"octocat"}`))
	}))
	defer server.Close()

	database := openTestDB(t)
	service, err := New(database, HTTPTokenValidator{BaseURL: server.URL, Client: server.Client()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service.now = func() time.Time {
		return time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	}

	ref, err := service.SaveReference(context.Background(), SaveReferenceParams{
		StorageKey: "github:default",
		Token:      token,
	})
	if err != nil {
		t.Fatalf("SaveReference() error = %v", err)
	}
	if ref.Kind != CredentialKindGitHub {
		t.Fatalf("Kind = %q, want %q", ref.Kind, CredentialKindGitHub)
	}
	if ref.StorageProvider != StorageProviderElectronSafeStore {
		t.Fatalf("StorageProvider = %q", ref.StorageProvider)
	}
	if ref.StorageKey != "github:default" {
		t.Fatalf("StorageKey = %q", ref.StorageKey)
	}
	if strings.Contains(ref.MetadataJson, token) || strings.Contains(ref.DisplayName, token) {
		t.Fatalf("credential ref leaked token: %+v", ref)
	}

	var metadata struct {
		Login  string   `json:"login"`
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal([]byte(ref.MetadataJson), &metadata); err != nil {
		t.Fatalf("metadata decode: %v", err)
	}
	if metadata.Login != "octocat" || strings.Join(metadata.Scopes, ",") != "repo,read:user" {
		t.Fatalf("metadata = %+v", metadata)
	}

	loaded, err := service.GetReference(context.Background())
	if err != nil {
		t.Fatalf("GetReference() error = %v", err)
	}
	if loaded.ID != ref.ID || loaded.StorageKey != ref.StorageKey {
		t.Fatalf("GetReference() = %+v, want %+v", loaded, ref)
	}
}

func TestSaveReferenceRejectsMissingInputs(t *testing.T) {
	t.Parallel()

	service, err := New(openTestDB(t), staticValidator{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = service.SaveReference(context.Background(), SaveReferenceParams{Token: "token"})
	assertAppError(t, err, apperror.CodeInvalidRequest)

	_, err = service.SaveReference(context.Background(), SaveReferenceParams{StorageKey: "github:default"})
	assertAppError(t, err, apperror.CodeInvalidRequest)
}

func TestSaveReferenceRejectsDeniedToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	service, err := New(openTestDB(t), HTTPTokenValidator{BaseURL: server.URL, Client: server.Client()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = service.SaveReference(context.Background(), SaveReferenceParams{
		StorageKey: "github:default",
		Token:      "bad-token",
	})
	assertAppError(t, err, apperror.CodeUnauthorized)
}

func TestGetReferenceMissingReturnsTypedError(t *testing.T) {
	t.Parallel()

	service, err := New(openTestDB(t), staticValidator{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = service.GetReference(context.Background())
	assertAppError(t, err, apperror.CodeInvalidRequest)
}

type staticValidator struct{}

func (staticValidator) Validate(context.Context, string) (ValidationResult, error) {
	return ValidationResult{Login: "octocat", Scopes: []string{"repo"}}, nil
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	return database
}

func assertAppError(t *testing.T, err error, code apperror.Code) {
	t.Helper()

	if err == nil {
		t.Fatal("error = nil, want *apperror.Error")
	}
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %T, want *apperror.Error", err)
	}
	if appErr.Code != code {
		t.Fatalf("Code = %q, want %q", appErr.Code, code)
	}
}
