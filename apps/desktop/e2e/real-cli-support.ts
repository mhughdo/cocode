import { execFileSync } from "node:child_process";
import { accessSync, constants, readdirSync, statSync } from "node:fs";
import { homedir } from "node:os";
import { basename, join } from "node:path";

import { apiRequest, type BackendInfo } from "./test-support";

export const smokeMarker = "COCODE_REAL_CLI_SMOKE_OK";

export const commonEnvAllowlist = [
  "PATH",
  "HOME",
  "USER",
  "LOGNAME",
  "SHELL",
  "TMPDIR",
  "TERM",
  "COLORTERM",
  "LANG",
  "NO_COLOR",
  "CODEX_HOME",
  "ANTHROPIC_API_KEY",
  "OPENAI_API_KEY",
  "GEMINI_API_KEY",
  "GOOGLE_API_KEY",
  "GOOGLE_GENAI_USE_VERTEXAI",
  "GOOGLE_APPLICATION_CREDENTIALS",
  "GOOGLE_CLOUD_PROJECT",
  "GOOGLE_CLOUD_LOCATION",
  "ANTIGRAVITY_API_KEY",
  "KIRO_API_KEY",
  "KIRO_HOME",
  "OPENROUTER_API_KEY",
  "XAI_API_KEY",
];

export type RealCliTarget = {
  id: string;
  name: string;
  command: string;
  args: string[];
  outputMode: "json" | "jsonl" | "text";
  promptDelivery: "stdin" | "arg";
  modelLabel: string;
  reasoningLabel?: string;
  provider: string;
  versionArgs?: string[];
};

export const realCliTargets: Record<string, RealCliTarget> = {
  codex: {
    id: "codex",
    name: "E2E Real Codex CLI",
    command: "codex",
    args: [
      "-a",
      "never",
      "exec",
      "--json",
      "--sandbox",
      "read-only",
      "--skip-git-repo-check",
      "--ephemeral",
      "--ignore-rules",
      "--color",
      "never",
      "-",
    ],
    outputMode: "jsonl",
    promptDelivery: "stdin",
    modelLabel: process.env.COCODE_E2E_CODEX_MODEL || "gpt-5.4-mini",
    reasoningLabel: process.env.COCODE_E2E_CODEX_REASONING || "low",
    provider: "openai",
  },
  gemini: {
    id: "gemini",
    name: "E2E Real Gemini CLI",
    command: "gemini",
    args: [
      "-p",
      "{{prompt}}",
      "--output-format",
      "json",
      "--approval-mode",
      "default",
      "--skip-trust",
    ],
    outputMode: "json",
    promptDelivery: "arg",
    modelLabel: "default",
    provider: "google",
  },
  opencode: {
    id: "opencode",
    name: "E2E Real OpenCode Go Kimi",
    command: "opencode",
    args: ["run", "--pure", "--format", "json", "--thinking", "{{prompt}}"],
    outputMode: "jsonl",
    promptDelivery: "arg",
    modelLabel:
      process.env.COCODE_E2E_OPENCODE_MODEL || "opencode-go/kimi-k2.6",
    reasoningLabel: process.env.COCODE_E2E_OPENCODE_VARIANT || "high",
    provider: process.env.COCODE_E2E_OPENCODE_PROVIDER || "opencode-go",
  },
  agy: {
    id: "agy",
    name: "E2E Real Antigravity CLI",
    command: "agy",
    args: [
      "--print",
      "--sandbox",
      "--dangerously-skip-permissions",
      "--print-timeout",
      "30m0s",
    ],
    outputMode: "text",
    promptDelivery: "stdin",
    modelLabel: process.env.COCODE_E2E_AGY_MODEL || "gemini-3.5-flash",
    reasoningLabel: process.env.COCODE_E2E_AGY_REASONING || "high",
    provider: "antigravity",
  },
  kiro: {
    id: "kiro",
    name: "E2E Real Kiro CLI",
    command: "kiro-cli",
    args: [
      "chat",
      "--no-interactive",
      "--trust-tools=read,grep,glob,code",
      "--wrap",
      "never",
      "{{prompt}}",
    ],
    outputMode: "text",
    promptDelivery: "arg",
    modelLabel: process.env.COCODE_E2E_KIRO_MODEL || "auto",
    provider: "kiro",
  },
  claude: {
    id: "claude",
    name: "E2E Real Claude Code CLI",
    command: "claude",
    args: [
      "-p",
      "{{prompt}}",
      "--output-format",
      "json",
      "--permission-mode",
      "plan",
      "--no-session-persistence",
      "--tools",
      "Read,Grep,Glob",
    ],
    outputMode: "json",
    promptDelivery: "arg",
    modelLabel: "claude",
    reasoningLabel: process.env.COCODE_E2E_CLAUDE_EFFORT || "high",
    provider: "anthropic",
  },
};

export function selectedRealCliTargets(envName: string): RealCliTarget[] {
  const raw = process.env[envName]?.trim();
  if (!raw) {
    return [];
  }
  const requested =
    raw.toLowerCase() === "all"
      ? Object.keys(realCliTargets)
      : raw
          .split(",")
          .map((item) => item.trim().toLowerCase())
          .filter(Boolean);
  return requested.map((id) => {
    const target = realCliTargets[id];
    if (!target) {
      throw new Error(`Unknown real CLI target in ${envName}: ${id}`);
    }
    return target;
  });
}

