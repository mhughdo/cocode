# cocode

cocode is a local-first desktop cockpit for evidence-backed multi-agent code review.

The MVP is scoped to local review agents, a Go/Gin local backend, SQLite persistence, an Electron + React + TypeScript desktop shell, and shadcn/ui for the renderer. Agent execution supports non-interactive CLIs plus first stdio protocol connectors for Codex App Server and ACP-compatible agents.

## Workspace layout

```text
cocode/
  apps/
    desktop/              Electron main/preload and React renderer
  services/
    cocoded/              Local Go backend
  packages/
    schemas/              Shared JSON schemas
    prompts/              Versioned prompt templates
  testdata/
    repos/                Golden fixture repositories
    fake-agents/          Fake CLI agents for tests
  docs/                   PRD, TDD, task breakdown, UI mockups
```

## Common commands

```sh
pnpm install
pnpm dev
pnpm format:check
pnpm lint
pnpm test
pnpm typecheck
pnpm backend:test
pnpm check
```

The desktop shell launches the local backend and runs resumable, evidence-backed review sessions against local repository snapshots.

## Optional git hooks

Enable local pre-commit checks with:

```sh
pnpm hooks:install
```

Disable them with:

```sh
pnpm hooks:uninstall
```
