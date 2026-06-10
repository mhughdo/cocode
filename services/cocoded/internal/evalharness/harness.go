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
	ReviewOutcomes   []ReviewOutcome
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
	RepoCount                 int      `json:"repo_count"`
	ExpectedFindings          int      `json:"expected_findings"`
	ActualFindings            int      `json:"actual_findings"`
	DuplicateClusters         int      `json:"duplicate_clusters"`
	DuplicateFindings         int      `json:"duplicate_findings"`
	DuplicateRate             float64  `json:"duplicate_rate"`
	AcceptedExpected          int      `json:"accepted_expected"`
	MissingExpected           int      `json:"missing_expected"`
	FalsePositives            int      `json:"false_positives"`
	PrecisionIsh              float64  `json:"precision_ish"`
	AcceptedExpectedRate      float64  `json:"accepted_expected_rate"`
	ReviewedFindings          int      `json:"reviewed_findings"`
	AcceptedFindings          int      `json:"accepted_findings"`
	DismissedFindings         int      `json:"dismissed_findings"`
	PublishableFindings       int      `json:"publishable_findings"`
	SuppressedFindings        int      `json:"suppressed_findings"`
	NotActionableFindings     int      `json:"not_actionable_findings"`
	DuplicateDecisionFindings int      `json:"duplicate_decision_findings"`
	StaleFindings             int      `json:"stale_findings"`
	AcceptedFindingRate       float64  `json:"accepted_finding_rate"`
	FalsePositiveRate         float64  `json:"false_positive_rate"`
	SuppressionRate           float64  `json:"suppression_rate"`
	ReviewOutcomeSource       string   `json:"review_outcome_source"`
	DurationMs                int64    `json:"duration_ms"`
	CostUSD                   *float64 `json:"cost_usd"`
	CostSource                string   `json:"cost_source"`
}

type RepoReport struct {
	Name                string            `json:"name"`
	Path                string            `json:"path"`
	ExpectedFindings    []ExpectedFinding `json:"expected_findings"`
	ActualFindings      []Finding         `json:"actual_findings"`
	AcceptedExpectedIDs []string          `json:"accepted_expected_ids"`
	MissingExpectedIDs  []string          `json:"missing_expected_ids"`
	FalsePositiveIDs    []string          `json:"false_positive_ids"`
	DuplicateMetrics    DuplicateMetrics  `json:"duplicate_metrics"`
	ReviewOutcomes      []ReviewOutcome   `json:"review_outcomes,omitempty"`
	ReviewMetrics       ReviewMetrics     `json:"review_metrics"`
	FileExpectations    []FileExpectation `json:"file_expectations,omitempty"`
	FileResults         []FileCheckResult `json:"file_results,omitempty"`
	DurationMs          int64             `json:"duration_ms"`
	Diagnostics         []string          `json:"diagnostics,omitempty"`
}

type Finding struct {
	ID           string   `json:"id"`
	Claim        string   `json:"claim"`
	Category     string   `json:"category"`
	Severity     string   `json:"severity"`
	Path         string   `json:"path"`
	StartLine    int      `json:"start_line,omitempty"`
	MatchTerms   []string `json:"match_terms"`
	DuplicateKey string   `json:"duplicate_key,omitempty"`
	SourceAgent  string   `json:"source_agent,omitempty"`
}

type DuplicateMetrics struct {
	ClusterCount      int     `json:"cluster_count"`
	DuplicateFindings int     `json:"duplicate_findings"`
	DuplicateRate     float64 `json:"duplicate_rate"`
}

