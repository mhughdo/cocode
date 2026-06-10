package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

type AuditLogResponse struct {
	Entries []AuditLogEntryResponse `json:"entries"`
}

type AuditLogEntryResponse struct {
	ID                  string                      `json:"id"`
	Kind                string                      `json:"kind"`
	Title               string                      `json:"title"`
	ReviewSessionID     string                      `json:"review_session_id"`
	Level               string                      `json:"level,omitempty"`
	Status              string                      `json:"status,omitempty"`
	DurationMs          int64                       `json:"duration_ms,omitempty"`
	FailureReason       string                      `json:"failure_reason,omitempty"`
	FindingCounts       *AuditFindingCountsResponse `json:"finding_counts,omitempty"`
	Sequence            int64                       `json:"sequence,omitempty"`
	FindingID           string                      `json:"finding_id,omitempty"`
	AgentRunID          string                      `json:"agent_run_id,omitempty"`
	ArtifactID          string                      `json:"artifact_id,omitempty"`
	CopyPacketID        string                      `json:"copy_packet_id,omitempty"`
	PublishDraftID      string                      `json:"publish_draft_id,omitempty"`
	GitHubPublicationID string                      `json:"github_publication_id,omitempty"`
	ReviewEvent         string                      `json:"review_event,omitempty"`
	CreatedAt           string                      `json:"created_at"`
	Metadata            json.RawMessage             `json:"metadata"`
}

type AuditFindingCountsResponse struct {
	Candidates           int            `json:"candidates"`
	Findings             int            `json:"findings"`
	BySeverity           map[string]int `json:"by_severity,omitempty"`
	ByVerificationStatus map[string]int `json:"by_verification_status,omitempty"`
	ByDecisionStatus     map[string]int `json:"by_decision_status,omitempty"`
}

func reviewSessionAuditLogHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("id")
		response, appErr := buildAuditLogResponse(c.Request.Context(), services.queries, sessionID)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, response)
	}
}

