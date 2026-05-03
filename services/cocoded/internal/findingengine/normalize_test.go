package findingengine

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/agentoutput"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestNormalizeCandidateMapsChangedFileLocations(t *testing.T) {
	t.Parallel()

	candidate := agentoutput.Candidate{
		Claim:      "Settings mutation lacks admin guard",
		Category:   "security",
		Severity:   "high",
		Confidence: 0.9,
		Locations: []agentoutput.CandidateLocation{
			{Path: "b/src/new.go", StartLine: 2, EndLine: 3, Side: "RIGHT"},
			{Path: "/etc/passwd", StartLine: 1, EndLine: 1, Side: "RIGHT"},
			{Path: "src/new.go", StartLine: 20, EndLine: 21, Side: "RIGHT"},
		},
	}

	normalized := NormalizeCandidate(candidate, []dbgen.ChangedFile{changedFile("changed_file_1", "src/new.go", "", `[[1,3]]`)})
	if normalized.PrimaryPath != "src/new.go" ||
		normalized.PrimaryStartLine != 2 ||
		normalized.Fingerprint == "" {
		t.Fatalf("normalized = %+v", normalized)
	}
	assertLocation(t, normalized.Locations[0], true, "changed_file_1", "")
	assertLocation(t, normalized.Locations[1], false, "", "unsafe")
	assertLocation(t, normalized.Locations[2], false, "changed_file_1", "outside")
}

func TestNormalizeCandidateMapsRenamedLeftSideLocations(t *testing.T) {
	t.Parallel()

	candidate := agentoutput.Candidate{
		Claim:      "Old file cleanup removed a required guard",
		Category:   "correctness",
		Severity:   "medium",
		Confidence: 0.75,
		Locations: []agentoutput.CandidateLocation{
			{Path: "src/old.go", StartLine: 42, EndLine: 45, Side: "LEFT"},
		},
	}

	normalized := NormalizeCandidate(candidate, []dbgen.ChangedFile{changedFile("changed_file_1", "src/new.go", "src/old.go", `[[2,3]]`)})
	assertLocation(t, normalized.Locations[0], true, "changed_file_1", "")
	if normalized.PrimaryPath != "src/old.go" || normalized.PrimaryStartLine != 42 {
		t.Fatalf("normalized = %+v", normalized)
	}
}

func TestFingerprintIsStableAcrossSeverityAndFormatting(t *testing.T) {
	t.Parallel()

	base := agentoutput.Candidate{
		Claim:            "Settings mutation lacks admin guard.",
		Category:         "security",
		Severity:         "high",
		PrimaryPath:      "src/new.go",
		PrimaryStartLine: 87,
	}
	sameIssue := base
	sameIssue.Claim = "settings, mutation lacks admin guard"
	sameIssue.Severity = "low"
	if Fingerprint(base) != Fingerprint(sameIssue) {
		t.Fatalf("fingerprint changed for equivalent claim/severity disagreement")
	}

	differentLocation := base
	differentLocation.PrimaryStartLine = 120
	if Fingerprint(base) == Fingerprint(differentLocation) {
		t.Fatalf("fingerprint did not change for different location bucket")
	}

	differentCategory := base
	differentCategory.Category = "reliability"
	if Fingerprint(base) == Fingerprint(differentCategory) {
		t.Fatalf("fingerprint did not change for different category")
	}
}

func assertLocation(t *testing.T, location agentoutput.CandidateLocation, wantValid bool, wantChangedFileID string, messageContains string) {
	t.Helper()

	if location.Valid == nil || *location.Valid != wantValid || location.ChangedFileID != wantChangedFileID {
		t.Fatalf("location = %+v, want valid=%v changed_file_id=%q", location, wantValid, wantChangedFileID)
	}
	if messageContains != "" && !strings.Contains(location.Message, messageContains) {
		t.Fatalf("location message = %q, want substring %q", location.Message, messageContains)
	}
}

func changedFile(id string, path string, oldPath string, ranges string) dbgen.ChangedFile {
	return dbgen.ChangedFile{
		ID:             id,
		Path:           path,
		OldPath:        sql.NullString{String: oldPath, Valid: oldPath != ""},
		LineRangesJson: ranges,
	}
}
