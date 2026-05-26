import { useCallback, useEffect, useMemo, useState } from "react";
import {
  FolderOpenIcon,
  GitBranchIcon,
  GitPullRequestIcon,
  PlusIcon,
  SettingsIcon,
  TerminalIcon,
} from "lucide-react";

import {
  AppShell,
  SearchCommandDialog,
  type SearchCommandGroup,
} from "@/components/app/chrome";
import { Toaster } from "@/components/ui/sonner";
import {
  type AgentConfig,
  type AgentModelCatalog,
  type ApiClient,
  createCocodeClient,
  errorApiState,
  idleApiState,
  type Loadable,
  loadApiResource,
  loadingApiState,
  type ApiSessionResponse,
  type OpenRepositoryResponse,
  type Repository,
  type ReviewSession,
  type Snapshot,
  successApiState,
  type Workspace,
} from "@/lib/api";
import { toast } from "sonner";
import { AgentSettingsScreen } from "./agents/agent-settings-screen";
import {
  AppConnectionNotice,
  Sidebar,
  TopNav,
  type SetupNavContext,
} from "./shell/app-navigation";
import {
  loadAgentConfigs,
  loadAgentModelCatalogs,
  shouldRecheckAgentModelCatalogs,
} from "./agents/agent-config-model";
import { NewThreadScreen } from "./setup/new-thread-screen";
import { ReviewThread } from "./review/review-thread";
import { formatRelativeAge } from "./shared/time-format";

const MAX_SEARCH_RESULTS = 5;
type MainView = "new-thread" | "review" | "agent-settings";
export function App() {
  const [client, setClient] = useState<ApiClient | null>(null);
  const [mainView, setMainView] = useState<MainView>("new-thread");
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
  const [activeSnapshot, setActiveSnapshot] =
    useState<Loadable<Snapshot>>(idleApiState());
  const [setupNavContext, setSetupNavContext] =
    useState<SetupNavContext | null>(null);
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

        const nextClient = createCocodeClient(info);
        setClient(nextClient);
        void loadApiResource(() => nextClient.session()).then((state) => {
          if (canceled) {
            return;
          }
          setApiSession(state);
          if (state.status === "error") {
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
  const displayedSession =
    currentReviewSession ?? (mainView === "review" ? activeSession : undefined);
  const displayedSnapshot =
    activeSnapshot.status === "success" ? activeSnapshot.data : undefined;

  useEffect(() => {
    const snapshotId = displayedSession?.snapshot_id;
    let canceled = false;
    void Promise.resolve().then(async () => {
      if (!client || !snapshotId) {
        if (!canceled) {
          setActiveSnapshot(idleApiState());
        }
        return;
      }

      if (!canceled) {
        setActiveSnapshot(loadingApiState());
      }
      const state = await loadApiResource(() => client.getSnapshot(snapshotId));
      if (!canceled) {
        setActiveSnapshot(state);
      }
    });
    return () => {
      canceled = true;
    };
  }, [client, displayedSession?.snapshot_id]);

  const handleSelectWorkspace = useCallback(
    (workspaceId: string) => {
      const selectedWorkspace = workspaceList.find(
        (workspace) => workspace.id === workspaceId,
      );
      if (!client || !selectedWorkspace) {
        return;
      }
      setCurrentReviewSession(null);
      setSetupNavContext(null);
      setMainView("new-thread");
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
    setCurrentReviewSession(null);
    setSetupNavContext(null);
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
    setSetupNavContext(null);
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
              onSelect: () => {
                setCurrentReviewSession(null);
                setMainView("new-thread");
              },
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
            description: "Codex, Gemini, Antigravity, OpenCode, and custom CLIs",
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
            activeSessionId={displayedSession?.id}
            activeWorkspaceId={activeWorkspaceId}
            workspaces={workspaces}
            reviewSessions={reviewSessions}
            repositoryOpenState={repositoryOpenState}
            deletingReviewSessionId={deletingReviewSessionId}
            onOpenRepository={handleOpenRepository}
            onOpenSearch={() => setSearchOpen(true)}
            onOpenAgentSettings={() => setMainView("agent-settings")}
            onOpenNewThread={() => {
              setCurrentReviewSession(null);
              setMainView("new-thread");
            }}
            onDeleteReviewSession={handleDeleteReviewSession}
            onSelectReviewSession={handleSelectReviewSession}
            onSelectWorkspace={handleSelectWorkspace}
          />
        }
        header={
          <TopNav
            activeRepository={activeRepository}
            activeSession={displayedSession}
            activeSnapshot={displayedSnapshot}
            activeWorkspace={activeWorkspace}
            setupContext={setupNavContext}
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
            onSetupContextChange={setSetupNavContext}
            onReviewStarted={(session) => {
              setCurrentReviewSession(session);
              setSetupNavContext(null);
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
