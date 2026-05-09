import { expect, test, type Page } from "@playwright/test";
import { join } from "node:path";

import {
  closeCocode,
  createBranchReviewRepo,
  createFakeAgentConfig,
  createFakeReviewAgent,
  launchCocode,
} from "./test-support";

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
  await expect(
    page.getByRole("button", { name: /agents selected/ }),
  ).toBeVisible();
}

test("routes a centralized chat follow-up to a selected CLI reviewer", async ({
  browserName,
}, testInfo) => {
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
    await expect(page.getByText("Review chat")).toBeVisible();
    await expect(page.getByText("Orchestrator").first()).toBeVisible();

    await page
      .getByRole("button", { name: "Choose centralized chat responder" })
      .click();
    await page.getByRole("menuitem", { name: /E2E Fake Reviewer/ }).click();
    await page
      .getByLabel("Centralized review message")
      .fill("Can you re-check the authorization delta?");
    await page
      .getByRole("button", { name: "Send centralized chat message" })
      .click();

    await expect(
      page.getByText("Can you re-check the authorization delta?"),
    ).toBeVisible();
    await expect(
      page.getByText("Found one deterministic branch comparison issue.").last(),
    ).toBeVisible({ timeout: 20_000 });
    await expect(page.getByText("agent run").last()).toBeVisible();
  } finally {
    await closeCocode(app);
  }
});
