import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  CodeSnippetViewer,
  EvidenceCardList,
  FindingCard,
} from "../findings/finding-components";
import { ChatMessageCard } from "../chat/chat-message-card";
import { FinalFindingsMessage } from "../chat/final-findings-message";
import { EvidenceMapGraphCanvas } from "../evidence/review-evidence-map";
import { EvidenceMapInspectorPanel } from "../evidence/evidence-map-inspector-panel";
import {
  MarkdownMessage,
  parseFileReference,
} from "../shared/markdown-message";
import { FileReferenceActionsProvider } from "../shared/file-reference-actions";
import type {
  ChatMessage,
  EvidenceItem,
  EvidenceMapResponse,
  Finding,
  FindingDetailResponse,
} from "@/lib/api";

describe("review component surfaces", () => {
  it("renders finding cards with a status selector", () => {
    const html = renderToStaticMarkup(
      <FindingCard
        actionState={{ status: "idle" }}
        finding={findingFixture}
        selected
        onDecisionChange={vi.fn()}
        onOpenDetail={vi.fn()}
        onSelect={vi.fn()}
      />,
    );

    expect(html).toContain("Repository settings update lacks admin guard");
    expect(html).toContain("apps/api/src/routes/repositories.ts");
    expect(html).toContain("high");
    expect(html).toContain("Needs triage");
    expect(html).toContain(
      'aria-label="Set status for Repository settings update lacks admin guard"',
    );
  });

  it("renders evidence cards in prioritized order with locations", () => {
    const html = renderToStaticMarkup(
      <EvidenceCardList detail={findingDetailFixture} />,
    );

    expect(html.indexOf("Changed route")).toBeLessThan(
      html.indexOf("Admin route test"),
    );
    expect(html).toContain("apps/api/src/routes/repositories.ts:L87-L112");
    expect(html).toContain("Supporting");
    expect(html).toContain("Test signal");
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

  it("draws caller relationships from caller to callee issue line", () => {
    const html = renderToStaticMarkup(
      <EvidenceMapGraphCanvas
        map={callerEvidenceMapFixture}
        selection={{ kind: "node", id: "node_caller" }}
        onSelect={vi.fn()}
      />,
    );

    expect(html).toContain("gopls resolved pickTokenPrice caller");
    expect(html).toContain("Unchecked sell-price dereference");
    expect(html).toContain('d="M 143 152 L 143 182"');
  });

  it("renders code snippets with stable line numbers and copy path action", () => {
    const html = renderToStaticMarkup(
      <CodeSnippetViewer
        evidence={[evidenceItems[0]]}
        finding={findingFixture}
        onCopyPath={vi.fn()}
      />,
    );

    expect(html).toContain("Primary code");
    expect(html).toContain("Copy path");
    expect(html).toContain("requireWorkspaceAdmin");
    expect(html).toContain("rs-line-number");
    expect(html).toContain("--line-start:87");
  });

  it("renders code snippets from evidence metadata when the lifted field is absent", () => {
    const html = renderToStaticMarkup(
      <CodeSnippetViewer
        evidence={[
          {
            ...evidenceItems[0],
            code_snippet: undefined,
            line_window: undefined,
            metadata: {
              code_snippet:
                "87: router.patch('/repositories/:id/settings', requireWorkspaceMember, updateRepositorySettings)\n88: requireWorkspaceAdmin",
              line_window: { start_line: 87, end_line: 88 },
            },
          },
        ]}
        finding={findingFixture}
        onCopyPath={vi.fn()}
      />,
    );

    expect(html).toContain("requireWorkspaceAdmin");
    expect(html).toContain("rs-line-number");
    expect(html).toContain("--line-start:87");
    expect(html).not.toContain("No code window is attached yet");
  });

  it("renders the evidence map source file inline without an editor action", () => {
    const html = renderToStaticMarkup(
      <EvidenceMapInspectorPanel
        activeRepositoryPath="/repo/cocode"
        map={evidenceMapFixture}
        selectedNode={evidenceMapFixture.nodes[0]}
      />,
    );

    expect(html).toContain("Source file");
    expect(html).toContain("full file");
    expect(html).toContain("rs-highlighted-line");
    expect(html).toContain("router.patch");
    expect(html).toContain("rs-line-number");
    expect(html).toContain("Mount requireWorkspaceAdmin");
    expect(html).not.toContain("Open in editor");
  });

  it("renders diff fences with addition and deletion highlighting", () => {
    const html = renderToStaticMarkup(
      <MarkdownMessage
        content={[
          "```diff",
          " if len(filter.Protocols) == 1 {",
          "-  sources = []aggregatedposition.PositionSource{targetSource}",
          "+  if cfg.Mode != config.ProtocolSourceModeHybrid {",
          "+    sources = []aggregatedposition.PositionSource{targetSource}",
          "+  }",
          " }",
          "```",
        ].join("\n")}
      />,
    );

    expect(html).toContain("bg-red-50");
    expect(html).toContain("bg-emerald-50");
    expect(html).toContain('aria-label="Copy code snippet"');
    expect(html).toContain("sources = []aggregatedposition.PositionSource");
  });

  it("renders markdown code fences with a copy action", () => {
    const html = renderToStaticMarkup(
      <MarkdownMessage content={"```go\nreturn nil\n```"} />,
    );

    expect(html).toContain('aria-label="Copy code snippet"');
    expect(html).toContain("return nil");
  });

  it("unwraps bracketed inline file references in markdown", () => {
    const openFileReference = vi.fn();
    const html = renderToStaticMarkup(
      <FileReferenceActionsProvider value={{ openFileReference }}>
        <MarkdownMessage content="In [migrations/clickhouse/008_dedup_pool_active_apr_snapshots.sql:31-57], and [`migrations/clickhouse/009_cleanup_active_apr_spikes.sql:70-70`], the new table is:" />
      </FileReferenceActionsProvider>,
    );

    expect(html).toContain(
      "migrations/clickhouse/008_dedup_pool_active_apr_snapshots.sql:31-57",
    );
    expect(html).toContain(
      "migrations/clickhouse/009_cleanup_active_apr_spikes.sql:70-70",
    );
    expect(html).toContain(">008_dedup_pool_active_apr_snapshots.sql</button>");
    expect(html).toContain(">009_cleanup_active_apr_spikes.sql</button>");
    expect(html).toContain("<button");
    expect(html).toContain("Open migrations/clickhouse");
    expect(html).not.toContain("In [<code");
    expect(html).not.toContain("In [<button");
    expect(html).not.toContain("</code>],");
  });

  it("parses file reference line ranges for right panel highlighting", () => {
    expect(
      parseFileReference(
        "migrations/clickhouse/008_dedup_pool_active_apr_snapshots.sql:31-57",
      ),
    ).toMatchObject({
      endLine: 57,
      path: "migrations/clickhouse/008_dedup_pool_active_apr_snapshots.sql",
      startLine: 31,
    });
  });

  it("keeps finalized chat output expanded while allowing collapse", () => {
    const longBody = Array.from(
      { length: 18 },
      (_, index) => `Line ${index + 1} with enough detail to keep.`,
    ).join("\n");
    const completed = renderToStaticMarkup(
      <ChatMessageCard
        events={[]}
        message={{ ...chatMessageFixture, body: longBody, status: "completed" }}
      />,
    );
    const streaming = renderToStaticMarkup(
      <ChatMessageCard
        events={[]}
        message={{ ...chatMessageFixture, body: longBody, status: "streaming" }}
      />,
    );

    expect(completed).toContain("Show less");
    expect(completed).toContain("Line 18");
    expect(streaming).toContain("See more");
  });

  it("renders a copy action for finalized agent messages", () => {
    const html = renderToStaticMarkup(
      <ChatMessageCard
        events={[]}
        message={{ ...chatMessageFixture, body: "Readable agent output." }}
      />,
    );

    expect(html).toContain('aria-label="Copy message from Orchestrator"');
  });

  it("renders a copy action for user messages", () => {
    const html = renderToStaticMarkup(
      <ChatMessageCard
        events={[]}
        message={{
          ...chatMessageFixture,
          author_display_name: "You",
          author_type: "user",
          body: "Can you explain this finding again?",
        }}
      />,
    );

    expect(html).toContain('aria-label="Copy message from You"');
  });

  it("formats persisted raw agent event bodies before rendering", () => {
    const codexBody = JSON.stringify({
      type: "item.completed",
      item: {
        id: "item_43",
        type: "agent_message",
        text: JSON.stringify({
          clusters: [
            {
              canonical_claim:
                "Migration backfill uses the same inserted_at value for historical rows.",
              severity: "medium",
              confidence: 0.86,
              verification_status: "locally_supported",
              primary_location: {
                path: "migrations/clickhouse/008_dedup_pool_active_apr_snapshots.sql",
                start_line: 54,
                end_line: 55,
              },
              evidence_summary:
                "The backfill explicitly inserts now() for all copied rows.",
            },
          ],
        }),
      },
    });
    const claudeSignatureBody = JSON.stringify({
      type: "stream_event",
      event: {
        type: "content_block_delta",
        index: 0,
        delta: {
          type: "signature_delta",
          signature: "EoyNAQpjCA4YAipAr48AuWx35QMatCF5q",
        },
      },
    });

    const codex = renderToStaticMarkup(
      <ChatMessageCard
        events={[]}
        message={{
          ...chatMessageFixture,
          agent_run_id: "agent_run_1",
          body: codexBody,
          status: "completed",
        }}
      />,
    );
    const claude = renderToStaticMarkup(
      <ChatMessageCard
        events={[]}
        message={{
          ...chatMessageFixture,
          agent_run_id: "agent_run_2",
          body: claudeSignatureBody,
          status: "streaming",
        }}
      />,
    );

    expect(codex).toContain("Findings (1)");
    expect(codex).toContain("Migration backfill uses the same inserted_at");
    expect(codex).not.toContain("item.completed");
    expect(codex).not.toContain("&quot;clusters&quot;");
    expect(claude).toContain("streaming output back to cocode");
    expect(claude).not.toContain("signature_delta");
    expect(claude).not.toContain("EoyNAQpj");
  });

  it("brands finalized findings as Cocode instead of a generic system message", () => {
    const html = renderToStaticMarkup(
      <FinalFindingsMessage
        findings={[findingFixture]}
        onOpenFindingDetail={vi.fn()}
        onOpenFindings={vi.fn()}
      />,
    );

    expect(html).toContain("Cocode");
    expect(html).toContain("Findings finalized");
    expect(html).not.toContain(">System<");
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
  trust_state: "verifier_survived",
  publishable: false,
  publish_blockers: ["finding must be accepted by a human"],
  primary_path: "apps/api/src/routes/repositories.ts",
  primary_start_line: 87,
  primary_end_line: 112,
  evidence_summary: "Primary changed code reaches updateSettings.",
  suggested_fix: "Mount requireWorkspaceAdmin before updateSettings.",
  fingerprint: "auth-guard",
  merged_from_count: 2,
  first_seen_at: "2026-05-04T00:00:00Z",
  updated_at: "2026-05-04T00:01:00Z",
  source_agents: [
    {
      agent_run_id: "agent_run_1",
      agent_config_id: "agent_config_1",
      name: "Codex CLI",
      model_label: "GPT-5.5",
      severity: "high",
      confidence: 0.92,
    },
  ],
} satisfies Finding;

const chatMessageFixture: ChatMessage = {
  id: "message_1",
  thread_id: "thread_1",
  author_type: "orchestrator",
  author_display_name: "Orchestrator",
  body: "Reviewed the diff.",
  status: "completed",
  metadata: {},
  created_at: "2026-05-04T00:01:00Z",
  updated_at: "2026-05-04T00:01:00Z",
};

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

const sourceFileContent = [
  "import { FastifyInstance } from 'fastify';",
  "import { requireWorkspaceMember } from '../middleware/auth';",
  "",
  "export async function registerRepositoryRoutes(router: FastifyInstance) {",
  "  router.get('/repositories/:id/settings', requireWorkspaceMember, async (request, reply) => {",
  "    return reply.send({ ok: true });",
  "  });",
  "  router.patch('/repositories/:id/settings', requireWorkspaceMember, updateRepositorySettings);",
  "  requireWorkspaceAdmin",
  "}",
].join("\n");

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
      code_snippet:
        "router.patch('/repositories/:id/settings', requireWorkspaceMember, updateRepositorySettings)\nrequireWorkspaceAdmin",
      line_window: { start_line: 87, end_line: 88 },
      file_content: sourceFileContent,
      file_line_count: 10,
      file_truncated: false,
      deep_link: {
        kind: "file",
        path: "apps/api/src/routes/repositories.ts",
        start_line: 8,
        end_line: 9,
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
    suggested_fix: findingFixture.suggested_fix,
    evidence: [
      {
        id: "evidence_route",
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
        file_content: sourceFileContent,
        file_line_count: 10,
        file_truncated: false,
      },
    ],
  },
} satisfies EvidenceMapResponse;

const callerEvidenceMapFixture = {
  ...evidenceMapFixture,
  nodes: [
    {
      id: "node_caller",
      kind: "changed_code",
      label: "gopls resolved pickTokenPrice caller",
      path: "internal/app/aggregatedposition/fetcher/kyberdata/fetcher.go",
      start_line: 344,
      end_line: 348,
      confidence: 0.96,
      metadata: { relationship: "caller" },
    },
    {
      id: "node_issue",
      kind: "changed_code",
      label: "Unchecked sell-price dereference",
      path: "internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go",
      start_line: 207,
      end_line: 208,
      confidence: 0.93,
      metadata: {},
    },
  ],
  edges: [
    {
      id: "edge_caller_to_issue",
      source: "node_caller",
      target: "node_issue",
      kind: "calls",
      status: "observed",
      label: "caller reaches callee",
      confidence: 0.94,
      metadata: {},
    },
  ],
  finding: {
    ...evidenceMapFixture.finding,
    primary_path:
      "internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go",
    primary_start_line: 207,
    primary_end_line: 208,
  },
  panel: {
    ...evidenceMapFixture.panel,
    evidence: [],
  },
} satisfies EvidenceMapResponse;
