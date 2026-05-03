import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ArrowUpIcon,
  BellIcon,
  BotIcon,
  CheckIcon,
  ChevronDownIcon,
  CircleIcon,
  ClockIcon,
  Code2Icon,
  CopyIcon,
  FileSearchIcon,
  FolderOpenIcon,
  GitBranchIcon,
  GitPullRequestIcon,
  InboxIcon,
  MessageSquareIcon,
  MoreHorizontalIcon,
  PanelRightIcon,
  PauseIcon,
  PlusIcon,
  SearchIcon,
  SettingsIcon,
  ShieldCheckIcon,
  SparklesIcon,
  SquareIcon,
  TerminalIcon,
  type LucideIcon,
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
  DropdownMenuLabel,
  DropdownMenuSeparator,
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
import { Progress } from "@/components/ui/progress";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import {
  type AgentConfig,
  type AgentConfigHealth,
  type AgentConfigInput,
  type AgentPreset,
  type ApiClient,
  type ChangedFile,
  createCocodeClient,
  errorApiState,
  idleApiState,
  loadApiResource,
  loadingApiState,
  successApiState,
  type ApiSessionResponse,
  type Loadable,
  type EvidenceItem,
  type Finding,
  type FindingDetailResponse,
  type FindingListResponse,
  type OpenRepositoryResponse,
  type Repository,
  type ReviewContextPolicy,
  type ReviewEvent,
  type ReviewSessionSummary,
  type ReviewSession,
  type Snapshot,
  type Workspace,
} from "@/lib/api";
import { cn } from "@/lib/utils";

const MAX_SIDEBAR_SESSIONS = 12;
const MAX_SEARCH_RESULTS = 5;
const MAX_CHANGED_FILES_RENDERED = 120;
const MAX_REVIEW_EVENTS_RENDERED = 120;
const MAX_FINDINGS_RENDERED = 150;
const MAX_CODE_LINES_RENDERED = 80;

type MainView = "new-thread" | "configure" | "review" | "agent-settings";
type SnapshotSource = "github" | "local-changes" | "branch-compare";
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
  const [backendUrl, setBackendUrl] = useState("");
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
  const [snapshot, setSnapshot] = useState<Loadable<Snapshot>>(idleApiState());
  const [changedFilesState, setChangedFilesState] =
    useState<Loadable<ChangedFile[]>>(idleApiState());
  const [agentConfigs, setAgentConfigs] =
    useState<Loadable<AgentConfig[]>>(idleApiState());
  const [currentReviewSession, setCurrentReviewSession] =
    useState<ReviewSession | null>(null);
  const [activeWorkspaceId, setActiveWorkspaceId] = useState("");
  const [activeRepositoryId, setActiveRepositoryId] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);

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

  const loadConfigureData = useCallback(
    async (api: ApiClient, nextSnapshot: Snapshot) => {
      setChangedFilesState(loadingApiState());
      setAgentConfigs(loadingApiState());
      const [changedFiles, agents] = await Promise.all([
        loadApiResource(() => api.listChangedFiles(nextSnapshot.id)),
        loadApiResource(() => api.listAgentConfigs()),
      ]);
      setChangedFilesState(changedFiles);
      setAgentConfigs(agents);
    },
    [],
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
        setBackendUrl(info.baseUrl);

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

    setActiveWorkspaceId(state.data.workspace.id);
    setActiveRepositoryId(state.data.repository.id);
    setRepositories(successApiState(state.data.repositories));
    setSnapshot(idleApiState());
    setMainView("new-thread");
    await refreshNavigation(client, state.data.workspace.id);
  }, [client, refreshNavigation]);

  const handleCreateSnapshot = useCallback(
    async (request: {
      source: SnapshotSource;
      githubUrl: string;
      baseRef: string;
      headRef: string;
    }) => {
      if (!client || !activeWorkspace || !activeRepository) {
        setSnapshot(
          errorApiState(
            new Error("Open a local repository before creating a review"),
          ),
        );
        return;
      }

      setSnapshot(loadingApiState());
      const nextSnapshot = await loadApiResource(() => {
        if (request.source === "github") {
          return client.createGitHubSnapshot({
            workspace_id: activeWorkspace.id,
            repository_id: activeRepository.id,
            url: request.githubUrl,
            github_token: "",
          });
        }
        if (request.source === "branch-compare") {
          return client.createLocalCompareSnapshot({
            workspace_id: activeWorkspace.id,
            repository_id: activeRepository.id,
            base_ref: request.baseRef,
            head_ref: request.headRef,
          });
        }
        return client.createLocalChangesSnapshot({
          workspace_id: activeWorkspace.id,
          repository_id: activeRepository.id,
        });
      });

      setSnapshot(nextSnapshot);
      if (nextSnapshot.status !== "success") {
        return;
      }
      setMainView("configure");
      await loadConfigureData(client, nextSnapshot.data);
    },
    [activeRepository, activeWorkspace, client, loadConfigureData],
  );

  const handleSelectReviewSession = useCallback((session: ReviewSession) => {
    setCurrentReviewSession(session);
    setMainView("review");
  }, []);

  const backendDetail =
    apiSession.status === "error"
      ? apiSession.error.message
      : backendUrl || "Waiting for backend info";
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
        heading: "Workspaces",
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
                  title: "Open local repository",
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
            onOpenRepository={handleOpenRepository}
            onOpenSearch={() => setSearchOpen(true)}
            onOpenAgentSettings={() => setMainView("agent-settings")}
            onOpenNewThread={() => setMainView("new-thread")}
            onSelectReviewSession={handleSelectReviewSession}
            onSelectWorkspace={handleSelectWorkspace}
          />
        }
        header={
          <TopNav
            activeRepository={activeRepository}
            activeSession={displayedSession}
            activeWorkspace={activeWorkspace}
            isOpeningRepository={repositoryOpenState.status === "loading"}
            onOpenRepository={handleOpenRepository}
            onOpenSearch={() => setSearchOpen(true)}
          />
        }
        detailPane={
          mainView === "new-thread" || mainView === "configure" ? (
            <ReviewPane />
          ) : undefined
        }
      >
        {mainView === "new-thread" && (
          <NewThreadScreen
            activeRepository={activeRepository}
            activeWorkspace={activeWorkspace}
            onCreateSnapshot={handleCreateSnapshot}
            onOpenRepository={handleOpenRepository}
            snapshot={snapshot}
          />
        )}
        {mainView === "configure" && (
          <ConfigureReviewScreen
            activeRepository={activeRepository}
            activeWorkspace={activeWorkspace}
            agentConfigs={agentConfigs}
            changedFiles={changedFilesState}
            client={client}
            onReviewStarted={(session) => {
              setCurrentReviewSession(session);
              setMainView("review");
              if (client) {
                void refreshNavigation(client, session.workspace_id);
              }
            }}
            snapshot={snapshot}
          />
        )}
        {mainView === "review" && (
          <ReviewThread
            agentConfigs={agentConfigs}
            apiSession={apiSession}
            backendDetail={backendDetail}
            client={client}
            session={displayedSession}
          />
        )}
        {mainView === "agent-settings" && (
          <AgentSettingsScreen
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
    </>
  );
}

