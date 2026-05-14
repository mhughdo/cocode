import {
  Fragment,
  type CSSProperties,
  type ReactElement,
  type ReactNode,
  useEffect,
  useMemo,
  useState,
} from "react";

import {
  highlightCodeLines,
  normalizeSyntaxLanguage,
  type HighlightedCodeLine,
} from "@/lib/syntax-highlighting";
import { cn } from "@/lib/utils";
import { formatKnownAgentJSONPayload } from "./agent-output-formatting";

export function MarkdownMessage({
  className,
  content,
  muted,
}: {
  className?: string;
  content: string;
  muted?: boolean;
}) {
  const displayContent = normalizeMarkdownMessageContent(content);
  return (
    <div
      className={cn(
        "cocode-markdown min-w-0 space-y-2 text-[13px] leading-6 [overflow-wrap:anywhere] break-words",
        muted && "text-muted-foreground",
        className,
      )}
    >
      {renderMarkdownBlocks(displayContent)}
    </div>
  );
}

export function normalizeMarkdownMessageContent(content: string) {
  const raw = content.trim();
  if (!raw) {
    return content;
  }
  const extracted =
    extractJSONLinesAnswer(raw) ??
    extractJSONFenceAnswer(raw) ??
    extractEmbeddedJSONAnswer(raw) ??
    extractJSONAnswer(raw) ??
    extractEscapedJSONAnswer(raw);
  return extracted ?? content;
}

function extractJSONFenceAnswer(raw: string) {
  const match = raw.match(/```(?:json)?\s*([\s\S]*?)\s*```/i);
  const fenced = match?.[1]?.trim();
  return fenced ? extractJSONAnswer(fenced) : null;
}

function extractJSONLinesAnswer(raw: string) {
  if (!raw.includes("\n")) {
    return null;
  }
  let answer = "";
  let parsedAny = false;
  for (const line of raw.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed.startsWith("{") && !trimmed.startsWith("[")) {
      continue;
    }
    const extracted = extractJSONAnswer(trimmed);
    if (extracted !== null) {
      parsedAny = true;
      answer = extracted;
    }
  }
  return parsedAny ? answer : null;
}

function extractEmbeddedJSONAnswer(raw: string) {
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
  return extractJSONAnswer(candidate);
}

function extractEscapedJSONAnswer(raw: string) {
  if (!raw.includes("\\n") && !raw.includes('\\"')) {
    return null;
  }
  const decoded = raw
    .replace(/\\r\\n/g, "\n")
    .replace(/\\n/g, "\n")
    .replace(/\\"/g, '"')
    .trim();
  if (decoded === raw) {
    return null;
  }
  return (
    extractJSONFenceAnswer(decoded) ??
    extractEmbeddedJSONAnswer(decoded) ??
    extractJSONAnswer(decoded)
  );
}

function extractJSONAnswer(raw: string): string | null {
  try {
    const parsed = JSON.parse(raw) as unknown;
    return formatKnownAgentJSONPayload(parsed) ?? answerFromUnknown(parsed);
  } catch {
    return null;
  }
}

function answerFromUnknown(value: unknown): string | null {
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (!trimmed) {
      return null;
    }
    if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
      return extractJSONAnswer(trimmed) ?? trimmed;
    }
    return trimmed;
  }
  if (Array.isArray(value)) {
    return value.reduce<string | null>(
      (latest, item) => answerFromUnknown(item) ?? latest,
      null,
    );
  }
  if (!isPlainRecord(value)) {
    return null;
  }
  if (isIgnorableAnswerRecord(value)) {
    return null;
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
    const extracted = answerFromUnknown(value[key]);
    if (extracted) {
      return extracted;
    }
  }
  for (const key of ["item", "part"]) {
    const extracted = answerFromUnknown(value[key]);
    if (extracted) {
      return extracted;
    }
  }
  return null;
}

function isIgnorableAnswerRecord(value: Record<string, unknown>) {
  const type = textFromUnknown(value.type).toLowerCase();
  const subtype = textFromUnknown(value.subtype).toLowerCase();
  const hookName = textFromUnknown(value.hook_name).toLowerCase();
  const nestedItem = isPlainRecord(value.item) ? value.item : undefined;
  const nestedType = textFromUnknown(nestedItem?.type).toLowerCase();
  return (
    (type === "system" && (subtype.includes("hook") || Boolean(hookName))) ||
    type === "thread.started" ||
    type === "turn.started" ||
    type === "session.update" ||
    nestedType.includes("command_execution") ||
    nestedType.includes("tool") ||
    nestedType.includes("function")
  );
}

