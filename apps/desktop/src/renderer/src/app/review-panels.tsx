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
import { useEffect, useRef, type ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Textarea } from "@/components/ui/textarea";
import { languageForFilePath } from "@/lib/syntax-highlighting";
import type {
  EvidenceItem,
  EvidenceMapCallPath,
  EvidenceMapEdge,
  EvidenceMapNode,
  EvidenceMapResponse,
  Finding,
  FindingDetailResponse,
} from "@/lib/api";
import {
  compactEvidencePanelTitle,
  evidenceBadgeVariant,
  evidenceCodeSnippet,
  evidenceItemsOrEmpty,
  evidenceKindDisplayLabel,
  formatCompactEvidenceNodeLocation,
  formatCompactEvidenceRefLocation,
  formatConfidence,
  formatDecisionLabel,
  formatEvidenceLocation,
  formatFindingLocation,
  formatShortDate,
  firstMeaningfulSnippetLine,
  matchingEvidenceRefsForNode,
  pathLeafForDisplay,
  shortPath,
} from "./review-evidence-utils";
import { MarkdownMessage, SyntaxCodeBlock } from "./markdown-message";
import { cn } from "@/lib/utils";

type LoadingState = "idle" | "loading" | "success" | "error";

function PanelFrame({
  actions,
  children,
  className,
  eyebrow,
  title,
  subtitle,
}: {
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  eyebrow?: string;
  title: string;
  subtitle?: string;
}) {
  return (
    <aside
      data-review-panel="true"
      className={cn(
        "border-border/70 flex min-h-0 min-w-0 flex-col overflow-hidden rounded-xl border bg-white shadow-[0_1px_2px_rgb(17_18_20/0.03)]",
        className,
      )}
    >
      <div className="min-w-0 border-b bg-white px-4 py-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            {eyebrow && (
              <div className="text-muted-foreground text-xs font-medium">
                {eyebrow}
              </div>
            )}
            <div className="text-base font-semibold [overflow-wrap:anywhere] break-words">
              {title}
            </div>
            {subtitle && (
              <p className="text-muted-foreground mt-1 text-xs leading-5 [overflow-wrap:anywhere] break-words">
                {subtitle}
              </p>
            )}
          </div>
          {actions && (
            <div className="flex shrink-0 items-center gap-2">{actions}</div>
          )}
        </div>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="flex w-full max-w-full min-w-0 flex-col gap-4 overflow-x-hidden p-4">
          {children}
        </div>
      </ScrollArea>
    </aside>
  );
}

function Section({
  title,
  children,
  description,
}: {
  title: string;
  children: ReactNode;
  description?: string;
}) {
  return (
    <section className="border-border/70 max-w-full min-w-0 overflow-hidden rounded-lg border bg-white">
      <div className="px-3 py-2.5">
        <div className="text-sm font-semibold">{title}</div>
        {description && (
          <p className="text-muted-foreground mt-1 text-xs leading-5 [overflow-wrap:anywhere] break-words">
            {description}
          </p>
        )}
      </div>
      <div className="max-w-full min-w-0 border-t px-3 py-3">{children}</div>
    </section>
  );
}

function PanelMarkdown({
  children,
  className,
}: {
  children?: string;
  className?: string;
}) {
  if (!children?.trim()) {
    return null;
  }
  return (
    <MarkdownMessage
      className={cn(
        "text-muted-foreground text-sm leading-6 [overflow-wrap:anywhere] break-words",
        className,
      )}
      content={children}
    />
  );
}

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
          className="min-w-0 justify-start overflow-hidden"
          disabled={disabled}
          size="sm"
          onClick={onAccept}
        >
          <CheckIcon data-icon="inline-start" />
          Accept
        </Button>
        <Button
          className="min-w-0 justify-start overflow-hidden"
          disabled={disabled}
          size="sm"
          variant="outline"
          onClick={onDismiss}
        >
          <MinusIcon data-icon="inline-start" />
          Dismiss
        </Button>
      </div>
      <div className="mt-2 grid min-w-0 grid-cols-1 gap-2 sm:grid-cols-2">
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
              finding.severity === "high" || finding.severity === "blocker"
                ? "destructive"
                : "secondary"
            }
          >
            {finding.severity}
          </Badge>
          <Badge variant="outline">
            {formatDecisionLabel(finding.verification_status)}
          </Badge>
          <Badge variant="secondary">
            {formatDecisionLabel(finding.decision_status)}
          </Badge>
        </div>
        <PanelMarkdown className="mt-3">
          {finding.evidence_summary}
        </PanelMarkdown>
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

type SourcePreview = {
  codeSnippet?: string;
  endLine?: number;
  fileContent?: string;
  fileLineCount?: number;
  fileTruncated?: boolean;
  path?: string;
  startLine?: number;
  windowEndLine?: number;
  windowStartLine?: number;
};

