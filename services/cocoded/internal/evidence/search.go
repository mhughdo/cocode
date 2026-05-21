package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultSearchLimit       = 20
	defaultSearchTimeout     = 5 * time.Second
	defaultSearchOutputLimit = 1 << 20
)

var errSearchOutputLimit = errors.New("search output exceeded limit")

type CodeSearcher interface {
	Search(ctx context.Context, options SearchOptions) ([]SearchMatch, error)
}

type SearchOptions struct {
	RepoRoot    string
	Query       string
	Paths       []string
	ExcludePath []string
	Limit       int
	Timeout     time.Duration
	OutputLimit int64
}

type SearchMatch struct {
	Path string `json:"path"`
	Line int64  `json:"line"`
	Text string `json:"text"`
}

var lookPath = exec.LookPath

type RipgrepSearcher struct {
	Command string
}

type GoSearcher struct{}

func (s RipgrepSearcher) Search(ctx context.Context, options SearchOptions) ([]SearchMatch, error) {
	query := strings.TrimSpace(options.Query)
	if query == "" {
		return nil, nil
	}
	root, err := safeRepoRoot(options.RepoRoot)
	if err != nil {
		return nil, err
	}
	limit := options.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultSearchTimeout
	}
	outputLimit := options.OutputLimit
	if outputLimit <= 0 {
		outputLimit = defaultSearchOutputLimit
	}
	command := strings.TrimSpace(s.Command)
	if command == "" {
		command = "rg"
	}
	if fallback, ok := shouldUseGoSearchFallback(command, s.Command); ok {
		return fallback.Search(ctx, options)
	}
	paths, err := cleanSearchPaths(options.Paths)
	if err != nil {
		return nil, err
	}
	exclude, err := cleanSearchPaths(options.ExcludePath)
	if err != nil {
		return nil, err
	}

	args := []string{
		"--json",
		"--line-number",
		"--color", "never",
		"--fixed-strings",
		"--max-count", strconv.Itoa(limit),
		"--max-filesize", "512K",
		"--glob", "!.git/**",
	}
	for _, path := range exclude {
		if path == "." {
			continue
		}
		args = append(args, "--glob", "!"+path)
	}
	args = append(args, "--", query)
	args = append(args, paths...)

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var remaining = outputLimit
	var exceeded bool
	var mu sync.Mutex
	stdout := &limitedSearchBuffer{remaining: &remaining, exceeded: &exceeded, mu: &mu}
	stderr := &limitedSearchBuffer{remaining: &remaining, exceeded: &exceeded, mu: &mu}
	cmd := exec.CommandContext(runCtx, command, args...)
	cmd.Dir = root
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if runCtx.Err() != nil {
		return nil, runCtx.Err()
	}
	if exceeded || errors.Is(err, errSearchOutputLimit) {
		return nil, errSearchOutputLimit
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("run ripgrep: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parseRipgrepJSON(stdout.Bytes(), limit)
}

func shouldUseGoSearchFallback(command string, configuredCommand string) (GoSearcher, bool) {
	if strings.TrimSpace(configuredCommand) != "" && strings.TrimSpace(configuredCommand) != "rg" {
		return GoSearcher{}, false
	}
	if filepath.Base(command) != "rg" {
		return GoSearcher{}, false
	}
	if _, err := lookPath(command); err != nil && errors.Is(err, exec.ErrNotFound) {
		return GoSearcher{}, true
	}
	return GoSearcher{}, false
}

func (s GoSearcher) Search(ctx context.Context, options SearchOptions) ([]SearchMatch, error) {
	query := strings.TrimSpace(options.Query)
	if query == "" {
		return nil, nil
	}
	root, err := safeRepoRoot(options.RepoRoot)
	if err != nil {
		return nil, err
	}
	limit := options.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultSearchTimeout
	}
	outputLimit := options.OutputLimit
	if outputLimit <= 0 {
		outputLimit = defaultSearchOutputLimit
	}
	paths, err := cleanSearchPaths(options.Paths)
	if err != nil {
		return nil, err
	}
	exclude, err := cleanSearchPaths(options.ExcludePath)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	search := goSearchRun{
		root:        root,
		query:       query,
		limit:       limit,
		outputLimit: outputLimit,
		exclude:     exclude,
	}
	for _, path := range paths {
		if err := runCtx.Err(); err != nil {
			return nil, err
		}
		if err := search.searchPath(runCtx, path); err != nil {
			return nil, err
		}
		if len(search.matches) >= limit {
			break
		}
	}
	return search.matches, nil
}

type goSearchRun struct {
	root        string
	query       string
	limit       int
	outputLimit int64
	exclude     []string
	matches     []SearchMatch
	seen        map[string]struct{}
}

func (r *goSearchRun) searchPath(ctx context.Context, relativePath string) error {
	abs, clean, err := safeRepoFileOrDirPath(r.root, relativePath)
	if err != nil {
		return err
	}
	stat, err := os.Lstat(abs)
	if err != nil {
		return fmt.Errorf("search path %s cannot be inspected: %w", clean, err)
	}
	if shouldSkipSearchPath(clean, stat, r.exclude) {
		return nil
	}
	if stat.IsDir() {
		return filepath.WalkDir(abs, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			rel, ok := repoRelativePath(r.root, path)
			if !ok {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if shouldSkipDirEntry(rel, entry, r.exclude) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return nil
			}
			return r.searchFile(path, rel, info)
		})
	}
	return r.searchFile(abs, clean, stat)
}

