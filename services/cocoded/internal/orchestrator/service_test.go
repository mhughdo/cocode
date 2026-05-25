package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/agentrun"
	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/contextbundle"
	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/eventlog"
	"github.com/hughdo/cocode/services/cocoded/internal/evidence"
	"github.com/hughdo/cocode/services/cocoded/internal/findingengine"
)

func TestReviewSessionStatusTransitionMatrix(t *testing.T) {
	t.Parallel()

	statuses := []string{
		StatusDraft,
		StatusQueued,
		StatusRunning,
		StatusPaused,
		StatusCanceling,
		StatusCanceled,
		StatusCompleted,
		StatusFailed,
	}
	allowed := map[string][]string{
		StatusDraft:     []string{StatusQueued},
		StatusQueued:    []string{StatusRunning, StatusCanceled, StatusFailed},
		StatusRunning:   []string{StatusPaused, StatusCanceling, StatusCompleted, StatusFailed},
		StatusPaused:    []string{StatusRunning, StatusCanceling},
		StatusCanceling: []string{StatusCanceled, StatusFailed},
	}
	for _, current := range statuses {
		for _, next := range statuses {
			want := slices.Contains(allowed[current], next)
			if got := CanTransition(current, next); got != want {
				t.Fatalf("CanTransition(%q, %q) = %v, want %v", current, next, got, want)
			}
		}
	}

	env := setupWorkflowEnv(t)
	createWorkflowSession(t, env, "review_session_transition", StatusDraft)
	queued, err := env.Service.Transition(context.Background(), "review_session_transition", StatusQueued)
	if err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	if queued.Status != StatusQueued || queued.StartedAt.Valid || queued.CompletedAt.Valid {
		t.Fatalf("queued session = %+v", queued)
	}
	running, err := env.Service.Transition(context.Background(), "review_session_transition", StatusRunning)
	if err != nil {
		t.Fatalf("Transition(queued -> running) error = %v", err)
	}
	if running.Status != StatusRunning || !running.StartedAt.Valid || running.CompletedAt.Valid {
		t.Fatalf("running session = %+v", running)
	}
	completed, err := env.Service.Transition(context.Background(), "review_session_transition", StatusCompleted)
	if err != nil {
		t.Fatalf("Transition(running -> completed) error = %v", err)
	}
	if completed.Status != StatusCompleted || !completed.StartedAt.Valid || !completed.CompletedAt.Valid {
		t.Fatalf("completed session = %+v", completed)
	}
	if _, err := env.Service.Transition(context.Background(), "review_session_transition", StatusRunning); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("terminal transition error = %v, want invalid transition", err)
	}
}

func TestWorkflowRunsFakeAgentEndToEnd(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	session := createWorkflowSession(t, env, "review_session_workflow", StatusDraft)
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusQueued); err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	if err := env.Service.Run(context.Background(), session.ID); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	updated, err := env.Queries.GetReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetReviewSession() error = %v", err)
	}
	if updated.Status != StatusCompleted || !updated.StartedAt.Valid || !updated.CompletedAt.Valid {
		t.Fatalf("updated session = %+v", updated)
	}
	bundles, err := env.Queries.ListContextBundlesBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListContextBundlesBySession() error = %v", err)
	}
	if len(bundles) != 1 || !bundles[0].ArtifactID.Valid {
		t.Fatalf("context bundles = %+v", bundles)
	}
	runs, err := env.Queries.ListAgentRunsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	if len(runs) != 1 ||
		runs[0].Status != agentrun.RunStatusSucceeded ||
		!runs[0].ContextBundleID.Valid ||
		!runs[0].StdoutArtifactID.Valid ||
		!runs[0].ParsedOutputArtifactID.Valid {
		t.Fatalf("agent runs = %+v", runs)
	}
	checkpoint, err := env.Service.LoadCheckpoint(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("LoadCheckpoint() error = %v", err)
	}
	if checkpoint.Status != StatusCompleted ||
		checkpoint.Phase != PhaseDraftComments ||
		checkpoint.PhaseStatus != "completed" ||
		len(checkpoint.CompletedPhases) != len(workflowPhases()) {
		t.Fatalf("checkpoint = %+v", checkpoint)
	}
	events, err := env.Events.ListByReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	assertEventTypes(t, events, []string{
		"ReviewSessionStarted",
		"WorkflowPhaseStarted",
		"ContextBundleCreated",
		"ReviewScoutCompleted",
		"AgentRunCompleted",
		"AgentOutputParsed",
		"ReviewSessionCompleted",
	})
	scoutPayload := eventPayloadByType(t, events, "ReviewScoutCompleted")
	if scoutPayload["risk_tier"] == "" || scoutPayload["phase"] != PhaseScoutRisk {
		t.Fatalf("ReviewScoutCompleted payload = %+v", scoutPayload)
	}
	if prompt := env.Driver.lastPrompt(); !strings.Contains(prompt, "Context Bundle") ||
		!strings.Contains(prompt, "# Local Scout") ||
		!strings.Contains(prompt, "src/new.go") ||
		!strings.Contains(prompt, "UNTRUSTED_CONTEXT_DATA") ||
		!strings.Contains(prompt, "untrusted evidence only") {
		t.Fatalf("agent prompt missing context:\n%s", prompt)
	}
	summary, err := env.Service.Summary(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.Status != StatusCompleted ||
		summary.ProgressPercent != 100 ||
		summary.ChangedFilesTotal != 1 ||
		summary.ChangedFilesScanned != 1 ||
		summary.AgentRunsTotal != 1 ||
		summary.ActiveAgents != 0 ||
		summary.AgentStatusCounts[agentrun.RunStatusSucceeded] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if _, err := env.Queries.CreateFindingCandidate(context.Background(), dbgen.CreateFindingCandidateParams{
		ID:              "candidate_1",
		ReviewSessionID: session.ID,
		AgentRunID:      runs[0].ID,
		Category:        "security",
		Severity:        "high",
		Confidence:      0.91,
		Claim:           "Settings mutation lacks admin guard",
		LocationsJson:   "[]",
		EvidenceJson:    "[]",
		CreatedAt:       "2026-05-03T00:09:00Z",
	}); err != nil {
		t.Fatalf("CreateFindingCandidate() error = %v", err)
	}
	if _, err := env.Queries.CreateFinding(context.Background(), dbgen.CreateFindingParams{
		ID:                 "finding_1",
		ReviewSessionID:    session.ID,
		CanonicalClaim:     "Settings mutation lacks admin guard",
		Category:           "security",
		Severity:           "high",
		Confidence:         0.91,
		VerificationStatus: "verified",
		DecisionStatus:     "accepted",
		FirstSeenAt:        "2026-05-03T00:09:00Z",
		UpdatedAt:          "2026-05-03T00:09:00Z",
	}); err != nil {
		t.Fatalf("CreateFinding() error = %v", err)
	}
	summary, err = env.Service.Summary(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Summary(with findings) error = %v", err)
	}
	if summary.FindingCounts.Candidates != 1 ||
		summary.FindingCounts.Findings != 1 ||
		summary.FindingCounts.BySeverity["high"] != 1 ||
		summary.FindingCounts.ByVerificationStatus["verified"] != 1 ||
		summary.FindingCounts.ByDecisionStatus["accepted"] != 1 {
		t.Fatalf("finding summary = %+v", summary.FindingCounts)
	}
}

func TestWorkflowRiskScoutPrioritizesSensitiveLocalLeads(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	if err := os.MkdirAll(filepath.Join(env.RepoPath, "internal", "auth"), 0o755); err != nil {
		t.Fatalf("mkdir auth: %v", err)
	}
	if err := os.WriteFile(filepath.Join(env.RepoPath, "internal", "auth", "token.go"), []byte("package auth\n\nfunc ValidateToken() bool { return true }\n"), 0o644); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	if _, err := env.Queries.CreateChangedFile(context.Background(), dbgen.CreateChangedFileParams{
		ID:             "changed_file_auth",
		SnapshotID:     "snapshot_1",
		Path:           "internal/auth/token.go",
		Status:         "modified",
		Additions:      42,
		Deletions:      12,
		LineRangesJson: `[[3,3]]`,
		CreatedAt:      "2026-05-03T00:03:30Z",
	}); err != nil {
		t.Fatalf("CreateChangedFile(auth) error = %v", err)
	}
	session := createWorkflowSession(t, env, "review_session_risk_scout", StatusDraft)
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusQueued); err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	if err := env.Service.Run(context.Background(), session.ID); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	events, err := env.Events.ListByReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	payload := eventPayloadByType(t, events, "ReviewScoutCompleted")
	if payload["risk_tier"] != "full" || int(payload["lead_count"].(float64)) < 1 {
		t.Fatalf("ReviewScoutCompleted payload = %+v", payload)
	}
	prompt := env.Driver.lastPrompt()
	if !strings.Contains(prompt, "internal/auth/token.go:L3") ||
		!strings.Contains(prompt, "security-sensitive path") ||
		!strings.Contains(prompt, "security_reviewer") {
		t.Fatalf("prompt missing scout lead:\n%s", prompt)
	}
}