function NewThreadScreen({
  activeRepository,
  activeWorkspace,
  onCreateSnapshot,
  onOpenRepository,
  snapshot,
}: {
  activeRepository?: Repository;
  activeWorkspace?: Workspace;
  onCreateSnapshot: (request: {
    source: SnapshotSource;
    githubUrl: string;
    baseRef: string;
    headRef: string;
  }) => void;
  onOpenRepository: () => void;
  snapshot: Loadable<Snapshot>;
}) {
  const [source, setSource] = useState<SnapshotSource>("local-changes");
  const [githubUrl, setGitHubUrl] = useState("");
  const [baseRef, setBaseRef] = useState(
    activeRepository?.default_branch ?? "main",
  );
  const [headRef, setHeadRef] = useState("HEAD");
  const [localError, setLocalError] = useState("");

  const canCreate = Boolean(activeWorkspace && activeRepository);

  function submit() {
    if (!canCreate) {
      setLocalError("Open a git repository before creating a review.");
      return;
    }
    if (source === "github" && githubUrl.trim() === "") {
      setLocalError("Enter a GitHub pull request URL.");
      return;
    }
    if (
      source === "branch-compare" &&
      (baseRef.trim() === "" || headRef.trim() === "")
    ) {
      setLocalError("Enter both base and head refs.");
      return;
    }
    setLocalError("");
    onCreateSnapshot({
      source,
      githubUrl: githubUrl.trim(),
      baseRef: baseRef.trim(),
      headRef: headRef.trim(),
    });
  }

  return (
    <section className="flex min-w-0 flex-col">
      <ScrollArea className="flex-1 px-6 py-5">
        <div className="mx-auto flex max-w-4xl flex-col gap-5">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <h1 className="text-xl font-semibold">New review thread</h1>
              <p className="text-muted-foreground mt-1 text-sm">
                Start from a PR URL, local changes, or a branch comparison.
              </p>
            </div>
            <Button variant="outline" onClick={onOpenRepository}>
              <FolderOpenIcon data-icon="inline-start" />
              {activeRepository ? "Switch repo" : "Open repo"}
            </Button>
          </div>

          {!canCreate && (
            <EmptyState
              title="Open a repository first"
              description="cocode needs a local git repository so snapshots, branch comparisons, and local-only context stay grounded on your machine."
              action={
                <Button onClick={onOpenRepository}>
                  <FolderOpenIcon data-icon="inline-start" />
                  Open local repo
                </Button>
              }
              icon={FolderOpenIcon}
            />
          )}

          {canCreate && (
            <>
              <section className="bg-surface-raised rounded-lg border p-4">
                <div className="flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <div className="text-sm font-medium">
                      {activeRepository?.name ?? "Repository"}
                    </div>
                    <div className="text-muted-foreground truncate text-xs">
                      {activeRepository?.local_path ??
                        activeWorkspace?.root_path}
                    </div>
                  </div>
                  <Badge variant="secondary">
                    {activeRepository?.default_branch ?? "git repo"}
                  </Badge>
                </div>
              </section>

              <div className="grid grid-cols-3 gap-3">
                <SourceButton
                  active={source === "local-changes"}
                  description="Review staged, unstaged, and untracked local changes."
                  icon={Code2Icon}
                  label="Local changes"
                  onClick={() => setSource("local-changes")}
                />
                <SourceButton
                  active={source === "branch-compare"}
                  description="Compare two refs in the selected local repository."
                  icon={GitBranchIcon}
                  label="Branch compare"
                  onClick={() => setSource("branch-compare")}
                />
                <SourceButton
                  active={source === "github"}
                  description="Fetch PR metadata and diff from GitHub."
                  icon={GitPullRequestIcon}
                  label="GitHub PR"
                  onClick={() => setSource("github")}
                />
              </div>

              <section className="bg-surface-raised rounded-lg border p-4">
                {source === "github" && (
                  <div className="flex flex-col gap-2">
                    <label className="text-sm font-medium" htmlFor="github-url">
                      Pull request URL
                    </label>
                    <Input
                      id="github-url"
                      placeholder="https://github.com/owner/repo/pull/123"
                      value={githubUrl}
                      onChange={(event) => setGitHubUrl(event.target.value)}
                    />
                  </div>
                )}

                {source === "branch-compare" && (
                  <div className="grid grid-cols-2 gap-3">
                    <div className="flex flex-col gap-2">
                      <label className="text-sm font-medium" htmlFor="base-ref">
                        Base ref
                      </label>
                      <Input
                        id="base-ref"
                        value={baseRef}
                        onChange={(event) => setBaseRef(event.target.value)}
                      />
                    </div>
                    <div className="flex flex-col gap-2">
                      <label className="text-sm font-medium" htmlFor="head-ref">
                        Head ref
                      </label>
                      <Input
                        id="head-ref"
                        value={headRef}
                        onChange={(event) => setHeadRef(event.target.value)}
                      />
                    </div>
                  </div>
                )}

                {source === "local-changes" && (
                  <div className="flex items-start gap-3">
                    <div className="bg-muted flex size-8 shrink-0 items-center justify-center rounded-lg">
                      <Code2Icon />
                    </div>
                    <div className="min-w-0">
                      <div className="text-sm font-medium">
                        Snapshot current working tree
                      </div>
                      <p className="text-muted-foreground mt-1 text-sm">
                        Captures tracked and untracked local changes through the
                        backend git collector with existing output limits.
                      </p>
                    </div>
                  </div>
                )}
              </section>

              {(localError || snapshot.status === "error") && (
                <ErrorState
                  title="Could not create snapshot"
                  description={
                    localError ||
                    (snapshot.status === "error"
                      ? snapshot.error.message
                      : undefined)
                  }
                />
              )}

              <div className="flex justify-end">
                <Button
                  disabled={snapshot.status === "loading"}
                  onClick={submit}
                >
                  {snapshot.status === "loading"
                    ? "Creating snapshot..."
                    : "Continue to configure"}
                  <ArrowUpIcon data-icon="inline-end" />
                </Button>
              </div>
            </>
          )}
        </div>
      </ScrollArea>
    </section>
  );
}

function SourceButton({
  active,
  description,
  icon: Icon,
  label,
  onClick,
}: {
  active: boolean;
  description: string;
  icon: LucideIcon;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      className={cn(
        "bg-surface-raised hover:bg-surface flex min-h-28 flex-col gap-2 rounded-lg border p-4 text-left transition-colors",
        active && "border-foreground ring-foreground/10 ring-2",
      )}
      type="button"
      onClick={onClick}
    >
      <Icon />
      <span className="text-sm font-medium">{label}</span>
      <span className="text-muted-foreground text-xs leading-5">
        {description}
      </span>
    </button>
  );
}

