import {
  cpSync,
  existsSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawn } from "node:child_process";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const desktopDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const productName = "Cocode";
const bundleIdentifier = "dev.cocode.desktop.dev";
const devBundleVersion = "2";

function main() {
  const env = { ...process.env };
  if (process.platform === "darwin") {
    const devBundle = ensureDarwinDevBundle();
    env.ELECTRON_OVERRIDE_DIST_PATH = devBundle.distDir;
    env.ELECTRON_EXEC_PATH = devBundle.execPath;
  }
  if (process.argv.includes("--prepare-only")) {
    console.log(env.ELECTRON_EXEC_PATH ?? "native-electron");
    return;
  }
  runElectronVite(env);
}

function ensureDarwinDevBundle() {
  const electronPackageDir = dirname(require.resolve("electron/package.json"));
  const electronVersion = require("electron/package.json").version;
  const sourceDistDir = join(electronPackageDir, "dist");
  const devDistDir = join(desktopDir, ".electron-dev", "dist");
  const devAppDir = join(devDistDir, "Electron.app");
  const markerPath = join(devDistDir, ".cocode-electron-version");
  const expectedMarker = `${electronVersion}:${devBundleVersion}`;

  const marker = existsSync(markerPath)
    ? readFileSync(markerPath, "utf8").trim()
    : "";
  if (!existsSync(devAppDir) || marker !== expectedMarker) {
    rmSync(devDistDir, { recursive: true, force: true });
    mkdirSync(dirname(devDistDir), { recursive: true });
    cpSync(sourceDistDir, devDistDir, {
      recursive: true,
      verbatimSymlinks: true,
    });
    patchInfoPlist(join(devAppDir, "Contents", "Info.plist"));
    writeFileSync(markerPath, expectedMarker);
  }

  return {
    distDir: devDistDir,
    execPath: join(devAppDir, "Contents", "MacOS", "Electron"),
  };
}

function patchInfoPlist(plistPath) {
  let plist = readFileSync(plistPath, "utf8");
  plist = replacePlistString(plist, "CFBundleDisplayName", productName);
  plist = replacePlistString(plist, "CFBundleName", productName);
  plist = replacePlistString(plist, "CFBundleIdentifier", bundleIdentifier);
  writeFileSync(plistPath, plist);
}

function replacePlistString(plist, key, value) {
  const pattern = new RegExp(
    `(<key>${escapeRegExp(key)}</key>\\s*<string>)([^<]*)(</string>)`,
  );
  if (!pattern.test(plist)) {
    throw new Error(`Missing ${key} in Electron Info.plist`);
  }
  return plist.replace(pattern, `$1${value}$3`);
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function runElectronVite(env) {
  const child = spawn("electron-vite", ["dev"], {
    cwd: desktopDir,
    env,
    shell: process.platform === "win32",
    stdio: "inherit",
  });

  child.on("exit", (code, signal) => {
    if (signal) {
      process.kill(process.pid, signal);
      return;
    }
    process.exit(code ?? 0);
  });
}

main();
