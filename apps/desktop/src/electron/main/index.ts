import { appendFileSync, existsSync, mkdirSync } from "node:fs";
import { join } from "node:path";

import {
  app,
  BrowserWindow,
  Menu,
  nativeImage,
  session,
  shell,
  type MenuItemConstructorOptions,
} from "electron";
import { electronApp, is, optimizer } from "@electron-toolkit/utils";

import { BackendController } from "./backend";
import { registerIpc } from "./ipc";
import { SecretStore } from "./secret-store";

const backend = new BackendController();
const secretStore = new SecretStore();
const applicationName = "Cocode";

app.setName(applicationName);

function createWindow(): void {
  const appIconPath = resolveAppIconPath();
  const mainWindow = new BrowserWindow({
    width: 1448,
    height: 1086,
    minWidth: 960,
    minHeight: 760,
    autoHideMenuBar: true,
    frame: process.platform === "darwin",
    titleBarStyle: process.platform === "darwin" ? "hiddenInset" : "hidden",
    ...(process.platform === "darwin"
      ? { trafficLightPosition: { x: 16, y: 18 } }
      : {}),
    show: false,
    title: applicationName,
    ...(appIconPath ? { icon: appIconPath } : {}),
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
  mainWindow.webContents.on("before-input-event", (event, input) => {
    const hasPlatformModifier =
      process.platform === "darwin" ? input.meta : input.control;
    if (!hasPlatformModifier || input.alt || input.type !== "keyDown") {
      return;
    }

    const code = input.code;
    const key = input.key.toLowerCase();
    const currentZoom = mainWindow.webContents.getZoomFactor();
    if (
      code === "Equal" ||
      code === "NumpadAdd" ||
      key === "=" ||
      key === "+"
    ) {
      event.preventDefault();
      mainWindow.webContents.setZoomFactor(
        Math.min(1.4, Math.round((currentZoom + 0.1) * 10) / 10),
      );
      return;
    }
    if (
      code === "Minus" ||
      code === "NumpadSubtract" ||
      key === "-" ||
      key === "_"
    ) {
      event.preventDefault();
      mainWindow.webContents.setZoomFactor(
        Math.max(0.75, Math.round((currentZoom - 0.1) * 10) / 10),
      );
      return;
    }
    if (code === "Digit0" || code === "Numpad0" || key === "0") {
      event.preventDefault();
      mainWindow.webContents.setZoomFactor(1);
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
  app.setName(applicationName);
  installApplicationMenu();
  const appIconPath = resolveAppIconPath();
  if (appIconPath) {
    app.setAboutPanelOptions({
      applicationName,
      applicationVersion: app.getVersion(),
      iconPath: appIconPath,
    });
    if (process.platform === "darwin") {
      app.dock?.setIcon(nativeImage.createFromPath(appIconPath));
    }
  }
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

function resolveAppIconPath(): string {
  const candidates = [
    join(__dirname, "../../../../assets/app-icon/app-icon-512.png"),
    join(__dirname, "../assets/app-icon/app-icon-512.png"),
    join(process.cwd(), "../../assets/app-icon/app-icon-512.png"),
    join(process.resourcesPath ?? "", "assets/app-icon/app-icon-512.png"),
  ];
  return candidates.find((candidate) => existsSync(candidate)) ?? "";
}

function installApplicationMenu(): void {
  const template: MenuItemConstructorOptions[] =
    process.platform === "darwin"
      ? [
          {
            label: applicationName,
            submenu: [
              { role: "about", label: `About ${applicationName}` },
              { type: "separator" },
              { role: "services" },
              { type: "separator" },
              { role: "hide", label: `Hide ${applicationName}` },
              { role: "hideOthers" },
              { role: "unhide" },
              { type: "separator" },
              { role: "quit", label: `Quit ${applicationName}` },
            ],
          },
          {
            label: "File",
            submenu: [{ role: "close" }],
          },
          {
            label: "Edit",
            submenu: [
              { role: "undo" },
              { role: "redo" },
              { type: "separator" },
              { role: "cut" },
              { role: "copy" },
              { role: "paste" },
              { role: "selectAll" },
            ],
          },
          {
            label: "View",
            submenu: [
              { role: "reload" },
              { role: "forceReload" },
              { type: "separator" },
              { role: "resetZoom" },
              { role: "zoomIn" },
              { role: "zoomOut" },
              { type: "separator" },
              { role: "togglefullscreen" },
            ],
          },
          {
            label: "Window",
            submenu: [
              { role: "minimize" },
              { role: "zoom" },
              { type: "separator" },
              { role: "front" },
            ],
          },
        ]
      : [
          {
            label: "File",
            submenu: [{ role: "quit" }],
          },
          {
            label: "View",
            submenu: [
              { role: "reload" },
              { role: "forceReload" },
              { type: "separator" },
              { role: "resetZoom" },
              { role: "zoomIn" },
              { role: "zoomOut" },
              { type: "separator" },
              { role: "togglefullscreen" },
            ],
          },
        ];
  Menu.setApplicationMenu(Menu.buildFromTemplate(template));
}
