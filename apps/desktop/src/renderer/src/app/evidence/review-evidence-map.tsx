import { MapIcon } from "lucide-react";
import { useId, useMemo } from "react";

import { EmptyState } from "@/components/app/chrome";
import { cn } from "@/lib/utils";
import type {
  EvidenceMapEdge,
  EvidenceMapNode,
  EvidenceMapResponse,
} from "@/lib/api";
import {
  evidenceMapNodePath,
  formatCompactEvidenceNodeLocation,
  formatEvidenceNodeLocation,
  truncate,
} from "./review-evidence-utils";

const NODE_WIDTH = 190;
const NODE_HEIGHT = 96;
const COLUMN_GAP = 210;
const LEFT_PADDING = 48;
const ROW_GAP = 126;

export type EvidenceMapSelection =
  | { kind: "node"; id: string }
  | { kind: "edge"; id: string }
  | { kind: "call_path"; id: string };

interface PositionedEvidenceMapNode {
  node: EvidenceMapNode;
  x: number;
  y: number;
}

interface EvidenceMapLayout {
  nodes: PositionedEvidenceMapNode[];
  nodeById: Map<string, PositionedEvidenceMapNode>;
  width: number;
  height: number;
}

export function firstEvidenceMapSelection(
  map: EvidenceMapResponse,
): EvidenceMapSelection | null {
  const callPathNode = map.call_path.find((step) => step.node_id)?.node_id;
  const primaryNode =
    map.nodes.find((node) => node.kind === "changed_code")?.id ??
    callPathNode ??
    map.nodes[0]?.id;
  if (primaryNode) {
    return { kind: "node", id: primaryNode };
  }
  if (map.edges[0]) {
    return { kind: "edge", id: map.edges[0].id };
  }
  if (map.call_paths[0]) {
    return { kind: "call_path", id: map.call_paths[0].id };
  }
  return null;
}

