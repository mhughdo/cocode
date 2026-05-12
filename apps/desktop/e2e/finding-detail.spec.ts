import { expect, test } from "@playwright/test";

import { closeCocode, launchCocode, seedCocodeData } from "./test-support";

test("opens finding detail and Evidence Map from seeded data", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const seeded = seedCocodeData(testInfo);
  const app = await launchCocode(testInfo, {}, seeded.dataDir);
  const { page } = app;

  try {
    await page
      .getByRole("button", { name: /PR #42 - repository settings review/ })
      .click();
    await expect(
      page.getByText("PR #42 - repository settings review").first(),
    ).toBeVisible();

    await page.getByRole("tab", { name: "Findings" }).click();
    await expect(page.getByLabel("Review findings board")).toBeVisible();
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

    await page.getByRole("tab", { exact: true, name: "Code" }).click();
    await expect(page.getByText("Changed code")).toBeVisible();
    await expect(page.getByText("requireWorkspaceAdmin")).toBeVisible();
    await expect(
      page.getByText("apps/api/src/routes/repositories.ts").first(),
    ).toBeVisible();

    await page
      .locator("button")
      .filter({
        hasText: "Repository settings updates miss the workspace admin guard.",
      })
      .first()
      .click();
    await expect(
      page.getByRole("heading", {
        name: "Repository settings updates miss the workspace admin guard.",
      }),
    ).toBeVisible();
    await expect(page.getByText("Changed file")).toBeVisible();
    await expect(page.getByText("Finding thread")).toBeVisible();

    await page
      .getByRole("button", { exact: true, name: "Evidence map" })
      .click();
    await expect(
      page.getByRole("heading", { name: "Evidence Map" }),
    ).toBeVisible();
    await expect(page.getByText("Evidence flow")).toBeVisible();
    await expect(page.getByText("Code hierarchy")).toBeVisible();
    await expect(page.getByText("Changed code").nth(1)).toBeVisible();
    await expect(
      page.getByText("Finding claim", { exact: true }),
    ).toBeVisible();
    await expect(page.getByText("Evidence checks")).toBeVisible();
    await expect(
      page.getByText("Call path", { exact: true }).first(),
    ).toBeVisible();
    await expect(page.getByText("Selected context")).toBeVisible();
    await expect(
      page.getByText("Mutation route reaches settings write").first(),
    ).toBeVisible();
  } finally {
    await closeCocode(app);
  }
});
