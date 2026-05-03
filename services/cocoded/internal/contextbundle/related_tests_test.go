package contextbundle

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/diffparse"
)

func TestBuildRelatedTestContextItemsFindsSiblingAndReferencedTests(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRepoFile(t, root, "src/auth.ts", "export function RequireAdmin() {}\n")
	writeRepoFile(t, root, "src/auth.test.ts", "import { RequireAdmin } from './auth'\nRequireAdmin()\n")
	writeRepoFile(t, root, "src/payment.ts", "export function authorizePayment() {}\n")
	writeRepoFile(t, root, "tests/payment_flow.test.ts", "authorizePayment('order_1')\n")

	items, err := BuildRelatedTestContextItems(RelatedTestOptions{
		BundleID: "bundle_1",
		RepoRoot: root,
		MaxItems: 10,
	}, []RelatedTestInput{
		{
			ChangedFileID: "changed_auth",
			Path:          "src/auth.ts",
			Symbols:       []string{"RequireAdmin"},
		},
		{
			ChangedFileID: "changed_payment",
			Path:          "src/payment.ts",
			Symbols:       []string{"authorizePayment"},
		},
	})
	if err != nil {
		t.Fatalf("BuildRelatedTestContextItems() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2: %+v", len(items), items)
	}
	if items[0].Kind != ItemRelatedTest ||
		items[0].Path != "src/auth.test.ts" ||
		!strings.Contains(items[0].Content, "RequireAdmin") {
		t.Fatalf("sibling test item = %+v", items[0])
	}
	assertRelatedTestMetadata(t, items[0].Metadata, "changed_auth", "path_candidate", false)
	if items[1].Path != "tests/payment_flow.test.ts" ||
		!strings.Contains(items[1].Content, "authorizePayment") {
		t.Fatalf("referenced test item = %+v", items[1])
	}
	assertRelatedTestMetadata(t, items[1].Metadata, "changed_payment", "reference_search", false)
}

func TestBuildRelatedTestContextItemsRecordsMissingAndSkipsExcluded(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRepoFile(t, root, "src/no_test.ts", "export const noTest = true\n")
	writeRepoFile(t, root, "src/generated.ts", "export const generated = true\n")

	items, err := BuildRelatedTestContextItems(RelatedTestOptions{
		BundleID: "bundle_1",
		RepoRoot: root,
		MaxItems: 10,
	}, []RelatedTestInput{
		{
			ChangedFileID: "changed_missing",
			Path:          "src/no_test.ts",
			Symbols:       []string{"noTest"},
		},
		{
			ChangedFileID: "changed_generated",
			Path:          "src/generated.ts",
			Excluded:      true,
			Symbols:       []string{"generated"},
		},
	})
	if err != nil {
		t.Fatalf("BuildRelatedTestContextItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1: %+v", len(items), items)
	}
	if items[0].Path != "" ||
		!strings.Contains(items[0].Content, "No related test file") {
		t.Fatalf("missing item = %+v", items[0])
	}
	assertRelatedTestMetadata(t, items[0].Metadata, "changed_missing", "missing", true)
}

func TestBuildRelatedTestContextItemsFindsImportPathReferences(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRepoFile(t, root, "lib/user.ts", "export function makeUser() {}\n")
	writeRepoFile(t, root, "tests/user_access.spec.ts", "import { makeUser } from '../lib/user'\n")

	items, err := BuildRelatedTestContextItems(RelatedTestOptions{
		BundleID: "bundle_1",
		RepoRoot: root,
		MaxItems: 10,
	}, []RelatedTestInput{
		{
			ChangedFileID: "changed_user",
			Path:          "lib/user.ts",
		},
	})
	if err != nil {
		t.Fatalf("BuildRelatedTestContextItems() error = %v", err)
	}
	if len(items) != 1 ||
		items[0].Path != "tests/user_access.spec.ts" ||
		!strings.Contains(items[0].Content, "../lib/user") {
		t.Fatalf("items = %+v", items)
	}
	assertRelatedTestMetadata(t, items[0].Metadata, "changed_user", "reference_search", false)
}

