import {
  type CSSProperties,
  type ReactNode,
  type UIEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  ActivityIcon,
  BookOpenIcon,
  CheckIcon,
  ChevronDownIcon,
  CircleIcon,
  Code2Icon,
  CopyIcon,
  DatabaseIcon,
  ExternalLinkIcon,
  FileSearchIcon,
  FileTextIcon,
  GaugeIcon,
  GitBranchIcon,
  GitPullRequestIcon,
  KeyRoundIcon,
  PanelRightCloseIcon,
  PanelRightOpenIcon,
  PlayIcon,
  PlusIcon,
  RefreshCwIcon,
  SearchIcon,
  ShieldCheckIcon,
  UsersIcon,
  XIcon,
  type LucideIcon,
} from "lucide-react";

import { ErrorState, LoadingRows } from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { Textarea } from "@/components/ui/textarea";
import {
  type AgentConfig,
  type AgentModelCatalog,
  type AgentModelOption,
  type ApiClient,
  type ChangedFile,
  type ChangedFilePatch,
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
import githubLogoUrl from "../../../../../../assets/agents/github.svg";

import {
  agentLogoUrl,
  buildSetupAgentSelection,
  formatSetupAgentChoiceLabel,
  formatSetupAgentCompactChoiceLabel,
  formatSetupAgentModelChoiceLabel,
  groupSetupModelsByProvider,
  setupChoiceForAgent,
  setupChoiceFromModel,
  setupModelsForAgent,
  type AgentVisibilitySource,
  type SetupAgentModelChoice,
} from "./agent-utils";

type SnapshotSource = "github" | "local-changes" | "branch-compare";
type GitHubSnapshotAuthMethod = "token" | "gh_cli";

type SetupSourcePreview = {
  key: string;
  snapshot: Snapshot;
  files: ChangedFile[];
};

type SetupPreviewStats = {
  total: number;
  reviewable: number;
  additions: number;
  deletions: number;
  generated: number;
  binary: number;
  excluded: number;
};

type SetupReviewAgentAssignment = {
  id: string;
  agent: AgentConfig;
  role: SetupReviewRoleOption;
  index: number;
  manual?: boolean;
};

type ManualReviewAgentAssignment = {
  id: string;
  agentId: string;
  roleId: string;
};

const sourceInspectorMinWidth = 380;
const sourceInspectorMaxWidth = 860;
const sourceInspectorMainMinWidth = 760;
const sourceInspectorTransitionMs = 220;
const setupInitialDiffFileRenderCount = 6;
const setupDiffFileRenderBatchSize = 6;
const setupMaxRenderedDiffFiles = 200;
const emptyCollapsedFileIds = new Set<string>();

export function NewThreadScreen({
  activeRepository,
  activeWorkspace,
  agentConfigs,
  agentModelCatalogs,
  client,
  onOpenRepository,
  onReviewStarted,
}: {
  activeRepository?: Repository;
  activeWorkspace?: Workspace;
  agentConfigs: Loadable<AgentConfig[]>;
  agentModelCatalogs: Loadable<AgentModelCatalog[]>;
  client: ApiClient | null;
  onOpenRepository: () => void;
  onReviewStarted: (session: ReviewSession) => void;
}) {
  const [source, setSource] = useState<SnapshotSource>("github");
  const [githubUrl, setGitHubUrl] = useState("");
  const [githubAuthMethod, setGithubAuthMethod] =
    useState<GitHubSnapshotAuthMethod>("token");
  const [baseRefInput, setBaseRefInput] = useState("");
  const [headRef, setHeadRef] = useState("");
  const [focusPrompt, setFocusPrompt] = useState("");
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
  const requestedPatchFileIds = useRef(new Set<string>());
  const loadedPatchFileIds = useRef(new Set<string>());
  const sourceInspectorLayoutRef = useRef<HTMLDivElement | null>(null);
  const [branchState, setBranchState] =
    useState<Loadable<RepositoryBranch[]>>(idleApiState());
  const [sourcePreview, setSourcePreview] =
    useState<Loadable<SetupSourcePreview>>(idleApiState());
  const [sourceInspectorOpen, setSourceInspectorOpen] = useState(false);
  const [sourceInspectorRendered, setSourceInspectorRendered] = useState(false);
  const [sourceInspectorVisible, setSourceInspectorVisible] = useState(false);
  const [sourceInspectorOpenCount, setSourceInspectorOpenCount] = useState(0);
  const [sourceInspectorWidth, setSourceInspectorWidth] = useState(560);
  const [sourceInspectorResizing, setSourceInspectorResizing] = useState(false);
  const [renderedDiffFileCount, setRenderedDiffFileCount] = useState(
    setupInitialDiffFileRenderCount,
  );
  const [filePatchPreviews, setFilePatchPreviews] = useState<
    Record<string, Loadable<ChangedFilePatch>>
  >({});
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
  const sourceInspectorLayoutActive =
    sourceInspectorOpen || sourceInspectorRendered;
  const sourceInspectorVisualOpen =
    sourceInspectorOpen && sourceInspectorVisible;
  const sourceInspectorLayoutStyle = sourceInspectorLayoutActive
    ? ({
        "--source-inspector-width": `${sourceInspectorWidth}px`,
      } as CSSProperties)
    : undefined;
  const sourceInspectorPanelStyle = sourceInspectorLayoutActive
    ? ({
        width: `min(${sourceInspectorWidth}px, calc(100% - 16px))`,
      } as CSSProperties)
    : undefined;

  useEffect(() => {
    let canceled = false;
    if (sourceInspectorOpen) {
      queueMicrotask(() => {
        if (!canceled) {
          setSourceInspectorRendered(true);
        }
      });
      const frame = window.requestAnimationFrame(() => {
        if (!canceled) {
          setSourceInspectorVisible(true);
        }
      });
      return () => {
        canceled = true;
        window.cancelAnimationFrame(frame);
      };
    }

    queueMicrotask(() => {
      if (!canceled) {
        setSourceInspectorVisible(false);
        setSourceInspectorResizing(false);
      }
    });
    if (!sourceInspectorRendered) {
      return () => {
        canceled = true;
      };
    }
    const timer = window.setTimeout(() => {
      if (!canceled) {
        setSourceInspectorRendered(false);
      }
    }, sourceInspectorTransitionMs);
    return () => {
      canceled = true;
      window.clearTimeout(timer);
    };
  }, [sourceInspectorOpen, sourceInspectorRendered]);

  useEffect(() => {
    if (!sourceInspectorResizing) {
      return;
    }
    const previousCursor = document.body.style.cursor;
    const previousUserSelect = document.body.style.userSelect;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";

    function resizeFromClientX(clientX: number) {
      const layoutBounds =
        sourceInspectorLayoutRef.current?.getBoundingClientRect();
      const availableWidth = layoutBounds?.width ?? window.innerWidth;
      const rightEdge = layoutBounds?.right ?? window.innerWidth;
      const maxWidth = Math.max(
        sourceInspectorMinWidth,
        Math.min(
          sourceInspectorMaxWidth,
          availableWidth - sourceInspectorMainMinWidth,
        ),
      );
      const nextWidth = Math.min(
        maxWidth,
        Math.max(sourceInspectorMinWidth, rightEdge - clientX),
      );
      setSourceInspectorWidth(nextWidth);
    }

    function handlePointerMove(event: PointerEvent) {
      resizeFromClientX(event.clientX);
    }

    function handleMouseMove(event: MouseEvent) {
      resizeFromClientX(event.clientX);
    }

    function handleResizeEnd() {
      setSourceInspectorResizing(false);
    }

    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("mousemove", handleMouseMove);
    window.addEventListener("pointerup", handleResizeEnd, { once: true });
    window.addEventListener("mouseup", handleResizeEnd, { once: true });
    return () => {
      document.body.style.cursor = previousCursor;
      document.body.style.userSelect = previousUserSelect;
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("mousemove", handleMouseMove);
      window.removeEventListener("pointerup", handleResizeEnd);
      window.removeEventListener("mouseup", handleResizeEnd);
    };
  }, [sourceInspectorResizing]);

  useEffect(() => {
    function clampInspectorWidth() {
      const availableWidth =
        sourceInspectorLayoutRef.current?.clientWidth ?? window.innerWidth;
      const maxWidth = Math.max(
        sourceInspectorMinWidth,
        Math.min(
          sourceInspectorMaxWidth,
          availableWidth - sourceInspectorMainMinWidth,
        ),
      );
      setSourceInspectorWidth((current) =>
        Math.min(maxWidth, Math.max(sourceInspectorMinWidth, current)),
      );
    }

    clampInspectorWidth();
    window.addEventListener("resize", clampInspectorWidth);
    return () => {
      window.removeEventListener("resize", clampInspectorWidth);
    };
  }, []);

  useEffect(() => {
    if (!sourceInspectorOpen) {
      return;
    }
    const availableWidth =
      sourceInspectorLayoutRef.current?.clientWidth ?? window.innerWidth;
    const maxWidth = Math.max(
      sourceInspectorMinWidth,
      Math.min(
        sourceInspectorMaxWidth,
        availableWidth - sourceInspectorMainMinWidth,
      ),
    );
    setSourceInspectorWidth((current) =>
      Math.min(maxWidth, Math.max(sourceInspectorMinWidth, current)),
    );
  }, [sourceInspectorOpen]);

  useEffect(() => {
    let canceled = false;
    if (!client || !activeWorkspace || !activeRepository) {
      return () => {
        canceled = true;
      };
    }
    queueMicrotask(() => {
      if (!canceled) {
        setBranchState(loadingApiState());
      }
    });
    void loadApiResource(() =>
      client.listRepositoryBranches(activeRepository.id, {
        workspaceId: activeWorkspace.id,
      }),
    ).then((state) => {
      if (!canceled) {
        setBranchState(state);
      }
    });
    return () => {
      canceled = true;
    };
  }, [activeRepository, activeWorkspace, client]);
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
  const selectedFocusLabels = setupFocusOptions
    .filter((item) => selectedFocusIds.has(item.id))
    .map((item) => item.label);
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
  const previewReady =
    sourcePreview.status === "success" && sourcePreview.data.key === sourceKey;
  const previewSnapshot = previewReady ? sourcePreview.data.snapshot : null;
  const previewFiles = useMemo(
    () => (previewReady ? sourcePreview.data.files : []),
    [previewReady, sourcePreview],
  );
  const previewStats = setupPreviewStats(previewFiles);
  const renderedDiffFiles = useMemo(
    () => previewFiles.slice(0, renderedDiffFileCount),
    [previewFiles, renderedDiffFileCount],
  );
  const patchLoadFiles = useMemo(
    () =>
      renderedDiffFiles.filter(
        (file) => file.patch_artifact_id && !file.is_binary,
      ),
    [renderedDiffFiles],
  );

  useEffect(() => {
    let canceled = false;
    if (!client || !previewReady) {
      return () => {
        canceled = true;
      };
    }
    const requestedPatchFileIDs = requestedPatchFileIds.current;
    const loadedPatchFileIDs = loadedPatchFileIds.current;

    const filesToLoad = patchLoadFiles.filter((file) => {
      if (requestedPatchFileIDs.has(file.id)) {
        return false;
      }
      return !loadedPatchFileIDs.has(file.id);
    });
    if (filesToLoad.length === 0) {
      return () => {
        canceled = true;
      };
    }
    for (const file of filesToLoad) {
      requestedPatchFileIDs.add(file.id);
    }

    setFilePatchPreviews((current) => {
      const next = { ...current };
      for (const file of filesToLoad) {
        next[file.id] = loadingApiState();
      }
      return next;
    });

    for (const file of filesToLoad) {
      void loadApiResource(() =>
        client.getChangedFilePatch(file.snapshot_id, file.id),
      ).then((state) => {
        requestedPatchFileIDs.delete(file.id);
        if (!canceled) {
          if (
            state.status === "success" &&
            state.data.changed_file_id === file.id
          ) {
            loadedPatchFileIDs.add(file.id);
          }
          setFilePatchPreviews((current) => ({
            ...current,
            [file.id]: state,
          }));
        }
      });
    }

    return () => {
      canceled = true;
      for (const file of filesToLoad) {
        requestedPatchFileIDs.delete(file.id);
      }
    };
  }, [client, patchLoadFiles, previewReady]);

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

  const shouldAutoLoadSourcePreview = useCallback(() => {
    return (
      canCreate &&
      sourcePreview.status !== "loading" &&
      !previewReady &&
      (source !== "github" || githubUrl.trim() !== "") &&
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

  function toggleSourceInspector() {
    if (sourceInspectorOpen) {
      setSourceInspectorOpen(false);
      return;
    }
    setSourceInspectorRendered(true);
    setSourceInspectorVisible(false);
    setSourceInspectorOpenCount((value) => value + 1);
    setSourceInspectorOpen(true);
  }

  function addFocusHint() {
    setFocusPrompt((current) => {
      const trimmed = current.trim();
      const selectedHints = setupFocusOptions
        .filter((item) => selectedFocusIds.has(item.id))
        .map((item) => setupFocusHintById[item.id])
        .filter(Boolean);
      const nextHint =
        selectedHints.length > 0
          ? selectedHints.join("\n")
          : "Prioritize correctness, user-visible regressions, and missing tests.";
      if (trimmed.includes(nextHint)) {
        return current;
      }
      return trimmed ? `${trimmed}\n${nextHint}` : nextHint;
    });
  }

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

  function loadMoreDiffFiles() {
    setRenderedDiffFileCount((current) =>
      Math.min(
        current + setupDiffFileRenderBatchSize,
        Math.min(previewFiles.length, setupMaxRenderedDiffFiles),
      ),
    );
  }

  function resetSetup() {
    setSource("github");
    setGitHubUrl("");
    setGithubAuthMethod("token");
    setBaseRefInput("");
    setHeadRef("");
    setFocusPrompt("");
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
    setSourceInspectorOpen(false);
    setSourcePreview(idleApiState());
    setRenderedDiffFileCount(setupInitialDiffFileRenderCount);
    setFilePatchPreviews({});
    requestedPatchFileIds.current.clear();
    loadedPatchFileIds.current.clear();
    setLocalError("");
    setStartState(idleApiState());
  }

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

  const loadSourcePreview = useCallback(async () => {
    if (!client) {
      setSourcePreview(
        errorApiState(new Error("Backend client is unavailable.")),
      );
      return;
    }
    if (!validateSource()) {
      return;
    }
    setRenderedDiffFileCount(setupInitialDiffFileRenderCount);
    setFilePatchPreviews({});
    requestedPatchFileIds.current.clear();
    loadedPatchFileIds.current.clear();
    setSourcePreview(loadingApiState());
    const requestedKey = sourceKey;
    const nextSnapshot = await loadApiResource(createSourceSnapshot);
    if (nextSnapshot.status !== "success") {
      setSourcePreview(
        errorApiState(
          nextSnapshot.status === "error"
            ? nextSnapshot.error
            : new Error("Snapshot creation did not complete."),
        ),
      );
      return;
    }
    const files = await loadApiResource(() =>
      client.listChangedFiles(nextSnapshot.data.id),
    );
    if (files.status !== "success") {
      setSourcePreview(
        errorApiState(
          files.status === "error"
            ? files.error
            : new Error("Changed files did not load."),
        ),
      );
      return;
    }
    setLocalError("");
    setSourcePreview(
      successApiState({
        key: requestedKey,
        snapshot: nextSnapshot.data,
        files: files.data,
      }),
    );
  }, [client, createSourceSnapshot, sourceKey, validateSource]);

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
        focus_prompt: setupFocusPrompt(focusPrompt, selectedFocusLabels),
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
    <section className="flex min-h-0 min-w-0 flex-col overflow-hidden bg-[#fbfbfa]">
      <div className="grid h-full w-full grid-rows-[auto_minmax(0,1fr)]">
        <div className="flex items-start justify-between gap-4 px-4 pt-5 pb-3 min-[900px]:px-6">
          <div className="min-w-0">
            <h1 className="text-[1.34rem] leading-7 font-semibold tracking-[-0.01em]">
              Set up review
            </h1>
            <p className="text-muted-foreground mt-0.5 text-[0.82rem]">
              Configure the source, focus, orchestration, and presets for this
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
                <div className="grid grid-cols-[repeat(auto-fit,minmax(175px,1fr))] gap-2">
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
                        value={baseRef}
                        onSelect={setBaseRefInput}
                      />
                      <SetupBranchSelector
                        branches={branchState}
                        disabled={!canCreate}
                        label="Head branch"
                        value={headRefValue}
                        onSelect={setHeadRef}
                      />
                    </div>
                  )}
                </div>
              </SetupStepPanel>

              <SetupStepPanel
                description="What should the review prioritize?"
                compact={sourceInspectorLayoutActive}
                number={2}
                title="Review focus"
              >
                <Textarea
                  aria-label="Focus prompt"
                  className="min-h-[54px] resize-none bg-white text-[0.82rem]"
                  placeholder="Describe what this review should focus on, for example auth logic, billing flows, or data integrity..."
                  value={focusPrompt}
                  onChange={(event) => setFocusPrompt(event.target.value)}
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
                  <Button
                    className="h-8 cursor-pointer"
                    size="sm"
                    variant="outline"
                    onClick={addFocusHint}
                  >
                    <PlusIcon data-icon="inline-start" />
                    Add focus
                  </Button>
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
                    <div className="border-border/70 flex h-full min-h-[206px] flex-col rounded-lg border bg-white/85 p-3">
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
                  setSourceInspectorResizing(true);
                }}
                onMouseDown={(event) => {
                  event.preventDefault();
                  setSourceInspectorResizing(true);
                }}
              />
            </aside>
          )}
        </div>
      </div>
    </section>
  );
}

