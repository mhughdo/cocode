import { describe, expect, it, vi } from "vitest";

import {
  ApiError,
  createCocodeClient,
  idleApiState,
  loadApiResource,
  loadingApiState,
  successApiState,
} from "./api";

describe("ApiClient", () => {
  it("sends auth headers, query params, and decodes data envelopes", async () => {
    let seen: { url: string; headers: Headers } | undefined;
    const fetcher = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        seen = {
          url: String(input),
          headers: new Headers(init?.headers),
        };
        return jsonResponse({
          data: { status: "authenticated" },
          error: null,
          request_id: "req_1",
        });
      },
    );
    const client = createCocodeClient({
      baseUrl: "http://127.0.0.1:17658/",
      authToken: "local-token",
      fetch: fetcher,
    });

    const data = await client.get<{ status: string }>("/api/session", {
      query: { q: "hello world", page: 2, empty: undefined },
    });

    expect(data).toEqual({ status: "authenticated" });
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(seen).toBeDefined();
    expect(seen?.url).toBe(
      "http://127.0.0.1:17658/api/session?q=hello+world&page=2",
    );
    expect(seen?.headers.get("Authorization")).toBe("Bearer local-token");
    expect(seen?.headers.get("Accept")).toBe("application/json");
  });

  it("encodes JSON bodies for write requests", async () => {
    let body = "";
    let method = "";
    const fetcher = vi.fn(
      async (_input: RequestInfo | URL, init?: RequestInit) => {
        method = init?.method ?? "";
        body = String(init?.body ?? "");
        return jsonResponse({
          data: { ok: true },
          error: null,
        });
      },
    );
    const client = createCocodeClient({
      baseUrl: "http://127.0.0.1:17658",
      authToken: "local-token",
      fetch: fetcher,
    });

    await client.post<{ ok: boolean }>("/api/review-sessions", {
      snapshot_id: "snapshot_1",
    });

    expect(method).toBe("POST");
    expect(JSON.parse(body)).toEqual({ snapshot_id: "snapshot_1" });
  });

  it("opens repositories through the workspace endpoint", async () => {
    let seenPath = "";
    const fetcher = vi.fn(
      async (_input: RequestInfo | URL, init?: RequestInit) => {
        seenPath = JSON.parse(String(init?.body ?? "{}")).path as string;
        return jsonResponse({
          data: {
            workspace: workspaceFixture,
            repository: repositoryFixture,
            repositories: [repositoryFixture],
          },
          error: null,
        });
      },
    );
    const client = createCocodeClient({
      baseUrl: "http://127.0.0.1:17658",
      authToken: "local-token",
      fetch: fetcher,
    });

    await expect(client.openRepository("/repo/cocode")).resolves.toMatchObject({
      workspace: { id: "workspace_1" },
      repository: { id: "repo_1" },
    });
    expect(seenPath).toBe("/repo/cocode");
  });

  it("creates snapshots and starts reviews through typed helpers", async () => {
    const seen: { url: string; body: unknown }[] = [];
    const fetcher = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        seen.push({
          url: String(input),
          body: init?.body ? JSON.parse(String(init.body)) : null,
        });
        const url = String(input);
        if (url.endsWith("/api/pr-snapshots/from-local-changes")) {
          return jsonResponse({ data: snapshotFixture, error: null });
        }
        if (url.endsWith("/api/pr-snapshots/snapshot_1/changed-files")) {
          return jsonResponse({ data: [changedFileFixture], error: null });
        }
        if (url.endsWith("/api/review-sessions")) {
          return jsonResponse({ data: reviewSessionFixture, error: null });
        }
        if (url.endsWith("/api/review-sessions/session_1/start")) {
          return jsonResponse({
            data: { ...reviewSessionFixture, status: "queued" },
            error: null,
          });
        }
        return jsonResponse({ data: null, error: null });
      },
    );
    const client = createCocodeClient({
      baseUrl: "http://127.0.0.1:17658",
      authToken: "local-token",
      fetch: fetcher,
    });

    await expect(
      client.createLocalChangesSnapshot({
        workspace_id: "workspace_1",
        repository_id: "repo_1",
      }),
    ).resolves.toMatchObject({ id: "snapshot_1" });
    await expect(client.listChangedFiles("snapshot_1")).resolves.toHaveLength(
      1,
    );
    await expect(
      client.createReviewSession({
        workspace_id: "workspace_1",
        snapshot_id: "snapshot_1",
        title: "Review local changes",
        review_depth: "standard",
        agent_config_ids: ["agent_1"],
        runtime_limit_seconds: 1800,
        context_policy: { redact_secrets: true },
      }),
    ).resolves.toMatchObject({ id: "session_1", status: "draft" });
    await expect(client.startReviewSession("session_1")).resolves.toMatchObject(
      { status: "queued" },
    );

    expect(seen.map((request) => request.url)).toEqual([
      "http://127.0.0.1:17658/api/pr-snapshots/from-local-changes",
      "http://127.0.0.1:17658/api/pr-snapshots/snapshot_1/changed-files",
      "http://127.0.0.1:17658/api/review-sessions",
      "http://127.0.0.1:17658/api/review-sessions/session_1/start",
    ]);
  });

  it("controls live reviews, queries findings, and reads SSE events", async () => {
    const seen: {
      url: string;
      method: string;
      headers: Headers;
      body: unknown;
    }[] = [];
    const fetcher = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? "GET";
        seen.push({
          url,
          method,
          headers: new Headers(init?.headers),
          body: init?.body ? JSON.parse(String(init.body)) : null,
        });

        if (url.endsWith("/api/review-sessions/session_1/pause")) {
          return jsonResponse({
            data: { ...reviewSessionFixture, status: "paused" },
            error: null,
          });
        }
        if (url.endsWith("/api/review-sessions/session_1/resume")) {
          return jsonResponse({
            data: { ...reviewSessionFixture, status: "running" },
            error: null,
          });
        }
        if (url.endsWith("/api/review-sessions/session_1/cancel")) {
          return jsonResponse({
            data: { ...reviewSessionFixture, status: "canceled" },
            error: null,
          });
        }
        if (url.includes("/api/review-sessions/session_1/findings")) {
          return jsonResponse({ data: findingListFixture, error: null });
        }
        if (url.endsWith("/api/findings/finding_1") && method === "GET") {
          return jsonResponse({ data: findingDetailFixture, error: null });
        }
        if (url.endsWith("/api/findings/finding_1/decision")) {
          return jsonResponse({
            data: {
              ...findingDetailFixture,
              finding: { ...findingFixture, decision_status: "accepted" },
            },
            error: null,
          });
        }
        if (url.endsWith("/api/findings/finding_1/draft-comment")) {
          return jsonResponse({
            data: {
              ...findingFixture,
              draft_comment: "Please add the missing middleware.",
            },
            error: null,
          });
        }
        if (
          url.endsWith("/api/findings/finding_1/thread") &&
          method === "GET"
        ) {
          return jsonResponse({ data: findingThreadViewFixture, error: null });
        }
        if (url.endsWith("/api/findings/finding_1/question")) {
          return jsonResponse({
            data: askFindingQuestionFixture,
            error: null,
          });
        }
        if (url.endsWith("/api/findings/finding_1/thread/actions")) {
          return jsonResponse({
            data: findingQuickActionFixture,
            error: null,
          });
        }
        if (url.endsWith("/api/review-sessions/session_1/export/copy-packet")) {
          return jsonResponse({
            data: copyPacketFixture,
            error: null,
          });
        }
        if (url.endsWith("/api/copy-packets/copy_packet_1/copied")) {
          return jsonResponse({
            data: copiedPacketFixture,
            error: null,
          });
        }
        if (url.endsWith("/api/review-sessions/session_1/github/preview")) {
          return jsonResponse({
            data: githubPreviewFixture,
            error: null,
          });
        }
        if (
          url.endsWith("/api/findings/finding_1/evidence-map") &&
          method === "GET"
        ) {
          return jsonResponse({ data: evidenceMapFixture, error: null });
        }
        if (url.endsWith("/api/findings/finding_1/evidence-map/rebuild")) {
          return jsonResponse({
            data: {
              ...evidenceMapFixture,
              graph: { ...evidenceMapFixture.graph, status: "ready" },
            },
            error: null,
          });
        }
        if (url.endsWith("/api/findings/finding_1/evidence-map/question")) {
          return jsonResponse({
            data: askEvidenceMapQuestionFixture,
            error: null,
          });
        }
        if (
          url.endsWith("/api/review-sessions/session_1/events?after_sequence=2")
        ) {
          return streamResponse(
            [
              ": keep-alive",
              "event: heartbeat",
              "data: {}",
              "",
              "id: 3",
              "event: review.event",
              `data: ${JSON.stringify(reviewEventFixture)}`,
              "",
            ].join("\n"),
          );
        }
        return jsonResponse({ data: null, error: null });
      },
    );
    const client = createCocodeClient({
      baseUrl: "http://127.0.0.1:17658",
      authToken: "local-token",
      fetch: fetcher,
    });

    await expect(client.pauseReviewSession("session_1")).resolves.toMatchObject(
      { status: "paused" },
    );
    await expect(
      client.resumeReviewSession("session_1"),
    ).resolves.toMatchObject({ status: "running" });
    await expect(
      client.cancelReviewSession("session_1"),
    ).resolves.toMatchObject({ status: "canceled" });
    await expect(
      client.listFindings("session_1", {
        status: "needs_triage",
        severity: "high",
        q: "auth",
      }),
    ).resolves.toEqual(findingListFixture);
    await expect(client.getFindingDetail("finding_1")).resolves.toEqual(
      findingDetailFixture,
    );
    await expect(
      client.updateFindingDecision("finding_1", {
        decision: "accepted",
        reason: "verified from board",
      }),
    ).resolves.toMatchObject({
      finding: { id: "finding_1", decision_status: "accepted" },
    });
    await expect(
      client.updateFindingDraftComment(
        "finding_1",
        "Please add the missing middleware.",
      ),
    ).resolves.toMatchObject({
      id: "finding_1",
      draft_comment: "Please add the missing middleware.",
    });
    await expect(client.getFindingThread("finding_1")).resolves.toEqual(
      findingThreadViewFixture,
    );
    await expect(
      client.askFindingQuestion("finding_1", {
        question: "Can you check counter-evidence?",
        agent_config_id: "agent_config_1",
      }),
    ).resolves.toMatchObject({
      assistant_message: { content: "No counter-evidence was found." },
    });
    await expect(
      client.runFindingQuickAction("finding_1", {
        action: "copy",
        reason: "sent to agent",
      }),
    ).resolves.toMatchObject({
      action: "copy",
      finding: { decision_status: "copied" },
    });
    await expect(
      client.createReviewCopyPacket("session_1", {
        finding_ids: ["finding_1"],
        format: "markdown",
      }),
    ).resolves.toMatchObject({
      copy_packet_id: "copy_packet_1",
      finding_count: 1,
    });
    await expect(
      client.markCopyPacketCopied("copy_packet_1"),
    ).resolves.toMatchObject({
      copy_packet_id: "copy_packet_1",
      finding_ids: ["finding_1"],
    });
    await expect(
      client.createGitHubPreview("session_1", {
        finding_ids: ["finding_1"],
        review_event: "COMMENT",
      }),
    ).resolves.toMatchObject({
      publish_draft_id: "publish_draft_1",
      checklist: { has_selected_findings: true },
    });
    await expect(client.getFindingEvidenceMap("finding_1")).resolves.toEqual(
      evidenceMapFixture,
    );
    await expect(
      client.rebuildFindingEvidenceMap("finding_1"),
    ).resolves.toMatchObject({ graph: { status: "ready" } });
    await expect(
      client.askEvidenceMapQuestion("finding_1", {
        question: "Does this graph prove the missing guard?",
        agent_config_id: "agent_config_1",
        graph_refs: [{ node_id: "node_1" }],
        context_policy: { max_tokens: 5000, max_items: 30 },
      }),
    ).resolves.toMatchObject({
      assistant_message: { content: "The graph still shows a missing guard." },
    });

    const events: unknown[] = [];
    await client.streamReviewEvents("session_1", {
      afterSequence: 2,
      onEvent: (event) => events.push(event),
    });

    expect(events).toEqual([reviewEventFixture]);
    expect(seen.map((request) => `${request.method} ${request.url}`)).toEqual([
      "POST http://127.0.0.1:17658/api/review-sessions/session_1/pause",
      "POST http://127.0.0.1:17658/api/review-sessions/session_1/resume",
      "POST http://127.0.0.1:17658/api/review-sessions/session_1/cancel",
      "GET http://127.0.0.1:17658/api/review-sessions/session_1/findings?status=needs_triage&severity=high&q=auth",
      "GET http://127.0.0.1:17658/api/findings/finding_1",
      "PATCH http://127.0.0.1:17658/api/findings/finding_1/decision",
      "PATCH http://127.0.0.1:17658/api/findings/finding_1/draft-comment",
      "GET http://127.0.0.1:17658/api/findings/finding_1/thread",
      "POST http://127.0.0.1:17658/api/findings/finding_1/question",
      "POST http://127.0.0.1:17658/api/findings/finding_1/thread/actions",
      "POST http://127.0.0.1:17658/api/review-sessions/session_1/export/copy-packet",
      "POST http://127.0.0.1:17658/api/copy-packets/copy_packet_1/copied",
      "POST http://127.0.0.1:17658/api/review-sessions/session_1/github/preview",
      "GET http://127.0.0.1:17658/api/findings/finding_1/evidence-map",
      "POST http://127.0.0.1:17658/api/findings/finding_1/evidence-map/rebuild",
      "POST http://127.0.0.1:17658/api/findings/finding_1/evidence-map/question",
      "GET http://127.0.0.1:17658/api/review-sessions/session_1/events?after_sequence=2",
    ]);
    expect(seen[5]?.body).toEqual({
      decision: "accepted",
      reason: "verified from board",
    });
    expect(seen[6]?.body).toEqual({
      draft_comment: "Please add the missing middleware.",
    });
    expect(seen[8]?.body).toEqual({
      question: "Can you check counter-evidence?",
      agent_config_id: "agent_config_1",
    });
    expect(seen[9]?.body).toEqual({
      action: "copy",
      reason: "sent to agent",
    });
    expect(seen[10]?.body).toEqual({
      finding_ids: ["finding_1"],
      format: "markdown",
    });
    expect(seen[12]?.body).toEqual({
      finding_ids: ["finding_1"],
      review_event: "COMMENT",
    });
    expect(seen[15]?.body).toEqual({
      question: "Does this graph prove the missing guard?",
      agent_config_id: "agent_config_1",
      graph_refs: [{ node_id: "node_1" }],
      context_policy: { max_tokens: 5000, max_items: 30 },
    });
    for (const request of seen) {
      expect(request.headers.get("Authorization")).toBe("Bearer local-token");
    }
  });

  it("manages agent configs through typed helpers", async () => {
    const seen: { url: string; method: string; body: unknown }[] = [];
    const fetcher = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? "GET";
        const body = init?.body ? JSON.parse(String(init.body)) : null;
        seen.push({ url, method, body });

        if (url.endsWith("/api/agents/configs") && method === "POST") {
          return jsonResponse({
            data: { ...agentConfigFixture, ...(body as object) },
            error: null,
          });
        }
        if (
          url.endsWith("/api/agents/configs/agent_config_1") &&
          method === "PATCH"
        ) {
          return jsonResponse({
            data: { ...agentConfigFixture, ...(body as object) },
            error: null,
          });
        }
        if (url.endsWith("/api/agents/configs/agent_config_1/test")) {
          return jsonResponse({ data: agentHealthFixture, error: null });
        }
        if (
          url.endsWith("/api/agents/configs/agent_config_1") &&
          method === "DELETE"
        ) {
          return jsonResponse({ data: { deleted: true }, error: null });
        }
        return jsonResponse({ data: null, error: null });
      },
    );
    const client = createCocodeClient({
      baseUrl: "http://127.0.0.1:17658",
      authToken: "local-token",
      fetch: fetcher,
    });

    await expect(
      client.createAgentConfig({
        name: "OpenCode",
        role: "primary_reviewer",
        adapter_kind: "cli_non_interactive",
        command: "opencode",
        args: ["run", "--format", "json", "{{prompt}}"],
        cwd_mode: "repo_root",
        env_allowlist: ["OPENROUTER_API_KEY"],
        output_mode: "jsonl",
        capabilities: {
          can_read: true,
          output_modes: ["jsonl", "json", "text"],
          metadata: { provider: "opencode", egress: "external" },
        },
        settings: { prompt_delivery: "arg", timeout_seconds: 1800 },
        enabled: true,
      }),
    ).resolves.toMatchObject({
      id: "agent_config_1",
      command: "opencode",
      capabilities: {
        metadata: { provider: "opencode", egress: "external" },
      },
    });
    await expect(
      client.updateAgentConfig("agent_config_1", { enabled: false }),
    ).resolves.toMatchObject({ enabled: false });
    await expect(client.testAgentConfig("agent_config_1")).resolves.toEqual(
      agentHealthFixture,
    );
    await expect(client.deleteAgentConfig("agent_config_1")).resolves.toEqual({
      deleted: true,
    });

    expect(seen.map((request) => `${request.method} ${request.url}`)).toEqual([
      "POST http://127.0.0.1:17658/api/agents/configs",
      "PATCH http://127.0.0.1:17658/api/agents/configs/agent_config_1",
      "POST http://127.0.0.1:17658/api/agents/configs/agent_config_1/test",
      "DELETE http://127.0.0.1:17658/api/agents/configs/agent_config_1",
    ]);
  });

  it("throws ApiError for backend envelopes with errors", async () => {
    const client = createCocodeClient({
      baseUrl: "http://127.0.0.1:17658",
      authToken: "local-token",
      fetch: async () =>
        jsonResponse(
          {
            data: null,
            error: {
              code: "INVALID_REQUEST",
              message: "snapshot_id is required",
              details: { field: "snapshot_id" },
            },
            request_id: "req_bad",
          },
          400,
        ),
    });

    await expect(client.get("/api/review-sessions")).rejects.toMatchObject({
      name: "ApiError",
      status: 400,
      code: "INVALID_REQUEST",
      message: "snapshot_id is required",
      requestId: "req_bad",
      details: { field: "snapshot_id" },
    });
  });

  it("reports invalid JSON and invalid envelope responses", async () => {
    const invalidJSON = createCocodeClient({
      baseUrl: "http://127.0.0.1:17658",
      authToken: "local-token",
      fetch: async () => new Response("not-json", { status: 200 }),
    });
    await expect(invalidJSON.get("/api/session")).rejects.toMatchObject({
      code: "INVALID_JSON",
    });

    const invalidEnvelope = createCocodeClient({
      baseUrl: "http://127.0.0.1:17658",
      authToken: "local-token",
      fetch: async () => jsonResponse({ status: "ok" }),
    });
    await expect(invalidEnvelope.get("/api/session")).rejects.toMatchObject({
      code: "INVALID_ENVELOPE",
    });
  });
});

