package githubpr

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

var ErrPublicationInvalid = errors.New("GitHub publication record is invalid")

type PublicationService struct {
	Database *sql.DB
	Queries  *dbgen.Queries
	Now      func() time.Time
	NewID    func(prefix string) string
}

type RecordPublicationParams struct {
	PublishDraftID     string
	GitHubReviewID     string
	GitHubCommentIDs   []int64
	Status             string
	ErrorMessage       string
	UpdateFindingState bool
}

type RecordPublicationResult struct {
	Publication dbgen.GithubPublication
	Decisions   []dbgen.HumanDecision
	FindingIDs  []string
}

func (s PublicationService) RecordPublication(ctx context.Context, params RecordPublicationParams) (RecordPublicationResult, error) {
	if s.Database == nil || s.Queries == nil {
		return RecordPublicationResult{}, fmt.Errorf("%w: database is required", ErrPublicationInvalid)
	}
	draftID := strings.TrimSpace(params.PublishDraftID)
	if draftID == "" {
		return RecordPublicationResult{}, fmt.Errorf("%w: publish draft id is required", ErrPublicationInvalid)
	}
	status := normalizePublicationStatus(params.Status)
	draft, err := s.Queries.GetPublishDraft(ctx, draftID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RecordPublicationResult{}, ErrPublicationInvalid
		}
		return RecordPublicationResult{}, fmt.Errorf("read publish draft: %w", err)
	}
	findingIDs, err := findingIDsFromComments(draft.CommentsJson)
	if err != nil {
		return RecordPublicationResult{}, err
	}
	commentIDsJSON, err := json.Marshal(params.GitHubCommentIDs)
	if err != nil {
		return RecordPublicationResult{}, fmt.Errorf("encode GitHub comment IDs: %w", err)
	}
	tx, err := s.Database.BeginTx(ctx, nil)
	if err != nil {
		return RecordPublicationResult{}, fmt.Errorf("begin publication record: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	txQueries := s.Queries.WithTx(tx)
	now := s.now().Format(time.RFC3339Nano)
	publication, err := txQueries.CreateGitHubPublication(ctx, dbgen.CreateGitHubPublicationParams{
		ID:                   s.newID("github_publication_"),
		ReviewSessionID:      draft.ReviewSessionID,
		PublishDraftID:       sql.NullString{String: draft.ID, Valid: true},
		GithubReviewID:       nullableString(params.GitHubReviewID),
		GithubCommentIdsJson: string(commentIDsJSON),
		Status:               status,
		ErrorMessage:         nullableString(params.ErrorMessage),
		CreatedAt:            now,
	})
	if err != nil {
		return RecordPublicationResult{}, fmt.Errorf("create GitHub publication: %w", err)
	}
	decisions := []dbgen.HumanDecision{}
	if status == "published" && params.UpdateFindingState {
		decisions, err = s.markFindingsPublished(ctx, txQueries, draft.ReviewSessionID, findingIDs, publication, now)
		if err != nil {
			return RecordPublicationResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return RecordPublicationResult{}, fmt.Errorf("commit publication record: %w", err)
	}
	committed = true
	return RecordPublicationResult{Publication: publication, Decisions: decisions, FindingIDs: findingIDs}, nil
}

func (s PublicationService) markFindingsPublished(ctx context.Context, queries *dbgen.Queries, reviewSessionID string, findingIDs []string, publication dbgen.GithubPublication, now string) ([]dbgen.HumanDecision, error) {
	decisions := make([]dbgen.HumanDecision, 0, len(findingIDs))
	metadata, err := publishedDecisionMetadata(publication)
	if err != nil {
		return nil, err
	}
	for _, findingID := range findingIDs {
		finding, err := queries.GetFinding(ctx, findingID)
		if err != nil {
			return nil, fmt.Errorf("read published finding: %w", err)
		}
		if finding.ReviewSessionID != reviewSessionID {
			return nil, fmt.Errorf("%w: finding %s is outside publish draft session", ErrPublicationInvalid, findingID)
		}
		if _, err := queries.UpdateFindingDecisionStatus(ctx, dbgen.UpdateFindingDecisionStatusParams{
			ID:             finding.ID,
			DecisionStatus: "published",
			UpdatedAt:      now,
		}); err != nil {
			return nil, fmt.Errorf("update published finding: %w", err)
		}
		decision, err := queries.CreateHumanDecision(ctx, dbgen.CreateHumanDecisionParams{
			ID:              s.newID("human_decision_"),
			FindingID:       finding.ID,
			ReviewSessionID: reviewSessionID,
			Decision:        "published",
			Reason:          sql.NullString{String: "github_review", Valid: true},
			MetadataJson:    string(metadata),
			CreatedAt:       now,
		})
		if err != nil {
			return nil, fmt.Errorf("store published decision: %w", err)
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

func findingIDsFromComments(commentsJSON string) ([]string, error) {
	var comments []ReviewCommentDraft
	if err := json.Unmarshal([]byte(commentsJSON), &comments); err != nil {
		return nil, fmt.Errorf("%w: publish draft comments are invalid", ErrPublicationInvalid)
	}
	seen := make(map[string]struct{}, len(comments))
	ids := make([]string, 0, len(comments))
	for _, comment := range comments {
		id := strings.TrimSpace(comment.FindingID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: publish draft has no finding comments", ErrPublicationInvalid)
	}
	return ids, nil
}

func publishedDecisionMetadata(publication dbgen.GithubPublication) (json.RawMessage, error) {
	metadata, err := json.Marshal(map[string]any{
		"source":                "github_publication",
		"github_publication_id": publication.ID,
		"github_review_id":      nullableStringValue(publication.GithubReviewID),
	})
	if err != nil {
		return nil, fmt.Errorf("encode published decision metadata: %w", err)
	}
	return metadata, nil
}

func normalizePublicationStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "published", "succeeded", "success":
		return "published"
	case "failed", "error":
		return "failed"
	default:
		return "pending"
	}
}

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullableStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func (s PublicationService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s PublicationService) newID(prefix string) string {
	if s.NewID != nil {
		return s.NewID(prefix)
	}
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + fmt.Sprint(time.Now().UTC().UnixNano())
	}
	return prefix + hex.EncodeToString(bytes[:])
}
