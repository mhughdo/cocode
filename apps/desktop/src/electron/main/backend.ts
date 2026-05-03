import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { randomBytes } from "node:crypto";
import { existsSync } from "node:fs";
import { createServer } from "node:net";
import { basename, dirname, join, resolve } from "node:path";

import { app } from "electron";
import { is } from "@electron-toolkit/utils";

export interface BackendInfo {
  baseUrl: string;
  authToken: string;
  logPath: string;
  status: "starting" | "ready" | "stopped";
}

export class BackendController {
  private process: ChildProcessWithoutNullStreams | null = null;
  private info: BackendInfo | null = null;

  async start(): Promise<BackendInfo> {
    if (this.info?.status === "ready") {
      return this.info;
    }

    const port = await getAvailablePort();
    const token = randomBytes(32).toString("base64url");
    const baseUrl = `http://127.0.0.1:${port}`;
    const logPath = join(app.getPath("logs"), "cocoded.log");

    this.info = {
      baseUrl,
      authToken: token,
      logPath,
      status: "starting",
    };

    const child = this.spawnBackend(port, token, logPath);
    this.process = child;

    child.stderr.on("data", (chunk: Buffer) => {
      process.stderr.write(chunk);
    });
    child.stdout.on("data", (chunk: Buffer) => {
      process.stdout.write(chunk);
    });
    child.on("exit", () => {
      if (this.info) {
        this.info.status = "stopped";
      }
    });

    await waitForHealth(baseUrl);
    this.info.status = "ready";
    return this.info;
  }

  getInfo(): BackendInfo | null {
    return this.info;
  }

  stop(): void {
    if (this.process && !this.process.killed) {
      this.process.kill();
    }
    this.process = null;
    if (this.info) {
      this.info.status = "stopped";
    }
  }

  private spawnBackend(
    port: number,
    authToken: string,
    logPath: string,
  ): ChildProcessWithoutNullStreams {
    const env = {
      ...process.env,
      COCODED_ADDR: `127.0.0.1:${port}`,
      COCODED_AUTH_TOKEN: authToken,
      COCODED_LOG_PATH: logPath,
    };

    if (is.dev) {
      return spawn("go", ["run", "./cmd/cocoded"], {
        cwd: join(workspaceRoot(), "services/cocoded"),
        env,
      });
    }

    const binaryName = process.platform === "win32" ? "cocoded.exe" : "cocoded";
    const binaryPath = join(process.resourcesPath, binaryName);
    if (!existsSync(binaryPath)) {
      throw new Error(`Bundled cocoded binary not found at ${binaryPath}`);
    }
    return spawn(binaryPath, [], { env });
  }
}

function workspaceRoot(): string {
  const cwd = process.cwd();
  if (basename(cwd) === "desktop" && basename(dirname(cwd)) === "apps") {
    return resolve(cwd, "../..");
  }
  return cwd;
}

function getAvailablePort(): Promise<number> {
  return new Promise((resolvePort, reject) => {
    const server = createServer();
    server.on("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (!address || typeof address === "string") {
        server.close();
        reject(new Error("Could not allocate backend port"));
        return;
      }
      const port = address.port;
      server.close(() => resolvePort(port));
    });
  });
}

async function waitForHealth(baseUrl: string): Promise<void> {
  const deadline = Date.now() + 20_000;
  let lastError: unknown;

  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${baseUrl}/api/health`);
      if (response.ok) {
        return;
      }
      lastError = new Error(`Health check returned ${response.status}`);
    } catch (error) {
      lastError = error;
    }
    await sleep(250);
  }

  throw new Error(`Backend did not become ready: ${String(lastError)}`);
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolveSleep) => {
    setTimeout(resolveSleep, ms);
  });
}
