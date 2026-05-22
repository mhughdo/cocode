import type {
  AgentConfig,
  AgentRunSummary,
  ChatMessage,
  Loadable,
  ReviewEvent,
  ReviewSession,
  ReviewSessionAgent,
  ReviewSessionSummary,
} from "@/lib/api";

import { modelIDDisplayLabel } from "../agents/agent-utils";
import { extractDisplayableAgentOutput } from "./agent-output-formatting";
import {
  summarizeRuntimeTraceEvents,
  type RuntimeTraceSummary,
} from "./agent-runtime-trace";
import {
  compactAgentLabel,
  compactRoleLabel,
  isOrchestratorAssignment,
  isOrchestratorEntry,
  isPlainRecord,
  metadataNumber,
  metadataString,
  payloadString,
} from "./chat-message-utils";
import type { ChatAudience, ChatResponderOption } from "./chat-types";

export function withLiveAgentRunMessages({
  agentConfigs,
  events,
  messages,
  session,
  summary,
  threadID,
}: {
  agentConfigs: Loadable<AgentConfig[]>;
  events: ReviewEvent[];
  messages: ChatMessage[];
  session: ReviewSession;
  summary: Loadable<ReviewSessionSummary>;
  threadID: string;
}) {
  const summaryRuns =
    summary.status === "success" ? (summary.data.agent_runs ?? []) : [];
  const agentByConfigID =
    agentConfigs.status === "success"
      ? new Map(agentConfigs.data.map((agent) => [agent.id, agent]))
      : new Map<string, AgentConfig>();
  const sessionAgentByID = new Map(
    session.agents.map((agent) => [agent.id, agent]),
  );
  const runByID = new Map(summaryRuns.map((run) => [run.id, run]));
  const messagesWithProgress = withLocalReviewProgressMessages({
    events,
    messages,
    session,
    threadID,
  });
  const messagesWithRunMetadata = messagesWithProgress.map((message) =>
    enrichAgentRunMessageForDisplay({
      agentByConfigID,
      message,
      run: message.agent_run_id ? runByID.get(message.agent_run_id) : undefined,
      session,
      sessionAgentByID,
    }),
  );
  const existingRunIDs = new Set(
    messagesWithRunMetadata
      .map((message) => message.agent_run_id)
      .filter((id): id is string => Boolean(id)),
  );
  const eventsByRunID = new Map<string, ReviewEvent[]>();
  const firstEventByRunID = new Map<string, ReviewEvent>();
  for (const event of events) {
    if (!event.agent_run_id || !event.type.startsWith("AgentRun")) {
      continue;
    }
    const runEvents = eventsByRunID.get(event.agent_run_id) ?? [];
    runEvents.push(event);
    eventsByRunID.set(event.agent_run_id, runEvents);
    if (!firstEventByRunID.has(event.agent_run_id)) {
      firstEventByRunID.set(event.agent_run_id, event);
    }
  }
  const liveMessages: ChatMessage[] = [];
  const representedAgentConfigIDs = new Set(
    messagesWithRunMetadata
      .map((message) => message.agent_config_id)
      .filter((id): id is string => Boolean(id)),
  );
  const agentRunMessagesCanRender =
    !["queued", "running"].includes(session.status) ||
    hasReviewStartProgress(messagesWithRunMetadata);
  for (const run of summaryRuns) {
    representedAgentConfigIDs.add(run.agent_config_id);
    if (existingRunIDs.has(run.id)) {
      continue;
    }
    if (!agentRunMessagesCanRender) {
      continue;
    }
    const sessionAgent = sessionAgentForRun(session, run, sessionAgentByID);
    const agent = agentWithRunSelection(
      agentByConfigID.get(run.agent_config_id),
      run,
      sessionAgent,
    );
    const displayMetadata = displayMetadataForRun(run, sessionAgent);
    const runEvents = eventsByRunID.get(run.id) ?? [];
    const runtimeSummary = summarizeRuntimeTraceEvents(runEvents);
    const latestEvent = runEvents.at(-1);
    const timestamp = stableAgentRunTimestamp(
      run,
      firstEventByRunID.get(run.id),
      session,
    );
    const sortRank = agentSortRank(session, run.agent_config_id);
    if (isLiveAgentRun(run)) {
      liveMessages.push({
        id: `live-${run.id}`,
        thread_id: threadID,
        author_type: authorTypeForAgentRun(run),
        author_display_name: agentRunDisplayName(run, agent),
        agent_config_id: run.agent_config_id,
        agent_run_id: run.id,
        body: liveAgentRunBody(run, latestEvent, runtimeSummary, agent),
        status: "streaming",
        metadata: {
          chat_sort_at: timestamp,
          chat_sort_rank: sortRank,
          ...displayMetadata,
          live: true,
          agent_run_status: run.status,
          reasoning_events: runtimeSummary.reasoning.length,
          tool_call_events: runtimeSummary.toolCalls.length,
          output_events: runtimeSummary.output.length,
        },
        created_at: timestamp,
        updated_at: timestamp,
      });
      continue;
    }
    if (!shouldSynthesizeCompletedAgentRunMessage(run, runtimeSummary)) {
      continue;
    }
    liveMessages.push({
      id: `live-${run.id}`,
      thread_id: threadID,
      author_type: authorTypeForAgentRun(run),
      author_display_name: agentRunDisplayName(run, agent),
      agent_config_id: run.agent_config_id,
      agent_run_id: run.id,
      body: completedAgentRunBody(run, runtimeSummary, agent),
      status: run.status === "failed" ? "failed" : "completed",
      metadata: {
        chat_sort_at: timestamp,
        chat_sort_rank: sortRank,
        ...displayMetadata,
        live: true,
        agent_run_status: run.status,
        reasoning_events: runtimeSummary.reasoning.length,
        tool_call_events: runtimeSummary.toolCalls.length,
        output_events: runtimeSummary.output.length,
      },
      created_at: timestamp,
      updated_at: timestamp,
    });
  }
  const plannedMessages: ChatMessage[] = [];
  if (
    ["queued", "running"].includes(session.status) &&
    hasReviewStartProgress(messagesWithRunMetadata)
  ) {
    const now = plannedAgentTimestamp(messagesWithRunMetadata, session);
    for (const assignment of session.agents) {
      if (!assignment.enabled) {
        continue;
      }
      if (representedAgentConfigIDs.has(assignment.agent_config_id)) {
        continue;
      }
      const agent = agentWithSessionAssignmentSelection(
        agentByConfigID.get(assignment.agent_config_id),
        assignment,
      );
      if (agent && isOrchestratorEntry({ agent, assignment })) {
        continue;
      }
      if (!agent && isOrchestratorAssignment(assignment)) {
        continue;
      }
      const label = agent
        ? compactAgentLabel(agent)
        : assignment.role
          ? compactRoleLabel(assignment.role)
          : "Reviewer";
      plannedMessages.push({
        id: `planned-${session.id}-${assignment.agent_config_id}`,
        thread_id: threadID,
        author_type: "agent",
        author_display_name: label,
        agent_config_id: assignment.agent_config_id,
        body: `${label} is queued for an execution slot.`,
        status: "streaming",
        metadata: {
          agent_run_status: "queued",
          chat_sort_at: now,
          chat_sort_rank: agentSortRank(session, assignment.agent_config_id),
          ...displayMetadataForAssignment(assignment),
          local: true,
          planned: true,
        },
        created_at: now,
        updated_at: now,
      });
    }
  }
  return sortChatMessages(
    [...messagesWithRunMetadata, ...liveMessages, ...plannedMessages],
    session,
  );
}

