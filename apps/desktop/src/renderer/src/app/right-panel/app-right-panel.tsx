import {
  type ReactNode,
  type PointerEvent as ReactPointerEvent,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  ChevronDownIcon,
  ChevronRightIcon,
  Code2Icon,
  FileTextIcon,
  FolderClosedIcon,
  FolderOpenIcon,
  FolderTreeIcon,
  GitPullRequestIcon,
  PanelRightCloseIcon,
  SearchIcon,
  XIcon,
  type LucideIcon,
} from "lucide-react";

import { EmptyState, ErrorState, LoadingRows } from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  type ApiClient,
  type ChangedFile,
  type ChangedFilePatch,
  errorApiState,
  idleApiState,
  type Loadable,
  loadApiResource,
  loadingApiState,
  type Repository,
  type RepositoryFile,
  type RepositoryFileTree,
  type RepositoryFileContent,
  type Snapshot,
  type Workspace,
} from "@/lib/api";
import { languageForFilePath } from "@/lib/syntax-highlighting";
import { cn } from "@/lib/utils";
import { SyntaxCodeBlock } from "../shared/markdown-message";
import { ResizableRightPanelHandle } from "../shared/resizable-right-panel";
import {
  type AppRightPanelState,
  type AppRightPanelTab,
  type AppRightPanelTool,
} from "./use-app-right-panel";

const fileTreeLimit = 2000;
const fileReadLimitBytes = 256 << 10;
const renderedFileLineLimit = 1600;
const renderedDiffLineLimit = 1400;