export function EvidenceMapGraphCanvas({
  map,
  onSelect,
  selection,
}: {
  map: EvidenceMapResponse;
  onSelect: (selection: EvidenceMapSelection) => void;
  selection: EvidenceMapSelection | null;
}) {
  const graphId = useId().replaceAll(":", "");
  const arrowMarkerId = `${graphId}-evidence-map-arrow`;
  const nodeShadowId = `${graphId}-evidence-map-node-shadow`;
  const focusedMap = useMemo(() => focusedEvidenceMap(map), [map]);
  const layout = useMemo(
    () => buildEvidenceMapLayout(focusedMap),
    [focusedMap],
  );

  if (focusedMap.nodes.length === 0) {
    return (
      <div className="bg-surface/30 flex min-h-[360px] min-w-0 flex-1 items-center justify-center">
        <EmptyState
          className="border-0"
          title="No graph nodes"
          description="The graph builder did not return renderable nodes for this finding."
          icon={MapIcon}
        />
      </div>
    );
  }

  return (
    <div className="evidence-map-canvas h-full min-h-[560px] min-w-0 flex-1 overflow-auto [scrollbar-width:thin]">
      <svg
        aria-label="Evidence Map graph"
        className="block min-h-[420px]"
        height={layout.height}
        role="img"
        viewBox={`0 0 ${layout.width} ${layout.height}`}
        width={layout.width}
      >
        <defs>
          <filter
            id={nodeShadowId}
            x="-12%"
            y="-24%"
            width="124%"
            height="148%"
          >
            <feDropShadow
              dx="0"
              dy="8"
              floodColor="oklch(0.2 0.02 255)"
              floodOpacity="0.08"
              stdDeviation="7"
            />
          </filter>
          <marker
            id={arrowMarkerId}
            markerHeight="7"
            markerWidth="7"
            orient="auto"
            refX="6"
            refY="3.5"
          >
            <path d="M0,0 L7,3.5 L0,7 Z" className="fill-muted-foreground" />
          </marker>
        </defs>
        {[
          { label: "Issue line", x: LEFT_PADDING },
          { label: "Finding", x: LEFT_PADDING + COLUMN_GAP },
          { label: "Verification", x: LEFT_PADDING + COLUMN_GAP * 2 },
        ].map((heading) => (
          <text
            key={heading.label}
            className="fill-muted-foreground text-[11px] font-semibold"
            x={heading.x}
            y="28"
          >
            {heading.label}
          </text>
        ))}

        {focusedMap.edges.map((edge) => {
          const source = layout.nodeById.get(edge.source);
          const target = layout.nodeById.get(edge.target);
          if (!source || !target) {
            return null;
          }
          const selected =
            selection?.kind === "edge" && selection.id === edge.id;
          const sameColumn = Math.abs(source.x - target.x) < 12;
          const targetIsBelow = target.y >= source.y;
          const targetIsRight = target.x >= source.x;
          const sourceX = sameColumn
            ? source.x + NODE_WIDTH / 2
            : targetIsRight
              ? source.x + NODE_WIDTH
              : source.x;
          const sourceY = sameColumn
            ? targetIsBelow
              ? source.y + NODE_HEIGHT
              : source.y
            : source.y + NODE_HEIGHT / 2;
          const targetX = sameColumn
            ? target.x + NODE_WIDTH / 2
            : targetIsRight
              ? target.x
              : target.x + NODE_WIDTH;
          const targetY = sameColumn
            ? targetIsBelow
              ? target.y
              : target.y + NODE_HEIGHT
            : target.y + NODE_HEIGHT / 2;
          const control = Math.max(76, Math.abs(targetX - sourceX) / 2);
          const path = sameColumn
            ? `M ${sourceX} ${sourceY} L ${targetX} ${targetY}`
            : `M ${sourceX} ${sourceY} C ${
                sourceX + (targetIsRight ? control : -control)
              } ${sourceY}, ${
                targetX + (targetIsRight ? -control : control)
              } ${targetY}, ${targetX} ${targetY}`;
          return (
            <g
              key={edge.id}
              className="cursor-pointer"
              role="button"
              tabIndex={0}
              onClick={() => onSelect({ kind: "edge", id: edge.id })}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  onSelect({ kind: "edge", id: edge.id });
                }
              }}
            >
              <path
                className={cn(
                  "stroke-muted-foreground/45 fill-none",
                  selected ? "stroke-primary" : "",
                  edge.status === "missing" ? "stroke-destructive" : "",
                )}
                d={path}
                markerEnd={`url(#${arrowMarkerId})`}
                strokeDasharray={edge.status === "missing" ? "7 5" : undefined}
                strokeWidth={selected ? 2.6 : 1.6}
              />
              {edge.status === "missing" ? (
                <text
                  className="fill-destructive text-[22px] font-semibold"
                  x={(sourceX + targetX) / 2}
                  y={(sourceY + targetY) / 2 + 7}
                >
                  x
                </text>
              ) : null}
            </g>
          );
        })}

        {layout.nodes.map(({ node, x, y }) => {
          const selected =
            selection?.kind === "node" && selection.id === node.id;
          const label = evidenceMapGraphNodeTitle(node);
          const labelLines = wrapSvgLabel(label, 22).slice(0, 3);
          const location = formatEvidenceNodeLocation(node);
          const title = `${evidenceMapReadableNodeLabel(node)}${
            location ? `\n${location}` : ""
          }`;
          const style = evidenceMapNodeStyle(node.kind);
          return (
            <g
              key={node.id}
              className="cursor-pointer"
              role="button"
              tabIndex={0}
              transform={`translate(${x}, ${y})`}
              onClick={() => onSelect({ kind: "node", id: node.id })}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  onSelect({ kind: "node", id: node.id });
                }
              }}
            >
              <title>{title}</title>
              <rect
                className={cn(style.surface, selected ? style.selected : "")}
                height={NODE_HEIGHT}
                filter={`url(#${nodeShadowId})`}
                rx="8"
                strokeWidth={selected ? 2.4 : 1.2}
                width={NODE_WIDTH}
              />
              <rect
                className={style.bar}
                height="4"
                rx="1.5"
                width={NODE_WIDTH - 22}
                x="11"
                y="9"
              />
              <text className="fill-foreground text-[12px] font-semibold">
                {labelLines.map((line, index) => (
                  <tspan key={`${node.id}:${index}`} x="12" y={30 + index * 15}>
                    {line}
                  </tspan>
                ))}
              </text>
              <text className="fill-muted-foreground text-[10px]">
                <tspan x="12" y={NODE_HEIGHT - 14}>
                  {truncate(evidenceMapGraphNodeMeta(node), 22)}
                </tspan>
                <tspan x={NODE_WIDTH - 54} y={NODE_HEIGHT - 14}>
                  {Math.round(node.confidence * 100)}%
                </tspan>
              </text>
            </g>
          );
        })}
      </svg>
    </div>
  );
}