describe("API load states", () => {
  it("creates idle, loading, success, and error states", async () => {
    expect(idleApiState()).toEqual({ status: "idle" });
    expect(loadingApiState()).toEqual({ status: "loading" });
    expect(successApiState({ ok: true })).toEqual({
      status: "success",
      data: { ok: true },
    });

    await expect(loadApiResource(async () => "ready")).resolves.toEqual({
      status: "success",
      data: "ready",
    });
    const failed = await loadApiResource(async () => {
      throw new ApiError({
        message: "offline",
        status: 0,
        code: "NETWORK_ERROR",
      });
    });
    expect(failed.status).toBe("error");
    if (failed.status === "error") {
      expect(failed.error.code).toBe("NETWORK_ERROR");
    }
  });
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function streamResponse(body: string): Response {
  const encoder = new TextEncoder();
  return new Response(
    new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode(body));
        controller.close();
      },
    }),
    {
      status: 200,
      headers: { "Content-Type": "text/event-stream" },
    },
  );
}

const workspaceFixture = {
  id: "workspace_1",
  name: "cocode",
  root_path: "/repo/cocode",
  default_repo_id: "repo_1",
  settings_json: "{}",
  settings: {},
  created_at: "2026-05-04T00:00:00Z",
  updated_at: "2026-05-04T00:00:00Z",
};

