import { app, clipboard, dialog, ipcMain, shell } from "electron";

import type { BackendController } from "./backend";

const maxClipboardBytes = 1_000_000;

export interface OpenFileRequest {
  filePath: string;
  line?: number;
  column?: number;
}

export function registerIpc(backend: BackendController): void {
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
    const error = await shell.openPath(parsed.filePath);
    if (error) {
      throw new Error(error);
    }
    return { ok: true };
  });

  ipcMain.handle("cocode:open-logs", async () => {
    const error = await shell.openPath(app.getPath("logs"));
    if (error) {
      throw new Error(error);
    }
    return { ok: true };
  });
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
