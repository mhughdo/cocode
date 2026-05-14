package agentoutput

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

type Answer struct {
	Content          string
	ReasoningSummary string
	EvidenceRefs     json.RawMessage
}

func ExtractAnswer(parsed ParsedOutput) Answer {
	answer := Answer{EvidenceRefs: json.RawMessage("[]")}
	for _, raw := range parsed.Documents {
		content, refs := answerFromDocument(raw)
		reasoning := reasoningFromValue(raw)
		if content != "" {
			answer.Content = content
		}
		if reasoning != "" {
			answer.ReasoningSummary = mergeReasoning(answer.ReasoningSummary, reasoning)
		}
		if len(refs) > 0 {
			answer.EvidenceRefs = refs
		}
	}
	if strings.TrimSpace(answer.Content) == "" && parsed.Structured {
		if structured := structuredFindingsSummary(parsed); structured != "" {
			answer.Content = structured
		}
	}
	if parsed.Structured {
		if structured := structuredFindingsSummary(parsed); structured != "" && shouldPreferStructuredFindings(answer.Content) {
			answer.Content = structured
		}
	}
	if strings.TrimSpace(answer.Content) == "" && !parsed.Structured {
		answer.Content = strings.TrimSpace(parsed.Text)
	}
	return answer
}

func shouldPreferStructuredFindings(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return true
	}
	return !looksLikeFindingsAnswer(content) && !looksLikeReviewResultAnswer(content)
}

func looksLikeFindingsAnswer(content string) bool {
	content = strings.ToLower(strings.TrimSpace(content))
	return strings.Contains(content, "## findings") ||
		strings.Contains(content, "findings (") ||
		strings.Contains(content, "- **severity:**") ||
		strings.Contains(content, "severity:") && strings.Contains(content, "location:")
}

func looksLikeReviewResultAnswer(content string) bool {
	content = strings.ToLower(strings.TrimSpace(content))
	if content == "" {
		return false
	}
	if strings.Contains(content, "no findings") ||
		strings.Contains(content, "no actionable findings") ||
		strings.Contains(content, "no issues found") {
		return true
	}
	return strings.Contains(content, "found") &&
		(strings.Contains(content, "finding") ||
			strings.Contains(content, "issue") ||
			strings.Contains(content, "bug") ||
			strings.Contains(content, "defect"))
}

func structuredFindingsSummary(parsed ParsedOutput) string {
	extracted := ExtractCandidates(parsed)
	if len(extracted.Candidates) == 0 {
		return ""
	}
	limit := len(extracted.Candidates)
	if limit > 8 {
		limit = 8
	}
	blocks := make([]string, 0, limit+2)
	blocks = append(blocks, fmt.Sprintf("## Findings (%d)", len(extracted.Candidates)))
	for index := 0; index < limit; index++ {
		blocks = append(blocks, formatCandidateSummary(extracted.Candidates[index], index))
	}
	if omitted := len(extracted.Candidates) - limit; omitted > 0 {
		blocks = append(blocks, fmt.Sprintf("%d more finding%s omitted.", omitted, plural(omitted)))
	}
	return strings.Join(blocks, "\n\n")
}

func formatCandidateSummary(candidate Candidate, index int) string {
	title := firstNonEmpty(
		candidate.Claim,
		candidate.Category,
		fmt.Sprintf("Finding %d", index+1),
	)
	lines := []string{fmt.Sprintf("### %d. %s", index+1, title)}
	if severity := strings.TrimSpace(candidate.Severity); severity != "" {
		lines = append(lines, "- **Severity:** "+severity)
	}
	if category := strings.TrimSpace(candidate.Category); category != "" {
		lines = append(lines, "- **Category:** "+category)
	}
	if location := formatCandidateLocation(candidate); location != "" {
		lines = append(lines, "- **Location:** `"+location+"`")
	}
	if evidence := firstCandidateEvidence(candidate.Evidence); evidence != "" {
		lines = append(lines, "- **Evidence:** "+truncateCandidateText(evidence, 400))
	}
	if fix := strings.TrimSpace(candidate.SuggestedFix); fix != "" {
		lines = append(lines, "- **Suggested fix:** "+truncateCandidateText(fix, 400))
	}
	if comment := strings.TrimSpace(candidate.DraftComment); comment != "" {
		lines = append(lines, "- **Draft comment:** "+truncateCandidateText(comment, 400))
	}
	if request := strings.TrimSpace(candidate.CounterEvidenceRequest); request != "" {
		lines = append(lines, "- **What would disprove it:** "+truncateCandidateText(request, 300))
	}
	return strings.Join(lines, "\n")
}

