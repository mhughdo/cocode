package evalharness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/fileclassify"
)

type Options struct {
	ReposRoot string
	Specs     []RepoSpec
	Now       func() time.Time
}

type RepoSpec struct {
	Name             string
	ExpectedFindings []ExpectedFinding
	Detectors        []string
	FileExpectations []FileExpectation
}

type ExpectedFinding struct {
	ID         string   `json:"id"`
	Claim      string   `json:"claim"`
	Path       string   `json:"path"`
	MatchTerms []string `json:"match_terms"`
}

type FileExpectation struct {
	Path          string `json:"path"`
	Excluded      bool   `json:"excluded"`
	ExpectedLabel string `json:"expected_label"`
}

type Report struct {
	GeneratedAt string       `json:"generated_at"`
	ReposRoot   string       `json:"repos_root"`
	Metrics     Metrics      `json:"metrics"`
	Repos       []RepoReport `json:"repos"`
}

type Metrics struct {
	RepoCount            int      `json:"repo_count"`
	ExpectedFindings     int      `json:"expected_findings"`
	ActualFindings       int      `json:"actual_findings"`
	AcceptedExpected     int      `json:"accepted_expected"`
	MissingExpected      int      `json:"missing_expected"`
	FalsePositives       int      `json:"false_positives"`
	PrecisionIsh         float64  `json:"precision_ish"`
	AcceptedExpectedRate float64  `json:"accepted_expected_rate"`
	DurationMs           int64    `json:"duration_ms"`
	CostUSD              *float64 `json:"cost_usd"`
	CostSource           string   `json:"cost_source"`
}

type RepoReport struct {
	Name                string            `json:"name"`
	Path                string            `json:"path"`
	ExpectedFindings    []ExpectedFinding `json:"expected_findings"`
	ActualFindings      []Finding         `json:"actual_findings"`
	AcceptedExpectedIDs []string          `json:"accepted_expected_ids"`
	MissingExpectedIDs  []string          `json:"missing_expected_ids"`
	FalsePositiveIDs    []string          `json:"false_positive_ids"`
	FileExpectations    []FileExpectation `json:"file_expectations,omitempty"`
	FileResults         []FileCheckResult `json:"file_results,omitempty"`
	DurationMs          int64             `json:"duration_ms"`
	Diagnostics         []string          `json:"diagnostics,omitempty"`
}

type Finding struct {
	ID         string   `json:"id"`
	Claim      string   `json:"claim"`
	Category   string   `json:"category"`
	Severity   string   `json:"severity"`
	Path       string   `json:"path"`
	StartLine  int      `json:"start_line,omitempty"`
	MatchTerms []string `json:"match_terms"`
}

type FileCheckResult struct {
	Path          string   `json:"path"`
	Excluded      bool     `json:"excluded"`
	Expected      bool     `json:"expected"`
	Matched       bool     `json:"matched"`
	Label         string   `json:"label"`
	ExpectedLabel string   `json:"expected_label"`
	Reasons       []string `json:"reasons"`
}

const (
	detectorAuthAdminGuard        = "auth_admin_guard"
	detectorWebhookSignature      = "webhook_signature_validation"
	detectorGeneratedNoiseControl = "generated_noise_control"
)

