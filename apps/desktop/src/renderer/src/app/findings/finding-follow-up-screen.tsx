import { useEffect, useMemo, useState } from "react";
import {
  ArrowLeftIcon,
  BotIcon,
  CheckIcon,
  CopyIcon,
  FileSearchIcon,
  SearchIcon,
} from "lucide-react";

import {
  EmptyState,
  ErrorState,
  LoadingRows,
  PaneHeader,
} from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  type AgentConfig,
  type ApiClient,
  type AskFindingQuestionResponse,
  errorApiState,
  type Finding,
  type FindingDetailResponse,
  type FindingQuickActionResponse,
  type FindingThreadView,
  idleApiState,
  type Loadable,
  loadApiResource,
  loadingApiState,
  type ReviewContextPolicy,
  type ReviewEvent,
  successApiState,
} from "@/lib/api";
import { AgentRuntimeTrace } from "../chat/agent-runtime-trace";
import { FollowUpMessages, followUpRuntimeEvents } from "./finding-thread";
import { MessageComposer } from "../chat/message-composer";
import {
  evidenceBadgeVariant,
  evidenceKindDisplayLabel,
  formatEvidenceLocation,
  formatFindingLocation,
  prioritizedEvidenceItems,
} from "../evidence/review-evidence-utils";

export function FindingFollowUpScreen({
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
    <div className="flex min-h-0 flex-col gap-4">
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
                      {evidenceKindDisplayLabel(item.kind)}
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
