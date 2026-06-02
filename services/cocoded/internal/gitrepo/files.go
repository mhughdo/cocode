package gitrepo

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/security"
)

const defaultFileSearchLimit = 20
const maxFileSearchLimit = 50
const defaultFileTreeLimit = 2000
const maxFileTreeLimit = 5000

type FileMatch struct {
	Path      string
	Name      string
	Directory string
	Kind      string
	Score     int
}

func (c Collector) SearchFiles(ctx context.Context, selectedPath string, query string, limit int) ([]FileMatch, error) {
	runner := c.runner()
	info, err := validate(ctx, runner, selectedPath)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultFileSearchLimit
	}
	if limit > maxFileSearchLimit {
		limit = maxFileSearchLimit
	}

	paths, err := listRepositoryFilePaths(ctx, runner, info.RootPath)
	if err != nil {
		return nil, err
	}
	matches := make([]FileMatch, 0, min(len(paths), limit))
	for _, path := range paths {
		score, ok := scoreFileMatch(path, query)
		if !ok {
			continue
		}
		name := filepath.Base(filepath.FromSlash(path))
		directory := strings.TrimSuffix(filepath.ToSlash(filepath.Dir(path)), ".")
		matches = append(matches, FileMatch{
			Path:      path,
			Name:      name,
			Directory: strings.Trim(directory, "/"),
			Kind:      "file",
			Score:     score,
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		left := matches[i]
		right := matches[j]
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if len(left.Path) != len(right.Path) {
			return len(left.Path) < len(right.Path)
		}
		return strings.ToLower(left.Path) < strings.ToLower(right.Path)
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}

func (c Collector) ListFiles(ctx context.Context, selectedPath string, limit int) ([]FileMatch, bool, error) {
	runner := c.runner()
	info, err := validate(ctx, runner, selectedPath)
	if err != nil {
		return nil, false, err
	}
	if limit <= 0 {
		limit = defaultFileTreeLimit
	}
	if limit > maxFileTreeLimit {
		limit = maxFileTreeLimit
	}

	paths, err := listRepositoryFilePaths(ctx, runner, info.RootPath)
	if err != nil {
		return nil, false, err
	}
	sort.Slice(paths, func(i, j int) bool {
		return strings.ToLower(paths[i]) < strings.ToLower(paths[j])
	})
	truncated := len(paths) > limit
	if truncated {
		paths = paths[:limit]
	}
	files := make([]FileMatch, 0, len(paths))
	for _, path := range paths {
		name := filepath.Base(filepath.FromSlash(path))
		directory := strings.TrimSuffix(filepath.ToSlash(filepath.Dir(path)), ".")
		files = append(files, FileMatch{
			Path:      path,
			Name:      name,
			Directory: strings.Trim(directory, "/"),
			Kind:      "file",
			Score:     defaultFileMatchScore(path),
		})
	}
	return files, truncated, nil
}

func listRepositoryFilePaths(ctx context.Context, runner Runner, rootPath string) ([]string, error) {
	result, err := runner.RunRaw(ctx, rootPath, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	return parseGitFileList(result.Stdout), nil
}

func parseGitFileList(output string) []string {
	seen := map[string]struct{}{}
	paths := []string{}
	for _, raw := range strings.Split(output, "\x00") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		clean, err := security.CleanRelativePath(raw)
		if err != nil || clean == "." {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		paths = append(paths, clean)
	}
	return paths
}

func scoreFileMatch(path string, query string) (int, bool) {
	query = normalizeFileSearchQuery(query)
	if query == "" {
		return defaultFileMatchScore(path), true
	}
	pathLower := strings.ToLower(path)
	baseLower := strings.ToLower(filepath.Base(filepath.FromSlash(path)))
	if pathLower == query || baseLower == query {
		return 4000 - len(path), true
	}
	if strings.HasPrefix(baseLower, query) {
		return 3400 - len(path), true
	}
	if strings.HasPrefix(pathLower, query) {
		return 3000 - len(path), true
	}
	if strings.Contains(baseLower, query) {
		return 2600 - len(path), true
	}
	if strings.Contains(pathLower, query) {
		return 2200 - len(path), true
	}
	if score, ok := fuzzyFileScore(pathLower, query); ok {
		return 1200 + score - len(path), true
	}
	return 0, false
}

func normalizeFileSearchQuery(query string) string {
	query = strings.TrimSpace(strings.TrimPrefix(query, "@"))
	query = strings.ReplaceAll(query, "\\", "/")
	return strings.ToLower(query)
}

func defaultFileMatchScore(path string) int {
	lower := strings.ToLower(path)
	base := filepath.Base(filepath.FromSlash(lower))
	score := 1000 - len(path)
	switch base {
	case "agents.md":
		score += 700
	case "readme.md":
		score += 600
	case "contributing.md", "design.md":
		score += 450
	}
	if strings.HasPrefix(lower, "docs/") {
		score += 300
	}
	if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".mdx") {
		score += 250
	}
	if strings.Contains(lower, "test") || strings.Contains(lower, "spec") {
		score -= 60
	}
	return score
}

func fuzzyFileScore(text string, query string) (int, bool) {
	if query == "" {
		return 0, true
	}
	score := 0
	textIndex := 0
	previous := -2
	for _, q := range query {
		found := -1
		for textIndex < len(text) {
			if rune(text[textIndex]) == q {
				found = textIndex
				textIndex++
				break
			}
			textIndex++
		}
		if found < 0 {
			return 0, false
		}
		score += 20
		if found == previous+1 {
			score += 15
		}
		if found == 0 || text[found-1] == '/' || text[found-1] == '-' || text[found-1] == '_' || text[found-1] == '.' {
			score += 20
		}
		previous = found
	}
	return score, true
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
