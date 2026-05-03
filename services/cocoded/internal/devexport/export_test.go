package devexport

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestExportDumpsRowsAndRedactsSecrets(t *testing.T) {
	t.Parallel()

	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	queries := dbgen.New(database)
	if _, err := queries.CreateWorkspace(context.Background(), dbgen.CreateWorkspaceParams{
		ID:           "workspace_1",
		Name:         "cocode",
		RootPath:     "/tmp/cocode",
		SettingsJson: `{"api_token":"secret","theme":"system","nested":{"password":"pw"}}`,
		CreatedAt:    "2026-05-03T00:00:00Z",
		UpdatedAt:    "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, err := database.ExecContext(context.Background(), `
INSERT INTO credential_refs(id, kind, display_name, storage_provider, storage_key, metadata_json, created_at, updated_at)
VALUES ('credential_1', 'github', 'GitHub token', 'safe_storage', 'secret-storage-key', '{"keep":"value","credential_token":"hidden"}', '2026-05-03T00:01:00Z', '2026-05-03T00:01:00Z')`); err != nil {
		t.Fatalf("insert credential_refs: %v", err)
	}

	exportedAt := time.Date(2026, 5, 3, 0, 2, 0, 0, time.UTC)
	dump, err := Export(context.Background(), database, exportedAt)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if dump.ExportedAt != "2026-05-03T00:02:00Z" {
		t.Fatalf("ExportedAt = %q", dump.ExportedAt)
	}

	workspaces := tableByName(t, dump, "workspaces")
	if len(workspaces.Rows) != 1 {
		t.Fatalf("workspaces rows = %d, want 1", len(workspaces.Rows))
	}
	settings, ok := workspaces.Rows[0]["settings_json"].(map[string]any)
	if !ok {
		t.Fatalf("settings_json = %T, want map", workspaces.Rows[0]["settings_json"])
	}
	if settings["api_token"] != redacted || settings["theme"] != "system" {
		t.Fatalf("settings_json redaction = %+v", settings)
	}
	nested, ok := settings["nested"].(map[string]any)
	if !ok || nested["password"] != redacted {
		t.Fatalf("nested redaction = %+v", settings["nested"])
	}

	credentials := tableByName(t, dump, "credential_refs")
	if len(credentials.Rows) != 1 {
		t.Fatalf("credential_refs rows = %d, want 1", len(credentials.Rows))
	}
	if credentials.Rows[0]["storage_key"] != redacted {
		t.Fatalf("storage_key = %v, want redacted", credentials.Rows[0]["storage_key"])
	}
	credentialMetadata, ok := credentials.Rows[0]["metadata_json"].(map[string]any)
	if !ok {
		t.Fatalf("metadata_json = %T, want map", credentials.Rows[0]["metadata_json"])
	}
	if credentialMetadata["credential_token"] != redacted || credentialMetadata["keep"] != "value" {
		t.Fatalf("credential metadata redaction = %+v", credentialMetadata)
	}

	for _, table := range dump.Tables {
		if table.Name == "finding_search_data" || table.Name == "evidence_search_data" {
			t.Fatalf("dump includes FTS shadow table %q", table.Name)
		}
	}
}

func TestOpenReadOnlyRejectsMissingPath(t *testing.T) {
	t.Parallel()

	if _, err := OpenReadOnly(context.Background(), ""); err == nil {
		t.Fatal("OpenReadOnly(empty) error = nil, want error")
	}
}

func TestOpenReadOnlyOpensExistingDatabaseWithoutWriteAccess(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "cocode.db")
	writable, err := db.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.Apply(context.Background(), writable, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if err := writable.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	readonly, err := OpenReadOnly(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly() error = %v", err)
	}
	defer readonly.Close()

	if _, err := Export(context.Background(), readonly, time.Now()); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if _, err := readonly.ExecContext(context.Background(), `INSERT INTO workspaces(id, name, root_path, settings_json, created_at, updated_at) VALUES ('workspace_ro', 'readonly', '/tmp/ro', '{}', '', '')`); err == nil {
		t.Fatal("read-only insert error = nil, want error")
	}
}

func tableByName(t *testing.T, dump Dump, name string) TableDump {
	t.Helper()

	for _, table := range dump.Tables {
		if table.Name == name {
			return table
		}
	}
	t.Fatalf("table %q not found in dump", name)
	return TableDump{}
}
