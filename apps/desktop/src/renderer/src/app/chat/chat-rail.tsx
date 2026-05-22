import {
  AlertTriangleIcon,
  CheckIcon,
  CircleSlashIcon,
  ClockIcon,
  ShieldCheckIcon,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import type {
  FindingListResponse,
  Loadable,
  ReviewEvent,
  ReviewSession,
  ReviewSessionSummary,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import { formatEventLabel, formatRelativeTime } from "./chat-message-utils";

export function CentralizedChatRail({
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
