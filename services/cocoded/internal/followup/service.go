package followup

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
	"unicode/utf8"

	"github.com/hughdo/cocode/services/cocoded/internal/agentrun"
	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/contextbundle"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const (
	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
	MessageRoleSystem    = "system"
	MessageRoleAgent     = "agent"

	defaultThreadTitleBytes = 96
)

var (
	ErrServiceNotConfigured = errors.New("follow-up service is not configured")
	ErrFindingNotFound      = errors.New("finding was not found")
	ErrThreadNotFound       = errors.New("finding thread was not found")
	ErrInvalidMessage       = errors.New("finding thread message is invalid")
	ErrAgentConfigNotFound  = errors.New("follow-up agent config was not found")
	ErrInvalidAgentConfig   = errors.New("follow-up agent config is invalid")
	ErrAgentRunFailed       = errors.New("follow-up agent run failed")
	ErrInvalidQuickAction   = errors.New("follow-up quick action is invalid")
)

type Service struct {
	Database       *sql.DB
	Queries        *dbgen.Queries
	ContextBuilder *contextbundle.Service
	Artifacts      *artifact.Store
	AgentManager   *agentrun.Manager
	Now            func() time.Time
	NewID          func(prefix string) string
}

type EnsureThreadParams struct {
	FindingID       string
	ReviewSessionID string
}

type AppendMessageParams struct {
	ThreadID         string
	Role             string
	AgentConfigID    string
	Content          string
	EvidenceRefsJSON json.RawMessage
	ArtifactID       string
}

type ThreadView struct {
	Finding  dbgen.Finding
	Thread   dbgen.FindingThread
	Messages []dbgen.FindingThreadMessage
}

