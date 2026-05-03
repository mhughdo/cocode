package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/eventlog"
	evidencepkg "github.com/hughdo/cocode/services/cocoded/internal/evidence"
)

type FindingListResponse struct {
	Items []FindingResponse `json:"items"`
	Stats FindingListStats  `json:"stats"`
}

type FindingListStats struct {
	Total       int            `json:"total"`
	Filtered    int            `json:"filtered"`
	ByDecision  map[string]int `json:"by_decision"`
	BySeverity  map[string]int `json:"by_severity"`
	ByVerify    map[string]int `json:"by_verification"`
	NeedsTriage int            `json:"needs_triage"`
}

type FindingResponse struct {
	ID                     string  `json:"id"`
	ReviewSessionID        string  `json:"review_session_id"`
	CanonicalClaim         string  `json:"canonical_claim"`
	Category               string  `json:"category"`
	Severity               string  `json:"severity"`
	Confidence             float64 `json:"confidence"`
	VerificationStatus     string  `json:"verification_status"`
	DecisionStatus         string  `json:"decision_status"`
	PrimaryPath            string  `json:"primary_path,omitempty"`
	PrimaryStartLine       int64   `json:"primary_start_line,omitempty"`
	PrimaryEndLine         int64   `json:"primary_end_line,omitempty"`
	EvidenceSummary        string  `json:"evidence_summary,omitempty"`
	CounterEvidenceSummary string  `json:"counter_evidence_summary,omitempty"`
	SuggestedFix           string  `json:"suggested_fix,omitempty"`
	DraftComment           string  `json:"draft_comment,omitempty"`
	Fingerprint            string  `json:"fingerprint"`
	MergedFromCount        int64   `json:"merged_from_count"`
	IntroducedInSHA        string  `json:"introduced_in_sha,omitempty"`
	FirstSeenAt            string  `json:"first_seen_at"`
	UpdatedAt              string  `json:"updated_at"`
}

type FindingDetailResponse struct {
	Finding        FindingResponse            `json:"finding"`
	Candidates     []FindingCandidateResponse `json:"candidates"`
	EvidenceItems  []EvidenceItemResponse     `json:"evidence_items"`
	EvidenceGroups EvidenceGroupsResponse     `json:"evidence_groups"`
	Decisions      []HumanDecisionResponse    `json:"decisions"`
}

type FindingEvidenceResponse struct {
	Finding FindingResponse        `json:"finding"`
	Items   []EvidenceItemResponse `json:"items"`
	Groups  EvidenceGroupsResponse `json:"groups"`
	Counts  map[string]int         `json:"counts"`
}

type FindingEvidenceMapResponse = evidencepkg.MapView

type EvidenceGroupsResponse struct {
	Supporting     []EvidenceItemResponse `json:"supporting"`
	Counter        []EvidenceItemResponse `json:"counter"`
	Neutral        []EvidenceItemResponse `json:"neutral"`
	Missing        []EvidenceItemResponse `json:"missing"`
	Test           []EvidenceItemResponse `json:"test"`
	Search         []EvidenceItemResponse `json:"search"`
	Agent          []EvidenceItemResponse `json:"agent"`
	StaticAnalysis []EvidenceItemResponse `json:"static_analysis"`
}

type FindingCandidateResponse struct {
	ID               string          `json:"id"`
	ReviewSessionID  string          `json:"review_session_id"`
	AgentRunID       string          `json:"agent_run_id"`
	RawArtifactID    string          `json:"raw_artifact_id,omitempty"`
	Category         string          `json:"category"`
	Severity         string          `json:"severity"`
	Confidence       float64         `json:"confidence"`
	Claim            string          `json:"claim"`
	PrimaryPath      string          `json:"primary_path,omitempty"`
	PrimaryStartLine int64           `json:"primary_start_line,omitempty"`
	PrimaryEndLine   int64           `json:"primary_end_line,omitempty"`
	Locations        json.RawMessage `json:"locations"`
	Evidence         json.RawMessage `json:"evidence"`
	SuggestedFix     string          `json:"suggested_fix,omitempty"`
	DraftComment     string          `json:"draft_comment,omitempty"`
	Fingerprint      string          `json:"fingerprint,omitempty"`
	CreatedAt        string          `json:"created_at"`
	Relation         string          `json:"relation,omitempty"`
}

