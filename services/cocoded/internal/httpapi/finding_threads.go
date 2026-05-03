package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/followup"
)

type FindingThreadViewResponse struct {
	Finding  FindingResponse                `json:"finding"`
	Thread   FindingThreadResponse          `json:"thread"`
	Messages []FindingThreadMessageResponse `json:"messages"`
}

type FindingThreadResponse struct {
	ID              string `json:"id"`
	FindingID       string `json:"finding_id"`
	ReviewSessionID string `json:"review_session_id"`
	Title           string `json:"title"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type FindingThreadMessageResponse struct {
	ID            string          `json:"id"`
	ThreadID      string          `json:"thread_id"`
	Role          string          `json:"role"`
	AgentConfigID string          `json:"agent_config_id,omitempty"`
	Content       string          `json:"content"`
	EvidenceRefs  json.RawMessage `json:"evidence_refs"`
	ArtifactID    string          `json:"artifact_id,omitempty"`
	CreatedAt     string          `json:"created_at"`
}

type AskFindingQuestionRequest struct {
	Question      string          `json:"question"`
	AgentConfigID string          `json:"agent_config_id"`
	ContextPolicy json.RawMessage `json:"context_policy"`
}

type FindingQuickActionRequest struct {
	Action        string          `json:"action"`
	Reason        string          `json:"reason"`
	AgentConfigID string          `json:"agent_config_id"`
	ContextPolicy json.RawMessage `json:"context_policy"`
}

type AskFindingQuestionResponse struct {
	Thread           FindingThreadViewResponse    `json:"thread"`
	UserMessage      FindingThreadMessageResponse `json:"user_message"`
	AssistantMessage FindingThreadMessageResponse `json:"assistant_message"`
	AgentRunID       string                       `json:"agent_run_id,omitempty"`
	ContextBundleID  string                       `json:"context_bundle_id,omitempty"`
}

type FindingQuickActionResponse struct {
	Action           string                        `json:"action"`
	Thread           FindingThreadViewResponse     `json:"thread"`
	Finding          FindingResponse               `json:"finding"`
	Decision         *HumanDecisionResponse        `json:"decision,omitempty"`
	Message          *FindingThreadMessageResponse `json:"message,omitempty"`
	AssistantMessage *FindingThreadMessageResponse `json:"assistant_message,omitempty"`
	AgentRunID       string                        `json:"agent_run_id,omitempty"`
	ContextBundleID  string                        `json:"context_bundle_id,omitempty"`
}

func findingThreadHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		service := services.followups
		if service == nil {
			service = &followup.Service{Queries: services.queries}
		}
		view, err := service.EnsureThread(c.Request.Context(), followup.EnsureThreadParams{
			FindingID:       c.Param("finding_id"),
			ReviewSessionID: c.Param("id"),
		})
		if err != nil {
			respondError(c, followupError(err))
			return
		}
		response, appErr := findingThreadViewResponse(view)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, response)
	}
}

func findingQuickActionHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request FindingQuickActionRequest
		if !bindJSON(c, &request) {
			return
		}
		service := services.followups
		if service == nil {
			service = &followup.Service{Database: services.database, Queries: services.queries}
		}
		result, err := service.RunQuickAction(c.Request.Context(), followup.QuickActionParams{
			FindingID:       c.Param("finding_id"),
			ReviewSessionID: c.Param("id"),
			Action:          request.Action,
			Reason:          request.Reason,
			AgentConfigID:   request.AgentConfigID,
			ContextPolicy:   request.ContextPolicy,
		})
		if err != nil {
			respondError(c, followupError(err))
			return
		}
		response, appErr := findingQuickActionResponse(result)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, response)
	}
}

func askFindingQuestionHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request AskFindingQuestionRequest
		if !bindJSON(c, &request) {
			return
		}
		service := services.followups
		if service == nil {
			service = &followup.Service{Queries: services.queries}
		}
		result, err := service.AskQuestion(c.Request.Context(), followup.AskQuestionParams{
			FindingID:       c.Param("finding_id"),
			ReviewSessionID: c.Param("id"),
			Question:        request.Question,
			AgentConfigID:   request.AgentConfigID,
			ContextPolicy:   request.ContextPolicy,
		})
		if err != nil {
			respondError(c, followupError(err))
			return
		}
		thread, appErr := findingThreadViewResponse(result.View)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		userMessage, appErr := findingThreadMessageResponse(result.UserMessage)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		assistantMessage, appErr := findingThreadMessageResponse(result.AssistantMessage)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, AskFindingQuestionResponse{
			Thread:           thread,
			UserMessage:      userMessage,
			AssistantMessage: assistantMessage,
			AgentRunID:       result.AgentRun.ID,
			ContextBundleID:  result.ContextBundle.ID,
		})
	}
}

func findingQuickActionResponse(result followup.QuickActionResult) (FindingQuickActionResponse, *apperror.Error) {
	thread, appErr := findingThreadViewResponse(result.View)
	if appErr != nil {
		return FindingQuickActionResponse{}, appErr
	}
	response := FindingQuickActionResponse{
		Action:          result.Action,
		Thread:          thread,
		Finding:         findingResponse(result.Finding),
		AgentRunID:      result.AgentRun.ID,
		ContextBundleID: result.ContextBundle.ID,
	}
	if result.Decision.ID != "" {
		decision, appErr := humanDecisionResponse(result.Decision)
		if appErr != nil {
			return FindingQuickActionResponse{}, appErr
		}
		response.Decision = &decision
	}
	if result.Message.ID != "" {
		message, appErr := findingThreadMessageResponse(result.Message)
		if appErr != nil {
			return FindingQuickActionResponse{}, appErr
		}
		response.Message = &message
	}
	if result.AssistantMessage.ID != "" {
		message, appErr := findingThreadMessageResponse(result.AssistantMessage)
		if appErr != nil {
			return FindingQuickActionResponse{}, appErr
		}
		response.AssistantMessage = &message
	}
	return response, nil
}

func findingThreadViewResponse(view followup.ThreadView) (FindingThreadViewResponse, *apperror.Error) {
	messages := make([]FindingThreadMessageResponse, 0, len(view.Messages))
	for _, message := range view.Messages {
		item, appErr := findingThreadMessageResponse(message)
		if appErr != nil {
			return FindingThreadViewResponse{}, appErr
		}
		messages = append(messages, item)
	}
	return FindingThreadViewResponse{
		Finding:  findingResponse(view.Finding),
		Thread:   findingThreadResponse(view.Thread),
		Messages: messages,
	}, nil
}

func findingThreadResponse(row dbgen.FindingThread) FindingThreadResponse {
	return FindingThreadResponse{
		ID:              row.ID,
		FindingID:       row.FindingID,
		ReviewSessionID: row.ReviewSessionID,
		Title:           row.Title,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func findingThreadMessageResponse(row dbgen.FindingThreadMessage) (FindingThreadMessageResponse, *apperror.Error) {
	refs := json.RawMessage(row.EvidenceRefsJson)
	if len(refs) == 0 || !json.Valid(refs) {
		return FindingThreadMessageResponse{}, apperror.Internal("stored finding thread evidence refs are invalid")
	}
	return FindingThreadMessageResponse{
		ID:            row.ID,
		ThreadID:      row.ThreadID,
		Role:          row.Role,
		AgentConfigID: nullableValue(row.AgentConfigID),
		Content:       row.Content,
		EvidenceRefs:  refs,
		ArtifactID:    nullableValue(row.ArtifactID),
		CreatedAt:     row.CreatedAt,
	}, nil
}

func followupError(err error) *apperror.Error {
	switch {
	case errors.Is(err, followup.ErrServiceNotConfigured):
		return apperror.Internal("follow-up service is not configured")
	case errors.Is(err, followup.ErrFindingNotFound), errors.Is(err, sql.ErrNoRows):
		return apperror.NotFound("finding was not found")
	case errors.Is(err, followup.ErrThreadNotFound):
		return apperror.NotFound("finding thread was not found")
	case errors.Is(err, followup.ErrInvalidMessage):
		return apperror.InvalidRequest("finding thread message is invalid")
	case errors.Is(err, followup.ErrInvalidQuickAction):
		return apperror.InvalidRequest("follow-up quick action is invalid")
	case errors.Is(err, followup.ErrInvalidAgentConfig):
		return apperror.InvalidRequest("follow-up agent config is invalid")
	case errors.Is(err, followup.ErrAgentConfigNotFound):
		return apperror.NotFound("follow-up agent config was not found")
	case errors.Is(err, followup.ErrAgentRunFailed):
		return apperror.Internal("follow-up agent run failed")
	default:
		return apperror.Internal("failed to load finding thread")
	}
}
