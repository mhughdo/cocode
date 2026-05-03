package contextbundle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRelatedMaxItems          = 50
	defaultRelatedMaxMatchesPerTerm = 20
	defaultRipgrepTimeout           = 5 * time.Second
)

type RelatedCodeSearchOptions struct {
	BundleID          string
	RepoRoot          string
	Searcher          CodeSearcher
	MaxItems          int
	MaxMatchesPerTerm int
}

type RelatedSearchInput struct {
	ChangedFileID string
	Path          string
	Symbols       []string
	Excluded      bool
	Binary        bool
}

type CodeSearchMatch struct {
	Path string
	Line int64
	Text string
}

type CodeSearcher interface {
	Search(ctx context.Context, root string, term string, limit int) ([]CodeSearchMatch, error)
}

type RipgrepSearcher struct {
	Command string
	Timeout time.Duration
}

func BuildRelatedCodeContextItems(ctx context.Context, options RelatedCodeSearchOptions, inputs []RelatedSearchInput) ([]Item, error) {
	options = normalizeRelatedCodeSearchOptions(options)
	if strings.TrimSpace(options.BundleID) == "" {
		return nil, errors.New("context bundle id is required")
	}
	root, err := safeRepoRoot(options.RepoRoot)
	if err != nil {
		return nil, err
	}

	items := []Item{}
	seen := map[string]struct{}{}
	for _, input := range inputs {
		if input.Excluded || input.Binary {
			continue
		}
		terms := relatedSearchTerms(input)
		for _, term := range terms {
			if len(items) >= options.MaxItems {
				return items, nil
			}
			matches, err := options.Searcher.Search(ctx, root, term, options.MaxMatchesPerTerm)
			if err != nil {
				return nil, fmt.Errorf("search related code for %q: %w", term, err)
			}
			for _, match := range matches {
				if len(items) >= options.MaxItems {
					return items, nil
				}
				path, ok := cleanSearchMatchPath(match.Path)
				if !ok || path == input.Path || match.Line <= 0 {
					continue
				}
				key := path + ":" + strconv.FormatInt(match.Line, 10)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				metadata, err := relatedCodeMetadata(input, term)
				if err != nil {
					return nil, err
				}
				content := fmt.Sprintf("%d: %s", match.Line, strings.TrimRight(match.Text, "\r\n"))
				item := Item{
					ID:              stableFileContextItemID(options.BundleID, path, ItemRelatedCode, match.Line, match.Line),
					ContextBundleID: options.BundleID,
					Kind:            ItemRelatedCode,
					Path:            path,
					StartLine:       match.Line,
					EndLine:         match.Line,
					Title:           fmt.Sprintf("%s reference to %s", path, term),
					Content:         content,
					TokenEstimate:   estimateTokens(content),
					Metadata:        metadata,
				}
				if err := item.Validate(); err != nil {
					return nil, err
				}
				items = append(items, item)
			}
		}
	}
	return items, nil
}

func (s RipgrepSearcher) Search(ctx context.Context, root string, term string, limit int) ([]CodeSearchMatch, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, nil
	}
	root, err := safeRepoRoot(root)
	if err != nil {
		return nil, err
	}
	command := strings.TrimSpace(s.Command)
	if command == "" {
		command = "rg"
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = defaultRipgrepTimeout
	}
	if limit <= 0 {
		limit = defaultRelatedMaxMatchesPerTerm
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, command,
		"--json",
		"--line-number",
		"--color", "never",
		"--fixed-strings",
		"--max-count", strconv.Itoa(limit),
		"--max-filesize", "256K",
		"--glob", "!.git/**",
		"--", term, ".",
	)
	cmd.Dir = root
	output, err := cmd.Output()
	if runCtx.Err() != nil {
		return nil, runCtx.Err()
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	return parseRipgrepJSON(output, limit)
}

func normalizeRelatedCodeSearchOptions(options RelatedCodeSearchOptions) RelatedCodeSearchOptions {
	if options.Searcher == nil {
		options.Searcher = RipgrepSearcher{}
	}
	if options.MaxItems <= 0 {
		options.MaxItems = defaultRelatedMaxItems
	}
	if options.MaxMatchesPerTerm <= 0 {
		options.MaxMatchesPerTerm = defaultRelatedMaxMatchesPerTerm
	}
	return options
}

func relatedSearchTerms(input RelatedSearchInput) []string {
	terms := make([]string, 0, len(input.Symbols)+2)
	for _, symbol := range input.Symbols {
		addSearchTerm(&terms, symbol)
	}
	base := filepath.Base(filepath.ToSlash(input.Path))
	extension := filepath.Ext(base)
	if extension != "" {
		base = strings.TrimSuffix(base, extension)
	}
	addSearchTerm(&terms, base)
	return terms
}

func addSearchTerm(terms *[]string, value string) {
	value = strings.TrimSpace(value)
	if len(value) < 3 || slices.Contains(*terms, value) {
		return
	}
	*terms = append(*terms, value)
}

func relatedCodeMetadata(input RelatedSearchInput, term string) (json.RawMessage, error) {
	payload := map[string]any{
		"changed_file_id": input.ChangedFileID,
		"source_path":     input.Path,
		"search_term":     term,
		"source":          "related_code_search",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode related code metadata: %w", err)
	}
	return data, nil
}

func cleanSearchMatchPath(path string) (string, bool) {
	path = strings.TrimSpace(filepath.ToSlash(path))
	path = strings.TrimPrefix(path, "./")
	if path == "" || strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
		return "", false
	}
	return path, true
}

type ripgrepEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int64 `json:"line_number"`
	} `json:"data"`
}

func parseRipgrepJSON(output []byte, limit int) ([]CodeSearchMatch, error) {
	lines := bytes.Split(output, []byte{'\n'})
	matches := make([]CodeSearchMatch, 0, min(len(lines), limit))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var event ripgrepEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("parse ripgrep json: %w", err)
		}
		if event.Type != "match" {
			continue
		}
		path, ok := cleanSearchMatchPath(event.Data.Path.Text)
		if !ok {
			continue
		}
		matches = append(matches, CodeSearchMatch{
			Path: path,
			Line: event.Data.LineNumber,
			Text: event.Data.Lines.Text,
		})
		if limit > 0 && len(matches) >= limit {
			break
		}
	}
	return matches, nil
}
