package search

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const defaultLimit int64 = 25

type Store struct {
	database *sql.DB
	queries  *dbgen.Queries
}

func New(database *sql.DB) (*Store, error) {
	if database == nil {
		return nil, errors.New("search database is required")
	}
	return &Store{
		database: database,
		queries:  dbgen.New(database),
	}, nil
}

func (s *Store) SyncFinding(ctx context.Context, finding dbgen.Finding) error {
	if s == nil {
		return errors.New("search store is required")
	}
	if finding.ID == "" {
		return errors.New("finding id is required")
	}

	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin finding search sync tx: %w", err)
	}
	defer tx.Rollback()

	queries := s.queries.WithTx(tx)
	if err := queries.DeleteFindingSearch(ctx, finding.ID); err != nil {
		return fmt.Errorf("delete finding search row: %w", err)
	}
	if err := queries.InsertFindingSearch(ctx, dbgen.InsertFindingSearchParams{
		FindingID:       finding.ID,
		Claim:           finding.CanonicalClaim,
		EvidenceSummary: nullableStringValue(finding.EvidenceSummary),
		SuggestedFix:    nullableStringValue(finding.SuggestedFix),
		DraftComment:    nullableStringValue(finding.DraftComment),
	}); err != nil {
		return fmt.Errorf("insert finding search row: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit finding search sync: %w", err)
	}
	return nil
}

func (s *Store) DeleteFinding(ctx context.Context, findingID string) error {
	if s == nil {
		return errors.New("search store is required")
	}
	if findingID == "" {
		return errors.New("finding id is required")
	}
	if err := s.queries.DeleteFindingSearch(ctx, findingID); err != nil {
		return fmt.Errorf("delete finding search row: %w", err)
	}
	return nil
}

func (s *Store) SearchFindings(ctx context.Context, text string, limit int64) ([]string, error) {
	query := buildFTSQuery(text)
	if query == "" {
		return []string{}, nil
	}
	if limit <= 0 {
		limit = defaultLimit
	}

	ids, err := s.queries.SearchFindings(ctx, dbgen.SearchFindingsParams{
		Claim:           query,
		EvidenceSummary: query,
		SuggestedFix:    query,
		DraftComment:    query,
		Limit:           limit,
	})
	if err != nil {
		return nil, fmt.Errorf("search findings: %w", err)
	}
	return ids, nil
}

func (s *Store) SyncEvidenceItem(ctx context.Context, item dbgen.EvidenceItem) error {
	if s == nil {
		return errors.New("search store is required")
	}
	if item.ID == "" {
		return errors.New("evidence item id is required")
	}

	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin evidence search sync tx: %w", err)
	}
	defer tx.Rollback()

	queries := s.queries.WithTx(tx)
	if err := queries.DeleteEvidenceSearch(ctx, item.ID); err != nil {
		return fmt.Errorf("delete evidence search row: %w", err)
	}
	if err := queries.InsertEvidenceSearch(ctx, dbgen.InsertEvidenceSearchParams{
		EvidenceItemID: item.ID,
		Title:          item.Title,
		Summary:        item.Summary,
		Path:           nullableStringValue(item.Path),
	}); err != nil {
		return fmt.Errorf("insert evidence search row: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit evidence search sync: %w", err)
	}
	return nil
}

func (s *Store) DeleteEvidenceItem(ctx context.Context, evidenceItemID string) error {
	if s == nil {
		return errors.New("search store is required")
	}
	if evidenceItemID == "" {
		return errors.New("evidence item id is required")
	}
	if err := s.queries.DeleteEvidenceSearch(ctx, evidenceItemID); err != nil {
		return fmt.Errorf("delete evidence search row: %w", err)
	}
	return nil
}

func (s *Store) SearchEvidence(ctx context.Context, text string, limit int64) ([]string, error) {
	query := buildFTSQuery(text)
	if query == "" {
		return []string{}, nil
	}
	if limit <= 0 {
		limit = defaultLimit
	}

	ids, err := s.queries.SearchEvidence(ctx, dbgen.SearchEvidenceParams{
		Title:   query,
		Summary: query,
		Path:    query,
		Limit:   limit,
	})
	if err != nil {
		return nil, fmt.Errorf("search evidence: %w", err)
	}
	return ids, nil
}

func buildFTSQuery(text string) string {
	terms := strings.Fields(text)
	phrases := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		term = strings.ReplaceAll(term, `"`, `""`)
		phrases = append(phrases, `"`+term+`"`)
	}
	return strings.Join(phrases, " ")
}

func nullableStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
