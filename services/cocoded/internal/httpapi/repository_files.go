package httpapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/gitrepo"
	"github.com/hughdo/cocode/services/cocoded/internal/security"
)

const (
	defaultRepositoryFileContentLimit = 256 << 10
	maxRepositoryFileContentLimit     = 1 << 20
	defaultRepositoryFileTreeLimit    = 2000
	maxRepositoryFileTreeLimit        = 5000
)

type RepositoryFileResponse struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Directory string `json:"directory,omitempty"`
	Kind      string `json:"kind"`
	Score     int    `json:"score"`
}

type RepositoryFileContentResponse struct {
	Path             string `json:"path"`
	Name             string `json:"name"`
	Directory        string `json:"directory,omitempty"`
	Content          string `json:"content,omitempty"`
	ContentType      string `json:"content_type"`
	SizeBytes        int64  `json:"size_bytes"`
	ContentTruncated bool   `json:"content_truncated"`
	Binary           bool   `json:"binary"`
}

type RepositoryFileTreeResponse struct {
	Files     []RepositoryFileResponse `json:"files"`
	Truncated bool                     `json:"truncated"`
	Limit     int                      `json:"limit"`
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

func repositoryFileTreeHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		repository, appErr := repositoryForBranchListing(c.Request.Context(), services.queries, c.Param("id"), c.Query("workspace_id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		limit := parseRepositoryFileTreeLimit(c.Query("limit"))
		files, truncated, err := services.gitCollector.ListFiles(c.Request.Context(), repository.LocalPath, limit)
		if err != nil {
			respondAppError(c, err)
			return
		}
		respondOK(c, RepositoryFileTreeResponse{
			Files:     repositoryFileResponses(files),
			Truncated: truncated,
			Limit:     limit,
		})
	}
}

func repositoryFileContentHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		repository, appErr := repositoryForBranchListing(c.Request.Context(), services.queries, c.Param("id"), c.Query("workspace_id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		path := strings.TrimSpace(c.Query("path"))
		if path == "" {
			respondError(c, apperror.InvalidRequest("file path is required"))
			return
		}
		target, cleanPath, err := security.ResolveExistingWithinRoot(repository.LocalPath, path)
		if err != nil {
			if errors.Is(err, security.ErrPathEscapesRoot) {
				respondError(c, apperror.InvalidRequest("file path is outside the repository"))
				return
			}
			respondError(c, apperror.NotFound("file was not found"))
			return
		}
		stat, err := os.Stat(target)
		if err != nil {
			respondError(c, apperror.NotFound("file was not found"))
			return
		}
		if stat.IsDir() {
			respondError(c, apperror.InvalidRequest("file path points to a directory"))
			return
		}

		content, truncated, binary, err := readRepositoryFileContent(target, parseRepositoryFileContentLimit(c.Query("max_bytes")))
		if err != nil {
			respondError(c, apperror.Internal("failed to read repository file"))
			return
		}
		name := filepath.Base(filepath.FromSlash(cleanPath))
		directory := strings.Trim(strings.TrimSuffix(filepath.ToSlash(filepath.Dir(cleanPath)), "."), "/")
		response := RepositoryFileContentResponse{
			Path:             cleanPath,
			Name:             name,
			Directory:        directory,
			ContentType:      detectRepositoryFileContentType(content, binary),
			SizeBytes:        stat.Size(),
			ContentTruncated: truncated,
			Binary:           binary,
		}
		if !binary {
			response.Content = string(content)
		}
		respondOK(c, response)
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

func parseRepositoryFileTreeLimit(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultRepositoryFileTreeLimit
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return defaultRepositoryFileTreeLimit
	}
	if limit > maxRepositoryFileTreeLimit {
		return maxRepositoryFileTreeLimit
	}
	return limit
}

func parseRepositoryFileContentLimit(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultRepositoryFileContentLimit
	}
	limit, err := strconv.ParseInt(value, 10, 64)
	if err != nil || limit <= 0 {
		return defaultRepositoryFileContentLimit
	}
	if limit > maxRepositoryFileContentLimit {
		return maxRepositoryFileContentLimit
	}
	return limit
}

func readRepositoryFileContent(path string, limit int64) ([]byte, bool, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, false, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, false, false, err
	}
	truncated := int64(len(content)) > limit
	if truncated {
		content = content[:limit]
	}
	binary := bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content)
	return content, truncated, binary, nil
}

func detectRepositoryFileContentType(content []byte, binary bool) string {
	if binary {
		if len(content) == 0 {
			return "application/octet-stream"
		}
		return http.DetectContentType(content)
	}
	return "text/plain; charset=utf-8"
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
