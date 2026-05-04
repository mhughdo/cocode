# Troubleshooting

## App Does Not Open

- Run `pnpm desktop:dist:dir` again and check for build errors.
- Confirm `apps/desktop/dist/mac-arm64/cocode.app` exists.
- Open `~/Library/Logs/@cocode/desktop/main.log`.
- If the backend is ready but no window appears, check for renderer load errors in the app process output.

## Backend Not Ready

- Check `~/Library/Logs/@cocode/desktop/cocoded.log`.
- Confirm no other process is forcing `COCODED_ADDR`.
- Confirm the app can bind a localhost port.
- In packaged builds, confirm `Contents/Resources/cocoded` exists and is executable.
- In development, confirm `go run ./cmd/cocoded` works from `services/cocoded`.

## GitHub Token Fails

- Confirm the token is not empty and has repository read access.
- Confirm the GitHub API is reachable.
- For local fake-GitHub tests, set `COCODED_GITHUB_API_BASE_URL`.
- Delete and re-save the token if safe storage was reset.
- Check backend logs for `/api/credentials/github` validation errors.

## Missing Agent Command

- Run the CLI or protocol server command directly in a normal shell first.
- Confirm the command is on the app process `PATH`.
- Use the agent health check in Settings.
- For custom CLIs, avoid shell wrappers unless the command policy explicitly allows them.
- Confirm the agent is read-only for review mode.
- For Codex App Server, confirm `codex app-server --help` works.
- For ACP agents, confirm `gemini --acp` or `opencode acp --help` works.

## Invalid Agent Output

- Check the agent run stdout/stderr artifacts.
- Confirm the configured output mode matches the agent output: JSON, JSONL, or text.
- For JSON mode, ensure the top-level object includes a `findings` array or a supported schema.
- For JSONL mode, ensure finding events are newline-delimited.
- If repair fails, treat the raw output as untrusted evidence only.

## Review Times Out

- Confirm the runtime limit is high enough for the PR size.
- Check each agent config timeout.
- Rerun with fewer agents or a narrower focus prompt.
- For large diffs, exclude generated/lock/vendor files and reduce context budget.
- A timed-out agent should not discard findings from successful agents.

## GitHub Preview Has Anchor Warnings

- Confirm the finding path exists in the PR diff.
- Confirm the line is on the expected side of the diff.
- Use summary-only fallback for unanchored findings.
- Rebuild the preview after editing draft comments or finding decisions.
- Check previous comments context to avoid duplicate suggestions.

## Evidence Map Is Partial

- Confirm the finding has evidence items.
- Confirm the repository path still exists locally.
- Check graph status and unavailable reason in the Evidence Map panel.
- Use Follow-up to ask for missing call paths or tests when the map is incomplete.

## Large PR Feels Slow

- Confirm changed-file previews are bounded.
- Exclude generated, lock, binary, vendor, and build-output files.
- Prefer branch compare or GitHub PR snapshots over broad local changes when possible.
- Watch memory and CPU while switching Findings, Evidence Map, and Publish.
- Record stalls in `docs/dogfood_checklist.md` and the risk register.
