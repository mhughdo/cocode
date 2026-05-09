import { type FormEvent, useEffect, useMemo, useState } from "react";
import {
  BotIcon,
  CheckIcon,
  ChevronDownIcon,
  ClockIcon,
  Loader2Icon,
  MessageSquareIcon,
  SendIcon,
  SparklesIcon,
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
  type ApiClient,
  type ChatMessage,
  type ChatThreadView,
  type Finding,
  type FindingListResponse,
  type Loadable,
  type ReviewEvent,
  type ReviewSession,
  type ReviewSessionSummary,
  errorApiState,
  loadApiResource,
  loadingApiState,
  successApiState,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import { agentLogoUrl, formatSetupAgentLabel } from "./agent-utils";

type ChatAudience = "orchestrator" | "all_agents" | "selected_agent";

type ChatResponderOption = {
  id: string;
  label: string;
  description: string;
  audience: ChatAudience;
  agentConfigId?: string;
  icon: "orchestrator" | "all" | "agent";
};

export function CentralizedChatScreen({
  agentConfigs,
  client,
  events,
  findings,
  session,
  summary,
}: {
  agentConfigs: Loadable<AgentConfig[]>;
  client: ApiClient | null;
  events: ReviewEvent[];
  findings: Loadable<FindingListResponse>;
  session: ReviewSession;
  summary: Loadable<ReviewSessionSummary>;
}) {
  const [thread, setThread] =
    useState<Loadable<ChatThreadView>>(loadingApiState());
  const [message, setMessage] = useState("");
  const [responderID, setResponderID] = useState("orchestrator");
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const agents = useMemo(() => {
    if (agentConfigs.status !== "success") {
      return [];
    }
    const byID = new Map(agentConfigs.data.map((agent) => [agent.id, agent]));
    return session.agents
      .filter((assignment) => assignment.enabled)
      .map((assignment) => byID.get(assignment.agent_config_id))
      .filter((agent): agent is AgentConfig => Boolean(agent));
  }, [agentConfigs, session.agents]);
  const findingItems = findings.status === "success" ? findings.data.items : [];

  const responderOptions = useMemo<ChatResponderOption[]>(
    () => [
      {
        id: "orchestrator",
        label: "Orchestrator",
        description: "Ask cocode to synthesize current review state.",
        audience: "orchestrator",
        icon: "orchestrator",
      },
      {
        id: "all_agents",
        label: "All review agents",
        description: `Ask ${agents.length || "all"} configured reviewer${
          agents.length === 1 ? "" : "s"
        }.`,
        audience: "all_agents",
        icon: "all",
      },
      ...agents.map((agent) => ({
        id: `agent:${agent.id}`,
        label: compactAgentLabel(agent),
        description: agent.role || "Reviewer",
        audience: "selected_agent" as const,
        agentConfigId: agent.id,
        icon: "agent" as const,
      })),
    ],
    [agents],
  );
  const selectedResponder =
    responderOptions.find((option) => option.id === responderID) ??
    responderOptions[0];

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
    const next = await loadApiResource(() =>
      client.createReviewSessionChatTurn(session.id, {
        body,
        mode: "follow_up",
        audience: selectedResponder.audience,
        responder_agent_config_id: selectedResponder.agentConfigId,
        include_evidence: true,
        include_recent_messages: true,
      }),
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
    setSubmitting(false);
  }

  return (
    <div className="grid min-h-[620px] grid-cols-[minmax(0,1fr)_300px] gap-5 max-xl:grid-cols-1">
      <div className="flex min-w-0 flex-col gap-4">
        <div className="border-border/75 bg-surface-raised flex min-h-[500px] flex-col rounded-xl border shadow-[0_1px_2px_rgba(17,18,20,0.04)]">
          <div className="border-border/70 flex items-center justify-between border-b px-4 py-3">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-sm font-semibold">
                <MessageSquareIcon className="size-4" />
                Review chat
              </div>
              <p className="text-muted-foreground mt-0.5 truncate text-xs">
                Ask cocode, one reviewer, or every configured reviewer from one
                thread.
              </p>
            </div>
            <Badge variant="outline" className="hidden sm:inline-flex">
              {agents.length} reviewer{agents.length === 1 ? "" : "s"}
            </Badge>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
            {thread.status === "loading" && <LoadingRows rows={5} />}
            {thread.status === "error" && (
              <EmptyState
                title="Central chat is unavailable"
                description={thread.error.message}
                icon={MessageSquareIcon}
              />
            )}
            {thread.status === "success" &&
              thread.data.messages.length === 0 && (
                <EmptyState
                  title="No chat messages yet"
                  description="Start with a question for the orchestrator or a reviewer."
                  icon={MessageSquareIcon}
                />
              )}
            {thread.status === "success" && thread.data.messages.length > 0 && (
              <div className="flex flex-col gap-3">
                {thread.data.messages.map((item) => (
                  <ChatMessageCard
                    agent={agentByID(agents, item.agent_config_id)}
                    key={item.id}
                    message={item}
                  />
                ))}
                {submitting && (
                  <div className="text-muted-foreground flex items-center gap-2 px-2 py-1 text-xs">
                    <Loader2Icon className="size-3.5 animate-spin" />
                    Waiting for {selectedResponder.label}
                  </div>
                )}
              </div>
            )}
          </div>

          <form
            className="border-border/75 bg-background/80 border-t p-3 backdrop-blur"
            onSubmit={submitMessage}
          >
            <div className="border-border/80 bg-surface-raised focus-within:border-ring/60 rounded-xl border shadow-[0_1px_8px_rgba(17,18,20,0.04)]">
              <Textarea
                aria-label="Centralized review message"
                className="max-h-36 min-h-18 resize-none border-0 bg-transparent px-3 py-3 text-[13px] shadow-none focus-visible:ring-0"
                disabled={!client || submitting}
                onChange={(event) => setMessage(event.target.value)}
                onKeyDown={(event) => {
                  if (
                    (event.metaKey || event.ctrlKey) &&
                    event.key === "Enter"
                  ) {
                    event.currentTarget.form?.requestSubmit();
                  }
                }}
                placeholder="Ask for a follow-up, challenge a finding, or request another pass..."
                value={message}
              />
              <div className="flex flex-wrap items-center justify-between gap-2 px-2.5 pb-2">
                <ResponderDropdown
                  options={responderOptions}
                  selected={selectedResponder}
                  onSelect={setResponderID}
                />
                <Button
                  aria-label="Send centralized chat message"
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
      </div>

      <CentralizedChatRail
        events={events}
        findings={findingItems}
        session={session}
        summary={summary}
      />
    </div>
  );
}

function ChatMessageCard({
  agent,
  message,
}: {
  agent?: AgentConfig;
  message: ChatMessage;
}) {
  const isUser = message.author_type === "user";
  const isSystem = message.author_type === "system";
  const failed = message.status === "failed";
  const logo = agent ? agentLogoUrl(agent) : "";
  return (
    <article
      className={cn(
        "flex gap-3",
        isUser && "justify-end",
        isSystem && "justify-center",
      )}
    >
      {!isUser && !isSystem && (
        <AgentAvatar authorType={message.author_type} logo={logo} />
      )}
      <div
        className={cn(
          "min-w-0 rounded-xl px-3 py-2.5 text-sm leading-6",
          isUser
            ? "bg-primary text-primary-foreground max-w-[78%]"
            : "border-border/75 text-foreground max-w-[84%] border bg-white",
          isSystem &&
            "bg-surface text-muted-foreground max-w-[92%] border-0 text-xs",
          failed && "border-destructive/30 bg-destructive/5",
        )}
      >
        {!isUser && !isSystem && (
          <div className="mb-1.5 flex items-center gap-2">
            <span className="font-medium">{message.author_display_name}</span>
            {message.agent_run_id && (
              <Badge variant="secondary" className="h-4 px-1.5 text-[10px]">
                agent run
              </Badge>
            )}
            {failed && (
              <Badge variant="destructive" className="h-4 px-1.5 text-[10px]">
                failed
              </Badge>
            )}
          </div>
        )}
        <p className="break-words whitespace-pre-wrap">{message.body}</p>
      </div>
    </article>
  );
}

function AgentAvatar({
  authorType,
  logo,
}: {
  authorType: string;
  logo: string;
}) {
  if (logo) {
    return (
      <span className="bg-surface mt-1 flex size-8 shrink-0 items-center justify-center rounded-lg border">
        <img alt="" className="size-4.5" src={logo} />
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
          className="max-w-full justify-start"
          size="sm"
          type="button"
          variant="ghost"
        >
          <ResponderIcon option={selected} />
          <span className="truncate">Ask: {selected.label}</span>
          <ChevronDownIcon className="size-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-72">
        <DropdownMenuLabel>Route question to</DropdownMenuLabel>
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

function ResponderIcon({ option }: { option: ChatResponderOption }) {
  if (option.icon === "all") {
    return <UsersIcon className="size-4 shrink-0" />;
  }
  if (option.icon === "orchestrator") {
    return <SparklesIcon className="size-4 shrink-0" />;
  }
  return <BotIcon className="size-4 shrink-0" />;
}

function CentralizedChatRail({
  events,
  findings,
  session,
  summary,
}: {
  events: ReviewEvent[];
  findings: Finding[];
  session: ReviewSession;
  summary: Loadable<ReviewSessionSummary>;
}) {
  const topFinding =
    findings.find((finding) => finding.severity === "critical") ??
    findings.find((finding) => finding.severity === "high") ??
    findings[0];
  const latestEvents = events.slice(-6).reverse();
  const agentCount =
    summary.status === "success"
      ? summary.data.agent_runs_total || session.agents.length
      : session.agents.length;
  return (
    <aside className="border-border/75 bg-surface-raised flex min-h-0 flex-col gap-5 rounded-xl border p-4 shadow-[0_1px_2px_rgba(17,18,20,0.04)] max-xl:grid max-xl:grid-cols-3 max-lg:grid-cols-1">
      <section className="space-y-3">
        <h2 className="text-sm font-semibold">Review summary</h2>
        <RailStatus
          detail={session.status}
          label="Review thread active"
          ok={session.status !== "failed"}
        />
        <RailStatus
          detail={`${agentCount} configured`}
          label="Review agents"
          ok={agentCount > 0}
        />
        <RailStatus
          detail={`${findings.length} finding${findings.length === 1 ? "" : "s"}`}
          label="Findings available"
          ok={findings.length > 0}
        />
      </section>

      <section className="space-y-3">
        <h2 className="text-sm font-semibold">Top finding</h2>
        {topFinding ? (
          <div className="border-border/75 rounded-lg border bg-white p-3">
            <div className="mb-2 flex items-center gap-2">
              <Badge variant="secondary">{topFinding.severity}</Badge>
              <span className="text-muted-foreground text-xs">
                {topFinding.verification_status}
              </span>
            </div>
            <p className="line-clamp-4 text-sm leading-5 font-medium">
              {topFinding.canonical_claim}
            </p>
            {topFinding.primary_path && (
              <p className="text-muted-foreground mt-2 truncate font-mono text-xs">
                {topFinding.primary_path}
              </p>
            )}
          </div>
        ) : (
          <div className="text-muted-foreground border-border/75 rounded-lg border bg-white p-3 text-sm">
            Findings will appear here as reviewers report structured evidence.
          </div>
        )}
      </section>

      <section className="min-h-0 space-y-3">
        <h2 className="text-sm font-semibold">Activity</h2>
        {latestEvents.length > 0 ? (
          <div className="max-h-60 space-y-2 overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
            {latestEvents.map((event) => (
              <div
                className="border-border/60 rounded-lg border bg-white px-3 py-2"
                key={event.id}
              >
                <div className="flex items-center gap-2 text-sm font-medium">
                  <ClockIcon className="text-muted-foreground size-3.5" />
                  <span className="truncate">{event.type}</span>
                </div>
                <p className="text-muted-foreground mt-1 text-xs">
                  {formatRelativeTime(event.created_at)}
                </p>
              </div>
            ))}
          </div>
        ) : (
          <div className="text-muted-foreground border-border/75 rounded-lg border bg-white p-3 text-sm">
            Activity will stream here after the review starts.
          </div>
        )}
      </section>
    </aside>
  );
}

function RailStatus({
  detail,
  label,
  ok,
}: {
  detail: string;
  label: string;
  ok: boolean;
}) {
  return (
    <div className="flex items-start gap-2">
      <span
        className={cn(
          "mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full",
          ok ? "bg-success/10 text-success" : "bg-muted text-muted-foreground",
        )}
      >
        {ok ? (
          <CheckIcon className="size-3.5" />
        ) : (
          <ClockIcon className="size-3.5" />
        )}
      </span>
      <span className="min-w-0">
        <span className="block text-sm font-medium">{label}</span>
        <span className="text-muted-foreground block truncate text-xs">
          {detail}
        </span>
      </span>
    </div>
  );
}

function compactAgentLabel(agent: AgentConfig) {
  return formatSetupAgentLabel(agent)
    .replace(/\bCLI\b/g, "")
    .replace(/\s+/g, " ")
    .trim();
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
