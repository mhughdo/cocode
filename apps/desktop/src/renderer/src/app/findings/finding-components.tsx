import { CopyIcon, FileSearchIcon } from "lucide-react";

import { EmptyState, LoadingRows } from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import type { EvidenceItem, Finding, FindingDetailResponse } from "@/lib/api";
import { languageForFilePath } from "@/lib/syntax-highlighting";
import { cn } from "@/lib/utils";
import { SyntaxCodeBlock } from "../chat/markdown-message";
import {
  evidenceBadgeVariant,
  evidenceCodeSnippet,
  evidenceKindDisplayLabel,
  evidenceLineWindow,
  formatEvidenceLocation,
  formatFindingLocation,
  prioritizedCodeSnippetItems,
  prioritizedEvidenceItems,
  sourceAgentSummary,
} from "../evidence/review-evidence-utils";

export type FindingDecisionMutation = "accepted" | "dismissed" | "deferred";

type FindingDecisionSelectValue =
  | "accepted"
  | "dismissed"
  | "deferred"
  | "pending";

export function FindingCard({
  actionState,
  finding,
  layout = "wide",
  onDecisionChange,
  onOpenDetail,
  onSelect,
  selected,
}: {
  actionState: {
    status: "idle" | "loading" | "success" | "error";
    findingId?: string;
    action?: string;
  };
  finding: Finding;
  layout?: "wide" | "inspector";
  onDecisionChange: (decision: FindingDecisionMutation) => void;
  onOpenDetail: () => void;
  onSelect: () => void;
  selected: boolean;
}) {
  const pending =
    actionState.status === "loading" && actionState.findingId === finding.id;
  const confidence = `${Math.round(finding.confidence * 100)}%`;
  const sourceAgents = finding.source_agents ?? [];
  const selectRow = (target: EventTarget | null) => {
    if (
      target instanceof HTMLElement &&
      target.closest("button,select,a,input,textarea,[role='button']")
    ) {
      return;
    }
    onSelect();
  };
  return (
    <div
      className={cn(
        "grid w-full cursor-pointer grid-cols-1 gap-3 border-b border-l-2 border-l-transparent px-4 py-3 text-left transition-colors last:border-b-0 hover:bg-[#fbfbfa]",
        layout === "inspector"
          ? "lg:grid-cols-[72px_minmax(0,1fr)_132px_72px]"
          : "lg:grid-cols-[72px_minmax(0,1.35fr)_minmax(96px,0.64fr)_132px_minmax(112px,0.64fr)_72px]",
        selected && "border-l-foreground bg-[#f7f7f5]",
      )}
      aria-selected={selected}
      data-testid={`finding-row-${finding.id}`}
      onClick={(event) => selectRow(event.target)}
    >
      <div className="flex min-w-0 items-start gap-2 lg:block">
        <Badge
          className="shrink-0"
          variant={
            finding.severity === "high" || finding.severity === "blocker"
              ? "destructive"
              : finding.severity === "medium"
                ? "secondary"
                : "outline"
          }
        >
          {finding.severity}
        </Badge>
      </div>
      <div className="min-w-0">
        <button
          className="focus-visible:ring-ring block w-full max-w-full min-w-0 cursor-pointer truncate rounded-sm text-left text-sm font-semibold underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none"
          type="button"
          onClick={(event) => {
            event.stopPropagation();
            onOpenDetail();
          }}
        >
          {finding.canonical_claim}
        </button>
        {finding.evidence_summary && (
          <div className="text-muted-foreground mt-1 line-clamp-2 text-xs leading-5 [overflow-wrap:anywhere] break-words">
            {finding.evidence_summary}
          </div>
        )}
      </div>
      <div
        className={cn(
          "text-muted-foreground min-w-0 text-xs lg:pt-0.5",
          layout === "inspector" && "lg:hidden",
        )}
      >
        <div className="truncate font-mono">
          {finding.primary_path || "no location"}
        </div>
        {finding.primary_start_line ? (
          <div className="mt-1">L{finding.primary_start_line}</div>
        ) : null}
      </div>
      <div className="flex min-w-0 items-start lg:pt-0.5">
        <NativeSelect
          aria-label={`Set status for ${finding.canonical_claim}`}
          className="w-full min-w-0"
          disabled={pending}
          size="sm"
          value={findingDecisionSelectValue(finding)}
          onChange={(event) => {
            event.stopPropagation();
            const decision = findingDecisionMutationFromValue(
              event.target.value,
            );
            if (decision) {
              onDecisionChange(decision);
            }
          }}
          onClick={(event) => event.stopPropagation()}
        >
          <NativeSelectOption value="pending" disabled>
            Needs triage
          </NativeSelectOption>
          <NativeSelectOption value="accepted">Accepted</NativeSelectOption>
          <NativeSelectOption value="dismissed">Dismissed</NativeSelectOption>
          <NativeSelectOption value="deferred">Deferred</NativeSelectOption>
        </NativeSelect>
      </div>
      <div
        className={cn(
          "min-w-0 lg:pt-0.5",
          layout === "inspector" && "lg:hidden",
        )}
      >
        {sourceAgents.length > 0 ? (
          <div className="flex min-w-0 flex-col gap-1">
            <div className="truncate text-xs font-medium">
              {sourceAgentSummary(sourceAgents)}
            </div>
            <div className="text-muted-foreground truncate text-[11px]">
              {sourceAgents.length} signal
              {sourceAgents.length === 1 ? "" : "s"}
            </div>
          </div>
        ) : (
          <span className="text-muted-foreground text-xs">No agent source</span>
        )}
      </div>
      <div className="flex min-w-0 items-start justify-start gap-2 lg:justify-end">
        <div className="text-muted-foreground pt-1 text-xs tabular-nums">
          {confidence}
        </div>
      </div>
    </div>
  );
}

