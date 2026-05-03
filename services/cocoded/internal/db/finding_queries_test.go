package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestFindingQueriesCRUD(t *testing.T) {
	t.Parallel()

	queries := seededReviewQueries(t)
	createReviewSessionForTest(t, queries, "review_session_1", "Review cocode")
	createAgentRunForFindingTest(t, queries)

	candidate, err := queries.CreateFindingCandidate(context.Background(), dbgen.CreateFindingCandidateParams{
		ID:               "candidate_1",
		ReviewSessionID:  "review_session_1",
		AgentRunID:       "agent_run_1",
		Category:         "correctness",
		Severity:         "high",
		Confidence:       0.82,
		Claim:            "Session status can be updated",
		PrimaryPath:      nullableString("services/cocoded/internal/db/review_agent_queries_test.go"),
		PrimaryStartLine: nullableInt64(10),
		PrimaryEndLine:   nullableInt64(20),
		LocationsJson:    `[{"path":"services/cocoded/internal/db/review_agent_queries_test.go"}]`,
		EvidenceJson:     `[{"kind":"supporting","summary":"test covers status updates"}]`,
		SuggestedFix:     nullableString("keep status updates typed"),
		DraftComment:     nullableString("Please keep the lifecycle path typed."),
		Fingerprint:      nullableString("finding-fingerprint-1"),
		CreatedAt:        "2026-05-03T00:09:00Z",
	})
	if err != nil {
		t.Fatalf("CreateFindingCandidate() error = %v", err)
	}
	if candidate.AgentRunID != "agent_run_1" || candidate.Fingerprint.String != "finding-fingerprint-1" {
		t.Fatalf("CreateFindingCandidate() = %+v", candidate)
	}

	gotCandidate, err := queries.GetFindingCandidate(context.Background(), "candidate_1")
	if err != nil {
		t.Fatalf("GetFindingCandidate() error = %v", err)
	}
	if gotCandidate.ID != candidate.ID {
		t.Fatalf("GetFindingCandidate() ID = %q, want %q", gotCandidate.ID, candidate.ID)
	}

	candidates, err := queries.ListFindingCandidatesBySession(context.Background(), "review_session_1")
	if err != nil {
		t.Fatalf("ListFindingCandidatesBySession() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != "candidate_1" {
		t.Fatalf("ListFindingCandidatesBySession() = %+v", candidates)
	}

	finding, err := queries.CreateFinding(context.Background(), dbgen.CreateFindingParams{
		ID:                     "finding_1",
		ReviewSessionID:        "review_session_1",
		CanonicalClaim:         "Session status can be updated",
		Category:               "correctness",
		Severity:               "high",
		Confidence:             0.82,
		VerificationStatus:     "unverified",
		DecisionStatus:         "undecided",
		PrimaryPath:            nullableString("services/cocoded/internal/db/review_agent_queries_test.go"),
		PrimaryStartLine:       nullableInt64(10),
		PrimaryEndLine:         nullableInt64(20),
		EvidenceSummary:        nullableString("status updates are persisted"),
		CounterEvidenceSummary: nullableString("no counter evidence found"),
		SuggestedFix:           nullableString("keep typed lifecycle storage"),
		DraftComment:           nullableString("The review status lifecycle is now persisted."),
		Fingerprint:            "finding-fingerprint-1",
		MergedFromCount:        1,
		IntroducedInSha:        nullableString("head-sha"),
		FirstSeenAt:            "2026-05-03T00:10:00Z",
		UpdatedAt:              "2026-05-03T00:10:00Z",
	})
	if err != nil {
		t.Fatalf("CreateFinding() error = %v", err)
	}
	if finding.VerificationStatus != "unverified" || finding.DecisionStatus != "undecided" {
		t.Fatalf("CreateFinding() = %+v", finding)
	}

	if err := queries.LinkFindingCandidate(context.Background(), dbgen.LinkFindingCandidateParams{
		FindingID:          "finding_1",
		FindingCandidateID: "candidate_1",
		Relation:           "merged",
	}); err != nil {
		t.Fatalf("LinkFindingCandidate() error = %v", err)
	}

	links, err := queries.ListFindingCandidateLinks(context.Background(), "finding_1")
	if err != nil {
		t.Fatalf("ListFindingCandidateLinks() error = %v", err)
	}
	if len(links) != 1 || links[0].FindingCandidateID != "candidate_1" {
		t.Fatalf("ListFindingCandidateLinks() = %+v", links)
	}

	updated, err := queries.UpdateFinding(context.Background(), dbgen.UpdateFindingParams{
		ID:                     "finding_1",
		CanonicalClaim:         "Session status updates persist",
		Category:               "correctness",
		Severity:               "medium",
		Confidence:             0.9,
		PrimaryPath:            nullableString("services/cocoded/internal/db/review_agent_queries_test.go"),
		PrimaryStartLine:       nullableInt64(11),
		PrimaryEndLine:         nullableInt64(21),
		EvidenceSummary:        nullableString("query tests exercise the lifecycle"),
		CounterEvidenceSummary: nullableString("none"),
		SuggestedFix:           nullableString("keep tests around status updates"),
		DraftComment:           nullableString("Lifecycle storage is covered."),
		MergedFromCount:        2,
		IntroducedInSha:        nullableString("head-sha"),
		UpdatedAt:              "2026-05-03T00:11:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateFinding() error = %v", err)
	}
	if updated.CanonicalClaim != "Session status updates persist" || updated.Severity != "medium" || updated.MergedFromCount != 2 {
		t.Fatalf("UpdateFinding() = %+v", updated)
	}

	verified, err := queries.UpdateFindingVerificationStatus(context.Background(), dbgen.UpdateFindingVerificationStatusParams{
		ID:                 "finding_1",
		VerificationStatus: "verified",
		UpdatedAt:          "2026-05-03T00:12:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateFindingVerificationStatus() error = %v", err)
	}
	if verified.VerificationStatus != "verified" {
		t.Fatalf("UpdateFindingVerificationStatus() = %+v", verified)
	}

	accepted, err := queries.UpdateFindingDecisionStatus(context.Background(), dbgen.UpdateFindingDecisionStatusParams{
		ID:             "finding_1",
		DecisionStatus: "accepted",
		UpdatedAt:      "2026-05-03T00:13:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateFindingDecisionStatus() error = %v", err)
	}
	if accepted.DecisionStatus != "accepted" {
		t.Fatalf("UpdateFindingDecisionStatus() = %+v", accepted)
	}

	decision, err := queries.CreateHumanDecision(context.Background(), dbgen.CreateHumanDecisionParams{
		ID:              "decision_1",
		FindingID:       "finding_1",
		ReviewSessionID: "review_session_1",
		Decision:        "accepted",
		Reason:          nullableString("valid storage coverage"),
		MetadataJson:    `{"source":"test"}`,
		CreatedAt:       "2026-05-03T00:14:00Z",
	})
	if err != nil {
		t.Fatalf("CreateHumanDecision() error = %v", err)
	}
	if decision.Decision != "accepted" {
		t.Fatalf("CreateHumanDecision() = %+v", decision)
	}

	decisionsByFinding, err := queries.ListHumanDecisionsByFinding(context.Background(), "finding_1")
	if err != nil {
		t.Fatalf("ListHumanDecisionsByFinding() error = %v", err)
	}
	if len(decisionsByFinding) != 1 || decisionsByFinding[0].ID != "decision_1" {
		t.Fatalf("ListHumanDecisionsByFinding() = %+v", decisionsByFinding)
	}

	decisionsBySession, err := queries.ListHumanDecisionsBySession(context.Background(), "review_session_1")
	if err != nil {
		t.Fatalf("ListHumanDecisionsBySession() error = %v", err)
	}
	if len(decisionsBySession) != 1 || decisionsBySession[0].FindingID != "finding_1" {
		t.Fatalf("ListHumanDecisionsBySession() = %+v", decisionsBySession)
	}

	findings, err := queries.ListFindingsBySession(context.Background(), "review_session_1")
	if err != nil {
		t.Fatalf("ListFindingsBySession() error = %v", err)
	}
	if len(findings) != 1 || findings[0].ID != "finding_1" {
		t.Fatalf("ListFindingsBySession() = %+v", findings)
	}

	if err := queries.DeleteFinding(context.Background(), "finding_1"); err != nil {
		t.Fatalf("DeleteFinding() error = %v", err)
	}
	if _, err := queries.GetFinding(context.Background(), "finding_1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetFinding(deleted) error = %v, want sql.ErrNoRows", err)
	}
	if _, err := queries.GetFindingCandidate(context.Background(), "candidate_1"); err != nil {
		t.Fatalf("GetFindingCandidate(after finding delete) error = %v", err)
	}

	if err := queries.DeleteFindingCandidate(context.Background(), "candidate_1"); err != nil {
		t.Fatalf("DeleteFindingCandidate() error = %v", err)
	}
	if _, err := queries.GetFindingCandidate(context.Background(), "candidate_1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetFindingCandidate(deleted) error = %v, want sql.ErrNoRows", err)
	}
}

