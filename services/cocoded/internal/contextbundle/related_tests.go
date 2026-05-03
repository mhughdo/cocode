package contextbundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	defaultMaxTestFileBytes int64 = 64 * 1024
	defaultMaxTestItems           = 50
)

type RelatedTestOptions struct {
	BundleID         string
	RepoRoot         string
	MaxTestFileBytes int64
	MaxItems         int
}

type RelatedTestInput struct {
	ChangedFileID string
	Path          string
	Symbols       []string
	Excluded      bool
	Binary        bool
}

func BuildRelatedTestContextItems(options RelatedTestOptions, inputs []RelatedTestInput) ([]Item, error) {
	options = normalizeRelatedTestOptions(options)
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
		path, ok := cleanSearchMatchPath(input.Path)
		if !ok {
			return nil, fmt.Errorf("changed file path %q is unsafe", input.Path)
		}
		foundForInput := false
		candidates := candidateTestPaths(path)
		for _, candidate := range candidates {
			if len(items) >= options.MaxItems {
				return items, nil
			}
			item, ok, err := relatedTestItemFromPath(options, root, input, candidate, "path_candidate")
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			if _, exists := seen[item.Path]; exists {
				foundForInput = true
				continue
			}
			seen[item.Path] = struct{}{}
			items = append(items, item)
			foundForInput = true
		}

		if len(items) < options.MaxItems {
			referenceOptions := options
			referenceOptions.MaxItems = options.MaxItems - len(items)
			walked, found, err := discoverReferencedTests(referenceOptions, root, input, seen)
			if err != nil {
				return nil, err
			}
			if len(walked) > 0 {
				items = append(items, walked...)
			}
			if found {
				foundForInput = true
			}
		}
		if !foundForInput && len(items) < options.MaxItems {
			item, err := missingRelatedTestItem(options.BundleID, input)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
	}
	return items, nil
}

func normalizeRelatedTestOptions(options RelatedTestOptions) RelatedTestOptions {
	if options.MaxTestFileBytes <= 0 {
		options.MaxTestFileBytes = defaultMaxTestFileBytes
	}
	if options.MaxItems <= 0 {
		options.MaxItems = defaultMaxTestItems
	}
	return options
}

func candidateTestPaths(path string) []string {
	dir := filepath.ToSlash(filepath.Dir(path))
	if dir == "." {
		dir = ""
	}
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	join := func(parts ...string) string {
		values := []string{}
		if dir != "" {
			values = append(values, dir)
		}
		values = append(values, parts...)
		return filepath.ToSlash(filepath.Join(values...))
	}

	candidates := []string{}
	switch ext {
	case ".go":
		candidates = append(candidates, join(stem+"_test.go"))
	case ".ts", ".tsx", ".js", ".jsx":
		extensions := []string{ext}
		if ext == ".ts" {
			extensions = append(extensions, ".tsx")
		}
		if ext == ".js" {
			extensions = append(extensions, ".jsx")
		}
		for _, testExt := range extensions {
			candidates = append(candidates,
				join(stem+".test"+testExt),
				join(stem+".spec"+testExt),
				join("__tests__", stem+".test"+testExt),
				join("__tests__", stem+".spec"+testExt),
			)
		}
	case ".py":
		candidates = append(candidates,
			join("test_"+stem+".py"),
			join(stem+"_test.py"),
			filepath.ToSlash(filepath.Join("tests", "test_"+stem+".py")),
		)
	}
	return dedupeStrings(candidates)
}

func relatedTestItemFromPath(options RelatedTestOptions, root string, input RelatedTestInput, candidate string, source string) (Item, bool, error) {
	path, err := safeRepoFilePath(root, candidate)
	if err != nil {
		if os.IsNotExist(err) {
			return Item{}, false, nil
		}
		return Item{}, false, err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return Item{}, false, err
	}
	if stat.IsDir() || stat.Size() > options.MaxTestFileBytes {
		return Item{}, false, nil
	}
	content, truncated, err := readFullFile(path, options.MaxTestFileBytes)
	if err != nil {
		return Item{}, false, err
	}
	if content == "" {
		return Item{}, false, nil
	}
	return buildRelatedTestItem(options.BundleID, input, candidate, content, truncated, source)
}

