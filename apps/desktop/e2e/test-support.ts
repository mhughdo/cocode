import { _electron as electron, expect, type Page } from "@playwright/test";
import type { ElectronApplication, TestInfo } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { chmodSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";
import type { AddressInfo } from "node:net";
import { dirname, join, resolve } from "node:path";

type BackendInfo = {
  baseUrl: string;
  authToken: string;
  logPath: string;
  status: "starting" | "ready" | "stopped";
};

type CocodeBridge = {
  getBackendInfo: () => Promise<BackendInfo>;
  saveGitHubToken: (request: {
    token: string;
    displayName?: string;
  }) => Promise<unknown>;
};

type ApiEnvelope<T> = {
  data: T | null;
  error: { message?: string } | null;
};

type AgentConfig = {
  id: string;
};

export type CocodeApp = {
  backendInfo: BackendInfo;
  dataDir: string;
  electronApp: ElectronApplication;
  page: Page;
};

export type SeededCocodeData = {
  dataDir: string;
  workspaceRoot: string;
};

export type FakeGitHubServer = {
  url: string;
  close: () => Promise<void>;
};

export async function launchCocode(
  testInfo: TestInfo,
  env: Record<string, string> = {},
  dataDir = testInfo.outputPath("cocode-data"),
): Promise<CocodeApp> {
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

export function seedCocodeData(testInfo: TestInfo): SeededCocodeData {
  const dataDir = testInfo.outputPath("seeded-cocode-data");
  const workspaceRoot = testInfo.outputPath("seed-workspace");
  rmSync(dataDir, { recursive: true, force: true });
  mkdirSync(dataDir, { recursive: true });
  mkdirSync(workspaceRoot, { recursive: true });
  execFileSync(
    "go",
    [
      "run",
      "./cmd/cocode-db-seed",
      "-db",
      join(dataDir, "cocoded.sqlite"),
      "-artifacts",
      join(dataDir, "artifacts"),
      "-workspace-root",
      workspaceRoot,
      "-quiet",
    ],
    {
      cwd: resolve("../..", "services/cocoded"),
      env: process.env,
      stdio: "pipe",
    },
  );
  return { dataDir, workspaceRoot };
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

export async function saveGitHubToken(page: Page): Promise<void> {
  await page.evaluate(async () => {
    const bridge = (window as Window & { cocode?: CocodeBridge }).cocode;
    if (!bridge) {
      throw new Error("cocode preload bridge is unavailable");
    }
    await bridge.saveGitHubToken({
      token: "ghp_e2e_test",
      displayName: "E2E GitHub",
    });
  });
}

export async function startFakeGitHubServer(): Promise<FakeGitHubServer> {
  const server = createServer(handleFakeGitHubRequest);
  await new Promise<void>((resolveListen) => {
    server.listen(0, "127.0.0.1", resolveListen);
  });
  const address = server.address() as AddressInfo;
  return {
    url: `http://127.0.0.1:${address.port}`,
    close: () =>
      new Promise<void>((resolveClose, rejectClose) => {
        server.close((error) => {
          if (error) {
            rejectClose(error);
            return;
          }
          resolveClose();
        });
      }),
  };
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

export function createFakeReviewAgent(commandPath: string): string {
  mkdirSync(dirname(commandPath), { recursive: true });
  writeFileSync(
    commandPath,
    `#!/bin/sh
set -eu

if [ "\${1:-}" = "--version" ]; then
  printf 'cocode-e2e-fake-agent 0.1.0\\n'
  exit 0
fi

/bin/cat >/dev/null || true

cat <<'JSON'
{
  "summary": "Found one deterministic branch comparison issue.",
  "findings": [
    {
      "claim": "Repository update permissions now allow members to mutate settings.",
      "category": "security",
      "severity": "high",
      "confidence": 0.91,
      "locations": [
        {
          "path": "src/auth.ts",
          "start_line": 2,
          "end_line": 2,
          "side": "RIGHT"
        }
      ],
      "evidence": [
        {
          "title": "Branch change permits member writes",
          "summary": "The changed permission predicate now returns true for member roles.",
          "path": "src/auth.ts",
          "start_line": 2,
          "end_line": 2
        }
      ],
      "counter_evidence_request": "Show a later admin-only guard before repository settings mutation.",
      "suggested_fix": "Keep repository settings updates restricted to admin roles and add a member-denied regression test.",
      "draft_comment": "Please keep repository settings updates admin-only."
    }
  ]
}
JSON
`,
  );
  chmodSync(commandPath, 0o755);
  return commandPath;
}

export async function createFakeAgentConfig(
  backendInfo: BackendInfo,
  commandPath: string,
): Promise<AgentConfig> {
  return apiRequest<AgentConfig>(backendInfo, "/api/agents/configs", {
    method: "POST",
    body: {
      name: "E2E Fake Reviewer",
      role: "primary_reviewer",
      adapter_kind: "cli_noninteractive",
      command: commandPath,
      args: [],
      cwd_mode: "repo_root",
      env_allowlist: [],
      output_mode: "json",
      model_label: "deterministic",
      reasoning_label: "fixture",
      capabilities: {
        supports_json: true,
        supports_streaming: false,
        supports_sessions: false,
        can_read: true,
        can_write: false,
        can_cancel: true,
        output_modes: ["json", "text"],
      },
      settings: {
        prompt_delivery: "stdin",
        version_args: ["--version"],
      },
      enabled: true,
    },
  });
}

async function apiRequest<T>(
  backendInfo: BackendInfo,
  path: string,
  options: { method?: "GET" | "POST"; body?: unknown } = {},
): Promise<T> {
  const response = await fetch(`${backendInfo.baseUrl}${path}`, {
    method: options.method ?? "GET",
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${backendInfo.authToken}`,
      ...(options.body ? { "Content-Type": "application/json" } : {}),
    },
    body: options.body ? JSON.stringify(options.body) : undefined,
  });
  const envelope = (await response.json()) as ApiEnvelope<T>;
  if (!response.ok || envelope.error) {
    throw new Error(
      envelope.error?.message ??
        `Backend request failed with status ${response.status}`,
    );
  }
  if (envelope.data === null) {
    throw new Error("Backend response did not include data");
  }
  return envelope.data;
}

function handleFakeGitHubRequest(
  request: IncomingMessage,
  response: ServerResponse,
): void {
  if (!request.headers.authorization?.startsWith("Bearer ")) {
    writeFakeGitHubJSON(response, 401, { message: "bad credentials" });
    return;
  }

  const requestURL = new URL(request.url ?? "/", "http://127.0.0.1");
  if (request.method === "GET" && requestURL.pathname === "/user") {
    response.setHeader("X-OAuth-Scopes", "repo, read:user");
    writeFakeGitHubJSON(response, 200, { login: "octocat" });
    return;
  }

  const expectedPullPath = "/repos/octo-org/hello-world/pulls/42";
  if (request.method === "GET" && requestURL.pathname === expectedPullPath) {
    if (request.headers.accept?.includes("application/vnd.github.diff")) {
      response.writeHead(200, {
        "Content-Type": "text/plain; charset=utf-8",
      });
      response.end(
        [
          "diff --git a/apps/api/src/routes/repositories.ts b/apps/api/src/routes/repositories.ts",
          "--- a/apps/api/src/routes/repositories.ts",
          "+++ b/apps/api/src/routes/repositories.ts",
          "@@ -87,3 +87,4 @@ function updateRepositorySettings() {",
          "-  return updateSettings()",
          "+  requireWorkspaceAdmin()",
          "+  return updateSettings()",
          " }",
          "",
        ].join("\n"),
      );
      return;
    }
    writeFakeGitHubJSON(response, 200, {
      title: "Tighten repository auth",
      html_url: "https://github.com/octo-org/hello-world/pull/42",
      user: { login: "mona" },
      base: { ref: "main", sha: "base-sha" },
      head: { ref: "feature/auth", sha: "head-sha" },
    });
    return;
  }

  if (
    request.method === "GET" &&
    requestURL.pathname === `${expectedPullPath}/files`
  ) {
    writeFakeGitHubJSON(response, 200, [
      {
        sha: "file-sha",
        filename: "apps/api/src/routes/repositories.ts",
        status: "modified",
        additions: 2,
        deletions: 1,
        changes: 3,
        patch: "@@ -87,3 +87,4 @@\n-old\n+new\n+guard\n",
      },
    ]);
    return;
  }

  if (
    request.method === "GET" &&
    (requestURL.pathname === "/repos/octo-org/hello-world/issues/42/comments" ||
      requestURL.pathname === `${expectedPullPath}/comments` ||
      requestURL.pathname === `${expectedPullPath}/reviews`)
  ) {
    writeFakeGitHubJSON(response, 200, []);
    return;
  }

  writeFakeGitHubJSON(response, 404, { message: "not found" });
}

function writeFakeGitHubJSON(
  response: ServerResponse,
  status: number,
  body: unknown,
): void {
  response.writeHead(status, {
    "Content-Type": "application/json",
  });
  response.end(JSON.stringify(body));
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