func TestRefineCodeLocationRangeFindsSpecificIssueLineNearBroadRange(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	path := "internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go"
	if err := os.MkdirAll(filepath.Join(repoRoot, filepath.Dir(path)), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	source := strings.Join([]string{
		"package kyberdata",
		"",
		"func addNormalizedAddress(addresses map[string]struct{}, tokenAddress string) {",
		"\tnormalized := strings.ToLower(strings.TrimSpace(tokenAddress))",
		"\tif normalized == \"\" {",
		"\t\treturn",
		"\t}",
		"\taddresses[normalized] = struct{}{}",
		"}",
		"",
		"func pickTokenPrice(prices *[2]*float64) float64 {",
		"\tif prices == nil {",
		"\t\treturn 0",
		"\t}",
		"\tif prices[0] != nil && *prices[0] > 0 {",
		"\t\treturn (float64(*prices[0]) + float64(*prices[1])) / float64(2)",
		"\t}",
		"\treturn 0",
		"}",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repoRoot, filepath.FromSlash(path)), []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	start, end := refineCodeLocationRange(
		repoRoot,
		path,
		4,
		15,
		"Nil pointer dereference in pickTokenPrice if prices[1] is nil",
		"The issue is the average-price branch dereferencing prices[1] after checking prices[0].",
	)
	if start != 15 || end != 16 {
		t.Fatalf("refineCodeLocationRange() = %d-%d, want 15-16", start, end)
	}
}

func TestWorkflowRunsSelectedOrchestratorAsReviewerBeforeCuration(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	env.Driver.stdout = `{
		"summary": "orchestrator reviewed code",
		"findings": [
			{
				"claim": "Repository settings update misses an admin guard",
				"category": "security",
				"severity": "high",
				"confidence": 0.91,
				"locations": [{"path":"src/new.go","start_line":3,"end_line":3,"side":"RIGHT"}],
				"evidence": [{"title":"route reaches mutation","summary":"The orchestrator review pass found that the changed route reaches the settings mutation without an admin guard.","kind":"changed_code"}],
				"suggested_fix": "Mount requireWorkspaceAdmin before the mutation."
			}
		]
	}`
	createOrchestratorCLIConfig(t, env, "agent_config_orchestrator_review")
	session := createWorkflowSession(t, env, "review_session_orchestrator_reviews_then_curates", StatusDraft)
	if _, err := env.Queries.CreateReviewSessionAgent(context.Background(), dbgen.CreateReviewSessionAgentParams{
		ID:                   "review_session_agent_orchestrator_review",
		ReviewSessionID:      session.ID,
		AgentConfigID:        "agent_config_orchestrator_review",
		Role:                 "orchestrator",
		RunOrder:             0,
		Enabled:              1,
		SettingsOverrideJson: "{}",
	}); err != nil {
		t.Fatalf("CreateReviewSessionAgent(orchestrator) error = %v", err)
	}
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusQueued); err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	if err := env.Service.Run(context.Background(), session.ID); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	runs, err := env.Queries.ListAgentRunsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	if len(runs) < 2 {
		t.Fatalf("agent runs = %+v", runs)
	}
	phases := map[string]bool{}
	orchestratorReviewRunID := ""
	for _, run := range runs {
		if run.AgentConfigID != "agent_config_orchestrator_review" {
			continue
		}
		var metadata map[string]any
		if err := json.Unmarshal([]byte(run.MetadataJson), &metadata); err != nil {
			t.Fatalf("decode run metadata: %v", err)
		}
		if phase, ok := metadata["phase"].(string); ok {
			phases[phase] = true
			if phase == PhaseRunAgents {
				orchestratorReviewRunID = run.ID
			}
		}
	}
	if !phases[PhaseRunAgents] || !phases[PhaseDeduplicate] || !phases[PhaseVerifyFindings] {
		t.Fatalf("orchestrator should review and then orchestrate, phases = %+v runs = %+v", phases, runs)
	}
	candidates, err := env.Queries.ListFindingCandidatesBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListFindingCandidatesBySession() error = %v", err)
	}
	hasOrchestratorCandidate := false
	for _, candidate := range candidates {
		if candidate.AgentRunID == orchestratorReviewRunID {
			hasOrchestratorCandidate = true
			break
		}
	}
	if !hasOrchestratorCandidate {
		t.Fatalf("orchestrator review candidates = %+v", candidates)
	}
}

func TestSanitizeCommandArgsRepairsStaleClaudeToolsFlag(t *testing.T) {
	args := agents.SanitizeCommandArgs("claude", []string{
		"-p",
		agents.PromptArgPlaceholder,
		"--output-format",
		"json",
		"--tools",
		"",
		"--permission-mode",
		"plan",
	})
	want := []string{
		"-p",
		agents.PromptArgPlaceholder,
		"--output-format",
		"json",
		"--permission-mode",
		"plan",
	}
	if !slices.Equal(args, want) {
		t.Fatalf("SanitizeCommandArgs() = %#v, want %#v", args, want)
	}
}

func TestSanitizeCommandArgsKeepsConfiguredClaudeTools(t *testing.T) {
	args := agents.SanitizeCommandArgs("claude", []string{
		"--tools",
		"Read,Glob,Grep",
		"-p",
		agents.PromptArgPlaceholder,
	})
	want := []string{
		"--tools",
		"Read,Glob,Grep",
		"-p",
		agents.PromptArgPlaceholder,
	}
	if !slices.Equal(args, want) {
		t.Fatalf("SanitizeCommandArgs() = %#v, want %#v", args, want)
	}
}

func TestWorkflowPersistsStructuredFindingCandidates(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	env.Driver.stdout = `{
		"summary": "one finding",
		"findings": [
			{
				"claim": "Settings mutation lacks admin guard",
				"category": "security",
				"severity": "high",
				"confidence": 0.91,
				"locations": [{"path":"src/new.go","start_line":3,"end_line":3,"side":"RIGHT"}],
				"evidence": [{"title":"handler is reachable","summary":"the changed function can be called without an admin guard","kind":"changed_code"}],
				"suggested_fix": "Require admin before mutation.",
				"draft_comment": "Please require admin permission before mutating settings."
			}
		]
	}`
	session := createWorkflowSession(t, env, "review_session_candidates", StatusDraft)
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusQueued); err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	if err := env.Service.Run(context.Background(), session.ID); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	candidates, err := env.Queries.ListFindingCandidatesBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListFindingCandidatesBySession() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %+v", candidates)
	}
	candidate := candidates[0]
	if candidate.Claim != "Settings mutation lacks admin guard" ||
		candidate.Category != "security" ||
		candidate.Severity != "high" ||
		candidate.Confidence != 0.91 ||
		candidate.PrimaryPath.String != "src/new.go" ||
		candidate.PrimaryStartLine.Int64 != 3 ||
		!candidate.RawArtifactID.Valid ||
		!candidate.Fingerprint.Valid ||
		!strings.Contains(candidate.EvidenceJson, `"kind":"changed_code"`) {
		t.Fatalf("candidate = %+v", candidate)
	}

	events, err := env.Events.ListByReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	assertEventTypes(t, events, []string{
		"FindingCandidateCreated",
		"FindingNormalized",
		"ReviewSessionCompleted",
	})
	candidateCreated := eventPayloadByType(t, events, "FindingCandidateCreated")
	if candidateCreated["source_trust"] != "untrusted_agent_output" ||
		candidateCreated["candidate_trust"] != "unverified_agent_claim" ||
		candidateCreated["side_effects_allowed"] != false {
		t.Fatalf("candidate event payload = %+v", candidateCreated)
	}
	normalized := eventPayloadByType(t, events, "FindingNormalized")
	if normalized["source_trust"] != "untrusted_agent_output" ||
		normalized["side_effects_allowed"] != false {
		t.Fatalf("normalized event payload = %+v", normalized)
	}
	runs, err := env.Queries.ListAgentRunsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	if len(runs) != 1 || !runs[0].ParsedOutputArtifactID.Valid {
		t.Fatalf("agent runs = %+v", runs)
	}
	parsedArtifact, err := env.Queries.GetArtifact(context.Background(), runs[0].ParsedOutputArtifactID.String)
	if err != nil {
		t.Fatalf("GetArtifact(parsed output) error = %v", err)
	}
	var parsedMetadata map[string]any
	if err := json.Unmarshal([]byte(parsedArtifact.MetadataJson), &parsedMetadata); err != nil {
		t.Fatalf("decode parsed metadata: %v", err)
	}
	if parsedMetadata["source_trust"] != "untrusted_agent_output" ||
		parsedMetadata["side_effects_allowed"] != false ||
		parsedMetadata["requires_human_decision"] != true {
		t.Fatalf("parsed artifact metadata = %+v", parsedMetadata)
	}

	summary, err := env.Service.Summary(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.FindingCounts.Candidates != 1 || summary.FindingCounts.Findings != 1 {
		t.Fatalf("finding summary = %+v", summary.FindingCounts)
	}
}

func TestWorkflowDeduplicatesCandidatesIntoCanonicalFinding(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	env.Driver.stdout = `{
		"summary": "duplicate findings",
		"findings": [
			{
				"claim": "Settings mutation lacks admin guard",
				"category": "security",
				"severity": "high",
				"confidence": 0.91,
				"locations": [{"path":"src/new.go","start_line":2,"end_line":3,"side":"RIGHT"}],
				"evidence": [{"title":"route is unguarded","summary":"the mutation is reachable without admin authorization"}],
				"suggested_fix": "Require admin before mutation.",
				"draft_comment": "Please require admin permission before mutating settings."
			},
			{
				"claim": "settings mutation lacks admin guard",
				"category": "security",
				"severity": "medium",
				"confidence": 0.74,
				"locations": [{"path":"b/src/new.go","start_line":2,"end_line":3,"side":"RIGHT"}],
				"evidence": [{"title":"same route","summary":"another agent reported the same missing guard"}]
			}
		]
	}`
	session := createWorkflowSession(t, env, "review_session_dedupe", StatusDraft)
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusQueued); err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	if err := env.Service.Run(context.Background(), session.ID); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	candidates, err := env.Queries.ListFindingCandidatesBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListFindingCandidatesBySession() error = %v", err)
	}
	if len(candidates) != 2 || candidates[0].Fingerprint.String != candidates[1].Fingerprint.String {
		t.Fatalf("candidates = %+v", candidates)
	}
	findings, err := env.Queries.ListFindingsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListFindingsBySession() error = %v", err)
	}
	if len(findings) != 1 || findings[0].MergedFromCount != 2 || findings[0].Severity != "high" {
		t.Fatalf("findings = %+v", findings)
	}
	links, err := env.Queries.ListFindingCandidateLinks(context.Background(), findings[0].ID)
	if err != nil {
		t.Fatalf("ListFindingCandidateLinks() error = %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("links = %+v", links)
	}
	events, err := env.Events.ListByReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	assertEventTypes(t, events, []string{"FindingMerged", "FindingDeduplicated"})
}

