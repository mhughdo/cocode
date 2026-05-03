package contextbundle

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/diffparse"
)

func TestBuildChangedFileContentItemsFullFileAndSlices(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRepoFile(t, root, "src/small.go", "package src\n\nfunc small() {}\n")
	writeRepoFile(t, root, "src/large.go", strings.Join([]string{
		"line1", "line2", "line3", "line4", "line5",
		"line6", "line7", "line8", "line9", "line10",
	}, "\n")+"\n")

	items, err := BuildChangedFileContentItems(FileContextOptions{
		BundleID:         "bundle_1",
		RepoRoot:         root,
		ContextLines:     1,
		MaxFullFileBytes: 32,
		MaxSliceBytes:    256,
		MaxTotalBytes:    4096,
	}, []ChangedFileContentInput{
		{
			ChangedFileID: "changed_small",
			Path:          "src/small.go",
			Status:        string(diffparse.StatusModified),
			LineRanges:    []diffparse.LineRange{{Start: 3, End: 3}},
		},
		{
			ChangedFileID: "changed_large",
			Path:          "src/large.go",
			Status:        string(diffparse.StatusModified),
			LineRanges: []diffparse.LineRange{
				{Start: 4, End: 4},
				{Start: 8, End: 9},
			},
		},
		{
			ChangedFileID: "changed_excluded",
			Path:          "src/small.go",
			Excluded:      true,
			LineRanges:    []diffparse.LineRange{{Start: 1, End: 1}},
		},
		{
			ChangedFileID: "changed_binary",
			Path:          "src/small.go",
			Binary:        true,
			LineRanges:    []diffparse.LineRange{{Start: 1, End: 1}},
		},
	})
	if err != nil {
		t.Fatalf("BuildChangedFileContentItems() error = %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items len = %d, want 3: %+v", len(items), items)
	}

	full := items[0]
	if full.Kind != ItemFullFile ||
		full.Path != "src/small.go" ||
		full.StartLine != 1 ||
		full.EndLine != 3 ||
		!strings.Contains(full.Content, "func small()") ||
		full.TokenEstimate == 0 {
		t.Fatalf("full item = %+v", full)
	}
	assertFileMetadata(t, full.Metadata, "changed_small", "full_file", false)

	firstSlice := items[1]
	if firstSlice.Kind != ItemFileSlice ||
		firstSlice.Path != "src/large.go" ||
		firstSlice.StartLine != 3 ||
		firstSlice.EndLine != 5 ||
		!strings.Contains(firstSlice.Content, "3: line3") ||
		!strings.Contains(firstSlice.Content, "5: line5") ||
		strings.Contains(firstSlice.Content, "2: line2") {
		t.Fatalf("first slice = %+v", firstSlice)
	}

	secondSlice := items[2]
	if secondSlice.StartLine != 7 ||
		secondSlice.EndLine != 10 ||
		!strings.Contains(secondSlice.Content, "7: line7") ||
		!strings.Contains(secondSlice.Content, "10: line10") {
		t.Fatalf("second slice = %+v", secondSlice)
	}
	assertFileMetadata(t, secondSlice.Metadata, "changed_large", "file_slice", false)
	if items[1].ID == items[2].ID {
		t.Fatalf("slice item IDs should differ: %+v", items)
	}
}

func TestBuildChangedFileContentItemsBudgetsAndUnsafePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRepoFile(t, root, "src/large.go", strings.Repeat("abcdef\n", 20))

	items, err := BuildChangedFileContentItems(FileContextOptions{
		BundleID:         "bundle_1",
		RepoRoot:         root,
		ContextLines:     1,
		MaxFullFileBytes: 8,
		MaxSliceBytes:    20,
		MaxTotalBytes:    20,
		MaxItems:         4,
	}, []ChangedFileContentInput{{
		ChangedFileID: "changed_large",
		Path:          "src/large.go",
		Status:        string(diffparse.StatusModified),
		LineRanges:    []diffparse.LineRange{{Start: 8, End: 8}},
	}})
	if err != nil {
		t.Fatalf("BuildChangedFileContentItems(budget) error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("budget items len = %d, want 1", len(items))
	}
	if int64(len(items[0].Content)) > 20 {
		t.Fatalf("content len = %d, want <= 20", len(items[0].Content))
	}
	assertFileMetadata(t, items[0].Metadata, "changed_large", "file_slice", true)

	if _, err := BuildChangedFileContentItems(FileContextOptions{BundleID: "bundle_1", RepoRoot: root}, []ChangedFileContentInput{{
		ChangedFileID: "changed_escape",
		Path:          "../outside.go",
		Status:        string(diffparse.StatusModified),
		LineRanges:    []diffparse.LineRange{{Start: 1, End: 1}},
	}}); err == nil || !strings.Contains(err.Error(), "escapes repo root") {
		t.Fatalf("BuildChangedFileContentItems(escape) error = %v", err)
	}

	outside := t.TempDir()
	writeRepoFile(t, outside, "secret.go", "package outside\n")
	if err := os.Symlink(filepath.Join(outside, "secret.go"), filepath.Join(root, "link.go")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := BuildChangedFileContentItems(FileContextOptions{BundleID: "bundle_1", RepoRoot: root}, []ChangedFileContentInput{{
		ChangedFileID: "changed_symlink",
		Path:          "link.go",
		Status:        string(diffparse.StatusModified),
		LineRanges:    []diffparse.LineRange{{Start: 1, End: 1}},
	}}); err == nil || !strings.Contains(err.Error(), "escapes repo root") {
		t.Fatalf("BuildChangedFileContentItems(symlink escape) error = %v", err)
	}
}

func TestChangedFileContentInputFromRow(t *testing.T) {
	t.Parallel()

	input, err := ChangedFileContentInputFromRow(dbgen.ChangedFile{
		ID:             "changed_file_1",
		Path:           "app/main.go",
		OldPath:        sql.NullString{String: "app/old.go", Valid: true},
		Status:         "renamed",
		IsBinary:       0,
		IsGenerated:    1,
		IsExcluded:     0,
		LineRangesJson: `[[2,4],[10,10]]`,
	})
	if err != nil {
		t.Fatalf("ChangedFileContentInputFromRow() error = %v", err)
	}
	if input.ChangedFileID != "changed_file_1" ||
		input.OldPath != "app/old.go" ||
		!input.Generated ||
		len(input.LineRanges) != 2 ||
		input.LineRanges[0].Start != 2 ||
		input.LineRanges[1].End != 10 {
		t.Fatalf("input = %+v", input)
	}

	if _, err := ChangedFileContentInputFromRow(dbgen.ChangedFile{LineRangesJson: `[[3,2]]`}); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("ChangedFileContentInputFromRow(invalid) error = %v", err)
	}
}

func assertFileMetadata(t *testing.T, raw json.RawMessage, changedFileID string, source string, truncated bool) {
	t.Helper()

	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatalf("Unmarshal(metadata) error = %v", err)
	}
	if metadata["changed_file_id"] != changedFileID ||
		metadata["source"] != source ||
		metadata["truncated"] != truncated {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func writeRepoFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
