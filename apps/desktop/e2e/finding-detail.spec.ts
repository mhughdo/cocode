import { expect, test } from "@playwright/test";

import { launchCocode, seedCocodeData } from "./test-support";

test("opens finding detail and Evidence Map from seeded data", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const seeded = seedCocodeData(testInfo);
  const { electronApp, page } = await launchCocode(
    testInfo,
    {},
    seeded.dataDir,
  );

  try {
    await page
      .getByRole("button", { name: /PR #42 - repository settings review/ })
      .click();
    await expect(
      page.getByText("PR #42 - repository settings review").first(),
    ).toBeVisible();

    await page.getByRole("tab", { name: "Findings" }).click();
    await expect(
      page.getByRole("heading", {
        name: "Repository settings updates miss the workspace admin guard.",
      }),
    ).toBeVisible();
    await expect(
      page.getByText("apps/api/src/routes/repositories.ts").first(),
    ).toBeVisible();
    await expect(
      page.getByRole("tab", { exact: true, name: "Evidence" }),
    ).toBeVisible();
    await page.getByRole("tab", { exact: true, name: "Evidence" }).click();
    await expect(
      page.getByText("Mutation route reaches settings write"),
    ).toBeVisible();

    await page.getByRole("button", { exact: true, name: "Map" }).click();
    await expect(page.getByRole("tab", { name: "Evidence Map" })).toBeVisible();
    await expect(page.getByText("Code hierarchy")).toBeVisible();
    await expect(
      page.getByRole("img", { name: "Evidence Map graph" }),
    ).toBeVisible();
    await expect(page.getByText("Call path")).toBeVisible();
    await expect(page.getByText("Selected context")).toBeVisible();
    await expect(page.getByText("PATCH repository settings")).toBeVisible();
  } finally {
    await electronApp.close();
  }
});
