import { expect, test } from "@playwright/test";

import { launchCocode } from "./test-support";
import {
  createRealCliAgentConfig,
  ensureCommandAvailable,
  isExternalProviderLimit,
  selectedRealCliTargets,
} from "./real-cli-support";

const selectedTargets = selectedRealCliTargets("COCODE_E2E_REAL_CLIS");

if (selectedTargets.length === 0) {
  test("real CLI smoke checks are opt-in", async () => {
    test.skip(
      true,
      "Set COCODE_E2E_REAL_CLIS=codex,gemini,opencode,agy,claude or all to run real local CLI smoke checks.",
    );
  });
} else {
  for (const target of selectedTargets) {
    test(`runs ${target.id} through the Settings health check`, async ({
      browserName,
    }, testInfo) => {
      test.setTimeout(180_000);
      expect(browserName).toBe("chromium");
      ensureCommandAvailable(target.command);

      const { backendInfo, electronApp, page } = await launchCocode(testInfo);

      try {
        const config = await createRealCliAgentConfig(backendInfo, target, {
          purpose: "smoke",
          timeoutSeconds: 120,
        });

        await page.getByRole("button", { name: "Settings" }).click();
        await expect(
          page.getByRole("heading", { name: "Settings" }),
        ).toBeVisible();
        await page.getByRole("button", { name: config.name }).click();
        await page.getByRole("button", { name: "Test" }).click();

        await expect(
          page.getByText(
            /command smoke check (succeeded|failed|did not include expected output)/,
          ),
        ).toBeVisible({ timeout: 150_000 });

        const healthText = await page.locator("body").innerText();
        if (isExternalProviderLimit(healthText)) {
          testInfo.annotations.push({
            type: "external-provider-limit",
            description: `${target.command} returned a provider capacity or rate-limit response during the smoke check.`,
          });
          test.skip(
            true,
            `${target.command} provider capacity or rate limit blocked this smoke run.`,
          );
        }

        await expect(
          page.getByText("command smoke check succeeded"),
        ).toBeVisible();
        await expect(page.getByText(/^available$/).first()).toBeVisible();
      } finally {
        await electronApp.close();
      }
    });
  }
}