func buildAuditLogResponse(ctx context.Context, queries *dbgen.Queries, sessionID string) (AuditLogResponse, *apperror.Error) {
	if _, err := queries.GetReviewSession(ctx, sessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AuditLogResponse{}, apperror.NotFound("review session was not found")
		}
		return AuditLogResponse{}, apperror.Internal("failed to read review session")
	}

	entries := []AuditLogEntryResponse{}
	events, err := queries.ListEventsByReviewSession(ctx, sql.NullString{String: sessionID, Valid: true})
	if err != nil {
		return AuditLogResponse{}, apperror.Internal("failed to list review events")
	}
	for _, event := range events {
		payload := auditJSON(event.PayloadJson)
		status, durationMs, failureReason, findingCounts := auditEventMetrics(payload)
		entries = append(entries, AuditLogEntryResponse{
			ID:              event.ID,
			Kind:            "event",
			Title:           event.Type,
			ReviewSessionID: sessionID,
			Level:           event.Level,
			Status:          status,
			DurationMs:      durationMs,
			FailureReason:   failureReason,
			FindingCounts:   findingCounts,
			Sequence:        event.Sequence,
			AgentRunID:      nullableEventResponseString(event.AgentRunID),
			ArtifactID:      nullableEventResponseString(event.ArtifactID),
			CreatedAt:       event.CreatedAt,
			Metadata:        payload,
		})
	}

	decisions, err := queries.ListHumanDecisionsBySession(ctx, sessionID)
	if err != nil {
		return AuditLogResponse{}, apperror.Internal("failed to list human decisions")
	}
	for _, decision := range decisions {
		entries = append(entries, AuditLogEntryResponse{
			ID:              decision.ID,
			Kind:            "decision",
			Title:           "Finding " + decision.Decision,
			ReviewSessionID: sessionID,
			Status:          decision.Decision,
			FindingID:       decision.FindingID,
			CreatedAt:       decision.CreatedAt,
			Metadata:        auditJSON(decision.MetadataJson),
		})
	}

	copyPackets, err := queries.ListCopyPacketsBySession(ctx, sessionID)
	if err != nil {
		return AuditLogResponse{}, apperror.Internal("failed to list copy packets")
	}
	for _, packet := range copyPackets {
		entries = append(entries, AuditLogEntryResponse{
			ID:              packet.ID,
			Kind:            "copy_packet",
			Title:           "Copy packet created",
			ReviewSessionID: sessionID,
			Status:          packet.Format,
			FindingID:       nullableValue(packet.FindingID),
			ArtifactID:      packet.ContentArtifactID,
			CopyPacketID:    packet.ID,
			CreatedAt:       packet.CreatedAt,
			Metadata: auditObject(map[string]any{
				"format":         packet.Format,
				"finding_count":  packet.FindingCount,
				"token_estimate": packet.TokenEstimate,
				"copied_at":      nullableValue(packet.CopiedAt),
			}),
		})
		if packet.CopiedAt.Valid {
			entries = append(entries, AuditLogEntryResponse{
				ID:              packet.ID + ":copied",
				Kind:            "copy_packet_copied",
				Title:           "Copy packet copied",
				ReviewSessionID: sessionID,
				Status:          "copied",
				FindingID:       nullableValue(packet.FindingID),
				ArtifactID:      packet.ContentArtifactID,
				CopyPacketID:    packet.ID,
				CreatedAt:       packet.CopiedAt.String,
				Metadata: auditObject(map[string]any{
					"format":         packet.Format,
					"finding_count":  packet.FindingCount,
					"token_estimate": packet.TokenEstimate,
				}),
			})
		}
	}

	drafts, err := queries.ListPublishDraftsBySession(ctx, sessionID)
	if err != nil {
		return AuditLogResponse{}, apperror.Internal("failed to list publish drafts")
	}
	for _, draft := range drafts {
		entries = append(entries, AuditLogEntryResponse{
			ID:              draft.ID,
			Kind:            "publish_draft",
			Title:           "GitHub preview created",
			ReviewSessionID: sessionID,
			Status:          draft.Status,
			ArtifactID:      nullableValue(draft.ArtifactID),
			PublishDraftID:  draft.ID,
			ReviewEvent:     nullableValue(draft.ReviewEvent),
			CreatedAt:       draft.CreatedAt,
			Metadata: auditObject(map[string]any{
				"provider":       draft.Provider,
				"review_event":   nullableValue(draft.ReviewEvent),
				"comment_count":  auditJSONArrayCount(draft.CommentsJson),
				"body_preview":   nullablePreview(draft.Body, 220),
				"body_available": draft.Body.Valid,
			}),
		})
	}

	publications, err := queries.ListGitHubPublicationsBySession(ctx, sessionID)
	if err != nil {
		return AuditLogResponse{}, apperror.Internal("failed to list GitHub publications")
	}
	for _, publication := range publications {
		entries = append(entries, AuditLogEntryResponse{
			ID:                  publication.ID,
			Kind:                "github_publication",
			Title:               "GitHub publication recorded",
			ReviewSessionID:     sessionID,
			Status:              publication.Status,
			PublishDraftID:      nullableValue(publication.PublishDraftID),
			GitHubPublicationID: publication.ID,
			CreatedAt:           publication.CreatedAt,
			Metadata: auditObject(map[string]any{
				"github_review_id":   nullableValue(publication.GithubReviewID),
				"github_comment_ids": auditJSONArrayCount(publication.GithubCommentIdsJson),
				"error_message":      nullableValue(publication.ErrorMessage),
			}),
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].CreatedAt != entries[j].CreatedAt {
			return entries[i].CreatedAt > entries[j].CreatedAt
		}
		if entries[i].Sequence != entries[j].Sequence {
			return entries[i].Sequence > entries[j].Sequence
		}
		return entries[i].ID > entries[j].ID
	})

	return AuditLogResponse{Entries: entries}, nil
}

func auditJSON(value string) json.RawMessage {
	if value == "" || !json.Valid([]byte(value)) {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(value)
}

func auditEventMetrics(payload json.RawMessage) (string, int64, string, *AuditFindingCountsResponse) {
	var decoded struct {
		Status        string                      `json:"status"`
		DurationMs    int64                       `json:"duration_ms"`
		FailureReason string                      `json:"failure_reason"`
		FindingCounts *AuditFindingCountsResponse `json:"finding_counts"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return "", 0, "", nil
	}
	return decoded.Status, decoded.DurationMs, decoded.FailureReason, decoded.FindingCounts
}

func auditObject(value map[string]any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil || !json.Valid(encoded) {
		return json.RawMessage(`{}`)
	}
	return encoded
}

func auditJSONArrayCount(value string) int {
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(value), &items); err != nil {
		return 0
	}
	return len(items)
}

func nullablePreview(value sql.NullString, limit int) string {
	if !value.Valid {
		return ""
	}
	return trimAuditPreview(value.String, limit)
}

func trimAuditPreview(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
