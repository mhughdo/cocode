import {
  ArrowLeftIcon,
  CheckIcon,
  CopyIcon,
  ExternalLinkIcon,
  MapIcon,
  MessageSquareIcon,
  MinusIcon,
  PencilLineIcon,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import type { EvidenceItem, Finding, FindingDetailResponse } from "@/lib/api";

import {
  evidenceCodeSnippet,
  evidenceItemsOrEmpty,
  formatConfidence,
  formatDecisionLabel,
  formatEvidenceLocation,
  formatFindingLocation,
  formatShortDate,
  firstMeaningfulSnippetLine,
} from "./review-evidence-utils";
import {
  PanelFrame,
  PanelMarkdown,
  Section,
  type LoadingState,
} from "./review-panel-shell";

function FactsGrid({
  detail,
  finding,
}: {
  detail: FindingDetailResponse;
  finding: Finding;
}) {
  const rows: Array<[string, string]> = [
    ["Severity", formatDecisionLabel(finding.severity)],
    ["Status", formatDecisionLabel(finding.verification_status)],
    ["Trust", formatDecisionLabel(finding.trust_state)],
    ["Decision", formatDecisionLabel(finding.decision_status)],
    ["Signals", String(detail.candidates.length || finding.merged_from_count)],
    ["Confidence", formatConfidence(finding.confidence)],
    ["First seen", formatShortDate(finding.first_seen_at)],
    ["Updated", formatShortDate(finding.updated_at)],
  ];
  return (
    <dl className="grid gap-3 text-sm">
      {rows.map(([label, value]) => (
        <div className="grid grid-cols-[82px_minmax(0,1fr)] gap-3" key={label}>
          <dt className="text-muted-foreground min-w-0">{label}</dt>
          <dd className="min-w-0 text-right font-medium [overflow-wrap:anywhere] break-words">
            {value}
          </dd>
        </div>
      ))}
    </dl>
  );
}

function FindingActions({
  actionState,
  disabled,
  onAccept,
  onCopyFixPacket,
  onDismiss,
  onOpenDetail,
  onOpenEvidenceMap,
  onOpenFollowUp,
}: {
  actionState: { status: LoadingState; message?: string };
  disabled?: boolean;
  onAccept: () => void;
  onCopyFixPacket: () => void;
  onDismiss: () => void;
  onOpenDetail?: () => void;
  onOpenEvidenceMap: () => void;
  onOpenFollowUp: () => void;
}) {
  return (
    <Section title="Actions">
      <div className="grid min-w-0 grid-cols-2 gap-2">
        <Button
          className="min-w-0 justify-center overflow-hidden"
          disabled={disabled}
          onClick={onAccept}
        >
          <CheckIcon data-icon="inline-start" />
          Accept finding
        </Button>
        <Button
          className="min-w-0 justify-center overflow-hidden"
          disabled={disabled}
          variant="outline"
          onClick={onDismiss}
        >
          <MinusIcon data-icon="inline-start" />
          Dismiss
        </Button>
      </div>
      <div className="mt-2 grid min-w-0 grid-cols-2 gap-2">
        <Button
          className="min-w-0 justify-start overflow-hidden"
          disabled={disabled}
          size="sm"
          variant="outline"
          onClick={onCopyFixPacket}
        >
          <CopyIcon data-icon="inline-start" />
          Copy fix packet
        </Button>
        {onOpenDetail ? (
          <Button
            className="min-w-0 justify-start overflow-hidden"
            disabled={disabled}
            size="sm"
            variant="outline"
            onClick={onOpenDetail}
          >
            <ExternalLinkIcon data-icon="inline-start" />
            Open full detail
          </Button>
        ) : null}
        <Button
          className="min-w-0 justify-start overflow-hidden"
          disabled={disabled}
          size="sm"
          variant="outline"
          onClick={onOpenEvidenceMap}
        >
          <MapIcon data-icon="inline-start" />
          Evidence map
        </Button>
        <Button
          className="min-w-0 justify-start overflow-hidden"
          disabled={disabled}
          size="sm"
          variant="outline"
          onClick={onOpenFollowUp}
        >
          <MessageSquareIcon data-icon="inline-start" />
          Follow-up
        </Button>
      </div>
      {actionState.message && actionState.status !== "idle" ? (
        <div className="text-muted-foreground mt-3 text-xs">
          {actionState.message}
        </div>
      ) : null}
    </Section>
  );
}

function DraftCommentPanel({
  body,
  onBodyChange,
  onCopy,
  onSave,
  disabled,
  title = "Draft GitHub comment",
}: {
  body: string;
  onBodyChange: (value: string) => void;
  onCopy: () => void;
  onSave: () => void;
  disabled?: boolean;
  title?: string;
}) {
  return (
    <Section title={title}>
      <div className="flex items-center justify-end gap-2">
        <Button
          disabled={disabled}
          size="sm"
          variant="outline"
          onClick={onCopy}
        >
          <CopyIcon data-icon="inline-start" />
          Copy
        </Button>
        <Button
          disabled={disabled}
          size="sm"
          variant="outline"
          onClick={onSave}
        >
          <PencilLineIcon data-icon="inline-start" />
          Save
        </Button>
      </div>
      <Textarea
        aria-label={title}
        className="mt-3 max-h-64 min-h-32 resize-y overflow-auto font-mono text-xs leading-5 [scrollbar-width:thin]"
        value={body}
        onChange={(event) => onBodyChange(event.target.value)}
      />
    </Section>
  );
}

function EvidenceStory({
  counterEvidence,
  detail,
  finding,
  supportingEvidence,
  testEvidence,
  verificationLeads,
}: {
  counterEvidence: EvidenceItem[];
  detail: FindingDetailResponse;
  finding: Finding;
  supportingEvidence: EvidenceItem[];
  testEvidence: EvidenceItem[];
  verificationLeads: EvidenceItem[];
}) {
  const primary = supportingEvidence[0];
  const counter = counterEvidence[0];
  const lead = verificationLeads[0];
  const test = testEvidence[0];
  const observedSnippet = primary ? evidenceCodeSnippet(primary) : "";
  const observedCode = observedSnippet
    ? firstMeaningfulSnippetLine(observedSnippet)
    : "";
  const sourceCount = Math.max(
    detail.candidates.length,
    finding.merged_from_count,
  );
  return (
    <Section title="Evidence story">
      <div className="space-y-3 text-sm leading-6">
        <div>
          <div className="text-muted-foreground text-xs font-medium uppercase">
            Issue
          </div>
          <p className="mt-1 font-medium [overflow-wrap:anywhere] break-words">
            {finding.canonical_claim}
          </p>
          <p className="text-muted-foreground mt-1 font-mono text-xs break-all">
            {formatFindingLocation(finding)}
          </p>
        </div>
        {primary ? (
          <div>
            <div className="text-muted-foreground text-xs font-medium uppercase">
              Supporting evidence
            </div>
            <PanelMarkdown className="mt-1">{primary.summary}</PanelMarkdown>
            <p className="text-muted-foreground mt-1 text-xs break-all">
              {formatEvidenceLocation(primary)}
            </p>
            {observedCode ? (
              <p className="text-muted-foreground mt-1 font-mono text-xs break-all">
                {observedCode}
              </p>
            ) : null}
          </div>
        ) : null}
        {counter ? (
          <div>
            <div className="text-muted-foreground text-xs font-medium uppercase">
              Verified contradiction
            </div>
            <PanelMarkdown className="mt-1">{counter.summary}</PanelMarkdown>
            <p className="text-muted-foreground mt-1 text-xs break-all">
              {formatEvidenceLocation(counter)}
            </p>
          </div>
        ) : lead ? (
          <div>
            <div className="text-muted-foreground text-xs font-medium uppercase">
              Verification lead
            </div>
            <PanelMarkdown className="mt-1">{lead.summary}</PanelMarkdown>
            <p className="text-muted-foreground mt-1 text-xs break-all">
              {formatEvidenceLocation(lead)}
            </p>
          </div>
        ) : null}
        {test ? (
          <div>
            <div className="text-muted-foreground text-xs font-medium uppercase">
              Test signal
            </div>
            <PanelMarkdown className="mt-1">{test.summary}</PanelMarkdown>
            <p className="text-muted-foreground mt-1 text-xs break-all">
              {formatEvidenceLocation(test)}
            </p>
          </div>
        ) : null}
        {finding.suggested_fix ? (
          <div>
            <div className="text-muted-foreground text-xs font-medium uppercase">
              Suggested fix
            </div>
            <PanelMarkdown className="mt-1">
              {finding.suggested_fix}
            </PanelMarkdown>
          </div>
        ) : null}
        {sourceCount > 0 ? (
          <div className="text-muted-foreground text-xs">
            {sourceCount} reviewer signal{sourceCount === 1 ? "" : "s"} merged
            here.
          </div>
        ) : null}
      </div>
    </Section>
  );
}

export function FindingsInspectorPanel({
  actionState,
  className,
  detail,
  draftComment,
  finding,
  onAccept,
  onCopyFixPacket,
  onCopyPath,
  onDismiss,
  onDraftCommentChange,
  onOpenDetail,
  onOpenEvidenceMap,
  onOpenFollowUp,
  onSaveDraftComment,
}: {
  actionState: { status: LoadingState; message?: string };
  className?: string;
  detail?: FindingDetailResponse;
  draftComment: string;
  finding: Finding;
  onAccept: () => void;
  onCopyFixPacket: () => void;
  onCopyPath: () => void;
  onDismiss: () => void;
  onDraftCommentChange: (value: string) => void;
  onOpenDetail?: () => void;
  onOpenEvidenceMap: () => void;
  onOpenFollowUp: () => void;
  onSaveDraftComment: () => void;
}) {
  const supportingEvidence = evidenceItemsOrEmpty(
    detail?.evidence_groups?.supporting,
  );
  const counterEvidence = evidenceItemsOrEmpty(
    detail?.evidence_groups?.counter,
  );
  const missingEvidence = evidenceItemsOrEmpty(
    detail?.evidence_groups?.missing,
  );
  const testEvidence = evidenceItemsOrEmpty(detail?.evidence_groups?.test);
  const searchEvidence = evidenceItemsOrEmpty(detail?.evidence_groups?.search);
  const verificationLeads = [
    ...counterEvidence,
    ...missingEvidence,
    ...testEvidence,
    ...searchEvidence,
  ];

  return (
    <PanelFrame
      className={className}
      actions={
        onOpenDetail ? (
          <Button
            size="icon-sm"
            variant="ghost"
            onClick={onOpenDetail}
            title="Open detail"
          >
            <ArrowLeftIcon className="size-4 rotate-180" />
          </Button>
        ) : undefined
      }
      subtitle={formatFindingLocation(finding)}
      title={finding.canonical_claim}
      eyebrow="Finding"
    >
      <Section title="Overview">
        <div className="flex flex-wrap gap-2">
          <Badge
            variant={
              finding.severity === "blocker"
                ? "severity-blocker"
                : finding.severity === "high"
                  ? "severity-high"
                  : finding.severity === "medium"
                    ? "severity-medium"
                    : "severity-low"
            }
          >
            {finding.severity}
          </Badge>
          <Badge variant="status-verified">
            {formatDecisionLabel(finding.verification_status)}
          </Badge>
          <Badge variant="status-triage">
            {formatDecisionLabel(finding.decision_status)}
          </Badge>
        </div>
        <PanelMarkdown className="mt-3">
          {finding.evidence_summary}
        </PanelMarkdown>
        {finding.publish_blockers?.length ? (
          <div className="border-border-subtle bg-surface-sunken text-muted-foreground mt-3 rounded-md border px-3 py-2 text-xs">
            <div className="text-foreground font-medium">Publish blockers</div>
            <ul className="mt-1 list-disc space-y-1 pl-4">
              {finding.publish_blockers.map((blocker) => (
                <li key={blocker}>{blocker}</li>
              ))}
            </ul>
          </div>
        ) : null}
      </Section>
      {detail ? (
        <>
          <FindingActions
            actionState={actionState}
            onAccept={onAccept}
            onCopyFixPacket={onCopyFixPacket}
            onDismiss={onDismiss}
            onOpenDetail={onOpenDetail}
            onOpenEvidenceMap={onOpenEvidenceMap}
            onOpenFollowUp={onOpenFollowUp}
          />
          <Section title="Finding details">
            <FactsGrid detail={detail} finding={finding} />
          </Section>
          <Section title="Primary code">
            <div className="flex flex-col gap-3">
              <div className="min-w-0">
                <div className="truncate font-mono text-xs">
                  {finding.primary_path || "No primary file"}
                </div>
                <div className="text-muted-foreground mt-1 text-xs">
                  {formatFindingLocation(finding)}
                </div>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <Button size="sm" variant="outline" onClick={onCopyPath}>
                  <CopyIcon data-icon="inline-start" />
                  Copy path
                </Button>
                <Button size="sm" variant="outline" onClick={onOpenEvidenceMap}>
                  <MapIcon data-icon="inline-start" />
                  Map
                </Button>
              </div>
            </div>
          </Section>
          <EvidenceStory
            counterEvidence={counterEvidence}
            detail={detail}
            finding={finding}
            supportingEvidence={supportingEvidence}
            testEvidence={testEvidence}
            verificationLeads={verificationLeads}
          />
          <DraftCommentPanel
            body={draftComment}
            onBodyChange={onDraftCommentChange}
            onCopy={onCopyFixPacket}
            onSave={onSaveDraftComment}
            disabled={actionState.status === "loading"}
          />
        </>
      ) : (
        <Section title="Loading">
          <div className="text-muted-foreground text-sm">
            Loading details...
          </div>
        </Section>
      )}
    </PanelFrame>
  );
}
