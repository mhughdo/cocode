package findingengine

import (
	"database/sql"
	"errors"
	"strconv"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestDeduplicateMergesExactFingerprints(t *testing.T) {
	t.Parallel()

	candidates := []dbgen.FindingCandidate{
		candidate("c1", "security", "high", 0.8, "Settings mutation lacks admin guard", "src/new.go", 20, 22, "changed_file_1", "fp_same"),
		candidate("c2", "security", "low", 0.7, "Settings mutation lacks admin guard", "src/new.go", 20, 22, "changed_file_1", "fp_same"),
	}
	clusters := Deduplicate(candidates)
	if len(clusters) != 1 || len(clusters[0].Candidates) != 2 {
		t.Fatalf("clusters = %+v", clusters)
	}
	if Representative(clusters[0]).ID != "c1" {
		t.Fatalf("representative = %+v", Representative(clusters[0]))
	}
}

func TestDeduplicateMergesOverlappingSimilarClaims(t *testing.T) {
	t.Parallel()

	candidates := []dbgen.FindingCandidate{
		candidate("c1", "correctness", "medium", 0.7, "Role cache can be stale during repository update", "src/new.go", 20, 25, "changed_file_1", "fp_one"),
		candidate("c2", "correctness", "medium", 0.6, "repository update reads stale role cache", "src/new.go", 24, 27, "changed_file_1", "fp_two"),
		candidate("c3", "correctness", "medium", 0.6, "repository update reads stale role cache", "src/new.go", 60, 62, "changed_file_1", "fp_three"),
	}
	clusters := Deduplicate(candidates)
	if len(clusters) != 2 {
		t.Fatalf("clusters = %+v", clusters)
	}
	if len(clusters[0].Candidates) != 2 || len(clusters[1].Candidates) != 1 {
		t.Fatalf("clusters = %+v", clusters)
	}
}

func TestDeduplicateMergesNearbySamePathClaims(t *testing.T) {
	t.Parallel()

	candidates := []dbgen.FindingCandidate{
		candidate("c1", "security", "high", 0.9, "Missing admin authorization in cancelSubscription", "src/server.js", 10, 10, "changed_file_1", "fp_one"),
		candidate("c2", "security", "blocker", 0.8, "Admin authorization is missing from cancelSubscription", "src/server.js", 11, 11, "changed_file_1", "fp_two"),
		candidate("c3", "security", "high", 0.8, "Invoice export is missing admin authorization", "src/server.js", 18, 18, "changed_file_1", "fp_three"),
	}
	clusters := Deduplicate(candidates)
	if len(clusters) != 2 {
		t.Fatalf("clusters = %+v", clusters)
	}
	if len(clusters[0].Candidates) != 2 || len(clusters[1].Candidates) != 1 {
		t.Fatalf("clusters = %+v", clusters)
	}
}

func TestDeduplicateDoesNotMergeDissimilarOverlap(t *testing.T) {
	t.Parallel()

	candidates := []dbgen.FindingCandidate{
		candidate("c1", "correctness", "medium", 0.7, "Role cache can be stale during repository update", "src/new.go", 20, 25, "changed_file_1", "fp_one"),
		candidate("c2", "correctness", "medium", 0.6, "Payment amount truncates decimal precision", "src/new.go", 22, 24, "changed_file_1", "fp_two"),
	}
	clusters := Deduplicate(candidates)
	if len(clusters) != 2 {
		t.Fatalf("clusters = %+v", clusters)
	}
}

func TestValidateDedupeResultRejectsInvalidHookOutput(t *testing.T) {
	t.Parallel()

	candidates := []dbgen.FindingCandidate{
		candidate("c1", "correctness", "medium", 0.7, "Role cache can be stale during repository update", "src/new.go", 20, 25, "changed_file_1", "fp_one"),
		candidate("c2", "correctness", "medium", 0.6, "Payment amount truncates decimal precision", "src/new.go", 22, 24, "changed_file_1", "fp_two"),
	}
	tests := []struct {
		name     string
		clusters []Cluster
	}{
		{
			name:     "dropped candidate",
			clusters: []Cluster{{Candidates: candidates[:1]}},
		},
		{
			name:     "duplicate candidate",
			clusters: []Cluster{{Candidates: []dbgen.FindingCandidate{candidates[0], candidates[0]}}, {Candidates: candidates[1:]}},
		},
		{
			name:     "unknown candidate",
			clusters: []Cluster{{Candidates: append(candidates, candidate("c3", "correctness", "low", 0.4, "Unknown", "src/new.go", 30, 31, "changed_file_1", "fp_three"))}},
		},
		{
			name:     "empty cluster",
			clusters: []Cluster{{Candidates: candidates[:1]}, {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateDedupeResult(candidates, tt.clusters)
			if !errors.Is(err, ErrInvalidDedupeResult) {
				t.Fatalf("ValidateDedupeResult() error = %v, want ErrInvalidDedupeResult", err)
			}
		})
	}
}

func TestValidateDedupeResultAcceptsCompletePartition(t *testing.T) {
	t.Parallel()

	candidates := []dbgen.FindingCandidate{
		candidate("c1", "security", "high", 0.8, "Settings mutation lacks admin guard", "src/new.go", 20, 22, "changed_file_1", "fp_same"),
		candidate("c2", "security", "low", 0.7, "Settings mutation lacks admin guard", "src/new.go", 20, 22, "changed_file_1", "fp_same"),
	}
	if err := ValidateDedupeResult(candidates, []Cluster{{Candidates: candidates}}); err != nil {
		t.Fatalf("ValidateDedupeResult() error = %v", err)
	}
}

func candidate(id string, category string, severity string, confidence float64, claim string, path string, start int64, end int64, changedFileID string, fingerprint string) dbgen.FindingCandidate {
	return dbgen.FindingCandidate{
		ID:               id,
		Category:         category,
		Severity:         severity,
		Confidence:       confidence,
		Claim:            claim,
		PrimaryPath:      sql.NullString{String: path, Valid: path != ""},
		PrimaryStartLine: sql.NullInt64{Int64: start, Valid: start > 0},
		PrimaryEndLine:   sql.NullInt64{Int64: end, Valid: end > 0},
		LocationsJson:    `[{"path":"` + path + `","start_line":` + strconv.FormatInt(start, 10) + `,"end_line":` + strconv.FormatInt(end, 10) + `,"side":"RIGHT","changed_file_id":"` + changedFileID + `","valid":true}]`,
		EvidenceJson:     `[{"title":"evidence","summary":"supporting evidence","kind":"unknown"}]`,
		Fingerprint:      sql.NullString{String: fingerprint, Valid: fingerprint != ""},
	}
}