func TestWorkflowUsesOptionalDedupeHook(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	env.Driver.stdout = `{
		"summary": "dedupe refinement",
		"findings": [
			{
				"claim": "Settings mutation lacks admin guard",
				"category": "security",
				"severity": "high",
				"confidence": 0.91,
				"locations": [{"path":"src/new.go","start_line":2,"end_line":3,"side":"RIGHT"}],
				"evidence": [{"title":"route is unguarded","summary":"the mutation is reachable without admin authorization"}]
			},
			{
				"claim": "Settings update skips audit actor attribution",
				"category": "reliability",
				"severity": "medium",
				"confidence": 0.77,
				"locations": [{"path":"src/new.go","start_line":3,"end_line":3,"side":"RIGHT"}],
				"evidence": [{"title":"audit is incomplete","summary":"the update does not persist the actor"}]
			}
		]
	}`
	calls := 0
	env.Service.EnableDedupeHook = true
	env.Service.DedupeHook = fakeDedupeHook{refine: func(_ context.Context, input findingengine.DedupeInput) (findingengine.DedupeResult, error) {
		calls++
		if input.ReviewSessionID != "review_session_dedupe_hook" || len(input.Candidates) != 2 || len(input.DeterministicClusters) != 2 {
			t.Fatalf("dedupe input = %+v", input)
		}
		return findingengine.DedupeResult{Clusters: []findingengine.Cluster{{
			Candidates: append([]dbgen.FindingCandidate{}, input.Candidates...),
		}}}, nil
	}}

	session := createWorkflowSession(t, env, "review_session_dedupe_hook", StatusDraft)
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusQueued); err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	if err := env.Service.Run(context.Background(), session.ID); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("dedupe hook calls = %d, want 1", calls)
	}
	findings, err := env.Queries.ListFindingsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListFindingsBySession() error = %v", err)
	}
	if len(findings) != 1 || findings[0].MergedFromCount != 2 {
		t.Fatalf("findings = %+v", findings)
	}
	events, err := env.Events.ListByReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	assertEventTypes(t, events, []string{"FindingDedupeRefined", "FindingMerged", "FindingDeduplicated"})
}

func TestWorkflowUsesSelectedOrchestratorForDedupeCuration(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	createOrchestratorCLIConfig(t, env, "agent_config_dedupe_curator")
	session := createWorkflowSession(t, env, "review_session_orchestrator_dedupe", StatusDraft)
	if _, err := env.Queries.CreateReviewSessionAgent(context.Background(), dbgen.CreateReviewSessionAgentParams{
		ID:                   "review_session_agent_dedupe_curator",
		ReviewSessionID:      session.ID,
		AgentConfigID:        "agent_config_dedupe_curator",
		Role:                 "orchestrator",
		RunOrder:             0,
		Enabled:              1,
		SettingsOverrideJson: "{}",
	}); err != nil {
		t.Fatalf("CreateReviewSessionAgent(orchestrator) error = %v", err)
	}
	createWorkflowCandidate(t, env, session.ID, "candidate_nil_a", "pickTokenPrice dereferences prices[1] without a nil check, causing a runtime panic", "correctness", "high", 0.85, "internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go", 207, 208, "fp_nil_a")
	createWorkflowCandidate(t, env, session.ID, "candidate_nil_b", "Missing nil check for prices[1] in pickTokenPrice can panic when the second price is absent", "correctness", "high", 0.82, "internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go", 203, 208, "fp_nil_b")
	createWorkflowCandidate(t, env, session.ID, "candidate_order", "Reward amounts are returned in nondeterministic map iteration order", "correctness", "medium", 0.77, "internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go", 218, 234, "fp_order")
	env.Driver.stdout = `{
		"clusters": [
			{
				"candidate_ids": ["candidate_nil_a", "candidate_nil_b"],
				"canonical_claim": "pickTokenPrice dereferences prices[1] without a nil check when only one price is present",
				"category": "correctness",
				"severity": "high",
				"confidence": 0.88,
				"verification_status": "verified",
				"primary_location": {"path":"internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go","start_line":207,"end_line":208,"side":"RIGHT"},
				"evidence_summary": "The average branch checks prices[0] but dereferences prices[1] without proving it is non-nil.",
				"counter_evidence_summary": "No direct contradiction was verified. Related guards and tests are checks, not refutations.",
				"supporting_evidence": [
					{"title":"Nil check misses prices[1]","summary":"Line 207 only checks prices[0], then line 208 dereferences prices[1].","path":"internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go","start_line":207,"end_line":208,"confidence":0.9}
				],
				"related_context": [
					{"kind":"counter","title":"Nearby guard mention is only a lead","summary":"This guard mention needs comparison and does not refute the prices[1] dereference.","path":"internal/app/api_route.go","start_line":101,"end_line":103,"refutes_claim":false,"confidence":0.4}
				],
				"relationship_evidence": [
					{"title":"fetchRewardTokenInfo reaches pickTokenPrice","summary":"gopls call_hierarchy shows fetchRewardTokenInfo calls pickTokenPrice while enriching KEM rewards, so wallet reward data can trigger this branch.","path":"internal/app/aggregatedposition/fetcher/kyberdata/fetcher.go","start_line":345,"end_line":345,"relationship":"caller","confidence":0.78}
				],
				"suggested_fix": "Check prices[1] for nil before averaging.",
				"dedupe_reason": "Both candidates describe the same nil-safety failure in pickTokenPrice."
			},
			{
				"candidate_ids": ["candidate_order"],
				"canonical_claim": "Reward amount output order depends on map iteration",
				"category": "correctness",
				"severity": "medium",
				"confidence": 0.77,
				"verification_status": "plausible",
				"primary_location": {"path":"internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go","start_line":218,"end_line":234,"side":"RIGHT"},
				"evidence_summary": "The loop iterates a map before appending results.",
				"counter_evidence_summary": "No direct contradiction was verified.",
				"supporting_evidence": [
					{"title":"Map iteration appends results","summary":"The changed loop appends output while ranging over a map.","path":"internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go","start_line":218,"end_line":234,"confidence":0.75}
				],
				"dedupe_reason": "Distinct ordering issue."
			}
		]
	}`

	if err := env.Service.deduplicateFindings(context.Background(), session); err != nil {
		t.Fatalf("deduplicateFindings() error = %v", err)
	}

	findings, err := env.Queries.ListFindingsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListFindingsBySession() error = %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %+v", findings)
	}
	var nilFinding dbgen.Finding
	for _, finding := range findings {
		if strings.Contains(finding.CanonicalClaim, "prices[1]") {
			nilFinding = finding
			break
		}
	}
	if nilFinding.ID == "" ||
		nilFinding.MergedFromCount != 2 ||
		nilFinding.VerificationStatus != evidence.StatusVerified ||
		nilFinding.PrimaryStartLine.Int64 != 207 ||
		!strings.Contains(nullableTestValue(nilFinding.EvidenceSummary), "average branch") ||
		!strings.Contains(nullableTestValue(nilFinding.CounterEvidenceSummary), "No direct contradiction") {
		t.Fatalf("nil finding = %+v", nilFinding)
	}
	items, err := env.Queries.ListEvidenceItemsByFinding(context.Background(), nilFinding.ID)
	if err != nil {
		t.Fatalf("ListEvidenceItemsByFinding() error = %v", err)
	}
	if countWorkflowEvidenceKind(items, evidence.KindSupporting) != 1 ||
		countWorkflowEvidenceKind(items, evidence.KindStaticAnalysis) != 1 ||
		countWorkflowEvidenceKind(items, evidence.KindSearch) != 1 ||
		countWorkflowEvidenceKind(items, evidence.KindCounter) != 0 {
		t.Fatalf("curated evidence items = %+v", items)
	}
	if prompt := env.Driver.lastPrompt(); !strings.Contains(prompt, "orchestrator-curator") ||
		!strings.Contains(prompt, "Every input candidate id must appear exactly once") ||
		!strings.Contains(prompt, "gopls call_hierarchy") {
		t.Fatalf("curator prompt missing contract:\n%s", prompt)
	}
	events, err := env.Events.ListByReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	assertEventTypes(t, events, []string{
		"ContextBundleCreated",
		"AgentOutputParsed",
		"FindingDedupeRefined",
		"FindingMerged",
		"FindingDeduplicated",
	})
	refined := eventPayloadByType(t, events, "FindingDedupeRefined")
	if refined["refiner"] != "orchestrator" || refined["curated_findings"].(float64) != 2 {
		t.Fatalf("refined event = %+v", refined)
	}
}

func TestParseFindingCuratorOutputReadsWrappedTextJSON(t *testing.T) {
	t.Parallel()

	candidates := []dbgen.FindingCandidate{{
		ID:               "candidate_wrapped",
		Category:         "correctness",
		Severity:         "high",
		Confidence:       0.8,
		Claim:            "Wrapped curation candidate",
		PrimaryPath:      nullableTestString("src/server.go"),
		PrimaryStartLine: sql.NullInt64{Int64: 10, Valid: true},
		PrimaryEndLine:   sql.NullInt64{Int64: 10, Valid: true},
		Fingerprint:      nullableTestString("wrapped-fingerprint"),
	}}
	raw := []byte(`{"type":"thread.started","thread_id":"thread_1"}
{"type":"item.completed","item":{"id":"item_30","type":"agent_message","text":"{\"clusters\":[{\"candidate_ids\":[\"candidate_wrapped\"],\"canonical_claim\":\"Curated wrapped claim\",\"verification_status\":\"verified\",\"primary_location\":{\"path\":\"src/server.go\",\"start_line\":10,\"end_line\":10},\"supporting_evidence\":[{\"title\":\"Issue line\",\"summary\":\"Exact line supports the claim\",\"path\":\"src/server.go\",\"start_line\":10,\"end_line\":10}]}]}"}}
{"type":"turn.completed"}`)

	result, err := parseFindingCuratorOutput(raw, candidates)
	if err != nil {
		t.Fatalf("parseFindingCuratorOutput() error = %v", err)
	}
	if len(result.Clusters) != 1 || len(result.Curated) != 1 {
		t.Fatalf("result = %+v", result)
	}
	curated := result.Curated[clusterKey(result.Clusters[0])]
	if curated.CanonicalClaim != "Curated wrapped claim" ||
		curated.VerificationStatus != evidence.StatusVerified ||
		len(curated.Evidence) != 1 {
		t.Fatalf("curated = %+v", curated)
	}
}

