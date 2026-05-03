import { contextBridge } from "electron";

contextBridge.exposeInMainWorld("cocode", {
  appName: "cocode",
  version: "0.1.0-dev",
});
