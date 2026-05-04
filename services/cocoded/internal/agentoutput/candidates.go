package agentoutput

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const CandidateSchemaVersion = "finding-candidate/v1"

var trailingJSONCommaRE = regexp.MustCompile(`,\s*([}\]])`)
var jsonFenceRE = regexp.MustCompile("(?is)```(?:json)?\\s*(.*?)\\s*```")

type CandidateParseResult struct {
	Candidates  []Candidate  `json:"candidates"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type Candidate struct {
	SchemaVersion          string              `json:"schema_version,omitempty"`
	Claim                  string              `json:"claim"`
	Category               string              `json:"category"`
	Severity               string              `json:"severity"`
	Confidence             float64             `json:"confidence"`
	Locations              []CandidateLocation `json:"locations"`
	PrimaryPath            string              `json:"primary_path,omitempty"`
	PrimaryStartLine       int64               `json:"primary_start_line,omitempty"`
	PrimaryEndLine         int64               `json:"primary_end_line,omitempty"`
	Evidence               []CandidateEvidence `json:"evidence"`
	CounterEvidenceRequest string              `json:"counter_evidence_request,omitempty"`
	SuggestedFix           string              `json:"suggested_fix,omitempty"`
	DraftComment           string              `json:"draft_comment,omitempty"`
	Fingerprint            string              `json:"fingerprint,omitempty"`
}

func (c *Candidate) UnmarshalJSON(data []byte) error {
	var raw struct {
		SchemaVersion          string              `json:"schema_version"`
		Claim                  string              `json:"claim"`
		Title                  string              `json:"title"`
		Message                string              `json:"message"`
		Description            string              `json:"description"`
		Body                   string              `json:"body"`
		Category               string              `json:"category"`
		Severity               string              `json:"severity"`
		Confidence             json.RawMessage     `json:"confidence"`
		Locations              []CandidateLocation `json:"locations"`
		Location               *CandidateLocation  `json:"location"`
		Path                   string              `json:"path"`
		File                   string              `json:"file"`
		Filename               string              `json:"filename"`
		StartLine              json.RawMessage     `json:"start_line"`
		StartLineAlt           json.RawMessage     `json:"startLine"`
		Line                   json.RawMessage     `json:"line"`
		EndLine                json.RawMessage     `json:"end_line"`
		EndLineAlt             json.RawMessage     `json:"endLine"`
		PrimaryPath            string              `json:"primary_path"`
		PrimaryStartLine       int64               `json:"primary_start_line"`
		PrimaryEndLine         int64               `json:"primary_end_line"`
		Evidence               json.RawMessage     `json:"evidence"`
		CounterEvidenceRequest string              `json:"counter_evidence_request"`
		SuggestedFix           string              `json:"suggested_fix"`
		SuggestedFixAlt        string              `json:"suggestedFix"`
		Recommendation         string              `json:"recommendation"`
		DraftComment           string              `json:"draft_comment"`
		Fingerprint            string              `json:"fingerprint"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	claim := firstNonEmpty(raw.Claim, raw.Title, raw.Message, raw.Description, raw.Body)
	category := firstNonEmpty(raw.Category, inferCandidateCategory(claim, raw.Description, raw.Body, raw.Message, raw.Recommendation))
	topPath := firstNonEmpty(raw.Path, raw.File, raw.Filename)
	topStartLine := firstNonZeroInt64(
		int64FromRaw(raw.StartLine),
		int64FromRaw(raw.StartLineAlt),
		int64FromRaw(raw.Line),
	)
	topEndLine := firstNonZeroInt64(
		int64FromRaw(raw.EndLine),
		int64FromRaw(raw.EndLineAlt),
		int64FromRaw(raw.Line),
		topStartLine,
	)
	*c = Candidate{
		SchemaVersion:          raw.SchemaVersion,
		Claim:                  claim,
		Category:               category,
		Severity:               raw.Severity,
		Confidence:             confidenceFromRaw(raw.Confidence, confidenceFromSeverity(raw.Severity)),
		Locations:              raw.Locations,
		PrimaryPath:            firstNonEmpty(raw.PrimaryPath, topPath),
		PrimaryStartLine:       firstNonZeroInt64(raw.PrimaryStartLine, topStartLine),
		PrimaryEndLine:         firstNonZeroInt64(raw.PrimaryEndLine, topEndLine),
		CounterEvidenceRequest: raw.CounterEvidenceRequest,
		SuggestedFix:           firstNonEmpty(raw.SuggestedFix, raw.SuggestedFixAlt, raw.Recommendation),
		DraftComment:           raw.DraftComment,
		Fingerprint:            raw.Fingerprint,
	}
	if raw.Location != nil {
		c.Locations = append(c.Locations, *raw.Location)
	}
	if topPath != "" && topStartLine > 0 && len(c.Locations) == 0 {
		c.Locations = append(c.Locations, CandidateLocation{
			Path:      topPath,
			StartLine: topStartLine,
			EndLine:   topEndLine,
			Side:      "RIGHT",
		})
	}
	evidence, err := evidenceFromRaw(raw.Evidence)
	if err != nil {
		return err
	}
	if len(evidence) == 0 {
		evidence = synthesizedEvidence(claim, raw.Description, raw.Body, raw.Message, raw.Recommendation, topPath, topStartLine, topEndLine)
	}
	c.Evidence = evidence
	return nil
}

