package devseed

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	SeedWorkspaceID = "seed_workspace_cocode"
	seedRepoID      = "seed_repo_cocode"
	seedSnapshotID  = "seed_snapshot_pr_42"

	completedSessionID = "seed_session_pr_42"
	runningSessionID   = "seed_session_local_changes"

	agentCodexID    = "seed_agent_codex"
	agentVerifierID = "seed_agent_verifier"
	agentStaticID   = "seed_agent_static"
)

type Options struct {
	ArtifactDir   string
	WorkspaceRoot string
	Now           time.Time
}

type Result struct {
	WorkspaceID      string
	RepositoryID     string
	SnapshotID       string
	ReviewSessionIDs []string
	FindingIDs       []string
	ArtifactIDs      []string
}

type seededArtifact struct {
	ID              string
	ReviewSessionID string
	Kind            string
	RelativePath    string
	ContentType     string
	MetadataJSON    string
	Content         string
	CreatedAt       string
}

func Seed(ctx context.Context, database *sql.DB, options Options) (Result, error) {
	if database == nil {
		return Result{}, errors.New("db is required")
	}
	if options.ArtifactDir == "" {
		return Result{}, errors.New("artifact dir is required")
	}
	if options.WorkspaceRoot == "" {
		options.WorkspaceRoot = filepath.Join(os.TempDir(), "cocode-seed-workspace")
	}
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	}
	options.Now = options.Now.UTC()

	artifactDir, err := filepath.Abs(options.ArtifactDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve artifact dir: %w", err)
	}
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create artifact dir: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(artifactDir, SeedWorkspaceID)); err != nil {
		return Result{}, fmt.Errorf("reset seed artifact workspace: %w", err)
	}

	workspaceRoot, err := filepath.Abs(options.WorkspaceRoot)
	if err != nil {
		return Result{}, fmt.Errorf("resolve workspace root: %w", err)
	}
	if err := writeSeedRepositoryFiles(workspaceRoot); err != nil {
		return Result{}, err
	}

	if _, err := database.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return Result{}, fmt.Errorf("enable foreign keys: %w", err)
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, fmt.Errorf("begin seed tx: %w", err)
	}
	defer tx.Rollback()

	if err := resetSeedRows(ctx, tx); err != nil {
		return Result{}, err
	}

	result := Result{
		WorkspaceID:      SeedWorkspaceID,
		RepositoryID:     seedRepoID,
		SnapshotID:       seedSnapshotID,
		ReviewSessionIDs: []string{completedSessionID, runningSessionID},
		FindingIDs: []string{
			"seed_finding_auth_guard",
			"seed_finding_renderer_budget",
			"seed_finding_false_positive",
		},
	}

	ts := func(offset time.Duration) string {
		return options.Now.Add(offset).UTC().Format(time.RFC3339Nano)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO workspaces(id, name, root_path, default_repo_id, settings_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		SeedWorkspaceID,
		"cocode Demo",
		workspaceRoot,
		seedRepoID,
		`{"theme":"system","seeded":true,"default_review_depth":"deep"}`,
		ts(-90*time.Minute),
		ts(0),
	); err != nil {
		return Result{}, fmt.Errorf("insert workspace: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO repositories(id, workspace_id, name, owner, remote_url, local_path, default_branch, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		seedRepoID,
		SeedWorkspaceID,
		"cocode",
		"hughdo",
		"https://github.com/hughdo/cocode",
		workspaceRoot,
		"main",
		ts(-90*time.Minute),
		ts(0),
	); err != nil {
		return Result{}, fmt.Errorf("insert repository: %w", err)
	}

	preSessionArtifacts := []seededArtifact{
		{
			ID:           "seed_artifact_diff",
			Kind:         "diff",
			RelativePath: "seed/pr-42/diff.patch",
			ContentType:  "text/x-diff",
			MetadataJSON: `{"source":"seed","pr_number":42}`,
			Content: `diff --git a/apps/api/src/routes/repositories.ts b/apps/api/src/routes/repositories.ts
@@ -84,6 +84,14 @@ export async function updateRepositorySettings(req, res) {
+  // TODO: require workspace admin before saving repository settings
+  await repositoryService.updateSettings(req.params.repositoryId, req.body)
 }`,
			CreatedAt: ts(-80 * time.Minute),
		},
		{
			ID:           "seed_artifact_patch_repositories",
			Kind:         "patch",
			RelativePath: "seed/pr-42/apps-api-src-routes-repositories.patch",
			ContentType:  "text/x-diff",
			MetadataJSON: `{"source":"seed","path":"apps/api/src/routes/repositories.ts"}`,
			Content: `@@ -84,6 +84,14 @@ export async function updateRepositorySettings(req, res) {
+  await repositoryService.updateSettings(req.params.repositoryId, req.body)
 }`,
			CreatedAt: ts(-79 * time.Minute),
		},
		{
			ID:           "seed_artifact_patch_auth",
			Kind:         "patch",
			RelativePath: "seed/pr-42/apps-api-src-middleware-auth.patch",
			ContentType:  "text/x-diff",
			MetadataJSON: `{"source":"seed","path":"apps/api/src/middleware/auth.ts"}`,
			Content: `@@ -22,6 +22,10 @@ export function requireWorkspaceMember(req, res, next) {
+export function requireWorkspaceAdmin(req, res, next) {
+  return requireRole("admin")(req, res, next)
+}`,
			CreatedAt: ts(-78 * time.Minute),
		},
		{
			ID:           "seed_artifact_patch_tests",
			Kind:         "patch",
			RelativePath: "seed/pr-42/apps-api-src-routes-repositories-test.patch",
			ContentType:  "text/x-diff",
			MetadataJSON: `{"source":"seed","path":"apps/api/src/routes/repositories.test.ts"}`,
			Content: `@@ -0,0 +1,8 @@
+it("blocks repository settings updates from members", async () => {
+  await expect(updateAsMember()).rejects.toMatchObject({ status: 403 })
+})`,
			CreatedAt: ts(-77 * time.Minute),
		},
	}
	for _, artifact := range preSessionArtifacts {
		if err := insertArtifact(ctx, tx, artifactDir, artifact); err != nil {
			return Result{}, err
		}
		result.ArtifactIDs = append(result.ArtifactIDs, artifact.ID)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO pull_request_snapshots(
  id, repository_id, source_type, provider, owner, repo, pr_number, pr_title, pr_url,
  base_ref, head_ref, base_sha, head_sha, diff_artifact_id, metadata_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		seedSnapshotID,
		seedRepoID,
		"github_pr",
		"github",
		"hughdo",
		"cocode",
		42,
		"Add repository settings panel",
		"https://github.com/hughdo/cocode/pull/42",
		"main",
		"feature/repository-settings",
		"8d1f4af0",
		"bd77a91c",
		"seed_artifact_diff",
		`{"seeded":true,"files_changed":4,"additions":218,"deletions":37}`,
		ts(-75*time.Minute),
	); err != nil {
		return Result{}, fmt.Errorf("insert snapshot: %w", err)
	}

	changedFiles := []struct {
		id         string
		path       string
		status     string
		additions  int
		deletions  int
		patchID    string
		lineRanges string
	}{
		{"seed_file_repositories", "apps/api/src/routes/repositories.ts", "modified", 71, 12, "seed_artifact_patch_repositories", `[[84,112],[156,190]]`},
		{"seed_file_auth", "apps/api/src/middleware/auth.ts", "modified", 24, 4, "seed_artifact_patch_auth", `[[22,40]]`},
		{"seed_file_tests", "apps/api/src/routes/repositories.test.ts", "added", 96, 0, "seed_artifact_patch_tests", `[[1,96]]`},
	}
	for _, file := range changedFiles {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO changed_files(
  id, snapshot_id, path, old_path, status, additions, deletions, is_binary, is_generated,
  is_excluded, line_ranges_json, patch_artifact_id, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			file.id,
			seedSnapshotID,
			file.path,
			nil,
			file.status,
			file.additions,
			file.deletions,
			0,
			0,
			0,
			file.lineRanges,
			file.patchID,
			ts(-74*time.Minute),
		); err != nil {
			return Result{}, fmt.Errorf("insert changed file %s: %w", file.id, err)
		}
	}

	if err := insertAgentConfigs(ctx, tx, ts); err != nil {
		return Result{}, err
	}
	if err := insertReviewSessions(ctx, tx, ts); err != nil {
		return Result{}, err
	}

	sessionArtifacts := []seededArtifact{
		{
			ID:              "seed_artifact_context_review",
			ReviewSessionID: completedSessionID,
			Kind:            "context_bundle",
			RelativePath:    "seed/pr-42/context-review.md",
			ContentType:     "text/markdown",
			MetadataJSON:    `{"source":"seed","scope":"review","token_estimate":14280}`,
			Content:         "# Review context\n\nRepository settings route, auth middleware, and route tests for PR #42.\n",
			CreatedAt:       ts(-68 * time.Minute),
		},
		{
			ID:              "seed_artifact_codex_stdout",
			ReviewSessionID: completedSessionID,
			Kind:            "raw_output",
			RelativePath:    "seed/pr-42/agents/codex-stdout.json",
			ContentType:     "application/json",
			MetadataJSON:    `{"source":"seed","agent":"Codex Reviewer"}`,
			Content:         `{"findings":[{"severity":"high","claim":"Repository settings updates miss the workspace admin guard."}]}` + "\n",
			CreatedAt:       ts(-55 * time.Minute),
		},
		{
			ID:              "seed_artifact_verifier_stdout",
			ReviewSessionID: completedSessionID,
			Kind:            "raw_output",
			RelativePath:    "seed/pr-42/agents/verifier-stdout.json",
			ContentType:     "application/json",
			MetadataJSON:    `{"source":"seed","agent":"Local Verifier"}`,
			Content:         `{"verification_status":"verified","evidence":["route lacks requireWorkspaceAdmin","test covers only member read access"]}` + "\n",
			CreatedAt:       ts(-48 * time.Minute),
		},
		{
			ID:              "seed_artifact_parsed_findings",
			ReviewSessionID: completedSessionID,
			Kind:            "parsed_output",
			RelativePath:    "seed/pr-42/parsed-findings.json",
			ContentType:     "application/json",
			MetadataJSON:    `{"source":"seed","candidate_count":4,"canonical_count":3}`,
			Content:         `{"canonical_findings":["seed_finding_auth_guard","seed_finding_renderer_budget","seed_finding_false_positive"]}` + "\n",
			CreatedAt:       ts(-44 * time.Minute),
		},
		{
			ID:              "seed_artifact_evidence_auth",
			ReviewSessionID: completedSessionID,
			Kind:            "evidence",
			RelativePath:    "seed/pr-42/evidence/auth-guard.md",
			ContentType:     "text/markdown",
			MetadataJSON:    `{"source":"seed","finding_id":"seed_finding_auth_guard"}`,
			Content:         "The write route updates repository settings after member auth, but no admin guard is mounted before mutation.\n",
			CreatedAt:       ts(-36 * time.Minute),
		},
		{
			ID:              "seed_artifact_running_context",
			ReviewSessionID: runningSessionID,
			Kind:            "context_bundle",
			RelativePath:    "seed/local-changes/context-review.md",
			ContentType:     "text/markdown",
			MetadataJSON:    `{"source":"seed","scope":"review","token_estimate":7340}`,
			Content:         "# Running review context\n\nEvidence map layout and renderer polish changes.\n",
			CreatedAt:       ts(-14 * time.Minute),
		},
	}
	for _, artifact := range sessionArtifacts {
		if err := insertArtifact(ctx, tx, artifactDir, artifact); err != nil {
			return Result{}, err
		}
		result.ArtifactIDs = append(result.ArtifactIDs, artifact.ID)
	}

	if err := insertSessionAssignments(ctx, tx); err != nil {
		return Result{}, err
	}
	if err := insertContextBundles(ctx, tx, ts); err != nil {
		return Result{}, err
	}
	if err := insertAgentRuns(ctx, tx, ts); err != nil {
		return Result{}, err
	}
	if err := insertFindings(ctx, tx, ts); err != nil {
		return Result{}, err
	}
	if err := insertEvidence(ctx, tx, ts); err != nil {
		return Result{}, err
	}
	if err := insertEvents(ctx, tx, ts); err != nil {
		return Result{}, err
	}
	if err := insertSearchRows(ctx, tx); err != nil {
		return Result{}, err
	}

	if err := tx.Commit(); err != nil {
		return Result{}, fmt.Errorf("commit seed tx: %w", err)
	}
	return result, nil
}

func resetSeedRows(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		`DELETE FROM finding_search WHERE finding_id IN ('seed_finding_auth_guard', 'seed_finding_renderer_budget', 'seed_finding_false_positive')`,
		`DELETE FROM evidence_search WHERE evidence_item_id LIKE 'seed_evidence_%'`,
		`DELETE FROM workspaces WHERE id = 'seed_workspace_cocode'`,
		`DELETE FROM agent_configs WHERE id IN ('seed_agent_codex', 'seed_agent_verifier', 'seed_agent_static')`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("reset seed rows: %w", err)
		}
	}
	return nil
}

