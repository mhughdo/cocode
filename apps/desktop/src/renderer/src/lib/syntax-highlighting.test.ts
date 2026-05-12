import { describe, expect, it } from "vitest";

import { highlightCodeLines, languageForFilePath } from "./syntax-highlighting";

describe("syntax highlighting", () => {
  it("infers languages from common review file paths", () => {
    expect(
      languageForFilePath("services/cocoded/internal/chat/service.go"),
    ).toBe("go");
    expect(languageForFilePath("apps/desktop/src/App.tsx")).toBe("tsx");
    expect(languageForFilePath("Dockerfile")).toBe("dockerfile");
    expect(languageForFilePath("scripts/review.sh")).toBe("shellscript");
    expect(languageForFilePath("unknown.lockfile")).toBe("plaintext");
  });

  it("returns highlighted token lines with a plain-text fallback shape", async () => {
    const lines = await highlightCodeLines("const value = 1", "typescript");

    expect(lines).toHaveLength(1);
    expect(lines[0]?.map((token) => token.content).join("")).toBe(
      "const value = 1",
    );
    expect(lines[0]?.some((token) => token.color)).toBe(true);
  });
});
