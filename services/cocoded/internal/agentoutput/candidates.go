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

type CandidateLocation struct {
	Path          string `json:"path"`
	StartLine     int64  `json:"start_line"`
	EndLine       int64  `json:"end_line"`
	Side          string `json:"side"`
	ChangedFileID string `json:"changed_file_id,omitempty"`
	Valid         *bool  `json:"valid,omitempty"`
	Message       string `json:"message,omitempty"`
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
	default:
		return nil, nil
	}
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
	candidate.Severity = strings.TrimSpace(candidate.Severity)
	candidate.CounterEvidenceRequest = strings.TrimSpace(candidate.CounterEvidenceRequest)
	candidate.SuggestedFix = strings.TrimSpace(candidate.SuggestedFix)
	candidate.DraftComment = strings.TrimSpace(candidate.DraftComment)
	candidate.Fingerprint = strings.TrimSpace(candidate.Fingerprint)

	for index := range candidate.Locations {
		location := &candidate.Locations[index]
		location.Path = strings.TrimSpace(location.Path)
		location.Side = strings.TrimSpace(location.Side)
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
		evidence.Kind = firstNonEmpty(strings.TrimSpace(evidence.Kind), "unknown")
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
