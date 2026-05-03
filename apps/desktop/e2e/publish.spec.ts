import { expect, test } from "@playwright/test";

import { launchCocode, seedCocodeData } from "./test-support";

test("copies selected packet and previews GitHub payload", async ({
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
    await page.getByRole("tab", { name: "Publish" }).click();

    await expect(
      page.getByText("Accepted findings", { exact: true }),
    ).toBeVisible();
    await expect(page.getByText("1 selected")).toBeVisible();
    await expect(
      page.getByText(
        "Repository settings updates miss the workspace admin guard.",
      ),
    ).toBeVisible();

    await page.getByRole("button", { name: "Copy packet" }).click();
    await expect(
      page.getByText("Copy packet copied to clipboard"),
    ).toBeVisible();
    await expect(page.getByText("1 findings")).toBeVisible();
    await expect(page.getByText("markdown", { exact: true })).toBeVisible();

    const clipboardText = await electronApp.evaluate(({ clipboard }) =>
      clipboard.readText(),
    );
    expect(clipboardText).toContain(
      "Repository settings updates miss the workspace admin guard.",
    );
    expect(clipboardText).toContain("apps/api/src/routes/repositories.ts");

    await page.getByRole("button", { name: "Preview" }).click();
    await expect(
      page.getByText("GitHub preview", { exact: true }),
    ).toBeVisible();
    await expect(page.getByText("Review body", { exact: true })).toBeVisible();
    await expect(page.getByText("Inline comments").first()).toBeVisible();
    await expect(page.getByText("Selected findings").first()).toBeVisible();
    await expect(page.getByText("Can publish inline").first()).toBeVisible();
    await expect(
      page.getByText("Can publish summary-only").first(),
    ).toBeVisible();
    await expect(
      page.getByText("apps/api/src/routes/repositories.ts").first(),
    ).toBeVisible();
  } finally {
    await electronApp.close();
  }
});
