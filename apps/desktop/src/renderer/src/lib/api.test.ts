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