function focusedEvidenceMap(map: EvidenceMapResponse): EvidenceMapResponse {
  const story = evidenceMapNarrativeStory(map);
  const claimNode = evidenceMapClaimNode(map);
  const focusedNodes = dedupeEvidenceMapNodes([
    ...story.changed.slice(0, 2),
    claimNode,
    ...story.checks,
  ]);
  return {
    ...map,
    nodes: focusedNodes,
    edges: synthesizeFocusedEvidenceEdges(focusedNodes, map.edges),
  };
}

function evidenceMapNarrativeStory(map: EvidenceMapResponse) {
  const nodes = dedupeEvidenceMapNodes(map.nodes);
  const primaryPath = map.finding.primary_path;
  const changedKinds = new Set([
    "changed_code",
    "handler",
    "route",
    "entrypoint",
  ]);
  const changed = nodes
    .filter(
      (node) =>
        changedKinds.has(node.kind) ||
        Boolean(primaryPath && evidenceMapNodePath(node) === primaryPath),
    )
    .sort(
      (left, right) =>
        evidenceMapChangedNodeRank(left, primaryPath) -
          evidenceMapChangedNodeRank(right, primaryPath) ||
        right.confidence - left.confidence,
    )
    .slice(0, 3);
  const focusedChanged =
    changed.length > 0 ? changed : nodes.length > 0 ? [nodes[0]] : [];
  const changedIDs = new Set(focusedChanged.map((node) => node.id));
  return {
    changed: focusedChanged,
    checks: selectEvidenceMapChecks(
      nodes.filter((node) => !changedIDs.has(node.id)),
    ),
  };
}

function selectEvidenceMapChecks(nodes: EvidenceMapNode[]) {
  const ranked = [...nodes].sort(
    (left, right) =>
      evidenceMapCheckNodeRank(left.kind) -
        evidenceMapCheckNodeRank(right.kind) ||
      right.confidence - left.confidence,
  );
  const selected: EvidenceMapNode[] = [];
  const seenGroups = new Set<string>();
  for (const node of ranked) {
    const group = [
      evidenceMapCheckGroup(node.kind),
      evidenceMapNodePath(node),
    ].join(":");
    if (seenGroups.has(group)) {
      continue;
    }
    selected.push(node);
    seenGroups.add(group);
    if (selected.length >= 3) {
      break;
    }
  }
  if (selected.length < 3) {
    for (const node of ranked) {
      if (selected.some((item) => item.id === node.id)) {
        continue;
      }
      selected.push(node);
      if (selected.length >= 3) {
        break;
      }
    }
  }
  return selected;
}

function evidenceMapCheckGroup(kind: string) {
  switch (kind) {
    case "missing_guard":
      return "missing_guard";
    case "test":
      return "test";
    case "counter_evidence":
      return "counter";
    case "static_analysis":
      return "static";
    default:
      return kind;
  }
}

function evidenceMapClaimNode(map: EvidenceMapResponse): EvidenceMapNode {
  return {
    id: `finding_claim_${map.finding.id}`,
    kind: "finding_claim",
    label: map.finding.canonical_claim,
    confidence: map.finding.confidence,
    metadata: { synthetic: true, finding_id: map.finding.id },
  };
}

