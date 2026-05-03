package httpapi

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/exports"
)

type CreateCopyPacketRequest struct {
	Format                 string   `json:"format"`
	FindingIDs             []string `json:"finding_ids"`
	IncludeCodeSnippets    *bool    `json:"include_code_snippets"`
	IncludeEvidence        *bool    `json:"include_evidence"`
	IncludeCounterEvidence *bool    `json:"include_counter_evidence"`
	TargetAgent            string   `json:"target_agent"`
}

type CreateCopyPacketResponse struct {
	CopyPacketID      string `json:"copy_packet_id"`
	Content           string `json:"content"`
	Format            string `json:"format"`
	FindingCount      int64  `json:"finding_count"`
	TokenEstimate     int64  `json:"token_estimate"`
	ContentArtifactID string `json:"content_artifact_id"`
}

type MarkCopyPacketCopiedResponse struct {
	CopyPacketID string                  `json:"copy_packet_id"`
	CopiedAt     string                  `json:"copied_at"`
	FindingIDs   []string                `json:"finding_ids"`
	Decisions    []HumanDecisionResponse `json:"decisions"`
}

func createCopyPacketHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request CreateCopyPacketRequest
		if !bindJSON(c, &request) {
			return
		}
		service := services.copyPackets
		if service == nil {
			service = &exports.Service{Queries: services.queries}
		}
		result, err := service.CreateCopyPacket(c.Request.Context(), exports.CreateCopyPacketParams{
			ReviewSessionID:        c.Param("id"),
			FindingID:              c.Param("finding_id"),
			FindingIDs:             request.FindingIDs,
			Format:                 exports.Format(request.Format),
			IncludeCodeSnippets:    boolValue(request.IncludeCodeSnippets, false),
			IncludeEvidence:        boolValue(request.IncludeEvidence, true),
			IncludeCounterEvidence: boolValue(request.IncludeCounterEvidence, true),
			TargetAgent:            request.TargetAgent,
		})
		if err != nil {
			respondError(c, copyPacketError(err))
			return
		}
		respondOK(c, CreateCopyPacketResponse{
			CopyPacketID:      result.Packet.ID,
			Content:           result.Rendered.Content,
			Format:            result.Packet.Format,
			FindingCount:      result.Packet.FindingCount,
			TokenEstimate:     result.Packet.TokenEstimate,
			ContentArtifactID: result.Packet.ContentArtifactID,
		})
	}
}

func markCopyPacketCopiedHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		service := services.copyPackets
		if service == nil {
			service = &exports.Service{Database: services.database, Queries: services.queries}
		}
		result, err := service.MarkCopyPacketCopied(c.Request.Context(), exports.MarkCopyPacketCopiedParams{
			CopyPacketID: c.Param("copy_packet_id"),
		})
		if err != nil {
			respondError(c, copyPacketError(err))
			return
		}
		decisions := make([]HumanDecisionResponse, 0, len(result.Decisions))
		for _, decision := range result.Decisions {
			response, appErr := humanDecisionResponse(decision)
			if appErr != nil {
				respondError(c, appErr)
				return
			}
			decisions = append(decisions, response)
		}
		respondOK(c, MarkCopyPacketCopiedResponse{
			CopyPacketID: result.Packet.ID,
			CopiedAt:     nullableValue(result.Packet.CopiedAt),
			FindingIDs:   result.FindingIDs,
			Decisions:    decisions,
		})
	}
}

func copyPacketError(err error) *apperror.Error {
	switch {
	case errors.Is(err, exports.ErrInvalidCopyPacket):
		return apperror.InvalidRequest("copy packet request is invalid")
	case errors.Is(err, exports.ErrCopyPacketSourceNotFound):
		return apperror.NotFound("copy packet source was not found")
	default:
		return apperror.Internal("failed to create copy packet")
	}
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
