import { contextBridge, ipcRenderer } from "electron";

contextBridge.exposeInMainWorld("cocode", {
  appName: "cocode",
  version: "0.1.0-dev",
  getBackendInfo: () => ipcRenderer.invoke("cocode:get-backend-info"),
  writeClipboard: (text: string) =>
    ipcRenderer.invoke("cocode:write-clipboard", text),
  getGitHubCredential: () => ipcRenderer.invoke("cocode:get-github-credential"),
  saveGitHubToken: (request: { token: string; displayName?: string }) =>
    ipcRenderer.invoke("cocode:save-github-token", request),
  deleteGitHubToken: () => ipcRenderer.invoke("cocode:delete-github-token"),
  createGitHubSnapshot: (request: {
    workspaceId: string;
    repositoryId: string;
    url: string;
  }) => ipcRenderer.invoke("cocode:create-github-snapshot", request),
  selectRepository: () => ipcRenderer.invoke("cocode:select-repository"),
  openFile: (request: { filePath: string; line?: number; column?: number }) =>
    ipcRenderer.invoke("cocode:open-file", request),
  openLogs: () => ipcRenderer.invoke("cocode:open-logs"),
});
