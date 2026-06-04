import {
  Fragment,
  type CSSProperties,
  type ReactElement,
  type ReactNode,
  useEffect,
  useMemo,
  useState,
} from "react";

import { CheckIcon, CopyIcon } from "lucide-react";

import {
  highlightCodeLines,
  languageForFilePath,
  normalizeSyntaxLanguage,
  type HighlightedCodeLine,
} from "@/lib/syntax-highlighting";
import { cn } from "@/lib/utils";
import {
  extractDisplayableAgentOutput,
  formatKnownAgentJSONPayload,
} from "./agent-output-formatting";
import {
  type FileReferenceActions,
  type FileReferenceTarget,
  useFileReferenceActions,
} from "./file-reference-actions";

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
  const fileReferenceActions = useFileReferenceActions();
  return (
    <div
      className={cn(
        "cocode-markdown min-w-0 space-y-2 text-[13px] leading-6 [overflow-wrap:anywhere] break-words",
        muted && "text-muted-foreground",
        className,
      )}
    >
      {renderMarkdownBlocks(displayContent, fileReferenceActions)}
    </div>
  );
}

export function normalizeMarkdownMessageContent(content: string) {
  const raw = content.trim();
  if (!raw) {
    return content;
  }
  const displayable = extractDisplayableAgentOutput(raw);
  if (displayable !== raw) {
    return displayable;
  }
  return extractEscapedJSONAnswer(raw) ?? content;
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
  const displayable = extractDisplayableAgentOutput(decoded);
  return displayable !== decoded ? displayable : extractJSONAnswer(decoded);
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

function textFromUnknown(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function renderMarkdownBlocks(
  content: string,
  fileReferenceActions: FileReferenceActions | null,
) {
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
        {renderInline(paragraph.join(" "), fileReferenceActions)}
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
            {renderInline(item, fileReferenceActions)}
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
            {renderInline(line, fileReferenceActions)}
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
        <MarkdownTable
          fileReferenceActions={fileReferenceActions}
          key={`table-${blocks.length}`}
          lines={tableLines}
        />,
      );
      continue;
    }
    if (/^#{1,4}\s+/.test(trimmed)) {
      flushLoose();
      const depth = Math.min(4, trimmed.match(/^#+/)?.[0].length ?? 2);
      const text = trimmed.replace(/^#{1,4}\s+/, "");
      blocks.push(
        renderHeading(
          depth,
          text,
          `heading-${blocks.length}`,
          fileReferenceActions,
        ),
      );
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

function renderHeading(
  depth: number,
  text: string,
  key: string,
  fileReferenceActions: FileReferenceActions | null,
) {
  const className = cn(
    "font-semibold text-balance break-words [overflow-wrap:anywhere]",
    depth <= 2 ? "mt-3 text-base" : "mt-2 text-sm",
  );
  const children = renderInline(text, fileReferenceActions);
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
  if (isDiffLanguage(normalizedLanguage)) {
    return <DiffCodeBlock lines={lines} />;
  }
  return (
    <SyntaxCodeBlock
      code={lines.join("\n")}
      copyable
      language={normalizedLanguage}
    />
  );
}

export function SyntaxCodeBlock({
  className,
  code,
  copyable,
  highlightEndLine,
  highlightStartLine,
  language,
  lineNumbers,
  startLine,
}: {
  className?: string;
  code: string;
  copyable?: boolean;
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
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">(
    "idle",
  );

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

  useEffect(() => {
    if (copyState !== "copied" && copyState !== "failed") {
      return;
    }
    const timeout = window.setTimeout(() => setCopyState("idle"), 1400);
    return () => window.clearTimeout(timeout);
  }, [copyState]);

  async function copyCode() {
    try {
      if (window.cocode?.writeClipboard) {
        await window.cocode.writeClipboard(normalizedCode);
      } else if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(normalizedCode);
      } else {
        throw new Error("Clipboard API is unavailable");
      }
      setCopyState("copied");
    } catch {
      setCopyState("failed");
    }
  }

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
        "cocode-shiki-block border-border/70 bg-muted/55 relative max-w-full overflow-hidden rounded-lg border",
        className,
      )}
      style={style}
    >
      {copyable ? (
        <button
          aria-label="Copy code snippet"
          className="bg-background/80 hover:bg-background focus-visible:border-ring focus-visible:ring-ring/50 text-muted-foreground hover:text-foreground absolute top-2 right-2 z-10 flex size-7 cursor-pointer items-center justify-center rounded-md border shadow-sm transition-colors focus-visible:ring-2 focus-visible:outline-none"
          title={copyState === "failed" ? "Copy failed" : "Copy code"}
          type="button"
          onClick={() => void copyCode()}
        >
          {copyState === "copied" ? (
            <CheckIcon className="size-3.5" />
          ) : (
            <CopyIcon className="size-3.5" />
          )}
        </button>
      ) : null}
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

function isDiffLanguage(language: string) {
  const normalized = normalizeSyntaxLanguage(language).toLowerCase();
  return normalized === "diff" || normalized === "patch";
}

function DiffCodeBlock({ lines }: { lines: string[] }) {
  const renderedLines = useMemo(() => trimOuterEmptyLines(lines), [lines]);
  const code = useMemo(
    () => renderedLines.map(diffCodeContent).join("\n") || " ",
    [renderedLines],
  );
  const language = useMemo(
    () => inferDiffLanguage(renderedLines),
    [renderedLines],
  );
  const plainLines = useMemo<HighlightedCodeLine[]>(
    () => code.split(/\r?\n/).map((line) => [{ content: line || " " }]),
    [code],
  );
  const highlightKey = `${language}:${code}`;
  const [highlightResult, setHighlightResult] = useState<{
    key: string;
    lines: HighlightedCodeLine[];
  } | null>(null);
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">(
    "idle",
  );

  useEffect(() => {
    let canceled = false;
    void highlightCodeLines(code, language).then((highlightedLines) => {
      if (!canceled) {
        setHighlightResult({
          key: highlightKey,
          lines: highlightedLines.length > 0 ? highlightedLines : plainLines,
        });
      }
    });
    return () => {
      canceled = true;
    };
  }, [code, highlightKey, language, plainLines]);

  useEffect(() => {
    if (copyState !== "copied" && copyState !== "failed") {
      return;
    }
    const timeout = window.setTimeout(() => setCopyState("idle"), 1400);
    return () => window.clearTimeout(timeout);
  }, [copyState]);

  async function copyCode() {
    const rawDiff = renderedLines.join("\n") || " ";
    try {
      if (window.cocode?.writeClipboard) {
        await window.cocode.writeClipboard(rawDiff);
      } else if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(rawDiff);
      } else {
        throw new Error("Clipboard API is unavailable");
      }
      setCopyState("copied");
    } catch {
      setCopyState("failed");
    }
  }

  const highlightedLines =
    highlightResult?.key === highlightKey ? highlightResult.lines : plainLines;

  return (
    <div className="cocode-shiki-block border-border/70 bg-muted/55 relative max-w-full overflow-hidden rounded-lg border">
      <button
        aria-label="Copy code snippet"
        className="bg-background/80 hover:bg-background focus-visible:border-ring focus-visible:ring-ring/50 text-muted-foreground hover:text-foreground absolute top-2 right-2 z-10 flex size-7 cursor-pointer items-center justify-center rounded-md border shadow-sm transition-colors focus-visible:ring-2 focus-visible:outline-none"
        title={copyState === "failed" ? "Copy failed" : "Copy code"}
        type="button"
        onClick={() => void copyCode()}
      >
        {copyState === "copied" ? (
          <CheckIcon className="size-3.5" />
        ) : (
          <CopyIcon className="size-3.5" />
        )}
      </button>
      <div className="cocode-shiki cocode-diff">
        <pre className="min-w-0 overflow-auto bg-transparent">
          <code className="block min-w-full">
            {renderedLines.map((line, index) => {
              const prefix = diffLinePrefix(line);
              const lineTokens = highlightedLines[index] ?? [{ content: " " }];
              return (
                <span
                  className={cn(
                    "block px-1 whitespace-pre",
                    diffLineClass(line),
                  )}
                  key={`${index}-${line}`}
                >
                  {prefix ? (
                    <span className={diffPrefixClass(prefix)}>{prefix}</span>
                  ) : null}
                  {lineTokens.map((token, tokenIndex) => (
                    <span
                      key={`${index}-${tokenIndex}`}
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

function trimOuterEmptyLines(lines: string[]) {
  let start = 0;
  let end = lines.length;
  while (start < end && !lines[start]?.trim()) {
    start++;
  }
  while (end > start && !lines[end - 1]?.trim()) {
    end--;
  }
  return lines.slice(start, end);
}

function diffLineClass(line: string) {
  const trimmed = line.trimStart();
  if (trimmed.startsWith("@@")) {
    return "bg-sky-50";
  }
  if (trimmed.startsWith("diff --git") || trimmed.startsWith("index ")) {
    return "text-muted-foreground font-semibold";
  }
  if (trimmed.startsWith("+++") || trimmed.startsWith("---")) {
    return "text-muted-foreground";
  }
  if (trimmed.startsWith("+")) {
    return "bg-emerald-50";
  }
  if (trimmed.startsWith("-")) {
    return "bg-red-50";
  }
  return "text-foreground";
}

function diffLinePrefix(line: string) {
  const trimmedStartLength = line.length - line.trimStart().length;
  if (trimmedStartLength > 0) {
    return "";
  }
  const first = line[0];
  if (first === "+" || first === "-") {
    if (line.startsWith("+++") || line.startsWith("---")) {
      return "";
    }
    return first;
  }
  return "";
}

function diffCodeContent(line: string) {
  const prefix = diffLinePrefix(line);
  return prefix ? line.slice(1) : line;
}

function diffPrefixClass(prefix: string) {
  if (prefix === "+") {
    return "text-emerald-700";
  }
  if (prefix === "-") {
    return "text-red-700";
  }
  return "text-muted-foreground";
}

function inferDiffLanguage(lines: string[]) {
  const headerPath = lines
    .map((line) => {
      const match = line.match(
        /^(?:diff --git a\/\S+ b\/(\S+)|\+\+\+ b\/(\S+)|--- a\/(\S+))/,
      );
      return match?.[1] ?? match?.[2] ?? match?.[3] ?? "";
    })
    .find(Boolean);
  if (headerPath) {
    return languageForFilePath(headerPath);
  }
  const content = lines.map(diffCodeContent).join("\n");
  if (
    /\bfunc\s+\w+|\bpackage\s+\w+|:=|\bnil\b|\berr\s*!=\s*nil/.test(content)
  ) {
    return "go";
  }
  if (/\b(?:const|let|var|function)\b|=>|import\s+.*from/.test(content)) {
    return "typescript";
  }
  if (/\bdef\s+\w+|^\s*from\s+\S+\s+import\s+/m.test(content)) {
    return "python";
  }
  return "plaintext";
}

function MarkdownTable({
  fileReferenceActions,
  lines,
}: {
  fileReferenceActions: FileReferenceActions | null;
  lines: string[];
}) {
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
                {renderInline(cell, fileReferenceActions)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, rowIndex) => (
            <tr className="border-b last:border-b-0" key={rowIndex}>
              {row.map((cell, cellIndex) => (
                <td className="px-3 py-2 align-top" key={cellIndex}>
                  {renderInline(cell, fileReferenceActions)}
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

function renderInline(
  text: string,
  fileReferenceActions: FileReferenceActions | null,
): ReactNode[] {
  const normalizedText = normalizeInlineFileReferences(text);
  const nodes: ReactNode[] = [];
  const pattern =
    /(`[^`]+`)|(\*\*[^*]+\*\*)|(\[[^\]]+\]\([^)]+\))|((?:\.{0,2}\/)?[A-Za-z0-9_.-]+(?:\/[A-Za-z0-9_.-]+)+\.[A-Za-z0-9]+(?::L?\d+(?:-L?\d+)?)?)/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(normalizedText))) {
    if (match.index > lastIndex) {
      nodes.push(normalizedText.slice(lastIndex, match.index));
    }
    const token = match[0];
    if (token.startsWith("`")) {
      const inlineCode = token.slice(1, -1);
      const reference = parseFileReference(inlineCode);
      nodes.push(
        reference ? (
          renderFileReference(
            reference,
            inlineCode,
            match.index,
            fileReferenceActions,
          )
        ) : (
          <code
            className="bg-muted rounded px-1.5 py-0.5 font-mono text-[0.9em] [overflow-wrap:anywhere] break-words whitespace-normal"
            key={`${match.index}-code`}
          >
            {inlineCode}
          </code>
        ),
      );
    } else if (token.startsWith("**")) {
      nodes.push(
        <strong className="font-semibold" key={`${match.index}-strong`}>
          {renderInline(token.slice(2, -2), fileReferenceActions)}
        </strong>,
      );
    } else if (token.startsWith("[")) {
      nodes.push(renderLinkToken(token, match.index, fileReferenceActions));
    } else {
      nodes.push(
        renderFileReferenceToken(token, match.index, fileReferenceActions),
      );
    }
    lastIndex = match.index + token.length;
  }
  if (lastIndex < normalizedText.length) {
    nodes.push(normalizedText.slice(lastIndex));
  }
  return nodes.map((node, index) => <Fragment key={index}>{node}</Fragment>);
}

function normalizeInlineFileReferences(text: string) {
  return text
    .replace(/\[`([^`]+)`\](?!\()/g, (match, reference: string) =>
      isFileReference(reference) ? `\`${reference}\`` : match,
    )
    .replace(/\[([^\]\n]+)\](?!\()/g, (match, reference: string) =>
      isFileReference(reference) ? `\`${reference}\`` : match,
    );
}

function isFileReference(reference: string) {
  return /^(?:\.{0,2}\/)?[A-Za-z0-9_.-]+(?:\/[A-Za-z0-9_.-]+)+\.[A-Za-z0-9]+(?::L?\d+(?:-L?\d+)?|:\d+(?:-\d+)?)?$/.test(
    reference.trim(),
  );
}

function renderLinkToken(
  token: string,
  index: number,
  fileReferenceActions: FileReferenceActions | null,
) {
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
  const reference = parseFileReference(href) ?? parseFileReference(label);
  if (reference) {
    return renderFileReference(reference, label, index, fileReferenceActions);
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

function renderFileReferenceToken(
  token: string,
  index: number,
  fileReferenceActions: FileReferenceActions | null,
) {
  const reference = parseFileReference(token);
  if (!reference) {
    return (
      <code
        className="bg-muted rounded px-1.5 py-0.5 font-mono text-[0.9em] [overflow-wrap:anywhere] break-words whitespace-normal"
        key={`${index}-file`}
      >
        {token}
      </code>
    );
  }
  return renderFileReference(reference, token, index, fileReferenceActions);
}

function renderFileReference(
  reference: FileReferenceTarget,
  label: string,
  index: number,
  fileReferenceActions: FileReferenceActions | null,
) {
  const displayLabel = isFileReference(label)
    ? fileReferenceDisplayLabel(reference)
    : label;
  if (!fileReferenceActions) {
    return (
      <code
        className="bg-muted rounded px-1.5 py-0.5 font-mono text-[0.9em] [overflow-wrap:anywhere] break-words whitespace-normal"
        key={`${index}-file-reference`}
        title={reference.raw}
      >
        {displayLabel}
      </code>
    );
  }
  return (
    <button
      className="bg-muted hover:bg-muted/80 focus-visible:border-ring focus-visible:ring-ring/50 inline cursor-pointer rounded px-1.5 py-0.5 text-left font-mono text-[0.9em] [overflow-wrap:anywhere] break-words whitespace-normal underline-offset-2 transition-colors hover:underline focus-visible:ring-2 focus-visible:outline-none"
      key={`${index}-file-reference-button`}
      aria-label={`Open ${reference.raw} in the right panel`}
      title={`Open ${reference.raw} in the right panel`}
      type="button"
      onClick={() => fileReferenceActions.openFileReference(reference)}
    >
      {displayLabel}
    </button>
  );
}

function fileReferenceDisplayLabel(reference: FileReferenceTarget) {
  return reference.path.split(/[\\/]/).filter(Boolean).at(-1) || reference.path;
}

export function parseFileReference(value: string): FileReferenceTarget | null {
  const raw = value.trim().replace(/^<|>$/g, "");
  if (!isFileReference(raw)) {
    return null;
  }
  const match = raw.match(
    /^(.*?)(?::L?(\d+)(?:-L?(\d+))?|:(\d+)(?:-(\d+))?)?$/,
  );
  const path = match?.[1]?.trim() ?? raw;
  const startLine = numberFromMatch(match?.[2] ?? match?.[4]);
  const endLine = numberFromMatch(match?.[3] ?? match?.[5]) ?? startLine;
  if (!path) {
    return null;
  }
  return {
    endLine,
    path,
    raw,
    startLine,
  };
}

function numberFromMatch(value: string | undefined) {
  if (!value) {
    return undefined;
  }
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}
