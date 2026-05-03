package githubpr

import (
	"errors"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/diffparse"
)

func TestMapDiffLineAnchorsAddedDeletedContextAndMultiHunkLines(t *testing.T) {
	files := mustParseDiff(t, mapperFixtureDiff)
	tests := []struct {
		name     string
		request  DiffAnchorRequest
		wantPath string
		wantLine int
		wantSide string
		wantPos  int
	}{
		{
			name:     "added line on right side",
			request:  DiffAnchorRequest{Path: "app/auth.go", Line: 12, Side: SideRight},
			wantPath: "app/auth.go",
			wantLine: 12,
			wantSide: SideRight,
			wantPos:  3,
		},
		{
			name:     "deleted line on left side",
			request:  DiffAnchorRequest{Path: "app/auth.go", Line: 31, Side: SideLeft},
			wantPath: "app/auth.go",
			wantLine: 31,
			wantSide: SideLeft,
			wantPos:  8,
		},
		{
			name:     "context line can anchor on right side",
			request:  DiffAnchorRequest{Path: "app/auth.go", Line: 14, Side: SideRight},
			wantPath: "app/auth.go",
			wantLine: 14,
			wantSide: SideRight,
			wantPos:  5,
		},
		{
			name:     "second hunk position continues from first hunk",
			request:  DiffAnchorRequest{Path: "app/auth.go", Line: 32, Side: SideRight},
			wantPath: "app/auth.go",
			wantLine: 32,
			wantSide: SideRight,
			wantPos:  9,
		},
		{
			name:     "added file line",
			request:  DiffAnchorRequest{Path: "app/new.go", Line: 2, Side: SideRight},
			wantPath: "app/new.go",
			wantLine: 2,
			wantSide: SideRight,
			wantPos:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			anchor, err := MapDiffLine(files, tt.request)
			if err != nil {
				t.Fatalf("MapDiffLine() error = %v", err)
			}
			if anchor.Path != tt.wantPath ||
				anchor.Line != tt.wantLine ||
				anchor.Side != tt.wantSide ||
				anchor.Position != tt.wantPos {
				t.Fatalf("anchor = %+v", anchor)
			}
		})
	}
}

func TestMapDiffLineHandlesRemovedFileAndUnknownSide(t *testing.T) {
	files := mustParseDiff(t, mapperFixtureDiff)

	deleted, err := MapDiffLine(files, DiffAnchorRequest{Path: "app/removed.go", Line: 2, Side: SideLeft})
	if err != nil {
		t.Fatalf("MapDiffLine(deleted) error = %v", err)
	}
	if deleted.Path != "app/removed.go" || deleted.Side != SideLeft || deleted.Position != 2 {
		t.Fatalf("deleted anchor = %+v", deleted)
	}

	unknown, err := MapDiffLine(files, DiffAnchorRequest{Path: "app/auth.go", Line: 12, Side: SideUnknown})
	if err != nil {
		t.Fatalf("MapDiffLine(unknown) error = %v", err)
	}
	if unknown.Side != SideRight || unknown.Position != 3 {
		t.Fatalf("unknown anchor = %+v", unknown)
	}
}

func TestMapDiffLineReportsUnanchoredLines(t *testing.T) {
	files := mustParseDiff(t, mapperFixtureDiff)
	_, err := MapDiffLine(files, DiffAnchorRequest{Path: "app/auth.go", Line: 99, Side: SideRight})
	if !errors.Is(err, ErrDiffAnchorMissing) {
		t.Fatalf("MapDiffLine() error = %v, want ErrDiffAnchorMissing", err)
	}
	_, err = MapDiffLine(files, DiffAnchorRequest{Path: "../outside.go", Line: 1, Side: SideRight})
	if !errors.Is(err, ErrDiffAnchorMissing) {
		t.Fatalf("MapDiffLine() error = %v, want ErrDiffAnchorMissing", err)
	}
	_, err = MapDiffLine(files, DiffAnchorRequest{Path: "app/auth.go", Line: 1, Side: "middle"})
	if !errors.Is(err, ErrInvalidDiffAnchor) {
		t.Fatalf("MapDiffLine() error = %v, want ErrInvalidDiffAnchor", err)
	}
}

func mustParseDiff(t *testing.T, patch string) []diffparse.File {
	t.Helper()
	files, err := diffparse.Parse(patch)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return files
}

const mapperFixtureDiff = `diff --git a/app/auth.go b/app/auth.go
--- a/app/auth.go
+++ b/app/auth.go
@@ -10,5 +10,6 @@ func handler() {
  before()
  requireUser()
+ requireAdmin()
  updateSettings()
  after()
@@ -30,4 +31,4 @@ func helper() {
  keepA()
- oldCheck()
+ newCheck()
  keepB()
diff --git a/app/new.go b/app/new.go
new file mode 100644
--- /dev/null
+++ b/app/new.go
@@ -0,0 +1,2 @@
+package app
+func NewThing() {}
diff --git a/app/removed.go b/app/removed.go
deleted file mode 100644
--- a/app/removed.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package app
-func Removed() {}
-func Gone() {}
`