const repositoryFixture = {
  id: "repo_1",
  workspace_id: "workspace_1",
  name: "cocode",
  owner: null,
  remote_url: null,
  local_path: "/repo/cocode",
  default_branch: "main",
  created_at: "2026-05-04T00:00:00Z",
  updated_at: "2026-05-04T00:00:00Z",
};

const snapshotFixture = {
  id: "snapshot_1",
  repository_id: "repo_1",
  source_type: "local_changes",
  base_ref: "HEAD",
  head_ref: "WORKTREE",
  metadata: {},
  changed_file_count: 1,
};

const changedFileFixture = {
  id: "changed_file_1",
  snapshot_id: "snapshot_1",
  path: "src/app.ts",
  status: "modified",
  additions: 10,
  deletions: 2,
  is_binary: false,
  is_generated: false,
  is_excluded: false,
  line_ranges: [],
};

const reviewSessionFixture = {
  id: "session_1",
  workspace_id: "workspace_1",
  repository_id: "repo_1",
  snapshot_id: "snapshot_1",
  title: "Review local changes",
  status: "draft",
  review_depth: "standard",
  context_policy: {},
  runtime_limit_seconds: 1800,
  agents: [],
  created_at: "2026-05-04T00:00:00Z",
  updated_at: "2026-05-04T00:00:00Z",
};

