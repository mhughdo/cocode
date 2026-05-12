import { expect, test, type Page } from "@playwright/test";
import { writeFileSync } from "node:fs";
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

async function waitForReviewAgentOption(page: Page, name: RegExp) {
  const addAgentButton = page.getByRole("button", { name: "Add agent" });
  await expect(addAgentButton).toBeVisible();
  for (let attempt = 0; attempt < 6; attempt += 1) {
    await addAgentButton.click();
    const option = page.getByRole("menuitem", { name });
    if (
      await option.waitFor({ state: "visible", timeout: 1_000 }).then(
        () => true,
        () => false,
      )
    ) {
      await page.keyboard.press("Escape");
      return;
    }
    await page.keyboard.press("Escape");
    await page.waitForTimeout(500);
  }
  await addAgentButton.click();
  await expect(page.getByRole("menuitem", { name })).toBeVisible();
  await page.keyboard.press("Escape");
}

async function setPrimaryPreset(page: Page, name: string, selected: boolean) {
  const preset = page.getByRole("button", { name, exact: true });
  if (((await preset.getAttribute("aria-pressed")) === "true") !== selected) {
    await preset.click();
  }
}

async function expectAgentCount(page: Page, count: number) {
  await expect(
    page.getByRole("button", {
      name: `${count} agents selected`,
      exact: true,
    }),
  ).toBeVisible();
}

async function clearPrimaryPresets(page: Page) {
  await setPrimaryPreset(page, primaryPresetButtons.standard, false);
  await setPrimaryPreset(page, primaryPresetButtons.security, false);
  await setPrimaryPreset(page, primaryPresetButtons.performance, false);
  await setPrimaryPreset(page, primaryPresetButtons.sql, false);
}

async function toggleMorePreset(page: Page, query: string, name: RegExp) {
  if ((await page.getByPlaceholder("Search presets...").count()) === 0) {
    await page.getByRole("button", { name: "More presets" }).click();
  }
  await page.getByPlaceholder("Search presets...").fill(query);
  await page.getByRole("menuitemcheckbox", { name }).click();
}

test("opens a local repository and configures a branch comparison", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const repoPath = createBranchReviewRepo(testInfo.outputPath("branch-repo"));
  const app = await launchCocode(testInfo, {
    COCODE_E2E_REPOSITORY_PATH: repoPath,
  });
  const { page } = app;

  try {
    await openSeededProject(page);
    await expect(page.getByText("branch-repo").first()).toBeVisible();

    await page.getByRole("button", { name: /Compare branches/ }).click();
    await page.getByRole("button", { name: /Base branch/ }).click();
    await page.getByLabel("Search base branch").fill("main");
    await page.getByRole("menuitem", { name: /main/ }).click();
    await page.getByRole("button", { name: /Head branch/ }).click();
    await page.getByLabel("Search head branch").fill("feature");
    await page.getByRole("menuitem", { name: /feature\/review-auth/ }).click();

    await expect(page.getByText("Review source")).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Source details" }),
    ).toHaveCount(0);
    await page.getByRole("button", { name: "Show source details" }).click();
    await expect(
      page.getByRole("heading", { name: "Source details" }),
    ).toBeVisible();
    await expect(
      page.getByText("main..feature/review-auth").first(),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: /Base branch: main/ }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Start review" }),
    ).toBeVisible();
    await expect(
      page.getByText("Changed files", { exact: true }),
    ).toBeVisible();
    await expect(page.getByText("src/auth.ts").first()).toBeVisible();
  } finally {
    await closeCocode(app);
  }
});

test("loads local changes source details and collapses file diffs", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const repoPath = createBranchReviewRepo(testInfo.outputPath("local-repo"));
  writeFileSync(
    join(repoPath, "src/auth.ts"),
    [
      "export function canUpdateRepository(role: string): boolean {",
      '  return role === "admin" || role === "member" || role === "viewer";',
      "}",
      "",
      "export function canReadRepository(role: string): boolean {",
      '  return role === "admin" || role === "member" || role === "viewer";',
      "}",
      "",
      "export const permissionMatrix = [];",
      "",
    ].join("\n"),
  );
  const app = await launchCocode(testInfo, {
    COCODE_E2E_REPOSITORY_PATH: repoPath,
  });
  const { page } = app;

  try {
    await openSeededProject(page);
    await page.getByRole("button", { name: /Local changes/ }).click();
    await page.getByRole("button", { name: "Show source details" }).click();
    await expect(page.getByText("src/auth.ts").first()).toBeVisible();
    await expect(page.getByTestId("setup-source-stack-scroll")).toBeVisible();
    const fileToggle = page
      .getByRole("button", { name: /src\/auth\.ts/ })
      .first();
    await expect(fileToggle).toHaveAttribute("aria-expanded", "true");
    await fileToggle.click();
    await expect(fileToggle).toHaveAttribute("aria-expanded", "false");
    await expect(page.getByTestId("setup-diff-scroll")).toHaveCount(0);
    await fileToggle.click();
    await expect(fileToggle).toHaveAttribute("aria-expanded", "true");
  } finally {
    await closeCocode(app);
  }
});