function synthesizeFocusedEvidenceEdges(
  nodes: EvidenceMapNode[],
  sourceEdges: EvidenceMapEdge[],
) {
  if (nodes.length <= 1) {
    return [];
  }
  const nodeIDs = new Set(nodes.map((node) => node.id));
  const existingKeys = new Set<string>();
  const claim = nodes.find((node) => node.kind === "finding_claim");
  const changedNodes = nodes.filter((node) =>
    ["entrypoint", "route", "handler", "changed_code"].includes(node.kind),
  );
  const edges: EvidenceMapEdge[] = sourceEdges.filter((edge) => {
    if (!nodeIDs.has(edge.source) || !nodeIDs.has(edge.target)) {
      return false;
    }
    const key = `${edge.source}->${edge.target}`;
    existingKeys.add(key);
    return true;
  });

  for (let index = 1; index < changedNodes.length; index += 1) {
    const source = changedNodes[index - 1];
    const target = changedNodes[index];
    const key = `${source.id}->${target.id}`;
    if (existingKeys.has(key)) {
      continue;
    }
    edges.push(syntheticEvidenceEdge(source.id, target.id, "reachable path"));
    existingKeys.add(key);
  }

  if (!claim) {
    return edges;
  }
  for (const source of changedNodes.slice(-1)) {
    const key = `${source.id}->${claim.id}`;
    if (!existingKeys.has(key)) {
      edges.push(syntheticEvidenceEdge(source.id, claim.id, "grounds claim"));
      existingKeys.add(key);
    }
  }
  for (const node of nodes) {
    if (node.id === claim.id || changedNodes.includes(node)) {
      continue;
    }
    const key = `${claim.id}->${node.id}`;
    if (existingKeys.has(key)) {
      continue;
    }
    edges.push(
      syntheticEvidenceEdge(
        claim.id,
        node.id,
        evidenceMapFocusedEdgeLabel(node),
        node.kind === "missing_guard" ? "missing" : "supported",
      ),
    );
    existingKeys.add(key);
  }
  return edges;
}

function syntheticEvidenceEdge(
  source: string,
  target: string,
  label: string,
  status = "supported",
): EvidenceMapEdge {
  return {
    id: `focus_${source}_${target}_${label.replace(/\W+/g, "_")}`,
    source,
    target,
    kind: status === "missing" ? "missing_guard" : "evidence_flow",
    status,
    label,
    confidence: 0.75,
    metadata: { synthetic: true },
  };
}

function evidenceMapFocusedEdgeLabel(node: EvidenceMapNode) {
  switch (node.kind) {
    case "missing_guard":
      return "missing guard";
    case "test":
      return "test signal";
    case "counter_evidence":
      return "contradiction";
    case "static_analysis":
      return "code relationship";
    case "guard":
    case "middleware":
    case "config":
      return "verification lead";
    default:
      return "related check";
  }
}

function dedupeEvidenceMapNodes(nodes: EvidenceMapNode[]) {
  const byKey = new Map<string, EvidenceMapNode>();
  for (const node of nodes) {
    const key = [
      node.kind,
      evidenceMapNodePath(node),
      node.start_line ?? node.deep_link?.start_line ?? "",
      evidenceMapReadableNodeLabel(node).toLowerCase(),
    ].join(":");
    const existing = byKey.get(key);
    if (!existing || node.confidence > existing.confidence) {
      byKey.set(key, node);
    }
  }
  return [...byKey.values()];
}

function evidenceMapGraphNodeTitle(node: EvidenceMapNode) {
  const label = evidenceMapReadableNodeLabel(node);
  const path = evidenceMapNodePath(node);
  if (path && evidenceMapLabelLooksLikeLocation(label, path)) {
    return formatCompactEvidenceNodeLocation(node, 1);
  }
  return label;
}

function evidenceMapGraphNodeMeta(node: EvidenceMapNode) {
  const path = evidenceMapNodePath(node);
  if (path) {
    return formatCompactEvidenceNodeLocation(node, 2);
  }
  return evidenceMapNodeMeta(node);
}

