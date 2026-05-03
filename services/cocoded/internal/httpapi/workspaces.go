package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

type WorkspaceResponse struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	RootPath      string         `json:"root_path"`
	DefaultRepoID *string        `json:"default_repo_id"`
	SettingsJson  string         `json:"settings_json"`
	Settings      map[string]any `json:"settings"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
}

type RepositoryResponse struct {
	ID            string  `json:"id"`
	WorkspaceID   string  `json:"workspace_id"`
	Name          string  `json:"name"`
	Owner         *string `json:"owner"`
	RemoteURL     *string `json:"remote_url"`
	LocalPath     string  `json:"local_path"`
	DefaultBranch *string `json:"default_branch"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

type OpenRepositoryRequest struct {
	Path string `json:"path"`
}

type OpenRepositoryResponse struct {
	Workspace    WorkspaceResponse    `json:"workspace"`
	Repository   RepositoryResponse   `json:"repository"`
	Repositories []RepositoryResponse `json:"repositories"`
}

func listWorkspacesHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		workspaces, err := services.queries.ListWorkspaces(c.Request.Context())
		if err != nil {
			respondError(c, apperror.Internal("failed to list workspaces"))
			return
		}
		response, appErr := workspaceResponses(workspaces)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, response)
	}
}

func openRepositoryHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request OpenRepositoryRequest
		if !bindJSON(c, &request) {
			return
		}
		if services.gitRepositoriesErr != nil {
			respondError(c, apperror.Internal("git repository service is not configured"))
			return
		}
		if services.gitRepositories == nil {
			respondError(c, apperror.Internal("git repository service is not configured"))
			return
		}

		result, err := services.gitRepositories.Open(c.Request.Context(), request.Path)
		if err != nil {
			respondAppError(c, err)
			return
		}
		repositories, err := services.queries.ListRepositoriesByWorkspace(c.Request.Context(), result.Workspace.ID)
		if err != nil {
			respondError(c, apperror.Internal("failed to list repositories"))
			return
		}

		workspace, appErr := workspaceResponse(result.Workspace)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, OpenRepositoryResponse{
			Workspace:    workspace,
			Repository:   repositoryResponse(result.Repository),
			Repositories: repositoryResponses(repositories),
		})
	}
}

func listWorkspaceRepositoriesHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		workspaceID := strings.TrimSpace(c.Param("id"))
		if workspaceID == "" {
			respondError(c, apperror.InvalidRequest("workspace id is required"))
			return
		}
		if _, appErr := getWorkspace(c.Request.Context(), services.queries, workspaceID); appErr != nil {
			respondError(c, appErr)
			return
		}
		repositories, err := services.queries.ListRepositoriesByWorkspace(c.Request.Context(), workspaceID)
		if err != nil {
			respondError(c, apperror.Internal("failed to list repositories"))
			return
		}
		respondOK(c, repositoryResponses(repositories))
	}
}

func getWorkspace(ctx context.Context, queries *dbgen.Queries, id string) (dbgen.Workspace, *apperror.Error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return dbgen.Workspace{}, apperror.InvalidRequest("workspace id is required")
	}
	workspace, err := queries.GetWorkspace(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.Workspace{}, apperror.NotFound("workspace was not found")
		}
		return dbgen.Workspace{}, apperror.Internal("failed to read workspace")
	}
	return workspace, nil
}

func workspaceResponses(rows []dbgen.Workspace) ([]WorkspaceResponse, *apperror.Error) {
	response := make([]WorkspaceResponse, 0, len(rows))
	for _, row := range rows {
		item, appErr := workspaceResponse(row)
		if appErr != nil {
			return nil, appErr
		}
		response = append(response, item)
	}
	return response, nil
}

func workspaceResponse(row dbgen.Workspace) (WorkspaceResponse, *apperror.Error) {
	settings, appErr := workspaceSettings(row.SettingsJson)
	if appErr != nil {
		return WorkspaceResponse{}, appErr
	}
	return WorkspaceResponse{
		ID:            row.ID,
		Name:          row.Name,
		RootPath:      row.RootPath,
		DefaultRepoID: nullableStringPointer(row.DefaultRepoID),
		SettingsJson:  row.SettingsJson,
		Settings:      settings,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}, nil
}

func workspaceSettings(raw string) (map[string]any, *apperror.Error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, apperror.Internal("stored workspace settings are invalid")
	}
	if settings == nil {
		settings = map[string]any{}
	}
	return settings, nil
}

func repositoryResponses(rows []dbgen.Repository) []RepositoryResponse {
	response := make([]RepositoryResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, repositoryResponse(row))
	}
	return response
}

func repositoryResponse(row dbgen.Repository) RepositoryResponse {
	return RepositoryResponse{
		ID:            row.ID,
		WorkspaceID:   row.WorkspaceID,
		Name:          row.Name,
		Owner:         nullableStringPointer(row.Owner),
		RemoteURL:     nullableStringPointer(row.RemoteUrl),
		LocalPath:     row.LocalPath,
		DefaultBranch: nullableStringPointer(row.DefaultBranch),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func nullableStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