func (r *goSearchRun) searchFile(abs string, clean string, info os.FileInfo) error {
	if len(r.matches) >= r.limit {
		return nil
	}
	if !info.Mode().IsRegular() || info.Size() > 512*1024 {
		return nil
	}
	content, err := os.ReadFile(abs)
	if err != nil {
		return nil
	}
	lines := bytes.SplitAfter(content, []byte{'\n'})
	for index, line := range lines {
		if len(r.matches) >= r.limit {
			return nil
		}
		if !bytes.Contains(line, []byte(r.query)) {
			continue
		}
		text := string(line)
		if int64(len(text)) > r.outputLimit {
			return errSearchOutputLimit
		}
		r.outputLimit -= int64(len(text))
		key := clean + ":" + strconv.Itoa(index+1)
		if r.seen == nil {
			r.seen = map[string]struct{}{}
		}
		if _, ok := r.seen[key]; ok {
			continue
		}
		r.seen[key] = struct{}{}
		r.matches = append(r.matches, SearchMatch{
			Path: clean,
			Line: int64(index + 1),
			Text: text,
		})
	}
	return nil
}

func safeRepoFileOrDirPath(root string, relativePath string) (string, string, error) {
	clean, ok := cleanRelativePath(relativePath)
	if !ok {
		return "", "", fmt.Errorf("search path %q escapes repo root", relativePath)
	}
	if clean == "." {
		return root, clean, nil
	}
	abs := filepath.Join(root, filepath.FromSlash(clean))
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", "", fmt.Errorf("search path %s cannot be resolved: %w", clean, err)
	}
	if !pathInsideRoot(root, resolved) {
		return "", "", fmt.Errorf("search path %s escapes repo root", clean)
	}
	return resolved, clean, nil
}

func repoRelativePath(root string, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func shouldSkipDirEntry(path string, entry os.DirEntry, exclude []string) bool {
	if path == "." {
		return false
	}
	if entry.Name() == ".git" {
		return true
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return true
	}
	info, err := entry.Info()
	if err != nil {
		return true
	}
	return shouldSkipSearchPath(path, info, exclude)
}

func shouldSkipSearchPath(path string, info os.FileInfo, exclude []string) bool {
	if path == ".git" || strings.HasPrefix(path, ".git/") {
		return true
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	for _, item := range exclude {
		if item == "." {
			return true
		}
		if path == item || strings.HasPrefix(path, item+"/") {
			return true
		}
	}
	return false
}

func cleanSearchPaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return []string{"."}, nil
	}
	cleaned := make([]string, 0, len(paths))
	for _, path := range paths {
		clean, ok := cleanRelativePath(path)
		if !ok {
			return nil, fmt.Errorf("search path %q escapes repo root", path)
		}
		cleaned = append(cleaned, clean)
	}
	if len(cleaned) == 0 {
		return []string{"."}, nil
	}
	return cleaned, nil
}

func safeRepoRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("repo root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("repo root is invalid: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("repo root cannot be resolved: %w", err)
	}
	stat, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("repo root cannot be inspected: %w", err)
	}
	if !stat.IsDir() {
		return "", errors.New("repo root must be a directory")
	}
	return resolved, nil
}

func safeRepoFilePath(root string, relativePath string) (string, string, error) {
	root, err := safeRepoRoot(root)
	if err != nil {
		return "", "", err
	}
	clean, ok := cleanRelativePath(relativePath)
	if !ok || clean == "." {
		return "", "", fmt.Errorf("file path %q escapes repo root", relativePath)
	}
	abs := filepath.Join(root, filepath.FromSlash(clean))
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", "", fmt.Errorf("file path %s cannot be resolved: %w", clean, err)
	}
	if !pathInsideRoot(root, resolved) {
		return "", "", fmt.Errorf("file path %s escapes repo root", clean)
	}
	return resolved, clean, nil
}

func cleanRelativePath(path string) (string, bool) {
	path = strings.TrimSpace(filepath.ToSlash(path))
	path = strings.TrimPrefix(path, "./")
	if path == "" || path == "." {
		return ".", true
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return "", false
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return "", false
		}
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." {
		return "", false
	}
	return clean, true
}

func pathInsideRoot(root string, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel))
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

func parseRipgrepJSON(output []byte, limit int) ([]SearchMatch, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	lines := bytes.Split(output, []byte{'\n'})
	matches := make([]SearchMatch, 0, min(len(lines), limit))
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
		path, ok := cleanRelativePath(event.Data.Path.Text)
		if !ok || path == "." || event.Data.LineNumber <= 0 {
			continue
		}
		matches = append(matches, SearchMatch{
			Path: path,
			Line: event.Data.LineNumber,
			Text: event.Data.Lines.Text,
		})
		if len(matches) >= limit {
			break
		}
	}
	return matches, nil
}

type limitedSearchBuffer struct {
	buf       bytes.Buffer
	remaining *int64
	exceeded  *bool
	mu        *sync.Mutex
}

func (b *limitedSearchBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if *b.remaining <= 0 {
		*b.exceeded = true
		return 0, errSearchOutputLimit
	}
	if int64(len(p)) > *b.remaining {
		allowed := int(*b.remaining)
		_, _ = b.buf.Write(p[:allowed])
		*b.remaining = 0
		*b.exceeded = true
		return allowed, errSearchOutputLimit
	}
	written, err := b.buf.Write(p)
	*b.remaining -= int64(written)
	return written, err
}

func (b *limitedSearchBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

func (b *limitedSearchBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
