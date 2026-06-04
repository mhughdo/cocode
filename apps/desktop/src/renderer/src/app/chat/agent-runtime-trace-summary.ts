import type { ReviewEvent } from "@/lib/api";

import { formatKnownAgentJSONPayload } from "../shared/agent-output-formatting";

export type RuntimeTraceSummary = {
  diagnostics: string[];
  errors: string[];
  eventCount: number;
  lifecycle: string[];
  output: string[];
  reasoning: string[];
  toolCalls: string[];
};

type RuntimeTraceAccumulator = {
  diagnostics: string[];
  errors: string[];
  finalOutput: string[];
  lifecycle: string[];
  output: string[];
  outputBuffer: string;
  reasoning: string[];
  reasoningBuffer: string;
  toolCalls: string[];
};

export function summarizeRuntimeTraceEvents(
  events: ReviewEvent[],
): RuntimeTraceSummary {
  const runtimeEvents = events.filter(isVisibleRuntimeEvent);
  const accumulator: RuntimeTraceAccumulator = {
    diagnostics: [],
    errors: [],
    finalOutput: [],
    lifecycle: [],
    output: [],
    outputBuffer: "",
    reasoning: [],
    reasoningBuffer: "",
    toolCalls: [],
  };

  for (const event of runtimeEvents) {
    collectEventSummary(event, accumulator);
  }
  flushAccumulatorTextBuffer(accumulator, "output");
  flushAccumulatorTextBuffer(accumulator, "reasoning");
  const output = preferredModelOutput(
    accumulator.output,
    accumulator.finalOutput,
  );

  return {
    diagnostics: accumulator.diagnostics,
    errors: accumulator.errors,
    eventCount: runtimeEvents.length,
    lifecycle: accumulator.lifecycle,
    output,
    reasoning: accumulator.reasoning,
    toolCalls: accumulator.toolCalls,
  };
}

function collectEventSummary(
  event: ReviewEvent,
  accumulator: RuntimeTraceAccumulator,
) {
  const preview = stringFromUnknown(event.payload.text_preview);
  const error = stringFromUnknown(event.payload.error);
  const message = stringFromUnknown(event.payload.message);
  const stream = stringFromUnknown(event.payload.stream);
  const typeLabel = formatTraceEventLabel(event.type);

  if (error) {
    pushTraceItem(accumulator.errors, error);
  }

  if (
    (event.type === "AgentRunOutput" || event.type === "AgentRunProgress") &&
    preview
  ) {
    const parsed = collectPreviewJSON(preview, accumulator);
    if (!parsed) {
      if (stream === "stderr" || event.level === "error") {
        pushTraceItem(accumulator.diagnostics, preview);
      } else {
        appendTextDelta(accumulator, "output", preview);
      }
    }
    return;
  }

  if (event.type === "AgentRunFailed") {
    pushTraceItem(accumulator.errors, error || message || typeLabel);
    return;
  }

  if (event.type === "AgentRunArtifact") {
    const artifactID =
      stringFromUnknown(event.artifact_id) ||
      stringFromUnknown(event.payload.artifact_id);
    const parts = [
      "Artifact saved",
      artifactID ? `id: ${artifactID}` : "",
      message ? `message: ${message}` : "",
    ].filter(Boolean);
    pushTraceItem(accumulator.lifecycle, parts.join("\n"));
    return;
  }

  if (message) {
    pushTraceItem(accumulator.lifecycle, `${typeLabel}: ${message}`);
  } else if (event.type !== "AgentRunOutput") {
    pushTraceItem(accumulator.lifecycle, typeLabel);
  }
}

function collectPreviewJSON(
  preview: string,
  accumulator: RuntimeTraceAccumulator,
) {
  const values = parseJSONValues(preview);
  if (values.length === 0) {
    return false;
  }
  for (const value of values) {
    collectTraceValue(value, accumulator);
  }
  return true;
}

