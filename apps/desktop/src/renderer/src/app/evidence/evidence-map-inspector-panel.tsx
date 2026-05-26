import { useEffect, useRef } from "react";

import { Badge } from "@/components/ui/badge";
import { languageForFilePath } from "@/lib/syntax-highlighting";
import type {
  EvidenceMapCallPath,
  EvidenceMapEdge,
  EvidenceMapNode,
  EvidenceMapResponse,
} from "@/lib/api";

import { SyntaxCodeBlock } from "../shared/markdown-message";
import {
  compactEvidencePanelTitle,
  evidenceBadgeVariant,
  evidenceKindDisplayLabel,
  formatCompactEvidenceNodeLocation,
  formatCompactEvidenceRefLocation,
  formatConfidence,
  formatDecisionLabel,
  matchingEvidenceRefsForNode,
  pathLeafForDisplay,
  shortPath,
} from "./review-evidence-utils";
import { PanelFrame, PanelMarkdown, Section } from "./review-panel-shell";

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
      <div className="border-border-subtle bg-card max-w-full min-w-0 overflow-hidden rounded-lg border shadow-xs">
        <div className="border-border-subtle bg-surface flex min-w-0 flex-wrap items-center justify-between gap-2 border-b px-3 py-2">
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
              className="border-border-subtle bg-card rounded-lg border p-3"
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