function withLocalReviewProgressMessages({
  events,
  messages,
  session,
  threadID,
}: {
  events: ReviewEvent[];
  messages: ChatMessage[];
  session: ReviewSession;
  threadID: string;
}) {
  const seenProgressEventIDs = new Set<string>();
  for (const message of messages) {
    const eventID = metadataString(message.metadata, "progress_event_id");
    if (eventID) {
      seenProgressEventIDs.add(eventID);
    }
  }
  const progressMessages: ChatMessage[] = [];
  for (const event of events) {
    if (seenProgressEventIDs.has(event.id)) {
      continue;
    }
    const progress = localReviewProgressMessage(event);
    if (!progress) {
      continue;
    }
    progressMessages.push({
      id: `progress-${event.id}`,
      thread_id: threadID,
      author_type: progress.authorType,
      author_display_name: progress.displayName,
      body: progress.body,
      status: progress.status,
      metadata: {
        answer_source: "review_progress",
        chat_sort_at: event.created_at,
        local: true,
        progress_event_created_at: event.created_at,
        progress_event_id: event.id,
        progress_event_sequence: event.sequence,
        progress_event_type: event.type,
      },
      created_at: event.created_at,
      updated_at: event.created_at || session.updated_at,
    });
    seenProgressEventIDs.add(event.id);
  }
  return [...messages, ...progressMessages];
}

