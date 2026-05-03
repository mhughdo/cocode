package httpapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/agentrun"
	"github.com/hughdo/cocode/services/cocoded/internal/app"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/contextbundle"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/eventbus"
	"github.com/hughdo/cocode/services/cocoded/internal/eventlog"
	"github.com/hughdo/cocode/services/cocoded/internal/githubpr"
	"github.com/hughdo/cocode/services/cocoded/internal/gitrepo"
	"github.com/hughdo/cocode/services/cocoded/internal/orchestrator"
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

type BuildReviewContextRequest struct {
	AgentConfigID string          `json:"agent_config_id"`
	Persist       bool            `json:"persist"`
	ContextPolicy json.RawMessage `json:"context_policy"`
}

type BuildReviewContextResponse struct {
	Bundle                    contextbundle.Bundle          `json:"bundle"`
	Dropped                   []contextbundle.DroppedItem   `json:"dropped"`
	Warnings                  []string                      `json:"warnings,omitempty"`
	RedactionReport           contextbundle.RedactionReport `json:"redaction_report"`
	Persisted                 bool                          `json:"persisted"`
	ArtifactID                string                        `json:"artifact_id,omitempty"`
	RedactionReportArtifactID string                        `json:"redaction_report_artifact_id,omitempty"`
}

type ContextBundleDebugResponse struct {
	Bundles  []ContextBundleDebugBundle `json:"bundles"`
	Warnings []string                   `json:"warnings,omitempty"`
}

type ContextBundleDebugBundle struct {
	Bundle        contextbundle.Bundle             `json:"bundle"`
	Artifact      *ArtifactDebugResponse           `json:"artifact,omitempty"`
	ItemArtifacts map[string]ArtifactDebugResponse `json:"item_artifacts,omitempty"`
	AgentRunIDs   []string                         `json:"agent_run_ids,omitempty"`
}

type ArtifactDebugResponse struct {
	ID               string          `json:"id"`
	Kind             string          `json:"kind"`
	RelativePath     string          `json:"relative_path"`
	ContentType      string          `json:"content_type"`
	SizeBytes        int64           `json:"size_bytes"`
	SHA256           string          `json:"sha256,omitempty"`
	Metadata         json.RawMessage `json:"metadata"`
	Content          string          `json:"content,omitempty"`
	ContentTruncated bool            `json:"content_truncated,omitempty"`
}

type routerServices struct {
	queries             *dbgen.Queries
	snapshots           *snapshot.Service
	snapshotInitErr     error
	contextBuilder      *contextbundle.Service
	contextBuilderErr   error
	reviewWorkflow      *orchestrator.Service
	reviewWorkflowErr   error
	eventBus            *eventbus.Bus
	gitCollector        gitrepo.Collector
	githubClientFactory func(token string) githubpr.Client
}

