import { useMemo, useState } from "react";
import {
  BotIcon,
  CheckIcon,
  ChevronDownIcon,
  ClockIcon,
  Loader2Icon,
  SparklesIcon,
  UserIcon,
  UsersIcon,
} from "lucide-react";

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
import type { AgentConfig, ChatMessage, ReviewEvent } from "@/lib/api";
import { cn } from "@/lib/utils";
import cocodeMarkUrl from "../../../../../../../assets/app-icon/cocode-logo-mark.svg";
import { agentLogoUrl } from "../agents/agent-utils";
import {
  AgentRuntimeTrace,
  summarizeRuntimeTraceEvents,
  type RuntimeTraceSummary,
} from "./agent-runtime-trace";
import {
  displayNameForAuthor,
  formatClockTime,
  metadataString,
} from "./chat-message-utils";
import type {
  ChatAskTargetOption,
  ChatAudience,
  ChatResponderOption,
} from "./chat-types";
import { MarkdownMessage } from "../shared/markdown-message";

export function ChatMessageCard({
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
              authorType={message.author_type}
              failed={failed}
              modelLabel={metadataString(message.metadata, "model_label")}
              reasoningLabel={metadataString(
                message.metadata,
                "reasoning_label",
              )}
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
  authorType,
  failed,
  modelLabel,
  reasoningLabel,
  runtimeSummary,
  streaming,
}: {
  agent?: AgentConfig;
  authorType: ChatMessage["author_type"];
  failed: boolean;
  modelLabel: string;
  reasoningLabel: string;
  runtimeSummary: RuntimeTraceSummary;
  streaming: boolean;
}) {
  const modelBits = [
    modelLabel.trim() || agent?.model_label?.trim(),
    reasoningLabel.trim() || agent?.reasoning_label?.trim(),
  ].filter(Boolean);
  const isOrchestrator =
    authorType === "orchestrator" ||
    Boolean(agent?.role.toLowerCase().includes("orchestrator"));
  return (
    <>
      {isOrchestrator && (
        <Badge
          data-testid="orchestrator-agent-badge"
          variant="outline"
          className="h-4 border-violet-200 bg-violet-50 px-1.5 text-[10px] text-violet-800"
        >
          orchestrator
        </Badge>
      )}
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

export function AskTargetDropdown({
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

export function ResponderDropdown({
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
