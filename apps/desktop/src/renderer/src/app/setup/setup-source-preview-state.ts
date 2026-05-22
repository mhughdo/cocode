import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import {
  type ApiClient,
  type ChangedFilePatch,
  errorApiState,
  idleApiState,
  loadApiResource,
  loadingApiState,
  type Loadable,
  type Snapshot,
  successApiState,
} from "@/lib/api";

import {
  type SetupSourcePreview,
  type SetupPreviewStats,
  setupDiffFileRenderBatchSize,
  setupInitialDiffFileRenderCount,
  setupMaxRenderedDiffFiles,
  setupPreviewStats,
} from "./setup-source-preview-model";

export function useSetupSourcePreviewState({
  client,
  createSourceSnapshot,
  sourceKey,
  validateSource,
}: {
  client: ApiClient | null;
  createSourceSnapshot: () => Promise<Snapshot>;
  sourceKey: string;
  validateSource: () => boolean;
}) {
  const loadRequestId = useRef(0);
  const requestedPatchFileIds = useRef(new Set<string>());
  const loadedPatchFileIds = useRef(new Set<string>());
  const currentSourceKey = useRef(sourceKey);
  const [preview, setPreview] =
    useState<Loadable<SetupSourcePreview>>(idleApiState());
  const [renderedFileCount, setRenderedFileCount] = useState(
    setupInitialDiffFileRenderCount,
  );
  const [patchPreviews, setPatchPreviews] = useState<
    Record<string, Loadable<ChangedFilePatch>>
  >({});
  const [expandedFileState, setExpandedFileState] = useState<{
    ids: Set<string>;
    key: string;
  }>(() => ({ ids: new Set(), key: "" }));

  const ready = preview.status === "success" && preview.data.key === sourceKey;
  const previewKey = ready ? preview.data.key : "";
  const snapshot = ready ? preview.data.snapshot : null;
  const files = useMemo(
    () => (ready ? preview.data.files : []),
    [preview, ready],
  );
  const stats: SetupPreviewStats = setupPreviewStats(files);
  const renderedFiles = useMemo(
    () => files.slice(0, renderedFileCount),
    [files, renderedFileCount],
  );
  const expandedFileIds =
    previewKey && expandedFileState.key === previewKey
      ? expandedFileState.ids
      : emptyExpandedFileIds;
  const patchLoadFiles = useMemo(
    () =>
      renderedFiles.filter(
        (file) =>
          expandedFileIds.has(file.id) &&
          file.patch_artifact_id &&
          !file.is_binary,
      ),
    [expandedFileIds, renderedFiles],
  );

  const resetPatchState = useCallback(() => {
    setRenderedFileCount(setupInitialDiffFileRenderCount);
    setPatchPreviews({});
    setExpandedFileState({ ids: new Set(), key: "" });
    requestedPatchFileIds.current.clear();
    loadedPatchFileIds.current.clear();
  }, []);

  const reset = useCallback(() => {
    loadRequestId.current += 1;
    resetPatchState();
    setPreview(idleApiState());
  }, [resetPatchState]);

  const load = useCallback(async () => {
    const requestId = loadRequestId.current + 1;
    loadRequestId.current = requestId;
    if (!client) {
      setPreview(errorApiState(new Error("Backend client is unavailable.")));
      return;
    }
    if (!validateSource()) {
      resetPatchState();
      setPreview(idleApiState());
      return;
    }

    resetPatchState();
    setPreview(loadingApiState());
    const requestedKey = sourceKey;
    const isCurrentRequest = () =>
      loadRequestId.current === requestId &&
      currentSourceKey.current === requestedKey;
    const settleStaleRequest = () => {
      if (loadRequestId.current === requestId) {
        resetPatchState();
        setPreview(idleApiState());
      }
    };
    const nextSnapshot = await loadApiResource(createSourceSnapshot);
    if (!isCurrentRequest()) {
      settleStaleRequest();
      return;
    }
    if (nextSnapshot.status !== "success") {
      setPreview(
        errorApiState(
          nextSnapshot.status === "error"
            ? nextSnapshot.error
            : new Error("Snapshot creation did not complete."),
        ),
      );
      return;
    }

    const nextFiles = await loadApiResource(() =>
      client.listChangedFiles(nextSnapshot.data.id),
    );
    if (!isCurrentRequest()) {
      settleStaleRequest();
      return;
    }
    if (nextFiles.status !== "success") {
      setPreview(
        errorApiState(
          nextFiles.status === "error"
            ? nextFiles.error
            : new Error("Changed files did not load."),
        ),
      );
      return;
    }

    setPreview(
      successApiState({
        key: requestedKey,
        snapshot: nextSnapshot.data,
        files: nextFiles.data,
      }),
    );
    setExpandedFileState({
      ids: defaultExpandedSetupFileIds(nextFiles.data),
      key: requestedKey,
    });
  }, [
    client,
    createSourceSnapshot,
    resetPatchState,
    sourceKey,
    validateSource,
  ]);

  const loadMoreFiles = useCallback(() => {
    setRenderedFileCount((current) =>
      Math.min(
        current + setupDiffFileRenderBatchSize,
        Math.min(files.length, setupMaxRenderedDiffFiles),
      ),
    );
  }, [files.length]);

  const setFileExpanded = useCallback(
    (fileId: string, expanded: boolean) => {
      if (!previewKey) {
        return;
      }
      setExpandedFileState((current) => {
        const currentIds =
          current.key === previewKey ? current.ids : emptyExpandedFileIds;
        const next = new Set(currentIds);
        if (expanded) {
          next.add(fileId);
        } else {
          next.delete(fileId);
        }
        return { ids: next, key: previewKey };
      });
    },
    [previewKey],
  );

  useLayoutEffect(() => {
    currentSourceKey.current = sourceKey;
  }, [sourceKey]);

  useEffect(() => {
    let canceled = false;
    if (!client || !ready) {
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

    setPatchPreviews((current) => {
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
          setPatchPreviews((current) => ({
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
  }, [client, patchLoadFiles, ready]);

  return {
    files,
    expandedFileIds,
    load,
    loadMoreFiles,
    patchPreviews,
    preview,
    ready,
    renderedFileCount,
    reset,
    setFileExpanded,
    snapshot,
    stats,
  };
}

const emptyExpandedFileIds = new Set<string>();
const defaultExpandedFileCount = 3;

function defaultExpandedSetupFileIds(files: SetupSourcePreview["files"]) {
  const expanded = new Set<string>();
  for (const file of files) {
    if (expanded.size >= defaultExpandedFileCount) {
      break;
    }
    if (file.is_binary || !file.patch_artifact_id) {
      continue;
    }
    expanded.add(file.id);
  }
  return expanded;
}
