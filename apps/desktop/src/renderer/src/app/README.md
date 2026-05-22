# Renderer App Structure

The desktop renderer is organized by feature, with `App.tsx` kept as the shell
coordinator. Prefer direct imports between feature modules instead of broad
barrels, so ownership and bundle boundaries stay visible.

- `agents/`: agent configuration, settings, presets, provider metadata.
- `chat/`: centralized review chat, markdown/output rendering, runtime traces,
  and shared chat composer controls.
- `evidence/`: evidence map graph, evidence formatting, and inspector panels.
- `findings/`: finding cards, detail/follow-up screens, comments, and finding
  thread rendering.
- `review/`: review session tabs, live data, details, findings board, publish
  preview, and review breadcrumbs.
- `setup/`: new review setup flow and source/diff selection.
- `shell/`: app navigation chrome.
- `shared/`: small reusable UI hooks/components and cross-feature formatting.

When adding a screen, put the screen in the owning feature folder and keep
shared primitives in `shared/` only when at least two features already need
them. Keep tests close to the feature they protect.
