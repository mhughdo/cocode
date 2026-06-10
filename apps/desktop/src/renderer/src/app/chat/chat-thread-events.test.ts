import { describe, expect, it } from "vitest";

import type { ChatMessage, ChatThreadView, ReviewEvent } from "@/lib/api";

import { applyChatEventsToThread } from "./chat-thread-events";

describe("applyChatEventsToThread", () => {
  it("applies full durable deltas to streaming messages", () => {
    const view = applyChatEventsToThread(emptyThread(), [
      chatMessageEvent("ChatMessageCreated", {
        ...message("chat_message_1"),
        body: "",
        status: "streaming",
      }),
      reviewEvent("ChatMessageDelta", {
        message_id: "chat_message_1",
        text_delta: "first chunk ",
      }),
      reviewEvent("ChatMessageDelta", {
        message_id: "chat_message_1",
        text_delta: "second chunk",
      }),
    ]);

    expect(view.messages).toHaveLength(1);
    expect(view.messages[0]?.body).toBe("first chunk second chunk");
    expect(view.messages[0]?.status).toBe("streaming");
  });

  it("keeps final persisted answer over stale created or delta events", () => {
    const existing = message("chat_message_1", {
      body: "Final parsed answer",
      status: "completed",
      updated_at: "2026-05-03T00:00:05Z",
    });
    const view = applyChatEventsToThread(
      { ...emptyThread(), messages: [existing] },
      [
        chatMessageEvent("ChatMessageCreated", {
          ...message("chat_message_1", {
            body: "",
            status: "streaming",
            updated_at: "2026-05-03T00:00:01Z",
          }),
        }),
        reviewEvent("ChatMessageDelta", {
          message_id: "chat_message_1",
          text_delta: "raw stdout",
        }),
      ],
    );

    expect(view.messages[0]?.body).toBe("Final parsed answer");
    expect(view.messages[0]?.status).toBe("completed");
  });

  it("replaces the streaming body when the final update arrives", () => {
    const view = applyChatEventsToThread(emptyThread(), [
      chatMessageEvent("ChatMessageCreated", {
        ...message("chat_message_1", {
          body: "",
          status: "streaming",
          updated_at: "2026-05-03T00:00:01Z",
        }),
      }),
      reviewEvent("ChatMessageDelta", {
        message_id: "chat_message_1",
        text_delta: '{"raw":"provider event"}',
      }),
      chatMessageEvent("ChatMessageUpdated", {
        ...message("chat_message_1", {
          body: "Parsed Markdown answer",
          status: "completed",
          updated_at: "2026-05-03T00:00:03Z",
        }),
      }),
    ]);

    expect(view.messages[0]?.body).toBe("Parsed Markdown answer");
    expect(view.messages[0]?.status).toBe("completed");
  });
});

function emptyThread(): ChatThreadView {
  return {
    thread: {
      id: "chat_thread_1",
      review_session_id: "review_session_1",
      title: "Review",
      status: "active",
      created_at: "2026-05-03T00:00:00Z",
      updated_at: "2026-05-03T00:00:00Z",
    },
    messages: [],
  };
}

function message(id: string, overrides: Partial<ChatMessage> = {}): ChatMessage {
  return {
    id,
    thread_id: "chat_thread_1",
    author_type: "agent",
    author_display_name: "Codex CLI",
    agent_config_id: "agent_config_1",
    body: "body",
    status: "completed",
    metadata: {},
    created_at: "2026-05-03T00:00:01Z",
    updated_at: "2026-05-03T00:00:01Z",
    ...overrides,
  };
}

function chatMessageEvent(
  type: "ChatMessageCreated" | "ChatMessageUpdated",
  chatMessage: ChatMessage,
): ReviewEvent {
  return reviewEvent(type, {
    thread_id: chatMessage.thread_id,
    message_id: chatMessage.id,
    message: chatMessage,
  });
}

function reviewEvent(type: string, payload: Record<string, unknown>): ReviewEvent {
  return {
    id: `event_${type}_${Math.random()}`,
    review_session_id: "review_session_1",
    type,
    level: "info",
    sequence: 1,
    payload,
    created_at: "2026-05-03T00:00:02Z",
  };
}
