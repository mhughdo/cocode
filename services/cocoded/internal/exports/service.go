package exports

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

var ErrCopyPacketSourceNotFound = errors.New("copy packet source was not found")

type Service struct {
	Database  *sql.DB
	Queries   *dbgen.Queries
	Artifacts *artifact.Store
	Now       func() time.Time
	NewID     func(prefix string) string
}

type CreateCopyPacketParams struct {
	ReviewSessionID        string
	FindingID              string
	FindingIDs             []string
	Format                 Format
	IncludeCodeSnippets    bool
	IncludeEvidence        bool
	IncludeCounterEvidence bool
	TargetAgent            string
}

type CreateCopyPacketResult struct {
	Packet   dbgen.CopyPacket
	Artifact dbgen.Artifact
	Rendered Packet
}

type MarkCopyPacketCopiedParams struct {
	CopyPacketID string
}

type MarkCopyPacketCopiedResult struct {
	Packet     dbgen.CopyPacket
	FindingIDs []string
	Decisions  []dbgen.HumanDecision
}

func (s Service) CreateCopyPacket(ctx context.Context, params CreateCopyPacketParams) (CreateCopyPacketResult, error) {
	if s.Queries == nil {
		return CreateCopyPacketResult{}, fmt.Errorf("%w: queries are required", ErrInvalidCopyPacket)
	}
	if s.Artifacts == nil {
		return CreateCopyPacketResult{}, fmt.Errorf("%w: artifact store is required", ErrInvalidCopyPacket)
	}
	selected, reviewSessionID, err := s.selectCopyPacketFindings(ctx, params)
	if err != nil {
		return CreateCopyPacketResult{}, err
	}
	session, err := s.Queries.GetReviewSession(ctx, reviewSessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CreateCopyPacketResult{}, ErrCopyPacketSourceNotFound
		}
		return CreateCopyPacketResult{}, fmt.Errorf("read review session: %w", err)
	}
	snapshot, err := s.Queries.GetPullRequestSnapshot(ctx, session.SnapshotID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CreateCopyPacketResult{}, ErrCopyPacketSourceNotFound
		}
		return CreateCopyPacketResult{}, fmt.Errorf("read snapshot: %w", err)
	}
	repository, err := s.Queries.GetRepository(ctx, session.RepositoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CreateCopyPacketResult{}, ErrCopyPacketSourceNotFound
		}
		return CreateCopyPacketResult{}, fmt.Errorf("read repository: %w", err)
	}
	evidenceByFinding, err := s.evidenceByFinding(ctx, reviewSessionID)
	if err != nil {
		return CreateCopyPacketResult{}, err
	}
	input := Input{
		Snapshot: snapshotForPacket(snapshot, repository),
		Session: ReviewSession{
			ID:    session.ID,
			Title: session.Title,
		},
		Findings: findingsForPacket(selected, evidenceByFinding),
		Options: Options{
			Format:                 params.Format,
			IncludeEvidence:        params.IncludeEvidence,
			IncludeCounterEvidence: params.IncludeCounterEvidence,
			IncludeCodeSnippets:    params.IncludeCodeSnippets,
		},
	}
	rendered, err := RenderCopyPacket(input)
	if err != nil {
		return CreateCopyPacketResult{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	packetID := s.newID("copy_packet_")
	metadata, err := copyPacketMetadata(params, selected, rendered.Format)
	if err != nil {
		return CreateCopyPacketResult{}, err
	}
	contentType := copyPacketContentType(rendered.Format)
	artifactRow, err := s.Artifacts.Save(ctx, artifact.SaveParams{
		ID:              s.newID("artifact_"),
		WorkspaceID:     session.WorkspaceID,
		ReviewSessionID: sql.NullString{String: session.ID, Valid: true},
		Kind:            "copy_packet",
		RelativePath:    copyPacketRelativePath(session.ID, packetID, rendered.Format),
		ContentType:     contentType,
		MetadataJSON:    string(metadata),
		CreatedAt:       now,
	}, []byte(rendered.Content))
	if err != nil {
		return CreateCopyPacketResult{}, fmt.Errorf("save copy packet artifact: %w", err)
	}
	packetRow, err := s.Queries.CreateCopyPacket(ctx, dbgen.CreateCopyPacketParams{
		ID:                packetID,
		ReviewSessionID:   session.ID,
		FindingID:         copyPacketFindingID(selected),
		Format:            string(rendered.Format),
		ContentArtifactID: artifactRow.ID,
		FindingCount:      int64(rendered.FindingCount),
		TokenEstimate:     int64(rendered.TokenEstimate),
		CreatedAt:         now,
	})
	if err != nil {
		return CreateCopyPacketResult{}, fmt.Errorf("create copy packet row: %w", err)
	}
	return CreateCopyPacketResult{Packet: packetRow, Artifact: artifactRow, Rendered: rendered}, nil
}

func (s Service) MarkCopyPacketCopied(ctx context.Context, params MarkCopyPacketCopiedParams) (MarkCopyPacketCopiedResult, error) {
	if s.Queries == nil {
		return MarkCopyPacketCopiedResult{}, fmt.Errorf("%w: queries are required", ErrInvalidCopyPacket)
	}
	if s.Database == nil {
		return MarkCopyPacketCopiedResult{}, fmt.Errorf("%w: database is required", ErrInvalidCopyPacket)
	}
	packetID := strings.TrimSpace(params.CopyPacketID)
	if packetID == "" {
		return MarkCopyPacketCopiedResult{}, fmt.Errorf("%w: copy packet id is required", ErrInvalidCopyPacket)
	}
	packet, err := s.Queries.GetCopyPacket(ctx, packetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MarkCopyPacketCopiedResult{}, ErrCopyPacketSourceNotFound
		}
		return MarkCopyPacketCopiedResult{}, fmt.Errorf("read copy packet: %w", err)
	}
	findingIDs, err := s.copyPacketFindingIDs(ctx, packet)
	if err != nil {
		return MarkCopyPacketCopiedResult{}, err
	}
	tx, err := s.Database.BeginTx(ctx, nil)
	if err != nil {
		return MarkCopyPacketCopiedResult{}, fmt.Errorf("begin copy packet copied update: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	txQueries := s.Queries.WithTx(tx)
	now := s.now().Format(time.RFC3339Nano)
	updatedPacket, err := txQueries.MarkCopyPacketCopied(ctx, dbgen.MarkCopyPacketCopiedParams{
		ID:       packet.ID,
		CopiedAt: sql.NullString{String: now, Valid: true},
	})
	if err != nil {
		return MarkCopyPacketCopiedResult{}, fmt.Errorf("mark copy packet copied: %w", err)
	}
	decisions := make([]dbgen.HumanDecision, 0, len(findingIDs))
	for _, findingID := range findingIDs {
		finding, err := txQueries.GetFinding(ctx, findingID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return MarkCopyPacketCopiedResult{}, ErrCopyPacketSourceNotFound
			}
			return MarkCopyPacketCopiedResult{}, fmt.Errorf("read copied finding: %w", err)
		}
		if finding.ReviewSessionID != packet.ReviewSessionID {
			return MarkCopyPacketCopiedResult{}, ErrCopyPacketSourceNotFound
		}
		if _, err := txQueries.UpdateFindingDecisionStatus(ctx, dbgen.UpdateFindingDecisionStatusParams{
			ID:             finding.ID,
			DecisionStatus: "copied",
			UpdatedAt:      now,
		}); err != nil {
			return MarkCopyPacketCopiedResult{}, fmt.Errorf("update copied finding: %w", err)
		}
		metadata, err := copiedDecisionMetadata(packet)
		if err != nil {
			return MarkCopyPacketCopiedResult{}, err
		}
		decision, err := txQueries.CreateHumanDecision(ctx, dbgen.CreateHumanDecisionParams{
			ID:              s.newID("human_decision_"),
			FindingID:       finding.ID,
			ReviewSessionID: finding.ReviewSessionID,
			Decision:        "copied",
			Reason:          sql.NullString{String: "copy_packet", Valid: true},
			MetadataJson:    string(metadata),
			CreatedAt:       now,
		})
		if err != nil {
			return MarkCopyPacketCopiedResult{}, fmt.Errorf("store copied decision: %w", err)
		}
		decisions = append(decisions, decision)
	}
	if err := tx.Commit(); err != nil {
		return MarkCopyPacketCopiedResult{}, fmt.Errorf("commit copy packet copied update: %w", err)
	}
	committed = true
	return MarkCopyPacketCopiedResult{Packet: updatedPacket, FindingIDs: findingIDs, Decisions: decisions}, nil
}

func (s Service) selectCopyPacketFindings(ctx context.Context, params CreateCopyPacketParams) ([]dbgen.Finding, string, error) {
	if strings.TrimSpace(params.FindingID) != "" {
		finding, err := s.Queries.GetFinding(ctx, strings.TrimSpace(params.FindingID))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, "", ErrCopyPacketSourceNotFound
			}
			return nil, "", fmt.Errorf("read finding: %w", err)
		}
		if sessionID := strings.TrimSpace(params.ReviewSessionID); sessionID != "" && finding.ReviewSessionID != sessionID {
			return nil, "", ErrCopyPacketSourceNotFound
		}
		return []dbgen.Finding{finding}, finding.ReviewSessionID, nil
	}
	reviewSessionID := strings.TrimSpace(params.ReviewSessionID)
	if reviewSessionID == "" {
		return nil, "", fmt.Errorf("%w: review session id is required", ErrInvalidCopyPacket)
	}
	rows, err := s.Queries.ListFindingsBySession(ctx, reviewSessionID)
	if err != nil {
		return nil, "", fmt.Errorf("list findings: %w", err)
	}
	if len(params.FindingIDs) == 0 {
		selected := make([]dbgen.Finding, 0, len(rows))
		for _, row := range rows {
			if row.DecisionStatus == "accepted" {
				selected = append(selected, row)
			}
		}
		if len(selected) == 0 {
			return nil, "", fmt.Errorf("%w: no accepted findings are available", ErrInvalidCopyPacket)
		}
		return selected, reviewSessionID, nil
	}
	byID := make(map[string]dbgen.Finding, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	ids := uniqueStrings(params.FindingIDs)
	selected := make([]dbgen.Finding, 0, len(ids))
	for _, id := range ids {
		row, ok := byID[id]
		if !ok {
			return nil, "", ErrCopyPacketSourceNotFound
		}
		selected = append(selected, row)
	}
	return selected, reviewSessionID, nil
}

