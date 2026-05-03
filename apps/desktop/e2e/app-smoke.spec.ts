import { expect, test } from "@playwright/test";

import { launchCocode } from "./test-support";

test("launches Electron app with backend bridge", async ({
  browserName,
}, testInfo) => {
  expect(browserName).toBe("chromium");
  const { backendInfo, electronApp } = await launchCocode(testInfo);

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
  } finally {
    await electronApp.close();
  }
});
