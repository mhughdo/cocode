package contextbundle

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestBuildRelatedCodeContextItems(t *testing.T) {
	t.Parallel()

	searcher := fakeCodeSearcher{
		matches: map[string][]CodeSearchMatch{
			"RequireAdmin": {
				{Path: "src/auth.ts", Line: 12, Text: "export function RequireAdmin() {}"},
				{Path: "src/routes.ts", Line: 24, Text: "router.use(RequireAdmin)"},
				{Path: "src/routes.ts", Line: 24, Text: "router.use(RequireAdmin)"},
			},
			"auth": {
				{Path: "./src/app.ts", Line: 5, Text: "import { RequireAdmin } from './auth'"},
			},
		},
	}
	root := t.TempDir()
	items, err := BuildRelatedCodeContextItems(context.Background(), RelatedCodeSearchOptions{
		BundleID: "bundle_1",
		RepoRoot: root,
		Searcher: searcher,
		MaxItems: 10,
	}, []RelatedSearchInput{{
		ChangedFileID: "changed_auth",
		Path:          "src/auth.ts",
		Symbols:       []string{"RequireAdmin", "RequireAdmin", "if"},
	}})
	if err != nil {
		t.Fatalf("BuildRelatedCodeContextItems() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2: %+v", len(items), items)
	}
	if items[0].Kind != ItemRelatedCode ||
		items[0].Path != "src/routes.ts" ||
		items[0].StartLine != 24 ||
		!strings.Contains(items[0].Content, "RequireAdmin") {
		t.Fatalf("first item = %+v", items[0])
	}
	if items[1].Path != "src/app.ts" || items[1].StartLine != 5 {
		t.Fatalf("second item = %+v", items[1])
	}
	var metadata map[string]any
	if err := json.Unmarshal(items[0].Metadata, &metadata); err != nil {
		t.Fatalf("Unmarshal(metadata) error = %v", err)
	}
	if metadata["changed_file_id"] != "changed_auth" ||
		metadata["source_path"] != "src/auth.ts" ||
		metadata["search_term"] != "RequireAdmin" {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func TestBuildRelatedCodeContextItemsLimitsAndErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	searcher := fakeCodeSearcher{
		matches: map[string][]CodeSearchMatch{
			"Symbol": {
				{Path: "a.go", Line: 1, Text: "Symbol()"},
				{Path: "b.go", Line: 2, Text: "Symbol()"},
			},
		},
	}
	items, err := BuildRelatedCodeContextItems(context.Background(), RelatedCodeSearchOptions{
		BundleID: "bundle_1",
		RepoRoot: root,
		Searcher: searcher,
		MaxItems: 1,
	}, []RelatedSearchInput{{Path: "src/source.go", Symbols: []string{"Symbol"}}})
	if err != nil {
		t.Fatalf("BuildRelatedCodeContextItems(limit) error = %v", err)
	}
	if len(items) != 1 || items[0].Path != "a.go" {
		t.Fatalf("limited items = %+v", items)
	}

	_, err = BuildRelatedCodeContextItems(context.Background(), RelatedCodeSearchOptions{
		BundleID: "bundle_1",
		RepoRoot: root,
		Searcher: fakeCodeSearcher{err: errors.New("boom")},
	}, []RelatedSearchInput{{Path: "src/source.go", Symbols: []string{"Symbol"}}})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("BuildRelatedCodeContextItems(error) = %v", err)
	}

	empty, err := BuildRelatedCodeContextItems(context.Background(), RelatedCodeSearchOptions{
		BundleID: "bundle_1",
		RepoRoot: root,
		Searcher: searcher,
	}, []RelatedSearchInput{{Path: "a.go", Excluded: true, Symbols: []string{"Symbol"}}})
	if err != nil {
		t.Fatalf("BuildRelatedCodeContextItems(excluded) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("excluded items = %+v, want empty", empty)
	}
}

func TestRipgrepSearcherFindsFixedString(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg is not installed")
	}
	root := t.TempDir()
	writeRepoFile(t, root, "src/auth.ts", "export function RequireAdmin() {}\n")
	writeRepoFile(t, root, "src/routes.ts", "router.use(RequireAdmin)\n")

	matches, err := (RipgrepSearcher{}).Search(context.Background(), root, "RequireAdmin", 10)
	if err != nil {
		t.Fatalf("RipgrepSearcher.Search() error = %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches len = %d, want 2: %+v", len(matches), matches)
	}
	paths := map[string]bool{}
	for _, match := range matches {
		paths[match.Path] = true
		if match.Line != 1 || !strings.Contains(match.Text, "RequireAdmin") {
			t.Fatalf("match = %+v", match)
		}
	}
	if !paths["src/auth.ts"] || !paths["src/routes.ts"] {
		t.Fatalf("paths = %+v", paths)
	}
}

func TestParseRipgrepJSONRejectsMalformedEvents(t *testing.T) {
	t.Parallel()

	if _, err := parseRipgrepJSON([]byte("{not-json}\n"), 10); err == nil {
		t.Fatal("parseRipgrepJSON(malformed) error = nil")
	}
}

type fakeCodeSearcher struct {
	matches map[string][]CodeSearchMatch
	err     error
}

func (s fakeCodeSearcher) Search(_ context.Context, _ string, term string, limit int) ([]CodeSearchMatch, error) {
	if s.err != nil {
		return nil, s.err
	}
	matches := append([]CodeSearchMatch(nil), s.matches[term]...)
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}
