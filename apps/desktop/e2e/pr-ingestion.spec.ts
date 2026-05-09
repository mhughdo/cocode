import { expect, test } from "@playwright/test";
import { join } from "node:path";

import {
  createBranchReviewRepo,
  createFakeAgentConfig,
  createFakeReviewAgent,
  launchCocode,
  saveGitHubToken,
  startFakeGitHubServer,
} from "./test-support";

test("creates a GitHub PR snapshot and reports invalid PR URLs", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const repoPath = createBranchReviewRepo(testInfo.outputPath("github-repo"));
  const fakeAgentPath = createFakeReviewAgent(
    join(testInfo.outputPath("bin"), "fake-review-agent"),
  );
  const github = await startFakeGitHubServer();
  const { backendInfo, electronApp, page } = await launchCocode(testInfo, {
    COCODED_GITHUB_API_BASE_URL: github.url,
    COCODE_E2E_REPOSITORY_PATH: repoPath,
  });
  await createFakeAgentConfig(backendInfo, fakeAgentPath);

  try {
    await saveGitHubToken(page);
    await page.getByRole("button", { name: "Open project" }).last().click();

    await page.getByRole("button", { name: /GitHub PR/ }).click();
    await page
      .getByLabel("Pull request URL")
      .fill("https://github.com/octo-org/hello-world/issues/42");
    await page.getByRole("button", { name: "Start review" }).click();
    await expect(page.getByText("Could not create snapshot")).toBeVisible();
    await expect(
      page.getByText(
        "GitHub PR URL must look like https://github.com/{owner}/{repo}/pull/{number}",
      ),
    ).toBeVisible();

    await page
      .getByLabel("Pull request URL")
      .fill("https://github.com/octo-org/hello-world/pull/42");
    await page.getByRole("button", { name: "Start review" }).click();

    await expect(
      page.getByRole("heading", { name: "Tighten repository auth" }),
    ).toBeVisible();
    await expect(page.getByRole("tab", { name: "Findings" })).toBeVisible();
  } finally {
    await electronApp.close();
    await github.close();
  }
});