func createAgentRunForFindingTest(t *testing.T, queries *dbgen.Queries) {
	t.Helper()

	if _, err := queries.CreateAgentConfig(context.Background(), dbgen.CreateAgentConfigParams{
		ID:               "agent_config_1",
		Name:             "Codex reviewer",
		Role:             "reviewer",
		AdapterKind:      "cli_noninteractive",
		Command:          nullableString("codex"),
		ArgsJson:         `["exec"]`,
		CwdMode:          "repo_root",
		EnvAllowlistJson: `["OPENAI_API_KEY"]`,
		OutputMode:       "json",
		ModelLabel:       nullableString("gpt-5.5"),
		ReasoningLabel:   nullableString("high"),
		CapabilitiesJson: "{}",
		SettingsJson:     "{}",
		Enabled:          1,
		CreatedAt:        "2026-05-03T00:06:00Z",
		UpdatedAt:        "2026-05-03T00:06:00Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig() error = %v", err)
	}

	if _, err := queries.CreateAgentRun(context.Background(), dbgen.CreateAgentRunParams{
		ID:              "agent_run_1",
		ReviewSessionID: "review_session_1",
		AgentConfigID:   "agent_config_1",
		Status:          "succeeded",
		Role:            "reviewer",
		StartedAt:       nullableString("2026-05-03T00:07:00Z"),
		CompletedAt:     nullableString("2026-05-03T00:08:00Z"),
		DurationMs:      nullableInt64(60000),
		ExitCode:        nullableInt64(0),
		MetadataJson:    "{}",
	}); err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
}
