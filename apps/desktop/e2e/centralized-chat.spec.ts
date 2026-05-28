import { expect, test, type Page } from "@playwright/test";
import { join } from "node:path";

import {
  apiRequest,
  closeCocode,
  createBranchReviewRepo,
  createFakeAgentConfig,
  createFakeReviewAgent,
  launchCocode,
} from "./test-support";

type Workspace = {
  id: string;
  name: string;
};

type ReviewSession = {
  id: string;
  title: string;
};

const primaryPresetButtons = {
  standard: "Standard Review Default protocol",
  security: "Security & Auth Auth, tenant, secrets",
  performance: "Performance CPU, memory, I/O",
  sql: "SQL Review SQL and indexes",
} as const;

async function openSeededProject(page: Page) {
  await page.getByRole("button", { name: "Open project" }).last().click();
  await expect(
    page.getByRole("heading", { name: "Set up review" }),
  ).toBeVisible();
}

async function setPrimaryPreset(page: Page, name: string, selected: boolean) {
  const preset = page.getByRole("button", { name, exact: true });
  if (((await preset.getAttribute("aria-pressed")) === "true") !== selected) {
    await preset.click();
  }
}

async function clearPrimaryPresets(page: Page) {
  await setPrimaryPreset(page, primaryPresetButtons.standard, false);
  await setPrimaryPreset(page, primaryPresetButtons.security, false);
  await setPrimaryPreset(page, primaryPresetButtons.performance, false);
  await setPrimaryPreset(page, primaryPresetButtons.sql, false);
}

async function selectFakeOrchestrator(page: Page) {
  await page.getByRole("button", { name: "Select orchestrator" }).click();
  await page.getByRole("menuitem", { name: /E2E Fake Reviewer/ }).click();
}

async function configureBranchComparison(page: Page) {
  await page.getByRole("button", { name: /Compare branches/ }).click();
  await page.getByRole("button", { name: /Base branch/ }).click();
  await page.getByLabel("Search base branch").fill("main");
  await page.getByRole("menuitem", { name: /main/ }).click();
  await page.getByRole("button", { name: /Head branch/ }).click();
  await page.getByLabel("Search head branch").fill("feature");
  await page.getByRole("menuitem", { name: /feature\/review-auth/ }).click();
}

async function addFakeReviewer(page: Page) {
  await expect(page.getByRole("button", { name: "Add agent" })).toBeVisible();
  await page.getByRole("button", { name: "Add agent" }).click();
  await page.getByRole("menuitem", { name: /E2E Fake Reviewer/ }).click();
  const removeButtons = page.getByRole("button", { name: /^Remove / });
  for (let attempt = 0; attempt < 8; attempt++) {
    const count = await removeButtons.count();
    let removed = false;
    for (let index = 0; index < count; index++) {
      const button = removeButtons.nth(index);
      const label = (await button.getAttribute("aria-label")) ?? "";
      if (!label.includes("E2E Fake Reviewer")) {
        await button.click();
        removed = true;
        break;
      }
    }
    if (!removed) {
      break;
    }
  }
  await expect(
    page.getByRole("button", { name: /agents selected/ }),
  ).toBeVisible();
}

