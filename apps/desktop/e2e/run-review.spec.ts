import { expect, test } from "@playwright/test";
import { join } from "node:path";

import {
  createBranchReviewRepo,
  createFakeAgentConfig,
  createFakeReviewAgent,
  launchCocode,
} from "./test-support";

test("starts a fake review and renders findings", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const repoPath = createBranchReviewRepo(testInfo.outputPath("branch-repo"));
  const fakeAgentPath = createFakeReviewAgent(
    join(testInfo.outputPath("bin"), "fake-review-agent"),
  );
  const { backendInfo, electronApp, page } = await launchCocode(testInfo, {
    COCODE_E2E_REPOSITORY_PATH: repoPath,
  });
  await createFakeAgentConfig(backendInfo, fakeAgentPath);

  try {
    await page.getByRole("button", { name: "Open local repo" }).last().click();
    await page.getByRole("button", { name: /Branch compare/ }).click();
    await page.getByLabel("Base ref").fill("main");
    await page.getByLabel("Head ref").fill("feature/review-auth");
    await page.getByRole("button", { name: "Continue to configure" }).click();

    await expect(
      page.getByRole("heading", { name: "Configure review" }),
    ).toBeVisible();
    await expect(page.getByText("E2E Fake Reviewer")).toBeVisible();
    await page.getByRole("button", { name: "Start review" }).click();

    await expect(page.getByRole("tab", { name: "Findings" })).toBeVisible();
    await page.getByRole("tab", { name: "Findings" }).click();
    await expect(
      page.getByRole("heading", {
        name: "Repository update permissions now allow members to mutate settings.",
      }),
    ).toBeVisible();
    await expect(page.getByText("src/auth.ts").first()).toBeVisible();
    await expect(page.getByText("Verified").first()).toBeVisible();
  } finally {
    await electronApp.close();
  }
});