function ConfigureReviewScreen({
  activeRepository,
  activeWorkspace,
  agentConfigs,
  changedFiles,
  client,
  onReviewStarted,
  snapshot,
}: {
  activeRepository?: Repository;
  activeWorkspace?: Workspace;
  agentConfigs: Loadable<AgentConfig[]>;
  changedFiles: Loadable<ChangedFile[]>;
  client: ApiClient | null;
  onReviewStarted: (session: ReviewSession) => void;
  snapshot: Loadable<Snapshot>;
}) {
  const [reviewDepth, setReviewDepth] = useState<"quick" | "standard" | "deep">(
    "standard",
  );
  const [runtimeLimitSeconds, setRuntimeLimitSeconds] = useState(1800);
  const [focusPrompt, setFocusPrompt] = useState("");
  const [startState, setStartState] =
    useState<Loadable<ReviewSession>>(idleApiState());
  const [selectedAgentIds, setSelectedAgentIds] = useState<Set<string> | null>(
    null,
  );
  const [contextPolicy, setContextPolicy] = useState<ReviewContextPolicy>({
    include_prompt_material: true,
    include_changed_code: true,
    include_related_call_sites: true,
    include_related_tests: true,
    include_project_conventions: true,
    include_prior_comments: true,
    include_prior_decisions: true,
    redact_secrets: true,
    local_only_paths: [],
    max_tokens: 18_000,
    max_items: 200,
  });

  const safeAgents = useMemo(
    () =>
      agentConfigs.status === "success"
        ? agentConfigs.data.filter(
            (agent) => agent.enabled && !agent.capabilities.can_write,
          )
        : [],
    [agentConfigs],
  );

  const effectiveSelectedAgentIds = useMemo(
    () => selectedAgentIds ?? new Set(safeAgents.map((agent) => agent.id)),
    [safeAgents, selectedAgentIds],
  );

  const visibleChangedFiles = useMemo(
    () =>
      changedFiles.status === "success"
        ? changedFiles.data.slice(0, MAX_CHANGED_FILES_RENDERED)
        : [],
    [changedFiles],
  );
  const hiddenChangedFiles =
    changedFiles.status === "success"
      ? Math.max(changedFiles.data.length - visibleChangedFiles.length, 0)
      : 0;
  const localOnlyPaths = contextPolicy.local_only_paths ?? [];
  const externalAgentCount = safeAgents.filter(
    (agent) => agentEgress(agent) === "external",
  ).length;

  async function startReview() {
    if (
      !client ||
      !activeWorkspace ||
      snapshot.status !== "success" ||
      effectiveSelectedAgentIds.size === 0 ||
      !Number.isFinite(runtimeLimitSeconds) ||
      runtimeLimitSeconds < 60
    ) {
      setStartState(
        errorApiState(
          new Error(
            "Choose a snapshot, at least one review-safe agent, and a runtime limit of 60 seconds or more.",
          ),
        ),
      );
      return;
    }

    setStartState(loadingApiState());
    const created = await loadApiResource(() =>
      client.createReviewSession({
        workspace_id: activeWorkspace.id,
        snapshot_id: snapshot.data.id,
        title: snapshotTitle(snapshot.data, activeRepository),
        review_depth: reviewDepth,
        focus_prompt: focusPrompt.trim() || undefined,
        agent_config_ids: Array.from(effectiveSelectedAgentIds),
        runtime_limit_seconds: runtimeLimitSeconds,
        context_policy: contextPolicy,
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
    <section className="flex min-w-0 flex-col">
      <ScrollArea className="flex-1 px-6 py-5">
        <div className="mx-auto flex max-w-5xl flex-col gap-5">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <h1 className="text-xl font-semibold">Configure review</h1>
              <p className="text-muted-foreground mt-1 truncate text-sm">
                {snapshot.status === "success"
                  ? snapshotTitle(snapshot.data, activeRepository)
                  : "Snapshot details are loading"}
              </p>
            </div>
            <Badge variant="secondary">
              {snapshot.status === "success"
                ? `${snapshot.data.changed_file_count ?? 0} files`
                : "snapshot"}
            </Badge>
          </div>

          {snapshot.status === "error" && (
            <ErrorState
              title="Snapshot failed"
              description={snapshot.error.message}
            />
          )}

          <div className="grid grid-cols-[minmax(0,1.1fr)_minmax(320px,0.9fr)] gap-4">
            <section className="bg-surface-raised rounded-lg border">
              <div className="flex items-center justify-between border-b px-3 py-2">
                <span className="text-sm font-medium">Changed files</span>
                {changedFiles.status === "success" && (
                  <Badge variant="secondary">
                    {changedFiles.data.length} total
                  </Badge>
                )}
              </div>
              <div className="max-h-[420px] overflow-y-auto">
                {changedFiles.status === "loading" && (
                  <LoadingRows rows={4} className="p-4" />
                )}
                {changedFiles.status === "error" && (
                  <ErrorState
                    className="m-3"
                    title="Changed files unavailable"
                    description={changedFiles.error.message}
                  />
                )}
                {changedFiles.status === "success" &&
                  visibleChangedFiles.map((file) => (
                    <ChangedFileRow key={file.id} file={file} />
                  ))}
                {changedFiles.status === "success" &&
                  visibleChangedFiles.length === 0 && (
                    <EmptyState
                      className="border-0 p-6"
                      title="No changed files"
                      description="The selected snapshot did not contain reviewable changed files."
                      icon={InboxIcon}
                    />
                  )}
                {hiddenChangedFiles > 0 && (
                  <div className="text-muted-foreground border-t px-3 py-2 text-xs">
                    {hiddenChangedFiles} more files hidden in this preview.
                    Filters and virtualized diff browsing arrive in later
                    screens.
                  </div>
                )}
              </div>
            </section>

            <div className="flex min-w-0 flex-col gap-4">
              <section className="bg-surface-raised rounded-lg border p-4">
                <div className="mb-3 text-sm font-medium">Agents</div>
                {agentConfigs.status === "loading" && <LoadingRows rows={3} />}
                {agentConfigs.status === "error" && (
                  <ErrorState
                    title="Agents unavailable"
                    description={agentConfigs.error.message}
                  />
                )}
                {agentConfigs.status === "success" &&
                  safeAgents.length === 0 && (
                    <EmptyState
                      className="border-0 p-2"
                      title="No review-safe agents"
                      description="Enable at least one read-only CLI agent in settings before starting."
                      icon={TerminalIcon}
                    />
                  )}
                <div className="flex flex-col gap-2">
                  {safeAgents.map((agent) => (
                    <label
                      key={agent.id}
                      className="hover:bg-surface flex items-center gap-3 rounded-md px-2 py-2 text-sm"
                    >
                      <Checkbox
                        checked={effectiveSelectedAgentIds.has(agent.id)}
                        onCheckedChange={(checked) => {
                          const next = new Set(effectiveSelectedAgentIds);
                          if (checked === true) {
                            next.add(agent.id);
                          } else {
                            next.delete(agent.id);
                          }
                          setSelectedAgentIds(next);
                        }}
                      />
                      <span className="min-w-0 flex-1 truncate">
                        {agent.name}
                      </span>
                      <Badge
                        variant={
                          agentEgress(agent) === "local"
                            ? "outline"
                            : "secondary"
                        }
                      >
                        {agentProvider(agent)}
                      </Badge>
                    </label>
                  ))}
                </div>
              </section>

              <section className="bg-surface-raised rounded-lg border p-4">
                <div className="mb-3 text-sm font-medium">Runtime</div>
                <div className="grid grid-cols-3 gap-2">
                  {(["quick", "standard", "deep"] as const).map((depth) => (
                    <Button
                      key={depth}
                      variant={reviewDepth === depth ? "default" : "outline"}
                      onClick={() => setReviewDepth(depth)}
                    >
                      {depth}
                    </Button>
                  ))}
                </div>
                <label className="mt-3 flex flex-col gap-2 text-sm">
                  Runtime limit seconds
                  <Input
                    min={60}
                    step={60}
                    type="number"
                    value={runtimeLimitSeconds}
                    onChange={(event) =>
                      setRuntimeLimitSeconds(Number(event.target.value))
                    }
                  />
                </label>
              </section>

              <section className="bg-surface-raised rounded-lg border p-4">
                <div className="mb-3 flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <div className="text-sm font-medium">Context policy</div>
                    <div className="text-muted-foreground mt-1 text-xs">
                      Explicitly maps to the backend review context schema.
                    </div>
                  </div>
                  <Badge
                    variant={externalAgentCount > 0 ? "secondary" : "outline"}
                  >
                    {externalAgentCount > 0
                      ? `${externalAgentCount} external`
                      : "local only"}
                  </Badge>
                </div>
                <div className="grid grid-cols-2 gap-2">
                  {(
                    [
                      "include_prompt_material",
                      "include_changed_code",
                      "include_related_call_sites",
                      "include_related_tests",
                      "include_project_conventions",
                      "include_prior_comments",
                      "include_prior_decisions",
                      "redact_secrets",
                    ] as const
                  ).map((key) => (
                    <PolicySwitch
                      key={key}
                      checked={Boolean(contextPolicy[key])}
                      label={formatPolicyLabel(key)}
                      onCheckedChange={(checked) =>
                        setContextPolicy((current) => ({
                          ...current,
                          [key]: checked,
                        }))
                      }
                    />
                  ))}
                </div>

                <div className="mt-4 grid grid-cols-2 gap-3">
                  <label className="flex flex-col gap-2 text-xs font-medium">
                    Token budget
                    <Input
                      min={1000}
                      step={1000}
                      type="number"
                      value={contextPolicy.max_tokens ?? 18_000}
                      onChange={(event) =>
                        setContextPolicy((current) => ({
                          ...current,
                          max_tokens: Number(event.target.value),
                        }))
                      }
                    />
                  </label>
                  <label className="flex flex-col gap-2 text-xs font-medium">
                    Item budget
                    <Input
                      min={1}
                      step={10}
                      type="number"
                      value={contextPolicy.max_items ?? 200}
                      onChange={(event) =>
                        setContextPolicy((current) => ({
                          ...current,
                          max_items: Number(event.target.value),
                        }))
                      }
                    />
                  </label>
                </div>

                <div className="mt-4 rounded-md border">
                  <div className="flex items-center justify-between gap-3 border-b px-3 py-2">
                    <div className="min-w-0">
                      <div className="text-xs font-medium">
                        Local-only files
                      </div>
                      <div className="text-muted-foreground mt-1 text-xs">
                        These paths are omitted from external-agent context.
                      </div>
                    </div>
                    <Badge variant="outline">{localOnlyPaths.length}</Badge>
                  </div>
                  <div className="max-h-44 overflow-y-auto p-2">
                    {visibleChangedFiles.length === 0 && (
                      <div className="text-muted-foreground px-1 py-2 text-xs">
                        Changed files will appear here after snapshot loading.
                      </div>
                    )}
                    {visibleChangedFiles.map((file) => (
                      <label
                        key={file.id}
                        className="hover:bg-surface flex items-center gap-2 rounded-md px-2 py-1.5 text-xs"
                      >
                        <Checkbox
                          checked={localOnlyPaths.includes(file.path)}
                          onCheckedChange={(checked) => {
                            setContextPolicy((current) => ({
                              ...current,
                              local_only_paths: nextLocalOnlyPaths(
                                current.local_only_paths ?? [],
                                file.path,
                                checked === true,
                              ),
                            }));
                          }}
                        />
                        <span className="min-w-0 flex-1 truncate font-mono">
                          {file.path}
                        </span>
                      </label>
                    ))}
                  </div>
                </div>
              </section>
            </div>
          </div>

          <section className="bg-surface-raised rounded-lg border p-4">
            <label className="flex flex-col gap-2 text-sm font-medium">
              Focus prompt
              <InputGroup className="min-h-24 items-stretch">
                <InputGroupTextarea
                  className="min-h-20"
                  placeholder="Optional review focus, risk areas, or files to prioritize..."
                  value={focusPrompt}
                  onChange={(event) => setFocusPrompt(event.target.value)}
                />
              </InputGroup>
            </label>
          </section>

          {startState.status === "error" && (
            <ErrorState
              title="Could not start review"
              description={startState.error.message}
            />
          )}

          <div className="flex justify-end">
            <Button
              disabled={startState.status === "loading"}
              onClick={startReview}
            >
              {startState.status === "loading"
                ? "Starting review..."
                : "Start review"}
              <ArrowUpIcon data-icon="inline-end" />
            </Button>
          </div>
        </div>
      </ScrollArea>
    </section>
  );
}

function ChangedFileRow({ file }: { file: ChangedFile }) {
  return (
    <div className="flex items-center justify-between gap-3 border-b px-3 py-2 text-sm last:border-b-0">
      <div className="min-w-0">
        <div className="truncate font-mono text-xs">{file.path}</div>
        <div className="mt-1 flex items-center gap-1">
          {file.is_generated && <Badge variant="outline">generated</Badge>}
          {file.is_binary && <Badge variant="outline">binary</Badge>}
          {file.is_excluded && <Badge variant="outline">excluded</Badge>}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2 text-xs">
        <Badge variant="outline">{file.status}</Badge>
        <span className="text-success">+{file.additions}</span>
        <span className="text-destructive">-{file.deletions}</span>
      </div>
    </div>
  );
}

function PolicySwitch({
  checked,
  label,
  onCheckedChange,
}: {
  checked: boolean;
  label: string;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <label className="hover:bg-surface flex items-center justify-between gap-3 rounded-md px-2 py-1.5 text-xs">
      <span className="min-w-0 truncate">{label}</span>
      <Switch checked={checked} size="sm" onCheckedChange={onCheckedChange} />
    </label>
  );
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

function AgentSettingsScreen({
  client,
  onBack,
}: {
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

  const presetList = presets.status === "success" ? presets.data : [];
  const configList = configs.status === "success" ? configs.data : [];
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
        return;
      }

      setPresets(loadingApiState());
      setConfigs(loadingApiState());
      void Promise.all([
        loadApiResource(() => client.listAgentPresets()),
        loadApiResource(() => client.listAgentConfigs()),
      ]).then(([presetState, configState]) => {
        if (canceled) {
          return;
        }
        setPresets(presetState);
        setConfigs(configState);

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
  }, [client]);

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
    <section className="flex min-w-0 flex-col">
      <ScrollArea className="flex-1 px-6 py-5">
        <div className="mx-auto flex max-w-6xl flex-col gap-5">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <h1 className="text-xl font-semibold">Agent settings</h1>
              <p className="text-muted-foreground mt-1 text-sm">
                Configure local CLI reviewers, presets, health checks, and
                review-safe capabilities.
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
                  Presets
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
                  <span className="text-sm font-medium">Configured</span>
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
                    One argument per line. Use {"{{prompt}}"} only for arg-mode
                    CLIs.
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
                      repo_root
                    </NativeSelectOption>
                    <NativeSelectOption value="workspace_root">
                      workspace_root
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
                        promptDelivery: event.target.value as PromptDelivery,
                      }))
                    }
                  >
                    <NativeSelectOption value="stdin">stdin</NativeSelectOption>
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

                <div className="col-span-2 grid grid-cols-3 gap-3">
                  <AgentSettingSwitch
                    checked={form.enabled}
                    label="Enabled"
                    onCheckedChange={(checked) =>
                      setForm((current) => ({ ...current, enabled: checked }))
                    }
                  />
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
        </div>
      </ScrollArea>
    </section>
  );
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

