#!/usr/bin/env node
import { mkdirSync, rmSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { spawnSync } from "node:child_process";

const args = new Map();
for (let index = 2; index < process.argv.length; index += 1) {
  const arg = process.argv[index];
  if (!arg.startsWith("--")) {
    throw new Error(`Unexpected argument: ${arg}`);
  }
  const [key, inlineValue] = arg.slice(2).split("=", 2);
  const value =
    inlineValue ??
    (process.argv[index + 1]?.startsWith("--")
      ? undefined
      : process.argv[++index]);
  args.set(key, value ?? "true");
}

const platform = args.get("platform") ?? platformToGOOS(process.platform);
const arch = args.get("arch") ?? archToGOARCH(process.arch);
const binaryName = platform === "windows" ? "cocoded.exe" : "cocoded";
const outputPath = resolve("apps/desktop/build/bin", binaryName);

rmSync(dirname(outputPath), { recursive: true, force: true });
mkdirSync(dirname(outputPath), { recursive: true });

const result = spawnSync(
  "go",
  [
    "build",
    "-trimpath",
    "-ldflags",
    "-s -w",
    "-o",
    outputPath,
    "./cmd/cocoded",
  ],
  {
    cwd: resolve("services/cocoded"),
    env: {
      ...process.env,
      GOOS: platformToGOOS(platform),
      GOARCH: archToGOARCH(arch),
    },
    stdio: "inherit",
  },
);

if (result.status !== 0) {
  process.exit(result.status ?? 1);
}

console.log(`Built cocoded ${platform}/${arch} at ${outputPath}`);

function platformToGOOS(value) {
  switch (value) {
    case "darwin":
    case "linux":
    case "windows":
      return value;
    case "win32":
      return "windows";
    default:
      throw new Error(`Unsupported platform: ${value}`);
  }
}

function archToGOARCH(value) {
  switch (value) {
    case "arm64":
    case "amd64":
      return value;
    case "x64":
      return "amd64";
    default:
      throw new Error(`Unsupported architecture: ${value}`);
  }
}
