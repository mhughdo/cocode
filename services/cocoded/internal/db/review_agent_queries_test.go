package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

func TestReviewSessionQueriesCRUD(t *testing.T) {
	t.Parallel()

	queries := seededReviewQueries(t)
	session := createReviewSessionForTest(t, queries, "review_session_1", "Review cocode")
	if session.Status != "draft" || session.ReviewDepth != "standard" {
		t.Fatalf("CreateReviewSession() = %+v", session)
	}

	got, err := queries.GetReviewSession(context.Background(), "review_session_1")
	if err != nil {
		t.Fatalf("GetReviewSession() error = %v", err)
	}
	if got.ID != session.ID {
		t.Fatalf("GetReviewSession() ID = %q, want %q", got.ID, session.ID)
	}

	_ = createReviewSessionForTest(t, queries, "review_session_2", "Later review")
	sessions, err := queries.ListReviewSessionsByWorkspace(context.Background(), "workspace_1")
	if err != nil {
		t.Fatalf("ListReviewSessionsByWorkspace() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("ListReviewSessionsByWorkspace() len = %d, want 2", len(sessions))
	}
	if sessions[0].ID != "review_session_2" || sessions[1].ID != "review_session_1" {
		t.Fatalf("ListReviewSessionsByWorkspace() order = [%s, %s]", sessions[0].ID, sessions[1].ID)
	}

	updated, err := queries.UpdateReviewSession(context.Background(), dbgen.UpdateReviewSessionParams{
		ID:                  "review_session_1",
		Title:               "Review cocode deeply",
		ReviewDepth:         "deep",
		FocusPrompt:         nullableString("focus on backend storage"),
		Preset:              nullableString("backend"),
		RuntimeLimitSeconds: 2400,
		ContextPolicyJson:   `{"include_tests":true}`,
		UpdatedAt:           "2026-05-03T00:04:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateReviewSession() error = %v", err)
	}
	if updated.Title != "Review cocode deeply" || updated.ReviewDepth != "deep" || updated.RuntimeLimitSeconds != 2400 {
		t.Fatalf("UpdateReviewSession() = %+v", updated)
	}

	running, err := queries.UpdateReviewSessionStatus(context.Background(), dbgen.UpdateReviewSessionStatusParams{
		ID:          "review_session_1",
		Status:      "running",
		StartedAt:   nullableString("2026-05-03T00:05:00Z"),
		CompletedAt: sql.NullString{},
		UpdatedAt:   "2026-05-03T00:05:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateReviewSessionStatus(running) error = %v", err)
	}
	if running.Status != "running" || !running.StartedAt.Valid {
		t.Fatalf("UpdateReviewSessionStatus(running) = %+v", running)
	}

	completed, err := queries.UpdateReviewSessionStatus(context.Background(), dbgen.UpdateReviewSessionStatusParams{
		ID:          "review_session_1",
		Status:      "completed",
		StartedAt:   nullableString("2026-05-03T00:05:00Z"),
		CompletedAt: nullableString("2026-05-03T00:10:00Z"),
		UpdatedAt:   "2026-05-03T00:10:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateReviewSessionStatus(completed) error = %v", err)
	}
	if completed.Status != "completed" || !completed.CompletedAt.Valid {
		t.Fatalf("UpdateReviewSessionStatus(completed) = %+v", completed)
	}

	if err := queries.DeleteReviewSession(context.Background(), "review_session_1"); err != nil {
		t.Fatalf("DeleteReviewSession() error = %v", err)
	}
	if _, err := queries.GetReviewSession(context.Background(), "review_session_1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetReviewSession(deleted) error = %v, want sql.ErrNoRows", err)
	}
}