function collectTraceValue(
  value: unknown,
  accumulator: RuntimeTraceAccumulator,
) {
  if (Array.isArray(value)) {
    for (const item of value) {
      collectTraceValue(item, accumulator);
    }
    return;
  }
  if (!isPlainRecord(value)) {
    return;
  }

  if (collectClaudeStreamValue(value, accumulator)) {
    return;
  }
  if (collectGeminiJSONValue(value, accumulator)) {
    return;
  }
  if (collectOpenCodeStreamValue(value, accumulator)) {
    return;
  }
  if (collectCodexStreamValue(value, accumulator)) {
    return;
  }

  const type = stringFromUnknown(value.type).toLowerCase();
  const part = isPlainRecord(value.part) ? value.part : undefined;
  const item = isPlainRecord(value.item) ? value.item : undefined;
  const delta = isPlainRecord(value.delta) ? value.delta : undefined;
  const nestedType = [
    type,
    stringFromUnknown(part?.type).toLowerCase(),
    stringFromUnknown(item?.type).toLowerCase(),
    stringFromUnknown(delta?.type).toLowerCase(),
  ].join(" ");

  if (type === "result" || type === "response.completed") {
    const result =
      stringFromUnknown(value.result) ||
      stringFromUnknown(value.response) ||
      stringFromUnknown(value.output);
    pushTraceItem(accumulator.finalOutput, humanizeModelText(result));
    return;
  }

  if (isReasoningType(nestedType) || value.thought === true) {
    const text =
      stringFromUnknown(value.reasoning) ||
      stringFromUnknown(value.thinking) ||
      stringFromUnknown(value.summary) ||
      stringFromUnknown(value.text) ||
      stringFromUnknown(part?.text) ||
      stringFromUnknown(delta?.thinking) ||
      stringFromUnknown(delta?.text);
    appendTextDelta(accumulator, "reasoning", text);
  }

  if (isToolType(nestedType)) {
    pushToolTraceItem(accumulator.toolCalls, traceToolDescription(value));
  }

  const structuredFindings = structuredFindingsFromValue(value);
  if (structuredFindings.length > 0) {
    pushTraceItem(
      accumulator.output,
      formatStructuredFindings(structuredFindings),
    );
    return;
  }

  const structuredReviewOutput = formatKnownAgentJSONPayload(value);
  if (structuredReviewOutput !== null) {
    pushTraceItem(accumulator.output, structuredReviewOutput);
    return;
  }

  if (!isReasoningType(nestedType) && !isToolType(nestedType)) {
    const text =
      stringFromUnknown(value.answer) ||
      stringFromUnknown(value.response) ||
      stringFromUnknown(value.result) ||
      stringFromUnknown(value.message) ||
      stringFromUnknown(value.text) ||
      stringFromUnknown(part?.text) ||
      stringFromUnknown(item?.text);
    if (text) {
      appendTextDelta(accumulator, "output", text);
    }
  }

  for (const key of [
    "event",
    "part",
    "item",
    "delta",
    "message",
    "content",
    "response",
    "result",
    "answer",
    "text",
    "output",
    "summary",
    "findings",
    "finding",
    "params",
    "update",
    "sessionUpdate",
    "data",
  ]) {
    collectTraceValue(value[key], accumulator);
  }
}

function collectGeminiJSONValue(
  value: Record<string, unknown>,
  accumulator: RuntimeTraceAccumulator,
) {
  if (!("response" in value)) {
    return false;
  }
  pushTraceItem(accumulator.output, stringFromUnknown(value.response));
  if (isPlainRecord(value.stats)) {
    collectGeminiToolStats(value.stats, accumulator);
    collectGeminiThinkingStats(value.stats, accumulator);
  }
  return true;
}