type HumanDecisionResponse struct {
	ID              string          `json:"id"`
	FindingID       string          `json:"finding_id"`
	ReviewSessionID string          `json:"review_session_id"`
	Decision        string          `json:"decision"`
	Reason          string          `json:"reason,omitempty"`
	Metadata        json.RawMessage `json:"metadata"`
	CreatedAt       string          `json:"created_at"`
}

type EvidenceItemResponse struct {
	ID          string              `json:"id"`
	FindingID   string              `json:"finding_id"`
	Kind        string              `json:"kind"`
	Title       string              `json:"title"`
	Summary     string              `json:"summary"`
	Path        string              `json:"path,omitempty"`
	StartLine   int64               `json:"start_line,omitempty"`
	EndLine     int64               `json:"end_line,omitempty"`
	ArtifactID  string              `json:"artifact_id,omitempty"`
	Confidence  float64             `json:"confidence"`
	CodeSnippet string              `json:"code_snippet,omitempty"`
	LineWindow  *EvidenceLineWindow `json:"line_window,omitempty"`
	Metadata    json.RawMessage     `json:"metadata"`
	CreatedAt   string              `json:"created_at"`
}

type EvidenceLineWindow struct {
	StartLine int64 `json:"start_line"`
	EndLine   int64 `json:"end_line"`
}

type UpdateFindingDecisionRequest struct {
	Decision             string `json:"decision"`
	Reason               string `json:"reason"`
	RuleMemorySuggestion string `json:"rule_memory_suggestion"`
}

type UpdateDraftCommentRequest struct {
	DraftComment string `json:"draft_comment"`
}

func listFindingsHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, appErr := getReviewSession(c.Request.Context(), queries, c.Param("id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		rows, err := queries.ListFindingsBySession(c.Request.Context(), session.ID)
		if err != nil {
			respondError(c, apperror.Internal("failed to list findings"))
			return
		}
		sortFindings(rows)
		stats := findingListStats(rows)
		filtered := filterFindings(rows, c.Query("status"), c.Query("severity"), c.Query("q"))
		response := make([]FindingResponse, 0, len(filtered))
		for _, row := range filtered {
			response = append(response, findingResponse(row))
		}
		stats.Filtered = len(response)
		respondOK(c, FindingListResponse{Items: response, Stats: stats})
	}
}

func findingDetailHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		detail, appErr := findingDetailResponse(c.Request.Context(), queries, c.Param("id"), c.Param("finding_id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, detail)
	}
}

func findingEvidenceHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		finding, appErr := getFindingScoped(c.Request.Context(), queries, c.Param("id"), c.Param("finding_id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		items, appErr := evidenceItemsForFinding(c.Request.Context(), queries, finding.ID)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, FindingEvidenceResponse{
			Finding: findingResponse(finding),
			Items:   items,
			Groups:  groupEvidenceItems(items),
			Counts:  evidenceCounts(items),
		})
	}
}

