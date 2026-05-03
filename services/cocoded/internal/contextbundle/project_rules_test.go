package contextbundle

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildProjectRuleContextItemsDiscoversCommonRules(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRepoFile(t, root, "CODEOWNERS", "*.go @backend\n")
	writeRepoFile(t, root, "README.md", "# cocode\nA desktop code review assistant.\n")
	writeRepoFile(t, root, "package.json", `{"scripts":{"test":"vitest"}}`)
	writeRepoFile(t, root, "eslint.config.js", "export default []\n")
	writeRepoFile(t, root, "apps/desktop/vite.config.ts", "export default {}\n")
	writeRepoFile(t, root, "node_modules/pkg/package.json", `{"ignored":true}`)

	items, err := BuildProjectRuleContextItems(ProjectRuleOptions{
		BundleID:        "bundle_1",
		RepoRoot:        root,
		MaxContentBytes: 256,
		MaxItems:        10,
	})
	if err != nil {
		t.Fatalf("BuildProjectRuleContextItems() error = %v", err)
	}

	for _, path := range []string{"CODEOWNERS", "README.md", "package.json", "eslint.config.js", "apps/desktop/vite.config.ts"} {
		if itemByPath(items, path).Path != path {
			t.Fatalf("item %q not found in %+v", path, itemPaths(items))
		}
	}
	if itemByPath(items, "node_modules/pkg/package.json").Path != "" {
		t.Fatalf("node_modules package should be skipped: %+v", itemPaths(items))
	}

	codeowners := itemByPath(items, "CODEOWNERS")
	if codeowners.Kind != ItemProjectRule ||
		codeowners.Title != "CODEOWNERS" ||
		!strings.Contains(codeowners.Content, "@backend") {
		t.Fatalf("CODEOWNERS item = %+v", codeowners)
	}
	assertProjectRuleMetadata(t, codeowners.Metadata, "codeowners", false)
}

func TestBuildProjectRuleContextItemsPrioritizesAndTruncates(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRepoFile(t, root, "README.md", strings.Repeat("readme-line\n", 20))
	writeRepoFile(t, root, "package.json", `{"name":"cocode"}`)
	writeRepoFile(t, root, "CODEOWNERS", "*.ts @frontend\n")

	items, err := BuildProjectRuleContextItems(ProjectRuleOptions{
		BundleID:        "bundle_1",
		RepoRoot:        root,
		MaxContentBytes: 12,
		MaxItems:        2,
	})
	if err != nil {
		t.Fatalf("BuildProjectRuleContextItems() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2: %+v", len(items), items)
	}
	if items[0].Path != "CODEOWNERS" || items[1].Path != "README.md" {
		t.Fatalf("items order = %+v", itemPaths(items))
	}
	if len(items[1].Content) > 12 {
		t.Fatalf("README content len = %d, want <= 12", len(items[1].Content))
	}
	assertProjectRuleMetadata(t, items[1].Metadata, "readme", true)
}

func TestBuildProjectRuleContextItemsRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRepoFile(t, root, "README.md", "# cocode\n")

	if _, err := BuildProjectRuleContextItems(ProjectRuleOptions{RepoRoot: root}); err == nil {
		t.Fatal("BuildProjectRuleContextItems(empty bundle) error = nil, want error")
	}
	if _, err := BuildProjectRuleContextItems(ProjectRuleOptions{BundleID: "bundle_1"}); err == nil {
		t.Fatal("BuildProjectRuleContextItems(empty root) error = nil, want error")
	}
}

func assertProjectRuleMetadata(t *testing.T, raw json.RawMessage, kind string, truncated bool) {
	t.Helper()

	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatalf("Unmarshal(metadata) error = %v", err)
	}
	if metadata["rule_kind"] != kind ||
		metadata["source"] != "project_rules_discovery" ||
		metadata["context_source"] != "project_rule_context" ||
		metadata["truncated"] != truncated {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func itemByPath(items []Item, path string) Item {
	for _, item := range items {
		if item.Path == path {
			return item
		}
	}
	return Item{}
}

func itemPaths(items []Item) []string {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
	}
	return paths
}