function collectGeminiToolStats(
  stats: Record<string, unknown>,
  accumulator: RuntimeTraceAccumulator,
) {
  const tools = isPlainRecord(stats.tools) ? stats.tools : undefined;
  if (!tools) {
    return;
  }
  const totalCalls = numberFromUnknown(tools.totalCalls);
  const totalFail = numberFromUnknown(tools.totalFail);
  const byName = isPlainRecord(tools.byName) ? tools.byName : {};
  const lines = [`total calls: ${totalCalls}`, `failed calls: ${totalFail}`];
  for (const [name, raw] of Object.entries(byName)) {
    if (!isPlainRecord(raw)) {
      continue;
    }
    lines.push(
      `${name}: ${numberFromUnknown(raw.count)} call(s), ${numberFromUnknown(raw.fail)} failed`,
    );
  }
  if (totalCalls > 0 || lines.length > 2) {
    pushTraceItem(accumulator.toolCalls, lines.join("\n"));
  }
}

function collectGeminiThinkingStats(
  stats: Record<string, unknown>,
  accumulator: RuntimeTraceAccumulator,
) {
  const models = isPlainRecord(stats.models) ? stats.models : undefined;
  if (!models) {
    return;
  }
  const lines: string[] = [];
  for (const [model, raw] of Object.entries(models)) {
    if (!isPlainRecord(raw) || !isPlainRecord(raw.tokens)) {
      continue;
    }
    const thoughts = numberFromUnknown(raw.tokens.thoughts);
    if (thoughts > 0) {
      lines.push(
        `${model}: ${thoughts} thinking token(s) reported by Gemini; private reasoning text was not exposed.`,
      );
    }
  }
  if (lines.length > 0) {
    pushTraceItem(accumulator.reasoning, lines.join("\n"));
  }
}

function collectClaudeStreamValue(
  value: Record<string, unknown>,
  accumulator: RuntimeTraceAccumulator,
) {
  if (stringFromUnknown(value.type) !== "stream_event") {
    return false;
  }
  const event = isPlainRecord(value.event) ? value.event : undefined;
  if (!event) {
    return true;
  }
  const eventType = stringFromUnknown(event.type);
  const delta = isPlainRecord(event.delta) ? event.delta : undefined;
  const contentBlock = isPlainRecord(event.content_block)
    ? event.content_block
    : undefined;

  switch (eventType) {
    case "content_block_delta": {
      const deltaType = stringFromUnknown(delta?.type).toLowerCase();
      if (deltaType.includes("thinking")) {
        appendTextDelta(
          accumulator,
          "reasoning",
          textFromUnknown(delta?.thinking) || textFromUnknown(delta?.text),
        );
      } else {
        appendTextDelta(accumulator, "output", textFromUnknown(delta?.text));
      }
      return true;
    }
    case "content_block_start": {
      const blockType = stringFromUnknown(contentBlock?.type).toLowerCase();
      if (blockType.includes("tool")) {
        pushTraceItem(
          accumulator.toolCalls,
          traceToolDescription(contentBlock),
        );
      }
      if (blockType.includes("thinking")) {
        appendTextDelta(
          accumulator,
          "reasoning",
          stringFromUnknown(contentBlock?.thinking) ||
            stringFromUnknown(contentBlock?.text),
        );
      }
      return true;
    }
    case "message_delta":
    case "message_start":
    case "message_stop":
    case "content_block_stop":
      return true;
    default:
      collectTraceValue(event, accumulator);
      return true;
  }
}

function collectCodexStreamValue(
  value: Record<string, unknown>,
  accumulator: RuntimeTraceAccumulator,
) {
  const type = stringFromUnknown(value.type);
  const item = isPlainRecord(value.item) ? value.item : undefined;
  if (!type.startsWith("item.") || !item) {
    return false;
  }
  const itemType = stringFromUnknown(item.type).toLowerCase();
  if (itemType === "agent_message" || itemType === "message") {
    const text = stringFromUnknown(item.text);
    flushAccumulatorTextBuffer(accumulator, "output");
    accumulator.outputBuffer = "";
    pushTraceItem(
      accumulator.output,
      formatKnownAgentJSONPayload(text) ?? text,
    );
    return true;
  }
  if (isReasoningType(itemType)) {
    flushAccumulatorTextBuffer(accumulator, "reasoning");
    accumulator.reasoningBuffer = "";
    pushTraceItem(
      accumulator.reasoning,
      stringFromUnknown(item.summary) ||
        stringFromUnknown(item.text) ||
        stringFromUnknown(item.reasoning),
    );
    return true;
  }
  if (isToolType(itemType)) {
    pushToolTraceItem(accumulator.toolCalls, traceToolDescription(item));
    return true;
  }
  return true;
}

