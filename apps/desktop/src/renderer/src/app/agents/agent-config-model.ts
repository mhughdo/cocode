import {
  type AgentConfig,
  type AgentConfigInput,
  type AgentModelCatalog,
  type AgentPreset,
  type ApiClient,
  errorApiState,
  idleApiState,
  loadApiResource,
  type Loadable,
  successApiState,
} from "@/lib/api";
import {
  applyDiscoveredModel,
  BUILTIN_REVIEW_AGENT_PRESET_IDS,
} from "./agent-utils";

export type PromptDelivery = "stdin" | "arg" | "temp_file";

export type AgentConfigFormState = {
  id?: string;
  sourcePresetId?: string;
  name: string;
  role: string;
  adapterKind: string;
  command: string;
  argsText: string;
  cwdMode: string;
  envAllowlistText: string;
  outputMode: string;
  modelLabel: string;
  reasoningLabel: string;
  credentialRefsText: string;
  enabled: boolean;
  capabilities: AgentConfigInput["capabilities"];
  settings: Record<string, unknown>;
  promptDelivery: PromptDelivery;
  timeoutSeconds: number;
  versionArgsText: string;
  skipVersion: boolean;
  smokePromptEnabled: boolean;
  smokePrompt: string;
  allowRiskyCommand: boolean;
};

const DEFAULT_CWD_MODE = "repo_root";

export async function loadAgentConfigs(
  client: ApiClient,
  options: {
    bootstrapBuiltIns?: boolean;
    modelCatalogs?: AgentModelCatalog[];
  } = {},
): Promise<Loadable<AgentConfig[]>> {
  const configs = await loadApiResource(() => client.listAgentConfigs());
  const catalogState = options.modelCatalogs
    ? successApiState(options.modelCatalogs)
    : configs.status === "success"
      ? await loadApiResource(() => client.listAgentModelCatalog())
      : idleApiState<AgentModelCatalog[]>();
  const catalogs = catalogState.status === "success" ? catalogState.data : [];
  const applyCatalogs = (
    state: Loadable<AgentConfig[]>,
  ): Loadable<AgentConfig[]> =>
    state.status === "success" && catalogs.length > 0
      ? successApiState(
          state.data.map((agent) => applyDiscoveredModel(agent, catalogs)),
        )
      : state;

  if (
    configs.status !== "success" ||
    configs.data.length > 0 ||
    !options.bootstrapBuiltIns
  ) {
    return applyCatalogs(configs);
  }

  const presets = await loadApiResource(() => client.listAgentPresets());
  if (presets.status !== "success") {
    return presets.status === "error" ? errorApiState(presets.error) : configs;
  }

  const presetById = new Map(presets.data.map((preset) => [preset.id, preset]));
  let createdAny = false;
  let firstError: Error | null = null;
  for (const id of BUILTIN_REVIEW_AGENT_PRESET_IDS) {
    const preset = presetById.get(id);
    if (!preset?.enabled) {
      continue;
    }
    let body: AgentConfigInput;
    try {
      body = agentConfigInputFromPreset(preset, catalogs);
    } catch (error) {
      if (!firstError) {
        firstError = error instanceof Error ? error : new Error(String(error));
      }
      continue;
    }
    const created = await loadApiResource(() => client.createAgentConfig(body));
    if (created.status === "success") {
      createdAny = true;
    } else if (created.status === "error" && !firstError) {
      firstError = created.error;
    }
  }
  if (!createdAny && firstError) {
    return errorApiState(firstError);
  }
  return applyCatalogs(await loadApiResource(() => client.listAgentConfigs()));
}

export async function loadAgentModelCatalogs(
  client: ApiClient,
  options: { refresh?: boolean } = {},
) {
  return loadApiResource(() => client.listAgentModelCatalog(options));
}

export function shouldRecheckAgentModelCatalogs(catalogs: AgentModelCatalog[]) {
  return catalogs.some((catalog) => catalog.stale || catalog.refreshing);
}