func TestWorkflowRejectsInvalidDedupeHookOutput(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	env.Driver.stdout = `{
		"summary": "invalid dedupe refinement",
		"findings": [
			{
				"claim": "Settings mutation lacks admin guard",
				"category": "security",
				"severity": "high",
				"confidence": 0.91,
				"locations": [{"path":"src/new.go","start_line":2,"end_line":3,"side":"RIGHT"}]
			},
			{
				"claim": "Settings update skips audit actor attribution",
				"category": "reliability",
				"severity": "medium",
				"confidence": 0.77,
				"locations": [{"path":"src/new.go","start_line":3,"end_line":3,"side":"RIGHT"}]
			}
		]
	}`
	env.Service.EnableDedupeHook = true
	env.Service.DedupeHook = fakeDedupeHook{refine: func(_ context.Context, input findingengine.DedupeInput) (findingengine.DedupeResult, error) {
		return findingengine.DedupeResult{Clusters: input.DeterministicClusters[:1]}, nil
	}}

	session := createWorkflowSession(t, env, "review_session_dedupe_hook_invalid", StatusDraft)
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusQueued); err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	err := env.Service.Run(context.Background(), session.ID)
	if !errors.Is(err, findingengine.ErrInvalidDedupeResult) {
		t.Fatalf("Run() error = %v, want ErrInvalidDedupeResult", err)
	}
}

func TestWorkflowPersistsDelimitedFindingCandidateEvents(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	config, err := env.Queries.GetAgentConfig(context.Background(), "agent_config_1")
	if err != nil {
		t.Fatalf("GetAgentConfig() error = %v", err)
	}
	if _, err := env.Queries.UpdateAgentConfig(context.Background(), dbgen.UpdateAgentConfigParams{
		ID:               config.ID,
		Name:             config.Name,
		Role:             config.Role,
		Command:          config.Command,
		ArgsJson:         config.ArgsJson,
		CwdMode:          config.CwdMode,
		EnvAllowlistJson: config.EnvAllowlistJson,
		OutputMode:       string(agents.OutputNDJSON),
		ModelLabel:       config.ModelLabel,
		ReasoningLabel:   config.ReasoningLabel,
		CapabilitiesJson: config.CapabilitiesJson,
		SettingsJson:     config.SettingsJson,
		Enabled:          config.Enabled,
		UpdatedAt:        "2026-05-03T00:04:30Z",
	}); err != nil {
		t.Fatalf("UpdateAgentConfig(output mode) error = %v", err)
	}
	env.Driver.stdout = `review started
{"event":"finding","finding":{"claim":"Role cache can be stale","category":"reliability","severity":"medium","confidence":0.72,"locations":[{"path":"src/new.go","start_line":1,"end_line":3,"side":"RIGHT"}],"evidence":[{"title":"cache lacks expiry","summary":"the event cites stale role handling"}]}}
{"event":"done","count":1}
`
	session := createWorkflowSession(t, env, "review_session_ndjson_candidates", StatusDraft)
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusQueued); err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	if err := env.Service.Run(context.Background(), session.ID); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	candidates, err := env.Queries.ListFindingCandidatesBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListFindingCandidatesBySession() error = %v", err)
	}
	if len(candidates) != 1 ||
		candidates[0].Claim != "Role cache can be stale" ||
		candidates[0].Severity != "medium" {
		t.Fatalf("candidates = %+v", candidates)
	}
	events, err := env.Events.ListByReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	assertEventTypes(t, events, []string{"FindingNormalizationDiagnostics", "FindingCandidateCreated"})
}

func TestWorkflowRunsSelectedAgentsInParallel(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	env.Driver.delay = 100 * time.Millisecond
	if _, err := env.Queries.CreateAgentConfig(context.Background(), dbgen.CreateAgentConfigParams{
		ID:               "agent_config_2",
		Name:             "Fake Reviewer 2",
		Role:             "secondary_reviewer",
		AdapterKind:      string(agents.AdapterCLINonInteractive),
		Command:          nullableTestString("fake-agent"),
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       string(agents.OutputJSON),
		CapabilitiesJson: `{"supports_json":true,"can_read":true,"output_modes":["json"]}`,
		SettingsJson:     `{"prompt_delivery":"stdin","timeout_seconds":30}`,
		Enabled:          1,
		CreatedAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:        "2026-05-03T00:04:00Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig(second) error = %v", err)
	}
	session := createWorkflowSession(t, env, "review_session_parallel", StatusDraft)
	if _, err := env.Queries.CreateReviewSessionAgent(context.Background(), dbgen.CreateReviewSessionAgentParams{
		ID:                   "review_session_agent_parallel_2",
		ReviewSessionID:      session.ID,
		AgentConfigID:        "agent_config_2",
		Role:                 "secondary_reviewer",
		RunOrder:             2,
		Enabled:              1,
		SettingsOverrideJson: "{}",
	}); err != nil {
		t.Fatalf("CreateReviewSessionAgent(second) error = %v", err)
	}
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusQueued); err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	if err := env.Service.Run(context.Background(), session.ID); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if max := env.Driver.maxConcurrent(); max < 2 {
		t.Fatalf("max concurrent agent sends = %d, want at least 2", max)
	}
	runs, err := env.Queries.ListAgentRunsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("agent runs len = %d, want 2: %+v", len(runs), runs)
	}
}

func TestWorkflowContinuesWhenOneAgentFails(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	env.Driver.failConfigs = map[string]bool{"agent_config_2": true}
	if _, err := env.Queries.CreateAgentConfig(context.Background(), dbgen.CreateAgentConfigParams{
		ID:               "agent_config_2",
		Name:             "Failing Reviewer",
		Role:             "secondary_reviewer",
		AdapterKind:      string(agents.AdapterCLINonInteractive),
		Command:          nullableTestString("fake-agent"),
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       string(agents.OutputJSON),
		CapabilitiesJson: `{"supports_json":true,"can_read":true,"output_modes":["json"]}`,
		SettingsJson:     `{"prompt_delivery":"stdin","timeout_seconds":30}`,
		Enabled:          1,
		CreatedAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:        "2026-05-03T00:04:00Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig(second) error = %v", err)
	}
	session := createWorkflowSession(t, env, "review_session_partial", StatusDraft)
	if _, err := env.Queries.CreateReviewSessionAgent(context.Background(), dbgen.CreateReviewSessionAgentParams{
		ID:                   "review_session_agent_partial_2",
		ReviewSessionID:      session.ID,
		AgentConfigID:        "agent_config_2",
		Role:                 "secondary_reviewer",
		RunOrder:             2,
		Enabled:              1,
		SettingsOverrideJson: "{}",
	}); err != nil {
		t.Fatalf("CreateReviewSessionAgent(second) error = %v", err)
	}
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusQueued); err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	if err := env.Service.Run(context.Background(), session.ID); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	updated, err := env.Queries.GetReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetReviewSession() error = %v", err)
	}
	if updated.Status != StatusCompleted {
		t.Fatalf("session status = %s, want completed", updated.Status)
	}
	runs, err := env.Queries.ListAgentRunsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	statuses := map[string]bool{}
	for _, run := range runs {
		statuses[run.Status] = true
	}
	if len(runs) != 2 || !statuses[agentrun.RunStatusSucceeded] || !statuses[agentrun.RunStatusFailed] {
		t.Fatalf("agent runs = %+v", runs)
	}
	events, err := env.Events.ListByReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	assertEventTypes(t, events, []string{"AgentRunFailed", "ReviewSessionPartialFailure", "ReviewSessionCompleted"})
}