function nextLocalOnlyPaths(paths: string[], path: string, enabled: boolean) {
  const clean = path.trim();
  if (!clean) {
    return paths;
  }
  const next = enabled
    ? [...paths, clean]
    : paths.filter((candidate) => candidate !== clean);
  return Array.from(new Set(next)).sort((left, right) =>
    left.localeCompare(right),
  );
}

type AgentVisibilitySource = {
  capabilities: AgentConfigInput["capabilities"];
};

function agentProvider(agent: AgentVisibilitySource) {
  const provider = agent.capabilities.metadata?.provider;
  return typeof provider === "string" && provider.trim() ? provider : "local";
}

function agentEgress(agent: AgentVisibilitySource) {
  const egress = agent.capabilities.metadata?.egress;
  return typeof egress === "string" && egress.trim() ? egress : "local";
}

function formatHealthMetadata(metadata: Record<string, unknown>) {
  return ["version", "resolved_path", "path", "error"]
    .map((key) => [key, metadata[key]] as const)
    .filter(([, value]) => typeof value === "string" && value.trim())
    .map(([key, value]) => [key, value as string] as const);
}

function Sidebar({
  activeSessionId,
  activeWorkspaceId,
  backendStatus,
  repositoryOpenState,
  reviewSessions,
  workspaces,
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
  repositoryOpenState: Loadable<OpenRepositoryResponse>;
  reviewSessions: Loadable<ReviewSession[]>;
  workspaces: Loadable<Workspace[]>;
  onOpenAgentSettings: () => void;
  onOpenNewThread: () => void;
  onOpenRepository: () => void;
  onOpenSearch: () => void;
  onSelectReviewSession: (session: ReviewSession) => void;
  onSelectWorkspace: (workspaceId: string) => void;
}) {
  const workspaceList = workspaces.status === "success" ? workspaces.data : [];
  const sessionList =
    reviewSessions.status === "success"
      ? reviewSessions.data.slice(0, MAX_SIDEBAR_SESSIONS)
      : [];

  return (
    <>
      <div className="flex h-12 items-center gap-2 px-4">
        <div className="bg-destructive/80 size-3 rounded-full" />
        <div className="bg-warning/80 size-3 rounded-full" />
        <div className="bg-success/80 size-3 rounded-full" />
      </div>

      <div className="flex items-center gap-2 px-4 pb-4">
        <div className="bg-primary text-primary-foreground flex size-8 items-center justify-center rounded-lg">
          <BotIcon />
        </div>
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold">cocode</p>
          <p className="text-sidebar-muted truncate text-xs">
            Local review cockpit
          </p>
        </div>
      </div>

      <nav className="flex flex-col gap-1 px-2">
        <SidebarNavButton
          icon={PlusIcon}
          label="New thread"
          onClick={onOpenNewThread}
        />
        <SidebarNavButton
          icon={FolderOpenIcon}
          label={
            repositoryOpenState.status === "loading"
              ? "Opening repo..."
              : "Open repo"
          }
          onClick={onOpenRepository}
        />
        <SidebarNavButton
          icon={SearchIcon}
          label="Search"
          onClick={onOpenSearch}
        />
        <SidebarNavButton icon={SparklesIcon} label="Plugins" />
        <SidebarNavButton icon={ClockIcon} label="Automations" />
      </nav>

      <SidebarSection
        title="Threads"
        action={<SquareIcon className="opacity-70" />}
      >
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
        {reviewSessions.status === "success" && sessionList.length === 0 && (
          <div className="text-sidebar-muted px-2 py-1 text-xs">
            No review threads yet
          </div>
        )}
        {sessionList.map((session) => (
          <SidebarNavButton
            key={session.id}
            label={session.title}
            meta={formatRelativeAge(session.updated_at)}
            active={session.id === activeSessionId}
            onClick={() => onSelectReviewSession(session)}
          />
        ))}
      </SidebarSection>

      <SidebarSection
        title="Workspaces"
        action={<SquareIcon className="opacity-70" />}
      >
        {workspaces.status === "loading" && (
          <div className="text-sidebar-muted px-2 py-1 text-xs">
            Loading workspaces...
          </div>
        )}
        {workspaces.status === "error" && (
          <div className="text-destructive px-2 py-1 text-xs">
            {workspaces.error.message}
          </div>
        )}
        {workspaces.status === "success" && workspaceList.length === 0 && (
          <SidebarNavButton
            icon={FolderOpenIcon}
            label="Open local repo"
            onClick={onOpenRepository}
          />
        )}
        {workspaceList.map((workspace) => (
          <SidebarNavButton
            key={workspace.id}
            active={workspace.id === activeWorkspaceId}
            icon={GitBranchIcon}
            label={workspace.name}
            meta={workspace.default_repo_id ? "active" : undefined}
            onClick={() => onSelectWorkspace(workspace.id)}
          />
        ))}
      </SidebarSection>

      {repositoryOpenState.status === "error" && (
        <div className="text-destructive px-4 pt-3 text-xs">
          {repositoryOpenState.error.message}
        </div>
      )}

      <div className="mt-auto flex flex-col gap-1 p-2">
        <Separator className="-mx-2 mb-1" />
        <SidebarNavButton
          icon={SettingsIcon}
          label="Settings"
          onClick={onOpenAgentSettings}
        />
        <div className="text-sidebar-muted px-2 pt-1 pb-2 text-xs">
          Backend {backendStatus}
        </div>
      </div>
    </>
  );
}

