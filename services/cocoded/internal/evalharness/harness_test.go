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

func TestRunReportsDuplicateNoiseMetrics(t *testing.T) {
	t.Parallel()

	report, err := Run(context.Background(), Options{
		ReposRoot: repoRoot(t),
		Specs: []RepoSpec{{
			Name:      "go-api-auth-bug",
			Detectors: []string{detectorDuplicateNoiseControl},
			ExpectedFindings: []ExpectedFinding{{
				ID:   "auth-admin-guard",
				Path: "apps/api/src/routes/repositories.ts",
				MatchTerms: []string{
					"workspace member",
					"admin guard",
					"repository settings",
				},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if report.Metrics.ActualFindings != 3 ||
		report.Metrics.DuplicateClusters != 1 ||
		report.Metrics.DuplicateFindings != 2 ||
		report.Metrics.FalsePositives != 2 ||
		report.Metrics.PrecisionIsh < 0.333 ||
		report.Metrics.PrecisionIsh > 0.334 ||
		report.Metrics.DuplicateRate < 0.666 ||
		report.Metrics.DuplicateRate > 0.667 {
		t.Fatalf("metrics = %+v", report.Metrics)
	}
	auth := repoReport(t, report, "go-api-auth-bug")
	if auth.DuplicateMetrics.ClusterCount != 1 ||
		auth.DuplicateMetrics.DuplicateFindings != 2 ||
		auth.DuplicateMetrics.DuplicateRate < 0.666 ||
		auth.DuplicateMetrics.DuplicateRate > 0.667 {
		t.Fatalf("duplicate metrics = %+v", auth.DuplicateMetrics)
	}
}

func TestSummarizeReviewOutcomesReportsSuppressionAndPublishableRates(t *testing.T) {
	t.Parallel()

	metrics := summarizeReviewOutcomes([]ReviewOutcome{
		{FindingID: "accepted", Decision: reviewDecisionAccepted, Publishable: true},
		{FindingID: "dismissed", Decision: reviewDecisionDismissed},
		{FindingID: "suppressed", Decision: reviewDecisionSuppressed},
		{FindingID: "not-actionable", Decision: reviewDecisionNotActionable},
		{FindingID: "duplicate", Decision: reviewDecisionDuplicate},
		{FindingID: "stale", Decision: reviewDecisionStale},
	}, reviewOutcomeSourceExplicit)

	if metrics.ReviewedFindings != 6 ||
		metrics.AcceptedFindings != 1 ||
		metrics.DismissedFindings != 1 ||
		metrics.PublishableFindings != 1 ||
		metrics.SuppressedFindings != 1 ||
		metrics.NotActionableFindings != 1 ||
		metrics.DuplicateDecisionFindings != 1 ||
		metrics.StaleFindings != 1 ||
		metrics.AcceptedFindingRate != 1.0/6.0 ||
		metrics.SuppressionRate != 4.0/6.0 ||
		metrics.OutcomeSource != reviewOutcomeSourceExplicit {
		t.Fatalf("metrics = %+v", metrics)
	}
}

func TestEvaluateGatesReportsThresholdAndBaselineRegressions(t *testing.T) {
	t.Parallel()

	minPrecision := 0.9
	maxFalsePositive := 0.2
	minAcceptedExpected := 0.7
	maxSuppression := 0.3
	maxDuplicate := 0.2
	maxPrecisionDrop := 0.05
	maxAcceptedDrop := 0.05
	maxFPIncrease := 0.05
	maxSuppressionIncrease := 0.05
	maxDuplicateIncrease := 0.05

	result := EvaluateGates(Metrics{
		PrecisionIsh:         0.84,
		FalsePositiveRate:    0.28,
		AcceptedExpectedRate: 0.60,
		SuppressionRate:      0.40,
		DuplicateRate:        0.30,
	}, GateThresholds{
		MinPrecisionIsh:              &minPrecision,
		MaxFalsePositiveRate:         &maxFalsePositive,
		MinAcceptedExpectedRate:      &minAcceptedExpected,
		MaxSuppressionRate:           &maxSuppression,
		MaxDuplicateRate:             &maxDuplicate,
		Baseline:                     &Metrics{PrecisionIsh: 0.90, FalsePositiveRate: 0.20, AcceptedExpectedRate: 0.70, SuppressionRate: 0.30, DuplicateRate: 0.20},
		MaxPrecisionDrop:             &maxPrecisionDrop,
		MaxAcceptedExpectedRateDrop:  &maxAcceptedDrop,
		MaxFalsePositiveRateIncrease: &maxFPIncrease,
		MaxSuppressionRateIncrease:   &maxSuppressionIncrease,
		MaxDuplicateRateIncrease:     &maxDuplicateIncrease,
	})

	if result.Passed || len(result.Failures) < 5 {
		t.Fatalf("result = %+v", result)
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
