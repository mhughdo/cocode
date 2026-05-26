import { expect, test, type TestInfo } from "@playwright/test";

import {
  apiRequest,
  createBranchReviewRepo,
  launchCocode,
  type BackendInfo,
} from "./test-support";
import {
  createRealCliAgentConfig,
  ensureCommandAvailable,
  isExternalProviderLimit,
  selectedRealCliTargets,
} from "./real-cli-support";

type Workspace = {
  id: string;
  root_path: string;
};

type Repository = {
  id: string;
  workspace_id: string;
  local_path: string;
};

type OpenRepositoryResponse = {
  workspace: Workspace;
  repository: Repository;
  repositories: Repository[];
};

type Snapshot = {
  id: string;
};

type ReviewSession = {
  id: string;
  status: string;
};

type AgentRunSummary = {
  agent_config_id: string;
  status: string;
  error_code?: string;
  error_message?: string;
};

type ReviewSessionSummary = {
  status: string;
  active_agents: number;
  agent_runs_total: number;
  agent_runs?: AgentRunSummary[];
};

type Finding = {
  canonical_claim: string;
  primary_path?: string;
  evidence_summary?: string;
  suggested_fix?: string;
  draft_comment?: string;
};

type FindingListResponse = {
  items: Finding[];
};

type RealCliAgentConfig = {
  id: string;
  name: string;
};

const selectedTargets = selectedRealCliTargets("COCODE_E2E_REAL_REVIEW_CLIS");
const reviewFocus =
  "Focus on security regressions in authorization changes. Treat expanding canUpdateRepository from admin-only to member as a high-severity finding if visible. Return only JSON with findings.";

if (selectedTargets.length === 0) {
  test("real CLI branch reviews are opt-in", async () => {
    test.skip(
      true,
      "Set COCODE_E2E_REAL_REVIEW_CLIS=codex,gemini,opencode,agy,claude or all to run a real local CLI review.",
    );
  });
} else {
  test("runs selected local CLIs through a real branch review", async ({
    browserName,
  }, testInfo) => {
    const timeoutMs = Math.max(420_000, selectedTargets.length * 210_000);
    test.setTimeout(timeoutMs + 90_000);
    expect(browserName).toBe("chromium");
    for (const target of selectedTargets) {
      ensureCommandAvailable(target.command);
    }

    const repoPath = createBranchReviewRepo(
      testInfo.outputPath("real-cli-review-repo"),
    );
    const { backendInfo, electronApp, page } = await launchCocode(testInfo, {
      COCODE_E2E_REPOSITORY_PATH: repoPath,
    });

    try {
      const configs: RealCliAgentConfig[] = [];
      for (const target of selectedTargets) {
        configs.push(
          await createRealCliAgentConfig(backendInfo, target, {
            purpose: "review",
            timeoutSeconds: 420,
          }),
        );
      }

      await expect(page.getByText("cocode").first()).toBeVisible();
      const opened = await openRepository(backendInfo, repoPath);
      const snapshot = await createBranchSnapshot(
        backendInfo,
        opened.workspace.id,
        opened.repository.id,
      );
      const session = await createReviewSession(
        backendInfo,
        opened.workspace.id,
        snapshot.id,
        configs,
      );
      await startReviewSession(backendInfo, session.id);

      const summary = await waitForReviewCompletion(
        backendInfo,
        session.id,
        timeoutMs,
      );
      await assertSelectedRunsCompleted(
        backendInfo,
        session.id,
        summary,
        configs.map((config) => config.id),
        testInfo,
      );

      const findings = await apiRequest<FindingListResponse>(
        backendInfo,
        `/api/review-sessions/${encodeURIComponent(session.id)}/findings`,
      );
      expect(findings.items.length).toBeGreaterThan(0);
      expect(
        findings.items.some(
          (finding) => finding.primary_path === "src/auth.ts",
        ),
      ).toBe(true);
      const findingText = findings.items
        .map((finding) =>
          [
            finding.canonical_claim,
            finding.evidence_summary,
            finding.suggested_fix,
            finding.draft_comment,
          ].join("\n"),
        )
        .join("\n")
        .toLowerCase();
      expect(findingText).toMatch(
        /member|admin|authorization|permission|privilege/,
      );

      const refreshedSession = await apiRequest<ReviewSession>(
        backendInfo,
        `/api/review-sessions/${encodeURIComponent(session.id)}`,
      );
      expect(refreshedSession.status).toBe("completed");
    } finally {
      await electronApp.close();
    }
  });
}

