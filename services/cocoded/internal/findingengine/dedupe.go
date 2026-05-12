package findingengine

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/agentoutput"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

var ErrInvalidDedupeResult = errors.New("dedupe result is invalid")

type Cluster struct {
	Candidates []dbgen.FindingCandidate
}

type DedupeInput struct {
	ReviewSessionID       string
	Candidates            []dbgen.FindingCandidate
	DeterministicClusters []Cluster
}

type DedupeResult struct {
	Clusters []Cluster
}

func Deduplicate(candidates []dbgen.FindingCandidate) []Cluster {
	if len(candidates) == 0 {
		return nil
	}
	parent := make([]int, len(candidates))
	for i := range parent {
		parent[i] = i
	}
	find := func(index int) int {
		for parent[index] != index {
			parent[index] = parent[parent[index]]
			index = parent[index]
		}
		return index
	}
	union := func(a int, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}

	for i := range candidates {
		for j := i + 1; j < len(candidates); j++ {
			if sameFingerprint(candidates[i], candidates[j]) || overlappingCandidate(candidates[i], candidates[j]) {
				union(i, j)
			}
		}
	}

	clusterByRoot := map[int][]dbgen.FindingCandidate{}
	for i, candidate := range candidates {
		root := find(i)
		clusterByRoot[root] = append(clusterByRoot[root], candidate)
	}
	roots := make([]int, 0, len(clusterByRoot))
	for root := range clusterByRoot {
		roots = append(roots, root)
	}
	sort.Ints(roots)
	clusters := make([]Cluster, 0, len(roots))
	for _, root := range roots {
		items := clusterByRoot[root]
		sort.SliceStable(items, func(i, j int) bool {
			return candidateScore(items[i]) > candidateScore(items[j])
		})
		clusters = append(clusters, Cluster{Candidates: items})
	}
	return clusters
}

func ValidateDedupeResult(candidates []dbgen.FindingCandidate, clusters []Cluster) error {
	if len(candidates) == 0 {
		if len(clusters) == 0 {
			return nil
		}
		return fmt.Errorf("%w: clusters returned for empty candidate set", ErrInvalidDedupeResult)
	}
	expected := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.ID) == "" {
			return fmt.Errorf("%w: candidate id is empty", ErrInvalidDedupeResult)
		}
		expected[candidate.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(candidates))
	representativeFingerprints := make(map[string]struct{}, len(clusters))
	for _, cluster := range clusters {
		if len(cluster.Candidates) == 0 {
			return fmt.Errorf("%w: empty cluster", ErrInvalidDedupeResult)
		}
		representative := Representative(cluster)
		if !representative.Fingerprint.Valid || strings.TrimSpace(representative.Fingerprint.String) == "" {
			return fmt.Errorf("%w: representative fingerprint is empty", ErrInvalidDedupeResult)
		}
		if _, ok := representativeFingerprints[representative.Fingerprint.String]; ok {
			return fmt.Errorf("%w: representative fingerprint %s appears more than once", ErrInvalidDedupeResult, representative.Fingerprint.String)
		}
		representativeFingerprints[representative.Fingerprint.String] = struct{}{}
		for _, candidate := range cluster.Candidates {
			if _, ok := expected[candidate.ID]; !ok {
				return fmt.Errorf("%w: unexpected candidate %s", ErrInvalidDedupeResult, candidate.ID)
			}
			if _, ok := seen[candidate.ID]; ok {
				return fmt.Errorf("%w: candidate %s appears more than once", ErrInvalidDedupeResult, candidate.ID)
			}
			seen[candidate.ID] = struct{}{}
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("%w: %d of %d candidates represented", ErrInvalidDedupeResult, len(seen), len(expected))
	}
	return nil
}

func Representative(cluster Cluster) dbgen.FindingCandidate {
	if len(cluster.Candidates) == 0 {
		return dbgen.FindingCandidate{}
	}
	return cluster.Candidates[0]
}

func EvidenceSummary(candidate dbgen.FindingCandidate) sql.NullString {
	var evidence []agentoutput.CandidateEvidence
	if err := json.Unmarshal([]byte(candidate.EvidenceJson), &evidence); err != nil || len(evidence) == 0 {
		return sql.NullString{}
	}
	parts := make([]string, 0, len(evidence))
	for _, item := range evidence {
		summary := strings.TrimSpace(item.Summary)
		if summary == "" {
			summary = strings.TrimSpace(item.Title)
		}
		if summary != "" {
			parts = append(parts, summary)
		}
	}
	if len(parts) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{String: strings.Join(parts, "\n"), Valid: true}
}

func sameFingerprint(a dbgen.FindingCandidate, b dbgen.FindingCandidate) bool {
	return a.Fingerprint.Valid && b.Fingerprint.Valid && a.Fingerprint.String == b.Fingerprint.String
}

func overlappingCandidate(a dbgen.FindingCandidate, b dbgen.FindingCandidate) bool {
	if a.Category != b.Category || !SimilarClaims(a.Claim, b.Claim) {
		return false
	}
	aLocations := decodeCandidateLocations(a.LocationsJson)
	bLocations := decodeCandidateLocations(b.LocationsJson)
	for _, left := range aLocations {
		for _, right := range bLocations {
			if !sameNormalizedLocation(left, right) {
				continue
			}
			if Overlap(left.StartLine, left.EndLine, right.StartLine, right.EndLine) {
				return true
			}
			if nearbyLineMatch(left.StartLine, right.StartLine) {
				return true
			}
		}
	}
	return false
}

func nearbyLineMatch(a int64, b int64) bool {
	if a < 1 || b < 1 {
		return false
	}
	return lineBucket(a) == lineBucket(b) && absInt64(a-b) <= 2
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func sameNormalizedLocation(a agentoutput.CandidateLocation, b agentoutput.CandidateLocation) bool {
	if a.ChangedFileID != "" && b.ChangedFileID != "" {
		return a.ChangedFileID == b.ChangedFileID
	}
	return normalizePathForCompare(a.Path) == normalizePathForCompare(b.Path)
}

func decodeCandidateLocations(raw string) []agentoutput.CandidateLocation {
	var locations []agentoutput.CandidateLocation
	_ = json.Unmarshal([]byte(raw), &locations)
	return locations
}

func normalizePathForCompare(path string) string {
	normalized, ok := normalizePath(path)
	if !ok {
		return ""
	}
	return normalized
}

func candidateScore(candidate dbgen.FindingCandidate) float64 {
	score := float64(severityRank(candidate.Severity))*100 + candidate.Confidence
	if candidate.PrimaryPath.Valid {
		score += 10
	}
	if hasValidLocation(candidate.LocationsJson) {
		score += 20
	}
	return score
}

func hasValidLocation(raw string) bool {
	for _, location := range decodeCandidateLocations(raw) {
		if location.Valid != nil && *location.Valid {
			return true
		}
	}
	return false
}

func severityRank(severity string) int {
	switch severity {
	case "blocker":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	case "nit":
		return 1
	default:
		return 0
	}
}
