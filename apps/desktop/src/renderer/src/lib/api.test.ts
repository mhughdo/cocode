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