func findingEvidenceMapHandler(services routerServices, rebuild bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		finding, appErr := getFindingScoped(c.Request.Context(), services.queries, c.Param("id"), c.Param("finding_id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		builder := evidencepkg.Service{Queries: services.queries}
		var (
			view FindingEvidenceMapResponse
			err  error
		)
		if rebuild {
			view, err = builder.RebuildEvidenceMap(c.Request.Context(), finding)
		} else {
			view, _, err = builder.LoadOrRebuildEvidenceMap(c.Request.Context(), finding)
		}
		if err != nil {
			respondError(c, apperror.Internal("failed to build evidence map"))
			return
		}
		if rebuild {
			if appErr := appendEvidenceMapRebuiltEvent(c.Request.Context(), services, view); appErr != nil {
				respondError(c, appErr)
				return
			}
		}
		respondOK(c, view)
	}
}

func appendEvidenceMapRebuiltEvent(ctx context.Context, services routerServices, view FindingEvidenceMapResponse) *apperror.Error {
	if services.eventBus == nil {
		return apperror.Internal("event bus is not configured")
	}
	payload, err := json.Marshal(map[string]any{
		"finding_id":        view.Finding.ID,
		"evidence_graph_id": view.Graph.ID,
		"status":            view.Graph.Status,
		"nodes":             len(view.Nodes),
		"edges":             len(view.Edges),
	})
	if err != nil {
		return apperror.Internal("failed to encode evidence map event")
	}
	if _, err := services.eventBus.Append(ctx, eventlog.AppendParams{
		ID:              "event_" + newRequestID(),
		ReviewSessionID: view.Finding.ReviewSessionID,
		Type:            "EvidenceMapRebuilt",
		Level:           "info",
		PayloadJSON:     string(payload),
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return apperror.Internal("failed to append evidence map event")
	}
	return nil
}

func updateFindingDecisionHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request UpdateFindingDecisionRequest
		if !bindJSON(c, &request) {
			return
		}
		if services.database == nil {
			respondError(c, apperror.Internal("database is not configured"))
			return
		}
		finding, appErr := getFindingScoped(c.Request.Context(), services.queries, c.Param("id"), c.Param("finding_id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		decision, appErr := normalizeDecision(request)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		tx, err := services.database.BeginTx(c.Request.Context(), nil)
		if err != nil {
			respondError(c, apperror.Internal("failed to begin finding decision update"))
			return
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback()
			}
		}()
		txQueries := services.queries.WithTx(tx)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		updated, err := txQueries.UpdateFindingDecisionStatus(c.Request.Context(), dbgen.UpdateFindingDecisionStatusParams{
			ID:             finding.ID,
			DecisionStatus: decision,
			UpdatedAt:      now,
		})
		if err != nil {
			respondError(c, apperror.Internal("failed to update finding decision"))
			return
		}
		ruleSuggestion := strings.TrimSpace(request.RuleMemorySuggestion)
		ruleID := ""
		if decision == "dismissed" && ruleSuggestion != "" {
			session, appErr := getReviewSession(c.Request.Context(), txQueries, updated.ReviewSessionID)
			if appErr != nil {
				respondError(c, appErr)
				return
			}
			rule, appErr := createOrReuseReviewRule(c.Request.Context(), txQueries, reviewRuleWrite{
				WorkspaceID: session.WorkspaceID,
				Scope:       "workspace",
				RuleType:    "dismissal",
				Content:     ruleSuggestion,
				Enabled:     true,
			})
			if appErr != nil {
				respondError(c, appErr)
				return
			}
			ruleID = rule.ID
		}
		decisionMetadata := map[string]string{
			"rule_memory_suggestion": ruleSuggestion,
		}
		if ruleID != "" {
			decisionMetadata["review_rule_id"] = ruleID
		}
		metadata, err := json.Marshal(decisionMetadata)
		if err != nil {
			respondError(c, apperror.Internal("failed to encode decision metadata"))
			return
		}
		if _, err := txQueries.CreateHumanDecision(c.Request.Context(), dbgen.CreateHumanDecisionParams{
			ID:              "human_decision_" + newRequestID(),
			FindingID:       updated.ID,
			ReviewSessionID: updated.ReviewSessionID,
			Decision:        decision,
			Reason:          nullableSQLString(request.Reason),
			MetadataJson:    string(metadata),
			CreatedAt:       now,
		}); err != nil {
			respondError(c, apperror.Internal("failed to store finding decision"))
			return
		}
		if err := tx.Commit(); err != nil {
			respondError(c, apperror.Internal("failed to commit finding decision"))
			return
		}
		committed = true
		detail, appErr := findingDetailResponse(c.Request.Context(), services.queries, updated.ReviewSessionID, updated.ID)
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		respondOK(c, detail)
	}
}

func updateDraftCommentHandler(services routerServices) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request UpdateDraftCommentRequest
		if !bindJSON(c, &request) {
			return
		}
		if services.database == nil {
			respondError(c, apperror.Internal("database is not configured"))
			return
		}
		finding, appErr := getFindingScoped(c.Request.Context(), services.queries, c.Param("id"), c.Param("finding_id"))
		if appErr != nil {
			respondError(c, appErr)
			return
		}
		tx, err := services.database.BeginTx(c.Request.Context(), nil)
		if err != nil {
			respondError(c, apperror.Internal("failed to begin draft comment update"))
			return
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback()
			}
		}()
		txQueries := services.queries.WithTx(tx)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		updated, err := txQueries.UpdateFindingDraftComment(c.Request.Context(), dbgen.UpdateFindingDraftCommentParams{
			ID:           finding.ID,
			DraftComment: nullableSQLString(request.DraftComment),
			UpdatedAt:    now,
		})
		if err != nil {
			respondError(c, apperror.Internal("failed to update draft comment"))
			return
		}
		if _, err := txQueries.CreateHumanDecision(c.Request.Context(), dbgen.CreateHumanDecisionParams{
			ID:              "human_decision_" + newRequestID(),
			FindingID:       updated.ID,
			ReviewSessionID: updated.ReviewSessionID,
			Decision:        "edited",
			Reason:          nullableSQLString("draft_comment"),
			MetadataJson:    `{"field":"draft_comment"}`,
			CreatedAt:       now,
		}); err != nil {
			respondError(c, apperror.Internal("failed to store draft comment edit"))
			return
		}
		if err := tx.Commit(); err != nil {
			respondError(c, apperror.Internal("failed to commit draft comment update"))
			return
		}
		committed = true
		respondOK(c, findingResponse(updated))
	}
}