var DefaultSpecs = []RepoSpec{
	{
		Name:      "go-api-auth-bug",
		Detectors: []string{detectorAuthAdminGuard},
		ExpectedFindings: []ExpectedFinding{{
			ID:    "auth-admin-guard",
			Claim: "Repository settings updates can be performed by a workspace member because the mutation route omits the admin guard.",
			Path:  "apps/api/src/routes/repositories.ts",
			MatchTerms: []string{
				"workspace member",
				"admin guard",
				"repository settings",
			},
		}},
	},
	{
		Name:      "webhook-validation-bug",
		Detectors: []string{detectorWebhookSignature},
		ExpectedFindings: []ExpectedFinding{{
			ID:    "webhook-signature-validation",
			Claim: "Stripe webhook payloads are parsed without validating the signature header.",
			Path:  "apps/api/src/webhooks/stripe.ts",
			MatchTerms: []string{
				"webhook",
				"signature",
				"raw payload",
			},
		}},
	},
	{
		Name:      "generated-files-noise",
		Detectors: []string{detectorGeneratedNoiseControl},
		FileExpectations: []FileExpectation{
			{
				Path:          "services/api/internal/db/dbgen/snapshots.sql.go",
				Excluded:      true,
				ExpectedLabel: "generated",
			},
			{
				Path:          "web/src/generated/client.generated.ts",
				Excluded:      true,
				ExpectedLabel: "generated",
			},
			{
				Path:          "pnpm-lock.yaml",
				Excluded:      true,
				ExpectedLabel: "lockfile",
			},
			{
				Path:          "services/api/src/handler.go",
				Excluded:      false,
				ExpectedLabel: "handwritten",
			},
		},
	},
}

func Run(ctx context.Context, options Options) (Report, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	started := now(options.Now)
	reposRoot := strings.TrimSpace(options.ReposRoot)
	if reposRoot == "" {
		reposRoot = DefaultReposRoot()
	}
	specs := options.Specs
	if len(specs) == 0 {
		specs = DefaultSpecs
	}

	report := Report{
		GeneratedAt: started.Format(time.RFC3339Nano),
		ReposRoot:   reposRoot,
		Repos:       make([]RepoReport, 0, len(specs)),
	}
	for _, spec := range specs {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		repoReport, err := runRepo(ctx, reposRoot, spec, options.Now)
		if err != nil {
			return Report{}, err
		}
		report.Repos = append(report.Repos, repoReport)
	}
	report.Metrics = summarize(report.Repos, time.Since(started))
	return report, nil
}

func DefaultReposRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("testdata", "repos")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	return filepath.Join(root, "testdata", "repos")
}

func runRepo(ctx context.Context, reposRoot string, spec RepoSpec, nowFunc func() time.Time) (RepoReport, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, string(os.PathSeparator)) {
		return RepoReport{}, fmt.Errorf("golden repo name %q is invalid", spec.Name)
	}
	started := now(nowFunc)
	repoPath := filepath.Join(reposRoot, name)
	stat, err := os.Stat(repoPath)
	if err != nil {
		return RepoReport{}, fmt.Errorf("golden repo %s is unavailable: %w", name, err)
	}
	if !stat.IsDir() {
		return RepoReport{}, fmt.Errorf("golden repo %s is not a directory", name)
	}

	actual, diagnostics, err := runDetectors(ctx, repoPath, spec.Detectors)
	if err != nil {
		return RepoReport{}, err
	}
	fileResults, err := checkFileExpectations(repoPath, spec.FileExpectations)
	if err != nil {
		return RepoReport{}, err
	}
	acceptedExpected, missingExpected, falsePositiveIDs := matchExpected(spec.ExpectedFindings, actual)
	return RepoReport{
		Name:                name,
		Path:                repoPath,
		ExpectedFindings:    append([]ExpectedFinding(nil), spec.ExpectedFindings...),
		ActualFindings:      actual,
		AcceptedExpectedIDs: acceptedExpected,
		MissingExpectedIDs:  missingExpected,
		FalsePositiveIDs:    falsePositiveIDs,
		FileExpectations:    append([]FileExpectation(nil), spec.FileExpectations...),
		FileResults:         fileResults,
		DurationMs:          time.Since(started).Milliseconds(),
		Diagnostics:         diagnostics,
	}, nil
}

func runDetectors(ctx context.Context, repoPath string, detectors []string) ([]Finding, []string, error) {
	findings := []Finding{}
	diagnostics := []string{}
	for _, detector := range detectors {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		switch detector {
		case detectorAuthAdminGuard:
			finding, ok, err := detectAuthAdminGuard(repoPath)
			if err != nil {
				return nil, nil, err
			}
			if ok {
				findings = append(findings, finding)
			}
		case detectorWebhookSignature:
			finding, ok, err := detectWebhookSignature(repoPath)
			if err != nil {
				return nil, nil, err
			}
			if ok {
				findings = append(findings, finding)
			}
		case detectorGeneratedNoiseControl:
			diagnostics = append(diagnostics, "generated-file repo uses file expectations only")
		default:
			return nil, nil, fmt.Errorf("unknown eval detector %q", detector)
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path == findings[j].Path {
			return findings[i].ID < findings[j].ID
		}
		return findings[i].Path < findings[j].Path
	})
	return findings, diagnostics, nil
}