const setupSourceOptions: Array<{
  id: SnapshotSource;
  label: string;
  icon: LucideIcon;
  logoUrl?: string;
}> = [
  {
    id: "github",
    label: "GitHub PR URL",
    icon: GitPullRequestIcon,
    logoUrl: githubLogoUrl,
  },
  { id: "local-changes", label: "Local changes", icon: FileTextIcon },
  { id: "branch-compare", label: "Compare branches", icon: GitBranchIcon },
];

const setupFocusOptions: Array<{
  id: string;
  label: string;
  icon: LucideIcon;
}> = [
  { id: "security", label: "Security issues", icon: ShieldCheckIcon },
  { id: "data", label: "Data leaks", icon: DatabaseIcon },
  { id: "quality", label: "General quality", icon: Code2Icon },
  { id: "edge", label: "Edge cases", icon: GaugeIcon },
];

const setupFocusHintById: Record<string, string> = {
  security:
    "Prioritize security issues, unsafe authorization boundaries, secret exposure, and injection risks.",
  data: "Trace sensitive data paths across persistence, logs, telemetry, exports, and error responses.",
  quality:
    "Check correctness, maintainability, error handling, API compatibility, and user-visible regressions.",
  edge: "Exercise edge cases around empty input, large diffs, retries, cancellation, concurrency, and rollback paths.",
};