func findingDetailResponse(ctx context.Context, queries *dbgen.Queries, reviewSessionID string, findingID string) (FindingDetailResponse, *apperror.Error) {
	finding, appErr := getFindingScoped(ctx, queries, reviewSessionID, findingID)
	if appErr != nil {
		return FindingDetailResponse{}, appErr
	}
	links, err := queries.ListFindingCandidateLinks(ctx, finding.ID)
	if err != nil {
		return FindingDetailResponse{}, apperror.Internal("failed to list finding candidates")
	}
	candidates := make([]FindingCandidateResponse, 0, len(links))
	for _, link := range links {
		candidate, err := queries.GetFindingCandidate(ctx, link.FindingCandidateID)
		if err != nil {
			return FindingDetailResponse{}, apperror.Internal("failed to read finding candidate")
		}
		item, appErr := findingCandidateResponse(candidate, link.Relation)
		if appErr != nil {
			return FindingDetailResponse{}, appErr
		}
		candidates = append(candidates, item)
	}
	decisions, err := queries.ListHumanDecisionsByFinding(ctx, finding.ID)
	if err != nil {
		return FindingDetailResponse{}, apperror.Internal("failed to list finding decisions")
	}
	evidenceResponses, appErr := evidenceItemsForFinding(ctx, queries, finding.ID)
	if appErr != nil {
		return FindingDetailResponse{}, appErr
	}
	decisionResponses := make([]HumanDecisionResponse, 0, len(decisions))
	for _, decision := range decisions {
		item, appErr := humanDecisionResponse(decision)
		if appErr != nil {
			return FindingDetailResponse{}, appErr
		}
		decisionResponses = append(decisionResponses, item)
	}
	return FindingDetailResponse{
		Finding:        findingResponse(finding),
		Candidates:     candidates,
		EvidenceItems:  evidenceResponses,
		EvidenceGroups: groupEvidenceItems(evidenceResponses),
		Decisions:      decisionResponses,
	}, nil
}

