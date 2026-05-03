# MVP Release Candidate Review

Date: 2026-05-04

## Verdict

Release-candidate implementation review passes for the local MVP codebase. All implementation tasks in `docs/cocode_mvp_task_breakdown.md` are complete, automated checks pass, the macOS packaged app launches with the bundled backend, and the UI does not expose an automatic GitHub publish/write path.

Before a public release tag, run the real-PR dogfood checklist in `docs/dogfood_checklist.md` and attach the results to the release notes.

## Verification

- `pnpm verify`: passed.
- `pnpm --filter @cocode/desktop e2e`: passed, 7/7 Playwright tests.
- `pnpm backend:eval`: passed, 3 golden repos, 2 expected findings, 2 matched expected, 0 missing expected, 0 false positives, precision-ish 1.0, cost unavailable for static harness.
- `pnpm desktop:dist:dir`: passed, produced `apps/desktop/dist/mac-arm64/cocode.app`.
- Packaged app smoke: passed, unpacked macOS app opened, `window.cocode.getBackendInfo()` returned ready, bundled backend returned `/api/health` with `status: "ok"`.
- `git diff --check`: passed after the final review update.

## Security And Side Effects

- Renderer runs with sandbox, context isolation, disabled Node integration, and preload-only bridge exposure.
- Backend binds to localhost and requires a per-launch auth token.
- Review-mode agent selection filters out write-capable agents.
- Copy packet writes require explicit user clipboard action.
- GitHub preview persists a preview draft only; the renderer shows direct GitHub submit as unavailable.
- No auto-update or background publish flow is enabled for MVP.

## Known Issues

- macOS arm64 is the only packaged target smoke-tested in this pass. macOS x64/universal, Windows, and Linux need platform-specific backend builds and smoke runs before being advertised.
- The packaged app currently uses Electron's default icon; add a cocode app icon before a polished public release.
- Real PR dogfood is not included in automated verification. Run the small, medium, large, security-sensitive, and UI-heavy PR checklist before tagging.
- Direct GitHub submission remains intentionally disabled in the renderer for MVP; users can preview GitHub output and copy packets without hidden external writes.

## Release Artifacts To Keep

- `apps/desktop/dist/mac-arm64/cocode.app` from `pnpm desktop:dist:dir`.
- `docs/release_packaging.md`.
- `docs/first_run_setup.md`.
- `docs/troubleshooting.md`.
- `docs/dogfood_checklist.md`.
