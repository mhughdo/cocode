import { useEffect, useMemo, useState } from "react";
import {
  BellIcon,
  ClockIcon,
  FileSearchIcon,
  ShieldCheckIcon,
  TerminalIcon,
} from "lucide-react";

import { EmptyState, ErrorState, LoadingRows } from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  type AgentConfig,
  type ApiClient,
  type ContextBundlePreview,
  errorApiState,
  idleApiState,
  type Loadable,
  loadApiResource,
  loadingApiState,
  preserveSuccessfulLoadable,
  type RedactionReportItem,
  type ReviewAuditLogEntry,
  type ReviewAuditLogResponse,
  type ReviewContextPolicy,
  type ReviewEvent,
  type ReviewSession,
} from "@/lib/api";
import { agentEgress, agentProvider } from "../agents/agent-utils";
import { formatRelativeAge } from "../shared/time-format";

const MAX_AUDIT_ENTRIES_RENDERED = 120;

export function ReviewDetailsScreen({
  agentConfigs,
  client,
  events,
  session,
  streamState,
}: {
  agentConfigs: Loadable<AgentConfig[]>;
  client: ApiClient | null;
  events: ReviewEvent[];
  session?: ReviewSession;
  streamState: Loadable<true>;
}) {
  if (!session) {
    return (
      <EmptyState
        title="No review selected"
        description="Select a review thread to inspect context safety and audit activity."
        icon={FileSearchIcon}
      />
    );
  }

  return (
    <div className="flex min-h-0 flex-col gap-4">
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(340px,0.72fr)]">
        <ReviewContextSafetyPanel
          agentConfigs={agentConfigs}
          client={client}
          session={session}
        />
        <ReviewAuditLogPanel client={client} session={session} />
      </div>
      <ReviewEventTimeline events={events} streamState={streamState} />
    </div>
  );
}

