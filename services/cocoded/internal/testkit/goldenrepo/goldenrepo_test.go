package goldenrepo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGoldenRepoPathsResolveExpectedFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file string
	}{
		{AuthBug, "apps/api/src/routes/repositories.ts"},
		{WebhookValidation, "apps/api/src/webhooks/stripe.ts"},
		{GeneratedFilesNoise, "services/api/internal/db/dbgen/snapshots.sql.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := Path(t, tt.name)
			if _, err := os.Stat(filepath.Join(path, filepath.FromSlash(tt.file))); err != nil {
				t.Fatalf("fixture file %s: %v", tt.file, err)
			}
		})
	}
}
