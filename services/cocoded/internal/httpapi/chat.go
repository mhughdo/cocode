package httpapi

import (
	"database/sql"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/chat"
)

func reviewSessionChatThreadHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		service := services.chat
		if service == nil {
			service = &chat.Service{Database: services.database, Queries: services.queries}
		}
		view, err := service.EnsureSessionThread(c.Request.Context(), c.Param("id"))
		if err != nil {
			respondError(c, chatError(err))
			return
		}
		respondOK(c, view)
	}
}

func chatThreadHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		service := services.chat
		if service == nil {
			service = &chat.Service{Database: services.database, Queries: services.queries}
		}
		view, err := service.LoadThread(c.Request.Context(), c.Param("thread_id"))
		if err != nil {
			respondError(c, chatError(err))
			return
		}
		respondOK(c, view)
	}
}

func createReviewSessionChatTurnHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request chat.AskParams
		if !bindJSON(c, &request) {
			return
		}
		request.ReviewSessionID = c.Param("id")
		service := services.chat
		if service == nil {
			service = &chat.Service{Database: services.database, Queries: services.queries}
		}
		result, err := service.Ask(c.Request.Context(), request)
		if err != nil {
			respondError(c, chatError(err))
			return
		}
		respondOK(c, result)
	}
}

func chatError(err error) *apperror.Error {
	switch {
	case errors.Is(err, chat.ErrServiceNotConfigured):
		return apperror.Internal("chat service is not configured")
	case errors.Is(err, chat.ErrReviewSessionNotFound), errors.Is(err, sql.ErrNoRows):
		return apperror.NotFound("review session was not found")
	case errors.Is(err, chat.ErrThreadNotFound):
		return apperror.NotFound("chat thread was not found")
	case errors.Is(err, chat.ErrInvalidMessage):
		return apperror.InvalidRequest("chat message is invalid")
	case errors.Is(err, chat.ErrInvalidTurn):
		return apperror.InvalidRequest("chat turn is invalid")
	case errors.Is(err, chat.ErrAgentConfigNotFound):
		return apperror.NotFound("chat agent config was not found")
	case errors.Is(err, chat.ErrInvalidAgentConfig):
		return apperror.InvalidRequest("chat agent config is invalid")
	default:
		return apperror.Internal("failed to load centralized chat")
	}
}
