package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/contextbundle"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/orchestrator"
)

const defaultReviewRuntimeLimitSeconds int64 = 1800

type CreateReviewSessionRequest struct {
	WorkspaceID         string          `json:"workspace_id"`
	SnapshotID          string          `json:"snapshot_id"`
	Title               string          `json:"title"`
	ReviewDepth         string          `json:"review_depth"`
	Preset              string          `json:"preset"`
	FocusPrompt         string          `json:"focus_prompt"`
	AgentConfigIDs      []string        `json:"agent_config_ids"`
	RuntimeLimitSeconds int64           `json:"runtime_limit_seconds"`
	ContextPolicy       json.RawMessage `json:"context_policy"`
}

type ReviewSessionResponse struct {
	ID                  string                       `json:"id"`
	WorkspaceID         string                       `json:"workspace_id"`
	RepositoryID        string                       `json:"repository_id"`
	SnapshotID          string                       `json:"snapshot_id"`
	Title               string                       `json:"title"`
	Status              string                       `json:"status"`
	ReviewDepth         string                       `json:"review_depth"`
	FocusPrompt         string                       `json:"focus_prompt,omitempty"`
	Preset              string                       `json:"preset,omitempty"`
	RuntimeLimitSeconds int64                        `json:"runtime_limit_seconds"`
	ContextPolicy       json.RawMessage              `json:"context_policy"`
	StartedAt           string                       `json:"started_at,omitempty"`
	CompletedAt         string                       `json:"completed_at,omitempty"`
	CreatedAt           string                       `json:"created_at"`
	UpdatedAt           string                       `json:"updated_at"`
	Agents              []ReviewSessionAgentResponse `json:"agents"`
}

type ReviewSessionAgentResponse struct {
	ID               string          `json:"id"`
	ReviewSessionID  string          `json:"review_session_id"`
	AgentConfigID    string          `json:"agent_config_id"`
	Role             string          `json:"role"`
	RunOrder         int64           `json:"run_order"`
	Enabled          bool            `json:"enabled"`
	SettingsOverride json.RawMessage `json:"settings_override"`
}

type normalizedReviewSessionCreate struct {
	Snapshot            dbgen.PullRequestSnapshot
	Repository          dbgen.Repository
	Title               string
	ReviewDepth         contextbundle.ReviewDepth
	Preset              string
	FocusPrompt         string
	RuntimeLimitSeconds int64
	ContextPolicyJSON   string
	AgentConfigs        []dbgen.AgentConfig
}

func createReviewSessionHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request CreateReviewSessionRequest
		if !bindJSON(c, &request) {
			return
		}
		normalized, appErr := normalizeReviewSessionCreate(c.Request.Context(), queries, request)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		sessionID := "review_session_" + newRequestID()
		session, err := queries.CreateReviewSession(c.Request.Context(), dbgen.CreateReviewSessionParams{
			ID:                  sessionID,
			WorkspaceID:         normalized.Repository.WorkspaceID,
			RepositoryID:        normalized.Repository.ID,
			SnapshotID:          normalized.Snapshot.ID,
			Title:               normalized.Title,
			Status:              "draft",
			ReviewDepth:         string(normalized.ReviewDepth),
			FocusPrompt:         nullableSQLString(normalized.FocusPrompt),
			Preset:              nullableSQLString(normalized.Preset),
			RuntimeLimitSeconds: normalized.RuntimeLimitSeconds,
			ContextPolicyJson:   normalized.ContextPolicyJSON,
			CreatedAt:           now,
			UpdatedAt:           now,
		})
		if err != nil {
			respondAppError(c, err)
			return
		}
		cleanup := true
		defer func() {
			if cleanup {
				_ = queries.DeleteReviewSession(context.Background(), sessionID)
			}
		}()

		for index, agent := range normalized.AgentConfigs {
			if _, err := queries.CreateReviewSessionAgent(c.Request.Context(), dbgen.CreateReviewSessionAgentParams{
				ID:                   "review_session_agent_" + newRequestID(),
				ReviewSessionID:      sessionID,
				AgentConfigID:        agent.ID,
				Role:                 agent.Role,
				RunOrder:             int64(index + 1),
				Enabled:              1,
				SettingsOverrideJson: "{}",
			}); err != nil {
				respondAppError(c, err)
				return
			}
		}
		cleanup = false

		response, appErr := reviewSessionResponse(c.Request.Context(), queries, session)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, response)
	}
}

func listReviewSessionsHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		workspaceID := strings.TrimSpace(c.Query("workspace_id"))
		if workspaceID == "" {
			respondError(c, apperror.InvalidRequest("workspace_id query parameter is required"))
			return
		}
		if _, err := queries.GetWorkspace(c.Request.Context(), workspaceID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondError(c, apperror.NotFound("workspace was not found"))
				return
			}
			respondError(c, apperror.Internal("failed to read workspace"))
			return
		}
		rows, err := queries.ListReviewSessionsByWorkspace(c.Request.Context(), workspaceID)
		if err != nil {
			respondError(c, apperror.Internal("failed to list review sessions"))
			return
		}
		response := make([]ReviewSessionResponse, 0, len(rows))
		for _, row := range rows {
			item, appErr := reviewSessionResponse(c.Request.Context(), queries, row)
			if appErr != nil {
				respondError(c, appErr)
				return
			}
			response = append(response, item)
		}
		respondOK(c, response)
	}
}

func getReviewSessionHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		row, appErr := getReviewSession(c.Request.Context(), queries, c.Param("id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		response, appErr := reviewSessionResponse(c.Request.Context(), queries, row)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, response)
	}
}

func startReviewSessionHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		if services.reviewWorkflowErr != nil || services.reviewWorkflow == nil {
			respondError(c, apperror.Internal("review workflow is not configured"))
			return
		}
		result, err := services.reviewWorkflow.Start(c.Request.Context(), c.Param("id"))
		if err != nil {
			respondReviewWorkflowError(c, err)
			return
		}
		response, appErr := reviewSessionResponse(c.Request.Context(), services.queries, result.Session)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, response)
	}
}

func cancelReviewSessionHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		if services.reviewWorkflowErr != nil || services.reviewWorkflow == nil {
			respondError(c, apperror.Internal("review workflow is not configured"))
			return
		}
		session, err := services.reviewWorkflow.Cancel(c.Request.Context(), c.Param("id"))
		if err != nil {
			respondReviewWorkflowError(c, err)
			return
		}
		response, appErr := reviewSessionResponse(c.Request.Context(), services.queries, session)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, response)
	}
}

func reviewSessionCheckpointHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		if services.reviewWorkflowErr != nil || services.reviewWorkflow == nil {
			respondError(c, apperror.Internal("review workflow is not configured"))
			return
		}
		checkpoint, err := services.reviewWorkflow.LoadCheckpoint(c.Request.Context(), c.Param("id"))
		if err != nil {
			respondReviewWorkflowError(c, err)
			return
		}
		respondOK(c, checkpoint)
	}
}

func normalizeReviewSessionCreate(ctx context.Context, queries *dbgen.Queries, request CreateReviewSessionRequest) (normalizedReviewSessionCreate, *apperror.Error) {
	snapshotID := strings.TrimSpace(request.SnapshotID)
	if snapshotID == "" {
		return normalizedReviewSessionCreate{}, apperror.InvalidRequest("snapshot_id is required")
	}
	snapshot, err := queries.GetPullRequestSnapshot(ctx, snapshotID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return normalizedReviewSessionCreate{}, apperror.NotFound("snapshot was not found")
		}
		return normalizedReviewSessionCreate{}, apperror.Internal("failed to read snapshot")
	}
	repository, err := queries.GetRepository(ctx, snapshot.RepositoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return normalizedReviewSessionCreate{}, apperror.NotFound("repository was not found")
		}
		return normalizedReviewSessionCreate{}, apperror.Internal("failed to read repository")
	}
	if workspaceID := strings.TrimSpace(request.WorkspaceID); workspaceID != "" && workspaceID != repository.WorkspaceID {
		return normalizedReviewSessionCreate{}, apperror.InvalidRequest("workspace_id does not match snapshot repository")
	}

	depth := contextbundle.ReviewDepth(strings.TrimSpace(request.ReviewDepth))
	if depth == "" {
		depth = contextbundle.ReviewDepthStandard
	}
	if !depth.Valid() {
		return normalizedReviewSessionCreate{}, apperror.InvalidRequest("review_depth is invalid")
	}
	runtimeLimit := request.RuntimeLimitSeconds
	if runtimeLimit < 0 {
		return normalizedReviewSessionCreate{}, apperror.InvalidRequest("runtime_limit_seconds cannot be negative")
	}
	if runtimeLimit == 0 {
		runtimeLimit = defaultReviewRuntimeLimitSeconds
	}
	policy, err := contextbundle.DecodeReviewContextPolicy(request.ContextPolicy)
	if err != nil {
		return normalizedReviewSessionCreate{}, apperror.InvalidRequest("context_policy is invalid: " + err.Error())
	}
	agentConfigs, appErr := selectedAgentConfigs(ctx, queries, request.AgentConfigIDs)
	if appErr != nil {
		return normalizedReviewSessionCreate{}, appErr
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		title = defaultReviewSessionTitle(snapshot)
	}

	return normalizedReviewSessionCreate{
		Snapshot:            snapshot,
		Repository:          repository,
		Title:               title,
		ReviewDepth:         depth,
		Preset:              strings.TrimSpace(request.Preset),
		FocusPrompt:         strings.TrimSpace(request.FocusPrompt),
		RuntimeLimitSeconds: runtimeLimit,
		ContextPolicyJSON:   string(policy.JSON()),
		AgentConfigs:        agentConfigs,
	}, nil
}

