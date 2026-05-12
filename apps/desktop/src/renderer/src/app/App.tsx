import {
  Fragment,
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  ArrowDownIcon,
  ArrowLeftIcon,
  ArrowUpIcon,
  BellIcon,
  BookOpenIcon,
  BotIcon,
  CheckIcon,
  ChevronDownIcon,
  CircleIcon,
  ClockIcon,
  Code2Icon,
  CopyIcon,
  ExternalLinkIcon,
  FileSearchIcon,
  FileTextIcon,
  FolderOpenIcon,
  GitBranchIcon,
  GitPullRequestIcon,
  InboxIcon,
  MapIcon,
  MessageSquareIcon,
  PauseIcon,
  PlusIcon,
  RefreshCwIcon,
  SearchIcon,
  SendIcon,
  SettingsIcon,
  ShieldCheckIcon,
  TerminalIcon,
  Trash2Icon,
} from "lucide-react";

import {
  AppShell,
  EmptyState,
  ErrorState,
  LoadingRows,
  PaneHeader,
  SearchCommandDialog,
  SidebarNavButton,
  SidebarSection,
  TooltipIconButton,
  type SearchCommandGroup,
} from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  InputGroup,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { Toaster } from "@/components/ui/sonner";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import {
  type AgentConfig,
  type AgentConfigHealth,
  type AgentConfigInput,
  type AgentModelCatalog,
  type AgentPreset,
  type ApiClient,
  type ContextBundlePreview,
  createCocodeClient,
  type DeleteGitHubCredentialResponse,
  errorApiState,
  type GitHubCredentialStatusResponse,
  idleApiState,
  loadApiResource,
  loadingApiState,
  preserveSuccessfulLoadable,
  successApiState,
  type ApiSessionResponse,
  type Loadable,
  type AskFindingQuestionResponse,
  type EvidenceItem,
  type EvidenceMapCallPath,
  type EvidenceMapCallPathStep,
  type EvidenceMapEdge,
  type EvidenceMapFinding,
  type EvidenceMapGraphRef,
  type EvidenceMapHierarchyItem,
  type EvidenceMapNode,
  type EvidenceMapPanelEvidenceRef,
  type EvidenceMapResponse,
  type Finding,
  type FindingDetailResponse,
  type FindingListResponse,
  type FindingQuickActionResponse,
  type FindingSourceAgent,
  type FindingThreadView,
  type CreateCopyPacketResponse,
  type GitHubPreviewResponse,
  type OpenRepositoryResponse,
  type Repository,
  type ReviewContextPolicy,
  type RedactionReportItem,
  type ReviewAuditLogEntry,
  type ReviewAuditLogResponse,
  type ReviewEvent,
  type ReviewRule,
  type ReviewRuleListResponse,
  type ReviewSessionSummary,
  type ReviewSession,
  type SettingsExportPayload,
  type SettingsImportResponse,
  type Workspace,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import { AgentRuntimeTrace } from "./agent-runtime-trace";
import {
  agentEgress,
  agentProvider,
  applyDiscoveredModel,
  BUILTIN_REVIEW_AGENT_PRESET_IDS,
  formatSetupAgentLabel,
} from "./agent-utils";
import { CentralizedChatScreen } from "./centralized-chat-screen";
import { MarkdownMessage } from "./markdown-message";
import { NewThreadScreen } from "./new-thread-screen";

const MAX_SIDEBAR_SESSIONS = 12;
const MAX_SEARCH_RESULTS = 5;
const MAX_REVIEW_EVENTS_RENDERED = 20000;
const MAX_REVIEW_EVENTS_PER_AGENT_RUN = 2500;
const MAX_NON_AGENT_RUN_EVENTS = 1200;
const MAX_AUDIT_ENTRIES_RENDERED = 120;
const MAX_FINDINGS_RENDERED = 150;
const MAX_CODE_LINES_RENDERED = 80;
const EVIDENCE_MAP_NODE_WIDTH = 232;
const EVIDENCE_MAP_NODE_HEIGHT = 82;
const EVIDENCE_MAP_COLUMN_GAP = 326;
const REVIEW_THREAD_TAB_CLASS =
  "mt-4 min-h-0 overflow-y-auto pr-1 pl-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden";
const COMPOSER_RUNTIME_POLICIES = {
  quick: { max_tokens: 4_000, max_items: 40 },
  standard: { max_tokens: 8_000, max_items: 80 },
  deep: { max_tokens: 12_000, max_items: 120 },
} satisfies Record<
  ComposerRuntime,
  Pick<ReviewContextPolicy, "max_tokens" | "max_items">
>;

type ComposerMode = "review" | "finding follow-up";
type ComposerRuntime = "quick" | "standard" | "deep";
type ComposerReasoning = "low" | "medium" | "high";
type ComposerPermission = "review-mode" | "local-only";

export function composerContextPolicy(
  runtime: ComposerRuntime,
  permission: ComposerPermission,
): ReviewContextPolicy {
  const limits = COMPOSER_RUNTIME_POLICIES[runtime];
  return {
    ...limits,
    redact_secrets: true,
    include_prior_comments: permission === "review-mode",
    include_prior_decisions: true,
  };
}

async function loadAgentConfigs(
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

async function loadAgentModelCatalogs(
  client: ApiClient,
  options: { refresh?: boolean } = {},
) {
  return loadApiResource(() => client.listAgentModelCatalog(options));
}

function shouldRecheckAgentModelCatalogs(catalogs: AgentModelCatalog[]) {
  return catalogs.some((catalog) => catalog.stale || catalog.refreshing);
}

type MainView = "new-thread" | "review" | "agent-settings";
type PromptDelivery = "stdin" | "arg" | "temp_file";
type FindingStatusFilter =
  | "all"
  | "needs_triage"
  | "verified"
  | "accepted"
  | "dismissed"
  | "deferred"
  | "copied"
  | "published";
type FindingSeverityFilter =
  | "all"
  | "blocker"
  | "high"
  | "medium"
  | "low"
  | "info";
type ReviewRefreshState =
  | { status: "idle" }
  | { status: "refreshing" }
  | { status: "error"; message: string };

const changedFiles = [
  { path: "api/routes/billing.go", additions: 132, deletions: 18 },
  { path: "middleware/auth.go", additions: 89, deletions: 4 },
  { path: "handlers/payouts.go", additions: 64, deletions: 7 },
  { path: "tests/billing_routes_test.go", additions: 25, deletions: 1 },
];

const findings = [
  {
    title: "Auth middleware skipped on billing route",
    file: "api/routes/billing.go",
    lines: "L132-L135",
    severity: "High",
    status: "Verified",
  },
  {
    title: "Webhook payload not validated",
    file: "api/webhooks/stripe.go",
    lines: "L78-L92",
    severity: "Medium",
    status: "Needs triage",
  },
  {
    title: "Admin export route lacks role check",
    file: "api/routes/admin.go",
    lines: "L41-L48",
    severity: "High",
    status: "Needs triage",
  },
];

export function App() {
  const [client, setClient] = useState<ApiClient | null>(null);
  const [mainView, setMainView] = useState<MainView>("new-thread");
  const [backendStatus, setBackendStatus] = useState("loading");
  const [apiSession, setApiSession] =
    useState<Loadable<ApiSessionResponse>>(loadingApiState);
  const [workspaces, setWorkspaces] =
    useState<Loadable<Workspace[]>>(idleApiState());
  const [repositories, setRepositories] =
    useState<Loadable<Repository[]>>(idleApiState());
  const [reviewSessions, setReviewSessions] =
    useState<Loadable<ReviewSession[]>>(idleApiState());
  const [repositoryOpenState, setRepositoryOpenState] =
    useState<Loadable<OpenRepositoryResponse>>(idleApiState());
  const [agentConfigs, setAgentConfigs] =
    useState<Loadable<AgentConfig[]>>(idleApiState());
  const [agentModelCatalogs, setAgentModelCatalogs] =
    useState<Loadable<AgentModelCatalog[]>>(idleApiState());
  const [agentBootstrapState, setAgentBootstrapState] = useState<
    "idle" | "loading" | "done"
  >("idle");
  const [currentReviewSession, setCurrentReviewSession] =
    useState<ReviewSession | null>(null);
  const [activeWorkspaceId, setActiveWorkspaceId] = useState("");
  const [activeRepositoryId, setActiveRepositoryId] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const [deletingReviewSessionId, setDeletingReviewSessionId] = useState("");

  useEffect(() => {
    let appZoom = 1;

    function setAppZoom(nextZoom: number) {
      appZoom = Math.min(1.4, Math.max(0.75, Math.round(nextZoom * 10) / 10));
      document.documentElement.style.zoom =
        appZoom === 1 ? "" : String(appZoom);
    }

    function handleAppShortcuts(event: KeyboardEvent) {
      const hasPlatformModifier = event.metaKey || event.ctrlKey;
      if (!hasPlatformModifier || event.altKey) {
        return;
      }

      const key = event.key.toLowerCase();
      const code = event.code;
      if (key === "k") {
        event.preventDefault();
        setSearchOpen(true);
        return;
      }
      if (key === "=" || key === "+" || code === "Equal") {
        event.preventDefault();
        setAppZoom(appZoom + 0.1);
        return;
      }
      if (key === "-" || key === "_" || code === "Minus") {
        event.preventDefault();
        setAppZoom(appZoom - 0.1);
        return;
      }
      if (key === "0" || code === "Digit0" || code === "Numpad0") {
        event.preventDefault();
        setAppZoom(1);
      }
    }

    window.addEventListener("keydown", handleAppShortcuts);
    return () => {
      document.documentElement.style.zoom = "";
      window.removeEventListener("keydown", handleAppShortcuts);
    };
  }, []);

  const loadWorkspaceDetails = useCallback(
    async (api: ApiClient, workspace: Workspace) => {
      setRepositories(loadingApiState());
      setReviewSessions(loadingApiState());
      const [repositoryState, sessionState] = await Promise.all([
        loadApiResource(() => api.listRepositories(workspace.id)),
        loadApiResource(() => api.listReviewSessions(workspace.id)),
      ]);

      setRepositories(repositoryState);
      setReviewSessions(sessionState);
      if (repositoryState.status === "success") {
        const nextRepository =
          repositoryState.data.find(
            (repository) => repository.id === workspace.default_repo_id,
          ) ?? repositoryState.data[0];
        setActiveRepositoryId(nextRepository?.id ?? "");
      }
    },
    [],
  );

  const refreshNavigation = useCallback(
    async (api: ApiClient, preferredWorkspaceId = "") => {
      setWorkspaces(loadingApiState());
      const workspaceState = await loadApiResource(() => api.listWorkspaces());
      setWorkspaces(workspaceState);

      if (workspaceState.status !== "success") {
        setRepositories(successApiState([]));
        setReviewSessions(successApiState([]));
        setActiveWorkspaceId("");
        setActiveRepositoryId("");
        return;
      }

      const nextWorkspace =
        workspaceState.data.find(
          (workspace) => workspace.id === preferredWorkspaceId,
        ) ?? workspaceState.data[0];

      if (!nextWorkspace) {
        setRepositories(successApiState([]));
        setReviewSessions(successApiState([]));
        setActiveWorkspaceId("");
        setActiveRepositoryId("");
        return;
      }

      setActiveWorkspaceId(nextWorkspace.id);
      await loadWorkspaceDetails(api, nextWorkspace);
    },
    [loadWorkspaceDetails],
  );

  useEffect(() => {
    let canceled = false;

    const bridge = window.cocode;
    if (!bridge) {
      queueMicrotask(() => {
        if (!canceled) {
          setBackendStatus("unavailable");
          setApiSession(
            errorApiState(new Error("Desktop bridge is unavailable")),
          );
        }
      });
      return () => {
        canceled = true;
      };
    }

    void bridge
      .getBackendInfo()
      .then((info) => {
        if (canceled) {
          return;
        }
        setBackendStatus(info.status);

        const nextClient = createCocodeClient(info);
        setClient(nextClient);
        void loadApiResource(() => nextClient.session()).then((state) => {
          if (canceled) {
            return;
          }
          setApiSession(state);
          if (state.status === "error") {
            setBackendStatus("unavailable");
            return;
          }
          void refreshNavigation(nextClient);
          setAgentModelCatalogs(loadingApiState());
          setAgentConfigs(loadingApiState());
          void (async () => {
            const initialAgentState = await loadAgentConfigs(nextClient, {
              bootstrapBuiltIns: true,
              modelCatalogs: [],
            });
            if (canceled) {
              return;
            }
            setAgentConfigs(initialAgentState);

            const catalogState = await loadAgentModelCatalogs(nextClient);
            if (canceled) {
              return;
            }
            setAgentModelCatalogs(catalogState);
            const catalogs =
              catalogState.status === "success" ? catalogState.data : undefined;
            const agentState = await loadAgentConfigs(nextClient, {
              bootstrapBuiltIns: true,
              modelCatalogs: catalogs,
            });
            if (canceled) {
              return;
            }
            setAgentConfigs(agentState);
            if (
              catalogState.status === "success" &&
              shouldRecheckAgentModelCatalogs(catalogState.data)
            ) {
              window.setTimeout(() => {
                if (canceled) {
                  return;
                }
                void loadAgentModelCatalogs(nextClient).then((nextState) => {
                  if (!canceled) {
                    setAgentModelCatalogs(nextState);
                  }
                });
              }, 1500);
            }
          })();
        });
      })
      .catch(() => {
        if (!canceled) {
          setApiSession(
            errorApiState(new Error("Backend info is unavailable")),
          );
          setBackendStatus("unavailable");
        }
      });

    return () => {
      canceled = true;
    };
  }, [refreshNavigation]);

  const workspaceList = useMemo(
    () => (workspaces.status === "success" ? workspaces.data : []),
    [workspaces],
  );
  const repositoryList = useMemo(
    () => (repositories.status === "success" ? repositories.data : []),
    [repositories],
  );
  const sessionList = useMemo(
    () => (reviewSessions.status === "success" ? reviewSessions.data : []),
    [reviewSessions],
  );
  const activeWorkspace =
    workspaceList.find((workspace) => workspace.id === activeWorkspaceId) ??
    workspaceList[0];
  const activeRepository =
    repositoryList.find((repository) => repository.id === activeRepositoryId) ??
    repositoryList[0];
  const activeSession = sessionList[0];
  const displayedSession = currentReviewSession ?? activeSession;

  const handleSelectWorkspace = useCallback(
    (workspaceId: string) => {
      const selectedWorkspace = workspaceList.find(
        (workspace) => workspace.id === workspaceId,
      );
      if (!client || !selectedWorkspace) {
        return;
      }
      setActiveWorkspaceId(workspaceId);
      void loadWorkspaceDetails(client, selectedWorkspace);
    },
    [client, loadWorkspaceDetails, workspaceList],
  );

  const handleOpenRepository = useCallback(async () => {
    if (!client || !window.cocode) {
      setRepositoryOpenState(
        errorApiState(new Error("Desktop repository picker is unavailable")),
      );
      return;
    }

    const selectedPath = await window.cocode.selectRepository();
    if (!selectedPath) {
      return;
    }

    setRepositoryOpenState(loadingApiState());
    const state = await loadApiResource(() =>
      client.openRepository(selectedPath),
    );
    setRepositoryOpenState(state);
    if (state.status !== "success") {
      return;
    }

    setAgentBootstrapState("loading");
    setActiveWorkspaceId(state.data.workspace.id);
    setActiveRepositoryId(state.data.repository.id);
    setRepositories(successApiState(state.data.repositories));
    setMainView("new-thread");
    await refreshNavigation(client, state.data.workspace.id);
    setAgentConfigs(loadingApiState());
    const catalogs =
      agentModelCatalogs.status === "success"
        ? agentModelCatalogs.data
        : undefined;
    const nextAgentConfigs = await loadAgentConfigs(client, {
      bootstrapBuiltIns: true,
      modelCatalogs: catalogs ?? [],
    });
    setAgentConfigs(nextAgentConfigs);
    setAgentBootstrapState("done");
  }, [agentModelCatalogs, client, refreshNavigation]);

  useEffect(() => {
    if (
      !client ||
      !activeRepository ||
      agentBootstrapState !== "idle" ||
      agentConfigs.status !== "success" ||
      agentConfigs.data.length > 0
    ) {
      return;
    }
    let canceled = false;
    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      setAgentBootstrapState("loading");
      setAgentConfigs(loadingApiState());
      const catalogs =
        agentModelCatalogs.status === "success"
          ? agentModelCatalogs.data
          : undefined;
      void loadAgentConfigs(client, {
        bootstrapBuiltIns: true,
        modelCatalogs: catalogs ?? [],
      }).then((state) => {
        if (canceled) {
          return;
        }
        setAgentConfigs(state);
        setAgentBootstrapState("done");
      });
    });
    return () => {
      canceled = true;
    };
  }, [
    activeRepository,
    agentBootstrapState,
    agentConfigs,
    agentModelCatalogs,
    client,
  ]);

  const handleSelectReviewSession = useCallback((session: ReviewSession) => {
    setCurrentReviewSession(session);
    setMainView("review");
  }, []);

  const handleDeleteReviewSession = useCallback(
    async (session: ReviewSession) => {
      if (!client) {
        toast.error("Could not delete thread", {
          description: "Backend client is unavailable.",
        });
        return;
      }
      const confirmed = window.confirm(
        `Delete "${session.title}" and its review data?`,
      );
      if (!confirmed) {
        return;
      }
      setDeletingReviewSessionId(session.id);
      const deleted = await loadApiResource(() =>
        client.deleteReviewSession(session.id),
      );
      setDeletingReviewSessionId("");
      if (deleted.status !== "success") {
        toast.error("Could not delete thread", {
          description:
            deleted.status === "error"
              ? deleted.error.message
              : "Delete did not complete.",
        });
        return;
      }
      setReviewSessions((current) =>
        current.status === "success"
          ? successApiState(
              current.data.filter((item) => item.id !== session.id),
            )
          : current,
      );
      if (currentReviewSession?.id === session.id) {
        setCurrentReviewSession(null);
        setMainView("new-thread");
      }
    },
    [client, currentReviewSession?.id],
  );

  const searchGroups = useMemo<SearchCommandGroup[]>(() => {
    const reviewCommands =
      sessionList.length > 0
        ? sessionList.slice(0, MAX_SEARCH_RESULTS).map((session) => ({
            title: session.title,
            description: `${session.status} • ${formatRelativeAge(session.updated_at)}`,
            icon: GitPullRequestIcon,
            onSelect: () => handleSelectReviewSession(session),
          }))
        : [
            {
              title: "Create new review thread",
              description:
                "Start from PR URL, local changes, or branch compare",
              shortcut: "N",
              icon: PlusIcon,
              onSelect: () => setMainView("new-thread"),
            },
          ];

    return [
      {
        heading: "Reviews",
        commands: reviewCommands,
      },
      {
        heading: "Projects",
        commands:
          workspaceList.length > 0
            ? workspaceList.slice(0, MAX_SEARCH_RESULTS).map((workspace) => ({
                title: workspace.name,
                description: workspace.root_path,
                icon: GitBranchIcon,
                onSelect: () => handleSelectWorkspace(workspace.id),
              }))
            : [
                {
                  title: "Open project",
                  description: "Select a git repository on this computer",
                  icon: FolderOpenIcon,
                  onSelect: handleOpenRepository,
                },
              ],
      },
      {
        heading: "Actions",
        commands: [
          {
            title: "Configure CLI agents",
            description: "Codex, Gemini, OpenCode, and custom CLIs",
            icon: TerminalIcon,
            onSelect: () => setMainView("agent-settings"),
          },
          {
            title: "Open app settings",
            description: "Credentials, presets, privacy, and logs",
            icon: SettingsIcon,
            onSelect: () => setMainView("agent-settings"),
          },
        ],
      },
    ];
  }, [
    handleOpenRepository,
    handleSelectReviewSession,
    handleSelectWorkspace,
    sessionList,
    workspaceList,
  ]);

  return (
    <>
      <AppShell
        sidebar={
          <Sidebar
            backendStatus={backendStatus}
            activeSessionId={displayedSession?.id}
            activeWorkspaceId={activeWorkspaceId}
            workspaces={workspaces}
            reviewSessions={reviewSessions}
            repositoryOpenState={repositoryOpenState}
            deletingReviewSessionId={deletingReviewSessionId}
            onOpenRepository={handleOpenRepository}
            onOpenSearch={() => setSearchOpen(true)}
            onOpenAgentSettings={() => setMainView("agent-settings")}
            onOpenNewThread={() => setMainView("new-thread")}
            onDeleteReviewSession={handleDeleteReviewSession}
            onSelectReviewSession={handleSelectReviewSession}
            onSelectWorkspace={handleSelectWorkspace}
          />
        }
        header={
          <TopNav
            activeRepository={activeRepository}
            activeSession={displayedSession}
            activeWorkspace={activeWorkspace}
          />
        }
        statusBanner={<AppConnectionNotice apiSession={apiSession} />}
      >
        {mainView === "new-thread" && (
          <NewThreadScreen
            activeRepository={activeRepository}
            activeWorkspace={activeWorkspace}
            agentConfigs={agentConfigs}
            agentModelCatalogs={agentModelCatalogs}
            client={client}
            onReviewStarted={(session) => {
              setCurrentReviewSession(session);
              setMainView("review");
              if (client) {
                void refreshNavigation(client, session.workspace_id);
              }
            }}
            onOpenRepository={handleOpenRepository}
          />
        )}
        {mainView === "review" && (
          <ReviewThread
            activeRepository={activeRepository}
            agentConfigs={agentConfigs}
            client={client}
            session={displayedSession}
          />
        )}
        {mainView === "agent-settings" && (
          <AgentSettingsScreen
            activeWorkspace={activeWorkspace}
            client={client}
            onBack={() => setMainView("new-thread")}
          />
        )}
      </AppShell>
      {searchOpen && (
        <SearchCommandDialog
          open={searchOpen}
          onOpenChange={setSearchOpen}
          groups={searchGroups}
        />
      )}
      <Toaster position="bottom-right" />
    </>
  );
}

function AppConnectionNotice({
  apiSession,
}: {
  apiSession: Loadable<ApiSessionResponse>;
}) {
  if (apiSession.status === "loading") {
    return (
      <div className="border-b px-4 py-2">
        <LoadingRows rows={1} />
      </div>
    );
  }
  if (apiSession.status === "error") {
    return (
      <div className="border-b p-3">
        <ErrorState
          title="Backend connection failed"
          description={apiSession.error.message}
        />
      </div>
    );
  }
  return null;
}

