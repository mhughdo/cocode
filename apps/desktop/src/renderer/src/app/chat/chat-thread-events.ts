import type { ChatMessage, ChatThreadView, ReviewEvent } from "@/lib/api";

export function applyChatEventsToThread(
  view: ChatThreadView,
  events: ReviewEvent[],
): ChatThreadView {
  let next = view;
  for (const event of events) {
    next = applyChatEventToThread(next, event);
  }
  return next;
}

export function applyChatEventToThread(
  view: ChatThreadView,
  event: ReviewEvent,
): ChatThreadView {
  switch (event.type) {
    case "ChatMessageCreated":
    case "ChatMessageUpdated": {
      const message = messageFromPayload(event.payload.message);
      if (!message || message.thread_id !== view.thread.id) {
        return view;
      }
      return upsertMessage(view, message);
    }
    case "ChatMessageDelta": {
      const messageID = stringFromUnknown(event.payload.message_id);
      const delta = rawStringFromUnknown(event.payload.text_delta);
      if (!messageID || !delta) {
        return view;
      }
      return appendMessageDelta(view, messageID, delta, event.created_at);
    }
    default:
      return view;
  }
}

export function isChatThreadEvent(event: ReviewEvent) {
  return (
    event.type === "ChatMessageCreated" ||
    event.type === "ChatMessageUpdated" ||
    event.type === "ChatMessageDelta" ||
    event.type === "ChatTurnStatusChanged"
  );
}

export function chatEventTurnStatus(event: ReviewEvent) {
  if (event.type !== "ChatTurnStatusChanged") {
    return null;
  }
  const id = stringFromUnknown(event.payload.chat_turn_id);
  const status = stringFromUnknown(event.payload.status);
  return id && status ? { id, status } : null;
}

function upsertMessage(view: ChatThreadView, incoming: ChatMessage) {
  const index = view.messages.findIndex((message) => message.id === incoming.id);
  if (index === -1) {
    return sortThreadMessages({
      ...view,
      messages: [...view.messages, incoming],
    });
  }
  const existing = view.messages[index];
  if (existing && !incomingIsNewer(existing, incoming)) {
    return view;
  }
  const messages = [...view.messages];
  messages[index] = incoming;
  return sortThreadMessages({ ...view, messages });
}

function appendMessageDelta(
  view: ChatThreadView,
  messageID: string,
  delta: string,
  updatedAt: string,
) {
  const index = view.messages.findIndex((message) => message.id === messageID);
  if (index === -1) {
    return view;
  }
  const existing = view.messages[index];
  if (!existing || existing.status !== "streaming") {
    return view;
  }
  const messages = [...view.messages];
  messages[index] = {
    ...existing,
    body: existing.body + delta,
    updated_at: updatedAt || existing.updated_at,
  };
  return { ...view, messages };
}

function sortThreadMessages(view: ChatThreadView) {
  return {
    ...view,
    messages: [...view.messages].sort((left, right) => {
      const byTime = timestamp(left.created_at) - timestamp(right.created_at);
      return byTime === 0 ? left.id.localeCompare(right.id) : byTime;
    }),
  };
}

function incomingIsNewer(existing: ChatMessage, incoming: ChatMessage) {
  const existingTime = timestamp(existing.updated_at || existing.created_at);
  const incomingTime = timestamp(incoming.updated_at || incoming.created_at);
  if (incomingTime === existingTime) {
    return incoming.status !== "streaming" || existing.status === "streaming";
  }
  return incomingTime > existingTime;
}

function messageFromPayload(value: unknown): ChatMessage | null {
  if (!isRecord(value)) {
    return null;
  }
  const id = stringFromUnknown(value.id);
  const threadID = stringFromUnknown(value.thread_id);
  const authorType = stringFromUnknown(value.author_type);
  const displayName = stringFromUnknown(value.author_display_name);
  const body = rawStringFromUnknown(value.body);
  const status = stringFromUnknown(value.status);
  const createdAt = stringFromUnknown(value.created_at);
  const updatedAt = stringFromUnknown(value.updated_at);
  if (
    !id ||
    !threadID ||
    !authorType ||
    !displayName ||
    !status ||
    !createdAt ||
    !updatedAt
  ) {
    return null;
  }
  return {
    id,
    thread_id: threadID,
    parent_message_id: optionalString(value.parent_message_id),
    author_type: authorType,
    author_display_name: displayName,
    agent_config_id: optionalString(value.agent_config_id),
    agent_run_id: optionalString(value.agent_run_id),
    context_bundle_id: optionalString(value.context_bundle_id),
    artifact_id: optionalString(value.artifact_id),
    body,
    status,
    metadata: value.metadata ?? {},
    created_at: createdAt,
    updated_at: updatedAt,
  };
}

function optionalString(value: unknown) {
  return stringFromUnknown(value) || undefined;
}

function stringFromUnknown(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

function rawStringFromUnknown(value: unknown) {
  return typeof value === "string" ? value : "";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function timestamp(value: string) {
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : 0;
}