func NewRouter(config app.Config, logger *slog.Logger, database *sql.DB) http.Handler {
	gin.SetMode(gin.ReleaseMode)

	queries := dbgen.New(database)
	snapshotService, snapshotErr := snapshot.New(database, artifactRoot(config))
	artifactStore, artifactErr := artifact.New(artifactRoot(config), queries)
	var contextBuilder *contextbundle.Service
	if artifactErr == nil {
		contextBuilder = &contextbundle.Service{
			Queries:   queries,
			Artifacts: artifactStore,
		}
	}
	eventStore, eventErr := eventlog.New(database)
	var bus *eventbus.Bus
	busErr := eventErr
	if busErr == nil {
		bus, busErr = eventbus.New(eventStore)
	}
	var reviewWorkflow *orchestrator.Service
	workflowErr := artifactErr
	if workflowErr == nil {
		workflowErr = busErr
	}
	if workflowErr == nil {
		runner := agentrun.Runner{
			Queries:   queries,
			Artifacts: artifactStore,
		}
		reviewWorkflow = &orchestrator.Service{
			Queries:        queries,
			ContextBuilder: contextBuilder,
			Artifacts:      artifactStore,
			Events:         bus,
			AgentManager: &agentrun.Manager{
				Runner:                  runner,
				MaxConcurrent:           2,
				MaxConcurrentPerSession: 2,
			},
		}
	}
	services := routerServices{
		queries:           queries,
		snapshots:         snapshotService,
		snapshotInitErr:   snapshotErr,
		contextBuilder:    contextBuilder,
		contextBuilderErr: artifactErr,
		reviewWorkflow:    reviewWorkflow,
		reviewWorkflowErr: workflowErr,
		eventBus:          bus,
		gitCollector:      gitrepo.NewCollector(gitrepo.DefaultRunner()),
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
	api.POST("/review-sessions", createReviewSessionHandler(queries))
	api.GET("/review-sessions", listReviewSessionsHandler(queries))
	api.GET("/review-sessions/:id", getReviewSessionHandler(queries))
	api.POST("/review-sessions/:id/start", startReviewSessionHandler(services))
	api.POST("/review-sessions/:id/pause", pauseReviewSessionHandler(services))
	api.POST("/review-sessions/:id/resume", resumeReviewSessionHandler(services))
	api.POST("/review-sessions/:id/cancel", cancelReviewSessionHandler(services))
	api.GET("/review-sessions/:id/checkpoint", reviewSessionCheckpointHandler(services))
	api.GET("/review-sessions/:id/summary", reviewSessionSummaryHandler(services))
	api.GET("/review-sessions/:id/events", reviewSessionEventsHandler(services))
	api.GET("/agents/presets", listAgentPresetsHandler())
	api.GET("/agents/configs", listAgentConfigsHandler(queries))
	api.POST("/agents/configs", createAgentConfigHandler(queries))
	api.PATCH("/agents/configs/:id", updateAgentConfigHandler(queries))
	api.POST("/agents/configs/:id/test", testAgentConfigHandler(queries))
	api.DELETE("/agents/configs/:id", deleteAgentConfigHandler(queries))
	api.POST("/review-sessions/:id/context-bundles/preview", buildReviewContextHandler(services))
	api.GET("/review-sessions/:id/context-bundles", contextBundleDebugHandler(services))

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

func buildReviewContextHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := strings.TrimSpace(c.Param("id"))
		if sessionID == "" {
			respondError(c, apperror.InvalidRequest("review session id is required"))
			return
		}
		var request BuildReviewContextRequest
		if !bindOptionalJSON(c, &request) {
			return
		}
		if err := services.ensureContextBuilder(); err != nil {
			respondError(c, err)
			return
		}
		result, err := services.contextBuilder.BuildReviewContext(c.Request.Context(), contextbundle.BuildReviewContextParams{
			ReviewSessionID: sessionID,
			AgentConfigID:   request.AgentConfigID,
			PolicyOverride:  request.ContextPolicy,
			Persist:         request.Persist,
		})
		if err != nil {
			respondReviewContextError(c, err)
			return
		}
		respondOK(c, buildReviewContextResponse(result))
	}
}

func contextBundleDebugHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := strings.TrimSpace(c.Param("id"))
		if sessionID == "" {
			respondError(c, apperror.InvalidRequest("review session id is required"))
			return
		}
		if err := services.ensureContextBuilder(); err != nil {
			respondError(c, err)
			return
		}
		response, err := buildContextBundleDebugResponse(c.Request.Context(), services, sessionID)
		if err != nil {
			respondReviewContextError(c, err)
			return
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

func (s routerServices) ensureContextBuilder() *apperror.Error {
	if s.contextBuilder == nil {
		message := "context builder is not configured"
		if s.contextBuilderErr != nil {
			message = fmt.Sprintf("%s: %v", message, s.contextBuilderErr)
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

func bindOptionalJSON(c *gin.Context, target any) bool {
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return true
	}
	if err := c.ShouldBindJSON(target); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
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

func buildReviewContextResponse(result contextbundle.BuildReviewContextResult) BuildReviewContextResponse {
	response := BuildReviewContextResponse{
		Bundle:          result.Bundle,
		Dropped:         result.Dropped,
		Warnings:        result.Warnings,
		RedactionReport: result.RedactionReport,
		Persisted:       result.Persisted,
		ArtifactID:      result.Artifact.ID,
	}
	if result.RedactionReportArtifact.ID != "" {
		response.RedactionReportArtifactID = result.RedactionReportArtifact.ID
	}
	return response
}

func buildContextBundleDebugResponse(ctx context.Context, services routerServices, sessionID string) (ContextBundleDebugResponse, error) {
	if _, err := services.queries.GetReviewSession(ctx, sessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ContextBundleDebugResponse{}, contextbundle.ErrReviewSessionNotFound
		}
		return ContextBundleDebugResponse{}, fmt.Errorf("read review session: %w", err)
	}
	rows, err := services.queries.ListContextBundlesBySession(ctx, sessionID)
	if err != nil {
		return ContextBundleDebugResponse{}, fmt.Errorf("list context bundles: %w", err)
	}
	runs, err := services.queries.ListAgentRunsBySession(ctx, sessionID)
	if err != nil {
		return ContextBundleDebugResponse{}, fmt.Errorf("list agent runs: %w", err)
	}
	runIDsByBundle := map[string][]string{}
	for _, run := range runs {
		if run.ContextBundleID.Valid && strings.TrimSpace(run.ContextBundleID.String) != "" {
			runIDsByBundle[run.ContextBundleID.String] = append(runIDsByBundle[run.ContextBundleID.String], run.ID)
		}
	}

	response := ContextBundleDebugResponse{
		Bundles: make([]ContextBundleDebugBundle, 0, len(rows)),
	}
	for _, row := range rows {
		itemRows, err := services.queries.ListContextItemsByBundle(ctx, row.ID)
		if err != nil {
			return ContextBundleDebugResponse{}, fmt.Errorf("list context items for %s: %w", row.ID, err)
		}
		bundle, err := contextbundle.BundleFromRows(row, itemRows)
		if err != nil {
			return ContextBundleDebugResponse{}, err
		}
		debugBundle := ContextBundleDebugBundle{
			Bundle:      bundle,
			AgentRunIDs: append([]string(nil), runIDsByBundle[bundle.ID]...),
		}
		if bundle.ArtifactID != "" {
			artifactResponse, warning, err := artifactDebugResponse(ctx, services.contextBuilder.Artifacts, bundle.ArtifactID)
			if err != nil {
				response.Warnings = appendResponseWarning(response.Warnings, warning)
			} else {
				debugBundle.Artifact = &artifactResponse
			}
		}
		for _, item := range bundle.Items {
			if item.ContentArtifactID == "" {
				continue
			}
			artifactResponse, warning, err := artifactDebugResponse(ctx, services.contextBuilder.Artifacts, item.ContentArtifactID)
			if err != nil {
				response.Warnings = appendResponseWarning(response.Warnings, warning)
				continue
			}
			if debugBundle.ItemArtifacts == nil {
				debugBundle.ItemArtifacts = map[string]ArtifactDebugResponse{}
			}
			debugBundle.ItemArtifacts[item.ID] = artifactResponse
		}
		response.Bundles = append(response.Bundles, debugBundle)
	}
	return response, nil
}

func appendResponseWarning(warnings []string, warning string) []string {
	warning = strings.TrimSpace(warning)
	if warning == "" {
		return warnings
	}
	for _, existing := range warnings {
		if existing == warning {
			return warnings
		}
	}
	return append(warnings, warning)
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

const contextDebugArtifactContentLimit = 256 * 1024

func artifactDebugResponse(ctx context.Context, store *artifact.Store, artifactID string) (ArtifactDebugResponse, string, error) {
	content, row, err := store.Read(ctx, artifactID)
	if err != nil {
		return ArtifactDebugResponse{}, fmt.Sprintf("artifact %s could not be read: %v", artifactID, err), err
	}
	metadata := json.RawMessage(row.MetadataJson)
	if len(metadata) == 0 || !json.Valid(metadata) {
		metadata = json.RawMessage("{}")
	}
	truncated := false
	if len(content) > contextDebugArtifactContentLimit {
		content = content[:contextDebugArtifactContentLimit]
		truncated = true
	}
	return ArtifactDebugResponse{
		ID:               row.ID,
		Kind:             row.Kind,
		RelativePath:     row.RelativePath,
		ContentType:      row.ContentType,
		SizeBytes:        row.SizeBytes,
		SHA256:           nullableResponseString(row.Sha256),
		Metadata:         metadata,
		Content:          string(content),
		ContentTruncated: truncated,
	}, "", nil
}

func respondReviewContextError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, contextbundle.ErrReviewSessionNotFound):
		respondError(c, apperror.NotFound("review session was not found"))
	case errors.Is(err, contextbundle.ErrAgentConfigNotFound):
		respondError(c, apperror.NotFound("agent config was not found"))
	case errors.Is(err, contextbundle.ErrInvalidReviewContextPolicy):
		respondError(c, apperror.InvalidRequest(err.Error()))
	case errors.Is(err, contextbundle.ErrInvalidReviewContextSource):
		respondError(c, apperror.InvalidRequest(err.Error()))
	default:
		respondAppError(c, err)
	}
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