func (s Service) evidenceByFinding(ctx context.Context, reviewSessionID string) (map[string][]EvidenceItem, error) {
	rows, err := s.Queries.ListEvidenceItemsBySession(ctx, reviewSessionID)
	if err != nil {
		return nil, fmt.Errorf("list finding evidence: %w", err)
	}
	byFinding := make(map[string][]EvidenceItem)
	for _, row := range rows {
		byFinding[row.FindingID] = append(byFinding[row.FindingID], evidenceForPacket(row))
	}
	return byFinding, nil
}

func snapshotForPacket(snapshot dbgen.PullRequestSnapshot, repository dbgen.Repository) Snapshot {
	return Snapshot{
		Repository: firstNonEmpty(repositoryFullName(repository), snapshotFullName(snapshot), repository.Name),
		SourceType: snapshot.SourceType,
		PRNumber:   nullableInt64Value(snapshot.PrNumber),
		PRTitle:    nullableStringValue(snapshot.PrTitle),
		PRURL:      nullableStringValue(snapshot.PrUrl),
		BaseRef:    nullableStringValue(snapshot.BaseRef),
		HeadRef:    nullableStringValue(snapshot.HeadRef),
		BaseSHA:    nullableStringValue(snapshot.BaseSha),
		HeadSHA:    nullableStringValue(snapshot.HeadSha),
	}
}

