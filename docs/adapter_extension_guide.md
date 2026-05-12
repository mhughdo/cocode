# cocode Adapter Extension Guide

This guide describes how to add a new CLI or protocol adapter without coupling review workflows to one vendor runtime. The initial runtime supports non-interactive CLI agents plus stdio JSON-RPC protocol connectors for Codex App Server and ACP-compatible agents.

## Core Interfaces

Adapter work starts in `services/cocoded/internal/agents`.

The stable boundary is:

```go
type AgentAdapter interface {
	ID() string
	Kind() AdapterKind
	HealthCheck(ctx context.Context) (AgentHealth, error)
	Capabilities(ctx context.Context) (AgentCapabilities, error)
	RunTask(ctx context.Context, task AgentTask) (<-chan AgentEvent, error)
	Cancel(ctx context.Context, runID string) error
}

type ConnectionDriver interface {
	Open(ctx context.Context, config ConnectionConfig) (Connection, error)
}

type Connection interface {
	SendTask(ctx context.Context, task AgentTask) (<-chan AgentEvent, error)
	Close(ctx context.Context) error
}
```

Keep workflow code adapter-agnostic. Every adapter should emit the same `AgentEvent` lifecycle: queued when scheduled, started when execution begins, output/progress for live data, and completed/failed/canceled as terminal events.

## Adding A CLI Preset

Use a CLI preset when the agent can run one task non-interactively and exit.

1. Add a constructor in `services/cocoded/internal/agentpreset/presets.go`.
2. Add it to `agentpreset.List()`.
3. Set `AdapterKind` to `cli_noninteractive`.
4. Use arg arrays, not shell strings.
5. Choose `PromptDelivery`:
   - `stdin` for CLIs that read prompt text from stdin.
   - `arg` for CLIs with a prompt/message argument; include `{{prompt}}`.
   - `temp_file` for CLIs that prefer reading a file; include `{{prompt_file}}`.
6. Set `OutputMode` to the parser contract the CLI actually emits: `text`, `json`, `jsonl`, or `ndjson`.
7. Keep `EnvAllowlist` explicit and small.
8. Add tests in `services/cocoded/internal/agentpreset/presets_test.go`.
9. Update `services/cocoded/internal/httpapi/router_test.go` so `/api/agents/presets` proves the preset is visible to the UI.

Current examples:

| Preset | Prompt | Output | Notes |
|---|---|---|---|
| Codex CLI | stdin | JSONL | `codex exec --json --sandbox read-only --skip-git-repo-check --ephemeral --ignore-rules --color never -` |
| Claude Code CLI | arg | JSON | `claude -p {{prompt}} --output-format json --permission-mode plan --no-session-persistence` |
| Gemini CLI | arg | JSON | `gemini -p {{prompt}} --output-format json --approval-mode default --skip-trust` |
| OpenCode CLI | arg | JSONL | `opencode run --pure --format json --thinking --model opencode-go/kimi-k2.6 --variant high {{prompt}}` |
| Codex App Server | JSON-RPC stdio | JSON | `codex app-server --listen stdio://` |
| Gemini ACP | ACP stdio | JSON | `gemini --acp` |
| OpenCode ACP | ACP stdio | JSON | `opencode acp` |
| Custom CLI | stdin | text | disabled template for user-defined commands |

## Implementing CLI Execution

`CommandOnceDriver` is the MVP runtime for `cli_noninteractive` adapters.

Required behavior:

- Use `exec.CommandContext`.
- Pass args as arrays.
- Do not invoke a shell by default.
- Set cwd to the selected repository when the preset uses `repo_root`.
- Pass only explicitly allowlisted environment variables.
- Capture stdout and stderr separately.
- Enforce stdout/stderr/prompt limits.
- Preserve raw output as artifacts before parsing or normalization.
- Treat non-zero exits, timeouts, and cancellation as terminal events with useful errors.

Do not pass secrets in command-line args. Prefer local provider login/config files or explicit environment allowlists.

## Output Parsing

Parser behavior lives in `services/cocoded/internal/agentoutput/parser.go`.

