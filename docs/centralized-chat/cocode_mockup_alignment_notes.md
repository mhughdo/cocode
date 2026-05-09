# cocode Mockup Alignment Notes — Centralized Chat Update

**Date:** 2026-05-06  
**Purpose:** Cross-check latest mockups against PRD/TDD/task breakdown.

---

## Adopted Mockup Changes

| Mockup feature                                       | Decision                        | Document update                                                      |
| ---------------------------------------------------- | ------------------------------- | -------------------------------------------------------------------- |
| Centralized chat as main thread surface              | Adopt                           | PRD/TDD now define central chat, chat turns, messages, routing, SSE. |
| Chat includes orchestrator and system updates        | Adopt                           | TDD has author types and message block types.                        |
| Chat input can ask all agents or select responder    | Adopt                           | PRD/TDD have audience/responder selection and router.                |
| Findings tab simplified to Chat / Findings / Publish | Adopt                           | PRD/TDD route structure updated.                                     |
| Finding detail includes finding thread               | Adopt with technical adjustment | Finding thread is filtered central-thread messages via context refs. |
| Evidence Map screen                                  | Adopt                           | PRD/TDD/task breakdown include Evidence Map MVP.                     |
| Publish screen includes copy fix packet              | Adopt                           | Copy and publish are peer actions.                                   |
| Settings / Adapters screen                           | Adopt                           | TDD has CLI-only Phase 1 and disabled future drivers.                |
| Adapter health checks                                | Adopt                           | Task A-004 through A-007.                                            |
| Future Codex App Server row                          | Adopt as disabled               | TDD future driver hook; not Phase 1 implementation.                  |

---

## Adjusted or Skipped Mockup Details

| Mockup detail                   | Decision                                 | Reason                                               |
| ------------------------------- | ---------------------------------------- | ---------------------------------------------------- |
| “OpenCode API”                  | Adjust to OpenCode CLI in Phase 1        | Phase 1 supports CLI non-interactive only.           |
| “Read & Write” default tools    | Make review read-only                    | In-app fixing is out of MVP.                         |
| Multiple human users/accounts   | Local profile only                       | Team/cloud mode out of MVP.                          |
| True live persistent agent chat | Simulate through cocode-owned chat turns | Phase 1 CLIs are non-interactive workers.            |
| Disabled App Server/ACP rows    | Allowed if clearly marked unavailable    | Keeps architecture visible without implying support. |

---

## Centralized Chat Implementation Interpretation

The mockups imply a chat app, but cocode should not become generic chat. The chat should always be tied to review state:

```text
Chat message
  -> may reference review source, review session, finding, evidence map, artifact, file, copy packet, publish draft
```

This ensures every useful answer can navigate to structured UI.

---

## Evidence Map Interpretation

The Evidence Map is the screen that directly addresses the original pain point: “I do not want to scout the codebase manually to see if a finding is true.”

The graph should prioritize:

- small number of relevant files,
- exact line ranges,
- clear relationship labels,
- missing guard/coverage relationships,
- interpretation and remediation.

It should not try to render the entire repository.

---

## MVP UI Route Map

```text
Set up review
  -> Chat
     -> Findings
        -> Finding detail
           -> Evidence map
     -> Publish
Settings
  -> Adapters
```

---

## Product Guardrail

Centralized chat should be powerful, but Findings and Evidence Map remain the source of review truth. If chat output and structured finding state disagree, the UI should show the structured verification status and provenance.
