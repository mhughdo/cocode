import { ArrowRightIcon, ListChecksIcon } from "lucide-react";

import { Badge, type BadgeVariant } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { Finding } from "@/lib/api";
import {
  formatConfidence,
  formatDecisionLabel,
  formatFindingLocation,
} from "../evidence/review-evidence-utils";

export function FinalFindingsMessage({
  findings,
  onOpenFindingDetail,
  onOpenFindings,
}: {
  findings: Finding[];
  onOpenFindingDetail: (finding: Finding) => void;
  onOpenFindings: () => void;
}) {
  const findingCount = findings.length;
  return (
    <article
      aria-label="Finalized findings"
      className="bg-card border-border-subtle flex gap-3 rounded-xl border px-4 py-3"
    >
      <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-[#0f0f0f] text-white">
        <ListChecksIcon className="size-4" />
      </span>
      <div className="min-w-0 flex-1">
        <div className="mb-1 flex min-w-0 flex-wrap items-center gap-2 text-[13px]">
          <span className="font-semibold">System</span>
          <Badge variant="outline" className="h-4 px-1.5 text-[10px]">
            finalized
          </Badge>
        </div>
        <div className="space-y-3">
          <div className="min-w-0">
            <h3 className="text-sm font-semibold">Findings finalized</h3>
            <p className="text-muted-foreground mt-1 text-[13px] leading-6 break-words">
              The orchestrator finalized {findingCount}{" "}
              {findingCount === 1 ? "finding" : "findings"}. Open the full
              findings table or jump straight into an individual finding.
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              className="h-8 rounded-lg"
              size="sm"
              type="button"
              onClick={onOpenFindings}
            >
              Open Findings
              <ArrowRightIcon data-icon="inline-end" />
            </Button>
          </div>
          {findings.length > 0 && (
            <div className="border-border-subtle max-h-72 overflow-y-auto rounded-lg border">
              {findings.map((finding, index) => (
                <button
                  className="hover:bg-surface flex w-full min-w-0 items-start gap-3 border-b px-3 py-3 text-left last:border-b-0"
                  key={finding.id}
                  type="button"
                  onClick={() => onOpenFindingDetail(finding)}
                >
                  <span className="text-muted-foreground mt-0.5 w-5 shrink-0 text-xs tabular-nums">
                    {index + 1}.
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm leading-5 font-medium break-words [overflow-wrap:anywhere]">
                      {finding.canonical_claim}
                    </span>
                    <span className="text-muted-foreground mt-1 block font-mono text-[11px] leading-5 break-all">
                      {formatFindingLocation(finding)}
                    </span>
                  </span>
                  <span className="flex max-w-[12rem] shrink-0 flex-wrap justify-end gap-1">
                    <Badge
                      variant={severityBadgeVariant(finding.severity)}
                      className="capitalize"
                    >
                      {finding.severity || "unknown"}
                    </Badge>
                    <Badge variant="outline">
                      {formatDecisionLabel(finding.verification_status)}
                    </Badge>
                    <Badge variant="outline">
                      {formatConfidence(finding.confidence)}
                    </Badge>
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
    </article>
  );
}

function severityBadgeVariant(severity: string): BadgeVariant {
  switch (severity) {
    case "blocker":
      return "severity-blocker";
    case "high":
      return "severity-high";
    case "medium":
      return "severity-medium";
    case "low":
    case "info":
      return "severity-low";
    default:
      return "outline";
  }
}
