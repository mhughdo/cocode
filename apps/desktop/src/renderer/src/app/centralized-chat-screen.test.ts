import { describe, expect, it } from "vitest";

import type {
  AgentConfig,
  ChatMessage,
  ReviewEvent,
  ReviewSession,
  ReviewSessionSummary,
} from "@/lib/api";
import { successApiState } from "@/lib/api";

import { withLiveAgentRunMessages } from "./centralized-chat-screen";

describe("withLiveAgentRunMessages", () => {
  it("keeps completed orchestrator runs visible when no chat row was persisted", () => {
    const messages = withLiveAgentRunMessages({
      agentConfigs: successApiState([orchestratorConfig]),
      events: [
        reviewEvent("event_1", "AgentRunOutput", {
          text_preview:
            '{"type":"item.completed","item":{"type":"agent_message","text":"Curated the findings into one verified issue."}}',
        }),
        reviewEvent("event_2", "AgentRunCompleted", {
          message: "command completed",
        }),
      ],
      messages: [],
      session: reviewSession,
      summary: successApiState({
        ...summaryFixture,
        agent_runs: [
          {
            id: "agent_run_orchestrator",
            review_session_id: reviewSession.id,
            agent_config_id: orchestratorConfig.id,
            status: "succeeded",
            role: "orchestrator",
            started_at: "2026-05-14T02:00:00Z",
            completed_at: "2026-05-14T02:01:00Z",
          },
        ],
      }),
      threadID: "thread_1",
    });

    expect(messages).toHaveLength(1);
    expect(messages[0]).toMatchObject({
      agent_run_id: "agent_run_orchestrator",
      author_type: "orchestrator",
      status: "completed",
    });
    expect(messages[0]?.body).toContain("Curated the findings");
  });

  it("does not render Codex command execution JSON as the completed message body", () => {
    const messages = withLiveAgentRunMessages({
      agentConfigs: successApiState([orchestratorConfig]),
      events: [
        reviewEvent("event_1", "AgentRunOutput", {
          text_preview: JSON.stringify({
            type: "item.completed",
            item: {
              id: "item_27",
              type: "command_execution",
              command: "/bin/zsh -lc rg -n Price",
              aggregated_output: "internal/app/fetcher.go:211:return prices[1]",
            },
          }),
        }),
        reviewEvent("event_2", "AgentRunCompleted", {
          message: "command completed",
        }),
      ],
      messages: [],
      session: reviewSession,
      summary: successApiState({
        ...summaryFixture,
        agent_runs: [
          {
            id: "agent_run_orchestrator",
            review_session_id: reviewSession.id,
            agent_config_id: orchestratorConfig.id,
            status: "succeeded",
            role: "orchestrator",
            completed_at: "2026-05-14T02:01:00Z",
          },
        ],
      }),
      threadID: "thread_1",
    });

    expect(messages[0]?.body).toBe(
      "cocode finished synthesizing the review findings and evidence map.",
    );
    expect(messages[0]?.body).not.toContain("command_execution");
    expect(messages[0]?.body).not.toContain("aggregated_output");
  });

  it("formats verifier JSON output into readable markdown", () => {
    const messages = withLiveAgentRunMessages({
      agentConfigs: successApiState([orchestratorConfig]),
      events: [
        reviewEvent("event_1", "AgentRunOutput", {
          text_preview: JSON.stringify({
            verification_status: "verified",
            evidence_summary:
              "The panic condition is confirmed at the dereference site.",
            counter_evidence_summary:
              "No guard was found that guarantees prices[1] is populated.",
            confidence: 0.91,
            evidence: [
              {
                kind: "supporting",
                title: "Unchecked prices[1] dereference",
                summary: "The function averages prices[0] with prices[1].",
                path: "internal/app/fetcher.go",
                start_line: 203,
                end_line: 211,
              },
            ],
          }),
        }),
        reviewEvent("event_2", "AgentRunCompleted", {
          message: "command completed",
        }),
      ],
      messages: [],
      session: reviewSession,
      summary: successApiState({
        ...summaryFixture,
        agent_runs: [
          {
            id: "agent_run_orchestrator",
            review_session_id: reviewSession.id,
            agent_config_id: orchestratorConfig.id,
            status: "succeeded",
            role: "verifier",
            completed_at: "2026-05-14T02:01:00Z",
          },
        ],
      }),
      threadID: "thread_1",
    });

    expect(messages[0]?.body).toContain("### Verification: Verified");
    expect(messages[0]?.body).toContain("**Confidence:** 91%");
    expect(messages[0]?.body).toContain("The panic condition is confirmed");
    expect(messages[0]?.body).toContain("Unchecked prices[1] dereference");
    expect(messages[0]?.body).toContain("internal/app/fetcher.go:L203-L211");
    expect(messages[0]?.body).not.toContain('"verification_status"');
  });

  it("formats orchestrator cluster JSON output into readable markdown", () => {
    const messages = withLiveAgentRunMessages({
      agentConfigs: successApiState([orchestratorConfig]),
      events: [
        reviewEvent("event_1", "AgentRunOutput", {
          text_preview: JSON.stringify({
            clusters: [
              {
                canonical_claim:
                  "pickTokenPrice can panic when GetPrices returns a nil sell-price slot.",
                category: "reliability",
                severity: "medium",
                confidence: 0.93,
                verification_status: "verified",
                primary_location: {
                  path: "internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go",
                  start_line: 208,
                  end_line: 208,
                },
                evidence_summary:
                  "The helper checks prices[0] before averaging both price slots.",
                counter_evidence_summary: "none verified",
                supporting_evidence: [
                  {
                    kind: "supporting",
                    title: "Unchecked sell-price dereference",
                    summary: "The code dereferences prices[1].",
                    path: "internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go",
                    start_line: 207,
                    end_line: 208,
                  },
                ],
                relationship_evidence: [
                  {
                    kind: "static_analysis",
                    title: "fetchRewardTokenInfo caller",
                    summary: "The caller reaches pickTokenPrice.",
                    path: "internal/app/aggregatedposition/fetcher/kyberdata/fetcher.go",
                    start_line: 389,
                    end_line: 389,
                  },
                ],
                suggested_fix: "Guard both price slots before averaging.",
              },
            ],
          }),
        }),
        reviewEvent("event_2", "AgentRunCompleted", {
          message: "command completed",
        }),
      ],
      messages: [],
      session: reviewSession,
      summary: successApiState({
        ...summaryFixture,
        agent_runs: [
          {
            id: "agent_run_orchestrator",
            review_session_id: reviewSession.id,
            agent_config_id: orchestratorConfig.id,
            status: "succeeded",
            role: "orchestrator",
            completed_at: "2026-05-14T02:01:00Z",
          },
        ],
      }),
      threadID: "thread_1",
    });

    expect(messages[0]?.body).toContain("## Findings (1)");
    expect(messages[0]?.body).toContain(
      "pickTokenPrice can panic when GetPrices returns a nil sell-price slot.",
    );
    expect(messages[0]?.body).toContain("**Status:** Verified");
    expect(messages[0]?.body).toContain("**Confidence:** 93%");
    expect(messages[0]?.body).toContain(
      "internal/app/aggregatedposition/fetcher/kyberdata/kem_rewards.go:208",
    );
    expect(messages[0]?.body).toContain("Unchecked sell-price dereference");
    expect(messages[0]?.body).toContain("fetchRewardTokenInfo caller");
    expect(messages[0]?.body).not.toContain('"clusters"');
  });

  it("does not duplicate completed runs that already have persisted messages", () => {
    const persisted: ChatMessage = {
      id: "message_1",
      thread_id: "thread_1",
      author_type: "orchestrator",
      author_display_name: "cocode",
      agent_config_id: orchestratorConfig.id,
      agent_run_id: "agent_run_orchestrator",
      body: "Already stored.",
      status: "completed",
      metadata: {},
      created_at: "2026-05-14T02:01:00Z",
      updated_at: "2026-05-14T02:01:00Z",
    };

    const messages = withLiveAgentRunMessages({
      agentConfigs: successApiState([orchestratorConfig]),
      events: [],
      messages: [persisted],
      session: reviewSession,
      summary: successApiState({
        ...summaryFixture,
        agent_runs: [
          {
            id: "agent_run_orchestrator",
            review_session_id: reviewSession.id,
            agent_config_id: orchestratorConfig.id,
            status: "succeeded",
            role: "orchestrator",
          },
        ],
      }),
      threadID: "thread_1",
    });

    expect(messages).toEqual([persisted]);
  });

  it("keeps planned agents below the queued progress message", () => {
    const runningSession = reviewSessionWithReviewer("running");
    const messagesWithoutProgress = withLiveAgentRunMessages({
      agentConfigs: successApiState([orchestratorConfig, reviewerConfig]),
      events: [],
      messages: [],
      session: runningSession,
      summary: successApiState({ ...summaryFixture, status: "running" }),
      threadID: "thread_1",
    });

    expect(messagesWithoutProgress).toHaveLength(0);

    const messages = withLiveAgentRunMessages({
      agentConfigs: successApiState([orchestratorConfig, reviewerConfig]),
      events: [
        sessionEvent("event_queued", "ReviewSessionQueued", {
          status: "queued",
        }),
      ],
      messages: [],
      session: runningSession,
      summary: successApiState({ ...summaryFixture, status: "running" }),
      threadID: "thread_1",
    });

    expect(messages.map((message) => message.body)).toEqual([
      "Review queued. cocode is preparing context and assigning the selected reviewers.",
      "Codex GPT-5.5 GPT 5.5 is queued for an execution slot.",
    ]);
  });

  it("anchors live agent cards to the run start time instead of the latest stream event", () => {
    const runningSession = reviewSessionWithReviewer("running");
    const firstPass = withLiveAgentRunMessages({
      agentConfigs: successApiState([orchestratorConfig, reviewerConfig]),
      events: [
        sessionEvent("event_started", "ReviewSessionStarted", {
          status: "running",
        }),
        agentEvent("event_output_1", "agent_run_reviewer", "AgentRunOutput", {
          text_preview: "Checking changed files.",
        }, "2026-05-14T02:04:00Z"),
      ],
      messages: [],
      session: runningSession,
      summary: successApiState({
        ...summaryFixture,
        status: "running",
        agent_runs: [
          {
            id: "agent_run_reviewer",
            review_session_id: reviewSession.id,
            agent_config_id: reviewerConfig.id,
            status: "running",
            role: "primary_reviewer",
            started_at: "2026-05-14T02:02:00Z",
          },
        ],
      }),
      threadID: "thread_1",
    });
    const secondPass = withLiveAgentRunMessages({
      agentConfigs: successApiState([orchestratorConfig, reviewerConfig]),
      events: [
        sessionEvent("event_started", "ReviewSessionStarted", {
          status: "running",
        }),
        agentEvent("event_output_1", "agent_run_reviewer", "AgentRunOutput", {
          text_preview: "Checking changed files.",
        }, "2026-05-14T02:04:00Z"),
        agentEvent("event_output_2", "agent_run_reviewer", "AgentRunOutput", {
          text_preview: "Still checking changed files.",
        }, "2026-05-14T02:09:00Z"),
      ],
      messages: [],
      session: runningSession,
      summary: successApiState({
        ...summaryFixture,
        status: "running",
        agent_runs: [
          {
            id: "agent_run_reviewer",
            review_session_id: reviewSession.id,
            agent_config_id: reviewerConfig.id,
            status: "running",
            role: "primary_reviewer",
            started_at: "2026-05-14T02:02:00Z",
          },
        ],
      }),
      threadID: "thread_1",
    });

    expect(firstPass.find((message) => message.agent_run_id)?.created_at).toBe(
      "2026-05-14T02:02:00Z",
    );
    expect(secondPass.find((message) => message.agent_run_id)?.created_at).toBe(
      "2026-05-14T02:02:00Z",
    );
    expect(secondPass.map((message) => message.author_display_name)).toEqual([
      "Orchestrator",
      "Codex GPT-5.5 GPT 5.5",
    ]);
  });

  it("uses the selected Kiro model instead of the preset auto label", () => {
    const kiroConfig: AgentConfig = {
      ...reviewerConfig,
      id: "agent_config_kiro",
      name: "Kiro",
      command: "kiro-cli",
      model_label: "auto",
      settings: { model_label: "auto" },
      capabilities: {
        can_read: true,
        supports_json: false,
        metadata: { provider: "kiro", egress: "external" },
      },
    };
    const session: ReviewSession = {
      ...reviewSession,
      agents: [
        {
          id: "review_session_agent_kiro",
          review_session_id: reviewSession.id,
          agent_config_id: kiroConfig.id,
          role: "primary_reviewer",
          run_order: 1,
          enabled: true,
          settings_override: { model_label: "claude-opus-4.7" },
        },
      ],
    };

    const messages = withLiveAgentRunMessages({
      agentConfigs: successApiState([kiroConfig]),
      events: [
        agentEvent("event_output_1", "agent_run_kiro", "AgentRunOutput", {
          text_preview: "Reviewed changed files.",
        }, "2026-05-14T02:00:30Z"),
      ],
      messages: [],
      session,
      summary: successApiState({
        ...summaryFixture,
        agent_runs: [
          {
            id: "agent_run_kiro",
            review_session_id: reviewSession.id,
            agent_config_id: kiroConfig.id,
            review_session_agent_id: "review_session_agent_kiro",
            status: "running",
            role: "primary_reviewer",
            model_label: "claude-opus-4.7",
            started_at: "2026-05-14T02:00:00Z",
          },
        ],
      }),
      threadID: "thread_1",
    });

    expect(messages[0]).toMatchObject({
      author_display_name: "Kiro Claude Opus 4.7",
      metadata: { model_label: "Claude Opus 4.7" },
    });
  });

  it("shows orchestrator enrichment progress from workflow phase events", () => {
    const messages = withLiveAgentRunMessages({
      agentConfigs: successApiState([orchestratorConfig]),
      events: [
        sessionEvent("event_verify", "WorkflowPhaseStarted", {
          phase: "verify_findings",
        }),
        sessionEvent("event_evidence", "WorkflowPhaseStarted", {
          phase: "build_evidence_maps",
        }),
      ],
      messages: [],
      session: reviewSessionWithReviewer("running"),
      summary: successApiState({ ...summaryFixture, status: "running" }),
      threadID: "thread_1",
    });

    expect(messages.map((message) => message.body)).toEqual([
      "Orchestrator is re-checking each finding against code evidence and counter-evidence.",
      "Orchestrator is enriching findings with evidence maps and source context.",
    ]);
  });
});