func TestWorkflowAgentTimeoutKeepsOtherFindings(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	env.Driver.timeoutConfigs = map[string]bool{"agent_config_2": true}
	env.Driver.stdout = `{
		"summary": "one finding",
		"findings": [
			{
				"claim": "Settings mutation lacks admin guard",
				"category": "security",
				"severity": "high",
				"confidence": 0.91,
				"locations": [{"path":"src/new.go","start_line":3,"end_line":3,"side":"RIGHT"}],
				"evidence": [{"title":"handler is reachable","summary":"the changed function can be called without an admin guard"}]
			}
		]
	}`
	if _, err := env.Queries.CreateAgentConfig(context.Background(), dbgen.CreateAgentConfigParams{
		ID:               "agent_config_2",
		Name:             "Slow Reviewer",
		Role:             "secondary_reviewer",
		AdapterKind:      string(agents.AdapterCLINonInteractive),
		Command:          nullableTestString("slow-fake-agent"),
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       string(agents.OutputJSON),
		CapabilitiesJson: `{"supports_json":true,"can_read":true,"output_modes":["json"]}`,
		SettingsJson:     `{"prompt_delivery":"stdin","timeout_seconds":30}`,
		Enabled:          1,
		CreatedAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:        "2026-05-03T00:04:00Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig(slow) error = %v", err)
	}
	session := createWorkflowSession(t, env, "review_session_timeout_partial", StatusDraft)
	if _, err := env.Queries.CreateReviewSessionAgent(context.Background(), dbgen.CreateReviewSessionAgentParams{
		ID:                   "review_session_agent_timeout_2",
		ReviewSessionID:      session.ID,
		AgentConfigID:        "agent_config_2",
		Role:                 "secondary_reviewer",
		RunOrder:             2,
		Enabled:              1,
		SettingsOverrideJson: "{}",
	}); err != nil {
		t.Fatalf("CreateReviewSessionAgent(slow) error = %v", err)
	}
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusQueued); err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	if err := env.Service.Run(context.Background(), session.ID); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	updated, err := env.Queries.GetReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetReviewSession() error = %v", err)
	}
	if updated.Status != StatusCompleted {
		t.Fatalf("session status = %s, want completed", updated.Status)
	}
	runs, err := env.Queries.ListAgentRunsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	statuses := map[string]dbgen.AgentRun{}
	for _, run := range runs {
		statuses[run.Status] = run
	}
	timedOut := statuses[agentrun.RunStatusTimedOut]
	if len(runs) != 2 ||
		statuses[agentrun.RunStatusSucceeded].ID == "" ||
		timedOut.ID == "" ||
		timedOut.ErrorCode.String != "timeout" {
		t.Fatalf("agent runs = %+v", runs)
	}
	candidates, err := env.Queries.ListFindingCandidatesBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListFindingCandidatesBySession() error = %v", err)
	}
	findings, err := env.Queries.ListFindingsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListFindingsBySession() error = %v", err)
	}
	if len(candidates) != 1 || len(findings) != 1 || candidates[0].Claim != "Settings mutation lacks admin guard" {
		t.Fatalf("candidates = %+v findings = %+v", candidates, findings)
	}
	summary, err := env.Service.Summary(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.AgentStatusCounts[agentrun.RunStatusSucceeded] != 1 ||
		summary.AgentStatusCounts[agentrun.RunStatusTimedOut] != 1 ||
		summary.FindingCounts.Candidates != 1 ||
		summary.FindingCounts.Findings != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	events, err := env.Events.ListByReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	assertEventTypes(t, events, []string{"AgentRunCanceled", "ReviewSessionPartialFailure", "ReviewSessionCompleted"})
	payload := eventPayloadByType(t, events, "ReviewSessionPartialFailure")
	if payload["failed_agent_runs"] != float64(1) || payload["succeeded_agent_runs"] != float64(1) {
		t.Fatalf("partial failure payload = %+v", payload)
	}
}

func TestVerifyFindingsRunsVerifierCLIWithFindingContext(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	createVerifierCLIConfig(t, env, "agent_config_verifier")
	env.Driver.stdout = `{
		"verification_status": "plausible",
		"evidence_summary": "Verifier confirmed the changed function is the primary code path.",
		"counter_evidence_summary": "Verifier found a nearby guard-like call that needs human review.",
		"evidence": [
			{
				"kind": "counter",
				"title": "Nearby guard-like call",
				"summary": "The scoped context includes RequireAdmin, so the claim is plausible rather than fully verified.",
				"path": "src/new.go",
				"start_line": 3,
				"end_line": 3,
				"confidence": 0.73
			}
		]
	}`
	session := createWorkflowSession(t, env, "review_session_verifier_cli", StatusDraft)
	finding := createWorkflowFinding(t, env, session.ID, "finding_verifier_cli")
	repository, err := env.Queries.GetRepository(context.Background(), session.RepositoryID)
	if err != nil {
		t.Fatalf("GetRepository() error = %v", err)
	}

	if err := env.Service.verifyFindings(context.Background(), session, repository); err != nil {
		t.Fatalf("verifyFindings() error = %v", err)
	}

	updated, err := env.Queries.GetFinding(context.Background(), finding.ID)
	if err != nil {
		t.Fatalf("GetFinding() error = %v", err)
	}
	if updated.VerificationStatus != evidence.StatusPlausible ||
		!updated.EvidenceSummary.Valid ||
		!strings.Contains(updated.EvidenceSummary.String, "Verifier confirmed") ||
		!updated.CounterEvidenceSummary.Valid ||
		!strings.Contains(updated.CounterEvidenceSummary.String, "verification leads rather than counter-evidence") {
		t.Fatalf("updated finding = %+v", updated)
	}
	items, err := env.Queries.ListEvidenceItemsByFinding(context.Background(), finding.ID)
	if err != nil {
		t.Fatalf("ListEvidenceItemsByFinding() error = %v", err)
	}
	localItems := 0
	verifierItems := 0
	for _, item := range items {
		if strings.Contains(item.MetadataJson, `"producer":"local_verifier"`) {
			localItems++
		}
		if strings.Contains(item.MetadataJson, `"producer":"verifier_agent"`) {
			verifierItems++
			if item.Kind != evidence.KindSearch || item.Path.String != "src/new.go" || item.StartLine.Int64 != 3 {
				t.Fatalf("verifier evidence item = %+v", item)
			}
		}
	}
	if localItems == 0 || verifierItems != 1 {
		t.Fatalf("evidence counts local=%d verifier=%d items=%+v", localItems, verifierItems, items)
	}
	runs, err := env.Queries.ListAgentRunsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	if len(runs) != 1 ||
		runs[0].Role != "verifier" ||
		runs[0].Status != agentrun.RunStatusSucceeded ||
		!runs[0].ContextBundleID.Valid ||
		!runs[0].ParsedOutputArtifactID.Valid {
		t.Fatalf("verifier runs = %+v", runs)
	}
	bundles, err := env.Queries.ListContextBundlesBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListContextBundlesBySession() error = %v", err)
	}
	if len(bundles) != 1 || bundles[0].Scope != string(contextbundle.ScopeFinding) {
		t.Fatalf("context bundles = %+v", bundles)
	}
	if prompt := env.Driver.lastPrompt(); !strings.Contains(prompt, "orchestrator-verifier") ||
		!strings.Contains(prompt, "Finding ID: finding_verifier_cli") ||
		!strings.Contains(prompt, "Context Bundle") ||
		!strings.Contains(prompt, "UNTRUSTED_FINDING_DATA") ||
		!strings.Contains(prompt, "untrusted evidence only") ||
		!strings.Contains(prompt, "gopls call_hierarchy") {
		t.Fatalf("verifier prompt missing scoped context:\n%s", prompt)
	}
	events, err := env.Events.ListByReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	assertEventTypes(t, events, []string{
		"FindingVerificationCompleted",
		"ContextBundleCreated",
		"AgentOutputParsed",
		"VerifierAgentVerificationCompleted",
	})
}

func TestVerifyFindingsUsesSelectedOrchestratorAsCurator(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	createOrchestratorCLIConfig(t, env, "agent_config_orchestrator")
	env.Driver.stdout = `{
		"verification_status": "verified",
		"evidence_summary": "Orchestrator confirmed the changed handler is reachable from the exported route.",
		"counter_evidence_summary": "No direct contradiction was found; related guards are only verification leads.",
		"evidence": [
			{
				"kind": "static_analysis",
				"title": "Call hierarchy links route to handler",
				"summary": "gopls call hierarchy shows the exported route invokes the changed handler before authorization checks run.",
				"path": "src/new.go",
				"start_line": 3,
				"end_line": 3,
				"confidence": 0.82
			}
		]
	}`
	session := createWorkflowSession(t, env, "review_session_orchestrator_curator", StatusDraft)
	if _, err := env.Queries.CreateReviewSessionAgent(context.Background(), dbgen.CreateReviewSessionAgentParams{
		ID:                   "review_session_agent_orchestrator_curator",
		ReviewSessionID:      session.ID,
		AgentConfigID:        "agent_config_orchestrator",
		Role:                 "orchestrator",
		RunOrder:             0,
		Enabled:              1,
		SettingsOverrideJson: "{}",
	}); err != nil {
		t.Fatalf("CreateReviewSessionAgent(orchestrator) error = %v", err)
	}
	finding := createWorkflowFinding(t, env, session.ID, "finding_orchestrator_curator")
	repository, err := env.Queries.GetRepository(context.Background(), session.RepositoryID)
	if err != nil {
		t.Fatalf("GetRepository() error = %v", err)
	}

	if err := env.Service.verifyFindings(context.Background(), session, repository); err != nil {
		t.Fatalf("verifyFindings() error = %v", err)
	}

	updated, err := env.Queries.GetFinding(context.Background(), finding.ID)
	if err != nil {
		t.Fatalf("GetFinding() error = %v", err)
	}
	if updated.VerificationStatus != evidence.StatusVerified ||
		!strings.Contains(nullableTestValue(updated.EvidenceSummary), "Orchestrator confirmed") {
		t.Fatalf("updated finding = %+v", updated)
	}
	runs, err := env.Queries.ListAgentRunsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	if len(runs) != 1 || runs[0].AgentConfigID != "agent_config_orchestrator" || runs[0].Role != "verifier" {
		t.Fatalf("runs = %+v", runs)
	}
	items, err := env.Queries.ListEvidenceItemsByFinding(context.Background(), finding.ID)
	if err != nil {
		t.Fatalf("ListEvidenceItemsByFinding() error = %v", err)
	}
	if countWorkflowEvidenceKind(items, evidence.KindStaticAnalysis) != 1 {
		t.Fatalf("expected static analysis evidence from orchestrator, got %+v", items)
	}
}

