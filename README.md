# cocode

cocode is a local-first desktop cockpit for evidence-backed multi-agent code review.

The MVP is scoped to non-interactive CLI agents, a Go/Gin local backend, SQLite persistence, an Electron + React + TypeScript desktop shell, and shadcn/ui for the renderer.

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

The desktop shell is currently a scaffold for T010-T016. The backend exposes a minimal health endpoint and is not launched by Electron yet.

## Optional git hooks

Enable local pre-commit checks with:

```sh
pnpm hooks:install
```

Disable them with:

```sh
pnpm hooks:uninstall
```
