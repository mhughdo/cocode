import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  CodeSnippetViewer,
  EvidenceCardList,
  EvidenceMapGraphCanvas,
  FindingCard,
} from "./App";
import type {
  EvidenceItem,
  EvidenceMapResponse,
  Finding,
  FindingDetailResponse,
} from "@/lib/api";

describe("review component surfaces", () => {
  it("renders finding cards with status badges and copy action", () => {
    const html = renderToStaticMarkup(
      <FindingCard
        actionState={{ status: "idle" }}
        finding={findingFixture}
        selected
        onAccept={vi.fn()}
        onCopy={vi.fn()}
        onSelect={vi.fn()}
      />,
    );

    expect(html).toContain("Repository settings update lacks admin guard");
    expect(html).toContain("apps/api/src/routes/repositories.ts");
    expect(html).toContain("high");
    expect(html).toContain("Verified");
    expect(html).toContain("Copy");
  });

  it("renders evidence cards in prioritized order with locations", () => {
    const html = renderToStaticMarkup(
      <EvidenceCardList detail={findingDetailFixture} />,
    );

    expect(html.indexOf("Changed route")).toBeLessThan(
      html.indexOf("Admin route test"),
    );
    expect(html).toContain("apps/api/src/routes/repositories.ts:L87-L112");
    expect(html).toContain("supporting");
    expect(html).toContain("test");
  });

  it("renders graph nodes, edges, labels, and missing edge states", () => {
    const html = renderToStaticMarkup(
      <EvidenceMapGraphCanvas
        map={evidenceMapFixture}
        selection={{ kind: "node", id: "node_changed" }}
        onSelect={vi.fn()}
      />,
    );

    expect(html).toContain('aria-label="Evidence Map graph"');
    expect(html).toContain("PATCH repository");
    expect(html).toContain("settings");
    expect(html).toContain("Expected admin guard");
    expect(html).toContain("Missing guard");
    expect(html).toContain("stroke-dasharray");
  });

  it("renders code snippets with stable line numbers and copy path action", () => {
    const html = renderToStaticMarkup(
      <CodeSnippetViewer
        evidence={[evidenceItems[0]]}
        finding={findingFixture}
        onCopyPath={vi.fn()}
      />,
    );

    expect(html).toContain("Changed code");
    expect(html).toContain("Copy path");
    expect(html).toContain("requireWorkspaceAdmin");
    expect(html).toContain(">87<");
  });
});

const findingFixture = {
  id: "finding_auth",
  review_session_id: "session_1",
  canonical_claim: "Repository settings update lacks admin guard",
  category: "security",
  severity: "high",
  confidence: 0.92,
  verification_status: "verified",
  decision_status: "needs_triage",
  primary_path: "apps/api/src/routes/repositories.ts",
  primary_start_line: 87,
  primary_end_line: 112,
  evidence_summary: "Primary changed code reaches updateSettings.",
  suggested_fix: "Mount requireWorkspaceAdmin before updateSettings.",
  fingerprint: "auth-guard",
  merged_from_count: 2,
  first_seen_at: "2026-05-04T00:00:00Z",
  updated_at: "2026-05-04T00:01:00Z",
} satisfies Finding;

const evidenceItems = [
  {
    id: "evidence_route",
    finding_id: "finding_auth",
    kind: "supporting",
    title: "Changed route",
    summary: "The route reaches settings write after member auth.",
    path: "apps/api/src/routes/repositories.ts",
    start_line: 87,
    end_line: 112,
    confidence: 0.94,
    code_snippet:
      "router.patch('/repositories/:id/settings', requireWorkspaceMember, updateRepositorySettings)\nrequireWorkspaceAdmin",
    line_window: { start_line: 87, end_line: 88 },
    metadata: {},
    created_at: "2026-05-04T00:02:00Z",
  },
  {
    id: "evidence_test",
    finding_id: "finding_auth",
    kind: "test",
    title: "Admin route test",
    summary: "A related test mentions admin-only route behavior.",
    path: "apps/api/src/routes/repositories.test.ts",
    start_line: 14,
    end_line: 18,
    confidence: 0.6,
    metadata: {},
    created_at: "2026-05-04T00:03:00Z",
  },
] satisfies EvidenceItem[];

const findingDetailFixture = {
  finding: findingFixture,
  candidates: [],
  evidence_items: evidenceItems,
  evidence_groups: {
    supporting: [evidenceItems[0]],
    counter: [],
    neutral: [],
    missing: [],
    test: [evidenceItems[1]],
    search: [],
    agent: [],
    static_analysis: [],
  },
  decisions: [],
} satisfies FindingDetailResponse;

const evidenceMapFixture = {
  graph: {
    id: "graph_1",
    finding_id: "finding_auth",
    review_session_id: "session_1",
    status: "ready",
    summary: "The changed route lacks a visible admin guard.",
    layout: {},
    created_at: "2026-05-04T00:00:00Z",
    updated_at: "2026-05-04T00:01:00Z",
  },
  finding: {
    id: "finding_auth",
    review_session_id: "session_1",
    canonical_claim: findingFixture.canonical_claim,
    category: findingFixture.category,
    severity: findingFixture.severity,
    confidence: findingFixture.confidence,
    verification_status: findingFixture.verification_status,
    decision_status: findingFixture.decision_status,
    primary_path: findingFixture.primary_path,
    primary_start_line: findingFixture.primary_start_line,
    primary_end_line: findingFixture.primary_end_line,
  },
  hierarchy: [],
  nodes: [
    {
      id: "node_changed",
      kind: "changed_code",
      label: "PATCH repository settings",
      path: "apps/api/src/routes/repositories.ts",
      start_line: 87,
      end_line: 112,
      evidence_item_id: "evidence_route",
      confidence: 0.94,
      deep_link: {
        kind: "file",
        path: "apps/api/src/routes/repositories.ts",
        start_line: 87,
        end_line: 112,
      },
      metadata: {},
    },
    {
      id: "node_missing",
      kind: "missing_guard",
      label: "Expected admin guard",
      confidence: 0.8,
      metadata: {},
    },
  ],
  edges: [
    {
      id: "edge_missing",
      source: "node_changed",
      target: "node_missing",
      kind: "missing_guard",
      status: "missing",
      label: "Missing guard",
      confidence: 0.8,
      metadata: {},
    },
  ],
  call_path: [],
  call_paths: [],
  legend: [],
  panel: {
    claim: findingFixture.canonical_claim,
    severity: findingFixture.severity,
    verification_status: findingFixture.verification_status,
    decision_status: findingFixture.decision_status,
    evidence_counts: { supporting: 1, missing: 1 },
    evidence: [],
  },
} satisfies EvidenceMapResponse;