function evidenceMapLabelLooksLikeLocation(label: string, path: string) {
  return (
    label.includes(path) ||
    label.includes("/") ||
    /(?:^|\s)[\w.-]+\.[A-Za-z0-9]+(?::L?\d+)?/.test(label)
  );
}

function evidenceMapReadableNodeLabel(node: EvidenceMapNode) {
  const label = evidenceMapNodeLabel(node).trim();
  if (looksLikeStructuredEnvelopeLabel(label)) {
    return node.kind.replaceAll("_", " ");
  }
  if (/^potential counter-evidence at\b/i.test(label)) {
    if (node.kind === "test") {
      return "Related test check";
    }
    if (node.kind === "counter_evidence") {
      return "Verified contradiction";
    }
    return "Verification lead";
  }
  if (/^counter-evidence/i.test(label)) {
    return "Verified contradiction";
  }
  if (/^verification lead at\b/i.test(label)) {
    return "Verification lead";
  }
  if (/^related test signal at\b/i.test(label)) {
    return "Related test check";
  }
  if (/^missing guard/i.test(label)) {
    return "Missing guard";
  }
  if (node.kind === "missing_guard" && label) {
    return `Missing guard: ${label}`;
  }
  return label || node.kind.replaceAll("_", " ");
}

function looksLikeStructuredEnvelopeLabel(label: string) {
  const trimmed = label.trim();
  if (!trimmed) {
    return false;
  }
  if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
    return true;
  }
  return (
    trimmed.includes('"type"') ||
    trimmed.includes('"event"') ||
    trimmed.includes('"hook_name"') ||
    trimmed.includes('"hook_event"') ||
    trimmed.includes('"session_id"')
  );
}

function evidenceMapChangedNodeRank(
  node: EvidenceMapNode,
  primaryPath: string | undefined,
) {
  const relation = evidenceMapNodeRelationship(node);
  if (relation === "caller" || relation === "entrypoint") {
    return 0;
  }
  const nodePath = evidenceMapNodePath(node);
  if (primaryPath && nodePath === primaryPath) {
    return 1;
  }
  switch (node.kind) {
    case "changed_code":
      return 2;
    case "handler":
      return 3;
    case "route":
      return 4;
    case "entrypoint":
      return 5;
    default:
      return 9;
  }
}

function evidenceMapNodeRelationship(node: EvidenceMapNode) {
  if (!node.metadata || typeof node.metadata !== "object") {
    return "";
  }
  const metadata = node.metadata as Record<string, unknown>;
  const relationship =
    typeof metadata.relationship === "string" ? metadata.relationship : "";
  const direction =
    typeof metadata.direction === "string" ? metadata.direction : "";
  return relationship || direction;
}

function evidenceMapCheckNodeRank(kind: string) {
  switch (kind) {
    case "missing_guard":
      return 0;
    case "counter_evidence":
      return 1;
    case "static_analysis":
      return 2;
    case "test":
      return 3;
    default:
      return 8;
  }
}

function evidenceMapNodeLabel(node: EvidenceMapNode) {
  if (node.label.trim()) {
    return node.label;
  }
  if (node.kind === "missing_guard") {
    return "Missing guard";
  }
  if (node.kind === "counter_evidence") {
    return "Verified contradiction";
  }
  if (node.kind === "changed_code") {
    return "Issue line";
  }
  if (node.kind === "finding_claim") {
    return "Finding claim";
  }
  if (node.kind === "test") {
    return "Related test";
  }
  return node.label;
}

function evidenceMapNodeMeta(node: EvidenceMapNode) {
  const path = evidenceMapNodePath(node);
  if (path) {
    const parts = path.split("/");
    if (parts.length <= 3) {
      return path;
    }
    return `${parts.at(-3)}/${parts.at(-2)}/${parts.at(-1)}`;
  }
  return node.kind.replaceAll("_", " ");
}

