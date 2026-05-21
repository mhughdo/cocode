import {
  type AgentConfig,
  type AgentConfigInput,
  type AgentModelCatalog,
  type AgentModelOption,
  type AgentPreset,
  type AgentReasoningOption,
} from "@/lib/api";

import claudeLogoUrl from "../../../../../../assets/agents/claude-color.svg";
import codexLogoUrl from "../../../../../../assets/agents/codex-color.svg";
import geminiLogoUrl from "../../../../../../assets/agents/gemini-color.svg";
import githubLogoUrl from "../../../../../../assets/agents/github.svg";
import kiroLogoUrl from "../../../../../../assets/agents/kiro-color.svg";
import opencodeLogoUrl from "../../../../../../assets/agents/opencode-logo-light.svg";

export const BUILTIN_REVIEW_AGENT_PRESET_IDS = [
  "codex-cli",
  "claude-code-cli",
  "gemini-cli",
  "opencode-cli",
  "kiro-cli",
] as const;

export type SetupAgentModelChoice = {
  modelId?: string;
  modelLabel?: string;
  provider?: string;
  providerLabel?: string;
  reasoning?: string;
  reasoningLabel?: string;
};

export type AgentVisibilitySource = {
  capabilities: AgentConfigInput["capabilities"];
  command?: string;
  model_label?: string;
  name?: string;
  settings?: Record<string, unknown>;
};

export function applyDiscoveredModel<T extends AgentConfig | AgentPreset>(
  agent: T,
  catalogs: AgentModelCatalog[],
): T {
  const model = defaultModelForAgent(agent, catalogs);
  if (!model || !shouldUseDiscoveredModel(agent.model_label)) {
    return agent;
  }
  return {
    ...agent,
    model_label: model.id,
    capabilities: {
      ...agent.capabilities,
      metadata: {
        ...(agent.capabilities.metadata ?? {}),
        model_id: model.id,
        model_label: model.label,
        model_source: model.source,
      },
    },
    settings: {
      ...agent.settings,
      model_id: model.id,
      model_label: model.label,
      model_source: model.source,
    },
  };
}

export function setupModelsForAgent(
  agent: AgentConfig,
  catalogs: AgentModelCatalog[],
): AgentModelOption[] {
  return setupCatalogForAgent(agent, catalogs)?.models ?? [];
}

export function groupSetupModelsByProvider(models: AgentModelOption[]) {
  const groups: Array<{
    provider: string;
    providerLabel: string;
    models: AgentModelOption[];
  }> = [];
  for (const model of models) {
    const provider = model.provider || "default";
    let group = groups.find((item) => item.provider === provider);
    if (!group) {
      group = {
        provider,
        providerLabel: model.provider_label || modelIDDisplayLabel(provider),
        models: [],
      };
      groups.push(group);
    }
    group.models.push(model);
  }
  return groups;
}

export function setupChoiceFromModel(
  model: AgentModelOption,
  reasoning?: AgentReasoningOption,
): SetupAgentModelChoice {
  const defaultReasoning =
    reasoning ??
    model.reasoning_efforts?.find((effort) => effort.default) ??
    model.reasoning_efforts?.[0];
  return {
    modelId: model.id,
    modelLabel: model.label,
    provider: model.provider,
    providerLabel: model.provider_label,
    reasoning: defaultReasoning?.id,
    reasoningLabel: defaultReasoning?.label,
  };
}

export function setupChoiceForAgent(
  agent: AgentConfig,
  choices: Record<string, SetupAgentModelChoice>,
  catalogs: AgentModelCatalog[],
): SetupAgentModelChoice {
  const explicit = choices[agent.id];
  if (explicit?.modelId || explicit?.reasoning) {
    return explicit;
  }
  const models = setupModelsForAgent(agent, catalogs);
  const configuredModel = agent.model_label?.trim();
  const configuredReasoning = agent.reasoning_label?.trim();
  const model =
    models.find((item) => item.id === configuredModel) ??
    models.find((item) => item.default) ??
    models[0];
  if (!model) {
    return {
      modelLabel: agentDisplayModel(agent),
      reasoning: configuredReasoning,
      reasoningLabel: configuredReasoning
        ? modelIDDisplayLabel(configuredReasoning)
        : undefined,
    };
  }
  const reasoning =
    model.reasoning_efforts?.find((item) => item.id === configuredReasoning) ??
    model.reasoning_efforts?.find((item) => item.default) ??
    model.reasoning_efforts?.[0];
  return setupChoiceFromModel(model, reasoning);
}

export function formatSetupAgentChoiceLabel(
  agent: AgentConfig,
  choices: Record<string, SetupAgentModelChoice>,
  catalogs: AgentModelCatalog[],
) {
  const choice = setupChoiceForAgent(agent, choices, catalogs);
  return [agent.name, choice.modelLabel, choice.reasoningLabel]
    .filter(Boolean)
    .join(" · ");
}

export function formatSetupAgentCompactChoiceLabel(
  agent: AgentConfig,
  choices: Record<string, SetupAgentModelChoice>,
  catalogs: AgentModelCatalog[],
) {
  const choice = setupChoiceForAgent(agent, choices, catalogs);
  return [
    choice.modelLabel || shortSetupAgentName(agent),
    choice.reasoningLabel,
  ]
    .filter(Boolean)
    .join(" · ");
}

export function formatSetupAgentModelChoiceLabel(
  agent: AgentConfig,
  choices: Record<string, SetupAgentModelChoice>,
  catalogs: AgentModelCatalog[],
) {
  const choice = setupChoiceForAgent(agent, choices, catalogs);
  return [choice.modelLabel || "Configured model", choice.reasoningLabel]
    .filter(Boolean)
    .join(" · ");
}