export function AppRightPanel({
  activeRepository,
  activeSnapshot,
  activeWorkspace,
  client,
  panel,
  onClose,
  onResizePointerDown,
}: {
  activeRepository?: Repository;
  activeSnapshot?: Snapshot;
  activeWorkspace?: Workspace;
  client: ApiClient | null;
  panel: AppRightPanelState;
  onClose: () => void;
  onResizePointerDown: (event: ReactPointerEvent) => void;
}) {
  const [fileTreeVisible, setFileTreeVisible] = useState(true);
  const active = panel.active;
  const activeTab =
    active.kind === "tab"
      ? panel.tabs.find((tab) => tab.id === active.id)
      : undefined;
  const activeTool = active.kind === "tool" ? active.tool : undefined;
  const shouldShowFileTree = Boolean(
    fileTreeVisible &&
    activeRepository &&
    activeWorkspace &&
    (activeTool === "files" || activeTab),
  );
  const primaryContent = activeTab ? (
    <PanelTabView client={client} tab={activeTab} />
  ) : activeTool === "files" ? (
    <FilesTool activeRepository={activeRepository} />
  ) : activeTool === "review" ? (
    <ReviewTool
      activeSnapshot={activeSnapshot}
      client={client}
      onOpenDiff={panel.openDiff}
    />
  ) : (
    <PanelHome activeSnapshot={activeSnapshot} onSelectTool={panel.showTool} />
  );

  return (
    <aside
      aria-label="App right panel"
      className="bg-background @container/right-panel relative flex min-h-0 min-w-0 flex-col border-l"
    >
      <ResizableRightPanelHandle onPointerDown={onResizePointerDown} />
      <div className="border-border-subtle flex h-11 shrink-0 items-center gap-1 border-b px-2">
        <PanelToolButton
          active={activeTool === "home"}
          label="Panel home"
          onClick={() => panel.showTool("home")}
        >
          <Code2Icon className="size-3.5" />
        </PanelToolButton>
        <PanelToolButton
          active={activeTool === "files"}
          label="Files"
          onClick={() => panel.showTool("files")}
        >
          <FolderOpenIcon className="size-3.5" />
        </PanelToolButton>
        <PanelToolButton
          active={activeTool === "review"}
          disabled={!activeSnapshot}
          label="Review"
          onClick={() => panel.showTool("review")}
        >
          <GitPullRequestIcon className="size-3.5" />
        </PanelToolButton>
        <div className="bg-border mx-1 h-5 w-px" />
        <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {panel.tabs.map((tab) => (
            <div
              key={tab.id}
              className={cn(
                "hover:bg-muted flex h-8 max-w-48 min-w-0 shrink-0 cursor-pointer items-center gap-1.5 rounded-md px-2 text-xs transition-colors",
                activeTab?.id === tab.id &&
                  "bg-surface-muted text-foreground font-medium",
              )}
            >
              <button
                className="flex h-full min-w-0 flex-1 cursor-pointer items-center gap-1.5"
                type="button"
                onClick={() => panel.activateTab(tab.id)}
              >
                {tab.kind === "file" ? (
                  <FileTextIcon className="text-muted-foreground size-3.5 shrink-0" />
                ) : (
                  <GitPullRequestIcon className="text-muted-foreground size-3.5 shrink-0" />
                )}
                <span className="min-w-0 truncate">{tab.title}</span>
              </button>
              <button
                aria-label={`Close ${tab.title}`}
                className="hover:bg-background/80 -mr-1 flex size-5 shrink-0 items-center justify-center rounded"
                type="button"
                onClick={(event) => {
                  event.stopPropagation();
                  panel.closeTab(tab.id);
                }}
              >
                <XIcon className="size-3" />
              </button>
            </div>
          ))}
        </div>
        <Button
          aria-label={fileTreeVisible ? "Hide file tree" : "Show file tree"}
          className="size-8 shrink-0"
          disabled={!activeRepository || !activeWorkspace}
          size="icon-sm"
          type="button"
          variant={fileTreeVisible ? "secondary" : "ghost"}
          onClick={() => setFileTreeVisible((visible) => !visible)}
        >
          <FolderTreeIcon className="size-4" />
        </Button>
        <Button
          aria-label="Hide right panel"
          className="size-8 shrink-0"
          size="icon-sm"
          type="button"
          variant="ghost"
          onClick={onClose}
        >
          <PanelRightCloseIcon className="size-4" />
        </Button>
      </div>

      <div
        className={cn(
          "grid min-h-0 flex-1",
          shouldShowFileTree
            ? "grid-rows-[minmax(0,1fr)_minmax(240px,40%)] @min-[680px]/right-panel:grid-cols-[minmax(0,1fr)_260px] @min-[680px]/right-panel:grid-rows-1"
            : "grid-cols-1",
        )}
      >
        <div className="min-h-0 min-w-0">{primaryContent}</div>
        {shouldShowFileTree && activeRepository && activeWorkspace ? (
          <FileTreePane
            activeRepository={activeRepository}
            activeTab={activeTab}
            activeWorkspace={activeWorkspace}
            client={client}
            onOpenFile={panel.openFile}
          />
        ) : null}
      </div>
    </aside>
  );
}

function PanelToolButton({
  active,
  children,
  disabled,
  label,
  onClick,
}: {
  active: boolean;
  children: ReactNode;
  disabled?: boolean;
  label: string;
  onClick: () => void;
}) {
  return (
    <Button
      aria-label={label}
      className="size-8 shrink-0"
      disabled={disabled}
      size="icon-sm"
      type="button"
      variant={active ? "secondary" : "ghost"}
      onClick={onClick}
    >
      {children}
    </Button>
  );
}

function PanelHome({
  activeSnapshot,
  onSelectTool,
}: {
  activeSnapshot?: Snapshot;
  onSelectTool: (tool: AppRightPanelTool) => void;
}) {
  return (
    <div className="flex h-full items-center justify-center p-6">
      <div className="grid w-full max-w-sm gap-3">
        <PanelHomeButton
          description="Search and open repository files"
          icon={FolderOpenIcon}
          title="Files"
          onClick={() => onSelectTool("files")}
        />
        <PanelHomeButton
          description={
            activeSnapshot
              ? "Inspect changed files and patches"
              : "Open a review thread to inspect changes"
          }
          disabled={!activeSnapshot}
          icon={GitPullRequestIcon}
          title="Review"
          onClick={() => onSelectTool("review")}
        />
      </div>
    </div>
  );
}

