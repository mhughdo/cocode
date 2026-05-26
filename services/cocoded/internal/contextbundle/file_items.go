package contextbundle

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/diffparse"
)

const (
	defaultFullFileBytes int64 = 16 * 1024
	defaultSliceBytes    int64 = 8 * 1024
	defaultTotalBytes    int64 = 64 * 1024
	defaultContextLines        = 4
	defaultMaxFileItems        = 128
)

type FileContextOptions struct {
	BundleID         string
	RepoRoot         string
	ContextLines     int
	MaxFullFileBytes int64
	MaxSliceBytes    int64
	MaxTotalBytes    int64
	MaxItems         int
}

type ChangedFileContentInput struct {
	ChangedFileID string
	Path          string
	OldPath       string
	Status        string
	Binary        bool
	Generated     bool
	Excluded      bool
	LineRanges    []diffparse.LineRange
}

type lineWindow struct {
	Start int
	End   int
}

func ChangedFileContentInputFromRow(row dbgen.ChangedFile) (ChangedFileContentInput, error) {
	ranges, err := decodeLineRanges(row.LineRangesJson)
	if err != nil {
		return ChangedFileContentInput{}, err
	}
	return ChangedFileContentInput{
		ChangedFileID: row.ID,
		Path:          row.Path,
		OldPath:       nullableString(row.OldPath),
		Status:        row.Status,
		Binary:        row.IsBinary != 0,
		Generated:     row.IsGenerated != 0,
		Excluded:      row.IsExcluded != 0,
		LineRanges:    ranges,
	}, nil
}

func BuildChangedFileContentItems(options FileContextOptions, files []ChangedFileContentInput) ([]Item, error) {
	options = normalizeFileContextOptions(options)
	if strings.TrimSpace(options.BundleID) == "" {
		return nil, errors.New("context bundle id is required")
	}
	root, err := safeRepoRoot(options.RepoRoot)
	if err != nil {
		return nil, err
	}

	items := []Item{}
	var usedBytes int64
	for _, file := range files {
		if file.Excluded || file.Binary || strings.EqualFold(file.Status, string(diffparse.StatusDeleted)) {
			continue
		}
		if options.MaxItems > 0 && len(items) >= options.MaxItems {
			break
		}
		path, err := safeRepoFilePath(root, file.Path)
		if err != nil {
			return nil, err
		}
		stat, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect changed file %s: %w", file.Path, err)
		}
		if stat.IsDir() {
			return nil, fmt.Errorf("changed file %s is a directory", file.Path)
		}

		if stat.Size() <= options.MaxFullFileBytes {
			content, truncated, err := readFullFile(path, remainingBudget(options.MaxTotalBytes, usedBytes))
			if err != nil {
				return nil, fmt.Errorf("read changed file %s: %w", file.Path, err)
			}
			if content == "" {
				continue
			}
			usedBytes += int64(len(content))
			item, err := buildFileContentItem(options.BundleID, file, ItemFullFile, 1, countContentLines(content), content, truncated, "full_file")
			if err != nil {
				return nil, err
			}
			items = append(items, item)
			continue
		}

		windows := buildLineWindows(file.LineRanges, options.ContextLines)
		for _, window := range windows {
			if options.MaxItems > 0 && len(items) >= options.MaxItems {
				break
			}
			budget := minPositive(options.MaxSliceBytes, remainingBudget(options.MaxTotalBytes, usedBytes))
			if budget <= 0 {
				break
			}
			content, truncated, err := readLineWindow(path, window, budget)
			if err != nil {
				return nil, fmt.Errorf("read changed file slice %s:%d-%d: %w", file.Path, window.Start, window.End, err)
			}
			if content == "" {
				continue
			}
			usedBytes += int64(len(content))
			item, err := buildFileContentItem(options.BundleID, file, ItemFileSlice, int64(window.Start), int64(window.End), content, truncated, "file_slice")
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
	}
	return items, nil
}

