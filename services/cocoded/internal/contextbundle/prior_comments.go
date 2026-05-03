package contextbundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hughdo/cocode/services/cocoded/internal/githubpr"
)

const (
	defaultPriorCommentMaxBytes int64 = 4 * 1024
	defaultPriorCommentMaxItems       = 40
)

type PriorCommentOptions struct {
	BundleID                   string
	PreviousCommentsArtifactID string
	MaxCommentBytes            int64
	MaxItems                   int
}

func BuildPriorCommentContextItemsFromJSON(options PriorCommentOptions, content []byte) ([]Item, error) {
	if len(strings.TrimSpace(string(content))) == 0 {
		return nil, nil
	}
	var comments githubpr.PreviousComments
	if err := json.Unmarshal(content, &comments); err != nil {
		return nil, fmt.Errorf("decode previous comments artifact: %w", err)
	}
	return BuildPriorCommentContextItems(options, comments)
}

func BuildPriorCommentContextItems(options PriorCommentOptions, comments githubpr.PreviousComments) ([]Item, error) {
	options = normalizePriorCommentOptions(options)
	if strings.TrimSpace(options.BundleID) == "" {
		return nil, errors.New("context bundle id is required")
	}
	if len(comments.Comments) == 0 {
		return nil, nil
	}

	ordered := append([]githubpr.PreviousComment(nil), comments.Comments...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := priorCommentTime(ordered[i])
		right := priorCommentTime(ordered[j])
		if left != right {
			return left > right
		}
		if ordered[i].Source != ordered[j].Source {
			return ordered[i].Source < ordered[j].Source
		}
		return ordered[i].ID > ordered[j].ID
	})

	items := make([]Item, 0, min(len(ordered), options.MaxItems))
	for _, comment := range ordered {
		if len(items) >= options.MaxItems {
			break
		}
		item, ok, err := priorCommentContextItem(options, comment)
		if err != nil {
			return nil, err
		}
		if ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func normalizePriorCommentOptions(options PriorCommentOptions) PriorCommentOptions {
	if options.MaxCommentBytes <= 0 {
		options.MaxCommentBytes = defaultPriorCommentMaxBytes
	}
	if options.MaxItems <= 0 {
		options.MaxItems = defaultPriorCommentMaxItems
	}
	return options
}

func priorCommentContextItem(options PriorCommentOptions, comment githubpr.PreviousComment) (Item, bool, error) {
	if strings.TrimSpace(comment.Body) == "" && strings.TrimSpace(comment.DiffHunk) == "" {
		return Item{}, false, nil
	}
	path, startLine, endLine := priorCommentLocation(comment)
	content, truncated := boundedPriorCommentContent(renderPriorComment(comment), options.MaxCommentBytes)
	if strings.TrimSpace(content) == "" {
		return Item{}, false, nil
	}
	metadata, err := priorCommentMetadata(options, comment, truncated, int64(len(content)))
	if err != nil {
		return Item{}, false, err
	}
	item := Item{
		ID:              stableContextItemID(options.BundleID, priorCommentStableKey(comment), 0),
		ContextBundleID: options.BundleID,
		Kind:            ItemPriorComment,
		Path:            path,
		StartLine:       startLine,
		EndLine:         endLine,
		Title:           priorCommentTitle(comment),
		Content:         content,
		TokenEstimate:   estimateTokens(content),
		Metadata:        metadata,
	}
	if err := item.Validate(); err != nil {
		return Item{}, false, err
	}
	return item, true, nil
}

func priorCommentLocation(comment githubpr.PreviousComment) (string, int64, int64) {
	path, ok := cleanSearchMatchPath(comment.Path)
	if !ok {
		return "", 0, 0
	}
	endLine := comment.Line
	if endLine <= 0 {
		endLine = comment.OriginalLine
	}
	startLine := comment.StartLine
	if startLine <= 0 {
		startLine = comment.OriginalStartLine
	}
	if endLine <= 0 {
		return path, 0, 0
	}
	if startLine <= 0 || startLine > endLine {
		startLine = endLine
	}
	return path, startLine, endLine
}

func renderPriorComment(comment githubpr.PreviousComment) string {
	var builder strings.Builder
	source := strings.TrimSpace(comment.Source)
	if source == "" {
		source = "comment"
	}
	builder.WriteString("Source: ")
	builder.WriteString(source)
	if comment.ID > 0 {
		builder.WriteString(" #")
		builder.WriteString(strconv.FormatInt(comment.ID, 10))
	}
	if strings.TrimSpace(comment.Author) != "" {
		builder.WriteString(" by ")
		builder.WriteString(strings.TrimSpace(comment.Author))
	}
	if timestamp := priorCommentTime(comment); timestamp != "" {
		builder.WriteString(" at ")
		builder.WriteString(timestamp)
	}
	builder.WriteByte('\n')
	if strings.TrimSpace(comment.State) != "" {
		builder.WriteString("State: ")
		builder.WriteString(strings.TrimSpace(comment.State))
		builder.WriteByte('\n')
	}
	if strings.TrimSpace(comment.HTMLURL) != "" {
		builder.WriteString("URL: ")
		builder.WriteString(strings.TrimSpace(comment.HTMLURL))
		builder.WriteByte('\n')
	}
	if strings.TrimSpace(comment.Path) != "" {
		builder.WriteString("Path: ")
		builder.WriteString(strings.TrimSpace(comment.Path))
		if comment.Line > 0 {
			builder.WriteByte(':')
			builder.WriteString(strconv.FormatInt(comment.Line, 10))
		}
		builder.WriteByte('\n')
	}
	if strings.TrimSpace(comment.Body) != "" {
		builder.WriteString("\nBody:\n")
		builder.WriteString(strings.TrimSpace(comment.Body))
		builder.WriteByte('\n')
	}
	if strings.TrimSpace(comment.DiffHunk) != "" {
		builder.WriteString("\nDiff hunk:\n")
		builder.WriteString(strings.TrimSpace(comment.DiffHunk))
		builder.WriteByte('\n')
	}
	return builder.String()
}

func boundedPriorCommentContent(content string, maxBytes int64) (string, bool) {
	if maxBytes <= 0 {
		return "", true
	}
	if int64(len(content)) <= maxBytes {
		return content, false
	}
	cut := []byte(content)
	cut = cut[:maxBytes]
	for len(cut) > 0 && !utf8.Valid(cut) {
		cut = cut[:len(cut)-1]
	}
	return string(cut), true
}

func priorCommentTitle(comment githubpr.PreviousComment) string {
	source := strings.TrimSpace(comment.Source)
	if source == "" {
		source = "comment"
	}
	if strings.TrimSpace(comment.Path) != "" {
		return fmt.Sprintf("Prior %s on %s", source, comment.Path)
	}
	return fmt.Sprintf("Prior %s", source)
}

func priorCommentStableKey(comment githubpr.PreviousComment) string {
	return fmt.Sprintf("prior_comment\x00%s\x00%d\x00%d\x00%s", comment.Source, comment.ID, comment.ReviewID, comment.Path)
}

func priorCommentTime(comment githubpr.PreviousComment) string {
	for _, value := range []string{comment.UpdatedAt, comment.SubmittedAt, comment.CreatedAt} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func priorCommentMetadata(options PriorCommentOptions, comment githubpr.PreviousComment, truncated bool, contentBytes int64) (json.RawMessage, error) {
	payload := map[string]any{
		"source":                        "previous_pr_comment",
		"previous_comments_artifact_id": options.PreviousCommentsArtifactID,
		"comment_source":                comment.Source,
		"comment_id":                    comment.ID,
		"review_id":                     comment.ReviewID,
		"author":                        comment.Author,
		"author_association":            comment.AuthorAssociation,
		"state":                         comment.State,
		"html_url":                      comment.HTMLURL,
		"path":                          comment.Path,
		"line":                          comment.Line,
		"original_line":                 comment.OriginalLine,
		"start_line":                    comment.StartLine,
		"original_start_line":           comment.OriginalStartLine,
		"side":                          comment.Side,
		"start_side":                    comment.StartSide,
		"in_reply_to_id":                comment.InReplyToID,
		"created_at":                    comment.CreatedAt,
		"updated_at":                    comment.UpdatedAt,
		"submitted_at":                  comment.SubmittedAt,
		"truncated":                     truncated,
		"content_bytes":                 contentBytes,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode prior comment metadata: %w", err)
	}
	return data, nil
}