function PanelHomeButton({
  description,
  disabled,
  icon: Icon,
  title,
  onClick,
}: {
  description: string;
  disabled?: boolean;
  icon: LucideIcon;
  title: string;
  onClick: () => void;
}) {
  return (
    <button
      className="bg-surface-muted/60 hover:bg-surface-muted flex min-h-[112px] cursor-pointer flex-col items-center justify-center rounded-xl border border-transparent px-4 text-center transition-colors disabled:cursor-default disabled:opacity-55"
      disabled={disabled}
      type="button"
      onClick={onClick}
    >
      <Icon className="text-muted-foreground size-5" />
      <div className="mt-3 text-sm font-semibold">{title}</div>
      <div className="text-muted-foreground mt-1 text-xs">{description}</div>
    </button>
  );
}

function FilesTool({ activeRepository }: { activeRepository?: Repository }) {
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PanelSectionHeader
        description={
          activeRepository
            ? activeRepository.local_path
            : "Choose a project to browse files"
        }
        title="Files"
      />
      <div className="flex min-h-0 flex-1 items-center justify-center p-6">
        <EmptyState
          className="border-0 py-10"
          description={
            activeRepository ? activeRepository.local_path : "No project open."
          }
          icon={FolderOpenIcon}
          title={activeRepository ? "No file selected" : "No project selected"}
        />
      </div>
    </div>
  );
}

type FileTreeNode = {
  children: FileTreeNode[];
  kind: "directory" | "file";
  name: string;
  path: string;
};

