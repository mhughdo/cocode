package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestWorkspaceQueriesCRUD(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	if err := Apply(context.Background(), database, Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	queries := dbgen.New(database)
	created, err := queries.CreateWorkspace(context.Background(), dbgen.CreateWorkspaceParams{
		ID:           "workspace_1",
		Name:         "cocode",
		RootPath:     "/tmp/cocode",
		SettingsJson: "{}",
		CreatedAt:    "2026-05-03T00:00:00Z",
		UpdatedAt:    "2026-05-03T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if created.ID != "workspace_1" || created.Name != "cocode" || created.RootPath != "/tmp/cocode" {
		t.Fatalf("CreateWorkspace() = %+v", created)
	}

	got, err := queries.GetWorkspace(context.Background(), "workspace_1")
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("GetWorkspace() ID = %q, want %q", got.ID, created.ID)
	}

	byRoot, err := queries.GetWorkspaceByRootPath(context.Background(), "/tmp/cocode")
	if err != nil {
		t.Fatalf("GetWorkspaceByRootPath() error = %v", err)
	}
	if byRoot.ID != created.ID {
		t.Fatalf("GetWorkspaceByRootPath() ID = %q, want %q", byRoot.ID, created.ID)
	}

	if _, err := queries.CreateWorkspace(context.Background(), dbgen.CreateWorkspaceParams{
		ID:           "workspace_2",
		Name:         "alpha",
		RootPath:     "/tmp/alpha",
		SettingsJson: "{}",
		CreatedAt:    "2026-05-03T00:01:00Z",
		UpdatedAt:    "2026-05-03T00:01:00Z",
	}); err != nil {
		t.Fatalf("CreateWorkspace(workspace_2) error = %v", err)
	}

	workspaces, err := queries.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces() error = %v", err)
	}
	if len(workspaces) != 2 {
		t.Fatalf("ListWorkspaces() len = %d, want 2", len(workspaces))
	}
	if workspaces[0].ID != "workspace_2" || workspaces[1].ID != "workspace_1" {
		t.Fatalf("ListWorkspaces() order = [%s, %s], want [workspace_2, workspace_1]", workspaces[0].ID, workspaces[1].ID)
	}

	updated, err := queries.UpdateWorkspace(context.Background(), dbgen.UpdateWorkspaceParams{
		ID:            "workspace_1",
		Name:          "cocode desktop",
		DefaultRepoID: sql.NullString{String: "repo_1", Valid: true},
		SettingsJson:  `{"theme":"system"}`,
		UpdatedAt:     "2026-05-03T00:02:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateWorkspace() error = %v", err)
	}
	if updated.Name != "cocode desktop" || updated.DefaultRepoID.String != "repo_1" || !updated.DefaultRepoID.Valid {
		t.Fatalf("UpdateWorkspace() = %+v", updated)
	}

	if err := queries.DeleteWorkspace(context.Background(), "workspace_1"); err != nil {
		t.Fatalf("DeleteWorkspace() error = %v", err)
	}
	if _, err := queries.GetWorkspace(context.Background(), "workspace_1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetWorkspace(deleted) error = %v, want sql.ErrNoRows", err)
	}
}
