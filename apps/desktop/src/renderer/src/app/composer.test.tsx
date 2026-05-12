import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("react-shiki", () => ({
  default: ({
    children,
    startingLineNumber,
  }: {
    children: string;
    startingLineNumber?: number;
  }) => (
    <pre>
      {startingLineNumber ? <span>{startingLineNumber}</span> : null}
      <code>{children}</code>
    </pre>
  ),
}));

import { composerContextPolicy, MessageComposer } from "./App";
import type { AgentConfig } from "@/lib/api";

const reviewerAgent = {
  id: "agent_codex",
  name: "Codex Reviewer",
  role: "reviewer",
  adapter_kind: "cli_noninteractive",
  command: "codex",
  args: ["exec", "--json"],
  cwd_mode: "repo_root",
  env_allowlist: [],
  output_mode: "json",
  model_label: "gpt-5.5",
  reasoning_label: "high",
  settings: {},
  capabilities: {
    can_read: true,
    can_write: false,
    metadata: { provider: "openai", egress: "external" },
  },
  enabled: true,
  created_at: "2026-05-04T00:00:00Z",
  updated_at: "2026-05-04T00:00:00Z",
} satisfies AgentConfig;

describe("MessageComposer", () => {
  it("turns runtime and permission controls into a scoped context policy", () => {
    expect(composerContextPolicy("quick", "review-mode")).toEqual({
      max_tokens: 4000,
      max_items: 40,
      redact_secrets: true,
      include_prior_comments: true,
      include_prior_decisions: true,
    });
    expect(composerContextPolicy("deep", "local-only")).toEqual({
      max_tokens: 12000,
      max_items: 120,
      redact_secrets: true,
      include_prior_comments: false,
      include_prior_decisions: true,
    });
  });

  it("renders an enabled finding-scoped composer with model controls", () => {
    const html = renderToStaticMarkup(
      <MessageComposer
        agents={[reviewerAgent]}
        defaultMode="finding follow-up"
        onQuestionChange={vi.fn()}
        onSelectedAgentIdChange={vi.fn()}
        onSubmit={vi.fn()}
        question="Can you verify the route guard?"
        selectedAgentId="agent_codex"
      />,
    );

    expect(html).toContain("Tool: finding follow-up");
    expect(html).toContain("Codex Reviewer GPT 5.5");
    expect(html).toContain("Context: standard");
    expect(html).toContain("Reasoning: high");
    expect(html).toContain("Permission: review-mode");
    expect(html).toContain('aria-label="Send follow-up question"');
  });

  it("renders a disabled review footer when no backed endpoint is available", () => {
    const html = renderToStaticMarkup(
      <MessageComposer
        agents={[reviewerAgent]}
        disabled
        disabledReason="Open Follow-up from a selected finding."
      />,
    );

    expect(html).toContain("Open Follow-up from a selected finding.");
    expect(html).toContain("disabled");
  });
});
