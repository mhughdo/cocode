import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { InboxIcon } from "lucide-react";

import { EmptyState, ErrorState, LoadingRows } from "./chrome";

describe("shared app states", () => {
  it("renders empty states with title, description, and action", () => {
    const html = renderToStaticMarkup(
      <EmptyState
        title="No accepted findings"
        description="Accept findings before building a publish preview."
        icon={InboxIcon}
        action={<button type="button">Review findings</button>}
      />,
    );

    expect(html).toContain("No accepted findings");
    expect(html).toContain(
      "Accept findings before building a publish preview.",
    );
    expect(html).toContain("Review findings");
  });

  it("renders error states with actionable copy", () => {
    const html = renderToStaticMarkup(
      <ErrorState
        title="Event stream disconnected"
        description="Reconnect failed after the latest poll."
        action={<button type="button">Retry</button>}
      />,
    );

    expect(html).toContain("Event stream disconnected");
    expect(html).toContain("Reconnect failed after the latest poll.");
    expect(html).toContain("Retry");
  });

  it("renders loading rows with a stable progress label", () => {
    const html = renderToStaticMarkup(<LoadingRows rows={3} />);

    expect(html).toContain("Loading latest local state");
    expect(html.match(/data-slot="skeleton"/g)?.length).toBe(9);
  });
});
