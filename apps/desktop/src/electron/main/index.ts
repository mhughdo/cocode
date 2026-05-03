import { appendFileSync, mkdirSync } from "node:fs";
import { join } from "node:path";

import { app, BrowserWindow, session, shell } from "electron";
import { electronApp, is, optimizer } from "@electron-toolkit/utils";

import { BackendController } from "./backend";
import { registerIpc } from "./ipc";
import { SecretStore } from "./secret-store";

const backend = new BackendController();
const secretStore = new SecretStore();

function createWindow(): void {
  const mainWindow = new BrowserWindow({
    width: 1448,
    height: 1086,
    minWidth: 1180,
    minHeight: 760,
    show: false,
    title: "cocode",
    webPreferences: {
      preload: join(__dirname, "../preload/index.cjs"),
      sandbox: true,
      contextIsolation: true,
      nodeIntegration: false,
      webSecurity: true,
    },
  });

  mainWindow.on("ready-to-show", () => {
    mainWindow.show();
  });

  mainWindow.webContents.setWindowOpenHandler((details) => {
    void shell.openExternal(details.url);
    return { action: "deny" };
  });
  mainWindow.webContents.on("will-navigate", (event, url) => {
    if (url !== mainWindow.webContents.getURL()) {
      event.preventDefault();
      void shell.openExternal(url);
    }
  });

  if (is.dev && process.env.ELECTRON_RENDERER_URL) {
    void mainWindow.loadURL(process.env.ELECTRON_RENDERER_URL);
  } else {
    void mainWindow.loadFile(join(__dirname, "../renderer/index.html"));
  }
}

void app.whenReady().then(async () => {
  electronApp.setAppUserModelId("dev.cocode.desktop");
  logMainEvent("app ready");
  session.defaultSession.setPermissionRequestHandler(
    (_contents, _permission, callback) => {
      callback(false);
    },
  );

  app.on("browser-window-created", (_, window) => {
    optimizer.watchWindowShortcuts(window);
  });

  await backend.start();
  logMainEvent("backend ready", { backend: backend.getInfo() });
  registerIpc(backend, secretStore);
  logMainEvent("secret store registered");
  createWindow();

  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") {
    app.quit();
  }
});

app.on("before-quit", () => {
  logMainEvent("app quitting");
  backend.stop();
});

function logMainEvent(
  message: string,
  fields: Record<string, unknown> | null = null,
): void {
  const logDir = app.getPath("logs");
  mkdirSync(logDir, { recursive: true });
  appendFileSync(
    join(logDir, "main.log"),
    `${JSON.stringify({
      time: new Date().toISOString(),
      level: "info",
      message,
      ...fields,
    })}\n`,
  );
}
