import { describe, expect, it } from "vitest";

import type { ReviewEvent } from "@/lib/api";
import {
  appendBoundedEvent,
  compactReviewEvents,
  reviewEventRefreshScope,
} from "./review-live-data";

describe("review live data helpers", () => {
  it("deduplicates events by id or sequence and keeps sequence order", () => {
    const first = reviewEvent({ id: "event_2", sequence: 2 });
    const second = reviewEvent({ id: "event_1", sequence: 1 });
    const duplicateId = reviewEvent({
      id: "event_2",
      sequence: 3,
      type: "FindingUpdated",
    });
    const duplicateSequence = reviewEvent({
      id: "event_3",
      sequence: 1,
      type: "ReviewSessionUpdated",
    });

    const events = [first, second, duplicateId, duplicateSequence].reduce(
      appendBoundedEvent,
      [] as ReviewEvent[],
    );

    expect(events.map((event) => event.id)).toEqual(["event_1", "event_2"]);
    expect(events.map((event) => event.sequence)).toEqual([1, 2]);
  });

  it("routes streamed event refreshes to the smallest needed reload", () => {
    expect(reviewEventRefreshScope("FindingDecisionUpdated")).toEqual({
      findings: true,
      session: undefined,
    });
    expect(reviewEventRefreshScope("ReviewSessionCompleted")).toEqual({
      findings: undefined,
      session: true,
    });
    expect(reviewEventRefreshScope("AgentRunCompleted")).toEqual({
      findings: undefined,
      session: undefined,
    });
  });

  it("compacts long streams while preserving lifecycle events", () => {
    const nonAgentEvents = Array.from({ length: 1300 }, (_, index) =>
      reviewEvent({
        id: `non_agent_${index}`,
        sequence: index + 1,
        type: "Heartbeat",
      }),
    );
    const agentRunEvents = Array.from({ length: 2600 }, (_, index) =>
      reviewEvent({
        agent_run_id: "agent_run_1",
        id: `agent_run_event_${index}`,
        sequence: 2000 + index,
        type: "AgentRunOutput",
      }),
    );
    const lifecycle = reviewEvent({
      agent_run_id: "agent_run_1",
      id: "agent_run_started",
      sequence: 1,
      type: "AgentRunStarted",
    });

    const compacted = compactReviewEvents([
      lifecycle,
      ...nonAgentEvents,
      ...agentRunEvents,
    ]);

    expect(compacted).toContain(lifecycle);
    expect(
      compacted.filter((event) => event.type === "Heartbeat"),
    ).toHaveLength(1200);
    expect(
      compacted.filter((event) => event.type === "AgentRunOutput"),
    ).toHaveLength(2500);
  });
});

function reviewEvent(overrides: Partial<ReviewEvent> = {}): ReviewEvent {
  return {
    id: "event_1",
    review_session_id: "review_session_1",
    type: "AgentRunOutput",
    level: "info",
    sequence: 1,
    payload: {},
    created_at: "2026-05-22T00:00:00Z",
    ...overrides,
  };
}