export function buildSetupAgentSelection(
  agent: AgentConfig,
  choices: Record<string, SetupAgentModelChoice>,
  catalogs: AgentModelCatalog[],
  role?: string,
) {
  const choice = setupChoiceForAgent(agent, choices, catalogs);
  return {
    agent_config_id: agent.id,
    role,
    model_label:
      choice.modelId && !shouldUseDiscoveredModel(choice.modelId)
        ? choice.modelId
        : undefined,
    reasoning_label: choice.reasoning,
  };
}

export function agentProvider(agent: AgentVisibilitySource) {
  const provider = agent.capabilities.metadata?.provider;
  return typeof provider === "string" && provider.trim() ? provider : "local";
}

export function agentEgress(agent: AgentVisibilitySource) {
  const egress = agent.capabilities.metadata?.egress;
  return typeof egress === "string" && egress.trim() ? egress : "local";
}

export function agentLogoUrl(agent: AgentVisibilitySource) {
  const marker = [agentProvider(agent), agent.command ?? "", agent.name ?? ""]
    .join(" ")
    .toLowerCase();
  if (marker.includes("opencode")) {
    return opencodeLogoUrl;
  }
  if (marker.includes("kiro")) {
    return kiroLogoUrl;
  }
  if (marker.includes("gemini") || marker.includes("google")) {
    return geminiLogoUrl;
  }
  if (marker.includes("claude") || marker.includes("anthropic")) {
    return claudeLogoUrl;
  }
  if (marker.includes("github")) {
    return githubLogoUrl;
  }
  if (marker.includes("codex") || marker.includes("openai")) {
    return codexLogoUrl;
  }
  return "";
}

export function formatSetupAgentLabel(agent: AgentVisibilitySource) {
  const name = agent.name?.trim() || "Agent";
  const model = agentDisplayModel(agent);
  return model ? `${name} ${model}` : name;
}

export function modelIDDisplayLabel(id: string) {
  const raw = id.includes("/") ? id.split("/").at(-1)! : id;
  return raw
    .split(/[-_]+/)
    .filter(Boolean)
    .map((part) => {
      const lower = part.toLowerCase();
      return ["gpt", "api", "cli", "json", "xai"].includes(lower)
        ? lower.toUpperCase()
        : part.slice(0, 1).toUpperCase() + part.slice(1);
    })
    .join(" ");
}

export function shouldUseDiscoveredModel(modelLabel?: string) {
  const normalized = modelLabel?.trim().toLowerCase() ?? "";
  return (
    normalized === "" ||
    normalized === "default" ||
    normalized === "claude" ||
    normalized === "kiro" ||
    normalized === "opencode" ||
    normalized === "gemini-acp" ||
    normalized === "opencode-acp"
  );
}

function defaultModelForAgent(
  agent: AgentConfig | AgentPreset,
  catalogs: AgentModelCatalog[],
): AgentModelOption | null {
  const catalog = setupCatalogForAgent(agent, catalogs);
  if (!catalog) {
    return null;
  }
  return (
    catalog.models.find((model) => model.default) ?? catalog.models[0] ?? null
  );
}

function setupCatalogForAgent(
  agent: AgentConfig | AgentPreset,
  catalogs: AgentModelCatalog[],
): AgentModelCatalog | null {
  const command = agent.command.trim().toLowerCase();
  const provider = agentProvider(agent).toLowerCase();
  const marker = [
    provider,
    command,
    agent.name ?? "",
    agent.model_label ?? "",
    stringSetting(agent.capabilities.metadata?.provider),
  ]
    .join(" ")
    .toLowerCase();
  const preferredCommand = marker.includes("opencode")
    ? "opencode"
    : marker.includes("kiro")
      ? "kiro-cli"
      : marker.includes("gemini") || marker.includes("google")
        ? "gemini"
        : marker.includes("claude") || marker.includes("anthropic")
          ? "claude"
          : marker.includes("codex") || marker.includes("openai")
            ? "codex"
            : command;
  return (
    catalogs.find(
      (item) =>
        item.available &&
        item.models.length > 0 &&
        item.command.toLowerCase() === preferredCommand,
    ) ??
    catalogs.find(
      (item) =>
        item.available &&
        item.models.length > 0 &&
        (item.command.toLowerCase() === command ||
          item.provider.toLowerCase() === provider),
    ) ??
    null
  );
}

function shortSetupAgentName(agent: AgentConfig) {
  const name = agent.name.trim();
  const normalized = name.toLowerCase();
  if (normalized.includes("claude")) {
    return "Claude";
  }
  if (normalized.includes("opencode")) {
    return "OpenCode";
  }
  if (normalized.includes("kiro")) {
    return "Kiro";
  }
  if (normalized.includes("gemini")) {
    return "Gemini";
  }
  if (normalized.includes("codex")) {
    return "Codex";
  }
  return name || agentProvider(agent);
}

function agentDisplayModel(agent: AgentVisibilitySource) {
  const settingsLabel =
    stringSetting(agent.settings?.model_label).trim() ||
    stringSetting(agent.capabilities.metadata?.model_label).trim();
  if (settingsLabel && !shouldUseDiscoveredModel(settingsLabel)) {
    return settingsLabel;
  }
  const modelLabel = agent.model_label?.trim();
  if (!modelLabel || shouldUseDiscoveredModel(modelLabel)) {
    return "";
  }
  return modelIDDisplayLabel(modelLabel);
}

function stringSetting(value: unknown) {
  return typeof value === "string" ? value : "";
}