func selectedAgentConfigs(ctx context.Context, queries *dbgen.Queries, ids []string) ([]dbgen.AgentConfig, *apperror.Error) {
	if len(ids) == 0 {
		return nil, apperror.InvalidRequest("at least one agent_config_id is required")
	}
	seen := map[string]struct{}{}
	agents := make([]dbgen.AgentConfig, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, apperror.InvalidRequest("agent_config_ids cannot contain empty ids")
		}
		if _, ok := seen[id]; ok {
			return nil, apperror.InvalidRequest(fmt.Sprintf("agent_config_id %s is duplicated", id))
		}
		seen[id] = struct{}{}
		agent, err := queries.GetAgentConfig(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, apperror.NotFound("agent config was not found")
			}
			return nil, apperror.Internal("failed to read agent config")
		}
		if agent.Enabled == 0 {
			return nil, apperror.InvalidRequest(fmt.Sprintf("agent config %s is disabled", id))
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

func defaultReviewSessionTitle(snapshot dbgen.PullRequestSnapshot) string {
	if strings.TrimSpace(snapshot.PrTitle.String) != "" {
		return strings.TrimSpace(snapshot.PrTitle.String)
	}
	if snapshot.PrNumber.Valid && strings.TrimSpace(snapshot.Repo.String) != "" {
		return fmt.Sprintf("%s#%d", strings.TrimSpace(snapshot.Repo.String), snapshot.PrNumber.Int64)
	}
	return "Review " + snapshot.ID
}

func reviewSessionResponse(ctx context.Context, queries *dbgen.Queries, row dbgen.ReviewSession) (ReviewSessionResponse, *apperror.Error) {
	policy := json.RawMessage(row.ContextPolicyJson)
	if len(policy) == 0 || !json.Valid(policy) {
		return ReviewSessionResponse{}, apperror.Internal("stored review context policy is invalid")
	}
	agents, err := queries.ListReviewSessionAgents(ctx, row.ID)
	if err != nil {
		return ReviewSessionResponse{}, apperror.Internal("failed to list review session agents")
	}
	responseAgents := make([]ReviewSessionAgentResponse, 0, len(agents))
	for _, agent := range agents {
		item, appErr := reviewSessionAgentResponse(agent)
		if appErr != nil {
			return ReviewSessionResponse{}, appErr
		}
		responseAgents = append(responseAgents, item)
	}
	return ReviewSessionResponse{
		ID:                  row.ID,
		WorkspaceID:         row.WorkspaceID,
		RepositoryID:        row.RepositoryID,
		SnapshotID:          row.SnapshotID,
		Title:               row.Title,
		Status:              row.Status,
		ReviewDepth:         row.ReviewDepth,
		FocusPrompt:         nullableResponseString(row.FocusPrompt),
		Preset:              nullableResponseString(row.Preset),
		RuntimeLimitSeconds: row.RuntimeLimitSeconds,
		ContextPolicy:       policy,
		StartedAt:           nullableResponseString(row.StartedAt),
		CompletedAt:         nullableResponseString(row.CompletedAt),
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
		Agents:              responseAgents,
	}, nil
}

func reviewSessionAgentResponse(row dbgen.ReviewSessionAgent) (ReviewSessionAgentResponse, *apperror.Error) {
	settings := json.RawMessage(row.SettingsOverrideJson)
	if len(settings) == 0 || !json.Valid(settings) {
		return ReviewSessionAgentResponse{}, apperror.Internal("stored review session agent settings are invalid")
	}
	return ReviewSessionAgentResponse{
		ID:               row.ID,
		ReviewSessionID:  row.ReviewSessionID,
		AgentConfigID:    row.AgentConfigID,
		Role:             row.Role,
		RunOrder:         row.RunOrder,
		Enabled:          row.Enabled != 0,
		SettingsOverride: settings,
	}, nil
}

func getReviewSession(ctx context.Context, queries *dbgen.Queries, id string) (dbgen.ReviewSession, *apperror.Error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return dbgen.ReviewSession{}, apperror.InvalidRequest("review session id is required")
	}
	row, err := queries.GetReviewSession(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.ReviewSession{}, apperror.NotFound("review session was not found")
		}
		return dbgen.ReviewSession{}, apperror.Internal("failed to read review session")
	}
	return row, nil
}

func respondReviewWorkflowError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, orchestrator.ErrReviewSessionNotFound):
		respondError(c, apperror.NotFound("review session was not found"))
	case errors.Is(err, orchestrator.ErrInvalidStatusTransition):
		respondError(c, apperror.InvalidRequest(err.Error()))
	case errors.Is(err, orchestrator.ErrNoEnabledReviewAgents):
		respondError(c, apperror.InvalidRequest("review session has no enabled agents"))
	case errors.Is(err, orchestrator.ErrInvalidAgentConfiguration):
		respondError(c, apperror.InvalidRequest(err.Error()))
	case errors.Is(err, orchestrator.ErrServiceNotConfigured):
		respondError(c, apperror.Internal("review workflow is not configured"))
	default:
		respondError(c, apperror.Internal("review workflow failed"))
	}
}
