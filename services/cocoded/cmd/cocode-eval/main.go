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
	if *failOnRegression && (report.Metrics.MissingExpected > 0 || report.Metrics.FalsePositives > 0) {
		os.Exit(1)
	}
}
