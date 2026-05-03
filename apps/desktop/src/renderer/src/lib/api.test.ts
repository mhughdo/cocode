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
      "GET http://127.0.0.1:17658/api/review-sessions/session_1/events?after_sequence=2",
    ]);
    expect(seen[5]?.body).toEqual({
      decision: "accepted",
      reason: "verified from board",
    });
    expect(seen[6]?.body).toEqual({
      draft_comment: "Please add the missing middleware.",
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