type SetupPresetOption = {
  id: string;
  subtitle: string;
  title: string;
  icon: LucideIcon;
  tone: string;
  roleIds: string[];
};

type SetupReviewRoleOption = {
  id: string;
  label: string;
  shortLabel: string;
  description: string;
  icon: LucideIcon;
};

const setupReviewRoleOptions: SetupReviewRoleOption[] = [
  {
    id: "general-reviewer",
    label: "General Reviewer",
    shortLabel: "General",
    description:
      "Balanced review across correctness, maintainability, risk, and regressions.",
    icon: FileSearchIcon,
  },
  {
    id: "go-correctness",
    label: "Go Correctness & Idioms Reviewer",
    shortLabel: "Go review",
    description:
      "Checks Go control flow, errors, APIs, and idiomatic implementation details.",
    icon: Code2Icon,
  },
  {
    id: "go-performance",
    label: "Go Performance Reviewer",
    shortLabel: "Performance",
    description:
      "Looks for CPU, memory, allocation, I/O, and large-diff hot-path risks.",
    icon: GaugeIcon,
  },
  {
    id: "go-concurrency",
    label: "Go Concurrency Reviewer",
    shortLabel: "Concurrency",
    description:
      "Reviews goroutine, channel, locking, race, cancellation, and timeout behavior.",
    icon: ActivityIcon,
  },
  {
    id: "postgres-query-performance",
    label: "PostgreSQL Query Performance Reviewer",
    shortLabel: "SQL review",
    description:
      "Reviews query plans, indexes, N+1 patterns, scans, and pagination safety.",
    icon: DatabaseIcon,
  },
  {
    id: "postgres-migration-safety",
    label: "PostgreSQL Migration Safety Reviewer",
    shortLabel: "Migration",
    description:
      "Checks migration locks, backfills, data compatibility, and rollback paths.",
    icon: DatabaseIcon,
  },
  {
    id: "security",
    label: "Security Reviewer",
    shortLabel: "Security",
    description:
      "Looks for injection, unsafe defaults, secret exposure, and supply-chain risk.",
    icon: ShieldCheckIcon,
  },
  {
    id: "authz-tenant-isolation",
    label: "AuthZ & Tenant Isolation Reviewer",
    shortLabel: "AuthZ",
    description:
      "Checks permission boundaries, tenant isolation, identity flow, and confused deputy risks.",
    icon: KeyRoundIcon,
  },
  {
    id: "testing-regression",
    label: "Testing & Regression Reviewer",
    shortLabel: "Testing",
    description:
      "Finds missing coverage, regression boundaries, and brittle test assumptions.",
    icon: FileTextIcon,
  },
  {
    id: "evidence-verifier",
    label: "Evidence Verifier",
    shortLabel: "Evidence",
    description:
      "Verifies findings against exact code evidence and removes weak claims.",
    icon: CheckIcon,
  },
  {
    id: "counter-evidence-skeptic",
    label: "Counter-Evidence Skeptic",
    shortLabel: "Skeptic",
    description:
      "Searches for disproving paths, safeguards, and false-positive explanations.",
    icon: SearchIcon,
  },
  {
    id: "finding-synthesizer",
    label: "Finding Synthesizer",
    shortLabel: "Synthesis",
    description: "Merges overlapping findings into a clear review narrative.",
    icon: BookOpenIcon,
  },
  {
    id: "copy-fix-packet-writer",
    label: "Copy Fix Packet Writer",
    shortLabel: "Fix packet",
    description:
      "Turns verified findings into concise, actionable repair packets.",
    icon: CopyIcon,
  },
];

const setupPresetOptions: SetupPresetOption[] = [
  {
    id: "standard-pr-review",
    title: "Standard Review",
    subtitle: "Default protocol",
    icon: FileSearchIcon,
    tone: "border-sky-200 bg-sky-50 text-sky-700",
    roleIds: [
      "general-reviewer",
      "go-correctness",
      "security",
      "testing-regression",
      "evidence-verifier",
    ],
  },
  {
    id: "security-auth-focus",
    title: "Security & Auth",
    subtitle: "Auth, tenant, secrets",
    icon: ShieldCheckIcon,
    tone: "border-emerald-200 bg-emerald-50 text-emerald-700",
    roleIds: [
      "security",
      "authz-tenant-isolation",
      "general-reviewer",
      "evidence-verifier",
      "counter-evidence-skeptic",
    ],
  },
  {
    id: "go-performance-deep-dive",
    title: "Performance",
    subtitle: "CPU, memory, I/O",
    icon: GaugeIcon,
    tone: "border-violet-200 bg-violet-50 text-violet-700",
    roleIds: [
      "go-performance",
      "go-concurrency",
      "general-reviewer",
      "evidence-verifier",
    ],
  },
  {
    id: "postgres-query-performance",
    title: "SQL Review",
    subtitle: "SQL and indexes",
    icon: DatabaseIcon,
    tone: "border-blue-200 bg-blue-50 text-blue-700",
    roleIds: [
      "postgres-query-performance",
      "general-reviewer",
      "testing-regression",
      "evidence-verifier",
    ],
  },
  {
    id: "postgres-migration-safety",
    title: "Migrations",
    subtitle: "Locks and backfills",
    icon: DatabaseIcon,
    tone: "border-cyan-200 bg-cyan-50 text-cyan-700",
    roleIds: [
      "postgres-migration-safety",
      "postgres-query-performance",
      "general-reviewer",
      "testing-regression",
      "counter-evidence-skeptic",
    ],
  },
  {
    id: "data-integrity-transactions",
    title: "Integrity",
    subtitle: "Money, ledgers, writes",
    icon: KeyRoundIcon,
    tone: "border-amber-200 bg-amber-50 text-amber-700",
    roleIds: [
      "general-reviewer",
      "go-correctness",
      "postgres-migration-safety",
      "testing-regression",
      "evidence-verifier",
    ],
  },
  {
    id: "reliability-production-readiness",
    title: "Reliability",
    subtitle: "Timeouts and queues",
    icon: ActivityIcon,
    tone: "border-lime-200 bg-lime-50 text-lime-700",
    roleIds: [
      "go-concurrency",
      "go-performance",
      "general-reviewer",
      "testing-regression",
      "counter-evidence-skeptic",
    ],
  },
  {
    id: "testing-regression-coverage",
    title: "Testing",
    subtitle: "Risk and boundaries",
    icon: FileTextIcon,
    tone: "border-stone-200 bg-stone-50 text-stone-700",
    roleIds: [
      "testing-regression",
      "general-reviewer",
      "evidence-verifier",
      "counter-evidence-skeptic",
    ],
  },
  {
    id: "api-compatibility-client-impact",
    title: "API Impact",
    subtitle: "Contracts and SDKs",
    icon: Code2Icon,
    tone: "border-indigo-200 bg-indigo-50 text-indigo-700",
    roleIds: [
      "general-reviewer",
      "go-correctness",
      "testing-regression",
      "finding-synthesizer",
      "copy-fix-packet-writer",
    ],
  },
  {
    id: "privacy-sensitive-data",
    title: "Privacy",
    subtitle: "PII, logs, exports",
    icon: KeyRoundIcon,
    tone: "border-rose-200 bg-rose-50 text-rose-700",
    roleIds: [
      "security",
      "authz-tenant-isolation",
      "general-reviewer",
      "evidence-verifier",
      "copy-fix-packet-writer",
    ],
  },
];

const setupPrimaryPresetIds = [
  "standard-pr-review",
  "security-auth-focus",
  "go-performance-deep-dive",
  "postgres-query-performance",
];

