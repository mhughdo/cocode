package gitrepo

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/diffparse"
	"github.com/hughdo/cocode/services/cocoded/internal/security"
)

const (
	SourceBranchCompare = "branch_compare"
	SourceLocalChanges  = "local_changes"

	diffOutputLimit        int64 = 16 << 20
	maxUntrackedPatchBytes int64 = 512 << 10
)

type Collector struct {
	Runner Runner
}

type DiffSnapshot struct {
	SourceType string
	BaseRef    string
	HeadRef    string
	BaseSHA    string
	HeadSHA    string
	Diff       []byte
	Files      []DiffFile
	Metadata   map[string]any
}

type DiffFile struct {
	Path           string
	OldPath        string
	Status         string
	Additions      int64
	Deletions      int64
	IsBinary       bool
	LineRangesJSON string
	Patch          string
}

func NewCollector(runner Runner) Collector {
	return Collector{Runner: runner}
}

func (c Collector) CompareBranches(ctx context.Context, selectedPath string, baseRef string, headRef string) (DiffSnapshot, error) {
	baseRef, err := normalizeRefInput(baseRef, "base ref")
	if err != nil {
		return DiffSnapshot{}, err
	}
	headRef, err = normalizeRefInput(headRef, "head ref")
	if err != nil {
		return DiffSnapshot{}, err
	}

	runner := c.runner()
	info, err := validate(ctx, runner, selectedPath)
	if err != nil {
		return DiffSnapshot{}, err
	}

	baseTipSHA, err := resolveCommit(ctx, runner, info.RootPath, baseRef)
	if err != nil {
		return DiffSnapshot{}, err
	}
	headSHA, err := resolveCommit(ctx, runner, info.RootPath, headRef)
	if err != nil {
		return DiffSnapshot{}, err
	}
	mergeBaseSHA, err := runGitText(ctx, runner, info.RootPath, "merge-base", baseTipSHA, headSHA)
	if err != nil {
		return DiffSnapshot{}, err
	}

	diffResult, err := runner.RunRaw(ctx, info.RootPath, "diff", "--binary", "--full-index", "--find-renames", mergeBaseSHA, headSHA, "--")
	if err != nil {
		return DiffSnapshot{}, err
	}
	files, err := parseDiffFiles(diffResult.Stdout)
	if err != nil {
		return DiffSnapshot{}, err
	}
	dirty, err := hasWorktreeChanges(ctx, runner, info.RootPath)
	if err != nil {
		return DiffSnapshot{}, err
	}

	return DiffSnapshot{
		SourceType: SourceBranchCompare,
		BaseRef:    baseRef,
		HeadRef:    headRef,
		BaseSHA:    mergeBaseSHA,
		HeadSHA:    headSHA,
		Diff:       []byte(diffResult.Stdout),
		Files:      files,
		Metadata: map[string]any{
			"source":                        SourceBranchCompare,
			"base_tip_sha":                  baseTipSHA,
			"has_uncommitted_changes":       dirty,
			"comparison_base_is_merge_base": true,
		},
	}, nil
}

func (c Collector) LocalChanges(ctx context.Context, selectedPath string) (DiffSnapshot, error) {
	runner := c.runner()
	info, err := validate(ctx, runner, selectedPath)
	if err != nil {
		return DiffSnapshot{}, err
	}
	headSHA, err := resolveCommit(ctx, runner, info.RootPath, "HEAD")
	if err != nil {
		return DiffSnapshot{}, err
	}

	diffResult, err := runner.RunRaw(ctx, info.RootPath, "diff", "--binary", "--full-index", "--find-renames", "HEAD", "--")
	if err != nil {
		return DiffSnapshot{}, err
	}
	files, err := parseDiffFiles(diffResult.Stdout)
	if err != nil {
		return DiffSnapshot{}, err
	}

	statusResult, err := runner.RunRaw(ctx, info.RootPath, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return DiffSnapshot{}, err
	}
	untrackedFiles, untrackedPatches, err := collectUntrackedFiles(info.RootPath, parseUntrackedPaths(statusResult.Stdout), files)
	if err != nil {
		return DiffSnapshot{}, err
	}
	fullDiff := appendPatches(diffResult.Stdout, untrackedPatches)
	files = append(files, untrackedFiles...)

	return DiffSnapshot{
		SourceType: SourceLocalChanges,
		BaseRef:    "HEAD",
		HeadRef:    "WORKTREE",
		BaseSHA:    headSHA,
		HeadSHA:    headSHA,
		Diff:       []byte(fullDiff),
		Files:      files,
		Metadata: map[string]any{
			"source":               SourceLocalChanges,
			"tracked_file_count":   len(files) - len(untrackedFiles),
			"untracked_file_count": len(untrackedFiles),
		},
	}, nil
}

