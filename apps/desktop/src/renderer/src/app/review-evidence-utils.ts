import type {
  EvidenceItem,
  EvidenceMapCallPath,
  EvidenceMapCallPathStep,
  EvidenceMapNode,
  EvidenceMapPanelEvidenceRef,
  Finding,
  FindingDetailResponse,
  FindingSourceAgent,
  Repository,
} from "@/lib/api";

export function formatDecisionLabel(value: string) {
  const normalized = value === "undecided" ? "needs_triage" : value;
  return normalized
    .replace(/_/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

export function formatConfidence(value: number) {
  return `${Math.round(value * 100)}%`;
}

export function formatShortDate(value: string) {
  const timestamp = Date.parse(value);
  if (Number.isNaN(timestamp)) {
    return value;
  }
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(timestamp);
}

export function formatFindingLocation(finding: Finding) {
  if (!finding.primary_path) {
    return "No primary location";
  }
  if (finding.primary_start_line && finding.primary_end_line) {
    return `${finding.primary_path}:L${finding.primary_start_line}-L${finding.primary_end_line}`;
  }
  if (finding.primary_start_line) {
    return `${finding.primary_path}:L${finding.primary_start_line}`;
  }
  return finding.primary_path;
}

export function findingWorkflowStatusLabel(finding: Finding) {
  if (finding.decision_status) {
    return formatDecisionLabel(finding.decision_status);
  }
  return "Needs Triage";
}

export function sourceAgentSummary(sources: FindingSourceAgent[]) {
  const labels = sources
    .map((source) => {
      const model = source.model_label?.trim();
      const name = source.name?.trim() || source.agent_config_id || "Reviewer";
      return model && !name.toLowerCase().includes(model.toLowerCase())
        ? `${name} · ${model}`
        : name;
    })
    .filter(Boolean);
  if (labels.length <= 2) {
    return labels.join(", ");
  }
  return `${labels.slice(0, 2).join(", ")} +${labels.length - 2}`;
}

export function evidenceItemsOrEmpty(items?: EvidenceItem[] | null) {
  return Array.isArray(items) ? items : [];
}

export function prioritizedEvidenceItems(items: EvidenceItem[]) {
  const rank: Record<string, number> = {
    supporting: 7,
    static_analysis: 6,
    counter: 5,
    missing: 4,
    test: 3,
    search: 2,
    agent: 1,
  };
  return [...items].sort((left, right) => {
    const rankDelta = (rank[right.kind] ?? 0) - (rank[left.kind] ?? 0);
    if (rankDelta !== 0) {
      return rankDelta;
    }
    return right.confidence - left.confidence;
  });
}

export function evidenceBadgeVariant(kind: string) {
  if (kind === "counter" || kind === "missing") {
    return "destructive" as const;
  }
  if (kind === "supporting" || kind === "test" || kind === "static_analysis") {
    return "secondary" as const;
  }
  return "outline" as const;
}

export function evidenceKindDisplayLabel(kind: string) {
  switch (kind) {
    case "counter":
      return "Verified contradiction";
    case "missing":
      return "Missing evidence";
    case "search":
      return "Verification lead";
    case "test":
      return "Test signal";
    case "supporting":
      return "Supporting";
    case "static_analysis":
      return "Code relationship";
    case "agent":
      return "Agent note";
    default:
      return formatDecisionLabel(kind);
  }
}

export function formatEvidenceLocation(item: EvidenceItem) {
  if (!item.path) {
    return item.kind;
  }
  if (item.start_line && item.end_line) {
    return `${item.path}:L${item.start_line}-L${item.end_line}`;
  }
  if (item.start_line) {
    return `${item.path}:L${item.start_line}`;
  }
  return item.path;
}

export function evidenceCodeSnippet(item: EvidenceItem) {
  const direct = item.code_snippet?.trim();
  if (direct) {
    return item.code_snippet ?? "";
  }
  return metadataCodeSnippet(item.metadata);
}

export function evidenceLineWindow(item: EvidenceItem) {
  if (
    item.line_window &&
    item.line_window.start_line > 0 &&
    item.line_window.end_line >= item.line_window.start_line
  ) {
    return item.line_window;
  }
  return metadataLineWindow(item.metadata);
}

export function primaryEvidenceItem(items: EvidenceItem[], finding: Finding) {
  const normalizedPrimaryPath = finding.primary_path?.trim();
  if (!normalizedPrimaryPath) {
    return undefined;
  }
  return prioritizedEvidenceItems(items).find((item) => {
    if (!evidenceCodeSnippet(item)) {
      return false;
    }
    return item.path === normalizedPrimaryPath;
  });
}

export function prioritizedCodeSnippetItems(
  items: EvidenceItem[],
  finding: Finding,
) {
  const primaryPath = finding.primary_path?.trim();
  return prioritizedEvidenceItems(items)
    .filter((item) => evidenceCodeSnippet(item))
    .sort((left, right) => {
      const leftPrimary = primaryPath && left.path === primaryPath ? 1 : 0;
      const rightPrimary = primaryPath && right.path === primaryPath ? 1 : 0;
      if (leftPrimary !== rightPrimary) {
        return rightPrimary - leftPrimary;
      }
      const leftLineDistance =
        finding.primary_start_line && left.start_line
          ? Math.abs(left.start_line - finding.primary_start_line)
          : Number.MAX_SAFE_INTEGER;
      const rightLineDistance =
        finding.primary_start_line && right.start_line
          ? Math.abs(right.start_line - finding.primary_start_line)
          : Number.MAX_SAFE_INTEGER;
      if (leftLineDistance !== rightLineDistance) {
        return leftLineDistance - rightLineDistance;
      }
      return 0;
    });
}

export function snippetPreview(snippet: string, maxLines: number) {
  const lines = snippet.trimEnd().split(/\r?\n/);
  if (lines.length <= maxLines) {
    return lines.join("\n");
  }
  return `${lines.slice(0, maxLines).join("\n")}\n...`;
}

export function firstMeaningfulSnippetLine(snippet: string) {
  const line = snippet
    .split(/\r?\n/)
    .map((candidate) => candidate.replace(/^\s*\d+:\s?/, "").trim())
    .find((candidate) => candidate.length > 0);
  return truncate(line || "", 160);
}

export function formatEvidenceLocationMarkdown(item: EvidenceItem) {
  const location = formatEvidenceLocation(item);
  return location && location !== item.kind ? `(\`${location}\`)` : "";
}

export function metadataObject(
  value: unknown,
): Record<string, unknown> | undefined {
  if (!value) {
    return undefined;
  }
  if (typeof value === "string") {
    try {
      const parsed = JSON.parse(value) as unknown;
      return metadataObject(parsed);
    } catch {
      return undefined;
    }
  }
  if (typeof value === "object" && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  return undefined;
}

export function numberValue(value: unknown) {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string" && value.trim()) {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

function metadataCodeSnippet(metadata: unknown): string {
  const payload = metadataObject(metadata);
  if (!payload) {
    return "";
  }
  const snippet = payload.code_snippet;
  if (typeof snippet === "string" && snippet.trim()) {
    return snippet;
  }
  return metadataCodeSnippet(payload.agent_metadata);
}

function metadataLineWindow(metadata: unknown) {
  const payload = metadataObject(metadata);
  if (!payload) {
    return undefined;
  }
  const lineWindow = metadataObject(payload.line_window);
  const startLine = numberValue(lineWindow?.start_line);
  const endLine = numberValue(lineWindow?.end_line);
  if (startLine > 0 && endLine >= startLine) {
    return { start_line: startLine, end_line: endLine };
  }
  return metadataLineWindow(payload.agent_metadata);
}

export function detailEvidenceGroups(detail?: FindingDetailResponse) {
  const supporting = evidenceItemsOrEmpty(detail?.evidence_groups?.supporting);
  const counter = evidenceItemsOrEmpty(detail?.evidence_groups?.counter);
  const missing = evidenceItemsOrEmpty(detail?.evidence_groups?.missing);
  const test = evidenceItemsOrEmpty(detail?.evidence_groups?.test);
  const search = evidenceItemsOrEmpty(detail?.evidence_groups?.search);
  return {
    supporting,
    counter,
    missing,
    test,
    search,
    verificationLeads: [...counter, ...missing, ...test, ...search],
  };
}

export function evidenceMapOpenTarget(
  node: EvidenceMapNode | undefined,
  callPath: EvidenceMapCallPath | undefined,
  repository: Repository | undefined,
): { filePath: string; line?: number; column?: number } | null {
  const nodePath = node?.deep_link?.path ?? node?.path;
  if (nodePath) {
    return {
      filePath: resolveRepositoryFilePath(repository, nodePath),
      line: node?.deep_link?.start_line ?? node?.start_line,
    };
  }
  const step = callPath?.steps.find((candidate) => Boolean(candidate.path));
  if (!step?.path) {
    return null;
  }
  return {
    filePath: resolveRepositoryFilePath(repository, step.path),
    line: step.start_line,
  };
}

export function resolveRepositoryFilePath(
  repository: Repository | undefined,
  filePath: string,
) {
  if (/^(?:\/|[A-Za-z]:[\\/])/.test(filePath)) {
    return filePath;
  }
  const root = repository?.local_path?.replace(/\/+$/, "");
  if (!root) {
    return filePath;
  }
  return `${root}/${filePath.replace(/^\/+/, "")}`;
}

export function evidenceMapNodePath(node: EvidenceMapNode) {
  return node.deep_link?.path ?? node.path ?? "";
}

export function formatEvidenceNodeLocation(node: EvidenceMapNode) {
  const path = evidenceMapNodePath(node);
  if (!path) {
    return node.kind.replaceAll("_", " ");
  }
  return `${path}${
    node.start_line
      ? `:L${formatLineRange(node.start_line, node.end_line)}`
      : ""
  }`;
}

export function formatCompactEvidenceNodeLocation(
  node: EvidenceMapNode,
  maxSegments = 2,
) {
  const path = evidenceMapNodePath(node);
  if (!path) {
    return node.kind.replaceAll("_", " ");
  }
  return `${compactPathForDisplay(path, maxSegments)}${
    node.start_line
      ? `:L${formatLineRange(node.start_line, node.end_line)}`
      : ""
  }`;
}

export function formatEvidenceRefLocation(item: EvidenceMapPanelEvidenceRef) {
  if (!item.path) {
    return item.kind;
  }
  return `${item.path}${
    item.start_line
      ? `:L${formatLineRange(item.start_line, item.end_line)}`
      : ""
  }`;
}

export function formatCompactEvidenceRefLocation(
  item: EvidenceMapPanelEvidenceRef,
  maxSegments = 2,
) {
  if (!item.path) {
    return item.kind;
  }
  return `${compactPathForDisplay(item.path, maxSegments)}${
    item.start_line
      ? `:L${formatLineRange(item.start_line, item.end_line)}`
      : ""
  }`;
}

export function compactEvidencePanelTitle(item: EvidenceMapPanelEvidenceRef) {
  if (!item.path) {
    return item.title;
  }
  const compactLocation = formatCompactEvidenceRefLocation(item, 2);
  const location = formatEvidenceRefLocation(item);
  if (item.title.includes(location)) {
    return item.title.replace(location, compactLocation);
  }
  if (item.title.includes(item.path)) {
    return item.title.replace(item.path, compactPathForDisplay(item.path, 2));
  }
  return item.title;
}

export function matchingEvidenceRefsForNode(
  node: EvidenceMapNode | undefined,
  evidence: EvidenceMapPanelEvidenceRef[],
) {
  if (!node) {
    return [];
  }
  const nodePath = evidenceMapNodePath(node);
  const nodeEvidenceID = node.evidence_item_id;
  return evidence.filter((item) => {
    if (nodeEvidenceID && item.id === nodeEvidenceID) {
      return true;
    }
    if (!nodePath || item.path !== nodePath) {
      return false;
    }
    if (!node.start_line || !item.start_line) {
      return true;
    }
    const itemEnd = item.end_line ?? item.start_line;
    const nodeEnd = node.end_line ?? node.start_line;
    return item.start_line <= nodeEnd && itemEnd >= node.start_line;
  });
}

export function formatCallPathStepLocation(step: EvidenceMapCallPathStep) {
  if (!step.path) {
    return "No file location";
  }
  return `${step.path}${
    step.start_line
      ? `:L${formatLineRange(step.start_line, step.end_line)}`
      : ""
  }`;
}

export function formatLineRange(startLine: number, endLine?: number) {
  if (endLine && endLine !== startLine) {
    return `${startLine}-L${endLine}`;
  }
  return String(startLine);
}

export function shortPath(path: string) {
  const parts = path.split("/");
  if (parts.length <= 3) {
    return path;
  }
  return `${parts.at(-3)}/${parts.at(-2)}/${parts.at(-1)}`;
}

export function compactPathForDisplay(path: string, maxSegments = 2) {
  const parts = path.split(/[\\/]/).filter(Boolean);
  if (parts.length <= maxSegments) {
    return path;
  }
  return `.../${parts.slice(-maxSegments).join("/")}`;
}

export function pathLeafForDisplay(path: string) {
  const parts = path.split(/[\\/]/).filter(Boolean);
  return parts.at(-1) ?? path;
}

export function truncate(value: string, maxLength: number) {
  if (value.length <= maxLength) {
    return value;
  }
  return `${value.slice(0, Math.max(0, maxLength - 3))}...`;
}