function SetupStepPanel({
  children,
  compact = false,
  description,
  number,
  title,
}: {
  children: ReactNode;
  compact?: boolean;
  description: string;
  number: number;
  title: string;
}) {
  return (
    <section className="grid grid-cols-[28px_minmax(0,1fr)] gap-3">
      <div className="relative flex justify-center pt-2">
        {number < 4 && (
          <span className="bg-border/80 absolute top-8 bottom-[-16px] left-1/2 w-px -translate-x-1/2" />
        )}
        <span className="bg-foreground text-background relative z-10 flex size-5 items-center justify-center rounded-full text-[0.68rem] font-semibold shadow-[0_1px_2px_rgb(17_18_20/0.14)]">
          {number}
        </span>
      </div>
      <div className="border-border/70 rounded-xl border bg-white/88 px-4 py-3 shadow-[0_1px_2px_rgb(17_18_20/0.03)]">
        <div
          className={cn(
            "grid gap-4",
            compact
              ? "min-[1540px]:grid-cols-[204px_minmax(0,1fr)]"
              : "lg:grid-cols-[204px_minmax(0,1fr)]",
          )}
        >
          <div className="min-w-0">
            <h2 className="text-[0.96rem] leading-5 font-semibold">{title}</h2>
            <p className="text-muted-foreground mt-1 text-[0.78rem] leading-5">
              {description}
            </p>
          </div>
          <div className="min-w-0">{children}</div>
        </div>
      </div>
    </section>
  );
}

function SetupSegment({
  active,
  icon: Icon,
  label,
  logoUrl,
  onClick,
}: {
  active: boolean;
  icon: LucideIcon;
  label: string;
  logoUrl?: string;
  onClick: () => void;
}) {
  return (
    <button
      className={cn(
        "border-border/70 hover:bg-surface-muted flex h-10 min-w-0 cursor-pointer items-center justify-between gap-2 rounded-lg border bg-white px-2.5 text-left text-[0.78rem] font-medium shadow-[0_1px_1px_rgb(17_18_20/0.025)] transition-colors",
        active &&
          "border-foreground/50 bg-white shadow-[0_1px_2px_rgb(17_18_20/0.08)]",
      )}
      type="button"
      onClick={onClick}
    >
      <span className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden">
        {logoUrl ? (
          <img alt="" className="size-4 shrink-0 rounded-[3px]" src={logoUrl} />
        ) : (
          <Icon className="size-3.5 shrink-0" />
        )}
        <span className="min-w-0 truncate">{label}</span>
      </span>
      <span
        className={cn(
          "border-border flex size-3.5 shrink-0 items-center justify-center rounded-full border bg-white",
          active && "border-foreground bg-foreground",
        )}
      >
        {active && <span className="bg-background size-1.5 rounded-full" />}
      </span>
    </button>
  );
}

function SetupBranchSelector({
  branches,
  disabled,
  label,
  value,
  onSelect,
}: {
  branches: Loadable<RepositoryBranch[]>;
  disabled: boolean;
  label: string;
  value: string;
  onSelect: (value: string) => void;
}) {
  const [query, setQuery] = useState("");
  const options = branches.status === "success" ? branches.data : [];
  const filtered = options.filter((branch) =>
    branch.name.toLowerCase().includes(query.trim().toLowerCase()),
  );
  return (
    <label className="flex min-w-0 flex-col gap-1.5 text-xs font-medium">
      {label}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            aria-label={`${label}: ${value}`}
            className="border-border/70 hover:bg-surface-muted flex h-9 w-full cursor-pointer items-center justify-between gap-2 rounded-lg border bg-white px-3 text-left text-[0.8rem] font-medium shadow-[0_1px_1px_rgb(17_18_20/0.03)] disabled:cursor-default disabled:opacity-60"
            disabled={disabled}
            type="button"
          >
            <span className="flex min-w-0 items-center gap-2">
              <GitBranchIcon className="size-3.5 shrink-0" />
              <span className="truncate">{value || "Choose branch"}</span>
            </span>
            <ChevronDownIcon className="text-muted-foreground size-3.5 shrink-0" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent className="w-80 p-2">
          <Input
            aria-label={`Search ${label.toLowerCase()}`}
            className="mb-2 h-8"
            placeholder="Search branches..."
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => event.stopPropagation()}
          />
          <div className="max-h-60 overflow-y-auto pr-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
            {branches.status === "loading" && (
              <div className="px-2 py-2">
                <LoadingRows rows={3} />
              </div>
            )}
            {branches.status === "error" && (
              <div className="text-destructive px-2 py-2 text-xs leading-5">
                {branches.error.message}
              </div>
            )}
            {branches.status === "success" &&
              filtered.map((branch) => (
                <DropdownMenuItem
                  key={`${branch.remote ? "remote" : "local"}:${branch.name}`}
                  className="cursor-pointer"
                  onSelect={() => onSelect(branch.name)}
                >
                  <GitBranchIcon className="size-3.5" />
                  <span className="min-w-0 flex-1 truncate">{branch.name}</span>
                  {branch.remote && (
                    <Badge
                      className="h-5 px-1.5 text-[0.62rem]"
                      variant="outline"
                    >
                      remote
                    </Badge>
                  )}
                  {branch.name === value && <CheckIcon className="size-3.5" />}
                </DropdownMenuItem>
              ))}
            {branches.status === "success" && filtered.length === 0 && (
              <DropdownMenuItem disabled>No matching branches</DropdownMenuItem>
            )}
          </div>
        </DropdownMenuContent>
      </DropdownMenu>
    </label>
  );
}

function SetupFocusChip({
  active,
  icon: Icon,
  label,
  onClick,
}: {
  active: boolean;
  icon: LucideIcon;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      className={cn(
        "border-border/70 text-muted-foreground hover:bg-surface-muted hover:text-foreground inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-full border bg-white px-3 text-[0.76rem] font-medium transition-colors",
        active &&
          "border-border bg-surface text-foreground shadow-[0_1px_1px_rgb(17_18_20/0.025)]",
      )}
      type="button"
      onClick={onClick}
    >
      <Icon className="size-3.5 shrink-0" />
      {label}
    </button>
  );
}

