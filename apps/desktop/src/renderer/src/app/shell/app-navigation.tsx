import { useEffect, useState } from "react";
import {
  ChevronDownIcon,
  Code2Icon,
  FileTextIcon,
  FolderOpenIcon,
  GitBranchIcon,
  PlusIcon,
  SearchIcon,
  SettingsIcon,
  Trash2Icon,
} from "lucide-react";

import {
  ErrorState,
  LoadingRows,
  SidebarNavButton,
  SidebarSection,
} from "@/components/app/chrome";
import { Separator } from "@/components/ui/separator";
import type {
  ApiSessionResponse,
  Loadable,
  OpenRepositoryResponse,
  Repository,
  ReviewSession,
  Snapshot,
  Workspace,
} from "@/lib/api";
import { formatRelativeAge } from "../shared/time-format";

const MAX_SIDEBAR_SESSIONS = 12;

export type SetupNavContext = {
  branch?: string;
  subtitle?: string;
  title?: string;
};

export function AppConnectionNotice({
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

export function Sidebar({
  activeSessionId,
  activeWorkspaceId,
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
              <div className="border-border-subtle ml-3.5 mt-1 mb-2 flex flex-col gap-1 border-l pl-3">
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
      </div>
      {threadContextMenu && (
        <div
          className="app-no-drag bg-popover text-popover-foreground ring-border fixed z-50 min-w-40 rounded-lg p-1 text-sm shadow-lg ring-1"
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

export function TopNav({
  activeRepository,
  activeSession,
  activeSnapshot,
  activeWorkspace,
  setupContext,
}: {
  activeRepository?: Repository;
  activeSession?: ReviewSession;
  activeSnapshot?: Snapshot;
  activeWorkspace?: Workspace;
  setupContext?: SetupNavContext | null;
}) {
  const repositoryLabel = activeRepository?.owner
    ? `${activeRepository.owner}/${activeRepository.name}`
    : (activeRepository?.name ??
      activeWorkspace?.name ??
      "No project selected");
  const snapshotRepository = activeSnapshot
    ? snapshotRepositoryLabel(activeSnapshot, repositoryLabel)
    : repositoryLabel;
  const branchLabel = activeSnapshot
    ? snapshotBranchLabel(activeSnapshot, activeRepository)
    : (setupContext?.branch ?? activeRepository?.default_branch ?? "main");
  const titleLabel =
    activeSnapshot?.pr_title ||
    activeSession?.title ||
    setupContext?.title ||
    repositoryLabel;
  const subtitleLabel = activeSession
    ? snapshotRepository
    : (setupContext?.subtitle ?? "Set up review");

  return (
    <div className="app-drag bg-background border-border-subtle flex h-14 shrink-0 items-center justify-between gap-4 border-b px-5">
      <div className="flex min-w-0 items-center gap-3">
        <div className="min-w-0">
          <div className="truncate text-[0.94rem] font-semibold tracking-[-0.005em]">
            {titleLabel}
          </div>
          <div className="text-muted-foreground mt-0.5 flex min-w-0 items-center gap-1.5 text-xs">
            <Code2Icon className="size-3.5 shrink-0" />
            <span className="truncate">{subtitleLabel}</span>
          </div>
        </div>
      </div>

      <div className="app-no-drag text-muted-foreground bg-surface-raised border-border-subtle flex h-7 max-w-[220px] min-w-0 items-center gap-1.5 rounded-md border px-2.5 text-xs">
        <GitBranchIcon className="size-[13px] shrink-0" />
        <span className="truncate">{branchLabel}</span>
      </div>
    </div>
  );
}

function snapshotRepositoryLabel(snapshot: Snapshot, fallback: string) {
  if (snapshot.owner && snapshot.repo) {
    return `${snapshot.owner}/${snapshot.repo}`;
  }
  if (snapshot.repo) {
    return snapshot.repo;
  }
  return fallback;
}

function snapshotBranchLabel(snapshot: Snapshot, repository?: Repository) {
  const base = snapshot.base_ref?.trim();
  const head = snapshot.head_ref?.trim();
  if (base && head) {
    return `${base}..${head}`;
  }
  if (head) {
    return head;
  }
  if (base) {
    return base;
  }
  if (snapshot.source_type === "local_changes") {
    return repository?.default_branch ?? "working tree";
  }
  if (snapshot.pr_number) {
    return `PR #${snapshot.pr_number}`;
  }
  return repository?.default_branch ?? "main";
}
