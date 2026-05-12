import { describe, expect, it, vi } from "vitest";

vi.mock("react-shiki", () => ({
  default: ({ children }: { children: string }) => children,
}));

import type { ReviewEvent } from "@/lib/api";
import { summarizeRuntimeTraceEvents } from "./agent-runtime-trace";

describe("summarizeRuntimeTraceEvents", () => {
  it("coalesces Claude stream-json text deltas and hides stop envelopes", () => {
    const summary = summarizeRuntimeTraceEvents([
      event("AgentRunStarted", { message: "command started" }),
      output({
        type: "stream_event",
        event: {
          type: "content_block_delta",
          delta: { type: "text_delta", text: "Review " },
        },
      }),
      output({
        type: "stream_event",
        event: {
          type: "content_block_delta",
          delta: { type: "text_delta", text: "complete." },
        },
      }),
      output({
        type: "stream_event",
        event: { type: "content_block_stop", index: 0 },
      }),
      event("AgentRunCompleted", { message: "command completed" }),
    ]);

    expect(summary.output).toEqual(["Review complete."]);
    expect(summary.output.join("\n")).not.toContain("content_block_stop");
    expect(summary.lifecycle).toContain("Agent Run Started: command started");
    expect(summary.lifecycle).toContain(
      "Agent Run Completed: command completed",
    );
  });

  it("extracts Codex JSONL agent messages and command tool calls", () => {
    const summary = summarizeRuntimeTraceEvents([
      output({
        type: "item.started",
        item: {
          type: "command_execution",
          command: "git diff --stat main...feature/auth",
          status: "in_progress",
        },
      }),
      output({
        type: "item.completed",
        item: {
          type: "command_execution",
          command: "git diff --stat main...feature/auth",
          status: "completed",
          aggregated_output: "src/server.js | 10 +++++-----",
        },
      }),
      output({
        type: "item.completed",
        item: {
          type: "agent_message",
          text: "Found a missing authorization check in cancelSubscription.",
        },
      }),
    ]);

    expect(summary.toolCalls.join("\n")).toContain("git diff --stat");
    expect(summary.toolCalls.join("\n")).toContain("status: completed");
    expect(summary.toolCalls.join("\n")).toContain("src/server.js");
    expect(summary.output).toEqual([
      "Found a missing authorization check in cancelSubscription.",
    ]);
  });

  it("keeps full CLI diagnostics and failed-run errors readable", () => {
    const errorText =
      "innerError Error: Cannot find module '../build/Debug/pty.node'\nRequire stack:\n- /Users/hughdo/Library/pnpm/global/5/.pnpm/node-pty@1.0.0/node_modules/node-pty/lib/unixTerminal.js";
    const summary = summarizeRuntimeTraceEvents([
      event(
        "AgentRunOutput",
        {
          stream: "stderr",
          text_preview: "Warning: True color support not detected.",
        },
        "warn",
      ),
      event("AgentRunFailed", { error: errorText, message: "command failed" }),
    ]);

    expect(summary.diagnostics).toEqual([
      "Warning: True color support not detected.",
    ]);
    expect(summary.errors.join("\n")).toContain("build/Debug/pty.node");
    expect(summary.errors.join("\n")).toContain("unixTerminal.js");
  });

  it("extracts Gemini JSON responses, thinking stats, and tool counts", () => {
    const summary = summarizeRuntimeTraceEvents([
      output({
        session_id: "gemini-session",
        response: "Found an authorization bug in src/server.js.",
        stats: {
          models: {
            "gemini-2.5-flash": {
              tokens: { thoughts: 417 },
            },
          },
          tools: {
            totalCalls: 3,
            totalFail: 1,
            byName: {
              read_file: { count: 2, fail: 0 },
              write_file: { count: 1, fail: 1 },
            },
          },
        },
      }),
    ]);

    expect(summary.output).toEqual([
      "Found an authorization bug in src/server.js.",
    ]);
    expect(summary.reasoning.join("\n")).toContain("417 thinking token");
    expect(summary.toolCalls.join("\n")).toContain("total calls: 3");
    expect(summary.toolCalls.join("\n")).toContain(
      "write_file: 1 call(s), 1 failed",
    );
  });

  it("coalesces Gemini pretty JSON lines before extracting stats", () => {
    const summary = summarizeRuntimeTraceEvents([
      outputLine("{"),
      outputLine('"session_id": "gemini-session",'),
      outputLine('"response": "Found an authorization bug.",'),
      outputLine('"stats": {'),
      outputLine('"models": {'),
      outputLine('"gemini-3-pro": { "tokens": { "thoughts": 288 } }'),
      outputLine("},"),
      outputLine('"tools": { "totalCalls": 2, "totalFail": 0, "byName": {} }'),
      outputLine("}"),
      outputLine("}"),
    ]);

    expect(summary.output).toEqual(["Found an authorization bug."]);
    expect(summary.reasoning.join("\n")).toContain("288 thinking token");
    expect(summary.toolCalls.join("\n")).toContain("total calls: 2");
  });

  it("extracts OpenCode visible thinking and model text without raw envelopes", () => {
    const summary = summarizeRuntimeTraceEvents([
      output({
        type: "step_start",
        part: { type: "step-start" },
      }),
      output({
        type: "reasoning",
        part: {
          type: "reasoning",
          text: "I checked the requested branch diff and auth-sensitive routes.",
        },
      }),
      output({
        type: "text",
        part: {
          type: "text",
          text: "Found a missing admin check in cancelSubscription.",
        },
      }),
      output({
        type: "tool_use",
        part: {
          type: "tool",
          tool: "bash",
          state: {
            status: "completed",
            input: { command: "git diff main..feature/auth-bug" },
            output: "diff --git a/src/server.js b/src/server.js",
          },
        },
      }),
      output({
        type: "step_finish",
        part: {
          type: "step-finish",
          reason: "stop",
          tokens: { total: 22058 },
          cost: 0,
        },
      }),
    ]);

    expect(summary.reasoning).toEqual([
      "I checked the requested branch diff and auth-sensitive routes.",
    ]);
    expect(summary.output).toEqual([
      "Found a missing admin check in cancelSubscription.",
    ]);
    expect(summary.lifecycle.join("\n")).toContain("OpenCode step started");
    expect(summary.lifecycle.join("\n")).toContain("tokens: 22058");
    expect(summary.toolCalls.join("\n")).toContain(
      "git diff main..feature/auth-bug",
    );
    expect(summary.toolCalls.join("\n")).toContain("status: completed");
    expect(summary.output.join("\n")).not.toContain("step_finish");
  });

  it("formats structured finding JSON as readable model output", () => {
    const summary = summarizeRuntimeTraceEvents([
      output({
        type: "result",
        subtype: "success",
        result:
          'Here is the review.\n```json\n{"findings":[{"severity":"critical","category":"security","title":"Missing admin check","file":"src/server.js","line":10,"description":"Any authenticated user can cancel a subscription.","suggested_fix":"Call requireAdmin before the DB mutation."}]}\n```',
      }),
    ]);

    expect(summary.output.join("\n")).toContain("Findings (1)");
    expect(summary.output.join("\n")).toContain("Missing admin check");
    expect(summary.output.join("\n")).toContain(
      "- **Location:** `src/server.js:10`",
    );
    expect(summary.output.join("\n")).toContain("- **Suggested fix:**");
    expect(summary.output.join("\n")).not.toContain('\\"findings\\"');
  });

  it("prefers final structured output over fragmented stream deltas", () => {
    const summary = summarizeRuntimeTraceEvents([
      output({
        type: "stream_event",
        event: {
          type: "content_block_delta",
          delta: { type: "text_delta", text: '```json\n{"find' },
        },
      }),
      output({
        type: "stream_event",
        event: {
          type: "content_block_delta",
          delta: { type: "text_delta", text: 'ings":[]}\n```' },
        },
      }),
      output({
        type: "result",
        subtype: "success",
        result:
          '{"findings":[{"severity":"high","category":"security","title":"Missing admin check","file":"src/server.js","line":10,"description":"Mutation is reachable without admin authorization."}]}',
      }),
    ]);

    expect(summary.output).toHaveLength(1);
    expect(summary.output[0]).toContain("Findings (1)");
    expect(summary.output[0]).toContain("Missing admin check");
    expect(summary.output[0]).not.toContain("```json");
  });

  it("suppresses machine hook envelopes from trace output", () => {
    const summary = summarizeRuntimeTraceEvents([
      output({
        type: "system",
        subtype: "hook_started",
        hook_name: "SessionStart:startup",
        hook_event: "SessionStart",
        session_id: "session_1",
      }),
    ]);

    expect(summary.output).toEqual([]);
    expect(summary.reasoning).toEqual([]);
    expect(summary.toolCalls).toEqual([]);
  });

  it("keeps artifact events visible in run state", () => {
    const summary = summarizeRuntimeTraceEvents([
      {
        ...event("AgentRunArtifact", {
          artifact_id: "artifact_stdout_1",
          message: "stdout captured",
        }),
        artifact_id: "artifact_stdout_1",
      },
    ]);

    expect(summary.eventCount).toBe(1);
    expect(summary.lifecycle.join("\n")).toContain("Artifact saved");
    expect(summary.lifecycle.join("\n")).toContain("artifact_stdout_1");
    expect(summary.lifecycle.join("\n")).toContain("stdout captured");
  });
});

function output(payload: unknown) {
  return event("AgentRunOutput", {
    stream: "stdout",
    text_preview: `${JSON.stringify(payload)}\n`,
  });
}

function outputLine(line: string) {
  return event("AgentRunOutput", {
    stream: "stdout",
    text_preview: line,
  });
}

function event(
  type: string,
  payload: Record<string, unknown>,
  level = "info",
): ReviewEvent {
  return {
    id: `${type}-${Math.random()}`,
    review_session_id: "review_session_1",
    agent_run_id: "agent_run_1",
    type,
    level,
    sequence: 1,
    payload,
    created_at: "2026-05-11T00:00:00Z",
  };
}