function FileTreePane({
  activeRepository,
  activeTab,
  activeWorkspace,
  client,
  onOpenFile,
}: {
  activeRepository: Repository;
  activeTab?: AppRightPanelTab;
  activeWorkspace: Workspace;
  client: ApiClient | null;
  onOpenFile: (repositoryId: string, workspaceId: string, path: string) => void;
}) {
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebouncedValue(query, 120);
  const [tree, setTree] =
    useState<Loadable<RepositoryFileTree>>(loadingApiState());
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const selectedPath =
    activeTab?.kind === "file"
      ? activeTab.path
      : activeTab?.kind === "diff"
        ? activeTab.changedFile.path
        : "";

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      if (!client) {
        setTree(errorApiState(new Error("Backend client is unavailable.")));
        return;
      }
      setTree(loadingApiState());
      void loadApiResource(() =>
        client.listRepositoryFileTree(activeRepository.id, {
          workspaceId: activeWorkspace.id,
          limit: fileTreeLimit,
        }),
      ).then((state) => {
        if (!canceled) {
          setTree(state);
        }
      });
    });
    return () => {
      canceled = true;
    };
  }, [activeRepository.id, activeWorkspace.id, client]);

  const filteredFiles = useMemo(() => {
    if (tree.status !== "success") {
      return [];
    }
    return filterTreeFiles(tree.data.files, debouncedQuery);
  }, [debouncedQuery, tree]);
  const root = useMemo(() => buildFileTree(filteredFiles), [filteredFiles]);

  useEffect(() => {
    let canceled = false;
    const automatic = new Set<string>();
    if (debouncedQuery.trim()) {
      for (const file of filteredFiles) {
        addAncestorPaths(automatic, file.path);
      }
    }
    if (selectedPath) {
      addAncestorPaths(automatic, selectedPath);
    }
    if (automatic.size === 0) {
      return () => {
        canceled = true;
      };
    }
    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      setExpanded((current) => {
        const next = new Set(current);
        let changed = false;
        for (const path of automatic) {
          if (!next.has(path)) {
            next.add(path);
            changed = true;
          }
        }
        return changed ? next : current;
      });
    });
    return () => {
      canceled = true;
    };
  }, [debouncedQuery, filteredFiles, selectedPath]);

  const toggleDirectory = (path: string) => {
    setExpanded((current) => {
      const next = new Set(current);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  };

  return (
    <div className="border-border-subtle flex min-h-0 min-w-0 flex-col border-t @min-[680px]/right-panel:border-t-0 @min-[680px]/right-panel:border-l">
      <div className="border-border-subtle border-b p-3">
        <div className="relative">
          <SearchIcon className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
          <Input
            aria-label="Filter files"
            className="h-9 pl-9"
            placeholder="Filter files..."
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
        </div>
        {tree.status === "success" && tree.data.truncated ? (
          <div className="text-muted-foreground mt-2 text-xs">
            Showing first {tree.data.limit} files
          </div>
        ) : null}
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="px-2 py-2">
          {tree.status === "loading" ? <LoadingRows rows={10} /> : null}
          {tree.status === "error" ? (
            <ErrorState
              title="File tree unavailable"
              description={tree.error.message}
            />
          ) : null}
          {tree.status === "success" && root.children.length === 0 ? (
            <EmptyState
              className="border-0 py-10"
              description="No matching files."
              icon={SearchIcon}
              title="No files found"
            />
          ) : null}
          {tree.status === "success" ? (
            <FileTreeRows
              depth={0}
              expanded={expanded}
              nodes={root.children}
              selectedPath={selectedPath}
              onOpenFile={(path) =>
                onOpenFile(activeRepository.id, activeWorkspace.id, path)
              }
              onToggleDirectory={toggleDirectory}
            />
          ) : null}
        </div>
      </ScrollArea>
    </div>
  );
}

function FileTreeRows({
  depth,
  expanded,
  nodes,
  selectedPath,
  onOpenFile,
  onToggleDirectory,
}: {
  depth: number;
  expanded: Set<string>;
  nodes: FileTreeNode[];
  selectedPath: string;
  onOpenFile: (path: string) => void;
  onToggleDirectory: (path: string) => void;
}) {
  return (
    <>
      {nodes.map((node) =>
        node.kind === "directory" ? (
          <div key={node.path}>
            <button
              aria-expanded={expanded.has(node.path)}
              className="hover:bg-muted flex h-8 w-full cursor-pointer items-center gap-1.5 rounded-md pr-2 text-left text-sm transition-colors"
              style={{ paddingLeft: 6 + depth * 14 }}
              type="button"
              onClick={() => onToggleDirectory(node.path)}
            >
              {expanded.has(node.path) ? (
                <ChevronDownIcon className="text-muted-foreground size-4 shrink-0" />
              ) : (
                <ChevronRightIcon className="text-muted-foreground size-4 shrink-0" />
              )}
              <FolderClosedIcon className="text-muted-foreground size-4 shrink-0" />
              <span className="min-w-0 truncate">{node.name}</span>
            </button>
            {expanded.has(node.path) ? (
              <FileTreeRows
                depth={depth + 1}
                expanded={expanded}
                nodes={node.children}
                selectedPath={selectedPath}
                onOpenFile={onOpenFile}
                onToggleDirectory={onToggleDirectory}
              />
            ) : null}
          </div>
        ) : (
          <button
            key={node.path}
            aria-current={selectedPath === node.path ? "page" : undefined}
            className={cn(
              "hover:bg-muted flex h-8 w-full cursor-pointer items-center gap-1.5 rounded-md pr-2 text-left text-sm transition-colors",
              selectedPath === node.path &&
                "bg-surface-muted text-foreground font-medium",
            )}
            style={{ paddingLeft: 26 + depth * 14 }}
            type="button"
            onClick={() => onOpenFile(node.path)}
          >
            <FileTextIcon className="text-muted-foreground size-4 shrink-0" />
            <span className="min-w-0 truncate">{node.name}</span>
          </button>
        ),
      )}
    </>
  );
}

function ReviewTool({
  activeSnapshot,
  client,
  onOpenDiff,
}: {
  activeSnapshot?: Snapshot;
  client: ApiClient | null;
  onOpenDiff: (snapshotId: string, changedFile: ChangedFile) => void;
}) {
  const [query, setQuery] = useState("");
  const [files, setFiles] = useState<Loadable<ChangedFile[]>>(idleApiState());

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      if (!client || !activeSnapshot) {
        setFiles(idleApiState());
        return;
      }
      setFiles(loadingApiState());
      void loadApiResource(() =>
        client.listChangedFiles(activeSnapshot.id),
      ).then((state) => {
        if (!canceled) {
          setFiles(state);
        }
      });
    });
    return () => {
      canceled = true;
    };
  }, [activeSnapshot, client]);

  const filteredFiles = useMemo(() => {
    if (files.status !== "success") {
      return [];
    }
    const needle = query.trim().toLowerCase();
    if (!needle) {
      return files.data;
    }
    return files.data.filter((file) =>
      file.path.toLowerCase().includes(needle),
    );
  }, [files, query]);
  const stats = useMemo(() => changedFileStats(filteredFiles), [filteredFiles]);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <PanelSectionHeader
        description={
          activeSnapshot
            ? snapshotTitle(activeSnapshot)
            : "Select a review thread to inspect changed files"
        }
        title="Review"
      />
      <div className="border-border-subtle border-b p-3">
        <div className="relative">
          <SearchIcon className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
          <Input
            aria-label="Filter changed files"
            className="h-9 pl-9"
            disabled={!activeSnapshot}
            placeholder="Filter changed files..."
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
        </div>
        {files.status === "success" ? (
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <Badge variant="secondary">{filteredFiles.length} files</Badge>
            <Badge variant="outline">
              <span className="text-emerald-700">+{stats.additions}</span>
              <span className="text-muted-foreground mx-1">/</span>
              <span className="text-red-700">-{stats.deletions}</span>
            </Badge>
          </div>
        ) : null}
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="p-2">
          {files.status === "idle" ? (
            <EmptyState
              className="border-0 py-12"
              description="Diffs are available after opening a review thread."
              icon={GitPullRequestIcon}
              title="No review selected"
            />
          ) : null}
          {files.status === "loading" ? <LoadingRows rows={8} /> : null}
          {files.status === "error" ? (
            <ErrorState
              title="Review files unavailable"
              description={files.error.message}
            />
          ) : null}
          {files.status === "success" && filteredFiles.length === 0 ? (
            <EmptyState
              className="border-0 py-12"
              description="Try another path filter."
              icon={SearchIcon}
              title="No changed files found"
            />
          ) : null}
          {files.status === "success"
            ? filteredFiles.slice(0, 300).map((file) => (
                <button
                  key={file.id}
                  className="hover:bg-muted flex w-full cursor-pointer items-center gap-2 rounded-lg px-2 py-2 text-left transition-colors"
                  type="button"
                  onClick={() => onOpenDiff(file.snapshot_id, file)}
                >
                  <GitPullRequestIcon className="text-muted-foreground size-4 shrink-0" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-mono text-xs font-medium">
                      {file.path}
                    </span>
                    <span className="text-muted-foreground mt-0.5 flex items-center gap-2 text-xs">
                      <span>{file.status}</span>
                      {file.is_binary ? <span>binary</span> : null}
                    </span>
                  </span>
                  <span className="shrink-0 font-mono text-xs">
                    <span className="text-emerald-700">+{file.additions}</span>{" "}
                    <span className="text-red-700">-{file.deletions}</span>
                  </span>
                </button>
              ))
            : null}
        </div>
      </ScrollArea>
    </div>
  );
}

