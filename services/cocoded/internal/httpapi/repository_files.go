package httpapi

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/gitrepo"
)

type RepositoryFileResponse struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Directory string `json:"directory,omitempty"`
	Kind      string `json:"kind"`
	Score     int    `json:"score"`
}

func searchRepositoryFilesHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		repository, appErr := repositoryForBranchListing(c.Request.Context(), services.queries, c.Param("id"), c.Query("workspace_id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		matches, err := services.gitCollector.SearchFiles(
			c.Request.Context(),
			repository.LocalPath,
			c.Query("q"),
			parseRepositoryFileLimit(c.Query("limit")),
		)
		if err != nil {
			respondAppError(c, err)
			return
		}
		respondOK(c, repositoryFileResponses(matches))
	}
}

func parseRepositoryFileLimit(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	limit, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return limit
}

func repositoryFileResponses(matches []gitrepo.FileMatch) []RepositoryFileResponse {
	response := make([]RepositoryFileResponse, 0, len(matches))
	for _, match := range matches {
		response = append(response, RepositoryFileResponse{
			Path:      match.Path,
			Name:      match.Name,
			Directory: match.Directory,
			Kind:      match.Kind,
			Score:     match.Score,
		})
	}
	return response
}