export function CodeSnippetViewer({
  evidence,
  finding,
  onCopyPath,
}: {
  evidence: EvidenceItem[];
  finding: Finding;
  onCopyPath: () => void;
}) {
  const primaryPath = finding.primary_path?.trim();
  const snippets = prioritizedCodeSnippetItems(evidence, finding)
    .filter((item) => item.path && item.path === primaryPath)
    .slice(0, 1);

  if (snippets.length === 0) {
    const lineNumber = finding.primary_start_line || 1;
    const path = finding.primary_path || formatFindingLocation(finding);
    return (
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-2">
          <div className="text-xs font-medium">Primary code</div>
          <Button size="sm" variant="outline" onClick={onCopyPath}>
            <CopyIcon data-icon="inline-start" />
            Copy path
          </Button>
        </div>
        <div className="border-border/70 overflow-hidden rounded-lg border bg-white shadow-[0_1px_2px_rgb(17_18_20/0.03)]">
          <div className="border-border/60 flex items-center justify-between gap-2 border-b bg-[#fbfbfa] px-3 py-2">
            <span className="truncate font-mono text-xs">{path}</span>
            <div className="flex shrink-0 items-center gap-1.5">
              <Badge variant="outline">location</Badge>
              <Badge variant="secondary">L{lineNumber}</Badge>
            </div>
          </div>
          <div className="p-3">
            <p className="text-muted-foreground text-sm leading-6">
              No code window is attached yet. Refresh or rebuild evidence to
              hydrate the exact location.
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        <div className="text-xs font-medium">Primary code</div>
        <Button size="sm" variant="outline" onClick={onCopyPath}>
          <CopyIcon data-icon="inline-start" />
          Copy path
        </Button>
      </div>
      {snippets.map((item) => {
        const codeSnippet = evidenceCodeSnippet(item);
        const lineWindow = evidenceLineWindow(item);
        return (
          <div
            key={item.id}
            className="border-border/70 overflow-hidden rounded-lg border bg-white shadow-[0_1px_2px_rgb(17_18_20/0.03)]"
          >
            <div className="border-border/60 flex items-center justify-between gap-2 border-b bg-[#fbfbfa] px-3 py-2">
              <span className="truncate font-mono text-xs">
                {item.path || formatFindingLocation(finding)}
              </span>
              <div className="flex shrink-0 items-center gap-1.5">
                <Badge variant="outline">
                  {evidenceKindDisplayLabel(item.kind)}
                </Badge>
                {lineWindow?.start_line || item.start_line ? (
                  <Badge variant="secondary">
                    L{lineWindow?.start_line ?? item.start_line}
                  </Badge>
                ) : null}
              </div>
            </div>
            <div className="max-h-[420px] overflow-auto bg-white [scrollbar-gutter:stable_both-edges] [scrollbar-width:thin]">
              <SyntaxCodeBlock
                className="rounded-none border-0 bg-white"
                code={codeSnippet}
                language={languageForFilePath(
                  item.path || finding.primary_path || "",
                )}
                lineNumbers
                startLine={lineWindow?.start_line ?? item.start_line ?? 1}
              />
            </div>
          </div>
        );
      })}
    </div>
  );
}

export function EvidenceCardList({
  detail,
}: {
  detail?: FindingDetailResponse;
}) {
  if (!detail) {
    return <LoadingRows rows={4} />;
  }
  const items = prioritizedEvidenceItems(detail.evidence_items);
  if (items.length === 0) {
    return (
      <EmptyState
        className="border-0"
        title="No evidence yet"
        description="Evidence rows will appear after verification."
        icon={FileSearchIcon}
      />
    );
  }
  return (
    <div className="flex flex-col gap-2">
      {items.map((item) => (
        <div key={item.id} className="rounded-md border p-3">
          <div className="mb-2 flex items-center justify-between gap-2">
            <div className="min-w-0">
              <div className="truncate text-sm font-medium">{item.title}</div>
              <div className="text-muted-foreground mt-1 truncate font-mono text-xs">
                {item.path ? formatEvidenceLocation(item) : item.kind}
              </div>
            </div>
            <Badge variant={evidenceBadgeVariant(item.kind)}>
              {evidenceKindDisplayLabel(item.kind)}
            </Badge>
          </div>
          <p className="text-muted-foreground line-clamp-3 text-sm leading-6">
            {item.summary}
          </p>
        </div>
      ))}
    </div>
  );
}

function findingDecisionSelectValue(
  finding: Finding,
): FindingDecisionSelectValue {
  const decision = finding.decision_status.trim();
  if (
    decision === "accepted" ||
    decision === "dismissed" ||
    decision === "deferred"
  ) {
    return decision;
  }
  return "pending";
}

function findingDecisionMutationFromValue(
  value: string,
): FindingDecisionMutation | null {
  if (value === "accepted" || value === "dismissed" || value === "deferred") {
    return value;
  }
  return null;
}