function sortChatMessages(messages: ChatMessage[], session: ReviewSession) {
  return [...messages].sort((left, right) => {
    const leftTime = Date.parse(chatSortTimestamp(left));
    const rightTime = Date.parse(chatSortTimestamp(right));
    if (
      !Number.isNaN(leftTime) &&
      !Number.isNaN(rightTime) &&
      leftTime !== rightTime
    ) {
      return leftTime - rightTime;
    }
    const leftRank = chatSortRank(left, session);
    const rightRank = chatSortRank(right, session);
    if (leftRank !== rightRank) {
      return leftRank - rightRank;
    }
    return left.id.localeCompare(right.id);
  });
}

function localReviewProgressMessage(event: ReviewEvent): {
  authorType: ChatMessage["author_type"];
  body: string;
  displayName: string;
  status: ChatMessage["status"];
} | null {
  const phase = payloadString(event.payload.phase);
  const phaseLabel = humanWorkflowPhase(phase);
  const error = payloadString(event.payload.error) || "unknown error";
  switch (event.type) {
    case "ReviewSessionQueued":
      return {
        authorType: "system",
        body: "Review queued. cocode is preparing context and assigning the selected reviewers.",
        displayName: "System",
        status: "completed",
      };
    case "ReviewSessionStarted":
      return {
        authorType: "orchestrator",
        body: "Review started. I’ll build the context bundle, run reviewers in parallel, normalize their outputs, and surface verified findings as they land.",
        displayName: "Orchestrator",
        status: "completed",
      };
    case "WorkflowPhaseStarted": {
      const orchestratorMessage = orchestratorPhaseStartMessage(phase);
      if (orchestratorMessage) {
        return {
          authorType: "orchestrator",
          body: orchestratorMessage,
          displayName: "Orchestrator",
          status: "completed",
        };
      }
      return phaseLabel
        ? {
            authorType: "system",
            body: `${phaseLabel} started.`,
            displayName: "System",
            status: "completed",
          }
        : null;
    }
    case "WorkflowPhaseCompleted":
      return phaseLabel
        ? {
            authorType: "system",
            body: `${phaseLabel} completed.`,
            displayName: "System",
            status: "completed",
          }
        : null;
    case "WorkflowPhaseFailed":
      return phaseLabel
        ? {
            authorType: "system",
            body: `${phaseLabel} failed: ${error}`,
            displayName: "System",
            status: "failed",
          }
        : null;
    case "ReviewSessionPartialFailure":
      return {
        authorType: "system",
        body: "Some reviewers failed, but cocode will continue with the successful outputs and keep the failures visible.",
        displayName: "System",
        status: "completed",
      };
    case "ReviewSessionCompleted":
      return {
        authorType: "cocode",
        body: "Review completed. Findings, evidence, and publish-ready comments are ready to inspect.",
        displayName: "cocode",
        status: "completed",
      };
    case "ReviewSessionFailed":
      return {
        authorType: "system",
        body: `Review failed: ${error || "the review workflow failed"}`,
        displayName: "System",
        status: "failed",
      };
    case "ReviewSessionCanceled":
      return {
        authorType: "system",
        body: "Review canceled.",
        displayName: "System",
        status: "completed",
      };
    default:
      return null;
  }
}

function orchestratorPhaseStartMessage(phase: string) {
  switch (phase.trim()) {
    case "normalize_outputs":
      return "Orchestrator is reading reviewer outputs and extracting candidate findings.";
    case "deduplicate":
    case "deduplicate_findings":
      return "Orchestrator is re-checking and deduplicating findings across reviewer outputs.";
    case "verify_findings":
      return "Orchestrator is re-checking each finding against code evidence and counter-evidence.";
    case "build_evidence":
    case "build_evidence_maps":
      return "Orchestrator is enriching findings with evidence maps and source context.";
    default:
      return "";
  }
}

function humanWorkflowPhase(phase: string) {
  switch (phase.trim()) {
    case "build_context":
    case "build_review_context":
      return "Context build";
    case "run_agents":
    case "run_review_agents":
      return "Agent review";
    case "normalize_outputs":
      return "Finding normalization";
    case "deduplicate":
    case "deduplicate_findings":
      return "Finding deduplication";
    case "verify_findings":
      return "Finding verification";
    case "build_evidence":
    case "build_evidence_maps":
      return "Evidence map build";
    case "draft_comments":
      return "Publish draft preparation";
    default:
      return phase ? phase.replaceAll("_", " ") : "";
  }
}

