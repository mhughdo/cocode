import { useEffect, useMemo, useState } from "react";

import { ErrorState, LoadingRows } from "@/components/app/chrome";
import {
  type AgentConfig,
  type ApiClient,
  type AskFindingQuestionResponse,
  errorApiState,
  type Finding,
  type FindingDetailResponse,
  type FindingThreadView,
  idleApiState,
  type Loadable,
  loadApiResource,
  loadingApiState,
  type ReviewContextPolicy,
  type ReviewEvent,
  successApiState,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import { AgentRuntimeTrace } from "../chat/agent-runtime-trace";
import { CodeSnippetViewer } from "./finding-components";
import { detailedFindingDraftComment } from "./finding-copy";
import { FollowUpMessages, followUpRuntimeEvents } from "./finding-thread";
import { MessageComposer } from "../chat/message-composer";
import { ReviewBreadcrumb } from "../shared/review-breadcrumb";
import {
  formatFindingLocation,
  truncate,
} from "../evidence/review-evidence-utils";
import { FindingsInspectorPanel } from "../evidence/findings-inspector-panel";
import {
  ResizableRightPanelHandle,
  useResizableRightPanel,
} from "../shared/resizable-right-panel";
import { panelMotionClass, usePanelPresence } from "../shared/panel-motion";

export function FindingDetailScreen({
  agentConfigs,
  client,
  events,
  finding,
  globalRightPanelOpen,
  onBack,
  onOpenEvidenceMap,
  onOpenFollowUp,
}: {
  agentConfigs: Loadable<AgentConfig[]>;
  client: ApiClient | null;
  events: ReviewEvent[];
  finding: Finding;
  globalRightPanelOpen?: boolean;
  onBack: () => void;
  onOpenEvidenceMap: (finding: Finding) => void;
  onOpenFollowUp: (finding: Finding) => void;
}) {
  const [detailState, setDetailState] =
    useState<Loadable<FindingDetailResponse>>(loadingApiState());
  const [threadState, setThreadState] =
    useState<Loadable<FindingThreadView>>(loadingApiState());
  const [draftComment, setDraftComment] = useState("");
  const [question, setQuestion] = useState("");
  const [selectedAgentId, setSelectedAgentId] = useState("");
  const [actionState, setActionState] =
    useState<Loadable<FindingDetailResponse | AskFindingQuestionResponse>>(
      idleApiState(),
    );

  useEffect(() => {
    let canceled = false;
    const api = client;
    const findingId = finding.id;
    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      if (!api) {
        const error = new Error("Backend client is unavailable");
        setDetailState(errorApiState(error));
        setThreadState(errorApiState(error));
        return;
      }
      setDetailState(loadingApiState());
      setThreadState(loadingApiState());
      setDraftComment("");
      setActionState(idleApiState());
      void Promise.all([
        loadApiResource(() => api.getFindingDetail(findingId)),
        loadApiResource(() => api.getFindingThread(findingId)),
      ]).then(([detail, thread]) => {
        if (canceled) {
          return;
        }
        if (
          detail.status === "success" &&
          detail.data.finding.id !== findingId
        ) {
          return;
        }
        setDetailState(detail);
        setThreadState(thread);
        if (detail.status === "success") {
          setDraftComment(
            detail.data.finding.draft_comment ||
              detailedFindingDraftComment(detail.data.finding, detail.data),
          );
        }
      });
    });
    return () => {
      canceled = true;
    };
  }, [client, finding.id]);

  const detail =
    detailState.status === "success" ? detailState.data : undefined;
  const activeFinding = detail?.finding ?? finding;
  const runtimeEvents = useMemo(
    () => followUpRuntimeEvents(events, activeFinding.id),
    [activeFinding.id, events],
  );
  const inspectorPanel = useResizableRightPanel({
    defaultWidth: 500,
    maxWidth: 760,
    minWidth: 360,
  });
  const showInspectorPanel = !globalRightPanelOpen;
  const inspectorPresence = usePanelPresence(
    Boolean(detail && showInspectorPanel),
  );
  const inspectorLayoutActive = Boolean(inspectorPresence.rendered && detail);
  const inspectorVisible = inspectorPresence.visible && showInspectorPanel;
  const agents = agentConfigs.status === "success" ? agentConfigs.data : [];
  const followUpAgents = agents.filter(
    (agent) => agent.enabled && !agent.capabilities.can_write,
  );

  async function updateDecision(decision: "accepted" | "dismissed") {
    if (!client) {
      setActionState(errorApiState(new Error("Backend client is unavailable")));
      return;
    }
    setActionState(loadingApiState());
    const state = await loadApiResource(() =>
      client.updateFindingDecision(activeFinding.id, {
        decision,
        reason:
          decision === "dismissed"
            ? "dismissed from finding detail"
            : "accepted from finding detail",
      }),
    );
    setActionState(state);
    if (state.status === "success") {
      setDetailState(state);
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
      <div className="min-w-0">
        <ReviewBreadcrumb
          items={[
            { label: "Findings", onClick: onBack },
            { label: truncate(activeFinding.canonical_claim, 88) },
          ]}
        />
        <h2 className="text-xl leading-7 font-semibold break-words">
          {activeFinding.canonical_claim}
        </h2>
        <p className="text-muted-foreground mt-1 font-mono text-xs break-all">
          {formatFindingLocation(activeFinding)}
        </p>
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
        <div
          className={cn(
            "grid min-w-0 gap-4 transition-[grid-template-columns]",
            inspectorPanel.resizing ? "transition-none" : panelMotionClass,
            inspectorLayoutActive
              ? "xl:grid-cols-[minmax(0,1fr)_minmax(360px,var(--right-panel-width))]"
              : "grid-cols-1",
          )}
          style={inspectorLayoutActive ? inspectorPanel.gridStyle : undefined}
        >
          <div className="flex min-w-0 flex-col gap-4">
            <section className="cocode-panel min-w-0 overflow-hidden">
              <div className="border-b px-4 py-3">
                <div className="text-sm font-semibold">Issue location</div>
                <div className="text-muted-foreground mt-1 font-mono text-xs break-all">
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

            <section className="cocode-panel">
              <div className="border-b px-4 py-3">
                <div className="text-sm font-semibold">Finding thread</div>
                <div className="text-muted-foreground mt-1 text-xs">
                  Ask scoped questions and keep the answers attached to this
                  finding.
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

          {inspectorLayoutActive && (
            <div
              aria-hidden={!inspectorVisible}
              className={cn(
                "relative min-w-0 transform-gpu transition-[opacity,transform] will-change-transform",
                panelMotionClass,
                inspectorVisible
                  ? "translate-x-0 opacity-100"
                  : "pointer-events-none translate-x-8 opacity-0",
              )}
            >
              <ResizableRightPanelHandle
                onPointerDown={inspectorPanel.startResize}
              />
              <FindingsInspectorPanel
                actionState={{
                  status:
                    actionState.status === "error"
                      ? "error"
                      : actionState.status,
                  message:
                    actionState.status === "error"
                      ? actionState.error.message
                      : actionState.status === "success"
                        ? "Changes saved"
                        : undefined,
                }}
                detail={detail}
                draftComment={draftComment}
                finding={activeFinding}
                onAccept={() => void updateDecision("accepted")}
                onCopyFixPacket={() => void copyDraftComment()}
                onCopyPath={() => {
                  void window.cocode?.writeClipboard?.(
                    formatFindingLocation(activeFinding),
                  );
                }}
                onDismiss={() => void updateDecision("dismissed")}
                onDraftCommentChange={setDraftComment}
                onOpenEvidenceMap={() => onOpenEvidenceMap(activeFinding)}
                onOpenFollowUp={() => onOpenFollowUp(activeFinding)}
                onSaveDraftComment={() => void saveDraftComment()}
              />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
