/// <reference types="vite/client" />

interface Window {
  cocode?: {
    appName: string;
    version: string;
    getBackendInfo: () => Promise<{
      baseUrl: string;
      authToken: string;
      logPath: string;
      status: "starting" | "ready" | "stopped";
    }>;
    writeClipboard: (text: string) => Promise<{ ok: true }>;
    selectRepository: () => Promise<string | null>;
    openFile: (request: {
      filePath: string;
      line?: number;
      column?: number;
    }) => Promise<{ ok: true }>;
    openLogs: () => Promise<{ ok: true }>;
  };
}
