import { useCallback, useState } from "react";

import type { ChangedFile } from "@/lib/api";

export type AppRightPanelTool = "home" | "files" | "review";

export type AppRightPanelTab =
  | {
      id: string;
      kind: "file";
      path: string;
      repositoryId: string;
      title: string;
      workspaceId: string;
    }
  | {
      changedFile: ChangedFile;
      id: string;
      kind: "diff";
      snapshotId: string;
      title: string;
    };

export type AppRightPanelActive =
  | { kind: "tool"; tool: AppRightPanelTool }
  | { id: string; kind: "tab" };

export function useAppRightPanelState() {
  const [active, setActive] = useState<AppRightPanelActive>({
    kind: "tool",
    tool: "home",
  });
  const [tabs, setTabs] = useState<AppRightPanelTab[]>([]);

  const showTool = useCallback((tool: AppRightPanelTool) => {
    setActive({ kind: "tool", tool });
  }, []);

  const openFile = useCallback(
    (repositoryId: string, workspaceId: string, path: string) => {
      const tab = fileTab(repositoryId, workspaceId, path);
      setTabs((current) => upsertTab(current, tab));
      setActive({ id: tab.id, kind: "tab" });
    },
    [],
  );

  const openDiff = useCallback(
    (snapshotId: string, changedFile: ChangedFile) => {
      const tab = diffTab(snapshotId, changedFile);
      setTabs((current) => upsertTab(current, tab));
      setActive({ id: tab.id, kind: "tab" });
    },
    [],
  );

  const closeTab = useCallback(
    (id: string) => {
      setTabs((current) => {
        const nextTabs = current.filter((tab) => tab.id !== id);
        if (active.kind === "tab" && active.id === id) {
          const fallback = nextTabs.at(-1);
          setActive(
            fallback
              ? { id: fallback.id, kind: "tab" }
              : { kind: "tool", tool: "home" },
          );
        }
        return nextTabs;
      });
    },
    [active],
  );

  const activateTab = useCallback((id: string) => {
    setActive({ id, kind: "tab" });
  }, []);

  return {
    active,
    activateTab,
    closeTab,
    openDiff,
    openFile,
    showTool,
    tabs,
  };
}

export type AppRightPanelState = ReturnType<typeof useAppRightPanelState>;

function upsertTab(tabs: AppRightPanelTab[], tab: AppRightPanelTab) {
  if (tabs.some((item) => item.id === tab.id)) {
    return tabs;
  }
  return [...tabs, tab];
}

function fileTab(
  repositoryId: string,
  workspaceId: string,
  path: string,
): AppRightPanelTab {
  return {
    id: `file:${repositoryId}:${path}`,
    kind: "file",
    path,
    repositoryId,
    title: path.split("/").at(-1) || path,
    workspaceId,
  };
}

function diffTab(
  snapshotId: string,
  changedFile: ChangedFile,
): AppRightPanelTab {
  return {
    changedFile,
    id: `diff:${snapshotId}:${changedFile.id}`,
    kind: "diff",
    snapshotId,
    title: changedFile.path.split("/").at(-1) || changedFile.path,
  };
}
