package httpapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/app"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/githubpr"
	"github.com/hughdo/cocode/services/cocoded/internal/gitrepo"
	"github.com/hughdo/cocode/services/cocoded/internal/snapshot"
)

type Envelope struct {
	Data      any    `json:"data"`
	Error     any    `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

type VersionResponse struct {
	Service string `json:"service"`
	Version string `json:"version"`
	DataDir string `json:"data_dir"`
}

type ChangedFileResponse struct {
	ID              string          `json:"id"`
	SnapshotID      string          `json:"snapshot_id"`
	Path            string          `json:"path"`
	OldPath         string          `json:"old_path,omitempty"`
	Status          string          `json:"status"`
	Additions       int64           `json:"additions"`
	Deletions       int64           `json:"deletions"`
	IsBinary        bool            `json:"is_binary"`
	IsGenerated     bool            `json:"is_generated"`
	IsExcluded      bool            `json:"is_excluded"`
	LineRanges      json.RawMessage `json:"line_ranges"`
	PatchArtifactID string          `json:"patch_artifact_id,omitempty"`
}

type SnapshotResponse struct {
	ID                         string          `json:"id"`
	RepositoryID               string          `json:"repository_id"`
	SourceType                 string          `json:"source_type"`
	Provider                   string          `json:"provider,omitempty"`
	Owner                      string          `json:"owner,omitempty"`
	Repo                       string          `json:"repo,omitempty"`
	PRNumber                   int64           `json:"pr_number,omitempty"`
	PRTitle                    string          `json:"pr_title,omitempty"`
	PRURL                      string          `json:"pr_url,omitempty"`
	BaseRef                    string          `json:"base_ref,omitempty"`
	HeadRef                    string          `json:"head_ref,omitempty"`
	BaseSHA                    string          `json:"base_sha,omitempty"`
	HeadSHA                    string          `json:"head_sha,omitempty"`
	DiffArtifactID             string          `json:"diff_artifact_id,omitempty"`
	PreviousCommentsArtifactID string          `json:"previous_comments_artifact_id,omitempty"`
	Metadata                   json.RawMessage `json:"metadata"`
	ChangedFileCount           int             `json:"changed_file_count,omitempty"`
}

type CreateGitHubSnapshotRequest struct {
	WorkspaceID  string `json:"workspace_id"`
	RepositoryID string `json:"repository_id"`
	URL          string `json:"url"`
	GitHubToken  string `json:"github_token"`
}

type CreateLocalCompareSnapshotRequest struct {
	WorkspaceID  string `json:"workspace_id"`
	RepositoryID string `json:"repository_id"`
	BaseRef      string `json:"base_ref"`
	HeadRef      string `json:"head_ref"`
}

type CreateLocalChangesSnapshotRequest struct {
	WorkspaceID  string `json:"workspace_id"`
	RepositoryID string `json:"repository_id"`
}

type routerServices struct {
	queries             *dbgen.Queries
	snapshots           *snapshot.Service
	snapshotInitErr     error
	gitCollector        gitrepo.Collector
	githubClientFactory func(token string) githubpr.Client
}

func NewRouter(config app.Config, logger *slog.Logger, database *sql.DB) http.Handler {
	gin.SetMode(gin.ReleaseMode)

	queries := dbgen.New(database)
	snapshotService, snapshotErr := snapshot.New(database, artifactRoot(config))
	services := routerServices{
		queries:         queries,
		snapshots:       snapshotService,
		snapshotInitErr: snapshotErr,
		gitCollector:    gitrepo.NewCollector(gitrepo.DefaultRunner()),
		githubClientFactory: func(token string) githubpr.Client {
			return githubpr.Client{
				BaseURL: config.GitHubAPIBaseURL,
				Token:   token,
			}
		},
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestIDMiddleware())
	router.Use(loggingMiddleware(logger))

	router.GET("/api/health", func(c *gin.Context) {
		respondOK(c, HealthResponse{
			Status:  "ok",
			Service: "cocoded",
			Version: config.Version,
		})
	})

	router.GET("/api/version", func(c *gin.Context) {
		respondOK(c, VersionResponse{
			Service: "cocoded",
			Version: config.Version,
			DataDir: config.DataDir,
		})
	})

	api := router.Group("/api")
	api.Use(authMiddleware(config.AuthToken))
	api.GET("/session", func(c *gin.Context) {
		respondOK(c, gin.H{
			"status": "authenticated",
		})
	})
	api.POST("/pr-snapshots/from-github-url", createGitHubSnapshotHandler(services))
	api.POST("/pr-snapshots/from-local-compare", createLocalCompareSnapshotHandler(services))
	api.POST("/pr-snapshots/from-local-changes", createLocalChangesSnapshotHandler(services))
	api.GET("/pr-snapshots/:id", snapshotHandler(services))
	api.GET("/pr-snapshots/:id/changed-files", changedFilesHandler(queries))
	api.GET("/agents/presets", listAgentPresetsHandler())
	api.GET("/agents/configs", listAgentConfigsHandler(queries))
	api.POST("/agents/configs", createAgentConfigHandler(queries))
	api.PATCH("/agents/configs/:id", updateAgentConfigHandler(queries))
	api.POST("/agents/configs/:id/test", testAgentConfigHandler(queries))
	api.DELETE("/agents/configs/:id", deleteAgentConfigHandler(queries))

	return router
}

func artifactRoot(config app.Config) string {
	if strings.TrimSpace(config.ArtifactDir) != "" {
		return config.ArtifactDir
	}
	return filepath.Join(config.DataDir, "artifacts")
}

func createGitHubSnapshotHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request CreateGitHubSnapshotRequest
		if !bindJSON(c, &request) {
			return
		}
		if err := services.ensureSnapshots(); err != nil {
			respondError(c, err)
			return
		}
		if _, appErr := repositoryForSnapshot(c.Request.Context(), services.queries, request.WorkspaceID, request.RepositoryID); appErr != nil {
			respondError(c, appErr)
			return
		}

		ref, err := githubpr.ParseURL(c.Request.Context(), request.URL)
		if err != nil {
			respondAppError(c, err)
			return
		}
		client := services.githubClientFactory(strings.TrimSpace(request.GitHubToken))
		metadata, err := client.FetchMetadata(c.Request.Context(), ref)
		if err != nil {
			respondAppError(c, err)
			return
		}
		files, err := client.FetchChangedFiles(c.Request.Context(), ref)
		if err != nil {
			respondAppError(c, err)
			return
		}
		diff, err := client.FetchDiff(c.Request.Context(), ref)
		if err != nil {
			respondAppError(c, err)
			return
		}
		var previousComments *githubpr.PreviousComments
		previousCommentsFetchError := ""
		fetchedComments, err := client.FetchPreviousComments(c.Request.Context(), ref)
		if err != nil {
			previousCommentsFetchError = err.Error()
		} else {
			previousComments = &fetchedComments
		}

		result, err := services.snapshots.CreateGitHubSnapshot(c.Request.Context(), snapshot.GitHubSnapshotParams{
			WorkspaceID:                strings.TrimSpace(request.WorkspaceID),
			RepositoryID:               strings.TrimSpace(request.RepositoryID),
			Metadata:                   metadata,
			Files:                      files,
			Diff:                       diff,
			PreviousComments:           previousComments,
			PreviousCommentsFetchError: previousCommentsFetchError,
		})
		if err != nil {
			respondAppError(c, err)
			return
		}
		respondSnapshotResult(c, result)
	}
}

func createLocalCompareSnapshotHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request CreateLocalCompareSnapshotRequest
		if !bindJSON(c, &request) {
			return
		}
		if err := services.ensureSnapshots(); err != nil {
			respondError(c, err)
			return
		}

		repository, appErr := repositoryForSnapshot(c.Request.Context(), services.queries, request.WorkspaceID, request.RepositoryID)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		collected, err := services.gitCollector.CompareBranches(c.Request.Context(), repository.LocalPath, request.BaseRef, request.HeadRef)
		if err != nil {
			respondAppError(c, err)
			return
		}
		result, err := services.snapshots.CreateGitSnapshot(c.Request.Context(), gitSnapshotParams(request.WorkspaceID, request.RepositoryID, collected))
		if err != nil {
			respondAppError(c, err)
			return
		}
		respondSnapshotResult(c, result)
	}
}

func createLocalChangesSnapshotHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request CreateLocalChangesSnapshotRequest
		if !bindJSON(c, &request) {
			return
		}
		if err := services.ensureSnapshots(); err != nil {
			respondError(c, err)
			return
		}

		repository, appErr := repositoryForSnapshot(c.Request.Context(), services.queries, request.WorkspaceID, request.RepositoryID)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		collected, err := services.gitCollector.LocalChanges(c.Request.Context(), repository.LocalPath)
		if err != nil {
			respondAppError(c, err)
			return
		}
		result, err := services.snapshots.CreateGitSnapshot(c.Request.Context(), gitSnapshotParams(request.WorkspaceID, request.RepositoryID, collected))
		if err != nil {
			respondAppError(c, err)
			return
		}
		respondSnapshotResult(c, result)
	}
}

func snapshotHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshotID := strings.TrimSpace(c.Param("id"))
		if snapshotID == "" {
			respondError(c, apperror.InvalidRequest("snapshot id is required"))
			return
		}
		row, err := services.queries.GetPullRequestSnapshot(c.Request.Context(), snapshotID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondError(c, apperror.NotFound("snapshot was not found"))
				return
			}
			respondError(c, apperror.Internal("failed to read snapshot"))
			return
		}
		files, err := services.queries.ListChangedFilesBySnapshot(c.Request.Context(), snapshotID)
		if err != nil {
			respondError(c, apperror.Internal("failed to list changed files"))
			return
		}
		respondOK(c, snapshotResponse(row, len(files)))
	}
}

func changedFilesHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshotID := strings.TrimSpace(c.Param("id"))
		if snapshotID == "" {
			respondError(c, apperror.InvalidRequest("snapshot id is required"))
			return
		}
		if _, err := queries.GetPullRequestSnapshot(c.Request.Context(), snapshotID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondError(c, apperror.NotFound("snapshot was not found"))
				return
			}
			respondError(c, apperror.Internal("failed to read snapshot"))
			return
		}

		files, err := queries.ListChangedFilesBySnapshot(c.Request.Context(), snapshotID)
		if err != nil {
			respondError(c, apperror.Internal("failed to list changed files"))
			return
		}
		response := make([]ChangedFileResponse, 0, len(files))
		for _, file := range files {
			item, err := changedFileResponse(file)
			if err != nil {
				respondError(c, apperror.Internal("changed file line ranges are invalid"))
				return
			}
			response = append(response, item)
		}
		respondOK(c, response)
	}
}

func (s routerServices) ensureSnapshots() *apperror.Error {
	if s.snapshots == nil {
		message := "snapshot service is not configured"
		if s.snapshotInitErr != nil {
			message = fmt.Sprintf("%s: %v", message, s.snapshotInitErr)
		}
		return apperror.Internal(message)
	}
	return nil
}

func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		respondError(c, apperror.InvalidRequest("request body is invalid JSON"))
		return false
	}
	return true
}

func repositoryForSnapshot(ctx context.Context, queries *dbgen.Queries, workspaceID string, repositoryID string) (dbgen.Repository, *apperror.Error) {
	workspaceID = strings.TrimSpace(workspaceID)
	repositoryID = strings.TrimSpace(repositoryID)
	if workspaceID == "" {
		return dbgen.Repository{}, apperror.InvalidRequest("workspace id is required")
	}
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
	if repository.WorkspaceID != workspaceID {
		return dbgen.Repository{}, apperror.InvalidRequest("repository does not belong to workspace")
	}
	if strings.TrimSpace(repository.LocalPath) == "" {
		return dbgen.Repository{}, apperror.InvalidRequest("repository local path is not configured")
	}
	return repository, nil
}

func gitSnapshotParams(workspaceID string, repositoryID string, collected gitrepo.DiffSnapshot) snapshot.GitSnapshotParams {
	return snapshot.GitSnapshotParams{
		WorkspaceID:  strings.TrimSpace(workspaceID),
		RepositoryID: strings.TrimSpace(repositoryID),
		SourceType:   collected.SourceType,
		BaseRef:      collected.BaseRef,
		HeadRef:      collected.HeadRef,
		BaseSHA:      collected.BaseSHA,
		HeadSHA:      collected.HeadSHA,
		Metadata:     collected.Metadata,
		Files:        changedFileInputs(collected.Files),
		Diff:         collected.Diff,
	}
}

func changedFileInputs(files []gitrepo.DiffFile) []snapshot.ChangedFileInput {
	inputs := make([]snapshot.ChangedFileInput, 0, len(files))
	for _, file := range files {
		inputs = append(inputs, snapshot.ChangedFileInput{
			Path:           file.Path,
			OldPath:        file.OldPath,
			Status:         file.Status,
			Additions:      file.Additions,
			Deletions:      file.Deletions,
			IsBinary:       file.IsBinary,
			LineRangesJSON: file.LineRangesJSON,
			Patch:          file.Patch,
		})
	}
	return inputs
}

func respondSnapshotResult(c *gin.Context, result snapshot.SnapshotResult) {
	response := snapshotResponse(result.Snapshot, len(result.ChangedFiles))
	response.PreviousCommentsArtifactID = result.PreviousCommentsArtifact.ID
	respondOK(c, response)
}

func snapshotResponse(row dbgen.PullRequestSnapshot, changedFileCount int) SnapshotResponse {
	metadata := json.RawMessage(row.MetadataJson)
	if len(metadata) == 0 || !json.Valid(metadata) {
		metadata = json.RawMessage("{}")
	}
	return SnapshotResponse{
		ID:               row.ID,
		RepositoryID:     row.RepositoryID,
		SourceType:       row.SourceType,
		Provider:         nullableResponseString(row.Provider),
		Owner:            nullableResponseString(row.Owner),
		Repo:             nullableResponseString(row.Repo),
		PRNumber:         nullableResponseInt64(row.PrNumber),
		PRTitle:          nullableResponseString(row.PrTitle),
		PRURL:            nullableResponseString(row.PrUrl),
		BaseRef:          nullableResponseString(row.BaseRef),
		HeadRef:          nullableResponseString(row.HeadRef),
		BaseSHA:          nullableResponseString(row.BaseSha),
		HeadSHA:          nullableResponseString(row.HeadSha),
		DiffArtifactID:   nullableResponseString(row.DiffArtifactID),
		Metadata:         metadata,
		ChangedFileCount: changedFileCount,
	}
}

func changedFileResponse(file dbgen.ChangedFile) (ChangedFileResponse, error) {
	lineRanges := json.RawMessage(file.LineRangesJson)
	if len(lineRanges) == 0 {
		lineRanges = json.RawMessage("[]")
	}
	if !json.Valid(lineRanges) {
		return ChangedFileResponse{}, errors.New("invalid line ranges JSON")
	}
	return ChangedFileResponse{
		ID:              file.ID,
		SnapshotID:      file.SnapshotID,
		Path:            file.Path,
		OldPath:         nullableResponseString(file.OldPath),
		Status:          file.Status,
		Additions:       file.Additions,
		Deletions:       file.Deletions,
		IsBinary:        file.IsBinary != 0,
		IsGenerated:     file.IsGenerated != 0,
		IsExcluded:      file.IsExcluded != 0,
		LineRanges:      lineRanges,
		PatchArtifactID: nullableResponseString(file.PatchArtifactID),
	}, nil
}

func nullableResponseString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullableResponseInt64(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{
		Data:      data,
		Error:     nil,
		RequestID: requestID(c),
	})
}

func respondError(c *gin.Context, err *apperror.Error) {
	c.JSON(err.Status, Envelope{
		Data:      nil,
		Error:     err,
		RequestID: requestID(c),
	})
}

func respondAppError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		respondError(c, appErr)
		return
	}
	respondError(c, apperror.Internal(err.Error()))
}

func authMiddleware(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if subtleTokenMatch(c.GetHeader("X-Cocode-Token"), token) ||
			subtleTokenMatch(bearerToken(c.GetHeader("Authorization")), token) {
			c.Next()
			return
		}

		respondError(c, apperror.Unauthorized("missing or invalid local auth token"))
		c.Abort()
	}
}

func bearerToken(header string) string {
	value := strings.TrimSpace(header)
	if value == "" {
		return ""
	}

	prefix := "Bearer "
	if !strings.HasPrefix(value, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, prefix))
}

func subtleTokenMatch(got string, want string) bool {
	if got == "" || want == "" || len(got) != len(want) {
		return false
	}

	var result byte
	for i := range got {
		result |= got[i] ^ want[i]
	}
	return result == 0
}

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func loggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		logger.Info(
			"http request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"request_id", requestID(c),
		)
	}
}

func requestID(c *gin.Context) string {
	value, ok := c.Get("request_id")
	if !ok {
		return ""
	}
	id, _ := value.(string)
	return id
}

func newRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "request-id-unavailable"
	}
	return hex.EncodeToString(bytes[:])
}
