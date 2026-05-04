import { expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";

import { apiRequest, launchCocode } from "./test-support";

const smokeMarker = "COCODE_REAL_CLI_SMOKE_OK";
const commonEnvAllowlist = [
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

type RealCliSmokeTarget = {
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

const realCliTargets: Record<string, RealCliSmokeTarget> = {
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
      "",
    ],
    outputMode: "json",
    promptDelivery: "arg",
    modelLabel: "claude",
    provider: "anthropic",
  },
};

const selectedTargets = selectedRealCliTargets();

if (selectedTargets.length === 0) {
  test("real CLI smoke checks are opt-in", async () => {
    test.skip(
      true,
      "Set COCODE_E2E_REAL_CLIS=codex,gemini,opencode,claude or all to run real local CLI smoke checks.",
    );
  });
} else {
  for (const target of selectedTargets) {
    test(`runs ${target.id} through the Settings health check`, async (
      { browserName },
      testInfo,
    ) => {
      test.setTimeout(180_000);
      expect(browserName).toBe("chromium");
      ensureCommandAvailable(target.command);

      const { backendInfo, electronApp, page } = await launchCocode(testInfo);

      try {
        const config = await createRealCliAgentConfig(backendInfo, target);

        await page.getByRole("button", { name: "Settings" }).click();
        await expect(
          page.getByRole("heading", { name: "Settings" }),
        ).toBeVisible();
        await page.getByRole("button", { name: config.name }).click();
        await page.getByRole("button", { name: "Test" }).click();

        await expect(
          page.getByText(
            /command smoke check (succeeded|failed|did not include expected output)/,
          ),
        ).toBeVisible({ timeout: 150_000 });

        const healthText = await page.locator("body").innerText();
        if (isExternalProviderLimit(healthText)) {
          testInfo.annotations.push({
            type: "external-provider-limit",
            description: `${target.command} returned a provider capacity or rate-limit response during the smoke check.`,
          });
          test.skip(
            true,
            `${target.command} provider capacity or rate limit blocked this smoke run.`,
          );
        }

        await expect(page.getByText("command smoke check succeeded")).toBeVisible();
        await expect(page.getByText(/^available$/).first()).toBeVisible();
      } finally {
        await electronApp.close();
      }
    });
  }
}

async function createRealCliAgentConfig(
  backendInfo: Awaited<ReturnType<typeof launchCocode>>["backendInfo"],
  target: RealCliSmokeTarget,
) {
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
        reasoning_label: "smoke",
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
            real_cli_smoke: true,
          },
        },
        settings: {
          prompt_delivery: target.promptDelivery,
          version_args: target.versionArgs ?? ["--version"],
          version_timeout_seconds: 20,
          smoke_prompt_enabled: true,
          smoke_prompt:
            "This is a CLI health check. Output only the token made from prefix COCODE_REAL_CLI_SMOKE_ followed by the two-letter uppercase version of ok. Do not explain.",
          smoke_prompt_expected: smokeMarker,
          smoke_timeout_seconds: 120,
          timeout_seconds: 120,
        },
        enabled: true,
      },
    },
  );
}

function selectedRealCliTargets() {
  const raw = process.env.COCODE_E2E_REAL_CLIS?.trim();
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
      throw new Error(`Unknown real CLI smoke target: ${id}`);
    }
    return target;
  });
}

function ensureCommandAvailable(command: string) {
  try {
    execFileSync("which", [command], { stdio: "pipe" });
  } catch {
    throw new Error(`${command} is not installed or not on PATH`);
  }
}

function isExternalProviderLimit(text: string) {
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