const findingFixture = {
  id: "finding_1",
  review_session_id: "session_1",
  canonical_claim: "Auth middleware is not applied to the admin route.",
  category: "security",
  severity: "high",
  confidence: 0.91,
  verification_status: "verified",
  decision_status: "needs_triage",
  primary_path: "src/app.ts",
  primary_start_line: 42,
  primary_end_line: 45,
  evidence_summary: "Route registration bypasses the protected group.",
  counter_evidence_summary: "",
  suggested_fix: "Mount the handler under the protected router group.",
  draft_comment: "This route appears to bypass auth.",
  fingerprint: "finding_auth_admin",
  merged_from_count: 2,
  introduced_in_sha: "abc123",
  first_seen_at: "2026-05-04T00:00:00Z",
  updated_at: "2026-05-04T00:00:00Z",
};

const findingListFixture = {
  items: [findingFixture],
  stats: {
    total: 1,
    filtered: 1,
    by_decision: { needs_triage: 1 },
    by_severity: { high: 1 },
    by_verification: { verified: 1 },
    needs_triage: 1,
  },
};

const findingDetailFixture = {
  finding: findingFixture,
  candidates: [],
  evidence_items: [],
  evidence_groups: {
    supporting: [],
    counter: [],
    neutral: [],
    missing: [],
    test: [],
    search: [],
    agent: [],
    static_analysis: [],
  },
  decisions: [],
};

