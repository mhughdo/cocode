import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ChevronDownIcon,
  ExternalLinkIcon,
  GitPullRequestIcon,
  PanelRightCloseIcon,
  PanelRightOpenIcon,
  PlayIcon,
  PlusIcon,
  RefreshCwIcon,
  SearchIcon,
  UsersIcon,
} from "lucide-react";

import { ErrorState } from "@/components/app/chrome";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  type AgentConfig,
  type AgentModelCatalog,
  type ApiClient,
  errorApiState,
  idleApiState,
  loadApiResource,
  loadingApiState,
  type Loadable,
  type Repository,
  type RepositoryBranch,
  type ReviewSession,
  type Snapshot,
  successApiState,
  type Workspace,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import { ReviewFocusComposer } from "./review-focus-composer";
import {
  AgentProviderGlyph,
  SetupAgentRow,
  SetupAgentSelector,
  SetupBranchSelector,
  SetupFocusChip,
  SetupPresetTile,
  SetupScopeRow,
  SetupSegment,
  SetupStepPanel,
} from "./setup-controls";
import {
  type ManualReviewAgentAssignment,
  type SnapshotSource,
  type SetupFocusFileMention,
  removeRecordKey,
  setupDefaultBaseRef,
  setupFocusHintById,
  setupFocusOptions,
  setupFocusPrompt,
  setupManualReviewAgentAssignments,
  setupPresetOptions,
  setupPrimaryPresetIds,
  setupReviewAgentAssignments,
  setupReviewRoleById,
  setupReviewRoleOptions,
  setupRoleIdsForPresets,
  setupRuntimeLimitSeconds,
  setupSourceKey,
  setupSourceOptions,
  snapshotTitle,
  toggleSetValue,
} from "./setup-model";
import { SetupSourceInspectorPanel } from "./setup-source-preview";
import { useSetupSourceInspectorPanel } from "./setup-source-inspector-state";
import { useSetupSourcePreviewState } from "./setup-source-preview-state";

import {
  buildSetupAgentSelection,
  formatSetupAgentChoiceLabel,
  type SetupAgentModelChoice,
} from "../agents/agent-utils";

type GitHubSnapshotAuthMethod = "token" | "gh_cli";

