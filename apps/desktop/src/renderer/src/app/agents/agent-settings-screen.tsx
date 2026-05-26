import { useEffect, useState } from "react";
import {
  ArrowUpIcon,
  BotIcon,
  CheckIcon,
  ChevronDownIcon,
  ClockIcon,
  TerminalIcon,
} from "lucide-react";

import { EmptyState, ErrorState, LoadingRows } from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { InputGroup, InputGroupTextarea } from "@/components/ui/input-group";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  type AgentConfig,
  type AgentConfigHealth,
  type AgentPreset,
  type ApiClient,
  type DeleteGitHubCredentialResponse,
  errorApiState,
  type GitHubCredentialStatusResponse,
  idleApiState,
  loadApiResource,
  type Loadable,
  loadingApiState,
  type ReviewRule,
  type ReviewRuleListResponse,
  type SettingsExportPayload,
  type SettingsImportResponse,
  successApiState,
  type Workspace,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  agentConfigBodyFromForm,
  type AgentConfigFormState,
  defaultAgentConfigForm,
  formFromAgentConfig,
  formFromAgentPreset,
  type PromptDelivery,
  supportedOutputModes,
} from "./agent-config-model";
import {
  AgentSettingSwitch,
  GitHubCredentialPanel,
  HealthSummary,
  ReviewRuleMemoryPanel,
  SettingsPortabilityPanel,
  type ReviewRuleDraftState,
  type SettingsCollisionPolicy,
  defaultReviewRuleDraft,
  removeReviewRuleState,
  upsertReviewRuleState,
} from "./agent-settings-panels";
import { agentEgress, agentProvider } from "./agent-utils";

