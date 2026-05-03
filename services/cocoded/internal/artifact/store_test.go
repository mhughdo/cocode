package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestStoreSaveReadDelete(t *testing.T) {
	t.Parallel()

	queries := artifactTestQueries(t)
	store, err := New(filepath.Join(t.TempDir(), "artifacts"), queries)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	content := []byte("agent output\nwith a useful trace")
	artifact, err := store.Save(context.Background(), SaveParams{
		ID:              "artifact_1",
		WorkspaceID:     "workspace_1",
		ReviewSessionID: sql.NullString{String: "review_session_1", Valid: true},
		Kind:            "agent_stdout",
		RelativePath:    "review_session_1/stdout.txt",
		ContentType:     "text/plain",
		MetadataJSON:    `{"agent":"codex"}`,
		CreatedAt:       "2026-05-03T00:20:00Z",
	}, content)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	digest := sha256.Sum256(content)
	if artifact.SizeBytes != int64(len(content)) {
		t.Fatalf("artifact SizeBytes = %d, want %d", artifact.SizeBytes, len(content))
	}
	if artifact.Sha256.String != hex.EncodeToString(digest[:]) {
		t.Fatalf("artifact Sha256 = %q, want %q", artifact.Sha256.String, hex.EncodeToString(digest[:]))
	}

	readContent, readArtifact, err := store.Read(context.Background(), "artifact_1")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !bytes.Equal(readContent, content) {
		t.Fatalf("Read() content = %q, want %q", string(readContent), string(content))
	}
	if readArtifact.ID != artifact.ID {
		t.Fatalf("Read() artifact ID = %q, want %q", readArtifact.ID, artifact.ID)
	}

	workspaceArtifacts, err := queries.ListArtifactsByWorkspace(context.Background(), "workspace_1")
	if err != nil {
		t.Fatalf("ListArtifactsByWorkspace() error = %v", err)
	}
	if len(workspaceArtifacts) != 1 || workspaceArtifacts[0].ID != "artifact_1" {
		t.Fatalf("ListArtifactsByWorkspace() = %+v", workspaceArtifacts)
	}

	sessionArtifacts, err := queries.ListArtifactsByReviewSession(context.Background(), sql.NullString{String: "review_session_1", Valid: true})
	if err != nil {
		t.Fatalf("ListArtifactsByReviewSession() error = %v", err)
	}
	if len(sessionArtifacts) != 1 || sessionArtifacts[0].ID != "artifact_1" {
		t.Fatalf("ListArtifactsByReviewSession() = %+v", sessionArtifacts)
	}

	target, err := store.pathFor("workspace_1", "review_session_1/stdout.txt")
	if err != nil {
		t.Fatalf("pathFor() error = %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("stat artifact file: %v", err)
	}

	if err := store.Delete(context.Background(), "artifact_1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat deleted artifact error = %v, want os.ErrNotExist", err)
	}
	if _, err := queries.GetArtifact(context.Background(), "artifact_1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetArtifact(deleted) error = %v, want sql.ErrNoRows", err)
	}
}

func TestStoreRejectsPathTraversal(t *testing.T) {
	t.Parallel()

	queries := artifactTestQueries(t)
	root := filepath.Join(t.TempDir(), "artifacts")
	store, err := New(root, queries)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, params := range []SaveParams{
		{ID: "artifact_parent", WorkspaceID: "workspace_1", Kind: "log", RelativePath: "../outside.txt"},
		{ID: "artifact_absolute", WorkspaceID: "workspace_1", Kind: "log", RelativePath: filepath.Join(root, "outside.txt")},
		{ID: "artifact_workspace", WorkspaceID: "../workspace_1", Kind: "log", RelativePath: "safe.txt"},
	} {
		if _, err := store.Save(context.Background(), params, []byte("nope")); err == nil {
			t.Fatalf("Save(%s) error = nil, want traversal error", params.ID)
		}
	}

	artifacts, err := queries.ListArtifactsByWorkspace(context.Background(), "workspace_1")
	if err != nil {
		t.Fatalf("ListArtifactsByWorkspace() error = %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("ListArtifactsByWorkspace() len = %d, want 0", len(artifacts))
	}
}

func TestStoreRejectsSymlinkEscapes(t *testing.T) {
	t.Parallel()

	queries := artifactTestQueries(t)
	root := filepath.Join(t.TempDir(), "artifacts")
	store, err := New(root, queries)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	outside := t.TempDir()
	workspaceRoot := filepath.Join(root, "workspace_1")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(workspaceRoot, "out")); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}
	if _, err := store.Save(context.Background(), SaveParams{
		ID:           "artifact_symlink_parent",
		WorkspaceID:  "workspace_1",
		Kind:         "log",
		RelativePath: "out/escape.txt",
	}, []byte("nope")); err == nil {
		t.Fatal("Save() error = nil, want symlink parent escape error")
	}

	content := []byte("safe")
	artifact, err := store.Save(context.Background(), SaveParams{
		ID:           "artifact_safe",
		WorkspaceID:  "workspace_1",
		Kind:         "log",
		RelativePath: "safe.txt",
	}, content)
	if err != nil {
		t.Fatalf("Save(safe) error = %v", err)
	}
	target, err := store.pathFor("workspace_1", artifact.RelativePath)
	if err != nil {
		t.Fatalf("pathFor() error = %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove safe target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("write outside secret: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), target); err != nil {
		t.Fatalf("symlink file: %v", err)
	}
	if _, _, err := store.Read(context.Background(), "artifact_safe"); err == nil {
		t.Fatal("Read() error = nil, want symlink file escape error")
	}
}

func artifactTestQueries(t *testing.T) *dbgen.Queries {
	t.Helper()

	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	queries := dbgen.New(database)
	if _, err := queries.CreateWorkspace(context.Background(), dbgen.CreateWorkspaceParams{
		ID:           "workspace_1",
		Name:         "cocode",
		RootPath:     "/tmp/cocode",
		SettingsJson: "{}",
		CreatedAt:    "2026-05-03T00:00:00Z",
		UpdatedAt:    "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, err := queries.CreateRepository(context.Background(), dbgen.CreateRepositoryParams{
		ID:            "repo_1",
		WorkspaceID:   "workspace_1",
		Name:          "cocode",
		LocalPath:     "/tmp/cocode",
		DefaultBranch: sql.NullString{String: "main", Valid: true},
		CreatedAt:     "2026-05-03T00:01:00Z",
		UpdatedAt:     "2026-05-03T00:01:00Z",
	}); err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
	if _, err := queries.CreatePullRequestSnapshot(context.Background(), dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_1",
		RepositoryID: "repo_1",
		SourceType:   "local_changes",
		MetadataJson: "{}",
		CreatedAt:    "2026-05-03T00:02:00Z",
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}
	if _, err := queries.CreateReviewSession(context.Background(), dbgen.CreateReviewSessionParams{
		ID:                  "review_session_1",
		WorkspaceID:         "workspace_1",
		RepositoryID:        "repo_1",
		SnapshotID:          "snapshot_1",
		Title:               "Review cocode",
		Status:              "draft",
		ReviewDepth:         "standard",
		RuntimeLimitSeconds: 1800,
		ContextPolicyJson:   "{}",
		CreatedAt:           "2026-05-03T00:03:00Z",
		UpdatedAt:           "2026-05-03T00:03:00Z",
	}); err != nil {
		t.Fatalf("CreateReviewSession() error = %v", err)
	}

	return queries
}