type AgentConfigFormState = {
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

type ReviewRuleDraftState = {
  scope: string;
  ruleType: string;
  content: string;
  enabled: boolean;
};

type SettingsCollisionPolicy = "skip" | "replace" | "rename" | "fail";

function AgentSettingsScreen({
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
              <section className="bg-surface-raised rounded-lg border">
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
                        "hover:bg-surface flex w-full items-start gap-3 rounded-md px-2 py-2 text-left text-sm",
                        formMode === "create" &&
                          form.sourcePresetId === preset.id &&
                          "bg-surface",
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

              <section className="bg-surface-raised rounded-lg border">
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
                        "hover:bg-surface flex w-full items-center gap-3 rounded-md px-2 py-2 text-left text-sm",
                        formMode === "edit" &&
                          form.id === config.id &&
                          "bg-surface",
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

            <section className="bg-surface-raised min-w-0 rounded-lg border">
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

          <section className="bg-surface-raised rounded-lg border">
            <button
              className="hover:bg-surface/70 flex w-full items-center justify-between gap-3 rounded-t-lg px-4 py-3 text-left"
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

function GitHubCredentialPanel({
  deleteState,
  displayName,
  saveState,
  status,
  token,
  onDelete,
  onDisplayNameChange,
  onSave,
  onTokenChange,
}: {
  deleteState: Loadable<DeleteGitHubCredentialResponse>;
  displayName: string;
  saveState: Loadable<GitHubCredentialStatusResponse>;
  status: Loadable<GitHubCredentialStatusResponse>;
  token: string;
  onDelete: () => void;
  onDisplayNameChange: (value: string) => void;
  onSave: () => void;
  onTokenChange: (value: string) => void;
}) {
  const credential =
    status.status === "success" && status.data.configured
      ? status.data.credential
      : undefined;
  const metadata = credential?.metadata ?? {};
  const login = typeof metadata.login === "string" ? metadata.login : "";
  const scopes = Array.isArray(metadata.scopes)
    ? metadata.scopes.filter(
        (scope): scope is string => typeof scope === "string",
      )
    : [];
  const validatedAt =
    typeof metadata.validated_at === "string" ? metadata.validated_at : "";
  const isSaving = saveState.status === "loading";
  const isDeleting = deleteState.status === "loading";

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium">
            <GitPullRequestIcon className="size-4" />
            GitHub credentials
          </div>
          <div className="text-muted-foreground mt-1 text-xs">
            Token value is encrypted by the desktop safe store; cocoded keeps
            only a credential reference.
          </div>
        </div>
        {status.status === "loading" ? (
          <Badge variant="outline">checking</Badge>
        ) : credential ? (
          <Badge variant="secondary">configured</Badge>
        ) : (
          <Badge variant="outline">missing</Badge>
        )}
      </div>

      <div className="grid gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_minmax(280px,0.7fr)]">
        <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)]">
          <label className="flex min-w-0 flex-col gap-2 text-sm font-medium">
            Display name
            <Input
              placeholder="GitHub token"
              value={displayName}
              onChange={(event) => onDisplayNameChange(event.target.value)}
            />
          </label>
          <label className="flex min-w-0 flex-col gap-2 text-sm font-medium">
            Token
            <Input
              placeholder="ghp_..."
              type="password"
              value={token}
              onChange={(event) => onTokenChange(event.target.value)}
            />
          </label>
          <div className="flex flex-wrap items-center gap-2 sm:col-span-2">
            <Button disabled={isSaving || !token.trim()} onClick={onSave}>
              <CheckIcon data-icon="inline-start" />
              {isSaving ? "Saving..." : "Save token"}
            </Button>
            <Button
              disabled={!credential || isDeleting}
              variant="outline"
              onClick={onDelete}
            >
              {isDeleting ? "Deleting..." : "Delete token"}
            </Button>
          </div>
          {saveState.status === "error" && (
            <ErrorState
              className="sm:col-span-2"
              title="Could not save GitHub token"
              description={saveState.error.message}
            />
          )}
          {deleteState.status === "error" && (
            <ErrorState
              className="sm:col-span-2"
              title="Could not delete GitHub token"
              description={deleteState.error.message}
            />
          )}
        </div>

        <div className="rounded-md border p-3">
          {status.status === "loading" && <LoadingRows rows={3} />}
          {status.status === "error" && (
            <ErrorState
              className="border-0 p-0"
              title="Credential status unavailable"
              description={status.error.message}
            />
          )}
          {status.status === "success" && !credential && (
            <EmptyState
              className="border-0 p-0"
              title="No GitHub token"
              description="GitHub PR ingestion will ask for a saved token before it calls the API."
              icon={GitPullRequestIcon}
            />
          )}
          {credential && (
            <div className="space-y-3 text-sm">
              <div>
                <div className="truncate font-medium">
                  {credential.display_name}
                </div>
                <div className="text-muted-foreground mt-1 truncate text-xs">
                  {login || credential.kind}
                  {validatedAt ? ` • ${formatRelativeAge(validatedAt)}` : ""}
                </div>
              </div>
              <div className="flex flex-wrap gap-1">
                {scopes.length > 0 ? (
                  scopes.slice(0, 6).map((scope) => (
                    <Badge key={scope} variant="outline">
                      {scope}
                    </Badge>
                  ))
                ) : (
                  <Badge variant="outline">no scopes reported</Badge>
                )}
              </div>
              <div className="text-muted-foreground truncate font-mono text-xs">
                {credential.storage_provider}
              </div>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

function ReviewRuleMemoryPanel({
  actionState,
  draft,
  rules,
  workspace,
  onCreate,
  onDelete,
  onDraftChange,
  onReload,
  onToggle,
}: {
  actionState: Loadable<ReviewRule | { deleted: boolean; id: string }>;
  draft: ReviewRuleDraftState;
  rules: Loadable<ReviewRuleListResponse>;
  workspace?: Workspace;
  onCreate: () => void;
  onDelete: (rule: ReviewRule) => void;
  onDraftChange: (draft: ReviewRuleDraftState) => void;
  onReload: () => void;
  onToggle: (rule: ReviewRule, enabled: boolean) => void;
}) {
  const items = rules.status === "success" ? rules.data.items : [];
  const isBusy = actionState.status === "loading";

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium">
            <BookOpenIcon className="size-4" />
            Review rule memory
          </div>
          <div className="text-muted-foreground mt-1 truncate text-xs">
            Dismissed findings can become local guidance for future review
            context.
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant={items.length > 0 ? "secondary" : "outline"}>
            {items.length} rules
          </Badge>
          <TooltipIconButton
            disabled={!workspace || rules.status === "loading"}
            label="Refresh review rules"
            size="icon-sm"
            variant="ghost"
            onClick={onReload}
          >
            <RefreshCwIcon />
          </TooltipIconButton>
        </div>
      </div>

      {!workspace ? (
        <div className="p-4">
          <EmptyState
            className="border-0 p-0"
            title="No project selected"
            description="Open a project before managing local review guidance."
            icon={BookOpenIcon}
          />
        </div>
      ) : (
        <div className="grid gap-4 p-4 lg:grid-cols-[minmax(280px,0.65fr)_minmax(0,1fr)]">
          <div className="flex min-w-0 flex-col gap-3 rounded-md border p-3">
            <div className="text-sm font-medium">Add rule</div>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="flex min-w-0 flex-col gap-2 text-sm font-medium">
                Scope
                <NativeSelect
                  value={draft.scope}
                  onChange={(event) =>
                    onDraftChange({ ...draft, scope: event.target.value })
                  }
                >
                  <NativeSelectOption value="workspace">
                    project
                  </NativeSelectOption>
                  <NativeSelectOption value="repository">
                    repository
                  </NativeSelectOption>
                  <NativeSelectOption value="path">path</NativeSelectOption>
                </NativeSelect>
              </label>
              <label className="flex min-w-0 flex-col gap-2 text-sm font-medium">
                Type
                <NativeSelect
                  value={draft.ruleType}
                  onChange={(event) =>
                    onDraftChange({ ...draft, ruleType: event.target.value })
                  }
                >
                  <NativeSelectOption value="dismissal">
                    dismissal
                  </NativeSelectOption>
                  <NativeSelectOption value="false_positive">
                    false_positive
                  </NativeSelectOption>
                  <NativeSelectOption value="review_guidance">
                    review_guidance
                  </NativeSelectOption>
                  <NativeSelectOption value="custom">custom</NativeSelectOption>
                </NativeSelect>
              </label>
            </div>
            <label className="flex min-w-0 flex-col gap-2 text-sm font-medium">
              Guidance
              <Textarea
                className="min-h-24 text-sm"
                maxLength={2000}
                placeholder="Do not report generated client snapshots unless runtime behavior changes."
                value={draft.content}
                onChange={(event) =>
                  onDraftChange({ ...draft, content: event.target.value })
                }
              />
            </label>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <label className="flex items-center gap-2 text-sm">
                <Switch
                  checked={draft.enabled}
                  size="sm"
                  onCheckedChange={(enabled) =>
                    onDraftChange({ ...draft, enabled })
                  }
                />
                Enabled
              </label>
              <Button
                disabled={isBusy || !draft.content.trim()}
                size="sm"
                onClick={onCreate}
              >
                <PlusIcon data-icon="inline-start" />
                Add rule
              </Button>
            </div>
            {actionState.status === "error" && (
              <ErrorState
                title="Rule update failed"
                description={actionState.error.message}
              />
            )}
          </div>

          <div className="min-w-0 rounded-md border">
            <div className="flex items-center justify-between gap-3 border-b px-3 py-2">
              <div className="min-w-0">
                <div className="truncate text-sm font-medium">
                  {workspace.name}
                </div>
                <div className="text-muted-foreground truncate text-xs">
                  Stored locally and injected only when prior decisions are
                  included.
                </div>
              </div>
            </div>
            <div className="flex max-h-80 flex-col gap-2 overflow-auto p-3">
              {rules.status === "loading" && <LoadingRows rows={4} />}
              {rules.status === "error" && (
                <ErrorState
                  className="border-0 p-0"
                  title="Rules unavailable"
                  description={rules.error.message}
                />
              )}
              {rules.status === "success" && items.length === 0 && (
                <EmptyState
                  className="border-0 p-0"
                  title="No remembered rules"
                  description="Dismissed findings can be saved as local guidance from the findings board."
                  icon={BookOpenIcon}
                />
              )}
              {items.slice(0, 100).map((rule) => (
                <div
                  key={rule.id}
                  className={cn(
                    "bg-background flex min-w-0 items-start gap-3 rounded-md border p-3",
                    !rule.enabled && "opacity-70",
                  )}
                >
                  <Switch
                    checked={rule.enabled}
                    className="mt-0.5"
                    disabled={isBusy}
                    size="sm"
                    onCheckedChange={(enabled) => onToggle(rule, enabled)}
                  />
                  <div className="min-w-0 flex-1">
                    <div className="mb-2 flex flex-wrap items-center gap-1">
                      <Badge variant="outline">
                        {formatReviewRuleScope(rule.scope)}
                      </Badge>
                      <Badge variant="secondary">{rule.rule_type}</Badge>
                      {!rule.enabled && <Badge variant="outline">off</Badge>}
                    </div>
                    <p className="line-clamp-3 text-sm">{rule.content}</p>
                    <div className="text-muted-foreground mt-2 truncate text-xs">
                      Updated {formatRelativeAge(rule.updated_at)}
                    </div>
                  </div>
                  <TooltipIconButton
                    disabled={isBusy}
                    label="Delete rule"
                    size="icon-sm"
                    variant="ghost"
                    onClick={() => onDelete(rule)}
                  >
                    <Trash2Icon />
                  </TooltipIconButton>
                </div>
              ))}
              {items.length > 100 && (
                <div className="text-muted-foreground px-1 text-xs">
                  Showing 100 of {items.length} rules.
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </section>
  );
}

function SettingsPortabilityPanel({
  collisionPolicy,
  exportText,
  importText,
  state,
  workspace,
  onCollisionPolicyChange,
  onExport,
  onImport,
  onImportTextChange,
}: {
  collisionPolicy: SettingsCollisionPolicy;
  exportText: string;
  importText: string;
  state: Loadable<SettingsExportPayload | SettingsImportResponse>;
  workspace?: Workspace;
  onCollisionPolicyChange: (policy: SettingsCollisionPolicy) => void;
  onExport: () => void;
  onImport: () => void;
  onImportTextChange: (value: string) => void;
}) {
  const isBusy = state.status === "loading";
  const importResult =
    state.status === "success" && isSettingsImportResponse(state.data)
      ? state.data
      : undefined;

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium">
            <CopyIcon className="size-4" />
            Settings portability
          </div>
          <div className="text-muted-foreground mt-1 truncate text-xs">
            Portable JSON excludes secrets, local paths, review artifacts, and
            credential refs.
          </div>
        </div>
        <Badge variant={workspace ? "secondary" : "outline"}>
          {workspace?.name ?? "no project"}
        </Badge>
      </div>

      <div className="grid gap-4 p-4 lg:grid-cols-2">
        <div className="flex min-w-0 flex-col gap-3">
          <div className="flex items-center justify-between gap-3">
            <div className="text-sm font-medium">Export</div>
            <Button
              disabled={!workspace || isBusy}
              size="sm"
              onClick={onExport}
            >
              <ArrowUpIcon data-icon="inline-start" />
              Export JSON
            </Button>
          </div>
          <Textarea
            aria-label="Settings export JSON"
            className="min-h-56 font-mono text-xs"
            readOnly
            placeholder="Exported settings JSON appears here."
            value={exportText}
          />
        </div>

        <div className="flex min-w-0 flex-col gap-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="text-sm font-medium">Import</div>
            <div className="flex items-center gap-2">
              <NativeSelect
                className="w-28"
                value={collisionPolicy}
                onChange={(event) =>
                  onCollisionPolicyChange(
                    event.target.value as SettingsCollisionPolicy,
                  )
                }
              >
                <NativeSelectOption value="skip">skip</NativeSelectOption>
                <NativeSelectOption value="replace">replace</NativeSelectOption>
                <NativeSelectOption value="rename">rename</NativeSelectOption>
                <NativeSelectOption value="fail">fail</NativeSelectOption>
              </NativeSelect>
              <Button
                disabled={!workspace || isBusy || !importText.trim()}
                size="sm"
                variant="outline"
                onClick={onImport}
              >
                <ArrowDownIcon data-icon="inline-start" />
                Import
              </Button>
            </div>
          </div>
          <Textarea
            aria-label="Settings import JSON"
            className="min-h-56 font-mono text-xs"
            placeholder="Paste a cocode.settings_export.v1 JSON payload."
            value={importText}
            onChange={(event) => onImportTextChange(event.target.value)}
          />
        </div>
      </div>

      {(state.status === "error" || importResult) && (
        <div className="border-t px-4 py-3">
          {state.status === "error" && (
            <ErrorState
              title="Settings portability failed"
              description={state.error.message}
            />
          )}
          {importResult && (
            <div className="grid gap-2 sm:grid-cols-3">
              <SettingsImportReportChip
                label="Project"
                report={importResult.workspace_settings}
              />
              <SettingsImportReportChip
                label="Agents"
                report={importResult.agent_configs}
              />
              <SettingsImportReportChip
                label="Rules"
                report={importResult.review_rules}
              />
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function SettingsImportReportChip({
  label,
  report,
}: {
  label: string;
  report: SettingsImportResponse["agent_configs"];
}) {
  return (
    <div className="bg-background rounded-md border px-3 py-2 text-sm">
      <div className="font-medium">{label}</div>
      <div className="text-muted-foreground mt-1 text-xs">
        {report.created} created, {report.updated} updated, {report.skipped}{" "}
        skipped
        {report.redacted ? `, ${report.redacted} redacted` : ""}
      </div>
    </div>
  );
}

function isSettingsImportResponse(
  value: SettingsExportPayload | SettingsImportResponse,
): value is SettingsImportResponse {
  return "imported_at" in value;
}

function AgentSettingSwitch({
  checked,
  label,
  onCheckedChange,
}: {
  checked: boolean;
  label: string;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <label className="bg-background flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm">
      <span className="truncate">{label}</span>
      <Switch checked={checked} size="sm" onCheckedChange={onCheckedChange} />
    </label>
  );
}

function HealthSummary({ health }: { health: AgentConfigHealth }) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <Badge
          variant={
            health.status === "unavailable" ? "destructive" : "secondary"
          }
        >
          {health.status}
        </Badge>
        {health.message && (
          <span className="text-muted-foreground min-w-0 truncate text-sm">
            {health.message}
          </span>
        )}
      </div>
      {formatHealthMetadata(health.metadata).length > 0 && (
        <div className="grid grid-cols-2 gap-2">
          {formatHealthMetadata(health.metadata).map(([key, value]) => (
            <div
              key={key}
              className="bg-background rounded-md border px-2 py-1"
            >
              <div className="text-muted-foreground text-xs">{key}</div>
              <div className="truncate font-mono text-xs">{value}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function defaultReviewRuleDraft(): ReviewRuleDraftState {
  return {
    scope: "workspace",
    ruleType: "dismissal",
    content: "",
    enabled: true,
  };
}

function formatReviewRuleScope(scope: string) {
  return scope === "workspace" ? "project" : scope;
}

function upsertReviewRuleState(
  current: Loadable<ReviewRuleListResponse>,
  rule: ReviewRule,
): Loadable<ReviewRuleListResponse> {
  const items = current.status === "success" ? current.data.items : [];
  const exists = items.some((item) => item.id === rule.id);
  const next = exists
    ? items.map((item) => (item.id === rule.id ? rule : item))
    : [rule, ...items];
  return successApiState({ items: next });
}

function removeReviewRuleState(
  current: Loadable<ReviewRuleListResponse>,
  id: string,
): Loadable<ReviewRuleListResponse> {
  if (current.status !== "success") {
    return successApiState({ items: [] });
  }
  return successApiState({
    items: current.data.items.filter((item) => item.id !== id),
  });
}

function defaultAgentConfigForm(): AgentConfigFormState {
  return {
    name: "Custom CLI",
    role: "custom_reviewer",
    adapterKind: "cli_non_interactive",
    command: "",
    argsText: "",
    cwdMode: "repo_root",
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

function formFromAgentPreset(preset: AgentPreset): AgentConfigFormState {
  return formFromAgentLike({
    ...preset,
    name: preset.id === "custom-cli" ? "Custom CLI" : preset.name,
    enabled: preset.enabled && preset.id !== "custom-cli",
    sourcePresetId: preset.id,
  });
}

function agentConfigInputFromPreset(
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

function formFromAgentConfig(config: AgentConfig): AgentConfigFormState {
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
    cwdMode: source.cwd_mode || "repo_root",
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

function agentConfigBodyFromForm(
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
    cwd_mode: form.cwdMode.trim() || "repo_root",
    env_allowlist: parseInlineList(form.envAllowlistText),
    output_mode: form.outputMode,
    model_label: form.modelLabel.trim(),
    reasoning_label: form.reasoningLabel.trim(),
    capabilities: form.capabilities,
    settings,
    enabled: form.enabled,
  };
}

function supportedOutputModes(
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

function formatHealthMetadata(metadata: Record<string, unknown>) {
  return ["version", "resolved_path", "path", "error"]
    .map((key) => [key, metadata[key]] as const)
    .filter(([, value]) => typeof value === "string" && value.trim())
    .map(([key, value]) => [key, value as string] as const);
}

function CocodeMark() {
  return (
    <svg
      aria-hidden="true"
      className="size-9 shrink-0"
      viewBox="0 0 256 256"
      fill="none"
    >
      <rect width="256" height="256" rx="64" fill="#111214" />
      <path
        d="M83 94.5L117.5 60H177V94H132L101 125L132 156H177V190H117.5L83 155.5C66.2 138.7 66.2 111.3 83 94.5Z"
        fill="#FBFBFA"
      />
      <path d="M149 122H194V154H149V122Z" fill="#FBFBFA" />
    </svg>
  );
}

function Sidebar({
  activeSessionId,
  activeWorkspaceId,
  backendStatus,
  deletingReviewSessionId,
  repositoryOpenState,
  reviewSessions,
  workspaces,
  onDeleteReviewSession,
  onOpenAgentSettings,
  onOpenNewThread,
  onOpenRepository,
  onOpenSearch,
  onSelectReviewSession,
  onSelectWorkspace,
}: {
  activeSessionId?: string;
  activeWorkspaceId: string;
  backendStatus: string;
  deletingReviewSessionId: string;
  repositoryOpenState: Loadable<OpenRepositoryResponse>;
  reviewSessions: Loadable<ReviewSession[]>;
  workspaces: Loadable<Workspace[]>;
  onDeleteReviewSession: (session: ReviewSession) => void;
  onOpenAgentSettings: () => void;
  onOpenNewThread: () => void;
  onOpenRepository: () => void;
  onOpenSearch: () => void;
  onSelectReviewSession: (session: ReviewSession) => void;
  onSelectWorkspace: (workspaceId: string) => void;
}) {
  const [threadContextMenu, setThreadContextMenu] = useState<{
    session: ReviewSession;
    x: number;
    y: number;
  } | null>(null);
  const workspaceList = workspaces.status === "success" ? workspaces.data : [];
  const sessionList =
    reviewSessions.status === "success"
      ? reviewSessions.data.slice(0, MAX_SIDEBAR_SESSIONS)
      : [];
  const activeWorkspaceHasThreads =
    reviewSessions.status === "success" && sessionList.length > 0;

  useEffect(() => {
    if (!threadContextMenu) {
      return;
    }
    function closeMenu() {
      setThreadContextMenu(null);
    }
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        closeMenu();
      }
    }
    window.addEventListener("pointerdown", closeMenu);
    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("pointerdown", closeMenu);
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [threadContextMenu]);

  return (
    <>
      <div className="app-drag flex items-center gap-3 px-5 pt-12 pb-3">
        <CocodeMark />
        <div className="min-w-0">
          <p className="truncate text-[1.18rem] leading-6 font-semibold tracking-[-0.02em]">
            cocode
          </p>
        </div>
      </div>

      <nav className="app-no-drag flex flex-col gap-1 px-3">
        <SidebarNavButton
          icon={PlusIcon}
          label="New thread"
          onClick={onOpenNewThread}
        />
        <SidebarNavButton
          icon={SearchIcon}
          label="Search"
          meta={<span className="font-mono text-[0.68rem]">⌘K</span>}
          onClick={onOpenSearch}
        />
        <SidebarNavButton
          disabled={repositoryOpenState.status === "loading"}
          icon={FolderOpenIcon}
          label={
            repositoryOpenState.status === "loading"
              ? "Opening project"
              : "Open project"
          }
          onClick={onOpenRepository}
        />
      </nav>

      <SidebarSection title="Projects">
        {workspaces.status === "loading" && (
          <div className="text-sidebar-muted px-2 py-1 text-xs">
            Loading projects...
          </div>
        )}
        {workspaces.status === "error" && (
          <div className="text-destructive px-2 py-1 text-xs">
            {workspaces.error.message}
          </div>
        )}
        {workspaces.status === "success" && workspaceList.length === 0 && (
          <div className="text-sidebar-muted px-2 py-1 text-xs">
            No projects yet
          </div>
        )}
        {workspaceList.map((workspace) => (
          <div key={workspace.id} className="min-w-0">
            <SidebarNavButton
              active={workspace.id === activeWorkspaceId}
              icon={FolderOpenIcon}
              label={workspace.name}
              meta={
                workspace.id === activeWorkspaceId ? (
                  <ChevronDownIcon className="size-3.5" />
                ) : undefined
              }
              onClick={() => onSelectWorkspace(workspace.id)}
            />
            {workspace.id === activeWorkspaceId && (
              <div className="border-border/70 mt-1 mb-2 flex flex-col gap-1 border-l pl-3">
                {reviewSessions.status === "loading" && (
                  <div className="text-sidebar-muted px-2 py-1 text-xs">
                    Loading threads...
                  </div>
                )}
                {reviewSessions.status === "error" && (
                  <div className="text-destructive px-2 py-1 text-xs">
                    {reviewSessions.error.message}
                  </div>
                )}
                {reviewSessions.status === "success" &&
                  !activeWorkspaceHasThreads && (
                    <SidebarNavButton
                      active={!activeSessionId}
                      className="h-8 text-[0.78rem]"
                      icon={FileTextIcon}
                      label="Set up review"
                      meta="Draft"
                      onClick={onOpenNewThread}
                    />
                  )}
                {sessionList.map((session) => (
                  <SidebarNavButton
                    key={session.id}
                    active={session.id === activeSessionId}
                    className="h-auto min-h-9 items-start py-1.5 text-[0.78rem]"
                    disabled={deletingReviewSessionId === session.id}
                    icon={FileTextIcon}
                    label={session.title}
                    meta={
                      deletingReviewSessionId === session.id
                        ? "Deleting"
                        : formatRelativeAge(session.updated_at)
                    }
                    onClick={() => onSelectReviewSession(session)}
                    onContextMenu={(event) => {
                      event.preventDefault();
                      setThreadContextMenu({
                        session,
                        x: event.clientX,
                        y: event.clientY,
                      });
                    }}
                  />
                ))}
              </div>
            )}
          </div>
        ))}
      </SidebarSection>

      {repositoryOpenState.status === "error" && (
        <div className="text-destructive px-4 pt-3 text-xs">
          {repositoryOpenState.error.message}
        </div>
      )}

      <div className="mt-auto flex flex-col gap-1 p-3">
        <Separator className="-mx-3 mb-2 opacity-70" />
        <SidebarNavButton
          icon={SettingsIcon}
          label="Settings"
          onClick={onOpenAgentSettings}
        />
        <div className="mt-3 flex items-center gap-3 rounded-lg px-2 py-2">
          <div className="bg-surface-raised flex size-9 items-center justify-center rounded-full border text-sm font-semibold">
            A
          </div>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">Ana Lee</div>
            <div className="text-sidebar-muted truncate text-xs">ana@local</div>
          </div>
          <ChevronDownIcon className="text-sidebar-muted size-4" />
        </div>
        <div className="text-sidebar-muted flex items-center justify-between px-2 pb-2 text-xs">
          <span className="inline-flex items-center gap-1.5">
            <span
              className={cn(
                "size-1.5 rounded-full",
                backendStatus === "ready" ? "bg-success" : "bg-warning",
              )}
            />
            Backend {backendStatus}
          </span>
        </div>
      </div>
      {threadContextMenu && (
        <div
          className="app-no-drag bg-popover text-popover-foreground ring-foreground/10 fixed z-50 min-w-40 rounded-lg p-1 text-sm shadow-lg ring-1"
          style={{
            left: Math.min(threadContextMenu.x, window.innerWidth - 176),
            top: Math.min(threadContextMenu.y, window.innerHeight - 52),
          }}
          onPointerDown={(event) => event.stopPropagation()}
        >
          <button
            className="text-destructive hover:bg-destructive/10 flex h-8 w-full cursor-pointer items-center gap-2 rounded-md px-2 text-left"
            type="button"
            onClick={() => {
              const { session } = threadContextMenu;
              setThreadContextMenu(null);
              onDeleteReviewSession(session);
            }}
          >
            <Trash2Icon className="size-3.5" />
            Delete thread
          </button>
        </div>
      )}
    </>
  );
}

function TopNav({
  activeRepository,
  activeSession,
  activeWorkspace,
}: {
  activeRepository?: Repository;
  activeSession?: ReviewSession;
  activeWorkspace?: Workspace;
}) {
  const repositoryLabel = activeRepository?.owner
    ? `${activeRepository.owner}/${activeRepository.name}`
    : (activeRepository?.name ??
      activeWorkspace?.name ??
      "No project selected");
  const branchLabel =
    activeSession?.status === "running"
      ? "review/running"
      : (activeRepository?.default_branch ?? "main");
  const titleLabel = activeSession?.title ?? repositoryLabel;
  const subtitleLabel = activeSession ? repositoryLabel : "Set up review";

  return (
    <div className="app-drag bg-surface-raised/96 flex h-[52px] shrink-0 items-center justify-between gap-4 border-b px-5 backdrop-blur">
      <div className="flex min-w-0 items-center gap-3">
        <div className="min-w-0">
          <div className="truncate text-[0.92rem] font-semibold">
            {titleLabel}
          </div>
          <div className="text-muted-foreground mt-0.5 flex min-w-0 items-center gap-1.5 text-xs">
            <Code2Icon className="size-3.5 shrink-0" />
            <span className="truncate">{subtitleLabel}</span>
          </div>
        </div>
      </div>

      <div className="app-no-drag text-muted-foreground flex h-7 max-w-[220px] min-w-0 items-center gap-1.5 rounded-lg border bg-white px-2.5 text-xs">
        <GitBranchIcon className="size-[13px] shrink-0" />
        <span className="truncate">{branchLabel}</span>
      </div>
    </div>
  );
}

function ReviewThread({
  activeRepository,
  agentConfigs,
  client,
  session,
}: {
  activeRepository?: Repository;
  agentConfigs: Loadable<AgentConfig[]>;
  client: ApiClient | null;
  session?: ReviewSession;
}) {
  const [activeTab, setActiveTab] = useState("chat");
  const [evidenceMapFinding, setEvidenceMapFinding] = useState<Finding | null>(
    null,
  );
  const [followUpFinding, setFollowUpFinding] = useState<Finding | null>(null);
  const [detailFinding, setDetailFinding] = useState<Finding | null>(null);
  const live = useReviewSessionLiveData(client, session);

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (!canceled) {
        setEvidenceMapFinding(null);
        setFollowUpFinding(null);
        setDetailFinding(null);
      }
    });
    return () => {
      canceled = true;
    };
  }, [session?.id]);

  const openEvidenceMap = useCallback((finding: Finding) => {
    setEvidenceMapFinding(finding);
    setActiveTab("evidence-map");
  }, []);

  const openFollowUp = useCallback((finding: Finding) => {
    setFollowUpFinding(finding);
    setActiveTab("follow-up");
  }, []);

  return (
    <section className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
      <div className="min-h-0 flex-1 overflow-hidden px-6 py-5">
        <div className="mx-auto flex h-full min-h-0 w-full max-w-[1500px] flex-col gap-5">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <h1 className="truncate text-xl font-semibold">
                  {session?.title ?? "Review thread"}
                </h1>
                {live.refreshState.status === "refreshing" && (
                  <Badge variant="outline">Refreshing</Badge>
                )}
                {live.refreshState.status === "error" && (
                  <Badge variant="outline">Refresh issue</Badge>
                )}
              </div>
              <p className="text-muted-foreground mt-1 text-sm">
                {session
                  ? `${session.review_depth} review • ${session.status}`
                  : "Create or select a review session to stream live progress."}
              </p>
              {live.refreshState.status === "error" && (
                <p className="text-destructive mt-1 max-w-2xl truncate text-xs">
                  {live.refreshState.message}
                </p>
              )}
            </div>
            {session && (
              <ReviewControlButtons
                client={client}
                onSessionUpdated={live.setSession}
                session={live.session ?? session}
              />
            )}
          </div>

          <Tabs
            value={activeTab}
            onValueChange={setActiveTab}
            className="min-h-0 flex-1"
          >
            <TabsList
              variant="line"
              className="border-border h-9 w-full justify-start gap-8 border-b p-0"
            >
              <TabsTrigger
                value="chat"
                className="h-9 flex-none rounded-none border-0 px-0 text-[13px]"
              >
                Chat
              </TabsTrigger>
              <TabsTrigger
                value="findings"
                className="h-9 flex-none rounded-none border-0 px-0 text-[13px]"
              >
                Findings
              </TabsTrigger>
              <TabsTrigger
                value="publish"
                className="h-9 flex-none rounded-none border-0 px-0 text-[13px]"
              >
                Publish
              </TabsTrigger>
              <TabsTrigger value="details" className="hidden">
                Details
              </TabsTrigger>
              <TabsTrigger value="finding-detail" className="hidden">
                Finding detail
              </TabsTrigger>
              <TabsTrigger value="evidence-map" className="hidden">
                Evidence map
              </TabsTrigger>
              <TabsTrigger value="follow-up" className="hidden">
                Follow-up
              </TabsTrigger>
            </TabsList>

            <TabsContent
              value="chat"
              className={cn(REVIEW_THREAD_TAB_CLASS, "overflow-hidden")}
            >
              {session ? (
                <CentralizedChatScreen
                  agentConfigs={agentConfigs}
                  client={client}
                  events={live.events}
                  findings={live.findings}
                  onOpenFindings={() => setActiveTab("findings")}
                  session={live.session ?? session}
                  summary={live.summary}
                />
              ) : (
                <>
                  <div className="bg-surface self-end rounded-full px-4 py-2 text-sm">
                    Review this PR for auth, billing, and data integrity.
                  </div>
                  <div className="flex items-start gap-3">
                    <div className="bg-primary text-primary-foreground mt-1 flex size-7 items-center justify-center rounded-md">
                      <BotIcon />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="mb-2 flex items-center gap-2 text-sm">
                        <span className="font-medium">cocode</span>
                        <Badge variant="secondary">4 agents</Badge>
                        <span className="text-muted-foreground">
                          Phase 1 of 3
                        </span>
                      </div>
                      <p className="text-sm leading-6">
                        I found a likely authorization bypass in the billing
                        route group. Codex, Gemini, OpenCode, and Local Verifier
                        agree on the affected line range and there is supporting
                        evidence from route setup, middleware, and tests.
                      </p>
                    </div>
                  </div>
                  <ChangedFilesPanel />
                  <FindingsPanel />
                </>
              )}
            </TabsContent>

            <TabsContent value="details" className={REVIEW_THREAD_TAB_CLASS}>
              <ReviewDetailsScreen
                agentConfigs={agentConfigs}
                client={client}
                events={live.events}
                session={live.session ?? session}
                streamState={live.streamState}
              />
            </TabsContent>

            <TabsContent value="findings" className={REVIEW_THREAD_TAB_CLASS}>
              <ReviewFindingsBoard
                client={client}
                findings={live.findings}
                onOpenDetail={(finding) => {
                  setDetailFinding(finding);
                  setActiveTab("finding-detail");
                }}
                onOpenEvidenceMap={openEvidenceMap}
                onOpenFollowUp={openFollowUp}
                session={live.session ?? session}
              />
            </TabsContent>

            <TabsContent
              value="finding-detail"
              className={REVIEW_THREAD_TAB_CLASS}
            >
              {detailFinding ? (
                <FindingDetailScreen
                  agentConfigs={agentConfigs}
                  client={client}
                  events={live.events}
                  finding={detailFinding}
                  onBack={() => setActiveTab("findings")}
                  onOpenEvidenceMap={openEvidenceMap}
                />
              ) : (
                <EmptyState
                  title="Select a finding first"
                  description="Open full detail from a selected finding to inspect code, evidence, and discussion."
                  icon={FileSearchIcon}
                />
              )}
            </TabsContent>

            <TabsContent
              value="evidence-map"
              className={REVIEW_THREAD_TAB_CLASS}
            >
              {evidenceMapFinding ? (
                <EvidenceMapScreen
                  activeRepository={activeRepository}
                  agentConfigs={agentConfigs}
                  client={client}
                  events={live.events}
                  finding={evidenceMapFinding}
                  onBack={() => setActiveTab("findings")}
                />
              ) : (
                <EmptyState
                  title="Select a finding first"
                  description="Open Evidence Map from a selected finding to inspect graph context."
                  icon={MapIcon}
                />
              )}
            </TabsContent>

            <TabsContent value="follow-up" className={REVIEW_THREAD_TAB_CLASS}>
              {followUpFinding ? (
                <FindingFollowUpScreen
                  agentConfigs={agentConfigs}
                  client={client}
                  events={live.events}
                  finding={followUpFinding}
                  onBack={() => setActiveTab("findings")}
                />
              ) : (
                <EmptyState
                  title="Select a finding first"
                  description="Open Follow-up from a selected finding to ask scoped questions."
                  icon={MessageSquareIcon}
                />
              )}
            </TabsContent>

            <TabsContent value="publish" className={REVIEW_THREAD_TAB_CLASS}>
              <PublishReviewScreen
                client={client}
                session={live.session ?? session}
              />
            </TabsContent>
          </Tabs>
        </div>
      </div>
    </section>
  );
}

function ReviewDetailsScreen({
  agentConfigs,
  client,
  events,
  session,
  streamState,
}: {
  agentConfigs: Loadable<AgentConfig[]>;
  client: ApiClient | null;
  events: ReviewEvent[];
  session?: ReviewSession;
  streamState: Loadable<true>;
}) {
  if (!session) {
    return (
      <EmptyState
        title="No review selected"
        description="Select a review thread to inspect context safety and audit activity."
        icon={FileSearchIcon}
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(340px,0.72fr)]">
        <ReviewContextSafetyPanel
          agentConfigs={agentConfigs}
          client={client}
          session={session}
        />
        <ReviewAuditLogPanel client={client} session={session} />
      </div>
      <ReviewEventTimeline events={events} streamState={streamState} />
    </div>
  );
}

function ReviewContextSafetyPanel({
  agentConfigs,
  client,
  session,
}: {
  agentConfigs: Loadable<AgentConfig[]>;
  client: ApiClient | null;
  session: ReviewSession;
}) {
  const reviewAgents = useMemo(() => {
    if (agentConfigs.status !== "success") {
      return [];
    }
    const sessionAgentIds = new Set(
      session.agents.map((agent) => agent.agent_config_id),
    );
    return agentConfigs.data.filter((agent) => sessionAgentIds.has(agent.id));
  }, [agentConfigs, session.agents]);
  const [selectedAgentId, setSelectedAgentId] = useState("");
  const [preview, setPreview] =
    useState<Loadable<ContextBundlePreview>>(idleApiState());

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      setPreview(idleApiState());
      setSelectedAgentId((current) => {
        if (reviewAgents.some((agent) => agent.id === current)) {
          return current;
        }
        return reviewAgents[0]?.id ?? "";
      });
    });
    return () => {
      canceled = true;
    };
  }, [reviewAgents, session.id]);

  async function runPreview() {
    if (!client || !selectedAgentId) {
      setPreview(
        errorApiState(
          new Error("Choose a configured review agent before previewing."),
        ),
      );
      return;
    }
    setPreview(loadingApiState());
    const state = await loadApiResource(() =>
      client.previewReviewContext(session.id, {
        agent_config_id: selectedAgentId,
        persist: false,
        context_policy: session.context_policy as ReviewContextPolicy,
      }),
    );
    setPreview(state);
  }

  const selectedAgent = reviewAgents.find(
    (agent) => agent.id === selectedAgentId,
  );

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium">
            <ShieldCheckIcon className="size-4" />
            Context safety
          </div>
          <div className="text-muted-foreground mt-1 text-xs">
            Redaction, recipient visibility, and local-only enforcement.
          </div>
        </div>
        <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
          {agentConfigs.status === "success" && reviewAgents.length > 0 && (
            <NativeSelect
              aria-label="Context preview agent"
              className="w-48"
              size="sm"
              value={selectedAgentId}
              onChange={(event) => {
                setSelectedAgentId(event.target.value);
                setPreview(idleApiState());
              }}
            >
              {reviewAgents.map((agent) => (
                <NativeSelectOption key={agent.id} value={agent.id}>
                  {agent.name}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          )}
          <Button
            disabled={
              !client ||
              !selectedAgentId ||
              preview.status === "loading" ||
              agentConfigs.status === "loading"
            }
            size="sm"
            onClick={() => void runPreview()}
          >
            <ShieldCheckIcon data-icon="inline-start" />
            Preview
          </Button>
        </div>
      </div>

      {agentConfigs.status === "loading" && (
        <LoadingRows rows={3} className="p-4" />
      )}
      {agentConfigs.status === "error" && (
        <ErrorState
          className="m-3"
          title="Agents unavailable"
          description={agentConfigs.error.message}
        />
      )}
      {agentConfigs.status === "success" && reviewAgents.length === 0 && (
        <EmptyState
          className="border-0 p-6"
          title="No review agents"
          description="This session does not have configured agent rows to preview."
          icon={TerminalIcon}
        />
      )}
      {agentConfigs.status === "success" && reviewAgents.length > 0 && (
        <ContextSafetyReport
          agent={selectedAgent}
          preview={preview}
          session={session}
        />
      )}
    </section>
  );
}

function ContextSafetyReport({
  agent,
  preview,
  session,
}: {
  agent?: AgentConfig;
  preview: Loadable<ContextBundlePreview>;
  session: ReviewSession;
}) {
  if (preview.status === "idle") {
    return (
      <div className="p-4">
        <div className="grid gap-3 sm:grid-cols-3">
          <MetricTile
            label="Redaction"
            value={
              session.context_policy.redact_secrets === false ? "off" : "on"
            }
          />
          <MetricTile
            label="Recipient"
            value={agent ? agentProvider(agent) : "agent"}
          />
          <MetricTile
            label="Egress"
            value={agent ? agentEgress(agent) : "unknown"}
          />
        </div>
      </div>
    );
  }

  if (preview.status === "loading") {
    return <LoadingRows rows={4} className="p-4" />;
  }

  if (preview.status === "error") {
    return (
      <ErrorState
        className="m-3"
        title="Context preview failed"
        description={preview.error.message}
      />
    );
  }

  const redaction = preview.data.redaction_report;
  const visibility = preview.data.visibility_report;
  const omitted = visibility.omitted ?? [];
  const redactedItems = redaction.items.slice(0, 6);

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="grid gap-3 sm:grid-cols-4">
        <MetricTile
          label="Redactions"
          value={String(redaction.redaction_count)}
        />
        <MetricTile label="Sent" value={String(visibility.sent_item_count)} />
        <MetricTile
          label="Tokens"
          value={String(preview.data.bundle.token_estimate)}
        />
        <MetricTile label="Omitted" value={String(omitted.length)} />
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Badge variant="outline">
          {visibility.recipient.provider ??
            agentProvider(agent ?? { capabilities: {} })}
        </Badge>
        <Badge
          variant={
            (visibility.recipient.egress ??
              agentEgress(agent ?? { capabilities: {} })) === "local"
              ? "outline"
              : "secondary"
          }
        >
          {visibility.recipient.egress ??
            agentEgress(agent ?? { capabilities: {} })}
        </Badge>
        {visibility.local_only_enforced && (
          <Badge variant="secondary">local-only enforced</Badge>
        )}
        {preview.data.redaction_report_artifact_id && (
          <Badge variant="outline">
            report {preview.data.redaction_report_artifact_id}
          </Badge>
        )}
      </div>

      <div className="grid gap-3 lg:grid-cols-2">
        <div className="rounded-md border">
          <div className="border-b px-3 py-2 text-xs font-medium">
            Redaction report
          </div>
          {redactedItems.length === 0 ? (
            <EmptyState
              className="border-0 p-4"
              title="No redactions"
              description="The current preview did not match configured secret detectors."
              icon={ShieldCheckIcon}
            />
          ) : (
            <div className="divide-y">
              {redactedItems.map((item) => (
                <RedactionReportRow key={item.item_id} item={item} />
              ))}
            </div>
          )}
        </div>

        <div className="rounded-md border">
          <div className="border-b px-3 py-2 text-xs font-medium">
            Visibility report
          </div>
          <div className="space-y-3 p-3">
            <div className="grid gap-2 text-xs sm:grid-cols-2">
              {Object.entries(visibility.sent_item_by_kind ?? {}).map(
                ([kind, count]) => (
                  <div
                    key={kind}
                    className="bg-surface flex items-center justify-between gap-2 rounded-md px-2 py-1.5"
                  >
                    <span className="truncate">{formatContextKind(kind)}</span>
                    <span className="text-muted-foreground">{count}</span>
                  </div>
                ),
              )}
            </div>
            {visibility.local_only_paths &&
              visibility.local_only_paths.length > 0 && (
                <div className="space-y-1">
                  <div className="text-muted-foreground text-xs">
                    Local-only paths
                  </div>
                  {visibility.local_only_paths.slice(0, 5).map((path) => (
                    <div
                      key={path}
                      className="bg-surface truncate rounded-md px-2 py-1.5 font-mono text-xs"
                    >
                      {path}
                    </div>
                  ))}
                </div>
              )}
            {omitted.length > 0 && (
              <div className="space-y-1">
                <div className="text-muted-foreground text-xs">Omitted</div>
                {omitted.slice(0, 6).map((item, index) => (
                  <div
                    key={`${item.item_id ?? item.path ?? "omitted"}-${index}`}
                    className="bg-surface rounded-md px-2 py-1.5 text-xs"
                  >
                    <div className="truncate font-mono">
                      {item.path ?? item.item_id ?? item.kind ?? "context item"}
                    </div>
                    <div className="text-muted-foreground mt-1 truncate">
                      {item.reason}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function MetricTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-surface rounded-md border px-3 py-2">
      <div className="text-muted-foreground text-xs">{label}</div>
      <div className="mt-1 truncate text-sm font-medium">{value}</div>
    </div>
  );
}

function RedactionReportRow({ item }: { item: RedactionReportItem }) {
  return (
    <div className="px-3 py-2 text-xs">
      <div className="flex min-w-0 items-center justify-between gap-2">
        <span className="truncate font-medium">
          {item.path ?? item.title ?? item.item_id}
        </span>
        <Badge variant="outline">{item.redaction_count}</Badge>
      </div>
      <div className="text-muted-foreground mt-1 truncate">
        {formatContextKind(item.kind)} • {formatDetectorSummary(item.detectors)}
      </div>
    </div>
  );
}

function ReviewAuditLogPanel({
  client,
  session,
}: {
  client: ApiClient | null;
  session: ReviewSession;
}) {
  const [auditLog, setAuditLog] =
    useState<Loadable<ReviewAuditLogResponse>>(idleApiState());

  useEffect(() => {
    if (!client) {
      let canceled = false;
      queueMicrotask(() => {
        if (!canceled) {
          setAuditLog(idleApiState());
        }
      });
      return () => {
        canceled = true;
      };
    }
    const api = client;
    let canceled = false;

    async function load(initialLoad = false) {
      if (initialLoad) {
        setAuditLog((current) =>
          current.status === "success" ? current : loadingApiState(),
        );
      }
      const next = await loadApiResource(() =>
        api.getReviewAuditLog(session.id),
      );
      if (canceled) {
        return;
      }
      setAuditLog((current) => preserveSuccessfulLoadable(current, next));
    }

    queueMicrotask(() => {
      if (!canceled) {
        void load(true);
      }
    });
    const interval = window.setInterval(() => void load(), 5000);
    return () => {
      canceled = true;
      window.clearInterval(interval);
    };
  }, [client, session.id]);

  const entries =
    auditLog.status === "success"
      ? auditLog.data.entries.slice(0, MAX_AUDIT_ENTRIES_RENDERED)
      : [];

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium">
            <BellIcon className="size-4" />
            Audit log
          </div>
          <div className="text-muted-foreground mt-1 text-xs">
            Decisions, copy packets, publish drafts, and workflow events.
          </div>
        </div>
        <Badge variant="secondary">{entries.length}</Badge>
      </div>
      {auditLog.status === "idle" && (
        <EmptyState
          className="border-0 p-6"
          title="Audit unavailable"
          description="Connect to cocoded to load the session audit trail."
          icon={ClockIcon}
        />
      )}
      {auditLog.status === "loading" && (
        <LoadingRows rows={5} className="p-4" />
      )}
      {auditLog.status === "error" && (
        <ErrorState
          className="m-3"
          title="Audit log unavailable"
          description={auditLog.error.message}
        />
      )}
      {auditLog.status === "success" && entries.length === 0 && (
        <EmptyState
          className="border-0 p-6"
          title="No audit entries"
          description="Review activity will appear here as it is recorded."
          icon={ClockIcon}
        />
      )}
      {entries.length > 0 && (
        <div className="max-h-[560px] overflow-y-auto">
          {entries.map((entry) => (
            <AuditLogEntryRow key={`${entry.kind}-${entry.id}`} entry={entry} />
          ))}
        </div>
      )}
    </section>
  );
}

function AuditLogEntryRow({ entry }: { entry: ReviewAuditLogEntry }) {
  const metadata = formatAuditMetadata(entry.metadata);
  return (
    <div className="grid grid-cols-[88px_minmax(0,1fr)] gap-3 border-b px-4 py-3 text-sm last:border-b-0">
      <div className="text-muted-foreground text-xs">
        {formatRelativeAge(entry.created_at)}
      </div>
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <Badge
            variant={
              entry.level === "error" || entry.status === "failed"
                ? "destructive"
                : entry.kind === "publish_draft" ||
                    entry.kind === "github_publication"
                  ? "secondary"
                  : "outline"
            }
          >
            {formatAuditKind(entry.kind)}
          </Badge>
          {entry.status && <Badge variant="outline">{entry.status}</Badge>}
          <span className="min-w-0 truncate font-medium">{entry.title}</span>
        </div>
        <div className="text-muted-foreground mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs">
          {entry.sequence ? <span>#{entry.sequence}</span> : null}
          {entry.finding_id ? <span>{entry.finding_id}</span> : null}
          {entry.agent_run_id ? <span>{entry.agent_run_id}</span> : null}
          {entry.artifact_id ? <span>{entry.artifact_id}</span> : null}
          {entry.copy_packet_id ? <span>{entry.copy_packet_id}</span> : null}
          {entry.publish_draft_id ? (
            <span>{entry.publish_draft_id}</span>
          ) : null}
        </div>
        {metadata && (
          <div className="text-muted-foreground mt-2 line-clamp-2 font-mono text-xs">
            {metadata}
          </div>
        )}
      </div>
    </div>
  );
}

function useReviewSessionLiveData(
  client: ApiClient | null,
  initialSession?: ReviewSession,
) {
  const [session, setSession] = useState<ReviewSession | undefined>(
    initialSession,
  );
  const [summary, setSummary] =
    useState<Loadable<ReviewSessionSummary>>(idleApiState());
  const [findings, setFindings] =
    useState<Loadable<FindingListResponse>>(idleApiState());
  const [events, setEvents] = useState<ReviewEvent[]>([]);
  const [refreshState, setRefreshState] = useState<ReviewRefreshState>({
    status: "idle",
  });
  const [streamState, setStreamState] =
    useState<Loadable<true>>(idleApiState());
  const sessionStatusRef = useRef(initialSession?.status);
  const initialSessionRef = useRef(initialSession);

  useEffect(() => {
    sessionStatusRef.current = session?.status;
  }, [session?.status]);

  useEffect(() => {
    initialSessionRef.current = initialSession;
  }, [initialSession]);

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (!canceled) {
        setSession(initialSessionRef.current);
        setEvents([]);
        setRefreshState({ status: "idle" });
        setStreamState(idleApiState());
      }
    });
    return () => {
      canceled = true;
    };
  }, [initialSession?.id]);

  useEffect(() => {
    if (!client || !initialSession) {
      return;
    }
    const api = client;
    const sessionId = initialSession.id;
    let canceled = false;

    async function load(initialLoad = false) {
      if (initialLoad) {
        setSummary((current) =>
          current.status === "success" ? current : loadingApiState(),
        );
        setFindings((current) =>
          current.status === "success" ? current : loadingApiState(),
        );
      } else {
        setRefreshState({ status: "refreshing" });
      }
      const [summaryState, findingsState] = await Promise.all([
        loadApiResource(() => api.reviewSessionSummary(sessionId)),
        loadApiResource(() => api.listFindings(sessionId)),
      ]);
      if (canceled) {
        return;
      }
      setSummary((current) =>
        preserveSuccessfulLoadable(current, summaryState),
      );
      setFindings((current) =>
        preserveSuccessfulLoadable(current, findingsState),
      );
      const refreshErrors = [summaryState, findingsState]
        .filter((state) => state.status === "error")
        .map((state) => (state.status === "error" ? state.error.message : ""));
      setRefreshState(
        refreshErrors.length > 0
          ? { status: "error", message: refreshErrors.join(" ") }
          : { status: "idle" },
      );
    }

    queueMicrotask(() => {
      if (!canceled) {
        void load(true);
      }
    });
    const interval = window.setInterval(() => {
      if (isActiveReviewStatus(sessionStatusRef.current)) {
        void load();
      }
    }, 2500);
    return () => {
      canceled = true;
      window.clearInterval(interval);
    };
  }, [client, initialSession]);

  useEffect(() => {
    if (!client || !initialSession) {
      return;
    }
    const api = client;
    const sessionId = initialSession.id;
    const controller = new AbortController();
    let refreshTimer: number | undefined;
    let refreshInFlight = false;
    let refreshNeedsFindings = false;
    let refreshNeedsSession = false;
    let refreshAgain = false;

    const flushEventRefresh = async () => {
      refreshTimer = undefined;
      if (refreshInFlight) {
        refreshAgain = true;
        return;
      }
      refreshInFlight = true;
      const shouldLoadFindings = refreshNeedsFindings;
      const shouldLoadSession = refreshNeedsSession;
      refreshNeedsFindings = false;
      refreshNeedsSession = false;
      const [sessionState, summaryState, findingsState] = await Promise.all([
        shouldLoadSession
          ? loadApiResource(() => api.getReviewSession(sessionId))
          : Promise.resolve(undefined),
        loadApiResource(() => api.reviewSessionSummary(sessionId)),
        shouldLoadFindings
          ? loadApiResource(() => api.listFindings(sessionId))
          : Promise.resolve(undefined),
      ]);
      if (controller.signal.aborted) {
        return;
      }
      if (sessionState?.status === "success") {
        setSession(sessionState.data);
      }
      setSummary((current) =>
        preserveSuccessfulLoadable(current, summaryState),
      );
      if (findingsState) {
        setFindings((current) =>
          preserveSuccessfulLoadable(current, findingsState),
        );
      }
      refreshInFlight = false;
      if (refreshAgain || refreshNeedsFindings || refreshNeedsSession) {
        refreshAgain = false;
        refreshTimer = window.setTimeout(() => void flushEventRefresh(), 150);
      }
    };

    const scheduleEventRefresh = ({
      findings: includeFindings,
      session: includeSession,
    }: {
      findings?: boolean;
      session?: boolean;
    }) => {
      refreshNeedsFindings = refreshNeedsFindings || Boolean(includeFindings);
      refreshNeedsSession = refreshNeedsSession || Boolean(includeSession);
      if (refreshTimer === undefined) {
        refreshTimer = window.setTimeout(() => void flushEventRefresh(), 150);
      }
    };

    queueMicrotask(() => {
      if (!controller.signal.aborted) {
        setStreamState(loadingApiState());
      }
    });
    void api
      .streamReviewEvents(sessionId, {
        signal: controller.signal,
        onEvent: (event) => {
          setStreamState(successApiState(true));
          setEvents((current) => appendBoundedEvent(current, event));
          if (
            event.type.startsWith("ReviewSession") ||
            event.type.startsWith("Finding") ||
            event.type.startsWith("AgentRun")
          ) {
            scheduleEventRefresh({
              findings: event.type.startsWith("Finding"),
              session: event.type.startsWith("ReviewSession"),
            });
          }
        },
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setStreamState(errorApiState(error));
          setEvents((current) =>
            appendBoundedEvent(current, {
              id: "local_stream_error",
              review_session_id: sessionId,
              type: "EventStreamError",
              level: "warn",
              sequence: current.at(-1)?.sequence ?? 0,
              payload: { message: toErrorMessage(error) },
              created_at: new Date().toISOString(),
            }),
          );
        }
      });
    return () => {
      controller.abort();
      if (refreshTimer !== undefined) {
        window.clearTimeout(refreshTimer);
      }
    };
  }, [client, initialSession]);

  return {
    events,
    findings,
    refreshState,
    session,
    setSession,
    streamState,
    summary,
  };
}

function isActiveReviewStatus(status: ReviewSession["status"] | undefined) {
  return status === "queued" || status === "running" || status === "canceling";
}

function ReviewControlButtons({
  client,
  onSessionUpdated,
  session,
}: {
  client: ApiClient | null;
  onSessionUpdated: (session: ReviewSession) => void;
  session: ReviewSession;
}) {
  const [controlState, setControlState] =
    useState<Loadable<ReviewSession>>(idleApiState());
  const isPaused = session.status === "paused";
  const isTerminal = ["completed", "failed", "canceled"].includes(
    session.status,
  );

  async function runControl(action: "pause" | "resume" | "cancel") {
    if (!client) {
      setControlState(
        errorApiState(new Error("Backend client is unavailable")),
      );
      return;
    }
    setControlState(loadingApiState());
    const state = await loadApiResource(() => {
      if (action === "pause") {
        return client.pauseReviewSession(session.id);
      }
      if (action === "resume") {
        return client.resumeReviewSession(session.id);
      }
      return client.cancelReviewSession(session.id);
    });
    setControlState(state);
    if (state.status === "success") {
      onSessionUpdated(state.data);
    }
  }

  return (
    <div className="flex shrink-0 items-center gap-2">
      {controlState.status === "error" && (
        <span className="text-destructive max-w-56 truncate text-xs">
          {controlState.error.message}
        </span>
      )}
      <Button
        disabled={isTerminal || controlState.status === "loading"}
        size="sm"
        variant="outline"
        onClick={() => void runControl(isPaused ? "resume" : "pause")}
      >
        <PauseIcon data-icon="inline-start" />
        {isPaused ? "Resume" : "Pause"}
      </Button>
      <Button
        disabled={isTerminal || controlState.status === "loading"}
        size="sm"
        variant="outline"
        onClick={() => void runControl("cancel")}
      >
        Cancel
      </Button>
    </div>
  );
}

function ReviewEventTimeline({
  events,
  streamState,
}: {
  events: ReviewEvent[];
  streamState: Loadable<true>;
}) {
  const streamLabel =
    streamState.status === "loading"
      ? "connecting"
      : streamState.status === "error"
        ? "stream issue"
        : streamState.status === "success"
          ? "live"
          : "idle";

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="text-sm font-medium">Event timeline</div>
          <div className="text-muted-foreground mt-1 text-xs">
            Live SSE events with sequence IDs for replay/debugging.
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant="outline">{streamLabel}</Badge>
          <Badge variant="secondary">{events.length}</Badge>
        </div>
      </div>
      {events.length === 0 && streamState.status === "loading" ? (
        <LoadingRows rows={3} className="p-4" />
      ) : events.length === 0 && streamState.status === "error" ? (
        <ErrorState
          className="m-3"
          title="Event stream disconnected"
          description={streamState.error.message}
        />
      ) : events.length === 0 ? (
        <EmptyState
          className="border-0 p-6"
          title="No events yet"
          description="Start a review to stream workflow, agent, and finding events."
          icon={ClockIcon}
        />
      ) : (
        <>
          {streamState.status === "error" && (
            <ErrorState
              className="m-3"
              title="Event stream disconnected"
              description={streamState.error.message}
            />
          )}
          <div className="max-h-[520px] overflow-y-auto">
            {events.map((event) => (
              <div
                key={`${event.sequence}-${event.id}`}
                className="grid grid-cols-[72px_minmax(0,1fr)] gap-3 border-b px-4 py-3 text-sm last:border-b-0"
              >
                <div className="text-muted-foreground text-xs">
                  #{event.sequence}
                </div>
                <div className="min-w-0">
                  <div className="flex min-w-0 items-center gap-2">
                    <Badge
                      variant={
                        event.level === "error" ? "destructive" : "outline"
                      }
                    >
                      {event.level || "info"}
                    </Badge>
                    <span className="truncate font-medium">{event.type}</span>
                  </div>
                  <div className="text-muted-foreground mt-1 truncate text-xs">
                    {formatRelativeAge(event.created_at)}
                    {event.agent_run_id ? ` • ${event.agent_run_id}` : ""}
                    {event.artifact_id ? ` • ${event.artifact_id}` : ""}
                  </div>
                  <div className="text-muted-foreground mt-2 line-clamp-2 font-mono text-xs">
                    {JSON.stringify(event.payload)}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </section>
  );
}

function RunMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="border-border/70 rounded-lg border bg-white px-3 py-2 shadow-[0_1px_2px_rgb(17_18_20/0.02)]">
      <div className="text-muted-foreground text-[11px] leading-4">{label}</div>
      <div className="mt-1 truncate text-[15px] leading-5 font-semibold tabular-nums">
        {value}
      </div>
    </div>
  );
}

function ReviewFindingsBoard({
  client,
  findings,
  onOpenDetail,
  onOpenEvidenceMap,
  onOpenFollowUp,
  session,
}: {
  client: ApiClient | null;
  findings: Loadable<FindingListResponse>;
  onOpenDetail: (finding: Finding) => void;
  onOpenEvidenceMap: (finding: Finding) => void;
  onOpenFollowUp: (finding: Finding) => void;
  session?: ReviewSession;
}) {
  const [statusFilter, setStatusFilter] = useState<FindingStatusFilter>("all");
  const [severityFilter, setSeverityFilter] =
    useState<FindingSeverityFilter>("all");
  const [agentFilter, setAgentFilter] = useState("all");
  const [fileFilter, setFileFilter] = useState("all");
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebouncedValue(query, 250);
  const [boardFindings, setBoardFindings] =
    useState<Loadable<FindingListResponse>>(findings);
  const [selectedFindingId, setSelectedFindingId] = useState<string | null>(
    null,
  );
  const [selectedDetail, setSelectedDetail] =
    useState<Loadable<FindingDetailResponse>>(idleApiState());
  const [dismissReason, setDismissReason] = useState("");
  const [saveDismissalRule, setSaveDismissalRule] = useState(false);
  const [ruleMemorySuggestion, setRuleMemorySuggestion] = useState("");
  const [draftComment, setDraftComment] = useState("");
  const [boardReloadKey, setBoardReloadKey] = useState(0);
  const boardSessionId = session?.id;
  const [actionState, setActionState] = useState<{
    status: "idle" | "loading" | "success" | "error";
    findingId?: string;
    action?: string;
    message?: string;
  }>({ status: "idle" });
  const hasFilters =
    statusFilter !== "all" ||
    severityFilter !== "all" ||
    agentFilter !== "all" ||
    fileFilter !== "all" ||
    query.trim() !== "";

  useEffect(() => {
    if (!client || !boardSessionId) {
      let canceled = false;
      queueMicrotask(() => {
        if (!canceled) {
          setBoardFindings(idleApiState());
        }
      });
      return () => {
        canceled = true;
      };
    }

    const api = client;
    const sessionId = boardSessionId;
    let canceled = false;
    async function load() {
      setBoardFindings(loadingApiState());
      const state = await loadApiResource(() =>
        api.listFindings(sessionId, {
          status: statusFilter === "all" ? undefined : statusFilter,
          severity: severityFilter === "all" ? undefined : severityFilter,
          agent: agentFilter === "all" ? undefined : agentFilter,
          file: fileFilter === "all" ? undefined : fileFilter,
          q: debouncedQuery.trim() || undefined,
        }),
      );
      if (!canceled) {
        setBoardFindings(state);
      }
    }

    queueMicrotask(() => {
      if (!canceled) {
        void load();
      }
    });
    return () => {
      canceled = true;
    };
  }, [
    boardReloadKey,
    boardSessionId,
    agentFilter,
    client,
    debouncedQuery,
    fileFilter,
    severityFilter,
    statusFilter,
  ]);

  useEffect(() => {
    if (hasFilters) {
      return;
    }
    let canceled = false;
    queueMicrotask(() => {
      if (!canceled) {
        setBoardFindings(findings);
      }
    });
    return () => {
      canceled = true;
    };
  }, [findings, hasFilters]);

  useEffect(() => {
    if (boardFindings.status !== "success" || selectedFindingId) {
      return;
    }
    const firstFinding = boardFindings.data.items[0];
    if (!firstFinding) {
      return;
    }
    let canceled = false;
    queueMicrotask(() => {
      if (!canceled) {
        setSelectedFindingId(firstFinding.id);
      }
    });
    return () => {
      canceled = true;
    };
  }, [boardFindings, selectedFindingId]);

  useEffect(() => {
    if (!client || !selectedFindingId) {
      let canceled = false;
      queueMicrotask(() => {
        if (!canceled) {
          setSelectedDetail(idleApiState());
        }
      });
      return () => {
        canceled = true;
      };
    }

    const api = client;
    const findingId = selectedFindingId;
    let canceled = false;
    async function load() {
      setSelectedDetail(loadingApiState());
      const state = await loadApiResource(() =>
        api.getFindingDetail(findingId),
      );
      if (!canceled) {
        setSelectedDetail(state);
      }
    }

    queueMicrotask(() => {
      if (!canceled) {
        void load();
      }
    });
    return () => {
      canceled = true;
    };
  }, [client, selectedFindingId]);

  useEffect(() => {
    if (selectedDetail.status !== "success") {
      return;
    }
    const nextDraft =
      selectedDetail.data.finding.draft_comment ||
      detailedFindingDraftComment(
        selectedDetail.data.finding,
        selectedDetail.data,
      );
    let canceled = false;
    queueMicrotask(() => {
      if (!canceled) {
        setDraftComment(nextDraft);
      }
    });
    return () => {
      canceled = true;
    };
  }, [selectedDetail]);

  const listState = boardFindings;
  const listedFindings =
    listState.status === "success" ? listState.data.items : [];
  const renderedFindings = listedFindings.slice(0, MAX_FINDINGS_RENDERED);
  const selectedFindingFromList = listedFindings.find(
    (finding) => finding.id === selectedFindingId,
  );
  const selectedFindingDetail =
    selectedDetail.status === "success" ? selectedDetail.data : undefined;
  const selectedFinding =
    selectedFindingDetail?.finding ??
    (selectedDetail.status === "idle" || selectedDetail.status === "loading"
      ? selectedFindingFromList
      : undefined);
  const selectedOutsideFilter = Boolean(
    selectedFinding &&
    listedFindings.length > 0 &&
    !listedFindings.some((finding) => finding.id === selectedFinding.id),
  );
  async function updateDecision(
    decision: "accepted" | "dismissed",
    finding = selectedFinding,
  ) {
    if (!client || !finding) {
      setActionState({
        status: "error",
        message: "Select a finding before updating it.",
      });
      return;
    }
    const reason =
      decision === "dismissed"
        ? dismissReason.trim()
        : "accepted from findings board";
    if (decision === "dismissed" && reason === "") {
      setActionState({
        status: "error",
        message: "Dismissal needs a reason.",
      });
      return;
    }
    setActionState({
      status: "loading",
      findingId: finding.id,
      action: decision,
    });
    const state = await loadApiResource(() =>
      client.updateFindingDecision(finding.id, {
        decision,
        reason,
        rule_memory_suggestion:
          decision === "dismissed" && saveDismissalRule
            ? ruleMemorySuggestion.trim() || reason
            : undefined,
      }),
    );
    if (state.status === "success") {
      setSelectedDetail(state);
      setSelectedFindingId(state.data.finding.id);
      setDismissReason("");
      setSaveDismissalRule(false);
      setRuleMemorySuggestion("");
      setBoardReloadKey((current) => current + 1);
      setActionState({
        status: "success",
        findingId: finding.id,
        action: decision,
        message: `${formatDecisionLabel(decision)} saved`,
      });
      return;
    }
    setActionState({
      status: "error",
      message:
        state.status === "error" ? state.error.message : "Decision failed",
    });
  }

  async function copyFinding(finding = selectedFinding) {
    if (!client || !finding) {
      setActionState({
        status: "error",
        message: "Select a finding before copying it.",
      });
      return;
    }
    const content =
      finding.id === selectedFinding?.id && draftComment.trim()
        ? draftComment.trim()
        : findingClipboardText(finding);
    setActionState({
      status: "loading",
      findingId: finding.id,
      action: "copied",
    });
    const state = await loadApiResource(async () => {
      if (!window.cocode?.writeClipboard) {
        throw new Error("Clipboard bridge is unavailable");
      }
      await window.cocode.writeClipboard(content);
      return client.updateFindingDecision(finding.id, {
        decision: "copied",
        reason: "copied from findings board",
      });
    });
    if (state.status === "success") {
      setSelectedDetail(state);
      setSelectedFindingId(state.data.finding.id);
      setBoardReloadKey((current) => current + 1);
      setActionState({
        status: "success",
        findingId: finding.id,
        action: "copied",
        message: "Copied",
      });
      return;
    }
    setActionState({
      status: "error",
      message: state.status === "error" ? state.error.message : "Copy failed",
    });
  }

  async function saveDraftComment() {
    if (!client || !selectedFinding) {
      setActionState({
        status: "error",
        message: "Select a finding before saving a draft.",
      });
      return;
    }
    setActionState({
      status: "loading",
      findingId: selectedFinding.id,
      action: "draft",
    });
    const state = await loadApiResource(async () => {
      const updated = await client.updateFindingDraftComment(
        selectedFinding.id,
        draftComment,
      );
      return client.getFindingDetail(updated.id);
    });
    if (state.status === "success") {
      setSelectedDetail(state);
      setBoardReloadKey((current) => current + 1);
      setActionState({
        status: "success",
        findingId: selectedFinding.id,
        action: "draft",
        message: "Draft saved",
      });
      return;
    }
    setActionState({
      status: "error",
      message: state.status === "error" ? state.error.message : "Save failed",
    });
  }

  async function copyFindingPath() {
    if (!selectedFinding) {
      setActionState({
        status: "error",
        message: "Select a finding before copying its path.",
      });
      return;
    }
    setActionState({
      status: "loading",
      findingId: selectedFinding.id,
      action: "copy-path",
    });
    const state = await loadApiResource(async () => {
      if (!window.cocode?.writeClipboard) {
        throw new Error("Clipboard bridge is unavailable");
      }
      await window.cocode.writeClipboard(
        formatFindingLocation(selectedFinding),
      );
      return true;
    });
    if (state.status === "success") {
      setActionState({
        status: "success",
        findingId: selectedFinding.id,
        action: "copy-path",
        message: "Path copied",
      });
      return;
    }
    setActionState({
      status: "error",
      message: state.status === "error" ? state.error.message : "Copy failed",
    });
  }

  if (!session) {
    return (
      <EmptyState
        title="No review selected"
        description="Findings load after a review session is available."
        icon={FileSearchIcon}
      />
    );
  }

  const stats =
    listState.status === "success"
      ? listState.data.stats
      : findings.status === "success"
        ? findings.data.stats
        : undefined;
  const filterOptions =
    listState.status === "success" ? listState.data.filters : undefined;

  return (
    <section
      aria-label="Review findings board"
      className="border-border/70 overflow-hidden rounded-xl border bg-white shadow-[0_1px_2px_rgb(17_18_20/0.03)]"
    >
      <div className="grid grid-cols-2 gap-3 border-b bg-[#fbfbfa] p-4 md:grid-cols-5">
        <RunMetric label="Total" value={String(stats?.total ?? 0)} />
        <RunMetric
          label="Verified"
          value={String(stats?.by_verification.verified ?? 0)}
        />
        <RunMetric
          label="Needs triage"
          value={String(stats?.needs_triage ?? 0)}
        />
        <RunMetric
          label="Accepted"
          value={String(stats?.by_decision.accepted ?? 0)}
        />
        <RunMetric
          label="Dismissed"
          value={String(stats?.by_decision.dismissed ?? 0)}
        />
      </div>

      <div className="flex flex-wrap items-center gap-2 border-b bg-white p-4">
        <div className="relative min-w-64 flex-[1.4]">
          <SearchIcon className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input
            aria-label="Search findings"
            className="pl-8"
            placeholder="Search findings"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
        </div>
        <NativeSelect
          aria-label="Finding status"
          className="w-36"
          size="sm"
          value={statusFilter}
          onChange={(event) =>
            setStatusFilter(event.target.value as FindingStatusFilter)
          }
        >
          <NativeSelectOption value="all">All statuses</NativeSelectOption>
          <NativeSelectOption value="needs_triage">
            Needs triage
          </NativeSelectOption>
          <NativeSelectOption value="verified">Verified</NativeSelectOption>
          <NativeSelectOption value="accepted">Accepted</NativeSelectOption>
          <NativeSelectOption value="dismissed">Dismissed</NativeSelectOption>
          <NativeSelectOption value="deferred">Deferred</NativeSelectOption>
          <NativeSelectOption value="copied">Copied</NativeSelectOption>
          <NativeSelectOption value="published">Published</NativeSelectOption>
        </NativeSelect>
        <NativeSelect
          aria-label="Finding severity"
          className="w-36"
          size="sm"
          value={severityFilter}
          onChange={(event) =>
            setSeverityFilter(event.target.value as FindingSeverityFilter)
          }
        >
          <NativeSelectOption value="all">All severities</NativeSelectOption>
          <NativeSelectOption value="blocker">Blocker</NativeSelectOption>
          <NativeSelectOption value="high">High</NativeSelectOption>
          <NativeSelectOption value="medium">Medium</NativeSelectOption>
          <NativeSelectOption value="low">Low</NativeSelectOption>
          <NativeSelectOption value="info">Info</NativeSelectOption>
        </NativeSelect>
        <NativeSelect
          aria-label="Finding source agent"
          className="w-44"
          size="sm"
          value={agentFilter}
          onChange={(event) => setAgentFilter(event.target.value)}
        >
          <NativeSelectOption value="all">All agents</NativeSelectOption>
          {(filterOptions?.agents ?? []).map((agent) => (
            <NativeSelectOption key={agent.id} value={agent.id}>
              {agent.label} ({agent.count})
            </NativeSelectOption>
          ))}
        </NativeSelect>
        <NativeSelect
          aria-label="Finding file"
          className="w-44"
          size="sm"
          value={fileFilter}
          onChange={(event) => setFileFilter(event.target.value)}
        >
          <NativeSelectOption value="all">All files</NativeSelectOption>
          {(filterOptions?.files ?? []).slice(0, 80).map((file) => (
            <NativeSelectOption key={file.id} value={file.id}>
              {shortPath(file.label)} ({file.count})
            </NativeSelectOption>
          ))}
        </NativeSelect>
        <Button
          disabled={!hasFilters}
          size="sm"
          variant="outline"
          onClick={() => {
            setStatusFilter("all");
            setSeverityFilter("all");
            setAgentFilter("all");
            setFileFilter("all");
            setQuery("");
          }}
        >
          Reset
        </Button>
      </div>

      {actionState.status === "error" && (
        <ErrorState
          className="m-3"
          title="Finding action failed"
          description={actionState.message ?? "The finding was not updated."}
        />
      )}

      <div className="grid min-h-[560px] bg-white xl:grid-cols-[minmax(0,1.25fr)_minmax(360px,0.72fr)]">
        <div className="min-w-0 border-r">
          <div className="bg-surface/60 text-muted-foreground grid grid-cols-[88px_minmax(0,1.45fr)_minmax(118px,0.75fr)_112px_140px_110px] gap-3 border-b px-4 py-2 text-xs font-medium max-lg:hidden">
            <span>Severity</span>
            <span>Finding</span>
            <span>Location</span>
            <span>Status</span>
            <span>Source / agents</span>
            <span>Confidence</span>
          </div>
          {listState.status === "loading" && (
            <LoadingRows rows={5} className="p-4" />
          )}
          {listState.status === "error" && (
            <ErrorState
              className="m-3"
              title="Findings unavailable"
              description={listState.error.message}
            />
          )}
          {listState.status === "success" && listedFindings.length === 0 && (
            <EmptyState
              className="border-0 p-6"
              title="No findings"
              description="No findings match the current view."
              icon={ShieldCheckIcon}
            />
          )}
          {renderedFindings.map((finding) => (
            <FindingCard
              key={finding.id}
              actionState={actionState}
              finding={finding}
              selected={finding.id === selectedFindingId}
              onAccept={() => {
                setSelectedFindingId(finding.id);
                void updateDecision("accepted", finding);
              }}
              onCopy={() => {
                setSelectedFindingId(finding.id);
                void copyFinding(finding);
              }}
              onOpenDetail={() => onOpenDetail(finding)}
              onSelect={() => setSelectedFindingId(finding.id)}
            />
          ))}
          {listState.status === "success" &&
            listedFindings.length > renderedFindings.length && (
              <div className="text-muted-foreground border-t px-4 py-3 text-xs">
                Showing {renderedFindings.length} of {listedFindings.length}
              </div>
            )}
        </div>

        <div className="min-w-0 bg-[#fbfbfa] p-4">
          {selectedDetail.status === "loading" && <LoadingRows rows={5} />}
          {selectedDetail.status === "error" && (
            <ErrorState
              title="Finding detail unavailable"
              description={selectedDetail.error.message}
            />
          )}
          {!selectedFinding &&
            selectedDetail.status !== "loading" &&
            selectedDetail.status !== "error" && (
              <EmptyState
                className="border-0"
                title="No finding selected"
                description="Select a finding to inspect evidence and actions."
                icon={FileSearchIcon}
              />
            )}
          {selectedFinding && selectedDetail.status === "idle" && (
            <EmptyState
              className="border-0"
              title="Finding detail pending"
              description="Detailed evidence and actions load after selection."
              icon={FileSearchIcon}
            />
          )}
          {selectedFinding && selectedDetail.status === "success" && (
            <div className="flex min-w-0 flex-col gap-4">
              <div>
                <div className="mb-3 flex flex-wrap items-center gap-2">
                  <Badge
                    variant={
                      selectedFinding.severity === "high" ||
                      selectedFinding.severity === "blocker"
                        ? "destructive"
                        : "secondary"
                    }
                  >
                    {selectedFinding.severity}
                  </Badge>
                  <Badge variant="outline">
                    {formatDecisionLabel(selectedFinding.decision_status)}
                  </Badge>
                  <Badge variant="secondary">
                    {formatDecisionLabel(selectedFinding.verification_status)}
                  </Badge>
                  {selectedOutsideFilter && (
                    <Badge variant="outline">Outside filter</Badge>
                  )}
                </div>
                <h2 className="text-base leading-6 font-semibold">
                  {selectedFinding.canonical_claim}
                </h2>
                <div className="text-muted-foreground mt-2 text-xs">
                  {formatFindingLocation(selectedFinding)}
                </div>
              </div>

              <AgentConsensusPanel
                detail={selectedFindingDetail}
                finding={selectedFinding}
              />

              <Tabs defaultValue="overview" className="gap-3">
                <TabsList variant="line">
                  <TabsTrigger value="overview">Overview</TabsTrigger>
                  <TabsTrigger value="code">Code</TabsTrigger>
                  <TabsTrigger value="evidence">Evidence</TabsTrigger>
                </TabsList>

                <TabsContent value="overview" className="flex flex-col gap-3">
                  {selectedFinding.evidence_summary && (
                    <div className="rounded-md border p-3">
                      <div className="text-xs font-medium">Evidence</div>
                      <p className="text-muted-foreground mt-2 text-sm leading-6">
                        {selectedFinding.evidence_summary}
                      </p>
                    </div>
                  )}

                  {selectedFinding.counter_evidence_summary && (
                    <div className="rounded-md border p-3">
                      <div className="text-xs font-medium">
                        Counter-evidence
                      </div>
                      <p className="text-muted-foreground mt-2 text-sm leading-6">
                        {selectedFinding.counter_evidence_summary}
                      </p>
                    </div>
                  )}

                  {selectedFinding.suggested_fix && (
                    <div className="rounded-md border p-3">
                      <div className="text-xs font-medium">Suggested fix</div>
                      <p className="text-muted-foreground mt-2 text-sm leading-6">
                        {selectedFinding.suggested_fix}
                      </p>
                    </div>
                  )}
                </TabsContent>

                <TabsContent value="code">
                  <CodeSnippetViewer
                    evidence={selectedFindingDetail?.evidence_items ?? []}
                    finding={selectedFinding}
                    onCopyPath={() => void copyFindingPath()}
                  />
                </TabsContent>

                <TabsContent value="evidence">
                  <EvidenceCardList detail={selectedFindingDetail} />
                </TabsContent>
              </Tabs>

              <section className="bg-surface/40 rounded-md border p-3">
                <div className="mb-2 flex items-center justify-between gap-2">
                  <div>
                    <div className="text-sm font-semibold">
                      Draft GitHub comment
                    </div>
                    <div className="text-muted-foreground mt-1 text-xs">
                      This is what Copy uses for the selected finding.
                    </div>
                  </div>
                  <Badge variant="outline">
                    {selectedFindingDetail
                      ? `${selectedFindingDetail.candidates.length} candidates`
                      : `${selectedFinding.merged_from_count} merged`}
                  </Badge>
                </div>
                <Textarea
                  aria-label="Draft GitHub comment"
                  className="min-h-40 font-mono text-xs leading-5"
                  value={draftComment}
                  onChange={(event) => setDraftComment(event.target.value)}
                />
                <div className="mt-2 flex justify-end">
                  <Button
                    disabled={actionState.status === "loading"}
                    size="sm"
                    variant="outline"
                    onClick={() => void saveDraftComment()}
                  >
                    Save draft
                  </Button>
                </div>
              </section>

              <div className="border-border/70 bg-surface/45 rounded-md border p-3">
                <div className="mb-2 flex items-center justify-between gap-2">
                  <div>
                    <div className="text-sm font-semibold">Decision</div>
                    <div className="text-muted-foreground mt-0.5 text-xs">
                      Current state:{" "}
                      {findingWorkflowStatusLabel(selectedFinding)}
                    </div>
                  </div>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    disabled={actionState.status === "loading"}
                    size="sm"
                    onClick={() => void updateDecision("accepted")}
                  >
                    <CheckIcon data-icon="inline-start" />
                    Accept
                  </Button>
                  <Button
                    disabled={actionState.status === "loading"}
                    size="sm"
                    variant="outline"
                    onClick={() => void copyFinding()}
                  >
                    <CopyIcon data-icon="inline-start" />
                    Copy fix packet
                  </Button>
                  <Button
                    disabled={actionState.status === "loading"}
                    size="sm"
                    variant="outline"
                    onClick={() => onOpenDetail(selectedFinding)}
                  >
                    <ExternalLinkIcon data-icon="inline-start" />
                    Open full detail
                  </Button>
                  <Button
                    disabled={actionState.status === "loading"}
                    size="sm"
                    variant="outline"
                    onClick={() => onOpenEvidenceMap(selectedFinding)}
                  >
                    <MapIcon data-icon="inline-start" />
                    Open evidence map
                  </Button>
                  <Button
                    disabled={actionState.status === "loading"}
                    size="sm"
                    variant="outline"
                    onClick={() => onOpenFollowUp(selectedFinding)}
                  >
                    <MessageSquareIcon data-icon="inline-start" />
                    Follow-up
                  </Button>
                  <Button
                    disabled={actionState.status === "loading"}
                    size="sm"
                    variant="outline"
                    onClick={() => void updateDecision("dismissed")}
                  >
                    Dismiss
                  </Button>
                </div>
                <details className="mt-3">
                  <summary className="text-muted-foreground flex cursor-pointer list-none items-center justify-between gap-3 text-xs font-medium [&::-webkit-details-marker]:hidden">
                    <span>Dismissal options</span>
                    <ChevronDownIcon className="size-3.5" />
                  </summary>
                  <div className="mt-3 grid gap-2">
                    <Input
                      aria-label="Dismissal reason"
                      placeholder="Dismissal reason"
                      value={dismissReason}
                      onChange={(event) => setDismissReason(event.target.value)}
                    />
                    <label className="flex items-center gap-2 text-sm">
                      <Checkbox
                        checked={saveDismissalRule}
                        onCheckedChange={(checked) =>
                          setSaveDismissalRule(checked === true)
                        }
                      />
                      Save dismissal as local rule
                    </label>
                    {saveDismissalRule && (
                      <Textarea
                        aria-label="Review rule suggestion"
                        className="min-h-20 text-sm"
                        placeholder="Optional guidance. Defaults to the dismissal reason."
                        value={ruleMemorySuggestion}
                        onChange={(event) =>
                          setRuleMemorySuggestion(event.target.value)
                        }
                      />
                    )}
                  </div>
                </details>
                {actionState.status === "success" && actionState.message && (
                  <div className="text-muted-foreground mt-3 text-xs">
                    {actionState.message}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

function PublishReviewScreen({
  client,
  session,
}: {
  client: ApiClient | null;
  session?: ReviewSession;
}) {
  const [acceptedFindings, setAcceptedFindings] =
    useState<Loadable<FindingListResponse>>(idleApiState());
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [reviewEvent, setReviewEvent] = useState("COMMENT");
  const [previewState, setPreviewState] =
    useState<Loadable<GitHubPreviewResponse>>(idleApiState());
  const [copyPacketState, setCopyPacketState] =
    useState<Loadable<CreateCopyPacketResponse>>(idleApiState());
  const [actionMessage, setActionMessage] = useState("");
  const previewAutoKeyRef = useRef("");

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      if (!client || !session) {
        setAcceptedFindings(idleApiState());
        setSelectedIds(new Set());
        return;
      }
      setAcceptedFindings(loadingApiState());
      void loadApiResource(() => client.listFindings(session.id, {})).then(
        (state) => {
          if (canceled) {
            return;
          }
          setAcceptedFindings(state);
          if (state.status === "success") {
            setSelectedIds(
              new Set(
                state.data.items
                  .filter((finding) => finding.decision_status === "accepted")
                  .map((finding) => finding.id),
              ),
            );
          }
        },
      );
    });
    return () => {
      canceled = true;
    };
  }, [client, session?.id, session]);

  const findings = useMemo(
    () =>
      acceptedFindings.status === "success"
        ? acceptedFindings.data.items.filter(
            (finding) => finding.decision_status === "accepted",
          )
        : [],
    [acceptedFindings],
  );
  const selectedFindings = useMemo(
    () => findings.filter((finding) => selectedIds.has(finding.id)),
    [findings, selectedIds],
  );
  const selectedIdList = useMemo(
    () => selectedFindings.map((finding) => finding.id),
    [selectedFindings],
  );
  const canPreview = Boolean(client && session && selectedIdList.length > 0);
  const selectedPreviewKey = `${session?.id ?? ""}:${reviewEvent}:${selectedIdList.join(",")}`;

  const buildPreview = useCallback(async () => {
    if (!client || !session) {
      return;
    }
    setPreviewState(loadingApiState());
    const state = await loadApiResource(() =>
      client.createGitHubPreview(session.id, {
        finding_ids: selectedIdList,
        review_event: reviewEvent,
      }),
    );
    setPreviewState(state);
  }, [client, reviewEvent, selectedIdList, session]);

  useEffect(() => {
    if (!canPreview) {
      previewAutoKeyRef.current = "";
      return;
    }
    if (previewAutoKeyRef.current === selectedPreviewKey) {
      return;
    }
    previewAutoKeyRef.current = selectedPreviewKey;
    queueMicrotask(() => void buildPreview());
  }, [buildPreview, canPreview, selectedPreviewKey]);

  async function buildCopyPacket(copyToClipboard: boolean) {
    if (!client || !session) {
      return;
    }
    setActionMessage("");
    setCopyPacketState(loadingApiState());
    const state = await loadApiResource(() =>
      client.createReviewCopyPacket(session.id, {
        finding_ids: selectedIdList,
        format: "markdown",
        include_evidence: true,
        include_counter_evidence: true,
        include_code_snippets: false,
        target_agent: "reviewer",
      }),
    );
    setCopyPacketState(state);
    if (state.status !== "success" || !copyToClipboard) {
      return;
    }
    if (!window.cocode?.writeClipboard) {
      setActionMessage("Clipboard bridge is unavailable.");
      return;
    }
    const copied = await loadApiResource(() =>
      window.cocode!.writeClipboard(state.data.content),
    );
    setActionMessage(
      copied.status === "success"
        ? "Copy packet copied to clipboard"
        : copied.status === "error"
          ? copied.error.message
          : "Copy failed",
    );
  }

  function toggleFinding(findingId: string) {
    setSelectedIds((current) => {
      const next = new Set(current);
      if (next.has(findingId)) {
        next.delete(findingId);
      } else {
        next.add(findingId);
      }
      return next;
    });
  }

  if (!client) {
    return (
      <ErrorState
        title="Backend client unavailable"
        description="Publish preview and copy packets need an active backend connection."
      />
    );
  }

  if (!session) {
    return (
      <EmptyState
        title="No review selected"
        description="Select a review session before preparing publish output."
        icon={GitPullRequestIcon}
      />
    );
  }

  return (
    <div aria-label="Publish review preview" className="flex flex-col gap-4">
      <PaneHeader
        title="Publish review"
        description="Preview the GitHub review body, inline comments, and detailed copy packet before publishing."
        actions={
          <div className="flex items-center gap-2">
            <Button
              disabled={!canPreview || previewState.status === "loading"}
              size="sm"
              variant="outline"
              onClick={() => void buildPreview()}
            >
              <FileSearchIcon data-icon="inline-start" />
              Preview
            </Button>
            <Button
              disabled={!canPreview || copyPacketState.status === "loading"}
              size="sm"
              onClick={() => void buildCopyPacket(true)}
            >
              <CopyIcon data-icon="inline-start" />
              Copy packet
            </Button>
          </div>
        }
      />

      <div className="grid gap-4 2xl:grid-cols-[360px_minmax(0,1fr)]">
        <aside className="flex min-w-0 flex-col gap-4">
          <div className="rounded-lg border">
            <div className="flex items-center justify-between gap-2 border-b px-4 py-3">
              <div>
                <div className="text-sm font-medium">Accepted findings</div>
                <div className="text-muted-foreground mt-1 text-xs">
                  {selectedIds.size} selected
                </div>
              </div>
              <Button
                disabled={findings.length === 0}
                size="sm"
                variant="outline"
                onClick={() =>
                  setSelectedIds(new Set(findings.map((finding) => finding.id)))
                }
              >
                All
              </Button>
            </div>
            {acceptedFindings.status === "loading" && (
              <LoadingRows rows={4} className="p-4" />
            )}
            {acceptedFindings.status === "error" && (
              <div className="p-4">
                <ErrorState
                  title="Accepted findings failed to load"
                  description={acceptedFindings.error.message}
                />
              </div>
            )}
            {acceptedFindings.status === "success" && findings.length === 0 && (
              <EmptyState
                title="No accepted findings yet"
                description="Accept findings before building a publish preview."
                icon={InboxIcon}
              />
            )}
            {findings.length > 0 && (
              <div className="flex flex-col">
                {findings.map((finding) => (
                  <label
                    key={finding.id}
                    className="hover:bg-surface flex cursor-pointer gap-3 border-b px-4 py-3 last:border-b-0"
                  >
                    <Checkbox
                      checked={selectedIds.has(finding.id)}
                      onCheckedChange={() => toggleFinding(finding.id)}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="line-clamp-2 text-sm font-medium">
                        {finding.canonical_claim}
                      </span>
                      <span className="text-muted-foreground mt-1 block truncate text-xs">
                        {formatFindingLocation(finding)}
                      </span>
                    </span>
                  </label>
                ))}
              </div>
            )}
          </div>

          <div className="rounded-lg border p-4">
            <label className="flex flex-col gap-2 text-sm font-medium">
              Review event
              <NativeSelect
                value={reviewEvent}
                onChange={(event) => setReviewEvent(event.target.value)}
              >
                <NativeSelectOption value="COMMENT">Comment</NativeSelectOption>
                <NativeSelectOption value="REQUEST_CHANGES">
                  Request changes
                </NativeSelectOption>
                <NativeSelectOption value="APPROVE">Approve</NativeSelectOption>
              </NativeSelect>
            </label>
            <div className="mt-3 rounded-md border p-3">
              <Badge variant="outline">GitHub submit unavailable</Badge>
              <p className="text-muted-foreground mt-2 text-xs leading-5">
                Preview and copy packet are ready; direct submit waits for the
                backend publish route.
              </p>
            </div>
            {actionMessage && (
              <p className="text-muted-foreground mt-2 text-sm">
                {actionMessage}
              </p>
            )}
          </div>
        </aside>

        <div className="grid min-w-0 gap-4 min-[1800px]:grid-cols-2">
          <GitHubPreviewPane state={previewState} />
          <CopyPacketPreviewPane
            state={copyPacketState}
            onBuild={() => void buildCopyPacket(false)}
            disabled={!canPreview || copyPacketState.status === "loading"}
          />
        </div>
      </div>
    </div>
  );
}

function GitHubPreviewPane({
  state,
}: {
  state: Loadable<GitHubPreviewResponse>;
}) {
  const comments =
    state.status === "success" ? (state.data.comments ?? []) : [];
  const warnings =
    state.status === "success" ? (state.data.warnings ?? []) : [];

  return (
    <section className="rounded-lg border">
      <div className="border-b px-4 py-3">
        <div className="text-sm font-medium">GitHub review preview</div>
        <div className="text-muted-foreground mt-1 text-xs">
          Review body, inline comments, warnings, and checklist.
        </div>
      </div>
      {state.status === "idle" && (
        <EmptyState
          title="No preview yet"
          description="Select accepted findings and build a preview."
          icon={FileSearchIcon}
        />
      )}
      {state.status === "loading" && <LoadingRows rows={5} className="p-4" />}
      {state.status === "error" && (
        <div className="p-4">
          <ErrorState
            title="Preview failed"
            description={state.error.message}
          />
        </div>
      )}
      {state.status === "success" && (
        <ScrollArea className="h-[620px]">
          <div className="flex flex-col gap-4 p-4">
            <GitHubPreviewChecklistView preview={state.data} />
            <div className="rounded-md border p-3">
              <div className="mb-2 text-xs font-medium">Review body</div>
              <p className="text-muted-foreground text-sm leading-6 whitespace-pre-wrap">
                {state.data.body}
              </p>
            </div>
            <div>
              <div className="mb-2 flex items-center justify-between gap-2">
                <div className="text-sm font-medium">Inline comments</div>
                <Badge variant="outline">{comments.length}</Badge>
              </div>
              <div className="flex flex-col gap-2">
                {comments.map((comment) => (
                  <div
                    key={comment.finding_id}
                    className="rounded-md border p-3"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <span className="truncate text-sm font-medium">
                        {comment.path || "Summary comment"}
                      </span>
                      <Badge
                        className="shrink-0"
                        variant={comment.unanchored ? "destructive" : "outline"}
                      >
                        {comment.unanchored ? "unanchored" : "anchored"}
                      </Badge>
                    </div>
                    <p className="text-muted-foreground mt-2 text-sm leading-6 whitespace-pre-wrap">
                      {comment.body}
                    </p>
                    {comment.warning && (
                      <p className="text-destructive mt-2 text-xs">
                        {comment.warning}
                      </p>
                    )}
                  </div>
                ))}
              </div>
            </div>
            {warnings.length > 0 && (
              <div>
                <div className="mb-2 text-sm font-medium">Warnings</div>
                <div className="flex flex-col gap-2">
                  {warnings.map((warning) => (
                    <div
                      key={`${warning.finding_id}:${warning.message}`}
                      className="rounded-md border p-3"
                    >
                      <div className="text-sm font-medium">
                        {warning.path || warning.finding_id}
                      </div>
                      <p className="text-muted-foreground mt-1 text-sm">
                        {warning.message}
                      </p>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        </ScrollArea>
      )}
    </section>
  );
}

function GitHubPreviewChecklistView({
  preview,
}: {
  preview: GitHubPreviewResponse;
}) {
  const items = [
    ["Selected findings", preview.checklist.has_selected_findings],
    ["Inline comments", preview.checklist.has_inline_comments],
    ["Unanchored comments", preview.checklist.has_unanchored_comments],
    ["Can publish inline", preview.checklist.can_publish_inline],
    ["Can publish summary-only", preview.checklist.can_publish_summary_only],
  ] as const;
  return (
    <div className="grid grid-cols-[repeat(auto-fit,minmax(170px,1fr))] gap-2">
      {items.map(([label, ok]) => (
        <div
          key={label}
          className="flex items-center gap-2 rounded-md border p-2"
        >
          <Badge variant={ok ? "secondary" : "outline"}>
            {ok ? "yes" : "no"}
          </Badge>
          <span className="text-sm">{label}</span>
        </div>
      ))}
    </div>
  );
}

function CopyPacketPreviewPane({
  disabled,
  onBuild,
  state,
}: {
  disabled: boolean;
  onBuild: () => void;
  state: Loadable<CreateCopyPacketResponse>;
}) {
  return (
    <section className="rounded-lg border">
      <div className="flex items-center justify-between gap-2 border-b px-4 py-3">
        <div>
          <div className="text-sm font-medium">Copy packet</div>
          <div className="text-muted-foreground mt-1 text-xs">
            Markdown packet for handing findings to another agent.
          </div>
        </div>
        <Button
          disabled={disabled}
          size="sm"
          variant="outline"
          onClick={onBuild}
        >
          Build
        </Button>
      </div>
      {state.status === "idle" && (
        <EmptyState
          title="No packet yet"
          description="Build or copy a packet from the selected findings."
          icon={CopyIcon}
        />
      )}
      {state.status === "loading" && <LoadingRows rows={5} className="p-4" />}
      {state.status === "error" && (
        <div className="p-4">
          <ErrorState
            title="Copy packet failed"
            description={state.error.message}
          />
        </div>
      )}
      {state.status === "success" && (
        <ScrollArea className="h-[620px]">
          <div className="flex flex-col gap-3 p-4">
            <div className="flex flex-wrap gap-2">
              <Badge variant="secondary">
                {state.data.finding_count} findings
              </Badge>
              <Badge variant="outline">
                {state.data.token_estimate} tokens
              </Badge>
              <Badge variant="outline">{state.data.format}</Badge>
            </div>
            <pre className="bg-surface overflow-x-auto rounded-md border p-3 text-xs leading-5 whitespace-pre-wrap">
              {state.data.content}
            </pre>
          </div>
        </ScrollArea>
      )}
    </section>
  );
}

function evidenceItemsOrEmpty(items?: EvidenceItem[] | null): EvidenceItem[] {
  return Array.isArray(items) ? items : [];
}

function FindingDetailScreen({
  agentConfigs,
  client,
  events,
  finding,
  onBack,
  onOpenEvidenceMap,
}: {
  agentConfigs: Loadable<AgentConfig[]>;
  client: ApiClient | null;
  events: ReviewEvent[];
  finding: Finding;
  onBack: () => void;
  onOpenEvidenceMap: (finding: Finding) => void;
}) {
  const [detailState, setDetailState] =
    useState<Loadable<FindingDetailResponse>>(loadingApiState());
  const [threadState, setThreadState] =
    useState<Loadable<FindingThreadView>>(loadingApiState());
  const [draftComment, setDraftComment] = useState("");
  const [question, setQuestion] = useState("");
  const [selectedAgentId, setSelectedAgentId] = useState("");
  const [dismissReason, setDismissReason] = useState("");
  const [actionState, setActionState] =
    useState<Loadable<FindingDetailResponse | AskFindingQuestionResponse>>(
      idleApiState(),
    );

  const reload = useCallback(async () => {
    if (!client) {
      const error = new Error("Backend client is unavailable");
      setDetailState(errorApiState(error));
      setThreadState(errorApiState(error));
      return;
    }
    const [detail, thread] = await Promise.all([
      loadApiResource(() => client.getFindingDetail(finding.id)),
      loadApiResource(() => client.getFindingThread(finding.id)),
    ]);
    setDetailState(detail);
    setThreadState(thread);
    if (detail.status === "success") {
      setDraftComment(
        detail.data.finding.draft_comment ||
          detailedFindingDraftComment(detail.data.finding, detail.data),
      );
    }
  }, [client, finding.id]);

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (!canceled) {
        void reload();
      }
    });
    return () => {
      canceled = true;
    };
  }, [reload]);

  const detail =
    detailState.status === "success" ? detailState.data : undefined;
  const activeFinding = detail?.finding ?? finding;
  const supportingEvidence = evidenceItemsOrEmpty(
    detail?.evidence_groups?.supporting,
  );
  const counterEvidence = evidenceItemsOrEmpty(
    detail?.evidence_groups?.counter,
  );
  const testEvidence = evidenceItemsOrEmpty(detail?.evidence_groups?.test);
  const runtimeEvents = useMemo(
    () => followUpRuntimeEvents(events, activeFinding.id),
    [activeFinding.id, events],
  );
  const agents = agentConfigs.status === "success" ? agentConfigs.data : [];
  const followUpAgents = agents.filter(
    (agent) => agent.enabled && !agent.capabilities.can_write,
  );

  async function updateDecision(decision: "accepted" | "dismissed") {
    if (!client) {
      setActionState(errorApiState(new Error("Backend client is unavailable")));
      return;
    }
    if (decision === "dismissed" && !dismissReason.trim()) {
      setActionState(errorApiState(new Error("Dismissal reason is required.")));
      return;
    }
    setActionState(loadingApiState());
    const state = await loadApiResource(() =>
      client.updateFindingDecision(activeFinding.id, {
        decision,
        reason:
          decision === "dismissed"
            ? dismissReason.trim()
            : "accepted from finding detail",
      }),
    );
    setActionState(state);
    if (state.status === "success") {
      setDetailState(state);
      setDismissReason("");
    }
  }

  async function saveDraftComment() {
    if (!client) {
      setActionState(errorApiState(new Error("Backend client is unavailable")));
      return;
    }
    setActionState(loadingApiState());
    const state = await loadApiResource(async () => {
      const updated = await client.updateFindingDraftComment(
        activeFinding.id,
        draftComment,
      );
      return client.getFindingDetail(updated.id);
    });
    setActionState(state);
    if (state.status === "success") {
      setDetailState(state);
    }
  }

  async function copyDraftComment() {
    setActionState(loadingApiState());
    const state = await loadApiResource(async () => {
      if (!window.cocode?.writeClipboard) {
        throw new Error("Clipboard bridge is unavailable");
      }
      if (!detail) {
        throw new Error("Finding detail is still loading");
      }
      await window.cocode.writeClipboard(
        draftComment.trim() ||
          detailedFindingDraftComment(activeFinding, detail),
      );
      return detail;
    });
    setActionState(state);
  }

  async function askQuestion(
    nextQuestion: string,
    contextPolicy: ReviewContextPolicy,
    agentConfigId?: string,
  ) {
    if (!client || !nextQuestion.trim()) {
      return;
    }
    setActionState(loadingApiState());
    const state = await loadApiResource(() =>
      client.askFindingQuestion(activeFinding.id, {
        question: nextQuestion.trim(),
        agent_config_id: agentConfigId || selectedAgentId || undefined,
        context_policy: contextPolicy,
      }),
    );
    setActionState(state);
    if (state.status === "success") {
      setQuestion("");
      setThreadState(successApiState(state.data.thread));
    }
  }

  return (
    <div className="flex min-h-0 flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="text-muted-foreground mb-2 flex items-center gap-2 text-xs">
            <span>Findings</span>
            <ChevronDownIcon className="size-3 -rotate-90" />
            <span className="text-foreground">
              {truncate(activeFinding.canonical_claim, 72)}
            </span>
          </div>
          <h2 className="text-xl leading-7 font-semibold">
            {activeFinding.canonical_claim}
          </h2>
          <p className="text-muted-foreground mt-1 font-mono text-xs">
            {formatFindingLocation(activeFinding)}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="outline" onClick={onBack}>
            <ArrowLeftIcon data-icon="inline-start" />
            Findings
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => onOpenEvidenceMap(activeFinding)}
          >
            <MapIcon data-icon="inline-start" />
            Evidence map
          </Button>
        </div>
      </div>

      {detailState.status === "loading" && (
        <LoadingRows rows={8} className="cocode-panel p-4" />
      )}
      {detailState.status === "error" && (
        <ErrorState
          title="Finding detail unavailable"
          description={detailState.error.message}
        />
      )}
      {detail && (
        <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(300px,0.75fr)_280px]">
          <section className="cocode-panel min-w-0 overflow-hidden">
            <div className="border-b px-4 py-3">
              <div className="text-sm font-semibold">Changed file</div>
              <div className="text-muted-foreground mt-1 truncate font-mono text-xs">
                {activeFinding.primary_path || "No primary file"}
              </div>
            </div>
            <div className="p-4">
              <CodeSnippetViewer
                evidence={detail.evidence_items}
                finding={activeFinding}
                onCopyPath={() => {
                  void window.cocode?.writeClipboard?.(
                    formatFindingLocation(activeFinding),
                  );
                }}
              />
            </div>
          </section>

          <section className="flex min-w-0 flex-col gap-3">
            <div className="cocode-panel p-4">
              <div className="text-sm font-semibold">Detailed explanation</div>
              <p className="text-muted-foreground mt-2 text-sm leading-6">
                {activeFinding.evidence_summary ||
                  "The selected agents reported this finding from the changed code and evidence bundle."}
              </p>
            </div>
            <EvidenceNarrativeCard
              title="Supporting evidence"
              items={supportingEvidence}
              fallback={activeFinding.evidence_summary}
            />
            <EvidenceNarrativeCard
              title="Counter-evidence"
              items={counterEvidence}
              fallback={activeFinding.counter_evidence_summary}
            />
            <EvidenceNarrativeCard
              title="Related tests"
              items={testEvidence}
              fallback="No related test signal was found for this finding."
            />
            {activeFinding.suggested_fix && (
              <div className="cocode-panel p-4">
                <div className="text-sm font-semibold">Suggested fix</div>
                <p className="text-muted-foreground mt-2 text-sm leading-6">
                  {activeFinding.suggested_fix}
                </p>
              </div>
            )}
            <AgentConsensusPanel detail={detail} finding={activeFinding} />
          </section>

          <aside className="flex min-w-0 flex-col gap-3">
            <div className="cocode-panel p-4">
              <div className="mb-3 text-sm font-semibold">Finding details</div>
              <FindingDetailFacts finding={activeFinding} detail={detail} />
            </div>
            <div className="cocode-panel p-4">
              <div className="mb-3 text-sm font-semibold">Update status</div>
              <div className="grid gap-2">
                <div className="text-muted-foreground rounded-md border bg-[#fbfbfa] px-3 py-2 text-xs">
                  Current state: {findingWorkflowStatusLabel(activeFinding)}
                </div>
                <Button
                  disabled={actionState.status === "loading"}
                  onClick={() => void updateDecision("accepted")}
                >
                  <CheckIcon data-icon="inline-start" />
                  Accept finding
                </Button>
                <details>
                  <summary className="text-muted-foreground flex cursor-pointer list-none items-center justify-between rounded-md border bg-white px-3 py-2 text-xs font-medium [&::-webkit-details-marker]:hidden">
                    <span>Dismissal reason</span>
                    <ChevronDownIcon className="size-3.5" />
                  </summary>
                  <Input
                    aria-label="Dismissal reason"
                    className="mt-2"
                    placeholder="Why is this not actionable?"
                    value={dismissReason}
                    onChange={(event) => setDismissReason(event.target.value)}
                  />
                </details>
                <Button
                  disabled={actionState.status === "loading"}
                  variant="outline"
                  onClick={() => void updateDecision("dismissed")}
                >
                  Dismiss finding
                </Button>
              </div>
            </div>
            <div className="cocode-panel p-4">
              <div className="mb-2 flex items-center justify-between gap-2">
                <div className="text-sm font-semibold">
                  Draft GitHub comment
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => void copyDraftComment()}
                >
                  <CopyIcon data-icon="inline-start" />
                  Copy
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => void saveDraftComment()}
                >
                  Save
                </Button>
              </div>
              <Textarea
                aria-label="Detailed draft GitHub comment"
                className="min-h-44 font-mono text-xs"
                value={draftComment}
                onChange={(event) => setDraftComment(event.target.value)}
              />
            </div>
            {actionState.status === "error" && (
              <ErrorState
                title="Finding action failed"
                description={actionState.error.message}
              />
            )}
          </aside>
        </div>
      )}

      <section className="cocode-panel">
        <div className="border-b px-4 py-3">
          <div className="text-sm font-semibold">Finding thread</div>
          <div className="text-muted-foreground mt-1 text-xs">
            Ask scoped questions and keep the answers attached to this finding.
          </div>
        </div>
        {threadState.status === "loading" && <LoadingRows rows={3} />}
        {threadState.status === "error" && (
          <ErrorState
            className="m-4"
            title="Finding thread unavailable"
            description={threadState.error.message}
          />
        )}
        {threadState.status === "success" && (
          <FollowUpMessages messages={threadState.data.messages} />
        )}
        <AgentRuntimeTrace
          events={runtimeEvents}
          loading={actionState.status === "loading"}
        />
        <MessageComposer
          agents={followUpAgents}
          backendDetail="Uses finding evidence and prior thread messages."
          defaultMode="finding follow-up"
          disabled={!client}
          onQuestionChange={setQuestion}
          onSelectedAgentIdChange={setSelectedAgentId}
          onSubmit={(nextQuestion, options) =>
            askQuestion(
              nextQuestion,
              options.contextPolicy,
              options.agentConfigId,
            )
          }
          question={question}
          selectedAgentId={selectedAgentId}
          submitting={actionState.status === "loading"}
        />
      </section>
    </div>
  );
}

function EvidenceNarrativeCard({
  fallback,
  items,
  title,
}: {
  fallback?: string;
  items: EvidenceItem[];
  title: string;
}) {
  return (
    <div className="cocode-panel p-4">
      <div className="text-sm font-semibold">{title}</div>
      {items.length > 0 ? (
        <ul className="text-muted-foreground mt-2 space-y-2 text-sm leading-6">
          {items.slice(0, 5).map((item) => (
            <li key={item.id}>
              <span className="text-foreground font-medium">{item.title}</span>
              <span> — {item.summary}</span>
              {item.path && (
                <span className="font-mono text-xs">
                  {" "}
                  ({formatEvidenceLocation(item)})
                </span>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-muted-foreground mt-2 text-sm leading-6">
          {fallback || "No stored evidence for this section yet."}
        </p>
      )}
    </div>
  );
}

function FindingDetailFacts({
  detail,
  finding,
}: {
  detail: FindingDetailResponse;
  finding: Finding;
}) {
  const rows = [
    ["Finding ID", finding.id],
    ["Severity", formatDecisionLabel(finding.severity)],
    ["Status", formatDecisionLabel(finding.verification_status)],
    ["Decision", formatDecisionLabel(finding.decision_status)],
    ["Agents", String(detail.candidates.length || finding.merged_from_count)],
    ["Confidence", `${Math.round(finding.confidence * 100)}%`],
    ["First seen", formatShortDate(finding.first_seen_at)],
    ["Updated", formatShortDate(finding.updated_at)],
  ];
  return (
    <dl className="grid gap-3 text-sm">
      {rows.map(([label, value]) => (
        <div className="grid grid-cols-[90px_minmax(0,1fr)] gap-3" key={label}>
          <dt className="text-muted-foreground">{label}</dt>
          <dd className="min-w-0 truncate text-right font-medium">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function FindingFollowUpScreen({
  agentConfigs,
  client,
  events,
  finding,
  onBack,
}: {
  agentConfigs: Loadable<AgentConfig[]>;
  client: ApiClient | null;
  events: ReviewEvent[];
  finding: Finding;
  onBack: () => void;
}) {
  const [threadState, setThreadState] =
    useState<Loadable<FindingThreadView>>(idleApiState());
  const [detailState, setDetailState] =
    useState<Loadable<FindingDetailResponse>>(idleApiState());
  const [question, setQuestion] = useState("");
  const [dismissReason, setDismissReason] = useState("");
  const [selectedAgentId, setSelectedAgentId] = useState("");
  const [actionState, setActionState] =
    useState<Loadable<AskFindingQuestionResponse | FindingQuickActionResponse>>(
      idleApiState(),
    );

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      if (!client) {
        const error = new Error("Backend client is unavailable");
        setThreadState(errorApiState(error));
        setDetailState(errorApiState(error));
        return;
      }
      setThreadState(loadingApiState());
      setDetailState(loadingApiState());
      void Promise.all([
        loadApiResource(() => client.getFindingThread(finding.id)),
        loadApiResource(() => client.getFindingDetail(finding.id)),
      ]).then(([thread, detail]) => {
        if (canceled) {
          return;
        }
        setThreadState(thread);
        setDetailState(detail);
      });
    });
    return () => {
      canceled = true;
    };
  }, [client, finding.id]);

  const agentList = agentConfigs.status === "success" ? agentConfigs.data : [];
  const followUpAgents = agentList.filter(
    (agent) =>
      agent.enabled &&
      (agent.adapter_kind === "local_verifier" ||
        agent.adapter_kind === "cli_noninteractive" ||
        agent.adapter_kind === "cli_non_interactive" ||
        agent.adapter_kind === "jsonrpc_stdio" ||
        agent.adapter_kind === "acp_stdio"),
  );
  const evidenceItems =
    detailState.status === "success"
      ? prioritizedEvidenceItems(detailState.data.evidence_items).slice(0, 8)
      : [];
  const messages =
    threadState.status === "success" ? threadState.data.messages : [];
  const activeFinding =
    threadState.status === "success" ? threadState.data.finding : finding;
  const runtimeEvents = useMemo(
    () => followUpRuntimeEvents(events, activeFinding.id),
    [activeFinding.id, events],
  );
  const selectedAgent = followUpAgents.find(
    (agent) => agent.id === selectedAgentId,
  );

  async function askQuestion(
    nextQuestion: string,
    contextPolicy: ReviewContextPolicy,
    agentConfigId?: string,
  ) {
    if (!client || !nextQuestion.trim()) {
      return;
    }
    setActionState(loadingApiState());
    const state = await loadApiResource(() =>
      client.askFindingQuestion(finding.id, {
        question: nextQuestion.trim(),
        agent_config_id: agentConfigId || selectedAgentId || undefined,
        context_policy: contextPolicy,
      }),
    );
    setActionState(state);
    if (state.status === "success") {
      setQuestion("");
      setThreadState(successApiState(state.data.thread));
    }
  }

  async function runQuickAction(action: string) {
    if (!client) {
      return;
    }
    if (action === "dismiss" && !dismissReason.trim()) {
      setActionState(errorApiState(new Error("Dismissal reason is required.")));
      return;
    }
    setActionState(loadingApiState());
    const state = await loadApiResource(() =>
      client.runFindingQuickAction(finding.id, {
        action,
        reason: action === "dismiss" ? dismissReason.trim() : undefined,
        agent_config_id: selectedAgentId || undefined,
        context_policy: { max_tokens: 8_000, max_items: 80 },
      }),
    );
    setActionState(state);
    if (state.status === "success") {
      setThreadState(successApiState(state.data.thread));
      setDismissReason("");
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <PaneHeader
        title="Finding follow-up"
        description={activeFinding.canonical_claim}
        actions={
          <Button size="sm" variant="outline" onClick={onBack}>
            <ArrowLeftIcon data-icon="inline-start" />
            Findings
          </Button>
        }
      />

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="flex min-w-0 flex-col gap-4">
          <div className="rounded-lg border p-4">
            <div className="mb-3 flex flex-wrap items-center gap-2">
              <Badge>{activeFinding.severity}</Badge>
              <Badge variant="outline">
                {activeFinding.verification_status}
              </Badge>
              <Badge variant="outline">{activeFinding.decision_status}</Badge>
              <Badge variant="secondary">
                {activeFinding.merged_from_count} candidates
              </Badge>
            </div>
            <h2 className="text-lg leading-7 font-semibold">
              {activeFinding.canonical_claim}
            </h2>
            <div className="text-muted-foreground mt-2 text-sm">
              {formatFindingLocation(activeFinding)}
            </div>
            {activeFinding.evidence_summary && (
              <p className="text-muted-foreground mt-3 text-sm leading-6">
                {activeFinding.evidence_summary}
              </p>
            )}
          </div>

          <div className="rounded-lg border">
            <div className="flex items-center justify-between gap-2 border-b px-4 py-3">
              <div>
                <div className="text-sm font-medium">Thread</div>
                <div className="text-muted-foreground mt-1 text-xs">
                  {messages.length} messages
                </div>
              </div>
              {selectedAgent && (
                <Badge variant="outline">{selectedAgent.name}</Badge>
              )}
            </div>
            {threadState.status === "loading" && (
              <LoadingRows rows={4} className="p-4" />
            )}
            {threadState.status === "error" && (
              <div className="p-4">
                <ErrorState
                  title="Thread failed to load"
                  description={threadState.error.message}
                />
              </div>
            )}
            {threadState.status === "success" && (
              <FollowUpMessages messages={messages} />
            )}
          </div>

          <AgentRuntimeTrace
            events={runtimeEvents}
            loading={actionState.status === "loading"}
          />

          <MessageComposer
            agents={followUpAgents}
            backendDetail="Uses finding context and evidence refs."
            defaultMode="finding follow-up"
            disabled={!client}
            disabledReason={
              client ? undefined : "Connect to cocoded before asking follow-up."
            }
            onQuestionChange={setQuestion}
            onSelectedAgentIdChange={setSelectedAgentId}
            onSubmit={(nextQuestion, options) =>
              askQuestion(
                nextQuestion,
                options.contextPolicy,
                options.agentConfigId,
              )
            }
            question={question}
            selectedAgentId={selectedAgentId}
            submitting={actionState.status === "loading"}
          />
        </div>

        <aside className="flex min-w-0 flex-col gap-4">
          <div className="rounded-lg border p-4">
            <div className="mb-3 flex items-center justify-between gap-2">
              <div className="text-sm font-medium">Quick actions</div>
              {selectedAgent ? (
                <Badge variant="outline">{selectedAgent.name}</Badge>
              ) : (
                <Badge variant="secondary">Auto-select</Badge>
              )}
            </div>
            {agentConfigs.status === "loading" && (
              <LoadingRows rows={2} className="mt-3" />
            )}
            {agentConfigs.status === "error" && (
              <ErrorState
                className="mt-3"
                title="Follow-up agents unavailable"
                description={agentConfigs.error.message}
              />
            )}
            {agentConfigs.status === "success" &&
              followUpAgents.length === 0 && (
                <EmptyState
                  className="border-0 p-2"
                  title="No follow-up agents"
                  description="Enable a verifier or non-interactive CLI agent to target follow-up questions."
                  icon={BotIcon}
                />
              )}
            <div className="mt-4 grid grid-cols-2 gap-2">
              <Button
                disabled={actionState.status === "loading"}
                size="sm"
                variant="outline"
                onClick={() => void runQuickAction("ask_counter_evidence")}
              >
                <SearchIcon data-icon="inline-start" />
                Counter
              </Button>
              <Button
                disabled={actionState.status === "loading"}
                size="sm"
                variant="outline"
                onClick={() => void runQuickAction("accept")}
              >
                <CheckIcon data-icon="inline-start" />
                Accept
              </Button>
              <Button
                disabled={actionState.status === "loading"}
                size="sm"
                variant="outline"
                onClick={() => void runQuickAction("copy")}
              >
                <CopyIcon data-icon="inline-start" />
                Copy
              </Button>
              <Button
                disabled={actionState.status === "loading"}
                size="sm"
                variant="outline"
                onClick={() => void runQuickAction("dismiss")}
              >
                Dismiss
              </Button>
            </div>
            <Input
              aria-label="Follow-up dismissal reason"
              className="mt-2"
              placeholder="Dismissal reason"
              value={dismissReason}
              onChange={(event) => setDismissReason(event.target.value)}
            />
            {actionState.status === "error" && (
              <p className="text-destructive mt-2 text-sm">
                {actionState.error.message}
              </p>
            )}
            {actionState.status === "success" && (
              <p className="text-muted-foreground mt-2 text-sm">
                Follow-up updated
              </p>
            )}
          </div>

          <div className="rounded-lg border p-4">
            <div className="mb-3 flex items-center justify-between gap-2">
              <div className="text-sm font-medium">Evidence bundle</div>
              <Badge variant="outline">{evidenceItems.length}</Badge>
            </div>
            <div className="flex flex-col gap-2">
              {detailState.status === "loading" && <LoadingRows rows={3} />}
              {detailState.status === "error" && (
                <ErrorState
                  title="Evidence bundle unavailable"
                  description={detailState.error.message}
                />
              )}
              {evidenceItems.map((item) => (
                <div key={item.id} className="rounded-md border p-3">
                  <div className="flex items-center justify-between gap-2">
                    <span className="truncate text-sm font-medium">
                      {item.title}
                    </span>
                    <Badge variant={evidenceBadgeVariant(item.kind)}>
                      {item.kind}
                    </Badge>
                  </div>
                  <div className="text-muted-foreground mt-1 text-xs">
                    {formatEvidenceLocation(item)}
                  </div>
                  <p className="text-muted-foreground mt-2 line-clamp-3 text-sm leading-6">
                    {item.summary}
                  </p>
                </div>
              ))}
              {detailState.status === "success" &&
                evidenceItems.length === 0 && (
                  <EmptyState
                    className="border-0 p-2"
                    title="No evidence items"
                    description="This finding does not have evidence bundle entries yet."
                    icon={FileSearchIcon}
                  />
                )}
            </div>
          </div>
        </aside>
      </div>
    </div>
  );
}

function FollowUpMessages({
  messages,
}: {
  messages: FindingThreadView["messages"];
}) {
  const visibleMessages = messages.filter(
    (message) => message.role !== "system",
  );
  if (visibleMessages.length === 0) {
    return (
      <EmptyState
        title="No follow-ups yet"
        description="Ask a scoped question or use a quick action to start the thread."
        icon={MessageSquareIcon}
      />
    );
  }
  return (
    <div className="flex flex-col gap-3 p-4">
      {visibleMessages.map((message) => (
        <div
          key={message.id}
          className={cn(
            "rounded-lg border p-3",
            message.role === "user" && "bg-surface",
            message.role === "assistant" && "bg-background",
            message.role === "system" && "bg-muted/40",
          )}
        >
          <div className="mb-2 flex items-center justify-between gap-2">
            <Badge
              variant={message.role === "assistant" ? "secondary" : "outline"}
            >
              {message.role}
            </Badge>
            <span className="text-muted-foreground text-xs">
              {formatRelativeAge(message.created_at)}
            </span>
          </div>
          <MarkdownMessage content={message.content} />
        </div>
      ))}
    </div>
  );
}

function followUpRuntimeEvents(events: ReviewEvent[], findingId: string) {
  return events.filter((event) => {
    if (!event.type.startsWith("AgentRun")) {
      return false;
    }
    return payloadString(event.payload.finding_id) === findingId;
  });
}

function payloadString(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

type EvidenceMapSelection =
  | { kind: "node"; id: string }
  | { kind: "edge"; id: string }
  | { kind: "call_path"; id: string };

interface PositionedEvidenceMapNode {
  node: EvidenceMapNode;
  x: number;
  y: number;
}

interface EvidenceMapLayout {
  nodes: PositionedEvidenceMapNode[];
  nodeById: Map<string, PositionedEvidenceMapNode>;
  width: number;
  height: number;
}

function EvidenceMapScreen({
  activeRepository,
  agentConfigs,
  client,
  events,
  finding,
  onBack,
}: {
  activeRepository?: Repository;
  agentConfigs: Loadable<AgentConfig[]>;
  client: ApiClient | null;
  events: ReviewEvent[];
  finding: Finding;
  onBack: () => void;
}) {
  const [mapState, setMapState] =
    useState<Loadable<EvidenceMapResponse>>(idleApiState());
  const [selection, setSelection] = useState<EvidenceMapSelection | null>(null);
  const [question, setQuestion] = useState("");
  const [askState, setAskState] =
    useState<Loadable<AskFindingQuestionResponse>>(idleApiState());
  const [actionMessage, setActionMessage] = useState("");
  const [isRebuilding, setIsRebuilding] = useState(false);
  const [isOpeningEditor, setIsOpeningEditor] = useState(false);
  const runtimeEvents = useMemo(
    () => followUpRuntimeEvents(events, finding.id),
    [events, finding.id],
  );

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      if (!client) {
        setMapState(errorApiState(new Error("Backend client is unavailable")));
        return;
      }
      setMapState(loadingApiState());
      setSelection(null);
      setActionMessage("");
      void loadApiResource(() => client.getFindingEvidenceMap(finding.id)).then(
        (state) => {
          if (canceled) {
            return;
          }
          setMapState(state);
          if (state.status === "success") {
            setSelection(firstEvidenceMapSelection(state.data));
          }
        },
      );
    });
    return () => {
      canceled = true;
    };
  }, [client, finding.id]);

  const map = mapState.status === "success" ? mapState.data : undefined;
  const agentList = agentConfigs.status === "success" ? agentConfigs.data : [];
  const verifierAgent =
    agentList.find(
      (agent) =>
        agent.enabled &&
        (agent.role.toLowerCase().includes("verifier") ||
          agent.adapter_kind === "local_verifier"),
    ) ?? agentList.find((agent) => agent.enabled);
  const selectedNode =
    map && selection?.kind === "node"
      ? map.nodes.find((node) => node.id === selection.id)
      : undefined;
  const selectedEdge =
    map && selection?.kind === "edge"
      ? map.edges.find((edge) => edge.id === selection.id)
      : undefined;
  const selectedCallPath =
    map && selection?.kind === "call_path"
      ? map.call_paths.find((path) => path.id === selection.id)
      : undefined;

  async function rebuildMap() {
    if (!client) {
      setActionMessage("Backend client is unavailable.");
      return;
    }
    setIsRebuilding(true);
    setActionMessage("");
    const state = await loadApiResource(() =>
      client.rebuildFindingEvidenceMap(finding.id),
    );
    setIsRebuilding(false);
    setMapState(state);
    if (state.status === "success") {
      setSelection(firstEvidenceMapSelection(state.data));
      setActionMessage("Evidence Map rebuilt");
      return;
    }
    setActionMessage(
      state.status === "error" ? state.error.message : "Rebuild failed",
    );
  }

  async function askVerifier() {
    if (!client || !question.trim()) {
      return;
    }
    setAskState(loadingApiState());
    const state = await loadApiResource(() =>
      client.askEvidenceMapQuestion(finding.id, {
        question: question.trim(),
        agent_config_id: verifierAgent?.id,
        graph_refs: evidenceMapGraphRefs(selection),
        context_policy: { max_tokens: 10_000, max_items: 120 },
      }),
    );
    setAskState(state);
    if (state.status === "success") {
      setQuestion("");
    }
  }

  async function openSelectedInEditor() {
    const target = evidenceMapOpenTarget(
      selectedNode,
      selectedCallPath,
      activeRepository,
    );
    if (!target) {
      setActionMessage("Select a node or call path step with a file path.");
      return;
    }
    if (!window.cocode?.openFile) {
      setActionMessage("Editor bridge is unavailable.");
      return;
    }
    setIsOpeningEditor(true);
    setActionMessage("");
    const state = await loadApiResource(() => window.cocode!.openFile(target));
    setIsOpeningEditor(false);
    setActionMessage(
      state.status === "success"
        ? "Opened in editor"
        : state.status === "error"
          ? state.error.message
          : "Open failed",
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="text-muted-foreground mb-2 flex items-center gap-2 text-xs">
            <span>Findings</span>
            <ChevronDownIcon className="size-3 -rotate-90" />
            <span className="text-foreground">Evidence Map</span>
          </div>
          <h2 className="text-2xl leading-8 font-semibold">Evidence Map</h2>
          <p className="text-muted-foreground mt-1 max-w-3xl truncate text-sm">
            {finding.canonical_claim}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="outline" onClick={onBack}>
            <ArrowLeftIcon data-icon="inline-start" />
            Findings
          </Button>
          <Button
            disabled={mapState.status !== "success" || isRebuilding}
            size="sm"
            variant="outline"
            onClick={() => void rebuildMap()}
          >
            <RefreshCwIcon data-icon="inline-start" />
            Rebuild
          </Button>
        </div>
      </div>

      {mapState.status === "loading" && (
        <LoadingRows rows={8} className="cocode-panel p-4" />
      )}
      {mapState.status === "error" && (
        <ErrorState
          title="Evidence Map failed to load"
          description={mapState.error.message}
        />
      )}
      {map && (
        <div className="cocode-panel grid min-h-[620px] overflow-hidden lg:grid-cols-[260px_minmax(0,1fr)] xl:grid-cols-[280px_minmax(0,1fr)_360px]">
          <EvidenceMapHierarchyPane
            hierarchy={map.hierarchy}
            selection={selection}
            onSelect={setSelection}
          />

          <div className="bg-background flex min-w-0 flex-col">
            <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <span className="mr-1 text-sm font-semibold">
                  Evidence flow
                </span>
                <Badge
                  variant={map.graph.status === "ready" ? "default" : "outline"}
                >
                  {map.graph.status}
                </Badge>
                <Badge variant="secondary">{map.nodes.length} nodes</Badge>
                <Badge variant="secondary">{map.edges.length} edges</Badge>
                {map.missing_reasons && map.missing_reasons.length > 0 && (
                  <Badge variant="outline">
                    {map.missing_reasons.length} missing
                  </Badge>
                )}
              </div>
              {actionMessage && (
                <span className="text-muted-foreground text-xs">
                  {actionMessage}
                </span>
              )}
            </div>

            {map.missing_reasons && map.missing_reasons.length > 0 && (
              <ErrorState
                className="m-4"
                title="Evidence Map is partial"
                description={map.missing_reasons.join(" ")}
              />
            )}

            <div className="min-h-0 flex-1 overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
              <EvidenceMapGraphCanvas
                map={map}
                selection={selection}
                onSelect={setSelection}
              />

              <details className="border-t bg-white" open>
                <summary className="text-muted-foreground flex cursor-pointer list-none items-center justify-between px-4 py-3 text-sm font-medium [&::-webkit-details-marker]:hidden">
                  <span>Signal summary</span>
                  <ChevronDownIcon className="size-4" />
                </summary>
                <EvidenceMapNarrativeFlow
                  map={map}
                  selection={selection}
                  onSelect={setSelection}
                />
              </details>

              <details className="border-t bg-white">
                <summary className="text-muted-foreground flex cursor-pointer list-none items-center justify-between px-4 py-3 text-sm font-medium [&::-webkit-details-marker]:hidden">
                  <span>Call path</span>
                  <Badge variant="outline">{map.call_paths.length} paths</Badge>
                </summary>
                <EvidenceMapCallPathPanel
                  map={map}
                  selection={selection}
                  onSelect={setSelection}
                />
              </details>
            </div>
          </div>

          <EvidenceMapRightPanel
            activeRepository={activeRepository}
            askState={askState}
            canAsk={Boolean(client && question.trim())}
            isOpeningEditor={isOpeningEditor}
            map={map}
            onAsk={() => void askVerifier()}
            onOpenEditor={() => void openSelectedInEditor()}
            question={question}
            runtimeEvents={runtimeEvents}
            selectedCallPath={selectedCallPath}
            selectedEdge={selectedEdge}
            selectedNode={selectedNode}
            selection={selection}
            setQuestion={setQuestion}
            verifierAgent={verifierAgent}
          />
        </div>
      )}
    </div>
  );
}

function EvidenceMapHierarchyPane({
  hierarchy,
  onSelect,
  selection,
}: {
  hierarchy: EvidenceMapHierarchyItem[];
  onSelect: (selection: EvidenceMapSelection) => void;
  selection: EvidenceMapSelection | null;
}) {
  return (
    <aside className="bg-surface/60 min-w-0 border-b lg:border-r lg:border-b-0">
      <div className="border-b px-4 py-3">
        <div className="text-base font-semibold">Code hierarchy</div>
        <div className="text-muted-foreground mt-1 text-xs">
          {hierarchy.length} locations
        </div>
      </div>
      <ScrollArea className="h-48 lg:h-[430px] xl:h-[590px]">
        <div className="flex flex-col gap-1 p-2">
          {hierarchy.map((item) => {
            const targetNodeId = item.node_ids[0];
            const selected =
              selection?.kind === "node" &&
              Boolean(targetNodeId) &&
              selection.id === targetNodeId;
            return (
              <button
                key={`${item.path}:${item.start_line ?? 0}:${item.kind}`}
                className={cn(
                  "hover:bg-surface-raised flex min-w-0 flex-col items-start rounded-md border border-transparent px-3 py-2 text-left text-sm transition-colors",
                  selected &&
                    "border-primary/30 bg-surface-raised shadow-[0_1px_2px_oklch(0.2_0.02_255/0.04)]",
                )}
                disabled={!targetNodeId}
                type="button"
                onClick={() => {
                  if (targetNodeId) {
                    onSelect({ kind: "node", id: targetNodeId });
                  }
                }}
              >
                <span className="w-full truncate font-medium">
                  {shortPath(item.path)}
                </span>
                <span className="text-muted-foreground mt-1 flex max-w-full items-center gap-2 text-xs">
                  <span>{item.kind.replaceAll("_", " ")}</span>
                  {item.start_line ? (
                    <span>
                      L{formatLineRange(item.start_line, item.end_line)}
                    </span>
                  ) : null}
                </span>
              </button>
            );
          })}
          {hierarchy.length === 0 && (
            <EmptyState
              className="border-0 px-3 py-8"
              title="No hierarchy"
              description="No code locations are available for this graph."
              icon={FileSearchIcon}
            />
          )}
        </div>
      </ScrollArea>
    </aside>
  );
}

function EvidenceMapNarrativeFlow({
  map,
  onSelect,
  selection,
}: {
  map: EvidenceMapResponse;
  onSelect: (selection: EvidenceMapSelection) => void;
  selection: EvidenceMapSelection | null;
}) {
  const story = useMemo(() => evidenceMapNarrativeStory(map), [map]);

  return (
    <div className="min-h-[420px] min-w-0 flex-1 overflow-auto bg-[#fbfbfa] p-4 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
      <div className="grid min-w-0 gap-3 xl:grid-cols-[minmax(0,0.95fr)_minmax(0,1.05fr)_minmax(0,0.95fr)]">
        <EvidenceMapFlowColumn
          caption="Step 1"
          description="The changed location that grounds the finding."
          nodes={story.changed}
          onSelect={onSelect}
          selection={selection}
          title="Changed code"
          tone="green"
        />
        <EvidenceMapClaimColumn
          finding={map.finding}
          summary={story.claimSummary}
        />
        <EvidenceMapFlowColumn
          caption="Step 3"
          description="Signals that support, test, or challenge the claim."
          nodes={story.checks}
          onSelect={onSelect}
          selection={selection}
          title="Evidence checks"
          tone="amber"
        />
      </div>

      <div className="mt-4 grid gap-3 md:grid-cols-3">
        <EvidenceMapInsightCard
          label="Confidence"
          value={`${Math.round(map.finding.confidence * 100)}%`}
          detail={`${formatDecisionLabel(map.finding.severity)} severity`}
        />
        <EvidenceMapInsightCard
          label="Evidence"
          value={`${story.supportingCount}`}
          detail="supporting signal(s)"
        />
        <EvidenceMapInsightCard
          label="Counter checks"
          value={`${story.counterCount}`}
          detail="test or counter signal(s)"
        />
      </div>

      {story.omittedNodes > 0 && (
        <p className="text-muted-foreground mt-3 text-xs">
          Grouped {story.omittedNodes} lower-signal duplicate node
          {story.omittedNodes === 1 ? "" : "s"} so the flow stays readable.
        </p>
      )}
    </div>
  );
}

function EvidenceMapFlowColumn({
  caption,
  description,
  nodes,
  onSelect,
  selection,
  title,
  tone,
}: {
  caption: string;
  description: string;
  nodes: EvidenceMapNode[];
  onSelect: (selection: EvidenceMapSelection) => void;
  selection: EvidenceMapSelection | null;
  title: string;
  tone: "amber" | "green";
}) {
  return (
    <section className="border-border/70 flex min-h-[280px] flex-col rounded-xl border bg-white p-3 shadow-[0_1px_2px_rgb(17_18_20/0.03)]">
      <div className="text-muted-foreground text-[0.7rem] font-medium tracking-wide uppercase">
        {caption}
      </div>
      <div className="mt-1 text-base font-semibold">{title}</div>
      <p className="text-muted-foreground mt-1 text-xs leading-5">
        {description}
      </p>
      <div className="mt-3 flex flex-1 flex-col gap-2">
        {nodes.map((node) => (
          <EvidenceMapFlowNode
            key={node.id}
            node={node}
            selected={selection?.kind === "node" && selection.id === node.id}
            tone={tone}
            onSelect={() => onSelect({ kind: "node", id: node.id })}
          />
        ))}
        {nodes.length === 0 && (
          <div className="text-muted-foreground flex flex-1 items-center justify-center rounded-lg border border-dashed p-4 text-center text-xs leading-5">
            No focused node is available for this step.
          </div>
        )}
      </div>
    </section>
  );
}

function EvidenceMapClaimColumn({
  finding,
  summary,
}: {
  finding: EvidenceMapFinding;
  summary: string;
}) {
  return (
    <section className="border-foreground/15 flex min-h-[280px] flex-col rounded-xl border bg-white p-4 shadow-[0_1px_2px_rgb(17_18_20/0.03)]">
      <div className="text-muted-foreground text-[0.7rem] font-medium tracking-wide uppercase">
        Step 2
      </div>
      <div className="mt-1 text-base font-semibold">Finding claim</div>
      <p className="mt-3 text-[0.92rem] leading-6 font-semibold">
        {finding.canonical_claim}
      </p>
      <p className="text-muted-foreground mt-3 text-xs leading-5">
        {summary ||
          "This is the merged reviewer claim. The surrounding cards show the source and the checks that make it credible."}
      </p>
      <div className="mt-auto flex flex-wrap gap-2 pt-4">
        <Badge variant="default">{formatDecisionLabel(finding.severity)}</Badge>
        <Badge variant="secondary">
          {formatDecisionLabel(finding.verification_status)}
        </Badge>
      </div>
    </section>
  );
}

function EvidenceMapFlowNode({
  node,
  onSelect,
  selected,
  tone,
}: {
  node: EvidenceMapNode;
  onSelect: () => void;
  selected: boolean;
  tone: "amber" | "green";
}) {
  return (
    <button
      className={cn(
        "flex min-w-0 cursor-pointer flex-col items-start rounded-lg border px-3 py-2 text-left transition-colors",
        tone === "green" &&
          "border-emerald-200 bg-emerald-50/55 hover:bg-emerald-50",
        tone === "amber" && "border-amber-200 bg-amber-50/55 hover:bg-amber-50",
        selected &&
          "border-foreground bg-white shadow-[0_1px_3px_rgb(17_18_20/0.12)]",
      )}
      type="button"
      onClick={onSelect}
    >
      <span className="line-clamp-2 text-[0.82rem] leading-5 font-semibold">
        {evidenceMapReadableNodeLabel(node)}
      </span>
      <span className="text-muted-foreground mt-1 line-clamp-1 font-mono text-[0.68rem]">
        {formatEvidenceNodeLocation(node)}
      </span>
      <span className="text-muted-foreground mt-2 flex w-full items-center justify-between gap-2 text-[0.7rem]">
        <span>{node.kind.replaceAll("_", " ")}</span>
        <span>{Math.round(node.confidence * 100)}%</span>
      </span>
    </button>
  );
}

function EvidenceMapInsightCard({
  detail,
  label,
  value,
}: {
  detail: string;
  label: string;
  value: string;
}) {
  return (
    <div className="border-border/70 rounded-lg border bg-white px-3 py-2.5">
      <div className="text-muted-foreground text-xs">{label}</div>
      <div className="mt-1 text-lg font-semibold">{value}</div>
      <div className="text-muted-foreground mt-0.5 text-xs">{detail}</div>
    </div>
  );
}

export function EvidenceMapGraphCanvas({
  map,
  onSelect,
  selection,
}: {
  map: EvidenceMapResponse;
  onSelect: (selection: EvidenceMapSelection) => void;
  selection: EvidenceMapSelection | null;
}) {
  const focusedMap = useMemo(() => focusedEvidenceMap(map), [map]);
  const layout = useMemo(
    () => buildEvidenceMapLayout(focusedMap),
    [focusedMap],
  );

  if (focusedMap.nodes.length === 0) {
    return (
      <div className="bg-surface/30 flex min-h-[360px] min-w-0 flex-1 items-center justify-center">
        <EmptyState
          className="border-0"
          title="No graph nodes"
          description="The graph builder did not return renderable nodes for this finding."
          icon={MapIcon}
        />
      </div>
    );
  }

  return (
    <div className="evidence-map-canvas min-h-[420px] min-w-0 flex-1 overflow-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
      <svg
        aria-label="Evidence Map graph"
        className="mx-auto min-h-[420px]"
        height={layout.height}
        role="img"
        viewBox={`0 0 ${layout.width} ${layout.height}`}
        width={layout.width}
      >
        <defs>
          <filter
            id="evidence-map-node-shadow"
            x="-12%"
            y="-24%"
            width="124%"
            height="148%"
          >
            <feDropShadow
              dx="0"
              dy="8"
              floodColor="oklch(0.2 0.02 255)"
              floodOpacity="0.08"
              stdDeviation="7"
            />
          </filter>
          <marker
            id="evidence-map-arrow"
            markerHeight="7"
            markerWidth="7"
            orient="auto"
            refX="6"
            refY="3.5"
          >
            <path d="M0,0 L7,3.5 L0,7 Z" className="fill-muted-foreground" />
          </marker>
        </defs>
        {[
          { label: "Changed code", x: 64 },
          { label: "Finding", x: 64 + EVIDENCE_MAP_COLUMN_GAP },
          { label: "Checks", x: 64 + EVIDENCE_MAP_COLUMN_GAP * 2 },
        ].map((heading) => (
          <text
            key={heading.label}
            className="fill-muted-foreground text-[11px] font-semibold"
            x={heading.x}
            y="28"
          >
            {heading.label}
          </text>
        ))}
        {focusedMap.edges.map((edge) => {
          const source = layout.nodeById.get(edge.source);
          const target = layout.nodeById.get(edge.target);
          if (!source || !target) {
            return null;
          }
          const selected =
            selection?.kind === "edge" && selection.id === edge.id;
          const sameColumn = Math.abs(source.x - target.x) < 12;
          const sourceX = sameColumn
            ? source.x + EVIDENCE_MAP_NODE_WIDTH / 2
            : source.x + EVIDENCE_MAP_NODE_WIDTH;
          const sourceY = sameColumn
            ? source.y + EVIDENCE_MAP_NODE_HEIGHT
            : source.y + EVIDENCE_MAP_NODE_HEIGHT / 2;
          const targetX = sameColumn
            ? target.x + EVIDENCE_MAP_NODE_WIDTH / 2
            : target.x;
          const targetY = sameColumn
            ? target.y
            : target.y + EVIDENCE_MAP_NODE_HEIGHT / 2;
          const control = Math.max(76, Math.abs(targetX - sourceX) / 2);
          const path = sameColumn
            ? `M ${sourceX} ${sourceY} L ${targetX} ${targetY}`
            : `M ${sourceX} ${sourceY} C ${sourceX + control} ${sourceY}, ${
                targetX - control
              } ${targetY}, ${targetX} ${targetY}`;
          return (
            <g
              key={edge.id}
              className="cursor-pointer"
              role="button"
              tabIndex={0}
              onClick={() => onSelect({ kind: "edge", id: edge.id })}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  onSelect({ kind: "edge", id: edge.id });
                }
              }}
            >
              <path
                className={cn(
                  "stroke-muted-foreground/45 fill-none",
                  selected && "stroke-primary",
                  edge.status === "missing" && "stroke-destructive",
                )}
                d={path}
                markerEnd="url(#evidence-map-arrow)"
                strokeDasharray={edge.status === "missing" ? "7 5" : undefined}
                strokeWidth={selected ? 2.6 : 1.6}
              />
              {edge.label && (
                <text
                  className={cn(
                    "fill-muted-foreground text-[11px]",
                    edge.status === "missing" && "fill-destructive",
                  )}
                  x={(sourceX + targetX) / 2}
                  y={(sourceY + targetY) / 2 - 8}
                >
                  {truncate(edge.label, 28)}
                </text>
              )}
              {edge.status === "missing" && (
                <text
                  className="fill-destructive text-[22px] font-semibold"
                  x={(sourceX + targetX) / 2}
                  y={(sourceY + targetY) / 2 + 7}
                >
                  x
                </text>
              )}
            </g>
          );
        })}

        {layout.nodes.map(({ node, x, y }) => {
          const selected =
            selection?.kind === "node" && selection.id === node.id;
          const label = evidenceMapReadableNodeLabel(node);
          const labelLines = wrapSvgLabel(label, 24).slice(0, 2);
          const style = evidenceMapNodeStyle(node.kind);
          return (
            <g
              key={node.id}
              className="cursor-pointer"
              role="button"
              tabIndex={0}
              transform={`translate(${x}, ${y})`}
              onClick={() => onSelect({ kind: "node", id: node.id })}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  onSelect({ kind: "node", id: node.id });
                }
              }}
            >
              <title>{label}</title>
              <rect
                className={cn(style.surface, selected && style.selected)}
                height={EVIDENCE_MAP_NODE_HEIGHT}
                filter="url(#evidence-map-node-shadow)"
                rx="8"
                strokeWidth={selected ? 2.4 : 1.2}
                width={EVIDENCE_MAP_NODE_WIDTH}
              />
              <rect
                className={style.bar}
                height="4"
                rx="1.5"
                width={EVIDENCE_MAP_NODE_WIDTH - 22}
                x="11"
                y="9"
              />
              <text className="fill-foreground text-[12px] font-semibold">
                {labelLines.map((line, index) => (
                  <tspan key={`${node.id}:${index}`} x="12" y={31 + index * 15}>
                    {line}
                  </tspan>
                ))}
              </text>
              <text className="fill-muted-foreground text-[10px]">
                <tspan x="12" y="72">
                  {truncate(evidenceMapNodeMeta(node), 25)}
                </tspan>
                <tspan x="178" y="72">
                  {Math.round(node.confidence * 100)}%
                </tspan>
              </text>
            </g>
          );
        })}
      </svg>
    </div>
  );
}

function EvidenceMapCallPathPanel({
  map,
  onSelect,
  selection,
}: {
  map: EvidenceMapResponse;
  onSelect: (selection: EvidenceMapSelection) => void;
  selection: EvidenceMapSelection | null;
}) {
  return (
    <div className="bg-surface/35 border-t px-4 py-3">
      <div className="mb-2 flex items-center justify-between gap-2">
        <div className="text-sm font-semibold">Call path</div>
        <Badge variant="outline">{map.call_paths.length} paths</Badge>
      </div>
      {map.call_paths.length > 0 ? (
        <div className="flex gap-2 overflow-x-auto pb-1">
          {map.call_paths.map((path) => (
            <button
              key={path.id}
              className={cn(
                "hover:bg-surface-raised bg-background flex min-w-[240px] flex-col items-start rounded-md border px-3 py-2.5 text-left transition-colors",
                selection?.kind === "call_path" &&
                  selection.id === path.id &&
                  "border-primary bg-primary/[0.03]",
              )}
              type="button"
              onClick={() => onSelect({ kind: "call_path", id: path.id })}
            >
              <span className="truncate text-sm font-medium">
                {path.label || "Evidence path"}
              </span>
              <span className="text-muted-foreground mt-1 text-xs">
                {path.steps.length} steps / {Math.round(path.confidence * 100)}%
              </span>
              <span className="text-muted-foreground mt-2 line-clamp-2 text-xs">
                {path.steps.map((step) => step.label).join(" -> ")}
              </span>
            </button>
          ))}
        </div>
      ) : (
        <EmptyState
          className="border-0 p-2"
          title="No call path"
          description={
            map.call_path_unavailable_reason ||
            "No readable call path is available."
          }
          icon={MapIcon}
        />
      )}
    </div>
  );
}

function EvidenceMapRightPanel({
  activeRepository,
  askState,
  canAsk,
  isOpeningEditor,
  map,
  onAsk,
  onOpenEditor,
  question,
  runtimeEvents,
  selectedCallPath,
  selectedEdge,
  selectedNode,
  selection,
  setQuestion,
  verifierAgent,
}: {
  activeRepository?: Repository;
  askState: Loadable<AskFindingQuestionResponse>;
  canAsk: boolean;
  isOpeningEditor: boolean;
  map: EvidenceMapResponse;
  onAsk: () => void;
  onOpenEditor: () => void;
  question: string;
  runtimeEvents: ReviewEvent[];
  selectedCallPath?: EvidenceMapCallPath;
  selectedEdge?: EvidenceMapEdge;
  selectedNode?: EvidenceMapNode;
  selection: EvidenceMapSelection | null;
  setQuestion: (question: string) => void;
  verifierAgent?: AgentConfig;
}) {
  const openTarget = evidenceMapOpenTarget(
    selectedNode,
    selectedCallPath,
    activeRepository,
  );

  return (
    <aside className="bg-surface/60 min-w-0 overflow-hidden border-t lg:col-span-2 xl:col-span-1 xl:border-t-0 xl:border-l">
      <ScrollArea className="h-96 xl:h-[680px]">
        <div className="flex flex-col gap-4 p-4">
          <div className="bg-background min-w-0 rounded-md border p-3">
            <div className="mb-2 text-sm font-semibold">Why this matters</div>
            <p className="text-muted-foreground text-sm leading-6 break-words">
              {map.panel.evidence_summary ||
                map.graph.summary ||
                `Evidence map for "${map.panel.claim}" with ${map.nodes.length} node(s) and ${map.edges.length} edge(s).`}
            </p>
            <div className="mt-4 border-t pt-4">
              <div className="mb-2 text-sm font-semibold">
                Evidence highlights
              </div>
              <div className="flex flex-col gap-2">
                {map.panel.evidence.slice(0, 4).map((item) => (
                  <div
                    key={item.id}
                    className="grid grid-cols-[18px_minmax(0,1fr)] gap-2 text-sm"
                  >
                    <CheckIcon className="text-success mt-0.5 size-3.5" />
                    <div className="min-w-0">
                      <div className="line-clamp-1 font-medium">
                        {item.title}
                      </div>
                      <div className="text-muted-foreground mt-0.5 truncate text-xs">
                        {formatEvidenceRefLocation(item)}
                      </div>
                    </div>
                  </div>
                ))}
                {map.panel.evidence.length === 0 && (
                  <p className="text-muted-foreground text-sm">
                    No stored evidence highlights yet.
                  </p>
                )}
              </div>
            </div>
            <div className="mt-4 border-t pt-4">
              <div className="mb-2 text-sm font-semibold">Interpretation</div>
              <p className="text-muted-foreground text-sm leading-6 break-words">
                {map.graph.summary ||
                  "Follow the solid path for reachable code and the dashed red edge for the missing or disputed guard."}
              </p>
            </div>
            <div className="mt-4 border-t pt-4">
              <div className="mb-2 text-sm font-semibold">
                Suggested remediation
              </div>
              <p className="text-muted-foreground text-sm leading-6 break-words">
                {map.panel.suggested_fix ||
                  map.finding.suggested_fix ||
                  "Review the selected path, restore the missing guard when the path is reachable, or document the existing control that makes the finding safe."}
              </p>
            </div>
          </div>

          <div className="bg-background min-w-0 rounded-md border p-3">
            <div className="mb-2 text-sm font-semibold">Selected context</div>
            <SelectedEvidenceMapDetail
              edge={selectedEdge}
              node={selectedNode}
              callPath={selectedCallPath}
              selection={selection}
            />
            <Button
              className="mt-3 w-full justify-start"
              disabled={!openTarget || isOpeningEditor}
              size="sm"
              variant="outline"
              onClick={onOpenEditor}
            >
              <ExternalLinkIcon data-icon="inline-start" />
              Open in editor
            </Button>
          </div>

          <div className="bg-background min-w-0 rounded-md border p-3">
            <div className="mb-2 flex items-center justify-between gap-2">
              <div className="text-sm font-semibold">Ask verifier</div>
              <Badge className="max-w-32 truncate" variant="outline">
                {verifierAgent?.name ?? "auto-select"}
              </Badge>
            </div>
            <Textarea
              aria-label="Ask verifier about Evidence Map"
              className="min-h-24"
              placeholder="Ask about the selected graph path..."
              value={question}
              onChange={(event) => setQuestion(event.target.value)}
            />
            <Button
              className="mt-2 w-full justify-start"
              disabled={!canAsk || askState.status === "loading"}
              size="sm"
              onClick={onAsk}
            >
              <SendIcon data-icon="inline-start" />
              Ask verifier
            </Button>
            {askState.status === "error" && (
              <p className="text-destructive mt-2 text-sm">
                {askState.error.message}
              </p>
            )}
            {askState.status === "success" && (
              <div className="bg-surface/55 mt-3 rounded-md border p-3">
                <div className="text-xs font-medium">Verifier response</div>
                <div className="text-muted-foreground mt-2">
                  <MarkdownMessage
                    content={askState.data.assistant_message.content}
                  />
                </div>
              </div>
            )}
            <AgentRuntimeTrace
              events={runtimeEvents}
              loading={askState.status === "loading"}
              compact
            />
          </div>

          <div className="bg-background min-w-0 rounded-md border p-3">
            <EvidenceMapLegend map={map} />
          </div>

          {map.panel.evidence.length > 0 && (
            <div className="bg-background min-w-0 rounded-md border p-3">
              <div className="mb-2 text-sm font-semibold">Evidence bundle</div>
              <div className="flex flex-col gap-2">
                {map.panel.evidence.slice(0, 8).map((item) => (
                  <div
                    key={item.id}
                    className="bg-surface/35 rounded-md border p-3"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="truncate text-sm font-medium">
                        {item.title}
                      </span>
                      <Badge variant={evidenceBadgeVariant(item.kind)}>
                        {item.kind}
                      </Badge>
                    </div>
                    <div className="text-muted-foreground mt-1 text-xs">
                      {formatEvidenceRefLocation(item)}
                    </div>
                    <p className="text-muted-foreground mt-2 line-clamp-3 text-sm leading-6">
                      {item.summary}
                    </p>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </ScrollArea>
    </aside>
  );
}

function SelectedEvidenceMapDetail({
  callPath,
  edge,
  node,
  selection,
}: {
  callPath?: EvidenceMapCallPath;
  edge?: EvidenceMapEdge;
  node?: EvidenceMapNode;
  selection: EvidenceMapSelection | null;
}) {
  if (node) {
    return (
      <div className="bg-surface/35 min-w-0 rounded-md border p-3">
        <div className="flex items-center justify-between gap-2">
          <span className="min-w-0 truncate text-sm font-medium">
            {node.label}
          </span>
          <Badge className="max-w-28 shrink-0 truncate" variant="outline">
            {node.kind.replaceAll("_", " ")}
          </Badge>
        </div>
        <div className="text-muted-foreground mt-2 text-xs">
          {formatEvidenceNodeLocation(node)}
        </div>
        {node.symbol && (
          <div className="text-muted-foreground mt-2 text-sm">
            Symbol: {node.symbol}
          </div>
        )}
        <div className="text-muted-foreground mt-2 text-sm">
          Confidence {Math.round(node.confidence * 100)}%
        </div>
      </div>
    );
  }

  if (edge) {
    return (
      <div className="bg-surface/35 min-w-0 rounded-md border p-3">
        <div className="flex items-center justify-between gap-2">
          <span className="min-w-0 truncate text-sm font-medium">
            {edge.label || edge.kind.replaceAll("_", " ")}
          </span>
          <Badge
            variant={edge.status === "missing" ? "destructive" : "outline"}
          >
            {edge.status}
          </Badge>
        </div>
        <div className="text-muted-foreground mt-2 text-sm leading-6">
          {edge.source} {"->"} {edge.target}
        </div>
        <div className="text-muted-foreground mt-2 text-sm">
          Confidence {Math.round(edge.confidence * 100)}%
        </div>
      </div>
    );
  }

  if (callPath) {
    return (
      <div className="bg-surface/35 min-w-0 rounded-md border p-3">
        <div className="text-sm font-medium break-words">
          {callPath.label || "Evidence path"}
        </div>
        <div className="text-muted-foreground mt-2 text-sm">
          Confidence {Math.round(callPath.confidence * 100)}%
        </div>
        <div className="mt-3 flex flex-col gap-2">
          {callPath.steps.map((step) => (
            <div key={`${step.step_index}:${step.label}`} className="text-sm">
              <span className="text-muted-foreground mr-2">
                {step.step_index + 1}.
              </span>
              {step.label}
              {step.path && (
                <div className="text-muted-foreground ml-6 text-xs">
                  {formatCallPathStepLocation(step)}
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="text-muted-foreground bg-surface/35 rounded-md border p-3 text-sm">
      {selection
        ? "The selected graph item is no longer available."
        : "Select a node, edge, or call path to inspect it."}
    </div>
  );
}

function EvidenceMapLegend({ map }: { map: EvidenceMapResponse }) {
  return (
    <div>
      <div className="mb-2 text-sm font-semibold">Legend</div>
      <div className="flex flex-col gap-2">
        {map.legend.map((item) => (
          <div key={item.kind} className="bg-surface/35 rounded-md border p-3">
            <div className="text-sm font-medium">{item.label}</div>
            <p className="text-muted-foreground mt-1 text-xs leading-5">
              {item.description}
            </p>
          </div>
        ))}
        {map.legend.length === 0 && (
          <EmptyState
            className="border-0 p-2"
            title="No legend"
            description="No legend entries are available for this graph."
            icon={MapIcon}
          />
        )}
      </div>
    </div>
  );
}

export function FindingCard({
  actionState,
  finding,
  onAccept,
  onCopy,
  onOpenDetail,
  onSelect,
  selected,
}: {
  actionState: {
    status: "idle" | "loading" | "success" | "error";
    findingId?: string;
    action?: string;
  };
  finding: Finding;
  onAccept: () => void;
  onCopy: () => void;
  onOpenDetail: () => void;
  onSelect: () => void;
  selected: boolean;
}) {
  const pending =
    actionState.status === "loading" && actionState.findingId === finding.id;
  const confidence = `${Math.round(finding.confidence * 100)}%`;
  const sourceAgents = finding.source_agents ?? [];
  return (
    <div
      className={cn(
        "grid w-full cursor-pointer grid-cols-1 gap-3 border-b border-l-2 border-l-transparent px-4 py-3 text-left transition-colors last:border-b-0 hover:bg-[#fbfbfa] lg:grid-cols-[88px_minmax(0,1.45fr)_minmax(118px,0.75fr)_112px_140px_110px]",
        selected && "border-l-foreground bg-[#f7f7f5]",
      )}
      aria-selected={selected}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onSelect();
        }
      }}
      role="button"
      tabIndex={0}
    >
      <div className="flex min-w-0 items-start gap-2 lg:block">
        <Badge
          className="shrink-0"
          variant={
            finding.severity === "high" || finding.severity === "blocker"
              ? "destructive"
              : finding.severity === "medium"
                ? "secondary"
                : "outline"
          }
        >
          {finding.severity}
        </Badge>
      </div>
      <div className="min-w-0">
        <button
          className="focus-visible:ring-ring line-clamp-1 cursor-pointer rounded-sm text-left text-sm font-semibold underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none"
          type="button"
          onClick={(event) => {
            event.stopPropagation();
            onOpenDetail();
          }}
        >
          {finding.canonical_claim}
        </button>
        {finding.evidence_summary && (
          <div className="text-muted-foreground mt-1 line-clamp-2 text-xs leading-5">
            {finding.evidence_summary}
          </div>
        )}
        <div className="mt-2 flex flex-wrap gap-1 lg:hidden">
          <Button
            disabled={pending}
            size="sm"
            variant="ghost"
            onClick={(event) => {
              event.stopPropagation();
              onAccept();
            }}
          >
            <CheckIcon data-icon="inline-start" />
            Accept
          </Button>
          <Button
            disabled={pending}
            size="sm"
            variant="ghost"
            onClick={(event) => {
              event.stopPropagation();
              onCopy();
            }}
          >
            <CopyIcon data-icon="inline-start" />
            Copy
          </Button>
        </div>
      </div>
      <div className="text-muted-foreground min-w-0 text-xs lg:pt-0.5">
        <div className="truncate font-mono">
          {finding.primary_path || "no location"}
        </div>
        {finding.primary_start_line ? (
          <div className="mt-1">L{finding.primary_start_line}</div>
        ) : null}
      </div>
      <div className="flex min-w-0 items-start lg:pt-0.5">
        <Badge variant={findingStatusBadgeVariant(finding)}>
          {findingWorkflowStatusLabel(finding)}
        </Badge>
      </div>
      <div className="min-w-0 lg:pt-0.5">
        {sourceAgents.length > 0 ? (
          <div className="flex min-w-0 flex-col gap-1">
            <div className="line-clamp-1 text-xs font-medium">
              {sourceAgentSummary(sourceAgents)}
            </div>
            <div className="text-muted-foreground line-clamp-1 text-[11px]">
              {sourceAgents.length} signal
              {sourceAgents.length === 1 ? "" : "s"}
            </div>
          </div>
        ) : (
          <span className="text-muted-foreground text-xs">No agent source</span>
        )}
      </div>
      <div className="flex min-w-0 items-start justify-between gap-2 lg:justify-end">
        <div className="text-muted-foreground pt-1 text-xs tabular-nums">
          {confidence}
        </div>
        <div className="hidden shrink-0 items-center gap-1 lg:flex">
          <Button
            aria-label={`Accept ${finding.canonical_claim}`}
            disabled={pending}
            size="icon-sm"
            variant="ghost"
            onClick={(event) => {
              event.stopPropagation();
              onAccept();
            }}
          >
            <CheckIcon />
          </Button>
          <Button
            aria-label={`Copy draft comment for ${finding.canonical_claim}`}
            disabled={pending}
            size="icon-sm"
            variant="ghost"
            onClick={(event) => {
              event.stopPropagation();
              onCopy();
            }}
          >
            <CopyIcon />
          </Button>
        </div>
      </div>
    </div>
  );
}

function sourceAgentSummary(sources: FindingSourceAgent[]) {
  const labels = sources
    .map((source) => {
      const model = source.model_label?.trim();
      const name = source.name?.trim() || source.agent_config_id || "Reviewer";
      return model && !name.toLowerCase().includes(model.toLowerCase())
        ? `${name} · ${model}`
        : name;
    })
    .filter(Boolean);
  if (labels.length <= 2) {
    return labels.join(", ");
  }
  return `${labels.slice(0, 2).join(", ")} +${labels.length - 2}`;
}

function findingWorkflowStatusLabel(finding: Finding) {
  if (finding.decision_status) {
    return formatDecisionLabel(finding.decision_status);
  }
  return "Needs Triage";
}

function findingStatusBadgeVariant(
  finding: Finding,
): "default" | "secondary" | "outline" | "destructive" {
  if (["accepted", "published", "copied"].includes(finding.decision_status)) {
    return "default";
  }
  if (finding.decision_status === "dismissed") {
    return "secondary";
  }
  if (
    ["likely_false_positive", "duplicate", "not_actionable"].includes(
      finding.verification_status,
    )
  ) {
    return "secondary";
  }
  if (
    finding.verification_status === "needs_human" ||
    finding.verification_status === "unverified"
  ) {
    return "outline";
  }
  return "outline";
}

function AgentConsensusPanel({
  detail,
  finding,
}: {
  detail?: FindingDetailResponse;
  finding: Finding;
}) {
  const candidates = detail?.candidates ?? [];
  const aligned = candidates.filter(
    (candidate) =>
      candidate.severity === finding.severity &&
      candidate.category === finding.category,
  ).length;
  const total = candidates.length || finding.merged_from_count || 1;
  const agreement = total > 0 ? Math.round((aligned / total) * 100) : 0;

  return (
    <div className="rounded-md border p-3">
      <div className="mb-3 flex items-center justify-between gap-2">
        <div className="text-xs font-medium">Agent consensus</div>
        <Badge variant="outline">{total} signals</Badge>
      </div>
      <div className="grid grid-cols-3 gap-2">
        <RunMetric label="Agreement" value={`${agreement}%`} />
        <RunMetric
          label="Severity"
          value={formatDecisionLabel(finding.severity)}
        />
        <RunMetric
          label="Confidence"
          value={`${Math.round(finding.confidence * 100)}%`}
        />
      </div>
      {candidates.length > 0 && (
        <div className="mt-3 flex flex-col gap-2">
          {candidates.slice(0, 3).map((candidate) => (
            <div
              key={candidate.id}
              className="bg-surface flex min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-xs"
            >
              <CircleIcon
                className={cn(
                  "size-3",
                  candidate.severity === finding.severity
                    ? "text-success"
                    : "text-warning",
                )}
              />
              <span className="truncate font-medium">
                {candidate.agent_name || candidate.agent_run_id}
              </span>
              <Badge variant="secondary">{candidate.severity}</Badge>
              <span className="text-muted-foreground">
                {Math.round(candidate.confidence * 100)}%
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function CodeSnippetViewer({
  evidence,
  finding,
  onCopyPath,
}: {
  evidence: EvidenceItem[];
  finding: Finding;
  onCopyPath: () => void;
}) {
  const snippets = prioritizedEvidenceItems(evidence)
    .filter((item) => item.code_snippet && item.code_snippet.trim() !== "")
    .slice(0, 3);

  if (snippets.length === 0) {
    const lineNumber = finding.primary_start_line || 1;
    return (
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-2">
          <div className="text-xs font-medium">Changed code</div>
          <Button size="sm" variant="outline" onClick={onCopyPath}>
            <CopyIcon data-icon="inline-start" />
            Copy path
          </Button>
        </div>
        <div className="border-border/70 overflow-hidden rounded-lg border bg-white shadow-[0_1px_2px_rgb(17_18_20/0.03)]">
          <div className="border-border/60 flex items-center justify-between gap-2 border-b bg-[#fbfbfa] px-3 py-2">
            <span className="truncate font-mono text-xs">
              {finding.primary_path || formatFindingLocation(finding)}
            </span>
            <Badge variant="outline">location</Badge>
          </div>
          <div className="overflow-auto bg-white font-mono [scrollbar-gutter:stable_both-edges]">
            <div className="grid w-max min-w-full auto-rows-min grid-cols-[52px_minmax(520px,max-content)]">
              <span className="border-border/60 text-muted-foreground sticky top-0 z-[1] border-b bg-[#fbfbfa] px-2 py-1.5 text-right text-[0.64rem] font-medium tracking-[0.02em] uppercase">
                Line
              </span>
              <span className="border-border/60 text-muted-foreground sticky top-0 z-[1] border-b bg-[#fbfbfa] px-2 py-1.5 text-[0.64rem] font-medium tracking-[0.02em] uppercase">
                Code
              </span>
              <span className="text-muted-foreground/75 border-border/40 border-b pr-3 text-right text-[0.72rem] leading-6 select-none">
                {lineNumber}
              </span>
              <code className="border-border/40 border-b bg-amber-50/55 px-3 text-[0.72rem] leading-6 whitespace-pre">
                Code snippet unavailable. {finding.evidence_summary}
              </code>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        <div className="text-xs font-medium">Changed code</div>
        <Button size="sm" variant="outline" onClick={onCopyPath}>
          <CopyIcon data-icon="inline-start" />
          Copy path
        </Button>
      </div>
      {snippets.map((item) => (
        <div
          key={item.id}
          className="border-border/70 overflow-hidden rounded-lg border bg-white shadow-[0_1px_2px_rgb(17_18_20/0.03)]"
        >
          <div className="border-border/60 flex items-center justify-between gap-2 border-b bg-[#fbfbfa] px-3 py-2">
            <span className="truncate font-mono text-xs">
              {item.path || formatFindingLocation(finding)}
            </span>
            <Badge variant="outline">{item.kind}</Badge>
          </div>
          <div className="max-h-[420px] overflow-auto bg-white font-mono [scrollbar-gutter:stable_both-edges]">
            <div className="grid w-max min-w-full auto-rows-min grid-cols-[52px_minmax(520px,max-content)]">
              <span className="border-border/60 text-muted-foreground sticky top-0 z-[1] border-b bg-[#fbfbfa] px-2 py-1.5 text-right text-[0.64rem] font-medium tracking-[0.02em] uppercase">
                Line
              </span>
              <span className="border-border/60 text-muted-foreground sticky top-0 z-[1] border-b bg-[#fbfbfa] px-2 py-1.5 text-[0.64rem] font-medium tracking-[0.02em] uppercase">
                Code
              </span>
              {snippetLines(item).map((line) => (
                <Fragment key={`${item.id}-${line.number}`}>
                  <span
                    className={cn(
                      "text-muted-foreground/75 border-border/40 border-b pr-3 text-right text-[0.72rem] leading-6 select-none",
                      snippetLineTone(item, finding, line.number, "number"),
                    )}
                  >
                    {line.number}
                  </span>
                  <code
                    className={cn(
                      "border-border/40 border-b px-3 text-[0.72rem] leading-6 whitespace-pre",
                      snippetLineTone(item, finding, line.number, "code"),
                    )}
                  >
                    {line.text || " "}
                  </code>
                </Fragment>
              ))}
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

export function EvidenceCardList({
  detail,
}: {
  detail?: FindingDetailResponse;
}) {
  if (!detail) {
    return <LoadingRows rows={4} />;
  }
  const items = prioritizedEvidenceItems(detail.evidence_items);
  if (items.length === 0) {
    return (
      <EmptyState
        className="border-0"
        title="No evidence yet"
        description="Evidence rows will appear after verification."
        icon={FileSearchIcon}
      />
    );
  }
  return (
    <div className="flex flex-col gap-2">
      {items.map((item) => (
        <div key={item.id} className="rounded-md border p-3">
          <div className="mb-2 flex items-center justify-between gap-2">
            <div className="min-w-0">
              <div className="truncate text-sm font-medium">{item.title}</div>
              <div className="text-muted-foreground mt-1 truncate font-mono text-xs">
                {item.path ? formatEvidenceLocation(item) : item.kind}
              </div>
            </div>
            <Badge variant={evidenceBadgeVariant(item.kind)}>{item.kind}</Badge>
          </div>
          <p className="text-muted-foreground line-clamp-3 text-sm leading-6">
            {item.summary}
          </p>
        </div>
      ))}
    </div>
  );
}

function ChangedFilesPanel() {
  if (changedFiles.length === 0) {
    return (
      <EmptyState
        title="No changed files"
        description="Create or select a snapshot to review the diff."
        icon={InboxIcon}
      />
    );
  }

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex items-center justify-between border-b px-3 py-2 text-sm">
        <span className="font-medium">4 files changed</span>
        <Button size="sm" variant="ghost">
          Undo
        </Button>
      </div>
      {changedFiles.map((file) => (
        <div
          key={file.path}
          className="flex items-center justify-between gap-3 border-b px-3 py-2 text-sm last:border-b-0"
        >
          <span className="truncate font-mono text-xs">{file.path}</span>
          <span className="flex shrink-0 items-center gap-2 text-xs">
            <span className="text-success">+{file.additions}</span>
            <span className="text-destructive">-{file.deletions}</span>
          </span>
        </div>
      ))}
    </section>
  );
}

function FindingsPanel() {
  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex items-center justify-between border-b px-3 py-2">
        <div className="flex items-center gap-2">
          <ShieldCheckIcon />
          <span className="text-sm font-medium">Evidence-backed findings</span>
        </div>
        <Badge variant="secondary">18 total</Badge>
      </div>
      {findings.length === 0 ? (
        <EmptyState
          className="border-0"
          title="No findings yet"
          description="Findings will appear as agents and the local verifier produce evidence."
          icon={FileSearchIcon}
        />
      ) : (
        findings.map((finding) => (
          <FindingRow key={finding.title} finding={finding} />
        ))
      )}
    </section>
  );
}

export function MessageComposer({
  agents: directAgents,
  agentConfigs,
  backendDetail,
  disabled,
  disabledReason,
  defaultMode = "review",
  onQuestionChange,
  onSelectedAgentIdChange,
  onSubmit,
  question,
  selectedAgentId,
  submitting,
}: {
  agents?: AgentConfig[];
  agentConfigs?: Loadable<AgentConfig[]>;
  backendDetail?: string;
  disabled?: boolean;
  disabledReason?: string;
  defaultMode?: ComposerMode;
  onQuestionChange?: (value: string) => void;
  onSelectedAgentIdChange?: (value: string) => void;
  onSubmit?: (
    question: string,
    options: {
      agentConfigId?: string;
      contextPolicy: ReviewContextPolicy;
      mode: ComposerMode;
      permission: ComposerPermission;
      reasoning: ComposerReasoning;
      runtime: ComposerRuntime;
    },
  ) => void | Promise<void>;
  question?: string;
  selectedAgentId?: string;
  submitting?: boolean;
}) {
  const [mode, setMode] = useState<ComposerMode>(defaultMode);
  const [runtime, setRuntime] = useState<ComposerRuntime>("standard");
  const [reasoning, setReasoning] = useState<ComposerReasoning>("high");
  const [permission, setPermission] =
    useState<ComposerPermission>("review-mode");
  const [draftQuestion, setDraftQuestion] = useState("");
  const allAgents =
    directAgents ??
    (agentConfigs?.status === "success" ? agentConfigs.data : []);
  const safeAgents = allAgents.filter(
    (agent) => agent.enabled && !agent.capabilities.can_write,
  );
  const composerAgents =
    permission === "local-only"
      ? safeAgents.filter((agent) => agentEgress(agent) === "local")
      : safeAgents;
  const selectedAgent = composerAgents.find(
    (agent) => agent.id === selectedAgentId,
  );
  const effectiveQuestion = question ?? draftQuestion;
  const canSubmit =
    Boolean(onSubmit) &&
    !disabled &&
    !submitting &&
    Boolean(effectiveQuestion.trim());
  const detailMessage =
    disabledReason ??
    (onSubmit
      ? backendDetail
      : "Open Follow-up from a finding to send a scoped question.");

  function updateQuestion(value: string) {
    if (onQuestionChange) {
      onQuestionChange(value);
      return;
    }
    setDraftQuestion(value);
  }

  async function submitComposer(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canSubmit || !onSubmit) {
      return;
    }
    const trimmedQuestion = effectiveQuestion.trim();
    await onSubmit(trimmedQuestion, {
      agentConfigId: selectedAgent?.id,
      contextPolicy: composerContextPolicy(runtime, permission),
      mode,
      permission,
      reasoning,
      runtime,
    });
    if (!onQuestionChange) {
      setDraftQuestion("");
    }
  }

  return (
    <div className="bg-surface-raised/95 border-t p-4">
      <form
        className="cocode-panel mx-auto max-w-5xl overflow-hidden"
        onSubmit={(event) => void submitComposer(event)}
      >
        <InputGroup className="min-h-24 items-stretch border-0">
          <InputGroupTextarea
            aria-label="Follow-up prompt"
            disabled={disabled}
            value={effectiveQuestion}
            placeholder={
              disabled
                ? "Start a review before asking follow-up questions..."
                : "Ask a follow-up grounded in this review context..."
            }
            className="min-h-20"
            onChange={(event) => updateQuestion(event.target.value)}
          />
        </InputGroup>
        <div className="flex flex-wrap items-center justify-between gap-2 border-t px-3 py-2">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <ComposerDropdown
              label={`Tool: ${mode}`}
              onSelect={setMode}
              options={["review", "finding follow-up"]}
            />
            {composerAgents.length > 0 && (
              <NativeSelect
                aria-label="Follow-up agent"
                className="max-w-56"
                disabled={disabled || submitting}
                size="sm"
                value={selectedAgent?.id ?? ""}
                onChange={(event) =>
                  onSelectedAgentIdChange?.(event.target.value)
                }
              >
                <NativeSelectOption value="">
                  Auto-select agent
                </NativeSelectOption>
                {composerAgents.map((agent) => (
                  <NativeSelectOption key={agent.id} value={agent.id}>
                    {formatComposerAgentLabel(agent)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            )}
            {composerAgents.length === 0 && (
              <Button disabled size="sm" variant="ghost">
                <MessageSquareIcon data-icon="inline-start" />
                No read-only agents
              </Button>
            )}
            <ComposerDropdown
              label={`Context: ${runtime}`}
              onSelect={setRuntime}
              options={["quick", "standard", "deep"]}
            />
            <ComposerDropdown
              label={`Reasoning: ${reasoning}`}
              onSelect={setReasoning}
              options={["low", "medium", "high"]}
            />
            <ComposerDropdown
              label={`Permission: ${permission}`}
              onSelect={setPermission}
              options={["review-mode", "local-only"]}
            />
          </div>
          <InputGroupButton
            disabled={!canSubmit}
            size="icon-sm"
            type="submit"
            variant={canSubmit ? "default" : "ghost"}
            aria-label="Send follow-up question"
          >
            <ArrowUpIcon />
          </InputGroupButton>
        </div>
      </form>
      {detailMessage && (
        <div className="text-muted-foreground mx-auto mt-2 max-w-5xl truncate text-center text-xs">
          {detailMessage}
        </div>
      )}
    </div>
  );
}

function ComposerDropdown<T extends string>({
  label,
  onSelect,
  options,
}: {
  label: string;
  onSelect?: (value: T) => void;
  options: readonly T[];
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" variant="ghost">
          {label}
          <ChevronDownIcon data-icon="inline-end" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuGroup>
          {options.map((option) => (
            <DropdownMenuItem key={option} onSelect={() => onSelect?.(option)}>
              {option}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function formatComposerAgentLabel(agent: AgentConfig) {
  return formatSetupAgentLabel(agent);
}

function FindingRow({
  finding,
}: {
  finding: {
    title: string;
    file: string;
    lines: string;
    severity: string;
    status: string;
  };
}) {
  return (
    <button
      className="hover:bg-surface flex w-full items-start gap-3 border-b px-3 py-3 text-left last:border-b-0"
      type="button"
    >
      <CircleIcon className="text-destructive mt-1" />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{finding.title}</div>
        <div className="text-muted-foreground mt-1 flex items-center gap-2 text-xs">
          <span className="truncate font-mono">{finding.file}</span>
          <span>{finding.lines}</span>
        </div>
      </div>
      <div className="flex shrink-0 gap-1">
        <Badge
          variant={finding.severity === "High" ? "destructive" : "secondary"}
        >
          {finding.severity}
        </Badge>
        <Badge variant="outline">{finding.status}</Badge>
      </div>
    </button>
  );
}

function appendBoundedEvent(events: ReviewEvent[], event: ReviewEvent) {
  const exists = events.some(
    (candidate) =>
      candidate.id === event.id || candidate.sequence === event.sequence,
  );
  if (exists) {
    return events;
  }
  const sorted = [...events, event].sort(
    (left, right) => left.sequence - right.sequence,
  );
  if (sorted.length <= MAX_REVIEW_EVENTS_RENDERED) {
    return sorted;
  }
  return compactReviewEvents(sorted);
}

function compactReviewEvents(events: ReviewEvent[]) {
  const kept = new Set<string>();
  const byRun = new Map<string, number>();
  let nonAgentRunEvents = 0;

  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (!event) {
      continue;
    }
    const key = event.id || String(event.sequence);
    if (event.agent_run_id && event.type.startsWith("AgentRun")) {
      const count = byRun.get(event.agent_run_id) ?? 0;
      if (
        count < MAX_REVIEW_EVENTS_PER_AGENT_RUN ||
        isAgentRunLifecycleEvent(event)
      ) {
        kept.add(key);
        byRun.set(event.agent_run_id, count + 1);
      }
      continue;
    }
    if (nonAgentRunEvents < MAX_NON_AGENT_RUN_EVENTS) {
      kept.add(key);
      nonAgentRunEvents += 1;
    }
  }

  return events.filter((event) => kept.has(event.id || String(event.sequence)));
}

function isAgentRunLifecycleEvent(event: ReviewEvent) {
  return (
    event.type === "AgentRunQueued" ||
    event.type === "AgentRunStarted" ||
    event.type === "AgentRunCompleted" ||
    event.type === "AgentRunFailed" ||
    event.type === "AgentRunCanceled"
  );
}

function toErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

function formatDecisionLabel(value: string) {
  const normalized = value === "undecided" ? "needs_triage" : value;
  return normalized
    .replace(/_/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function formatFindingLocation(finding: Finding) {
  if (!finding.primary_path) {
    return "No primary location";
  }
  if (finding.primary_start_line && finding.primary_end_line) {
    return `${finding.primary_path}:L${finding.primary_start_line}-L${finding.primary_end_line}`;
  }
  if (finding.primary_start_line) {
    return `${finding.primary_path}:L${finding.primary_start_line}`;
  }
  return finding.primary_path;
}

function findingClipboardText(finding: Finding) {
  return detailedFindingDraftComment(finding);
}

function detailedFindingDraftComment(
  finding: Finding,
  detail?: FindingDetailResponse,
) {
  const candidates = detail?.candidates.length ?? finding.merged_from_count;
  const evidenceItems = prioritizedEvidenceItems(detail?.evidence_items ?? []);
  const lines = [
    `### ${finding.canonical_claim}`,
    "",
    `**Severity:** ${formatDecisionLabel(finding.severity)}`,
    `**Confidence:** ${Math.round(finding.confidence * 100)}%`,
    `**Status:** ${formatDecisionLabel(finding.verification_status)}`,
    `**Location:** \`${formatFindingLocation(finding)}\``,
    candidates > 0 ? `**Agent signals:** ${candidates}` : "",
    "",
    "#### Why this matters",
    finding.evidence_summary ||
      "The review agents flagged this changed path as needing human triage.",
    "",
    finding.counter_evidence_summary
      ? ["#### Counter-evidence", finding.counter_evidence_summary, ""].join(
          "\n",
        )
      : "",
    finding.suggested_fix
      ? ["#### Suggested fix", finding.suggested_fix, ""].join("\n")
      : "",
  ];
  if (evidenceItems.length > 0) {
    lines.push("#### Evidence");
    for (const item of evidenceItems.slice(0, 5)) {
      lines.push(
        `- ${item.title} (${formatEvidenceLocation(item)}): ${item.summary}`,
      );
    }
    lines.push("");
  }
  lines.push(
    "_Generated by cocode from the merged multi-agent review; please verify the cited lines before publishing._",
  );
  return lines.filter((line) => line !== "").join("\n");
}

function snippetLines(item: EvidenceItem) {
  const startLine = item.line_window?.start_line ?? item.start_line ?? 1;
  return (item.code_snippet ?? "")
    .split("\n")
    .slice(0, MAX_CODE_LINES_RENDERED)
    .map((text, index) => ({ number: startLine + index, text }));
}

function snippetLineTone(
  item: EvidenceItem,
  finding: Finding,
  lineNumber: number,
  part: "code" | "number",
) {
  const startLine = finding.primary_start_line || item.start_line || 0;
  const endLine = finding.primary_end_line || item.end_line || startLine;
  const highlighted =
    startLine > 0 && lineNumber >= startLine && lineNumber <= endLine;
  if (!highlighted) {
    return part === "code" ? "bg-white" : "bg-[#fbfbfa]";
  }
  if (item.kind === "counter" || item.kind === "test") {
    return part === "code"
      ? "bg-amber-50 text-amber-950"
      : "bg-amber-50 text-amber-800";
  }
  return part === "code"
    ? "bg-emerald-50 text-emerald-950"
    : "bg-emerald-50 text-emerald-800";
}

function prioritizedEvidenceItems(items: EvidenceItem[]) {
  const rank: Record<string, number> = {
    supporting: 6,
    counter: 5,
    missing: 4,
    test: 3,
    search: 2,
    agent: 1,
  };
  return [...items].sort((left, right) => {
    const rankDelta = (rank[right.kind] ?? 0) - (rank[left.kind] ?? 0);
    if (rankDelta !== 0) {
      return rankDelta;
    }
    return right.confidence - left.confidence;
  });
}

function formatEvidenceLocation(item: EvidenceItem) {
  if (!item.path) {
    return item.kind;
  }
  if (item.start_line && item.end_line) {
    return `${item.path}:L${item.start_line}-L${item.end_line}`;
  }
  if (item.start_line) {
    return `${item.path}:L${item.start_line}`;
  }
  return item.path;
}

function evidenceBadgeVariant(kind: string) {
  if (kind === "counter" || kind === "missing") {
    return "destructive";
  }
  if (kind === "supporting" || kind === "test") {
    return "secondary";
  }
  return "outline";
}

function firstEvidenceMapSelection(
  map: EvidenceMapResponse,
): EvidenceMapSelection | null {
  const callPathNode = map.call_path.find((step) => step.node_id)?.node_id;
  const primaryNode =
    map.nodes.find((node) => node.kind === "changed_code")?.id ??
    callPathNode ??
    map.nodes[0]?.id;
  if (primaryNode) {
    return { kind: "node", id: primaryNode };
  }
  if (map.edges[0]) {
    return { kind: "edge", id: map.edges[0].id };
  }
  if (map.call_paths[0]) {
    return { kind: "call_path", id: map.call_paths[0].id };
  }
  return null;
}

function evidenceMapNarrativeStory(map: EvidenceMapResponse): {
  changed: EvidenceMapNode[];
  checks: EvidenceMapNode[];
  claimSummary: string;
  supportingCount: number;
  counterCount: number;
  omittedNodes: number;
} {
  const nodes = dedupeEvidenceMapNodes(map.nodes);
  const primaryPath = map.finding.primary_path;
  const changedKinds = new Set([
    "changed_code",
    "handler",
    "route",
    "entrypoint",
  ]);
  const changed = nodes
    .filter(
      (node) =>
        changedKinds.has(node.kind) ||
        Boolean(primaryPath && evidenceMapNodePath(node) === primaryPath),
    )
    .sort(
      (left, right) =>
        evidenceMapChangedNodeRank(left, primaryPath) -
          evidenceMapChangedNodeRank(right, primaryPath) ||
        right.confidence - left.confidence,
    )
    .slice(0, 3);
  const focusedChanged =
    changed.length > 0 ? changed : nodes.length > 0 ? [nodes[0]] : [];
  const changedIDs = new Set(focusedChanged.map((node) => node.id));
  const checks = selectEvidenceMapChecks(
    nodes.filter((node) => !changedIDs.has(node.id)),
  );
  const visibleIDs = new Set(
    [...focusedChanged, ...checks].map((node) => node.id),
  );
  const evidenceCounts = map.panel.evidence_counts ?? {};
  const supportingCount =
    evidenceCounts.supporting ??
    evidenceCounts.changed_code ??
    map.panel.evidence.filter((item) =>
      ["supporting", "changed_code", "agent", "static_analysis"].includes(
        item.kind,
      ),
    ).length;
  const counterCount =
    evidenceCounts.counter ??
    map.panel.evidence.filter((item) =>
      ["counter", "missing", "test", "counter_evidence"].includes(item.kind),
    ).length;
  return {
    changed: focusedChanged,
    checks,
    claimSummary:
      map.panel.evidence_summary ||
      map.finding.evidence_summary ||
      map.graph.summary ||
      "",
    supportingCount,
    counterCount,
    omittedNodes: Math.max(0, nodes.length - visibleIDs.size),
  };
}

function focusedEvidenceMap(map: EvidenceMapResponse): EvidenceMapResponse {
  const story = evidenceMapNarrativeStory(map);
  const claimNode = evidenceMapClaimNode(map);
  const focusedNodes = dedupeEvidenceMapNodes([
    ...story.changed.slice(0, 2),
    claimNode,
    ...story.checks,
  ]);
  const synthesizedEdges = synthesizeFocusedEvidenceEdges(focusedNodes, []);
  return {
    ...map,
    nodes: focusedNodes,
    edges: synthesizedEdges,
  };
}

function selectEvidenceMapChecks(nodes: EvidenceMapNode[]) {
  const ranked = [...nodes].sort(
    (left, right) =>
      evidenceMapCheckNodeRank(left.kind) -
        evidenceMapCheckNodeRank(right.kind) ||
      right.confidence - left.confidence,
  );
  const selected: EvidenceMapNode[] = [];
  const seenGroups = new Set<string>();
  for (const node of ranked) {
    const group = [
      evidenceMapCheckGroup(node.kind),
      evidenceMapNodePath(node),
    ].join(":");
    if (seenGroups.has(group)) {
      continue;
    }
    selected.push(node);
    seenGroups.add(group);
    if (selected.length >= 3) {
      break;
    }
  }
  if (selected.length < 3) {
    for (const node of ranked) {
      if (selected.some((item) => item.id === node.id)) {
        continue;
      }
      selected.push(node);
      if (selected.length >= 3) {
        break;
      }
    }
  }
  return selected;
}

function evidenceMapCheckGroup(kind: string) {
  switch (kind) {
    case "missing_guard":
      return "missing_guard";
    case "test":
      return "test";
    case "counter_evidence":
      return "counter";
    case "static_analysis":
      return "static";
    default:
      return kind;
  }
}

function evidenceMapClaimNode(map: EvidenceMapResponse): EvidenceMapNode {
  return {
    id: `finding_claim_${map.finding.id}`,
    kind: "finding_claim",
    label: map.finding.canonical_claim,
    confidence: map.finding.confidence,
    metadata: { synthetic: true, finding_id: map.finding.id },
  };
}

function synthesizeFocusedEvidenceEdges(
  nodes: EvidenceMapNode[],
  existing: EvidenceMapEdge[],
) {
  if (nodes.length <= 1) {
    return [];
  }
  const existingKeys = new Set(
    existing.map((edge) => `${edge.source}->${edge.target}`),
  );
  const claim = nodes.find((node) => node.kind === "finding_claim");
  const changedNodes = nodes.filter((node) =>
    ["entrypoint", "route", "handler", "changed_code"].includes(node.kind),
  );
  const primary = changedNodes[changedNodes.length - 1] ?? nodes[0];
  const edges: EvidenceMapEdge[] = [];

  for (let index = 1; index < changedNodes.length; index += 1) {
    const source = changedNodes[index - 1];
    const target = changedNodes[index];
    const key = `${source.id}->${target.id}`;
    if (existingKeys.has(key)) {
      continue;
    }
    edges.push(syntheticEvidenceEdge(source.id, target.id, "reachable path"));
    existingKeys.add(key);
  }

  if (claim) {
    for (const source of changedNodes.slice(-1)) {
      const key = `${source.id}->${claim.id}`;
      if (!existingKeys.has(key)) {
        edges.push(syntheticEvidenceEdge(source.id, claim.id, "grounds claim"));
        existingKeys.add(key);
      }
    }
    for (const node of nodes) {
      if (node.id === claim.id || changedNodes.includes(node)) {
        continue;
      }
      const key = `${claim.id}->${node.id}`;
      if (existingKeys.has(key)) {
        continue;
      }
      edges.push(
        syntheticEvidenceEdge(
          claim.id,
          node.id,
          evidenceMapFocusedEdgeLabel(node),
          node.kind === "missing_guard" ? "missing" : "supported",
        ),
      );
      existingKeys.add(key);
    }
    return edges;
  }

  for (const node of nodes) {
    if (!primary || node.id === primary.id || changedNodes.includes(node)) {
      continue;
    }
    const key = `${primary.id}->${node.id}`;
    if (existingKeys.has(key)) {
      continue;
    }
    edges.push(
      syntheticEvidenceEdge(
        primary.id,
        node.id,
        evidenceMapFocusedEdgeLabel(node),
        node.kind === "missing_guard" ? "missing" : "supported",
      ),
    );
    existingKeys.add(key);
  }
  return edges;
}

function syntheticEvidenceEdge(
  source: string,
  target: string,
  label: string,
  status = "supported",
): EvidenceMapEdge {
  return {
    id: `focus_${source}_${target}_${label.replace(/\W+/g, "_")}`,
    source,
    target,
    kind: status === "missing" ? "missing_guard" : "evidence_flow",
    status,
    label,
    confidence: 0.75,
    metadata: { synthetic: true },
  };
}

function evidenceMapFocusedEdgeLabel(node: EvidenceMapNode) {
  switch (node.kind) {
    case "missing_guard":
      return "missing guard";
    case "test":
      return "test signal";
    case "counter_evidence":
      return "counter check";
    case "static_analysis":
      return "static signal";
    default:
      return "evidence";
  }
}

function dedupeEvidenceMapNodes(nodes: EvidenceMapNode[]) {
  const byKey = new Map<string, EvidenceMapNode>();
  for (const node of nodes) {
    const key = [
      node.kind,
      evidenceMapNodePath(node),
      node.start_line ?? node.deep_link?.start_line ?? "",
      evidenceMapReadableNodeLabel(node).toLowerCase(),
    ].join(":");
    const existing = byKey.get(key);
    if (!existing || node.confidence > existing.confidence) {
      byKey.set(key, node);
    }
  }
  return [...byKey.values()];
}

function evidenceMapNodePath(node: EvidenceMapNode) {
  return node.deep_link?.path ?? node.path ?? "";
}

function evidenceMapReadableNodeLabel(node: EvidenceMapNode) {
  const label = evidenceMapNodeLabel(node).trim();
  if (/^potential counter-evidence at\b/i.test(label)) {
    if (node.kind === "test") {
      return "Related test check";
    }
    return "Possible counter-evidence";
  }
  if (/^counter-evidence/i.test(label)) {
    return "Counter-evidence check";
  }
  if (/^missing guard/i.test(label)) {
    return "Missing guard";
  }
  if (node.kind === "missing_guard" && label) {
    return `Missing guard: ${label}`;
  }
  if (label) {
    return label;
  }
  return node.kind.replaceAll("_", " ");
}

function evidenceMapChangedNodeRank(
  node: EvidenceMapNode,
  primaryPath: string | undefined,
) {
  const nodePath = evidenceMapNodePath(node);
  if (primaryPath && nodePath === primaryPath) {
    return 0;
  }
  switch (node.kind) {
    case "changed_code":
      return 1;
    case "handler":
      return 2;
    case "route":
      return 3;
    case "entrypoint":
      return 4;
    default:
      return 9;
  }
}

function evidenceMapCheckNodeRank(kind: string) {
  switch (kind) {
    case "missing_guard":
      return 0;
    case "counter_evidence":
      return 1;
    case "test":
      return 2;
    case "static_analysis":
      return 3;
    default:
      return 8;
  }
}

function evidenceMapGraphRefs(
  selection: EvidenceMapSelection | null,
): EvidenceMapGraphRef[] {
  if (!selection) {
    return [];
  }
  if (selection.kind === "node") {
    return [{ node_id: selection.id }];
  }
  if (selection.kind === "edge") {
    return [{ edge_id: selection.id }];
  }
  return [{ call_path_id: selection.id }];
}

function evidenceMapNodeLabel(node: EvidenceMapNode) {
  if (node.label.trim()) {
    return node.label;
  }
  if (node.kind === "missing_guard") {
    return "Missing guard";
  }
  if (node.kind === "counter_evidence") {
    return "Counter-evidence";
  }
  if (node.kind === "changed_code") {
    return "Changed code";
  }
  if (node.kind === "finding_claim") {
    return "Finding claim";
  }
  if (node.kind === "test") {
    return "Related test";
  }
  return node.label;
}

function evidenceMapNodeMeta(node: EvidenceMapNode) {
  const path = node.deep_link?.path ?? node.path;
  if (path) {
    return shortPath(path);
  }
  return node.kind.replaceAll("_", " ");
}

function evidenceMapNodeStyle(kind: string) {
  switch (kind) {
    case "finding_claim":
      return {
        surface: "fill-background stroke-foreground/55",
        selected: "stroke-foreground",
        bar: "fill-foreground/80",
      };
    case "missing_guard":
      return {
        surface: "fill-destructive/5 stroke-destructive/70",
        selected: "stroke-destructive",
        bar: "fill-destructive/80",
      };
    case "counter_evidence":
    case "test":
      return {
        surface: "fill-warning/10 stroke-warning/70",
        selected: "stroke-warning",
        bar: "fill-warning/80",
      };
    case "entrypoint":
    case "route":
      return {
        surface: "fill-primary/5 stroke-primary/55",
        selected: "stroke-primary",
        bar: "fill-primary/80",
      };
    case "handler":
    case "changed_code":
      return {
        surface: "fill-success/10 stroke-success/60",
        selected: "stroke-success",
        bar: "fill-success/80",
      };
    default:
      return {
        surface: "fill-background stroke-border",
        selected: "stroke-primary",
        bar: "fill-muted-foreground/80",
      };
  }
}

function buildEvidenceMapLayout(map: EvidenceMapResponse): EvidenceMapLayout {
  const positioned: PositionedEvidenceMapNode[] = [];
  const columns = new Map<number, EvidenceMapNode[]>();
  for (const node of map.nodes) {
    const column = evidenceMapColumnForKind(node.kind);
    const columnNodes = columns.get(column) ?? [];
    columnNodes.push(node);
    columns.set(column, columnNodes);
  }
  for (const columnNodes of columns.values()) {
    columnNodes.sort(
      (left, right) =>
        evidenceMapSideNodeRank(left.kind) -
          evidenceMapSideNodeRank(right.kind) ||
        right.confidence - left.confidence,
    );
  }
  const maxRows = Math.max(
    1,
    ...[...columns.values()].map((nodes) => nodes.length),
  );
  for (const [column, columnNodes] of [...columns.entries()].sort(
    ([left], [right]) => left - right,
  )) {
    const verticalOffset = Math.max(0, (maxRows - columnNodes.length) * 62);
    for (const [index, node] of columnNodes.entries()) {
      positioned.push({
        node,
        x: 64 + column * EVIDENCE_MAP_COLUMN_GAP,
        y: 56 + verticalOffset + index * 124,
      });
    }
  }

  const maxX = positioned.reduce(
    (current, item) => Math.max(current, item.x),
    0,
  );
  const maxY = positioned.reduce(
    (current, item) => Math.max(current, item.y),
    0,
  );
  const width = Math.max(620, maxX + EVIDENCE_MAP_NODE_WIDTH + 96);
  const height = Math.max(440, maxY + EVIDENCE_MAP_NODE_HEIGHT + 80);
  const nodeById = new Map(positioned.map((node) => [node.node.id, node]));
  return { nodes: positioned, nodeById, width, height };
}

function evidenceMapColumnForKind(kind: string) {
  switch (kind) {
    case "entrypoint":
    case "route":
    case "handler":
    case "changed_code":
      return 0;
    case "finding_claim":
      return 1;
    case "missing_guard":
    case "test":
    case "counter_evidence":
    case "static_analysis":
      return 2;
    default:
      return 2;
  }
}

function evidenceMapSideNodeRank(kind: string) {
  switch (kind) {
    case "missing_guard":
      return 0;
    case "counter_evidence":
      return 1;
    case "test":
      return 2;
    default:
      return 3;
  }
}

function evidenceMapOpenTarget(
  node: EvidenceMapNode | undefined,
  callPath: EvidenceMapCallPath | undefined,
  repository: Repository | undefined,
): { filePath: string; line?: number; column?: number } | null {
  const nodePath = node?.deep_link?.path ?? node?.path;
  if (nodePath) {
    return {
      filePath: resolveRepositoryFilePath(repository, nodePath),
      line: node?.deep_link?.start_line ?? node?.start_line,
    };
  }
  const step = callPath?.steps.find((candidate) => Boolean(candidate.path));
  if (!step?.path) {
    return null;
  }
  return {
    filePath: resolveRepositoryFilePath(repository, step.path),
    line: step.start_line,
  };
}

function resolveRepositoryFilePath(
  repository: Repository | undefined,
  filePath: string,
) {
  if (/^(?:\/|[A-Za-z]:[\\/])/.test(filePath)) {
    return filePath;
  }
  const root = repository?.local_path?.replace(/\/+$/, "");
  if (!root) {
    return filePath;
  }
  return `${root}/${filePath.replace(/^\/+/, "")}`;
}

function formatEvidenceNodeLocation(node: EvidenceMapNode) {
  const path = node.deep_link?.path ?? node.path;
  if (!path) {
    return node.kind.replaceAll("_", " ");
  }
  return `${path}${node.start_line ? `:L${formatLineRange(node.start_line, node.end_line)}` : ""}`;
}

function formatEvidenceRefLocation(item: EvidenceMapPanelEvidenceRef) {
  if (!item.path) {
    return item.kind;
  }
  return `${item.path}${item.start_line ? `:L${formatLineRange(item.start_line, item.end_line)}` : ""}`;
}

function formatCallPathStepLocation(step: EvidenceMapCallPathStep) {
  if (!step.path) {
    return "No file location";
  }
  return `${step.path}${step.start_line ? `:L${formatLineRange(step.start_line, step.end_line)}` : ""}`;
}

function formatLineRange(startLine: number, endLine?: number) {
  if (endLine && endLine !== startLine) {
    return `${startLine}-L${endLine}`;
  }
  return String(startLine);
}

function shortPath(path: string) {
  const parts = path.split("/");
  if (parts.length <= 3) {
    return path;
  }
  return `${parts.at(-3)}/${parts.at(-2)}/${parts.at(-1)}`;
}

function truncate(value: string, maxLength: number) {
  if (value.length <= maxLength) {
    return value;
  }
  return `${value.slice(0, Math.max(0, maxLength - 3))}...`;
}

function formatShortDate(value: string) {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) {
    return value;
  }
  return new Date(timestamp).toLocaleString([], {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

function wrapSvgLabel(value: string, maxLineLength: number) {
  const words = value
    .split(/\s+/)
    .filter(Boolean)
    .flatMap((word) => splitLongWord(word, maxLineLength));
  const lines: string[] = [];
  let current = "";
  for (const word of words) {
    const next = current ? `${current} ${word}` : word;
    if (next.length > maxLineLength && current) {
      lines.push(current);
      current = word;
      continue;
    }
    current = next;
  }
  if (current) {
    lines.push(current);
  }
  return lines.length > 0 ? lines : [value];
}

function splitLongWord(word: string, maxLineLength: number) {
  if (word.length <= maxLineLength) {
    return [word];
  }
  const chunks: string[] = [];
  for (let index = 0; index < word.length; index += maxLineLength) {
    chunks.push(word.slice(index, index + maxLineLength));
  }
  return chunks;
}

function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const timeout = window.setTimeout(() => setDebounced(value), delayMs);
    return () => window.clearTimeout(timeout);
  }, [delayMs, value]);

  return debounced;
}

function formatContextKind(kind: string): string {
  return kind
    .replace(/_/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function formatDetectorSummary(detectors: Record<string, number>): string {
  const entries = Object.entries(detectors)
    .sort((left, right) => right[1] - left[1])
    .slice(0, 4)
    .map(([name, count]) => `${formatContextKind(name)} ${count}`);
  return entries.length > 0 ? entries.join(", ") : "No detector details";
}

function formatAuditKind(kind: string): string {
  return formatContextKind(kind);
}

function formatAuditMetadata(metadata: Record<string, unknown>): string {
  const entries = Object.entries(metadata)
    .filter(
      ([, value]) => value !== "" && value !== null && value !== undefined,
    )
    .slice(0, 6)
    .map(([key, value]) => `${key}: ${formatAuditMetadataValue(value)}`);
  const text = entries.join(" • ");
  return text.length > 260 ? `${text.slice(0, 260)}...` : text;
}

function formatAuditMetadataValue(value: unknown): string {
  if (typeof value === "string") {
    return value.length > 120 ? `${value.slice(0, 120)}...` : value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  if (Array.isArray(value)) {
    return `${value.length} items`;
  }
  if (value && typeof value === "object") {
    return JSON.stringify(value).slice(0, 120);
  }
  return String(value);
}

function formatRelativeAge(value: string): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) {
    return "now";
  }

  const elapsedMs = Date.now() - timestamp;
  if (elapsedMs < 60_000) {
    return "now";
  }

  const minute = 60_000;
  const hour = 60 * minute;
  const day = 24 * hour;
  const week = 7 * day;

  if (elapsedMs < hour) {
    return `${Math.floor(elapsedMs / minute)}m`;
  }
  if (elapsedMs < day) {
    return `${Math.floor(elapsedMs / hour)}h`;
  }
  if (elapsedMs < week) {
    return `${Math.floor(elapsedMs / day)}d`;
  }
  return `${Math.floor(elapsedMs / week)}w`;
}