type CandidateLocation struct {
	Path          string `json:"path"`
	StartLine     int64  `json:"start_line"`
	EndLine       int64  `json:"end_line"`
	Side          string `json:"side"`
	ChangedFileID string `json:"changed_file_id,omitempty"`
	Valid         *bool  `json:"valid,omitempty"`
	Message       string `json:"message,omitempty"`
}

func (l *CandidateLocation) UnmarshalJSON(data []byte) error {
	var raw struct {
		Path          string          `json:"path"`
		File          string          `json:"file"`
		Filename      string          `json:"filename"`
		StartLine     json.RawMessage `json:"start_line"`
		StartLineAlt  json.RawMessage `json:"startLine"`
		EndLine       json.RawMessage `json:"end_line"`
		EndLineAlt    json.RawMessage `json:"endLine"`
		Line          json.RawMessage `json:"line"`
		Side          string          `json:"side"`
		ChangedFileID string          `json:"changed_file_id"`
		Valid         *bool           `json:"valid"`
		Message       string          `json:"message"`
		Snippet       string          `json:"snippet"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	startLine := firstNonZeroInt64(
		int64FromRaw(raw.StartLine),
		int64FromRaw(raw.StartLineAlt),
		int64FromRaw(raw.Line),
	)
	endLine := firstNonZeroInt64(
		int64FromRaw(raw.EndLine),
		int64FromRaw(raw.EndLineAlt),
		int64FromRaw(raw.Line),
		startLine,
	)
	*l = CandidateLocation{
		Path:          firstNonEmpty(raw.Path, raw.File, raw.Filename),
		StartLine:     startLine,
		EndLine:       endLine,
		Side:          firstNonEmpty(raw.Side, "RIGHT"),
		ChangedFileID: raw.ChangedFileID,
		Valid:         raw.Valid,
		Message:       firstNonEmpty(raw.Message, raw.Snippet),
	}
	return nil
}

type CandidateEvidence struct {
	Title      string  `json:"title"`
	Summary    string  `json:"summary"`
	Kind       string  `json:"kind"`
	Path       string  `json:"path,omitempty"`
	StartLine  int64   `json:"start_line,omitempty"`
	EndLine    int64   `json:"end_line,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

func (e *CandidateEvidence) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*e = CandidateEvidence{
			Title:   "Agent evidence",
			Summary: text,
			Kind:    "unknown",
		}
		return nil
	}
	var raw struct {
		Title       string          `json:"title"`
		Summary     string          `json:"summary"`
		Message     string          `json:"message"`
		Description string          `json:"description"`
		Kind        string          `json:"kind"`
		Path        string          `json:"path"`
		File        string          `json:"file"`
		StartLine   json.RawMessage `json:"start_line"`
		StartLineA  json.RawMessage `json:"startLine"`
		Line        json.RawMessage `json:"line"`
		EndLine     json.RawMessage `json:"end_line"`
		EndLineAlt  json.RawMessage `json:"endLine"`
		Confidence  json.RawMessage `json:"confidence"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	startLine := firstNonZeroInt64(
		int64FromRaw(raw.StartLine),
		int64FromRaw(raw.StartLineA),
		int64FromRaw(raw.Line),
	)
	endLine := firstNonZeroInt64(
		int64FromRaw(raw.EndLine),
		int64FromRaw(raw.EndLineAlt),
		int64FromRaw(raw.Line),
		startLine,
	)
	*e = CandidateEvidence{
		Title:      firstNonEmpty(raw.Title, "Agent evidence"),
		Summary:    firstNonEmpty(raw.Summary, raw.Description, raw.Message, raw.Title),
		Kind:       raw.Kind,
		Path:       firstNonEmpty(raw.Path, raw.File),
		StartLine:  startLine,
		EndLine:    endLine,
		Confidence: confidenceFromRaw(raw.Confidence, 0),
	}
	return nil
}

func ExtractCandidates(parsed ParsedOutput) CandidateParseResult {
	result := CandidateParseResult{
		Diagnostics: append([]Diagnostic(nil), parsed.Diagnostics...),
	}
	if !parsed.Structured {
		if repaired, diagnostics := repairMalformedStructuredOutput(parsed.Text); repaired.Structured {
			result.Diagnostics = append(result.Diagnostics, diagnostics...)
			for index, document := range repaired.Documents {
				candidates, diagnostics := candidatesFromDocument(document, index+1)
				result.Candidates = append(result.Candidates, candidates...)
				result.Diagnostics = append(result.Diagnostics, diagnostics...)
			}
			return result
		}
		candidates, diagnostics := candidatesFromText(parsed.Text)
		result.Candidates = append(result.Candidates, candidates...)
		result.Diagnostics = append(result.Diagnostics, diagnostics...)
		return result
	}
	for index, document := range parsed.Documents {
		candidates, diagnostics := candidatesFromDocument(document, index+1)
		result.Candidates = append(result.Candidates, candidates...)
		result.Diagnostics = append(result.Diagnostics, diagnostics...)
	}
	return result
}

func repairMalformedStructuredOutput(text string) (ParsedOutput, []Diagnostic) {
	trimmed := strings.TrimSpace(stripJSONFence(text))
	if trimmed == "" || (!strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[")) {
		return ParsedOutput{}, nil
	}
	repaired := trailingJSONCommaRE.ReplaceAllString(trimmed, "$1")
	if repaired == trimmed {
		return ParsedOutput{}, nil
	}
	parsed := parseJSON([]byte(repaired))
	if !parsed.Structured {
		return ParsedOutput{}, nil
	}
	return parsed, []Diagnostic{{
		Code:    "repaired_json",
		Message: "removed trailing commas from malformed structured output",
	}}
}

func stripJSONFence(text string) string {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 2 {
		return trimmed
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return trimmed
	}
	return strings.Join(lines[1:len(lines)-1], "\n")
}

func candidatesFromText(text string) ([]Candidate, []Diagnostic) {
	fields, firstLine := textFields(text)
	claim := firstNonEmpty(fields["claim"], fields["finding"], firstLine)
	if claim == "" {
		return nil, []Diagnostic{{Code: "empty_text_output", Message: "text output does not contain a finding"}}
	}

	candidate := Candidate{
		SchemaVersion: CandidateSchemaVersion,
		Claim:         claim,
		Category:      textCategory(fields["category"]),
		Severity:      textSeverity(fields["severity"]),
		Confidence:    textConfidence(fields["confidence"], 0.35),
		Evidence: []CandidateEvidence{{
			Title:   firstNonEmpty(fields["evidence"], "Raw text finding"),
			Summary: firstNonEmpty(fields["evidence"], summarizeText(text)),
			Kind:    "unknown",
		}},
		SuggestedFix: fields["suggested_fix"],
		DraftComment: fields["draft_comment"],
	}
	if location, ok := textLocation(fields["location"]); ok {
		candidate.Locations = []CandidateLocation{location}
	}
	candidate = normalizeCandidate(candidate)
	if diagnostics := validateCandidate(candidate, 1, 1); len(diagnostics) > 0 {
		return nil, diagnostics
	}
	return []Candidate{candidate}, []Diagnostic{{
		Code:    "text_output_normalized",
		Message: "converted text output into a low-confidence finding candidate",
	}}
}

func textFields(text string) (map[string]string, string) {
	fields := map[string]string{}
	firstLine := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "-*0123456789. "))
		if line == "" {
			continue
		}
		if firstLine == "" {
			firstLine = line
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		key = strings.ReplaceAll(key, " ", "_")
		value = strings.TrimSpace(value)
		switch key {
		case "claim", "finding", "category", "severity", "confidence", "location", "evidence":
			fields[key] = value
		case "suggested_fix", "suggested_fix_direction", "fix":
			fields["suggested_fix"] = value
		case "draft_comment", "comment":
			fields["draft_comment"] = value
		}
	}
	return fields, firstLine
}

func textCategory(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if knownCategory(value) {
		return value
	}
	switch value {
	case "bug", "logic", "correctness issue":
		return "correctness"
	case "test", "tests":
		return "testing"
	case "perf":
		return "performance"
	case "maintainability issue":
		return "maintainability"
	default:
		return "other"
	}
}

func textSeverity(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if knownSeverity(value) {
		return value
	}
	switch value {
	case "critical":
		return "blocker"
	case "major":
		return "high"
	case "minor", "info":
		return "low"
	default:
		return "low"
	}
}

func textConfidence(value string, fallback float64) float64 {
	value = strings.TrimSuffix(strings.TrimSpace(value), "%")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	if parsed > 1 && parsed <= 100 {
		parsed = parsed / 100
	}
	if parsed < 0 || parsed > 1 {
		return fallback
	}
	return parsed
}

func confidenceFromRaw(raw json.RawMessage, fallback float64) float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return fallback
	}
	var numeric float64
	if err := json.Unmarshal(raw, &numeric); err == nil {
		if numeric >= 0 && numeric <= 1 {
			return numeric
		}
		if numeric >= 10 && numeric <= 100 {
			return numeric / 100
		}
		return numeric
	}
	var label string
	if err := json.Unmarshal(raw, &label); err != nil {
		return fallback
	}
	normalized := strings.ToLower(strings.TrimSpace(label))
	switch normalized {
	case "very high":
		return 0.95
	case "high":
		return 0.85
	case "medium", "moderate":
		return 0.6
	case "low":
		return 0.35
	default:
		return textConfidence(normalized, fallback)
	}
}

func confidenceFromSeverity(severity string) float64 {
	switch textSeverity(severity) {
	case "blocker":
		return 0.9
	case "high":
		return 0.78
	case "medium":
		return 0.62
	case "low":
		return 0.45
	case "nit":
		return 0.35
	default:
		return 0.55
	}
}

func inferCandidateCategory(values ...string) string {
	joined := strings.ToLower(strings.Join(values, " "))
	switch {
	case strings.Contains(joined, "security") ||
		strings.Contains(joined, "auth") ||
		strings.Contains(joined, "authorization") ||
		strings.Contains(joined, "permission") ||
		strings.Contains(joined, "privilege") ||
		strings.Contains(joined, "admin"):
		return "security"
	case strings.Contains(joined, "test") || strings.Contains(joined, "coverage"):
		return "testing"
	case strings.Contains(joined, "latency") ||
		strings.Contains(joined, "performance") ||
		strings.Contains(joined, "slow") ||
		strings.Contains(joined, "allocation"):
		return "performance"
	case strings.Contains(joined, "race") ||
		strings.Contains(joined, "flaky") ||
		strings.Contains(joined, "retry") ||
		strings.Contains(joined, "timeout") ||
		strings.Contains(joined, "stale"):
		return "reliability"
	case strings.Contains(joined, "api") ||
		strings.Contains(joined, "contract") ||
		strings.Contains(joined, "backward compatible"):
		return "api"
	case strings.Contains(joined, "doc") || strings.Contains(joined, "readme"):
		return "docs"
	case strings.Contains(joined, "style") || strings.Contains(joined, "format"):
		return "style"
	case strings.Contains(joined, "maintainability") ||
		strings.Contains(joined, "readability") ||
		strings.Contains(joined, "complexity") ||
		strings.Contains(joined, "type safety"):
		return "maintainability"
	case strings.TrimSpace(joined) != "":
		return "correctness"
	default:
		return ""
	}
}

func synthesizedEvidence(claim string, description string, body string, message string, recommendation string, path string, startLine int64, endLine int64) []CandidateEvidence {
	summary := firstNonEmpty(description, body, message, recommendation, claim)
	if strings.TrimSpace(summary) == "" {
		return nil
	}
	return []CandidateEvidence{{
		Title:     firstNonEmpty(claim, "Agent evidence"),
		Summary:   summary,
		Kind:      "unknown",
		Path:      path,
		StartLine: startLine,
		EndLine:   firstNonZeroInt64(endLine, startLine),
	}}
}

func evidenceFromRaw(raw json.RawMessage) ([]CandidateEvidence, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, nil
		}
		return []CandidateEvidence{{
			Title:   "Agent evidence",
			Summary: text,
			Kind:    "unknown",
		}}, nil
	}
	var list []CandidateEvidence
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var item CandidateEvidence
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, err
	}
	return []CandidateEvidence{item}, nil
}

func int64FromRaw(raw json.RawMessage) int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var integer int64
	if err := json.Unmarshal(raw, &integer); err == nil {
		return integer
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		return int64(number)
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0
	}
	parsed, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(text), "L"), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func textLocation(value string) (CandidateLocation, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return CandidateLocation{}, false
	}
	path, lineSpec, ok := strings.Cut(value, ":")
	if !ok {
		return CandidateLocation{}, false
	}
	path = strings.TrimSpace(path)
	lineSpec = strings.TrimPrefix(strings.TrimSpace(lineSpec), "L")
	startSpec, endSpec, hasEnd := strings.Cut(lineSpec, "-")
	start, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(startSpec), "L"), 10, 64)
	if err != nil {
		return CandidateLocation{}, false
	}
	end := start
	if hasEnd {
		end, err = strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(endSpec), "L"), 10, 64)
		if err != nil {
			return CandidateLocation{}, false
		}
	}
	return CandidateLocation{
		Path:      path,
		StartLine: start,
		EndLine:   end,
		Side:      "RIGHT",
	}, true
}

func summarizeText(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	summary := strings.Join(fields, " ")
	if len(summary) <= 280 {
		return summary
	}
	return strings.TrimSpace(summary[:280]) + "..."
}

type structuredDocument struct {
	Summary    string          `json:"summary"`
	Claim      string          `json:"claim"`
	Findings   json.RawMessage `json:"findings"`
	Finding    json.RawMessage `json:"finding"`
	Candidate  json.RawMessage `json:"candidate"`
	Response   json.RawMessage `json:"response"`
	Result     json.RawMessage `json:"result"`
	Item       json.RawMessage `json:"item"`
	Part       json.RawMessage `json:"part"`
	Type       string          `json:"type"`
	Event      string          `json:"event"`
	SchemaName string          `json:"schema_version"`
}

func candidatesFromDocument(document json.RawMessage, documentIndex int) ([]Candidate, []Diagnostic) {
	var envelope structuredDocument
	if err := json.Unmarshal(document, &envelope); err != nil {
		return nil, []Diagnostic{documentDiagnostic(documentIndex, "invalid_candidate_json", err.Error())}
	}

	switch {
	case len(envelope.Findings) > 0:
		return candidatesFromFindings(envelope.Findings, documentIndex)
	case len(envelope.Finding) > 0:
		return candidateFromRaw(envelope.Finding, documentIndex, 1)
	case len(envelope.Candidate) > 0:
		return candidateFromRaw(envelope.Candidate, documentIndex, 1)
	case looksLikeCandidate(envelope):
		return candidateFromRaw(document, documentIndex, 1)
	case len(envelope.Response) > 0:
		return candidatesFromWrappedText(envelope.Response, documentIndex, "response")
	case len(envelope.Result) > 0:
		return candidatesFromWrappedText(envelope.Result, documentIndex, "result")
	case len(envelope.Item) > 0:
		return candidatesFromWrappedObjectText(envelope.Item, documentIndex, "item")
	case len(envelope.Part) > 0:
		return candidatesFromWrappedObjectText(envelope.Part, documentIndex, "part")
	default:
		return nil, nil
	}
}

func candidatesFromWrappedObjectText(raw json.RawMessage, documentIndex int, field string) ([]Candidate, []Diagnostic) {
	var wrapper struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, []Diagnostic{documentDiagnostic(documentIndex, "invalid_"+field+"_wrapper", err.Error())}
	}
	if strings.TrimSpace(wrapper.Text) == "" {
		return nil, nil
	}
	return extractCandidatesFromWrappedText(wrapper.Text)
}

func candidatesFromWrappedText(raw json.RawMessage, documentIndex int, field string) ([]Candidate, []Diagnostic) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, []Diagnostic{documentDiagnostic(documentIndex, "invalid_"+field+"_text", err.Error())}
	}
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	return extractCandidatesFromWrappedText(text)
}

func extractCandidatesFromWrappedText(text string) ([]Candidate, []Diagnostic) {
	for _, candidateText := range candidateStructuredTexts(text) {
		parsed := ParseAuto([]byte(candidateText))
		if parsed.Structured {
			extracted := ExtractCandidates(parsed)
			return extracted.Candidates, extracted.Diagnostics
		}
		if repaired, diagnostics := repairMalformedStructuredOutput(candidateText); repaired.Structured {
			extracted := ExtractCandidates(repaired)
			return extracted.Candidates, append(diagnostics, extracted.Diagnostics...)
		}
	}
	return nil, nil
}

func candidateStructuredTexts(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	candidates := []string{}
	for _, match := range jsonFenceRE.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			candidates = append(candidates, strings.TrimSpace(match[1]))
		}
	}
	if len(candidates) > 0 {
		return candidates
	}
	return []string{trimmed}
}

func candidatesFromFindings(raw json.RawMessage, documentIndex int) ([]Candidate, []Diagnostic) {
	var findings []json.RawMessage
	if err := json.Unmarshal(raw, &findings); err != nil {
		return nil, []Diagnostic{documentDiagnostic(documentIndex, "invalid_findings_array", err.Error())}
	}
	candidates := make([]Candidate, 0, len(findings))
	diagnostics := []Diagnostic{}
	for index, finding := range findings {
		found, nextDiagnostics := candidateFromRaw(finding, documentIndex, index+1)
		candidates = append(candidates, found...)
		diagnostics = append(diagnostics, nextDiagnostics...)
	}
	return candidates, diagnostics
}

func candidateFromRaw(raw json.RawMessage, documentIndex int, findingIndex int) ([]Candidate, []Diagnostic) {
	var candidate Candidate
	if err := json.Unmarshal(raw, &candidate); err != nil {
		return nil, []Diagnostic{findingDiagnostic(documentIndex, findingIndex, "invalid_finding", err.Error())}
	}
	candidate = normalizeCandidate(candidate)
	if diagnostics := validateCandidate(candidate, documentIndex, findingIndex); len(diagnostics) > 0 {
		return nil, diagnostics
	}
	return []Candidate{candidate}, nil
}

func looksLikeCandidate(envelope structuredDocument) bool {
	return envelope.SchemaName == CandidateSchemaVersion ||
		strings.TrimSpace(envelope.Claim) != ""
}

func normalizeCandidate(candidate Candidate) Candidate {
	candidate.SchemaVersion = firstNonEmpty(candidate.SchemaVersion, CandidateSchemaVersion)
	candidate.Claim = strings.TrimSpace(candidate.Claim)
	candidate.Category = strings.TrimSpace(candidate.Category)
	if candidate.Category != "" {
		candidate.Category = textCategory(candidate.Category)
	}
	candidate.Severity = strings.TrimSpace(candidate.Severity)
	if candidate.Severity != "" {
		candidate.Severity = textSeverity(candidate.Severity)
	}
	candidate.CounterEvidenceRequest = strings.TrimSpace(candidate.CounterEvidenceRequest)
	candidate.SuggestedFix = strings.TrimSpace(candidate.SuggestedFix)
	candidate.DraftComment = strings.TrimSpace(candidate.DraftComment)
	candidate.Fingerprint = strings.TrimSpace(candidate.Fingerprint)

	for index := range candidate.Locations {
		location := &candidate.Locations[index]
		location.Path = strings.TrimSpace(location.Path)
		location.Side = strings.ToUpper(strings.TrimSpace(location.Side))
		if location.Side == "" {
			location.Side = "UNKNOWN"
		}
		location.ChangedFileID = strings.TrimSpace(location.ChangedFileID)
		location.Message = strings.TrimSpace(location.Message)
	}
	if candidate.PrimaryPath == "" && len(candidate.Locations) > 0 {
		candidate.PrimaryPath = candidate.Locations[0].Path
		candidate.PrimaryStartLine = candidate.Locations[0].StartLine
		candidate.PrimaryEndLine = candidate.Locations[0].EndLine
	}
	candidate.PrimaryPath = strings.TrimSpace(candidate.PrimaryPath)

	for index := range candidate.Evidence {
		evidence := &candidate.Evidence[index]
		evidence.Title = strings.TrimSpace(evidence.Title)
		evidence.Summary = strings.TrimSpace(evidence.Summary)
		evidence.Kind = strings.ToLower(firstNonEmpty(strings.TrimSpace(evidence.Kind), "unknown"))
		evidence.Path = strings.TrimSpace(evidence.Path)
	}
	return candidate
}

func validateCandidate(candidate Candidate, documentIndex int, findingIndex int) []Diagnostic {
	diagnostics := []Diagnostic{}
	if candidate.SchemaVersion != CandidateSchemaVersion {
		diagnostics = append(diagnostics, findingDiagnostic(documentIndex, findingIndex, "unsupported_candidate_schema", "candidate schema_version must be finding-candidate/v1"))
	}
	if candidate.Claim == "" {
		diagnostics = append(diagnostics, findingDiagnostic(documentIndex, findingIndex, "missing_claim", "claim is required"))
	}
	if candidate.Category == "" {
		diagnostics = append(diagnostics, findingDiagnostic(documentIndex, findingIndex, "missing_category", "category is required"))
	} else if !knownCategory(candidate.Category) {
		diagnostics = append(diagnostics, findingDiagnostic(documentIndex, findingIndex, "invalid_category", "category is not supported"))
	}
	if candidate.Severity == "" {
		diagnostics = append(diagnostics, findingDiagnostic(documentIndex, findingIndex, "missing_severity", "severity is required"))
	} else if !knownSeverity(candidate.Severity) {
		diagnostics = append(diagnostics, findingDiagnostic(documentIndex, findingIndex, "invalid_severity", "severity is not supported"))
	}
	if candidate.Confidence < 0 || candidate.Confidence > 1 {
		diagnostics = append(diagnostics, findingDiagnostic(documentIndex, findingIndex, "invalid_confidence", "confidence must be between 0 and 1"))
	}
	for index, location := range candidate.Locations {
		if location.Path == "" {
			diagnostics = append(diagnostics, findingDiagnostic(documentIndex, findingIndex, "missing_location_path", fmt.Sprintf("locations[%d].path is required", index)))
		}
		if location.StartLine < 1 || location.EndLine < 1 || location.EndLine < location.StartLine {
			diagnostics = append(diagnostics, findingDiagnostic(documentIndex, findingIndex, "invalid_location_range", fmt.Sprintf("locations[%d] has invalid line range", index)))
		}
		if !knownSide(location.Side) {
			diagnostics = append(diagnostics, findingDiagnostic(documentIndex, findingIndex, "invalid_location_side", fmt.Sprintf("locations[%d].side is not supported", index)))
		}
	}
	for index, evidence := range candidate.Evidence {
		if evidence.Title == "" || evidence.Summary == "" {
			diagnostics = append(diagnostics, findingDiagnostic(documentIndex, findingIndex, "invalid_evidence", fmt.Sprintf("evidence[%d] requires title and summary", index)))
		}
		if !knownEvidenceKind(evidence.Kind) {
			diagnostics = append(diagnostics, findingDiagnostic(documentIndex, findingIndex, "invalid_evidence_kind", fmt.Sprintf("evidence[%d].kind is not supported", index)))
		}
	}
	return diagnostics
}

func knownCategory(category string) bool {
	switch category {
	case "security", "correctness", "testing", "reliability", "performance", "maintainability", "api", "docs", "style", "other":
		return true
	default:
		return false
	}
}

func knownSeverity(severity string) bool {
	switch severity {
	case "blocker", "high", "medium", "low", "nit":
		return true
	default:
		return false
	}
}

func knownSide(side string) bool {
	switch side {
	case "RIGHT", "LEFT", "UNKNOWN":
		return true
	default:
		return false
	}
}

func knownEvidenceKind(kind string) bool {
	switch kind {
	case "changed_code", "related_code", "middleware", "guard", "handler", "test", "config", "counter_evidence", "missing_guard", "unknown":
		return true
	default:
		return false
	}
}

func documentDiagnostic(documentIndex int, code string, message string) Diagnostic {
	return Diagnostic{
		Code:    code,
		Message: fmt.Sprintf("document %d: %s", documentIndex, message),
	}
}

func findingDiagnostic(documentIndex int, findingIndex int, code string, message string) Diagnostic {
	return Diagnostic{
		Code:    code,
		Message: fmt.Sprintf("document %d finding %d: %s", documentIndex, findingIndex, message),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