func writeSeedRepositoryFiles(workspaceRoot string) error {
	files := map[string]string{
		"apps/api/src/routes/repositories.ts":      seedRepositoryRouteFile(),
		"apps/api/src/middleware/auth.ts":          seedAuthMiddlewareFile(),
		"apps/api/src/routes/repositories.test.ts": seedRepositoryRouteTestFile(),
	}
	for path, content := range files {
		target := filepath.Join(workspaceRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create seed source directory for %s: %w", path, err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write seed source file %s: %w", path, err)
		}
	}
	return nil
}

func seedRepositoryRouteFile() string {
	lines := make([]string, 130)
	for i := range lines {
		lines[i] = fmt.Sprintf("// repository route context line %03d", i+1)
	}
	lines[0] = "import { FastifyInstance } from 'fastify';"
	lines[1] = "import { requireWorkspaceMember } from '../middleware/auth';"
	lines[2] = "import { repositoryService } from '../services/repository-service';"
	lines[82] = "export async function registerRepositoryRoutes(router: FastifyInstance) {"
	lines[83] = "  router.get('/repositories/:id/settings', requireWorkspaceMember, async (request, reply) => {"
	lines[84] = "    return reply.send(await repositoryService.readSettings(request.params.id));"
	lines[85] = "  });"
	lines[86] = "  router.patch('/repositories/:id/settings', requireWorkspaceMember, async (request, reply) => {"
	lines[87] = "    await repositoryService.updateSettings(request.params.id, request.body);"
	lines[88] = "    return reply.send({ ok: true });"
	lines[89] = "  });"
	lines[90] = ""
	lines[91] = "  router.post('/repositories/:id/archive', requireWorkspaceMember, async (request, reply) => {"
	lines[92] = "    await repositoryService.archive(request.params.id);"
	lines[93] = "    return reply.code(202).send({ ok: true });"
	lines[94] = "  });"
	lines[111] = "}"
	return strings.Join(lines, "\n") + "\n"
}

