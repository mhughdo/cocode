package devseed

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/search"
)

func TestSeedCreatesRepeatableUIData(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := db.Open(ctx, db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()
	if err := db.Apply(ctx, database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	artifactDir := filepath.Join(t.TempDir(), "artifacts")
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	result, err := Seed(ctx, database, Options{
		ArtifactDir:   artifactDir,
		WorkspaceRoot: filepath.Join(t.TempDir(), "workspace"),
		Now:           now,
	})
	if err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	if result.WorkspaceID != SeedWorkspaceID {
		t.Fatalf("WorkspaceID = %q, want %q", result.WorkspaceID, SeedWorkspaceID)
	}
	if len(result.ReviewSessionIDs) != 2 {
		t.Fatalf("ReviewSessionIDs len = %d, want 2", len(result.ReviewSessionIDs))
	}
	if len(result.FindingIDs) != 3 {
		t.Fatalf("FindingIDs len = %d, want 3", len(result.FindingIDs))
	}

	assertCount(t, database, 2, `SELECT COUNT(*) FROM review_sessions WHERE workspace_id = ?`, SeedWorkspaceID)
	assertCount(t, database, 3, `SELECT COUNT(*) FROM findings WHERE review_session_id = ?`, completedSessionID)
	assertCount(t, database, 6, `SELECT COUNT(*) FROM evidence_items WHERE finding_id LIKE 'seed_finding_%'`)
	assertCount(t, database, 3, `SELECT COUNT(*) FROM evidence_graphs WHERE review_session_id = ?`, completedSessionID)
	assertCount(t, database, 5, `SELECT COUNT(*) FROM agent_runs WHERE review_session_id IN (?, ?)`, completedSessionID, runningSessionID)

	var relativePath string
	if err := database.QueryRowContext(ctx, `SELECT relative_path FROM artifacts WHERE id = ?`, "seed_artifact_diff").Scan(&relativePath); err != nil {
		t.Fatalf("query artifact relative path: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(artifactDir, SeedWorkspaceID, filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatalf("read seed artifact: %v", err)
	}
	if !strings.Contains(string(content), "repositoryService.updateSettings") {
		t.Fatalf("seed artifact content = %q", string(content))
	}

	searchStore, err := search.New(database)
	if err != nil {
		t.Fatalf("search.New() error = %v", err)
	}
	findingIDs, err := searchStore.SearchFindings(ctx, "admin guard", 10)
	if err != nil {
		t.Fatalf("SearchFindings() error = %v", err)
	}
	if !slices.Contains(findingIDs, "seed_finding_auth_guard") {
		t.Fatalf("SearchFindings() ids = %v, want seed_finding_auth_guard", findingIDs)
	}

	if _, err := Seed(ctx, database, Options{
		ArtifactDir:   artifactDir,
		WorkspaceRoot: filepath.Join(t.TempDir(), "workspace"),
		Now:           now,
	}); err != nil {
		t.Fatalf("second Seed() error = %v", err)
	}
	assertCount(t, database, 2, `SELECT COUNT(*) FROM review_sessions WHERE workspace_id = ?`, SeedWorkspaceID)
	assertCount(t, database, 3, `SELECT COUNT(*) FROM findings WHERE review_session_id = ?`, completedSessionID)
	assertCount(t, database, 1, `SELECT COUNT(*) FROM finding_search WHERE finding_id = ?`, "seed_finding_auth_guard")
}

func TestSeedRejectsMissingInputs(t *testing.T) {
	t.Parallel()

	if _, err := Seed(context.Background(), nil, Options{ArtifactDir: t.TempDir()}); err == nil {
		t.Fatal("Seed(nil db) error = nil, want error")
	}

	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()
	if _, err := Seed(context.Background(), database, Options{}); err == nil {
		t.Fatal("Seed(missing artifact dir) error = nil, want error")
	}
}

func assertCount(t *testing.T, database *sql.DB, want int, query string, args ...any) {
	t.Helper()

	var got int
	if err := database.QueryRowContext(context.Background(), query, args...).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("count query %q = %d, want %d", query, got, want)
	}
}
