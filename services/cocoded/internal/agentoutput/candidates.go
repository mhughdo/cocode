package agentoutput

import (
	"encoding/json"
	"fmt"
	"strings"
)

const CandidateSchemaVersion = "finding-candidate/v1"

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
		return result
	}
	for index, document := range parsed.Documents {
		candidates, diagnostics := candidatesFromDocument(document, index+1)
		result.Candidates = append(result.Candidates, candidates...)
		result.Diagnostics = append(result.Diagnostics, diagnostics...)
	}
	return result
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
