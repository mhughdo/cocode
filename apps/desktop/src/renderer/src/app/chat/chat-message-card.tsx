import { useEffect, useMemo, useState } from "react";
import {
  BotIcon,
  CheckIcon,
  ChevronDownIcon,
  ClockIcon,
  CopyIcon,
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
import { extractDisplayableAgentOutput } from "../shared/agent-output-formatting";

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
  const displayBody = useMemo(
    () =>
      isUser || !message.agent_run_id
        ? message.body
        : displayableAgentRunMessageBody({
            authorDisplayName: message.author_display_name,
            authorType: message.author_type,
            body: message.body,
            status: message.status,
          }),
    [
      isUser,
      message.agent_run_id,
      message.author_display_name,
      message.author_type,
      message.body,
      message.status,
    ],
  );
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">(
    "idle",
  );
  const canCopy = displayBody.trim().length > 0;

  useEffect(() => {
    if (copyState !== "copied" && copyState !== "failed") {
      return;
    }
    const timeout = window.setTimeout(() => setCopyState("idle"), 1400);
    return () => window.clearTimeout(timeout);
  }, [copyState]);

  async function copyMessage() {
    const text = displayBody.trim();
    if (!text) {
      return;
    }
    try {
      if (window.cocode?.writeClipboard) {
        await window.cocode.writeClipboard(text);
      } else if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text);
      } else {
        throw new Error("Clipboard API is unavailable");
      }
      setCopyState("copied");
    } catch {
      setCopyState("failed");
    }
  }

  return (
    <article
      className={cn(
        "bg-card border-border-subtle flex gap-3 rounded-xl border px-4 py-3",
        isSystem && "border-transparent bg-transparent",
        streaming && "bg-surface",
        failed && "border-destructive/30 bg-destructive/5",
      )}
    >
      <AgentAvatar
        authorType={message.author_type}
        isUser={isUser}
        logo={logo}
      />
      <div className="min-w-0 flex-1">
        <div className="mb-1 flex min-w-0 items-start gap-2">
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2 text-[13px]">
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
          {canCopy && (
            <Button
              aria-label={`Copy message from ${
                message.author_display_name ||
                displayNameForAuthor(message.author_type)
              }`}
              className="text-muted-foreground hover:text-foreground -mt-1 size-7 shrink-0 rounded-md"
              size="icon"
              title={copyState === "failed" ? "Copy failed" : "Copy message"}
              type="button"
              variant="ghost"
              onClick={() => void copyMessage()}
            >
              {copyState === "copied" ? (
                <CheckIcon className="size-3.5" />
              ) : (
                <CopyIcon className="size-3.5" />
              )}
            </Button>
          )}
        </div>
        <ExpandableMarkdownMessage
          key={`${message.id}:${streaming ? "streaming" : "final"}`}
          collapsible={!isUser}
          content={displayBody}
          defaultExpanded={!streaming}
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

function displayableAgentRunMessageBody({
  authorDisplayName,
  authorType,
  body,
  status,
}: {
  authorDisplayName?: string;
  authorType: ChatMessage["author_type"];
  body: string;
  status: ChatMessage["status"];
}) {
  const raw = body.trim();
  if (!raw) {
    return body;
  }
  const displayable = extractDisplayableAgentOutput(raw).trim();
  if (displayable) {
    return displayable;
  }

  const label = authorDisplayName || displayNameForAuthor(authorType);
  if (status === "failed") {
    return `${label} failed before returning displayable text. Open the trace to inspect raw events and diagnostics.`;
  }
  if (status === "completed") {
    return `${label} completed without displayable text. Open the trace to inspect raw events and diagnostics.`;
  }
  return `${label} is streaming output back to cocode. Open the trace to inspect live events and diagnostics.`;
}

function ExpandableMarkdownMessage({
  collapsible,
  content,
  defaultExpanded,
  muted,
}: {
  collapsible: boolean;
  content: string;
  defaultExpanded: boolean;
  muted?: boolean;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const normalizedContent = content
    .replace(/\n{0,2}\.\.\.\[truncated\]\s*$/i, "")
    .trim();
  const lineCount = normalizedContent.split("\n").length;
  const needsExpansion = normalizedContent.length > 900 || lineCount > 14;
  const collapsed = collapsible && needsExpansion && !expanded;

  return (
    <div className="min-w-0">
      <div
        className={cn(
          "relative min-w-0",
          collapsed && "max-h-56 overflow-hidden",
        )}
      >
        <MarkdownMessage content={normalizedContent || content} muted={muted} />
        {collapsed && (
          <div className="pointer-events-none absolute inset-x-0 bottom-0 h-16 bg-gradient-to-t from-white to-transparent" />
        )}
      </div>
      {collapsible && needsExpansion && (
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
          variant="signal-agent"
          className="h-4 px-1.5 text-[10px]"
        >
          orchestrator
        </Badge>
      )}
      <Badge
        variant={
          failed ? "destructive" : streaming ? "outline" : "status-verified"
        }
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
        <Badge variant="signal-trace" className="h-4 px-1.5 text-[10px]">
          reasoning {runtimeSummary.reasoning.length}
        </Badge>
      )}
      {runtimeSummary.toolCalls.length > 0 && (
        <Badge variant="signal-tool" className="h-4 px-1.5 text-[10px]">
          tools {runtimeSummary.toolCalls.length}
        </Badge>
      )}
      {runtimeSummary.output.length > 0 && (
        <Badge variant="signal-output" className="h-4 px-1.5 text-[10px]">
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
    <details className="border-border-subtle bg-surface mt-3 rounded-lg border px-3 py-2 text-xs">
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
          className="h-9 max-w-[280px] justify-start gap-2 rounded-full bg-white px-3"
          size="sm"
          type="button"
          variant="outline"
        >
          <ResponderIcon option={selected} />
          <span className="truncate">Agent: {selected.label}</span>
          <ChevronDownIcon className="size-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-72">
        <DropdownMenuLabel>Agent</DropdownMenuLabel>
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
  if (option.logoUrl) {
    return (
      <img
        alt=""
        className="size-4 shrink-0 rounded-[3px]"
        src={option.logoUrl}
      />
    );
  }
  if (option.icon === "orchestrator") {
    return <SparklesIcon className="size-4 shrink-0" />;
  }
  return <BotIcon className="size-4 shrink-0" />;
}