func seedAuthMiddlewareFile() string {
	lines := make([]string, 48)
	for i := range lines {
		lines[i] = fmt.Sprintf("// auth middleware context line %03d", i+1)
	}
	lines[0] = "export function requireWorkspaceMember(request, reply, next) {"
	lines[1] = "  if (!request.workspaceRole) {"
	lines[2] = "    return reply.code(401).send({ error: 'member required' });"
	lines[3] = "  }"
	lines[4] = "  return next();"
	lines[5] = "}"
	lines[21] = "export function requireWorkspaceAdmin(request, reply, next) {"
	lines[22] = "  if (!request.workspaceRole?.includes('admin')) {"
	lines[23] = "    return reply.code(403).send({ error: 'admin required' });"
	lines[24] = "  }"
	lines[25] = "  return next();"
	lines[26] = "}"
	return strings.Join(lines, "\n") + "\n"
}

func seedRepositoryRouteTestFile() string {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("// repository route test context line %03d", i+1)
	}
	lines[70] = "it('allows repository members to read settings', async () => {"
	lines[71] = "  await expect(readSettings(member)).resolves.toMatchObject({ ok: true });"
	lines[72] = "});"
	lines[74] = "it('allows an admin to update settings', async () => {"
	lines[75] = "  await expect(updateSettings(admin)).resolves.toMatchObject({ ok: true });"
	lines[76] = "});"
	return strings.Join(lines, "\n") + "\n"
}

func insertAgentConfigs(ctx context.Context, tx *sql.Tx, ts func(time.Duration) string) error {
	agents := []struct {
		id           string
		name         string
		role         string
		adapterKind  string
		command      string
		argsJSON     string
		modelLabel   string
		reasoning    string
		capabilities string
		settings     string
	}{
		{agentCodexID, "Codex Reviewer", "reviewer", "cli_noninteractive", "codex", `["exec","--json"]`, "gpt-5.5", "high", `{"supports_json":true,"supports_streaming":false,"supports_sessions":false,"can_read":true,"can_write":false,"can_cancel":true,"output_modes":["json","text"]}`, `{"seeded":true}`},
		{agentVerifierID, "Local Verifier", "verifier", "local_verifier", "cocode-verifier", `[]`, "local", "deterministic", `{"supports_json":true,"supports_streaming":false,"supports_sessions":false,"can_read":true,"can_write":false,"can_cancel":true,"output_modes":["json","text"]}`, `{"seeded":true}`},
		{agentStaticID, "Static Scout", "static_analysis", "local_verifier", "cocode-static", `[]`, "local", "fast", `{"supports_json":true,"supports_streaming":false,"supports_sessions":false,"can_read":true,"can_write":false,"can_cancel":true,"output_modes":["json","text"]}`, `{"seeded":true}`},
	}
	for _, agent := range agents {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_configs(
  id, name, role, adapter_kind, command, args_json, cwd_mode, env_allowlist_json, output_mode,
  model_label, reasoning_label, capabilities_json, settings_json, enabled, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			agent.id,
			agent.name,
			agent.role,
			agent.adapterKind,
			agent.command,
			agent.argsJSON,
			"repo_root",
			`["GITHUB_TOKEN","OPENAI_API_KEY"]`,
			"json",
			agent.modelLabel,
			agent.reasoning,
			agent.capabilities,
			agent.settings,
			1,
			ts(-90*time.Minute),
			ts(0),
		); err != nil {
			return fmt.Errorf("insert agent config %s: %w", agent.id, err)
		}
	}
	return nil
}

