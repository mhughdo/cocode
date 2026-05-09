# cocode MVP Task Breakdown — Centralized Chat Update

**Document status:** Updated for centralized chat mockups  
**Date:** 2026-05-06  
**Baseline:** Previous MVP task breakdown is considered done. This document adds the new/changed work needed to support the centralized chat redesign and latest mockups.

---

## Table of Contents

1. [Scope](#scope)
2. [Status Legend](#status-legend)
3. [Global Definition of Done](#global-definition-of-done)
4. [Parallelization Summary](#parallelization-summary)
5. [Baseline Completed Tasks](#baseline-completed-tasks)
6. [Centralized Chat Tasks](#centralized-chat-tasks)
7. [Mockup Alignment Tasks](#mockup-alignment-tasks)
8. [Adapter Flexibility Tasks](#adapter-flexibility-tasks)
9. [Evidence Map Tasks](#evidence-map-tasks)
10. [Testing and Hardening Tasks](#testing-and-hardening-tasks)
11. [MVP Completion Criteria](#mvp-completion-criteria)

---

## Scope

The latest UI direction introduces a centralized chat as the main review surface. The prior MVP work is considered complete, so this document focuses on delta tasks required to implement the new centralized chat model, update screens to match mockups, support chat-scoped follow-ups, and keep adapter architecture flexible for future Codex App Server/ACP integrations.

---

## Status Legend

| Status      | Meaning                                                  |
| ----------- | -------------------------------------------------------- |
| Not started | Task has not begun.                                      |
| In progress | Work has started but is not done.                        |
| Blocked     | Cannot proceed until dependency or decision is resolved. |
| Done        | Meets done criteria.                                     |
| Deferred    | Intentionally out of MVP.                                |

---

## Global Definition of Done

A task is considered **Done** only when:

1. The user-visible behavior works in the Electron app or the backend behavior is externally observable through API/tests.
2. The implementation is persisted where needed in SQLite/artifacts.
3. Errors are handled and surfaced.
4. Unit tests cover core logic.
5. Integration tests cover at least one success and one failure path when relevant.
6. SSE/UI updates work when the task affects long-running operations.
7. Security constraints are respected.
8. Documentation or comments are updated for non-obvious behavior.
9. The task has no known critical bugs.
10. The task has been manually smoke-tested in a local project.

---

## Parallelization Summary

| Workstream                | Can start after               | Parallel with                              |
| ------------------------- | ----------------------------- | ------------------------------------------ |
| Chat schema               | Immediately                   | Frontend mockup components, adapter design |
| Chat APIs                 | Chat schema                   | SSE, frontend chat shell                   |
| Chat router               | Chat APIs + adapter interface | Context bundle builder                     |
| CLI turn runner           | Adapter interface             | Chat UI                                    |
| Evidence Map updates      | Existing findings schema      | Chat scoped context                        |
| Frontend screen alignment | Immediately                   | Backend schema work                        |
| Adapter flexibility       | Immediately                   | Chat schema                                |
| Tests                     | After each feature slice      | Continuous                                 |

---

## Baseline Completed Tasks

| Task No. | Task Name                          | Status | Notes                                       |
| -------- | ---------------------------------- | ------ | ------------------------------------------- |
| B-001    | Original Electron app shell        | Done   | From prior task breakdown.                  |
| B-002    | Original Go + Gin backend skeleton | Done   | From prior task breakdown.                  |
| B-003    | Original SQLite persistence        | Done   | From prior task breakdown.                  |
| B-004    | Original review setup flow         | Done   | To be updated for latest setup mockup.      |
| B-005    | Original CLI adapter MVP           | Done   | To be extended for chat turns.              |
| B-006    | Original findings flow             | Done   | To be aligned with latest table/side panel. |
| B-007    | Original copy packet feature       | Done   | To be exposed in chat and publish.          |
| B-008    | Original publish flow              | Done   | To be aligned with latest publish mockup.   |

---

## Centralized Chat Tasks

| Task No. | Task Name                                | Description                                                                                                                    | Status      | Dependencies        | Can Parallelize With | Done Criteria                                                                |
| -------- | ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ | ----------- | ------------------- | -------------------- | ---------------------------------------------------------------------------- |
| C-001    | Define chat domain model                 | Finalize Thread, ThreadMessage, ChatTurn, MessageBlock, MessageContextRef, ThreadSummary models.                               | Not started | None                | C-002, A-001, M-001  | Types documented; DB schema updated; sample JSON fixtures created.           |
| C-002    | Add chat SQLite migrations               | Add tables for `threads`, `thread_messages`, `message_context_refs`, `chat_turns`, `chat_turn_agent_runs`, `thread_summaries`. | Not started | C-001               | frontend shell       | Migrations apply cleanly; rollback/dev reset works; indexes added.           |
| C-003    | Add sqlc queries for chat                | Add create/list/update queries for threads/messages/turns/summaries/context refs.                                              | Not started | C-002               | C-004                | Queries compile; unit tests cover create/list/update flows.                  |
| C-004    | Implement ThreadService                  | Implement CRUD for threads and thread status updates.                                                                          | Not started | C-003               | C-005                | API tests can create/list/update threads.                                    |
| C-005    | Implement MessageService                 | Implement message creation, block updates, streaming delta append, context ref linking.                                        | Not started | C-003               | C-006                | Messages persist; blocks persist; context refs queryable.                    |
| C-006    | Implement ChatTurnService                | Create chat turns, transition statuses, connect user message to execution.                                                     | Not started | C-004, C-005        | C-007                | Turn lifecycle tested from created to completed/failed/canceled.             |
| C-007    | Implement ChatRouter                     | Route user turns to local answer, single agent, all agents, review workflow, evidence map, copy, publish preview.              | Not started | C-006               | C-008, C-009         | Routing table tests pass for common user prompts and explicit UI selections. |
| C-008    | Implement ChatContextBundle builder      | Build scoped context from thread summary, recent messages, review state, findings, evidence maps, artifacts.                   | Not started | C-003               | C-007, A-002         | Bundle artifacts stored; token estimate computed; context refs included.     |
| C-009    | Implement ThreadSummary builder          | Compact long chat history into durable summary/facts JSON.                                                                     | Not started | C-005               | C-008                | Summary generated after threshold; future bundles include summary.           |
| C-010    | Implement local answerer                 | Answer state questions without invoking CLIs, such as review status/top findings/copy availability.                            | Not started | C-007               | C-011                | Local questions produce cocode message and no agent run.                     |
| C-011    | Implement single-agent chat turn runner  | Run one CLI agent from a chat message using context bundle.                                                                    | Not started | C-007, C-008, A-003 | C-012                | Fake CLI success/failure tests pass; output appears in chat.                 |
| C-012    | Implement ask-all-agents runner          | Fan out one chat question to selected CLIs and collect outputs.                                                                | Not started | C-011               | C-013                | Partial failure works; all agent run cards visible.                          |
| C-013    | Implement synthesis step                 | Synthesize multiple agent outputs into a single cocode answer.                                                                 | Not started | C-012               | C-014                | Synthesis message summarizes agreement/disagreement and links evidence.      |
| C-014    | Implement finding-scoped chat            | Allow follow-up questions from finding detail with automatic `finding_id` context.                                             | Not started | C-008, C-011        | E-004                | Finding messages show in central thread and finding detail.                  |
| C-015    | Implement evidence-map-scoped chat       | Allow follow-up questions from evidence map with `evidence_map_id` context.                                                    | Not started | C-008, E-004        | C-014                | Evidence-map follow-up includes graph context and persists refs.             |
| C-016    | Implement chat-triggered copy packet     | User can ask “copy accepted findings” or use chat UI to create copy packet.                                                    | Not started | C-007               | M-005                | Copy packet card appears in chat; clipboard action works.                    |
| C-017    | Implement chat-triggered publish preview | User can ask for publish preview from chat.                                                                                    | Not started | C-007               | M-006                | Publish draft card appears; publish tab can open draft.                      |
| C-018    | Implement chat cancellation              | Cancel active chat turn and related CLI processes.                                                                             | Not started | C-011               | T-006                | Process killed; status canceled; UI updates.                                 |
| C-019    | Implement chat resume/reload             | Reload thread with messages, statuses, context refs, active/past turns.                                                        | Not started | C-005               | frontend             | App restart shows prior conversation.                                        |
| C-020    | Implement chat search indexing           | Index message text, finding titles, and artifact summaries for top search box.                                                 | Not started | C-005               | M-007                | Search returns threads/messages/findings.                                    |

---

## Mockup Alignment Tasks

| Task No. | Task Name                                 | Description                                                                                                              | Status      | Dependencies | Can Parallelize With | Done Criteria                                                              |
| -------- | ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ | ----------- | ------------ | -------------------- | -------------------------------------------------------------------------- |
| M-001    | Update app shell layout                   | Match latest sidebar/topbar structure: cocode logo, projects, threads, settings/user, top project/branch/search actions. | Not started | None         | C-001                | Layout matches mockup at high level; responsive enough for desktop widths. |
| M-002    | Build review setup screen                 | Source, focus, orchestration, presets, setup summary, next steps.                                                        | Not started | B-004        | C-001                | User can create thread and review source from setup.                       |
| M-003    | Build central chat page                   | Chat tab with message stream, right summary/activity cards, input bar with audience/responder controls.                  | Not started | C-005        | C-011                | Messages render; user can submit; events update stream.                    |
| M-004    | Align findings page                       | Findings cards, filters, table, side preview, actions.                                                                   | Not started | B-006        | E-001                | Selecting row updates preview; actions work.                               |
| M-005    | Align finding detail page                 | Changed code, explanation, evidence, counter-evidence, related tests, consensus, finding thread, actions.                | Not started | C-014        | E-001                | Finding detail shows context and can send scoped follow-up.                |
| M-006    | Align evidence map page                   | Code hierarchy, graph, why it matters panel, open in editor.                                                             | Not started | E-004        | C-015                | Graph renders persisted evidence map.                                      |
| M-007    | Align publish page                        | Accepted findings selection, GitHub preview, copy packet panel, checklist.                                               | Not started | C-016, C-017 | GH tests             | Copy/publish actions work.                                                 |
| M-008    | Align adapters settings page              | CLI table, health status, config panel, disabled future drivers.                                                         | Not started | A-001        | A-003                | CLI config can be created/tested; future rows marked unavailable.          |
| M-009    | Add consistent empty/error/loading states | Add shadcn-style skeletons, error cards, retry buttons.                                                                  | Not started | Screens      | T-006                | Every main screen has loading/error/empty state.                           |

---

## Adapter Flexibility Tasks

| Task No. | Task Name                           | Description                                                                             | Status      | Dependencies | Can Parallelize With | Done Criteria                                              |
| -------- | ----------------------------------- | --------------------------------------------------------------------------------------- | ----------- | ------------ | -------------------- | ---------------------------------------------------------- |
| A-001    | Define ConnectionDriver interface   | Create driver interface for CLI now and Codex App Server/ACP later.                     | Not started | None         | C-001                | Interface supports health, start, run turn, cancel, close. |
| A-002    | Define DriverEvent model            | Standardize output events across CLI, future app-server, ACP.                           | Not started | A-001        | C-008                | Events map to SSE and agent run artifacts.                 |
| A-003    | Refactor CLI adapter to driver      | Implement CLI non-interactive driver through ConnectionDriver.                          | Not started | A-001, A-002 | C-011                | Existing CLI review and chat turn both use same driver.    |
| A-004    | Add Codex CLI profile               | Default command config for Codex CLI non-interactive exec.                              | Not started | A-003        | A-005                | Health check and fake/smoke run pass.                      |
| A-005    | Add Claude Code profile             | Default command config for Claude Code headless.                                        | Not started | A-003        | A-004                | Health check and fake/smoke run pass.                      |
| A-006    | Add Gemini CLI profile              | Default command config for Gemini CLI headless.                                         | Not started | A-003        | A-004                | Health check and fake/smoke run pass.                      |
| A-007    | Add OpenCode profile                | Default command config for OpenCode non-interactive run.                                | Not started | A-003        | A-004                | Health check and fake/smoke run pass.                      |
| A-008    | Add disabled future driver configs  | Add metadata rows for Codex App Server and ACP as disabled/coming later.                | Not started | A-001        | M-008                | UI communicates not implemented; no broken actions.        |
| A-009    | Add external session metadata store | Store optional CLI session IDs/future app-server session IDs without depending on them. | Not started | A-001        | C-011                | Session metadata can be stored and ignored safely.         |

---

## Evidence Map Tasks

| Task No. | Task Name                                | Description                                                              | Status      | Dependencies      | Can Parallelize With | Done Criteria                                         |
| -------- | ---------------------------------------- | ------------------------------------------------------------------------ | ----------- | ----------------- | -------------------- | ----------------------------------------------------- |
| E-001    | Finalize evidence map schema             | Define node/edge types, DB tables, graph artifact shape.                 | Not started | Existing findings | C-001                | Schema documented; migrations compile.                |
| E-002    | Implement evidence seed collector        | Collect finding locations, evidence items, changed files, related tests. | Not started | E-001             | C-008                | Unit tests with fixture finding pass.                 |
| E-003    | Implement graph builder                  | Build nodes/edges from seeds and code references.                        | Not started | E-002             | M-006                | Graph validator passes; missing_guard edge supported. |
| E-004    | Implement evidence map API               | Build/get graph APIs.                                                    | Not started | E-003             | C-015                | UI can fetch map by finding.                          |
| E-005    | Implement React evidence graph           | Render graph and code hierarchy using persisted nodes/edges.             | Not started | E-004             | M-006                | Graph screen matches mockup behavior.                 |
| E-006    | Implement open-in-editor bridge          | Electron main opens local file at line where supported.                  | Not started | E-005             | M-006                | Button opens file or shows unsupported message.       |
| E-007    | Implement evidence map generation prompt | Add prompt and parser for agent-assisted graph generation fallback.      | Not started | E-003             | prompt work          | Prompt output validates against schema.               |

---

## Testing and Hardening Tasks

| Task No. | Task Name                          | Description                                                                                    | Status      | Dependencies | Can Parallelize With | Done Criteria                                |
| -------- | ---------------------------------- | ---------------------------------------------------------------------------------------------- | ----------- | ------------ | -------------------- | -------------------------------------------- |
| T-001    | Chat service unit tests            | Test messages, turns, refs, summaries.                                                         | Not started | C-003-C006   | ongoing              | Coverage for success/failure transitions.    |
| T-002    | Chat router unit tests             | Test routing decisions.                                                                        | Not started | C-007        | ongoing              | At least 20 route scenarios pass.            |
| T-003    | CLI fake driver tests              | Fake success, timeout, bad JSON, stderr, cancellation.                                         | Not started | A-003        | ongoing              | All paths persist artifacts and statuses.    |
| T-004    | Ask-all-agents integration test    | Run 3 fake agents, one failing.                                                                | Not started | C-012, C-013 | ongoing              | Synthesis includes partial failure notice.   |
| T-005    | Finding follow-up integration test | Ask question scoped to finding.                                                                | Not started | C-014        | ongoing              | Prompt includes finding/evidence context.    |
| T-006    | Evidence map integration test      | Build and render map from fixture finding.                                                     | Not started | E-004, E-005 | ongoing              | Graph nodes/edges visible in UI.             |
| T-007    | Copy packet integration test       | Generate packet from chat and publish page.                                                    | Not started | C-016, M-007 | ongoing              | Clipboard content matches expected template. |
| T-008    | Publish preview integration test   | Generate GitHub preview from accepted findings.                                                | Not started | C-017, M-007 | ongoing              | Diff mapping failures handled.               |
| T-009    | SSE reconnect test                 | Disconnect/reconnect while chat turn runs.                                                     | Not started | C-011        | ongoing              | UI reloads latest messages and statuses.     |
| T-010    | Security tests                     | Env allowlist, path sandbox, prompt redaction.                                                 | Not started | A-003, C-008 | ongoing              | Secrets not included in prompt artifacts.    |
| T-011    | App restart test                   | Restart app after completed/failed turn.                                                       | Not started | C-019        | ongoing              | Thread is restored correctly.                |
| T-012    | Manual smoke script                | Document a full manual run through setup, chat, findings, evidence map, copy, publish preview. | Not started | all MVP      | final                | Smoke test checklist included in repo docs.  |

---

## MVP Completion Criteria

The centralized-chat MVP is complete when:

1. A user can create a review thread from the setup screen.
2. The thread opens into a central chat.
3. The user can start a review from chat/setup.
4. CLI agents run in non-interactive mode and produce visible chat progress/results.
5. Findings are created, shown in the Findings tab, and linked from chat.
6. The user can ask a follow-up from the central chat.
7. The user can ask a finding-scoped follow-up.
8. The user can ask all agents and receive synthesized output.
9. The user can open a finding detail page.
10. The user can open an evidence map for a finding.
11. The user can copy a fix packet.
12. The user can preview GitHub publishing.
13. Failed agents produce visible, recoverable errors.
14. The app can be restarted and the thread still shows messages/findings/evidence.
15. Tests cover the critical success/failure flows above.
