import type { AgentConfig, ReviewSessionAgent } from "@/lib/api";
import { formatSetupAgentLabel } from "../agents/agent-utils";

export function payloadString(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

export function isPlainRecord(
  value: unknown,
): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function metadataString(
  metadata: unknown,
  key: string,
  fallback = "",
): string {
  if (!isPlainRecord(metadata)) {
    return fallback;
  }
  const value = metadata[key];
  return typeof value === "string" && value.trim() ? value.trim() : fallback;
}

export function metadataNumber(metadata: unknown, key: string): number {
  if (!isPlainRecord(metadata)) {
    return Number.NaN;
  }
  const value = metadata[key];
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : Number.NaN;
}

export function formatEventLabel(type: string) {
  return type
    .replace(/([a-z])([A-Z])/g, "$1 $2")
    .replace(/^Review Session/, "Review")
    .replace(/^Workflow Phase/, "Phase")
    .replace(/^Agent Run/, "Agent");
}

export function displayNameForAuthor(authorType: string) {
  switch (authorType) {
    case "user":
      return "You";
    case "orchestrator":
      return "Orchestrator";
    case "system":
      return "System";
    case "cocode":
      return "cocode";
    default:
      return "Reviewer";
  }
}

export function formatClockTime(value: string) {
  const time = Date.parse(value);
  if (!Number.isFinite(time)) {
    return "";
  }
  return new Date(time).toLocaleTimeString([], {
    hour: "numeric",
    minute: "2-digit",
  });
}

export function compactAgentLabel(agent: AgentConfig) {
  return formatSetupAgentLabel(agent)
    .replace(/\bCLI\b/g, "")
    .replace(/\s+/g, " ")
    .trim();
}

export function compactRoleLabel(value: string) {
  return value
    .replace(/[_-]+/g, " ")
    .replace(/\s+/g, " ")
    .trim()
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

export function isOrchestratorEntry({
  assignment,
}: {
  agent: AgentConfig;
  assignment: ReviewSessionAgent;
}) {
  return isOrchestratorAssignment(assignment);
}

export function isOrchestratorAssignment(assignment: ReviewSessionAgent) {
  return assignment.role.toLowerCase().includes("orchestrator");
}

export function agentByID(agents: AgentConfig[], id?: string) {
  if (!id) {
    return undefined;
  }
  return agents.find((agent) => agent.id === id);
}

export function formatRelativeTime(value: string) {
  const time = Date.parse(value);
  if (!Number.isFinite(time)) {
    return value;
  }
  const seconds = Math.max(0, Math.round((Date.now() - time) / 1000));
  if (seconds < 60) {
    return "Just now";
  }
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) {
    return `${minutes}m ago`;
  }
  const hours = Math.round(minutes / 60);
  if (hours < 24) {
    return `${hours}h ago`;
  }
  return new Date(time).toLocaleDateString();
}
