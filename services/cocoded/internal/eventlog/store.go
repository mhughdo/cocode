package eventlog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const defaultLevel = "info"

type Store struct {
	database *sql.DB
	queries  *dbgen.Queries
}

type AppendParams struct {
	ID              string
	ReviewSessionID string
	AgentRunID      sql.NullString
	Type            string
	Level           string
	PayloadJSON     string
	ArtifactID      sql.NullString
	CreatedAt       string
}

func New(database *sql.DB) (*Store, error) {
	if database == nil {
		return nil, errors.New("event database is required")
	}
	return &Store{
		database: database,
		queries:  dbgen.New(database),
	}, nil
}

func (s *Store) Append(ctx context.Context, params AppendParams) (dbgen.Event, error) {
	if s == nil {
		return dbgen.Event{}, errors.New("event store is required")
	}
	if params.ID == "" {
		return dbgen.Event{}, errors.New("event id is required")
	}
	if params.ReviewSessionID == "" {
		return dbgen.Event{}, errors.New("review session id is required")
	}
	if params.Type == "" {
		return dbgen.Event{}, errors.New("event type is required")
	}
	if params.Level == "" {
		params.Level = defaultLevel
	}
	if params.PayloadJSON == "" {
		params.PayloadJSON = "{}"
	}
	if params.CreatedAt == "" {
		params.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return dbgen.Event{}, fmt.Errorf("begin event append tx: %w", err)
	}
	defer tx.Rollback()

	queries := s.queries.WithTx(tx)
	reviewSessionID := sql.NullString{String: params.ReviewSessionID, Valid: true}
	sequence, err := queries.NextEventSequence(ctx, reviewSessionID)
	if err != nil {
		return dbgen.Event{}, fmt.Errorf("next event sequence: %w", err)
	}

	event, err := queries.CreateEvent(ctx, dbgen.CreateEventParams{
		ID:              params.ID,
		ReviewSessionID: reviewSessionID,
		AgentRunID:      params.AgentRunID,
		Type:            params.Type,
		Level:           params.Level,
		Sequence:        sequence,
		PayloadJson:     params.PayloadJSON,
		ArtifactID:      params.ArtifactID,
		CreatedAt:       params.CreatedAt,
	})
	if err != nil {
		return dbgen.Event{}, fmt.Errorf("create event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return dbgen.Event{}, fmt.Errorf("commit event append: %w", err)
	}

	return event, nil
}

func (s *Store) ListByReviewSession(ctx context.Context, reviewSessionID string) ([]dbgen.Event, error) {
	if s == nil {
		return nil, errors.New("event store is required")
	}
	if reviewSessionID == "" {
		return nil, errors.New("review session id is required")
	}
	events, err := s.queries.ListEventsByReviewSession(ctx, sql.NullString{String: reviewSessionID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	return events, nil
}