export type RealCliConfigPurpose = "smoke" | "review";

export async function createRealCliAgentConfig(
  backendInfo: BackendInfo,
  target: RealCliTarget,
  options: {
    purpose: RealCliConfigPurpose;
    timeoutSeconds?: number;
  },
) {
  const timeoutSeconds =
    options.timeoutSeconds ?? (options.purpose === "smoke" ? 120 : 300);
  return apiRequest<{ id: string; name: string }>(
    backendInfo,
    "/api/agents/configs",
    {
      method: "POST",
      body: {
        name: target.name,
        role: "primary_reviewer",
        adapter_kind: "cli_noninteractive",
        command: target.command,
        args: target.args,
        cwd_mode: "repo_root",
        env_allowlist: commonEnvAllowlist,
        output_mode: target.outputMode,
        model_label: target.modelLabel,
        reasoning_label: target.reasoningLabel ?? "",
        capabilities: {
          supports_json: target.outputMode !== "text",
          supports_streaming: target.outputMode === "jsonl",
          supports_sessions: false,
          can_read: true,
          can_write: false,
          can_cancel: true,
          output_modes:
            target.outputMode === "text"
              ? ["text"]
              : [target.outputMode, "text"],
          metadata: {
            provider: target.provider,
            egress: "external",
            real_cli_smoke: options.purpose === "smoke",
            real_cli_review: options.purpose === "review",
          },
        },
        settings: {
          prompt_delivery: target.promptDelivery,
          version_args: target.versionArgs ?? ["--version"],
          version_timeout_seconds: 20,
          smoke_prompt_enabled: options.purpose === "smoke",
          smoke_prompt:
            options.purpose === "smoke"
              ? "This is a CLI health check. Output only the token made from prefix COCODE_REAL_CLI_SMOKE_ followed by the two-letter uppercase version of ok. Do not explain."
              : undefined,
          smoke_prompt_expected:
            options.purpose === "smoke" ? smokeMarker : undefined,
          smoke_timeout_seconds:
            options.purpose === "smoke" ? timeoutSeconds : undefined,
          timeout_seconds: timeoutSeconds,
        },
        enabled: true,
      },
    },
  );
}

export function ensureCommandAvailable(command: string) {
  if (resolveCommandExecutableForE2E(command)) {
    return;
  }
  throw new Error(`${command} is not installed or not on PATH`);
}

export function resolveCommandExecutableForE2E(command: string): string {
  if (isExecutableFile(command)) {
    return command;
  }
  try {
    return execFileSync("which", [command], {
      stdio: "pipe",
      encoding: "utf8",
    }).trim();
  } catch {
    if (basename(command) === "opencode") {
      const resolved = resolveOpenCodeExecutableForE2E();
      if (resolved) {
        return resolved;
      }
    }
    return "";
  }
}

export function isExternalProviderLimit(text: string) {
  const normalized = text.toLowerCase();
  return [
    "429",
    "capacity",
    "model_capacity_exhausted",
    "quota",
    "rate limit",
    "resource_exhausted",
    "too many requests",
  ].some((needle) => normalized.includes(needle));
}

function resolveOpenCodeExecutableForE2E(): string {
  const home = homedir();
  const platformPackage = opencodePlatformPackageForE2E();
  const candidates: string[] = [];
  const pnpmRoots = [
    process.env.PNPM_HOME,
    join(home, "Library", "pnpm"),
    join(home, ".local", "share", "pnpm"),
    join(home, ".pnpm"),
  ].filter((value): value is string => Boolean(value));
  for (const root of pnpmRoots) {
    const globalRoot = join(root, "global");
    for (const globalVersion of safeReadDir(globalRoot)) {
      const pnpmDir = join(globalRoot, globalVersion, ".pnpm");
      for (const entry of safeReadDir(pnpmDir)) {
        if (entry.startsWith(`${platformPackage}@`)) {
          candidates.push(
            join(
              pnpmDir,
              entry,
              "node_modules",
              platformPackage,
              "bin",
              "opencode",
            ),
          );
        }
        if (!entry.startsWith("opencode-ai@")) {
          continue;
        }
        candidates.push(
          join(
            pnpmDir,
            entry,
            "node_modules",
            "opencode-ai",
            "node_modules",
            platformPackage,
            "bin",
            "opencode",
          ),
          join(
            pnpmDir,
            entry,
            "node_modules",
            "opencode-ai",
            "bin",
            "opencode",
          ),
        );
      }
    }
  }
  candidates.sort().reverse();
  return candidates.find(isExecutableFile) ?? "";
}

function opencodePlatformPackageForE2E(): string {
  const arch = process.arch === "x64" ? "x64" : process.arch;
  const platform =
    process.platform === "darwin"
      ? "darwin"
      : process.platform === "win32"
        ? "win32"
        : process.platform;
  return `opencode-${platform}-${arch}`;
}

function safeReadDir(path: string): string[] {
  try {
    return readdirSync(path);
  } catch {
    return [];
  }
}

function isExecutableFile(path: string): boolean {
  try {
    const stat = statSync(path);
    if (!stat.isFile()) {
      return false;
    }
    accessSync(path, constants.X_OK);
    return true;
  } catch {
    return false;
  }
}
