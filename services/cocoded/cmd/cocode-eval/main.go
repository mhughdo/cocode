package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/hughdo/cocode/services/cocoded/internal/evalharness"
)

func main() {
	reposRoot := flag.String("repos-root", evalharness.DefaultReposRoot(), "path to golden repositories")
	format := flag.String("format", "json", "output format: json")
	failOnRegression := flag.Bool("fail-on-regression", false, "exit non-zero when expected findings are missing or false positives are present")
	minPrecision := flag.Float64("min-precision-ish", -1, "minimum precision-ish gate")
	maxFalsePositiveRate := flag.Float64("max-false-positive-rate", -1, "maximum false-positive rate gate")
	minAcceptedExpectedRate := flag.Float64("min-accepted-expected-rate", -1, "minimum accepted-expected rate gate")
	maxSuppressionRate := flag.Float64("max-suppression-rate", -1, "maximum suppression rate gate")
	maxDuplicateRate := flag.Float64("max-duplicate-rate", -1, "maximum duplicate rate gate")
	baselinePath := flag.String("baseline", "", "optional baseline report JSON for before/after comparison")
	maxPrecisionDrop := flag.Float64("max-precision-drop", -1, "maximum allowed precision-ish drop versus baseline")
	maxAcceptedExpectedDrop := flag.Float64("max-accepted-expected-rate-drop", -1, "maximum allowed accepted-expected rate drop versus baseline")
	maxFalsePositiveIncrease := flag.Float64("max-false-positive-rate-increase", -1, "maximum allowed false-positive rate increase versus baseline")
	maxSuppressionIncrease := flag.Float64("max-suppression-rate-increase", -1, "maximum allowed suppression rate increase versus baseline")
	maxDuplicateIncrease := flag.Float64("max-duplicate-rate-increase", -1, "maximum allowed duplicate rate increase versus baseline")
	flag.Parse()

	if *format != "json" {
		fmt.Fprintf(os.Stderr, "unsupported format %q\n", *format)
		os.Exit(2)
	}

	report, err := evalharness.Run(context.Background(), evalharness.Options{
		ReposRoot: *reposRoot,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "run evaluation harness: %v\n", err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "write evaluation report: %v\n", err)
		os.Exit(1)
	}
	thresholds := evalharness.GateThresholds{
		MinPrecisionIsh:              minPrecision,
		MaxFalsePositiveRate:         maxFalsePositiveRate,
		MinAcceptedExpectedRate:      minAcceptedExpectedRate,
		MaxSuppressionRate:           maxSuppressionRate,
		MaxDuplicateRate:             maxDuplicateRate,
		MaxPrecisionDrop:             maxPrecisionDrop,
		MaxAcceptedExpectedRateDrop:  maxAcceptedExpectedDrop,
		MaxFalsePositiveRateIncrease: maxFalsePositiveIncrease,
		MaxSuppressionRateIncrease:   maxSuppressionIncrease,
		MaxDuplicateRateIncrease:     maxDuplicateIncrease,
	}
	if *baselinePath != "" {
		baseline, err := loadBaselineReport(*baselinePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read baseline report: %v\n", err)
			os.Exit(1)
		}
		thresholds.Baseline = &baseline.Metrics
	}
	gateResult := evalharness.EvaluateGates(report.Metrics, thresholds)
	if !gateResult.Passed {
		for _, failure := range gateResult.Failures {
			fmt.Fprintln(os.Stderr, failure)
		}
		os.Exit(1)
	}
	if *failOnRegression && (report.Metrics.MissingExpected > 0 || report.Metrics.FalsePositives > 0) {
		os.Exit(1)
	}
}

func loadBaselineReport(path string) (evalharness.Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return evalharness.Report{}, err
	}
	var report evalharness.Report
	if err := json.Unmarshal(data, &report); err != nil {
		return evalharness.Report{}, err
	}
	return report, nil
}
