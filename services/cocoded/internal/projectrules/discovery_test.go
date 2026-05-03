package projectrules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverFindsCodeownersAndCommonConfigs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRuleFile(t, root, "CODEOWNERS", "# team owners\n*.go @core\n/docs/ @docs\n")
	writeRuleFile(t, root, ".github/CODEOWNERS", "*.ts @frontend\n")
	writeRuleFile(t, root, "docs/CODEOWNERS", "docs/** @docs\n")
	writeRuleFile(t, root, "README.md", "# cocode\n")
	writeRuleFile(t, root, "package.json", `{"scripts":{"test":"vitest"}}`)
	writeRuleFile(t, root, "apps/desktop/package.json", `{"name":"desktop"}`)
	writeRuleFile(t, root, "services/cocoded/go.mod", "module cocoded\n")
	writeRuleFile(t, root, "eslint.config.js", "export default []\n")
	writeRuleFile(t, root, ".editorconfig", "root = true\n")
	writeRuleFile(t, root, "apps/web/tsconfig.json", `{"compilerOptions":`)

	candidates, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	for _, path := range []string{
		"CODEOWNERS",
		".github/CODEOWNERS",
		"docs/CODEOWNERS",
		"README.md",
		"package.json",
		"apps/desktop/package.json",
		"services/cocoded/go.mod",
		"eslint.config.js",
		".editorconfig",
		"apps/web/tsconfig.json",
	} {
		if candidateByPath(candidates, path).Path != path {
			t.Fatalf("candidate %q not found in %+v", path, candidatePaths(candidates))
		}
	}

	rootOwners := candidateByPath(candidates, "CODEOWNERS")
	if rootOwners.Kind != "codeowners" ||
		rootOwners.Title != "CODEOWNERS" ||
		rootOwners.Metadata["location"] != "root" ||
		rootOwners.Metadata["rules_count"] != 2 {
		t.Fatalf("root CODEOWNERS candidate = %+v", rootOwners)
	}
	packageManifest := candidateByPath(candidates, "apps/desktop/package.json")
	if packageManifest.Kind != "package_manifest" || !strings.Contains(string(packageManifest.Content), "desktop") {
		t.Fatalf("nested package candidate = %+v content %q", packageManifest, string(packageManifest.Content))
	}
	malformedConfig := candidateByPath(candidates, "apps/web/tsconfig.json")
	if malformedConfig.Kind != "typescript_config" || !strings.Contains(string(malformedConfig.Content), "compilerOptions") {
		t.Fatalf("malformed config candidate = %+v content %q", malformedConfig, string(malformedConfig.Content))
	}
}

func TestDiscoverSkipsNoisyUnsafeAndUnsupportedFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRuleFile(t, root, "codeowners", "*.go @lowercase\n")
	writeRuleFile(t, root, "CODEOWNERS", "# comments only\n\n")
	writeRuleFile(t, root, "node_modules/pkg/package.json", `{"ignored":true}`)
	writeRuleFile(t, root, "dist/package.json", `{"ignored":true}`)
	writeRuleFile(t, root, "vendor/module/go.mod", "module ignored\n")
	writeRuleBytes(t, root, "package.json", []byte{0, 1, 2})
	writeRuleFile(t, root, "big/README.md", strings.Repeat("x", 32))

	outside := t.TempDir()
	writeRuleFile(t, outside, "go.mod", "module outside\n")
	if err := os.Symlink(filepath.Join(outside, "go.mod"), filepath.Join(root, "go.mod")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	candidates, err := DiscoverWithOptions(root, Options{MaxFileBytes: 16})
	if err != nil {
		t.Fatalf("DiscoverWithOptions() error = %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("candidates = %+v, want none", candidates)
	}
}

func TestDiscoverRejectsInvalidRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	filePath := filepath.Join(root, "README.md")
	if err := os.WriteFile(filePath, []byte("# file\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := Discover(""); err == nil {
		t.Fatal("Discover(empty root) error = nil, want error")
	}
	if _, err := Discover(filePath); err == nil {
		t.Fatal("Discover(file root) error = nil, want error")
	}
}

func writeRuleFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()
	writeRuleBytes(t, root, relativePath, []byte(content))
}

func writeRuleBytes(t *testing.T, root string, relativePath string, content []byte) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", relativePath, err)
	}
}

func candidateByPath(candidates []Candidate, path string) Candidate {
	for _, candidate := range candidates {
		if candidate.Path == path {
			return candidate
		}
	}
	return Candidate{}
}

func candidatePaths(candidates []Candidate) []string {
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		paths = append(paths, candidate.Path)
	}
	return paths
}
