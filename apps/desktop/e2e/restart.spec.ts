import { expect, test } from "@playwright/test";

import { launchCocode, seedCocodeData } from "./test-support";

test("reloads persisted findings and decisions after app restart", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const seeded = seedCocodeData(testInfo);
  const firstLaunch = await launchCocode(testInfo, {}, seeded.dataDir);

  try {
    await firstLaunch.page
      .getByRole("button", { name: /cocode Demo/ })
      .click();
    await firstLaunch.page
      .getByRole("button", { name: /PR #42 - repository settings review/ })
      .click();
    await firstLaunch.page.getByRole("tab", { name: "Findings" }).click();
    await expect(
      firstLaunch.page.getByRole("button", {
        name: "Repository settings updates miss the workspace admin guard.",
        exact: true,
      }),
    ).toBeVisible();
    await firstLaunch.page
      .locator('[data-testid^="finding-row-"]')
      .filter({
        hasText: "Repository settings updates miss the workspace admin guard.",
      })
      .first()
      .click({ position: { x: 12, y: 12 } });
    await expect(firstLaunch.page.getByText("Evidence story")).toBeVisible();
    await firstLaunch.page.getByRole("tab", { name: "Publish" }).click();
    await expect(firstLaunch.page.getByText("1 selected")).toBeVisible();
  } finally {
    await firstLaunch.electronApp.close();
  }

  const secondLaunch = await launchCocode(testInfo, {}, seeded.dataDir);
  try {
    await secondLaunch.page
      .getByRole("button", { name: /cocode Demo/ })
      .click();
    await secondLaunch.page
      .getByRole("button", { name: /PR #42 - repository settings review/ })
      .click();
    await expect(
      secondLaunch.page
        .getByText("PR #42 - repository settings review")
        .first(),
    ).toBeVisible();

    await secondLaunch.page.getByRole("tab", { name: "Findings" }).click();
    await expect(
      secondLaunch.page.getByRole("button", {
        name: "Repository settings updates miss the workspace admin guard.",
        exact: true,
      }),
    ).toBeVisible();
    await secondLaunch.page
      .locator('[data-testid^="finding-row-"]')
      .filter({
        hasText: "Repository settings updates miss the workspace admin guard.",
      })
      .first()
      .click({ position: { x: 12, y: 12 } });
    await expect(secondLaunch.page.getByText("Evidence story")).toBeVisible();

    await secondLaunch.page.getByRole("tab", { name: "Publish" }).click();
    await expect(secondLaunch.page.getByText("1 selected")).toBeVisible();
  } finally {
    await secondLaunch.electronApp.close();
  }
});