function PanelTabView({
  client,
  tab,
}: {
  client: ApiClient | null;
  tab: AppRightPanelTab;
}) {
  if (tab.kind === "file") {
    return (
      <FileTabView
        client={client}
        path={tab.path}
        repositoryId={tab.repositoryId}
        workspaceId={tab.workspaceId}
      />
    );
  }
  return <DiffTabView client={client} tab={tab} />;
}

function FileTabView({
  client,
  path,
  repositoryId,
  workspaceId,
}: {
  client: ApiClient | null;
  path: string;
  repositoryId: string;
  workspaceId: string;
}) {
  const [content, setContent] =
    useState<Loadable<RepositoryFileContent>>(loadingApiState());

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      if (!client) {
        setContent(errorApiState(new Error("Repository is unavailable.")));
        return;
      }
      setContent(loadingApiState());
      void loadApiResource(() =>
        client.getRepositoryFileContent(repositoryId, {
          workspaceId,
          path,
          maxBytes: fileReadLimitBytes,
        }),
      ).then((state) => {
        if (!canceled) {
          setContent(state);
        }
      });
    });
    return () => {
      canceled = true;
    };
  }, [client, path, repositoryId, workspaceId]);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <PanelSectionHeader description={path} title={pathLeaf(path)} />
      <ScrollArea className="min-h-0 flex-1">
        <div className="p-3">
          {content.status === "loading" ? <LoadingRows rows={10} /> : null}
          {content.status === "error" ? (
            <ErrorState
              title="File unavailable"
              description={content.error.message}
            />
          ) : null}
          {content.status === "success" ? (
            content.data.binary ? (
              <EmptyState
                className="border-0 py-12"
                description={`Binary content (${formatBytes(content.data.size_bytes)}) cannot be previewed inline.`}
                icon={FileTextIcon}
                title="Binary file"
              />
            ) : (
              <CodePreview
                content={content.data.content ?? ""}
                path={content.data.path}
                truncated={content.data.content_truncated}
              />
            )
          ) : null}
        </div>
      </ScrollArea>
    </div>
  );
}

