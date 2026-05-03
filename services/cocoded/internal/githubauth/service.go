package githubauth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const (
	CredentialKindGitHub             = "github"
	DefaultCredentialID              = "github_default"
	StorageProviderElectronSafeStore = "electron_safe_storage"
	defaultGitHubAPIBaseURL          = "https://api.github.com"
)

type Service struct {
	queries   *dbgen.Queries
	validator TokenValidator
	now       func() time.Time
}

type TokenValidator interface {
	Validate(ctx context.Context, token string) (ValidationResult, error)
}

type ValidationResult struct {
	Login  string   `json:"login,omitempty"`
	Scopes []string `json:"scopes,omitempty"`
}

type SaveReferenceParams struct {
	ID              string
	DisplayName     string
	StorageProvider string
	StorageKey      string
	Token           string
}

type HTTPTokenValidator struct {
	BaseURL string
	Client  *http.Client
}

func New(database *sql.DB, validator TokenValidator) (*Service, error) {
	if database == nil {
		return nil, errors.New("github auth database is required")
	}
	if validator == nil {
		validator = HTTPTokenValidator{}
	}
	return &Service{
		queries:   dbgen.New(database),
		validator: validator,
		now:       time.Now,
	}, nil
}

func (s *Service) SaveReference(ctx context.Context, params SaveReferenceParams) (dbgen.CredentialRef, error) {
	if s == nil {
		return dbgen.CredentialRef{}, apperror.Internal("GitHub auth service is not configured")
	}
	if strings.TrimSpace(params.StorageKey) == "" {
		return dbgen.CredentialRef{}, apperror.InvalidRequest("GitHub token storage key is required")
	}
	if strings.TrimSpace(params.Token) == "" {
		return dbgen.CredentialRef{}, apperror.InvalidRequest("GitHub token is required for validation")
	}
	if params.ID == "" {
		params.ID = DefaultCredentialID
	}
	if params.StorageProvider == "" {
		params.StorageProvider = StorageProviderElectronSafeStore
	}

	validation, err := s.validator.Validate(ctx, params.Token)
	if err != nil {
		return dbgen.CredentialRef{}, err
	}

	now := s.now().UTC().Format(time.RFC3339Nano)
	if params.DisplayName == "" {
		params.DisplayName = "GitHub token"
		if validation.Login != "" {
			params.DisplayName += " (" + validation.Login + ")"
		}
	}

	metadata, err := json.Marshal(map[string]any{
		"login":        validation.Login,
		"scopes":       validation.Scopes,
		"validated_at": now,
	})
	if err != nil {
		return dbgen.CredentialRef{}, apperror.Internal("failed to encode GitHub credential metadata")
	}

	ref, err := s.queries.UpsertCredentialRef(ctx, dbgen.UpsertCredentialRefParams{
		ID:              params.ID,
		Kind:            CredentialKindGitHub,
		DisplayName:     params.DisplayName,
		StorageProvider: params.StorageProvider,
		StorageKey:      params.StorageKey,
		MetadataJson:    string(metadata),
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		return dbgen.CredentialRef{}, apperror.Internal("failed to save GitHub token reference")
	}
	return ref, nil
}

func (s *Service) GetReference(ctx context.Context) (dbgen.CredentialRef, error) {
	if s == nil {
		return dbgen.CredentialRef{}, apperror.Internal("GitHub auth service is not configured")
	}
	ref, err := s.queries.GetLatestCredentialRefByKind(ctx, CredentialKindGitHub)
	if err == nil {
		return ref, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return dbgen.CredentialRef{}, apperror.InvalidRequest("GitHub token reference is not configured")
	}
	return dbgen.CredentialRef{}, apperror.Internal("failed to read GitHub token reference")
}

func (v HTTPTokenValidator) Validate(ctx context.Context, token string) (ValidationResult, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return ValidationResult{}, apperror.InvalidRequest("GitHub token is required")
	}

	baseURL := strings.TrimRight(v.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultGitHubAPIBaseURL
	}
	client := v.Client
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/user", nil)
	if err != nil {
		return ValidationResult{}, apperror.Internal("failed to build GitHub token validation request")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return ValidationResult{}, apperror.Internal("failed to validate GitHub token")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ValidationResult{}, apperror.Unauthorized("GitHub token was rejected")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ValidationResult{}, apperror.Internal(fmt.Sprintf("GitHub token validation failed with status %d", resp.StatusCode))
	}

	var body struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ValidationResult{}, apperror.Internal("failed to decode GitHub token validation response")
	}

	return ValidationResult{
		Login:  body.Login,
		Scopes: parseScopes(resp.Header.Get("X-OAuth-Scopes")),
	}, nil
}

func parseScopes(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	scopes := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			scopes = append(scopes, part)
		}
	}
	return scopes
}