func (s Service) EnsureThread(ctx context.Context, params EnsureThreadParams) (ThreadView, error) {
	if s.Queries == nil {
		return ThreadView{}, ErrServiceNotConfigured
	}
	finding, err := s.findingScoped(ctx, params.FindingID, params.ReviewSessionID)
	if err != nil {
		return ThreadView{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	thread, err := s.Queries.UpsertFindingThreadByFinding(ctx, dbgen.UpsertFindingThreadByFindingParams{
		ID:              s.newID("finding_thread_"),
		FindingID:       finding.ID,
		ReviewSessionID: finding.ReviewSessionID,
		Title:           defaultThreadTitle(finding),
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		return ThreadView{}, fmt.Errorf("upsert finding thread: %w", err)
	}
	messages, err := s.Queries.ListFindingThreadMessages(ctx, thread.ID)
	if err != nil {
		return ThreadView{}, fmt.Errorf("list finding thread messages: %w", err)
	}
	return ThreadView{Finding: finding, Thread: thread, Messages: messages}, nil
}

func (s Service) LoadThread(ctx context.Context, threadID string) (ThreadView, error) {
	if s.Queries == nil {
		return ThreadView{}, ErrServiceNotConfigured
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ThreadView{}, fmt.Errorf("%w: thread id is required", ErrThreadNotFound)
	}
	thread, err := s.Queries.GetFindingThread(ctx, threadID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ThreadView{}, ErrThreadNotFound
		}
		return ThreadView{}, fmt.Errorf("read finding thread: %w", err)
	}
	finding, err := s.Queries.GetFinding(ctx, thread.FindingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ThreadView{}, ErrFindingNotFound
		}
		return ThreadView{}, fmt.Errorf("read finding: %w", err)
	}
	messages, err := s.Queries.ListFindingThreadMessages(ctx, thread.ID)
	if err != nil {
		return ThreadView{}, fmt.Errorf("list finding thread messages: %w", err)
	}
	return ThreadView{Finding: finding, Thread: thread, Messages: messages}, nil
}

func (s Service) AppendMessage(ctx context.Context, params AppendMessageParams) (dbgen.FindingThreadMessage, error) {
	if s.Queries == nil {
		return dbgen.FindingThreadMessage{}, ErrServiceNotConfigured
	}
	threadID := strings.TrimSpace(params.ThreadID)
	if threadID == "" {
		return dbgen.FindingThreadMessage{}, fmt.Errorf("%w: thread id is required", ErrInvalidMessage)
	}
	thread, err := s.Queries.GetFindingThread(ctx, threadID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.FindingThreadMessage{}, ErrThreadNotFound
		}
		return dbgen.FindingThreadMessage{}, fmt.Errorf("read finding thread: %w", err)
	}
	role, err := normalizeMessageRole(params.Role)
	if err != nil {
		return dbgen.FindingThreadMessage{}, err
	}
	content := strings.TrimSpace(params.Content)
	if content == "" {
		return dbgen.FindingThreadMessage{}, fmt.Errorf("%w: content is required", ErrInvalidMessage)
	}
	evidenceRefs, err := normalizeEvidenceRefs(params.EvidenceRefsJSON)
	if err != nil {
		return dbgen.FindingThreadMessage{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	message, err := s.Queries.CreateFindingThreadMessage(ctx, dbgen.CreateFindingThreadMessageParams{
		ID:               s.newID("finding_thread_message_"),
		ThreadID:         thread.ID,
		Role:             role,
		AgentConfigID:    nullableString(params.AgentConfigID),
		Content:          content,
		EvidenceRefsJson: string(evidenceRefs),
		ArtifactID:       nullableString(params.ArtifactID),
		CreatedAt:        now,
	})
	if err != nil {
		return dbgen.FindingThreadMessage{}, fmt.Errorf("create finding thread message: %w", err)
	}
	if _, err := s.Queries.UpdateFindingThread(ctx, dbgen.UpdateFindingThreadParams{
		ID:        thread.ID,
		Title:     thread.Title,
		UpdatedAt: now,
	}); err != nil {
		return dbgen.FindingThreadMessage{}, fmt.Errorf("touch finding thread: %w", err)
	}
	return message, nil
}

func (s Service) findingScoped(ctx context.Context, findingID string, reviewSessionID string) (dbgen.Finding, error) {
	findingID = strings.TrimSpace(findingID)
	if findingID == "" {
		return dbgen.Finding{}, fmt.Errorf("%w: finding id is required", ErrFindingNotFound)
	}
	finding, err := s.Queries.GetFinding(ctx, findingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.Finding{}, ErrFindingNotFound
		}
		return dbgen.Finding{}, fmt.Errorf("read finding: %w", err)
	}
	if sessionID := strings.TrimSpace(reviewSessionID); sessionID != "" && finding.ReviewSessionID != sessionID {
		return dbgen.Finding{}, ErrFindingNotFound
	}
	return finding, nil
}

func normalizeMessageRole(role string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case MessageRoleUser:
		return MessageRoleUser, nil
	case MessageRoleAssistant:
		return MessageRoleAssistant, nil
	case MessageRoleSystem:
		return MessageRoleSystem, nil
	case MessageRoleAgent:
		return MessageRoleAgent, nil
	default:
		return "", fmt.Errorf("%w: role is invalid", ErrInvalidMessage)
	}
}

func normalizeEvidenceRefs(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return json.RawMessage("[]"), nil
	}
	if !json.Valid([]byte(trimmed)) {
		return nil, fmt.Errorf("%w: evidence_refs_json must be valid JSON", ErrInvalidMessage)
	}
	var refs []any
	if err := json.Unmarshal([]byte(trimmed), &refs); err != nil {
		return nil, fmt.Errorf("%w: evidence_refs_json must be an array", ErrInvalidMessage)
	}
	encoded, err := json.Marshal(refs)
	if err != nil {
		return nil, fmt.Errorf("encode evidence refs: %w", err)
	}
	return encoded, nil
}

func defaultThreadTitle(finding dbgen.Finding) string {
	claim := strings.TrimSpace(finding.CanonicalClaim)
	if claim == "" {
		return "Finding follow-up"
	}
	return truncateBytes(claim, defaultThreadTitleBytes)
}

func truncateBytes(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	truncated := value[:limit]
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
	return strings.TrimSpace(truncated)
}

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) newID(prefix string) string {
	if s.NewID != nil {
		return s.NewID(prefix)
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + "unavailable"
	}
	return prefix + hex.EncodeToString(bytes[:])
}