function reviewEvent(
  id: string,
  type: ReviewEvent["type"],
  payload: Record<string, unknown>,
): ReviewEvent {
  return {
    id,
    review_session_id: reviewSession.id,
    agent_run_id: "agent_run_orchestrator",
    type,
    level: "info",
    sequence: id === "event_1" ? 1 : 2,
    payload,
    created_at: "2026-05-14T02:01:00Z",
  };
}

function sessionEvent(
  id: string,
  type: ReviewEvent["type"],
  payload: Record<string, unknown>,
): ReviewEvent {
  return {
    id,
    review_session_id: reviewSession.id,
    type,
    level: "info",
    sequence: id === "event_evidence" ? 2 : 1,
    payload,
    created_at:
      id === "event_evidence" ? "2026-05-14T02:02:00Z" : "2026-05-14T02:01:00Z",
  };
}

function agentEvent(
  id: string,
  agentRunID: string,
  type: ReviewEvent["type"],
  payload: Record<string, unknown>,
  createdAt: string,
): ReviewEvent {
  return {
    id,
    review_session_id: reviewSession.id,
    agent_run_id: agentRunID,
    type,
    level: "info",
    sequence: id === "event_output_2" ? 3 : 2,
    payload,
    created_at: createdAt,
  };
}

const orchestratorConfig: AgentConfig = {
  id: "agent_config_orchestrator",
  name: "cocode",
  role: "orchestrator",
  adapter_kind: "cli_noninteractive",
  command: "cocode",
  args: [],
  cwd_mode: "repo_root",
  env_allowlist: [],
  output_mode: "json",
  model_label: "sonnet",
  settings: {},
  capabilities: { can_read: true, supports_json: true },
  enabled: true,
  created_at: "2026-05-14T01:00:00Z",
  updated_at: "2026-05-14T01:00:00Z",
};

