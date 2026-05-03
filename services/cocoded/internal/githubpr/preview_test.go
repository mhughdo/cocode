package githubpr

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildReviewPreviewAnchorsCommentsAndEmitsBody(t *testing.T) {
	files := mustParseDiff(t, mapperFixtureDiff)
	preview, err := BuildReviewPreview(ReviewPreviewInput{
		Title: "cocode findings",
		Diff:  files,
		Findings: []PreviewFinding{
			{
				ID:                 "finding_auth",
				CanonicalClaim:     "Repository route misses authorization guard.",
				Category:           "security",
				Severity:           "high",
				VerificationStatus: "verified",
				DecisionStatus:     "accepted",
				PrimaryPath:        "app/auth.go",
				PrimaryStartLine:   12,
				DraftComment:       "Please require admin permissions before updateSettings.",
			},
			{
				ID:                 "finding_new",
				CanonicalClaim:     "NewThing needs validation.",
				Severity:           "medium",
				VerificationStatus: "plausible",
				DecisionStatus:     "accepted",
				PrimaryPath:        "app/new.go",
				PrimaryStartLine:   2,
				SuggestedFix:       "Validate constructor inputs.",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildReviewPreview() error = %v", err)
	}
	if preview.CommentCount != 2 ||
		len(preview.Comments) != 2 ||
		len(preview.Warnings) != 0 ||
		!strings.Contains(preview.Body, "cocode findings") ||
		!strings.Contains(preview.Body, "Repository route misses authorization guard") {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.Comments[0].Path != "app/auth.go" ||
		preview.Comments[0].Line != 12 ||
		preview.Comments[0].Side != SideRight ||
		preview.Comments[0].Position != 3 ||
		preview.Comments[0].Body != "Please require admin permissions before updateSettings." {
		t.Fatalf("first comment = %+v", preview.Comments[0])
	}
	if !strings.Contains(preview.Comments[1].Body, "Suggested fix: Validate constructor inputs.") {
		t.Fatalf("second comment = %+v", preview.Comments[1])
	}
	commentsJSON, err := ReviewPreviewCommentsJSON(preview)
	if err != nil {
		t.Fatalf("ReviewPreviewCommentsJSON() error = %v", err)
	}
	var decoded []ReviewCommentDraft
	if err := json.Unmarshal([]byte(commentsJSON), &decoded); err != nil {
		t.Fatalf("comments JSON did not parse: %v", err)
	}
	if len(decoded) != 2 || decoded[0].FindingID != "finding_auth" {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestBuildReviewPreviewKeepsUnanchoredCommentWithWarning(t *testing.T) {
	files := mustParseDiff(t, mapperFixtureDiff)
	preview, err := BuildReviewPreview(ReviewPreviewInput{
		Diff: files,
		Findings: []PreviewFinding{{
			ID:               "finding_missing",
			CanonicalClaim:   "Missing file cannot be anchored.",
			Severity:         "low",
			PrimaryPath:      "app/missing.go",
			PrimaryStartLine: 10,
		}},
	})
	if err != nil {
		t.Fatalf("BuildReviewPreview() error = %v", err)
	}
	if len(preview.Comments) != 1 ||
		!preview.Comments[0].Unanchored ||
		preview.Comments[0].Warning == "" ||
		len(preview.Warnings) != 1 ||
		preview.Warnings[0].FindingID != "finding_missing" {
		t.Fatalf("preview = %+v", preview)
	}
}