func (c Collector) runner() Runner {
	runner := c.Runner
	if runner.Timeout <= 0 {
		runner.Timeout = gitTimeout
	}
	if runner.OutputLimit <= 0 {
		runner.OutputLimit = diffOutputLimit
	}
	return runner
}

func normalizeRefInput(ref string, label string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", apperror.InvalidRequest(label + " is required")
	}
	if strings.HasPrefix(ref, "-") || strings.Contains(ref, "..") || strings.ContainsRune(ref, '\x00') {
		return "", apperror.InvalidRequest(label + " is invalid")
	}
	return ref, nil
}

func resolveCommit(ctx context.Context, runner Runner, cwd string, ref string) (string, error) {
	return runGitText(ctx, runner, cwd, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
}

func runGitText(ctx context.Context, runner Runner, cwd string, args ...string) (string, error) {
	result, err := runner.Run(ctx, cwd, args...)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(result.Stdout)
	if value == "" {
		return "", apperror.InvalidRequest("git command returned empty output")
	}
	return value, nil
}

func hasWorktreeChanges(ctx context.Context, runner Runner, cwd string) (bool, error) {
	result, err := runner.RunRaw(ctx, cwd, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return false, err
	}
	return result.Stdout != "", nil
}

func parseDiffFiles(diff string) ([]DiffFile, error) {
	if strings.TrimSpace(diff) == "" {
		return nil, nil
	}
	parsed, err := diffparse.Parse(diff)
	if err != nil {
		return nil, apperror.Internal("failed to parse git diff")
	}
	patches := splitDiffPatches(diff)
	files := make([]DiffFile, 0, len(parsed))
	for i, file := range parsed {
		lineRangesJSON, err := file.LineRangesJSON()
		if err != nil {
			return nil, apperror.Internal("failed to encode changed line ranges")
		}
		patch := ""
		if i < len(patches) {
			patch = patches[i]
		}
		files = append(files, DiffFile{
			Path:           file.Path,
			OldPath:        file.OldPath,
			Status:         string(file.Status),
			Additions:      int64(file.Additions),
			Deletions:      int64(file.Deletions),
			IsBinary:       file.Binary,
			LineRangesJSON: lineRangesJSON,
			Patch:          patch,
		})
	}
	return files, nil
}

func splitDiffPatches(diff string) []string {
	lines := strings.SplitAfter(diff, "\n")
	patches := []string{}
	var current strings.Builder
	insidePatch := false
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if insidePatch {
				patches = append(patches, current.String())
				current.Reset()
			}
			insidePatch = true
		}
		if insidePatch {
			current.WriteString(line)
		}
	}
	if insidePatch {
		patches = append(patches, current.String())
	}
	return patches
}