function SourcePreviewPanel({ preview }: { preview: SourcePreview | null }) {
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const content = preview?.fileContent?.trim()
    ? preview.fileContent
    : preview?.codeSnippet;
  const hasFullFile = Boolean(preview?.fileContent?.trim());
  const targetStartLine = preview?.startLine ?? preview?.windowStartLine ?? 1;
  const targetEndLine = preview?.endLine ?? targetStartLine;
  const renderedStartLine = hasFullFile
    ? 1
    : (preview?.windowStartLine ?? targetStartLine);

  useEffect(() => {
    if (!preview?.path || !content?.trim()) {
      return;
    }
    const container = scrollRef.current;
    const line = container?.querySelector<HTMLElement>(
      `[data-line-number="${targetStartLine}"]`,
    );
    if (!container || !line) {
      return;
    }
    const containerRect = container.getBoundingClientRect();
    const lineRect = line.getBoundingClientRect();
    const delta = lineRect.top - containerRect.top;
    const nextTop = container.scrollTop + delta - container.clientHeight * 0.12;
    container.scrollTop = Math.max(0, nextTop);
  }, [content, preview?.path, targetStartLine]);

  if (!preview?.path || !content?.trim()) {
    return (
      <Section title="Source file">
        <div className="text-muted-foreground text-sm leading-6">
          Select a graph node with an available file to inspect source inline.
        </div>
      </Section>
    );
  }

  return (
    <Section title="Source file">
      <div className="max-w-full min-w-0 overflow-hidden rounded-lg border bg-white">
        <div className="flex min-w-0 flex-wrap items-center justify-between gap-2 border-b bg-[#fbfbfa] px-3 py-2">
          <span
            className="min-w-0 truncate font-mono text-xs"
            title={preview.path}
          >
            {preview.path}
          </span>
          <div className="flex shrink-0 flex-wrap items-center justify-end gap-1.5">
            <Badge variant="secondary">
              L{targetStartLine}
              {targetEndLine && targetEndLine !== targetStartLine
                ? `-L${targetEndLine}`
                : ""}
            </Badge>
            <Badge variant="outline">
              {hasFullFile
                ? preview.fileTruncated
                  ? "file preview"
                  : "full file"
                : "snippet"}
            </Badge>
            {hasFullFile && preview.fileLineCount ? (
              <Badge variant="outline">{preview.fileLineCount} lines</Badge>
            ) : null}
          </div>
        </div>
        <div
          className="max-h-[520px] max-w-full min-w-0 overflow-auto [scrollbar-gutter:stable_both-edges] [scrollbar-width:thin]"
          ref={scrollRef}
        >
          <SyntaxCodeBlock
            className="rounded-none border-0 bg-white"
            code={content}
            highlightEndLine={targetEndLine}
            highlightStartLine={targetStartLine}
            language={languageForFilePath(preview.path)}
            lineNumbers
            startLine={renderedStartLine}
          />
        </div>
      </div>
    </Section>
  );
}

function sourcePreviewForSelection(
  selectedNode: EvidenceMapNode | undefined,
  selectedEvidence: ReturnType<typeof matchingEvidenceRefsForNode>,
  selectedCallPath: EvidenceMapCallPath | undefined,
): SourcePreview | null {
  if (selectedNode) {
    const path = selectedNode.deep_link?.path ?? selectedNode.path;
    if (path && (selectedNode.file_content || selectedNode.code_snippet)) {
      return {
        codeSnippet: selectedNode.code_snippet,
        endLine:
          selectedNode.deep_link?.end_line ??
          selectedNode.end_line ??
          selectedNode.start_line,
        fileContent: selectedNode.file_content,
        fileLineCount: selectedNode.file_line_count,
        fileTruncated: selectedNode.file_truncated,
        path,
        startLine:
          selectedNode.deep_link?.start_line ?? selectedNode.start_line,
        windowEndLine: selectedNode.line_window?.end_line,
        windowStartLine: selectedNode.line_window?.start_line,
      };
    }
  }

  const evidence = selectedEvidence.find(
    (item) => item.file_content?.trim() || item.code_snippet?.trim(),
  );
  if (evidence?.path) {
    return {
      codeSnippet: evidence.code_snippet,
      endLine: evidence.end_line ?? evidence.start_line,
      fileContent: evidence.file_content,
      fileLineCount: evidence.file_line_count,
      fileTruncated: evidence.file_truncated,
      path: evidence.path,
      startLine: evidence.start_line,
      windowEndLine: evidence.line_window?.end_line,
      windowStartLine: evidence.line_window?.start_line,
    };
  }

  const step = selectedCallPath?.steps.find(
    (candidate) => candidate.path && candidate.start_line,
  );
  if (step?.path) {
    return {
      endLine: step.end_line ?? step.start_line,
      path: step.path,
      startLine: step.start_line,
    };
  }
  return null;
}