func TestVerifyFindingsPreservesCuratedEvidenceStory(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	createVerifierCLIConfig(t, env, "agent_config_verifier_preserve_curated")
	env.Driver.stdout = `{
		"verification_status": "verified",
		"evidence_summary": "Verifier generic overwrite should not replace curated story.",
		"counter_evidence_summary": "Verifier generic counter overwrite should not replace curated contradiction summary.",
		"evidence": [
			{
				"kind": "supporting",
				"title": "Verifier extra source window",
				"summary": "Verifier confirmed the same changed line but did not add a contradiction.",
				"path": "src/new.go",
				"start_line": 3,
				"end_line": 3,
				"confidence": 0.7
			}
		]
	}`
	session := createWorkflowSession(t, env, "review_session_preserve_curated_story", StatusDraft)
	finding := createWorkflowFinding(t, env, session.ID, "finding_preserve_curated_story")
	if _, err := env.Queries.UpdateFindingVerificationEvidence(context.Background(), dbgen.UpdateFindingVerificationEvidenceParams{
		VerificationStatus:     evidence.StatusPlausible,
		EvidenceSummary:        nullableTestString("Curated support story: line 3 reaches the changed handler through the exported route."),
		CounterEvidenceSummary: nullableTestString("Curated contradiction story: no direct contradiction was verified; related guards remain verification leads."),
		UpdatedAt:              "2026-05-03T00:10:00Z",
		ID:                     finding.ID,
	}); err != nil {
		t.Fatalf("UpdateFindingVerificationEvidence() error = %v", err)
	}
	if _, err := env.Queries.CreateEvidenceItem(context.Background(), dbgen.CreateEvidenceItemParams{
		ID:           "evidence_curated_preserve_story",
		FindingID:    finding.ID,
		Kind:         evidence.KindStaticAnalysis,
		Title:        "Curated route-to-handler relationship",
		Summary:      "The orchestrator verified that the exported route reaches the changed handler and that no cited guard directly refutes the finding.",
		Path:         nullableTestString("src/new.go"),
		StartLine:    sql.NullInt64{Int64: 3, Valid: true},
		EndLine:      sql.NullInt64{Int64: 3, Valid: true},
		Confidence:   0.83,
		MetadataJson: `{"producer":"orchestrator_curator","relationship":"caller","source":"dedupe_curation"}`,
		CreatedAt:    "2026-05-03T00:10:01Z",
	}); err != nil {
		t.Fatalf("CreateEvidenceItem(curated) error = %v", err)
	}
	repository, err := env.Queries.GetRepository(context.Background(), session.RepositoryID)
	if err != nil {
		t.Fatalf("GetRepository() error = %v", err)
	}

	if err := env.Service.verifyFindings(context.Background(), session, repository); err != nil {
		t.Fatalf("verifyFindings() error = %v", err)
	}

	updated, err := env.Queries.GetFinding(context.Background(), finding.ID)
	if err != nil {
		t.Fatalf("GetFinding() error = %v", err)
	}
	if updated.VerificationStatus != evidence.StatusPlausible ||
		!strings.Contains(nullableTestValue(updated.EvidenceSummary), "Curated support story") ||
		!strings.Contains(nullableTestValue(updated.CounterEvidenceSummary), "Curated contradiction story") {
		t.Fatalf("curated story was overwritten: %+v", updated)
	}
	items, err := env.Queries.ListEvidenceItemsByFinding(context.Background(), finding.ID)
	if err != nil {
		t.Fatalf("ListEvidenceItemsByFinding() error = %v", err)
	}
	curatedItems := 0
	verifierItems := 0
	localItems := 0
	for _, item := range items {
		if strings.Contains(item.MetadataJson, `"producer":"orchestrator_curator"`) {
			curatedItems++
		}
		if strings.Contains(item.MetadataJson, `"producer":"verifier_agent"`) {
			verifierItems++
		}
		if strings.Contains(item.MetadataJson, `"producer":"local_verifier"`) {
			localItems++
		}
	}
	if curatedItems != 1 || verifierItems != 1 || localItems == 0 {
		t.Fatalf("evidence producer counts curated=%d verifier=%d local=%d items=%+v", curatedItems, verifierItems, localItems, items)
	}
}

func TestVerifierCounterEvidenceKindRequiresDirectContradiction(t *testing.T) {
	t.Parallel()

	weak := verifierAgentEvidence{
		Kind:    "counter",
		Title:   "Nearby guard-like call",
		Summary: "RequireAdmin appears in the scoped context, but this needs comparison and does not refute the claim.",
		Path:    "src/auth.go",
	}
	if got := normalizeVerifierEvidenceKind(weak); got != evidence.KindSearch {
		t.Fatalf("weak counter kind = %s, want %s", got, evidence.KindSearch)
	}

	testLead := verifierAgentEvidence{
		Kind:    "counter",
		Title:   "Related test mentions admin",
		Summary: "The test mentions admin but does not refute the changed route claim.",
		Path:    "test/server.test.js",
	}
	if got := normalizeVerifierEvidenceKind(testLead); got != evidence.KindTest {
		t.Fatalf("test lead kind = %s, want %s", got, evidence.KindTest)
	}

	strong := verifierAgentEvidence{
		Kind:    "counter",
		Title:   "Admin guard already enforced before handler",
		Summary: "The router denies members before this handler, so this directly refutes the claim.",
		Path:    "src/router.go",
	}
	if got := normalizeVerifierEvidenceKind(strong); got != evidence.KindCounter {
		t.Fatalf("strong counter kind = %s, want %s", got, evidence.KindCounter)
	}
}

func TestVerifyFindingsKeepsLocalEvidenceWhenVerifierCLIFails(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	createVerifierCLIConfig(t, env, "agent_config_verifier_fail")
	env.Driver.failConfigs = map[string]bool{"agent_config_verifier_fail": true}
	session := createWorkflowSession(t, env, "review_session_verifier_fail", StatusDraft)
	finding := createWorkflowFinding(t, env, session.ID, "finding_verifier_fail")
	repository, err := env.Queries.GetRepository(context.Background(), session.RepositoryID)
	if err != nil {
		t.Fatalf("GetRepository() error = %v", err)
	}

	if err := env.Service.verifyFindings(context.Background(), session, repository); err != nil {
		t.Fatalf("verifyFindings() error = %v", err)
	}

	updated, err := env.Queries.GetFinding(context.Background(), finding.ID)
	if err != nil {
		t.Fatalf("GetFinding() error = %v", err)
	}
	if updated.VerificationStatus != evidence.StatusLocallySupported ||
		!updated.EvidenceSummary.Valid ||
		!strings.Contains(updated.EvidenceSummary.String, "anchored to changed code") {
		t.Fatalf("updated finding = %+v", updated)
	}
	items, err := env.Queries.ListEvidenceItemsByFinding(context.Background(), finding.ID)
	if err != nil {
		t.Fatalf("ListEvidenceItemsByFinding() error = %v", err)
	}
	localItems := 0
	verifierItems := 0
	for _, item := range items {
		if strings.Contains(item.MetadataJson, `"producer":"local_verifier"`) {
			localItems++
		}
		if strings.Contains(item.MetadataJson, `"producer":"verifier_agent"`) {
			verifierItems++
		}
	}
	if localItems == 0 || verifierItems != 0 {
		t.Fatalf("evidence counts local=%d verifier=%d items=%+v", localItems, verifierItems, items)
	}
	runs, err := env.Queries.ListAgentRunsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	if len(runs) != 1 || runs[0].Status != agentrun.RunStatusFailed {
		t.Fatalf("verifier runs = %+v", runs)
	}
	events, err := env.Events.ListByReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	assertEventTypes(t, events, []string{"AgentRunFailed", "VerifierAgentVerificationCompleted"})
}

func TestCheckpointLoadsPersistedPartialPhase(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	session := createWorkflowSession(t, env, "review_session_checkpoint", StatusDraft)
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusQueued); err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusRunning); err != nil {
		t.Fatalf("Transition(queued -> running) error = %v", err)
	}
	if err := env.Service.appendEvent(context.Background(), appendEventParams{
		ReviewSessionID: session.ID,
		Type:            "WorkflowPhaseStarted",
		Payload:         map[string]any{"phase": PhaseRunAgents},
	}); err != nil {
		t.Fatalf("append phase event: %v", err)
	}

	checkpoint, err := env.Service.LoadCheckpoint(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("LoadCheckpoint() error = %v", err)
	}
	if checkpoint.Status != StatusRunning ||
		checkpoint.Phase != PhaseRunAgents ||
		checkpoint.PhaseStatus != "running" ||
		checkpoint.LastSequence != 1 {
		t.Fatalf("checkpoint = %+v", checkpoint)
	}
}

func TestReconcileLocalSessionsPausesInterruptedSessionAndCancelsStaleRun(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	session := createWorkflowSession(t, env, "review_session_reconcile", StatusDraft)
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusQueued); err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusRunning); err != nil {
		t.Fatalf("Transition(queued -> running) error = %v", err)
	}
	if _, err := env.Queries.CreateAgentRun(context.Background(), dbgen.CreateAgentRunParams{
		ID:              "agent_run_interrupted",
		ReviewSessionID: session.ID,
		AgentConfigID:   "agent_config_1",
		Status:          agentrun.RunStatusRunning,
		Role:            "primary_reviewer",
		StartedAt:       nullableTestString("2026-05-03T00:08:00Z"),
		MetadataJson:    `{"phase":"run_review_agents"}`,
	}); err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	result, err := env.Service.ReconcileLocalSessions(context.Background())
	if err != nil {
		t.Fatalf("ReconcileLocalSessions() error = %v", err)
	}
	if result.SessionsPaused != 1 || result.AgentRunsCanceled != 1 {
		t.Fatalf("reconcile result = %+v", result)
	}
	updated, err := env.Queries.GetReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetReviewSession() error = %v", err)
	}
	if updated.Status != StatusPaused || updated.CompletedAt.Valid {
		t.Fatalf("updated session = %+v", updated)
	}
	run, err := env.Queries.GetAgentRun(context.Background(), "agent_run_interrupted")
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if run.Status != agentrun.RunStatusCanceled ||
		!run.CompletedAt.Valid ||
		run.ErrorCode.String != "app_restarted" {
		t.Fatalf("interrupted run = %+v", run)
	}
	summary, err := env.Service.Summary(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.Status != StatusPaused || summary.ActiveAgents != 0 || summary.AgentStatusCounts[agentrun.RunStatusCanceled] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	events, err := env.Events.ListByReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	assertEventTypes(t, events, []string{"AgentRunCanceled", "ReviewSessionReconciled"})
}