export function defaultAgentConfigForm(): AgentConfigFormState {
  return {
    name: "Custom CLI",
    role: "custom_reviewer",
    adapterKind: "cli_non_interactive",
    command: "",
    argsText: "",
    cwdMode: DEFAULT_CWD_MODE,
    envAllowlistText: "",
    outputMode: "text",
    modelLabel: "custom",
    reasoningLabel: "",
    credentialRefsText: "",
    enabled: false,
    capabilities: {
      can_read: true,
      can_cancel: true,
      supports_json: true,
      output_modes: ["text", "json", "jsonl", "ndjson"],
      metadata: { provider: "custom", egress: "external" },
    },
    settings: {
      prompt_delivery: "stdin",
      timeout_seconds: 1800,
      skip_version: true,
      smoke_prompt_enabled: false,
    },
    promptDelivery: "stdin",
    timeoutSeconds: 1800,
    versionArgsText: "",
    skipVersion: true,
    smokePromptEnabled: false,
    smokePrompt: "",
    allowRiskyCommand: false,
  };
}

export function formFromAgentPreset(preset: AgentPreset): AgentConfigFormState {
  return formFromAgentLike({
    ...preset,
    name: preset.id === "custom-cli" ? "Custom CLI" : preset.name,
    enabled: preset.enabled && preset.id !== "custom-cli",
    sourcePresetId: preset.id,
  });
}

export function agentConfigInputFromPreset(
  preset: AgentPreset,
  catalogs: AgentModelCatalog[] = [],
): AgentConfigInput {
  const discoveredPreset = applyDiscoveredModel(preset, catalogs);
  const body = agentConfigBodyFromForm(formFromAgentPreset(discoveredPreset));
  if (body instanceof Error) {
    throw body;
  }
  return {
    ...body,
    settings: {
      ...body.settings,
      source_preset_id: preset.id,
      builtin: true,
    },
  };
}

export function formFromAgentConfig(config: AgentConfig): AgentConfigFormState {
  return formFromAgentLike(config);
}

function formFromAgentLike(
  source: (AgentPreset | AgentConfig) & {
    id?: string;
    enabled: boolean;
    sourcePresetId?: string;
  },
): AgentConfigFormState {
  const settings = source.settings ?? {};
  return {
    id: "created_at" in source ? source.id : undefined,
    sourcePresetId: source.sourcePresetId,
    name: source.name,
    role: source.role,
    adapterKind: source.adapter_kind,
    command: source.command ?? "",
    argsText: (source.args ?? []).join("\n"),
    cwdMode: DEFAULT_CWD_MODE,
    envAllowlistText: (source.env_allowlist ?? []).join(", "),
    outputMode: source.output_mode || "text",
    modelLabel: source.model_label ?? "",
    reasoningLabel: source.reasoning_label ?? "",
    credentialRefsText: credentialRefsTextSetting(settings.credential_refs),
    enabled: source.enabled,
    capabilities: source.capabilities,
    settings,
    promptDelivery: promptDeliveryValue(settings.prompt_delivery),
    timeoutSeconds: numberSetting(settings.timeout_seconds, 1800),
    versionArgsText: stringArraySetting(settings.version_args).join(" "),
    skipVersion: Boolean(settings.skip_version),
    smokePromptEnabled: Boolean(settings.smoke_prompt_enabled),
    smokePrompt: stringSetting(settings.smoke_prompt),
    allowRiskyCommand: Boolean(settings.allow_risky_command),
  };
}

