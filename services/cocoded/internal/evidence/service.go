package evidence

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const (
	KindSupporting     = "supporting"
	KindCounter        = "counter"
	KindNeutral        = "neutral"
	KindMissing        = "missing"
	KindTest           = "test"
	KindSearch         = "search"
	KindAgent          = "agent"
	KindStaticAnalysis = "static_analysis"
)

const (
	StatusUnverified          = "unverified"
	StatusVerified            = "verified"
	StatusPlausible           = "plausible"
	StatusNeedsHuman          = "needs_human"
	StatusLikelyFalsePositive = "likely_false_positive"
	StatusDuplicate           = "duplicate"
	StatusNotActionable       = "not_actionable"
)

const (
	defaultEvidenceContextLines = 3
	defaultSnippetBytes         = 12 * 1024
	defaultCounterEvidenceLimit = 6
)

type Item struct {
	Kind       string          `json:"kind"`
	Title      string          `json:"title"`
	Summary    string          `json:"summary"`
	Path       string          `json:"path,omitempty"`
	StartLine  int64           `json:"start_line,omitempty"`
	EndLine    int64           `json:"end_line,omitempty"`
	Confidence float64         `json:"confidence"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

type Service struct {
	Queries         *dbgen.Queries
	Searcher        CodeSearcher
	Now             func() time.Time
	NewID           func(prefix string) string
	ContextLines    int
	MaxSnippetBytes int64
}

type VerificationSummary struct {
	Findings             int            `json:"findings"`
	EvidenceItemsCreated int            `json:"evidence_items_created"`
	ByVerificationStatus map[string]int `json:"by_verification_status"`
	SupportingEvidence   int            `json:"supporting_evidence"`
	CounterEvidence      int            `json:"counter_evidence"`
	MissingEvidence      int            `json:"missing_evidence"`
}

type findingEvidenceResult struct {
	status                 string
	evidenceSummary        string
	counterEvidenceSummary string
	created                int
	supporting             int
	counter                int
	missing                int
}

type changedFileIndex struct {
	byPath map[string]dbgen.ChangedFile
}

type ruleProfile struct {
	ID          string
	DisplayName string
	Terms       []string
}

func (s *Service) VerifySession(ctx context.Context, session dbgen.ReviewSession, repository dbgen.Repository) (VerificationSummary, error) {
	if s == nil || s.Queries == nil {
		return VerificationSummary{}, errors.New("evidence verifier queries are required")
	}
	if strings.TrimSpace(repository.LocalPath) == "" {
		return VerificationSummary{}, errors.New("repository local path is required")
	}
	findings, err := s.Queries.ListFindingsBySession(ctx, session.ID)
	if err != nil {
		return VerificationSummary{}, fmt.Errorf("list findings for verification: %w", err)
	}
	changedFiles, err := s.Queries.ListChangedFilesBySnapshot(ctx, session.SnapshotID)
	if err != nil {
		return VerificationSummary{}, fmt.Errorf("list changed files for verification: %w", err)
	}
	index := newChangedFileIndex(changedFiles)
	summary := VerificationSummary{
		Findings:             len(findings),
		ByVerificationStatus: map[string]int{},
	}
	for _, finding := range findings {
		result, err := s.verifyFinding(ctx, repository.LocalPath, index, finding)
		if err != nil {
			return VerificationSummary{}, err
		}
		summary.EvidenceItemsCreated += result.created
		summary.SupportingEvidence += result.supporting
		summary.CounterEvidence += result.counter
		summary.MissingEvidence += result.missing
		summary.ByVerificationStatus[result.status]++
	}
	return summary, nil
}

func (s *Service) verifyFinding(ctx context.Context, repoRoot string, index changedFileIndex, finding dbgen.Finding) (findingEvidenceResult, error) {
	if err := s.deletePreviousVerifierEvidence(ctx, finding.ID); err != nil {
		return findingEvidenceResult{}, err
	}
	result := findingEvidenceResult{}
	primary, err := s.attachPrimaryLocationEvidence(ctx, repoRoot, index, finding)
	if err != nil {
		return findingEvidenceResult{}, err
	}
	result.created += primary.created
	result.supporting += primary.supporting
	result.missing += primary.missing
	result.evidenceSummary = primary.evidenceSummary

	counter, err := s.attachCounterEvidence(ctx, repoRoot, finding)
	if err != nil {
		return findingEvidenceResult{}, err
	}
	result.created += counter.created
	result.counter += counter.counter
	result.counterEvidenceSummary = counter.counterEvidenceSummary
	result.status = assignVerificationStatus(finding, result.supporting, result.counter, result.missing)

	now := s.now().Format(time.RFC3339Nano)
	if _, err := s.Queries.UpdateFindingVerificationEvidence(ctx, dbgen.UpdateFindingVerificationEvidenceParams{
		VerificationStatus:     result.status,
		EvidenceSummary:        nullableString(result.evidenceSummary),
		CounterEvidenceSummary: nullableString(result.counterEvidenceSummary),
		UpdatedAt:              now,
		ID:                     finding.ID,
	}); err != nil {
		return findingEvidenceResult{}, fmt.Errorf("update finding verification %s: %w", finding.ID, err)
	}
	return result, nil
}

func (s *Service) attachPrimaryLocationEvidence(ctx context.Context, repoRoot string, index changedFileIndex, finding dbgen.Finding) (findingEvidenceResult, error) {
	if !finding.PrimaryPath.Valid || strings.TrimSpace(finding.PrimaryPath.String) == "" {
		item, err := s.createEvidenceItem(ctx, finding.ID, Item{
			Kind:       KindMissing,
			Title:      "Primary code location is missing",
			Summary:    "The finding does not include a concrete changed-file location, so local verification needs human review.",
			Confidence: 1,
			Metadata:   mustMetadata(map[string]any{"producer": "local_verifier", "source": "primary_location", "reason": "missing_location"}),
		})
		return findingEvidenceResult{
			created:         1,
			missing:         1,
			evidenceSummary: item.Summary,
		}, err
	}
	changedFile, ok := index.byPath[cleanPathKey(finding.PrimaryPath.String)]
	if !ok {
		item, err := s.createMissingPrimaryEvidence(ctx, finding.ID, finding.PrimaryPath.String, "not_changed_file", "The primary location does not map to this review snapshot's changed files.")
		return findingEvidenceResult{created: 1, missing: 1, evidenceSummary: item.Summary}, err
	}
	if changedFile.IsBinary != 0 || changedFile.IsExcluded != 0 {
		item, err := s.createMissingPrimaryEvidence(ctx, finding.ID, changedFile.Path, "unreadable_changed_file", "The primary changed file is binary or excluded from review context.")
		return findingEvidenceResult{created: 1, missing: 1, evidenceSummary: item.Summary}, err
	}
	startLine := nullableInt64Value(finding.PrimaryStartLine)
	endLine := startLine
	if startLine <= 0 {
		var ok bool
		startLine, endLine, ok = firstChangedLineRange(changedFile.LineRangesJson)
		if !ok {
			item, err := s.createMissingPrimaryEvidence(ctx, finding.ID, finding.PrimaryPath.String, "missing_line", "The finding has a file path but no primary line number.")
			return findingEvidenceResult{created: 1, missing: 1, evidenceSummary: item.Summary}, err
		}
	}
	if finding.PrimaryEndLine.Valid && finding.PrimaryEndLine.Int64 >= startLine {
		endLine = finding.PrimaryEndLine.Int64
	}
	snippet, windowStart, windowEnd, truncated, err := readSnippet(repoRoot, changedFile.Path, startLine, endLine, s.contextLines(), s.maxSnippetBytes())
	if err != nil {
		item, createErr := s.createMissingPrimaryEvidence(ctx, finding.ID, changedFile.Path, "read_failed", "Primary changed code could not be read: "+err.Error())
		if createErr != nil {
			return findingEvidenceResult{}, createErr
		}
		return findingEvidenceResult{created: 1, missing: 1, evidenceSummary: item.Summary}, nil
	}
	title := fmt.Sprintf("Changed code at %s:%d", changedFile.Path, startLine)
	if endLine != startLine {
		title = fmt.Sprintf("Changed code at %s:%d-%d", changedFile.Path, startLine, endLine)
	}
	summary := primaryEvidenceSummary(finding, changedFile.Path, startLine, endLine, snippet, truncated)
	item, err := s.createEvidenceItem(ctx, finding.ID, Item{
		Kind:       KindSupporting,
		Title:      title,
		Summary:    summary,
		Path:       changedFile.Path,
		StartLine:  startLine,
		EndLine:    endLine,
		Confidence: clampConfidence(finding.Confidence),
		Metadata: mustMetadata(map[string]any{
			"producer":        "local_verifier",
			"source":          "primary_location",
			"changed_file_id": changedFile.ID,
			"line_window": map[string]any{
				"start_line": windowStart,
				"end_line":   windowEnd,
			},
			"code_snippet": snippet,
			"truncated":    truncated,
		}),
	})
	if err != nil {
		return findingEvidenceResult{}, err
	}
	return findingEvidenceResult{created: 1, supporting: 1, evidenceSummary: item.Summary}, nil
}

func firstChangedLineRange(raw string) (int64, int64, bool) {
	var ranges [][]int64
	if err := json.Unmarshal([]byte(raw), &ranges); err != nil {
		return 0, 0, false
	}
	for _, item := range ranges {
		if len(item) != 2 || item[0] < 1 || item[1] < item[0] {
			continue
		}
		return item[0], item[1], true
	}
	return 0, 0, false
}

func (s *Service) createMissingPrimaryEvidence(ctx context.Context, findingID string, path string, reason string, summary string) (dbgen.EvidenceItem, error) {
	title := "Primary changed code unavailable"
	switch reason {
	case "missing_line":
		title = "Changed file needs a line anchor"
	case "not_changed_file":
		title = "Location is outside the reviewed diff"
	case "unreadable_changed_file":
		title = "Changed file cannot be previewed"
	case "read_failed":
		title = "Changed code could not be read"
	}
	return s.createEvidenceItem(ctx, findingID, Item{
		Kind:       KindMissing,
		Title:      title,
		Summary:    summary,
		Path:       path,
		Confidence: 1,
		Metadata:   mustMetadata(map[string]any{"producer": "local_verifier", "source": "primary_location", "reason": reason}),
	})
}

func (s *Service) attachCounterEvidence(ctx context.Context, repoRoot string, finding dbgen.Finding) (findingEvidenceResult, error) {
	searcher := s.searcher()
	profile := classifyRuleProfile(finding)
	terms := counterEvidenceTerms(finding, profile)
	seen := map[string]struct{}{}
	result := findingEvidenceResult{counterEvidenceSummary: "No counter-evidence found by local search."}
	for _, term := range terms {
		if result.counter >= defaultCounterEvidenceLimit {
			break
		}
		matches, err := searcher.Search(ctx, SearchOptions{
			RepoRoot:    repoRoot,
			Query:       term,
			ExcludePath: primaryExcludePath(finding),
			Limit:       defaultCounterEvidenceLimit,
			OutputLimit: defaultSearchOutputLimit,
		})
		if err != nil {
			return findingEvidenceResult{}, fmt.Errorf("search counter-evidence for finding %s: %w", finding.ID, err)
		}
		for _, match := range matches {
			if result.counter >= defaultCounterEvidenceLimit {
				break
			}
			if !looksLikeCounterEvidence(match) {
				continue
			}
			key := match.Path + ":" + fmt.Sprint(match.Line)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			kind := KindCounter
			if isLikelyTestPath(match.Path) {
				kind = KindTest
			}
			title := counterEvidenceTitle(kind, match)
			summary := counterEvidenceSummary(kind, profile, term, match)
			codeSnippet := fmt.Sprintf("%d: %s", match.Line, strings.TrimSpace(match.Text))
			lineWindow := map[string]any{
				"start_line": match.Line,
				"end_line":   match.Line,
			}
			truncated := false
			if snippet, windowStart, windowEnd, wasTruncated, err := readSnippet(repoRoot, match.Path, match.Line, match.Line, 2, s.maxSnippetBytes()); err == nil && strings.TrimSpace(snippet) != "" {
				codeSnippet = snippet
				lineWindow = map[string]any{
					"start_line": windowStart,
					"end_line":   windowEnd,
				}
				truncated = wasTruncated
			}
			if _, err := s.createEvidenceItem(ctx, finding.ID, Item{
				Kind:       kind,
				Title:      title,
				Summary:    summary,
				Path:       match.Path,
				StartLine:  match.Line,
				EndLine:    match.Line,
				Confidence: 0.6,
				Metadata: mustMetadata(map[string]any{
					"producer":     "local_verifier",
					"source":       "counter_evidence_search",
					"rule":         profile.ID,
					"search_term":  term,
					"line_window":  lineWindow,
					"code_snippet": codeSnippet,
					"truncated":    truncated,
				}),
			}); err != nil {
				return findingEvidenceResult{}, err
			}
			result.created++
			result.counter++
		}
	}
	if result.counter > 0 {
		result.counterEvidenceSummary = fmt.Sprintf("%d possible guard, config, or test signal(s) need comparison against the changed path before the finding is accepted or dismissed.", result.counter)
	}
	return result, nil
}

func primaryEvidenceSummary(finding dbgen.Finding, path string, startLine int64, endLine int64, snippet string, truncated bool) string {
	location := fmt.Sprintf("%s:%d", path, startLine)
	if endLine > startLine {
		location = fmt.Sprintf("%s:%d-%d", path, startLine, endLine)
	}
	parts := []string{
		fmt.Sprintf("The finding is anchored to changed code at %s.", location),
	}
	if claim := strings.TrimSpace(finding.CanonicalClaim); claim != "" {
		parts = append(parts, fmt.Sprintf("Claim: %s.", sentenceTrim(claim)))
	}
	if observed := firstSnippetLineInRange(snippet, startLine, endLine); observed != "" {
		parts = append(parts, fmt.Sprintf("Observed code: `%s`.", observed))
	}
	if finding.SuggestedFix.Valid && strings.TrimSpace(finding.SuggestedFix.String) != "" {
		parts = append(parts, fmt.Sprintf("Expected remediation: %s.", sentenceTrim(finding.SuggestedFix.String)))
	}
	if truncated {
		parts = append(parts, "The stored code window was truncated; open the file for full context.")
	}
	return strings.Join(parts, " ")
}

func firstSnippetLineInRange(snippet string, startLine int64, endLine int64) string {
	if startLine <= 0 {
		startLine = 1
	}
	if endLine < startLine {
		endLine = startLine
	}
	fallback := ""
	for _, raw := range strings.Split(snippet, "\n") {
		lineNumber, text, ok := parseNumberedSnippetLine(raw)
		if !ok {
			candidate := strings.TrimSpace(raw)
			if fallback == "" && candidate != "" {
				fallback = candidate
			}
			continue
		}
		candidate := strings.TrimSpace(text)
		if fallback == "" && candidate != "" {
			fallback = candidate
		}
		if lineNumber >= startLine && lineNumber <= endLine && candidate != "" {
			return sentenceTrim(candidate)
		}
	}
	return sentenceTrim(fallback)
}

func parseNumberedSnippetLine(line string) (int64, string, bool) {
	prefix, suffix, ok := strings.Cut(strings.TrimLeft(line, " \t"), ":")
	if !ok {
		return 0, "", false
	}
	var lineNumber int64
	for _, char := range prefix {
		if char < '0' || char > '9' {
			return 0, "", false
		}
		lineNumber = lineNumber*10 + int64(char-'0')
	}
	if lineNumber <= 0 {
		return 0, "", false
	}
	return lineNumber, strings.TrimSpace(suffix), true
}

func sentenceTrim(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimRight(value, ".")
	return value
}

func counterEvidenceTitle(kind string, match SearchMatch) string {
	if kind == KindTest {
		return fmt.Sprintf("Related test signal at %s:%d", match.Path, match.Line)
	}
	return fmt.Sprintf("Possible guard or config signal at %s:%d", match.Path, match.Line)
}

func counterEvidenceSummary(kind string, profile ruleProfile, term string, match SearchMatch) string {
	location := fmt.Sprintf("%s:%d", match.Path, match.Line)
	trimmed := strings.TrimSpace(match.Text)
	if len(trimmed) > 180 {
		trimmed = strings.TrimSpace(trimmed[:180]) + "..."
	}
	if kind == KindTest {
		return fmt.Sprintf("Related test search found %q at %s. Use this to check whether tests cover the claim or encode the same behavior. Matched line: `%s`.", term, location, trimmed)
	}
	return fmt.Sprintf("Possible %s counter-signal found %q at %s. Treat this as a lead for an existing guard/config path and compare it with the changed code before dismissing the finding. Matched line: `%s`.", profile.DisplayName, term, location, trimmed)
}

func (s *Service) createEvidenceItem(ctx context.Context, findingID string, item Item) (dbgen.EvidenceItem, error) {
	created, err := s.Queries.CreateEvidenceItem(ctx, dbgen.CreateEvidenceItemParams{
		ID:           s.newID("evidence_item_"),
		FindingID:    findingID,
		Kind:         normalizeKind(item.Kind),
		Title:        strings.TrimSpace(item.Title),
		Summary:      strings.TrimSpace(item.Summary),
		Path:         nullableString(item.Path),
		StartLine:    nullablePositiveInt64(item.StartLine),
		EndLine:      nullablePositiveInt64(item.EndLine),
		Confidence:   clampConfidence(item.Confidence),
		MetadataJson: string(defaultMetadata(item.Metadata)),
		CreatedAt:    s.now().Format(time.RFC3339Nano),
	})
	if err != nil {
		return dbgen.EvidenceItem{}, fmt.Errorf("create evidence item for finding %s: %w", findingID, err)
	}
	return created, nil
}

func (s *Service) deletePreviousVerifierEvidence(ctx context.Context, findingID string) error {
	items, err := s.Queries.ListEvidenceItemsByFinding(ctx, findingID)
	if err != nil {
		return fmt.Errorf("list previous verifier evidence: %w", err)
	}
	for _, item := range items {
		if !isLocalVerifierEvidence(item.MetadataJson) {
			continue
		}
		if err := s.Queries.DeleteEvidenceItem(ctx, item.ID); err != nil {
			return fmt.Errorf("delete previous verifier evidence %s: %w", item.ID, err)
		}
	}
	return nil
}

func isLocalVerifierEvidence(raw string) bool {
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return false
	}
	return metadata["producer"] == "local_verifier"
}

func assignVerificationStatus(finding dbgen.Finding, supporting int, counter int, missing int) string {
	switch {
	case supporting > 0 && counter == 0:
		return StatusVerified
	case supporting > 0 && counter > 0:
		return StatusPlausible
	case supporting == 0 && counter > 0:
		return StatusLikelyFalsePositive
	case missing > 0 && finding.Confidence < 0.4 && !finding.SuggestedFix.Valid:
		return StatusNotActionable
	case missing > 0:
		return StatusNeedsHuman
	default:
		return StatusNeedsHuman
	}
}

func readSnippet(repoRoot string, relativePath string, startLine int64, endLine int64, contextLines int, maxBytes int64) (string, int64, int64, bool, error) {
	path, cleanPath, err := safeRepoFilePath(repoRoot, relativePath)
	if err != nil {
		return "", 0, 0, false, err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return "", 0, 0, false, fmt.Errorf("inspect %s: %w", cleanPath, err)
	}
	if stat.IsDir() {
		return "", 0, 0, false, fmt.Errorf("%s is a directory", cleanPath)
	}
	if maxBytes <= 0 {
		maxBytes = defaultSnippetBytes
	}
	if startLine <= 0 {
		startLine = 1
	}
	if endLine < startLine {
		endLine = startLine
	}
	windowStart := maxInt64(1, startLine-int64(contextLines))
	windowEnd := endLine + int64(contextLines)

	file, err := os.Open(path)
	if err != nil {
		return "", 0, 0, false, fmt.Errorf("open %s: %w", cleanPath, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var builder strings.Builder
	var line int64
	truncated := false
	for scanner.Scan() {
		line++
		if line < windowStart {
			continue
		}
		if line > windowEnd {
			break
		}
		text := fmt.Sprintf("%d: %s\n", line, scanner.Text())
		if int64(builder.Len()+len(text)) > maxBytes {
			remaining := int(maxBytes) - builder.Len()
			if remaining > 0 {
				builder.WriteString(text[:remaining])
			}
			truncated = true
			break
		}
		builder.WriteString(text)
	}
	if err := scanner.Err(); err != nil {
		return "", 0, 0, false, fmt.Errorf("read %s: %w", cleanPath, err)
	}
	return strings.TrimRight(builder.String(), "\n"), windowStart, minInt64(windowEnd, line), truncated, nil
}

// ReadSnippet returns a bounded, line-numbered source window from a repository file.
func ReadSnippet(repoRoot string, relativePath string, startLine int64, endLine int64, contextLines int, maxBytes int64) (string, int64, int64, bool, error) {
	return readSnippet(repoRoot, relativePath, startLine, endLine, contextLines, maxBytes)
}

func newChangedFileIndex(files []dbgen.ChangedFile) changedFileIndex {
	index := changedFileIndex{byPath: map[string]dbgen.ChangedFile{}}
	for _, file := range files {
		index.byPath[cleanPathKey(file.Path)] = file
	}
	return index
}

func classifyRuleProfile(finding dbgen.Finding) ruleProfile {
	text := strings.ToLower(strings.Join([]string{
		finding.CanonicalClaim,
		finding.Category,
		nullableStringValue(finding.SuggestedFix),
		nullableStringValue(finding.DraftComment),
	}, " "))
	switch {
	case containsAny(text, "webhook", "signature", "hmac", "secret", "payload"):
		return ruleProfile{
			ID:          "webhook_validation",
			DisplayName: "webhook validation",
			Terms:       []string{"signature", "hmac", "verify", "validate", "secret", "webhook", "event type"},
		}
	case containsAny(text, "test", "coverage", "assert", "regression"):
		return ruleProfile{
			ID:          "test_coverage",
			DisplayName: "test coverage",
			Terms:       []string{"test", "expect", "assert", "require", "describe"},
		}
	case containsAny(text, "idempot", "duplicate", "replay", "retry", "nonce", "unique"):
		return ruleProfile{
			ID:          "idempotency",
			DisplayName: "idempotency",
			Terms:       []string{"idempotency", "unique", "retry", "idempotent", "constraint", "dedupe", "nonce"},
		}
	case containsAny(text, "auth", "admin", "permission", "authorize", "guard", "middleware", "role"):
		return ruleProfile{
			ID:          "auth_guard",
			DisplayName: "auth guard",
			Terms:       []string{"auth", "RequireAdmin", "authorize", "permission", "admin", "guard", "middleware", "role"},
		}
	default:
		return ruleProfile{
			ID:          "generic",
			DisplayName: "evidence",
			Terms:       []string{"validate", "guard", "test"},
		}
	}
}

func counterEvidenceTerms(finding dbgen.Finding, profile ruleProfile) []string {
	terms := make([]string, 0, 6)
	for _, term := range profile.Terms {
		addTerm(&terms, term)
	}
	if profile.ID != "generic" {
		for _, token := range claimTokens(finding.CanonicalClaim) {
			addTerm(&terms, token)
			if len(terms) >= 6 {
				break
			}
		}
	}
	if finding.PrimaryPath.Valid {
		base := strings.TrimSuffix(filepath.Base(filepath.ToSlash(finding.PrimaryPath.String)), filepath.Ext(finding.PrimaryPath.String))
		addTerm(&terms, base)
	}
	if len(terms) > 6 {
		terms = terms[:6]
	}
	return terms
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func nullableStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func claimTokens(claim string) []string {
	stop := map[string]struct{}{
		"before": {}, "after": {}, "without": {}, "with": {}, "from": {}, "this": {}, "that": {}, "when": {}, "where": {}, "does": {}, "can": {}, "could": {}, "should": {}, "would": {}, "missing": {}, "lacks": {},
	}
	fields := strings.FieldsFunc(strings.ToLower(claim), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		if len(field) < 4 {
			continue
		}
		if _, ok := stop[field]; ok {
			continue
		}
		tokens = append(tokens, field)
	}
	return tokens
}

func looksLikeCounterEvidence(match SearchMatch) bool {
	path := strings.ToLower(filepath.ToSlash(match.Path))
	text := strings.ToLower(match.Text)
	if isProjectMetadataPath(path) {
		return false
	}
	if isDocumentationPath(path) {
		return false
	}
	if isLikelyTestPath(path) || strings.Contains(path, "auth") || strings.Contains(path, "guard") ||
		strings.Contains(path, "middleware") || strings.Contains(path, "permission") || strings.Contains(path, "config") {
		return true
	}
	for _, token := range []string{"require", "authorize", "permission", "auth", "guard", "verify", "signature", "validate", "test", "expect"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func isLikelyTestPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	return strings.Contains(path, "_test.") ||
		strings.Contains(path, ".test.") ||
		strings.Contains(path, ".spec.") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/")
}

func isProjectMetadataPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(path)
	switch base {
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		"go.mod", "go.sum", "cargo.toml", "cargo.lock", "poetry.lock",
		"pyproject.toml", "requirements.txt", "docker-compose.yml", "docker-compose.yaml",
		"compose.yml", "compose.yaml":
		return true
	default:
		return false
	}
}

func isDocumentationPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(path)
	return strings.HasPrefix(path, "docs/") ||
		strings.Contains(path, "/docs/") ||
		strings.HasSuffix(base, ".md") ||
		strings.HasSuffix(base, ".mdx") ||
		strings.HasPrefix(base, "readme") ||
		strings.HasPrefix(base, "changelog")
}

func primaryExcludePath(finding dbgen.Finding) []string {
	if !finding.PrimaryPath.Valid {
		return nil
	}
	return []string{finding.PrimaryPath.String}
}

func addTerm(terms *[]string, term string) {
	term = strings.TrimSpace(term)
	if len(term) < 3 || slices.Contains(*terms, term) {
		return
	}
	*terms = append(*terms, term)
}

func normalizeKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case KindSupporting, KindCounter, KindNeutral, KindMissing, KindTest, KindSearch, KindAgent, KindStaticAnalysis:
		return strings.TrimSpace(kind)
	default:
		return KindNeutral
	}
}

func defaultMetadata(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}

func mustMetadata(payload map[string]any) json.RawMessage {
	data, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func (s *Service) searcher() CodeSearcher {
	if s.Searcher != nil {
		return s.Searcher
	}
	return RipgrepSearcher{}
}

func (s *Service) contextLines() int {
	if s.ContextLines < 0 {
		return 0
	}
	if s.ContextLines == 0 {
		return defaultEvidenceContextLines
	}
	return s.ContextLines
}

func (s *Service) maxSnippetBytes() int64 {
	if s.MaxSnippetBytes <= 0 {
		return defaultSnippetBytes
	}
	return s.MaxSnippetBytes
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) newID(prefix string) string {
	if s.NewID != nil {
		if id := strings.TrimSpace(s.NewID(prefix)); id != "" {
			return id
		}
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + "unavailable"
	}
	return prefix + hex.EncodeToString(bytes[:])
}

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullablePositiveInt64(value int64) sql.NullInt64 {
	if value <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: value, Valid: true}
}

func cleanPathKey(path string) string {
	clean, ok := cleanRelativePath(path)
	if !ok {
		return strings.TrimSpace(filepath.ToSlash(path))
	}
	return clean
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func minInt64(a int64, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