test("routes centralized chat follow-ups to a selected participant agent", async ({
  browserName,
}, testInfo) => {
  test.setTimeout(90_000);
  expect(browserName).toBe("chromium");
  const repoPath = createBranchReviewRepo(testInfo.outputPath("chat-repo"));
  const fakeAgentPath = createFakeReviewAgent(
    join(testInfo.outputPath("bin"), "fake-review-agent"),
  );
  const app = await launchCocode(testInfo, {
    COCODE_E2E_REPOSITORY_PATH: repoPath,
  });
  const { backendInfo, page } = app;

  try {
    await createFakeAgentConfig(backendInfo, fakeAgentPath);
    await page.reload();
    await openSeededProject(page);
    await configureBranchComparison(page);
    await clearPrimaryPresets(page);
    await selectFakeOrchestrator(page);
    await addFakeReviewer(page);

    await page.getByRole("button", { name: "Start review" }).click();
    await expect(
      page.getByRole("heading", {
        name: "chat-repo main..feature/review-auth",
      }),
    ).toBeVisible();
    await expect(page.getByRole("tab", { name: "Chat" })).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Review summary" }),
    ).toBeVisible();
    await expect(
      page.getByText("Review queued. cocode is preparing context"),
    ).toHaveCount(0);
    await expect(page.getByText("Orchestrator").first()).toBeVisible();
    await expect(
      page.getByTestId("orchestrator-agent-badge").first(),
    ).toBeVisible();

    await page
      .getByRole("button", { name: "Choose centralized chat responder" })
      .click();
    await expect(
      page.getByRole("menuitem", { name: /Orchestrator/ }),
    ).toBeVisible();
    await page
      .getByRole("menuitem", { name: /E2E Fake Reviewer/ })
      .first()
      .click();
    await expect(page.getByText(/Agent: E2E Fake Reviewer/)).toBeVisible();
    await page
      .getByLabel("Centralized review message")
      .fill("Can you re-check the authorization delta?");
    await page.getByLabel("Centralized review message").press("Enter");

    await expect(
      page.getByText("Can you re-check the authorization delta?"),
    ).toBeVisible();
    await expect(
      page.getByText("Found one deterministic branch comparison issue.").last(),
    ).toBeVisible({ timeout: 20_000 });
    await expect(page.getByText("completed").last()).toBeVisible();
    await expect(page.getByLabel("Finalized findings")).toBeVisible();
    await expect(
      page.getByLabel("Finalized findings").getByText("Cocode"),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: /Open Findings/ }),
    ).toBeVisible();

    const workspaces = await apiRequest<Workspace[]>(
      backendInfo,
      "/api/workspaces",
    );
    const workspace = workspaces.find((item) => item.name === "chat-repo");
    expect(workspace).toBeTruthy();
    const sessions = await apiRequest<ReviewSession[]>(
      backendInfo,
      `/api/review-sessions?workspace_id=${encodeURIComponent(workspace!.id)}`,
    );
    const session = sessions.find((item) =>
      item.title.includes("feature/review-auth"),
    );
    expect(session).toBeTruthy();
    for (let index = 0; index < 8; index++) {
      await apiRequest(
        backendInfo,
        `/api/review-sessions/${encodeURIComponent(session!.id)}/chat-turns`,
        {
          method: "POST",
          body: {
            body: `Scroll probe ${index + 1}: summarize the current review state.`,
            mode: "follow_up",
            audience: "orchestrator",
            include_evidence: false,
            include_recent_messages: false,
          },
        },
      );
    }
    await page.reload();
    await page.getByRole("button", { name: /chat-repo/ }).click();
    await page
      .getByRole("button", { name: /chat-repo main\.\.feature\/review-auth/ })
      .click();
    await expect(page.getByText("Scroll probe 8")).toBeVisible();
    await page.setViewportSize({ width: 1180, height: 620 });
    const chatMessages = page.getByLabel("Centralized chat messages");
    await expect
      .poll(async () =>
        chatMessages.evaluate((node) => node.scrollHeight > node.clientHeight),
      )
      .toBe(true);
    await chatMessages.evaluate((node) => {
      node.scrollTop = 0;
    });
    await expect
      .poll(async () => chatMessages.evaluate((node) => node.scrollTop))
      .toBe(0);
    await chatMessages.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
    });
    await expect
      .poll(async () => chatMessages.evaluate((node) => node.scrollTop))
      .toBeGreaterThan(0);

    await page.setViewportSize({ width: 1440, height: 900 });
    const chatBounds = await chatMessages.boundingBox();
    await page.getByRole("tab", { name: "Findings" }).click();
    const findingsBoard = page.getByLabel("Review findings board");
    await expect(findingsBoard).toBeVisible({ timeout: 20_000 });
    await page.getByRole("tab", { name: "Chat" }).click();
    await expect(page.getByText("Scroll probe 8")).toBeVisible();
    await expect(
      page.getByText("Can you re-check the authorization delta?"),
    ).toBeVisible();
    await page.getByRole("tab", { name: "Findings" }).click();
    await expect(findingsBoard).toBeVisible();
    const findingsBounds = await findingsBoard.boundingBox();
    expect(chatBounds).toBeTruthy();
    expect(findingsBounds).toBeTruthy();
    expect(
      Math.abs((chatBounds?.x ?? 0) - (findingsBounds?.x ?? 0)),
    ).toBeLessThanOrEqual(16);
    const firstFindingRow = findingsBoard
      .locator('[data-testid^="finding-row-"]')
      .first();
    await expect(firstFindingRow).toBeVisible();
    await firstFindingRow.click({ position: { x: 12, y: 12 } });
    await expect(
      findingsBoard
        .getByText(
          "Repository update permissions now allow members to mutate settings.",
          { exact: true },
        )
        .first(),
    ).toBeVisible();
    await expect(
      findingsBoard.getByLabel("Draft GitHub comment"),
    ).toBeVisible();
    await expect(findingsBoard.getByLabel("Draft GitHub comment")).toHaveValue(
      /Please keep repository settings updates admin-only\./,
    );
    await findingsBoard
      .getByRole("button", { exact: true, name: "Accept finding" })
      .click();
    await expect(page.getByText("Accepted saved")).toBeVisible();

    await page.getByRole("button", { name: "Open full detail" }).click();
    await expect(
      page.getByRole("heading", {
        name: "Repository update permissions now allow members to mutate settings.",
      }),
    ).toBeVisible();
    await expect(page.getByText("Finding thread")).toBeVisible();
    await page
      .getByLabel("Follow-up prompt")
      .fill("What evidence supports this finding?");
    await page.getByRole("button", { name: "Send follow-up question" }).click();
    await expect(
      page.getByText("What evidence supports this finding?"),
    ).toBeVisible();
    await expect(page.getByText("Reasoning and tool trace").last()).toBeVisible(
      {
        timeout: 20_000,
      },
    );
    await expect(
      page.getByText("Found one deterministic branch comparison issue.").last(),
    ).toBeVisible({ timeout: 20_000 });

    await page.getByRole("button", { name: "Evidence map" }).first().click();
    await expect(
      page.getByRole("heading", { name: "Evidence Map" }),
    ).toBeVisible();
    await expect(page.getByText("Evidence flow")).toBeVisible();
    await expect(page.getByText("Changed code").nth(1)).toBeVisible();
    await expect(page.getByText("Selected location")).toBeVisible();
    await expect(page.getByText("Source file")).toBeVisible();
    await expect(
      page.getByText("Related evidence", { exact: true }),
    ).toBeVisible();

    await page.getByRole("button", { name: "Findings" }).click();
    await expect(page.getByLabel("Review findings board")).toBeVisible();
    const publishTab = page.getByRole("tab", { name: "Publish" });
    await publishTab.click();
    await expect(publishTab).toHaveAttribute("aria-selected", "true");
    await expect(page.getByLabel("Publish review preview")).toBeVisible();
    await expect(page.getByText("GitHub review preview")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Copy packet" }),
    ).toBeVisible();
    await expect(
      page.getByText("Repository update permissions").first(),
    ).toBeVisible({ timeout: 20_000 });
  } finally {
    await closeCocode(app);
  }
});
