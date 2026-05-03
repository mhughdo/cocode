import { _electron as electron, expect, test } from "@playwright/test";
import { mkdirSync } from "node:fs";
import { join, resolve } from "node:path";

type BackendInfo = {
  baseUrl: string;
  authToken: string;
  logPath: string;
  status: "starting" | "ready" | "stopped";
};

type CocodeBridge = {
  getBackendInfo: () => Promise<BackendInfo>;
};

test("launches Electron app with backend bridge", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const dataDir = testInfo.outputPath("cocode-data");
  mkdirSync(dataDir, { recursive: true });

  const electronApp = await electron.launch({
    args: [resolve("out/main/index.js")],
    cwd: resolve("."),
    env: {
      ...process.env,
      COCODED_DATA_DIR: dataDir,
      COCODED_DB_PATH: join(dataDir, "cocoded.sqlite"),
      COCODED_ARTIFACT_DIR: join(dataDir, "artifacts"),
    },
  });
  testInfo.attach("data-dir", {
    body: dataDir,
    contentType: "text/plain",
  });

  try {
    const page = await electronApp.firstWindow();
    await expect(page.getByText("cocode").first()).toBeVisible();

    const backendInfo = await page.evaluate(() => {
      const bridge = (window as Window & { cocode?: CocodeBridge }).cocode;
      if (!bridge) {
        throw new Error("cocode preload bridge is unavailable");
      }
      return bridge.getBackendInfo();
    });
    expect(backendInfo.status).toBe("ready");
    expect(backendInfo.baseUrl).toMatch(/^http:\/\/127\.0\.0\.1:\d+$/);
    expect(backendInfo.authToken).toHaveLength(43);

    const sessionResponse = await fetch(`${backendInfo.baseUrl}/api/session`, {
      headers: { "X-Cocode-Token": backendInfo.authToken },
    });
    expect(sessionResponse.status).toBe(200);
    const sessionBody = await sessionResponse.json();
    expect(sessionBody.data.status).toBe("authenticated");
  } finally {
    await electronApp.close();
  }
});