func TestReconcileLocalSessionsCompletesPendingCancellation(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	session := createWorkflowSession(t, env, "review_session_reconcile_canceling", StatusCanceling)
	result, err := env.Service.ReconcileLocalSessions(context.Background())
	if err != nil {
		t.Fatalf("ReconcileLocalSessions() error = %v", err)
	}
	if result.SessionsCanceled != 1 {
		t.Fatalf("reconcile result = %+v", result)
	}
	updated, err := env.Queries.GetReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetReviewSession() error = %v", err)
	}
	if updated.Status != StatusCanceled || !updated.CompletedAt.Valid {
		t.Fatalf("updated session = %+v", updated)
	}
	events, err := env.Events.ListByReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListByReviewSession() error = %v", err)
	}
	assertEventTypes(t, events, []string{"ReviewSessionCanceled"})
}

func TestResumeRestartsFromCompletedBuildContext(t *testing.T) {
	t.Parallel()

	env := setupWorkflowEnv(t)
	session := createWorkflowSession(t, env, "review_session_resume_completed_build", StatusDraft)
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusQueued); err != nil {
		t.Fatalf("Transition(draft -> queued) error = %v", err)
	}
	if _, err := env.Service.Transition(context.Background(), session.ID, StatusRunning); err != nil {
		t.Fatalf("Transition(queued -> running) error = %v", err)
	}
	if err := env.Service.withPhase(context.Background(), session.ID, PhaseBuildContext, func() error {
		built, err := env.Service.ContextBuilder.BuildReviewContext(context.Background(), contextbundle.BuildReviewContextParams{
			ReviewSessionID: session.ID,
			AgentConfigID:   "agent_config_1",
			Persist:         true,
		})
		if err != nil {
			return err
		}
		return env.Service.appendEvent(context.Background(), appendEventParams{
			ReviewSessionID: session.ID,
			Type:            "ContextBundleCreated",
			ArtifactID:      nullableEventString(built.Bundle.ArtifactID),
			Payload: map[string]any{
				"phase":             PhaseBuildContext,
				"agent_config_id":   "agent_config_1",
				"context_bundle_id": built.Bundle.ID,
			},
		})
	}); err != nil {
		t.Fatalf("persist completed build context: %v", err)
	}

	if _, err := env.Service.ReconcileLocalSessions(context.Background()); err != nil {
		t.Fatalf("ReconcileLocalSessions() error = %v", err)
	}
	paused, err := env.Queries.GetReviewSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetReviewSession(paused) error = %v", err)
	}
	if paused.Status != StatusPaused {
		t.Fatalf("paused session = %+v", paused)
	}

	if _, err := env.Service.Resume(context.Background(), session.ID); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	completed := waitForWorkflowSessionStatus(t, env.Queries, session.ID, StatusCompleted)
	if !completed.CompletedAt.Valid {
		t.Fatalf("completed session missing completed_at: %+v", completed)
	}
	bundles, err := env.Queries.ListContextBundlesBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListContextBundlesBySession() error = %v", err)
	}
	if len(bundles) != 1 {
		t.Fatalf("resume rebuilt context bundles: %+v", bundles)
	}
	runs, err := env.Queries.ListAgentRunsBySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	if len(runs) != 1 || runs[0].Status != agentrun.RunStatusSucceeded {
		t.Fatalf("agent runs = %+v", runs)
	}
	checkpoint, err := env.Service.LoadCheckpoint(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("LoadCheckpoint() error = %v", err)
	}
	if checkpoint.Status != StatusCompleted || !phaseCompleted(checkpoint.CompletedPhases, PhaseBuildContext) {
		t.Fatalf("checkpoint = %+v", checkpoint)
	}
}

type workflowEnv struct {
	Database  *sql.DB
	Queries   *dbgen.Queries
	Artifacts *artifact.Store
	Events    *eventlog.Store
	Driver    *workflowDriver
	Service   *Service
	RepoPath  string
}

func setupWorkflowEnv(t *testing.T) workflowEnv {
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
	if err := os.MkdirAll(filepath.Join(repoPath, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "src", "new.go"), []byte("package src\n\nfunc RequireAdmin() bool { return true }\n"), 0o644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}
	createWorkflowBaseRows(t, queries, repoPath)
	artifactStore, err := artifact.New(filepath.Join(t.TempDir(), "artifacts"), queries)
	if err != nil {
		t.Fatalf("artifact.New() error = %v", err)
	}
	events, err := eventlog.New(database)
	if err != nil {
		t.Fatalf("eventlog.New() error = %v", err)
	}
	driver := &workflowDriver{}
	service := &Service{
		Queries:        queries,
		ContextBuilder: &contextbundle.Service{Queries: queries, Artifacts: artifactStore},
		Artifacts:      artifactStore,
		Events:         events,
		Evidence:       &evidence.Service{Queries: queries, Searcher: workflowEvidenceSearcher{}},
		AgentManager: &agentrun.Manager{
			Runner: agentrun.Runner{
				Queries:   queries,
				Artifacts: artifactStore,
				Driver:    driver,
				Now: func() time.Time {
					return time.Date(2026, 5, 3, 0, 0, 1, 0, time.UTC)
				},
			},
			MaxConcurrent:           2,
			MaxConcurrentPerSession: 2,
		},
		Now: func() time.Time {
			return time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
		},
	}
	return workflowEnv{
		Database:  database,
		Queries:   queries,
		Artifacts: artifactStore,
		Events:    events,
		Driver:    driver,
		Service:   service,
		RepoPath:  repoPath,
	}
}

func waitForWorkflowSessionStatus(t *testing.T, queries *dbgen.Queries, id string, status string) dbgen.ReviewSession {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		session, err := queries.GetReviewSession(context.Background(), id)
		if err != nil {
			t.Fatalf("GetReviewSession() error = %v", err)
		}
		if session.Status == status {
			return session
		}
		time.Sleep(20 * time.Millisecond)
	}
	session, err := queries.GetReviewSession(context.Background(), id)
	if err != nil {
		t.Fatalf("GetReviewSession(final) error = %v", err)
	}
	t.Fatalf("review session %s status = %s, want %s", id, session.Status, status)
	return dbgen.ReviewSession{}
}

func createWorkflowBaseRows(t *testing.T, queries *dbgen.Queries, repoPath string) {
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
	if _, err := queries.CreateRepository(context.Background(), dbgen.CreateRepositoryParams{
		ID:          "repo_1",
		WorkspaceID: "workspace_1",
		Name:        "cocode",
		LocalPath:   repoPath,
		CreatedAt:   "2026-05-03T00:01:00Z",
		UpdatedAt:   "2026-05-03T00:01:00Z",
	}); err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
	if _, err := queries.CreatePullRequestSnapshot(context.Background(), dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_1",
		RepositoryID: "repo_1",
		SourceType:   "branch_compare",
		BaseRef:      nullableTestString("main"),
		HeadRef:      nullableTestString("feature"),
		MetadataJson: "{}",
		CreatedAt:    "2026-05-03T00:02:00Z",
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}
	if _, err := queries.CreateChangedFile(context.Background(), dbgen.CreateChangedFileParams{
		ID:             "changed_file_1",
		SnapshotID:     "snapshot_1",
		Path:           "src/new.go",
		Status:         "modified",
		Additions:      3,
		LineRangesJson: `[[1,3]]`,
		CreatedAt:      "2026-05-03T00:03:00Z",
	}); err != nil {
		t.Fatalf("CreateChangedFile() error = %v", err)
	}
	if _, err := queries.CreateAgentConfig(context.Background(), dbgen.CreateAgentConfigParams{
		ID:               "agent_config_1",
		Name:             "Fake Reviewer",
		Role:             "primary_reviewer",
		AdapterKind:      string(agents.AdapterCLINonInteractive),
		Command:          nullableTestString("fake-agent"),
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       string(agents.OutputJSON),
		CapabilitiesJson: `{"supports_json":true,"can_read":true,"output_modes":["json"]}`,
		SettingsJson:     `{"prompt_delivery":"stdin","timeout_seconds":30}`,
		Enabled:          1,
		CreatedAt:        "2026-05-03T00:04:00Z",
		UpdatedAt:        "2026-05-03T00:04:00Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig() error = %v", err)
	}
}

func createWorkflowSession(t *testing.T, env workflowEnv, id string, status string) dbgen.ReviewSession {
	t.Helper()

	session, err := env.Queries.CreateReviewSession(context.Background(), dbgen.CreateReviewSessionParams{
		ID:                  id,
		WorkspaceID:         "workspace_1",
		RepositoryID:        "repo_1",
		SnapshotID:          "snapshot_1",
		Title:               "Review fixture",
		Status:              status,
		ReviewDepth:         string(contextbundle.ReviewDepthStandard),
		RuntimeLimitSeconds: 300,
		ContextPolicyJson: `{
			"include_prompt_material": true,
			"include_changed_code": true,
			"include_related_call_sites": false,
			"include_related_tests": false,
			"include_project_conventions": false,
			"include_prior_comments": false,
			"include_prior_decisions": false,
			"redact_secrets": true,
			"max_tokens": 4096,
			"max_items": 20
		}`,
		CreatedAt: "2026-05-03T00:05:00Z",
		UpdatedAt: "2026-05-03T00:05:00Z",
	})
	if err != nil {
		t.Fatalf("CreateReviewSession() error = %v", err)
	}
	if _, err := env.Queries.CreateReviewSessionAgent(context.Background(), dbgen.CreateReviewSessionAgentParams{
		ID:                   "review_session_agent_" + id,
		ReviewSessionID:      id,
		AgentConfigID:        "agent_config_1",
		Role:                 "primary_reviewer",
		RunOrder:             1,
		Enabled:              1,
		SettingsOverrideJson: "{}",
	}); err != nil {
		t.Fatalf("CreateReviewSessionAgent() error = %v", err)
	}
	return session
}