export function AgentSettingsScreen({
  activeWorkspace,
  client,
  onBack,
}: {
  activeWorkspace?: Workspace;
  client: ApiClient | null;
  onBack: () => void;
}) {
  const [presets, setPresets] =
    useState<Loadable<AgentPreset[]>>(idleApiState());
  const [configs, setConfigs] =
    useState<Loadable<AgentConfig[]>>(idleApiState());
  const [formMode, setFormMode] = useState<"create" | "edit">("create");
  const [form, setForm] = useState<AgentConfigFormState>(
    defaultAgentConfigForm(),
  );
  const [saveState, setSaveState] =
    useState<Loadable<AgentConfig>>(idleApiState());
  const [healthByConfigId, setHealthByConfigId] = useState<
    Record<string, Loadable<AgentConfigHealth>>
  >({});
  const [githubCredential, setGitHubCredential] =
    useState<Loadable<GitHubCredentialStatusResponse>>(idleApiState());
  const [githubToken, setGitHubToken] = useState("");
  const [githubDisplayName, setGitHubDisplayName] = useState("");
  const [githubSaveState, setGitHubSaveState] =
    useState<Loadable<GitHubCredentialStatusResponse>>(idleApiState());
  const [githubDeleteState, setGitHubDeleteState] =
    useState<Loadable<DeleteGitHubCredentialResponse>>(idleApiState());
  const [reviewRules, setReviewRules] =
    useState<Loadable<ReviewRuleListResponse>>(idleApiState());
  const [reviewRuleDraft, setReviewRuleDraft] = useState<ReviewRuleDraftState>(
    defaultReviewRuleDraft(),
  );
  const [reviewRuleAction, setReviewRuleAction] =
    useState<Loadable<ReviewRule | { deleted: boolean; id: string }>>(
      idleApiState(),
    );
  const [settingsExportText, setSettingsExportText] = useState("");
  const [settingsImportText, setSettingsImportText] = useState("");
  const [settingsCollisionPolicy, setSettingsCollisionPolicy] =
    useState<SettingsCollisionPolicy>("skip");
  const [settingsPortabilityState, setSettingsPortabilityState] =
    useState<Loadable<SettingsExportPayload | SettingsImportResponse>>(
      idleApiState(),
    );
  const [showAdvancedAgentSettings, setShowAdvancedAgentSettings] =
    useState(false);
  const [showProjectSettings, setShowProjectSettings] = useState(false);

  const presetList = presets.status === "success" ? presets.data : [];
  const configList = configs.status === "success" ? configs.data : [];
  const enabledConfigCount = configList.filter(
    (config) => config.enabled,
  ).length;
  const activeHealth = form.id ? healthByConfigId[form.id] : undefined;
  const outputModes = supportedOutputModes(form.capabilities, form.outputMode);

  useEffect(() => {
    let canceled = false;

    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      if (!client) {
        setPresets(errorApiState(new Error("Backend client is unavailable")));
        setConfigs(errorApiState(new Error("Backend client is unavailable")));
        setGitHubCredential(
          errorApiState(new Error("Backend client is unavailable")),
        );
        setReviewRules(
          errorApiState(new Error("Backend client is unavailable")),
        );
        return;
      }

      setPresets(loadingApiState());
      setConfigs(loadingApiState());
      setGitHubCredential(loadingApiState());
      setReviewRules(
        activeWorkspace ? loadingApiState() : successApiState({ items: [] }),
      );
      void Promise.all([
        loadApiResource(() => client.listAgentPresets()),
        loadApiResource(() => client.listAgentConfigs()),
        loadApiResource(() => client.getGitHubCredential()),
        activeWorkspace
          ? loadApiResource(() => client.listReviewRules(activeWorkspace.id))
          : Promise.resolve(successApiState({ items: [] })),
      ]).then(([presetState, configState, credentialState, ruleState]) => {
        if (canceled) {
          return;
        }
        setPresets(presetState);
        setConfigs(configState);
        setGitHubCredential(credentialState);
        setReviewRules(ruleState);

        if (configState.status === "success" && configState.data[0]) {
          setFormMode("edit");
          setForm(formFromAgentConfig(configState.data[0]));
          return;
        }
        if (presetState.status === "success") {
          const defaultPreset =
            presetState.data.find((preset) => preset.id === "codex-cli") ??
            presetState.data[0];
          if (defaultPreset) {
            setFormMode("create");
            setForm(formFromAgentPreset(defaultPreset));
          }
        }
      });
    });

    return () => {
      canceled = true;
    };
  }, [activeWorkspace, client]);

  function selectPreset(preset: AgentPreset) {
    setFormMode("create");
    setForm(formFromAgentPreset(preset));
    setSaveState(idleApiState());
  }

  function selectConfig(config: AgentConfig) {
    setFormMode("edit");
    setForm(formFromAgentConfig(config));
    setSaveState(idleApiState());
  }

  async function saveGitHubToken() {
    if (!window.cocode?.saveGitHubToken) {
      setGitHubSaveState(
        errorApiState(new Error("Desktop secure storage is unavailable")),
      );
      return;
    }
    if (!githubToken.trim()) {
      setGitHubSaveState(errorApiState(new Error("GitHub token is required")));
      return;
    }
    setGitHubSaveState(loadingApiState());
    const state = await loadApiResource(() =>
      window.cocode!.saveGitHubToken({
        token: githubToken,
        displayName: githubDisplayName.trim() || undefined,
      }),
    );
    setGitHubSaveState(state);
    if (state.status === "success") {
      setGitHubCredential(state);
      setGitHubToken("");
      setGitHubDisplayName("");
    }
  }

  async function deleteGitHubToken() {
    if (!window.cocode?.deleteGitHubToken) {
      setGitHubDeleteState(
        errorApiState(new Error("Desktop secure storage is unavailable")),
      );
      return;
    }
    setGitHubDeleteState(loadingApiState());
    const state = await loadApiResource(() =>
      window.cocode!.deleteGitHubToken(),
    );
    setGitHubDeleteState(state);
    if (state.status === "success") {
      setGitHubCredential(successApiState({ configured: false }));
    }
  }

  async function reloadReviewRules() {
    if (!client || !activeWorkspace) {
      setReviewRules(successApiState<ReviewRuleListResponse>({ items: [] }));
      return;
    }
    setReviewRules(loadingApiState());
    const state = await loadApiResource(() =>
      client.listReviewRules(activeWorkspace.id),
    );
    setReviewRules(state);
  }

  async function createReviewRule() {
    if (!client || !activeWorkspace) {
      setReviewRuleAction(
        errorApiState(new Error("Open a project before saving rules")),
      );
      return;
    }
    const content = reviewRuleDraft.content.trim();
    if (!content) {
      setReviewRuleAction(errorApiState(new Error("Rule content is required")));
      return;
    }
    setReviewRuleAction(loadingApiState());
    const state = await loadApiResource(() =>
      client.createReviewRule(activeWorkspace.id, {
        scope: reviewRuleDraft.scope,
        rule_type: reviewRuleDraft.ruleType,
        content,
        enabled: reviewRuleDraft.enabled,
      }),
    );
    setReviewRuleAction(state);
    if (state.status === "success") {
      setReviewRules((current) => upsertReviewRuleState(current, state.data));
      setReviewRuleDraft(defaultReviewRuleDraft());
    }
  }

  async function toggleReviewRule(rule: ReviewRule, enabled: boolean) {
    if (!client) {
      setReviewRuleAction(
        errorApiState(new Error("Backend client is unavailable")),
      );
      return;
    }
    setReviewRuleAction(loadingApiState());
    const state = await loadApiResource(() =>
      client.setReviewRuleEnabled(rule.id, { enabled }),
    );
    setReviewRuleAction(state);
    if (state.status === "success") {
      setReviewRules((current) => upsertReviewRuleState(current, state.data));
    }
  }

  async function deleteReviewRule(rule: ReviewRule) {
    if (!client) {
      setReviewRuleAction(
        errorApiState(new Error("Backend client is unavailable")),
      );
      return;
    }
    setReviewRuleAction(loadingApiState());
    const state = await loadApiResource(() => client.deleteReviewRule(rule.id));
    setReviewRuleAction(state);
    if (state.status === "success") {
      setReviewRules((current) => removeReviewRuleState(current, rule.id));
    }
  }

  async function exportWorkspaceSettings() {
    if (!client || !activeWorkspace) {
      setSettingsPortabilityState(
        errorApiState(new Error("Open a project before exporting settings")),
      );
      return;
    }
    setSettingsPortabilityState(loadingApiState());
    const state = await loadApiResource(() =>
      client.exportWorkspaceSettings(activeWorkspace.id),
    );
    setSettingsPortabilityState(state);
    if (state.status === "success") {
      const text = JSON.stringify(state.data, null, 2);
      setSettingsExportText(text);
      setSettingsImportText(text);
    }
  }

  async function importWorkspaceSettings() {
    if (!client || !activeWorkspace) {
      setSettingsPortabilityState(
        errorApiState(new Error("Open a project before importing settings")),
      );
      return;
    }
    let payload: SettingsExportPayload;
    try {
      payload = JSON.parse(settingsImportText) as SettingsExportPayload;
    } catch (error) {
      setSettingsPortabilityState(errorApiState(error));
      return;
    }
    setSettingsPortabilityState(loadingApiState());
    const state = await loadApiResource(() =>
      client.importWorkspaceSettings(activeWorkspace.id, {
        payload,
        collision_policy: settingsCollisionPolicy,
      }),
    );
    setSettingsPortabilityState(state);
    if (state.status === "success") {
      const [configState, ruleState] = await Promise.all([
        loadApiResource(() => client.listAgentConfigs()),
        loadApiResource(() => client.listReviewRules(activeWorkspace.id)),
      ]);
      setConfigs(configState);
      setReviewRules(ruleState);
    }
  }

  async function saveAgentConfig() {
    if (!client) {
      setSaveState(errorApiState(new Error("Backend client is unavailable")));
      return;
    }

    const bodyResult = agentConfigBodyFromForm(form);
    if (bodyResult instanceof Error) {
      setSaveState(errorApiState(bodyResult));
      return;
    }

    setSaveState(loadingApiState());
    const nextState =
      formMode === "edit" && form.id
        ? await loadApiResource(() =>
            client.updateAgentConfig(form.id as string, bodyResult),
          )
        : await loadApiResource(() => client.createAgentConfig(bodyResult));

    setSaveState(nextState);
    if (nextState.status !== "success") {
      return;
    }

    setFormMode("edit");
    setForm(formFromAgentConfig(nextState.data));
    setConfigs((current) => {
      if (current.status !== "success") {
        return successApiState([nextState.data]);
      }
      const exists = current.data.some((item) => item.id === nextState.data.id);
      return successApiState(
        exists
          ? current.data.map((item) =>
              item.id === nextState.data.id ? nextState.data : item,
            )
          : [nextState.data, ...current.data],
      );
    });
  }

  async function testHealth(configId: string) {
    if (!client) {
      return;
    }
    setHealthByConfigId((current) => ({
      ...current,
      [configId]: loadingApiState(),
    }));
    const state = await loadApiResource(() => client.testAgentConfig(configId));
    setHealthByConfigId((current) => ({ ...current, [configId]: state }));
  }

  return (
    <section className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
      <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
        <div className="mx-auto flex max-w-6xl flex-col gap-5">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <h1 className="text-xl font-semibold">Settings</h1>
              <p className="text-muted-foreground mt-1 text-sm">
                Pick the local CLIs cocode can run. Project rules and
                portability stay tucked away until you need them.
              </p>
            </div>
            <Button variant="outline" onClick={onBack}>
              New thread
              <ArrowUpIcon data-icon="inline-end" />
            </Button>
          </div>

          <div className="grid grid-cols-[320px_minmax(0,1fr)] gap-4">
            <div className="flex min-w-0 flex-col gap-4">
              <section className="bg-card border-border-subtle rounded-lg border">
                <div className="border-b px-3 py-2 text-sm font-medium">
                  Available CLIs
                </div>
                <div className="flex flex-col gap-1 p-2">
                  {presets.status === "loading" && <LoadingRows rows={4} />}
                  {presets.status === "error" && (
                    <ErrorState
                      className="border-0 p-2"
                      title="Presets unavailable"
                      description={presets.error.message}
                    />
                  )}
                  {presets.status === "success" && presetList.length === 0 && (
                    <EmptyState
                      className="border-0 p-3"
                      title="No presets available"
                      description="Preset metadata did not return any CLI templates."
                      icon={TerminalIcon}
                    />
                  )}
                  {presetList.map((preset) => (
                    <button
                      key={preset.id}
                      className={cn(
                        "hover:bg-muted flex w-full items-start gap-3 rounded-md px-2 py-2 text-left text-sm transition-colors",
                        formMode === "create" &&
                          form.sourcePresetId === preset.id &&
                          "bg-muted",
                      )}
                      type="button"
                      onClick={() => selectPreset(preset)}
                    >
                      <TerminalIcon />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate font-medium">
                          {preset.name}
                        </span>
                        <span className="text-muted-foreground mt-1 line-clamp-2 text-xs">
                          {preset.description}
                        </span>
                      </span>
                      <Badge
                        variant={
                          agentEgress(preset) === "local"
                            ? "outline"
                            : "secondary"
                        }
                      >
                        {agentProvider(preset)}
                      </Badge>
                    </button>
                  ))}
                </div>
              </section>

              <section className="bg-card border-border-subtle rounded-lg border">
                <div className="flex items-center justify-between gap-2 border-b px-3 py-2">
                  <span className="text-sm font-medium">Saved connections</span>
                  {configs.status === "success" && (
                    <Badge variant="secondary">{configs.data.length}</Badge>
                  )}
                </div>
                <div className="flex flex-col gap-1 p-2">
                  {configs.status === "loading" && <LoadingRows rows={4} />}
                  {configs.status === "error" && (
                    <ErrorState
                      className="border-0 p-2"
                      title="Agents unavailable"
                      description={configs.error.message}
                    />
                  )}
                  {configs.status === "success" && configList.length === 0 && (
                    <EmptyState
                      className="border-0 p-3"
                      title="No saved agents"
                      description="Choose a preset, adjust the command, and save it."
                      icon={TerminalIcon}
                    />
                  )}
                  {configList.map((config) => (
                    <button
                      key={config.id}
                      className={cn(
                        "hover:bg-muted flex w-full items-center gap-3 rounded-md px-2 py-2 text-left text-sm transition-colors",
                        formMode === "edit" &&
                          form.id === config.id &&
                          "bg-muted",
                      )}
                      type="button"
                      onClick={() => selectConfig(config)}
                    >
                      <BotIcon />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate font-medium">
                          {config.name}
                        </span>
                        <span className="text-muted-foreground mt-1 flex min-w-0 items-center gap-1 text-xs">
                          <span className="truncate">
                            {config.command || config.adapter_kind}
                          </span>
                          <span>•</span>
                          <span>{config.output_mode}</span>
                        </span>
                      </span>
                      <Badge variant={config.enabled ? "secondary" : "outline"}>
                        {config.enabled ? "enabled" : "off"}
                      </Badge>
                    </button>
                  ))}
                </div>
              </section>
            </div>

            <section className="bg-card border-border-subtle min-w-0 rounded-lg border">
              <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
                <div className="min-w-0">
                  <div className="truncate text-sm font-medium">
                    {formMode === "edit"
                      ? "Edit CLI agent"
                      : "Create CLI agent"}
                  </div>
                  <div className="text-muted-foreground mt-1 truncate text-xs">
                    Flags stay in args. The backend validates command safety and
                    output compatibility.
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <Badge
                    variant={
                      agentEgress(form) === "local" ? "outline" : "secondary"
                    }
                  >
                    {agentProvider(form)}
                  </Badge>
                  <Badge
                    variant={
                      form.capabilities.can_write ? "destructive" : "outline"
                    }
                  >
                    {form.capabilities.can_write
                      ? "write-capable"
                      : "review-safe"}
                  </Badge>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4 p-4">
                <label className="flex flex-col gap-2 text-sm font-medium">
                  Name
                  <Input
                    value={form.name}
                    onChange={(event) =>
                      setForm((current) => ({
                        ...current,
                        name: event.target.value,
                      }))
                    }
                  />
                </label>
                <label className="flex flex-col gap-2 text-sm font-medium">
                  Role
                  <Input
                    value={form.role}
                    onChange={(event) =>
                      setForm((current) => ({
                        ...current,
                        role: event.target.value,
                      }))
                    }
                  />
                </label>
                <label className="flex flex-col gap-2 text-sm font-medium">
                  Command
                  <Input
                    placeholder="codex"
                    value={form.command}
                    onChange={(event) =>
                      setForm((current) => ({
                        ...current,
                        command: event.target.value,
                      }))
                    }
                  />
                </label>
                <label className="flex flex-col gap-2 text-sm font-medium">
                  Output mode
                  <NativeSelect
                    className="w-full"
                    value={form.outputMode}
                    onChange={(event) =>
                      setForm((current) => ({
                        ...current,
                        outputMode: event.target.value,
                      }))
                    }
                  >
                    {outputModes.map((mode) => (
                      <NativeSelectOption key={mode} value={mode}>
                        {mode}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                </label>

                <AgentSettingSwitch
                  checked={form.enabled}
                  label="Enabled"
                  onCheckedChange={(checked) =>
                    setForm((current) => ({ ...current, enabled: checked }))
                  }
                />

                <div className="flex items-end justify-end">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() =>
                      setShowAdvancedAgentSettings((current) => !current)
                    }
                  >
                    {showAdvancedAgentSettings
                      ? "Hide advanced"
                      : "Advanced CLI settings"}
                    <ChevronDownIcon
                      className={cn(
                        "transition-transform",
                        showAdvancedAgentSettings && "rotate-180",
                      )}
                      data-icon="inline-end"
                    />
                  </Button>
                </div>

                {showAdvancedAgentSettings && (
                  <>
                    <label className="col-span-2 flex flex-col gap-2 text-sm font-medium">
                      Arguments
                      <InputGroup className="min-h-24 items-stretch">
                        <InputGroupTextarea
                          className="min-h-20 font-mono text-xs"
                          placeholder={"exec\n--json\n-"}
                          value={form.argsText}
                          onChange={(event) =>
                            setForm((current) => ({
                              ...current,
                              argsText: event.target.value,
                            }))
                          }
                        />
                      </InputGroup>
                      <span className="text-muted-foreground text-xs font-normal">
                        One argument per line. Use {"{{prompt}}"} only for
                        arg-mode CLIs.
                      </span>
                    </label>
                    <label className="flex flex-col gap-2 text-sm font-medium">
                      CWD mode
                      <NativeSelect
                        className="w-full"
                        value={form.cwdMode}
                        onChange={(event) =>
                          setForm((current) => ({
                            ...current,
                            cwdMode: event.target.value,
                          }))
                        }
                      >
                        <NativeSelectOption value="repo_root">
                          Repository root
                        </NativeSelectOption>
                        <NativeSelectOption value="workspace_root">
                          Project root
                        </NativeSelectOption>
                      </NativeSelect>
                    </label>
                    <label className="flex flex-col gap-2 text-sm font-medium">
                      Prompt delivery
                      <NativeSelect
                        className="w-full"
                        value={form.promptDelivery}
                        onChange={(event) =>
                          setForm((current) => ({
                            ...current,
                            promptDelivery: event.target
                              .value as PromptDelivery,
                          }))
                        }
                      >
                        <NativeSelectOption value="stdin">
                          stdin
                        </NativeSelectOption>
                        <NativeSelectOption value="arg">arg</NativeSelectOption>
                        <NativeSelectOption value="temp_file">
                          temp_file
                        </NativeSelectOption>
                      </NativeSelect>
                    </label>
                    <label className="flex flex-col gap-2 text-sm font-medium">
                      Model label
                      <Input
                        value={form.modelLabel}
                        onChange={(event) =>
                          setForm((current) => ({
                            ...current,
                            modelLabel: event.target.value,
                          }))
                        }
                      />
                    </label>
                    <label className="flex flex-col gap-2 text-sm font-medium">
                      Reasoning label
                      <Input
                        value={form.reasoningLabel}
                        onChange={(event) =>
                          setForm((current) => ({
                            ...current,
                            reasoningLabel: event.target.value,
                          }))
                        }
                      />
                    </label>
                    <label className="flex flex-col gap-2 text-sm font-medium">
                      Timeout seconds
                      <Input
                        min={1}
                        type="number"
                        value={form.timeoutSeconds}
                        onChange={(event) =>
                          setForm((current) => ({
                            ...current,
                            timeoutSeconds: Number(event.target.value),
                          }))
                        }
                      />
                    </label>
                    <label className="flex flex-col gap-2 text-sm font-medium">
                      Version args
                      <Input
                        placeholder="--version"
                        value={form.versionArgsText}
                        onChange={(event) =>
                          setForm((current) => ({
                            ...current,
                            versionArgsText: event.target.value,
                          }))
                        }
                      />
                    </label>
                    <label className="col-span-2 flex flex-col gap-2 text-sm font-medium">
                      Environment allowlist
                      <Input
                        placeholder="OPENAI_API_KEY, GEMINI_API_KEY"
                        value={form.envAllowlistText}
                        onChange={(event) =>
                          setForm((current) => ({
                            ...current,
                            envAllowlistText: event.target.value,
                          }))
                        }
                      />
                    </label>
                    <label className="col-span-2 flex flex-col gap-2 text-sm font-medium">
                      Credential refs
                      <InputGroup className="min-h-20 items-stretch">
                        <InputGroupTextarea
                          className="min-h-16 font-mono text-xs"
                          placeholder={"OPENAI_API_KEY=credential:openai"}
                          value={form.credentialRefsText}
                          onChange={(event) =>
                            setForm((current) => ({
                              ...current,
                              credentialRefsText: event.target.value,
                            }))
                          }
                        />
                      </InputGroup>
                      <span className="text-muted-foreground text-xs font-normal">
                        References only. Secret values stay in desktop safe
                        storage or each CLI provider's own auth store.
                      </span>
                    </label>

                    <div className="col-span-2 grid grid-cols-2 gap-3">
                      <AgentSettingSwitch
                        checked={form.skipVersion}
                        label="Skip version"
                        onCheckedChange={(checked) =>
                          setForm((current) => ({
                            ...current,
                            skipVersion: checked,
                          }))
                        }
                      />
                      <AgentSettingSwitch
                        checked={form.allowRiskyCommand}
                        label="Risky command"
                        onCheckedChange={(checked) =>
                          setForm((current) => ({
                            ...current,
                            allowRiskyCommand: checked,
                          }))
                        }
                      />
                    </div>
                  </>
                )}

                <div className="col-span-2 rounded-md border p-3">
                  <div className="mb-3 flex items-center justify-between gap-3">
                    <div className="text-sm font-medium">Health check</div>
                    <div className="flex items-center gap-2">
                      {form.id && (
                        <Button
                          disabled={activeHealth?.status === "loading"}
                          size="sm"
                          variant="outline"
                          onClick={() => void testHealth(form.id as string)}
                        >
                          <ClockIcon data-icon="inline-start" />
                          {activeHealth?.status === "loading"
                            ? "Testing..."
                            : "Test"}
                        </Button>
                      )}
                      <Button
                        disabled={saveState.status === "loading"}
                        size="sm"
                        onClick={() => void saveAgentConfig()}
                      >
                        <CheckIcon data-icon="inline-start" />
                        {saveState.status === "loading" ? "Saving..." : "Save"}
                      </Button>
                    </div>
                  </div>
                  {!form.id && (
                    <div className="text-muted-foreground text-sm">
                      Save this config before running command health checks.
                    </div>
                  )}
                  {activeHealth?.status === "success" && (
                    <HealthSummary health={activeHealth.data} />
                  )}
                  {activeHealth?.status === "error" && (
                    <ErrorState
                      className="border-0 p-0"
                      title="Health check failed"
                      description={activeHealth.error.message}
                    />
                  )}
                  {saveState.status === "error" && (
                    <ErrorState
                      className="mt-3"
                      title="Could not save agent"
                      description={saveState.error.message}
                    />
                  )}
                  {saveState.status === "success" && (
                    <div className="text-muted-foreground mt-3 text-xs">
                      Saved {saveState.data.name}. Run a health check to verify
                      the local command and optional version output.
                    </div>
                  )}
                </div>
              </div>
            </section>
          </div>

          <section className="bg-card border-border-subtle rounded-lg border">
            <button
              className="hover:bg-muted/70 flex w-full items-center justify-between gap-3 rounded-t-lg px-4 py-3 text-left transition-colors"
              type="button"
              onClick={() => setShowProjectSettings((current) => !current)}
            >
              <span className="min-w-0">
                <span className="block text-sm font-medium">
                  Project settings
                </span>
                <span className="text-muted-foreground mt-1 block truncate text-xs">
                  GitHub credentials, remembered review rules, and portable JSON
                  export.
                </span>
              </span>
              <span className="flex shrink-0 items-center gap-2">
                <Badge variant="secondary">
                  {enabledConfigCount} CLI
                  {enabledConfigCount === 1 ? "" : "s"} enabled
                </Badge>
                <ChevronDownIcon
                  className={cn(
                    "size-4 transition-transform",
                    showProjectSettings && "rotate-180",
                  )}
                />
              </span>
            </button>
            {showProjectSettings && (
              <div className="flex flex-col gap-4 border-t p-4">
                <GitHubCredentialPanel
                  deleteState={githubDeleteState}
                  displayName={githubDisplayName}
                  saveState={githubSaveState}
                  status={githubCredential}
                  token={githubToken}
                  onDelete={() => void deleteGitHubToken()}
                  onDisplayNameChange={setGitHubDisplayName}
                  onSave={() => void saveGitHubToken()}
                  onTokenChange={setGitHubToken}
                />

                <ReviewRuleMemoryPanel
                  actionState={reviewRuleAction}
                  draft={reviewRuleDraft}
                  rules={reviewRules}
                  workspace={activeWorkspace}
                  onCreate={() => void createReviewRule()}
                  onDelete={(rule) => void deleteReviewRule(rule)}
                  onDraftChange={setReviewRuleDraft}
                  onReload={() => void reloadReviewRules()}
                  onToggle={(rule, enabled) =>
                    void toggleReviewRule(rule, enabled)
                  }
                />

                <SettingsPortabilityPanel
                  collisionPolicy={settingsCollisionPolicy}
                  exportText={settingsExportText}
                  importText={settingsImportText}
                  state={settingsPortabilityState}
                  workspace={activeWorkspace}
                  onCollisionPolicyChange={setSettingsCollisionPolicy}
                  onExport={() => void exportWorkspaceSettings()}
                  onImport={() => void importWorkspaceSettings()}
                  onImportTextChange={setSettingsImportText}
                />
              </div>
            )}
          </section>
        </div>
      </div>
    </section>
  );
}
