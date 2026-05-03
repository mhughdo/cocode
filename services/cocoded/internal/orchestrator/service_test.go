package orchestrator

import (
	"context"
	"database/sql"
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
		"AgentRunCompleted",
		"AgentOutputParsed",
		"ReviewSessionCompleted",
	})
	if prompt := env.Driver.lastPrompt(); !strings.Contains(prompt, "Context Bundle") ||
		!strings.Contains(prompt, "src/new.go") {
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

	restarted := *env.Service
	checkpoint, err := restarted.LoadCheckpoint(context.Background(), session.ID)
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

type workflowDriver struct {
	mu          sync.Mutex
	prompts     []string
	delay       time.Duration
	current     int
	max         int
	failConfigs map[string]bool
}

func (d *workflowDriver) Open(context.Context, agents.ConnectionConfig) (agents.Connection, error) {
	return workflowConnection{driver: d}, nil
}

type workflowConnection struct {
	driver *workflowDriver
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
	exitCode := 0
	events := make(chan agents.AgentEvent, 3)
	events <- agents.AgentEvent{Type: agents.EventStarted, RunID: task.RunID, Message: "fake agent started"}
	events <- agents.AgentEvent{Type: agents.EventOutput, RunID: task.RunID, Stream: "stdout", Text: `{"summary":"ok","findings":[]}`}
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

func nullableTestString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}