function hasReviewStartProgress(messages: ChatMessage[]) {
  return messages.some((message) => {
    const type = metadataString(message.metadata, "progress_event_type");
    if (type === "ReviewSessionQueued" || type === "ReviewSessionStarted") {
      return true;
    }
    const body = message.body.trim();
    return (
      body.startsWith("Review queued.") || body.startsWith("Review started.")
    );
  });
}

function plannedAgentTimestamp(
  messages: ChatMessage[],
  session: ReviewSession,
) {
  for (let index = messages.length - 1; index >= 0; index--) {
    const message = messages[index];
    const type = metadataString(message?.metadata, "progress_event_type");
    if (type === "ReviewSessionQueued" || type === "ReviewSessionStarted") {
      return chatSortTimestamp(message);
    }
  }
  return session.updated_at || session.created_at || new Date().toISOString();
}

function stableAgentRunTimestamp(
  run: AgentRunSummary,
  firstEvent: ReviewEvent | undefined,
  session: ReviewSession,
) {
  return (
    run.started_at ||
    firstEvent?.created_at ||
    run.completed_at ||
    session.updated_at ||
    session.created_at ||
    new Date().toISOString()
  );
}

function chatSortTimestamp(message: ChatMessage | undefined) {
  if (!message) {
    return "";
  }
  return (
    metadataString(message.metadata, "chat_sort_at") ||
    metadataString(message.metadata, "review_agent_run_started_at") ||
    metadataString(message.metadata, "review_agent_run_created_at") ||
    metadataString(message.metadata, "progress_event_created_at") ||
    message.created_at
  );
}

function chatSortRank(message: ChatMessage, session: ReviewSession) {
  const explicit = metadataNumber(message.metadata, "chat_sort_rank");
  if (Number.isFinite(explicit)) {
    return explicit;
  }
  if (message.agent_config_id) {
    return agentSortRank(session, message.agent_config_id);
  }
  return 0;
}

function agentSortRank(session: ReviewSession, agentConfigID: string) {
  const assignment = session.agents.find(
    (item) => item.agent_config_id === agentConfigID,
  );
  return 100 + (assignment?.run_order ?? 1000);
}

function shouldSynthesizeCompletedAgentRunMessage(
  run: AgentRunSummary,
  runtimeSummary: RuntimeTraceSummary,
) {
  if (!["completed", "succeeded", "failed", "canceled"].includes(run.status)) {
    return false;
  }
  const role = run.role.toLowerCase();
  if (
    role.includes("orchestrator") ||
    role.includes("verifier") ||
    role.includes("reviewer")
  ) {
    return true;
  }
  return (
    runtimeSummary.output.length > 0 ||
    runtimeSummary.reasoning.length > 0 ||
    runtimeSummary.toolCalls.length > 0 ||
    runtimeSummary.errors.length > 0
  );
}

function authorTypeForAgentRun(
  run: AgentRunSummary,
): ChatMessage["author_type"] {
  const role = run.role.toLowerCase();
  if (role.includes("orchestrator")) {
    return "orchestrator";
  }
  if (role.includes("verifier")) {
    return "verifier";
  }
  return "agent";
}

function agentRunDisplayName(
  run: AgentRunSummary,
  agent: AgentConfig | undefined,
) {
  if (agent) {
    return compactAgentLabel(agent);
  }
  const role = run.role.toLowerCase();
  if (role.includes("orchestrator")) {
    return "cocode";
  }
  if (role.includes("verifier")) {
    return "Verifier";
  }
  return run.role ? compactRoleLabel(run.role) : "Reviewer";
}

function enrichAgentRunMessageForDisplay({
  agentByConfigID,
  message,
  run,
  session,
  sessionAgentByID,
}: {
  agentByConfigID: Map<string, AgentConfig>;
  message: ChatMessage;
  run?: AgentRunSummary;
  session: ReviewSession;
  sessionAgentByID: Map<string, ReviewSessionAgent>;
}): ChatMessage {
  if (!run) {
    return message;
  }
  const sessionAgent = sessionAgentForRun(session, run, sessionAgentByID);
  const agent = agentWithRunSelection(
    agentByConfigID.get(run.agent_config_id),
    run,
    sessionAgent,
  );
  const displayMetadata = displayMetadataForRun(run, sessionAgent);
  if (Object.keys(displayMetadata).length === 0) {
    return message;
  }
  return {
    ...message,
    author_display_name: agent
      ? agentRunDisplayName(run, agent)
      : message.author_display_name,
    metadata: {
      ...(isPlainRecord(message.metadata) ? message.metadata : {}),
      ...displayMetadata,
    },
  };
}