func BuildFocusFileContextItems(options FileContextOptions, paths []string) ([]Item, []string, error) {
	options = normalizeFileContextOptions(options)
	if strings.TrimSpace(options.BundleID) == "" {
		return nil, nil, errors.New("context bundle id is required")
	}
	root, err := safeRepoRoot(options.RepoRoot)
	if err != nil {
		return nil, nil, err
	}

	items := []Item{}
	warnings := []string{}
	seen := map[string]struct{}{}
	var usedBytes int64
	for _, rawPath := range paths {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		if options.MaxItems > 0 && len(items) >= options.MaxItems {
			break
		}
		budget := minPositive(options.MaxFullFileBytes, remainingBudget(options.MaxTotalBytes, usedBytes))
		if budget <= 0 {
			warnings = appendWarning(warnings, "focus file context budget exhausted")
			break
		}
		absolutePath, err := safeRepoFilePath(root, path)
		if err != nil {
			warnings = appendWarning(warnings, fmt.Sprintf("focus file %s skipped: %v", path, err))
			continue
		}
		stat, err := os.Stat(absolutePath)
		if err != nil {
			warnings = appendWarning(warnings, fmt.Sprintf("focus file %s skipped: %v", path, err))
			continue
		}
		if stat.IsDir() {
			warnings = appendWarning(warnings, fmt.Sprintf("focus file %s skipped: path is a directory", path))
			continue
		}
		content, truncated, err := readFullFile(absolutePath, budget)
		if err != nil {
			warnings = appendWarning(warnings, fmt.Sprintf("focus file %s skipped: %v", path, err))
			continue
		}
		if content == "" {
			continue
		}
		if strings.ContainsRune(content, '\x00') || !utf8.ValidString(content) {
			warnings = appendWarning(warnings, fmt.Sprintf("focus file %s skipped: binary content", path))
			continue
		}
		usedBytes += int64(len(content))
		item, err := buildFileContentItem(options.BundleID, ChangedFileContentInput{
			Path:   path,
			Status: "focus",
		}, ItemFocusFile, 1, countContentLines(content), content, truncated, "focus_file")
		if err != nil {
			return nil, nil, err
		}
		item.Title = fmt.Sprintf("Focus file %s", path)
		items = append(items, item)
	}
	return items, warnings, nil
}

func normalizeFileContextOptions(options FileContextOptions) FileContextOptions {
	if options.ContextLines < 0 {
		options.ContextLines = 0
	}
	if options.ContextLines == 0 {
		options.ContextLines = defaultContextLines
	}
	if options.MaxFullFileBytes <= 0 {
		options.MaxFullFileBytes = defaultFullFileBytes
	}
	if options.MaxSliceBytes <= 0 {
		options.MaxSliceBytes = defaultSliceBytes
	}
	if options.MaxTotalBytes <= 0 {
		options.MaxTotalBytes = defaultTotalBytes
	}
	if options.MaxItems <= 0 {
		options.MaxItems = defaultMaxFileItems
	}
	return options
}