const evidenceMapFixture = {
  graph: {
    id: "graph_1",
    finding_id: "finding_1",
    review_session_id: "session_1",
    status: "partial",
    summary: "Auth guard graph.",
    layout: {},
    created_at: "2026-05-04T00:00:00Z",
    updated_at: "2026-05-04T00:00:00Z",
  },
  finding: findingFixture,
  hierarchy: [
    {
      path: "src/app.ts",
      kind: "changed_code",
      start_line: 42,
      end_line: 45,
      node_ids: ["node_1"],
      evidence_item_ids: ["evidence_1"],
    },
  ],
  nodes: [
    {
      id: "node_1",
      kind: "changed_code",
      label: "Admin route",
      path: "src/app.ts",
      symbol: "router.patch",
      start_line: 42,
      end_line: 45,
      evidence_item_id: "evidence_1",
      confidence: 0.91,
      deep_link: {
        kind: "file",
        path: "src/app.ts",
        start_line: 42,
        end_line: 45,
      },
      metadata: {},
    },
  ],
  edges: [
    {
      id: "edge_1",
      source: "node_1",
      target: "node_missing_guard",
      kind: "missing_guard",
      status: "missing",
      label: "admin guard",
      confidence: 0.89,
      metadata: {},
    },
  ],
  call_path: [
    {
      id: "step_1",
      node_id: "node_1",
      step_index: 0,
      path: "src/app.ts",
      start_line: 42,
      end_line: 45,
      label: "PATCH /admin",
    },
  ],
  call_paths: [
    {
      id: "path_1",
      label: "Route to mutation",
      confidence: 0.89,
      steps: [
        {
          id: "step_1",
          node_id: "node_1",
          step_index: 0,
          path: "src/app.ts",
          start_line: 42,
          end_line: 45,
          label: "PATCH /admin",
        },
      ],
    },
  ],
  call_path_unavailable_reason: "No complete path.",
  legend: [
    {
      kind: "changed_code",
      label: "Changed code",
      description: "Node from the diff.",
    },
  ],
  panel: {
    claim: findingFixture.canonical_claim,
    severity: "high",
    verification_status: "verified",
    decision_status: "needs_triage",
    evidence_summary: findingFixture.evidence_summary,
    counter_evidence_summary: "",
    evidence_counts: { supporting: 1 },
    evidence: [
      {
        id: "evidence_1",
        kind: "supporting",
        title: "Route registration",
        summary: "The route bypasses the protected group.",
        path: "src/app.ts",
        start_line: 42,
        end_line: 45,
        confidence: 0.91,
      },
    ],
  },
  missing_reasons: ["No admin guard edge was found."],
};