func findingsForPacket(findings []dbgen.Finding, evidence map[string][]EvidenceItem) []Finding {
	result := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		result = append(result, Finding{
			ID:                 finding.ID,
			CanonicalClaim:     finding.CanonicalClaim,
			Category:           finding.Category,
			Severity:           finding.Severity,
			VerificationStatus: finding.VerificationStatus,
			DecisionStatus:     finding.DecisionStatus,
			Confidence:         finding.Confidence,
			PrimaryPath:        nullableStringValue(finding.PrimaryPath),
			PrimaryStartLine:   nullableInt64Value(finding.PrimaryStartLine),
			PrimaryEndLine:     nullableInt64Value(finding.PrimaryEndLine),
			EvidenceSummary:    nullableStringValue(finding.EvidenceSummary),
			CounterSummary:     nullableStringValue(finding.CounterEvidenceSummary),
			SuggestedFix:       nullableStringValue(finding.SuggestedFix),
			DraftComment:       nullableStringValue(finding.DraftComment),
			Evidence:           evidence[finding.ID],
		})
	}
	return result
}

func evidenceForPacket(row dbgen.EvidenceItem) EvidenceItem {
	return EvidenceItem{
		ID:          row.ID,
		Kind:        row.Kind,
		Title:       row.Title,
		Summary:     row.Summary,
		Path:        nullableStringValue(row.Path),
		StartLine:   nullableInt64Value(row.StartLine),
		EndLine:     nullableInt64Value(row.EndLine),
		Confidence:  row.Confidence,
		CodeSnippet: evidenceCodeSnippet(row.MetadataJson),
	}
}

