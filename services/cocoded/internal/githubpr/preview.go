package githubpr

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/diffparse"
)

type PreviewFinding struct {
	ID                 string
	CanonicalClaim     string
	Category           string
	Severity           string
	VerificationStatus string
	DecisionStatus     string
	PrimaryPath        string
	PrimaryStartLine   int64
	PrimaryEndLine     int64
	PrimarySide        string
	SuggestedFix       string
	DraftComment       string
}

type ReviewPreviewInput struct {
	Title    string
	Findings []PreviewFinding
	Diff     []diffparse.File
}

type ReviewPreview struct {
	Body         string               `json:"body"`
	Comments     []ReviewCommentDraft `json:"comments"`
	Warnings     []AnchorWarning      `json:"warnings"`
	CommentCount int                  `json:"comment_count"`
}

type ReviewCommentDraft struct {
	FindingID  string `json:"finding_id"`
	Path       string `json:"path,omitempty"`
	Body       string `json:"body"`
	Line       int64  `json:"line,omitempty"`
	Side       string `json:"side,omitempty"`
	Position   int    `json:"position,omitempty"`
	Unanchored bool   `json:"unanchored"`
	Warning    string `json:"warning,omitempty"`
}

type AnchorWarning struct {
	FindingID string `json:"finding_id"`
	Path      string `json:"path,omitempty"`
	Line      int64  `json:"line,omitempty"`
	Message   string `json:"message"`
}

func BuildReviewPreview(input ReviewPreviewInput) (ReviewPreview, error) {
	if len(input.Findings) == 0 {
		return ReviewPreview{}, fmt.Errorf("%w: at least one finding is required", ErrInvalidDiffAnchor)
	}
	comments := make([]ReviewCommentDraft, 0, len(input.Findings))
	warnings := make([]AnchorWarning, 0)
	for _, finding := range input.Findings {
		comment := ReviewCommentDraft{
			FindingID: finding.ID,
			Body:      previewCommentBody(finding),
		}
		if strings.TrimSpace(finding.PrimaryPath) == "" || finding.PrimaryStartLine < 1 {
			message := previewMissingAnchorWarning(finding)
			comment.Path = finding.PrimaryPath
			comment.Unanchored = true
			comment.Warning = message
			warnings = append(warnings, AnchorWarning{
				FindingID: finding.ID,
				Path:      finding.PrimaryPath,
				Message:   message,
			})
			comments = append(comments, comment)
			continue
		}
		anchor, err := MapDiffLine(input.Diff, DiffAnchorRequest{
			Path: finding.PrimaryPath,
			Line: int(finding.PrimaryStartLine),
			Side: firstNonEmpty(finding.PrimarySide, SideUnknown),
		})
		if err != nil {
			message := previewAnchorWarning(finding, err)
			comment.Path = finding.PrimaryPath
			comment.Line = finding.PrimaryStartLine
			comment.Unanchored = true
			comment.Warning = message
			warnings = append(warnings, AnchorWarning{
				FindingID: finding.ID,
				Path:      finding.PrimaryPath,
				Line:      finding.PrimaryStartLine,
				Message:   message,
			})
		} else {
			comment.Path = anchor.Path
			comment.Line = int64(anchor.Line)
			comment.Side = anchor.Side
			comment.Position = anchor.Position
		}
		comments = append(comments, comment)
	}
	return ReviewPreview{
		Body:         previewBody(input),
		Comments:     comments,
		Warnings:     warnings,
		CommentCount: len(comments),
	}, nil
}

func ReviewPreviewCommentsJSON(preview ReviewPreview) (string, error) {
	encoded, err := json.Marshal(preview.Comments)
	if err != nil {
		return "", fmt.Errorf("encode review comments: %w", err)
	}
	return string(encoded), nil
}

func previewBody(input ReviewPreviewInput) string {
	var builder strings.Builder
	if strings.TrimSpace(input.Title) != "" {
		builder.WriteString(strings.TrimSpace(input.Title))
	} else {
		builder.WriteString("cocode review preview")
	}
	builder.WriteString("\n\n")
	builder.WriteString("Selected findings:\n")
	for index, finding := range input.Findings {
		builder.WriteString(fmt.Sprintf("%d. [%s] %s", index+1, firstNonEmpty(finding.Severity, "severity unknown"), oneLine(finding.CanonicalClaim)))
		if finding.PrimaryPath != "" {
			builder.WriteString(" (")
			builder.WriteString(finding.PrimaryPath)
			if finding.PrimaryStartLine > 0 {
				builder.WriteString(fmt.Sprintf(":%d", finding.PrimaryStartLine))
			}
			builder.WriteString(")")
		}
		builder.WriteByte('\n')
	}
	return strings.TrimSpace(builder.String()) + "\n"
}

func previewCommentBody(finding PreviewFinding) string {
	if strings.TrimSpace(finding.DraftComment) != "" {
		return strings.TrimSpace(finding.DraftComment)
	}
	var builder strings.Builder
	builder.WriteString(finding.CanonicalClaim)
	if strings.TrimSpace(finding.SuggestedFix) != "" {
		builder.WriteString("\n\nSuggested fix: ")
		builder.WriteString(strings.TrimSpace(finding.SuggestedFix))
	}
	return strings.TrimSpace(builder.String())
}

func previewMissingAnchorWarning(finding PreviewFinding) string {
	path := strings.TrimSpace(finding.PrimaryPath)
	if path == "" {
		return fmt.Sprintf("Finding %s has no changed-file location; it will be included in the summary only.", finding.ID)
	}
	return fmt.Sprintf("Finding %s at %s has no positive line number; it will be included in the summary only.", finding.ID, path)
}

func previewAnchorWarning(finding PreviewFinding, err error) string {
	location := strings.TrimSpace(finding.PrimaryPath)
	if finding.PrimaryStartLine > 0 {
		location = fmt.Sprintf("%s:%d", location, finding.PrimaryStartLine)
	}
	if location == "" {
		location = "unknown location"
	}
	return fmt.Sprintf("Could not anchor finding %s at %s: %v", finding.ID, location, err)
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
