package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/diffparse"
	"github.com/hughdo/cocode/services/cocoded/internal/githubpr"
)

type CreateGitHubPreviewRequest struct {
	FindingIDs  []string `json:"finding_ids"`
	ReviewEvent string   `json:"review_event"`
}

type GitHubPreviewResponse struct {
	PublishDraftID string                        `json:"publish_draft_id"`
	ArtifactID     string                        `json:"artifact_id"`
	ReviewEvent    string                        `json:"review_event"`
	Body           string                        `json:"body"`
	Comments       []githubpr.ReviewCommentDraft `json:"comments"`
	Warnings       []githubpr.AnchorWarning      `json:"warnings"`
	Checklist      GitHubPreviewChecklist        `json:"checklist"`
}

type GitHubPreviewChecklist struct {
	HasSelectedFindings   bool `json:"has_selected_findings"`
	HasInlineComments     bool `json:"has_inline_comments"`
	HasUnanchoredComments bool `json:"has_unanchored_comments"`
	CanPublishInline      bool `json:"can_publish_inline"`
	CanPublishSummaryOnly bool `json:"can_publish_summary_only"`
}

func createGitHubPreviewHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request CreateGitHubPreviewRequest
		if !bindJSON(c, &request) {
			return
		}
		if services.artifacts == nil {
			respondError(c, apperrorInternal("artifact store is not configured"))
			return
		}
		reviewEvent, appErr := normalizeReviewEvent(request.ReviewEvent)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		session, err := services.queries.GetReviewSession(c.Request.Context(), c.Param("id"))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondError(c, apperrorNotFound("review session was not found"))
				return
			}
			respondError(c, apperrorInternal("failed to read review session"))
			return
		}
		findings, appErr := previewFindings(c.Request.Context(), services.queries, session.ID, request.FindingIDs)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		snapshot, err := services.queries.GetPullRequestSnapshot(c.Request.Context(), session.SnapshotID)
		if err != nil {
			respondError(c, apperrorInternal("failed to read review snapshot"))
			return
		}
		diffFiles, diffWarnings := previewDiffFiles(c.Request.Context(), services, snapshot)
		preview, err := githubpr.BuildReviewPreview(githubpr.ReviewPreviewInput{
			Title:    previewTitle(reviewEvent, session),
			Findings: findings,
			Diff:     diffFiles,
		})
		if err != nil {
			respondError(c, apperrorInvalid("GitHub preview request is invalid"))
			return
		}
		preview.Warnings = append(diffWarnings, preview.Warnings...)
		checklist := githubPreviewChecklist(preview)
		commentsJSON, err := githubpr.ReviewPreviewCommentsJSON(preview)
		if err != nil {
			respondError(c, apperrorInternal("failed to encode GitHub preview comments"))
			return
		}
		artifactRow, draft, err := persistGitHubPreview(c.Request.Context(), services, session, reviewEvent, preview, checklist, commentsJSON)
		if err != nil {
			respondError(c, apperrorInternal("failed to store GitHub preview"))
			return
		}
		respondOK(c, GitHubPreviewResponse{
			PublishDraftID: draft.ID,
			ArtifactID:     artifactRow.ID,
			ReviewEvent:    reviewEvent,
			Body:           preview.Body,
			Comments:       preview.Comments,
			Warnings:       preview.Warnings,
			Checklist:      checklist,
		})
	}
}

func previewFindings(ctx context.Context, queries *dbgen.Queries, reviewSessionID string, requestedIDs []string) ([]githubpr.PreviewFinding, *apperror.Error) {
	rows, err := queries.ListFindingsBySession(ctx, reviewSessionID)
	if err != nil {
		return nil, apperrorInternal("failed to list findings")
	}
	selectedRows := make([]dbgen.Finding, 0, len(rows))
	if len(requestedIDs) == 0 {
		for _, row := range rows {
			if row.DecisionStatus == "accepted" {
				selectedRows = append(selectedRows, row)
			}
		}
	} else {
		byID := make(map[string]dbgen.Finding, len(rows))
		for _, row := range rows {
			byID[row.ID] = row
		}
		for _, id := range uniqueRequestIDs(requestedIDs) {
			row, ok := byID[id]
			if !ok {
				return nil, apperrorNotFound("finding was not found")
			}
			selectedRows = append(selectedRows, row)
		}
	}
	if duplicate := firstPublishedDuplicate(rows, selectedRows); duplicate != "" {
		return nil, apperrorInvalid("finding was already published: " + duplicate)
	}
	if len(selectedRows) == 0 {
		return nil, apperrorInvalid("at least one finding is required")
	}
	findings := make([]githubpr.PreviewFinding, 0, len(selectedRows))
	for _, row := range selectedRows {
		findings = append(findings, githubpr.PreviewFinding{
			ID:                 row.ID,
			CanonicalClaim:     row.CanonicalClaim,
			Category:           row.Category,
			Severity:           row.Severity,
			VerificationStatus: row.VerificationStatus,
			DecisionStatus:     row.DecisionStatus,
			PrimaryPath:        nullableValue(row.PrimaryPath),
			PrimaryStartLine:   nullableInt64Value(row.PrimaryStartLine),
			PrimaryEndLine:     nullableInt64Value(row.PrimaryEndLine),
			PrimarySide:        githubpr.SideUnknown,
			SuggestedFix:       nullableValue(row.SuggestedFix),
			DraftComment:       nullableValue(row.DraftComment),
		})
	}
	return findings, nil
}