const reviewerConfig: AgentConfig = {
  id: "agent_config_reviewer",
  name: "Codex GPT-5.5",
  role: "primary_reviewer",
  adapter_kind: "cli_noninteractive",
  command: "codex",
  args: [],
  cwd_mode: "repo_root",
  env_allowlist: [],
  output_mode: "json",
  model_label: "gpt-5.5",
  settings: {},
  capabilities: { can_read: true, supports_json: true },
  enabled: true,
  created_at: "2026-05-14T01:00:00Z",
  updated_at: "2026-05-14T01:00:00Z",
};

function reviewSessionWithReviewer(status: ReviewSession["status"]): ReviewSession {
  return {
    ...reviewSession,
    status,
    agents: [
      ...reviewSession.agents,
      {
        id: "review_session_agent_2",
        review_session_id: "review_session_1",
        agent_config_id: reviewerConfig.id,
        role: "primary_reviewer",
        run_order: 1,
        enabled: true,
        settings_override: {},
      },
    ],
  };
}

const reviewSession: ReviewSession = {
  id: "review_session_1",
  workspace_id: "workspace_1",
  repository_id: "repo_1",
  snapshot_id: "snapshot_1",
  title: "Review",
  status: "completed",
  review_depth: "standard",
  context_policy: {},
  runtime_limit_seconds: 900,
  agents: [
    {
      id: "review_session_agent_1",
      review_session_id: "review_session_1",
      agent_config_id: orchestratorConfig.id,
      role: "orchestrator",
      run_order: 0,
      enabled: true,
      settings_override: {},
    },
  ],
  created_at: "2026-05-14T01:00:00Z",
  updated_at: "2026-05-14T02:01:00Z",
};

const summaryFixture: ReviewSessionSummary = {
  review_session_id: reviewSession.id,
  status: "completed",
  progress_percent: 100,
  changed_files_total: 1,
  changed_files_scanned: 1,
  agent_runs_total: 1,
  active_agents: 0,
  agent_status_counts: { succeeded: 1 },
  agent_runs: [],
};