export function NewThreadScreen({
  activeRepository,
  activeWorkspace,
  agentConfigs,
  agentModelCatalogs,
  client,
  onOpenRepository,
  onSetupContextChange,
  onReviewStarted,
}: {
  activeRepository?: Repository;
  activeWorkspace?: Workspace;
  agentConfigs: Loadable<AgentConfig[]>;
  agentModelCatalogs: Loadable<AgentModelCatalog[]>;
  client: ApiClient | null;
  onOpenRepository: () => void;
  onSetupContextChange?: (context: {
    branch?: string;
    subtitle?: string;
    title?: string;
  }) => void;
  onReviewStarted: (session: ReviewSession) => void;
}) {
  const [source, setSource] = useState<SnapshotSource>("github");
  const [githubUrl, setGitHubUrl] = useState("");
  const [githubAuthMethod, setGithubAuthMethod] =
    useState<GitHubSnapshotAuthMethod>("token");
  const [baseRefInput, setBaseRefInput] = useState("");
  const [headRef, setHeadRef] = useState("");
  const [focusPrompt, setFocusPrompt] = useState("");
  const [focusFiles, setFocusFiles] = useState<SetupFocusFileMention[]>([]);
  const [reviewDepth, setReviewDepth] = useState<"quick" | "standard" | "deep">(
    "standard",
  );
  const [selectedFocusIds, setSelectedFocusIds] = useState(
    () => new Set<string>(),
  );
  const [selectedPresetIds, setSelectedPresetIds] = useState(
    () => new Set<string>(),
  );
  const [presetSearch, setPresetSearch] = useState("");
  const [selectedAgentIds, setSelectedAgentIds] = useState<Set<string> | null>(
    null,
  );
  const [orchestratorAgentId, setOrchestratorAgentId] = useState("");
  const [agentModelChoices, setAgentModelChoices] = useState<
    Record<string, SetupAgentModelChoice>
  >({});
  const [reviewAgentModelChoices, setReviewAgentModelChoices] = useState<
    Record<string, SetupAgentModelChoice>
  >({});
  const [agentRoleChoices, setAgentRoleChoices] = useState<
    Record<string, string>
  >({});
  const [hiddenReviewAssignmentIds, setHiddenReviewAssignmentIds] = useState(
    () => new Set<string>(),
  );
  const [manualReviewAssignments, setManualReviewAssignments] = useState<
    ManualReviewAgentAssignment[]
  >([]);
  const manualReviewAssignmentSequence = useRef(0);
  const branchRequestSequence = useRef(0);
  const [branchState, setBranchState] =
    useState<Loadable<RepositoryBranch[]>>(idleApiState());
  const {
    close: closeSourceInspector,
    layoutActive: sourceInspectorLayoutActive,
    layoutRef: sourceInspectorLayoutRef,
    layoutStyle: sourceInspectorLayoutStyle,
    open: sourceInspectorOpen,
    openCount: sourceInspectorOpenCount,
    panelStyle: sourceInspectorPanelStyle,
    rendered: sourceInspectorRendered,
    resizing: sourceInspectorResizing,
    startResize: startSourceInspectorResize,
    toggle: toggleSourceInspector,
    visualOpen: sourceInspectorVisualOpen,
  } = useSetupSourceInspectorPanel();
  const [localError, setLocalError] = useState("");
  const [startState, setStartState] =
    useState<Loadable<ReviewSession>>(idleApiState());

  const canCreate = Boolean(activeWorkspace && activeRepository);
  const safeAgents = useMemo(
    () =>
      agentConfigs.status === "success"
        ? agentConfigs.data.filter(
            (agent) => agent.enabled && !agent.capabilities.can_write,
          )
        : [],
    [agentConfigs],
  );
  const modelCatalogs = useMemo(
    () =>
      agentModelCatalogs.status === "success" ? agentModelCatalogs.data : [],
    [agentModelCatalogs],
  );

  const activeWorkspaceId = activeWorkspace?.id ?? "";
  const activeRepositoryId = activeRepository?.id ?? "";
  const refreshBranches = useCallback(() => {
    if (!client || !activeWorkspaceId || !activeRepositoryId) {
      branchRequestSequence.current += 1;
      setBranchState(idleApiState());
      return;
    }

    const requestId = (branchRequestSequence.current += 1);
    setBranchState((current) =>
      current.status === "success" ? current : loadingApiState(),
    );
    void loadApiResource(() =>
      client.listRepositoryBranches(activeRepositoryId, {
        workspaceId: activeWorkspaceId,
      }),
    ).then((state) => {
      if (branchRequestSequence.current === requestId) {
        setBranchState(state);
      }
    });
  }, [activeRepositoryId, activeWorkspaceId, client]);

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (!canceled) {
        refreshBranches();
      }
    });
    return () => {
      canceled = true;
    };
  }, [refreshBranches]);
  const defaultAgentIds = useMemo(
    () => new Set(safeAgents.slice(0, 4).map((agent) => agent.id)),
    [safeAgents],
  );
  const effectiveSelectedAgentIds = selectedAgentIds ?? defaultAgentIds;
  const effectiveOrchestratorAgentId = safeAgents.some(
    (agent) => agent.id === orchestratorAgentId,
  )
    ? orchestratorAgentId
    : (safeAgents[0]?.id ?? "");
  const reviewRoleIds = useMemo(
    () => setupRoleIdsForPresets(selectedPresetIds),
    [selectedPresetIds],
  );
  const selectedReviewPoolIds = useMemo(() => {
    const safeIds = new Set(safeAgents.map((agent) => agent.id));
    const selectedIds = Array.from(effectiveSelectedAgentIds).filter((id) =>
      safeIds.has(id),
    );
    const nonOrchestrator = selectedIds.filter(
      (id) => id !== effectiveOrchestratorAgentId,
    );
    if (nonOrchestrator.length > 0) {
      return nonOrchestrator;
    }
    if (
      effectiveOrchestratorAgentId &&
      safeIds.has(effectiveOrchestratorAgentId)
    ) {
      return [effectiveOrchestratorAgentId];
    }
    return selectedIds;
  }, [effectiveOrchestratorAgentId, effectiveSelectedAgentIds, safeAgents]);
  const presetReviewAgentAssignments = useMemo(
    () =>
      setupReviewAgentAssignments(
        safeAgents,
        selectedReviewPoolIds,
        reviewRoleIds,
      ).filter((assignment) => !hiddenReviewAssignmentIds.has(assignment.id)),
    [
      hiddenReviewAssignmentIds,
      reviewRoleIds,
      safeAgents,
      selectedReviewPoolIds,
    ],
  );
  const manualReviewAgentAssignments = useMemo(
    () =>
      setupManualReviewAgentAssignments(
        safeAgents,
        manualReviewAssignments,
        presetReviewAgentAssignments.length,
      ),
    [manualReviewAssignments, presetReviewAgentAssignments.length, safeAgents],
  );
  const reviewAgentAssignments = useMemo(
    () => [...presetReviewAgentAssignments, ...manualReviewAgentAssignments],
    [manualReviewAgentAssignments, presetReviewAgentAssignments],
  );
  const agentSelectionsForRun = useMemo(() => {
    const selections = [];
    const orchestrator = safeAgents.find(
      (agent) => agent.id === effectiveOrchestratorAgentId,
    );
    if (orchestrator) {
      selections.push(
        buildSetupAgentSelection(
          orchestrator,
          agentModelChoices,
          modelCatalogs,
          "Orchestrator",
        ),
      );
    }
    for (const assignment of reviewAgentAssignments) {
      const role =
        setupReviewRoleById(agentRoleChoices[assignment.id]) ?? assignment.role;
      const modelChoice = reviewAgentModelChoices[assignment.id];
      selections.push(
        buildSetupAgentSelection(
          assignment.agent,
          modelChoice ? { [assignment.agent.id]: modelChoice } : {},
          modelCatalogs,
          role.label,
        ),
      );
    }
    return selections;
  }, [
    agentModelChoices,
    agentRoleChoices,
    effectiveOrchestratorAgentId,
    modelCatalogs,
    reviewAgentAssignments,
    reviewAgentModelChoices,
    safeAgents,
  ]);
  const reviewAgentIdsForRun = useMemo(
    () => agentSelectionsForRun.map((selection) => selection.agent_config_id),
    [agentSelectionsForRun],
  );
  const orchestratorAgent =
    safeAgents.find((agent) => agent.id === effectiveOrchestratorAgentId) ??
    reviewAgentAssignments[0]?.agent ??
    safeAgents[0];
  const selectedAgentCount =
    (orchestratorAgent ? 1 : 0) + reviewAgentAssignments.length;
  const selectedFocusAreas = setupFocusOptions
    .filter((item) => selectedFocusIds.has(item.id))
    .map((item) => ({
      id: item.id,
      instruction: setupFocusHintById[item.id] ?? item.label,
      label: item.label,
    }));
  const selectedPresetLabels = setupPresetOptions
    .filter((item) => selectedPresetIds.has(item.id))
    .map((item) => item.title);
  const visiblePresetOptions = useMemo(() => {
    const visibleIds = new Set(setupPrimaryPresetIds);
    return setupPresetOptions.filter((preset) => visibleIds.has(preset.id));
  }, []);
  const presetSearchResults = useMemo(() => {
    const query = presetSearch.trim().toLowerCase();
    if (!query) {
      return setupPresetOptions;
    }
    return setupPresetOptions.filter((preset) =>
      [preset.title, preset.subtitle, preset.id]
        .join(" ")
        .toLowerCase()
        .includes(query),
    );
  }, [presetSearch]);
  const branchOptions =
    branchState.status === "success" ? branchState.data : [];
  const currentBranch =
    branchOptions.find((branch) => branch.current && !branch.remote)?.name ??
    branchOptions.find((branch) => branch.current)?.name ??
    "";
  const baseRef =
    baseRefInput || setupDefaultBaseRef(activeRepository, branchOptions);
  const headRefValue = headRef || currentBranch || "HEAD";
  const sourceKey = setupSourceKey({
    repositoryId: activeRepository?.id ?? "",
    source,
    githubUrl,
    githubAuthMethod: source === "github" ? githubAuthMethod : "",
    baseRef: source === "branch-compare" ? baseRef : "",
    headRef: source === "branch-compare" ? headRefValue : "",
  });
  const validateSource = useCallback(() => {
    if (!canCreate) {
      setLocalError("Open a git repository before creating a review.");
      return false;
    }
    if (source === "github" && githubUrl.trim() === "") {
      setLocalError("Enter a GitHub pull request URL.");
      return false;
    }
    if (
      source === "branch-compare" &&
      (baseRef.trim() === "" || headRefValue.trim() === "")
    ) {
      setLocalError("Choose both base and head branches.");
      return false;
    }
    setLocalError("");
    return true;
  }, [baseRef, canCreate, githubUrl, headRefValue, source]);

  const createSourceSnapshot = useCallback((): Promise<Snapshot> => {
    if (!client || !activeWorkspace || !activeRepository) {
      throw new Error("Open a project before creating a review snapshot.");
    }
    if (source === "github") {
      if (window.cocode?.createGitHubSnapshot) {
        return window.cocode.createGitHubSnapshot({
          workspaceId: activeWorkspace.id,
          repositoryId: activeRepository.id,
          url: githubUrl.trim(),
          authMethod: githubAuthMethod,
        });
      }
      throw new Error("Desktop GitHub credential bridge is unavailable.");
    }
    if (source === "branch-compare") {
      return client.createLocalCompareSnapshot({
        workspace_id: activeWorkspace.id,
        repository_id: activeRepository.id,
        base_ref: baseRef.trim(),
        head_ref: headRefValue.trim(),
      });
    }
    return client.createLocalChangesSnapshot({
      workspace_id: activeWorkspace.id,
      repository_id: activeRepository.id,
    });
  }, [
    activeRepository,
    activeWorkspace,
    baseRef,
    client,
    githubUrl,
    githubAuthMethod,
    headRefValue,
    source,
  ]);

  const sourcePreviewState = useSetupSourcePreviewState({
    client,
    createSourceSnapshot,
    sourceKey,
    validateSource,
  });
  const sourcePreview = sourcePreviewState.preview;
  const previewReady = sourcePreviewState.ready;
  const previewSnapshot = sourcePreviewState.snapshot;
  const previewFiles = sourcePreviewState.files;
  const previewStats = sourcePreviewState.stats;
  const renderedDiffFileCount = sourcePreviewState.renderedFileCount;
  const filePatchPreviews = sourcePreviewState.patchPreviews;
  const expandedSourceFileIds = sourcePreviewState.expandedFileIds;
  const loadSourcePreview = sourcePreviewState.load;
  const loadMoreDiffFiles = sourcePreviewState.loadMoreFiles;
  const setSourceFileExpanded = sourcePreviewState.setFileExpanded;

  const shouldAutoLoadSourcePreview = useCallback(() => {
    return (
      canCreate &&
      sourcePreview.status !== "loading" &&
      !previewReady &&
      (source !== "github" || isGitHubPullRequestUrl(githubUrl)) &&
      (source !== "branch-compare" || branchState.status === "success")
    );
  }, [
    branchState.status,
    canCreate,
    githubUrl,
    previewReady,
    source,
    sourcePreview.status,
  ]);

  useEffect(() => {
    if (!onSetupContextChange) {
      return;
    }
    const repositoryLabel = activeRepository?.owner
      ? `${activeRepository.owner}/${activeRepository.name}`
      : (activeRepository?.name ??
        activeWorkspace?.name ??
        "No project selected");
    const branchLabel =
      source === "branch-compare"
        ? `${baseRef}..${headRefValue}`
        : source === "local-changes"
          ? currentBranch || activeRepository?.default_branch || "working tree"
          : githubUrl.trim()
            ? "GitHub PR"
            : activeRepository?.default_branch || "main";

    onSetupContextChange({
      branch: branchLabel,
      subtitle: "Set up review",
      title: repositoryLabel,
    });
  }, [
    activeRepository?.default_branch,
    activeRepository?.name,
    activeRepository?.owner,
    activeWorkspace?.name,
    baseRef,
    currentBranch,
    githubUrl,
    headRefValue,
    onSetupContextChange,
    source,
  ]);
  function addManualReviewAgent(agentId: string) {
    const agent = safeAgents.find((item) => item.id === agentId);
    if (!agent) {
      return;
    }
    manualReviewAssignmentSequence.current += 1;
    const role =
      setupReviewRoleById("general-reviewer") ?? setupReviewRoleOptions[0];
    const id = `manual-review-agent:${manualReviewAssignmentSequence.current}`;
    setManualReviewAssignments((current) => [
      ...current,
      { id, agentId: agent.id, roleId: role.id },
    ]);
    setAgentRoleChoices((current) => ({
      ...current,
      [id]: role.id,
    }));
  }

  function togglePreset(presetId: string) {
    setHiddenReviewAssignmentIds(new Set());
    setSelectedPresetIds((current) => toggleSetValue(current, presetId));
  }

  function resetSetup() {
    setSource("github");
    setGitHubUrl("");
    setGithubAuthMethod("token");
    setBaseRefInput("");
    setHeadRef("");
    setFocusPrompt("");
    setFocusFiles([]);
    setReviewDepth("standard");
    setSelectedFocusIds(new Set());
    setSelectedPresetIds(new Set());
    setPresetSearch("");
    setSelectedAgentIds(null);
    setOrchestratorAgentId("");
    setAgentModelChoices({});
    setReviewAgentModelChoices({});
    setAgentRoleChoices({});
    setHiddenReviewAssignmentIds(new Set());
    setManualReviewAssignments([]);
    closeSourceInspector();
    sourcePreviewState.reset();
    setLocalError("");
    setStartState(idleApiState());
  }

  useEffect(() => {
    if (
      !sourceInspectorOpen ||
      sourceInspectorOpenCount === 0 ||
      !shouldAutoLoadSourcePreview()
    ) {
      return;
    }
    const timer = window.setTimeout(() => {
      void loadSourcePreview();
    }, 0);
    return () => window.clearTimeout(timer);
  }, [
    loadSourcePreview,
    shouldAutoLoadSourcePreview,
    sourceInspectorOpen,
    sourceInspectorOpenCount,
  ]);

  async function startReview() {
    if (!client) {
      setStartState(errorApiState(new Error("Backend client is unavailable.")));
      return;
    }
    if (!validateSource()) {
      return;
    }
    if (!activeWorkspace || !activeRepository) {
      setStartState(
        errorApiState(new Error("Open a project before starting review.")),
      );
      return;
    }
    if (reviewAgentIdsForRun.length === 0) {
      setStartState(
        errorApiState(new Error("Select at least one read-only review agent.")),
      );
      return;
    }

    setStartState(loadingApiState());
    const nextSnapshot = previewSnapshot
      ? successApiState(previewSnapshot)
      : await loadApiResource(createSourceSnapshot);
    if (nextSnapshot.status !== "success") {
      setStartState(idleApiState());
      setLocalError(
        nextSnapshot.status === "error"
          ? nextSnapshot.error.message
          : "Snapshot creation did not complete.",
      );
      return;
    }
    setLocalError("");

    const created = await loadApiResource(() =>
      client.createReviewSession({
        workspace_id: activeWorkspace.id,
        snapshot_id: nextSnapshot.data.id,
        title: snapshotTitle(nextSnapshot.data, activeRepository),
        review_depth: reviewDepth,
        focus_prompt: setupFocusPrompt({
          files: focusFiles,
          focusAreas: selectedFocusAreas,
          prompt: focusPrompt,
        }),
        preset: selectedPresetLabels.join(", "),
        agent_config_ids: reviewAgentIdsForRun,
        agent_selections: agentSelectionsForRun,
        runtime_limit_seconds: setupRuntimeLimitSeconds(reviewDepth),
        context_policy: {
          include_prompt_material: true,
          include_changed_code: true,
          include_related_call_sites: true,
          include_related_tests: true,
          include_project_conventions: true,
          include_prior_comments: true,
          include_prior_decisions: true,
          redact_secrets: true,
          focus_paths: focusFiles.map((file) => file.path),
          max_tokens:
            reviewDepth === "deep"
              ? 24_000
              : reviewDepth === "quick"
                ? 10_000
                : 18_000,
          max_items:
            reviewDepth === "deep" ? 260 : reviewDepth === "quick" ? 120 : 200,
        },
      }),
    );
    if (created.status !== "success") {
      setStartState(created);
      return;
    }

    const started = await loadApiResource(() =>
      client.startReviewSession(created.data.id),
    );
    setStartState(started);
    if (started.status === "success") {
      onReviewStarted(started.data);
    }
  }

  return (
    <section className="bg-background flex min-h-0 min-w-0 flex-col overflow-hidden">
      <div className="grid h-full w-full grid-rows-[auto_minmax(0,1fr)]">
        <div className="flex items-start justify-between gap-4 px-6 pt-6 pb-4 min-[900px]:px-8">
          <div className="min-w-0">
            <h1 className="text-2xl leading-tight font-semibold tracking-tight">
              Set up review
            </h1>
            <p className="text-muted-foreground mt-1.5 text-sm">
              Configure the source, context, orchestration, and presets for this
              review.
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-1.5 min-[900px]:gap-2">
            <Button variant="outline" onClick={resetSetup}>
              <RefreshCwIcon data-icon="inline-start" />
              Reset
            </Button>
            <Button
              className="px-4"
              disabled={startState.status === "loading"}
              onClick={() => void startReview()}
            >
              {startState.status === "loading" ? "Starting..." : "Start review"}
              <PlayIcon data-icon="inline-end" />
            </Button>
            <Button
              aria-label={
                sourceInspectorOpen
                  ? "Hide source details"
                  : "Show source details"
              }
              aria-pressed={sourceInspectorOpen}
              className="cursor-pointer"
              size="icon"
              title={
                sourceInspectorOpen
                  ? "Hide source details"
                  : "Show source details"
              }
              variant="outline"
              onClick={toggleSourceInspector}
            >
              {sourceInspectorOpen ? (
                <PanelRightCloseIcon className="size-4" />
              ) : (
                <PanelRightOpenIcon className="size-4" />
              )}
            </Button>
          </div>
        </div>

        <div
          ref={sourceInspectorLayoutRef}
          className={cn(
            "border-border/60 relative grid h-full min-h-0 grid-cols-1 overflow-hidden border-t",
          )}
          style={sourceInspectorLayoutStyle}
        >
          <div
            className={cn(
              "min-w-0 overflow-hidden px-4 py-4 transition-all duration-[220ms] ease-[cubic-bezier(0.16,1,0.3,1)] motion-reduce:transition-none min-[900px]:px-6",
              sourceInspectorLayoutActive &&
                "min-[1180px]:pr-[calc(var(--source-inspector-width)+1.5rem)]",
            )}
          >
            <div className="flex h-full min-h-0 flex-col gap-2.5 overflow-y-auto pr-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
              {!canCreate && (
                <ErrorState
                  title="Open a project first"
                  description="cocode keeps review context grounded in a selected git repository."
                />
              )}

              <SetupStepPanel
                description="Choose where to review from."
                compact={sourceInspectorLayoutActive}
                number={1}
                title="Review source"
              >
                <div className="grid grid-cols-[repeat(auto-fit,minmax(min(175px,100%),1fr))] gap-2">
                  {setupSourceOptions.map((option) => (
                    <SetupSegment
                      key={option.id}
                      active={source === option.id}
                      icon={option.icon}
                      label={option.label}
                      logoUrl={option.logoUrl}
                      onClick={() => setSource(option.id)}
                    />
                  ))}
                </div>

                <div
                  className={cn(
                    "mt-3 grid gap-3",
                    !sourceInspectorLayoutActive &&
                      "min-[1320px]:grid-cols-[minmax(210px,0.36fr)_minmax(0,1fr)]",
                  )}
                >
                  <label className="flex min-w-0 flex-col gap-1.5 text-xs font-medium">
                    Repository
                    <Button
                      className="h-9 justify-between"
                      variant="outline"
                      onClick={onOpenRepository}
                    >
                      <span className="flex min-w-0 items-center gap-2">
                        <GitPullRequestIcon data-icon="inline-start" />
                        <span className="truncate">
                          {activeRepository?.owner
                            ? `${activeRepository.owner}/${activeRepository.name}`
                            : activeRepository?.name || "Open project"}
                        </span>
                      </span>
                      <ChevronDownIcon data-icon="inline-end" />
                    </Button>
                  </label>

                  {source === "github" && (
                    <div className="grid min-w-0 grid-cols-1 gap-3 min-[760px]:grid-cols-[minmax(220px,1fr)_150px]">
                      <label className="flex min-w-0 flex-col gap-1.5 text-xs font-medium">
                        PR URL
                        <div className="relative">
                          <Input
                            aria-label="Pull request URL"
                            className="h-9 pr-9"
                            disabled={!canCreate}
                            id="github-url"
                            placeholder="https://github.com/owner/repo/pull/123"
                            value={githubUrl}
                            onChange={(event) =>
                              setGitHubUrl(event.target.value)
                            }
                          />
                          <ExternalLinkIcon className="text-muted-foreground pointer-events-none absolute top-1/2 right-2.5 size-4 -translate-y-1/2" />
                        </div>
                      </label>
                      <label className="flex min-w-0 flex-col gap-1.5 text-xs font-medium">
                        Access
                        <NativeSelect
                          className="h-9"
                          value={githubAuthMethod}
                          onChange={(event) =>
                            setGithubAuthMethod(
                              event.target.value as GitHubSnapshotAuthMethod,
                            )
                          }
                        >
                          <NativeSelectOption value="token">
                            Saved token
                          </NativeSelectOption>
                          <NativeSelectOption value="gh_cli">
                            gh CLI
                          </NativeSelectOption>
                        </NativeSelect>
                      </label>
                    </div>
                  )}

                  {source === "local-changes" && (
                    <div className="flex min-w-0 items-end">
                      <div className="bg-surface-muted flex h-9 w-full min-w-0 items-center rounded-lg border px-3 text-[0.76rem] leading-4">
                        <span className="truncate">
                          <span className="font-medium">Working tree</span>
                          <span className="text-muted-foreground">
                            {" "}
                            staged, unstaged, and untracked files.
                          </span>
                        </span>
                      </div>
                    </div>
                  )}

                  {source === "branch-compare" && (
                    <div
                      className={cn(
                        "grid grid-cols-1 gap-3",
                        !sourceInspectorLayoutActive &&
                          "min-[760px]:grid-cols-2",
                      )}
                    >
                      <SetupBranchSelector
                        branches={branchState}
                        disabled={!canCreate}
                        label="Base branch"
                        onRefresh={refreshBranches}
                        value={baseRef}
                        onSelect={setBaseRefInput}
                      />
                      <SetupBranchSelector
                        branches={branchState}
                        disabled={!canCreate}
                        label="Head branch"
                        onRefresh={refreshBranches}
                        value={headRefValue}
                        onSelect={setHeadRef}
                      />
                    </div>
                  )}
                </div>
              </SetupStepPanel>

              <SetupStepPanel
                description="Add instructions, files, docs, and optional review lenses."
                compact={sourceInspectorLayoutActive}
                number={2}
                title="Review focus"
              >
                <ReviewFocusComposer
                  client={client}
                  disabled={!canCreate}
                  repositoryId={activeRepository?.id}
                  selectedFiles={focusFiles}
                  value={focusPrompt}
                  workspaceId={activeWorkspace?.id}
                  onSelectedFilesChange={setFocusFiles}
                  onValueChange={setFocusPrompt}
                />
                <div className="mt-2 flex flex-wrap gap-2">
                  {setupFocusOptions.map((item) => (
                    <SetupFocusChip
                      key={item.id}
                      active={selectedFocusIds.has(item.id)}
                      icon={item.icon}
                      label={item.label}
                      onClick={() =>
                        setSelectedFocusIds((current) =>
                          toggleSetValue(current, item.id),
                        )
                      }
                    />
                  ))}
                </div>
              </SetupStepPanel>

              <SetupStepPanel
                description="Choose the orchestrator and agents that will run your review."
                compact={sourceInspectorLayoutActive}
                number={3}
                title="Orchestration"
              >
                <div
                  className={cn(
                    "grid items-start gap-4",
                    !sourceInspectorLayoutActive &&
                      "lg:grid-cols-[minmax(160px,0.35fr)_minmax(0,1fr)]",
                  )}
                >
                  <div className="min-w-0">
                    <div className="mb-1.5 flex h-7 items-center">
                      <label className="block text-[0.78rem] font-medium">
                        Orchestrator
                      </label>
                    </div>
                    <SetupAgentSelector
                      agents={safeAgents}
                      catalogs={modelCatalogs}
                      choices={agentModelChoices}
                      disabled={agentConfigs.status === "loading"}
                      placeholder="Select orchestrator"
                      selectedAgentId={effectiveOrchestratorAgentId}
                      onSelect={(agentId, choice) => {
                        setOrchestratorAgentId(agentId);
                        setAgentModelChoices((current) => ({
                          ...current,
                          [agentId]: choice,
                        }));
                      }}
                    />
                    <p className="text-muted-foreground mt-1.5 text-[0.72rem]">
                      Coordinates the review and delegates focused checks.
                    </p>
                    {agentModelCatalogs.status === "loading" && (
                      <p className="text-muted-foreground mt-2 text-xs">
                        Loading available CLI models...
                      </p>
                    )}
                    {agentModelCatalogs.status === "error" && (
                      <p className="text-destructive mt-2 text-xs">
                        {agentModelCatalogs.error.message}
                      </p>
                    )}
                  </div>

                  <div className="min-w-0">
                    <div className="mb-1.5 flex h-7 items-center justify-between gap-2">
                      <label className="block text-[0.78rem] font-medium whitespace-nowrap">
                        Review agents
                      </label>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            className="h-7 cursor-pointer px-2 text-[0.72rem]"
                            size="sm"
                            variant="outline"
                          >
                            <PlusIcon data-icon="inline-start" />
                            Add agent
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-80">
                          <DropdownMenuLabel>
                            Add review agent
                          </DropdownMenuLabel>
                          {safeAgents.map((agent) => (
                            <DropdownMenuItem
                              key={agent.id}
                              className="cursor-pointer"
                              onSelect={() => addManualReviewAgent(agent.id)}
                            >
                              <AgentProviderGlyph agent={agent} />
                              <span className="min-w-0 flex-1 truncate">
                                {formatSetupAgentChoiceLabel(
                                  agent,
                                  agent.id === effectiveOrchestratorAgentId
                                    ? agentModelChoices
                                    : {},
                                  modelCatalogs,
                                )}
                              </span>
                              <PlusIcon className="text-muted-foreground size-3.5" />
                            </DropdownMenuItem>
                          ))}
                          {safeAgents.length === 0 && (
                            <DropdownMenuItem disabled>
                              No review-safe agents configured
                            </DropdownMenuItem>
                          )}
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <button
                          className="border-border/70 hover:bg-surface-muted mb-2 flex h-8 w-full cursor-pointer items-center justify-between rounded-lg border bg-white px-2.5 text-[0.8rem] font-medium shadow-[0_1px_1px_rgb(17_18_20/0.03)]"
                          type="button"
                        >
                          <span className="inline-flex items-center gap-2">
                            <UsersIcon className="size-3.5" />
                            {selectedAgentCount} agents selected
                          </span>
                          <ChevronDownIcon className="text-muted-foreground size-3.5" />
                        </button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent className="w-72">
                        <DropdownMenuLabel>
                          Available review agents
                        </DropdownMenuLabel>
                        {safeAgents.map((agent) => (
                          <DropdownMenuCheckboxItem
                            key={agent.id}
                            checked={reviewAgentIdsForRun.includes(agent.id)}
                            className="cursor-pointer"
                            disabled={agent.id === effectiveOrchestratorAgentId}
                            onCheckedChange={(checked) => {
                              const next = new Set(effectiveSelectedAgentIds);
                              if (checked) {
                                next.add(agent.id);
                              } else {
                                next.delete(agent.id);
                              }
                              setSelectedAgentIds(next);
                            }}
                          >
                            <AgentProviderGlyph agent={agent} />
                            <span className="min-w-0 flex-1 truncate">
                              {formatSetupAgentChoiceLabel(
                                agent,
                                agent.id === effectiveOrchestratorAgentId
                                  ? agentModelChoices
                                  : {},
                                modelCatalogs,
                              )}
                            </span>
                          </DropdownMenuCheckboxItem>
                        ))}
                        {safeAgents.length === 0 && (
                          <DropdownMenuItem disabled>
                            No review-safe agents configured
                          </DropdownMenuItem>
                        )}
                      </DropdownMenuContent>
                    </DropdownMenu>
                    <div className="border-border/70 flex max-h-[154px] flex-col overflow-y-auto rounded-lg border bg-white [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                      {orchestratorAgent && (
                        <SetupAgentRow
                          key={`orchestrator:${orchestratorAgent.id}`}
                          agent={orchestratorAgent}
                          catalogs={modelCatalogs}
                          checked
                          choices={agentModelChoices}
                          locked
                          roles={setupReviewRoleOptions}
                          onModelChoice={(choice) =>
                            setAgentModelChoices((current) => ({
                              ...current,
                              [orchestratorAgent.id]: choice,
                            }))
                          }
                          onRoleChange={() => undefined}
                          onCheckedChange={() => undefined}
                        />
                      )}
                      {reviewAgentAssignments.map((assignment) => {
                        const role =
                          setupReviewRoleById(
                            agentRoleChoices[assignment.id],
                          ) ?? assignment.role;
                        const modelChoice =
                          reviewAgentModelChoices[assignment.id];
                        return (
                          <SetupAgentRow
                            key={assignment.id}
                            agent={assignment.agent}
                            catalogs={modelCatalogs}
                            checked
                            choices={
                              modelChoice
                                ? { [assignment.agent.id]: modelChoice }
                                : {}
                            }
                            role={role}
                            roles={setupReviewRoleOptions}
                            onModelChoice={(choice) =>
                              setReviewAgentModelChoices((current) => ({
                                ...current,
                                [assignment.id]: choice,
                              }))
                            }
                            onRoleChange={(roleId) =>
                              setAgentRoleChoices((current) => ({
                                ...current,
                                [assignment.id]: roleId,
                              }))
                            }
                            onCheckedChange={() => {
                              if (assignment.manual) {
                                setManualReviewAssignments((current) =>
                                  current.filter(
                                    (item) => item.id !== assignment.id,
                                  ),
                                );
                                setAgentRoleChoices((current) =>
                                  removeRecordKey(current, assignment.id),
                                );
                                setReviewAgentModelChoices((current) =>
                                  removeRecordKey(current, assignment.id),
                                );
                                return;
                              }
                              setHiddenReviewAssignmentIds((current) => {
                                const next = new Set(current);
                                next.add(assignment.id);
                                return next;
                              });
                            }}
                          />
                        );
                      })}
                    </div>
                  </div>
                </div>
              </SetupStepPanel>

              <SetupStepPanel
                description="Select a preset and confirm what's included."
                compact={sourceInspectorLayoutActive}
                number={4}
                title="Scope & presets"
              >
                <div
                  className={cn(
                    "grid items-stretch gap-3",
                    !sourceInspectorLayoutActive &&
                      "min-[1180px]:grid-cols-[minmax(300px,1fr)_240px]",
                  )}
                >
                  <div className="flex min-w-0 flex-col">
                    <div className="grid grid-cols-2 gap-2.5">
                      {visiblePresetOptions.map((preset) => (
                        <SetupPresetTile
                          key={preset.id}
                          active={selectedPresetIds.has(preset.id)}
                          icon={preset.icon}
                          subtitle={preset.subtitle}
                          tone={preset.tone}
                          title={preset.title}
                          onClick={() => togglePreset(preset.id)}
                        />
                      ))}
                    </div>
                    <div className="mt-auto flex items-center justify-between gap-2 pt-2">
                      <p className="text-muted-foreground text-[0.72rem]">
                        {selectedPresetLabels.length} selected from{" "}
                        {setupPresetOptions.length} built-in presets.
                      </p>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            className="h-8 cursor-pointer"
                            size="sm"
                            variant="outline"
                          >
                            <SearchIcon data-icon="inline-start" />
                            More presets
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent className="w-80 p-2">
                          <Input
                            className="mb-2 h-8"
                            placeholder="Search presets..."
                            value={presetSearch}
                            onChange={(event) =>
                              setPresetSearch(event.target.value)
                            }
                            onKeyDown={(event) => event.stopPropagation()}
                          />
                          <div className="max-h-64 overflow-y-auto pr-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                            {presetSearchResults.map((preset) => (
                              <DropdownMenuCheckboxItem
                                key={preset.id}
                                checked={selectedPresetIds.has(preset.id)}
                                className="cursor-pointer"
                                onCheckedChange={() => togglePreset(preset.id)}
                              >
                                <span
                                  className={cn(
                                    "mr-2 flex size-6 shrink-0 items-center justify-center rounded-md border",
                                    preset.tone,
                                  )}
                                >
                                  <preset.icon className="size-3.5" />
                                </span>
                                <span className="min-w-0 flex-1">
                                  <span className="block truncate text-[0.78rem] font-medium">
                                    {preset.title}
                                  </span>
                                  <span className="text-muted-foreground block truncate text-[0.68rem]">
                                    {preset.subtitle}
                                  </span>
                                </span>
                              </DropdownMenuCheckboxItem>
                            ))}
                            {presetSearchResults.length === 0 && (
                              <DropdownMenuItem disabled>
                                No matching presets
                              </DropdownMenuItem>
                            )}
                          </div>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </div>
                  {!sourceInspectorLayoutActive && (
                    <div className="bg-card border-border-subtle flex h-full min-h-[206px] flex-col rounded-lg border p-3.5">
                      <div className="text-sm font-semibold">Scope summary</div>
                      <div className="mt-2 flex flex-col gap-1.5 text-xs">
                        <SetupScopeRow
                          label="Source"
                          value={
                            setupSourceOptions.find(
                              (item) => item.id === source,
                            )?.label ?? "Source"
                          }
                        />
                        <SetupScopeRow
                          label="Project"
                          value={activeRepository?.name ?? "No project"}
                        />
                        <SetupScopeRow
                          label="Range"
                          value={
                            source === "branch-compare"
                              ? `${baseRef}..${headRefValue || "head"}`
                              : source === "github"
                                ? githubUrl.trim() || "PR URL required"
                                : "Working tree"
                          }
                        />
                        <SetupScopeRow
                          label="Presets"
                          value={`${selectedPresetLabels.length} builtin`}
                        />
                        <SetupScopeRow
                          label="Files"
                          value={
                            previewReady
                              ? String(previewFiles.length)
                              : "Not loaded"
                          }
                        />
                      </div>
                    </div>
                  )}
                </div>

                <div className="mt-3 grid max-w-[180px] gap-1.5">
                  <label className="flex flex-col gap-1.5 text-[0.78rem] font-medium">
                    Review depth
                    <NativeSelect
                      size="sm"
                      value={reviewDepth}
                      onChange={(event) =>
                        setReviewDepth(
                          event.target.value as "quick" | "standard" | "deep",
                        )
                      }
                    >
                      <NativeSelectOption value="quick">
                        Quick
                      </NativeSelectOption>
                      <NativeSelectOption value="standard">
                        Standard
                      </NativeSelectOption>
                      <NativeSelectOption value="deep">Deep</NativeSelectOption>
                    </NativeSelect>
                  </label>
                </div>
              </SetupStepPanel>

              {(localError || startState.status === "error") && (
                <ErrorState
                  title={
                    localError
                      ? "Could not create snapshot"
                      : "Could not start review"
                  }
                  description={
                    localError ||
                    (startState.status === "error"
                      ? startState.error.message
                      : undefined)
                  }
                />
              )}
            </div>
          </div>

          {sourceInspectorRendered && (
            <aside
              className={cn(
                "border-border/60 absolute inset-y-0 right-0 z-20 flex h-full max-h-full min-h-0 min-w-0 transform-gpu flex-col overflow-hidden border-l bg-white shadow-[-18px_0_36px_rgb(17_18_20/0.08)] will-change-transform motion-reduce:transition-none",
                sourceInspectorResizing
                  ? "transition-none"
                  : "transition-all duration-[220ms] ease-[cubic-bezier(0.16,1,0.3,1)]",
                sourceInspectorVisualOpen
                  ? "pointer-events-auto translate-x-0 opacity-100"
                  : "pointer-events-none translate-x-full opacity-0",
              )}
              style={sourceInspectorPanelStyle}
            >
              <SetupSourceInspectorPanel
                canLoad={canCreate}
                expandedFileIds={expandedSourceFileIds}
                patchPreviews={filePatchPreviews}
                preview={sourcePreview}
                previewReady={previewReady}
                projectLabel={activeRepository?.name ?? "No project"}
                rangeLabel={
                  source === "branch-compare"
                    ? `${baseRef}..${headRefValue || "head"}`
                    : source === "github"
                      ? githubUrl.trim() || "PR URL required"
                      : "Working tree"
                }
                sourceLabel={
                  setupSourceOptions.find((item) => item.id === source)
                    ?.label ?? "Source"
                }
                renderedFileCount={renderedDiffFileCount}
                stats={previewStats}
                onFileExpandedChange={setSourceFileExpanded}
                onLoad={() => void loadSourcePreview()}
                onLoadMoreFiles={loadMoreDiffFiles}
              />
              <div
                aria-label="Resize source details"
                aria-orientation="vertical"
                className={cn(
                  "pointer-events-auto absolute inset-y-0 left-0 z-30 w-3 cursor-col-resize touch-none",
                  "before:bg-border/80 before:absolute before:inset-y-4 before:left-0.5 before:w-px before:rounded-full before:opacity-0 before:transition-opacity hover:bg-black/[0.02] hover:before:opacity-100",
                  sourceInspectorResizing && "before:opacity-100",
                )}
                role="separator"
                tabIndex={0}
                onPointerDown={(event) => {
                  event.preventDefault();
                  event.currentTarget.setPointerCapture(event.pointerId);
                  startSourceInspectorResize();
                }}
                onMouseDown={(event) => {
                  event.preventDefault();
                  startSourceInspectorResize();
                }}
              />
            </aside>
          )}
        </div>
      </div>
    </section>
  );
}

function isGitHubPullRequestUrl(value: string) {
  return /^https:\/\/github\.com\/[^/\s]+\/[^/\s]+\/pull\/\d+(?:[/?#].*)?$/i.test(
    value.trim(),
  );
}
