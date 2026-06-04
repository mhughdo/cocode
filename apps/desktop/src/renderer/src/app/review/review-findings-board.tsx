import { useEffect, useState } from "react";
import { FileSearchIcon, SearchIcon, ShieldCheckIcon } from "lucide-react";

import { EmptyState, ErrorState, LoadingRows } from "@/components/app/chrome";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  type ApiClient,
  type Finding,
  type FindingDetailResponse,
  type FindingListResponse,
  idleApiState,
  type Loadable,
  loadApiResource,
  loadingApiState,
  type ReviewSession,
  successApiState,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  FindingCard,
  type FindingDecisionMutation,
} from "../findings/finding-components";
import {
  detailedFindingDraftComment,
  findingClipboardText,
} from "../findings/finding-copy";
import {
  formatDecisionLabel,
  formatFindingLocation,
  shortPath,
} from "../evidence/review-evidence-utils";
import { FindingsInspectorPanel } from "../evidence/findings-inspector-panel";
import {
  ResizableRightPanelHandle,
  useResizableRightPanel,
} from "../shared/resizable-right-panel";
import { panelMotionClass, usePanelPresence } from "../shared/panel-motion";

const MAX_FINDINGS_RENDERED = 150;

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

export function ReviewFindingsBoard({
  client,
  findings,
  globalRightPanelOpen,
  onOpenDetail,
  onOpenEvidenceMap,
  onOpenFollowUp,
  session,
}: {
  client: ApiClient | null;
  findings: Loadable<FindingListResponse>;
  globalRightPanelOpen?: boolean;
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
  const [draftComment, setDraftComment] = useState("");
  const inspectorPanel = useResizableRightPanel({
    defaultWidth: 500,
    maxWidth: 760,
    minWidth: 360,
  });
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
    let canceled = false;
    queueMicrotask(() => {
      if (!canceled) {
        setSelectedFindingId(null);
        setSelectedDetail(idleApiState());
        setDraftComment("");
        setActionState({ status: "idle" });
      }
    });
    return () => {
      canceled = true;
    };
  }, [boardSessionId]);

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
    if (!client || !selectedFindingId || !boardSessionId) {
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
    const sessionId = boardSessionId;
    let canceled = false;
    async function load() {
      setSelectedDetail(loadingApiState());
      const state = await loadApiResource(() =>
        api.getFindingDetail(findingId),
      );
      if (canceled) {
        return;
      }
      if (
        state.status === "success" &&
        state.data.finding.review_session_id !== sessionId
      ) {
        return;
      }
      setSelectedDetail(state);
    }

    queueMicrotask(() => {
      if (!canceled) {
        void load();
      }
    });
    return () => {
      canceled = true;
    };
  }, [boardSessionId, client, selectedFindingId]);

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
    selectedFindingId && selectedDetail.status === "success"
      ? selectedDetail.data
      : undefined;
  const selectedFinding =
    selectedFindingDetail?.finding ??
    (selectedDetail.status === "idle" || selectedDetail.status === "loading"
      ? selectedFindingFromList
      : undefined);
  const showInspectorPanel = Boolean(selectedFinding && !globalRightPanelOpen);
  const inspectorPresence = usePanelPresence(showInspectorPanel);
  const inspectorLayoutActive = Boolean(
    inspectorPresence.rendered && selectedFinding,
  );
  const inspectorVisible = inspectorPresence.visible && showInspectorPanel;
  async function updateDecision(
    decision: FindingDecisionMutation,
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
        ? "dismissed from findings board"
        : `${formatDecisionLabel(decision).toLowerCase()} from findings board`;
    setActionState({
      status: "loading",
      findingId: finding.id,
      action: decision,
    });
    const state = await loadApiResource(() =>
      client.updateFindingDecision(finding.id, {
        decision,
        reason,
      }),
    );
    if (state.status === "success") {
      patchBoardFinding(state.data.finding);
      if (selectedFindingId === finding.id) {
        setSelectedDetail(state);
      }
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
    if (!finding) {
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
      return true;
    });
    if (state.status === "success") {
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
      patchBoardFinding(state.data.finding);
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

  const filterOptions =
    listState.status === "success" ? listState.data.filters : undefined;

  return (
    <section
      aria-label="Review findings board"
      className="bg-card border-border-subtle flex h-full min-h-[calc(100vh-220px)] min-w-0 flex-col overflow-hidden rounded-xl border"
    >
      <div className="border-border-subtle bg-surface/40 flex flex-wrap items-center gap-2 border-b p-4">
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

      <div
        style={inspectorLayoutActive ? inspectorPanel.gridStyle : undefined}
        className={cn(
          "grid min-h-0 flex-1 bg-white transition-[grid-template-columns]",
          inspectorPanel.resizing ? "transition-none" : panelMotionClass,
          inspectorLayoutActive
            ? "xl:grid-cols-[minmax(0,1fr)_minmax(360px,var(--right-panel-width))]"
            : "grid-cols-1",
        )}
      >
        <div className="min-w-0 overflow-auto [scrollbar-width:thin]">
          <div
            className={cn(
              "bg-surface/60 text-muted-foreground grid gap-3 border-b px-4 py-2 text-xs font-medium max-lg:hidden",
              inspectorLayoutActive
                ? "grid-cols-[72px_minmax(0,1fr)_132px_72px]"
                : "grid-cols-[72px_minmax(0,1.35fr)_minmax(96px,0.64fr)_132px_minmax(112px,0.64fr)_72px]",
            )}
          >
            <span>Severity</span>
            <span>Finding</span>
            {!showInspectorPanel ? <span>Location</span> : null}
            <span>Status</span>
            {!showInspectorPanel ? <span>Source / agents</span> : null}
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
              layout={inspectorLayoutActive ? "inspector" : "wide"}
              selected={finding.id === selectedFindingId}
              onDecisionChange={(decision) => {
                void updateDecision(decision, finding);
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
        {inspectorLayoutActive && selectedFinding && (
          <div
            aria-hidden={!inspectorVisible}
            className={cn(
              "bg-surface relative flex min-h-0 min-w-0 transform-gpu overflow-hidden border-t p-4 transition-[opacity,transform] will-change-transform xl:border-t-0 xl:border-l",
              panelMotionClass,
              inspectorVisible
                ? "translate-x-0 opacity-100"
                : "pointer-events-none translate-x-8 opacity-0",
            )}
          >
            <ResizableRightPanelHandle
              onPointerDown={inspectorPanel.startResize}
            />
            {selectedDetail.status === "error" ? (
              <ErrorState
                title="Finding detail unavailable"
                description={selectedDetail.error.message}
              />
            ) : (
              <FindingsInspectorPanel
                actionState={{
                  status: actionState.status,
                  message: actionState.message,
                }}
                className="h-full flex-1"
                detail={
                  selectedDetail.status === "success"
                    ? selectedDetail.data
                    : undefined
                }
                draftComment={draftComment}
                finding={selectedFinding}
                onAccept={() => void updateDecision("accepted")}
                onCopyFixPacket={() => void copyFinding()}
                onCopyPath={() => void copyFindingPath()}
                onDismiss={() => void updateDecision("dismissed")}
                onDraftCommentChange={setDraftComment}
                onOpenDetail={() => onOpenDetail(selectedFinding)}
                onOpenEvidenceMap={() => onOpenEvidenceMap(selectedFinding)}
                onOpenFollowUp={() => onOpenFollowUp(selectedFinding)}
                onSaveDraftComment={() => void saveDraftComment()}
              />
            )}
          </div>
        )}
      </div>
    </section>
  );

  function patchBoardFinding(updated: Finding) {
    setBoardFindings((current) => {
      if (current.status !== "success") {
        return current;
      }
      return successApiState({
        ...current.data,
        items: current.data.items.map((item) =>
          item.id === updated.id ? updated : item,
        ),
      });
    });
  }
}

function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const timeout = window.setTimeout(() => setDebounced(value), delayMs);
    return () => window.clearTimeout(timeout);
  }, [delayMs, value]);

  return debounced;
}
