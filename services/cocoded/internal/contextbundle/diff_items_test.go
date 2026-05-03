package contextbundle

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/diffparse"
)

func TestBuildDiffContextItemsFromMultipleHunksAndFiles(t *testing.T) {
	t.Parallel()

	diff := `diff --git a/app/main.go b/app/main.go
index 2a1b000..2a1b111 100644
--- a/app/main.go
+++ b/app/main.go
@@ -1,3 +1,4 @@ func main()
 line1
-old2
+new2
+new3
 line3
@@ -20,3 +21,4 @@ func next()
 alpha
+beta
 gamma
 delta
diff --git a/docs/guide.md b/docs/guide.md
index 2222222..3333333 100644
--- a/docs/guide.md
+++ b/docs/guide.md
@@ -4,2 +4,3 @@
 keep
+note
 done`

	files, err := diffparse.Parse(diff)
	if err != nil {
		t.Fatalf("diffparse.Parse() error = %v", err)
	}
	inputs := []DiffContextFile{
		DiffContextFileFromParsed("changed_file_main", "artifact_patch_main", files[0]),
		DiffContextFileFromParsed("changed_file_guide", "artifact_patch_guide", files[1]),
		{
			ChangedFileID: "changed_file_binary",
			Path:          "assets/logo.png",
			Binary:        true,
			Hunks:         files[0].Hunks,
		},
		{
			ChangedFileID: "changed_file_excluded",
			Path:          "generated/api.pb.go",
			Excluded:      true,
			Hunks:         files[0].Hunks,
		},
	}

	items, err := BuildDiffContextItems("bundle_1", inputs)
	if err != nil {
		t.Fatalf("BuildDiffContextItems() error = %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items len = %d, want 3: %+v", len(items), items)
	}
	first := items[0]
	if first.ContextBundleID != "bundle_1" ||
		first.Kind != ItemChangedHunk ||
		first.Path != "app/main.go" ||
		first.StartLine != 1 ||
		first.EndLine != 4 ||
		first.Title != "app/main.go hunk 1: func main()" ||
		!strings.Contains(first.Content, "@@ -1,3 +1,4 @@ func main()") ||
		!strings.Contains(first.Content, "-old2") ||
		!strings.Contains(first.Content, "+new3") {
		t.Fatalf("first item = %+v", first)
	}
	var metadata map[string]any
	if err := json.Unmarshal(first.Metadata, &metadata); err != nil {
		t.Fatalf("Unmarshal(metadata) error = %v", err)
	}
	if metadata["changed_file_id"] != "changed_file_main" ||
		metadata["patch_artifact_id"] != "artifact_patch_main" ||
		metadata["section"] != "func main()" ||
		metadata["hunk_index"].(float64) != 0 {
		t.Fatalf("metadata = %+v", metadata)
	}

	second := items[1]
	if second.Path != "app/main.go" || second.StartLine != 21 || second.EndLine != 24 {
		t.Fatalf("second item range = %+v", second)
	}
	third := items[2]
	if third.Path != "docs/guide.md" || third.StartLine != 4 || third.EndLine != 6 {
		t.Fatalf("third item range = %+v", third)
	}
	if items[0].ID == items[1].ID || items[1].ID == items[2].ID {
		t.Fatalf("item ids should be stable and unique: %+v", items)
	}
}

func TestBuildDiffContextItemsHandlesDeletedOnlyHunkAndInvalidInput(t *testing.T) {
	t.Parallel()

	diff := `diff --git a/old.txt b/old.txt
deleted file mode 100644
index 2222222..0000000
--- a/old.txt
+++ /dev/null
@@ -10,2 +0,0 @@ cleanup
-gone
-away`
	files, err := diffparse.Parse(diff)
	if err != nil {
		t.Fatalf("diffparse.Parse() error = %v", err)
	}
	items, err := BuildDiffContextItems("bundle_1", []DiffContextFile{
		DiffContextFileFromParsed("changed_file_deleted", "artifact_patch_deleted", files[0]),
	})
	if err != nil {
		t.Fatalf("BuildDiffContextItems(deleted) error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("deleted items len = %d, want 1", len(items))
	}
	if items[0].Path != "old.txt" || items[0].StartLine != 10 || items[0].EndLine != 11 || !strings.Contains(items[0].Content, "-gone") {
		t.Fatalf("deleted item = %+v", items[0])
	}

	if _, err := BuildDiffContextItems("", []DiffContextFile{}); err == nil || !strings.Contains(err.Error(), "bundle id") {
		t.Fatalf("BuildDiffContextItems(empty bundle) error = %v", err)
	}
	if _, err := BuildDiffContextItems("bundle_1", []DiffContextFile{{Hunks: files[0].Hunks}}); err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("BuildDiffContextItems(missing path) error = %v", err)
	}
}