func TestAgentQueriesLifecycle(t *testing.T) {
	t.Parallel()

	queries := seededReviewQueries(t)
	createReviewSessionForTest(t, queries, "review_session_1", "Review cocode")

	agent, err := queries.CreateAgentConfig(context.Background(), dbgen.CreateAgentConfigParams{
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
		CapabilitiesJson: `{"supports_json":true,"can_read":true,"can_write":false,"can_cancel":true,"output_modes":["json","text"]}`,
		SettingsJson:     "{}",
		Enabled:          1,
		CreatedAt:        "2026-05-03T00:05:00Z",
		UpdatedAt:        "2026-05-03T00:05:00Z",
	})
	if err != nil {
		t.Fatalf("CreateAgentConfig() error = %v", err)
	}
	if agent.AdapterKind != "cli_noninteractive" || agent.Enabled != 1 {
		t.Fatalf("CreateAgentConfig() = %+v", agent)
	}
	capabilities, err := agents.DecodeCapabilitiesJSON(agent.CapabilitiesJson, agents.AdapterKind(agent.AdapterKind))
	if err != nil {
		t.Fatalf("DecodeCapabilitiesJSON(created) error = %v", err)
	}
	if !capabilities.SupportsJSON || !capabilities.CanRead || capabilities.CanWrite || !capabilities.SupportsOutputMode(agents.OutputJSON) {
		t.Fatalf("created capabilities = %+v", capabilities)
	}

	if _, err := queries.CreateAgentConfig(context.Background(), dbgen.CreateAgentConfigParams{
		ID:               "agent_config_2",
		Name:             "Disabled verifier",
		Role:             "verifier",
		AdapterKind:      "local_verifier",
		ArgsJson:         "[]",
		CwdMode:          "repo_root",
		EnvAllowlistJson: "[]",
		OutputMode:       "text",
		CapabilitiesJson: "{}",
		SettingsJson:     "{}",
		Enabled:          0,
		CreatedAt:        "2026-05-03T00:06:00Z",
		UpdatedAt:        "2026-05-03T00:06:00Z",
	}); err != nil {
		t.Fatalf("CreateAgentConfig(disabled) error = %v", err)
	}

	agentConfigs, err := queries.ListAgentConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListAgentConfigs() error = %v", err)
	}
	if len(agentConfigs) != 2 {
		t.Fatalf("ListAgentConfigs() len = %d, want 2", len(agentConfigs))
	}
	if agentConfigs[0].ID != "agent_config_1" || agentConfigs[1].ID != "agent_config_2" {
		t.Fatalf("ListAgentConfigs() order = [%s, %s]", agentConfigs[0].ID, agentConfigs[1].ID)
	}

	updatedAgent, err := queries.UpdateAgentConfig(context.Background(), dbgen.UpdateAgentConfigParams{
		ID:               "agent_config_1",
		Name:             "Codex reviewer deep",
		Role:             "reviewer",
		Command:          nullableString("codex"),
		ArgsJson:         `["exec","--json"]`,
		CwdMode:          "repo_root",
		EnvAllowlistJson: `["OPENAI_API_KEY"]`,
		OutputMode:       "json",
		ModelLabel:       nullableString("gpt-5.5"),
		ReasoningLabel:   nullableString("xhigh"),
		CapabilitiesJson: `{"supports_json":true,"supports_streaming":true,"can_read":true,"can_write":false,"can_cancel":true,"output_modes":["jsonl","json"]}`,
		SettingsJson:     `{"timeout_seconds":600}`,
		Enabled:          1,
		UpdatedAt:        "2026-05-03T00:07:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateAgentConfig() error = %v", err)
	}
	if updatedAgent.Name != "Codex reviewer deep" || updatedAgent.ReasoningLabel.String != "xhigh" {
		t.Fatalf("UpdateAgentConfig() = %+v", updatedAgent)
	}
	updatedCapabilities, err := agents.DecodeCapabilitiesJSON(updatedAgent.CapabilitiesJson, agents.AdapterKind(updatedAgent.AdapterKind))
	if err != nil {
		t.Fatalf("DecodeCapabilitiesJSON(updated) error = %v", err)
	}
	if !updatedCapabilities.SupportsStreaming || !updatedCapabilities.SupportsOutputMode(agents.OutputJSONL) {
		t.Fatalf("updated capabilities = %+v", updatedCapabilities)
	}

	if err := queries.DeleteAgentConfig(context.Background(), "agent_config_2"); err != nil {
		t.Fatalf("DeleteAgentConfig(unlinked) error = %v", err)
	}
	if _, err := queries.GetAgentConfig(context.Background(), "agent_config_2"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetAgentConfig(deleted) error = %v, want sql.ErrNoRows", err)
	}

	sessionAgent, err := queries.CreateReviewSessionAgent(context.Background(), dbgen.CreateReviewSessionAgentParams{
		ID:                   "review_session_agent_1",
		ReviewSessionID:      "review_session_1",
		AgentConfigID:        "agent_config_1",
		Role:                 "reviewer",
		RunOrder:             10,
		Enabled:              1,
		SettingsOverrideJson: `{"focus":"storage"}`,
	})
	if err != nil {
		t.Fatalf("CreateReviewSessionAgent() error = %v", err)
	}
	if sessionAgent.ReviewSessionID != "review_session_1" {
		t.Fatalf("CreateReviewSessionAgent() = %+v", sessionAgent)
	}

	sessionAgents, err := queries.ListReviewSessionAgents(context.Background(), "review_session_1")
	if err != nil {
		t.Fatalf("ListReviewSessionAgents() error = %v", err)
	}
	if len(sessionAgents) != 1 || sessionAgents[0].ID != "review_session_agent_1" {
		t.Fatalf("ListReviewSessionAgents() = %+v", sessionAgents)
	}

	disabledSessionAgent, err := queries.UpdateReviewSessionAgentEnabled(context.Background(), dbgen.UpdateReviewSessionAgentEnabledParams{
		ID:      "review_session_agent_1",
		Enabled: 0,
	})
	if err != nil {
		t.Fatalf("UpdateReviewSessionAgentEnabled() error = %v", err)
	}
	if disabledSessionAgent.Enabled != 0 {
		t.Fatalf("UpdateReviewSessionAgentEnabled() Enabled = %d, want 0", disabledSessionAgent.Enabled)
	}

	agentRun, err := queries.CreateAgentRun(context.Background(), dbgen.CreateAgentRunParams{
		ID:              "agent_run_1",
		ReviewSessionID: "review_session_1",
		AgentConfigID:   "agent_config_1",
		Status:          "queued",
		Role:            "reviewer",
		MetadataJson:    "{}",
	})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if agentRun.Status != "queued" {
		t.Fatalf("CreateAgentRun() = %+v", agentRun)
	}

	completedRun, err := queries.UpdateAgentRunStatus(context.Background(), dbgen.UpdateAgentRunStatusParams{
		ID:           "agent_run_1",
		Status:       "succeeded",
		StartedAt:    nullableString("2026-05-03T00:08:00Z"),
		CompletedAt:  nullableString("2026-05-03T00:09:00Z"),
		DurationMs:   nullableInt64(60000),
		ExitCode:     nullableInt64(0),
		MetadataJson: `{"thread_id":"thread_1"}`,
	})
	if err != nil {
		t.Fatalf("UpdateAgentRunStatus() error = %v", err)
	}
	if completedRun.Status != "succeeded" || completedRun.DurationMs.Int64 != 60000 || completedRun.ExitCode.Int64 != 0 {
		t.Fatalf("UpdateAgentRunStatus() = %+v", completedRun)
	}

	runs, err := queries.ListAgentRunsBySession(context.Background(), "review_session_1")
	if err != nil {
		t.Fatalf("ListAgentRunsBySession() error = %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "agent_run_1" {
		t.Fatalf("ListAgentRunsBySession() = %+v", runs)
	}

	if err := queries.DeleteReviewSession(context.Background(), "review_session_1"); err != nil {
		t.Fatalf("DeleteReviewSession() error = %v", err)
	}
	if _, err := queries.GetAgentRun(context.Background(), "agent_run_1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetAgentRun(deleted session child) error = %v, want sql.ErrNoRows", err)
	}
}