const findingThreadViewFixture = {
  finding: findingFixture,
  thread: {
    id: "thread_1",
    finding_id: "finding_1",
    review_session_id: "session_1",
    title: "Auth middleware is not applied",
    created_at: "2026-05-04T00:00:00Z",
    updated_at: "2026-05-04T00:00:00Z",
  },
  messages: [],
};

const askFindingQuestionFixture = {
  thread: {
    ...findingThreadViewFixture,
    messages: [
      {
        id: "message_user_counter",
        thread_id: "thread_1",
        role: "user",
        content: "Can you check counter-evidence?",
        evidence_refs: [],
        created_at: "2026-05-04T00:00:00Z",
      },
      {
        id: "message_assistant_counter",
        thread_id: "thread_1",
        role: "assistant",
        agent_config_id: "agent_config_1",
        content: "No counter-evidence was found.",
        evidence_refs: [],
        created_at: "2026-05-04T00:00:00Z",
      },
    ],
  },
  user_message: {
    id: "message_user_counter",
    thread_id: "thread_1",
    role: "user",
    content: "Can you check counter-evidence?",
    evidence_refs: [],
    created_at: "2026-05-04T00:00:00Z",
  },
  assistant_message: {
    id: "message_assistant_counter",
    thread_id: "thread_1",
    role: "assistant",
    agent_config_id: "agent_config_1",
    content: "No counter-evidence was found.",
    evidence_refs: [],
    created_at: "2026-05-04T00:00:00Z",
  },
  agent_run_id: "run_counter_1",
  context_bundle_id: "bundle_counter_1",
};

