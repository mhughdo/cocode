import {
  type FormEvent,
  type KeyboardEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Loader2Icon, MessageSquareIcon, SendIcon } from "lucide-react";

import { EmptyState, LoadingRows } from "@/components/app/chrome";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  type AgentConfig,
  type ApiClient,
  type ChatMessage,
  type ChatThreadView,
  type Finding,
  type FindingListResponse,
  type Loadable,
  type ReviewEvent,
  type ReviewSession,
  type ReviewSessionAgent,
  type ReviewSessionSummary,
  errorApiState,
  loadApiResource,
  loadingApiState,
  successApiState,
} from "@/lib/api";
import { ChatMessageCard, ResponderDropdown } from "./chat-message-card";
import {
  pendingChatMessages,
  withLiveAgentRunMessages,
} from "./chat-live-messages";
import { CentralizedChatRail } from "./chat-rail";
import {
  agentByID,
  compactAgentLabel,
  compactRoleLabel,
  isOrchestratorEntry,
} from "./chat-message-utils";
import type { ChatAudience, ChatResponderOption } from "./chat-types";
import { FinalFindingsMessage } from "./final-findings-message";

const chatThreadCache = new Map<string, ChatThreadView>();

export function CentralizedChatScreen({
  agentConfigs,
  client,
  events,
  findings,
  onOpenFindingDetail,
  onOpenFindings,
  session,
  summary,
}: {
  agentConfigs: Loadable<AgentConfig[]>;
  client: ApiClient | null;
  events: ReviewEvent[];
  findings: Loadable<FindingListResponse>;
  onOpenFindingDetail: (finding: Finding) => void;
  onOpenFindings: () => void;
  session: ReviewSession;
  summary: Loadable<ReviewSessionSummary>;
}) {
  const [thread, setThread] = useState<Loadable<ChatThreadView>>(() => {
    const cached = chatThreadCache.get(session.id);
    return cached ? successApiState(cached) : loadingApiState();
  });
  const [message, setMessage] = useState("");
  const [responderID, setResponderID] = useState("orchestrator");
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [pendingAgentMessages, setPendingAgentMessages] = useState<
    ChatMessage[]
  >([]);
  const messageListRef = useRef<HTMLDivElement | null>(null);
  const shouldStickToBottomRef = useRef(true);

  const agentByConfigID = useMemo(
    () =>
      agentConfigs.status === "success"
        ? new Map(agentConfigs.data.map((agent) => [agent.id, agent]))
        : new Map<string, AgentConfig>(),
    [agentConfigs],
  );
  const sessionAgentEntries = useMemo(() => {
    return session.agents
      .filter((assignment) => assignment.enabled)
      .map((assignment) => {
        const agent = agentByConfigID.get(assignment.agent_config_id);
        return agent ? { agent, assignment } : null;
      })
      .filter(
        (
          entry,
        ): entry is { agent: AgentConfig; assignment: ReviewSessionAgent } =>
          Boolean(entry),
      );
  }, [agentByConfigID, session.agents]);
  const orchestratorAgent = useMemo(() => {
    return (
      sessionAgentEntries.find((entry) => isOrchestratorEntry(entry))?.agent ??
      sessionAgentEntries[0]?.agent
    );
  }, [sessionAgentEntries]);
  const allSessionAgents = useMemo(
    () => sessionAgentEntries.map((entry) => entry.agent),
    [sessionAgentEntries],
  );
  const responderOptions = useMemo<ChatResponderOption[]>(
    () =>
      [...sessionAgentEntries]
        .sort(
          (left, right) =>
            left.assignment.run_order - right.assignment.run_order,
        )
        .map(({ agent, assignment }) => {
          const isOrchestrator = isOrchestratorEntry({ agent, assignment });
          return {
            id: isOrchestrator ? "orchestrator" : `agent:${agent.id}`,
            label: isOrchestrator ? "Orchestrator" : compactAgentLabel(agent),
            description: isOrchestrator
              ? "Coordinate and synthesize review state."
              : `${compactRoleLabel(assignment.role || agent.role || "Reviewer")} in this review.`,
            icon: isOrchestrator ? "orchestrator" : "agent",
            agentConfigId: agent.id,
          } satisfies ChatResponderOption;
        }),
    [sessionAgentEntries],
  );
  const selectedResponder =
    responderOptions.find((option) => option.id === responderID) ??
    responderOptions.find((option) => option.id === "orchestrator") ??
    responderOptions[0] ?? {
      id: "orchestrator",
      label: "Orchestrator",
      description: "Coordinate and synthesize review state.",
      icon: "orchestrator",
      agentConfigId: orchestratorAgent?.id,
    };
  const selectedAudience: ChatAudience =
    selectedResponder.id === "orchestrator" ? "orchestrator" : "selected_agent";

  const setSuccessfulThread = useCallback(
    (view: ChatThreadView) => {
      chatThreadCache.set(session.id, view);
      setThread(successApiState(view));
    },
    [session.id],
  );

  const refreshThread = useCallback(async () => {
    const cached = chatThreadCache.get(session.id);
    const next = await loadApiResource(() =>
      client
        ? client.getReviewSessionChatThread(session.id)
        : Promise.reject(new Error("Backend client is unavailable")),
    );
    if (next.status === "success") {
      setSuccessfulThread(next.data);
      return;
    }
    if (!cached) {
      setThread(next);
    }
  }, [client, session.id, setSuccessfulThread]);

  useEffect(() => {
    let canceled = false;
    const cached = chatThreadCache.get(session.id);
    queueMicrotask(() => {
      if (!canceled) {
        setThread(cached ? successApiState(cached) : loadingApiState());
      }
    });
    void loadApiResource(() =>
      client
        ? client.getReviewSessionChatThread(session.id)
        : Promise.reject(new Error("Backend client is unavailable")),
    ).then((state) => {
      if (canceled) {
        return;
      }
      if (state.status === "success") {
        setSuccessfulThread(state.data);
        return;
      }
      if (!cached) {
        setThread(state);
      }
    });
    return () => {
      canceled = true;
    };
  }, [client, session.id, setSuccessfulThread]);

  useEffect(() => {
    if (events.length === 0 && session.status !== "queued") {
      return;
    }
    queueMicrotask(() => void refreshThread());
  }, [events.length, refreshThread, session.status]);

  useEffect(() => {
    if (!["queued", "running", "canceling"].includes(session.status)) {
      return;
    }
    const interval = window.setInterval(() => void refreshThread(), 2000);
    return () => window.clearInterval(interval);
  }, [refreshThread, session.status]);

  const liveMessages = useMemo(
    () =>
      thread.status === "success"
        ? withLiveAgentRunMessages({
            agentConfigs,
            events,
            messages: thread.data.messages,
            session,
            summary,
            threadID: thread.data.thread.id,
          })
        : [],
    [agentConfigs, events, session, summary, thread],
  );
  const displayedMessages = useMemo(
    () =>
      [...liveMessages, ...pendingAgentMessages].filter(
        (item) => item.author_type !== "system",
      ),
    [liveMessages, pendingAgentMessages],
  );
  const eventsByRunID = useMemo(() => {
    const next = new Map<string, ReviewEvent[]>();
    for (const event of events) {
      if (!event.agent_run_id || !event.type.startsWith("AgentRun")) {
        continue;
      }
      const runEvents = next.get(event.agent_run_id) ?? [];
      runEvents.push(event);
      next.set(event.agent_run_id, runEvents);
    }
    return next;
  }, [events]);
  const finalizedFindings =
    session.status === "completed" && findings.status === "success"
      ? findings.data.items
      : [];
  const messageCount = displayedMessages.length + finalizedFindings.length;
  useEffect(() => {
    const node = messageListRef.current;
    if (!node || !shouldStickToBottomRef.current) {
      return;
    }
    node.scrollTop = node.scrollHeight;
  }, [messageCount]);

  async function submitMessage(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const body = message.trim();
    if (!client || !body || !selectedResponder || submitting) {
      return;
    }
    setSubmitting(true);
    setSubmitError(null);
    const optimisticMessage: ChatMessage = {
      id: `local-${Date.now()}`,
      thread_id: thread.status === "success" ? thread.data.thread.id : "",
      author_type: "user",
      author_display_name: "You",
      body,
      status: "completed",
      metadata: { optimistic: true },
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };
    if (thread.status === "success") {
      setSuccessfulThread({
        ...thread.data,
        messages: [...thread.data.messages, optimisticMessage],
      });
    }
    setMessage("");
    setPendingAgentMessages(
      pendingChatMessages({
        agentByConfigID,
        audience: selectedAudience,
        responder: selectedResponder,
        sessionAgents: session.agents,
        threadID: thread.status === "success" ? thread.data.thread.id : "",
      }),
    );
    const request = {
      body,
      mode: "follow_up",
      audience: selectedAudience,
      include_evidence: true,
      include_recent_messages: true,
      ...(selectedResponder.agentConfigId
        ? { responder_agent_config_id: selectedResponder.agentConfigId }
        : {}),
    };
    const next = await loadApiResource(() =>
      client.createReviewSessionChatTurn(session.id, request),
    );
    if (next.status === "success") {
      setSuccessfulThread({
        thread: next.data.thread,
        messages: next.data.messages,
      });
    } else if (next.status === "error") {
      setSubmitError(next.error.message);
      setThread((current) => {
        if (current.status !== "success") {
          return errorApiState(next.error);
        }
        const failedView = {
          ...current.data,
          messages: current.data.messages.map((item) =>
            item.id === optimisticMessage.id
              ? { ...item, status: "failed" }
              : item,
          ),
        };
        chatThreadCache.set(session.id, failedView);
        return successApiState(failedView);
      });
    }
    setPendingAgentMessages([]);
    setSubmitting(false);
  }

  function handleComposerKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.nativeEvent.isComposing) {
      return;
    }
    if (event.key !== "Enter" || event.shiftKey) {
      return;
    }
    event.preventDefault();
    event.currentTarget.form?.requestSubmit();
  }

  return (
    <div className="grid h-full min-h-0 grid-cols-[minmax(0,1fr)_280px] gap-6 overflow-hidden max-xl:grid-cols-1">
      <div className="flex min-h-0 min-w-0 flex-col gap-3">
        <div
          aria-label="Centralized chat messages"
          className="min-h-0 flex-1 overflow-y-auto pr-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
          onScroll={(event) => {
            const node = event.currentTarget;
            shouldStickToBottomRef.current =
              node.scrollHeight - node.scrollTop - node.clientHeight < 80;
          }}
          ref={messageListRef}
        >
          {thread.status === "loading" && <LoadingRows rows={5} />}
          {thread.status === "error" && (
            <EmptyState
              title="Central chat is unavailable"
              description={thread.error.message}
              icon={MessageSquareIcon}
            />
          )}
          {thread.status === "success" &&
            displayedMessages.length === 0 &&
            finalizedFindings.length === 0 && (
            <EmptyState
              title="No chat messages yet"
              description="Start with a question for the orchestrator."
              icon={MessageSquareIcon}
            />
          )}
          {thread.status === "success" &&
            (displayedMessages.length > 0 || finalizedFindings.length > 0) && (
            <div className="flex flex-col gap-3">
              {displayedMessages.map((item) => (
                <ChatMessageCard
                  agent={agentByID(allSessionAgents, item.agent_config_id)}
                  events={
                    item.agent_run_id
                      ? (eventsByRunID.get(item.agent_run_id) ?? [])
                      : []
                  }
                  key={item.id}
                  message={item}
                />
              ))}
              {finalizedFindings.length > 0 && (
                <FinalFindingsMessage
                  findings={finalizedFindings}
                  onOpenFindingDetail={onOpenFindingDetail}
                  onOpenFindings={onOpenFindings}
                />
              )}
              {submitting && pendingAgentMessages.length === 0 && (
                <div className="text-muted-foreground flex items-center gap-2 rounded-xl border bg-white px-4 py-3 text-xs">
                  <Loader2Icon className="size-3.5 animate-spin" />
                  Waiting for {selectedResponder.label}
                </div>
              )}
            </div>
          )}
        </div>

        <form className="shrink-0" onSubmit={submitMessage}>
          <div className="border-border bg-surface-raised focus-within:border-foreground/35 rounded-xl border shadow-[0_1px_2px_rgba(17,18,20,0.04)]">
            <Textarea
              aria-label="Centralized review message"
              className="max-h-36 min-h-18 resize-none border-0 bg-transparent px-4 py-3 text-[13px] shadow-none focus-visible:ring-0"
              disabled={!client || submitting}
              onChange={(event) => setMessage(event.target.value)}
              onKeyDown={handleComposerKeyDown}
              placeholder="Ask cocode anything about this review..."
              value={message}
            />
            <div className="flex flex-wrap items-center justify-end gap-2 px-3 pb-3">
              <ResponderDropdown
                options={responderOptions}
                selected={selectedResponder}
                onSelect={setResponderID}
              />
              <Button
                aria-label="Send centralized chat message"
                className="size-9 rounded-full bg-[#141414] text-white hover:bg-[#2a2a2a]"
                disabled={!message.trim() || submitting || !client}
                size="icon"
                type="submit"
              >
                {submitting ? (
                  <Loader2Icon className="size-4 animate-spin" />
                ) : (
                  <SendIcon className="size-4" />
                )}
              </Button>
            </div>
          </div>
          {submitError && (
            <p className="text-destructive mt-2 text-xs">{submitError}</p>
          )}
        </form>
      </div>

      <CentralizedChatRail
        events={events}
        findings={findings}
        onOpenFindings={onOpenFindings}
        session={session}
        summary={summary}
      />
    </div>
  );
}