func insertReviewSessions(ctx context.Context, tx *sql.Tx, ts func(time.Duration) string) error {
	sessions := []struct {
		id          string
		title       string
		status      string
		depth       string
		focusPrompt string
		preset      string
		startedAt   any
		completedAt any
		createdAt   string
		updatedAt   string
		policy      string
	}{
		{
			id:          completedSessionID,
			title:       "PR #42 - repository settings review",
			status:      "completed",
			depth:       "deep",
			focusPrompt: "Prioritize authorization, persistence, and reviewer handoff quality.",
			preset:      "parallel-evidence",
			startedAt:   ts(-65 * time.Minute),
			completedAt: ts(-30 * time.Minute),
			createdAt:   ts(-70 * time.Minute),
			updatedAt:   ts(-30 * time.Minute),
			policy:      `{"max_files":40,"include_tests":true,"seeded":true}`,
		},
		{
			id:          runningSessionID,
			title:       "Local changes - evidence map polish",
			status:      "running",
			depth:       "standard",
			focusPrompt: "Check Evidence Map layout state, empty states, and copy packet affordances.",
			preset:      "ui-polish",
			startedAt:   ts(-12 * time.Minute),
			completedAt: nil,
			createdAt:   ts(-15 * time.Minute),
			updatedAt:   ts(-2 * time.Minute),
			policy:      `{"max_files":20,"include_tests":true,"seeded":true}`,
		},
	}
	for _, session := range sessions {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO review_sessions(
  id, workspace_id, repository_id, snapshot_id, title, status, review_depth, focus_prompt,
  preset, runtime_limit_seconds, context_policy_json, started_at, completed_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			session.id,
			SeedWorkspaceID,
			seedRepoID,
			seedSnapshotID,
			session.title,
			session.status,
			session.depth,
			session.focusPrompt,
			session.preset,
			1800,
			session.policy,
			session.startedAt,
			session.completedAt,
			session.createdAt,
			session.updatedAt,
		); err != nil {
			return fmt.Errorf("insert review session %s: %w", session.id, err)
		}
	}
	return nil
}

func insertSessionAssignments(ctx context.Context, tx *sql.Tx) error {
	assignments := []struct {
		id        string
		sessionID string
		agentID   string
		role      string
		order     int
		enabled   int
		settings  string
	}{
		{"seed_session_agent_codex", completedSessionID, agentCodexID, "primary_reviewer", 1, 1, `{"temperature":0}`},
		{"seed_session_agent_verifier", completedSessionID, agentVerifierID, "verifier", 2, 1, `{"after":"primary_reviewer"}`},
		{"seed_session_agent_static", completedSessionID, agentStaticID, "static_analysis", 0, 1, `{}`},
		{"seed_running_agent_codex", runningSessionID, agentCodexID, "primary_reviewer", 1, 1, `{"temperature":0}`},
		{"seed_running_agent_verifier", runningSessionID, agentVerifierID, "verifier", 2, 1, `{}`},
	}
	for _, assignment := range assignments {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO review_session_agents(id, review_session_id, agent_config_id, role, run_order, enabled, settings_override_json)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			assignment.id,
			assignment.sessionID,
			assignment.agentID,
			assignment.role,
			assignment.order,
			assignment.enabled,
			assignment.settings,
		); err != nil {
			return fmt.Errorf("insert review session agent %s: %w", assignment.id, err)
		}
	}
	return nil
}

func insertContextBundles(ctx context.Context, tx *sql.Tx, ts func(time.Duration) string) error {
	bundles := []struct {
		id         string
		sessionID  string
		agentID    any
		scope      string
		tokens     int
		items      int
		artifactID any
		policy     string
		createdAt  string
	}{
		{"seed_bundle_review", completedSessionID, agentCodexID, "review", 14280, 18, "seed_artifact_context_review", `{"max_tokens":18000,"seeded":true}`, ts(-67 * time.Minute)},
		{"seed_bundle_running_review", runningSessionID, agentCodexID, "review", 7340, 9, "seed_artifact_running_context", `{"max_tokens":12000,"seeded":true}`, ts(-13 * time.Minute)},
	}
	for _, bundle := range bundles {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO context_bundles(
  id, review_session_id, agent_config_id, scope, token_estimate, item_count, artifact_id, policy_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			bundle.id,
			bundle.sessionID,
			bundle.agentID,
			bundle.scope,
			bundle.tokens,
			bundle.items,
			bundle.artifactID,
			bundle.policy,
			bundle.createdAt,
		); err != nil {
			return fmt.Errorf("insert context bundle %s: %w", bundle.id, err)
		}
	}
	return nil
}

func insertAgentRuns(ctx context.Context, tx *sql.Tx, ts func(time.Duration) string) error {
	runs := []struct {
		id             string
		sessionID      string
		agentID        string
		bundleID       any
		status         string
		role           string
		startedAt      any
		completedAt    any
		durationMS     any
		exitCode       any
		stdoutID       any
		stderrID       any
		parsedOutputID any
		errorCode      any
		errorMessage   any
		metadata       string
	}{
		{"seed_run_codex", completedSessionID, agentCodexID, "seed_bundle_review", "succeeded", "primary_reviewer", ts(-62 * time.Minute), ts(-54 * time.Minute), 481000, 0, "seed_artifact_codex_stdout", nil, "seed_artifact_parsed_findings", nil, nil, `{"findings":3,"seeded":true}`},
		{"seed_run_verifier", completedSessionID, agentVerifierID, "seed_bundle_review", "succeeded", "verifier", ts(-52 * time.Minute), ts(-46 * time.Minute), 392000, 0, "seed_artifact_verifier_stdout", nil, nil, nil, nil, `{"verified":2,"downgraded":1,"seeded":true}`},
		{"seed_run_static", completedSessionID, agentStaticID, "seed_bundle_review", "succeeded", "static_analysis", ts(-60 * time.Minute), ts(-57 * time.Minute), 181000, 0, nil, nil, nil, nil, nil, `{"rules":["auth-route-guard","renderer-budget"],"seeded":true}`},
		{"seed_run_running_codex", runningSessionID, agentCodexID, "seed_bundle_running_review", "running", "primary_reviewer", ts(-12 * time.Minute), nil, nil, nil, nil, nil, nil, nil, nil, `{"phase":"normalizing candidates","seeded":true}`},
		{"seed_run_running_verifier", runningSessionID, agentVerifierID, "seed_bundle_running_review", "queued", "verifier", nil, nil, nil, nil, nil, nil, nil, nil, nil, `{"waiting_for":"primary_reviewer","seeded":true}`},
	}
	for _, run := range runs {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_runs(
  id, review_session_id, agent_config_id, context_bundle_id, status, role, started_at, completed_at,
  duration_ms, exit_code, stdout_artifact_id, stderr_artifact_id, parsed_output_artifact_id,
  error_code, error_message, metadata_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			run.id,
			run.sessionID,
			run.agentID,
			run.bundleID,
			run.status,
			run.role,
			run.startedAt,
			run.completedAt,
			run.durationMS,
			run.exitCode,
			run.stdoutID,
			run.stderrID,
			run.parsedOutputID,
			run.errorCode,
			run.errorMessage,
			run.metadata,
		); err != nil {
			return fmt.Errorf("insert agent run %s: %w", run.id, err)
		}
	}
	return nil
}