function TopNav({
  activeRepository,
  activeSession,
  activeWorkspace,
  isOpeningRepository,
  onOpenRepository,
  onOpenSearch,
}: {
  activeRepository?: Repository;
  activeSession?: ReviewSession;
  activeWorkspace?: Workspace;
  isOpeningRepository: boolean;
  onOpenRepository: () => void;
  onOpenSearch: () => void;
}) {
  const title =
    activeSession?.title ??
    activeRepository?.name ??
    activeWorkspace?.name ??
    "Open a repository";
  const description =
    activeRepository?.remote_url ??
    activeRepository?.local_path ??
    activeWorkspace?.root_path ??
    "Select a local git repository to begin";

  return (
    <PaneHeader
      icon={GitPullRequestIcon}
      title={title}
      description={description}
      actions={
        <>
          <Button size="sm" variant="outline" onClick={onOpenSearch}>
            <SearchIcon data-icon="inline-start" />
            Search
          </Button>
          <Button
            disabled={isOpeningRepository}
            size="sm"
            variant="outline"
            onClick={onOpenRepository}
          >
            <FolderOpenIcon data-icon="inline-start" />
            {activeRepository ? "Open repo" : "Select repo"}
          </Button>
          <Button size="sm" variant="outline">
            <PanelRightIcon data-icon="inline-start" />
            Ask all agents
          </Button>
          <CommitDropdown />
          <Badge variant="secondary">+938 -664</Badge>
          <TooltipIconButton
            label="Notifications"
            size="icon-sm"
            variant="ghost"
          >
            <BellIcon />
          </TooltipIconButton>
          <TooltipIconButton
            label="More actions"
            size="icon-sm"
            variant="ghost"
          >
            <MoreHorizontalIcon />
          </TooltipIconButton>
        </>
      }
    />
  );
}

