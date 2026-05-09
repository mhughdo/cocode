package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/gitrepo"
)

type RepositoryBranchResponse struct {
	Name      string `json:"name"`
	CommitSHA string `json:"commit_sha"`
	Upstream  string `json:"upstream,omitempty"`
	Current   bool   `json:"current"`
	Remote    bool   `json:"remote"`
}

func listRepositoryBranchesHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		repository, appErr := repositoryForBranchListing(c.Request.Context(), services.queries, c.Param("id"), c.Query("workspace_id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		branches, err := services.gitCollector.ListBranches(c.Request.Context(), repository.LocalPath)
		if err != nil {
			respondAppError(c, err)
			return
		}
		respondOK(c, repositoryBranchResponses(branches))
	}
}

func repositoryForBranchListing(ctx context.Context, queries *dbgen.Queries, repositoryID string, workspaceID string) (dbgen.Repository, *apperror.Error) {
	repositoryID = strings.TrimSpace(repositoryID)
	if repositoryID == "" {
		return dbgen.Repository{}, apperror.InvalidRequest("repository id is required")
	}
	repository, err := queries.GetRepository(ctx, repositoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.Repository{}, apperror.NotFound("repository was not found")
		}
		return dbgen.Repository{}, apperror.Internal("failed to read repository")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID != "" && repository.WorkspaceID != workspaceID {
		return dbgen.Repository{}, apperror.InvalidRequest("repository does not belong to workspace")
	}
	if strings.TrimSpace(repository.LocalPath) == "" {
		return dbgen.Repository{}, apperror.InvalidRequest("repository local path is not configured")
	}
	return repository, nil
}

func repositoryBranchResponses(branches []gitrepo.Branch) []RepositoryBranchResponse {
	response := make([]RepositoryBranchResponse, 0, len(branches))
	for _, branch := range branches {
		response = append(response, RepositoryBranchResponse{
			Name:      branch.Name,
			CommitSHA: branch.CommitSHA,
			Upstream:  branch.Upstream,
			Current:   branch.Current,
			Remote:    branch.Remote,
		})
	}
	return response
}
