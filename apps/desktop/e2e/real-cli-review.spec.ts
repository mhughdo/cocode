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

const selectedTargets = selectedRealCliTargets("COCODE_E2E_REAL_REVIEW_CLIS");
const reviewFocus =
  "Focus on security regressions in authorization changes. Treat expanding canUpdateRepository from admin-only to member as a high-severity finding if visible. Return only JSON with findings.";

if (selectedTargets.length === 0) {
  test("real CLI branch reviews are opt-in", async () => {
    test.skip(
      true,
      "Set COCODE_E2E_REAL_REVIEW_CLIS=codex,gemini,opencode,claude or all to run a real local CLI review.",
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
      const configs: Array<{ id: string; name: string }> = [];
      for (const target of selectedTargets) {
        configs.push(
          await createRealCliAgentConfig(backendInfo, target, {
            purpose: "review",
            timeoutSeconds: 300,
          }),
        );
      }

      await page.getByRole("button", { name: "Open project" }).last().click();
      const workspace = await waitForWorkspace(backendInfo, repoPath);

      await page.getByRole("button", { name: /Compare branches/ }).click();
      await page.getByLabel("Base ref").fill("main");
      await page.getByLabel("Head ref").fill("feature/review-auth");

      await expect(
        page.getByRole("heading", { name: "Set up review" }),
      ).toBeVisible();
      for (const config of configs) {
        await expect(
          page.getByRole("button", { name: config.name }).first(),
        ).toBeVisible();
      }
      await page.getByLabel("Focus prompt").fill(reviewFocus);
      await page.getByRole("button", { name: "Start review" }).click();

      const session = await waitForLatestSession(backendInfo, workspace.id);
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

      await expect(page.getByRole("tab", { name: "Findings" })).toBeVisible();
      await page.getByRole("tab", { name: "Findings" }).click();
      await expect(page.getByText("src/auth.ts").first()).toBeVisible();
    } finally {
      await electronApp.close();
    }
  });
}

async function waitForWorkspace(
  backendInfo: BackendInfo,
  repoPath: string,
): Promise<Workspace> {
  const deadline = Date.now() + 30_000;
  let lastWorkspaces: Workspace[] = [];
  while (Date.now() < deadline) {
    lastWorkspaces = await apiRequest<Workspace[]>(
      backendInfo,
      "/api/workspaces",
    );
    const workspace =
      lastWorkspaces.find((item) => item.root_path === repoPath) ??
      lastWorkspaces.find((item) => repoPath.startsWith(item.root_path));
    if (workspace) {
      return workspace;
    }
    await delay(500);
  }
  throw new Error(
    `Timed out waiting for workspace for ${repoPath}. Workspaces: ${JSON.stringify(lastWorkspaces)}`,
  );
}

async function waitForLatestSession(
  backendInfo: BackendInfo,
  workspaceID: string,
): Promise<ReviewSession> {
  const deadline = Date.now() + 30_000;
  let lastSessions: ReviewSession[] = [];
  while (Date.now() < deadline) {
    lastSessions = await apiRequest<ReviewSession[]>(
      backendInfo,
      `/api/review-sessions?workspace_id=${encodeURIComponent(workspaceID)}`,
    );
    if (lastSessions[0]) {
      return lastSessions[0];
    }
    await delay(500);
  }
  throw new Error(
    `Timed out waiting for a review session. Sessions: ${JSON.stringify(lastSessions)}`,
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
  expect(selectedRuns.length).toBe(agentConfigIDs.length);
  const successfulRunStatuses = new Set(["completed", "succeeded"]);
  const failedRuns = selectedRuns.filter(
    (run) => !successfulRunStatuses.has(run.status),
  );
  if (summary.status === "completed" && failedRuns.length === 0) {
    return;
  }

  const auditLog = await apiRequest<unknown>(
    backendInfo,
    `/api/review-sessions/${encodeURIComponent(sessionID)}/audit-log`,
  );
  const details = JSON.stringify({ summary, auditLog });
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