func formatCandidateLocation(candidate Candidate) string {
	if candidate.PrimaryPath != "" && candidate.PrimaryStartLine > 0 {
		return formatLineRange(candidate.PrimaryPath, candidate.PrimaryStartLine, candidate.PrimaryEndLine)
	}
	for _, location := range candidate.Locations {
		if location.Path == "" || location.StartLine <= 0 {
			continue
		}
		if location.Side != "" && !strings.EqualFold(location.Side, "RIGHT") && !strings.EqualFold(location.Side, "NEW") {
			continue
		}
		return formatLineRange(location.Path, location.StartLine, location.EndLine)
	}
	if len(candidate.Locations) > 0 {
		for _, location := range candidate.Locations {
			if location.Path != "" {
				return location.Path
			}
		}
	}
	if candidate.PrimaryPath != "" {
		return candidate.PrimaryPath
	}
	return ""
}

func formatLineRange(path string, start int64, end int64) string {
	if path == "" || start <= 0 {
		return ""
	}
	if end > start {
		return fmt.Sprintf("%s:%d-%d", path, start, end)
	}
	return fmt.Sprintf("%s:%d", path, start)
}

func firstCandidateEvidence(evidence []CandidateEvidence) string {
	for _, item := range evidence {
		if summary := strings.TrimSpace(item.Summary); summary != "" {
			return summary
		}
		if title := strings.TrimSpace(item.Title); title != "" {
			return title
		}
	}
	return ""
}

func truncateCandidateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	truncated := value[:limit]
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
	return strings.TrimSpace(truncated) + "..."
}

func plural(value int) string {
	if value == 1 {
		return ""
	}
	return "s"
}

type answerDocument struct {
	Answer       json.RawMessage `json:"answer"`
	Content      json.RawMessage `json:"content"`
	Message      json.RawMessage `json:"message"`
	Summary      json.RawMessage `json:"summary"`
	Text         json.RawMessage `json:"text"`
	Output       json.RawMessage `json:"output"`
	Response     json.RawMessage `json:"response"`
	Result       json.RawMessage `json:"result"`
	Item         json.RawMessage `json:"item"`
	Part         json.RawMessage `json:"part"`
	Delta        json.RawMessage `json:"delta"`
	Value        json.RawMessage `json:"value"`
	Event        json.RawMessage `json:"event"`
	EvidenceRefs json.RawMessage `json:"evidence_refs"`
	References   json.RawMessage `json:"references"`
}

func answerFromDocument(raw json.RawMessage) (string, json.RawMessage) {
	return answerFromValue(raw)
}

func answerFromValue(raw json.RawMessage) (string, json.RawMessage) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil
	}
	if text, ok := rawJSONString(raw); ok {
		return answerFromText(text)
	}

	var doc answerDocument
	if err := json.Unmarshal(raw, &doc); err == nil {
		var record map[string]json.RawMessage
		if json.Unmarshal(raw, &record) == nil && isReasoningRecord(record) {
			return "", firstRawJSON(doc.EvidenceRefs, doc.References)
		}
		content := ""
		refs := firstRawJSON(doc.EvidenceRefs, doc.References)
		for _, field := range []json.RawMessage{
			doc.Answer,
			doc.Content,
			doc.Message,
			doc.Summary,
			doc.Text,
			doc.Output,
		} {
			fieldContent, fieldRefs := answerFromValue(field)
			content = firstNonEmpty(content, fieldContent)
			refs = firstRawJSON(refs, fieldRefs)
		}
		for _, field := range []json.RawMessage{
			doc.Response,
			doc.Result,
			doc.Item,
			doc.Part,
			doc.Delta,
			doc.Value,
			doc.Event,
		} {
			fieldContent, fieldRefs := answerFromValue(field)
			content = firstNonEmpty(content, fieldContent)
			refs = firstRawJSON(refs, fieldRefs)
		}
		if strings.TrimSpace(content) != "" || len(refs) > 0 {
			return content, refs
		}
	}

	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err == nil {
		content := ""
		var refs json.RawMessage
		for _, item := range list {
			itemContent, itemRefs := answerFromValue(item)
			content = firstNonEmpty(content, itemContent)
			refs = firstRawJSON(refs, itemRefs)
		}
		return content, refs
	}
	return "", nil
}

func answerFromText(text string) (string, json.RawMessage) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	if content, refs, ok := structuredAnswerFromText(text); ok {
		return content, refs
	}
	if fenced, ok := firstFencedJSON(text); ok {
		if content, refs, ok := structuredAnswerFromText(string(fenced)); ok {
			return content, refs
		}
	}
	return text, nil
}

func structuredAnswerFromText(text string) (string, json.RawMessage, bool) {
	nested := ParseAuto([]byte(text))
	if !nested.Structured {
		return "", nil, false
	}
	answer := ExtractAnswer(nested)
	if strings.TrimSpace(answer.Content) != "" && strings.TrimSpace(answer.Content) != strings.TrimSpace(nested.Text) {
		return answer.Content, answer.EvidenceRefs, true
	}
	if len(bytes.TrimSpace(answer.EvidenceRefs)) > 0 {
		return "", answer.EvidenceRefs, true
	}
	return "", nil, true
}