const findingQuickActionFixture = {
  action: "copy",
  thread: {
    ...findingThreadViewFixture,
    finding: { ...findingFixture, decision_status: "copied" },
    messages: [
      {
        id: "message_system_copy",
        thread_id: "thread_1",
        role: "system",
        content: "Marked finding as copied. Note: sent to agent",
        evidence_refs: [],
        created_at: "2026-05-04T00:00:00Z",
      },
    ],
  },
  finding: { ...findingFixture, decision_status: "copied" },
  decision: {
    id: "decision_copy",
    finding_id: "finding_1",
    review_session_id: "session_1",
    decision: "copied",
    reason: "sent to agent",
    metadata: {},
    created_at: "2026-05-04T00:00:00Z",
  },
  message: {
    id: "message_system_copy",
    thread_id: "thread_1",
    role: "system",
    content: "Marked finding as copied. Note: sent to agent",
    evidence_refs: [],
    created_at: "2026-05-04T00:00:00Z",
  },
};

const askEvidenceMapQuestionFixture = {
  thread: findingThreadViewFixture,
  user_message: {
    id: "message_user_1",
    thread_id: "thread_1",
    role: "user",
    content: "Does this graph prove the missing guard?",
    evidence_refs: [{ node_id: "node_1" }],
    created_at: "2026-05-04T00:00:00Z",
  },
  assistant_message: {
    id: "message_assistant_1",
    thread_id: "thread_1",
    role: "assistant",
    agent_config_id: "agent_config_1",
    content: "The graph still shows a missing guard.",
    evidence_refs: [{ node_id: "node_1" }],
    created_at: "2026-05-04T00:00:00Z",
  },
  agent_run_id: "run_graph_1",
  context_bundle_id: "bundle_graph_1",
};

