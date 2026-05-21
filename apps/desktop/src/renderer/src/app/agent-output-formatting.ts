type Extraction = string | null;

export function extractDisplayableAgentOutput(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) {
    return "";
  }
  const extracted =
    extractKnownAgentJSONText(trimmed) ??
    extractKnownAgentJSONFence(trimmed) ??
    extractKnownEmbeddedAgentJSON(trimmed);
  return extracted ?? trimmed;
}

export function extractKnownAgentJSONText(raw: string): Extraction {
  const parsedWhole = parseJSON(raw);
  if (parsedWhole !== undefined) {
    return formatKnownAgentJSONPayload(parsedWhole);
  }
  if (!raw.includes("\n")) {
    return null;
  }
  let handled = false;
  let latest = "";
  for (const line of raw.split(/\r?\n/)) {
    const candidate = line.trim();
    if (
      !candidate ||
      (!candidate.startsWith("{") && !candidate.startsWith("["))
    ) {
      continue;
    }
    const parsed = parseJSON(candidate);
    if (parsed === undefined) {
      continue;
    }
    const formatted = formatKnownAgentJSONPayload(parsed);
    if (formatted !== null) {
      handled = true;
      if (formatted) {
        latest = formatted;
      }
    }
  }
  return handled ? latest : null;
}

export function formatKnownAgentJSONPayload(
  value: unknown,
  depth = 0,
): Extraction {
  if (depth > 8) {
    return null;
  }
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (!trimmed) {
      return "";
    }
    if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
      const parsed = parseJSON(trimmed);
      if (parsed !== undefined) {
        const extracted = formatKnownAgentJSONPayload(parsed, depth + 1);
        if (extracted !== null) {
          return extracted;
        }
      }
    }
    return trimmed;
  }
  if (Array.isArray(value)) {
    let handled = false;
    const parts: string[] = [];
    for (const item of value) {
      const formatted = formatKnownAgentJSONPayload(item, depth + 1);
      if (formatted !== null) {
        handled = true;
        if (formatted) {
          parts.push(formatted);
        }
      }
    }
    return handled ? parts.join("\n\n") : null;
  }
  if (!isPlainRecord(value)) {
    return null;
  }
  if (isIgnorableAgentEvent(value)) {
    return "";
  }

  const wrapped = extractWrappedAgentPayload(value, depth);
  if (wrapped !== null) {
    return wrapped;
  }

  const verification = formatVerificationMarkdown(value);
  if (verification) {
    return verification;
  }

  const findings = structuredFindingsFromRecord(value);
  if (findings.length > 0) {
    return formatStructuredFindingsMarkdown(findings);
  }

  for (const key of [
    "answer",
    "content",
    "message",
    "summary",
    "text",
    "output",
    "result",
    "response",
    "delta",
    "value",
  ]) {
    if (!(key in value)) {
      continue;
    }
    const formatted = formatKnownAgentJSONPayload(value[key], depth + 1);
    if (formatted !== null) {
      return formatted;
    }
  }

  return null;
}

function extractKnownAgentJSONFence(raw: string): Extraction {
  const match = raw.match(/```(?:json)?\s*([\s\S]*?)\s*```/i);
  const fenced = match?.[1]?.trim();
  return fenced ? extractKnownAgentJSONText(fenced) : null;
}

function extractKnownEmbeddedAgentJSON(raw: string): Extraction {
  if (!raw.includes("{") || !raw.includes("}")) {
    return null;
  }
  const start = raw.indexOf("{");
  const end = raw.lastIndexOf("}");
  if (start < 0 || end <= start) {
    return null;
  }
  const candidate = raw.slice(start, end + 1).trim();
  if (
    !candidate.includes('"findings"') &&
    !candidate.includes('"response"') &&
    !candidate.includes('"verification_status"') &&
    !candidate.includes('"evidence_summary"')
  ) {
    return null;
  }
  return extractKnownAgentJSONText(candidate);
}

function extractWrappedAgentPayload(
  value: Record<string, unknown>,
  depth: number,
): Extraction {
  for (const key of ["item", "part", "delta", "event"]) {
    if (!(key in value)) {
      continue;
    }
    const formatted = formatKnownAgentJSONPayload(value[key], depth + 1);
    if (formatted !== null) {
      return formatted;
    }
  }
  return null;
}