func insertFindings(ctx context.Context, tx *sql.Tx, ts func(time.Duration) string) error {
	candidates := []struct {
		id        string
		runID     string
		category  string
		severity  string
		conf      float64
		claim     string
		path      any
		startLine any
		endLine   any
		locations string
		evidence  string
		fix       any
		comment   any
		fp        string
	}{
		{"seed_candidate_auth_codex", "seed_run_codex", "security", "high", 0.91, "Repository settings updates miss the workspace admin guard.", "apps/api/src/routes/repositories.ts", 87, 112, `[{"path":"apps/api/src/routes/repositories.ts","start_line":87,"end_line":112}]`, `[{"title":"Write route mutates settings without admin guard"}]`, "Mount requireWorkspaceAdmin before updateRepositorySettings and add a 403 regression test.", "This write endpoint appears reachable by workspace members. Please require admin permission before saving repository settings.", "auth-guard-missing"},
		{"seed_candidate_auth_verifier", "seed_run_verifier", "security", "high", 0.88, "The update route uses member auth but does not enforce admin permissions before mutation.", "apps/api/src/routes/repositories.ts", 87, 112, `[{"path":"apps/api/src/routes/repositories.ts","start_line":87,"end_line":112}]`, `[{"title":"Verifier traced route to repositoryService.updateSettings"}]`, "Add admin middleware and cover member denial.", "Please add an admin guard for this mutation.", "auth-guard-missing"},
		{"seed_candidate_budget_codex", "seed_run_codex", "reliability", "medium", 0.72, "Renderer preview can load the full diff payload without a display budget.", "apps/desktop/src/renderer/src/app/App.tsx", 244, 284, `[{"path":"apps/desktop/src/renderer/src/app/App.tsx","start_line":244,"end_line":284}]`, `[{"title":"Finding card renders complete evidence text inline"}]`, "Clamp preview content and move full text behind the detail panel.", "Large diffs may make the board hard to scan.", "renderer-budget"},
		{"seed_candidate_theme_static", "seed_run_static", "ux", "low", 0.41, "Theme selection might not persist after app restart.", "apps/desktop/src/renderer/src/app/App.tsx", 40, 65, `[{"path":"apps/desktop/src/renderer/src/app/App.tsx","start_line":40,"end_line":65}]`, `[{"title":"Static scan did not see local storage access in component"}]`, nil, nil, "theme-persistence"},
	}
	for _, candidate := range candidates {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO finding_candidates(
  id, review_session_id, agent_run_id, raw_artifact_id, category, severity, confidence, claim,
  primary_path, primary_start_line, primary_end_line, locations_json, evidence_json, suggested_fix,
  draft_comment, fingerprint, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			candidate.id,
			completedSessionID,
			candidate.runID,
			"seed_artifact_parsed_findings",
			candidate.category,
			candidate.severity,
			candidate.conf,
			candidate.claim,
			candidate.path,
			candidate.startLine,
			candidate.endLine,
			candidate.locations,
			candidate.evidence,
			candidate.fix,
			candidate.comment,
			candidate.fp,
			ts(-43*time.Minute),
		); err != nil {
			return fmt.Errorf("insert finding candidate %s: %w", candidate.id, err)
		}
	}

	findings := []struct {
		id              string
		claim           string
		category        string
		severity        string
		conf            float64
		verification    string
		decision        string
		path            any
		startLine       any
		endLine         any
		evidenceSummary any
		counterSummary  any
		fix             any
		comment         any
		fp              string
		merged          int
	}{
		{"seed_finding_auth_guard", "Repository settings updates miss the workspace admin guard.", "security", "high", 0.92, "verified", "accepted", "apps/api/src/routes/repositories.ts", 87, 112, "The route reaches repositoryService.updateSettings after member authentication, while requireWorkspaceAdmin exists but is not mounted on the mutation path.", "No verified contradiction was found. Route tests cover happy path and member reads only, so they remain verification leads rather than counter-evidence.", "Mount requireWorkspaceAdmin on the PATCH route and add a member-denied regression test.", "This mutation appears reachable to workspace members. Please require workspace admin permissions before saving repository settings.", "auth-guard-missing", 2},
		{"seed_finding_renderer_budget", "Renderer preview can load the full diff payload without a display budget.", "reliability", "medium", 0.74, "plausible", "undecided", "apps/desktop/src/renderer/src/app/App.tsx", 244, 284, "The preview list renders long evidence and diff-like text inline, which can crowd the findings board on smaller windows.", "The detail view can hold complete content, but that does not refute the crowded preview claim; treat it as related UI context.", "Clamp board preview text and keep complete evidence in the detail panel.", "Consider limiting finding preview copy so large diffs do not crowd the board.", "renderer-budget", 1},
		{"seed_finding_false_positive", "Theme selection might not persist after app restart.", "ux", "low", 0.38, "likely_false_positive", "dismissed", "apps/desktop/src/renderer/src/app/App.tsx", 40, 65, "A static scan did not find persistence in the component itself.", "The theme provider owns persistence outside this component, so the component-level finding is not actionable.", nil, nil, "theme-persistence", 1},
	}
	for _, finding := range findings {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO findings(
  id, review_session_id, canonical_claim, category, severity, confidence, verification_status,
  decision_status, primary_path, primary_start_line, primary_end_line, evidence_summary,
  counter_evidence_summary, suggested_fix, draft_comment, fingerprint, merged_from_count,
  introduced_in_sha, first_seen_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			finding.id,
			completedSessionID,
			finding.claim,
			finding.category,
			finding.severity,
			finding.conf,
			finding.verification,
			finding.decision,
			finding.path,
			finding.startLine,
			finding.endLine,
			finding.evidenceSummary,
			finding.counterSummary,
			finding.fix,
			finding.comment,
			finding.fp,
			finding.merged,
			"bd77a91c",
			ts(-42*time.Minute),
			ts(-30*time.Minute),
		); err != nil {
			return fmt.Errorf("insert finding %s: %w", finding.id, err)
		}
	}

	links := []struct {
		findingID string
		candidate string
		relation  string
	}{
		{"seed_finding_auth_guard", "seed_candidate_auth_codex", "merged"},
		{"seed_finding_auth_guard", "seed_candidate_auth_verifier", "merged"},
		{"seed_finding_renderer_budget", "seed_candidate_budget_codex", "primary"},
		{"seed_finding_false_positive", "seed_candidate_theme_static", "primary"},
	}
	for _, link := range links {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO finding_candidate_links(finding_id, finding_candidate_id, relation)
VALUES (?, ?, ?)`,
			link.findingID,
			link.candidate,
			link.relation,
		); err != nil {
			return fmt.Errorf("insert finding link %s/%s: %w", link.findingID, link.candidate, err)
		}
	}

	decisions := []struct {
		id        string
		findingID string
		decision  string
		reason    string
		metadata  string
		createdAt string
	}{
		{"seed_decision_accept_auth", "seed_finding_auth_guard", "accepted", "Verified missing admin guard on a write path.", `{"seeded":true,"actor":"demo"}`, ts(-28 * time.Minute)},
		{"seed_decision_dismiss_theme", "seed_finding_false_positive", "dismissed", "Persistence is handled by the app theme provider.", `{"seeded":true,"actor":"demo"}`, ts(-26 * time.Minute)},
	}
	for _, decision := range decisions {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO human_decisions(id, finding_id, review_session_id, decision, reason, metadata_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			decision.id,
			decision.findingID,
			completedSessionID,
			decision.decision,
			decision.reason,
			decision.metadata,
			decision.createdAt,
		); err != nil {
			return fmt.Errorf("insert decision %s: %w", decision.id, err)
		}
	}
	return nil
}

func insertEvidence(ctx context.Context, tx *sql.Tx, ts func(time.Duration) string) error {
	items := []struct {
		id         string
		findingID  string
		kind       string
		title      string
		summary    string
		path       any
		startLine  any
		endLine    any
		artifactID any
		conf       float64
		metadata   string
	}{
		{"seed_evidence_auth_route", "seed_finding_auth_guard", "supporting", "Mutation route reaches settings write", "PATCH /repositories/:id/settings calls repositoryService.updateSettings after member auth without mounting requireWorkspaceAdmin.", "apps/api/src/routes/repositories.ts", 87, 112, "seed_artifact_evidence_auth", 0.94, `{"seeded":true,"node":"seed_node_changed_route","code_snippet":"router.patch('/repositories/:id/settings', requireWorkspaceMember, async (request, reply) => {\n  await repositoryService.updateSettings(request.params.id, request.body);\n  return reply.send({ ok: true });\n});","line_window":{"start_line":87,"end_line":90}}`},
		{"seed_evidence_auth_middleware", "seed_finding_auth_guard", "supporting", "Admin middleware exists but is unused", "requireWorkspaceAdmin is available in auth middleware but is not connected to the settings route.", "apps/api/src/middleware/auth.ts", 22, 40, nil, 0.86, `{"seeded":true,"node":"seed_node_admin_guard","code_snippet":"export function requireWorkspaceAdmin(request, reply, next) {\n  if (!request.workspaceRole?.includes('admin')) {\n    return reply.code(403).send({ error: 'admin required' });\n  }\n  return next();\n}","line_window":{"start_line":22,"end_line":27}}`},
		{"seed_evidence_auth_tests", "seed_finding_auth_guard", "missing", "No member-denied mutation test", "Route tests exercise successful updates and member reads, but there is no assertion that a non-admin member receives 403 for writes.", "apps/api/src/routes/repositories.test.ts", 1, 96, nil, 0.79, `{"seeded":true,"node":"seed_node_missing_test","code_snippet":"it('allows repository members to read settings', async () => {\n  await expect(readSettings(member)).resolves.toMatchObject({ ok: true });\n});\n\n// No test covers PATCH /repositories/:id/settings for a non-admin member.","line_window":{"start_line":72,"end_line":76}}`},
		{"seed_evidence_budget_preview", "seed_finding_renderer_budget", "supporting", "Finding preview renders long text inline", "The findings board preview combines claim, evidence summary, and draft text without a fixed preview budget.", "apps/desktop/src/renderer/src/app/App.tsx", 244, 284, nil, 0.72, `{"seeded":true}`},
		{"seed_evidence_budget_counter", "seed_finding_renderer_budget", "search", "Detail panel can hold complete content", "Full evidence can remain available in the detail surface, so the board only needs a concise preview.", "apps/desktop/src/renderer/src/app/App.tsx", 286, 330, nil, 0.62, `{"seeded":true}`},
		{"seed_evidence_theme_provider", "seed_finding_false_positive", "counter", "Theme provider owns persistence", "Theme persistence is handled at provider level, so the component-level static warning is not actionable.", "apps/desktop/src/renderer/src/app/providers.tsx", 12, 42, nil, 0.81, `{"seeded":true}`},
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO evidence_items(
  id, finding_id, kind, title, summary, path, start_line, end_line, artifact_id,
  confidence, metadata_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			item.id,
			item.findingID,
			item.kind,
			item.title,
			item.summary,
			item.path,
			item.startLine,
			item.endLine,
			item.artifactID,
			item.conf,
			item.metadata,
			ts(-35*time.Minute),
		); err != nil {
			return fmt.Errorf("insert evidence item %s: %w", item.id, err)
		}
	}

	graphs := []struct {
		id        string
		findingID string
		status    string
		layout    string
		summary   string
	}{
		{"seed_graph_auth_guard", "seed_finding_auth_guard", "ready", `{"direction":"LR","pinned":["seed_node_changed_route","seed_node_missing_guard"]}`, "Route, middleware, and test evidence show a missing admin guard on the write path."},
		{"seed_graph_renderer_budget", "seed_finding_renderer_budget", "ready", `{"direction":"TB","pinned":["seed_node_budget_board"]}`, "Preview content is plausible board clutter; detail view is the counterweight."},
		{"seed_graph_theme", "seed_finding_false_positive", "ready", `{"direction":"LR"}`, "A verified contradiction indicates the static theme warning is not actionable."},
	}
	for _, graph := range graphs {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO evidence_graphs(id, finding_id, review_session_id, status, layout_json, summary, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			graph.id,
			graph.findingID,
			completedSessionID,
			graph.status,
			graph.layout,
			graph.summary,
			ts(-34*time.Minute),
			ts(-30*time.Minute),
		); err != nil {
			return fmt.Errorf("insert evidence graph %s: %w", graph.id, err)
		}
	}

	nodes := []struct {
		id         string
		graphID    string
		kind       string
		label      string
		path       any
		symbol     any
		startLine  any
		endLine    any
		evidenceID any
		conf       float64
		metadata   string
	}{
		{"seed_node_changed_route", "seed_graph_auth_guard", "changed_code", "PATCH repository settings", "apps/api/src/routes/repositories.ts", "updateRepositorySettings", 87, 112, "seed_evidence_auth_route", 0.94, `{"x":120,"y":180}`},
		{"seed_node_admin_guard", "seed_graph_auth_guard", "guard", "requireWorkspaceAdmin", "apps/api/src/middleware/auth.ts", "requireWorkspaceAdmin", 22, 40, "seed_evidence_auth_middleware", 0.86, `{"x":420,"y":80}`},
		{"seed_node_missing_guard", "seed_graph_auth_guard", "missing_guard", "Admin guard not mounted", nil, nil, nil, nil, nil, 0.9, `{"x":420,"y":180}`},
		{"seed_node_missing_test", "seed_graph_auth_guard", "test", "Missing member-denied test", "apps/api/src/routes/repositories.test.ts", nil, 1, 96, "seed_evidence_auth_tests", 0.79, `{"x":420,"y":280}`},
		{"seed_node_budget_board", "seed_graph_renderer_budget", "changed_code", "Finding board preview", "apps/desktop/src/renderer/src/app/App.tsx", "FindingsBoard", 244, 284, "seed_evidence_budget_preview", 0.72, `{"x":160,"y":140}`},
		{"seed_node_budget_detail", "seed_graph_renderer_budget", "related_code", "Detail view holds full text", "apps/desktop/src/renderer/src/app/App.tsx", "FindingDetail", 286, 330, "seed_evidence_budget_counter", 0.62, `{"x":460,"y":140}`},
		{"seed_node_theme_provider", "seed_graph_theme", "counter_evidence", "Theme provider persistence", "apps/desktop/src/renderer/src/app/providers.tsx", "ThemeProvider", 12, 42, "seed_evidence_theme_provider", 0.81, `{"x":180,"y":120}`},
	}
	for _, node := range nodes {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO evidence_nodes(
  id, evidence_graph_id, kind, label, path, symbol, start_line, end_line,
  evidence_item_id, confidence, metadata_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			node.id,
			node.graphID,
			node.kind,
			node.label,
			node.path,
			node.symbol,
			node.startLine,
			node.endLine,
			node.evidenceID,
			node.conf,
			node.metadata,
		); err != nil {
			return fmt.Errorf("insert evidence node %s: %w", node.id, err)
		}
	}

	edges := []struct {
		id       string
		graphID  string
		source   string
		target   string
		kind     string
		status   string
		label    any
		conf     float64
		metadata string
	}{
		{"seed_edge_route_missing_guard", "seed_graph_auth_guard", "seed_node_changed_route", "seed_node_missing_guard", "missing_guard", "observed", "mutation lacks guard", 0.91, `{"seeded":true}`},
		{"seed_edge_admin_guard_protects", "seed_graph_auth_guard", "seed_node_admin_guard", "seed_node_changed_route", "protects", "missing", "should protect this route", 0.84, `{"seeded":true}`},
		{"seed_edge_test_supports", "seed_graph_auth_guard", "seed_node_missing_test", "seed_node_missing_guard", "supports", "observed", "test gap supports risk", 0.78, `{"seeded":true}`},
		{"seed_edge_detail_counter", "seed_graph_renderer_budget", "seed_node_budget_detail", "seed_node_budget_board", "supports", "observed", "detail keeps full text available", 0.62, `{"seeded":true}`},
		{"seed_edge_theme_counter", "seed_graph_theme", "seed_node_theme_provider", "seed_node_theme_provider", "contradicts", "observed", "provider owns persistence", 0.81, `{"seeded":true}`},
	}
	for _, edge := range edges {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO evidence_edges(
  id, evidence_graph_id, source_node_id, target_node_id, kind, status, label, confidence, metadata_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			edge.id,
			edge.graphID,
			edge.source,
			edge.target,
			edge.kind,
			edge.status,
			edge.label,
			edge.conf,
			edge.metadata,
		); err != nil {
			return fmt.Errorf("insert evidence edge %s: %w", edge.id, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO call_paths(id, evidence_graph_id, label, confidence, created_at)
VALUES (?, ?, ?, ?, ?)`,
		"seed_call_path_auth",
		"seed_graph_auth_guard",
		"PATCH /api/repositories/:id/settings -> updateRepositorySettings -> repositoryService.updateSettings",
		0.88,
		ts(-33*time.Minute),
	); err != nil {
		return fmt.Errorf("insert call path: %w", err)
	}
	steps := []struct {
		id        string
		index     int
		nodeID    any
		path      any
		startLine any
		endLine   any
		label     string
	}{
		{"seed_call_step_route", 0, "seed_node_changed_route", "apps/api/src/routes/repositories.ts", 87, 92, "Route receives settings update request"},
		{"seed_call_step_missing_guard", 1, "seed_node_missing_guard", nil, nil, nil, "Expected admin guard is missing"},
		{"seed_call_step_write", 2, "seed_node_changed_route", "apps/api/src/routes/repositories.ts", 106, 112, "Repository settings are persisted"},
	}
	for _, step := range steps {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO call_path_steps(id, call_path_id, step_index, node_id, path, start_line, end_line, label)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			step.id,
			"seed_call_path_auth",
			step.index,
			step.nodeID,
			step.path,
			step.startLine,
			step.endLine,
			step.label,
		); err != nil {
			return fmt.Errorf("insert call path step %s: %w", step.id, err)
		}
	}
	return nil
}

