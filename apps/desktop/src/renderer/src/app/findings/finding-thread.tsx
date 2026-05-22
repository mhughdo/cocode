import { MessageSquareIcon } from "lucide-react";

import { EmptyState } from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import type { FindingThreadView, ReviewEvent } from "@/lib/api";
import { cn } from "@/lib/utils";
import { MarkdownMessage } from "../chat/markdown-message";
import { formatRelativeAge } from "../shared/time-format";

export function FollowUpMessages({
  messages,
}: {
  messages: FindingThreadView["messages"];
}) {
  const visibleMessages = messages.filter(
    (message) => message.role !== "system",
  );
  if (visibleMessages.length === 0) {
    return (
      <EmptyState
        title="No follow-ups yet"
        description="Ask a scoped question or use a quick action to start the thread."
        icon={MessageSquareIcon}
      />
    );
  }
  return (
    <div className="flex flex-col gap-3 p-4">
      {visibleMessages.map((message) => (
        <div
          key={message.id}
          className={cn(
            "rounded-lg border p-3",
            message.role === "user" && "bg-surface",
            message.role === "assistant" && "bg-background",
            message.role === "system" && "bg-muted/40",
          )}
        >
          <div className="mb-2 flex items-center justify-between gap-2">
            <Badge
              variant={message.role === "assistant" ? "secondary" : "outline"}
            >
              {message.role}
            </Badge>
            <span className="text-muted-foreground text-xs">
              {formatRelativeAge(message.created_at)}
            </span>
          </div>
          <MarkdownMessage content={message.content} />
        </div>
      ))}
    </div>
  );
}

export function followUpRuntimeEvents(
  events: ReviewEvent[],
  findingId: string,
) {
  return events.filter((event) => {
    if (!event.type.startsWith("AgentRun")) {
      return false;
    }
    return payloadString(event.payload.finding_id) === findingId;
  });
}

function payloadString(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}