function collectOpenCodeStreamValue(
  value: Record<string, unknown>,
  accumulator: RuntimeTraceAccumulator,
) {
  const type = stringFromUnknown(value.type).toLowerCase();
  const part = isPlainRecord(value.part) ? value.part : undefined;
  const partType = stringFromUnknown(part?.type).toLowerCase();
  const supportedType =
    [
      "reasoning",
      "text",
      "step_start",
      "step-start",
      "step_finish",
      "step-finish",
    ].includes(type) ||
    isToolType(type) ||
    isToolType(partType);
  if (!part && !supportedType) {
    return false;
  }

  if (type === "reasoning" || partType === "reasoning") {
    flushAccumulatorTextBuffer(accumulator, "reasoning");
    accumulator.reasoningBuffer = "";
    pushTraceItem(
      accumulator.reasoning,
      stringFromUnknown(part?.text) || stringFromUnknown(value.text),
    );
    return true;
  }

  if (type === "text" || partType === "text") {
    flushAccumulatorTextBuffer(accumulator, "output");
    accumulator.outputBuffer = "";
    pushTraceItem(
      accumulator.output,
      stringFromUnknown(part?.text) || stringFromUnknown(value.text),
    );
    return true;
  }

  if (
    type === "step_start" ||
    type === "step-start" ||
    partType === "step-start" ||
    partType === "step_start"
  ) {
    pushTraceItem(accumulator.lifecycle, "OpenCode step started");
    return true;
  }

  if (
    type === "step_finish" ||
    type === "step-finish" ||
    partType === "step-finish" ||
    partType === "step_finish"
  ) {
    pushTraceItem(accumulator.lifecycle, openCodeStepFinishDescription(part));
    return true;
  }

  if (
    isToolType(`${type} ${partType}`) ||
    isToolType(type) ||
    isToolType(partType)
  ) {
    pushToolTraceItem(
      accumulator.toolCalls,
      traceToolDescription(part ?? value),
    );
    return true;
  }

  return false;
}

function openCodeStepFinishDescription(
  part: Record<string, unknown> | undefined,
) {
  const reason = stringFromUnknown(part?.reason);
  const tokens = isPlainRecord(part?.tokens) ? part.tokens : undefined;
  const tokenTotal = numberFromUnknown(tokens?.total);
  const cost = numberFromUnknown(part?.cost);
  return [
    "OpenCode step finished",
    reason && `reason: ${reason}`,
    tokenTotal > 0 && `tokens: ${tokenTotal}`,
    cost > 0 && `cost: ${cost}`,
  ]
    .filter(Boolean)
    .join("\n");
}

function openCodeTraceSummary(value: Record<string, unknown>) {
  const type = stringFromUnknown(value.type).toLowerCase();
  const part = isPlainRecord(value.part) ? value.part : undefined;
  const partType = stringFromUnknown(part?.type).toLowerCase();
  const text =
    stringFromUnknown(part?.text) ||
    stringFromUnknown(value.text) ||
    stringFromUnknown(value.output) ||
    stringFromUnknown(value.reasoning) ||
    stringFromUnknown(value.summary);

  if (
    type === "step_start" ||
    type === "step-start" ||
    partType === "step-start" ||
    partType === "step_start" ||
    type === "step_finish" ||
    type === "step-finish" ||
    partType === "step-finish" ||
    partType === "step_finish"
  ) {
    return "";
  }
  if (type === "reasoning" || partType === "reasoning") {
    return text;
  }
  if (type === "text" || partType === "text") {
    return text;
  }
  if (
    isToolType(`${type} ${partType}`) ||
    isToolType(type) ||
    isToolType(partType)
  ) {
    return traceToolDescription(part ?? value);
  }
  return null;
}

