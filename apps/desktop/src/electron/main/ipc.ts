import { app, clipboard, dialog, ipcMain, shell } from "electron";

import type { BackendController } from "./backend";
import { openExternalEditor, type OpenFileRequest } from "./editor";
import type { SecretStore } from "./secret-store";

const maxClipboardBytes = 1_000_000;
const maxGitHubTokenBytes = 20_000;
const githubTokenStorageKey = "github:default";

interface BackendEnvelope<T> {
  data: T | null;
  error: { message?: string; code?: string } | null;
}

interface GitHubSnapshotRequest {
  workspaceId: string;
  repositoryId: string;
  url: string;
}

interface SaveGitHubTokenRequest {
  token: string;
  displayName?: string;
}

interface DeleteGitHubCredentialResponse {
  deleted: boolean;
  storage_key?: string;
}

export function registerIpc(
  backend: BackendController,
  secretStore: SecretStore,
): void {
  ipcMain.handle("cocode:get-backend-info", () => {
    const info = backend.getInfo();
    if (!info) {
      throw new Error("Backend has not started");
    }
    return info;
  });

  ipcMain.handle("cocode:write-clipboard", (_event, text: unknown) => {
    if (typeof text !== "string") {
      throw new Error("Clipboard payload must be text");
    }
    if (Buffer.byteLength(text, "utf8") > maxClipboardBytes) {
      throw new Error("Clipboard payload is too large");
    }
    clipboard.writeText(text);
    return { ok: true };
  });

  ipcMain.handle("cocode:select-repository", async () => {
    const result = await dialog.showOpenDialog({
      properties: ["openDirectory"],
      title: "Select repository",
    });
    if (result.canceled || result.filePaths.length === 0) {
      return null;
    }
    return result.filePaths[0];
  });

  ipcMain.handle("cocode:open-file", async (_event, request: unknown) => {
    const parsed = parseOpenFileRequest(request);
    await openExternalEditor(parsed);
    return { ok: true };
  });

  ipcMain.handle("cocode:open-logs", async () => {
    const error = await shell.openPath(app.getPath("logs"));
    if (error) {
      throw new Error(error);
    }
    return { ok: true };
  });

  ipcMain.handle("cocode:get-github-credential", async () => {
    return backendRequest(backend, "/api/credentials/github");
  });

  ipcMain.handle(
    "cocode:save-github-token",
    async (_event, request: unknown) => {
      const parsed = parseSaveGitHubTokenRequest(request);
      await secretStore.set(githubTokenStorageKey, parsed.token);
      try {
        return await backendRequest(backend, "/api/credentials/github", {
          method: "POST",
          body: {
            display_name: parsed.displayName,
            storage_key: githubTokenStorageKey,
            token: parsed.token,
          },
        });
      } catch (error) {
        await secretStore.delete(githubTokenStorageKey).catch(() => undefined);
        throw error;
      }
    },
  );

  ipcMain.handle("cocode:delete-github-token", async () => {
    const response = await backendRequest<DeleteGitHubCredentialResponse>(
      backend,
      "/api/credentials/github",
      { method: "DELETE" },
    );
    await secretStore
      .delete(response.storage_key || githubTokenStorageKey)
      .catch(() => undefined);
    if (
      response.storage_key &&
      response.storage_key !== githubTokenStorageKey
    ) {
      await secretStore.delete(githubTokenStorageKey).catch(() => undefined);
    }
    return response;
  });

  ipcMain.handle(
    "cocode:create-github-snapshot",
    async (_event, request: unknown) => {
      const parsed = parseGitHubSnapshotRequest(request);
      const token = await secretStore.get(githubTokenStorageKey);
      if (!token) {
        throw new Error("GitHub token is not configured");
      }
      return backendRequest(backend, "/api/pr-snapshots/from-github-url", {
        method: "POST",
        body: {
          workspace_id: parsed.workspaceId,
          repository_id: parsed.repositoryId,
          url: parsed.url,
          github_token: token,
        },
      });
    },
  );
}

async function backendRequest<T>(
  backend: BackendController,
  path: string,
  options: { method?: string; body?: unknown } = {},
): Promise<T> {
  const info = backend.getInfo();
  if (!info) {
    throw new Error("Backend has not started");
  }
  const response = await fetch(`${info.baseUrl}${path}`, {
    method: options.method ?? "GET",
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${info.authToken}`,
      ...(options.body ? { "Content-Type": "application/json" } : {}),
    },
    body: options.body ? JSON.stringify(options.body) : undefined,
  });
  const envelope = (await response.json()) as BackendEnvelope<T>;
  if (!response.ok || envelope.error) {
    throw new Error(
      envelope.error?.message ||
        `Backend request failed with status ${response.status}`,
    );
  }
  if (envelope.data === null) {
    throw new Error("Backend response did not include data");
  }
  return envelope.data;
}

function parseSaveGitHubTokenRequest(request: unknown): SaveGitHubTokenRequest {
  if (!request || typeof request !== "object") {
    throw new Error("GitHub token request must be an object");
  }
  const value = request as Partial<SaveGitHubTokenRequest>;
  if (typeof value.token !== "string" || value.token.trim() === "") {
    throw new Error("GitHub token is required");
  }
  if (Buffer.byteLength(value.token, "utf8") > maxGitHubTokenBytes) {
    throw new Error("GitHub token is too large");
  }
  return {
    token: value.token.trim(),
    displayName:
      typeof value.displayName === "string" && value.displayName.trim()
        ? value.displayName.trim()
        : undefined,
  };
}

function parseGitHubSnapshotRequest(request: unknown): GitHubSnapshotRequest {
  if (!request || typeof request !== "object") {
    throw new Error("GitHub snapshot request must be an object");
  }
  const value = request as Partial<GitHubSnapshotRequest>;
  if (
    typeof value.workspaceId !== "string" ||
    value.workspaceId.trim() === ""
  ) {
    throw new Error("GitHub snapshot requires workspaceId");
  }
  if (
    typeof value.repositoryId !== "string" ||
    value.repositoryId.trim() === ""
  ) {
    throw new Error("GitHub snapshot requires repositoryId");
  }
  if (typeof value.url !== "string" || value.url.trim() === "") {
    throw new Error("GitHub snapshot requires url");
  }
  return {
    workspaceId: value.workspaceId.trim(),
    repositoryId: value.repositoryId.trim(),
    url: value.url.trim(),
  };
}

function parseOpenFileRequest(request: unknown): OpenFileRequest {
  if (!request || typeof request !== "object") {
    throw new Error("Open file request must be an object");
  }

  const maybeRequest = request as Partial<OpenFileRequest>;
  if (
    typeof maybeRequest.filePath !== "string" ||
    maybeRequest.filePath === ""
  ) {
    throw new Error("Open file request requires filePath");
  }
  if (
    maybeRequest.line !== undefined &&
    !isPositiveInteger(maybeRequest.line)
  ) {
    throw new Error("Open file request line must be a positive integer");
  }
  if (
    maybeRequest.column !== undefined &&
    !isPositiveInteger(maybeRequest.column)
  ) {
    throw new Error("Open file request column must be a positive integer");
  }

  return {
    filePath: maybeRequest.filePath,
    line: maybeRequest.line,
    column: maybeRequest.column,
  };
}

function isPositiveInteger(value: unknown): value is number {
  return Number.isInteger(value) && Number(value) > 0;
}
