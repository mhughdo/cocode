import { contextBridge, ipcRenderer } from "electron";

contextBridge.exposeInMainWorld("cocode", {
  appName: "cocode",
  version: "0.1.0-dev",
  getBackendInfo: () => ipcRenderer.invoke("cocode:get-backend-info"),
  writeClipboard: (text: string) =>
    ipcRenderer.invoke("cocode:write-clipboard", text),
  selectRepository: () => ipcRenderer.invoke("cocode:select-repository"),
  openFile: (request: { filePath: string; line?: number; column?: number }) =>
    ipcRenderer.invoke("cocode:open-file", request),
  openLogs: () => ipcRenderer.invoke("cocode:open-logs"),
});