function sessionAgentForRun(
  session: ReviewSession,
  run: AgentRunSummary,
  sessionAgentByID: Map<string, ReviewSessionAgent>,
) {
  if (run.review_session_agent_id) {
    const byID = sessionAgentByID.get(run.review_session_agent_id);
    if (byID) {
      return byID;
    }
  }
  return (
    session.agents.find(
      (agent) =>
        agent.agent_config_id === run.agent_config_id &&
        agent.role === run.role,
    ) ??
    session.agents.find(
      (agent) => agent.agent_config_id === run.agent_config_id,
    )
  );
}

function agentWithRunSelection(
  agent: AgentConfig | undefined,
  run: AgentRunSummary,
  assignment?: ReviewSessionAgent,
) {
  return agentWithDisplaySelection(
    agent,
    run.model_label ||
      metadataString(assignment?.settings_override, "model_label"),
    run.reasoning_label ||
      metadataString(assignment?.settings_override, "reasoning_label"),
  );
}

function agentWithSessionAssignmentSelection(
  agent: AgentConfig | undefined,
  assignment: ReviewSessionAgent,
) {
  return agentWithDisplaySelection(
    agent,
    metadataString(assignment.settings_override, "model_label"),
    metadataString(assignment.settings_override, "reasoning_label"),
  );
}

function agentWithDisplaySelection(
  agent: AgentConfig | undefined,
  modelLabel: string,
  reasoningLabel: string,
): AgentConfig | undefined {
  modelLabel = modelLabel.trim();
  reasoningLabel = reasoningLabel.trim();
  if (!agent || (!modelLabel && !reasoningLabel)) {
    return agent;
  }
  return {
    ...agent,
    model_label: modelLabel || agent.model_label,
    reasoning_label: reasoningLabel || agent.reasoning_label,
    settings: {
      ...agent.settings,
      ...(modelLabel ? { model_label: modelIDDisplayLabel(modelLabel) } : {}),
      ...(reasoningLabel ? { reasoning_label: reasoningLabel } : {}),
    },
  };
}

function displayMetadataForRun(
  run: AgentRunSummary,
  assignment?: ReviewSessionAgent,
) {
  return displayMetadataForSelection(
    run.model_label ||
      metadataString(assignment?.settings_override, "model_label"),
    run.reasoning_label ||
      metadataString(assignment?.settings_override, "reasoning_label"),
  );
}

function displayMetadataForAssignment(assignment: ReviewSessionAgent) {
  return displayMetadataForSelection(
    metadataString(assignment.settings_override, "model_label"),
    metadataString(assignment.settings_override, "reasoning_label"),
  );
}

function displayMetadataForSelection(
  modelLabel: string,
  reasoningLabel: string,
) {
  const metadata: Record<string, string> = {};
  modelLabel = modelLabel.trim();
  reasoningLabel = reasoningLabel.trim();
  if (modelLabel) {
    metadata.model_label = modelIDDisplayLabel(modelLabel);
  }
  if (reasoningLabel) {
    metadata.reasoning_label = reasoningLabel;
  }
  return metadata;
}