func insertEvents(ctx context.Context, tx *sql.Tx, ts func(time.Duration) string) error {
	events := []struct {
		id        string
		sessionID string
		runID     any
		typ       string
		level     string
		sequence  int
		payload   string
		artifact  any
		createdAt string
	}{
		{"seed_event_started", completedSessionID, nil, "ReviewSessionStarted", "info", 1, `{"phase":"starting","seeded":true}`, nil, ts(-65 * time.Minute)},
		{"seed_event_context", completedSessionID, nil, "ContextBundleCreated", "info", 2, `{"token_estimate":14280,"item_count":18}`, "seed_artifact_context_review", ts(-67 * time.Minute)},
		{"seed_event_codex_done", completedSessionID, "seed_run_codex", "AgentRunCompleted", "info", 3, `{"agent":"Codex Reviewer","findings":3}`, "seed_artifact_codex_stdout", ts(-54 * time.Minute)},
		{"seed_event_verifier_done", completedSessionID, "seed_run_verifier", "AgentRunCompleted", "info", 4, `{"agent":"Local Verifier","verified":2}`, "seed_artifact_verifier_stdout", ts(-46 * time.Minute)},
		{"seed_event_finding_auth", completedSessionID, nil, "FindingVerified", "info", 5, `{"finding_id":"seed_finding_auth_guard","severity":"high"}`, nil, ts(-38 * time.Minute)},
		{"seed_event_completed", completedSessionID, nil, "ReviewSessionCompleted", "info", 6, `{"findings":3,"accepted":1,"dismissed":1}`, nil, ts(-30 * time.Minute)},
		{"seed_running_started", runningSessionID, nil, "ReviewSessionStarted", "info", 1, `{"phase":"starting","seeded":true}`, nil, ts(-12 * time.Minute)},
		{"seed_running_context", runningSessionID, nil, "ContextBundleCreated", "info", 2, `{"token_estimate":7340,"item_count":9}`, "seed_artifact_running_context", ts(-11 * time.Minute)},
		{"seed_running_agent", runningSessionID, "seed_run_running_codex", "AgentRunStarted", "info", 3, `{"agent":"Codex Reviewer","phase":"reviewing"}`, nil, ts(-10 * time.Minute)},
	}
	for _, event := range events {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO events(id, review_session_id, agent_run_id, type, level, sequence, payload_json, artifact_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			event.id,
			event.sessionID,
			event.runID,
			event.typ,
			event.level,
			event.sequence,
			event.payload,
			event.artifact,
			event.createdAt,
		); err != nil {
			return fmt.Errorf("insert event %s: %w", event.id, err)
		}
	}
	return nil
}