function DiffTabView({
  client,
  tab,
}: {
  client: ApiClient | null;
  tab: Extract<AppRightPanelTab, { kind: "diff" }>;
}) {
  const [patch, setPatch] =
    useState<Loadable<ChangedFilePatch>>(loadingApiState());
  const file = tab.changedFile;

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      if (!client) {
        setPatch(errorApiState(new Error("Backend client is unavailable.")));
        return;
      }
      if (file.is_binary || !file.patch_artifact_id) {
        setPatch(errorApiState(new Error("No text patch is available.")));
        return;
      }
      setPatch(loadingApiState());
      void loadApiResource(() =>
        client.getChangedFilePatch(tab.snapshotId, file.id),
      ).then((state) => {
        if (!canceled) {
          setPatch(state);
        }
      });
    });
    return () => {
      canceled = true;
    };
  }, [client, file, tab.snapshotId]);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <PanelSectionHeader
        description={file.path}
        title={pathLeaf(file.path)}
        trailing={
          <span className="font-mono text-xs">
            <span className="text-emerald-700">+{file.additions}</span>{" "}
            <span className="text-red-700">-{file.deletions}</span>
          </span>
        }
      />
      <ScrollArea className="min-h-0 flex-1">
        <div className="p-3">
          {patch.status === "loading" ? <LoadingRows rows={10} /> : null}
          {patch.status === "error" ? (
            <ErrorState
              title="Diff unavailable"
              description={patch.error.message}
            />
          ) : null}
          {patch.status === "success" ? (
            <DiffPreview patch={patch.data} path={file.path} />
          ) : null}
        </div>
      </ScrollArea>
    </div>
  );
}

function CodePreview({
  content,
  path,
  truncated,
}: {
  content: string;
  path: string;
  truncated: boolean;
}) {
  const lines = useMemo(() => content.split(/\r?\n/), [content]);
  const rendered = useMemo(
    () => lines.slice(0, renderedFileLineLimit).join("\n"),
    [lines],
  );
  const clipped = truncated || lines.length > renderedFileLineLimit;
  return (
    <div className="grid gap-2">
      <SyntaxCodeBlock
        code={rendered}
        language={languageForFilePath(path)}
        lineNumbers
      />
      {clipped ? (
        <div className="text-muted-foreground border-border-subtle rounded-lg border px-3 py-2 text-xs">
          Preview limited for performance.
        </div>
      ) : null}
    </div>
  );
}

function DiffPreview({
  patch,
  path,
}: {
  patch: ChangedFilePatch;
  path: string;
}) {
  const lines = useMemo(() => patch.content.split(/\r?\n/), [patch.content]);
  const rendered = useMemo(
    () => lines.slice(0, renderedDiffLineLimit).join("\n"),
    [lines],
  );
  const clipped =
    patch.content_truncated || lines.length > renderedDiffLineLimit;
  return (
    <div className="grid gap-2">
      <SyntaxCodeBlock code={rendered} language="diff" />
      {clipped ? (
        <div className="text-muted-foreground border-border-subtle rounded-lg border px-3 py-2 text-xs">
          Diff preview for {path} was limited for performance.
        </div>
      ) : null}
    </div>
  );
}

