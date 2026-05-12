import {
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  AlertTriangleIcon,
  BotIcon,
  CheckIcon,
  ChevronDownIcon,
  CircleSlashIcon,
  ClockIcon,
  Loader2Icon,
  MessageSquareIcon,
  SendIcon,
  ShieldCheckIcon,
  SparklesIcon,
  UserIcon,
  UsersIcon,
} from "lucide-react";

import { EmptyState, LoadingRows } from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Textarea } from "@/components/ui/textarea";
import {
  type AgentConfig,
  type AgentRunSummary,
  type ApiClient,
  type ChatMessage,
  type ChatThreadView,
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
import { cn } from "@/lib/utils";
import {
  AgentRuntimeTrace,
  summarizeRuntimeTraceEvents,
  type RuntimeTraceSummary,
} from "./agent-runtime-trace";
import { agentLogoUrl, formatSetupAgentLabel } from "./agent-utils";
import { MarkdownMessage } from "./markdown-message";

import cocodeMarkUrl from "../../../../../../assets/app-icon/cocode-logo-mark.svg";

type ChatAudience = "orchestrator" | "all_agents" | "selected_agent";

type ChatResponderOption = {
  id: string;
  label: string;
  description: string;
  agentConfigId?: string;
  icon: "orchestrator" | "agent";
};

type ChatAskTargetOption = {
  id: ChatAudience;
  label: string;
  description: string;
  icon: "orchestrator" | "all" | "agent";
};

export function CentralizedChatScreen({
  agentConfigs,
  client,
  events,
  findings,
  onOpenFindings,
  session,
  summary,
}: {
  agentConfigs: Loadable<AgentConfig[]>;
  client: ApiClient | null;
  events: ReviewEvent[];
  findings: Loadable<FindingListResponse>;
  onOpenFindings: () => void;
  session: ReviewSession;
  summary: Loadable<ReviewSessionSummary>;
}) {
  const [thread, setThread] =
    useState<Loadable<ChatThreadView>>(loadingApiState());
  const [message, setMessage] = useState("");
  const [askTargetID, setAskTargetID] = useState<ChatAudience>("all_agents");
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
  const agents = useMemo(
    () =>
      sessionAgentEntries
        .filter((entry) => !isOrchestratorEntry(entry))
        .map((entry) => entry.agent),
    [sessionAgentEntries],
  );
  const allSessionAgents = useMemo(
    () => sessionAgentEntries.map((entry) => entry.agent),
    [sessionAgentEntries],
  );
  const askTargetOptions = useMemo<ChatAskTargetOption[]>(
    () => [
      {
        id: "all_agents",
        label: "All review agents",
        description: `Fan out to ${agents.length || "all"} reviewer${
          agents.length === 1 ? "" : "s"
        } and synthesize.`,
        icon: "all",
      },
      {
        id: "orchestrator",
        label: "Orchestrator",
        description: "Ask cocode to answer from review state.",
        icon: "orchestrator",
      },
      {
        id: "selected_agent",
        label: "Selected reviewer",
        description: "Route to one configured CLI reviewer.",
        icon: "agent",
      },
    ],
    [agents.length],
  );

  const responderOptions = useMemo<ChatResponderOption[]>(
    () => [
      {
        id: "orchestrator",
        label: "Orchestrator",
        description: "cocode synthesizer",
        icon: "orchestrator",
        agentConfigId: orchestratorAgent?.id,
      },
      ...agents.map((agent) => ({
        id: `agent:${agent.id}`,
        label: compactAgentLabel(agent),
        description: agent.role || "Reviewer",
        agentConfigId: agent.id,
        icon: "agent" as const,
      })),
    ],
    [agents, orchestratorAgent?.id],
  );
  const selectedResponder =
    responderOptions.find((option) => option.id === responderID) ??
    responderOptions[0];
  const effectiveAskTargetID =
    agents.length === 0 && askTargetID === "all_agents"
      ? "orchestrator"
      : askTargetID;
  const selectedAskTarget =
    askTargetOptions.find((option) => option.id === effectiveAskTargetID) ??
    askTargetOptions[0];

  const refreshThread = useCallback(async () => {
    const next = await loadApiResource(() =>
      client
        ? client.getReviewSessionChatThread(session.id)
        : Promise.reject(new Error("Backend client is unavailable")),
    );
    setThread(next);
  }, [client, session.id]);

  useEffect(() => {
    let canceled = false;
    void loadApiResource(() =>
      client
        ? client.getReviewSessionChatThread(session.id)
        : Promise.reject(new Error("Backend client is unavailable")),
    ).then((state) => {
      if (!canceled) {
        setThread(state);
      }
    });
    return () => {
      canceled = true;
    };
  }, [client, session.id]);

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
  const messageCount = displayedMessages.length;
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
      setThread(
        successApiState({
          ...thread.data,
          messages: [...thread.data.messages, optimisticMessage],
        }),
      );
    }
    setMessage("");
    const audience =
      effectiveAskTargetID === "selected_agent" &&
      !selectedResponder.agentConfigId
        ? "orchestrator"
        : effectiveAskTargetID;
    setPendingAgentMessages(
      pendingChatMessages({
        agentByConfigID,
        audience,
        responder: selectedResponder,
        sessionAgents: session.agents,
        threadID: thread.status === "success" ? thread.data.thread.id : "",
      }),
    );
    const request = {
      body,
      mode: "follow_up",
      audience,
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
      setThread(
        successApiState({
          thread: next.data.thread,
          messages: next.data.messages,
        }),
      );
    } else if (next.status === "error") {
      setSubmitError(next.error.message);
      setThread((current) =>
        current.status === "success"
          ? successApiState({
              ...current.data,
              messages: current.data.messages.map((item) =>
                item.id === optimisticMessage.id
                  ? { ...item, status: "failed" }
                  : item,
              ),
            })
          : errorApiState(next.error),
      );
    }
    setPendingAgentMessages([]);
    setSubmitting(false);
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
          {thread.status === "success" && displayedMessages.length === 0 && (
            <EmptyState
              title="No chat messages yet"
              description="Start with a question for the orchestrator or a reviewer."
              icon={MessageSquareIcon}
            />
          )}
          {thread.status === "success" && displayedMessages.length > 0 && (
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
              onKeyDown={(event) => {
                if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
                  event.currentTarget.form?.requestSubmit();
                }
              }}
              placeholder="Ask cocode anything about this review..."
              value={message}
            />
            <div className="flex flex-wrap items-center justify-between gap-2 px-3 pb-3">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <AskTargetDropdown
                  options={askTargetOptions}
                  selected={selectedAskTarget}
                  onSelect={(id) => {
                    setAskTargetID(id);
                    if (id !== "selected_agent") {
                      setResponderID("orchestrator");
                    }
                  }}
                />
                <ResponderDropdown
                  options={responderOptions}
                  selected={selectedResponder}
                  onSelect={(id) => {
                    setResponderID(id);
                    if (id !== "orchestrator") {
                      setAskTargetID("selected_agent");
                    }
                  }}
                />
              </div>
              <Button
                aria-label="Send centralized chat message"
                className="rounded-lg bg-[#141414] text-white hover:bg-[#2a2a2a]"
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

function withLiveAgentRunMessages({
  agentConfigs,
  events,
  messages,
  session,
  summary,
  threadID,
}: {
  agentConfigs: Loadable<AgentConfig[]>;
  events: ReviewEvent[];
  messages: ChatMessage[];
  session: ReviewSession;
  summary: Loadable<ReviewSessionSummary>;
  threadID: string;
}) {
  const existingRunIDs = new Set(
    messages
      .map((message) => message.agent_run_id)
      .filter((id): id is string => Boolean(id)),
  );
  const agentByConfigID =
    agentConfigs.status === "success"
      ? new Map(agentConfigs.data.map((agent) => [agent.id, agent]))
      : new Map<string, AgentConfig>();
  const eventsByRunID = new Map<string, ReviewEvent[]>();
  for (const event of events) {
    if (!event.agent_run_id || !event.type.startsWith("AgentRun")) {
      continue;
    }
    const runEvents = eventsByRunID.get(event.agent_run_id) ?? [];
    runEvents.push(event);
    eventsByRunID.set(event.agent_run_id, runEvents);
  }
  const liveMessages: ChatMessage[] = [];
  const representedAgentConfigIDs = new Set(
    messages
      .map((message) => message.agent_config_id)
      .filter((id): id is string => Boolean(id)),
  );
  const summaryRuns =
    summary.status === "success" ? (summary.data.agent_runs ?? []) : [];
  for (const run of summaryRuns) {
    representedAgentConfigIDs.add(run.agent_config_id);
    if (existingRunIDs.has(run.id) || !isLiveAgentRun(run)) {
      continue;
    }
    const agent = agentByConfigID.get(run.agent_config_id);
    const runEvents = eventsByRunID.get(run.id) ?? [];
    const runtimeSummary = summarizeRuntimeTraceEvents(runEvents);
    const latestEvent = runEvents.at(-1);
    const timestamp =
      latestEvent?.created_at ?? run.started_at ?? session.updated_at;
    liveMessages.push({
      id: `live-${run.id}`,
      thread_id: threadID,
      author_type: "agent",
      author_display_name: agent ? compactAgentLabel(agent) : "Reviewer",
      agent_config_id: run.agent_config_id,
      agent_run_id: run.id,
      body: liveAgentRunBody(run, latestEvent, runtimeSummary, agent),
      status: "streaming",
      metadata: {
        live: true,
        agent_run_status: run.status,
        reasoning_events: runtimeSummary.reasoning.length,
        tool_call_events: runtimeSummary.toolCalls.length,
        output_events: runtimeSummary.output.length,
      },
      created_at: timestamp,
      updated_at: timestamp,
    });
  }
  const plannedMessages: ChatMessage[] = [];
  if (["queued", "running"].includes(session.status)) {
    const now = session.updated_at || new Date().toISOString();
    for (const assignment of session.agents) {
      if (!assignment.enabled) {
        continue;
      }
      if (representedAgentConfigIDs.has(assignment.agent_config_id)) {
        continue;
      }
      const agent = agentByConfigID.get(assignment.agent_config_id);
      if (agent && isOrchestratorEntry({ agent, assignment })) {
        continue;
      }
      if (!agent && isOrchestratorAssignment(assignment)) {
        continue;
      }
      const label = agent
        ? compactAgentLabel(agent)
        : assignment.role
          ? compactRoleLabel(assignment.role)
          : "Reviewer";
      plannedMessages.push({
        id: `planned-${session.id}-${assignment.agent_config_id}`,
        thread_id: threadID,
        author_type: "agent",
        author_display_name: label,
        agent_config_id: assignment.agent_config_id,
        body: `${label} is queued for an execution slot.`,
        status: "streaming",
        metadata: { local: true, planned: true, agent_run_status: "queued" },
        created_at: now,
        updated_at: now,
      });
    }
  }
  return [...messages, ...liveMessages, ...plannedMessages];
}

function pendingChatMessages({
  agentByConfigID,
  audience,
  responder,
  sessionAgents,
  threadID,
}: {
  agentByConfigID: Map<string, AgentConfig>;
  audience: ChatAudience;
  responder: ChatResponderOption;
  sessionAgents: ReviewSessionAgent[];
  threadID: string;
}): ChatMessage[] {
  const now = new Date().toISOString();
  const labelForAssignment = (assignment: ReviewSessionAgent) => {
    const agent = agentByConfigID.get(assignment.agent_config_id);
    if (agent) {
      return compactAgentLabel(agent);
    }
    if (assignment.role) {
      return compactRoleLabel(assignment.role);
    }
    return "Reviewer";
  };
  if (audience === "all_agents") {
    const reviewers = sessionAgents.filter(
      (assignment) =>
        assignment.enabled && !isOrchestratorAssignment(assignment),
    );
    if (reviewers.length > 0) {
      return reviewers.map((assignment) => {
        const label = labelForAssignment(assignment);
        return {
          id: `pending-${assignment.id}-${now}`,
          thread_id: threadID,
          author_type: "agent",
          author_display_name: label,
          agent_config_id: assignment.agent_config_id,
          body: `${label} is reading the review context and preparing an answer.`,
          status: "streaming",
          metadata: { local: true, pending: true },
          created_at: now,
          updated_at: now,
        };
      });
    }
  }
  if (audience === "selected_agent" && responder.agentConfigId) {
    const assignment = sessionAgents.find(
      (item) => item.agent_config_id === responder.agentConfigId,
    );
    return [
      {
        id: `pending-${responder.agentConfigId}-${now}`,
        thread_id: threadID,
        author_type: "agent",
        author_display_name:
          (assignment ? labelForAssignment(assignment) : "") || responder.label,
        agent_config_id: responder.agentConfigId,
        body: `${
          (assignment ? labelForAssignment(assignment) : "") || responder.label
        } is reading the review context and preparing an answer.`,
        status: "streaming",
        metadata: { local: true, pending: true },
        created_at: now,
        updated_at: now,
      },
    ];
  }
  return [
    {
      id: `pending-orchestrator-${now}`,
      thread_id: threadID,
      author_type: "orchestrator",
      author_display_name: "Orchestrator",
      body: "cocode is synthesizing the latest review state and agent evidence.",
      status: "streaming",
      metadata: { local: true, pending: true },
      created_at: now,
      updated_at: now,
    },
  ];
}

function isLiveAgentRun(run: AgentRunSummary) {
  return run.status === "queued" || run.status === "running";
}

function liveAgentRunBody(
  run: AgentRunSummary,
  latestEvent: ReviewEvent | undefined,
  runtimeSummary: RuntimeTraceSummary,
  agent: AgentConfig | undefined,
) {
  const label = agent ? compactAgentLabel(agent) : "Reviewer";
  const reasoning = lastNonEmpty(runtimeSummary.reasoning);
  if (reasoning) {
    return `**Visible reasoning**\n\n${reasoning.trim()}`;
  }
  const modelOutput = lastNonEmpty(runtimeSummary.output);
  if (modelOutput) {
    return modelOutput.trim();
  }
  if (runtimeSummary.toolCalls.length > 0) {
    return `${label} is using tools and checking evidence. Open the trace to inspect live commands and diagnostics.`;
  }
  if (run.status === "queued") {
    return `${label} is queued and waiting for an execution slot.`;
  }
  if (latestEvent?.type === "AgentRunOutput") {
    const stream = payloadString(latestEvent.payload.stream);
    return `${label} is streaming ${stream || "output"} back to cocode.`;
  }
  return `${label} is ${liveAgentRunWork(run)}.`;
}

function lastNonEmpty(items: string[]) {
  for (let index = items.length - 1; index >= 0; index--) {
    const value = items[index]?.trim();
    if (value) {
      return value;
    }
  }
  return "";
}

function liveAgentRunWork(run: AgentRunSummary) {
  const role = run.role.toLowerCase();
  if (role.includes("chat")) {
    return "answering your follow-up";
  }
  if (role.includes("verifier")) {
    return "checking evidence";
  }
  if (role.includes("context")) {
    return "building review context";
  }
  return "reviewing changed files";
}

function payloadString(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function metadataString(metadata: unknown, key: string, fallback = ""): string {
  if (!isPlainRecord(metadata)) {
    return fallback;
  }
  const value = metadata[key];
  return typeof value === "string" && value.trim() ? value.trim() : fallback;
}

function ChatMessageCard({
  agent,
  events,
  message,
}: {
  agent?: AgentConfig;
  events: ReviewEvent[];
  message: ChatMessage;
}) {
  const isUser = message.author_type === "user";
  const isSystem = message.author_type === "system";
  const failed = message.status === "failed";
  const streaming = message.status !== "completed" && !failed;
  const logo = agent ? agentLogoUrl(agent) : "";
  const runtimeSummary = useMemo(
    () => summarizeRuntimeTraceEvents(events),
    [events],
  );
  return (
    <article
      className={cn(
        "border-border/80 flex gap-3 rounded-xl border bg-white px-4 py-3 shadow-[0_1px_2px_rgba(17,18,20,0.03)]",
        isSystem && "bg-[#fbfbfa]",
        streaming && "border-dashed bg-[#fbfbfa]",
        failed && "border-destructive/30 bg-destructive/5",
      )}
    >
      <AgentAvatar
        authorType={message.author_type}
        isUser={isUser}
        logo={logo}
      />
      <div className="min-w-0 flex-1">
        <div className="mb-1 flex min-w-0 flex-wrap items-center gap-2 text-[13px]">
          <span className="font-semibold">
            {message.author_display_name ||
              displayNameForAuthor(message.author_type)}
          </span>
          <span className="text-muted-foreground text-xs">
            {formatClockTime(message.created_at)}
          </span>
          {message.agent_run_id && (
            <AgentRunBadges
              agent={agent}
              failed={failed}
              runtimeSummary={runtimeSummary}
              streaming={streaming}
            />
          )}
          {!message.agent_run_id && streaming && (
            <Badge variant="outline" className="h-4 gap-1 px-1.5 text-[10px]">
              <Loader2Icon className="size-3 animate-spin" />
              streaming
            </Badge>
          )}
          {!message.agent_run_id && failed && (
            <Badge variant="destructive" className="h-4 px-1.5 text-[10px]">
              failed
            </Badge>
          )}
        </div>
        <ExpandableMarkdownMessage
          content={message.body}
          muted={isSystem || streaming}
        />
        <ReasoningSummary metadata={message.metadata} />
        {message.agent_run_id && events.length > 0 && (
          <AgentRuntimeTrace
            className="mt-3"
            compact
            events={events}
            failed={failed}
          />
        )}
      </div>
    </article>
  );
}

function ExpandableMarkdownMessage({
  content,
  muted,
}: {
  content: string;
  muted?: boolean;
}) {
  const [expanded, setExpanded] = useState(false);
  const normalizedContent = content
    .replace(/\n{0,2}\.\.\.\[truncated\]\s*$/i, "")
    .trim();
  const lineCount = normalizedContent.split("\n").length;
  const needsExpansion = normalizedContent.length > 900 || lineCount > 14;

  return (
    <div className="min-w-0">
      <div
        className={cn(
          "relative min-w-0",
          needsExpansion && !expanded && "max-h-56 overflow-hidden",
        )}
      >
        <MarkdownMessage content={normalizedContent || content} muted={muted} />
        {needsExpansion && !expanded && (
          <div className="pointer-events-none absolute inset-x-0 bottom-0 h-16 bg-gradient-to-t from-white to-transparent" />
        )}
      </div>
      {needsExpansion && (
        <Button
          className="mt-2 h-7 px-2 text-xs"
          size="sm"
          type="button"
          variant="outline"
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? "Show less" : "See more"}
        </Button>
      )}
    </div>
  );
}

function AgentRunBadges({
  agent,
  failed,
  runtimeSummary,
  streaming,
}: {
  agent?: AgentConfig;
  failed: boolean;
  runtimeSummary: RuntimeTraceSummary;
  streaming: boolean;
}) {
  const modelBits = [
    agent?.model_label?.trim(),
    agent?.reasoning_label?.trim(),
  ].filter(Boolean);
  return (
    <>
      <Badge
        variant={failed ? "destructive" : streaming ? "outline" : "secondary"}
        className="h-4 gap-1 px-1.5 text-[10px]"
      >
        {streaming && <Loader2Icon className="size-3 animate-spin" />}
        {failed ? "failed" : streaming ? "running" : "completed"}
      </Badge>
      {modelBits.length > 0 && (
        <Badge variant="outline" className="h-4 max-w-44 px-1.5 text-[10px]">
          <span className="truncate">{modelBits.join(" · ")}</span>
        </Badge>
      )}
      {runtimeSummary.reasoning.length > 0 && (
        <Badge
          variant="outline"
          className="h-4 border-amber-200 bg-amber-50 px-1.5 text-[10px] text-amber-800"
        >
          reasoning {runtimeSummary.reasoning.length}
        </Badge>
      )}
      {runtimeSummary.toolCalls.length > 0 && (
        <Badge
          variant="outline"
          className="h-4 border-blue-200 bg-blue-50 px-1.5 text-[10px] text-blue-800"
        >
          tools {runtimeSummary.toolCalls.length}
        </Badge>
      )}
      {runtimeSummary.output.length > 0 && (
        <Badge
          variant="outline"
          className="h-4 border-emerald-200 bg-emerald-50 px-1.5 text-[10px] text-emerald-800"
        >
          output {runtimeSummary.output.length}
        </Badge>
      )}
      {runtimeSummary.errors.length > 0 && (
        <Badge variant="destructive" className="h-4 px-1.5 text-[10px]">
          errors {runtimeSummary.errors.length}
        </Badge>
      )}
    </>
  );
}

function ReasoningSummary({ metadata }: { metadata: unknown }) {
  const reasoning = metadataString(metadata, "reasoning_summary");
  if (!reasoning) {
    return null;
  }
  return (
    <details className="border-border/70 bg-surface/50 mt-3 rounded-lg border px-3 py-2 text-xs">
      <summary className="text-muted-foreground flex cursor-pointer list-none items-center justify-between gap-3 font-medium [&::-webkit-details-marker]:hidden">
        <span>Reasoning summary</span>
        <span>model-visible</span>
      </summary>
      <div className="mt-2 text-[11px] leading-5">
        <p className="text-muted-foreground mb-2">
          Provider-returned reasoning or thinking summary, not private hidden
          chain-of-thought.
        </p>
        <MarkdownMessage content={reasoning} />
      </div>
    </details>
  );
}

function AgentAvatar({
  authorType,
  isUser,
  logo,
}: {
  authorType: string;
  isUser: boolean;
  logo: string;
}) {
  if (isUser) {
    return (
      <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full bg-[#e8e5df] text-[11px] font-semibold text-[#3a3834]">
        <UserIcon className="size-4" />
      </span>
    );
  }
  if (logo) {
    return (
      <span className="bg-surface mt-1 flex size-8 shrink-0 items-center justify-center rounded-lg border">
        <img alt="" className="size-4.5" src={logo} />
      </span>
    );
  }
  if (authorType === "cocode" || authorType === "orchestrator") {
    return (
      <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-[#0f0f0f]">
        <img alt="" className="size-4.5" src={cocodeMarkUrl} />
      </span>
    );
  }
  const Icon =
    authorType === "orchestrator"
      ? SparklesIcon
      : authorType === "system"
        ? ClockIcon
        : BotIcon;
  return (
    <span className="bg-primary text-primary-foreground mt-1 flex size-8 shrink-0 items-center justify-center rounded-lg">
      <Icon className="size-4" />
    </span>
  );
}

function AskTargetDropdown({
  onSelect,
  options,
  selected,
}: {
  onSelect: (id: ChatAudience) => void;
  options: ChatAskTargetOption[];
  selected: ChatAskTargetOption;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          aria-label="Choose centralized chat ask target"
          className="h-9 max-w-[230px] justify-start gap-2 rounded-lg bg-white px-3"
          size="sm"
          type="button"
          variant="outline"
        >
          <AskTargetIcon option={selected} />
          <span className="truncate">Ask: {selected.label}</span>
          <ChevronDownIcon className="size-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-72">
        <DropdownMenuLabel>Ask</DropdownMenuLabel>
        <DropdownMenuGroup>
          {options.map((option) => (
            <DropdownMenuItem
              key={option.id}
              onSelect={() => onSelect(option.id)}
            >
              <AskTargetIcon option={option} />
              <span className="min-w-0 flex-1">
                <span className="block truncate">{option.label}</span>
                <span className="text-muted-foreground block truncate text-xs">
                  {option.description}
                </span>
              </span>
              {selected.id === option.id && <CheckIcon className="size-4" />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ResponderDropdown({
  onSelect,
  options,
  selected,
}: {
  onSelect: (id: string) => void;
  options: ChatResponderOption[];
  selected: ChatResponderOption;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          aria-label="Choose centralized chat responder"
          className="h-9 max-w-[280px] justify-start gap-2 rounded-lg bg-white px-3"
          size="sm"
          type="button"
          variant="outline"
        >
          <ResponderIcon option={selected} />
          <span className="truncate">Responder: {selected.label}</span>
          <ChevronDownIcon className="size-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-72">
        <DropdownMenuLabel>Responder</DropdownMenuLabel>
        <DropdownMenuGroup>
          {options.map((option) => (
            <DropdownMenuItem
              key={option.id}
              onSelect={() => onSelect(option.id)}
            >
              <ResponderIcon option={option} />
              <span className="min-w-0 flex-1">
                <span className="block truncate">{option.label}</span>
                <span className="text-muted-foreground block truncate text-xs">
                  {option.description}
                </span>
              </span>
              {selected.id === option.id && <CheckIcon className="size-4" />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <p className="text-muted-foreground px-1.5 py-1 text-xs">
          Agents run non-interactively. Replies and failures stay in this chat.
        </p>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function AskTargetIcon({ option }: { option: ChatAskTargetOption }) {
  if (option.icon === "all") {
    return <UsersIcon className="size-4 shrink-0" />;
  }
  if (option.icon === "orchestrator") {
    return <SparklesIcon className="size-4 shrink-0" />;
  }
  return <BotIcon className="size-4 shrink-0" />;
}

function ResponderIcon({ option }: { option: ChatResponderOption }) {
  if (option.icon === "orchestrator") {
    return <SparklesIcon className="size-4 shrink-0" />;
  }
  return <BotIcon className="size-4 shrink-0" />;
}

function CentralizedChatRail({
  events,
  findings,
  onOpenFindings,
  session,
  summary,
}: {
  events: ReviewEvent[];
  findings: Loadable<FindingListResponse>;
  onOpenFindings: () => void;
  session: ReviewSession;
  summary: Loadable<ReviewSessionSummary>;
}) {
  const findingItems = findings.status === "success" ? findings.data.items : [];
  const stats = findings.status === "success" ? findings.data.stats : undefined;
  const topFinding =
    findingItems.find((finding) => finding.severity === "critical") ??
    findingItems.find((finding) => finding.severity === "high") ??
    findingItems[0];
  const latestEvents = events.slice(-6).reverse();
  const agentCount =
    summary.status === "success"
      ? summary.data.agent_runs_total || session.agents.length
      : session.agents.length;
  return (
    <aside className="flex min-h-0 flex-col gap-4 overflow-y-auto pr-1 [scrollbar-width:none] max-xl:grid max-xl:grid-cols-3 max-lg:grid-cols-1 [&::-webkit-scrollbar]:hidden">
      <section className="border-border/80 space-y-3 rounded-xl border bg-white p-4 shadow-[0_1px_2px_rgba(17,18,20,0.03)]">
        <h2 className="text-[15px] font-semibold">Review summary</h2>
        <RailStatus
          detail={`${stats?.by_verification.verified ?? 0}`}
          icon="verified"
          label="Verified"
          ok={session.status !== "failed"}
        />
        <RailStatus
          detail={`${stats?.needs_triage ?? 0}`}
          icon="triage"
          label="Needs triage"
          ok={(stats?.needs_triage ?? 0) === 0}
        />
        <RailStatus
          detail={`${stats?.by_decision.accepted ?? 0}`}
          icon="accepted"
          label="Accepted"
          ok
        />
        <RailStatus
          detail={`${stats?.by_decision.dismissed ?? 0}`}
          icon="dismissed"
          label="Dismissed"
          ok
        />
        <div className="text-muted-foreground border-border/70 border-t pt-3 text-xs">
          {agentCount} reviewer{agentCount === 1 ? "" : "s"} configured •{" "}
          {session.status}
        </div>
      </section>

      <section className="border-border/80 space-y-3 rounded-xl border bg-white p-4 shadow-[0_1px_2px_rgba(17,18,20,0.03)]">
        <h2 className="text-[15px] font-semibold">Top finding</h2>
        {topFinding ? (
          <div className="space-y-3">
            <div className="flex items-start gap-2">
              <AlertTriangleIcon className="text-destructive mt-0.5 size-4 shrink-0" />
              <p className="line-clamp-4 text-[13px] leading-5 font-semibold">
                {topFinding.canonical_claim}
              </p>
            </div>
            {topFinding.evidence_summary && (
              <p className="text-muted-foreground line-clamp-4 text-xs leading-5">
                {topFinding.evidence_summary}
              </p>
            )}
            <Button size="sm" variant="outline" onClick={onOpenFindings}>
              View in Findings
            </Button>
          </div>
        ) : (
          <p className="text-muted-foreground text-[13px] leading-5">
            Findings will appear here as reviewers report structured evidence.
          </p>
        )}
      </section>

      <section className="border-border/80 min-h-0 space-y-3 rounded-xl border bg-white p-4 shadow-[0_1px_2px_rgba(17,18,20,0.03)]">
        <h2 className="text-[15px] font-semibold">Activity</h2>
        {latestEvents.length > 0 ? (
          <div className="max-h-60 space-y-3 overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
            {latestEvents.map((event) => (
              <div
                className="grid grid-cols-[18px_minmax(0,1fr)_auto] items-start gap-2 text-[12px]"
                key={event.id}
              >
                <ClockIcon className="text-muted-foreground mt-0.5 size-3.5" />
                <span className="truncate font-medium">
                  {formatEventLabel(event.type)}
                </span>
                <span className="text-muted-foreground whitespace-nowrap">
                  {formatRelativeTime(event.created_at)}
                </span>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-muted-foreground text-[13px] leading-5">
            Activity will stream here after the review starts.
          </p>
        )}
      </section>
    </aside>
  );
}

function RailStatus({
  detail,
  icon,
  label,
  ok,
}: {
  detail: string;
  icon: "verified" | "triage" | "accepted" | "dismissed";
  label: string;
  ok: boolean;
}) {
  const Icon =
    icon === "triage"
      ? AlertTriangleIcon
      : icon === "dismissed"
        ? CircleSlashIcon
        : icon === "verified"
          ? ShieldCheckIcon
          : CheckIcon;
  return (
    <div className="grid grid-cols-[20px_minmax(0,1fr)_auto] items-center gap-3">
      <Icon
        className={cn(
          "size-4",
          icon === "triage"
            ? "text-amber-500"
            : icon === "dismissed"
              ? "text-muted-foreground"
              : ok
                ? "text-success"
                : "text-muted-foreground",
        )}
      />
      <span className="truncate text-[13px] font-medium">{label}</span>
      <span className="font-mono text-[13px]">{detail}</span>
    </div>
  );
}

function formatEventLabel(type: string) {
  return type
    .replace(/([a-z])([A-Z])/g, "$1 $2")
    .replace(/^Review Session/, "Review")
    .replace(/^Workflow Phase/, "Phase")
    .replace(/^Agent Run/, "Agent");
}

function displayNameForAuthor(authorType: string) {
  switch (authorType) {
    case "user":
      return "You";
    case "orchestrator":
      return "Orchestrator";
    case "system":
      return "System";
    case "cocode":
      return "cocode";
    default:
      return "Reviewer";
  }
}

function formatClockTime(value: string) {
  const time = Date.parse(value);
  if (!Number.isFinite(time)) {
    return "";
  }
  return new Date(time).toLocaleTimeString([], {
    hour: "numeric",
    minute: "2-digit",
  });
}

function compactAgentLabel(agent: AgentConfig) {
  return formatSetupAgentLabel(agent)
    .replace(/\bCLI\b/g, "")
    .replace(/\s+/g, " ")
    .trim();
}

function compactRoleLabel(value: string) {
  return value
    .replace(/[_-]+/g, " ")
    .replace(/\s+/g, " ")
    .trim()
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function isOrchestratorEntry({
  assignment,
}: {
  agent: AgentConfig;
  assignment: ReviewSessionAgent;
}) {
  return isOrchestratorAssignment(assignment);
}

function isOrchestratorAssignment(assignment: ReviewSessionAgent) {
  return assignment.role.toLowerCase().includes("orchestrator");
}

function agentByID(agents: AgentConfig[], id?: string) {
  if (!id) {
    return undefined;
  }
  return agents.find((agent) => agent.id === id);
}

function formatRelativeTime(value: string) {
  const time = Date.parse(value);
  if (!Number.isFinite(time)) {
    return value;
  }
  const seconds = Math.max(0, Math.round((Date.now() - time) / 1000));
  if (seconds < 60) {
    return "Just now";
  }
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) {
    return `${minutes}m ago`;
  }
  const hours = Math.round(minutes / 60);
  if (hours < 24) {
    return `${hours}h ago`;
  }
  return new Date(time).toLocaleDateString();
}