func createWorkflowFinding(t *testing.T, env workflowEnv, sessionID string, id string) dbgen.Finding {
	t.Helper()

	finding, err := env.Queries.CreateFinding(context.Background(), dbgen.CreateFindingParams{
		ID:                 id,
		ReviewSessionID:    sessionID,
		CanonicalClaim:     "Settings mutation lacks admin guard",
		Category:           "security",
		Severity:           "high",
		Confidence:         0.91,
		VerificationStatus: evidence.StatusUnverified,
		DecisionStatus:     "open",
		PrimaryPath:        nullableTestString("src/new.go"),
		PrimaryStartLine:   sql.NullInt64{Int64: 3, Valid: true},
		PrimaryEndLine:     sql.NullInt64{Int64: 3, Valid: true},
		Fingerprint:        id + "_fingerprint",
		FirstSeenAt:        "2026-05-03T00:09:00Z",
		UpdatedAt:          "2026-05-03T00:09:00Z",
	})
	if err != nil {
		t.Fatalf("CreateFinding() error = %v", err)
	}
	return finding
}

func createWorkflowCandidate(t *testing.T, env workflowEnv, sessionID string, id string, claim string, category string, severity string, confidence float64, path string, startLine int64, endLine int64, fingerprint string) dbgen.FindingCandidate {
	t.Helper()

	locations, err := json.Marshal([]map[string]any{{
		"path":       path,
		"start_line": startLine,
		"end_line":   endLine,
		"side":       "RIGHT",
	}})
	if err != nil {
		t.Fatalf("marshal locations: %v", err)
	}
	evidenceJSON, err := json.Marshal([]map[string]any{{
		"title":      "Agent evidence",
		"summary":    claim,
		"kind":       "supporting",
		"path":       path,
		"start_line": startLine,
		"end_line":   endLine,
	}})
	if err != nil {
		t.Fatalf("marshal evidence: %v", err)
	}
	runID := "agent_run_" + id
	if _, err := env.Queries.CreateAgentRun(context.Background(), dbgen.CreateAgentRunParams{
		ID:              runID,
		ReviewSessionID: sessionID,
		AgentConfigID:   "agent_config_1",
		Status:          agentrun.RunStatusSucceeded,
		Role:            "primary_reviewer",
		StartedAt:       nullableTestString("2026-05-03T00:08:00Z"),
		CompletedAt:     nullableTestString("2026-05-03T00:08:30Z"),
		MetadataJson:    "{}",
	}); err != nil {
		t.Fatalf("CreateAgentRun(%s) error = %v", runID, err)
	}
	candidate, err := env.Queries.CreateFindingCandidate(context.Background(), dbgen.CreateFindingCandidateParams{
		ID:               id,
		ReviewSessionID:  sessionID,
		AgentRunID:       runID,
		Category:         category,
		Severity:         severity,
		Confidence:       confidence,
		Claim:            claim,
		PrimaryPath:      nullableTestString(path),
		PrimaryStartLine: sql.NullInt64{Int64: startLine, Valid: true},
		PrimaryEndLine:   sql.NullInt64{Int64: endLine, Valid: true},
		LocationsJson:    string(locations),
		EvidenceJson:     string(evidenceJSON),
		Fingerprint:      nullableTestString(fingerprint),
		CreatedAt:        "2026-05-03T00:09:00Z",
	})
	if err != nil {
		t.Fatalf("CreateFindingCandidate(%s) error = %v", id, err)
	}
	return candidate
}

func createVerifierCLIConfig(t *testing.T, env workflowEnv, id string) {
	t.Helper()

	if _, err := env.Queries.CreateAgentConfig(context.Background(), dbgen.CreateAgentConfigParams{
		ID:               id,
		Name:             "Fake Verifier",
		Role:             "verifier",
		AdapterKind:      string(agents.AdapterCLINonInteractive),
		Command:          nullableTestString("fake-verifier"),
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       string(agents.OutputJSON),
		CapabilitiesJson: `{"supports_json":true,"can_read":true,"output_modes":["json"]}`,
		SettingsJson:     `{"prompt_delivery":"stdin","timeout_seconds":30}`,
		Enabled:          1,
		CreatedAt:        "2026-05-03T00:04:30Z",
		UpdatedAt:        "2026-05-03T00:04:30Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig(verifier) error = %v", err)
	}
}

func createOrchestratorCLIConfig(t *testing.T, env workflowEnv, id string) {
	t.Helper()

	if _, err := env.Queries.CreateAgentConfig(context.Background(), dbgen.CreateAgentConfigParams{
		ID:               id,
		Name:             "Fake Orchestrator",
		Role:             "orchestrator",
		AdapterKind:      string(agents.AdapterCLINonInteractive),
		Command:          nullableTestString("fake-orchestrator"),
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       string(agents.OutputJSON),
		CapabilitiesJson: `{"supports_json":true,"can_read":true,"output_modes":["json"]}`,
		SettingsJson:     `{"prompt_delivery":"stdin","timeout_seconds":30}`,
		Enabled:          1,
		CreatedAt:        "2026-05-03T00:04:30Z",
		UpdatedAt:        "2026-05-03T00:04:30Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig(orchestrator) error = %v", err)
	}
}

func countWorkflowEvidenceKind(items []dbgen.EvidenceItem, kind string) int {
	count := 0
	for _, item := range items {
		if item.Kind == kind {
			count++
		}
	}
	return count
}

func assertEventTypes(t *testing.T, events []dbgen.Event, want []string) {
	t.Helper()

	seen := map[string]bool{}
	for _, event := range events {
		seen[event.Type] = true
	}
	for _, typ := range want {
		if !seen[typ] {
			t.Fatalf("events missing %s; got %+v", typ, events)
		}
	}
}

func eventPayloadByType(t *testing.T, events []dbgen.Event, typ string) map[string]any {
	t.Helper()

	for _, event := range events {
		if event.Type != typ {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.PayloadJson), &payload); err != nil {
			t.Fatalf("decode payload for %s: %v", typ, err)
		}
		return payload
	}
	t.Fatalf("events missing %s; got %+v", typ, events)
	return nil
}

type workflowDriver struct {
	mu             sync.Mutex
	prompts        []string
	delay          time.Duration
	stdout         string
	current        int
	max            int
	failConfigs    map[string]bool
	timeoutConfigs map[string]bool
}

func (d *workflowDriver) Open(context.Context, agents.ConnectionConfig) (agents.Connection, error) {
	return workflowConnection{driver: d}, nil
}

type workflowConnection struct {
	driver *workflowDriver
}

type workflowEvidenceSearcher struct{}

func (workflowEvidenceSearcher) Search(context.Context, evidence.SearchOptions) ([]evidence.SearchMatch, error) {
	return nil, nil
}

type fakeDedupeHook struct {
	refine func(context.Context, findingengine.DedupeInput) (findingengine.DedupeResult, error)
}

func (h fakeDedupeHook) RefineDedupe(ctx context.Context, input findingengine.DedupeInput) (findingengine.DedupeResult, error) {
	return h.refine(ctx, input)
}

func (c workflowConnection) SendTask(_ context.Context, task agents.AgentTask) (<-chan agents.AgentEvent, error) {
	c.driver.enter(task.Prompt)
	if c.driver.delay > 0 {
		time.Sleep(c.driver.delay)
	}
	c.driver.leave()
	if c.driver.shouldFail(task.AgentConfigID) {
		exitCode := 7
		events := make(chan agents.AgentEvent, 3)
		events <- agents.AgentEvent{Type: agents.EventStarted, RunID: task.RunID, Message: "fake agent started"}
		events <- agents.AgentEvent{Type: agents.EventOutput, RunID: task.RunID, Stream: "stderr", Text: "agent failed\n"}
		events <- agents.AgentEvent{Type: agents.EventFailed, RunID: task.RunID, ExitCode: &exitCode, ErrorCode: "failed", Error: "agent failed"}
		close(events)
		return events, nil
	}
	if c.driver.shouldTimeout(task.AgentConfigID) {
		events := make(chan agents.AgentEvent, 3)
		events <- agents.AgentEvent{Type: agents.EventStarted, RunID: task.RunID, Message: "fake agent started"}
		events <- agents.AgentEvent{Type: agents.EventOutput, RunID: task.RunID, Stream: "stdout", Text: "partial before timeout\n"}
		events <- agents.AgentEvent{Type: agents.EventCanceled, RunID: task.RunID, ErrorCode: "timeout", Error: "agent exceeded timeout"}
		close(events)
		return events, nil
	}
	exitCode := 0
	events := make(chan agents.AgentEvent, 3)
	events <- agents.AgentEvent{Type: agents.EventStarted, RunID: task.RunID, Message: "fake agent started"}
	events <- agents.AgentEvent{Type: agents.EventOutput, RunID: task.RunID, Stream: "stdout", Text: c.driver.stdoutText()}
	events <- agents.AgentEvent{Type: agents.EventCompleted, RunID: task.RunID, ExitCode: &exitCode, Message: "fake agent completed"}
	close(events)
	return events, nil
}

func (workflowConnection) Close(context.Context) error {
	return nil
}

func (d *workflowDriver) enter(prompt string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.prompts = append(d.prompts, prompt)
	d.current++
	if d.current > d.max {
		d.max = d.current
	}
}

func (d *workflowDriver) leave() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.current--
}

func (d *workflowDriver) lastPrompt() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.prompts) == 0 {
		return ""
	}
	return d.prompts[len(d.prompts)-1]
}

func (d *workflowDriver) maxConcurrent() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.max
}

func (d *workflowDriver) shouldFail(agentConfigID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.failConfigs[agentConfigID]
}

func (d *workflowDriver) shouldTimeout(agentConfigID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.timeoutConfigs[agentConfigID]
}

func (d *workflowDriver) stdoutText() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stdout != "" {
		return d.stdout
	}
	return `{"summary":"ok","findings":[]}`
}

func nullableTestString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

func nullableTestValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