func insertSearchRows(ctx context.Context, tx *sql.Tx) error {
	findings := []struct {
		id       string
		claim    string
		evidence string
		fix      string
		comment  string
	}{
		{"seed_finding_auth_guard", "Repository settings updates miss the workspace admin guard.", "The route reaches repositoryService.updateSettings after member authentication, while requireWorkspaceAdmin exists but is not mounted on the mutation path.", "Mount requireWorkspaceAdmin on the PATCH route and add a member-denied regression test.", "This mutation appears reachable to workspace members. Please require workspace admin permissions before saving repository settings."},
		{"seed_finding_renderer_budget", "Renderer preview can load the full diff payload without a display budget.", "The preview list renders long evidence and diff-like text inline, which can crowd the findings board on smaller windows.", "Clamp board preview text and keep complete evidence in the detail panel.", "Consider limiting finding preview copy so large diffs do not crowd the board."},
		{"seed_finding_false_positive", "Theme selection might not persist after app restart.", "A static scan did not find persistence in the component itself.", "", ""},
	}
	for _, finding := range findings {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO finding_search(finding_id, claim, evidence_summary, suggested_fix, draft_comment)
VALUES (?, ?, ?, ?, ?)`,
			finding.id,
			finding.claim,
			finding.evidence,
			finding.fix,
			finding.comment,
		); err != nil {
			return fmt.Errorf("insert finding search %s: %w", finding.id, err)
		}
	}

	evidence := []struct {
		id      string
		title   string
		summary string
		path    string
	}{
		{"seed_evidence_auth_route", "Mutation route reaches settings write", "PATCH /repositories/:id/settings calls repositoryService.updateSettings after member auth without mounting requireWorkspaceAdmin.", "apps/api/src/routes/repositories.ts"},
		{"seed_evidence_auth_middleware", "Admin middleware exists but is unused", "requireWorkspaceAdmin is available in auth middleware but is not connected to the settings route.", "apps/api/src/middleware/auth.ts"},
		{"seed_evidence_auth_tests", "No member-denied mutation test", "Route tests exercise successful updates and member reads, but there is no assertion that a non-admin member receives 403 for writes.", "apps/api/src/routes/repositories.test.ts"},
		{"seed_evidence_budget_preview", "Finding preview renders long text inline", "The findings board preview combines claim, evidence summary, and draft text without a fixed preview budget.", "apps/desktop/src/renderer/src/app/App.tsx"},
		{"seed_evidence_budget_counter", "Detail panel can hold complete content", "Full evidence can remain available in the detail surface, so the board only needs a concise preview.", "apps/desktop/src/renderer/src/app/App.tsx"},
		{"seed_evidence_theme_provider", "Theme provider owns persistence", "Theme persistence is handled at provider level, so the component-level static warning is not actionable.", "apps/desktop/src/renderer/src/app/providers.tsx"},
	}
	for _, item := range evidence {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO evidence_search(evidence_item_id, title, summary, path)
VALUES (?, ?, ?, ?)`,
			item.id,
			item.title,
			item.summary,
			item.path,
		); err != nil {
			return fmt.Errorf("insert evidence search %s: %w", item.id, err)
		}
	}
	return nil
}