function parseJSONValues(preview: string) {
  const trimmed = preview.trim();
  if (!trimmed) {
    return [];
  }
  const parsedWhole = parseJSON(trimmed);
  if (parsedWhole !== undefined) {
    return [parsedWhole];
  }
  const values: unknown[] = [];
  for (const line of trimmed.split(/\r?\n/)) {
    const candidate = line.trim();
    if (
      !candidate ||
      (!candidate.startsWith("{") && !candidate.startsWith("["))
    ) {
      continue;
    }
    const parsed = parseJSON(candidate);
    if (parsed !== undefined) {
      values.push(parsed);
    }
  }
  return values;
}

function parseJSON(value: string) {
  try {
    return JSON.parse(value) as unknown;
  } catch {
    return undefined;
  }
}

function traceToolDescription(value: unknown) {
  if (!isPlainRecord(value)) {
    return "";
  }
  const state = isPlainRecord(value.state) ? value.state : undefined;
  const metadata = isPlainRecord(state?.metadata) ? state.metadata : undefined;
  const input =
    value.input ??
    state?.input ??
    value.arguments ??
    value.args ??
    value.params ??
    value.call ??
    value.payload ??
    value.request ??
    value.tool_input ??
    value.toolInput;
  const command =
    stringFromUnknown(value.command) ||
    stringFromUnknown(value.name) ||
    stringFromUnknown(value.tool) ||
    stringFromUnknown(value.action) ||
    stringFromUnknown(value.tool_name) ||
    stringFromUnknown(value.function_name) ||
    stringFromUnknown(value.operation);
  const status =
    stringFromUnknown(value.status) || stringFromUnknown(state?.status);
  const output =
    stringFromUnknown(value.aggregated_output) ||
    stringFromUnknown(value.output) ||
    stringFromUnknown(state?.output) ||
    stringFromUnknown(metadata?.output) ||
    stringFromUnknown(value.result) ||
    stringFromUnknown(value.response) ||
    stringFromUnknown(value.text);
  const inputText = describeTraceToolInput(input);
  return [
    command,
    inputText && `input: ${inputText}`,
    status && `status: ${status}`,
    output,
  ]
    .filter(Boolean)
    .join("\n");
}

function describeTraceToolInput(input: unknown) {
  if (typeof input === "string") {
    return input;
  }
  if (!isPlainRecord(input)) {
    return "";
  }
  for (const key of [
    "command",
    "cmd",
    "query",
    "prompt",
    "path",
    "file",
    "text",
    "input",
    "script",
    "url",
    "name",
  ]) {
    const value = stringFromUnknown(input[key]);
    if (value) {
      return value;
    }
  }
  const entries = Object.entries(input).filter(([, value]) =>
    ["string", "number", "boolean"].includes(typeof value),
  );
  if (entries.length > 0 && entries.length <= 4) {
    return entries.map(([key, value]) => `${key}: ${String(value)}`).join("\n");
  }
  return JSON.stringify(input);
}

function pushToolTraceItem(items: string[], value: string) {
  const trimmed = humanizeModelText(value);
  if (!trimmed) {
    return;
  }
  const key = toolTraceKey(trimmed);
  const existingIndex = items.findIndex((item) => toolTraceKey(item) === key);
  if (existingIndex >= 0) {
    if (trimmed.length >= items[existingIndex].length) {
      items[existingIndex] = trimmed;
    }
    return;
  }
  items.push(trimmed);
}

function toolTraceKey(value: string) {
  const firstLine = value.split(/\r?\n/).find((line) => line.trim()) ?? value;
  return firstLine.trim();
}

function appendTextDelta(
  accumulator: RuntimeTraceAccumulator,
  target: "output" | "reasoning",
  value: string,
) {
  if (!value) {
    return;
  }
  if (target === "output") {
    accumulator.outputBuffer += value;
    return;
  }
  accumulator.reasoningBuffer += value;
}

