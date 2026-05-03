package diffparse

import (
	"reflect"
	"testing"
)

func TestParseModifiedDiffWithMultipleHunks(t *testing.T) {
	t.Parallel()

	diff := `diff --git a/app/main.go b/app/main.go
index 2a1b000..2a1b111 100644
--- a/app/main.go
+++ b/app/main.go
@@ -1,3 +1,4 @@
 line1
-old2
+new2
+new3
 line3
@@ -20,3 +21,4 @@ func next()
 alpha
+beta
 gamma
 delta`

	files, err := Parse(diff)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Parse() len = %d, want 1", len(files))
	}

	file := files[0]
	if file.Path != "app/main.go" || file.OldPath != "" || file.Status != StatusModified {
		t.Fatalf("file identity = path %q old %q status %q", file.Path, file.OldPath, file.Status)
	}
	if file.Additions != 3 || file.Deletions != 1 {
		t.Fatalf("additions/deletions = %d/%d, want 3/1", file.Additions, file.Deletions)
	}
	wantRanges := []LineRange{{Start: 2, End: 3}, {Start: 22, End: 22}}
	if !reflect.DeepEqual(file.LineRanges, wantRanges) {
		t.Fatalf("LineRanges = %+v, want %+v", file.LineRanges, wantRanges)
	}
	if got, err := file.LineRangesJSON(); err != nil || got != `[[2,3],[22,22]]` {
		t.Fatalf("LineRangesJSON() = %q, %v; want [[2,3],[22,22]], nil", got, err)
	}
	if len(file.Hunks) != 2 {
		t.Fatalf("Hunks len = %d, want 2", len(file.Hunks))
	}
	if file.Hunks[1].OldStart != 20 || file.Hunks[1].NewStart != 21 || file.Hunks[1].Section != "func next()" {
		t.Fatalf("second hunk = %+v", file.Hunks[1])
	}
}

func TestParseAddedAndDeletedFiles(t *testing.T) {
	t.Parallel()

	diff := `diff --git a/docs/new file.md b/docs/new file.md
new file mode 100644
index 0000000..1111111
--- /dev/null
+++ b/docs/new file.md
@@ -0,0 +1,2 @@
+one
+two
diff --git a/old.txt b/old.txt
deleted file mode 100644
index 2222222..0000000
--- a/old.txt
+++ /dev/null
@@ -1,2 +0,0 @@
-gone
-away`

	files, err := Parse(diff)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("Parse() len = %d, want 2", len(files))
	}

	added := files[0]
	if added.Path != "docs/new file.md" || added.Status != StatusAdded || added.OldPath != "" {
		t.Fatalf("added file = %+v", added)
	}
	if added.Additions != 2 || added.Deletions != 0 {
		t.Fatalf("added additions/deletions = %d/%d, want 2/0", added.Additions, added.Deletions)
	}
	if !reflect.DeepEqual(added.LineRanges, []LineRange{{Start: 1, End: 2}}) {
		t.Fatalf("added ranges = %+v", added.LineRanges)
	}

	deleted := files[1]
	if deleted.Path != "old.txt" || deleted.Status != StatusDeleted || deleted.OldPath != "" {
		t.Fatalf("deleted file = %+v", deleted)
	}
	if deleted.Additions != 0 || deleted.Deletions != 2 {
		t.Fatalf("deleted additions/deletions = %d/%d, want 0/2", deleted.Additions, deleted.Deletions)
	}
	if len(deleted.LineRanges) != 0 {
		t.Fatalf("deleted ranges = %+v, want empty", deleted.LineRanges)
	}
}

func TestParseRenameWithTextChanges(t *testing.T) {
	t.Parallel()

	diff := `diff --git a/src/old name.go b/src/new name.go
similarity index 72%
rename from src/old name.go
rename to src/new name.go
index 3333333..4444444 100644
--- a/src/old name.go
+++ b/src/new name.go
@@ -3,2 +3,3 @@
 keep
+added after rename
 still here`

	files, err := Parse(diff)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Parse() len = %d, want 1", len(files))
	}
	file := files[0]
	if file.Path != "src/new name.go" || file.OldPath != "src/old name.go" || file.Status != StatusRenamed {
		t.Fatalf("renamed file = %+v", file)
	}
	if file.Additions != 1 || file.Deletions != 0 {
		t.Fatalf("additions/deletions = %d/%d, want 1/0", file.Additions, file.Deletions)
	}
	if !reflect.DeepEqual(file.LineRanges, []LineRange{{Start: 4, End: 4}}) {
		t.Fatalf("LineRanges = %+v", file.LineRanges)
	}
}

func TestParseBinaryDiff(t *testing.T) {
	t.Parallel()

	diff := `diff --git a/assets/logo.png b/assets/logo.png
index 5555555..6666666 100644
Binary files a/assets/logo.png and b/assets/logo.png differ`

	files, err := Parse(diff)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Parse() len = %d, want 1", len(files))
	}
	file := files[0]
	if file.Path != "assets/logo.png" || file.Status != StatusBinary || !file.Binary {
		t.Fatalf("binary file = %+v", file)
	}
	if len(file.Hunks) != 0 || len(file.LineRanges) != 0 {
		t.Fatalf("binary hunks/ranges = %d/%d, want 0/0", len(file.Hunks), len(file.LineRanges))
	}
}

func TestParseNoNewlineMarker(t *testing.T) {
	t.Parallel()

	diff := `diff --git a/README.md b/README.md
index 7777777..8888888 100644
--- a/README.md
+++ b/README.md
@@ -1 +1 @@
-old
\ No newline at end of file
+new
\ No newline at end of file`

	files, err := Parse(diff)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Parse() len = %d, want 1", len(files))
	}
	file := files[0]
	if file.Additions != 1 || file.Deletions != 1 {
		t.Fatalf("additions/deletions = %d/%d, want 1/1", file.Additions, file.Deletions)
	}
	if !reflect.DeepEqual(file.LineRanges, []LineRange{{Start: 1, End: 1}}) {
		t.Fatalf("LineRanges = %+v", file.LineRanges)
	}
	if len(file.Hunks) != 1 || len(file.Hunks[0].Lines) != 2 {
		t.Fatalf("hunk lines = %+v", file.Hunks)
	}
	if !file.Hunks[0].Lines[0].NoNewlineAtEOF || !file.Hunks[0].Lines[1].NoNewlineAtEOF {
		t.Fatalf("NoNewlineAtEOF flags = %+v", file.Hunks[0].Lines)
	}
}