func evidenceItemsForFinding(ctx context.Context, queries *dbgen.Queries, findingID string) ([]EvidenceItemResponse, *apperror.Error) {
	evidenceItems, err := queries.ListEvidenceItemsByFinding(ctx, findingID)
	if err != nil {
		return nil, apperror.Internal("failed to list finding evidence")
	}
	evidenceResponses := make([]EvidenceItemResponse, 0, len(evidenceItems))
	for _, item := range evidenceItems {
		response, appErr := evidenceItemResponse(item)
		if appErr != nil {
			return nil, appErr
		}
		evidenceResponses = append(evidenceResponses, response)
	}
	return evidenceResponses, nil
}

func groupEvidenceItems(items []EvidenceItemResponse) EvidenceGroupsResponse {
	groups := EvidenceGroupsResponse{}
	for _, item := range items {
		switch item.Kind {
		case "supporting":
			groups.Supporting = append(groups.Supporting, item)
		case "counter":
			groups.Counter = append(groups.Counter, item)
		case "missing":
			groups.Missing = append(groups.Missing, item)
		case "test":
			groups.Test = append(groups.Test, item)
		case "search":
			groups.Search = append(groups.Search, item)
		case "agent":
			groups.Agent = append(groups.Agent, item)
		case "static_analysis":
			groups.StaticAnalysis = append(groups.StaticAnalysis, item)
		default:
			groups.Neutral = append(groups.Neutral, item)
		}
	}
	return groups
}

func evidenceCounts(items []EvidenceItemResponse) map[string]int {
	counts := map[string]int{
		"supporting":      0,
		"counter":         0,
		"neutral":         0,
		"missing":         0,
		"test":            0,
		"search":          0,
		"agent":           0,
		"static_analysis": 0,
	}
	for _, item := range items {
		if _, ok := counts[item.Kind]; ok {
			counts[item.Kind]++
			continue
		}
		counts["neutral"]++
	}
	return counts
}

func getFindingScoped(ctx context.Context, queries *dbgen.Queries, reviewSessionID string, findingID string) (dbgen.Finding, *apperror.Error) {
	var session dbgen.ReviewSession
	if strings.TrimSpace(reviewSessionID) != "" {
		var appErr *apperror.Error
		session, appErr = getReviewSession(ctx, queries, reviewSessionID)
		if appErr != nil {
			return dbgen.Finding{}, appErr
		}
	}
	findingID = strings.TrimSpace(findingID)
	if findingID == "" {
		return dbgen.Finding{}, apperror.InvalidRequest("finding id is required")
	}
	finding, err := queries.GetFinding(ctx, findingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.Finding{}, apperror.NotFound("finding was not found")
		}
		return dbgen.Finding{}, apperror.Internal("failed to read finding")
	}
	if session.ID != "" && finding.ReviewSessionID != session.ID {
		return dbgen.Finding{}, apperror.NotFound("finding was not found")
	}
	return finding, nil
}

func normalizeDecision(request UpdateFindingDecisionRequest) (string, *apperror.Error) {
	decision := strings.ToLower(strings.TrimSpace(request.Decision))
	switch decision {
	case "accept", "accepted":
		return "accepted", nil
	case "dismiss", "dismissed":
		if strings.TrimSpace(request.Reason) == "" {
			return "", apperror.InvalidRequest("reason is required when dismissing a finding")
		}
		return "dismissed", nil
	case "defer", "deferred":
		return "deferred", nil
	case "copied":
		return "copied", nil
	case "publish", "published":
		return "published", nil
	default:
		return "", apperror.InvalidRequest("decision is invalid")
	}
}