func detectAuthAdminGuard(repoPath string) (Finding, bool, error) {
	const relativePath = "apps/api/src/routes/repositories.ts"
	content, err := readRepoText(repoPath, relativePath)
	if err != nil {
		return Finding{}, false, err
	}
	patchRoute := between(content, `router.patch(`, `);`)
	if !strings.Contains(patchRoute, `"/repositories/:id/settings"`) ||
		!strings.Contains(patchRoute, "requireWorkspaceMember") ||
		strings.Contains(patchRoute, "requireWorkspaceAdmin") ||
		!strings.Contains(content, "repositoryService.updateSettings") {
		return Finding{}, false, nil
	}
	return Finding{
		ID:        "auth-admin-guard",
		Claim:     "Repository settings update route allows workspace members without the admin guard.",
		Category:  "security",
		Severity:  "high",
		Path:      relativePath,
		StartLine: lineNumber(content, "router.patch("),
		MatchTerms: []string{
			"workspace member",
			"admin guard",
			"repository settings",
		},
	}, true, nil
}

func detectWebhookSignature(repoPath string) (Finding, bool, error) {
	const relativePath = "apps/api/src/webhooks/stripe.ts"
	content, err := readRepoText(repoPath, relativePath)
	if err != nil {
		return Finding{}, false, err
	}
	lower := strings.ToLower(content)
	if !strings.Contains(content, "JSON.parse(request.rawBody") ||
		strings.Contains(lower, "constructevent") ||
		strings.Contains(lower, "stripe-signature") ||
		strings.Contains(lower, "verify") {
		return Finding{}, false, nil
	}
	return Finding{
		ID:        "webhook-signature-validation",
		Claim:     "Stripe webhook handler parses raw payloads without validating the signature header.",
		Category:  "security",
		Severity:  "high",
		Path:      relativePath,
		StartLine: lineNumber(content, "JSON.parse"),
		MatchTerms: []string{
			"webhook",
			"signature",
			"raw payload",
		},
	}, true, nil
}

func checkFileExpectations(repoPath string, expectations []FileExpectation) ([]FileCheckResult, error) {
	results := make([]FileCheckResult, 0, len(expectations))
	for _, expectation := range expectations {
		content, err := readRepoBytes(repoPath, expectation.Path)
		if err != nil {
			return nil, err
		}
		classification := fileclassify.Classify(fileclassify.Input{
			Path:          expectation.Path,
			ContentPrefix: contentPrefix(content),
		})
		label := classificationLabel(classification)
		reasons := make([]string, 0, len(classification.Reasons))
		for _, reason := range classification.Reasons {
			reasons = append(reasons, string(reason))
		}
		results = append(results, FileCheckResult{
			Path:          expectation.Path,
			Excluded:      classification.ExcludedCandidate,
			Expected:      expectation.Excluded,
			Matched:       classification.ExcludedCandidate == expectation.Excluded,
			Label:         label,
			ExpectedLabel: expectation.ExpectedLabel,
			Reasons:       reasons,
		})
	}
	return results, nil
}

func matchExpected(expected []ExpectedFinding, actual []Finding) ([]string, []string, []string) {
	actualMatched := make(map[string]bool, len(actual))
	acceptedExpected := []string{}
	missingExpected := []string{}
	for _, want := range expected {
		matched := false
		for _, got := range actual {
			if actualMatched[got.ID] {
				continue
			}
			if findingMatches(want, got) {
				actualMatched[got.ID] = true
				acceptedExpected = append(acceptedExpected, want.ID)
				matched = true
				break
			}
		}
		if !matched {
			missingExpected = append(missingExpected, want.ID)
		}
	}
	falsePositiveIDs := []string{}
	for _, got := range actual {
		if !actualMatched[got.ID] {
			falsePositiveIDs = append(falsePositiveIDs, got.ID)
		}
	}
	sort.Strings(acceptedExpected)
	sort.Strings(missingExpected)
	sort.Strings(falsePositiveIDs)
	return acceptedExpected, missingExpected, falsePositiveIDs
}

