import { expect, test } from "@playwright/test";

import {
  createBranchReviewRepo,
  launchCocode,
  saveGitHubToken,
  startFakeGitHubServer,
} from "./test-support";

test("creates a GitHub PR snapshot and reports invalid PR URLs", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const repoPath = createBranchReviewRepo(testInfo.outputPath("github-repo"));
  const github = await startFakeGitHubServer();
  const { electronApp, page } = await launchCocode(testInfo, {
    COCODED_GITHUB_API_BASE_URL: github.url,
    COCODE_E2E_REPOSITORY_PATH: repoPath,
  });

  try {
    await saveGitHubToken(page);
    await page
      .getByRole("button", { name: "Open local project" })
      .last()
      .click();

    await page.getByRole("button", { name: /GitHub PR/ }).click();
    await page
      .getByLabel("Pull request URL")
      .fill("https://github.com/octo-org/hello-world/issues/42");
    await page.getByRole("button", { name: "Continue to configure" }).click();
    await expect(page.getByText("Could not create snapshot")).toBeVisible();
    await expect(
      page.getByText(
        "GitHub PR URL must look like https://github.com/{owner}/{repo}/pull/{number}",
      ),
    ).toBeVisible();

    await page
      .getByLabel("Pull request URL")
      .fill("https://github.com/octo-org/hello-world/pull/42");
    await page.getByRole("button", { name: "Continue to configure" }).click();

    await expect(
      page.getByRole("heading", { name: "Configure review" }),
    ).toBeVisible();
    await expect(page.getByText("Tighten repository auth")).toBeVisible();
    await expect(page.getByText("1 files")).toBeVisible();
    await expect(page.getByText("1 total")).toBeVisible();
    await expect(
      page.getByText("apps/api/src/routes/repositories.ts").first(),
    ).toBeVisible();
    await expect(page.getByText("modified").first()).toBeVisible();
    await expect(page.getByText("+2")).toBeVisible();
    await expect(page.getByText("-1")).toBeVisible();
  } finally {
    await electronApp.close();
    await github.close();
  }
});