func findingResponse(row dbgen.Finding) FindingResponse {
	return FindingResponse{
		ID:                     row.ID,
		ReviewSessionID:        row.ReviewSessionID,
		CanonicalClaim:         row.CanonicalClaim,
		Category:               row.Category,
		Severity:               row.Severity,
		Confidence:             row.Confidence,
		VerificationStatus:     row.VerificationStatus,
		DecisionStatus:         row.DecisionStatus,
		PrimaryPath:            nullableValue(row.PrimaryPath),
		PrimaryStartLine:       nullableInt64Value(row.PrimaryStartLine),
		PrimaryEndLine:         nullableInt64Value(row.PrimaryEndLine),
		EvidenceSummary:        nullableValue(row.EvidenceSummary),
		CounterEvidenceSummary: nullableValue(row.CounterEvidenceSummary),
		SuggestedFix:           nullableValue(row.SuggestedFix),
		DraftComment:           nullableValue(row.DraftComment),
		Fingerprint:            row.Fingerprint,
		MergedFromCount:        row.MergedFromCount,
		IntroducedInSHA:        nullableValue(row.IntroducedInSha),
		FirstSeenAt:            row.FirstSeenAt,
		UpdatedAt:              row.UpdatedAt,
	}
}

func findingCandidateResponse(row dbgen.FindingCandidate, relation string) (FindingCandidateResponse, *apperror.Error) {
	locations := json.RawMessage(row.LocationsJson)
	evidence := json.RawMessage(row.EvidenceJson)
	if !json.Valid(locations) || !json.Valid(evidence) {
		return FindingCandidateResponse{}, apperror.Internal("stored finding candidate JSON is invalid")
	}
	return FindingCandidateResponse{
		ID:               row.ID,
		ReviewSessionID:  row.ReviewSessionID,
		AgentRunID:       row.AgentRunID,
		RawArtifactID:    nullableValue(row.RawArtifactID),
		Category:         row.Category,
		Severity:         row.Severity,
		Confidence:       row.Confidence,
		Claim:            row.Claim,
		PrimaryPath:      nullableValue(row.PrimaryPath),
		PrimaryStartLine: nullableInt64Value(row.PrimaryStartLine),
		PrimaryEndLine:   nullableInt64Value(row.PrimaryEndLine),
		Locations:        locations,
		Evidence:         evidence,
		SuggestedFix:     nullableValue(row.SuggestedFix),
		DraftComment:     nullableValue(row.DraftComment),
		Fingerprint:      nullableValue(row.Fingerprint),
		CreatedAt:        row.CreatedAt,
		Relation:         relation,
	}, nil
}

func humanDecisionResponse(row dbgen.HumanDecision) (HumanDecisionResponse, *apperror.Error) {
	metadata := json.RawMessage(row.MetadataJson)
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	if !json.Valid(metadata) {
		return HumanDecisionResponse{}, apperror.Internal("stored human decision metadata is invalid")
	}
	return HumanDecisionResponse{
		ID:              row.ID,
		FindingID:       row.FindingID,
		ReviewSessionID: row.ReviewSessionID,
		Decision:        row.Decision,
		Reason:          nullableValue(row.Reason),
		Metadata:        metadata,
		CreatedAt:       row.CreatedAt,
	}, nil
}

func evidenceItemResponse(row dbgen.EvidenceItem) (EvidenceItemResponse, *apperror.Error) {
	metadata := json.RawMessage(row.MetadataJson)
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	if !json.Valid(metadata) {
		return EvidenceItemResponse{}, apperror.Internal("stored evidence metadata is invalid")
	}
	snippet, window := evidenceSnippetFields(metadata)
	return EvidenceItemResponse{
		ID:          row.ID,
		FindingID:   row.FindingID,
		Kind:        row.Kind,
		Title:       row.Title,
		Summary:     row.Summary,
		Path:        nullableValue(row.Path),
		StartLine:   nullableInt64Value(row.StartLine),
		EndLine:     nullableInt64Value(row.EndLine),
		ArtifactID:  nullableValue(row.ArtifactID),
		Confidence:  row.Confidence,
		CodeSnippet: snippet,
		LineWindow:  window,
		Metadata:    metadata,
		CreatedAt:   row.CreatedAt,
	}, nil
}