func TestBuildRelatedTestContextItemsDedupesSharedTestsWithoutMissing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRepoFile(t, root, "src/a.ts", "export function AlphaThing() {}\n")
	writeRepoFile(t, root, "src/b.ts", "export function BetaThing() {}\n")
	writeRepoFile(t, root, "tests/shared.test.ts", "AlphaThing()\nBetaThing()\n")

	items, err := BuildRelatedTestContextItems(RelatedTestOptions{
		BundleID: "bundle_1",
		RepoRoot: root,
		MaxItems: 10,
	}, []RelatedTestInput{
		{ChangedFileID: "changed_a", Path: "src/a.ts", Symbols: []string{"AlphaThing"}},
		{ChangedFileID: "changed_b", Path: "src/b.ts", Symbols: []string{"BetaThing"}},
	})
	if err != nil {
		t.Fatalf("BuildRelatedTestContextItems() error = %v", err)
	}
	if len(items) != 1 || items[0].Path != "tests/shared.test.ts" {
		t.Fatalf("items = %+v", items)
	}
}

func TestBuildRelatedTestContextItemsLimitsAndUnsafePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRepoFile(t, root, "src/a.ts", "export const A = true\n")
	writeRepoFile(t, root, "src/a.test.ts", "A\n")
	writeRepoFile(t, root, "src/b.ts", "export const B = true\n")
	writeRepoFile(t, root, "src/b.test.ts", "B\n")

	items, err := BuildRelatedTestContextItems(RelatedTestOptions{
		BundleID: "bundle_1",
		RepoRoot: root,
		MaxItems: 1,
	}, []RelatedTestInput{
		{ChangedFileID: "changed_a", Path: "src/a.ts", Symbols: []string{"A"}},
		{ChangedFileID: "changed_b", Path: "src/b.ts", Symbols: []string{"B"}},
	})
	if err != nil {
		t.Fatalf("BuildRelatedTestContextItems(limit) error = %v", err)
	}
	if len(items) != 1 || items[0].Path != "src/a.test.ts" {
		t.Fatalf("limited items = %+v", items)
	}

	if _, err := BuildRelatedTestContextItems(RelatedTestOptions{
		BundleID: "bundle_1",
		RepoRoot: root,
	}, []RelatedTestInput{{ChangedFileID: "changed_escape", Path: "../escape.ts"}}); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("BuildRelatedTestContextItems(unsafe) error = %v", err)
	}
}

func TestCandidateTestPaths(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"app/auth.go":        {"app/auth_test.go"},
		"app/auth.ts":        {"app/auth.test.ts", "app/auth.spec.ts", "app/__tests__/auth.test.ts", "app/__tests__/auth.spec.ts", "app/auth.test.tsx", "app/auth.spec.tsx", "app/__tests__/auth.test.tsx", "app/__tests__/auth.spec.tsx"},
		"service/user.py":    {"service/test_user.py", "service/user_test.py", "tests/test_user.py"},
		"README.md":          {},
		"components/App.tsx": {"components/App.test.tsx", "components/App.spec.tsx", "components/__tests__/App.test.tsx", "components/__tests__/App.spec.tsx"},
	}
	for path, want := range tests {
		got := candidateTestPaths(path)
		if len(got) != len(want) {
			t.Fatalf("candidateTestPaths(%q) len = %d, want %d: %+v", path, len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("candidateTestPaths(%q)[%d] = %q, want %q", path, i, got[i], want[i])
			}
		}
	}
}

func TestRelatedTestInputCanUseChangedFileContentInput(t *testing.T) {
	t.Parallel()

	input := ChangedFileContentInput{
		ChangedFileID: "changed_file_1",
		Path:          "src/auth.ts",
		Status:        string(diffparse.StatusModified),
		LineRanges:    []diffparse.LineRange{{Start: 1, End: 2}},
	}
	related := RelatedTestInput{
		ChangedFileID: input.ChangedFileID,
		Path:          input.Path,
	}
	if related.ChangedFileID != "changed_file_1" || related.Path != "src/auth.ts" {
		t.Fatalf("related = %+v", related)
	}
}

func assertRelatedTestMetadata(t *testing.T, raw json.RawMessage, changedFileID string, source string, missing bool) {
	t.Helper()

	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatalf("Unmarshal(metadata) error = %v", err)
	}
	if metadata["changed_file_id"] != changedFileID ||
		metadata["source"] != source ||
		metadata["missing"] != missing {
		t.Fatalf("metadata = %+v", metadata)
	}
}
