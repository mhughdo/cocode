import { execFileSync } from "node:child_process";

import { apiRequest, type BackendInfo } from "./test-support";

export const smokeMarker = "COCODE_REAL_CLI_SMOKE_OK";

export const commonEnvAllowlist = [
  "PATH",
  "HOME",
  "TERM",
  "LANG",
  "NO_COLOR",
  "FORCE_COLOR",
  "CODEX_HOME",
  "ANTHROPIC_API_KEY",
  "OPENAI_API_KEY",
  "GEMINI_API_KEY",
  "GOOGLE_API_KEY",
  "GOOGLE_GENAI_USE_VERTEXAI",
  "GOOGLE_APPLICATION_CREDENTIALS",
  "GOOGLE_CLOUD_PROJECT",
  "GOOGLE_CLOUD_LOCATION",
  "OPENROUTER_API_KEY",
  "XAI_API_KEY",
];

export type RealCliTarget = {
  id: string;
  name: string;
  command: string;
  args: string[];
  outputMode: "json" | "jsonl";
  promptDelivery: "stdin" | "arg";
  modelLabel: string;
  provider: string;
  versionArgs?: string[];
};

export const realCliTargets: Record<string, RealCliTarget> = {
  codex: {
    id: "codex",
    name: "E2E Real Codex CLI",
    command: "codex",
    args: [
      "exec",
      "--json",
      "-m",
      process.env.COCODE_E2E_CODEX_MODEL || "gpt-5.4-mini",
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
      "plan",
      "--skip-trust",
    ],
    outputMode: "json",
    promptDelivery: "arg",
    modelLabel: "default",
    provider: "google",
  },
  opencode: {
    id: "opencode",
    name: "E2E Real OpenCode CLI",
    command: "opencode",
    args: ["run", "--format", "json", "{{prompt}}"],
    outputMode: "jsonl",
    promptDelivery: "arg",
    modelLabel: "opencode",
    provider: "opencode",
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
        reasoning_label: options.purpose === "smoke" ? "smoke" : "real-review",
        capabilities: {
          supports_json: true,
          supports_streaming: target.outputMode === "jsonl",
          supports_sessions: false,
          can_read: true,
          can_write: false,
          can_cancel: true,
          output_modes: [target.outputMode, "text"],
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
  try {
    execFileSync("which", [command], { stdio: "pipe" });
  } catch {
    throw new Error(`${command} is not installed or not on PATH`);
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