function evidenceMapNodeStyle(kind: string) {
  switch (kind) {
    case "finding_claim":
      return {
        surface: "fill-background stroke-foreground/55",
        selected: "stroke-foreground",
        bar: "fill-foreground/80",
      };
    case "missing_guard":
      return {
        surface: "fill-destructive/5 stroke-destructive/70",
        selected: "stroke-destructive",
        bar: "fill-destructive/80",
      };
    case "counter_evidence":
    case "test":
      return {
        surface: "fill-warning/10 stroke-warning/70",
        selected: "stroke-warning",
        bar: "fill-warning/80",
      };
    case "entrypoint":
    case "route":
      return {
        surface: "fill-primary/5 stroke-primary/55",
        selected: "stroke-primary",
        bar: "fill-primary/80",
      };
    case "handler":
    case "changed_code":
      return {
        surface: "fill-success/10 stroke-success/60",
        selected: "stroke-success",
        bar: "fill-success/80",
      };
    default:
      return {
        surface: "fill-background stroke-border",
        selected: "stroke-primary",
        bar: "fill-muted-foreground/80",
      };
  }
}

function buildEvidenceMapLayout(map: EvidenceMapResponse): EvidenceMapLayout {
  const positioned: PositionedEvidenceMapNode[] = [];
  const columns = new Map<number, EvidenceMapNode[]>();
  for (const node of map.nodes) {
    const column = evidenceMapColumnForKind(node.kind);
    const columnNodes = columns.get(column) ?? [];
    columnNodes.push(node);
    columns.set(column, columnNodes);
  }
  for (const columnNodes of columns.values()) {
    columnNodes.sort(
      (left, right) =>
        evidenceMapSideNodeRank(left.kind) -
          evidenceMapSideNodeRank(right.kind) ||
        right.confidence - left.confidence,
    );
  }
  const maxRows = Math.max(
    1,
    ...[...columns.values()].map((nodes) => nodes.length),
  );
  for (const [column, columnNodes] of [...columns.entries()].sort(
    ([left], [right]) => left - right,
  )) {
    const verticalOffset = Math.max(
      0,
      ((maxRows - columnNodes.length) * ROW_GAP) / 2,
    );
    for (const [index, node] of columnNodes.entries()) {
      positioned.push({
        node,
        x: LEFT_PADDING + column * COLUMN_GAP,
        y: 56 + verticalOffset + index * ROW_GAP,
      });
    }
  }

  const maxX = positioned.reduce(
    (current, item) => Math.max(current, item.x),
    0,
  );
  const maxY = positioned.reduce(
    (current, item) => Math.max(current, item.y),
    0,
  );
  const width = Math.max(620, maxX + NODE_WIDTH + 40);
  const height = Math.max(440, maxY + NODE_HEIGHT + 80);
  const nodeById = new Map(positioned.map((node) => [node.node.id, node]));
  return { nodes: positioned, nodeById, width, height };
}

function evidenceMapColumnForKind(kind: string) {
  switch (kind) {
    case "entrypoint":
    case "route":
    case "handler":
    case "changed_code":
      return 0;
    case "finding_claim":
      return 1;
    case "missing_guard":
    case "test":
    case "counter_evidence":
    case "static_analysis":
    default:
      return 2;
  }
}

function evidenceMapSideNodeRank(kind: string) {
  switch (kind) {
    case "missing_guard":
      return 0;
    case "counter_evidence":
      return 1;
    case "test":
      return 2;
    default:
      return 3;
  }
}

function wrapSvgLabel(value: string, maxLineLength: number) {
  const words = value
    .split(/\s+/)
    .filter(Boolean)
    .flatMap((word) => splitLongWord(word, maxLineLength));
  const lines: string[] = [];
  let current = "";
  for (const word of words) {
    const next = current ? `${current} ${word}` : word;
    if (next.length > maxLineLength && current) {
      lines.push(current);
      current = word;
      continue;
    }
    current = next;
  }
  if (current) {
    lines.push(current);
  }
  return lines.length > 0 ? lines : [value];
}

function splitLongWord(word: string, maxLineLength: number) {
  if (word.length <= maxLineLength) {
    return [word];
  }
  const chunks: string[] = [];
  for (let index = 0; index < word.length; index += maxLineLength) {
    chunks.push(word.slice(index, index + maxLineLength));
  }
  return chunks;
}