func insertArtifact(ctx context.Context, tx *sql.Tx, artifactDir string, artifact seededArtifact) error {
	if artifact.ID == "" {
		return errors.New("artifact id is required")
	}
	if artifact.Kind == "" {
		return fmt.Errorf("artifact %s kind is required", artifact.ID)
	}
	if artifact.ContentType == "" {
		artifact.ContentType = "text/plain"
	}
	if artifact.MetadataJSON == "" {
		artifact.MetadataJSON = "{}"
	}

	relativePath := filepath.ToSlash(filepath.Clean(artifact.RelativePath))
	target, err := artifactPath(artifactDir, SeedWorkspaceID, relativePath)
	if err != nil {
		return fmt.Errorf("artifact %s path: %w", artifact.ID, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create artifact directory for %s: %w", artifact.ID, err)
	}

	content := []byte(artifact.Content)
	if err := os.WriteFile(target, content, 0o600); err != nil {
		return fmt.Errorf("write artifact %s: %w", artifact.ID, err)
	}
	digest := sha256.Sum256(content)

	var reviewSessionID any
	if artifact.ReviewSessionID != "" {
		reviewSessionID = artifact.ReviewSessionID
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO artifacts(
  id, workspace_id, review_session_id, kind, relative_path, content_type, size_bytes,
  sha256, metadata_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		artifact.ID,
		SeedWorkspaceID,
		reviewSessionID,
		artifact.Kind,
		relativePath,
		artifact.ContentType,
		len(content),
		hex.EncodeToString(digest[:]),
		artifact.MetadataJSON,
		artifact.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert artifact metadata %s: %w", artifact.ID, err)
	}
	return nil
}

func artifactPath(root string, workspaceID string, relativePath string) (string, error) {
	if workspaceID == "" || workspaceID != filepath.Base(workspaceID) || workspaceID == "." || workspaceID == ".." {
		return "", fmt.Errorf("unsafe workspace id %q", workspaceID)
	}
	if relativePath == "" || filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("unsafe artifact path %q", relativePath)
	}
	clean := filepath.Clean(relativePath)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe artifact path %q", relativePath)
	}
	target := filepath.Join(root, workspaceID, clean)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", fmt.Errorf("validate artifact path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("artifact path escapes root %q", relativePath)
	}
	return target, nil
}
