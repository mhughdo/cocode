import {
  type CSSProperties,
  type UIEvent,
  type WheelEvent,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  ChevronDownIcon,
  FileSearchIcon,
  FileTextIcon,
  RefreshCwIcon,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type {
  ChangedFile,
  ChangedFilePatch,
  Loadable,
  Snapshot,
} from "@/lib/api";
import {
  type HighlightedCodeLine,
  highlightCodeLines,
  languageForFilePath,
} from "@/lib/syntax-highlighting";
import { cn } from "@/lib/utils";

export type SetupSourcePreview = {
  key: string;
  snapshot: Snapshot;
  files: ChangedFile[];
};

export type SetupPreviewStats = {
  total: number;
  reviewable: number;
  additions: number;
  deletions: number;
  generated: number;
  binary: number;
  excluded: number;
};

export const sourceInspectorMinWidth = 380;
export const sourceInspectorDefaultWidth = 760;
export const sourceInspectorMaxWidth = 1280;
export const sourceInspectorMainMinWidth = 520;
export const sourceInspectorOverlayGutter = 16;
export const sourceInspectorSideBySideMinWidth = 1180;
export const sourceInspectorTransitionMs = 220;
export const setupInitialDiffFileRenderCount = 6;
export const setupDiffFileRenderBatchSize = 6;
export const setupMaxRenderedDiffFiles = 200;

const emptyCollapsedFileIds = new Set<string>();

export function SetupSourceInspectorPanel({
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
    maybeLoadMoreDiffFiles(target);
  }

  function handleStackWheel(event: WheelEvent<HTMLElement>) {
    if (Math.abs(event.deltaY) <= Math.abs(event.deltaX)) {
      return;
    }
    const target =
      event.target instanceof HTMLElement ? event.target : undefined;
    if (!target?.closest("[data-setup-diff-scroll]")) {
      return;
    }
    const scroller = stackScrollRef.current;
    if (!scroller) {
      return;
    }
    const maxScrollTop = scroller.scrollHeight - scroller.clientHeight;
    if (maxScrollTop <= 0) {
      return;
    }
    const nextScrollTop = Math.min(
      maxScrollTop,
      Math.max(0, scroller.scrollTop + event.deltaY),
    );
    if (nextScrollTop === scroller.scrollTop) {
      return;
    }
    event.preventDefault();
    scroller.scrollTop = nextScrollTop;
    maybeLoadMoreDiffFiles(scroller);
  }

  function maybeLoadMoreDiffFiles(target: HTMLElement) {
    if (
      hiddenFileCount > 0 &&
      target.scrollTop + target.clientHeight >= target.scrollHeight - 560
    ) {
      onLoadMoreFiles();
    }
  }

  return (
    <section
      ref={stackScrollRef}
      className="h-full overflow-y-auto bg-white [-webkit-overflow-scrolling:touch] [scrollbar-gutter:stable]"
      data-testid="setup-source-stack-scroll"
      onScroll={handleStackScroll}
      onWheel={handleStackWheel}
    >
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
          <div className="border-border/70 text-muted-foreground mx-4 mt-4 flex min-h-[220px] items-center justify-center rounded-lg border bg-white">
            <FileSearchIcon className="mr-2 size-4" />
            <span className="text-[0.78rem]">Not loaded</span>
          </div>
        )}

      {preview.status === "success" && previewReady && (
        <div className="flex flex-col">
          <div className="border-border/60 sticky top-0 z-10 flex min-h-10 shrink-0 items-center justify-between gap-3 border-y bg-white px-4">
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

          <div>
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
                <SetupDiffContent file={file} patch={patchPreview.data} />
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

function SetupDiffContent({
  file,
  patch,
}: {
  file: ChangedFile;
  patch: ChangedFilePatch;
}) {
  const rows = useMemo(
    () => setupSideBySideDiffRows(patch.content),
    [patch.content],
  );
  const language = useMemo(() => languageForFilePath(file.path), [file.path]);
  const oldCode = useMemo(
    () => rows.map((row) => row.oldText).join("\n"),
    [rows],
  );
  const newCode = useMemo(
    () => rows.map((row) => row.newText).join("\n"),
    [rows],
  );
  const [highlightedDiff, setHighlightedDiff] =
    useState<SetupHighlightedDiff | null>(null);

  useEffect(() => {
    let active = true;
    void Promise.all([
      highlightCodeLines(oldCode, language),
      highlightCodeLines(newCode, language),
    ]).then(([oldLines, newLines]) => {
      if (active) {
        setHighlightedDiff({ newLines, oldLines });
      }
    });
    return () => {
      active = false;
    };
  }, [language, newCode, oldCode]);

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
        className="overflow-x-auto bg-white [scrollbar-gutter:stable_both-edges]"
        data-setup-diff-scroll
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
              newLine={highlightedDiff?.newLines[index]}
              oldLine={highlightedDiff?.oldLines[index]}
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

function SetupSideBySideDiffRow({
  newLine,
  oldLine,
  row,
}: {
  newLine?: HighlightedCodeLine;
  oldLine?: HighlightedCodeLine;
  row: SetupSideBySideDiffRowData;
}) {
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
        <SetupHighlightedCodeLine line={oldLine} text={row.oldText} />
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
        <SetupHighlightedCodeLine line={newLine} text={row.newText} />
      </code>
    </>
  );
}

function SetupHighlightedCodeLine({
  line,
  text,
}: {
  line?: HighlightedCodeLine;
  text: string;
}) {
  if (text === "") {
    return " ";
  }
  if (!line || line.length === 0) {
    return text;
  }
  return (
    <>
      {line.map((token, index) => (
        <span
          key={`${index}:${token.content}`}
          style={setupSyntaxTokenStyle(token)}
        >
          {token.content}
        </span>
      ))}
    </>
  );
}

function setupSyntaxTokenStyle(token: HighlightedCodeLine[number]) {
  const style: CSSProperties = {};
  if (token.color) {
    style.color = token.color;
  }
  if (token.fontStyle && token.fontStyle > 0) {
    if (token.fontStyle & 1) {
      style.fontStyle = "italic";
    }
    if (token.fontStyle & 2) {
      style.fontWeight = 600;
    }
    if (token.fontStyle & 4) {
      style.textDecoration = "underline";
    }
  }
  return Object.keys(style).length > 0 ? style : undefined;
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

type SetupHighlightedDiff = {
  newLines: HighlightedCodeLine[];
  oldLines: HighlightedCodeLine[];
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

export function setupPreviewStats(files: ChangedFile[]): SetupPreviewStats {
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