type ReviewOutcome struct {
	FindingID   string `json:"finding_id"`
	Decision    string `json:"decision"`
	Publishable bool   `json:"publishable,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type ReviewMetrics struct {
	ReviewedFindings          int     `json:"reviewed_findings"`
	AcceptedFindings          int     `json:"accepted_findings"`
	DismissedFindings         int     `json:"dismissed_findings"`
	PublishableFindings       int     `json:"publishable_findings"`
	SuppressedFindings        int     `json:"suppressed_findings"`
	NotActionableFindings     int     `json:"not_actionable_findings"`
	DuplicateDecisionFindings int     `json:"duplicate_decision_findings"`
	StaleFindings             int     `json:"stale_findings"`
	AcceptedFindingRate       float64 `json:"accepted_finding_rate"`
	SuppressionRate           float64 `json:"suppression_rate"`
	OutcomeSource             string  `json:"outcome_source"`
}

type GateThresholds struct {
	MinPrecisionIsh              *float64
	MaxFalsePositiveRate         *float64
	MinAcceptedExpectedRate      *float64
	MaxSuppressionRate           *float64
	MaxDuplicateRate             *float64
	Baseline                     *Metrics
	MaxPrecisionDrop             *float64
	MaxAcceptedExpectedRateDrop  *float64
	MaxFalsePositiveRateIncrease *float64
	MaxSuppressionRateIncrease   *float64
	MaxDuplicateRateIncrease     *float64
}

type GateResult struct {
	Passed   bool     `json:"passed"`
	Failures []string `json:"failures,omitempty"`
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
	detectorDuplicateNoiseControl = "duplicate_noise_control"

	reviewDecisionAccepted      = "accepted"
	reviewDecisionDismissed     = "dismissed"
	reviewDecisionSuppressed    = "suppressed"
	reviewDecisionNotActionable = "not_actionable"
	reviewDecisionDuplicate     = "duplicate"
	reviewDecisionStale         = "stale"

	reviewOutcomeSourceDerived  = "derived_from_detector_matches"
	reviewOutcomeSourceExplicit = "explicit_review_outcomes"
	reviewOutcomeSourceMixed    = "mixed"
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
	duplicateMetrics := summarizeDuplicateFindings(actual)
	reviewOutcomes, reviewMetrics, err := evaluateReviewOutcomes(spec.ReviewOutcomes, actual, acceptedExpected, falsePositiveIDs)
	if err != nil {
		return RepoReport{}, fmt.Errorf("review outcomes for golden repo %s: %w", name, err)
	}
	return RepoReport{
		Name:                name,
		Path:                repoPath,
		ExpectedFindings:    append([]ExpectedFinding(nil), spec.ExpectedFindings...),
		ActualFindings:      actual,
		AcceptedExpectedIDs: acceptedExpected,
		MissingExpectedIDs:  missingExpected,
		FalsePositiveIDs:    falsePositiveIDs,
		DuplicateMetrics:    duplicateMetrics,
		ReviewOutcomes:      reviewOutcomes,
		ReviewMetrics:       reviewMetrics,
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
		case detectorDuplicateNoiseControl:
			findings = append(findings, duplicateNoiseFindings()...)
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

func duplicateNoiseFindings() []Finding {
	return []Finding{
		{
			ID:           "auth-admin-guard-agent-a",
			Claim:        "Repository settings update route allows workspace members without the admin guard.",
			Category:     "security",
			Severity:     "high",
			Path:         "apps/api/src/routes/repositories.ts",
			StartLine:    12,
			MatchTerms:   []string{"workspace member", "admin guard", "repository settings"},
			DuplicateKey: "auth-admin-guard",
			SourceAgent:  "security-reviewer",
		},
		{
			ID:           "auth-admin-guard-agent-b",
			Claim:        "Repository settings updates can be performed by workspace members because the route omits the admin guard.",
			Category:     "security",
			Severity:     "high",
			Path:         "apps/api/src/routes/repositories.ts",
			StartLine:    12,
			MatchTerms:   []string{"workspace member", "admin guard", "repository settings"},
			DuplicateKey: "auth-admin-guard",
			SourceAgent:  "general-reviewer",
		},
		{
			ID:           "auth-admin-guard-agent-c",
			Claim:        "Workspace member access reaches repository settings updates without an admin authorization check.",
			Category:     "security",
			Severity:     "medium",
			Path:         "apps/api/src/routes/repositories.ts",
			StartLine:    12,
			MatchTerms:   []string{"workspace member", "admin guard", "repository settings"},
			DuplicateKey: "auth-admin-guard",
			SourceAgent:  "architecture-reviewer",
		},
	}
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

func evaluateReviewOutcomes(configured []ReviewOutcome, actual []Finding, acceptedExpectedIDs []string, falsePositiveIDs []string) ([]ReviewOutcome, ReviewMetrics, error) {
	if len(configured) > 0 {
		outcomes, err := validateReviewOutcomes(configured, actual)
		if err != nil {
			return nil, ReviewMetrics{}, err
		}
		return outcomes, summarizeReviewOutcomes(outcomes, reviewOutcomeSourceExplicit), nil
	}
	outcomes := deriveReviewOutcomes(acceptedExpectedIDs, falsePositiveIDs)
	return outcomes, summarizeReviewOutcomes(outcomes, reviewOutcomeSourceDerived), nil
}

func validateReviewOutcomes(configured []ReviewOutcome, actual []Finding) ([]ReviewOutcome, error) {
	actualIDs := make(map[string]bool, len(actual))
	for _, finding := range actual {
		actualIDs[finding.ID] = true
	}
	seen := make(map[string]bool, len(configured))
	outcomes := make([]ReviewOutcome, 0, len(configured))
	for _, outcome := range configured {
		normalized := ReviewOutcome{
			FindingID:   strings.TrimSpace(outcome.FindingID),
			Decision:    strings.ToLower(strings.TrimSpace(outcome.Decision)),
			Publishable: outcome.Publishable,
			Reason:      strings.TrimSpace(outcome.Reason),
		}
		if normalized.FindingID == "" {
			return nil, fmt.Errorf("review outcome finding_id is required")
		}
		if !actualIDs[normalized.FindingID] {
			return nil, fmt.Errorf("review outcome references unknown finding %q", normalized.FindingID)
		}
		if seen[normalized.FindingID] {
			return nil, fmt.Errorf("review outcome for finding %q is duplicated", normalized.FindingID)
		}
		seen[normalized.FindingID] = true
		if !reviewDecisionKnown(normalized.Decision) {
			return nil, fmt.Errorf("review outcome for finding %q has unknown decision %q", normalized.FindingID, outcome.Decision)
		}
		if normalized.Publishable && normalized.Decision != reviewDecisionAccepted {
			return nil, fmt.Errorf("review outcome for finding %q is publishable without accepted decision", normalized.FindingID)
		}
		outcomes = append(outcomes, normalized)
	}
	sortReviewOutcomes(outcomes)
	return outcomes, nil
}

func deriveReviewOutcomes(acceptedExpectedIDs []string, falsePositiveIDs []string) []ReviewOutcome {
	outcomes := make([]ReviewOutcome, 0, len(acceptedExpectedIDs)+len(falsePositiveIDs))
	for _, id := range acceptedExpectedIDs {
		outcomes = append(outcomes, ReviewOutcome{
			FindingID:   id,
			Decision:    reviewDecisionAccepted,
			Publishable: true,
			Reason:      "matched expected finding",
		})
	}
	for _, id := range falsePositiveIDs {
		outcomes = append(outcomes, ReviewOutcome{
			FindingID: id,
			Decision:  reviewDecisionDismissed,
			Reason:    "unmatched eval finding",
		})
	}
	sortReviewOutcomes(outcomes)
	return outcomes
}

func summarizeDuplicateFindings(findings []Finding) DuplicateMetrics {
	if len(findings) == 0 {
		return DuplicateMetrics{}
	}
	clusters := map[string]int{}
	for _, finding := range findings {
		key := strings.TrimSpace(finding.DuplicateKey)
		if key == "" {
			key = inferredDuplicateKey(finding)
		}
		clusters[key]++
	}
	metrics := DuplicateMetrics{}
	for _, size := range clusters {
		if size <= 1 {
			continue
		}
		metrics.ClusterCount++
		metrics.DuplicateFindings += size - 1
	}
	metrics.DuplicateRate = float64(metrics.DuplicateFindings) / float64(len(findings))
	return metrics
}

func inferredDuplicateKey(finding Finding) string {
	claim := strings.ToLower(finding.Claim)
	claim = strings.Join(strings.FieldsFunc(claim, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}), " ")
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(finding.Path)),
		strings.ToLower(strings.TrimSpace(finding.Category)),
		claim,
	}, "|")
}

func summarizeReviewOutcomes(outcomes []ReviewOutcome, source string) ReviewMetrics {
	metrics := ReviewMetrics{
		ReviewedFindings: len(outcomes),
		OutcomeSource:    source,
	}
	for _, outcome := range outcomes {
		switch outcome.Decision {
		case reviewDecisionAccepted:
			metrics.AcceptedFindings++
		case reviewDecisionDismissed:
			metrics.DismissedFindings++
		case reviewDecisionSuppressed:
			metrics.SuppressedFindings++
		case reviewDecisionNotActionable:
			metrics.NotActionableFindings++
		case reviewDecisionDuplicate:
			metrics.DuplicateDecisionFindings++
		case reviewDecisionStale:
			metrics.StaleFindings++
		}
		if outcome.Publishable {
			metrics.PublishableFindings++
		}
	}
	if metrics.ReviewedFindings > 0 {
		metrics.AcceptedFindingRate = float64(metrics.AcceptedFindings) / float64(metrics.ReviewedFindings)
		metrics.SuppressionRate = float64(metrics.SuppressedFindings+metrics.NotActionableFindings+metrics.DuplicateDecisionFindings+metrics.StaleFindings) / float64(metrics.ReviewedFindings)
	} else {
		metrics.AcceptedFindingRate = 1
	}
	return metrics
}

func EvaluateGates(current Metrics, thresholds GateThresholds) GateResult {
	failures := []string{}
	addFailure := func(format string, args ...any) {
		failures = append(failures, fmt.Sprintf(format, args...))
	}
	if thresholds.MinPrecisionIsh != nil && *thresholds.MinPrecisionIsh >= 0 && current.PrecisionIsh < *thresholds.MinPrecisionIsh {
		addFailure("precision-ish %.3f below minimum %.3f", current.PrecisionIsh, *thresholds.MinPrecisionIsh)
	}
	if thresholds.MaxFalsePositiveRate != nil && *thresholds.MaxFalsePositiveRate >= 0 && current.FalsePositiveRate > *thresholds.MaxFalsePositiveRate {
		addFailure("false-positive rate %.3f above maximum %.3f", current.FalsePositiveRate, *thresholds.MaxFalsePositiveRate)
	}
	if thresholds.MinAcceptedExpectedRate != nil && *thresholds.MinAcceptedExpectedRate >= 0 && current.AcceptedExpectedRate < *thresholds.MinAcceptedExpectedRate {
		addFailure("accepted-expected rate %.3f below minimum %.3f", current.AcceptedExpectedRate, *thresholds.MinAcceptedExpectedRate)
	}
	if thresholds.MaxSuppressionRate != nil && *thresholds.MaxSuppressionRate >= 0 && current.SuppressionRate > *thresholds.MaxSuppressionRate {
		addFailure("suppression rate %.3f above maximum %.3f", current.SuppressionRate, *thresholds.MaxSuppressionRate)
	}
	if thresholds.MaxDuplicateRate != nil && *thresholds.MaxDuplicateRate >= 0 && current.DuplicateRate > *thresholds.MaxDuplicateRate {
		addFailure("duplicate rate %.3f above maximum %.3f", current.DuplicateRate, *thresholds.MaxDuplicateRate)
	}
	if thresholds.Baseline != nil {
		if thresholds.MaxPrecisionDrop != nil && *thresholds.MaxPrecisionDrop >= 0 {
			drop := thresholds.Baseline.PrecisionIsh - current.PrecisionIsh
			if drop > *thresholds.MaxPrecisionDrop {
				addFailure("precision-ish dropped by %.3f from baseline %.3f to %.3f", drop, thresholds.Baseline.PrecisionIsh, current.PrecisionIsh)
			}
		}
		if thresholds.MaxAcceptedExpectedRateDrop != nil && *thresholds.MaxAcceptedExpectedRateDrop >= 0 {
			drop := thresholds.Baseline.AcceptedExpectedRate - current.AcceptedExpectedRate
			if drop > *thresholds.MaxAcceptedExpectedRateDrop {
				addFailure("accepted-expected rate dropped by %.3f from baseline %.3f to %.3f", drop, thresholds.Baseline.AcceptedExpectedRate, current.AcceptedExpectedRate)
			}
		}
		if thresholds.MaxFalsePositiveRateIncrease != nil && *thresholds.MaxFalsePositiveRateIncrease >= 0 {
			increase := current.FalsePositiveRate - thresholds.Baseline.FalsePositiveRate
			if increase > *thresholds.MaxFalsePositiveRateIncrease {
				addFailure("false-positive rate increased by %.3f from baseline %.3f to %.3f", increase, thresholds.Baseline.FalsePositiveRate, current.FalsePositiveRate)
			}
		}
		if thresholds.MaxSuppressionRateIncrease != nil && *thresholds.MaxSuppressionRateIncrease >= 0 {
			increase := current.SuppressionRate - thresholds.Baseline.SuppressionRate
			if increase > *thresholds.MaxSuppressionRateIncrease {
				addFailure("suppression rate increased by %.3f from baseline %.3f to %.3f", increase, thresholds.Baseline.SuppressionRate, current.SuppressionRate)
			}
		}
		if thresholds.MaxDuplicateRateIncrease != nil && *thresholds.MaxDuplicateRateIncrease >= 0 {
			increase := current.DuplicateRate - thresholds.Baseline.DuplicateRate
			if increase > *thresholds.MaxDuplicateRateIncrease {
				addFailure("duplicate rate increased by %.3f from baseline %.3f to %.3f", increase, thresholds.Baseline.DuplicateRate, current.DuplicateRate)
			}
		}
	}
	return GateResult{
		Passed:   len(failures) == 0,
		Failures: failures,
	}
}

func sortReviewOutcomes(outcomes []ReviewOutcome) {
	sort.Slice(outcomes, func(i, j int) bool {
		if outcomes[i].FindingID == outcomes[j].FindingID {
			return outcomes[i].Decision < outcomes[j].Decision
		}
		return outcomes[i].FindingID < outcomes[j].FindingID
	})
}

func reviewDecisionKnown(decision string) bool {
	switch decision {
	case reviewDecisionAccepted, reviewDecisionDismissed, reviewDecisionSuppressed, reviewDecisionNotActionable, reviewDecisionDuplicate, reviewDecisionStale:
		return true
	default:
		return false
	}
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
		RepoCount:            len(reports),
		DurationMs:           duration.Milliseconds(),
		CostSource:           "unavailable_for_static_harness",
		AcceptedFindingRate:  1,
		AcceptedExpectedRate: 1,
	}
	for _, report := range reports {
		metrics.ExpectedFindings += len(report.ExpectedFindings)
		metrics.ActualFindings += len(report.ActualFindings)
		metrics.DuplicateClusters += report.DuplicateMetrics.ClusterCount
		metrics.DuplicateFindings += report.DuplicateMetrics.DuplicateFindings
		metrics.AcceptedExpected += len(report.AcceptedExpectedIDs)
		metrics.MissingExpected += len(report.MissingExpectedIDs)
		metrics.FalsePositives += len(report.FalsePositiveIDs)
		metrics.ReviewedFindings += report.ReviewMetrics.ReviewedFindings
		metrics.AcceptedFindings += report.ReviewMetrics.AcceptedFindings
		metrics.DismissedFindings += report.ReviewMetrics.DismissedFindings
		metrics.PublishableFindings += report.ReviewMetrics.PublishableFindings
		metrics.SuppressedFindings += report.ReviewMetrics.SuppressedFindings
		metrics.NotActionableFindings += report.ReviewMetrics.NotActionableFindings
		metrics.ReviewOutcomeSource = mergeReviewOutcomeSource(metrics.ReviewOutcomeSource, report.ReviewMetrics.OutcomeSource)
		for _, file := range report.FileResults {
			if !file.Matched {
				metrics.FalsePositives++
			}
		}
	}
	if metrics.ActualFindings > 0 {
		metrics.PrecisionIsh = float64(metrics.AcceptedExpected) / float64(metrics.ActualFindings)
		metrics.DuplicateRate = float64(metrics.DuplicateFindings) / float64(metrics.ActualFindings)
	} else if metrics.ExpectedFindings == 0 {
		metrics.PrecisionIsh = 1
	}
	if metrics.ExpectedFindings > 0 {
		metrics.AcceptedExpectedRate = float64(metrics.AcceptedExpected) / float64(metrics.ExpectedFindings)
	}
	if metrics.ActualFindings > 0 {
		metrics.FalsePositiveRate = float64(metrics.FalsePositives) / float64(metrics.ActualFindings)
	}
	if metrics.ReviewedFindings > 0 {
		metrics.AcceptedFindingRate = float64(metrics.AcceptedFindings) / float64(metrics.ReviewedFindings)
		metrics.SuppressionRate = float64(metrics.SuppressedFindings+metrics.NotActionableFindings) / float64(metrics.ReviewedFindings)
	}
	if metrics.ReviewOutcomeSource == "" {
		metrics.ReviewOutcomeSource = reviewOutcomeSourceDerived
	}
	return metrics
}

func mergeReviewOutcomeSource(current string, next string) string {
	if current == "" {
		return next
	}
	if next == "" || current == next {
		return current
	}
	return reviewOutcomeSourceMixed
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
