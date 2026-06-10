package projectrules

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const DefaultMaxFileBytes int64 = 256 << 10

type Candidate struct {
	Kind      string
	Path      string
	Title     string
	Content   []byte
	SizeBytes int64
	Metadata  map[string]any
}

type Options struct {
	MaxFileBytes int64
}

type fileRule struct {
	Kind  string
	Title string
}

func Discover(root string) ([]Candidate, error) {
	return DiscoverWithOptions(root, Options{})
}

func DiscoverWithOptions(root string, options Options) ([]Candidate, error) {
	if options.MaxFileBytes <= 0 {
		options.MaxFileBytes = DefaultMaxFileBytes
	}

	root, err := cleanRoot(root)
	if err != nil {
		return nil, err
	}

	candidates := []Candidate{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == root {
			return nil
		}

		relativePath, ok := repoRelative(root, path)
		if !ok {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if shouldSkipDir(relativePath) {
				return filepath.SkipDir
			}
			return nil
		}

		rule, ok := ruleForPath(relativePath)
		if !ok {
			return nil
		}
		candidate, ok, err := readCandidate(root, path, relativePath, entry, rule, options.MaxFileBytes)
		if err != nil {
			return err
		}
		if ok {
			candidates = append(candidates, candidate)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Kind != candidates[j].Kind {
			return candidates[i].Kind < candidates[j].Kind
		}
		return candidates[i].Path < candidates[j].Path
	})
	return candidates, nil
}

func cleanRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("repository root is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	stat, err := os.Stat(resolvedRoot)
	if err != nil {
		return "", fmt.Errorf("inspect repository root: %w", err)
	}
	if !stat.IsDir() {
		return "", errors.New("repository root must be a directory")
	}
	return resolvedRoot, nil
}

func readCandidate(root string, path string, relativePath string, entry fs.DirEntry, rule fileRule, maxBytes int64) (Candidate, bool, error) {
	if entry.Type()&fs.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return Candidate{}, false, nil
		}
		if _, ok := repoRelative(root, target); !ok {
			return Candidate{}, false, nil
		}
	}

	stat, err := os.Stat(path)
	if err != nil || stat.IsDir() {
		return Candidate{}, false, nil
	}
	if stat.Size() > maxBytes {
		return Candidate{}, false, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return Candidate{}, false, nil
	}
	if len(bytes.TrimSpace(content)) == 0 || looksBinary(content) {
		return Candidate{}, false, nil
	}

	metadata := map[string]any{
		"source": "project_rules_discovery",
	}
	if rule.Kind == "codeowners" {
		metadata["rules_count"] = countCodeownerRules(content)
		metadata["location"] = codeownersLocation(relativePath)
		if metadata["rules_count"] == 0 {
			return Candidate{}, false, nil
		}
	}

	return Candidate{
		Kind:      rule.Kind,
		Path:      filepath.ToSlash(relativePath),
		Title:     rule.Title,
		Content:   content,
		SizeBytes: stat.Size(),
		Metadata:  metadata,
	}, true, nil
}

func ruleForPath(relativePath string) (fileRule, bool) {
	path := filepath.ToSlash(relativePath)
	if rule, ok := codeownersRule(path); ok {
		return rule, true
	}
	if rule, ok := cursorRule(path); ok {
		return rule, true
	}

	base := filepath.Base(path)
	switch strings.ToLower(base) {
	case "agents.md":
		return fileRule{Kind: "agent_instructions", Title: "AGENTS.md"}, true
	case "claude.md":
		return fileRule{Kind: "claude_instructions", Title: "CLAUDE.md"}, true
	case "contributing.md", "contributing":
		return fileRule{Kind: "contributing", Title: "CONTRIBUTING"}, true
	case "readme.md", "readme":
		return fileRule{Kind: "readme", Title: "README"}, true
	case "go.mod":
		return fileRule{Kind: "go_module", Title: "Go module"}, true
	case "package.json":
		return fileRule{Kind: "package_manifest", Title: "package.json"}, true
	case "pnpm-workspace.yaml", "pnpm-workspace.yml":
		return fileRule{Kind: "workspace_config", Title: "pnpm workspace"}, true
	case "tsconfig.json":
		return fileRule{Kind: "typescript_config", Title: "TypeScript config"}, true
	case ".editorconfig":
		return fileRule{Kind: "editor_config", Title: "EditorConfig"}, true
	case "eslint.config.js", "eslint.config.mjs", "eslint.config.cjs", ".eslintrc", ".eslintrc.json", ".eslintrc.yaml", ".eslintrc.yml", ".eslintrc.js", ".eslintrc.cjs":
		return fileRule{Kind: "lint_config", Title: "ESLint config"}, true
	case "vite.config.ts", "vite.config.js", "vite.config.mts", "vite.config.mjs", "electron.vite.config.ts", "makefile", "dockerfile":
		return fileRule{Kind: "build_config", Title: base}, true
	case "tailwind.config.ts", "tailwind.config.js", "tailwind.config.mjs":
		return fileRule{Kind: "style_config", Title: "Tailwind config"}, true
	case "components.json":
		return fileRule{Kind: "ui_config", Title: "shadcn components config"}, true
	case "pyproject.toml":
		return fileRule{Kind: "python_config", Title: "Python project config"}, true
	case "cargo.toml":
		return fileRule{Kind: "rust_config", Title: "Rust manifest"}, true
	default:
		return fileRule{}, false
	}
}

func cursorRule(path string) (fileRule, bool) {
	if path == ".cursor/rules" || strings.HasPrefix(path, ".cursor/rules/") {
		return fileRule{Kind: "cursor_rule", Title: ".cursor/rules"}, true
	}
	return fileRule{}, false
}

func codeownersRule(path string) (fileRule, bool) {
	switch path {
	case "CODEOWNERS":
		return fileRule{Kind: "codeowners", Title: "CODEOWNERS"}, true
	case ".github/CODEOWNERS":
		return fileRule{Kind: "codeowners", Title: ".github/CODEOWNERS"}, true
	case "docs/CODEOWNERS":
		return fileRule{Kind: "codeowners", Title: "docs/CODEOWNERS"}, true
	default:
		return fileRule{}, false
	}
}

func codeownersLocation(path string) string {
	switch filepath.ToSlash(path) {
	case "CODEOWNERS":
		return "root"
	case ".github/CODEOWNERS":
		return "github"
	case "docs/CODEOWNERS":
		return "docs"
	default:
		return "unknown"
	}
}

func countCodeownerRules(content []byte) int {
	count := 0
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		count++
	}
	return count
}

func repoRelative(root string, path string) (string, bool) {
	relativePath, err := filepath.Rel(root, path)
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", false
	}
	return relativePath, true
}

func shouldSkipDir(relativePath string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(relativePath), "/") {
		switch segment {
		case ".git", ".next", ".nuxt", "build", "coverage", "dist", "node_modules", "target", "vendor":
			return true
		}
	}
	return false
}

func looksBinary(content []byte) bool {
	return bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content)
}
