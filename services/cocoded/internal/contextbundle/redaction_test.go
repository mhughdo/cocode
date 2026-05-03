package contextbundle

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
)

func TestRedactBundleRedactsSecretFixtures(t *testing.T) {
	t.Parallel()

	bundle := redactionTestBundle([]Item{
		{
			ID:              "item_secret",
			ContextBundleID: "bundle_1",
			Kind:            ItemFullFile,
			Path:            ".env",
			StartLine:       1,
			EndLine:         4,
			Title:           "OPENAI_API_KEY=sk-title-secret-123456789012345",
			Content: strings.Join([]string{
				"OPENAI_API_KEY=sk-testsecret12345678901234567890",
				"Authorization: Bearer abcdefghijklmnopqrstuvwxyz",
				"GITHUB_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz123456",
				"JWT=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signatureabcdef",
				"FROM_ENV=env-secret-value-12345",
			}, "\n"),
			Metadata: []byte(`{"token":"env-secret-value-12345"}`),
		},
		{
			ID:              "item_key",
			ContextBundleID: "bundle_1",
			Kind:            ItemFileSlice,
			Path:            "deploy/key.pem",
			StartLine:       1,
			EndLine:         3,
			Content:         "-----BEGIN PRIVATE KEY-----\nabc123\n-----END PRIVATE KEY-----\n",
			Metadata:        []byte("{}"),
		},
		{
			ID:              "item_safe",
			ContextBundleID: "bundle_1",
			Kind:            ItemChangedHunk,
			Path:            "src/auth.ts",
			StartLine:       1,
			EndLine:         1,
			Content:         "const timeout = 30\n",
			Metadata:        []byte("{}"),
		},
	})

	redacted, report, err := RedactBundle(bundle, RedactionOptions{
		EnvValues: map[string]string{
			"EXPLICIT_SECRET": "env-secret-value-12345",
			"SHORT_SECRET":    "short",
		},
	})
	if err != nil {
		t.Fatalf("RedactBundle() error = %v", err)
	}
	if report.RedactionCount < 6 || len(report.Items) != 2 {
		t.Fatalf("report = %+v", report)
	}
	first := redacted.Items[0]
	for _, secret := range []string{
		"sk-testsecret12345678901234567890",
		"abcdefghijklmnopqrstuvwxyz",
		"ghp_abcdefghijklmnopqrstuvwxyz123456",
		"eyJhbGciOiJIUzI1NiJ9",
		"env-secret-value-12345",
	} {
		if strings.Contains(first.Content, secret) || strings.Contains(first.Title, secret) || strings.Contains(string(first.Metadata), secret) {
			t.Fatalf("secret %q leaked in item %+v metadata %s", secret, first, string(first.Metadata))
		}
	}
	if !strings.Contains(first.Content, "Bearer [REDACTED]") ||
		!strings.Contains(first.Content, "OPENAI_API_KEY=[REDACTED]") ||
		!strings.Contains(string(first.Metadata), redactionPlaceholder) {
		t.Fatalf("redacted item = %+v metadata %s", first, string(first.Metadata))
	}
	second := redacted.Items[1]
	if strings.Contains(second.Content, "PRIVATE KEY") || second.Content != redactionPlaceholder+"\n" {
		t.Fatalf("private key item content = %q", second.Content)
	}
	if redacted.TokenEstimate == 0 || redacted.ItemCount != 3 {
		t.Fatalf("redacted bundle totals = %+v", redacted)
	}
}

func TestRedactContextItemsValidatesInputsAndJSONMetadata(t *testing.T) {
	t.Parallel()

	if _, _, err := RedactContextItems("", nil, RedactionOptions{}); err == nil || !strings.Contains(err.Error(), "bundle") {
		t.Fatalf("RedactContextItems(empty bundle) error = %v", err)
	}
	_, _, err := RedactContextItems("bundle_1", []Item{{
		ID:              "item_bad_metadata",
		ContextBundleID: "bundle_1",
		Kind:            ItemFullFile,
		Content:         "OPENAI_API_KEY=sk-testsecret12345678901234567890",
		Metadata:        []byte(`{"token":"sk-testsecret12345678901234567890}`),
	}}, RedactionOptions{})
	if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("RedactContextItems(bad metadata) error = %v", err)
	}
}

func TestSaveRedactionReportArtifact(t *testing.T) {
	t.Parallel()

	queries := contextBundleTestQueries(t)
	store, err := artifact.New(filepath.Join(t.TempDir(), "artifacts"), queries)
	if err != nil {
		t.Fatalf("artifact.New() error = %v", err)
	}
	report := RedactionReport{
		BundleID:       "bundle_1",
		RedactionCount: 2,
		Items: []RedactionReportItem{{
			ItemID:         "item_secret",
			Kind:           ItemFullFile,
			Path:           ".env",
			RedactionCount: 2,
			Detectors:      map[string]int{"openai_key": 1, "env_value:OPENAI_API_KEY": 1},
		}},
	}

	saved, err := SaveRedactionReportArtifact(context.Background(), store, RedactionArtifactParams{
		ID:              "artifact_redaction_report",
		WorkspaceID:     "workspace_1",
		ReviewSessionID: "review_session_1",
		BundleID:        "bundle_1",
		CreatedAt:       "2026-05-03T00:20:00Z",
	}, report)
	if err != nil {
		t.Fatalf("SaveRedactionReportArtifact() error = %v", err)
	}
	if saved.Kind != "context_redaction_report" ||
		saved.ContentType != "application/json" ||
		saved.ReviewSessionID != (sql.NullString{String: "review_session_1", Valid: true}) {
		t.Fatalf("saved artifact = %+v", saved)
	}
	content, _, err := store.Read(context.Background(), saved.ID)
	if err != nil {
		t.Fatalf("Read(redaction report) error = %v", err)
	}
	var decoded RedactionReport
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatalf("Unmarshal(redaction report) error = %v", err)
	}
	if decoded.RedactionCount != 2 || decoded.Items[0].Detectors["openai_key"] != 1 {
		t.Fatalf("decoded report = %+v", decoded)
	}
}

func redactionTestBundle(items []Item) Bundle {
	return Bundle{
		ID:              "bundle_1",
		ReviewSessionID: "review_session_1",
		Scope:           ScopeReview,
		Policy:          []byte("{}"),
		Items:           items,
	}
}
