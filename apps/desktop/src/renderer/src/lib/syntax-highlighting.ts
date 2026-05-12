import githubLight from "@shikijs/themes/github-light";
import css from "@shikijs/langs/css";
import diff from "@shikijs/langs/diff";
import dockerfile from "@shikijs/langs/dockerfile";
import go from "@shikijs/langs/go";
import html from "@shikijs/langs/html";
import javascript from "@shikijs/langs/javascript";
import json from "@shikijs/langs/json";
import jsonc from "@shikijs/langs/jsonc";
import jsx from "@shikijs/langs/jsx";
import make from "@shikijs/langs/make";
import markdown from "@shikijs/langs/markdown";
import python from "@shikijs/langs/python";
import rust from "@shikijs/langs/rust";
import scss from "@shikijs/langs/scss";
import shellscript from "@shikijs/langs/shellscript";
import sql from "@shikijs/langs/sql";
import toml from "@shikijs/langs/toml";
import tsx from "@shikijs/langs/tsx";
import typescript from "@shikijs/langs/typescript";
import yaml from "@shikijs/langs/yaml";
import { createHighlighterCore, type LanguageRegistration } from "shiki/core";
import { createJavaScriptRegexEngine } from "shiki/engine/javascript";

export const SYNTAX_THEME = "github-light";

const syntaxLanguages = [
  ...css,
  ...diff,
  ...dockerfile,
  ...go,
  ...html,
  ...javascript,
  ...json,
  ...jsonc,
  ...jsx,
  ...make,
  ...markdown,
  ...python,
  ...rust,
  ...scss,
  ...shellscript,
  ...sql,
  ...toml,
  ...tsx,
  ...typescript,
  ...yaml,
] satisfies LanguageRegistration[];

const supportedLanguages = new Set([
  "css",
  "diff",
  "dockerfile",
  "go",
  "html",
  "javascript",
  "json",
  "jsonc",
  "jsx",
  "make",
  "markdown",
  "python",
  "rust",
  "scss",
  "shellscript",
  "sql",
  "toml",
  "tsx",
  "typescript",
  "yaml",
  "plaintext",
]);

const extensionLanguages = new Map<string, string>([
  ["bash", "shellscript"],
  ["cjs", "javascript"],
  ["css", "css"],
  ["diff", "diff"],
  ["go", "go"],
  ["html", "html"],
  ["js", "javascript"],
  ["json", "json"],
  ["jsonc", "jsonc"],
  ["jsx", "jsx"],
  ["md", "markdown"],
  ["mdx", "markdown"],
  ["mjs", "javascript"],
  ["mts", "typescript"],
  ["patch", "diff"],
  ["py", "python"],
  ["rs", "rust"],
  ["scss", "scss"],
  ["sh", "shellscript"],
  ["sql", "sql"],
  ["toml", "toml"],
  ["ts", "typescript"],
  ["tsx", "tsx"],
  ["yaml", "yaml"],
  ["yml", "yaml"],
  ["zsh", "shellscript"],
]);

const filenameLanguages = new Map<string, string>([
  [".bashrc", "shellscript"],
  [".zshrc", "shellscript"],
  ["dockerfile", "dockerfile"],
  ["makefile", "make"],
]);

const languageAliases = new Map<string, string>([
  ["bash", "shellscript"],
  ["golang", "go"],
  ["js", "javascript"],
  ["md", "markdown"],
  ["node", "javascript"],
  ["patch", "diff"],
  ["plain", "plaintext"],
  ["sh", "shellscript"],
  ["shell", "shellscript"],
  ["text", "plaintext"],
  ["ts", "typescript"],
  ["zsh", "shellscript"],
]);

let highlighterPromise: ReturnType<typeof createHighlighterCore> | undefined;
const highlightedLineCache = new Map<string, HighlightedCodeLine[]>();
const maxHighlightedLineCacheEntries = 180;

export type HighlightedCodeToken = {
  color?: string;
  content: string;
  fontStyle?: number;
};

export type HighlightedCodeLine = HighlightedCodeToken[];

export function languageForFilePath(filePath: string) {
  const normalizedPath = filePath.trim().replace(/\\/g, "/");
  const filename = normalizedPath.split("/").pop()?.toLowerCase() ?? "";
  const filenameMatch = filenameLanguages.get(filename);
  if (filenameMatch) {
    return filenameMatch;
  }
  const extension = filename.includes(".")
    ? filename.slice(filename.lastIndexOf(".") + 1)
    : "";
  return extensionLanguages.get(extension) ?? "plaintext";
}

export function normalizeSyntaxLanguage(language: string) {
  const value = language.trim().toLowerCase();
  const alias = languageAliases.get(value) ?? value;
  return supportedLanguages.has(alias) ? alias : "plaintext";
}

export async function highlightCodeLines(
  code: string,
  language: string,
): Promise<HighlightedCodeLine[]> {
  const safeLanguage = normalizeSyntaxLanguage(language);
  const cacheKey = `${SYNTAX_THEME}:${safeLanguage}:${code}`;
  const cached = highlightedLineCache.get(cacheKey);
  if (cached) {
    return cached;
  }

  try {
    const highlighter = await getSyntaxHighlighter();
    const result = highlighter.codeToTokens(code, {
      lang: safeLanguage,
      theme: SYNTAX_THEME,
    });
    const lines = result.tokens.map((line) =>
      line.map((token) => ({
        color: token.color,
        content: token.content,
        fontStyle: token.fontStyle,
      })),
    );
    rememberHighlightedLines(cacheKey, lines);
    return lines;
  } catch {
    const fallback = code
      .split("\n")
      .map((line) => [{ content: line } satisfies HighlightedCodeToken]);
    rememberHighlightedLines(cacheKey, fallback);
    return fallback;
  }
}

function getSyntaxHighlighter() {
  highlighterPromise ??= createHighlighterCore({
    themes: [githubLight],
    langs: syntaxLanguages,
    engine: createJavaScriptRegexEngine({ forgiving: true }),
  });
  return highlighterPromise;
}

function rememberHighlightedLines(key: string, lines: HighlightedCodeLine[]) {
  if (highlightedLineCache.size >= maxHighlightedLineCacheEntries) {
    const oldestKey = highlightedLineCache.keys().next().value;
    if (oldestKey) {
      highlightedLineCache.delete(oldestKey);
    }
  }
  highlightedLineCache.set(key, lines);
}