func evidenceCodeSnippet(metadataJSON string) string {
	var payload struct {
		CodeSnippet string `json:"code_snippet"`
	}
	if err := json.Unmarshal([]byte(metadataJSON), &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.CodeSnippet)
}

func copyPacketMetadata(params CreateCopyPacketParams, findings []dbgen.Finding, format Format) (json.RawMessage, error) {
	findingIDs := make([]string, 0, len(findings))
	for _, finding := range findings {
		findingIDs = append(findingIDs, finding.ID)
	}
	metadata, err := json.Marshal(map[string]any{
		"format":                   format,
		"finding_ids":              findingIDs,
		"target_agent":             strings.TrimSpace(params.TargetAgent),
		"include_evidence":         params.IncludeEvidence,
		"include_counter_evidence": params.IncludeCounterEvidence,
		"include_code_snippets":    params.IncludeCodeSnippets,
	})
	if err != nil {
		return nil, fmt.Errorf("encode copy packet metadata: %w", err)
	}
	return metadata, nil
}

func (s Service) copyPacketFindingIDs(ctx context.Context, packet dbgen.CopyPacket) ([]string, error) {
	if packet.FindingID.Valid && strings.TrimSpace(packet.FindingID.String) != "" {
		return []string{strings.TrimSpace(packet.FindingID.String)}, nil
	}
	artifactRow, err := s.Queries.GetArtifact(ctx, packet.ContentArtifactID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCopyPacketSourceNotFound
		}
		return nil, fmt.Errorf("read copy packet artifact: %w", err)
	}
	var metadata struct {
		FindingIDs []string `json:"finding_ids"`
	}
	if err := json.Unmarshal([]byte(artifactRow.MetadataJson), &metadata); err != nil {
		return nil, fmt.Errorf("%w: copy packet metadata is invalid", ErrInvalidCopyPacket)
	}
	findingIDs := uniqueStrings(metadata.FindingIDs)
	if len(findingIDs) == 0 {
		return nil, fmt.Errorf("%w: copy packet has no findings", ErrInvalidCopyPacket)
	}
	return findingIDs, nil
}

func copiedDecisionMetadata(packet dbgen.CopyPacket) (json.RawMessage, error) {
	metadata, err := json.Marshal(map[string]any{
		"source":              "copy_packet",
		"copy_packet_id":      packet.ID,
		"content_artifact_id": packet.ContentArtifactID,
	})
	if err != nil {
		return nil, fmt.Errorf("encode copied decision metadata: %w", err)
	}
	return metadata, nil
}

func copyPacketFindingID(findings []dbgen.Finding) sql.NullString {
	if len(findings) == 1 {
		return sql.NullString{String: findings[0].ID, Valid: true}
	}
	return sql.NullString{}
}

func copyPacketRelativePath(sessionID string, packetID string, format Format) string {
	return filepath.ToSlash(filepath.Join("copy-packets", cleanPathSegment(sessionID), cleanPathSegment(packetID)+copyPacketExtension(format)))
}

func copyPacketExtension(format Format) string {
	switch format {
	case FormatJSON:
		return ".json"
	case FormatXMLish:
		return ".xml"
	case FormatCompact, FormatGitHubSummary:
		return ".txt"
	default:
		return ".md"
	}
}

func copyPacketContentType(format Format) string {
	switch format {
	case FormatJSON:
		return "application/json"
	case FormatXMLish:
		return "application/xml"
	default:
		return "text/markdown"
	}
}

func repositoryFullName(repository dbgen.Repository) string {
	if repository.Owner.Valid && strings.TrimSpace(repository.Owner.String) != "" {
		return strings.TrimSpace(repository.Owner.String) + "/" + repository.Name
	}
	return repository.Name
}

func snapshotFullName(snapshot dbgen.PullRequestSnapshot) string {
	if snapshot.Owner.Valid && snapshot.Repo.Valid {
		return strings.TrimSpace(snapshot.Owner.String) + "/" + strings.TrimSpace(snapshot.Repo.String)
	}
	return ""
}

func uniqueStrings(values []string) []string {
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

var unsafePathSegment = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func cleanPathSegment(value string) string {
	value = strings.Trim(unsafePathSegment.ReplaceAllString(value, "_"), "._-")
	if value == "" {
		return "unknown"
	}
	return value
}

func nullableStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullableInt64Value(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
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
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + fmt.Sprint(time.Now().UTC().UnixNano())
	}
	return prefix + hex.EncodeToString(bytes[:])
}
