package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

type ReviewEventResponse struct {
	ID              string          `json:"id"`
	ReviewSessionID string          `json:"review_session_id"`
	AgentRunID      string          `json:"agent_run_id,omitempty"`
	Type            string          `json:"type"`
	Level           string          `json:"level"`
	Sequence        int64           `json:"sequence"`
	Payload         json.RawMessage `json:"payload"`
	ArtifactID      string          `json:"artifact_id,omitempty"`
	CreatedAt       string          `json:"created_at"`
}

func reviewSessionEventsHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		if services.eventBus == nil {
			respondError(c, apperror.Internal("event bus is not configured"))
			return
		}
		reviewSessionID := strings.TrimSpace(c.Param("id"))
		if reviewSessionID == "" {
			respondError(c, apperror.InvalidRequest("review session id is required"))
			return
		}
		if _, err := services.queries.GetReviewSession(c.Request.Context(), reviewSessionID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondError(c, apperror.NotFound("review session was not found"))
				return
			}
			respondError(c, apperror.Internal("failed to read review session"))
			return
		}
		lastSequence, appErr := lastEventSequence(c)
		if appErr != nil {
			respondError(c, appErr)
			return
		}

		prepareSSE(c)
		events, unsubscribe, err := services.eventBus.Subscribe(reviewSessionID)
		if err != nil {
			respondError(c, apperror.Internal("failed to subscribe to review events"))
			return
		}
		defer unsubscribe()

		replay, err := services.eventBus.ListByReviewSession(c.Request.Context(), reviewSessionID)
		if err != nil {
			respondError(c, apperror.Internal("failed to read review events"))
			return
		}
		sentSequence := lastSequence
		for _, event := range replay {
			if event.Sequence <= sentSequence {
				continue
			}
			if err := writeReviewSSE(c, event); err != nil {
				return
			}
			sentSequence = event.Sequence
		}

		for {
			select {
			case <-c.Request.Context().Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				if event.Sequence <= sentSequence {
					continue
				}
				if err := writeReviewSSE(c, event); err != nil {
					return
				}
				sentSequence = event.Sequence
			}
		}
	}
}

func prepareSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func writeReviewSSE(c *gin.Context, event dbgen.Event) error {
	return WriteSSE(c, SSEEvent{
		ID:    fmt.Sprintf("%d", event.Sequence),
		Event: "review.event",
		Data:  reviewEventResponse(event),
	})
}

func reviewEventResponse(event dbgen.Event) ReviewEventResponse {
	payload := json.RawMessage(event.PayloadJson)
	if len(payload) == 0 || !json.Valid(payload) {
		payload = json.RawMessage("{}")
	}
	return ReviewEventResponse{
		ID:              event.ID,
		ReviewSessionID: nullableEventResponseString(event.ReviewSessionID),
		AgentRunID:      nullableEventResponseString(event.AgentRunID),
		Type:            event.Type,
		Level:           event.Level,
		Sequence:        event.Sequence,
		Payload:         payload,
		ArtifactID:      nullableEventResponseString(event.ArtifactID),
		CreatedAt:       event.CreatedAt,
	}
}

func lastEventSequence(c *gin.Context) (int64, *apperror.Error) {
	value := strings.TrimSpace(c.GetHeader("Last-Event-ID"))
	if value == "" {
		value = strings.TrimSpace(c.Query("after_sequence"))
	}
	if value == "" {
		return 0, nil
	}
	sequence, err := strconv.ParseInt(value, 10, 64)
	if err != nil || sequence < 0 {
		return 0, apperror.InvalidRequest("Last-Event-ID must be a non-negative event sequence")
	}
	return sequence, nil
}

func nullableEventResponseString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