export function pendingChatMessages({
  agentByConfigID,
  audience,
  responder,
  sessionAgents,
  threadID,
}: {
  agentByConfigID: Map<string, AgentConfig>;
  audience: ChatAudience;
  responder: ChatResponderOption;
  sessionAgents: ReviewSessionAgent[];
  threadID: string;
}): ChatMessage[] {
  const now = new Date().toISOString();
  const labelForAssignment = (assignment: ReviewSessionAgent) => {
    const agent = agentByConfigID.get(assignment.agent_config_id);
    if (agent) {
      return compactAgentLabel(agent);
    }
    if (assignment.role) {
      return compactRoleLabel(assignment.role);
    }
    return "Reviewer";
  };
  if (audience === "all_agents") {
    const reviewers = sessionAgents.filter(
      (assignment) =>
        assignment.enabled && !isOrchestratorAssignment(assignment),
    );
    if (reviewers.length > 0) {
      return reviewers.map((assignment) => {
        const label = labelForAssignment(assignment);
        return {
          id: `pending-${assignment.id}-${now}`,
          thread_id: threadID,
          author_type: "agent",
          author_display_name: label,
          agent_config_id: assignment.agent_config_id,
          body: `${label} is reading the review context and preparing an answer.`,
          status: "streaming",
          metadata: { local: true, pending: true },
          created_at: now,
          updated_at: now,
        };
      });
    }
  }
  if (audience === "selected_agent" && responder.agentConfigId) {
    const assignment = sessionAgents.find(
      (item) => item.agent_config_id === responder.agentConfigId,
    );
    return [
      {
        id: `pending-${responder.agentConfigId}-${now}`,
        thread_id: threadID,
        author_type: "agent",
        author_display_name:
          (assignment ? labelForAssignment(assignment) : "") || responder.label,
        agent_config_id: responder.agentConfigId,
        body: `${
          (assignment ? labelForAssignment(assignment) : "") || responder.label
        } is reading the review context and preparing an answer.`,
        status: "streaming",
        metadata: { local: true, pending: true },
        created_at: now,
        updated_at: now,
      },
    ];
  }
  return [
    {
      id: `pending-orchestrator-${now}`,
      thread_id: threadID,
      author_type: "orchestrator",
      author_display_name: "Orchestrator",
      body: "cocode is synthesizing the latest review state and agent evidence.",
      status: "streaming",
      metadata: { local: true, pending: true },
      created_at: now,
      updated_at: now,
    },
  ];
}

function isLiveAgentRun(run: AgentRunSummary) {
  return run.status === "queued" || run.status === "running";
}

function liveAgentRunBody(
  run: AgentRunSummary,
  latestEvent: ReviewEvent | undefined,
  runtimeSummary: RuntimeTraceSummary,
  agent: AgentConfig | undefined,
) {
  const label = agent ? compactAgentLabel(agent) : "Reviewer";
  const reasoning = lastNonEmpty(runtimeSummary.reasoning);
  if (reasoning) {
    return `**Visible reasoning**\n\n${reasoning.trim()}`;
  }
  const modelOutput = lastDisplayableAgentOutput(runtimeSummary.output);
  if (modelOutput) {
    return modelOutput;
  }
  if (runtimeSummary.toolCalls.length > 0) {
    return `${label} is using tools and checking evidence. Open the trace to inspect live commands and diagnostics.`;
  }
  if (run.status === "queued") {
    return `${label} is queued and waiting for an execution slot.`;
  }
  if (latestEvent?.type === "AgentRunOutput") {
    const stream = payloadString(latestEvent.payload.stream);
    return `${label} is streaming ${stream || "output"} back to cocode.`;
  }
  return `${label} is ${liveAgentRunWork(run)}.`;
}

function completedAgentRunBody(
  run: AgentRunSummary,
  runtimeSummary: RuntimeTraceSummary,
  agent: AgentConfig | undefined,
) {
  const output = lastDisplayableAgentOutput(runtimeSummary.output);
  if (output) {
    return output;
  }
  const reasoning = lastNonEmpty(runtimeSummary.reasoning);
  if (reasoning) {
    return `**Visible reasoning**\n\n${reasoning.trim()}`;
  }
  const error = lastNonEmpty(runtimeSummary.errors) || run.error_message || "";
  if (run.status === "failed" || error) {
    return (
      error ||
      `${agentRunDisplayName(run, agent)} failed before returning output.`
    );
  }
  const role = run.role.toLowerCase();
  if (role.includes("orchestrator")) {
    return "cocode finished synthesizing the review findings and evidence map.";
  }
  if (role.includes("verifier")) {
    return "Verifier finished checking the selected findings against the evidence.";
  }
  const label = agent ? compactAgentLabel(agent) : "Reviewer";
  return `${label} completed its review pass.`;
}

function lastNonEmpty(items: string[]) {
  for (let index = items.length - 1; index >= 0; index--) {
    const value = items[index]?.trim();
    if (value) {
      return value;
    }
  }
  return "";
}

function lastDisplayableAgentOutput(items: string[]) {
  for (let index = items.length - 1; index >= 0; index--) {
    const value = items[index]?.trim();
    if (!value) {
      continue;
    }
    const displayable = extractDisplayableAgentOutput(value).trim();
    if (displayable) {
      return displayable;
    }
  }
  return "";
}

function liveAgentRunWork(run: AgentRunSummary) {
  const role = run.role.toLowerCase();
  if (role.includes("chat")) {
    return "answering your follow-up";
  }
  if (role.includes("verifier")) {
    return "checking evidence";
  }
  if (role.includes("context")) {
    return "building review context";
  }
  return "reviewing changed files";
}
