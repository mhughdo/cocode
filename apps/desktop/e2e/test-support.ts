import { _electron as electron, expect, type Page } from "@playwright/test";
import type { ElectronApplication, TestInfo } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
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

export type CocodeApp = {
  backendInfo: BackendInfo;
  dataDir: string;
  electronApp: ElectronApplication;
  page: Page;
};

export async function launchCocode(
  testInfo: TestInfo,
  env: Record<string, string> = {},
): Promise<CocodeApp> {
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
      ...env,
    },
  });
  testInfo.attach("data-dir", {
    body: dataDir,
    contentType: "text/plain",
  });

  const page = await electronApp.firstWindow();
  await expect(page.getByText("cocode").first()).toBeVisible();

  return {
    backendInfo: await getBackendInfo(page),
    dataDir,
    electronApp,
    page,
  };
}

export async function getBackendInfo(page: Page): Promise<BackendInfo> {
  return page.evaluate(() => {
    const bridge = (window as Window & { cocode?: CocodeBridge }).cocode;
    if (!bridge) {
      throw new Error("cocode preload bridge is unavailable");
    }
    return bridge.getBackendInfo();
  });
}

export function createBranchReviewRepo(repoPath: string): string {
  mkdirSync(repoPath, { recursive: true });
  runGit(repoPath, "init");
  runGit(repoPath, "checkout", "-B", "main");
  runGit(repoPath, "config", "user.email", "cocode@example.test");
  runGit(repoPath, "config", "user.name", "cocode E2E");
  runGit(repoPath, "config", "commit.gpgsign", "false");

  const sourceDir = join(repoPath, "src");
  mkdirSync(sourceDir, { recursive: true });
  writeFileSync(
    join(sourceDir, "auth.ts"),
    [
      "export function canUpdateRepository(role: string): boolean {",
      '  return role === "admin";',
      "}",
      "",
    ].join("\n"),
  );
  runGit(repoPath, "add", "src/auth.ts");
  runGit(repoPath, "commit", "-m", "initial auth guard");

  runGit(repoPath, "checkout", "-B", "feature/review-auth");
  writeFileSync(
    join(sourceDir, "auth.ts"),
    [
      "export function canUpdateRepository(role: string): boolean {",
      '  return role === "admin" || role === "member";',
      "}",
      "",
    ].join("\n"),
  );
  runGit(repoPath, "add", "src/auth.ts");
  runGit(repoPath, "commit", "-m", "loosen repository update guard");

  return repoPath;
}

function runGit(cwd: string, ...args: string[]): void {
  execFileSync("git", args, {
    cwd,
    env: {
      ...process.env,
      GIT_CONFIG_NOSYSTEM: "1",
    },
    stdio: "pipe",
  });
}