function SetupAgentSelector({
  agents,
  catalogs,
  choices,
  disabled = false,
  placeholder,
  selectedAgentId,
  onSelect,
}: {
  agents: AgentConfig[];
  catalogs: AgentModelCatalog[];
  choices: Record<string, SetupAgentModelChoice>;
  disabled?: boolean;
  placeholder: string;
  selectedAgentId: string;
  onSelect: (agentId: string, choice: SetupAgentModelChoice) => void;
}) {
  const selectedAgent = agents.find((agent) => agent.id === selectedAgentId);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          aria-label={placeholder}
          className="border-border/70 hover:bg-surface-muted flex h-8 w-full cursor-pointer items-center justify-between gap-3 rounded-lg border bg-white px-2.5 text-left text-[0.8rem] font-medium shadow-[0_1px_1px_rgb(17_18_20/0.03)] disabled:cursor-default disabled:opacity-60"
          disabled={disabled}
          type="button"
        >
          <span className="flex min-w-0 items-center gap-2">
            {selectedAgent ? (
              <AgentProviderGlyph agent={selectedAgent} />
            ) : (
              <UsersIcon className="size-3.5" />
            )}
            <span className="truncate">
              {selectedAgent
                ? formatSetupAgentCompactChoiceLabel(
                    selectedAgent,
                    choices,
                    catalogs,
                  )
                : placeholder}
            </span>
          </span>
          <ChevronDownIcon className="text-muted-foreground size-3.5 shrink-0" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-80">
        <DropdownMenuLabel>CLI</DropdownMenuLabel>
        <DropdownMenuGroup>
          {agents.map((agent) => (
            <SetupAgentMenuBranch
              key={agent.id}
              agent={agent}
              catalogs={catalogs}
              choices={choices}
              selected={agent.id === selectedAgentId}
              onSelect={onSelect}
            />
          ))}
        </DropdownMenuGroup>
        {agents.length === 0 && (
          <DropdownMenuItem disabled>
            No review-safe agents found
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function SetupAgentMenuBranch({
  agent,
  catalogs,
  choices,
  selected,
  onSelect,
}: {
  agent: AgentConfig;
  catalogs: AgentModelCatalog[];
  choices: Record<string, SetupAgentModelChoice>;
  selected: boolean;
  onSelect: (agentId: string, choice: SetupAgentModelChoice) => void;
}) {
  const models = setupModelsForAgent(agent, catalogs);
  if (models.length === 0) {
    return (
      <DropdownMenuItem
        className="cursor-pointer"
        onSelect={() => onSelect(agent.id, {})}
      >
        <AgentProviderGlyph agent={agent} />
        <span className="min-w-0 flex-1 truncate">{agent.name}</span>
        {selected && <CheckIcon className="size-3.5" />}
      </DropdownMenuItem>
    );
  }
  const groups = groupSetupModelsByProvider(models);
  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger className="cursor-pointer">
        <AgentProviderGlyph agent={agent} />
        <span className="min-w-0 flex-1 truncate">{agent.name}</span>
        {selected && <CheckIcon className="text-muted-foreground size-3.5" />}
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent className="w-72">
        {groups.length > 1 ? (
          groups.map((group) => (
            <DropdownMenuSub key={group.provider}>
              <DropdownMenuSubTrigger className="cursor-pointer">
                {group.providerLabel}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent className="w-72">
                <SetupModelMenuItems
                  agent={agent}
                  choices={choices}
                  models={group.models}
                  onSelect={onSelect}
                />
              </DropdownMenuSubContent>
            </DropdownMenuSub>
          ))
        ) : (
          <SetupModelMenuItems
            agent={agent}
            choices={choices}
            models={groups[0]?.models ?? models}
            onSelect={onSelect}
          />
        )}
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  );
}

function SetupAgentModelSelector({
  agent,
  catalogs,
  choices,
  className,
  label,
  onSelect,
}: {
  agent: AgentConfig;
  catalogs: AgentModelCatalog[];
  choices: Record<string, SetupAgentModelChoice>;
  className?: string;
  label?: string;
  onSelect: (choice: SetupAgentModelChoice) => void;
}) {
  const models = setupModelsForAgent(agent, catalogs);
  const groups = groupSetupModelsByProvider(models);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          className={cn(
            "bg-surface text-muted-foreground hover:border-border hover:text-foreground flex h-6 min-w-0 cursor-pointer items-center justify-between gap-2 rounded-md border border-transparent px-1.5 text-left text-[0.73rem] font-medium hover:bg-white",
            className,
          )}
          type="button"
        >
          <span className="truncate">
            {label ??
              formatSetupAgentModelChoiceLabel(agent, choices, catalogs)}
          </span>
          <ChevronDownIcon className="size-3 shrink-0" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-72">
        {models.length === 0 ? (
          <DropdownMenuItem disabled>Use configured model</DropdownMenuItem>
        ) : groups.length > 1 ? (
          groups.map((group) => (
            <DropdownMenuSub key={group.provider}>
              <DropdownMenuSubTrigger className="cursor-pointer">
                {group.providerLabel}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent className="w-72">
                <SetupModelMenuItems
                  agent={agent}
                  choices={choices}
                  models={group.models}
                  onSelect={(_, choice) => onSelect(choice)}
                />
              </DropdownMenuSubContent>
            </DropdownMenuSub>
          ))
        ) : (
          <SetupModelMenuItems
            agent={agent}
            choices={choices}
            models={groups[0]?.models ?? models}
            onSelect={(_, choice) => onSelect(choice)}
          />
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function SetupModelMenuItems({
  agent,
  choices,
  models,
  onSelect,
}: {
  agent: AgentConfig;
  choices: Record<string, SetupAgentModelChoice>;
  models: AgentModelOption[];
  onSelect: (agentId: string, choice: SetupAgentModelChoice) => void;
}) {
  const current = setupChoiceForAgent(agent, choices, []);
  return (
    <>
      {models.map((model) => {
        const reasoning = model.reasoning_efforts ?? [];
        if (reasoning.length > 0) {
          return (
            <DropdownMenuSub key={model.id}>
              <DropdownMenuSubTrigger className="cursor-pointer">
                <span className="min-w-0 flex-1 truncate">{model.label}</span>
                {current.modelId === model.id && (
                  <CheckIcon className="text-muted-foreground size-3.5" />
                )}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent className="w-48">
                <DropdownMenuLabel>Reasoning effort</DropdownMenuLabel>
                {reasoning.map((effort) => (
                  <DropdownMenuItem
                    key={effort.id}
                    className="cursor-pointer"
                    onSelect={() =>
                      onSelect(agent.id, setupChoiceFromModel(model, effort))
                    }
                  >
                    <span className="min-w-0 flex-1 truncate">
                      {effort.label}
                    </span>
                    {current.modelId === model.id &&
                      current.reasoning === effort.id && (
                        <CheckIcon className="size-3.5" />
                      )}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuSubContent>
            </DropdownMenuSub>
          );
        }
        return (
          <DropdownMenuItem
            key={model.id}
            className="cursor-pointer"
            onSelect={() => onSelect(agent.id, setupChoiceFromModel(model))}
          >
            <span className="min-w-0 flex-1 truncate">{model.label}</span>
            {current.modelId === model.id && <CheckIcon className="size-3.5" />}
          </DropdownMenuItem>
        );
      })}
    </>
  );
}

function SetupAgentRow({
  agent,
  catalogs,
  checked,
  choices,
  locked = false,
  role,
  roles,
  onCheckedChange,
  onModelChoice,
  onRoleChange,
}: {
  agent: AgentConfig;
  catalogs: AgentModelCatalog[];
  checked: boolean;
  choices: Record<string, SetupAgentModelChoice>;
  locked?: boolean;
  role?: SetupReviewRoleOption;
  roles: SetupReviewRoleOption[];
  onCheckedChange: (checked: boolean) => void;
  onModelChoice: (choice: SetupAgentModelChoice) => void;
  onRoleChange: (roleId: string) => void;
}) {
  return (
    <div
      aria-selected={checked}
      className={cn(
        "border-border/60 grid min-h-9 grid-cols-[minmax(96px,1fr)_92px_24px] items-center gap-1.5 border-b px-2.5 py-1 text-[0.78rem] last:border-b-0",
        locked ? "bg-surface-muted/75" : "bg-white",
      )}
    >
      <span className="flex min-w-0 items-center gap-2">
        <AgentProviderGlyph agent={agent} />
        <SetupAgentModelSelector
          agent={agent}
          catalogs={catalogs}
          choices={choices}
          className="text-foreground hover:text-foreground h-auto w-full flex-1 border-0 bg-transparent p-0 text-[0.79rem] shadow-none hover:border-transparent hover:bg-transparent"
          label={formatSetupAgentCompactChoiceLabel(agent, choices, catalogs)}
          onSelect={onModelChoice}
        />
      </span>
      {locked ? (
        <span className="text-muted-foreground flex h-6 w-full items-center truncate px-1.5 text-left text-[0.7rem]">
          Orchestrator
        </span>
      ) : (
        <SetupRoleSelector
          role={role ?? roles[0]}
          roles={roles}
          onSelect={onRoleChange}
        />
      )}
      {locked ? (
        <CheckIcon className="text-muted-foreground mx-auto size-3.5" />
      ) : (
        <button
          aria-label={`Remove ${agent.name}`}
          className="text-muted-foreground hover:bg-surface-muted hover:text-foreground flex size-7 cursor-pointer items-center justify-center rounded-md"
          type="button"
          onClick={() => onCheckedChange(false)}
        >
          <XIcon className="size-3.5" />
        </button>
      )}
    </div>
  );
}

function SetupRoleSelector({
  role,
  roles,
  onSelect,
}: {
  role: SetupReviewRoleOption;
  roles: SetupReviewRoleOption[];
  onSelect: (roleId: string) => void;
}) {
  const [query, setQuery] = useState("");
  const normalizedQuery = query.trim().toLowerCase();
  const visibleRoles = normalizedQuery
    ? roles.filter((item) =>
        [
          item.label,
          item.shortLabel,
          item.description,
          item.id.replaceAll("-", " "),
        ]
          .join(" ")
          .toLowerCase()
          .includes(normalizedQuery),
      )
    : roles;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          className="text-muted-foreground hover:bg-surface-muted hover:text-foreground grid h-6 w-full cursor-pointer grid-cols-[minmax(0,1fr)_12px] items-center gap-1.5 rounded-md px-1.5 text-left text-[0.7rem]"
          type="button"
        >
          <span className="truncate">{role.shortLabel}</span>
          <ChevronDownIcon className="size-3 shrink-0" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-80 p-2">
        <div className="mb-2 px-1">
          <DropdownMenuLabel className="px-0">Reviewer role</DropdownMenuLabel>
          <Input
            aria-label="Search reviewer roles"
            className="mt-1 h-8 text-[0.78rem]"
            placeholder="Search roles..."
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => event.stopPropagation()}
          />
        </div>
        <div className="max-h-72 overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {visibleRoles.map((item) => (
            <DropdownMenuItem
              key={item.id}
              className="cursor-pointer gap-2"
              onSelect={() => onSelect(item.id)}
            >
              <item.icon className="size-3.5 shrink-0" />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-[0.78rem] font-medium">
                  {item.label}
                </span>
                <span className="text-muted-foreground line-clamp-2 block text-[0.68rem] leading-4">
                  {item.description}
                </span>
              </span>
              {item.id === role.id && <CheckIcon className="size-3.5" />}
            </DropdownMenuItem>
          ))}
          {visibleRoles.length === 0 && (
            <DropdownMenuItem disabled>No matching roles</DropdownMenuItem>
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function SetupPresetTile({
  active,
  icon: Icon,
  subtitle,
  tone,
  title,
  onClick,
}: {
  active: boolean;
  icon: LucideIcon;
  subtitle: string;
  tone: string;
  title: string;
  onClick: () => void;
}) {
  return (
    <button
      aria-pressed={active}
      className={cn(
        "border-border/70 hover:bg-surface-muted flex min-h-[72px] cursor-pointer items-start gap-2.5 rounded-lg border bg-white px-3 py-2.5 text-left transition-colors",
        active &&
          "border-foreground/35 bg-surface-muted ring-foreground/10 ring-1",
      )}
      type="button"
      onClick={onClick}
    >
      <span
        className={cn(
          "flex size-6 shrink-0 items-center justify-center rounded-md border",
          tone,
          active && "bg-white",
        )}
      >
        <Icon className="size-3.5" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="line-clamp-2 text-[0.8rem] leading-4 font-semibold">
          {title}
        </span>
        <span className="text-muted-foreground mt-0.5 line-clamp-2 text-[0.7rem] leading-4">
          {subtitle}
        </span>
      </span>
      {active && <CheckIcon className="size-3.5 shrink-0" />}
    </button>
  );
}

function SetupScopeRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className="min-w-0 truncate text-right font-mono">{value}</span>
    </div>
  );
}

function SetupSourceInspectorPanel({
  canLoad,
  patchPreviews,
  preview,
  previewReady,
  projectLabel,
  rangeLabel,
  renderedFileCount,
  sourceLabel,
  stats,
  onLoad,
  onLoadMoreFiles,
}: {
  canLoad: boolean;
  patchPreviews: Record<string, Loadable<ChangedFilePatch>>;
  preview: Loadable<SetupSourcePreview>;
  previewReady: boolean;
  projectLabel: string;
  rangeLabel: string;
  renderedFileCount: number;
  sourceLabel: string;
  stats: SetupPreviewStats;
  onLoad: () => void;
  onLoadMoreFiles: () => void;
}) {
  const stackScrollRef = useRef<HTMLDivElement | null>(null);
  const previewKey =
    preview.status === "success" && previewReady ? preview.data.key : "";
  const [collapsedFileState, setCollapsedFileState] = useState<{
    ids: Set<string>;
    key: string;
  }>(() => ({ ids: new Set(), key: "" }));
  const collapsedFileIds =
    collapsedFileState.key === previewKey
      ? collapsedFileState.ids
      : emptyCollapsedFileIds;
  const isLoading = preview.status === "loading";
  const actionLabel = isLoading
    ? "Loading..."
    : previewReady
      ? "Refresh source"
      : "Load source details";
  const visibleFiles =
    preview.status === "success" && previewReady
      ? preview.data.files.slice(
          0,
          Math.min(renderedFileCount, setupMaxRenderedDiffFiles),
        )
      : [];
  const hiddenFileCount =
    preview.status === "success" && previewReady
      ? Math.max(preview.data.files.length - visibleFiles.length, 0)
      : 0;
  const hiddenStats =
    preview.status === "success" && previewReady
      ? preview.data.files.slice(visibleFiles.length).reduce(
          (totals, file) => ({
            additions: totals.additions + file.additions,
            deletions: totals.deletions + file.deletions,
          }),
          { additions: 0, deletions: 0 },
        )
      : { additions: 0, deletions: 0 };

  useEffect(() => {
    if (
      preview.status !== "success" ||
      !previewReady ||
      hiddenFileCount === 0
    ) {
      return;
    }

    const frame = window.requestAnimationFrame(() => {
      const element = stackScrollRef.current;
      if (!element) {
        return;
      }
      if (element.scrollHeight <= element.clientHeight + 96) {
        onLoadMoreFiles();
      }
    });

    return () => window.cancelAnimationFrame(frame);
  }, [
    hiddenFileCount,
    onLoadMoreFiles,
    preview.status,
    previewReady,
    visibleFiles.length,
  ]);

  function handleStackScroll(event: UIEvent<HTMLDivElement>) {
    const target = event.currentTarget;
    if (
      hiddenFileCount > 0 &&
      target.scrollTop + target.clientHeight >= target.scrollHeight - 560
    ) {
      onLoadMoreFiles();
    }
  }

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden bg-white">
      <div className="flex items-start justify-between gap-3 px-4 pt-4 pb-3">
        <div className="min-w-0">
          <h2 className="text-[0.96rem] font-semibold">Source details</h2>
          <p className="text-muted-foreground mt-1 truncate text-xs leading-4">
            {sourceLabel} / {projectLabel} / {rangeLabel}
          </p>
        </div>
        <Button
          className="h-8 cursor-pointer"
          disabled={!canLoad || isLoading}
          size="sm"
          variant="outline"
          onClick={onLoad}
        >
          {actionLabel}
          <RefreshCwIcon data-icon="inline-end" />
        </Button>
      </div>

      {preview.status === "loading" && (
        <div className="px-4 py-3">
          <SetupDiffSkeleton rows={6} />
        </div>
      )}

      {preview.status === "error" && (
        <div className="border-destructive/20 bg-destructive/5 text-destructive mt-4 rounded-lg border px-3 py-2 text-[0.74rem] leading-5 break-words">
          {preview.error.message}
        </div>
      )}

      {preview.status === "success" && !previewReady && (
        <div className="text-warning border-border/70 mt-4 rounded-lg border bg-white px-3 py-2 text-[0.74rem] leading-5">
          Source inputs changed. Refresh source details to update the file list.
        </div>
      )}

      {preview.status !== "loading" &&
        preview.status !== "error" &&
        (preview.status !== "success" || !previewReady) && (
          <div className="border-border/70 text-muted-foreground mt-4 flex min-h-[180px] flex-1 items-center justify-center rounded-lg border bg-white">
            <FileSearchIcon className="mr-2 size-4" />
            <span className="text-[0.78rem]">Not loaded</span>
          </div>
        )}

      {preview.status === "success" && previewReady && (
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
          <div className="border-border/60 flex min-h-10 shrink-0 items-center justify-between gap-3 border-y px-4">
            <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
              <span className="text-muted-foreground text-[0.86rem]">
                Changed files
              </span>
              <Badge className="h-6 px-2 text-[0.72rem]" variant="secondary">
                {formatPreviewNumber(stats.total)}
              </Badge>
              {(stats.generated > 0 ||
                stats.binary > 0 ||
                stats.excluded > 0) && (
                <span className="text-muted-foreground text-[0.68rem]">
                  {[
                    stats.generated > 0
                      ? `${formatPreviewNumber(stats.generated)} generated`
                      : "",
                    stats.binary > 0
                      ? `${formatPreviewNumber(stats.binary)} binary`
                      : "",
                    stats.excluded > 0
                      ? `${formatPreviewNumber(stats.excluded)} excluded`
                      : "",
                  ]
                    .filter(Boolean)
                    .join(" / ")}
                </span>
              )}
            </div>
            <span className="shrink-0 font-mono text-[0.88rem]">
              <span className="text-emerald-700">
                +{formatPreviewNumber(stats.additions)}
              </span>{" "}
              <span className="text-red-700">
                -{formatPreviewNumber(stats.deletions)}
              </span>
            </span>
          </div>

          <div
            ref={stackScrollRef}
            className="min-h-0 flex-1 overflow-y-auto overscroll-contain [scrollbar-gutter:stable]"
            data-testid="setup-source-stack-scroll"
            onScroll={handleStackScroll}
          >
            {visibleFiles.length === 0 && (
              <div className="text-muted-foreground flex min-h-[180px] items-center justify-center rounded-lg border bg-white text-[0.78rem]">
                No changed files
              </div>
            )}
            {visibleFiles.map((file) => (
              <SetupFileDiffPreview
                key={file.id}
                file={file}
                patchPreview={patchPreviews[file.id]}
                collapsed={collapsedFileIds.has(file.id)}
                onToggleCollapsed={() =>
                  setCollapsedFileState((current) => {
                    const currentIds =
                      current.key === previewKey
                        ? current.ids
                        : emptyCollapsedFileIds;
                    const next = new Set(currentIds);
                    if (next.has(file.id)) {
                      next.delete(file.id);
                    } else {
                      next.add(file.id);
                    }
                    return { ids: next, key: previewKey };
                  })
                }
              />
            ))}
            {hiddenFileCount > 0 && (
              <div className="text-muted-foreground flex h-10 items-center justify-between gap-3 px-4 text-[0.72rem]">
                <span>
                  {formatPreviewNumber(hiddenFileCount)} more changed files
                </span>
                <span className="font-mono">
                  <span className="text-emerald-700">
                    +{formatPreviewNumber(hiddenStats.additions)}
                  </span>{" "}
                  <span className="text-red-700">
                    -{formatPreviewNumber(hiddenStats.deletions)}
                  </span>
                </span>
              </div>
            )}
          </div>
        </div>
      )}
    </section>
  );
}

function SetupFileDiffPreview({
  file,
  collapsed,
  patchPreview,
  onToggleCollapsed,
}: {
  file: ChangedFile;
  collapsed: boolean;
  patchPreview?: Loadable<ChangedFilePatch>;
  onToggleCollapsed: () => void;
}) {
  const unavailableReason = file.is_binary
    ? "Binary files do not have a text diff preview."
    : !file.patch_artifact_id
      ? "No text patch was stored for this file."
      : "";
  const hasSelectedPatch =
    patchPreview?.status === "success" &&
    patchPreview.data.changed_file_id === file.id;

  return (
    <article className="border-border/60 flex min-h-0 flex-col overflow-hidden border-b bg-white">
      <button
        aria-expanded={!collapsed}
        className="hover:bg-surface-muted/50 flex min-h-9 w-full cursor-pointer items-center justify-between gap-3 px-4 text-left transition-colors"
        type="button"
        onClick={onToggleCollapsed}
      >
        <div className="flex min-w-0 items-center gap-2">
          <ChevronDownIcon
            className={cn(
              "text-muted-foreground size-3.5 shrink-0 transition-transform",
              collapsed && "-rotate-90",
            )}
          />
          <FileTextIcon
            className={cn(
              "text-muted-foreground size-3.5 shrink-0",
              file.is_binary && "text-amber-700",
              file.is_excluded && "text-muted-foreground/60",
            )}
          />
          <span className="min-w-0 truncate font-mono text-[0.72rem]">
            {file.path}
          </span>
        </div>
        <span className="shrink-0 font-mono text-[0.72rem]">
          <span className="text-emerald-700">
            +{formatPreviewNumber(file.additions)}
          </span>{" "}
          <span className="text-red-700">
            -{formatPreviewNumber(file.deletions)}
          </span>
        </span>
      </button>

      {!collapsed && (
        <div className="border-border/60 border-t">
          {unavailableReason && (
            <div className="text-muted-foreground flex min-h-0 items-center justify-center px-4 py-5 text-center text-[0.78rem] leading-5">
              {unavailableReason}
            </div>
          )}
          {!unavailableReason &&
            (!patchPreview || patchPreview.status === "loading") && (
              <div className="px-4 py-3">
                <SetupDiffSkeleton rows={4} />
              </div>
            )}
          {!unavailableReason &&
            patchPreview?.status === "success" &&
            !hasSelectedPatch && (
              <div className="px-4 py-3">
                <SetupDiffSkeleton rows={4} />
              </div>
            )}
          {!unavailableReason && patchPreview?.status === "error" && (
            <div className="text-destructive flex min-h-0 items-center justify-center px-4 py-5 text-center text-[0.78rem] leading-5">
              {patchPreview.error.message}
            </div>
          )}
          {!unavailableReason && patchPreview?.status === "success" && (
            <>
              {hasSelectedPatch && (
                <SetupDiffContent patch={patchPreview.data} />
              )}
            </>
          )}
        </div>
      )}
    </article>
  );
}

function SetupDiffSkeleton({ rows }: { rows: number }) {
  return (
    <div className="grid gap-2" aria-label="Loading diff">
      {Array.from({ length: rows }, (_, index) => (
        <div
          key={index}
          className="bg-surface-muted/80 h-3 rounded-full"
          style={{ width: `${92 - (index % 4) * 13}%` }}
        />
      ))}
    </div>
  );
}

function SetupDiffContent({ patch }: { patch: ChangedFilePatch }) {
  const rows = setupSideBySideDiffRows(patch.content);
  if (rows.length === 0) {
    return (
      <div className="text-muted-foreground flex min-h-0 flex-1 items-center justify-center px-4 text-center text-[0.78rem] leading-5">
        No diff content.
      </div>
    );
  }
  return (
    <>
      <div
        className="overflow-x-auto overscroll-contain bg-white [scrollbar-gutter:stable_both-edges]"
        data-testid="setup-diff-scroll"
      >
        <div className="grid w-max min-w-full auto-rows-min grid-cols-[42px_minmax(320px,max-content)_42px_minmax(320px,max-content)]">
          <span className="border-border/60 text-muted-foreground sticky top-0 z-[2] border-b bg-white px-2 py-1.5 text-right text-[0.64rem] font-medium tracking-[0.02em] uppercase">
            Old
          </span>
          <span className="border-border/60 text-muted-foreground sticky top-0 z-[2] border-b bg-white px-2 py-1.5 text-[0.64rem] font-medium tracking-[0.02em] uppercase">
            Before
          </span>
          <span className="border-border/60 text-muted-foreground sticky top-0 z-[2] border-b border-l bg-white px-2 py-1.5 text-right text-[0.64rem] font-medium tracking-[0.02em] uppercase">
            New
          </span>
          <span className="border-border/60 text-muted-foreground sticky top-0 z-[2] border-b bg-white px-2 py-1.5 text-[0.64rem] font-medium tracking-[0.02em] uppercase">
            After
          </span>
          {rows.map((row, index) => (
            <SetupSideBySideDiffRow
              key={`${index}:${row.oldLineNumber ?? ""}:${row.newLineNumber ?? ""}:${row.oldText}:${row.newText}`}
              row={row}
            />
          ))}
        </div>
      </div>
      {(patch.content_truncated ||
        patch.content.split("\n").length > setupDiffPreviewLineLimit) && (
        <div className="text-muted-foreground border-border/60 shrink-0 border-t px-3 py-2 text-[0.68rem]">
          Preview truncated for performance.
        </div>
      )}
    </>
  );
}

function SetupSideBySideDiffRow({ row }: { row: SetupSideBySideDiffRowData }) {
  if (row.tone === "meta" || row.tone === "hunk") {
    return (
      <>
        <span className={cn("bg-white text-[0.68rem] leading-5 select-none")} />
        <code
          className={cn(
            "col-span-3 px-2 font-mono text-[0.68rem] leading-5 whitespace-pre",
            row.tone === "hunk"
              ? "bg-white text-blue-800"
              : "text-muted-foreground bg-white",
          )}
        >
          {row.newText || row.oldText || " "}
        </code>
      </>
    );
  }
  return (
    <>
      <span
        className={cn(
          "text-muted-foreground/75 pr-2 text-right font-mono text-[0.68rem] leading-5 select-none",
          setupDiffLineNumberTone(row.tone, "old"),
        )}
      >
        {row.oldLineNumber ?? ""}
      </span>
      <code
        className={cn(
          "px-2 font-mono text-[0.68rem] leading-5 whitespace-pre",
          setupDiffCellTone(row.tone, "old"),
        )}
      >
        {row.oldText || " "}
      </code>
      <span
        className={cn(
          "border-border/60 text-muted-foreground/75 border-l pr-2 text-right font-mono text-[0.68rem] leading-5 select-none",
          setupDiffLineNumberTone(row.tone, "new"),
        )}
      >
        {row.newLineNumber ?? ""}
      </span>
      <code
        className={cn(
          "px-2 font-mono text-[0.68rem] leading-5 whitespace-pre",
          setupDiffCellTone(row.tone, "new"),
        )}
      >
        {row.newText || " "}
      </code>
    </>
  );
}

function formatPreviewNumber(value: number) {
  return value.toLocaleString("en-US");
}

const setupDiffPreviewLineLimit = 500;

type SetupSideBySideDiffTone =
  | "add"
  | "change"
  | "context"
  | "delete"
  | "hunk"
  | "meta";

type SetupSideBySideDiffRowData = {
  newLineNumber?: number;
  newText: string;
  oldLineNumber?: number;
  oldText: string;
  tone: SetupSideBySideDiffTone;
};

function setupSideBySideDiffRows(
  content: string,
): SetupSideBySideDiffRowData[] {
  const rows: SetupSideBySideDiffRowData[] = [];
  const pendingDeletes: Array<{ number?: number; text: string }> = [];
  let oldLineNumber = 0;
  let newLineNumber = 0;

  function flushDeletes() {
    for (const deletion of pendingDeletes.splice(0)) {
      rows.push({
        oldLineNumber: deletion.number,
        oldText: deletion.text,
        newText: "",
        tone: "delete",
      });
    }
  }

  for (const line of content.split("\n").slice(0, setupDiffPreviewLineLimit)) {
    if (setupDiffLineIsMetadata(line)) {
      flushDeletes();
      rows.push({ oldText: "", newText: line, tone: "meta" });
      continue;
    }
    if (line.startsWith("@@")) {
      flushDeletes();
      const hunk = /@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(line);
      oldLineNumber = hunk ? Number(hunk[1]) : oldLineNumber;
      newLineNumber = hunk ? Number(hunk[2]) : newLineNumber;
      rows.push({ oldText: "", newText: line, tone: "hunk" });
      continue;
    }
    if (line.startsWith("-")) {
      pendingDeletes.push({
        number: oldLineNumber || undefined,
        text: line.slice(1),
      });
      if (oldLineNumber > 0) {
        oldLineNumber += 1;
      }
      continue;
    }
    if (line.startsWith("+")) {
      const deletion = pendingDeletes.shift();
      rows.push({
        newLineNumber: newLineNumber || undefined,
        newText: line.slice(1),
        oldLineNumber: deletion?.number,
        oldText: deletion?.text ?? "",
        tone: deletion ? "change" : "add",
      });
      if (newLineNumber > 0) {
        newLineNumber += 1;
      }
      continue;
    }
    flushDeletes();
    const text = line.startsWith(" ") ? line.slice(1) : line;
    rows.push({
      newLineNumber: newLineNumber || undefined,
      newText: text,
      oldLineNumber: oldLineNumber || undefined,
      oldText: text,
      tone: "context",
    });
    if (oldLineNumber > 0) {
      oldLineNumber += 1;
    }
    if (newLineNumber > 0) {
      newLineNumber += 1;
    }
  }

  flushDeletes();
  return rows;
}

function setupDiffLineIsMetadata(line: string) {
  return (
    line.startsWith("diff --git") ||
    line.startsWith("index ") ||
    line.startsWith("--- ") ||
    line.startsWith("+++ ") ||
    line.startsWith("new file mode ") ||
    line.startsWith("deleted file mode ") ||
    line.startsWith("similarity index ") ||
    line.startsWith("rename from ") ||
    line.startsWith("rename to ") ||
    line.startsWith("Binary files ")
  );
}

function setupDiffCellTone(tone: SetupSideBySideDiffTone, side: "new" | "old") {
  if (tone === "change") {
    return side === "old"
      ? "bg-red-50 text-red-900"
      : "bg-emerald-50 text-emerald-900";
  }
  if (tone === "delete" && side === "old") {
    return "bg-red-50 text-red-900";
  }
  if (tone === "add" && side === "new") {
    return "bg-emerald-50 text-emerald-900";
  }
  if (
    (tone === "delete" && side === "new") ||
    (tone === "add" && side === "old")
  ) {
    return "bg-white text-muted-foreground";
  }
  return "bg-white text-foreground";
}

function setupDiffLineNumberTone(
  tone: SetupSideBySideDiffTone,
  side: "new" | "old",
) {
  if ((tone === "delete" || tone === "change") && side === "old") {
    return "bg-red-50 text-red-700";
  }
  if ((tone === "add" || tone === "change") && side === "new") {
    return "bg-emerald-50 text-emerald-700";
  }
  if (
    (tone === "delete" && side === "new") ||
    (tone === "add" && side === "old")
  ) {
    return "bg-white";
  }
  return "bg-white";
}

function AgentProviderGlyph({ agent }: { agent: AgentVisibilitySource }) {
  const logo = agentLogoUrl(agent);
  if (logo) {
    return (
      <img
        alt=""
        className="size-4 shrink-0 rounded-[3px] object-contain"
        src={logo}
      />
    );
  }
  return <CircleIcon className="size-4 shrink-0" />;
}

function setupReviewAgentAssignments(
  agents: AgentConfig[],
  agentIds: string[],
  roleIds: string[],
): SetupReviewAgentAssignment[] {
  const agentById = new Map(agents.map((agent) => [agent.id, agent]));
  const pool = agentIds
    .map((id) => agentById.get(id))
    .filter((agent): agent is AgentConfig => Boolean(agent));
  if (pool.length === 0) {
    return [];
  }
  const roles = roleIds.length > 0 ? roleIds : [];

  return roles.map((roleId, index) => {
    const role =
      setupReviewRoleById(roleId) ??
      setupReviewRoleById("general-reviewer") ??
      setupReviewRoleOptions[0];
    return {
      id: `preset-review-agent:${role.id}:${index}`,
      agent: pool[index % pool.length],
      role,
      index,
      manual: false,
    };
  });
}

function setupManualReviewAgentAssignments(
  agents: AgentConfig[],
  assignments: ManualReviewAgentAssignment[],
  startIndex: number,
): SetupReviewAgentAssignment[] {
  const agentById = new Map(agents.map((agent) => [agent.id, agent]));
  const rows: SetupReviewAgentAssignment[] = [];
  for (const [index, assignment] of assignments.entries()) {
    const agent = agentById.get(assignment.agentId);
    if (!agent) {
      continue;
    }
    const role =
      setupReviewRoleById(assignment.roleId) ??
      setupReviewRoleById("general-reviewer") ??
      setupReviewRoleOptions[0];
    rows.push({
      id: assignment.id,
      agent,
      role,
      index: startIndex + index,
      manual: true,
    });
  }
  return rows;
}

function setupReviewRoleById(roleId?: string) {
  return setupReviewRoleOptions.find((role) => role.id === roleId);
}

function setupRoleIdsForPresets(selectedPresetIds: Set<string>) {
  const roleIds: string[] = [];
  const seen = new Set<string>();
  const selectedPresets = setupPresetOptions.filter((preset) =>
    selectedPresetIds.has(preset.id),
  );
  for (const preset of selectedPresets) {
    for (const roleId of preset.roleIds) {
      if (!seen.has(roleId)) {
        seen.add(roleId);
        roleIds.push(roleId);
      }
    }
  }
  return roleIds;
}

function setupFocusPrompt(prompt: string, labels: string[]) {
  const trimmed = prompt.trim();
  const focusLine =
    labels.length > 0 ? `Focus areas: ${labels.join(", ")}.` : "";
  return [trimmed, focusLine].filter(Boolean).join("\n\n");
}

function setupDefaultBaseRef(
  repository?: Repository,
  branches: RepositoryBranch[] = [],
) {
  const primary = setupPreferredBranch(branches, ["main", "master"]);
  if (primary) {
    return primary;
  }
  const fallback = setupPreferredBranch(branches, ["develop", "dev", "trunk"]);
  if (fallback) {
    return fallback;
  }
  const repositoryDefault = repository?.default_branch?.trim();
  if (
    repositoryDefault &&
    repositoryDefault !== "HEAD" &&
    branches.some((item) => item.name === repositoryDefault)
  ) {
    return repositoryDefault;
  }
  return repositoryDefault && repositoryDefault !== "HEAD"
    ? repositoryDefault
    : "main";
}

function setupPreferredBranch(branches: RepositoryBranch[], names: string[]) {
  for (const name of names) {
    const local = branches.find(
      (branch) => !branch.remote && branch.name === name,
    );
    if (local) {
      return local.name;
    }
  }
  for (const name of names) {
    const remote = branches.find(
      (branch) =>
        branch.remote &&
        (branch.name === name || branch.name.endsWith(`/${name}`)),
    );
    if (remote) {
      return remote.name;
    }
  }
  return "";
}

function setupRuntimeLimitSeconds(depth: "quick" | "standard" | "deep") {
  if (depth === "deep") {
    return 2700;
  }
  if (depth === "quick") {
    return 900;
  }
  return 1800;
}

function setupSourceKey({
  baseRef,
  githubAuthMethod,
  githubUrl,
  headRef,
  repositoryId,
  source,
}: {
  baseRef: string;
  githubAuthMethod: string;
  githubUrl: string;
  headRef: string;
  repositoryId: string;
  source: SnapshotSource;
}) {
  const parts = [repositoryId.trim(), source];
  if (source === "github") {
    parts.push(githubUrl.trim(), githubAuthMethod.trim());
  }
  if (source === "branch-compare") {
    parts.push(baseRef.trim(), headRef.trim());
  }
  return parts.join("\u001f");
}

function setupPreviewStats(files: ChangedFile[]): SetupPreviewStats {
  return files.reduce<SetupPreviewStats>(
    (stats, file) => ({
      total: stats.total + 1,
      reviewable:
        stats.reviewable + (file.is_binary || file.is_excluded ? 0 : 1),
      additions: stats.additions + file.additions,
      deletions: stats.deletions + file.deletions,
      generated: stats.generated + (file.is_generated ? 1 : 0),
      binary: stats.binary + (file.is_binary ? 1 : 0),
      excluded: stats.excluded + (file.is_excluded ? 1 : 0),
    }),
    {
      total: 0,
      reviewable: 0,
      additions: 0,
      deletions: 0,
      generated: 0,
      binary: 0,
      excluded: 0,
    },
  );
}

function toggleSetValue(values: Set<string>, value: string) {
  const next = new Set(values);
  if (next.has(value)) {
    next.delete(value);
  } else {
    next.add(value);
  }
  return next;
}

function removeRecordKey<T>(record: Record<string, T>, key: string) {
  if (!(key in record)) {
    return record;
  }
  const next = { ...record };
  delete next[key];
  return next;
}

function snapshotTitle(snapshot: Snapshot, repository?: Repository): string {
  if (snapshot.pr_title) {
    return snapshot.pr_title;
  }
  if (snapshot.pr_number && snapshot.owner && snapshot.repo) {
    return `${snapshot.owner}/${snapshot.repo}#${snapshot.pr_number}`;
  }
  if (snapshot.source_type === "branch_compare") {
    return `${repository?.name ?? "Repository"} ${snapshot.base_ref ?? "base"}..${snapshot.head_ref ?? "head"}`;
  }
  if (snapshot.source_type === "local_changes") {
    return `${repository?.name ?? "Repository"} local changes`;
  }
  return `Review ${snapshot.id}`;
}