func findingMatches(want ExpectedFinding, got Finding) bool {
	if want.ID != "" && want.ID == got.ID {
		return true
	}
	if want.Path != "" && want.Path != got.Path {
		return false
	}
	haystack := strings.ToLower(got.Claim + " " + strings.Join(got.MatchTerms, " "))
	for _, term := range want.MatchTerms {
		if !strings.Contains(haystack, strings.ToLower(term)) {
			return false
		}
	}
	return len(want.MatchTerms) > 0
}

func summarize(reports []RepoReport, duration time.Duration) Metrics {
	metrics := Metrics{
		RepoCount:  len(reports),
		DurationMs: duration.Milliseconds(),
		CostSource: "unavailable_for_static_harness",
	}
	for _, report := range reports {
		metrics.ExpectedFindings += len(report.ExpectedFindings)
		metrics.ActualFindings += len(report.ActualFindings)
		metrics.AcceptedExpected += len(report.AcceptedExpectedIDs)
		metrics.MissingExpected += len(report.MissingExpectedIDs)
		metrics.FalsePositives += len(report.FalsePositiveIDs)
		for _, file := range report.FileResults {
			if !file.Matched {
				metrics.FalsePositives++
			}
		}
	}
	if metrics.ActualFindings > 0 {
		metrics.PrecisionIsh = float64(metrics.AcceptedExpected) / float64(metrics.ActualFindings)
	} else if metrics.ExpectedFindings == 0 {
		metrics.PrecisionIsh = 1
	}
	if metrics.ExpectedFindings > 0 {
		metrics.AcceptedExpectedRate = float64(metrics.AcceptedExpected) / float64(metrics.ExpectedFindings)
	} else {
		metrics.AcceptedExpectedRate = 1
	}
	return metrics
}

func readRepoText(repoPath string, relativePath string) (string, error) {
	content, err := readRepoBytes(repoPath, relativePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func readRepoBytes(repoPath string, relativePath string) ([]byte, error) {
	if strings.TrimSpace(relativePath) == "" || filepath.IsAbs(relativePath) {
		return nil, fmt.Errorf("repo relative path %q is invalid", relativePath)
	}
	clean := filepath.Clean(filepath.FromSlash(relativePath))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return nil, fmt.Errorf("repo relative path %q escapes repository", relativePath)
	}
	content, err := os.ReadFile(filepath.Join(repoPath, clean))
	if err != nil {
		return nil, fmt.Errorf("read golden file %s: %w", relativePath, err)
	}
	return content, nil
}

func contentPrefix(content []byte) []byte {
	if len(content) <= 8192 {
		return content
	}
	return content[:8192]
}

func classificationLabel(classification fileclassify.Classification) string {
	switch {
	case classification.Binary:
		return "binary"
	case classification.Lockfile:
		return "lockfile"
	case classification.Generated:
		return "generated"
	case classification.Vendor:
		return "vendor"
	case classification.ExcludedCandidate:
		return "excluded"
	default:
		return "handwritten"
	}
}

func between(content string, start string, end string) string {
	startIndex := strings.Index(content, start)
	if startIndex < 0 {
		return ""
	}
	tail := content[startIndex:]
	endIndex := strings.Index(tail, end)
	if endIndex < 0 {
		return tail
	}
	return tail[:endIndex+len(end)]
}

func lineNumber(content string, needle string) int {
	index := strings.Index(content, needle)
	if index < 0 {
		return 0
	}
	return strings.Count(content[:index], "\n") + 1
}

func now(nowFunc func() time.Time) time.Time {
	if nowFunc != nil {
		return nowFunc()
	}
	return time.Now().UTC()
}