function isIgnorableAgentEvent(value: Record<string, unknown>) {
  const type = textFromUnknown(value.type).toLowerCase();
  const subtype = textFromUnknown(value.subtype).toLowerCase();
  const hookName = textFromUnknown(value.hook_name).toLowerCase();
  const hookEvent = textFromUnknown(value.hook_event).toLowerCase();
  const item = isPlainRecord(value.item) ? value.item : undefined;
  const part = isPlainRecord(value.part) ? value.part : undefined;
  const delta = isPlainRecord(value.delta) ? value.delta : undefined;
  const typeBundle = [
    type,
    textFromUnknown(item?.type).toLowerCase(),
    textFromUnknown(part?.type).toLowerCase(),
    textFromUnknown(delta?.type).toLowerCase(),
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
    typeBundle.includes("command_execution") ||
    typeBundle.includes("tool_call") ||
    typeBundle.includes("tool_use") ||
    typeBundle.includes("function_call")
  );
}

function formatVerificationMarkdown(value: Record<string, unknown>) {
  const status = textFromUnknown(value.verification_status);
  const confidence = percentText(value.confidence);
  const evidenceSummary = textFromUnknown(value.evidence_summary);
  const counterEvidence = textFromUnknown(value.counter_evidence_summary);
  const suggestedFix =
    textFromUnknown(value.suggested_fix) ||
    textFromUnknown(value.recommended_fix) ||
    textFromUnknown(value.fix);
  const evidence = Array.isArray(value.evidence)
    ? value.evidence.filter(isPlainRecord)
    : [];
  if (
    !status &&
    !confidence &&
    !evidenceSummary &&
    !counterEvidence &&
    !suggestedFix &&
    evidence.length === 0
  ) {
    return "";
  }

  const lines = [
    status
      ? `### Verification: ${formatLabel(status)}`
      : "### Verification result",
    confidence && `- **Confidence:** ${confidence}`,
    evidenceSummary && `\n**Evidence summary**\n\n${evidenceSummary}`,
    counterEvidence && `\n**Counter evidence**\n\n${counterEvidence}`,
  ].filter(Boolean);

  if (evidence.length > 0) {
    lines.push("\n**Evidence checked**");
    for (const item of evidence.slice(0, 6)) {
      lines.push(`- ${formatEvidenceBullet(item)}`);
    }
    const omitted = evidence.length - 6;
    if (omitted > 0) {
      lines.push(`- ${omitted} more evidence item${omitted === 1 ? "" : "s"}`);
    }
  }
  if (suggestedFix) {
    lines.push(`\n**Suggested fix**\n\n${suggestedFix}`);
  }
  return lines.join("\n");
}

function formatEvidenceBullet(item: Record<string, unknown>) {
  const kind = formatLabel(textFromUnknown(item.kind));
  const title =
    textFromUnknown(item.title) || textFromUnknown(item.summary) || "Evidence";
  const summary = textFromUnknown(item.summary);
  const path = textFromUnknown(item.path) || textFromUnknown(item.file);
  const start = numberText(item.start_line) || numberText(item.line);
  const end = numberText(item.end_line);
  const location = path
    ? ` (\`${path}${start ? `:L${start}${end && end !== start ? `-L${end}` : ""}` : ""}\`)`
    : "";
  const prefix = kind ? `**${kind}:** ` : "";
  const detail = summary && summary !== title ? ` - ${summary}` : "";
  return `${prefix}${title}${location}${detail}`;
}

function structuredFindingsFromRecord(value: Record<string, unknown>) {
  const rawFindings = Array.isArray(value.findings)
    ? value.findings
    : Array.isArray(value.clusters)
      ? value.clusters
      : isPlainRecord(value.finding)
        ? [value.finding]
        : [];
  return rawFindings.filter(isPlainRecord);
}