func buildFileContentItem(bundleID string, file ChangedFileContentInput, kind ItemKind, startLine int64, endLine int64, content string, truncated bool, source string) (Item, error) {
	metadata, err := fileContentMetadata(file, source, truncated, int64(len(content)))
	if err != nil {
		return Item{}, err
	}
	title := fmt.Sprintf("%s %s", file.Path, strings.ReplaceAll(string(kind), "_", " "))
	item := Item{
		ID:              stableFileContextItemID(bundleID, file.Path, kind, startLine, endLine),
		ContextBundleID: bundleID,
		Kind:            kind,
		Path:            file.Path,
		StartLine:       startLine,
		EndLine:         endLine,
		Title:           title,
		Content:         content,
		TokenEstimate:   estimateTokens(content),
		Metadata:        metadata,
	}
	if err := item.Validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func fileContentMetadata(file ChangedFileContentInput, source string, truncated bool, byteCount int64) (json.RawMessage, error) {
	payload := map[string]any{
		"changed_file_id": file.ChangedFileID,
		"old_path":        file.OldPath,
		"status":          file.Status,
		"generated":       file.Generated,
		"source":          source,
		"truncated":       truncated,
		"bytes":           byteCount,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode file context metadata: %w", err)
	}
	return data, nil
}

func decodeLineRanges(raw string) ([]diffparse.LineRange, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var pairs [][2]int
	if err := json.Unmarshal([]byte(raw), &pairs); err != nil {
		return nil, fmt.Errorf("decode changed file line ranges: %w", err)
	}
	ranges := make([]diffparse.LineRange, 0, len(pairs))
	for _, pair := range pairs {
		if pair[0] <= 0 || pair[1] < pair[0] {
			return nil, fmt.Errorf("changed file line range %d-%d is invalid", pair[0], pair[1])
		}
		ranges = append(ranges, diffparse.LineRange{Start: pair[0], End: pair[1]})
	}
	return ranges, nil
}

func buildLineWindows(ranges []diffparse.LineRange, contextLines int) []lineWindow {
	windows := make([]lineWindow, 0, len(ranges))
	for _, r := range ranges {
		if r.Start <= 0 || r.End < r.Start {
			continue
		}
		window := lineWindow{
			Start: max(1, r.Start-contextLines),
			End:   r.End + contextLines,
		}
		last := len(windows) - 1
		if last >= 0 && windows[last].End+1 >= window.Start {
			windows[last].End = max(windows[last].End, window.End)
			continue
		}
		windows = append(windows, window)
	}
	return windows
}

func safeRepoRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("repo root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve repo root symlinks: %w", err)
	}
	stat, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect repo root: %w", err)
	}
	if !stat.IsDir() {
		return "", errors.New("repo root must be a directory")
	}
	return resolved, nil
}

func safeRepoFilePath(root string, relativePath string) (string, error) {
	relativePath = strings.TrimSpace(relativePath)
	if relativePath == "" || filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("changed file path %q is unsafe", relativePath)
	}
	clean := filepath.Clean(filepath.FromSlash(relativePath))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("changed file path %q escapes repo root", relativePath)
	}
	target := filepath.Join(root, clean)
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("changed file path %q escapes repo root", relativePath)
	}
	return resolved, nil
}

func readFullFile(path string, maxBytes int64) (string, bool, error) {
	if maxBytes <= 0 {
		return "", true, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", false, err
	}
	if int64(len(content)) > maxBytes {
		return string(content[:maxBytes]), true, nil
	}
	return string(content), false, nil
}

func readLineWindow(path string, window lineWindow, maxBytes int64) (string, bool, error) {
	if maxBytes <= 0 || window.Start <= 0 || window.End < window.Start {
		return "", true, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var builder strings.Builder
	var lineNumber int
	truncated := false
	for {
		line, readErr := reader.ReadString('\n')
		if line != "" {
			lineNumber++
			if lineNumber >= window.Start && lineNumber <= window.End {
				prefix := fmt.Sprintf("%d: ", lineNumber)
				if !writeBounded(&builder, prefix+line, maxBytes) {
					truncated = true
					break
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return "", false, readErr
		}
		if lineNumber >= window.End {
			break
		}
	}
	return builder.String(), truncated, nil
}

func writeBounded(builder *strings.Builder, value string, maxBytes int64) bool {
	remaining := int(maxBytes) - builder.Len()
	if remaining <= 0 {
		return false
	}
	if len(value) > remaining {
		builder.WriteString(value[:remaining])
		return false
	}
	builder.WriteString(value)
	return true
}

func remainingBudget(maxBytes int64, usedBytes int64) int64 {
	if maxBytes <= 0 {
		return 0
	}
	remaining := maxBytes - usedBytes
	if remaining < 0 {
		return 0
	}
	return remaining
}

func minPositive(a int64, b int64) int64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	if a < b {
		return a
	}
	return b
}

func countContentLines(content string) int64 {
	if content == "" {
		return 0
	}
	lines := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		lines++
	}
	return int64(lines)
}

func estimateTokens(content string) int64 {
	return EstimateContentTokens(content)
}

func stableFileContextItemID(bundleID string, path string, kind ItemKind, startLine int64, endLine int64) string {
	return stableContextItemID(bundleID, fmt.Sprintf("%s\x00%s\x00%d\x00%d", path, kind, startLine, endLine), 0)
}