func parseUntrackedPaths(status string) []string {
	if status == "" {
		return nil
	}
	records := strings.Split(status, "\x00")
	paths := []string{}
	for _, record := range records {
		if len(record) < 4 || record[:2] != "??" {
			continue
		}
		path := strings.TrimSpace(record[3:])
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func collectUntrackedFiles(root string, paths []string, existing []DiffFile) ([]DiffFile, []string, error) {
	seen := map[string]struct{}{}
	for _, file := range existing {
		seen[file.Path] = struct{}{}
	}

	files := []DiffFile{}
	patches := []string{}
	for _, path := range paths {
		if _, ok := seen[path]; ok {
			continue
		}
		if _, err := security.CleanRelativePath(path); err != nil {
			return nil, nil, apperror.InvalidRequest("git status returned unsafe untracked path")
		}

		file, patch, err := buildUntrackedFile(root, path)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, file)
		if patch != "" {
			patches = append(patches, patch)
		}
		seen[path] = struct{}{}
	}
	return files, patches, nil
}

func buildUntrackedFile(root string, path string) (DiffFile, string, error) {
	absolutePath, cleanPath, err := security.ResolveExistingWithinRoot(root, path)
	if err != nil {
		return DiffFile{}, "", apperror.InvalidRequest("untracked file escapes repository root")
	}
	path = cleanPath
	stat, err := os.Stat(absolutePath)
	if err != nil {
		return DiffFile{}, "", apperror.InvalidRequest("untracked file cannot be inspected")
	}
	if stat.IsDir() {
		return DiffFile{}, "", apperror.InvalidRequest("untracked path is a directory")
	}

	mode := "100644"
	if stat.Mode()&0o111 != 0 {
		mode = "100755"
	}

	content, oversized, err := readBoundedFile(absolutePath, maxUntrackedPatchBytes)
	if err != nil {
		return DiffFile{}, "", apperror.InvalidRequest("untracked file cannot be read")
	}
	if oversized || isBinaryContent(content) {
		patch := buildBinaryUntrackedPatch(path, mode)
		return DiffFile{
			Path:           path,
			Status:         string(diffparse.StatusAdded),
			IsBinary:       true,
			LineRangesJSON: "[]",
			Patch:          patch,
		}, patch, nil
	}

	patch := buildTextUntrackedPatch(path, mode, string(content))
	files, err := parseDiffFiles(patch)
	if err != nil {
		return DiffFile{}, "", err
	}
	if len(files) != 1 {
		return DiffFile{}, "", apperror.Internal("failed to build untracked file diff")
	}
	files[0].Status = string(diffparse.StatusAdded)
	files[0].Patch = patch
	return files[0], patch, nil
}

func readBoundedFile(path string, limit int64) ([]byte, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(content)) > limit {
		return content[:limit], true, nil
	}
	return content, false, nil
}

func isBinaryContent(content []byte) bool {
	return bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content)
}

func buildTextUntrackedPatch(path string, mode string, content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines, hasFinalNewline := splitFileContentLines(content)

	var builder strings.Builder
	writeUntrackedHeader(&builder, path, mode)
	builder.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(lines)))
	for _, line := range lines {
		builder.WriteByte('+')
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	if len(lines) > 0 && !hasFinalNewline {
		builder.WriteString("\\ No newline at end of file\n")
	}
	return builder.String()
}

func buildBinaryUntrackedPatch(path string, mode string) string {
	var builder strings.Builder
	writeUntrackedHeader(&builder, path, mode)
	builder.WriteString("Binary files /dev/null and ")
	builder.WriteString(diffPath("b", path))
	builder.WriteString(" differ\n")
	return builder.String()
}

func writeUntrackedHeader(builder *strings.Builder, path string, mode string) {
	builder.WriteString("diff --git ")
	builder.WriteString(diffPath("a", path))
	builder.WriteByte(' ')
	builder.WriteString(diffPath("b", path))
	builder.WriteByte('\n')
	builder.WriteString("new file mode ")
	builder.WriteString(mode)
	builder.WriteByte('\n')
	builder.WriteString("index 0000000..0000000\n")
	builder.WriteString("--- /dev/null\n")
	builder.WriteString("+++ ")
	builder.WriteString(diffPath("b", path))
	builder.WriteByte('\n')
}

func splitFileContentLines(content string) ([]string, bool) {
	if content == "" {
		return nil, false
	}
	hasFinalNewline := strings.HasSuffix(content, "\n")
	if hasFinalNewline {
		content = strings.TrimSuffix(content, "\n")
	}
	if content == "" {
		return []string{""}, true
	}
	return strings.Split(content, "\n"), hasFinalNewline
}

func diffPath(prefix string, path string) string {
	value := prefix + "/" + filepath.ToSlash(path)
	if strings.ContainsAny(value, " \t\n\"\\") {
		return strconv.Quote(value)
	}
	return value
}

func appendPatches(base string, patches []string) string {
	full := base
	for _, patch := range patches {
		if patch == "" {
			continue
		}
		if full != "" && !strings.HasSuffix(full, "\n") {
			full += "\n"
		}
		full += patch
	}
	return full
}

func isSafeRelativePath(path string) bool {
	clean, err := security.CleanRelativePath(path)
	return err == nil && clean != "."
}