function flushAccumulatorTextBuffer(
  accumulator: RuntimeTraceAccumulator,
  target: "output" | "reasoning",
) {
  const value =
    target === "output"
      ? accumulator.outputBuffer
      : accumulator.reasoningBuffer;
  if (target === "output") {
    const parsed = parseJSON(value.trim());
    if (isPlainRecord(parsed) && collectGeminiJSONValue(parsed, accumulator)) {
      return;
    }
    pushTraceItem(accumulator.output, value);
    return;
  }
  pushTraceItem(accumulator.reasoning, value);
}

function preferredModelOutput(streamed: string[], finalOutput: string[]) {
  const readableStreamed = cleanModelOutputItems(streamed);
  if (finalOutput.length === 0) {
    return readableStreamed;
  }
  if (streamed.length === 0) {
    return finalOutput;
  }
  const finalHasStructuredFindings = finalOutput.some(isReadableFindingOutput);
  if (finalHasStructuredFindings) {
    return finalOutput;
  }
  const streamedLooksFragmented = streamed.some(looksLikeEscapedJSONFragment);
  const finalIsRicher =
    finalOutput.join("\n").length > streamed.join("\n").length * 1.25;
  if (streamedLooksFragmented && finalIsRicher) {
    return finalOutput;
  }
  const merged = [...readableStreamed];
  for (const item of finalOutput) {
    if (!merged.includes(item)) {
      merged.push(item);
    }
  }
  return merged;
}

function cleanModelOutputItems(items: string[]) {
  const readable = items.filter((item) => !isTraceNoiseFragment(item));
  const structured = readable.filter(isReadableFindingOutput);
  if (structured.length > 0) {
    return structured;
  }
  return readable;
}

function isReadableFindingOutput(item: string) {
  return /^#{0,2}\s*Findings \(\d+\)/.test(item.trim());
}

function looksLikeEscapedJSONFragment(item: string) {
  const trimmed = item.trim();
  return (
    trimmed.startsWith("```json") ||
    trimmed.includes('\\"') ||
    trimmed.includes("\\n") ||
    /^[,;}\]"']+$/.test(trimmed) ||
    /^[A-Za-z0-9_:"{},\s-]*"findings"\s*:/.test(trimmed)
  );
}

function isTraceNoiseFragment(item: string) {
  const trimmed = item.trim();
  if (!trimmed) {
    return true;
  }
  if (/^[,;}\]"']+$/.test(trimmed)) {
    return true;
  }
  if (
    trimmed.length < 90 &&
    (trimmed.startsWith('";') ||
      trimmed.startsWith(";\n") ||
      trimmed === "}" ||
      trimmed === "```")
  ) {
    return true;
  }
  const parsed = parseJSON(trimmed);
  if (isPlainRecord(parsed) && isIgnorableTraceRecord(parsed)) {
    return true;
  }
  return false;
}

function pushTraceItem(items: string[], value: string) {
  const trimmed = humanizeModelText(value);
  if (!trimmed || items.includes(trimmed)) {
    return;
  }
  items.push(trimmed);
}

function humanizeModelText(value: string, depth = 0): string {
  const text = stripAnsi(value).trim();
  if (!text) {
    return "";
  }
  const parsed = parseJSON(text);
  if (parsed !== undefined) {
    const formatted = formatTraceJSON(parsed, depth);
    if (formatted !== null) {
      return formatted;
    }
  }

  const fenced = firstJSONFence(text);
  if (fenced) {
    const parsedFence = parseJSON(fenced);
    if (parsedFence !== undefined) {
      const formatted = formatTraceJSON(parsedFence, depth);
      if (formatted !== null) {
        return formatted;
      }
    }
  }

  return normalizeEscapedText(text);
}

