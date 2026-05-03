package httpapi

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/githubauth"
)

type CredentialRefResponse struct {
	ID              string          `json:"id"`
	Kind            string          `json:"kind"`
	DisplayName     string          `json:"display_name"`
	StorageProvider string          `json:"storage_provider"`
	StorageKey      string          `json:"storage_key"`
	Metadata        json.RawMessage `json:"metadata"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

type GitHubCredentialStatusResponse struct {
	Configured bool                   `json:"configured"`
	Credential *CredentialRefResponse `json:"credential,omitempty"`
}

type SaveGitHubCredentialRequest struct {
	DisplayName string `json:"display_name"`
	StorageKey  string `json:"storage_key"`
	Token       string `json:"token"`
}

type DeleteGitHubCredentialResponse struct {
	Deleted    bool   `json:"deleted"`
	StorageKey string `json:"storage_key,omitempty"`
}

func getGitHubCredentialHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		service, appErr := githubAuthService(services)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		ref, err := service.GetReference(c.Request.Context())
		if err != nil {
			if isMissingCredential(err) {
				respondOK(c, GitHubCredentialStatusResponse{Configured: false})
				return
			}
			respondCredentialError(c, err)
			return
		}
		response := credentialRefResponse(ref)
		respondOK(c, GitHubCredentialStatusResponse{
			Configured: true,
			Credential: &response,
		})
	}
}

func saveGitHubCredentialHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		service, appErr := githubAuthService(services)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		var request SaveGitHubCredentialRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			respondError(c, apperror.InvalidRequest("invalid GitHub credential request"))
			return
		}
		storageKey := strings.TrimSpace(request.StorageKey)
		if storageKey == "" {
			storageKey = "github:default"
		}
		ref, err := service.SaveReference(c.Request.Context(), githubauth.SaveReferenceParams{
			DisplayName: request.DisplayName,
			StorageKey:  storageKey,
			Token:       request.Token,
		})
		if err != nil {
			respondCredentialError(c, err)
			return
		}
		response := credentialRefResponse(ref)
		respondOK(c, GitHubCredentialStatusResponse{
			Configured: true,
			Credential: &response,
		})
	}
}

func deleteGitHubCredentialHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		service, appErr := githubAuthService(services)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		storageKey, err := service.DeleteReference(c.Request.Context())
		if err != nil {
			if isMissingCredential(err) {
				respondOK(c, DeleteGitHubCredentialResponse{Deleted: false})
				return
			}
			respondCredentialError(c, err)
			return
		}
		respondOK(c, DeleteGitHubCredentialResponse{
			Deleted:    true,
			StorageKey: storageKey,
		})
	}
}

func githubAuthService(services routerServices) (*githubauth.Service, *apperror.Error) {
	if services.githubAuthErr != nil || services.githubAuth == nil {
		return nil, apperror.Internal("GitHub credential service is not configured")
	}
	return services.githubAuth, nil
}

func respondCredentialError(c *gin.Context, err error) {
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		respondError(c, appErr)
		return
	}
	respondError(c, apperror.Internal("GitHub credential request failed"))
}

func isMissingCredential(err error) bool {
	var appErr *apperror.Error
	return errors.As(err, &appErr) && appErr.Code == apperror.CodeInvalidRequest
}

func credentialRefResponse(ref dbgen.CredentialRef) CredentialRefResponse {
	return CredentialRefResponse{
		ID:              ref.ID,
		Kind:            ref.Kind,
		DisplayName:     ref.DisplayName,
		StorageProvider: ref.StorageProvider,
		StorageKey:      ref.StorageKey,
		Metadata:        auditJSON(ref.MetadataJson),
		CreatedAt:       ref.CreatedAt,
		UpdatedAt:       ref.UpdatedAt,
	}
}