func evidenceSnippetFields(metadata json.RawMessage) (string, *EvidenceLineWindow) {
	var payload struct {
		CodeSnippet string `json:"code_snippet"`
		LineWindow  struct {
			StartLine int64 `json:"start_line"`
			EndLine   int64 `json:"end_line"`
		} `json:"line_window"`
	}
	if err := json.Unmarshal(metadata, &payload); err != nil {
		return "", nil
	}
	var window *EvidenceLineWindow
	if payload.LineWindow.StartLine > 0 && payload.LineWindow.EndLine >= payload.LineWindow.StartLine {
		window = &EvidenceLineWindow{
			StartLine: payload.LineWindow.StartLine,
			EndLine:   payload.LineWindow.EndLine,
		}
	}
	return strings.TrimSpace(payload.CodeSnippet), window
}

func sortFindings(rows []dbgen.Finding) {
	sort.SliceStable(rows, func(i, j int) bool {
		left, right := rows[i], rows[j]
		if severityRank(left.Severity) != severityRank(right.Severity) {
			return severityRank(left.Severity) > severityRank(right.Severity)
		}
		if verificationRank(left.VerificationStatus) != verificationRank(right.VerificationStatus) {
			return verificationRank(left.VerificationStatus) > verificationRank(right.VerificationStatus)
		}
		if left.Confidence != right.Confidence {
			return left.Confidence > right.Confidence
		}
		if left.MergedFromCount != right.MergedFromCount {
			return left.MergedFromCount > right.MergedFromCount
		}
		if left.UpdatedAt != right.UpdatedAt {
			return left.UpdatedAt > right.UpdatedAt
		}
		return left.ID < right.ID
	})
}

func filterFindings(rows []dbgen.Finding, status string, severity string, query string) []dbgen.Finding {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		status = "all"
	}
	severity = strings.ToLower(strings.TrimSpace(severity))
	query = strings.ToLower(strings.TrimSpace(query))
	filtered := make([]dbgen.Finding, 0, len(rows))
	for _, row := range rows {
		if severity != "" && row.Severity != severity {
			continue
		}
		if !findingMatchesStatus(row, status) {
			continue
		}
		if query != "" && !findingMatchesQuery(row, query) {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func findingMatchesStatus(row dbgen.Finding, status string) bool {
	switch status {
	case "all":
		return true
	case "verified":
		return row.VerificationStatus == "verified"
	case "needs_triage":
		return row.DecisionStatus == "undecided"
	default:
		return row.DecisionStatus == status
	}
}

func findingMatchesQuery(row dbgen.Finding, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		row.CanonicalClaim,
		row.Category,
		row.Severity,
		nullableValue(row.PrimaryPath),
		nullableValue(row.EvidenceSummary),
	}, " "))
	return strings.Contains(haystack, query)
}

func findingListStats(rows []dbgen.Finding) FindingListStats {
	stats := FindingListStats{
		Total:      len(rows),
		ByDecision: map[string]int{},
		BySeverity: map[string]int{},
		ByVerify:   map[string]int{},
	}
	for _, row := range rows {
		stats.ByDecision[row.DecisionStatus]++
		stats.BySeverity[row.Severity]++
		stats.ByVerify[row.VerificationStatus]++
		if row.DecisionStatus == "undecided" {
			stats.NeedsTriage++
		}
	}
	return stats
}

func severityRank(severity string) int {
	switch severity {
	case "blocker":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	case "nit":
		return 1
	default:
		return 0
	}
}

func verificationRank(status string) int {
	switch status {
	case "verified":
		return 5
	case "plausible":
		return 4
	case "needs_human":
		return 3
	case "unverified":
		return 2
	case "likely_false_positive", "duplicate", "not_actionable":
		return 1
	default:
		return 0
	}
}

func nullableInt64Value(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func nullableValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
