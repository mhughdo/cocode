package evidence

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/testkit/goldenrepo"
)

func TestVerifySessionCreatesPrimaryAndCounterEvidence(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	finding := createEvidenceFinding(t, env.Queries, dbgen.CreateFindingParams{
		ID:                 "finding_auth",
		ReviewSessionID:    "session_1",
		CanonicalClaim:     "Settings mutation lacks admin guard",
		Category:           "security",
		Severity:           "high",
		Confidence:         0.92,
		VerificationStatus: StatusUnverified,
		DecisionStatus:     "undecided",
		PrimaryPath:        nullableTestString("src/handler.go"),
		PrimaryStartLine:   nullableTestInt64(4),
		PrimaryEndLine:     nullableTestInt64(4),
		Fingerprint:        "fp_auth",
		MergedFromCount:    1,
		FirstSeenAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:          "2026-05-03T00:04:00Z",
	})
	env.Searcher.matches = map[string][]SearchMatch{
		"admin": {
			{Path: "src/auth.go", Line: 2, Text: "func RequireAdmin() bool { return true }"},
			{Path: "src/handler_test.go", Line: 5, Text: "func TestRequireAdmin(t *testing.T) {}"},
		},
	}

	summary, err := env.Service.VerifySession(context.Background(), env.Session, env.Repository)
	if err != nil {
		t.Fatalf("VerifySession() error = %v", err)
	}
	if summary.Findings != 1 ||
		summary.EvidenceItemsCreated != 3 ||
		summary.SupportingEvidence != 1 ||
		summary.CounterEvidence != 2 ||
		summary.ByVerificationStatus[StatusPlausible] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	updated, err := env.Queries.GetFinding(context.Background(), finding.ID)
	if err != nil {
		t.Fatalf("GetFinding() error = %v", err)
	}
	if updated.VerificationStatus != StatusPlausible ||
		!strings.Contains(nullableTestValue(updated.EvidenceSummary), "anchored to changed code") ||
		!strings.Contains(nullableTestValue(updated.CounterEvidenceSummary), "possible guard") {
		t.Fatalf("updated finding = %+v", updated)
	}
	items, err := env.Queries.ListEvidenceItemsByFinding(context.Background(), finding.ID)
	if err != nil {
		t.Fatalf("ListEvidenceItemsByFinding() error = %v", err)
	}
	if len(items) != 3 ||
		countEvidenceKind(items, KindSupporting) != 1 ||
		countEvidenceKind(items, KindCounter) != 1 ||
		countEvidenceKind(items, KindTest) != 1 {
		t.Fatalf("items = %+v", items)
	}
	var metadata map[string]any
	supporting := evidenceItemByKind(t, items, KindSupporting)
	if err := json.Unmarshal([]byte(supporting.MetadataJson), &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadata["producer"] != "local_verifier" ||
		metadata["source"] != "primary_location" ||
		!strings.Contains(metadata["code_snippet"].(string), "RequireAdmin") {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func TestVerifySessionIgnoresProjectMetadataCounterEvidence(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	createEvidenceFinding(t, env.Queries, dbgen.CreateFindingParams{
		ID:                 "finding_auth_metadata",
		ReviewSessionID:    "session_1",
		CanonicalClaim:     "Invoice export lacks admin guard",
		Category:           "security",
		Severity:           "high",
		Confidence:         0.9,
		VerificationStatus: StatusUnverified,
		DecisionStatus:     "undecided",
		PrimaryPath:        nullableTestString("src/server.js"),
		PrimaryStartLine:   nullableTestInt64(19),
		PrimaryEndLine:     nullableTestInt64(19),
		Fingerprint:        "fp_auth_metadata",
		MergedFromCount:    1,
		FirstSeenAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:          "2026-05-03T00:04:00Z",
	})
	env.Searcher.matches = map[string][]SearchMatch{
		"admin": {
			{Path: "package.json", Line: 1, Text: `{"scripts":{"test":"node --test"}}`},
			{Path: "test/server.test.js", Line: 7, Text: "assert.throws(() => exportInvoices(), /admin required/)"},
		},
	}

	_, err := env.Service.VerifySession(context.Background(), env.Session, env.Repository)
	if err != nil {
		t.Fatalf("VerifySession() error = %v", err)
	}
	items, err := env.Queries.ListEvidenceItemsByFinding(context.Background(), "finding_auth_metadata")
	if err != nil {
		t.Fatalf("ListEvidenceItemsByFinding() error = %v", err)
	}
	for _, item := range items {
		if nullableTestValue(item.Path) == "package.json" {
			t.Fatalf("package metadata should not be counter-evidence: %+v", items)
		}
	}
	if countEvidenceKind(items, KindTest) != 1 {
		t.Fatalf("expected one useful test evidence item, got %+v", items)
	}
}

func TestVerifySessionAvoidsLooseGenericCounterEvidence(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	createEvidenceFinding(t, env.Queries, dbgen.CreateFindingParams{
		ID:                 "finding_generic",
		ReviewSessionID:    "session_1",
		CanonicalClaim:     "Rewards are matched by token id without the NFT contract address",
		Category:           "correctness",
		Severity:           "high",
		Confidence:         0.78,
		VerificationStatus: StatusUnverified,
		DecisionStatus:     "undecided",
		PrimaryPath:        nullableTestString("src/handler.go"),
		PrimaryStartLine:   nullableTestInt64(4),
		PrimaryEndLine:     nullableTestInt64(4),
		Fingerprint:        "fp_generic",
		MergedFromCount:    1,
		FirstSeenAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:          "2026-05-03T00:04:00Z",
	})
	env.Searcher.matches = map[string][]SearchMatch{
		"test": {
			{Path: "docs/rewards.md", Line: 9, Text: "token id examples"},
			{Path: "docker-compose.yaml", Line: 8, Text: "ENABLE_TOKEN_TEST=true"},
			{Path: "src/handler_test.go", Line: 12, Text: "func TestRewardsUseContractAddress(t *testing.T) {}"},
		},
		"token": {
			{Path: "docs/loose-token-notes.md", Line: 4, Text: "token id"},
		},
	}

	_, err := env.Service.VerifySession(context.Background(), env.Session, env.Repository)
	if err != nil {
		t.Fatalf("VerifySession() error = %v", err)
	}
	items, err := env.Queries.ListEvidenceItemsByFinding(context.Background(), "finding_generic")
	if err != nil {
		t.Fatalf("ListEvidenceItemsByFinding() error = %v", err)
	}
	if countEvidenceKind(items, KindTest) != 1 || countEvidenceKind(items, KindCounter) != 0 {
		t.Fatalf("expected only one useful test signal, got %+v", items)
	}
	for _, item := range items {
		path := nullableTestValue(item.Path)
		if strings.Contains(path, "docs/") || strings.Contains(path, "docker-compose") {
			t.Fatalf("loose metadata/docs signal should not become evidence: %+v", items)
		}
	}
	if containsString(env.Searcher.queries, "token") {
		t.Fatalf("generic profile should not issue broad claim-token searches: %+v", env.Searcher.queries)
	}
}

func TestVerifySessionAssignsVerifiedWhenNoCounterEvidence(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	createEvidenceFinding(t, env.Queries, dbgen.CreateFindingParams{
		ID:                 "finding_verified",
		ReviewSessionID:    "session_1",
		CanonicalClaim:     "Settings mutation lacks admin guard",
		Category:           "security",
		Severity:           "high",
		Confidence:         0.92,
		VerificationStatus: StatusUnverified,
		DecisionStatus:     "undecided",
		PrimaryPath:        nullableTestString("src/handler.go"),
		PrimaryStartLine:   nullableTestInt64(4),
		PrimaryEndLine:     nullableTestInt64(4),
		Fingerprint:        "fp_verified",
		MergedFromCount:    1,
		FirstSeenAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:          "2026-05-03T00:04:00Z",
	})

	summary, err := env.Service.VerifySession(context.Background(), env.Session, env.Repository)
	if err != nil {
		t.Fatalf("VerifySession() error = %v", err)
	}
	if summary.ByVerificationStatus[StatusVerified] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestVerifySessionAssignsNeedsHumanForMissingLocation(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	createEvidenceFinding(t, env.Queries, dbgen.CreateFindingParams{
		ID:                 "finding_missing",
		ReviewSessionID:    "session_1",
		CanonicalClaim:     "A claim without a location",
		Category:           "correctness",
		Severity:           "medium",
		Confidence:         0.5,
		VerificationStatus: StatusUnverified,
		DecisionStatus:     "undecided",
		Fingerprint:        "fp_missing",
		MergedFromCount:    1,
		FirstSeenAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:          "2026-05-03T00:04:00Z",
	})

	summary, err := env.Service.VerifySession(context.Background(), env.Session, env.Repository)
	if err != nil {
		t.Fatalf("VerifySession() error = %v", err)
	}
	if summary.ByVerificationStatus[StatusNeedsHuman] != 1 || summary.MissingEvidence != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	items, err := env.Queries.ListEvidenceItemsByFinding(context.Background(), "finding_missing")
	if err != nil {
		t.Fatalf("ListEvidenceItemsByFinding() error = %v", err)
	}
	if len(items) != 1 || items[0].Kind != KindMissing {
		t.Fatalf("items = %+v", items)
	}
}

func TestVerifySessionAssignsNotActionableForWeakUnlocatedClaim(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	createEvidenceFinding(t, env.Queries, dbgen.CreateFindingParams{
		ID:                 "finding_weak",
		ReviewSessionID:    "session_1",
		CanonicalClaim:     "This might be a little confusing",
		Category:           "other",
		Severity:           "low",
		Confidence:         0.2,
		VerificationStatus: StatusUnverified,
		DecisionStatus:     "undecided",
		Fingerprint:        "fp_weak",
		MergedFromCount:    1,
		FirstSeenAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:          "2026-05-03T00:04:00Z",
	})

	summary, err := env.Service.VerifySession(context.Background(), env.Session, env.Repository)
	if err != nil {
		t.Fatalf("VerifySession() error = %v", err)
	}
	if summary.ByVerificationStatus[StatusNotActionable] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestVerifySessionReplacesPriorLocalVerifierEvidence(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	createEvidenceFinding(t, env.Queries, dbgen.CreateFindingParams{
		ID:                 "finding_rerun",
		ReviewSessionID:    "session_1",
		CanonicalClaim:     "Settings mutation lacks admin guard",
		Category:           "security",
		Severity:           "high",
		Confidence:         0.92,
		VerificationStatus: StatusUnverified,
		DecisionStatus:     "undecided",
		PrimaryPath:        nullableTestString("src/handler.go"),
		PrimaryStartLine:   nullableTestInt64(4),
		PrimaryEndLine:     nullableTestInt64(4),
		Fingerprint:        "fp_rerun",
		MergedFromCount:    1,
		FirstSeenAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:          "2026-05-03T00:04:00Z",
	})

	if _, err := env.Queries.CreateEvidenceItem(context.Background(), dbgen.CreateEvidenceItemParams{
		ID:           "old_local",
		FindingID:    "finding_rerun",
		Kind:         KindSupporting,
		Title:        "old",
		Summary:      "old local verifier evidence",
		Confidence:   1,
		MetadataJson: `{"producer":"local_verifier"}`,
		CreatedAt:    "2026-05-03T00:05:00Z",
	}); err != nil {
		t.Fatalf("CreateEvidenceItem(local) error = %v", err)
	}
	if _, err := env.Queries.CreateEvidenceItem(context.Background(), dbgen.CreateEvidenceItemParams{
		ID:           "agent_evidence",
		FindingID:    "finding_rerun",
		Kind:         KindAgent,
		Title:        "agent",
		Summary:      "agent evidence should be preserved",
		Confidence:   1,
		MetadataJson: `{"producer":"agent"}`,
		CreatedAt:    "2026-05-03T00:06:00Z",
	}); err != nil {
		t.Fatalf("CreateEvidenceItem(agent) error = %v", err)
	}

	if _, err := env.Service.VerifySession(context.Background(), env.Session, env.Repository); err != nil {
		t.Fatalf("VerifySession() error = %v", err)
	}
	items, err := env.Queries.ListEvidenceItemsByFinding(context.Background(), "finding_rerun")
	if err != nil {
		t.Fatalf("ListEvidenceItemsByFinding() error = %v", err)
	}
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	if ids["old_local"] || !ids["agent_evidence"] || len(items) != 2 {
		t.Fatalf("items after rerun = %+v", items)
	}
}

func TestGoldenAuthRepoVerifierBuildsEvidenceMap(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	env.Repository.LocalPath = goldenrepo.Path(t, goldenrepo.AuthBug)
	if _, err := env.Queries.CreateChangedFile(context.Background(), dbgen.CreateChangedFileParams{
		ID:             "changed_auth_route",
		SnapshotID:     env.Session.SnapshotID,
		Path:           "apps/api/src/routes/repositories.ts",
		Status:         "modified",
		Additions:      4,
		Deletions:      1,
		LineRangesJson: `[[10,18]]`,
		CreatedAt:      "2026-05-03T00:03:10Z",
	}); err != nil {
		t.Fatalf("CreateChangedFile(auth route) error = %v", err)
	}
	env.Searcher.matches = map[string][]SearchMatch{
		"RequireAdmin": {
			{Path: "apps/api/src/middleware/auth.ts", Line: 8, Text: "export function requireWorkspaceAdmin(request, response, next) {"},
		},
	}
	finding := createEvidenceFinding(t, env.Queries, dbgen.CreateFindingParams{
		ID:                 "finding_golden_auth",
		ReviewSessionID:    env.Session.ID,
		CanonicalClaim:     "Repository settings update route lacks workspace admin guard",
		Category:           "security",
		Severity:           "high",
		Confidence:         0.92,
		VerificationStatus: StatusUnverified,
		DecisionStatus:     "undecided",
		PrimaryPath:        nullableTestString("apps/api/src/routes/repositories.ts"),
		PrimaryStartLine:   nullableTestInt64(10),
		PrimaryEndLine:     nullableTestInt64(18),
		Fingerprint:        "fp_golden_auth",
		MergedFromCount:    1,
		FirstSeenAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:          "2026-05-03T00:04:00Z",
	})

	summary, err := env.Service.VerifySession(context.Background(), env.Session, env.Repository)
	if err != nil {
		t.Fatalf("VerifySession() error = %v", err)
	}
	if summary.SupportingEvidence != 1 || summary.CounterEvidence != 1 || summary.ByVerificationStatus[StatusPlausible] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	updated, err := env.Queries.GetFinding(context.Background(), finding.ID)
	if err != nil {
		t.Fatalf("GetFinding() error = %v", err)
	}
	view, err := env.Service.RebuildEvidenceMap(context.Background(), updated)
	if err != nil {
		t.Fatalf("RebuildEvidenceMap() error = %v", err)
	}
	if view.Graph.Status != GraphStatusReady ||
		!hasMapNode(view.Nodes, NodeChangedCode, "apps/api/src/routes/repositories.ts") ||
		!hasMapNode(view.Nodes, NodeCounterEvidence, "apps/api/src/middleware/auth.ts") ||
		!hasMapEdge(view.Edges, EdgeMissingGuard, EdgeStatusMissing) {
		t.Fatalf("view = %+v", view)
	}
}

func TestVerifierInfersPrimaryLineFromChangedFileRange(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	createEvidenceFinding(t, env.Queries, dbgen.CreateFindingParams{
		ID:                 "finding_missing_line",
		ReviewSessionID:    env.Session.ID,
		CanonicalClaim:     "Handler changed file has a missing admin guard",
		Category:           "security",
		Severity:           "high",
		Confidence:         0.84,
		VerificationStatus: StatusUnverified,
		DecisionStatus:     "undecided",
		PrimaryPath:        nullableTestString("src/handler.go"),
		Fingerprint:        "fp_missing_line",
		MergedFromCount:    1,
		FirstSeenAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:          "2026-05-03T00:04:00Z",
	})

	if _, err := env.Service.VerifySession(context.Background(), env.Session, env.Repository); err != nil {
		t.Fatalf("VerifySession() error = %v", err)
	}
	items, err := env.Queries.ListEvidenceItemsByFinding(context.Background(), "finding_missing_line")
	if err != nil {
		t.Fatalf("ListEvidenceItemsByFinding() error = %v", err)
	}
	item := evidenceItemByKind(t, items, KindSupporting)
	if !item.StartLine.Valid || item.StartLine.Int64 != 4 || !strings.Contains(item.Summary, "src/handler.go:4") {
		t.Fatalf("supporting evidence = %+v", item)
	}
}

func TestGoldenWebhookRepoVerifierDetectsMissingValidation(t *testing.T) {
	t.Parallel()

	env := setupEvidenceEnv(t)
	env.Repository.LocalPath = goldenrepo.Path(t, goldenrepo.WebhookValidation)
	if _, err := env.Queries.CreateChangedFile(context.Background(), dbgen.CreateChangedFileParams{
		ID:             "changed_webhook",
		SnapshotID:     env.Session.SnapshotID,
		Path:           "apps/api/src/webhooks/stripe.ts",
		Status:         "modified",
		Additions:      8,
		LineRangesJson: `[[3,11]]`,
		CreatedAt:      "2026-05-03T00:03:20Z",
	}); err != nil {
		t.Fatalf("CreateChangedFile(webhook) error = %v", err)
	}
	createEvidenceFinding(t, env.Queries, dbgen.CreateFindingParams{
		ID:                 "finding_golden_webhook",
		ReviewSessionID:    env.Session.ID,
		CanonicalClaim:     "Webhook handler accepts payload without signature verification",
		Category:           "security",
		Severity:           "high",
		Confidence:         0.9,
		VerificationStatus: StatusUnverified,
		DecisionStatus:     "undecided",
		PrimaryPath:        nullableTestString("apps/api/src/webhooks/stripe.ts"),
		PrimaryStartLine:   nullableTestInt64(3),
		PrimaryEndLine:     nullableTestInt64(11),
		Fingerprint:        "fp_golden_webhook",
		MergedFromCount:    1,
		FirstSeenAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:          "2026-05-03T00:04:00Z",
	})

	summary, err := env.Service.VerifySession(context.Background(), env.Session, env.Repository)
	if err != nil {
		t.Fatalf("VerifySession() error = %v", err)
	}
	if summary.SupportingEvidence != 1 || summary.CounterEvidence != 0 || summary.ByVerificationStatus[StatusVerified] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	items, err := env.Queries.ListEvidenceItemsByFinding(context.Background(), "finding_golden_webhook")
	if err != nil {
		t.Fatalf("ListEvidenceItemsByFinding() error = %v", err)
	}
	supporting := evidenceItemByKind(t, items, KindSupporting)
	if !strings.Contains(supporting.MetadataJson, "JSON.parse") {
		t.Fatalf("supporting evidence metadata = %s", supporting.MetadataJson)
	}
}

func TestAssignVerificationStatusRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		finding    dbgen.Finding
		supporting int
		counter    int
		missing    int
		want       string
	}{
		{
			name:       "supporting only is verified",
			supporting: 1,
			want:       StatusVerified,
		},
		{
			name:       "supporting and counter evidence remains plausible",
			supporting: 1,
			counter:    1,
			want:       StatusPlausible,
		},
		{
			name:    "counter evidence only is likely false positive",
			counter: 1,
			want:    StatusLikelyFalsePositive,
		},
		{
			name: "weak missing evidence without fix is not actionable",
			finding: dbgen.Finding{
				Confidence: 0.3,
			},
			missing: 1,
			want:    StatusNotActionable,
		},
		{
			name: "missing evidence otherwise needs human",
			finding: dbgen.Finding{
				Confidence:   0.3,
				SuggestedFix: nullableTestString("add a guard"),
			},
			missing: 1,
			want:    StatusNeedsHuman,
		},
		{
			name: "empty evidence needs human",
			want: StatusNeedsHuman,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := assignVerificationStatus(tt.finding, tt.supporting, tt.counter, tt.missing)
			if got != tt.want {
				t.Fatalf("assignVerificationStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRuleProfilesAddDeterministicSearchTerms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		finding dbgen.Finding
		rule    string
		terms   []string
	}{
		{
			name: "auth guard",
			finding: dbgen.Finding{
				CanonicalClaim: "Repository update lacks admin permission guard",
				Category:       "security",
			},
			rule:  "auth_guard",
			terms: []string{"auth", "permission", "RequireAdmin"},
		},
		{
			name: "webhook",
			finding: dbgen.Finding{
				CanonicalClaim: "Webhook handler accepts payload without signature verification",
				Category:       "security",
			},
			rule:  "webhook_validation",
			terms: []string{"signature", "hmac", "webhook"},
		},
		{
			name: "idempotency",
			finding: dbgen.Finding{
				CanonicalClaim: "Retry can create duplicate payment without idempotency key",
				Category:       "reliability",
			},
			rule:  "idempotency",
			terms: []string{"idempotency", "unique", "retry"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			profile := classifyRuleProfile(tt.finding)
			if profile.ID != tt.rule {
				t.Fatalf("profile = %+v, want rule %q", profile, tt.rule)
			}
			terms := counterEvidenceTerms(tt.finding, profile)
			for _, term := range tt.terms {
				if !containsString(terms, term) {
					t.Fatalf("terms = %+v, want %q", terms, term)
				}
			}
		})
	}
}

type evidenceEnv struct {
	Database   *sql.DB
	Queries    *dbgen.Queries
	Service    *Service
	Searcher   *fakeEvidenceSearcher
	Session    dbgen.ReviewSession
	Repository dbgen.Repository
}

type fakeEvidenceSearcher struct {
	matches map[string][]SearchMatch
	err     error
	queries []string
}

func (s *fakeEvidenceSearcher) Search(_ context.Context, options SearchOptions) ([]SearchMatch, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.queries = append(s.queries, options.Query)
	matches := append([]SearchMatch(nil), s.matches[options.Query]...)
	if options.Limit > 0 && len(matches) > options.Limit {
		matches = matches[:options.Limit]
	}
	return matches, nil
}

func setupEvidenceEnv(t *testing.T) evidenceEnv {
	t.Helper()

	database, err := db.Open(context.Background(), db.MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Apply(context.Background(), database, db.Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	queries := dbgen.New(database)
	repoPath := t.TempDir()
	writeEvidenceRepoFile(t, repoPath, "src/handler.go", strings.Join([]string{
		"package src",
		"",
		"func UpdateSettings() {",
		"    _ = RequireAdmin()",
		"}",
	}, "\n")+"\n")
	writeEvidenceRepoFile(t, repoPath, "src/auth.go", "package src\n\nfunc RequireAdmin() bool { return true }\n")
	createEvidenceBaseRows(t, queries, repoPath)
	searcher := &fakeEvidenceSearcher{matches: map[string][]SearchMatch{}}
	service := &Service{
		Queries:  queries,
		Searcher: searcher,
		Now: func() time.Time {
			return time.Date(2026, 5, 3, 0, 10, 0, 0, time.UTC)
		},
		ContextLines: 1,
	}
	session, err := queries.GetReviewSession(context.Background(), "session_1")
	if err != nil {
		t.Fatalf("GetReviewSession() error = %v", err)
	}
	repository, err := queries.GetRepository(context.Background(), "repo_1")
	if err != nil {
		t.Fatalf("GetRepository() error = %v", err)
	}
	return evidenceEnv{
		Database:   database,
		Queries:    queries,
		Service:    service,
		Searcher:   searcher,
		Session:    session,
		Repository: repository,
	}
}

func createEvidenceBaseRows(t *testing.T, queries *dbgen.Queries, repoPath string) {
	t.Helper()

	if _, err := queries.CreateWorkspace(context.Background(), dbgen.CreateWorkspaceParams{
		ID:           "workspace_1",
		Name:         "cocode",
		RootPath:     repoPath,
		SettingsJson: "{}",
		CreatedAt:    "2026-05-03T00:00:00Z",
		UpdatedAt:    "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	repository, err := queries.CreateRepository(context.Background(), dbgen.CreateRepositoryParams{
		ID:          "repo_1",
		WorkspaceID: "workspace_1",
		Name:        "cocode",
		LocalPath:   repoPath,
		CreatedAt:   "2026-05-03T00:01:00Z",
		UpdatedAt:   "2026-05-03T00:01:00Z",
	})
	if err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
	if _, err := queries.CreatePullRequestSnapshot(context.Background(), dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_1",
		RepositoryID: repository.ID,
		SourceType:   "branch_compare",
		BaseRef:      nullableTestString("main"),
		HeadRef:      nullableTestString("feature"),
		HeadSha:      nullableTestString("head-sha"),
		MetadataJson: "{}",
		CreatedAt:    "2026-05-03T00:02:00Z",
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}
	if _, err := queries.CreateChangedFile(context.Background(), dbgen.CreateChangedFileParams{
		ID:             "changed_handler",
		SnapshotID:     "snapshot_1",
		Path:           "src/handler.go",
		Status:         "modified",
		Additions:      1,
		LineRangesJson: `[[4,4]]`,
		CreatedAt:      "2026-05-03T00:03:00Z",
	}); err != nil {
		t.Fatalf("CreateChangedFile() error = %v", err)
	}
	if _, err := queries.CreateReviewSession(context.Background(), dbgen.CreateReviewSessionParams{
		ID:                  "session_1",
		WorkspaceID:         "workspace_1",
		RepositoryID:        repository.ID,
		SnapshotID:          "snapshot_1",
		Title:               "Evidence fixture",
		Status:              "running",
		ReviewDepth:         "standard",
		RuntimeLimitSeconds: 300,
		ContextPolicyJson:   "{}",
		CreatedAt:           "2026-05-03T00:03:30Z",
		UpdatedAt:           "2026-05-03T00:03:30Z",
	}); err != nil {
		t.Fatalf("CreateReviewSession() error = %v", err)
	}
}

func createEvidenceFinding(t *testing.T, queries *dbgen.Queries, params dbgen.CreateFindingParams) dbgen.Finding {
	t.Helper()

	finding, err := queries.CreateFinding(context.Background(), params)
	if err != nil {
		t.Fatalf("CreateFinding() error = %v", err)
	}
	return finding
}

func countEvidenceKind(items []dbgen.EvidenceItem, kind string) int {
	count := 0
	for _, item := range items {
		if item.Kind == kind {
			count++
		}
	}
	return count
}

func evidenceItemByKind(t *testing.T, items []dbgen.EvidenceItem, kind string) dbgen.EvidenceItem {
	t.Helper()

	for _, item := range items {
		if item.Kind == kind {
			return item
		}
	}
	t.Fatalf("evidence kind %q missing from %+v", kind, items)
	return dbgen.EvidenceItem{}
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func nullableTestString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullableTestInt64(value int64) sql.NullInt64 {
	if value <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: value, Valid: true}
}

func nullableTestValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
