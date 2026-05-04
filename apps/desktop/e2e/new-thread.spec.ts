import { expect, test } from "@playwright/test";

import { createBranchReviewRepo, launchCocode } from "./test-support";

test("opens a local repository and configures a branch comparison", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const repoPath = createBranchReviewRepo(testInfo.outputPath("branch-repo"));
  const { electronApp, page } = await launchCocode(testInfo, {
    COCODE_E2E_REPOSITORY_PATH: repoPath,
  });

  try {
    await page
      .getByRole("button", { name: "Open local project" })
      .last()
      .click();

    await expect(
      page.getByRole("heading", { name: "What should we review?" }),
    ).toBeVisible();
    await expect(page.getByText(repoPath).first()).toBeVisible();

    await page.getByRole("button", { name: /Compare branches/ }).click();
    await page.getByLabel("Base ref").fill("main");
    await page.getByLabel("Head ref").fill("feature/review-auth");
    await page.getByRole("button", { name: "Continue to configure" }).click();

    await expect(
      page.getByRole("heading", { name: "Configure review" }),
    ).toBeVisible();
    await expect(
      page.getByText("branch-repo main..feature/review-auth"),
    ).toBeVisible();
    await expect(page.getByText("src/auth.ts").first()).toBeVisible();
    await expect(page.getByText("1 total")).toBeVisible();
  } finally {
    await electronApp.close();
  }
});