async function openRepository(
  backendInfo: BackendInfo,
  repoPath: string,
): Promise<OpenRepositoryResponse> {
  return apiRequest<OpenRepositoryResponse>(
    backendInfo,
    "/api/workspaces/open-repository",
    {
      method: "POST",
      body: { path: repoPath },
    },
  );
}

async function createBranchSnapshot(
  backendInfo: BackendInfo,
  workspaceID: string,
  repositoryID: string,
): Promise<Snapshot> {
  return apiRequest<Snapshot>(
    backendInfo,
    "/api/pr-snapshots/from-local-compare",
    {
      method: "POST",
      body: {
        workspace_id: workspaceID,
        repository_id: repositoryID,
        base_ref: "main",
        head_ref: "feature/review-auth",
      },
    },
  );
}

async function createReviewSession(
  backendInfo: BackendInfo,
  workspaceID: string,
  snapshotID: string,
  configs: RealCliAgentConfig[],
): Promise<ReviewSession> {
  return apiRequest<ReviewSession>(backendInfo, "/api/review-sessions", {
    method: "POST",
    body: {
      workspace_id: workspaceID,
      snapshot_id: snapshotID,
      title: "Real CLI authorization review",
      review_depth: "quick",
      focus_prompt: reviewFocus,
      preset: "real-cli-e2e",
      runtime_limit_seconds: 1800,
      context_policy: {
        include_prompt_material: true,
        include_changed_code: true,
        include_related_call_sites: false,
        include_related_tests: false,
        include_project_conventions: false,
        include_prior_comments: false,
        include_prior_decisions: false,
        redact_secrets: true,
        max_tokens: 10_000,
        max_items: 80,
      },
      agent_selections: configs.map((config) => ({
        agent_config_id: config.id,
        role: "General Reviewer",
      })),
    },
  });
}

async function startReviewSession(
  backendInfo: BackendInfo,
  sessionID: string,
): Promise<ReviewSession> {
  return apiRequest<ReviewSession>(
    backendInfo,
    `/api/review-sessions/${encodeURIComponent(sessionID)}/start`,
    { method: "POST" },
  );
}

async function waitForReviewCompletion(
  backendInfo: BackendInfo,
  sessionID: string,
  timeoutMs: number,
): Promise<ReviewSessionSummary> {
  const deadline = Date.now() + timeoutMs;
  let latest: ReviewSessionSummary | undefined;
  while (Date.now() < deadline) {
    latest = await apiRequest<ReviewSessionSummary>(
      backendInfo,
      `/api/review-sessions/${encodeURIComponent(sessionID)}/summary`,
    );
    if (
      latest.active_agents === 0 &&
      latest.agent_runs_total > 0 &&
      ["completed", "failed", "canceled"].includes(latest.status)
    ) {
      return latest;
    }
    await delay(2_000);
  }
  throw new Error(
    `Timed out waiting for real CLI review completion. Latest summary: ${JSON.stringify(latest)}`,
  );
}

async function assertSelectedRunsCompleted(
  backendInfo: BackendInfo,
  sessionID: string,
  summary: ReviewSessionSummary,
  agentConfigIDs: string[],
  testInfo: TestInfo,
) {
  const runs = summary.agent_runs ?? [];
  const selectedRuns = runs.filter((run) =>
    agentConfigIDs.includes(run.agent_config_id),
  );
  const configsWithRuns = new Set(
    selectedRuns.map((run) => run.agent_config_id),
  );
  const missingConfigIDs = agentConfigIDs.filter(
    (id) => !configsWithRuns.has(id),
  );
  const successfulRunStatuses = new Set(["completed", "succeeded"]);
  const failedRuns = selectedRuns.filter(
    (run) => !successfulRunStatuses.has(run.status),
  );
  if (
    summary.status === "completed" &&
    missingConfigIDs.length === 0 &&
    failedRuns.length === 0
  ) {
    return;
  }

  const auditLog = await apiRequest<unknown>(
    backendInfo,
    `/api/review-sessions/${encodeURIComponent(sessionID)}/audit-log`,
  );
  const details = JSON.stringify({
    summary,
    auditLog,
    expected_agent_config_ids: agentConfigIDs,
    missing_agent_config_ids: missingConfigIDs,
  });
  if (isExternalProviderLimit(details)) {
    testInfo.annotations.push({
      type: "external-provider-limit",
      description:
        "A real CLI provider returned a capacity, quota, or rate-limit response during the branch review.",
    });
    test.skip(
      true,
      "A provider capacity, quota, or rate-limit response blocked the real CLI review.",
    );
  }
  throw new Error(`Real CLI review did not complete cleanly: ${details}`);
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