function CommitDropdown() {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" variant="outline">
          Commit
          <ChevronDownIcon data-icon="inline-end" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel>Review actions</DropdownMenuLabel>
        <DropdownMenuGroup>
          <DropdownMenuItem>Copy selected packet</DropdownMenuItem>
          <DropdownMenuItem>Create draft review</DropdownMenuItem>
          <DropdownMenuItem>Open GitHub preview</DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem variant="destructive">Cancel review</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ReviewThread({
  agentConfigs,
  apiSession,
  backendDetail,
  client,
  session,
}: {
  agentConfigs: Loadable<AgentConfig[]>;
  apiSession: Loadable<ApiSessionResponse>;
  backendDetail: string;
  client: ApiClient | null;
  session?: ReviewSession;
}) {
  const [activeTab, setActiveTab] = useState("chat");
  const live = useReviewSessionLiveData(client, session);

  const agentList = agentConfigs.status === "success" ? agentConfigs.data : [];
  const selectedAgents =
    session?.agents
      .map((sessionAgent) =>
        agentList.find((agent) => agent.id === sessionAgent.agent_config_id),
      )
      .filter((agent): agent is AgentConfig => Boolean(agent)) ?? [];

  return (
    <section className="flex min-w-0 flex-col">
      <ScrollArea className="flex-1 px-6 py-5">
        <div className="mx-auto flex max-w-5xl flex-col gap-5">
          {apiSession.status === "loading" && (
            <LoadingRows rows={2} className="rounded-lg border p-4" />
          )}
          {apiSession.status === "error" && (
            <ErrorState
              title="Backend connection failed"
              description={apiSession.error.message}
            />
          )}

          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <h1 className="truncate text-xl font-semibold">
                {session?.title ?? "Review thread"}
              </h1>
              <p className="text-muted-foreground mt-1 text-sm">
                {session
                  ? `${session.review_depth} review • ${session.status}`
                  : "Create or select a review session to stream live progress."}
              </p>
            </div>
            {session && (
              <ReviewControlButtons
                client={client}
                onSessionUpdated={live.setSession}
                session={live.session ?? session}
              />
            )}
          </div>

          <Tabs value={activeTab} onValueChange={setActiveTab}>
            <TabsList variant="line">
              <TabsTrigger value="chat">Chat</TabsTrigger>
              <TabsTrigger value="details">Review details</TabsTrigger>
              <TabsTrigger value="findings">Findings</TabsTrigger>
              <TabsTrigger value="publish">Publish</TabsTrigger>
            </TabsList>

            <TabsContent value="chat" className="mt-4 flex flex-col gap-4">
              {session ? (
                <>
                  <ReviewRunningPanel
                    agents={selectedAgents}
                    summary={live.summary}
                    session={live.session ?? session}
                  />
                  <EarlyFindingsPanel findings={live.findings} />
                </>
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

            <TabsContent value="details" className="mt-4">
              <ReviewEventTimeline events={live.events} />
            </TabsContent>

            <TabsContent value="findings" className="mt-4">
              <ReviewFindingsBoard
                client={client}
                findings={live.findings}
                session={live.session ?? session}
              />
            </TabsContent>

            <TabsContent value="publish" className="mt-4">
              <EmptyState
                title="Publish preview comes next"
                description="Accepted findings will feed the GitHub preview and copy packet screens."
                icon={CopyIcon}
              />
            </TabsContent>
          </Tabs>
        </div>
      </ScrollArea>

      <MessageComposer
        agentConfigs={agentConfigs}
        backendDetail={backendDetail}
        disabled={!session}
      />
    </section>
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

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (!canceled) {
        setSession(initialSession);
        setEvents([]);
      }
    });
    return () => {
      canceled = true;
    };
  }, [initialSession?.id, initialSession]);

  useEffect(() => {
    if (!client || !initialSession) {
      return;
    }
    const api = client;
    const sessionId = initialSession.id;
    let canceled = false;

    async function load() {
      setSummary(loadingApiState());
      setFindings(loadingApiState());
      const [summaryState, findingsState] = await Promise.all([
        loadApiResource(() => api.reviewSessionSummary(sessionId)),
        loadApiResource(() => api.listFindings(sessionId)),
      ]);
      if (canceled) {
        return;
      }
      setSummary(summaryState);
      setFindings(findingsState);
    }

    queueMicrotask(() => {
      if (!canceled) {
        void load();
      }
    });
    const interval = window.setInterval(() => void load(), 2500);
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
    void api
      .streamReviewEvents(sessionId, {
        signal: controller.signal,
        onEvent: (event) => {
          setEvents((current) => appendBoundedEvent(current, event));
          if (
            event.type.startsWith("ReviewSession") ||
            event.type.startsWith("Finding") ||
            event.type.startsWith("AgentRun")
          ) {
            void loadApiResource(() =>
              api.reviewSessionSummary(sessionId),
            ).then(setSummary);
            if (event.type.includes("Finding")) {
              void loadApiResource(() => api.listFindings(sessionId)).then(
                setFindings,
              );
            }
          }
        },
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
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
    return () => controller.abort();
  }, [client, initialSession]);

  return { events, findings, session, setSession, summary };
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

function ReviewRunningPanel({
  agents,
  session,
  summary,
}: {
  agents: AgentConfig[];
  session: ReviewSession;
  summary: Loadable<ReviewSessionSummary>;
}) {
  const data = summary.status === "success" ? summary.data : undefined;
  const progress = data?.progress_percent ?? statusProgress(session.status);
  const activeAgents = data?.active_agents ?? 0;
  const agentCounts = data?.agent_status_counts ?? {};
  const agentCards =
    agents.length > 0
      ? agents.map((agent) => ({ id: agent.id, name: agent.name }))
      : session.agents.map((agent) => ({
          id: agent.id,
          name: agent.agent_config_id,
        }));

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="text-sm font-medium">Review running</div>
          <div className="text-muted-foreground mt-1 truncate text-xs">
            {data?.phase ?? "workflow"} • {data?.phase_status ?? session.status}
          </div>
        </div>
        <Badge variant="secondary">{session.status}</Badge>
      </div>
      <div className="flex flex-col gap-4 p-4">
        {summary.status === "error" && (
          <ErrorState
            title="Summary unavailable"
            description={summary.error.message}
          />
        )}
        <div className="flex items-center gap-3">
          <Progress value={progress} />
          <span className="w-12 text-right text-xs">{progress}%</span>
        </div>
        <div className="grid grid-cols-4 gap-3">
          <RunMetric label="Files" value={formatFileScan(data)} />
          <RunMetric
            label="Runs"
            value={String(data?.agent_runs_total ?? agents.length)}
          />
          <RunMetric label="Active" value={String(activeAgents)} />
          <RunMetric label="Findings" value={formatFindingCount(data)} />
        </div>
        <div className="grid grid-cols-2 gap-2">
          {agentCards.map((agent, index) => {
            const status =
              agentCounts.succeeded && index < Number(agentCounts.succeeded)
                ? "succeeded"
                : activeAgents > index
                  ? "running"
                  : "queued";
            return (
              <div
                key={agent.id}
                className="bg-background flex items-center gap-3 rounded-md border px-3 py-2"
              >
                <BotIcon />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-medium">
                    {agent.name}
                  </div>
                  <div className="text-muted-foreground text-xs">{status}</div>
                </div>
                <Badge variant="outline">{status}</Badge>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}

function RunMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-background rounded-md border px-3 py-2">
      <div className="text-muted-foreground text-xs">{label}</div>
      <div className="mt-1 truncate text-sm font-medium">{value}</div>
    </div>
  );
}

function EarlyFindingsPanel({
  findings,
}: {
  findings: Loadable<FindingListResponse>;
}) {
  const visibleFindings =
    findings.status === "success" ? findings.data.items.slice(0, 5) : [];

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
        <div className="flex items-center gap-2">
          <ShieldCheckIcon />
          <span className="text-sm font-medium">Early findings</span>
        </div>
        {findings.status === "success" && (
          <Badge variant="secondary">{findings.data.stats.total} total</Badge>
        )}
      </div>
      {findings.status === "loading" && (
        <LoadingRows rows={3} className="p-4" />
      )}
      {findings.status === "error" && (
        <ErrorState
          className="m-3"
          title="Findings unavailable"
          description={findings.error.message}
        />
      )}
      {findings.status === "success" && visibleFindings.length === 0 && (
        <EmptyState
          className="border-0 p-6"
          title="No findings yet"
          description="Findings will appear here as agents and the verifier emit evidence."
          icon={FileSearchIcon}
        />
      )}
      {visibleFindings.map((finding) => (
        <LiveFindingRow key={finding.id} finding={finding} />
      ))}
    </section>
  );
}

function ReviewEventTimeline({ events }: { events: ReviewEvent[] }) {
  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="text-sm font-medium">Event timeline</div>
          <div className="text-muted-foreground mt-1 text-xs">
            Live SSE events with sequence IDs for replay/debugging.
          </div>
        </div>
        <Badge variant="secondary">{events.length}</Badge>
      </div>
      {events.length === 0 ? (
        <EmptyState
          className="border-0 p-6"
          title="No events yet"
          description="Start a review to stream workflow, agent, and finding events."
          icon={ClockIcon}
        />
      ) : (
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
      )}
    </section>
  );
}

function ReviewFindingsBoard({
  client,
  findings,
  session,
}: {
  client: ApiClient | null;
  findings: Loadable<FindingListResponse>;
  session?: ReviewSession;
}) {
  const [statusFilter, setStatusFilter] = useState<FindingStatusFilter>("all");
  const [severityFilter, setSeverityFilter] =
    useState<FindingSeverityFilter>("all");
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
  const [draftComment, setDraftComment] = useState("");
  const [boardReloadKey, setBoardReloadKey] = useState(0);
  const boardSessionId = session?.id;
  const [actionState, setActionState] = useState<{
    status: "idle" | "loading" | "success" | "error";
    findingId?: string;
    action?: string;
    message?: string;
  }>({ status: "idle" });

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
    client,
    debouncedQuery,
    severityFilter,
    statusFilter,
  ]);

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
    const nextDraft = selectedDetail.data.finding.draft_comment || "";
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
  const selectedFinding =
    selectedDetail.status === "success"
      ? selectedDetail.data.finding
      : listedFindings.find((finding) => finding.id === selectedFindingId);
  const selectedFindingDetail =
    selectedDetail.status === "success" ? selectedDetail.data : undefined;
  const selectedOutsideFilter = Boolean(
    selectedFinding &&
    listedFindings.length > 0 &&
    !listedFindings.some((finding) => finding.id === selectedFinding.id),
  );
  const hasFilters =
    statusFilter !== "all" || severityFilter !== "all" || query.trim() !== "";

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
      client.updateFindingDecision(finding.id, { decision, reason }),
    );
    if (state.status === "success") {
      setSelectedDetail(state);
      setSelectedFindingId(state.data.finding.id);
      setDismissReason("");
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
    const content = findingClipboardText(finding);
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

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="grid grid-cols-4 gap-3 border-b p-3">
        <RunMetric label="Total" value={String(stats?.total ?? 0)} />
        <RunMetric label="Filtered" value={String(stats?.filtered ?? 0)} />
        <RunMetric
          label="Needs triage"
          value={String(stats?.needs_triage ?? 0)}
        />
        <RunMetric
          label="Verified"
          value={String(stats?.by_verification.verified ?? 0)}
        />
      </div>

      <div className="flex flex-wrap items-center gap-2 border-b p-3">
        <div className="relative min-w-56 flex-1">
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
          className="w-40"
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
        <Button
          disabled={!hasFilters}
          size="sm"
          variant="outline"
          onClick={() => {
            setStatusFilter("all");
            setSeverityFilter("all");
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

      <div className="grid min-h-[480px] grid-cols-[minmax(0,1fr)_minmax(320px,0.8fr)]">
        <div className="min-w-0 border-r">
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

        <div className="min-w-0 p-4">
          {selectedDetail.status === "loading" && <LoadingRows rows={5} />}
          {selectedDetail.status === "error" && (
            <ErrorState
              title="Finding detail unavailable"
              description={selectedDetail.error.message}
            />
          )}
          {!selectedFinding && selectedDetail.status !== "loading" && (
            <EmptyState
              className="border-0"
              title="No finding selected"
              description="Select a finding to inspect evidence and actions."
              icon={FileSearchIcon}
            />
          )}
          {selectedFinding && selectedDetail.status !== "loading" && (
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
                  <TabsTrigger value="draft">Draft</TabsTrigger>
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

                <TabsContent value="draft" className="flex flex-col gap-2">
                  <Textarea
                    aria-label="Draft GitHub comment"
                    className="min-h-36"
                    value={draftComment}
                    onChange={(event) => setDraftComment(event.target.value)}
                  />
                  <div className="flex items-center justify-between gap-2">
                    <Badge variant="outline">
                      {selectedFindingDetail
                        ? `${selectedFindingDetail.candidates.length} candidates`
                        : `${selectedFinding.merged_from_count} merged`}
                    </Badge>
                    <Button
                      disabled={actionState.status === "loading"}
                      size="sm"
                      variant="outline"
                      onClick={() => void saveDraftComment()}
                    >
                      Save draft
                    </Button>
                  </div>
                </TabsContent>
              </Tabs>

              <Input
                aria-label="Dismissal reason"
                placeholder="Dismissal reason"
                value={dismissReason}
                onChange={(event) => setDismissReason(event.target.value)}
              />

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
                  Copy
                </Button>
                <Button
                  disabled={actionState.status === "loading"}
                  size="sm"
                  variant="outline"
                  onClick={() => void updateDecision("dismissed")}
                >
                  Dismiss
                </Button>
                {actionState.status === "success" && actionState.message && (
                  <span className="text-muted-foreground text-xs">
                    {actionState.message}
                  </span>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

function FindingCard({
  actionState,
  finding,
  onAccept,
  onCopy,
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
  onSelect: () => void;
  selected: boolean;
}) {
  const pending =
    actionState.status === "loading" && actionState.findingId === finding.id;
  return (
    <div
      className={cn(
        "hover:bg-surface flex w-full cursor-pointer items-start gap-3 border-b px-4 py-3 text-left last:border-b-0",
        selected && "bg-surface",
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
      <CircleIcon
        className={cn(
          "mt-1",
          finding.severity === "high"
            ? "text-destructive"
            : "text-muted-foreground",
        )}
      />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">
          {finding.canonical_claim}
        </div>
        <div className="text-muted-foreground mt-1 flex min-w-0 items-center gap-2 text-xs">
          <span className="truncate font-mono">
            {finding.primary_path || "no location"}
          </span>
          {finding.primary_start_line ? (
            <span>L{finding.primary_start_line}</span>
          ) : null}
        </div>
        {finding.evidence_summary && (
          <div className="text-muted-foreground mt-2 line-clamp-2 text-xs">
            {finding.evidence_summary}
          </div>
        )}
      </div>
      <div className="flex shrink-0 flex-col items-end gap-2">
        <div className="flex gap-1">
          <Badge
            variant={finding.severity === "high" ? "destructive" : "secondary"}
          >
            {finding.severity}
          </Badge>
          <Badge variant="outline">
            {formatDecisionLabel(finding.verification_status)}
          </Badge>
        </div>
        <div className="flex gap-1">
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
    </div>
  );
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
                {candidate.agent_run_id}
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

function CodeSnippetViewer({
  evidence,
  finding,
  onCopyPath,
}: {
  evidence: EvidenceItem[];
  finding: Finding;
  onCopyPath: () => void;
}) {
  const snippets = evidence
    .filter((item) => item.code_snippet && item.code_snippet.trim() !== "")
    .slice(0, 3);

  if (snippets.length === 0) {
    return (
      <div className="rounded-md border p-3">
        <div className="mb-3 flex items-center justify-between gap-2">
          <div className="text-xs font-medium">Primary location</div>
          <Button size="sm" variant="outline" onClick={onCopyPath}>
            <CopyIcon data-icon="inline-start" />
            Copy path
          </Button>
        </div>
        <p className="text-muted-foreground truncate font-mono text-xs">
          {formatFindingLocation(finding)}
        </p>
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
        <div key={item.id} className="overflow-hidden rounded-md border">
          <div className="bg-surface flex items-center justify-between gap-2 border-b px-3 py-2">
            <span className="truncate font-mono text-xs">
              {item.path || formatFindingLocation(finding)}
            </span>
            <Badge variant="outline">{item.kind}</Badge>
          </div>
          <div className="max-h-80 overflow-auto font-mono text-xs">
            {snippetLines(item).map((line) => (
              <div
                key={`${item.id}-${line.number}`}
                className="grid grid-cols-[48px_minmax(0,1fr)] border-b border-transparent leading-6"
              >
                <span className="text-muted-foreground pr-3 text-right select-none">
                  {line.number}
                </span>
                <span className="truncate px-3 whitespace-pre">
                  {line.text || " "}
                </span>
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

function EvidenceCardList({ detail }: { detail?: FindingDetailResponse }) {
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

function LiveFindingRow({ finding }: { finding: Finding }) {
  return (
    <button
      className="hover:bg-surface flex w-full items-start gap-3 border-b px-4 py-3 text-left last:border-b-0"
      type="button"
    >
      <CircleIcon
        className={cn(
          "mt-1",
          finding.severity === "high"
            ? "text-destructive"
            : "text-muted-foreground",
        )}
      />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">
          {finding.canonical_claim}
        </div>
        <div className="text-muted-foreground mt-1 flex min-w-0 items-center gap-2 text-xs">
          <span className="truncate font-mono">
            {finding.primary_path || "no location"}
          </span>
          {finding.primary_start_line ? (
            <span>L{finding.primary_start_line}</span>
          ) : null}
        </div>
        {finding.evidence_summary && (
          <div className="text-muted-foreground mt-2 line-clamp-2 text-xs">
            {finding.evidence_summary}
          </div>
        )}
      </div>
      <div className="flex shrink-0 gap-1">
        <Badge
          variant={finding.severity === "high" ? "destructive" : "secondary"}
        >
          {finding.severity}
        </Badge>
        <Badge variant="outline">{finding.verification_status}</Badge>
      </div>
    </button>
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

function MessageComposer({
  agentConfigs,
  backendDetail,
  disabled,
}: {
  agentConfigs?: Loadable<AgentConfig[]>;
  backendDetail: string;
  disabled?: boolean;
}) {
  const [mode, setMode] = useState("review");
  const [runtime, setRuntime] = useState("standard");
  const [reasoning, setReasoning] = useState("high");
  const [permission, setPermission] = useState("review-mode");
  const agents = agentConfigs?.status === "success" ? agentConfigs.data : [];
  const availableAgentCount = agents.filter(
    (agent) => agent.enabled && !agent.capabilities.can_write,
  ).length;

  return (
    <div className="bg-surface-raised border-t p-4">
      <div className="bg-background mx-auto max-w-5xl rounded-2xl border shadow-sm">
        <InputGroup className="min-h-24 items-stretch border-0">
          <InputGroupTextarea
            aria-label="Follow-up prompt"
            disabled={disabled}
            placeholder={
              disabled
                ? "Start a review before asking follow-up questions..."
                : "Ask a follow-up grounded in this review context..."
            }
            className="min-h-20"
          />
        </InputGroup>
        <div className="flex items-center justify-between border-t px-3 py-2">
          <div className="flex items-center gap-2">
            <Button disabled={disabled} size="sm" variant="ghost">
              <MessageSquareIcon data-icon="inline-start" />
              {mode}
            </Button>
            <ComposerDropdown
              label={runtime}
              onSelect={setRuntime}
              options={["quick", "standard", "deep"]}
            />
            <ComposerDropdown
              label={reasoning}
              onSelect={setReasoning}
              options={["low", "medium", "high"]}
            />
            <ComposerDropdown
              label={permission}
              onSelect={setPermission}
              options={["review-mode", "local-only"]}
            />
            <ComposerDropdown
              label={`${availableAgentCount} agents`}
              onSelect={setMode}
              options={["review", "finding follow-up"]}
            />
          </div>
          <InputGroupButton
            disabled
            size="icon-sm"
            aria-label="Review-level follow-up submit is not available yet"
          >
            <ArrowUpIcon />
          </InputGroupButton>
        </div>
      </div>
      <div className="text-muted-foreground mx-auto mt-2 max-w-5xl truncate text-center text-xs">
        Review-level follow-up submit needs a backend endpoint; finding-scoped
        follow-up is wired in a later screen. {backendDetail}
      </div>
    </div>
  );
}

function ComposerDropdown({
  label,
  onSelect,
  options,
}: {
  label: string;
  onSelect?: (value: string) => void;
  options?: string[];
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
          {(options ?? ["Fast", "Balanced", "Deep"]).map((option) => (
            <DropdownMenuItem key={option} onSelect={() => onSelect?.(option)}>
              {option}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ReviewPane() {
  return (
    <aside className="bg-surface-raised min-w-0 border-l">
      <div className="flex h-full flex-col">
        <PaneHeader
          icon={Code2Icon}
          title="Review"
          actions={
            <TooltipIconButton
              label="Pause review"
              size="icon-sm"
              variant="ghost"
            >
              <PauseIcon />
            </TooltipIconButton>
          }
        />

        <Tabs defaultValue="diff" className="min-h-0 flex-1 gap-0">
          <div className="flex items-center justify-between gap-2 border-b px-4 py-2">
            <TabsList variant="line">
              <TabsTrigger value="diff">Diff</TabsTrigger>
              <TabsTrigger value="evidence">Evidence</TabsTrigger>
              <TabsTrigger value="publish">Publish</TabsTrigger>
            </TabsList>
            <div className="flex min-w-0 items-center gap-2 text-xs">
              <Badge variant="outline">Unstaged</Badge>
              <Badge variant="secondary">4</Badge>
              <span className="text-muted-foreground truncate">billing.go</span>
            </div>
          </div>

          <TabsContent value="diff" className="min-h-0">
            <ScrollArea className="h-full font-mono text-xs">
              <CodeLine
                num={2}
                text="@@ RegisterBillingRoutes @@"
                tone="context"
              />
              <CodeLine
                num={3}
                text="func RegisterBillingRoutes(r *mux.Router) {"
              />
              <CodeLine
                num={4}
                text={'  r.HandleFunc("/billing/invoices", listInvoices)'}
                tone="removed"
              />
              <CodeLine
                num={5}
                text={'  protected := r.PathPrefix("/api").Subrouter()'}
                tone="added"
              />
              <CodeLine
                num={6}
                text="  protected.Use(middleware.RequireAuth())"
                tone="added"
              />
              <CodeLine
                num={7}
                text={
                  '  protected.HandleFunc("/billing/invoices", listInvoices)'
                }
                tone="added"
              />
              <CodeLine
                num={8}
                text={
                  '  protected.HandleFunc("/billing/payouts", createPayout)'
                }
                tone="added"
              />
              <CodeLine num={9} text="}" />
              <CodeLine num={10} text="" />
              <CodeLine
                num={11}
                text="@@ handlers/payouts.go @@"
                tone="context"
              />
              <CodeLine
                num={12}
                text="func createPayout(w http.ResponseWriter, r *http.Request) {"
              />
              <CodeLine
                num={13}
                text={'  user := r.Context().Value("user").(*User)'}
              />
              <CodeLine num={14} text="  // payout logic..." />
              <CodeLine num={15} text="}" />
            </ScrollArea>
          </TabsContent>

          <TabsContent value="evidence" className="min-h-0">
            <div className="p-4">
              <ErrorState
                title="Counter-evidence incomplete"
                description="The verifier found route and middleware evidence, but no matching regression test yet."
              />
            </div>
          </TabsContent>

          <TabsContent value="publish" className="min-h-0">
            <div className="p-4">
              <EmptyState
                title="Nothing accepted yet"
                description="Accepted findings will appear here for copy packets and GitHub preview."
                icon={CopyIcon}
              />
            </div>
          </TabsContent>
        </Tabs>

        <div className="border-t p-4">
          <div className="bg-background rounded-lg border p-3">
            <div className="mb-3 flex items-center gap-2">
              <Badge variant="destructive">High</Badge>
              <Badge variant="secondary">Verified</Badge>
            </div>
            <p className="text-sm font-medium">
              Auth middleware skipped on billing route
            </p>
            <p className="text-muted-foreground mt-2 text-sm">
              Billing endpoints are reachable without RequireAuth on the route
              group.
            </p>
            <div className="mt-3 flex gap-2">
              <Button size="sm">
                <CheckIcon data-icon="inline-start" />
                Accept
              </Button>
              <Button size="sm" variant="outline">
                <CopyIcon data-icon="inline-start" />
                Copy
              </Button>
            </div>
          </div>
        </div>
      </div>
    </aside>
  );
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

function CodeLine({
  num,
  text,
  tone = "default",
}: {
  num: number;
  text: string;
  tone?: "default" | "added" | "removed" | "context";
}) {
  return (
    <div
      className={cn(
        "grid grid-cols-[48px_minmax(0,1fr)] border-b border-transparent leading-6",
        tone === "added" && "bg-code-added",
        tone === "removed" && "bg-code-removed",
        tone === "context" && "bg-surface",
      )}
    >
      <span className="text-muted-foreground pr-3 text-right select-none">
        {num}
      </span>
      <span className="truncate px-3 whitespace-pre">{text || " "}</span>
    </div>
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
  return [...events, event]
    .sort((left, right) => left.sequence - right.sequence)
    .slice(-MAX_REVIEW_EVENTS_RENDERED);
}

function statusProgress(status: string) {
  switch (status) {
    case "draft":
      return 0;
    case "queued":
      return 8;
    case "running":
      return 45;
    case "paused":
      return 45;
    case "canceling":
      return 70;
    case "completed":
      return 100;
    case "failed":
    case "canceled":
      return 100;
    default:
      return 0;
  }
}

function formatFileScan(summary?: ReviewSessionSummary) {
  if (!summary) {
    return "0/0";
  }
  return `${summary.changed_files_scanned}/${summary.changed_files_total}`;
}

function formatFindingCount(summary?: ReviewSessionSummary) {
  if (!summary?.finding_counts) {
    return "0";
  }
  const total = summary.finding_counts.total;
  return typeof total === "number" ? String(total) : "0";
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
  return [
    finding.draft_comment || finding.canonical_claim,
    "",
    `Finding: ${finding.canonical_claim}`,
    `Severity: ${formatDecisionLabel(finding.severity)}`,
    `Status: ${formatDecisionLabel(finding.verification_status)}`,
    `Location: ${formatFindingLocation(finding)}`,
    finding.evidence_summary ? `Evidence: ${finding.evidence_summary}` : "",
    finding.suggested_fix ? `Suggested fix: ${finding.suggested_fix}` : "",
  ]
    .filter(Boolean)
    .join("\n");
}

function snippetLines(item: EvidenceItem) {
  const startLine = item.line_window?.start_line ?? item.start_line ?? 1;
  return (item.code_snippet ?? "")
    .split("\n")
    .slice(0, MAX_CODE_LINES_RENDERED)
    .map((text, index) => ({ number: startLine + index, text }));
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

function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const timeout = window.setTimeout(() => setDebounced(value), delayMs);
    return () => window.clearTimeout(timeout);
  }, [delayMs, value]);

  return debounced;
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

function formatPolicyLabel(key: string): string {
  return key
    .replace(/^include_/, "")
    .replace(/_/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
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