const copyPacketFixture = {
  copy_packet_id: "copy_packet_1",
  content: "## Auth middleware is not applied\n\nPlease add the guard.",
  format: "markdown",
  finding_count: 1,
  token_estimate: 42,
  content_artifact_id: "artifact_packet_1",
};

const copiedPacketFixture = {
  copy_packet_id: "copy_packet_1",
  copied_at: "2026-05-04T00:01:00Z",
  finding_ids: ["finding_1"],
  decisions: [
    {
      id: "decision_packet_copy",
      finding_id: "finding_1",
      review_session_id: "session_1",
      decision: "copied",
      reason: "",
      metadata: {},
      created_at: "2026-05-04T00:01:00Z",
    },
  ],
};

const githubPreviewFixture = {
  publish_draft_id: "publish_draft_1",
  artifact_id: "artifact_preview_1",
  review_event: "COMMENT",
  body: "Review body for accepted findings.",
  comments: [
    {
      finding_id: "finding_1",
      path: "src/app.ts",
      body: "This route appears to bypass auth.",
      line: 42,
      side: "RIGHT",
      position: 1,
      unanchored: false,
    },
  ],
  warnings: [],
  checklist: {
    has_selected_findings: true,
    has_inline_comments: true,
    has_unanchored_comments: false,
    can_publish_inline: true,
    can_publish_summary_only: true,
  },
};

const reviewEventFixture = {
  id: "event_3",
  review_session_id: "session_1",
  agent_run_id: "run_1",
  type: "FindingCreated",
  level: "info",
  sequence: 3,
  payload: {
    finding_id: "finding_1",
    severity: "high",
  },
  artifact_id: "artifact_1",
  created_at: "2026-05-04T00:00:00Z",
};

const agentConfigFixture = {
  id: "agent_config_1",
  name: "OpenCode",
  description: "OpenCode CLI",
  role: "primary_reviewer",
  adapter_kind: "cli_non_interactive",
  command: "opencode",
  args: ["run", "--format", "json", "{{prompt}}"],
  cwd_mode: "repo_root",
  env_allowlist: ["OPENROUTER_API_KEY"],
  output_mode: "jsonl",
  model_label: "opencode",
  reasoning_label: "",
  settings: { prompt_delivery: "arg", timeout_seconds: 1800 },
  capabilities: {
    can_read: true,
    can_cancel: true,
    supports_json: true,
    supports_streaming: true,
    output_modes: ["jsonl", "json", "text"],
    metadata: { provider: "opencode", egress: "external" },
  },
  enabled: true,
  created_at: "2026-05-04T00:00:00Z",
  updated_at: "2026-05-04T00:00:00Z",
};

const agentHealthFixture = {
  agent_config_id: "agent_config_1",
  status: "available",
  message: "opencode is available",
  checked_at: "2026-05-04T00:00:00Z",
  capabilities: agentConfigFixture.capabilities,
  metadata: {
    version: "opencode 1.0.0",
    resolved_path: "/usr/local/bin/opencode",
  },
};
