# First-Run Setup

Use this guide after installing or launching cocode for the first time.

## 1. Open A Repository

1. Start cocode.
2. Choose **Open local repo**.
3. Select a local Git repository that matches the PR or branch you want to review.
4. Confirm the New Thread screen shows the repository path and default branch.

## 2. Configure GitHub

1. Open Settings.
2. Paste a GitHub token with enough read access for the target repositories.
3. Save the token.
4. Confirm the Settings status shows the token as configured.

cocode stores the token through Electron safe storage. The backend stores only a credential reference and metadata, not the raw token.

## 3. Configure Review Agents

1. Open Settings and review agent presets.
2. Enable the local CLIs or stdio protocol agents you want cocode to run.
3. Use read-only review mode for MVP dogfood.
4. Run health checks before the first real review.

Available local presets:

- Codex CLI.
- Codex App Server.
- Gemini CLI.
- Gemini ACP.
- OpenCode CLI.
- OpenCode ACP.
- Claude Code preset can remain disabled until the CLI is installed locally.
- Custom CLI for deterministic local tools or internal agents.

Each CLI or protocol server command should already be installed and authenticated in your shell before cocode runs it.

For dogfood verification, run the opt-in real CLI smoke suite from `apps/desktop`:

```sh
COCODE_E2E_REAL_CLIS=codex,gemini,opencode,claude pnpm e2e:real-cli
```

The suite creates temporary saved connections, runs each selected CLI through the same Settings health-check path as the app, and expects the smoke response marker back from the command.

## 4. Run A First Review

1. Choose a source: GitHub PR URL, local branch compare, or local changes.
2. Continue to Configure Review.
3. Verify changed files, excluded files, context policy, selected agents, and runtime limit.
4. Start review.
5. Watch the event stream for context build, agent runs, normalization, verification, Evidence Map, and draft-comment phases.

## 5. Triage And Export

1. Open Findings.
2. Inspect evidence and code context for each important finding.
3. Accept actionable findings and dismiss false positives with a reason.
4. Open Evidence Map for at least one accepted finding.
5. Use Follow-up when a finding needs clarification.
6. Open Publish.
7. Copy a packet or build a GitHub preview.

No GitHub write is performed without explicit user action.

## 6. Logs

macOS logs:

- Main app: `~/Library/Logs/@cocode/desktop/main.log`
- Backend: `~/Library/Logs/@cocode/desktop/cocoded.log`

Use Settings → Open logs when available.
