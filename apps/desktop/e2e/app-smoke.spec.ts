import { expect, test, type Route } from "@playwright/test";

import {
  apiRequest,
  createBranchReviewRepo,
  launchCocode,
} from "./test-support";

type OpenRepositorySummary = {
  workspace: {
    id: string;
  };
};

test("launches Electron app with backend bridge", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const { backendInfo, electronApp, page } = await launchCocode(testInfo);

  try {
    expect(backendInfo.status).toBe("ready");
    expect(backendInfo.baseUrl).toMatch(/^http:\/\/127\.0\.0\.1:\d+$/);
    expect(backendInfo.authToken).toHaveLength(43);

    const sessionResponse = await fetch(`${backendInfo.baseUrl}/api/session`, {
      headers: { "X-Cocode-Token": backendInfo.authToken },
    });
    expect(sessionResponse.status).toBe(200);
    const sessionBody = await sessionResponse.json();
    expect(sessionBody.data.status).toBe("authenticated");
    await expect(
      page.getByRole("heading", { name: "Choose a project to get started" }),
    ).toBeVisible();

    const firstRepo = createBranchReviewRepo(
      testInfo.outputPath("smoke-repo-one"),
    );
    const secondRepo = createBranchReviewRepo(
      testInfo.outputPath("smoke-repo-two"),
    );
    await apiRequest<OpenRepositorySummary>(
      backendInfo,
      "/api/workspaces/open-repository",
      {
        method: "POST",
        body: { path: firstRepo },
      },
    );
    const secondOpen = await apiRequest<OpenRepositorySummary>(
      backendInfo,
      "/api/workspaces/open-repository",
      {
        method: "POST",
        body: { path: secondRepo },
      },
    );
    await page.reload();
    await page.getByRole("button", { name: /smoke-repo-one/ }).click();
    await expect(
      page.getByRole("button", { name: "Set up review" }),
    ).toHaveCount(1);

    let releaseSecondRepositoryLoad = () => undefined;
    const secondRepositoryLoadReleased = new Promise<void>((resolve) => {
      releaseSecondRepositoryLoad = resolve;
    });
    let shouldPauseSecondRepositoryLoad = true;
    const secondRepositoryRoute = async (route: Route) => {
      if (shouldPauseSecondRepositoryLoad) {
        shouldPauseSecondRepositoryLoad = false;
        await secondRepositoryLoadReleased;
      }
      await route.continue();
    };
    await page.route(
      `**/api/workspaces/${secondOpen.workspace.id}/repositories`,
      secondRepositoryRoute,
    );
    await page.getByRole("button", { name: /smoke-repo-two/ }).click();
    await expect(
      page.getByRole("heading", { name: "Loading project" }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Set up review" }),
    ).toHaveCount(0);
    releaseSecondRepositoryLoad();
    await expect(
      page.getByRole("heading", { name: "Set up review" }),
    ).toBeVisible();
    await page.unroute(
      `**/api/workspaces/${secondOpen.workspace.id}/repositories`,
      secondRepositoryRoute,
    );
    await expect(
      page.getByRole("button", { name: "Set up review" }),
    ).toHaveCount(2);

    await page.getByRole("button", { name: "Settings" }).click();
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await expect(
      page.getByRole("button", { name: /Kiro CLI Runs Kiro CLI/ }),
    ).toBeVisible();
  } finally {
    await electronApp.close();
  }
});
