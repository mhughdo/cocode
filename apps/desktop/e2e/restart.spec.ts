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
      .getByRole("button", { name: /PR #42 - repository settings review/ })
      .click();
    await firstLaunch.page.getByRole("tab", { name: "Findings" }).click();
    await expect(
      firstLaunch.page.getByRole("heading", {
        name: "Repository settings updates miss the workspace admin guard.",
      }),
    ).toBeVisible();
    await firstLaunch.page
      .getByRole("tab", { exact: true, name: "Evidence" })
      .click();
    await expect(
      firstLaunch.page.getByText("Mutation route reaches settings write"),
    ).toBeVisible();
    await firstLaunch.page.getByRole("tab", { name: "Publish" }).click();
    await expect(firstLaunch.page.getByText("1 selected")).toBeVisible();
  } finally {
    await firstLaunch.electronApp.close();
  }

  const secondLaunch = await launchCocode(testInfo, {}, seeded.dataDir);
  try {
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
      secondLaunch.page.getByRole("heading", {
        name: "Repository settings updates miss the workspace admin guard.",
      }),
    ).toBeVisible();
    await secondLaunch.page
      .getByRole("tab", { exact: true, name: "Evidence" })
      .click();
    await expect(
      secondLaunch.page.getByText("Mutation route reaches settings write"),
    ).toBeVisible();

    await secondLaunch.page.getByRole("tab", { name: "Publish" }).click();
    await expect(secondLaunch.page.getByText("1 selected")).toBeVisible();
  } finally {
    await secondLaunch.electronApp.close();
  }
});