func firstFencedJSON(text string) ([]byte, bool) {
	searchFrom := 0
	for {
		startOffset := strings.Index(text[searchFrom:], "```")
		if startOffset < 0 {
			return nil, false
		}
		fenceStart := searchFrom + startOffset
		infoStart := fenceStart + 3
		lineEnd := strings.IndexAny(text[infoStart:], "\r\n")
		if lineEnd < 0 {
			return nil, false
		}
		infoEnd := infoStart + lineEnd
		contentStart := infoEnd + 1
		if text[infoEnd] == '\r' && contentStart < len(text) && text[contentStart] == '\n' {
			contentStart++
		}
		endOffset := strings.Index(text[contentStart:], "```")
		if endOffset < 0 {
			return nil, false
		}
		contentEnd := contentStart + endOffset
		info := strings.ToLower(strings.TrimSpace(text[infoStart:infoEnd]))
		content := strings.TrimSpace(text[contentStart:contentEnd])
		payload := []byte(content)
		if content != "" && (info == "" || strings.Contains(info, "json") || json.Valid(payload)) && json.Valid(payload) {
			return payload, true
		}
		searchFrom = contentEnd + 3
	}
}

func reasoningFromValue(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	if text, ok := rawJSONString(raw); ok {
		if nested := ParseAuto([]byte(text)); nested.Structured {
			return ExtractAnswer(nested).ReasoningSummary
		}
		return ""
	}

	var record map[string]json.RawMessage
	if err := json.Unmarshal(raw, &record); err == nil {
		if isReasoningRecord(record) {
			if text := firstReasoningText(record); text != "" {
				return text
			}
		}
		reasoning := ""
		for _, key := range []string{
			"reasoning_summary",
			"reasoningSummary",
			"thinking_summary",
			"thinkingSummary",
			"analysis_summary",
			"analysisSummary",
			"decision_trace",
			"decisionTrace",
			"reasoning",
			"reasoning_text",
			"reasoningText",
			"reasoning_content",
			"reasoningContent",
			"thinking",
			"thought",
			"thoughts",
			"event",
			"item",
			"part",
			"delta",
			"message",
			"content",
			"output",
			"response",
			"result",
			"value",
		} {
			if field, ok := record[key]; ok {
				reasoning = mergeReasoning(reasoning, reasoningFromValue(field))
			}
		}
		return reasoning
	}

	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err == nil {
		reasoning := ""
		for _, item := range list {
			reasoning = mergeReasoning(reasoning, reasoningFromValue(item))
		}
		return reasoning
	}
	return ""
}

func isReasoningRecord(record map[string]json.RawMessage) bool {
	if rawBool, ok := record["thought"]; ok {
		var thought bool
		if json.Unmarshal(rawBool, &thought) == nil && thought {
			return true
		}
	}
	for _, key := range []string{"type", "event", "role"} {
		if raw, ok := record[key]; ok {
			if text, ok := rawJSONString(raw); ok && reasoningType(text) {
				return true
			}
		}
	}
	if delta, ok := record["delta"]; ok {
		if nestedTypeMatches(delta, reasoningType) {
			return true
		}
	}
	if part, ok := record["part"]; ok {
		if nestedTypeMatches(part, reasoningType) {
			return true
		}
	}
	return false
}

func nestedTypeMatches(raw json.RawMessage, match func(string) bool) bool {
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil {
		return false
	}
	for _, key := range []string{"type", "event", "role"} {
		if value, ok := nested[key]; ok {
			if text, ok := rawJSONString(value); ok && match(text) {
				return true
			}
		}
	}
	return false
}

func reasoningType(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(value, "reasoning") ||
		strings.Contains(value, "thinking") ||
		strings.Contains(value, "thought")
}

func firstReasoningText(record map[string]json.RawMessage) string {
	for _, key := range []string{
		"text",
		"content",
		"summary",
		"message",
		"thinking",
		"reasoning",
		"reasoning_text",
		"reasoningText",
		"reasoning_content",
		"reasoningContent",
	} {
		if raw, ok := record[key]; ok {
			if text, ok := rawJSONString(raw); ok && text != "" {
				return text
			}
			if text := reasoningFromValue(raw); text != "" {
				return text
			}
		}
	}
	return ""
}

func mergeReasoning(existing string, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if next == "" || strings.Contains(existing, next) {
		return existing
	}
	if existing == "" {
		return next
	}
	return existing + "\n\n" + next
}

func rawJSONString(raw json.RawMessage) (string, bool) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", false
	}
	return strings.TrimSpace(text), true
}

func firstRawJSON(values ...json.RawMessage) json.RawMessage {
	for _, value := range values {
		if len(bytes.TrimSpace(value)) > 0 {
			return value
		}
	}
	return nil
}