export function EvidenceMapInspectorPanel({
  activeRepositoryPath,
  map,
  selectedCallPath,
  selectedEdge,
  selectedNode,
}: {
  activeRepositoryPath?: string;
  map: EvidenceMapResponse;
  selectedCallPath?: EvidenceMapCallPath;
  selectedEdge?: EvidenceMapEdge;
  selectedNode?: EvidenceMapNode;
}) {
  const selectedEvidence = matchingEvidenceRefsForNode(
    selectedNode,
    map.panel.evidence,
  );
  const selectedLocation = selectedNode
    ? formatCompactEvidenceNodeLocation(selectedNode, 2)
    : selectedCallPath?.steps.find((step) => step.path)
      ? (selectedCallPath.steps.find((step) => step.path)?.path ?? "")
      : "";
  const sourcePreview = sourcePreviewForSelection(
    selectedNode,
    selectedEvidence,
    selectedCallPath,
  );

  return (
    <PanelFrame
      subtitle={map.panel.claim}
      title="Source details"
      eyebrow="Evidence map"
    >
      <Section title="Why this matters">
        <PanelMarkdown>
          {map.panel.evidence_summary ||
            map.panel.connection_summary ||
            map.graph.summary ||
            `Evidence map for "${map.panel.claim}" with ${map.nodes.length} node(s) and ${map.edges.length} edge(s).`}
        </PanelMarkdown>
      </Section>
      {selectedNode ? (
        <Section title="Selected location">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="text-sm font-semibold [overflow-wrap:anywhere] break-words">
                {selectedNode.label ||
                  pathLeafForDisplay(
                    selectedNode.path || selectedNode.deep_link?.path || "node",
                  )}
              </div>
              <div className="text-muted-foreground mt-1 font-mono text-xs break-all">
                {selectedLocation || selectedNode.kind.replaceAll("_", " ")}
              </div>
              {selectedNode.explanation ? (
                <PanelMarkdown className="mt-2">
                  {selectedNode.explanation}
                </PanelMarkdown>
              ) : null}
            </div>
            <div className="flex max-w-full min-w-0 flex-wrap items-center justify-end gap-2">
              <Badge variant="outline">
                {selectedNode.kind.replaceAll("_", " ")}
              </Badge>
              <Badge variant="secondary">
                {formatConfidence(selectedNode.confidence)}
              </Badge>
            </div>
          </div>
        </Section>
      ) : null}
      <SourcePreviewPanel preview={sourcePreview} />
      {selectedEdge ? (
        <Section title="Connection">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="text-sm font-semibold [overflow-wrap:anywhere] break-words">
                {selectedEdge.label || selectedEdge.kind.replaceAll("_", " ")}
              </div>
              <p className="text-muted-foreground mt-1 text-xs break-all">
                {selectedEdge.source} {"->"} {selectedEdge.target}
              </p>
              {selectedEdge.explanation ? (
                <PanelMarkdown className="mt-2">
                  {selectedEdge.explanation}
                </PanelMarkdown>
              ) : null}
            </div>
            <Badge
              variant={
                selectedEdge.status === "missing" ? "destructive" : "outline"
              }
            >
              {selectedEdge.status}
            </Badge>
          </div>
        </Section>
      ) : null}
      {selectedCallPath ? (
        <Section title="Call path">
          <div className="text-sm font-medium">
            {selectedCallPath.label || "Evidence path"}
          </div>
          {selectedCallPath.summary ? (
            <PanelMarkdown className="mt-2">
              {selectedCallPath.summary}
            </PanelMarkdown>
          ) : null}
          <div className="text-muted-foreground mt-2 text-xs">
            {selectedCallPath.steps.length} step
            {selectedCallPath.steps.length === 1 ? "" : "s"}
          </div>
        </Section>
      ) : null}
      <Section title="Related evidence">
        <div className="flex flex-col gap-2">
          {selectedEvidence.slice(0, 4).map((item) => (
            <div
              key={item.id}
              className="border-border/70 rounded-lg border bg-white p-3"
            >
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <div className="line-clamp-2 text-sm font-medium [overflow-wrap:anywhere] break-words">
                    {compactEvidencePanelTitle(item)}
                  </div>
                  <div className="text-muted-foreground mt-1 text-xs break-all">
                    {formatCompactEvidenceRefLocation(item, 2)}
                  </div>
                </div>
                <Badge variant={evidenceBadgeVariant(item.kind)}>
                  {evidenceKindDisplayLabel(item.kind)}
                </Badge>
              </div>
              <PanelMarkdown className="mt-2 line-clamp-3">
                {item.summary}
              </PanelMarkdown>
            </div>
          ))}
          {selectedEvidence.length === 0 ? (
            <div className="text-muted-foreground text-sm">
              No direct evidence is attached to this selection.
            </div>
          ) : null}
        </div>
      </Section>
      <Section title="Finding summary">
        <div className="flex flex-wrap gap-2">
          <Badge variant="default">
            {formatDecisionLabel(map.finding.severity)}
          </Badge>
          <Badge variant="secondary">
            {formatDecisionLabel(map.finding.verification_status)}
          </Badge>
          <Badge variant="outline">
            {formatConfidence(map.finding.confidence)}
          </Badge>
        </div>
        <PanelMarkdown className="mt-3">
          {map.panel.suggested_fix ||
            map.finding.suggested_fix ||
            "Review the selected path and decide whether the observed code actually refutes the claim."}
        </PanelMarkdown>
      </Section>
      {activeRepositoryPath ? (
        <div className="text-muted-foreground text-xs">
          Repository: {shortPath(activeRepositoryPath)}
        </div>
      ) : null}
    </PanelFrame>
  );
}
