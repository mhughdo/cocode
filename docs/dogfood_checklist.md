# cocode Dogfood Checklist

Use this checklist for each real PR before an MVP release candidate. Record the PR URL, repo path, app build/commit, selected agents, elapsed time, and any owner-approved exceptions in `docs/risk_register.md`.

## 1. Setup

- [ ] Start from a clean packaged build or a fresh `pnpm dev` session.
- [ ] Open the target local Git repository from the desktop app.
- [ ] Confirm Settings shows a configured GitHub token.
- [ ] Confirm at least one real read-only CLI agent is enabled.
- [ ] Confirm Codex, Gemini, and OpenCode presets either pass health checks or are explicitly marked unavailable for this run.
- [ ] Confirm app/backend logs are reachable before starting the review.

## 2. Snapshot

- [ ] Create a snapshot from a GitHub PR URL.
- [ ] Verify title, changed-file count, base/head refs, and changed-file rows match GitHub.
- [ ] Repeat once with a local branch compare when the PR branch exists locally.
- [ ] Verify generated, lock, vendor, and binary files are excluded or clearly marked.
- [ ] For large PRs, confirm the changed-file list stays responsive and bounded.

## 3. Configure And Run

- [ ] Select the intended read-only agents.
- [ ] Set review depth and runtime limit appropriate to PR size.
- [ ] Add a focused prompt if the PR has a known risky subsystem.
- [ ] Confirm context policy keeps secrets redacted and local-only paths out of external-agent context.
- [ ] Start the review and confirm live events show context build, agent runs, normalization, verification, and Evidence Map phases.
- [ ] If one agent fails or times out, confirm the session completes partially when another agent succeeds.

## 4. Triage

- [ ] Open the Findings tab and confirm findings render without layout overlap.
- [ ] Search/filter by severity, verification status, and text.
- [ ] Inspect every high/blocker finding and at least one medium/low finding.
- [ ] Accept at least one actionable finding with a clear reason.
- [ ] Dismiss at least one false positive with a reason.
- [ ] Save a dismissal as a local rule when it should guide future reviews.
- [ ] Restart the app and confirm findings, evidence, decisions, and draft comments persist.

## 5. Evidence Map

- [ ] Open Evidence Map for an accepted finding.
- [ ] Confirm graph nodes, code hierarchy, call path, and selected context are readable.
- [ ] Verify supporting, missing, and counter-evidence are distinguishable.
- [ ] Confirm incomplete evidence maps show a useful partial/unavailable reason.
- [ ] For large diffs, confirm map rendering stays responsive.

## 6. Follow-Up

- [ ] Ask a finding-scoped follow-up question.
- [ ] Confirm the answer cites scoped evidence and does not invent unsupported files.
- [ ] Confirm follow-up messages persist after leaving and reopening the review.
- [ ] Confirm copy actions do not expose hidden secrets or local-only paths.

## 7. Copy And Publish

- [ ] Open Publish and verify accepted findings are selected.
- [ ] Copy the selected packet and confirm the clipboard contains claims, evidence, locations, and suggested fixes.
- [ ] Build GitHub preview and inspect review body, inline comments, warnings, and checklist.
- [ ] Confirm anchor warnings fall back to summary-only output when needed.
- [ ] Confirm no GitHub write occurs without explicit user action.
- [ ] Rerun preview after a decision change and confirm duplicate/previous-comment context is reflected.

## 8. Required PR Mix

- [ ] Small PR: fewer than 10 changed files.
- [ ] Medium PR: 10-50 changed files.
- [ ] Large PR: more than 50 changed files or more than 3,000 changed lines.
- [ ] Security-sensitive PR: auth, permissions, webhooks, secrets, billing, or persistence.
- [ ] UI-heavy PR: verify layout, overflow, and evidence readability.

## 9. Result Record

- [ ] PR URL and local branch/ref.
- [ ] Agents and presets used.
- [ ] Runtime, token/cost metadata where available, and timeout/failure count.
- [ ] Accepted expected findings and false positives.
- [ ] Any missing evidence, bad anchors, UI issues, or performance stalls.
- [ ] Known issue owner, severity, and release decision.
