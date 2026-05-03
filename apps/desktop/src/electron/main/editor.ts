import { spawn } from "node:child_process";

import { shell } from "electron";

export interface OpenFileRequest {
  filePath: string;
  line?: number;
  column?: number;
}

const editorCommands: Record<string, (request: OpenFileRequest) => string[]> = {
  code: (request) => ["-g", editorLocation(request)],
  cursor: (request) => ["-g", editorLocation(request)],
  windsurf: (request) => ["-g", editorLocation(request)],
  zed: (request) => [editorLocation(request)],
};

export async function openExternalEditor(
  request: OpenFileRequest,
): Promise<void> {
  const configuredEditor = process.env.COCODE_EDITOR?.trim();
  if (configuredEditor) {
    await openWithCommand(configuredEditor, request);
    return;
  }

  if (process.platform === "darwin") {
    const opened = await tryOpenWithCommand("code", request);
    if (opened) {
      return;
    }
  }

  const error = await shell.openPath(request.filePath);
  if (error) {
    throw new Error(error);
  }
}

async function tryOpenWithCommand(
  command: string,
  request: OpenFileRequest,
): Promise<boolean> {
  try {
    await openWithCommand(command, request);
    return true;
  } catch (error) {
    if (isCommandMissing(error)) {
      return false;
    }
    throw error;
  }
}

function openWithCommand(
  command: string,
  request: OpenFileRequest,
): Promise<void> {
  const args = editorArgs(command, request);

  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      detached: true,
      stdio: "ignore",
    });
    child.on("error", reject);
    child.on("spawn", () => {
      child.unref();
      resolve();
    });
  });
}

function editorArgs(command: string, request: OpenFileRequest): string[] {
  const key = command.split(/[\\/]/).pop()?.toLowerCase() ?? command;
  const buildArgs = editorCommands[key];
  if (buildArgs) {
    return buildArgs(request);
  }

  return [editorLocation(request)];
}

function editorLocation(request: OpenFileRequest): string {
  if (!request.line) {
    return request.filePath;
  }
  if (!request.column) {
    return `${request.filePath}:${request.line}`;
  }
  return `${request.filePath}:${request.line}:${request.column}`;
}

function isCommandMissing(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "code" in error &&
    error.code === "ENOENT"
  );
}
