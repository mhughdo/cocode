import { expect, test, type Locator, type Page } from "@playwright/test";

import {
  apiRequest,
  closeCocode,
  launchCocode,
  seedCocodeData,
} from "./test-support";

type FindingDetailResponse = {
  finding: {
    decision_status: string;
  };
};

test("opens finding detail and Evidence Map from seeded data", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const seeded = seedCocodeData(testInfo);
  const app = await launchCocode(testInfo, {}, seeded.dataDir);
  const { backendInfo, page } = app;

  try {
    await page.setViewportSize({ width: 1600, height: 980 });
    await page
      .getByRole("button", { name: /PR #42 - repository settings review/ })
      .click();
    await expect(
      page.getByText("PR #42 - repository settings review").first(),
    ).toBeVisible();
    await expect(page.getByText("Ana Lee")).toHaveCount(0);
    await expect(page.getByText("Backend ready")).toHaveCount(0);

    await page.getByRole("tab", { name: "Findings" }).click();
    const findingsBoard = page.getByLabel("Review findings board");
    await expect(findingsBoard).toBeVisible();
    await expectSurfaceFillsViewport(page, findingsBoard);
    await expect(
      page
        .locator('[data-testid^="finding-row-"]')
        .filter({
          hasText:
            "Repository settings updates miss the workspace admin guard.",
        })
        .first(),
    ).toBeVisible();
    await expect(
      page.getByText("apps/api/src/routes/repositories.ts").first(),
    ).toBeVisible();

    await expect(page.getByText("Evidence story")).toHaveCount(0);
    const findingRow = page
      .locator('[data-testid^="finding-row-"]')
      .filter({
        hasText: "Repository settings updates miss the workspace admin guard.",
      })
      .first();
    await findingRow.click({ position: { x: 12, y: 12 } });
    await expect(page.getByText("Evidence story")).toBeVisible();
    await expect(page.getByText("Actions", { exact: true })).toBeVisible();
    await expect(page.getByText("Draft GitHub comment")).toBeVisible();
    await expect(page.getByText("Dismissal", { exact: true })).toHaveCount(0);
    const actionsSection = page
      .locator("section")
      .filter({ has: page.getByText("Actions", { exact: true }) })
      .first();
    const acceptBox = await actionsSection
      .getByRole("button", { exact: true, name: "Accept" })
      .boundingBox();
    const dismissBox = await actionsSection
      .getByRole("button", { exact: true, name: "Dismiss" })
      .boundingBox();
    expect(acceptBox).toBeTruthy();
    expect(dismissBox).toBeTruthy();
    expect(Math.abs(acceptBox!.y - dismissBox!.y)).toBeLessThan(8);
    await actionsSection
      .getByRole("button", { name: "Copy fix packet" })
      .click();
    await expect(
      actionsSection.locator("div").filter({ hasText: /^Copied$/ }),
    ).toBeVisible();
    const detailAfterCopy = await apiRequest<FindingDetailResponse>(
      backendInfo,
      "/api/findings/seed_finding_auth_guard",
    );
    expect(detailAfterCopy.finding.decision_status).toBe("accepted");
    await expectResizableRightPanel(page);

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
    await expect(page.getByText("Issue location")).toBeVisible();
    await expect(page.getByText("Evidence story")).toBeVisible();
    await expect(page.getByText("Primary code").last()).toBeVisible();
    await expect(page.getByText("Draft GitHub comment")).toBeVisible();
    await expect(page.getByText("Finding thread")).toBeVisible();
    const detailBreadcrumb = page.getByRole("navigation", {
      name: "Review breadcrumb",
    });
    await expect(
      detailBreadcrumb.getByRole("button", { name: "Findings" }),
    ).toBeVisible();
    await expect(
      detailBreadcrumb.getByText(
        "Repository settings updates miss the workspace admin guard.",
      ),
    ).toBeVisible();
    await expectResizableRightPanel(page);

    await page
      .getByRole("button", { exact: true, name: "Evidence map" })
      .first()
      .click();
    await expect(
      page.getByRole("heading", { name: "Evidence Map" }),
    ).toBeVisible();
    await expect(page.getByText("Evidence flow")).toBeVisible();
    await expect(page.getByText("Source details")).toBeVisible();
    await expect(page.getByText("Why this matters")).toBeVisible();
    await expect(page.getByText("Code hierarchy")).toHaveCount(0);
    await expect(page.getByText("Selected context")).toHaveCount(0);
    await expect(page.getByText("Evidence bundle")).toHaveCount(0);
    await expectSurfaceFillsViewport(page, page.locator(".evidence-map-layout"));
    await expectSurfaceFillsViewport(page, page.locator(".evidence-map-canvas"));
    const mapBreadcrumb = page.getByRole("navigation", {
      name: "Review breadcrumb",
    });
    await expect(
      mapBreadcrumb.getByRole("button", { name: "Findings" }),
    ).toBeVisible();
    await expect(
      mapBreadcrumb.getByRole("button", {
        name: "Repository settings updates miss the workspace admin guard.",
      }),
    ).toBeVisible();
    await expect(mapBreadcrumb.getByText("Evidence map")).toBeVisible();

    await page
      .locator(
        'svg[aria-label="Evidence Map graph"] g[transform][role="button"]',
      )
      .first()
      .click();
    await expect(page.getByText("Selected location")).toBeVisible();
    await expect(
      page.getByText("Source file", { exact: true }),
    ).toBeVisible();
    await expect(page.getByText("router.patch").first()).toBeVisible();
    await expect(page.getByText("full file").first()).toBeVisible();
    await expect(
      page.getByRole("button", { name: /Open in editor/ }),
    ).toHaveCount(0);
    await expect(
      page.getByText("Mutation route reaches settings write").first(),
    ).toBeVisible();
    await mapBreadcrumb
      .getByRole("button", {
        name: "Repository settings updates miss the workspace admin guard.",
      })
      .click();
    await expect(
      page.getByRole("heading", {
        name: "Repository settings updates miss the workspace admin guard.",
      }),
    ).toBeVisible();
  } finally {
    await closeCocode(app);
  }
});

async function expectSurfaceFillsViewport(
  page: Page,
  surface: Locator,
) {
  const box = await surface.boundingBox();
  const viewport = page.viewportSize();
  expect(box).toBeTruthy();
  expect(viewport).toBeTruthy();
  expect(box!.height).toBeGreaterThan(viewport!.height * 0.62);
}

async function expectResizableRightPanel(page: Page) {
  const handle = page.getByLabel("Resize right panel").first();
  await expect(handle).toBeVisible();
  const before = await handle.boundingBox();
  expect(before).toBeTruthy();
  await page.mouse.move(before!.x + before!.width / 2, before!.y + 48);
  await page.mouse.down();
  await page.mouse.move(before!.x - 120, before!.y + 48);
  await page.mouse.up();
  const after = await handle.boundingBox();
  expect(after).toBeTruthy();
  expect(after!.x).toBeLessThan(before!.x - 48);
}