func seededReviewQueries(t *testing.T) *dbgen.Queries {
	t.Helper()

	database, err := Open(context.Background(), MemoryDatabase)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	if err := Apply(context.Background(), database, Migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	queries := dbgen.New(database)
	if _, err := queries.CreateWorkspace(context.Background(), dbgen.CreateWorkspaceParams{
		ID:           "workspace_1",
		Name:         "cocode",
		RootPath:     "/tmp/cocode",
		SettingsJson: "{}",
		CreatedAt:    "2026-05-03T00:00:00Z",
		UpdatedAt:    "2026-05-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, err := queries.CreateRepository(context.Background(), dbgen.CreateRepositoryParams{
		ID:            "repo_1",
		WorkspaceID:   "workspace_1",
		Name:          "cocode",
		Owner:         nullableString("hughdo"),
		RemoteUrl:     nullableString("git@github.com:hughdo/cocode.git"),
		LocalPath:     "/tmp/cocode",
		DefaultBranch: nullableString("main"),
		CreatedAt:     "2026-05-03T00:01:00Z",
		UpdatedAt:     "2026-05-03T00:01:00Z",
	}); err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}
	if _, err := queries.CreatePullRequestSnapshot(context.Background(), dbgen.CreatePullRequestSnapshotParams{
		ID:           "snapshot_1",
		RepositoryID: "repo_1",
		SourceType:   "local_changes",
		BaseRef:      nullableString("main"),
		HeadRef:      nullableString("working-tree"),
		MetadataJson: "{}",
		CreatedAt:    "2026-05-03T00:02:00Z",
	}); err != nil {
		t.Fatalf("CreatePullRequestSnapshot() error = %v", err)
	}

	return queries
}

func createReviewSessionForTest(t *testing.T, queries *dbgen.Queries, id string, title string) dbgen.ReviewSession {
	t.Helper()

	createdAt := "2026-05-03T00:03:00Z"
	if id == "review_session_2" {
		createdAt = "2026-05-03T00:03:30Z"
	}

	session, err := queries.CreateReviewSession(context.Background(), dbgen.CreateReviewSessionParams{
		ID:                  id,
		WorkspaceID:         "workspace_1",
		RepositoryID:        "repo_1",
		SnapshotID:          "snapshot_1",
		Title:               title,
		Status:              "draft",
		ReviewDepth:         "standard",
		RuntimeLimitSeconds: 1800,
		ContextPolicyJson:   "{}",
		CreatedAt:           createdAt,
		UpdatedAt:           createdAt,
	})
	if err != nil {
		t.Fatalf("CreateReviewSession(%s) error = %v", id, err)
	}
	return session
}
