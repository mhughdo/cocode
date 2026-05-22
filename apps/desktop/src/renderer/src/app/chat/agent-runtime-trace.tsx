import { useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import type { ReviewEvent } from "@/lib/api";
import { cn } from "@/lib/utils";

import { summarizeRuntimeTraceEvents } from "./agent-runtime-trace-summary";
import { MarkdownMessage } from "../shared/markdown-message";

export {
  summarizeRuntimeTraceEvents,
  type RuntimeTraceSummary,
} from "./agent-runtime-trace-summary";

type TraceTone = "amber" | "blue" | "green" | "red" | "neutral";

export function AgentRuntimeTrace({
  className,
  compact,
  events,
  failed,
  loading,
}: {
  className?: string;
  compact?: boolean;
  events: ReviewEvent[];
  failed?: boolean;
  loading?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const summary = useMemo(() => summarizeRuntimeTraceEvents(events), [events]);
  if (summary.eventCount === 0 && !loading) {
    return null;
  }
  return (
    <details
      className={cn(
        "border-border/70 bg-surface/50 rounded-lg border px-3 py-2 text-xs",
        compact ? "mt-3" : "mx-4 mb-4",
        failed && "border-destructive/20 bg-destructive/5",
        className,
      )}
      open={open}
      onToggle={(event) => {
        setOpen(event.currentTarget.open);
      }}
    >
      <summary className="text-muted-foreground flex cursor-pointer list-none items-center justify-between gap-3 font-medium [&::-webkit-details-marker]:hidden">
        <span>Reasoning and tool trace</span>
        <span className="font-mono">
          {loading && summary.eventCount === 0
            ? "waiting"
            : `${summary.eventCount} events`}
        </span>
      </summary>
      {open && (
        <div className="mt-3 space-y-3">
          <p className="text-muted-foreground text-[11px] leading-5">
            Shows provider-visible thinking summaries, model text, tool calls,
            and CLI diagnostics. Private hidden chain-of-thought is not exposed.
          </p>
          {summary.eventCount === 0 ? (
            <p className="text-muted-foreground text-[11px]">
              Waiting for the selected reviewer to emit output.
            </p>
          ) : (
            <>
              {summary.errors.length > 0 && (
                <RuntimeTraceSection
                  items={summary.errors}
                  title="Errors"
                  tone="red"
                />
              )}
              {summary.reasoning.length > 0 && (
                <RuntimeTraceSection
                  items={summary.reasoning}
                  markdown
                  title="Visible reasoning"
                  tone="amber"
                />
              )}
              {summary.toolCalls.length > 0 && (
                <RuntimeTraceSection
                  items={summary.toolCalls}
                  title="Tool calls"
                  tone="blue"
                />
              )}
              {summary.output.length > 0 && (
                <RuntimeTraceSection
                  items={summary.output}
                  markdown
                  title="Model output"
                  tone="green"
                />
              )}
              {summary.diagnostics.length > 0 && (
                <RuntimeTraceSection
                  items={summary.diagnostics}
                  title="CLI diagnostics"
                  tone="neutral"
                />
              )}
              {summary.lifecycle.length > 0 && (
                <RuntimeTraceSection
                  items={summary.lifecycle}
                  title="Run state"
                  tone="neutral"
                  compact={compact}
                />
              )}
            </>
          )}
        </div>
      )}
    </details>
  );
}

function RuntimeTraceSection({
  compact,
  items,
  markdown,
  title,
  tone,
}: {
  compact?: boolean;
  items: string[];
  markdown?: boolean;
  title: string;
  tone: TraceTone;
}) {
  return (
    <section
      className={cn(
        "rounded-md border px-2 py-1.5",
        tone === "amber" && "border-amber-200 bg-amber-50/60",
        tone === "blue" && "border-blue-200 bg-blue-50/60",
        tone === "green" && "border-emerald-200 bg-emerald-50/60",
        tone === "red" && "border-destructive/20 bg-destructive/5",
        tone === "neutral" && "border-border/70 bg-background/70",
      )}
    >
      <div className="text-muted-foreground mb-1 flex items-center justify-between gap-2 text-[10px] font-semibold tracking-wide uppercase">
        <span>{title}</span>
        <Badge variant="outline" className="h-4 px-1.5 text-[10px]">
          {items.length}
        </Badge>
      </div>
      <div
        className={cn(
          "space-y-1 overflow-auto [scrollbar-width:thin]",
          compact ? "max-h-48" : "max-h-72",
        )}
      >
        {items.map((item, index) => (
          <RuntimeTraceItem
            compact={compact}
            index={index}
            item={item}
            key={`${title}-${index}-${item.slice(0, 24)}`}
            markdown={markdown}
          />
        ))}
      </div>
    </section>
  );
}

function RuntimeTraceItem({
  compact,
  index,
  item,
  markdown,
}: {
  compact?: boolean;
  index: number;
  item: string;
  markdown?: boolean;
}) {
  const lines = item.split(/\r?\n/).filter(Boolean);
  const summary = traceItemSummary(item, index);
  const isLong = item.length > 220 || lines.length > 3;
  if (markdown) {
    if (!isLong) {
      return (
        <div className="bg-background/55 rounded-sm px-2 py-1.5">
          <MarkdownMessage className="text-[11px] leading-5" content={item} />
        </div>
      );
    }
    return (
      <details
        className="bg-background/55 rounded-sm px-2 py-1.5"
        open={!compact && index === 0}
      >
        <summary className="text-foreground/85 flex cursor-pointer list-none items-center justify-between gap-3 text-[11px] leading-4 [&::-webkit-details-marker]:hidden">
          <span className="min-w-0 truncate">{summary}</span>
          <span className="text-muted-foreground shrink-0">
            {lines.length} lines
          </span>
        </summary>
        <div className="mt-2 max-h-64 overflow-auto border-t pt-2 [scrollbar-width:thin]">
          <MarkdownMessage className="text-[11px] leading-5" content={item} />
        </div>
      </details>
    );
  }
  if (!isLong) {
    return (
      <pre className="bg-background/55 rounded-sm px-2 py-1.5 font-mono text-[11px] leading-4 break-words whitespace-pre-wrap">
        {item}
      </pre>
    );
  }
  return (
    <details
      className="bg-background/55 rounded-sm px-2 py-1.5"
      open={!compact && index === 0}
    >
      <summary className="text-foreground/85 flex cursor-pointer list-none items-center justify-between gap-3 font-mono text-[11px] leading-4 [&::-webkit-details-marker]:hidden">
        <span className="min-w-0 truncate">{summary}</span>
        <span className="text-muted-foreground shrink-0">
          {lines.length} lines
        </span>
      </summary>
      <pre className="mt-2 max-h-48 overflow-auto border-t pt-2 font-mono text-[11px] leading-4 break-words whitespace-pre-wrap [scrollbar-width:thin]">
        {item}
      </pre>
    </details>
  );
}

function traceItemSummary(item: string, index: number) {
  const firstLine =
    item
      .split(/\r?\n/)
      .map((line) => line.trim())
      .find(Boolean) ?? `event ${index + 1}`;
  return firstLine.length > 160 ? `${firstLine.slice(0, 157)}...` : firstLine;
}