function formatTraceJSON(value: unknown, depth: number): string | null {
  if (depth > 3) {
    return null;
  }
  if (typeof value === "string") {
    return humanizeModelText(value, depth + 1);
  }
  if (Array.isArray(value)) {
    const findings = structuredFindingsFromValue({ findings: value });
    if (findings.length > 0) {
      return formatStructuredFindings(findings);
    }
    const parts = value
      .map((item) => formatTraceJSON(item, depth + 1))
      .filter((item): item is string => Boolean(item));
    return parts.length > 0 ? parts.join("\n\n") : null;
  }
  if (!isPlainRecord(value)) {
    return null;
  }
  const openCodeSummary = openCodeTraceSummary(value);
  if (openCodeSummary !== null) {
    return openCodeSummary;
  }
  if (isIgnorableTraceRecord(value)) {
    return "";
  }
  const structuredReviewOutput = formatKnownAgentJSONPayload(value);
  if (structuredReviewOutput !== null) {
    return structuredReviewOutput;
  }
  const findings = structuredFindingsFromValue(value);
  if (findings.length > 0) {
    return formatStructuredFindings(findings);
  }
  const extracted = answerFromTraceJSON(value);
  if (extracted) {
    return humanizeModelText(extracted, depth + 1);
  }
  return JSON.stringify(value, null, 2);
}

function stripAnsi(value: string) {
  const escape = String.fromCharCode(27);
  return value.replace(new RegExp(`${escape}\\[[0-9;]*m`, "g"), "");
}

function firstJSONFence(text: string) {
  const match = text.match(/```(?:json)?\s*([\s\S]*?)\s*```/i);
  return match?.[1]?.trim() ?? "";
}