function PanelSectionHeader({
  description,
  title,
  trailing,
}: {
  description?: string;
  title: string;
  trailing?: ReactNode;
}) {
  return (
    <div className="border-border-subtle flex min-h-14 shrink-0 items-start justify-between gap-3 border-b px-4 py-3">
      <div className="min-w-0">
        <div className="truncate text-sm font-semibold">{title}</div>
        {description ? (
          <div className="text-muted-foreground mt-1 truncate text-xs">
            {description}
          </div>
        ) : null}
      </div>
      {trailing ? <div className="shrink-0 pt-0.5">{trailing}</div> : null}
    </div>
  );
}

function filterTreeFiles(files: RepositoryFile[], query: string) {
  const needle = query.trim().toLowerCase();
  if (!needle) {
    return files;
  }
  return files.filter((file) => {
    const path = file.path.toLowerCase();
    const name = file.name.toLowerCase();
    return path.includes(needle) || name.includes(needle);
  });
}

function buildFileTree(files: RepositoryFile[]): FileTreeNode {
  const root: FileTreeNode = {
    children: [],
    kind: "directory",
    name: "",
    path: "",
  };
  const directories = new Map<string, FileTreeNode>([["", root]]);

  for (const file of files) {
    const parts = file.path.split("/").filter(Boolean);
    if (parts.length === 0) {
      continue;
    }
    let parent = root;
    let currentPath = "";
    for (const part of parts.slice(0, -1)) {
      currentPath = currentPath ? `${currentPath}/${part}` : part;
      let directory = directories.get(currentPath);
      if (!directory) {
        directory = {
          children: [],
          kind: "directory",
          name: part,
          path: currentPath,
        };
        directories.set(currentPath, directory);
        parent.children.push(directory);
      }
      parent = directory;
    }
    parent.children.push({
      children: [],
      kind: "file",
      name: file.name || parts[parts.length - 1],
      path: file.path,
    });
  }

  sortFileTree(root);
  return root;
}

function sortFileTree(node: FileTreeNode) {
  node.children.sort((left, right) => {
    if (left.kind !== right.kind) {
      return left.kind === "directory" ? -1 : 1;
    }
    return left.name.localeCompare(right.name, undefined, {
      numeric: true,
      sensitivity: "base",
    });
  });
  for (const child of node.children) {
    sortFileTree(child);
  }
}

function addAncestorPaths(paths: Set<string>, filePath: string) {
  const parts = filePath.split("/").filter(Boolean);
  let current = "";
  for (const part of parts.slice(0, -1)) {
    current = current ? `${current}/${part}` : part;
    paths.add(current);
  }
}

function useDebouncedValue(value: string, delayMs: number) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delayMs);
    return () => window.clearTimeout(timer);
  }, [delayMs, value]);
  return debounced;
}

function changedFileStats(files: ChangedFile[]) {
  return files.reduce(
    (stats, file) => ({
      additions: stats.additions + file.additions,
      deletions: stats.deletions + file.deletions,
    }),
    { additions: 0, deletions: 0 },
  );
}

function snapshotTitle(snapshot: Snapshot) {
  if (snapshot.pr_title) {
    return snapshot.pr_title;
  }
  if (snapshot.base_ref && snapshot.head_ref) {
    return `${snapshot.base_ref}..${snapshot.head_ref}`;
  }
  if (snapshot.source_type === "local_changes") {
    return "Local changes";
  }
  return "Review snapshot";
}

function pathLeaf(path: string) {
  return path.split("/").at(-1) || path;
}

function formatBytes(value: number) {
  if (value < 1024) {
    return `${value} B`;
  }
  if (value < 1024 * 1024) {
    return `${Math.round(value / 1024)} KB`;
  }
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}