- `OutputJSON` expects one valid JSON document.
- `OutputJSONL` and `OutputNDJSON` parse one JSON document per non-empty line.
- Mixed JSONL preserves valid JSON documents and reports invalid-line diagnostics.
- Text fallback preserves raw text so normalization/repair can run later.

When adding a preset, set `Capabilities.OutputModes` so it includes the selected `OutputMode`; config save rejects mismatches.

## Health Checks

CLI health checks are implemented through `/api/agents/configs/:id/test`.

For `cli_noninteractive` configs, the backend checks:

- command lookup,
- version args when configured,
- optional smoke prompt when enabled.

For non-CLI adapter kinds, `/api/agents/configs/:id/test` still reports protocol health conservatively unless a lightweight version command is configured. Runtime execution is handled by the stdio protocol driver once the agent config is enabled.

Recommended preset settings:

```json
{
  "prompt_delivery": "stdin",
  "timeout_seconds": 1800,
  "version_args": ["--version"],
  "smoke_prompt_enabled": false
}
```

Use `"skip_version": true` only for custom templates or commands without stable version flags.

## Adding A Protocol Adapter

Use a protocol adapter when the agent has a long-lived session, JSON-RPC/stdout notifications, or resumable thread semantics.

Current stdio protocol connectors:

- `CodexAppServerAdapter` uses `AdapterJSONRPCStdio`.
- `ACPAdapter` uses `AdapterACPStdio`.
- `JSONRPCStdioDriver` launches the configured command, reads newline-delimited JSON-RPC frames, maps streaming notifications to `AgentEvent`, and rejects unsupported server-initiated requests.

When extending a protocol adapter:

1. Keep byte transport in `JSONRPCStdioConnection`.
2. Keep protocol-specific event/method mapping in the adapter layer.
3. Add an event mapper that converts every protocol notification to `AgentEvent`.
4. Persist foreign session/thread IDs in run metadata, not in workflow-only memory.
5. Preserve raw protocol outputs as artifacts when they influence findings.
6. Map approval/tool requests into cocode permission UI before allowing side effects. The current review flow rejects client-side file, terminal, and permission callbacks so agents stay read-only from cocode's perspective.
7. Make unknown protocol notifications progress events, not failures.
8. Add tests for disabled/config validation, happy path event mapping, malformed payloads, cancellation, and close behavior.

Protocol adapters must still produce the same finding candidates and evidence artifacts as CLI adapters.

## Security Rules

Adapter code must preserve cocode's local-review security posture:

- Bind local services to localhost only.
- Require the per-launch backend auth token for UI/API requests.
- Scope file reads to the selected workspace and app artifact directories.
- Do not let review-mode agents write repo files through cocode-managed tools.
- Redact secrets before sending context to cloud-backed CLIs.
- Keep environment inheritance deny-by-default.
- Never treat agent output as trusted instructions for publish/write actions.
- Store secrets outside SQLite plaintext.
- Record which context was sent to which agent once provider visibility metadata is implemented.

## Test Checklist

For every new adapter or preset:

- Unit test identity, kind, capabilities, and settings.
- Test prompt delivery mode and placeholder args.
- Test parser mode with at least one representative output fixture.
- Test health behavior for installed, missing, disabled, and bad-version cases when practical.
- Test timeout/cancellation with a fake slow command for CLI runtimes.
- Test output limits and stderr preservation.
- Test env allowlist behavior; empty env names must be rejected and duplicates deduped.
- Test unsupported protocol/runtime paths and server-initiated requests return clear errors.
- Run `go test ./...` from `services/cocoded`.
- Run `git diff --check` before committing.

## Documentation Checklist

When adding a new first-party preset, update:

- `services/cocoded/internal/agentpreset/presets.go`
- `services/cocoded/internal/agentpreset/presets_test.go`
- `services/cocoded/internal/httpapi/router_test.go`
- `docs/cocode_mvp_task_breakdown.md`
- User-facing setup/troubleshooting docs when T383/T384 are implemented.
