package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

type ReviewRuleListResponse struct {
	Items []ReviewRuleResponse `json:"items"`
}

type ReviewRuleResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Scope       string `json:"scope"`
	RuleType    string `json:"rule_type"`
	Content     string `json:"content"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type CreateReviewRuleRequest struct {
	Scope    string `json:"scope"`
	RuleType string `json:"rule_type"`
	Content  string `json:"content"`
	Enabled  *bool  `json:"enabled"`
}

type UpdateReviewRuleRequest struct {
	Scope    string `json:"scope"`
	RuleType string `json:"rule_type"`
	Content  string `json:"content"`
	Enabled  *bool  `json:"enabled"`
}

type SetReviewRuleEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

type reviewRuleWrite struct {
	WorkspaceID string
	Scope       string
	RuleType    string
	Content     string
	Enabled     bool
}

func listReviewRulesHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		workspace, appErr := getWorkspace(c.Request.Context(), queries, c.Param("id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		rows, err := queries.ListReviewRulesByWorkspace(c.Request.Context(), workspace.ID)
		if err != nil {
			respondError(c, apperror.Internal("failed to list review rules"))
			return
		}
		items := make([]ReviewRuleResponse, 0, len(rows))
		for _, row := range rows {
			items = append(items, reviewRuleResponse(row))
		}
		respondOK(c, ReviewRuleListResponse{Items: items})
	}
}

func createReviewRuleHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request CreateReviewRuleRequest
		if !bindJSON(c, &request) {
			return
		}
		workspace, appErr := getWorkspace(c.Request.Context(), queries, c.Param("id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		enabled := true
		if request.Enabled != nil {
			enabled = *request.Enabled
		}
		write, appErr := normalizeReviewRuleWrite(reviewRuleWrite{
			WorkspaceID: workspace.ID,
			Scope:       request.Scope,
			RuleType:    request.RuleType,
			Content:     request.Content,
			Enabled:     enabled,
		})
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		row, appErr := createOrReuseReviewRule(c.Request.Context(), queries, write)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, reviewRuleResponse(row))
	}
}

func updateReviewRuleHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request UpdateReviewRuleRequest
		if !bindJSON(c, &request) {
			return
		}
		existing, appErr := getReviewRule(c.Request.Context(), queries, c.Param("id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		enabled := existing.Enabled == 1
		if request.Enabled != nil {
			enabled = *request.Enabled
		}
		write, appErr := normalizeReviewRuleWrite(reviewRuleWrite{
			WorkspaceID: existing.WorkspaceID,
			Scope:       firstNonEmpty(request.Scope, existing.Scope),
			RuleType:    firstNonEmpty(request.RuleType, existing.RuleType),
			Content:     firstNonEmpty(request.Content, existing.Content),
			Enabled:     enabled,
		})
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		row, err := queries.UpdateReviewRule(c.Request.Context(), dbgen.UpdateReviewRuleParams{
			ID:        existing.ID,
			Scope:     write.Scope,
			RuleType:  write.RuleType,
			Content:   write.Content,
			Enabled:   boolInt64(write.Enabled),
			UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			respondError(c, apperror.Internal("failed to update review rule"))
			return
		}
		respondOK(c, reviewRuleResponse(row))
	}
}

func setReviewRuleEnabledHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request SetReviewRuleEnabledRequest
		if !bindJSON(c, &request) {
			return
		}
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			respondError(c, apperror.InvalidRequest("review rule id is required"))
			return
		}
		row, err := queries.SetReviewRuleEnabled(c.Request.Context(), dbgen.SetReviewRuleEnabledParams{
			ID:        id,
			Enabled:   boolInt64(request.Enabled),
			UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondError(c, apperror.NotFound("review rule was not found"))
				return
			}
			respondError(c, apperror.Internal("failed to update review rule"))
			return
		}
		respondOK(c, reviewRuleResponse(row))
	}
}

func deleteReviewRuleHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		rule, appErr := getReviewRule(c.Request.Context(), queries, c.Param("id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		if err := queries.DeleteReviewRule(c.Request.Context(), rule.ID); err != nil {
			respondError(c, apperror.Internal("failed to delete review rule"))
			return
		}
		respondOK(c, gin.H{"deleted": true, "id": rule.ID})
	}
}

func getReviewRule(ctx context.Context, queries *dbgen.Queries, id string) (dbgen.ReviewRule, *apperror.Error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return dbgen.ReviewRule{}, apperror.InvalidRequest("review rule id is required")
	}
	row, err := queries.GetReviewRule(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.ReviewRule{}, apperror.NotFound("review rule was not found")
		}
		return dbgen.ReviewRule{}, apperror.Internal("failed to read review rule")
	}
	return row, nil
}

func createOrReuseReviewRule(ctx context.Context, queries *dbgen.Queries, write reviewRuleWrite) (dbgen.ReviewRule, *apperror.Error) {
	normalized, appErr := normalizeReviewRuleWrite(write)
	if appErr != nil {
		return dbgen.ReviewRule{}, appErr
	}
	existing, err := queries.ListReviewRulesByWorkspace(ctx, normalized.WorkspaceID)
	if err != nil {
		return dbgen.ReviewRule{}, apperror.Internal("failed to list review rules")
	}
	for _, row := range existing {
		if row.Scope != normalized.Scope || row.RuleType != normalized.RuleType {
			continue
		}
		if normalizedRuleContent(row.Content) != normalizedRuleContent(normalized.Content) {
			continue
		}
		if normalized.Enabled && row.Enabled == 0 {
			updated, err := queries.SetReviewRuleEnabled(ctx, dbgen.SetReviewRuleEnabledParams{
				ID:        row.ID,
				Enabled:   1,
				UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			})
			if err != nil {
				return dbgen.ReviewRule{}, apperror.Internal("failed to enable review rule")
			}
			return updated, nil
		}
		return row, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	row, err := queries.CreateReviewRule(ctx, dbgen.CreateReviewRuleParams{
		ID:          "review_rule_" + newRequestID(),
		WorkspaceID: normalized.WorkspaceID,
		Scope:       normalized.Scope,
		RuleType:    normalized.RuleType,
		Content:     normalized.Content,
		Enabled:     boolInt64(normalized.Enabled),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return dbgen.ReviewRule{}, apperror.Internal("failed to create review rule")
	}
	return row, nil
}

func normalizeReviewRuleWrite(write reviewRuleWrite) (reviewRuleWrite, *apperror.Error) {
	workspaceID := strings.TrimSpace(write.WorkspaceID)
	if workspaceID == "" {
		return reviewRuleWrite{}, apperror.InvalidRequest("workspace id is required")
	}
	scope := strings.ToLower(strings.TrimSpace(write.Scope))
	if scope == "" {
		scope = "workspace"
	}
	switch scope {
	case "workspace", "repository", "path":
	default:
		return reviewRuleWrite{}, apperror.InvalidRequest("review rule scope is invalid")
	}
	ruleType := strings.ToLower(strings.TrimSpace(write.RuleType))
	if ruleType == "" {
		ruleType = "dismissal"
	}
	switch ruleType {
	case "dismissal", "false_positive", "review_guidance", "custom":
	default:
		return reviewRuleWrite{}, apperror.InvalidRequest("review rule type is invalid")
	}
	content := strings.TrimSpace(write.Content)
	if content == "" {
		return reviewRuleWrite{}, apperror.InvalidRequest("review rule content is required")
	}
	if len(content) > 2000 {
		return reviewRuleWrite{}, apperror.InvalidRequest("review rule content is too long")
	}
	return reviewRuleWrite{
		WorkspaceID: workspaceID,
		Scope:       scope,
		RuleType:    ruleType,
		Content:     content,
		Enabled:     write.Enabled,
	}, nil
}

func reviewRuleResponse(row dbgen.ReviewRule) ReviewRuleResponse {
	return ReviewRuleResponse{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		Scope:       row.Scope,
		RuleType:    row.RuleType,
		Content:     row.Content,
		Enabled:     row.Enabled == 1,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func normalizedRuleContent(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func firstNonEmpty(left string, right string) string {
	if strings.TrimSpace(left) != "" {
		return left
	}
	return right
}