function ReviewContextSafetyPanel({
  agentConfigs,
  client,
  session,
}: {
  agentConfigs: Loadable<AgentConfig[]>;
  client: ApiClient | null;
  session: ReviewSession;
}) {
  const reviewAgents = useMemo(() => {
    if (agentConfigs.status !== "success") {
      return [];
    }
    const sessionAgentIds = new Set(
      session.agents.map((agent) => agent.agent_config_id),
    );
    return agentConfigs.data.filter((agent) => sessionAgentIds.has(agent.id));
  }, [agentConfigs, session.agents]);
  const [selectedAgentId, setSelectedAgentId] = useState("");
  const [preview, setPreview] =
    useState<Loadable<ContextBundlePreview>>(idleApiState());

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      setPreview(idleApiState());
      setSelectedAgentId((current) => {
        if (reviewAgents.some((agent) => agent.id === current)) {
          return current;
        }
        return reviewAgents[0]?.id ?? "";
      });
    });
    return () => {
      canceled = true;
    };
  }, [reviewAgents, session.id]);

  async function runPreview() {
    if (!client || !selectedAgentId) {
      setPreview(
        errorApiState(
          new Error("Choose a configured review agent before previewing."),
        ),
      );
      return;
    }
    setPreview(loadingApiState());
    const state = await loadApiResource(() =>
      client.previewReviewContext(session.id, {
        agent_config_id: selectedAgentId,
        persist: false,
        context_policy: session.context_policy as ReviewContextPolicy,
      }),
    );
    setPreview(state);
  }

  const selectedAgent = reviewAgents.find(
    (agent) => agent.id === selectedAgentId,
  );

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium">
            <ShieldCheckIcon className="size-4" />
            Context safety
          </div>
          <div className="text-muted-foreground mt-1 text-xs">
            Redaction, recipient visibility, and local-only enforcement.
          </div>
        </div>
        <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
          {agentConfigs.status === "success" && reviewAgents.length > 0 && (
            <NativeSelect
              aria-label="Context preview agent"
              className="w-48"
              size="sm"
              value={selectedAgentId}
              onChange={(event) => {
                setSelectedAgentId(event.target.value);
                setPreview(idleApiState());
              }}
            >
              {reviewAgents.map((agent) => (
                <NativeSelectOption key={agent.id} value={agent.id}>
                  {agent.name}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          )}
          <Button
            disabled={
              !client ||
              !selectedAgentId ||
              preview.status === "loading" ||
              agentConfigs.status === "loading"
            }
            size="sm"
            onClick={() => void runPreview()}
          >
            <ShieldCheckIcon data-icon="inline-start" />
            Preview
          </Button>
        </div>
      </div>

      {agentConfigs.status === "loading" && (
        <LoadingRows rows={3} className="p-4" />
      )}
      {agentConfigs.status === "error" && (
        <ErrorState
          className="m-3"
          title="Agents unavailable"
          description={agentConfigs.error.message}
        />
      )}
      {agentConfigs.status === "success" && reviewAgents.length === 0 && (
        <EmptyState
          className="border-0 p-6"
          title="No review agents"
          description="This session does not have configured agent rows to preview."
          icon={TerminalIcon}
        />
      )}
      {agentConfigs.status === "success" && reviewAgents.length > 0 && (
        <ContextSafetyReport
          agent={selectedAgent}
          preview={preview}
          session={session}
        />
      )}
    </section>
  );
}

function ContextSafetyReport({
  agent,
  preview,
  session,
}: {
  agent?: AgentConfig;
  preview: Loadable<ContextBundlePreview>;
  session: ReviewSession;
}) {
  if (preview.status === "idle") {
    return (
      <div className="p-4">
        <div className="grid gap-3 sm:grid-cols-3">
          <MetricTile
            label="Redaction"
            value={
              session.context_policy.redact_secrets === false ? "off" : "on"
            }
          />
          <MetricTile
            label="Recipient"
            value={agent ? agentProvider(agent) : "agent"}
          />
          <MetricTile
            label="Egress"
            value={agent ? agentEgress(agent) : "unknown"}
          />
        </div>
      </div>
    );
  }

  if (preview.status === "loading") {
    return <LoadingRows rows={4} className="p-4" />;
  }

  if (preview.status === "error") {
    return (
      <ErrorState
        className="m-3"
        title="Context preview failed"
        description={preview.error.message}
      />
    );
  }

  const redaction = preview.data.redaction_report;
  const visibility = preview.data.visibility_report;
  const omitted = visibility.omitted ?? [];
  const redactedItems = redaction.items.slice(0, 6);

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="grid gap-3 sm:grid-cols-4">
        <MetricTile
          label="Redactions"
          value={String(redaction.redaction_count)}
        />
        <MetricTile label="Sent" value={String(visibility.sent_item_count)} />
        <MetricTile
          label="Tokens"
          value={String(preview.data.bundle.token_estimate)}
        />
        <MetricTile label="Omitted" value={String(omitted.length)} />
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Badge variant="outline">
          {visibility.recipient.provider ??
            agentProvider(agent ?? { capabilities: {} })}
        </Badge>
        <Badge
          variant={
            (visibility.recipient.egress ??
              agentEgress(agent ?? { capabilities: {} })) === "local"
              ? "outline"
              : "secondary"
          }
        >
          {visibility.recipient.egress ??
            agentEgress(agent ?? { capabilities: {} })}
        </Badge>
        {visibility.local_only_enforced && (
          <Badge variant="secondary">local-only enforced</Badge>
        )}
        {preview.data.redaction_report_artifact_id && (
          <Badge variant="outline">
            report {preview.data.redaction_report_artifact_id}
          </Badge>
        )}
      </div>

      <div className="grid gap-3 lg:grid-cols-2">
        <div className="rounded-md border">
          <div className="border-b px-3 py-2 text-xs font-medium">
            Redaction report
          </div>
          {redactedItems.length === 0 ? (
            <EmptyState
              className="border-0 p-4"
              title="No redactions"
              description="The current preview did not match configured secret detectors."
              icon={ShieldCheckIcon}
            />
          ) : (
            <div className="divide-y">
              {redactedItems.map((item) => (
                <RedactionReportRow key={item.item_id} item={item} />
              ))}
            </div>
          )}
        </div>

        <div className="rounded-md border">
          <div className="border-b px-3 py-2 text-xs font-medium">
            Visibility report
          </div>
          <div className="space-y-3 p-3">
            <div className="grid gap-2 text-xs sm:grid-cols-2">
              {Object.entries(visibility.sent_item_by_kind ?? {}).map(
                ([kind, count]) => (
                  <div
                    key={kind}
                    className="bg-surface flex items-center justify-between gap-2 rounded-md px-2 py-1.5"
                  >
                    <span className="truncate">{formatContextKind(kind)}</span>
                    <span className="text-muted-foreground">{count}</span>
                  </div>
                ),
              )}
            </div>
            {visibility.local_only_paths &&
              visibility.local_only_paths.length > 0 && (
                <div className="space-y-1">
                  <div className="text-muted-foreground text-xs">
                    Local-only paths
                  </div>
                  {visibility.local_only_paths.slice(0, 5).map((path) => (
                    <div
                      key={path}
                      className="bg-surface truncate rounded-md px-2 py-1.5 font-mono text-xs"
                    >
                      {path}
                    </div>
                  ))}
                </div>
              )}
            {omitted.length > 0 && (
              <div className="space-y-1">
                <div className="text-muted-foreground text-xs">Omitted</div>
                {omitted.slice(0, 6).map((item, index) => (
                  <div
                    key={`${item.item_id ?? item.path ?? "omitted"}-${index}`}
                    className="bg-surface rounded-md px-2 py-1.5 text-xs"
                  >
                    <div className="truncate font-mono">
                      {item.path ?? item.item_id ?? item.kind ?? "context item"}
                    </div>
                    <div className="text-muted-foreground mt-1 truncate">
                      {item.reason}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function MetricTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-surface rounded-md border px-3 py-2">
      <div className="text-muted-foreground text-xs">{label}</div>
      <div className="mt-1 truncate text-sm font-medium">{value}</div>
    </div>
  );
}

function RedactionReportRow({ item }: { item: RedactionReportItem }) {
  return (
    <div className="px-3 py-2 text-xs">
      <div className="flex min-w-0 items-center justify-between gap-2">
        <span className="truncate font-medium">
          {item.path ?? item.title ?? item.item_id}
        </span>
        <Badge variant="outline">{item.redaction_count}</Badge>
      </div>
      <div className="text-muted-foreground mt-1 truncate">
        {formatContextKind(item.kind)} • {formatDetectorSummary(item.detectors)}
      </div>
    </div>
  );
}

function ReviewAuditLogPanel({
  client,
  session,
}: {
  client: ApiClient | null;
  session: ReviewSession;
}) {
  const [auditLog, setAuditLog] =
    useState<Loadable<ReviewAuditLogResponse>>(idleApiState());

  useEffect(() => {
    if (!client) {
      let canceled = false;
      queueMicrotask(() => {
        if (!canceled) {
          setAuditLog(idleApiState());
        }
      });
      return () => {
        canceled = true;
      };
    }
    const api = client;
    let canceled = false;

    async function load(initialLoad = false) {
      if (initialLoad) {
        setAuditLog((current) =>
          current.status === "success" ? current : loadingApiState(),
        );
      }
      const next = await loadApiResource(() =>
        api.getReviewAuditLog(session.id),
      );
      if (canceled) {
        return;
      }
      setAuditLog((current) => preserveSuccessfulLoadable(current, next));
    }

    queueMicrotask(() => {
      if (!canceled) {
        void load(true);
      }
    });
    const interval = window.setInterval(() => void load(), 5000);
    return () => {
      canceled = true;
      window.clearInterval(interval);
    };
  }, [client, session.id]);

  const entries =
    auditLog.status === "success"
      ? auditLog.data.entries.slice(0, MAX_AUDIT_ENTRIES_RENDERED)
      : [];

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium">
            <BellIcon className="size-4" />
            Audit log
          </div>
          <div className="text-muted-foreground mt-1 text-xs">
            Decisions, copy packets, publish drafts, and workflow events.
          </div>
        </div>
        <Badge variant="secondary">{entries.length}</Badge>
      </div>
      {auditLog.status === "idle" && (
        <EmptyState
          className="border-0 p-6"
          title="Audit unavailable"
          description="Connect to cocoded to load the session audit trail."
          icon={ClockIcon}
        />
      )}
      {auditLog.status === "loading" && (
        <LoadingRows rows={5} className="p-4" />
      )}
      {auditLog.status === "error" && (
        <ErrorState
          className="m-3"
          title="Audit log unavailable"
          description={auditLog.error.message}
        />
      )}
      {auditLog.status === "success" && entries.length === 0 && (
        <EmptyState
          className="border-0 p-6"
          title="No audit entries"
          description="Review activity will appear here as it is recorded."
          icon={ClockIcon}
        />
      )}
      {entries.length > 0 && (
        <div className="max-h-[560px] overflow-y-auto">
          {entries.map((entry) => (
            <AuditLogEntryRow key={`${entry.kind}-${entry.id}`} entry={entry} />
          ))}
        </div>
      )}
    </section>
  );
}

function AuditLogEntryRow({ entry }: { entry: ReviewAuditLogEntry }) {
  const metadata = formatAuditMetadata(entry.metadata);
  return (
    <div className="grid grid-cols-[88px_minmax(0,1fr)] gap-3 border-b px-4 py-3 text-sm last:border-b-0">
      <div className="text-muted-foreground text-xs">
        {formatRelativeAge(entry.created_at)}
      </div>
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <Badge
            variant={
              entry.level === "error" || entry.status === "failed"
                ? "destructive"
                : entry.kind === "publish_draft" ||
                    entry.kind === "github_publication"
                  ? "secondary"
                  : "outline"
            }
          >
            {formatAuditKind(entry.kind)}
          </Badge>
          {entry.status && <Badge variant="outline">{entry.status}</Badge>}
          <span className="min-w-0 truncate font-medium">{entry.title}</span>
        </div>
        <div className="text-muted-foreground mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs">
          {entry.sequence ? <span>#{entry.sequence}</span> : null}
          {entry.finding_id ? <span>{entry.finding_id}</span> : null}
          {entry.agent_run_id ? <span>{entry.agent_run_id}</span> : null}
          {entry.artifact_id ? <span>{entry.artifact_id}</span> : null}
          {entry.copy_packet_id ? <span>{entry.copy_packet_id}</span> : null}
          {entry.publish_draft_id ? (
            <span>{entry.publish_draft_id}</span>
          ) : null}
        </div>
        {metadata && (
          <div className="text-muted-foreground mt-2 line-clamp-2 font-mono text-xs">
            {metadata}
          </div>
        )}
      </div>
    </div>
  );
}

function ReviewEventTimeline({
  events,
  streamState,
}: {
  events: ReviewEvent[];
  streamState: Loadable<true>;
}) {
  const streamLabel =
    streamState.status === "loading"
      ? "connecting"
      : streamState.status === "error"
        ? "stream issue"
        : streamState.status === "success"
          ? "live"
          : "idle";

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="text-sm font-medium">Event timeline</div>
          <div className="text-muted-foreground mt-1 text-xs">
            Live SSE events with sequence IDs for replay/debugging.
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant="outline">{streamLabel}</Badge>
          <Badge variant="secondary">{events.length}</Badge>
        </div>
      </div>
      {events.length === 0 && streamState.status === "loading" ? (
        <LoadingRows rows={3} className="p-4" />
      ) : events.length === 0 && streamState.status === "error" ? (
        <ErrorState
          className="m-3"
          title="Event stream disconnected"
          description={streamState.error.message}
        />
      ) : events.length === 0 ? (
        <EmptyState
          className="border-0 p-6"
          title="No events yet"
          description="Start a review to stream workflow, agent, and finding events."
          icon={ClockIcon}
        />
      ) : (
        <>
          {streamState.status === "error" && (
            <ErrorState
              className="m-3"
              title="Event stream disconnected"
              description={streamState.error.message}
            />
          )}
          <div className="max-h-[520px] overflow-y-auto">
            {events.map((event) => (
              <div
                key={`${event.sequence}-${event.id}`}
                className="grid grid-cols-[72px_minmax(0,1fr)] gap-3 border-b px-4 py-3 text-sm last:border-b-0"
              >
                <div className="text-muted-foreground text-xs">
                  #{event.sequence}
                </div>
                <div className="min-w-0">
                  <div className="flex min-w-0 items-center gap-2">
                    <Badge
                      variant={
                        event.level === "error" ? "destructive" : "outline"
                      }
                    >
                      {event.level || "info"}
                    </Badge>
                    <span className="truncate font-medium">{event.type}</span>
                  </div>
                  <div className="text-muted-foreground mt-1 truncate text-xs">
                    {formatRelativeAge(event.created_at)}
                    {event.agent_run_id ? ` • ${event.agent_run_id}` : ""}
                    {event.artifact_id ? ` • ${event.artifact_id}` : ""}
                  </div>
                  <div className="text-muted-foreground mt-2 line-clamp-2 font-mono text-xs">
                    {JSON.stringify(event.payload)}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </section>
  );
}

function formatContextKind(kind: string): string {
  return kind
    .replace(/_/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function formatDetectorSummary(detectors: Record<string, number>): string {
  const entries = Object.entries(detectors)
    .sort((left, right) => right[1] - left[1])
    .slice(0, 4)
    .map(([name, count]) => `${formatContextKind(name)} ${count}`);
  return entries.length > 0 ? entries.join(", ") : "No detector details";
}

function formatAuditKind(kind: string): string {
  return formatContextKind(kind);
}

function formatAuditMetadata(metadata: Record<string, unknown>): string {
  const entries = Object.entries(metadata)
    .filter(
      ([, value]) => value !== "" && value !== null && value !== undefined,
    )
    .slice(0, 6)
    .map(([key, value]) => `${key}: ${formatAuditMetadataValue(value)}`);
  const text = entries.join(" • ");
  return text.length > 260 ? `${text.slice(0, 260)}...` : text;
}

function formatAuditMetadataValue(value: unknown): string {
  if (typeof value === "string") {
    return value.length > 120 ? `${value.slice(0, 120)}...` : value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  if (Array.isArray(value)) {
    return `${value.length} items`;
  }
  if (value && typeof value === "object") {
    return JSON.stringify(value).slice(0, 120);
  }
  return String(value);
}
