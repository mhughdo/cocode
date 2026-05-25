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

async function selectFakeOnly(page: Page) {
  await page.getByRole("button", { name: "Select orchestrator" }).click();
  await page.getByRole("menuitem", { name: /E2E Fake Reviewer/ }).click();
  await expect(page.getByRole("button", { name: "Add agent" })).toBeVisible();
  await page.getByRole("button", { name: "Add agent" }).click();
  await page.getByRole("menuitem", { name: /E2E Fake Reviewer/ }).click();

  const removeButtons = page.getByRole("button", { name: /^Remove / });
  for (let attempt = 0; attempt < 8; attempt += 1) {
    const count = await removeButtons.count();
    let removed = false;
    for (let index = 0; index < count; index += 1) {
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
}

test("starts a fake review and renders findings", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const repoPath = createBranchReviewRepo(testInfo.outputPath("branch-repo"));
  const fakeAgentPath = createFakeReviewAgent(
    join(testInfo.outputPath("bin"), "fake-review-agent"),
  );
  const app = await launchCocode(testInfo, {
    COCODE_E2E_REPOSITORY_PATH: repoPath,
  });
  const { backendInfo, page } = app;
  await createFakeAgentConfig(backendInfo, fakeAgentPath);

  try {
    await page.reload();
    await page.getByRole("button", { name: "Open project" }).last().click();
    await page.getByRole("button", { name: /Compare branches/ }).click();
    await page.getByRole("button", { name: /Base branch/ }).click();
    await page.getByLabel("Search base branch").fill("main");
    await page.getByRole("menuitem", { name: /main/ }).click();
    await page.getByRole("button", { name: /Head branch/ }).click();
    await page.getByLabel("Search head branch").fill("feature");
    await page.getByRole("menuitem", { name: /feature\/review-auth/ }).click();

    await expect(
      page.getByRole("heading", { name: "Set up review" }),
    ).toBeVisible();
    await clearPrimaryPresets(page);
    await selectFakeOnly(page);
    await page.getByRole("button", { name: "Start review" }).click();

    await expect(
      page.getByRole("heading", {
        name: "branch-repo main..feature/review-auth",
      }),
    ).toBeVisible();
    await expect(page.getByRole("tab", { name: "Findings" })).toBeVisible();
    await page.getByRole("tab", { name: "Findings" }).click();
    const findingRow = page
      .locator('[data-testid^="finding-row-"]')
      .filter({
        hasText:
          "Repository update permissions now allow members to mutate settings.",
      })
      .first();
    await expect(
      findingRow.getByRole("button", {
        name: "Repository update permissions now allow members to mutate settings.",
      }),
    ).toBeVisible();
    await expect(findingRow).toContainText("Needs triage");
  } finally {
    await closeCocode(app);
  }
});