function structuredFindingsFromRecord(value: Record<string, unknown>) {
  const rawFindings = Array.isArray(value.findings)
    ? value.findings
    : isPlainRecord(value.finding)
      ? [value.finding]
      : [];
  return rawFindings.filter(isPlainRecord);
}

function formatStructuredFindingsMarkdown(findings: Record<string, unknown>[]) {
  const blocks = findings.slice(0, 8).map((finding, index) => {
    const title =
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
      firstLocationPath(finding.locations);
    const line =
      numberText(finding.line) ||
      numberText(finding.start_line) ||
      firstLocationLine(finding.locations);
    const description =
      textFromUnknown(finding.body) ||
      textFromUnknown(finding.description) ||
      textFromUnknown(finding.evidence) ||
      textFromUnknown(finding.summary);
    const fix =
      textFromUnknown(finding.suggested_fix) ||
      textFromUnknown(finding.recommendation) ||
      textFromUnknown(finding.fix);
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
  return textFromUnknown(location?.path) || textFromUnknown(location?.file);
}

function firstLocationLine(value: unknown) {
  if (!Array.isArray(value)) {
    return "";
  }
  const location = value.find(isPlainRecord);
  return numberText(location?.start_line) || numberText(location?.line);
}

function numberText(value: unknown) {
  return typeof value === "number" && Number.isFinite(value)
    ? String(value)
    : textFromUnknown(value);
}

function textFromUnknown(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function renderMarkdownBlocks(content: string) {
  const lines = content.replace(/\r\n/g, "\n").split("\n");
  const blocks: ReactElement[] = [];
  let paragraph: string[] = [];
  let listItems: string[] = [];
  let listType: "ol" | "ul" | null = null;
  let listStart = 1;
  let quoteLines: string[] = [];
  let codeLines: string[] = [];
  let codeLanguage = "";
  let inCode = false;

  const flushParagraph = () => {
    if (paragraph.length === 0) {
      return;
    }
    blocks.push(
      <p
        className="min-w-0 [overflow-wrap:anywhere]"
        key={`p-${blocks.length}`}
      >
        {renderInline(paragraph.join(" "))}
      </p>,
    );
    paragraph = [];
  };
  const flushList = () => {
    if (!listType || listItems.length === 0) {
      return;
    }
    const Tag = listType === "ol" ? "ol" : "ul";
    blocks.push(
      <Tag
        className={cn(
          "space-y-1 pl-5",
          listType === "ol" ? "list-decimal" : "list-disc",
        )}
        key={`list-${blocks.length}`}
        start={listType === "ol" && listStart > 1 ? listStart : undefined}
      >
        {listItems.map((item, index) => (
          <li
            className="min-w-0 [overflow-wrap:anywhere]"
            key={`${index}-${item}`}
          >
            {renderInline(item)}
          </li>
        ))}
      </Tag>,
    );
    listItems = [];
    listType = null;
    listStart = 1;
  };
  const flushQuote = () => {
    if (quoteLines.length === 0) {
      return;
    }
    blocks.push(
      <blockquote
        className="border-muted-foreground/25 text-muted-foreground border-l-2 pl-3"
        key={`quote-${blocks.length}`}
      >
        {quoteLines.map((line, index) => (
          <p
            className="min-w-0 [overflow-wrap:anywhere]"
            key={`${index}-${line}`}
          >
            {renderInline(line)}
          </p>
        ))}
      </blockquote>,
    );
    quoteLines = [];
  };
  const flushCode = () => {
    if (codeLines.length === 0) {
      return;
    }
    blocks.push(
      <CodeBlock
        key={`code-${blocks.length}`}
        language={codeLanguage}
        lines={codeLines}
      />,
    );
    codeLines = [];
    codeLanguage = "";
  };
  const flushLoose = () => {
    flushParagraph();
    flushList();
    flushQuote();
  };

  for (let index = 0; index < lines.length; index++) {
    const line = lines[index] ?? "";
    const trimmed = line.trim();
    if (trimmed.startsWith("```")) {
      if (inCode) {
        inCode = false;
        flushCode();
      } else {
        flushLoose();
        inCode = true;
        codeLanguage = trimmed.replace(/^```/, "").trim().toLowerCase();
      }
      continue;
    }
    if (inCode) {
      codeLines.push(line);
      continue;
    }
    if (trimmed === "") {
      flushLoose();
      continue;
    }
    if (looksLikeTableStart(lines, index)) {
      flushLoose();
      const tableLines = [line, lines[index + 1] ?? ""];
      index += 2;
      while (index < lines.length && lines[index]?.includes("|")) {
        tableLines.push(lines[index] ?? "");
        index++;
      }
      index--;
      blocks.push(
        <MarkdownTable key={`table-${blocks.length}`} lines={tableLines} />,
      );
      continue;
    }
    if (/^#{1,4}\s+/.test(trimmed)) {
      flushLoose();
      const depth = Math.min(4, trimmed.match(/^#+/)?.[0].length ?? 2);
      const text = trimmed.replace(/^#{1,4}\s+/, "");
      blocks.push(renderHeading(depth, text, `heading-${blocks.length}`));
      continue;
    }
    const ordered = trimmed.match(/^(\d+)[.)]\s+(.*)$/);
    const unordered = trimmed.match(/^[-*]\s+(.*)$/);
    if (ordered || unordered) {
      flushParagraph();
      flushQuote();
      const nextType = ordered ? "ol" : "ul";
      if (listType && listType !== nextType) {
        flushList();
      }
      listType = nextType;
      if (ordered && listItems.length === 0) {
        listStart = Number.parseInt(ordered[1] ?? "1", 10) || 1;
      }
      listItems.push(ordered?.[2] ?? unordered?.[1] ?? "");
      continue;
    }
    if (trimmed.startsWith(">")) {
      flushParagraph();
      flushList();
      quoteLines.push(trimmed.replace(/^>\s?/, ""));
      continue;
    }
    flushList();
    flushQuote();
    paragraph.push(trimmed);
  }
  if (inCode) {
    flushCode();
  }
  flushLoose();
  return blocks.length > 0 ? blocks : <p>{content}</p>;
}

function renderHeading(depth: number, text: string, key: string) {
  const className = cn(
    "font-semibold text-balance break-words [overflow-wrap:anywhere]",
    depth <= 2 ? "mt-3 text-base" : "mt-2 text-sm",
  );
  const children = renderInline(text);
  if (depth <= 1) {
    return (
      <h2 className={className} key={key}>
        {children}
      </h2>
    );
  }
  if (depth === 2) {
    return (
      <h3 className={className} key={key}>
        {children}
      </h3>
    );
  }
  if (depth === 3) {
    return (
      <h4 className={className} key={key}>
        {children}
      </h4>
    );
  }
  return (
    <h5 className={className} key={key}>
      {children}
    </h5>
  );
}

function CodeBlock({ language, lines }: { language: string; lines: string[] }) {
  const normalizedLanguage =
    language || (looksLikeDiffBlock(lines) ? "diff" : "plaintext");
  return (
    <SyntaxCodeBlock code={lines.join("\n")} language={normalizedLanguage} />
  );
}

export function SyntaxCodeBlock({
  className,
  code,
  highlightEndLine,
  highlightStartLine,
  language,
  lineNumbers,
  startLine,
}: {
  className?: string;
  code: string;
  highlightEndLine?: number;
  highlightStartLine?: number;
  language?: string;
  lineNumbers?: boolean;
  startLine?: number;
}) {
  const normalizedLanguage = normalizeSyntaxLanguage(language ?? "");
  const normalizedCode = code.trimEnd() || " ";
  const plainLines = useMemo<HighlightedCodeLine[]>(
    () =>
      normalizedCode.split(/\r?\n/).map((line) => [
        {
          content: line || " ",
        },
      ]),
    [normalizedCode],
  );
  const highlightKey = `${normalizedLanguage}:${normalizedCode}`;
  const [highlightResult, setHighlightResult] = useState<{
    key: string;
    lines: HighlightedCodeLine[];
  } | null>(null);

  useEffect(() => {
    let canceled = false;
    void highlightCodeLines(normalizedCode, normalizedLanguage).then(
      (lines) => {
        if (!canceled) {
          setHighlightResult({
            key: highlightKey,
            lines: lines.length > 0 ? lines : plainLines,
          });
        }
      },
    );
    return () => {
      canceled = true;
    };
  }, [highlightKey, normalizedCode, normalizedLanguage, plainLines]);

  const highlightedLines =
    highlightResult?.key === highlightKey ? highlightResult.lines : plainLines;

  const style =
    lineNumbers && startLine && startLine > 1
      ? ({
          "--line-start": String(startLine),
        } as CSSProperties)
      : undefined;

  return (
    <div
      className={cn(
        "cocode-shiki-block border-border/70 bg-muted/55 max-w-full overflow-hidden rounded-lg border",
        className,
      )}
      style={style}
    >
      <div className={cn("cocode-shiki", lineNumbers && "rs-has-line-numbers")}>
        <pre className="min-w-0 overflow-auto bg-transparent">
          <code className="block min-w-max">
            {highlightedLines.map((lineTokens, lineIndex) => {
              const actualLine = (startLine ?? 1) + lineIndex;
              const isHighlighted =
                highlightStartLine &&
                actualLine >= highlightStartLine &&
                actualLine <= (highlightEndLine ?? highlightStartLine);
              return (
                <span
                  className={cn(
                    "block whitespace-pre",
                    lineNumbers && "rs-line-number",
                    isHighlighted && "rs-highlighted-line",
                  )}
                  data-line-number={actualLine}
                  key={`${lineIndex}-${lineTokens
                    .map((token) => token.content)
                    .join("")}`}
                >
                  {lineTokens.map((token, tokenIndex) => (
                    <span
                      key={`${lineIndex}-${tokenIndex}`}
                      style={
                        token.color
                          ? {
                              color: token.color,
                            }
                          : undefined
                      }
                    >
                      {token.content}
                    </span>
                  ))}
                </span>
              );
            })}
          </code>
        </pre>
      </div>
    </div>
  );
}

function looksLikeDiffBlock(lines: string[]) {
  return lines.some((line) => {
    const trimmed = line.trimStart();
    return (
      trimmed.startsWith("diff --git") ||
      trimmed.startsWith("@@") ||
      trimmed.startsWith("+++") ||
      trimmed.startsWith("---")
    );
  });
}

function MarkdownTable({ lines }: { lines: string[] }) {
  const header = splitTableRow(lines[0] ?? "");
  const rows = lines
    .slice(2)
    .map(splitTableRow)
    .filter((row) => row.length > 0);
  return (
    <div className="max-w-full overflow-x-auto rounded-lg border">
      <table className="w-full min-w-max border-collapse text-left text-xs">
        <thead className="bg-muted/60">
          <tr>
            {header.map((cell, index) => (
              <th className="border-b px-3 py-2 font-semibold" key={index}>
                {renderInline(cell)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, rowIndex) => (
            <tr className="border-b last:border-b-0" key={rowIndex}>
              {row.map((cell, cellIndex) => (
                <td className="px-3 py-2 align-top" key={cellIndex}>
                  {renderInline(cell)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function looksLikeTableStart(lines: string[], index: number) {
  const current = lines[index] ?? "";
  const next = lines[index + 1] ?? "";
  return (
    current.includes("|") &&
    /^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?\s*$/.test(next)
  );
}

function splitTableRow(line: string) {
  return line
    .trim()
    .replace(/^\|/, "")
    .replace(/\|$/, "")
    .split("|")
    .map((cell) => cell.trim());
}

function renderInline(text: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  const pattern =
    /(`[^`]+`)|(\*\*[^*]+\*\*)|(\[[^\]]+\]\([^)]+\))|((?:\.{0,2}\/)?[A-Za-z0-9_.-]+(?:\/[A-Za-z0-9_.-]+)+\.[A-Za-z0-9]+(?::L?\d+(?:-L?\d+)?)?)/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(text))) {
    if (match.index > lastIndex) {
      nodes.push(text.slice(lastIndex, match.index));
    }
    const token = match[0];
    if (token.startsWith("`")) {
      nodes.push(
        <code
          className="bg-muted rounded px-1.5 py-0.5 font-mono text-[0.9em] [overflow-wrap:anywhere] break-words whitespace-normal"
          key={`${match.index}-code`}
        >
          {token.slice(1, -1)}
        </code>,
      );
    } else if (token.startsWith("**")) {
      nodes.push(
        <strong className="font-semibold" key={`${match.index}-strong`}>
          {renderInline(token.slice(2, -2))}
        </strong>,
      );
    } else if (token.startsWith("[")) {
      nodes.push(renderLinkToken(token, match.index));
    } else {
      nodes.push(
        <code
          className="bg-muted rounded px-1.5 py-0.5 font-mono text-[0.9em] [overflow-wrap:anywhere] break-words whitespace-normal"
          key={`${match.index}-file`}
        >
          {token}
        </code>,
      );
    }
    lastIndex = match.index + token.length;
  }
  if (lastIndex < text.length) {
    nodes.push(text.slice(lastIndex));
  }
  return nodes.map((node, index) => <Fragment key={index}>{node}</Fragment>);
}

function renderLinkToken(token: string, index: number) {
  const match = token.match(/^\[([^\]]+)\]\(([^)]+)\)$/);
  if (!match) {
    return token;
  }
  const [, label, href] = match;
  if (/^https?:\/\//.test(href)) {
    return (
      <a
        className="text-primary underline-offset-2 hover:underline"
        href={href}
        key={`${index}-link`}
        rel="noreferrer"
        target="_blank"
      >
        {label}
      </a>
    );
  }
  return (
    <code
      className="bg-muted rounded px-1.5 py-0.5 font-mono text-[0.9em] [overflow-wrap:anywhere] break-words whitespace-normal"
      key={`${index}-local-link`}
      title={href}
    >
      {label}
    </code>
  );
}