test("can manually add a review agent without selecting presets", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const repoPath = createBranchReviewRepo(testInfo.outputPath("manual-repo"));
  const fakeAgentPath = createFakeReviewAgent(
    testInfo.outputPath("bin/fake-review-agent"),
  );
  const app = await launchCocode(testInfo, {
    COCODE_E2E_REPOSITORY_PATH: repoPath,
  });
  const { backendInfo, page } = app;

  try {
    await createFakeAgentConfig(backendInfo, fakeAgentPath);
    await page.reload();
    await openSeededProject(page);
    await waitForReviewAgentOption(page, /E2E Fake Reviewer/);
    await clearPrimaryPresets(page);

    await expect(
      page.getByText("0 selected from 10 built-in presets."),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Add agent" })).toBeVisible();
    await page.getByRole("button", { name: "Add agent" }).click();
    await expect(page.getByText("Add review agent")).toBeVisible();
    await page.getByRole("menuitem", { name: /E2E Fake Reviewer/ }).click();

    await expect(
      page.getByRole("button", { name: /agents selected/ }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "General", exact: true }),
    ).toBeVisible();
  } finally {
    await closeCocode(app);
  }
});

test("updates generated review agents when presets are toggled", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const repoPath = createBranchReviewRepo(testInfo.outputPath("preset-repo"));
  const fakeAgentPath = createFakeReviewAgent(
    testInfo.outputPath("bin/fake-review-agent"),
  );
  const app = await launchCocode(testInfo, {
    COCODE_E2E_REPOSITORY_PATH: repoPath,
  });
  const { backendInfo, page } = app;

  try {
    await createFakeAgentConfig(backendInfo, fakeAgentPath);
    await page.reload();
    await openSeededProject(page);
    await waitForReviewAgentOption(page, /E2E Fake Reviewer/);

    await expect(
      page.getByText("0 selected from 10 built-in presets."),
    ).toBeVisible();
    await expectAgentCount(page, 1);

    await clearPrimaryPresets(page);
    await expect(
      page.getByText("0 selected from 10 built-in presets."),
    ).toBeVisible();
    await expectAgentCount(page, 1);

    await setPrimaryPreset(page, primaryPresetButtons.sql, true);
    await expect(
      page.getByText("1 selected from 10 built-in presets."),
    ).toBeVisible();
    await expectAgentCount(page, 5);
    await expect(
      page.getByRole("button", { name: "SQL review", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "General", exact: true }),
    ).toBeVisible();

    await page
      .getByRole("button", { name: /^Remove / })
      .first()
      .click();
    await expectAgentCount(page, 4);

    await setPrimaryPreset(page, primaryPresetButtons.sql, false);
    await expectAgentCount(page, 1);
    await setPrimaryPreset(page, primaryPresetButtons.sql, true);
    await expectAgentCount(page, 5);

    await setPrimaryPreset(page, primaryPresetButtons.security, true);
    await expect(
      page.getByText("2 selected from 10 built-in presets."),
    ).toBeVisible();
    await expectAgentCount(page, 8);
    await expect(
      page.getByRole("button", { name: "Security", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "AuthZ", exact: true }),
    ).toBeVisible();

    await setPrimaryPreset(page, primaryPresetButtons.performance, true);
    await expectAgentCount(page, 10);
    await setPrimaryPreset(page, primaryPresetButtons.security, false);
    await expectAgentCount(page, 7);
  } finally {
    await closeCocode(app);
  }
});

test("searches and toggles secondary presets from more presets", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const repoPath = createBranchReviewRepo(testInfo.outputPath("more-repo"));
  const fakeAgentPath = createFakeReviewAgent(
    testInfo.outputPath("bin/fake-review-agent"),
  );
  const app = await launchCocode(testInfo, {
    COCODE_E2E_REPOSITORY_PATH: repoPath,
  });
  const { backendInfo, page } = app;

  try {
    await createFakeAgentConfig(backendInfo, fakeAgentPath);
    await page.reload();
    await openSeededProject(page);
    await waitForReviewAgentOption(page, /E2E Fake Reviewer/);
    await expectAgentCount(page, 1);
    await clearPrimaryPresets(page);
    await expectAgentCount(page, 1);

    await toggleMorePreset(page, "privacy", /Privacy/);

    await expect(
      page.getByText("1 selected from 10 built-in presets."),
    ).toBeVisible();
    await page.getByRole("button", { name: "More presets" }).click();
    await page.getByPlaceholder("Search presets...").fill("privacy");
    await expect(
      page.getByRole("menuitemcheckbox", { name: /Privacy/ }),
    ).toHaveAttribute("aria-checked", "true");
    await page.keyboard.press("Escape");
    await expectAgentCount(page, 6);

    await toggleMorePreset(page, "privacy", /Privacy/);

    await expect(
      page.getByText("0 selected from 10 built-in presets."),
    ).toBeVisible();
    await expectAgentCount(page, 1);
  } finally {
    await closeCocode(app);
  }
});
