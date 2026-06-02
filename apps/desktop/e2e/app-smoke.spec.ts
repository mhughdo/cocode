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
    await page.getByRole("button", { name: "Show right panel" }).click();
    const appRightPanel = page.getByRole("complementary", {
      name: "App right panel",
    });
    await expect(appRightPanel).toBeVisible();
    await appRightPanel
      .getByRole("button", { exact: true, name: "Files" })
      .click();
    await expect(
      appRightPanel.getByRole("button", { name: "src", expanded: false }),
    ).toBeVisible();
    await appRightPanel
      .getByRole("textbox", { name: "Filter files" })
      .fill("auth");
    await expect(
      appRightPanel.getByRole("button", { name: /auth\.ts/ }).first(),
    ).toBeVisible();
    await appRightPanel
      .getByRole("button", { name: /auth\.ts/ })
      .first()
      .click();
    await expect(
      appRightPanel.getByText("canUpdateRepository").first(),
    ).toBeVisible();
    await expect(
      appRightPanel.getByRole("button", { name: "Close auth.ts" }),
    ).toBeVisible();
    await appRightPanel.getByRole("button", { name: "Hide file tree" }).click();
    await expect(
      appRightPanel.getByRole("textbox", { name: "Filter files" }),
    ).toHaveCount(0);
    await appRightPanel.getByRole("button", { name: "Show file tree" }).click();
    await expect(
      appRightPanel.getByRole("textbox", { name: "Filter files" }),
    ).toBeVisible();
    await page
      .getByRole("button", { name: "Hide right panel" })
      .first()
      .click();

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

    await page.getByRole("button", { name: /Custom CLI Template for/ }).click();
    await page.getByRole("button", { name: "Advanced CLI settings" }).click();
    await expect(page.getByText("Prompt delivery")).toBeVisible();
    await expect(page.getByText("Project root")).toHaveCount(0);
    await page.getByLabel("Name").fill("E2E Fresh CLI");
    await page.getByLabel("Role").fill("e2e_fresh_reviewer");
    await page.getByRole("textbox", { name: "Command" }).fill("codex");
    const enabledSwitch = page.getByRole("switch", { name: "Enabled" });
    if (!(await enabledSwitch.isChecked())) {
      await enabledSwitch.click();
    }
    await page.getByRole("button", { name: "Save" }).click();
    await expect(
      page.getByRole("button", { name: /E2E Fresh CLI/ }),
    ).toBeVisible();

    await page
      .getByRole("main")
      .getByRole("button", { name: "New thread" })
      .last()
      .click();
    await expect(
      page.getByRole("heading", { name: "Set up review" }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Add agent" }).click();
    await expect(
      page.getByRole("menuitem", { name: /E2E Fresh CLI/ }),
    ).toBeVisible();
  } finally {
    await electronApp.close();
  }
});
