# cocode MVP Release Checklist

Use this checklist before tagging an MVP release candidate. A release is ready only when every required item is checked or has a documented owner-approved exception in the risk register.

## Feature Readiness

- [ ] User can open a local Git repository through the desktop app.
- [ ] User can create a PR snapshot from GitHub URL, local branch compare, or local changes.
- [ ] User can configure review agents, context policy, and runtime limits.
- [ ] Review workflow runs with fake agents and at least one real local CLI preset.
- [ ] Findings are normalized, deduped, ranked, verified, and persisted.
- [ ] Finding detail shows evidence, draft comment, suggested fix, and decision actions.
- [ ] Evidence Map loads for complete and partial graph states.
- [ ] Follow-up questions run against finding-scoped context and persist answers.
- [ ] Copy packets render selected and accepted findings in all supported formats.
- [ ] GitHub preview produces body, inline comments, warnings, and summary-only fallback.
- [ ] GitHub publish is human-gated and records publication state.

## Quality Gates

- [ ] `services/cocoded`: `go test ./...` passes.
- [ ] Desktop/renderer: typecheck, lint, and build smoke pass.
- [ ] Backend unit tests cover parsers, diff mapping, redaction, packet rendering, and verification rules.
- [ ] Integration tests cover temp SQLite DB, fake repo, fake agents, and fake GitHub server.
- [ ] E2E smoke covers new thread, configure review, fake review, finding detail, Evidence Map, copy packet, and GitHub preview.
- [ ] Evaluation harness runs against golden repos and reports expected findings.

## Security And Privacy

- [ ] Electron uses `contextIsolation`, sandboxed renderer, disabled Node integration, and a restrictive CSP.
- [ ] Backend binds to localhost and enforces per-launch auth token.
- [ ] Browser-origin checks reject unexpected origins.
- [ ] Path sandbox tests prove workspace/app directory boundaries.
- [ ] CLI agent environment uses an explicit allowlist.
- [ ] Secret redaction report is generated for context bundles.
- [ ] Local-only files are excluded from external/cloud-backed agent context.
- [ ] Review-mode agents cannot modify files through cocode-managed tools.
- [ ] Publish/copy/write side effects require explicit user action.

## Packaging And Operations

- [ ] macOS packaged build launches desktop app and bundled backend.
- [ ] App log location is documented and reachable.
- [ ] First-run setup guide covers GitHub token, CLI presets, and troubleshooting.
- [ ] Signing/notarization requirements are documented, even if manual.
- [ ] Update strategy is documented as MVP manual update or future auto-update.

## Dogfood Criteria

- [ ] Run cocode on at least three real PRs: small, medium, and large diff.
- [ ] Verify large-diff review remains responsive and bounded by context budgets.
- [ ] Confirm duplicate publish prevention on a rerun.
- [ ] Confirm summary-only fallback when inline anchors fail.
- [ ] Capture known issues with owners before release.
