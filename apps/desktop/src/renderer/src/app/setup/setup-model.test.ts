import { describe, expect, it } from "vitest";

import { setupFocusHintById, setupFocusPrompt } from "./setup-model";

describe("setupFocusPrompt", () => {
  it("renders only checked focus areas and selected files", () => {
    const prompt = setupFocusPrompt({
      files: [
        { path: "docs/prd.md", name: "prd.md", directory: "docs" },
        { path: "docs/prd.md", name: "prd.md", directory: "docs" },
      ],
      focusAreas: [
        {
          id: "quality",
          instruction: setupFocusHintById.quality,
          label: "General quality",
        },
      ],
      prompt: "Pay attention to reward accounting.",
    });

    expect(prompt).toContain("Review lenses:");
    expect(prompt).toContain("General quality:");
    expect(prompt).toContain("Context files to read first:");
    expect(prompt.split("docs/prd.md")).toHaveLength(2);
    expect(prompt).toContain("Additional reviewer context:");
    expect(prompt).toContain("Pay attention to reward accounting.");
    expect(prompt).not.toContain("Security issues");
    expect(prompt).not.toContain(setupFocusHintById.security);
  });

  it("does not keep stale generated hints in unchecked focus state", () => {
    const prompt = setupFocusPrompt({
      files: [],
      focusAreas: [],
      prompt: "",
    });

    expect(prompt).toBe("");
    expect(prompt).not.toContain("security");
  });
});