export function agentConfigBodyFromForm(
  form: AgentConfigFormState,
): AgentConfigInput | Error {
  const command = form.command.trim();
  if (form.adapterKind === "cli_non_interactive" && command === "") {
    return new Error("CLI agents require a command before saving.");
  }
  if (!Number.isFinite(form.timeoutSeconds) || form.timeoutSeconds <= 0) {
    return new Error("Timeout seconds must be a positive number.");
  }
  if (
    !supportedOutputModes(form.capabilities, form.outputMode).includes(
      form.outputMode,
    )
  ) {
    return new Error("Selected output mode is not supported by this agent.");
  }

  const settings: Record<string, unknown> = {
    ...form.settings,
    prompt_delivery: form.promptDelivery,
    timeout_seconds: form.timeoutSeconds,
    skip_version: form.skipVersion,
    smoke_prompt_enabled: form.smokePromptEnabled,
    allow_risky_command: form.allowRiskyCommand,
  };
  const versionArgs = parseInlineList(form.versionArgsText);
  if (versionArgs.length > 0) {
    settings.version_args = versionArgs;
  } else {
    delete settings.version_args;
  }
  if (form.smokePrompt.trim()) {
    settings.smoke_prompt = form.smokePrompt.trim();
  } else {
    delete settings.smoke_prompt;
  }
  const credentialRefs = parseCredentialRefs(form.credentialRefsText);
  if (credentialRefs instanceof Error) {
    return credentialRefs;
  }
  if (Object.keys(credentialRefs).length > 0) {
    settings.credential_refs = credentialRefs;
  } else {
    delete settings.credential_refs;
  }

  return {
    name: form.name.trim(),
    role: form.role.trim(),
    adapter_kind: form.adapterKind,
    command,
    args: parseArgLines(form.argsText),
    cwd_mode: DEFAULT_CWD_MODE,
    env_allowlist: parseInlineList(form.envAllowlistText),
    output_mode: form.outputMode,
    model_label: form.modelLabel.trim(),
    reasoning_label: form.reasoningLabel.trim(),
    capabilities: form.capabilities,
    settings,
    enabled: form.enabled,
  };
}

export function supportedOutputModes(
  capabilities: AgentConfigInput["capabilities"],
  currentMode: string,
) {
  const modes = Array.isArray(capabilities.output_modes)
    ? capabilities.output_modes.filter(
        (mode): mode is string => typeof mode === "string",
      )
    : [];
  if (modes.length === 0) {
    if (capabilities.supports_json) {
      modes.push("json");
    }
    modes.push("text");
  }
  if (currentMode && !modes.includes(currentMode)) {
    modes.push(currentMode);
  }
  return Array.from(new Set(modes));
}

function promptDeliveryValue(value: unknown): PromptDelivery {
  return value === "arg" || value === "temp_file" ? value : "stdin";
}

function numberSetting(value: unknown, fallback: number) {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function stringSetting(value: unknown) {
  return typeof value === "string" ? value : "";
}

function stringArraySetting(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function credentialRefsTextSetting(value: unknown) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return "";
  }
  return Object.entries(value as Record<string, unknown>)
    .filter(
      (entry): entry is [string, string] =>
        typeof entry[1] === "string" && entry[1].trim() !== "",
    )
    .map(([name, ref]) => `${name}=${ref}`)
    .join("\n");
}

function parseArgLines(value: string) {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseInlineList(value: string) {
  return value
    .split(/[\n,]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseCredentialRefs(value: string): Record<string, string> | Error {
  const refs: Record<string, string> = {};
  for (const item of parseInlineList(value)) {
    const separator = item.indexOf("=");
    if (separator <= 0 || separator === item.length - 1) {
      return new Error("Credential refs must use ENV_NAME=credential:key.");
    }
    const name = item.slice(0, separator).trim();
    const ref = item.slice(separator + 1).trim();
    if (!/^[A-Z_][A-Z0-9_]*$/.test(name)) {
      return new Error(`Credential ref env name is invalid: ${name}`);
    }
    if (!/^[a-zA-Z0-9_.:-]{1,120}$/.test(ref)) {
      return new Error(`Credential ref key is invalid: ${name}`);
    }
    refs[name] = ref;
  }
  return refs;
}