func firstPublishedDuplicate(all []dbgen.Finding, selected []dbgen.Finding) string {
	for _, row := range selected {
		if row.DecisionStatus == "published" {
			return row.ID
		}
		for _, prior := range all {
			if prior.ID == row.ID || prior.DecisionStatus != "published" {
				continue
			}
			if sameFindingPublicationTarget(prior, row) {
				return row.ID
			}
		}
	}
	return ""
}

func sameFindingPublicationTarget(left dbgen.Finding, right dbgen.Finding) bool {
	if left.Fingerprint != "" && left.Fingerprint == right.Fingerprint {
		return true
	}
	return nullableValue(left.PrimaryPath) != "" &&
		nullableValue(left.PrimaryPath) == nullableValue(right.PrimaryPath) &&
		nullableInt64Value(left.PrimaryStartLine) == nullableInt64Value(right.PrimaryStartLine) &&
		nullableInt64Value(left.PrimaryEndLine) == nullableInt64Value(right.PrimaryEndLine)
}

func previewDiffFiles(ctx context.Context, services routerServices, snapshot dbgen.PullRequestSnapshot) ([]diffparse.File, []githubpr.AnchorWarning) {
	if !snapshot.DiffArtifactID.Valid {
		return nil, []githubpr.AnchorWarning{{Message: "Snapshot diff artifact is unavailable; comments will be shown as unanchored."}}
	}
	content, _, err := services.artifacts.Read(ctx, snapshot.DiffArtifactID.String)
	if err != nil {
		return nil, []githubpr.AnchorWarning{{Message: "Snapshot diff artifact could not be read; comments will be shown as unanchored."}}
	}
	files, err := diffparse.Parse(string(content))
	if err != nil {
		return nil, []githubpr.AnchorWarning{{Message: "Snapshot diff artifact could not be parsed; comments will be shown as unanchored."}}
	}
	return files, nil
}

func persistGitHubPreview(ctx context.Context, services routerServices, session dbgen.ReviewSession, reviewEvent string, preview githubpr.ReviewPreview, checklist GitHubPreviewChecklist, commentsJSON string) (dbgen.Artifact, dbgen.PublishDraft, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	draftID := "publish_draft_" + newRequestID()
	payload, err := json.MarshalIndent(map[string]any{
		"review_event": reviewEvent,
		"body":         preview.Body,
		"comments":     preview.Comments,
		"warnings":     preview.Warnings,
		"checklist":    checklist,
	}, "", "  ")
	if err != nil {
		return dbgen.Artifact{}, dbgen.PublishDraft{}, fmt.Errorf("encode preview artifact: %w", err)
	}
	artifactRow, err := services.artifacts.Save(ctx, artifact.SaveParams{
		ID:              "artifact_" + newRequestID(),
		WorkspaceID:     session.WorkspaceID,
		ReviewSessionID: sql.NullString{String: session.ID, Valid: true},
		Kind:            "github_preview",
		RelativePath:    filepath.ToSlash(filepath.Join("github-previews", session.ID, draftID+".json")),
		ContentType:     "application/json",
		MetadataJSON:    `{"provider":"github"}`,
		CreatedAt:       now,
	}, payload)
	if err != nil {
		return dbgen.Artifact{}, dbgen.PublishDraft{}, err
	}
	draft, err := services.queries.CreatePublishDraft(ctx, dbgen.CreatePublishDraftParams{
		ID:              draftID,
		ReviewSessionID: session.ID,
		Provider:        "github",
		Status:          "draft",
		ReviewEvent:     sql.NullString{String: reviewEvent, Valid: true},
		Body:            sql.NullString{String: preview.Body, Valid: true},
		CommentsJson:    commentsJSON,
		ArtifactID:      sql.NullString{String: artifactRow.ID, Valid: true},
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		return dbgen.Artifact{}, dbgen.PublishDraft{}, err
	}
	return artifactRow, draft, nil
}

func githubPreviewChecklist(preview githubpr.ReviewPreview) GitHubPreviewChecklist {
	hasUnanchored := len(preview.Warnings) > 0
	return GitHubPreviewChecklist{
		HasSelectedFindings:   preview.CommentCount > 0,
		HasInlineComments:     preview.CommentCount > 0,
		HasUnanchoredComments: hasUnanchored,
		CanPublishInline:      preview.CommentCount > 0 && !hasUnanchored,
		CanPublishSummaryOnly: strings.TrimSpace(preview.Body) != "",
	}
}

func normalizeReviewEvent(value string) (string, *apperror.Error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "COMMENT":
		return "COMMENT", nil
	case "REQUEST_CHANGES":
		return "REQUEST_CHANGES", nil
	case "APPROVE":
		return "APPROVE", nil
	default:
		return "", apperrorInvalid("review_event is invalid")
	}
}

func previewTitle(reviewEvent string, session dbgen.ReviewSession) string {
	return fmt.Sprintf("cocode GitHub preview (%s): %s", reviewEvent, session.Title)
}

func uniqueRequestIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func apperrorInvalid(message string) *apperror.Error {
	return apperror.InvalidRequest(message)
}

func apperrorNotFound(message string) *apperror.Error {
	return apperror.NotFound(message)
}

func apperrorInternal(message string) *apperror.Error {
	return apperror.Internal(message)
}
