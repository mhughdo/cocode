package evalharness

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRunDefaultGoldenReposReportsExpectedMetrics(t *testing.T) {
	t.Parallel()

	report, err := Run(context.Background(), Options{
		ReposRoot: repoRoot(t),
		Now: func() time.Time {
			return time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Metrics.RepoCount != 3 ||
		report.Metrics.ExpectedFindings != 2 ||
		report.Metrics.ActualFindings != 2 ||
		report.Metrics.AcceptedExpected != 2 ||
		report.Metrics.MissingExpected != 0 ||
		report.Metrics.FalsePositives != 0 ||
		report.Metrics.PrecisionIsh != 1 ||
		report.Metrics.AcceptedExpectedRate != 1 ||
		report.Metrics.CostUSD != nil ||
		report.Metrics.CostSource == "" {
		t.Fatalf("metrics = %+v", report.Metrics)
	}

	auth := repoReport(t, report, "go-api-auth-bug")
	if len(auth.AcceptedExpectedIDs) != 1 ||
		auth.AcceptedExpectedIDs[0] != "auth-admin-guard" ||
		len(auth.FalsePositiveIDs) != 0 {
		t.Fatalf("auth report = %+v", auth)
	}
	noise := repoReport(t, report, "generated-files-noise")
	if len(noise.FileResults) != 4 {
		t.Fatalf("noise file results = %+v", noise.FileResults)
	}
	for _, file := range noise.FileResults {
		if !file.Matched {
			t.Fatalf("file expectation did not match: %+v", file)
		}
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal(report) error = %v", err)
	}
	if !json.Valid(encoded) {
		t.Fatalf("report JSON is invalid: %s", string(encoded))
	}
}

func TestRunRejectsUnknownDetector(t *testing.T) {
	t.Parallel()

	_, err := Run(context.Background(), Options{
		ReposRoot: repoRoot(t),
		Specs: []RepoSpec{{
			Name:      "go-api-auth-bug",
			Detectors: []string{"unknown-detector"},
		}},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want unknown detector error")
	}
}

func repoReport(t *testing.T, report Report, name string) RepoReport {
	t.Helper()

	for _, repo := range report.Repos {
		if repo.Name == name {
			return repo
		}
	}
	t.Fatalf("report missing repo %s: %+v", name, report.Repos)
	return RepoReport{}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "testdata", "repos"))
}