function formatStructuredFindingsMarkdown(findings: Record<string, unknown>[]) {
  const blocks = findings.slice(0, 8).map((finding, index) => {
    const title =
      textFromUnknown(finding.canonical_claim) ||
      textFromUnknown(finding.title) ||
      textFromUnknown(finding.claim) ||
      textFromUnknown(finding.message) ||
      textFromUnknown(finding.description) ||
      `Finding ${index + 1}`;
    const severity = textFromUnknown(finding.severity);
    const category = textFromUnknown(finding.category);
    const path =
      textFromUnknown(finding.path) ||
      textFromUnknown(finding.file) ||
      locationPath(finding.primary_location) ||
      firstLocationPath(finding.locations);
    const line =
      numberText(finding.line) ||
      numberText(finding.start_line) ||
      locationLine(finding.primary_location) ||
      firstLocationLine(finding.locations);
    const description =
      textFromUnknown(finding.evidence_summary) ||
      textFromUnknown(finding.body) ||
      textFromUnknown(finding.description) ||
      textFromUnknown(finding.evidence) ||
      textFromUnknown(finding.summary);
    const counterEvidence = textFromUnknown(finding.counter_evidence_summary);
    const status = textFromUnknown(finding.verification_status);
    const confidence = percentText(finding.confidence);
    const fix =
      textFromUnknown(finding.suggested_fix) ||
      textFromUnknown(finding.recommendation) ||
      textFromUnknown(finding.fix);
    const evidence = structuredFindingEvidenceItems(finding);
    return [
      `### ${index + 1}. ${title}`,
      severity && `- **Severity:** ${severity}`,
      status && `- **Status:** ${formatLabel(status)}`,
      confidence && `- **Confidence:** ${confidence}`,
      category && `- **Category:** ${category}`,
      path && `- **Location:** \`${path}${line ? `:${line}` : ""}\``,
      description && `- **Evidence:** ${description}`,
      counterEvidence && `- **Verification checks:** ${counterEvidence}`,
      evidence.length > 0 &&
        [
          "- **Evidence map:**",
          ...evidence
            .slice(0, 6)
            .map((item) => `  - ${formatEvidenceBullet(item)}`),
          evidence.length > 6
            ? `  - ${evidence.length - 6} more evidence item${
                evidence.length - 6 === 1 ? "" : "s"
              }`
            : "",
        ]
          .filter(Boolean)
          .join("\n"),
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

function structuredFindingEvidenceItems(finding: Record<string, unknown>) {
  return [
    ...recordsFromUnknown(finding.supporting_evidence),
    ...recordsFromUnknown(finding.refuting_evidence),
    ...recordsFromUnknown(finding.relationship_evidence),
    ...recordsFromUnknown(finding.related_context),
    ...recordsFromUnknown(finding.evidence),
  ];
}

function recordsFromUnknown(value: unknown) {
  return Array.isArray(value) ? value.filter(isPlainRecord) : [];
}

function locationPath(value: unknown) {
  return isPlainRecord(value)
    ? textFromUnknown(value.path) || textFromUnknown(value.file)
    : "";
}

function locationLine(value: unknown) {
  return isPlainRecord(value)
    ? numberText(value.start_line) || numberText(value.line)
    : "";
}

function firstLocationPath(value: unknown) {
  if (!Array.isArray(value)) {
    return "";
  }
  const location = value.find(isPlainRecord);
  return textFromUnknown(location?.path) || textFromUnknown(location?.file);
}

function firstLocationLine(value: unknown) {
  if (!Array.isArray(value)) {
    return "";
  }
  const location = value.find(isPlainRecord);
  return numberText(location?.start_line) || numberText(location?.line);
}

function percentText(value: unknown) {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value <= 1 ? `${Math.round(value * 100)}%` : `${Math.round(value)}%`;
  }
  return textFromUnknown(value);
}

function numberText(value: unknown) {
  return typeof value === "number" && Number.isFinite(value)
    ? String(value)
    : textFromUnknown(value);
}

function formatLabel(value: string) {
  return value
    .replace(/[_-]+/g, " ")
    .trim()
    .replace(/\b\w/g, (character) => character.toUpperCase());
}

function parseJSON(value: string) {
  try {
    return JSON.parse(value) as unknown;
  } catch {
    return undefined;
  }
}

function textFromUnknown(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
