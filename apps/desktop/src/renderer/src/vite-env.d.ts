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
    getGitHubCredential: () => Promise<{
      configured: boolean;
      credential?: {
        id: string;
        kind: string;
        display_name: string;
        storage_provider: string;
        storage_key: string;
        metadata: Record<string, unknown>;
        created_at: string;
        updated_at: string;
      };
    }>;
    saveGitHubToken: (request: {
      token: string;
      displayName?: string;
    }) => Promise<{
      configured: boolean;
      credential?: {
        id: string;
        kind: string;
        display_name: string;
        storage_provider: string;
        storage_key: string;
        metadata: Record<string, unknown>;
        created_at: string;
        updated_at: string;
      };
    }>;
    deleteGitHubToken: () => Promise<{
      deleted: boolean;
      storage_key?: string;
    }>;
    createGitHubSnapshot: (request: {
      workspaceId: string;
      repositoryId: string;
      url: string;
      authMethod?: "token" | "api" | "gh_cli";
    }) => Promise<import("@/lib/api").Snapshot>;
    selectRepository: () => Promise<string | null>;
    openFile: (request: {
      filePath: string;
      line?: number;
      column?: number;
    }) => Promise<{ ok: true }>;
    openLogs: () => Promise<{ ok: true }>;
  };
}
