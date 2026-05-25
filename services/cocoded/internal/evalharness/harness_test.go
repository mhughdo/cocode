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
		report.Metrics.ReviewedFindings != 2 ||
		report.Metrics.AcceptedFindings != 2 ||
		report.Metrics.DismissedFindings != 0 ||
		report.Metrics.PublishableFindings != 2 ||
		report.Metrics.SuppressedFindings != 0 ||
		report.Metrics.NotActionableFindings != 0 ||
		report.Metrics.AcceptedFindingRate != 1 ||
		report.Metrics.FalsePositiveRate != 0 ||
		report.Metrics.SuppressionRate != 0 ||
		report.Metrics.ReviewOutcomeSource != reviewOutcomeSourceDerived ||
		report.Metrics.CostUSD != nil ||
		report.Metrics.CostSource == "" {
		t.Fatalf("metrics = %+v", report.Metrics)
	}

	auth := repoReport(t, report, "go-api-auth-bug")
	if len(auth.AcceptedExpectedIDs) != 1 ||
		auth.AcceptedExpectedIDs[0] != "auth-admin-guard" ||
		len(auth.FalsePositiveIDs) != 0 ||
		auth.ReviewMetrics.AcceptedFindings != 1 ||
		auth.ReviewMetrics.PublishableFindings != 1 ||
		auth.ReviewMetrics.OutcomeSource != reviewOutcomeSourceDerived {
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

func TestRunReportsExplicitReviewOutcomesSeparateFromDetectorMatches(t *testing.T) {
	t.Parallel()

	report, err := Run(context.Background(), Options{
		ReposRoot: repoRoot(t),
		Specs: []RepoSpec{{
			Name:      "go-api-auth-bug",
			Detectors: []string{detectorAuthAdminGuard},
			ExpectedFindings: []ExpectedFinding{{
				ID:   "auth-admin-guard",
				Path: "apps/api/src/routes/repositories.ts",
			}},
			ReviewOutcomes: []ReviewOutcome{{
				FindingID: "auth-admin-guard",
				Decision:  reviewDecisionDismissed,
				Reason:    "local dogfood marked this as duplicate",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if report.Metrics.AcceptedExpected != 1 ||
		report.Metrics.AcceptedFindings != 0 ||
		report.Metrics.DismissedFindings != 1 ||
		report.Metrics.PublishableFindings != 0 ||
		report.Metrics.AcceptedFindingRate != 0 ||
		report.Metrics.ReviewOutcomeSource != reviewOutcomeSourceExplicit {
		t.Fatalf("metrics = %+v", report.Metrics)
	}
	auth := repoReport(t, report, "go-api-auth-bug")
	if len(auth.ReviewOutcomes) != 1 ||
		auth.ReviewOutcomes[0].Decision != reviewDecisionDismissed ||
		auth.ReviewOutcomes[0].Reason == "" {
		t.Fatalf("review outcomes = %+v", auth.ReviewOutcomes)
	}
}

func TestSummarizeReviewOutcomesReportsSuppressionAndPublishableRates(t *testing.T) {
	t.Parallel()

	metrics := summarizeReviewOutcomes([]ReviewOutcome{
		{FindingID: "accepted", Decision: reviewDecisionAccepted, Publishable: true},
		{FindingID: "dismissed", Decision: reviewDecisionDismissed},
		{FindingID: "suppressed", Decision: reviewDecisionSuppressed},
		{FindingID: "not-actionable", Decision: reviewDecisionNotActionable},
	}, reviewOutcomeSourceExplicit)

	if metrics.ReviewedFindings != 4 ||
		metrics.AcceptedFindings != 1 ||
		metrics.DismissedFindings != 1 ||
		metrics.PublishableFindings != 1 ||
		metrics.SuppressedFindings != 1 ||
		metrics.NotActionableFindings != 1 ||
		metrics.AcceptedFindingRate != 0.25 ||
		metrics.SuppressionRate != 0.5 ||
		metrics.OutcomeSource != reviewOutcomeSourceExplicit {
		t.Fatalf("metrics = %+v", metrics)
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

func TestRunRejectsInvalidReviewOutcome(t *testing.T) {
	t.Parallel()

	_, err := Run(context.Background(), Options{
		ReposRoot: repoRoot(t),
		Specs: []RepoSpec{{
			Name:      "go-api-auth-bug",
			Detectors: []string{detectorAuthAdminGuard},
			ReviewOutcomes: []ReviewOutcome{{
				FindingID: "missing-finding",
				Decision:  reviewDecisionAccepted,
			}},
		}},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want invalid review outcome error")
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