function normalizeEscapedText(text: string) {
  const trimmed = text.trim();
  if (!trimmed.includes("\\n") || trimmed.includes("\n")) {
    return trimmed;
  }
  return trimmed
    .replace(/\\r\\n/g, "\n")
    .replace(/\\n/g, "\n")
    .replace(/\\"/g, '"')
    .trim();
}

function structuredFindingsFromValue(value: Record<string, unknown>) {
  const rawFindings = Array.isArray(value.findings)
    ? value.findings
    : isPlainRecord(value.finding)
      ? [value.finding]
      : [];
  return rawFindings.filter(isPlainRecord);
}

function formatStructuredFindings(findings: Record<string, unknown>[]) {
  const blocks = findings.slice(0, 8).map((finding, index) => {
    const title =
      stringFromUnknown(finding.title) ||
      stringFromUnknown(finding.claim) ||
      stringFromUnknown(finding.message) ||
      stringFromUnknown(finding.description) ||
      `Finding ${index + 1}`;
    const severity = stringFromUnknown(finding.severity);
    const category = stringFromUnknown(finding.category);
    const path =
      stringFromUnknown(finding.path) ||
      stringFromUnknown(finding.file) ||
      firstLocationPath(finding.locations);
    const line =
      stringFromUnknown(finding.line) ||
      numberTextFromUnknown(finding.line) ||
      numberTextFromUnknown(finding.start_line) ||
      firstLocationLine(finding.locations);
    const description =
      stringFromUnknown(finding.body) ||
      stringFromUnknown(finding.description) ||
      stringFromUnknown(finding.evidence) ||
      stringFromUnknown(finding.summary);
    const fix =
      stringFromUnknown(finding.suggested_fix) ||
      stringFromUnknown(finding.recommendation) ||
      stringFromUnknown(finding.fix);
    return [
      `### ${index + 1}. ${title}`,
      severity && `- **Severity:** ${severity}`,
      category && `- **Category:** ${category}`,
      path && `- **Location:** \`${path}${line ? `:${line}` : ""}\``,
      description && `- **Evidence:** ${description}`,
      fix && `- **Suggested fix:** ${fix}`,
    ]
      .filter(Boolean)
      .join("\n");
  });
  const omitted = Math.max(findings.length - blocks.length, 0);
  if (omitted > 0) {
    blocks.push(`${omitted} more finding${omitted === 1 ? "" : "s"} omitted.`);
  }
  return [`## Findings (${findings.length})`, ...blocks].join("\n\n");
}

function firstLocationPath(value: unknown) {
  if (!Array.isArray(value)) {
    return "";
  }
  const location = value.find(isPlainRecord);
  return stringFromUnknown(location?.path) || stringFromUnknown(location?.file);
}

function firstLocationLine(value: unknown) {
  if (!Array.isArray(value)) {
    return "";
  }
  const location = value.find(isPlainRecord);
  return (
    numberTextFromUnknown(location?.start_line) ||
    numberTextFromUnknown(location?.line)
  );
}

function numberTextFromUnknown(value: unknown) {
  return typeof value === "number" && Number.isFinite(value)
    ? String(value)
    : "";
}

function isIgnorableTraceRecord(value: Record<string, unknown>) {
  const type = stringFromUnknown(value.type).toLowerCase();
  const subtype = stringFromUnknown(value.subtype).toLowerCase();
  const hookName = stringFromUnknown(value.hook_name).toLowerCase();
  const hookEvent = stringFromUnknown(value.hook_event).toLowerCase();
  const item = isPlainRecord(value.item) ? value.item : undefined;
  const part = isPlainRecord(value.part) ? value.part : undefined;
  const delta = isPlainRecord(value.delta) ? value.delta : undefined;
  const nestedType = [
    type,
    stringFromUnknown(item?.type).toLowerCase(),
    stringFromUnknown(part?.type).toLowerCase(),
    stringFromUnknown(delta?.type).toLowerCase(),
  ].join(" ");
  return (
    (type === "system" &&
      (subtype.includes("hook") ||
        hookName.includes("hook") ||
        hookEvent.includes("hook") ||
        hookEvent.includes("sessionstart"))) ||
    type === "thread.started" ||
    type === "turn.started" ||
    type === "session.update" ||
    (isToolType(nestedType) && !nestedType.includes("agent_message"))
  );
}

function answerFromTraceJSON(value: unknown): string | null {
  if (typeof value === "string") {
    return value.trim() || null;
  }
  if (Array.isArray(value)) {
    for (let index = value.length - 1; index >= 0; index -= 1) {
      const extracted = answerFromTraceJSON(value[index]);
      if (extracted) {
        return extracted;
      }
    }
    return null;
  }
  if (!isPlainRecord(value)) {
    return null;
  }
  if (isIgnorableTraceRecord(value)) {
    return null;
  }
  for (const key of [
    "answer",
    "content",
    "message",
    "summary",
    "text",
    "output",
    "response",
    "result",
  ]) {
    const extracted = answerFromTraceJSON(value[key]);
    if (extracted) {
      return extracted;
    }
  }
  for (const key of ["item", "part", "delta", "event"]) {
    const extracted = answerFromTraceJSON(value[key]);
    if (extracted) {
      return extracted;
    }
  }
  return null;
}

function isVisibleRuntimeEvent(event: ReviewEvent) {
  return (
    event.type === "AgentRunQueued" ||
    event.type === "AgentRunStarted" ||
    event.type === "AgentRunProgress" ||
    event.type === "AgentRunOutput" ||
    event.type === "AgentRunArtifact" ||
    event.type === "AgentRunFailed" ||
    event.type === "AgentRunCompleted" ||
    event.type === "AgentRunCanceled"
  );
}

function isReasoningType(value: string) {
  return (
    value.includes("reasoning") ||
    value.includes("thinking") ||
    value.includes("thought")
  );
}

function isToolType(value: string) {
  const normalized = value.toLowerCase();
  return (
    normalized.includes("command_execution") ||
    normalized.includes("tool_call") ||
    normalized.includes("tool_use") ||
    normalized.includes("function_call") ||
    normalized.includes("command") ||
    normalized.includes("tool") ||
    normalized.includes("function") ||
    normalized.includes("bash") ||
    normalized.includes("glob") ||
    normalized.includes("grep") ||
    normalized.includes("read_file") ||
    normalized.includes("list_directory")
  );
}

function formatTraceEventLabel(type: string) {
  return type.replace(/([a-z])([A-Z])/g, "$1 $2");
}

function stringFromUnknown(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

function textFromUnknown(value: unknown) {
  return typeof value === "string" ? value : "";
}

function numberFromUnknown(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