func discoverReferencedTests(options RelatedTestOptions, root string, input RelatedTestInput, seen map[string]struct{}) ([]Item, bool, error) {
	terms := relatedTestTerms(input)
	if len(terms) == 0 {
		return nil, false, nil
	}
	sourcePath, _ := cleanSearchMatchPath(input.Path)
	items := []Item{}
	found := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if len(items) >= options.MaxItems {
			return filepath.SkipAll
		}
		if path == root {
			return nil
		}
		relativePath, ok := repoRelativePath(root, path)
		if !ok {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if shouldSkipTestDiscoveryDir(relativePath) {
				return filepath.SkipDir
			}
			return nil
		}
		if !looksLikeTestPath(relativePath) {
			return nil
		}
		if sourcePath != "" && relativePath == sourcePath {
			return nil
		}
		stat, err := entry.Info()
		if err != nil || stat.Size() > options.MaxTestFileBytes {
			return nil
		}
		content, truncated, err := readFullFile(path, options.MaxTestFileBytes)
		if err != nil || !containsAnyTerm(content, terms) {
			return nil
		}
		found = true
		if _, exists := seen[relativePath]; exists {
			return nil
		}
		item, ok, err := buildRelatedTestItem(options.BundleID, input, relativePath, content, truncated, "reference_search")
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		seen[relativePath] = struct{}{}
		items = append(items, item)
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return items, found, nil
}

func buildRelatedTestItem(bundleID string, input RelatedTestInput, path string, content string, truncated bool, source string) (Item, bool, error) {
	metadata, err := relatedTestMetadata(input, source, truncated, false)
	if err != nil {
		return Item{}, false, err
	}
	item := Item{
		ID:              stableFileContextItemID(bundleID, path, ItemRelatedTest, 1, countContentLines(content)),
		ContextBundleID: bundleID,
		Kind:            ItemRelatedTest,
		Path:            path,
		StartLine:       1,
		EndLine:         countContentLines(content),
		Title:           fmt.Sprintf("%s related test", path),
		Content:         content,
		TokenEstimate:   estimateTokens(content),
		Metadata:        metadata,
	}
	if err := item.Validate(); err != nil {
		return Item{}, false, err
	}
	return item, true, nil
}

func missingRelatedTestItem(bundleID string, input RelatedTestInput) (Item, error) {
	content := fmt.Sprintf("No related test file was found for %s.", input.Path)
	metadata, err := relatedTestMetadata(input, "missing", false, true)
	if err != nil {
		return Item{}, err
	}
	item := Item{
		ID:              stableFileContextItemID(bundleID, input.Path, ItemRelatedTest, 0, 0),
		ContextBundleID: bundleID,
		Kind:            ItemRelatedTest,
		Title:           "No related tests found",
		Content:         content,
		TokenEstimate:   estimateTokens(content),
		Metadata:        metadata,
	}
	if err := item.Validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func relatedTestMetadata(input RelatedTestInput, source string, truncated bool, missing bool) (json.RawMessage, error) {
	payload := map[string]any{
		"changed_file_id": input.ChangedFileID,
		"source_path":     input.Path,
		"source":          source,
		"truncated":       truncated,
		"missing":         missing,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode related test metadata: %w", err)
	}
	return data, nil
}

func relatedTestTerms(input RelatedTestInput) []string {
	terms := []string{}
	sourcePath := filepath.ToSlash(input.Path)
	base := filepath.Base(sourcePath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	pathWithoutExt := strings.TrimSuffix(sourcePath, ext)
	addSearchTerm(&terms, pathWithoutExt)
	addSearchTerm(&terms, "/"+stem)
	addSearchTerm(&terms, "\\"+stem)
	for _, symbol := range input.Symbols {
		addSearchTerm(&terms, symbol)
	}
	return terms
}

func containsAnyTerm(content string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(content, term) {
			return true
		}
	}
	return false
}

func repoRelativePath(root string, path string) (string, bool) {
	relativePath, err := filepath.Rel(root, path)
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(relativePath), true
}

func shouldSkipTestDiscoveryDir(path string) bool {
	switch filepath.Base(path) {
	case ".git", "node_modules", "vendor", "dist", "build", "coverage", ".next":
		return true
	default:
		return false
	}
}

func looksLikeTestPath(path string) bool {
	path = filepath.ToSlash(path)
	base := filepath.Base(path)
	if strings.Contains(path, "/__tests__/") || strings.Contains(path, "/tests/") || strings.HasPrefix(path, "tests/") {
		return true
	}
	return strings.Contains(base, "_test.") ||
		strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.") ||
		strings.HasPrefix(base, "test_")
}

func dedupeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || slices.Contains(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}
