# cocode MVP PRD — Centralized Multi-Agent Code Review Chat

**Document status:** Updated for centralized chat mockups  
**App name:** cocode  
**Date:** 2026-05-06  
**Owner:** Product / Engineering  
**Primary artifact type:** Markdown PRD  
**Related docs:** `cocode_mvp_tdd_centralized_chat.md`, `cocode_mvp_task_breakdown_centralized_chat.md`, `cocode_review_presets_roles_centralized_chat.md`

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Key Product Decision: Centralized Chat in Phase 1](#key-product-decision-centralized-chat-in-phase-1)
3. [Problem Statement](#problem-statement)
4. [Solution](#solution)
5. [MVP Experience](#mvp-experience)
6. [Functional Requirements](#functional-requirements)
7. [User Stories](#user-stories)
8. [Implementation Decisions](#implementation-decisions)
9. [Testing Decisions](#testing-decisions)
10. [Out of Scope](#out-of-scope)
11. [Further Notes](#further-notes)
12. [Sources and Design Inputs](#sources-and-design-inputs)

---

## Executive Summary

cocode is a local-first desktop app for multi-agent code review. It coordinates multiple CLI coding agents, verifies their findings against the codebase, visualizes the evidence behind each finding, and lets the user publish selected comments to GitHub or copy accepted findings into a separate coding agent.

The latest product direction changes the core UX from “findings-first with local Q&A panels” to a **centralized review chat**. The chat is the main coordination surface for starting reviews, monitoring progress, asking follow-up questions, querying all agents, querying specific agents, and navigating to Findings, Evidence Map, and Publish.

The MVP still keeps **findings as first-class domain objects**. The chat is the user interaction layer; findings, evidence, decisions, artifacts, agent runs, and publications remain structured backend entities.

---

## Key Product Decision: Centralized Chat in Phase 1

### Can cocode support centralized chat while only supporting CLIs in non-interactive mode?

**Yes.** Phase 1 can support a centralized chat, but the chat should be **owned by cocode**, not by the external CLI tools.

In Phase 1, cocode will treat every user chat message as a persisted `ChatTurn`. The backend will decide whether the turn should be answered by:

- the cocode orchestrator using existing review state,
- one selected CLI agent,
- multiple CLI agents in parallel,
- the local verifier,
- the finding/evidence-map engine,
- or a synthesizer that summarizes multiple CLI outputs.

The CLI tools do not need to maintain the central conversation. They are invoked as **stateless or semi-stateful workers**. cocode sends each CLI a rendered context packet containing the relevant thread summary, review snapshot, selected messages, finding/evidence references, and the user’s question. cocode then captures the CLI output, stores the raw output, normalizes it, and appends a clean assistant message back to the central chat.

### Phase 1 central chat support level

| Capability                           | Phase 1 support | Notes                                                       |
| ------------------------------------ | --------------: | ----------------------------------------------------------- |
| Main chat per review thread          |             Yes | Primary surface in the mockups.                             |
| Ask all agents                       |             Yes | Fan-out to selected CLIs, then synthesize.                  |
| Ask one selected CLI                 |             Yes | Runs one non-interactive CLI invocation with context.       |
| Ask orchestrator                     |             Yes | Uses local state and optionally one CLI.                    |
| Follow-up on finding                 |             Yes | Same central chat, scoped by `finding_id`.                  |
| Follow-up on Evidence Map            |             Yes | Same central chat, scoped by `evidence_map_id`.             |
| Thread history                       |             Yes | Persisted in SQLite.                                        |
| Streaming progress                   |             Yes | SSE events from cocode; CLI token streaming when available. |
| Agent-to-agent peer chat             |              No | Agents collaborate through cocode, not directly.            |
| Long-lived Codex App Server sessions |        Deferred | Code structure must allow it later.                         |
| ACP sessions                         |        Deferred | Code structure must allow it later.                         |
| MCP/A2A                              |        Deferred | Future adapter layers.                                      |

### Central principle

> The chat UI can feel like a multi-agent chat app, but technically it is an orchestrated workflow over persisted state, context bundles, CLI runs, and synthesized messages.

---

## Problem Statement

Developers use multiple coding agents for code review, but the workflow is fragmented and repetitive. A reviewer must paste the same PR context into several CLIs or IDEs, inspect each agent’s output separately, manually verify whether each finding is true, ask follow-up questions in scattered sessions, decide which issues matter, and then copy accepted findings into GitHub or a main coding agent.

The latest mockups add a clearer expectation: the user wants one central place to talk to cocode about the review. The app should feel like a review-focused chat workspace where the user can say:

- “Run a deep review of this PR.”
- “Ask all agents whether this auth finding is real.”
- “Open the evidence map for the top finding.”
- “Show me only high-risk issues.”
- “Copy the accepted findings as a fix packet.”
- “Publish the accepted findings to GitHub.”

Without a centralized chat, the product risks becoming a set of disconnected screens. With a centralized chat, the product becomes a coherent cockpit: the chat drives the workflow, while Findings, Evidence Map, and Publish provide structured review surfaces.

---

## Solution

Build cocode as a local-first Electron app with a Go + Gin local backend. The backend owns review threads, chat turns, agent runs, findings, evidence maps, decisions, copy packets, and GitHub publications.

The centralized chat becomes the main interaction shell. Users can start a thread, configure a review, monitor agent progress, ask questions, and navigate into structured screens. The app still uses explicit workflows for reliable code review:

1. Ingest review source.
2. Build context.
3. Run CLI reviewer agents.
4. Normalize and deduplicate findings.
5. Verify findings.
6. Build evidence maps.
7. Present findings in structured UI.
8. Let user accept/dismiss/copy/publish.

The chat does not replace the review workflow. It coordinates it.

---

## MVP Experience

### 1. Set Up Review

The user chooses a review source:

- GitHub PR URL
- Local changes
- Branch comparison

The user selects focus areas, presets, orchestrator, and reviewer CLIs. The setup screen should match the latest mockups conceptually: source, focus, orchestration, presets, setup summary, and “what happens next.”

### 2. Chat

The chat tab is the default view for a review thread.

It shows:

- the user’s initial review request,
- an orchestrator plan,
- system progress updates,
- early findings,
- agent responses,
- and follow-up answers.

The input bar supports:

- ask target: all agents, orchestrator, selected agent, verifier,
- responder/runtime: Codex CLI, Claude Code CLI, Gemini CLI, OpenCode, local verifier,
- mode: review, follow-up, evidence, publish, copy,
- permission mode,
- attachments/references later.

### 3. Findings

The findings tab shows structured review results:

- total findings,
- verified,
- needs triage,
- accepted,
- dismissed,
- table/list of findings,
- side detail preview,
- open full detail,
- open evidence map,
- copy fix packet.

### 4. Finding Detail

The finding detail screen shows:

- changed code,
- detailed explanation,
- supporting evidence,
- counter-evidence,
- related tests,
- agent consensus,
- finding thread,
- decision actions,
- copy fix packet,
- open evidence map.

### 5. Evidence Map

The evidence map screen is a first-class MVP screen. It visualizes how the finding connects across the codebase:

- code hierarchy,
- evidence graph,
- nodes for router/setup/route/handler/test/middleware/query/migration/config,
- edges such as calls, registers, guards, missing_guard, covers,
- interpretation,
- suggested remediation,
- open in editor.

### 6. Publish

The publish screen lets the user:

- choose accepted findings,
- preview GitHub comments,
- copy fix packet,
- save draft,
- publish selected comments to GitHub.

### 7. Settings / Adapters

The adapters screen lets the user configure CLI agents and future-ready connection types. Phase 1 supports CLI non-interactive mode. The UI may show future adapter rows such as Codex App Server or ACP, but these should be disabled or marked “coming later” until implemented.

---

## Functional Requirements

### Chat Requirements

| ID       | Requirement                                                                                                              |
| -------- | ------------------------------------------------------------------------------------------------------------------------ |
| CHAT-001 | Every review thread must have a centralized chat.                                                                        |
| CHAT-002 | Every user message must be persisted before execution begins.                                                            |
| CHAT-003 | Every orchestrator/system/agent response must be persisted as a thread message.                                          |
| CHAT-004 | Chat messages can reference a review session, finding, evidence map, artifact, changed file, or GitHub publication.      |
| CHAT-005 | The user can ask all agents a question.                                                                                  |
| CHAT-006 | The user can ask one selected CLI agent a question.                                                                      |
| CHAT-007 | The user can ask the orchestrator a state-aware question without launching external CLIs when local state is sufficient. |
| CHAT-008 | The user can ask follow-up questions from the finding detail screen and see the response in the same central thread.     |
| CHAT-009 | The user can ask questions from the evidence map screen and preserve evidence-map context.                               |
| CHAT-010 | Chat progress must stream through SSE.                                                                                   |
| CHAT-011 | A failed CLI run must create a visible failed agent-run card/message, not silently disappear.                            |
| CHAT-012 | Chat turn outputs must preserve provenance: responder, adapter, command, status, artifacts, and context bundle ID.       |
| CHAT-013 | Central chat must support cancellation of an in-flight turn.                                                             |
| CHAT-014 | A thread summary must be maintained to avoid unbounded prompt growth.                                                    |
| CHAT-015 | The user can resume a previous thread and continue asking questions.                                                     |

### CLI Adapter Requirements

| ID      | Requirement                                                                                                                                           |
| ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| CLI-001 | Phase 1 supports non-interactive CLI invocations only.                                                                                                |
| CLI-002 | Supported adapter configs must include command path, args template, environment allowlist, output mode, timeout, working directory, and safety flags. |
| CLI-003 | The backend must capture stdout, stderr, exit code, duration, and artifacts.                                                                          |
| CLI-004 | The backend must parse JSON/JSONL when available and preserve raw text when not.                                                                      |
| CLI-005 | The backend must support cancellation through process context cancellation.                                                                           |
| CLI-006 | Adapter interface must be flexible enough for future Codex App Server, ACP, MCP, and A2A drivers.                                                     |

### Findings Requirements

| ID      | Requirement                                                                                                                  |
| ------- | ---------------------------------------------------------------------------------------------------------------------------- |
| FND-001 | Every finding candidate must be normalized into a schema.                                                                    |
| FND-002 | Duplicate findings from multiple agents must be merged.                                                                      |
| FND-003 | Canonical findings must preserve agent provenance.                                                                           |
| FND-004 | Findings must support severity, confidence, status, category, affected location, evidence, suggested fix, and draft comment. |
| FND-005 | Findings must support accept, dismiss, defer, copy, and publish decisions.                                                   |
| FND-006 | Dismissal reasons must be stored for future prompt/preset improvements.                                                      |

### Evidence Map Requirements

| ID      | Requirement                                                                                                                                                             |
| ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| EVM-001 | Verified or high-risk findings must be able to produce an Evidence Map.                                                                                                 |
| EVM-002 | Evidence Map nodes must represent files, symbols, routes, handlers, queries, tests, migrations, configs, middleware, and policies where applicable.                     |
| EVM-003 | Evidence Map edges must encode relationships such as calls, registers, imports, guards, missing_guard, validates, covers, writes, reads, queries, migrates, configures. |
| EVM-004 | Evidence Map must support graph rendering and a tree/code-hierarchy panel.                                                                                              |
| EVM-005 | Every graph node should link to an artifact or file/line range.                                                                                                         |
| EVM-006 | The Evidence Map must support “Open in editor” where local file path and line range are known.                                                                          |
| EVM-007 | The user can ask chat questions scoped to an Evidence Map.                                                                                                              |

### Copy Packet Requirements

| ID       | Requirement                                                                                             |
| -------- | ------------------------------------------------------------------------------------------------------- |
| COPY-001 | The user can copy one finding as a fix packet.                                                          |
| COPY-002 | The user can copy accepted findings as a single fix packet.                                             |
| COPY-003 | Copy packets must include only accepted/selected findings unless the user explicitly chooses otherwise. |
| COPY-004 | Copy packets must include evidence and acceptance criteria.                                             |
| COPY-005 | Copy packets must be persisted as artifacts and re-copyable.                                            |

### GitHub Publishing Requirements

| ID     | Requirement                                                                            |
| ------ | -------------------------------------------------------------------------------------- |
| GH-001 | The user can preview GitHub comments before publishing.                                |
| GH-002 | The user can publish selected accepted findings to GitHub.                             |
| GH-003 | Diff mapping failure must be visible and must not silently publish misplaced comments. |
| GH-004 | The app must record GitHub publication IDs and status.                                 |

---

## User Stories

1. As a developer, I want to start a new review thread from a GitHub PR URL, so that cocode can review an existing PR.
2. As a developer, I want to start a new review thread from local changes, so that I can review work before opening a PR.
3. As a developer, I want to compare branches, so that I can review feature work against main.
4. As a developer, I want to describe the review focus in natural language, so that agents understand what matters.
5. As a developer, I want to choose review presets, so that I can apply expert review knowledge quickly.
6. As a developer, I want to select an orchestrator CLI, so that one primary model coordinates the review plan.
7. As a developer, I want to select multiple reviewer CLIs, so that the PR is reviewed from different perspectives.
8. As a developer, I want to see a setup summary, so that I know the review is ready to start.
9. As a developer, I want the review to open into a central chat, so that I can monitor and guide the review in one place.
10. As a developer, I want the orchestrator to explain its plan, so that I understand what the agents will do.
11. As a developer, I want progress updates in the chat, so that long reviews feel transparent.
12. As a developer, I want early findings to appear in chat, so that I can react before the full review completes.
13. As a developer, I want to ask all agents a follow-up question, so that I can compare perspectives.
14. As a developer, I want to ask one selected agent a follow-up question, so that I can use a specific model’s strengths.
15. As a developer, I want to ask the local verifier whether a claim is true, so that I can avoid false positives.
16. As a developer, I want chat messages to link to findings, so that I can jump from conversation to structured evidence.
17. As a developer, I want chat messages to link to files and line ranges, so that I can inspect code quickly.
18. As a developer, I want chat messages to show failed agent runs, so that I know which agents did not contribute.
19. As a developer, I want to cancel a running chat turn, so that I can stop wasted work.
20. As a developer, I want to resume a previous chat thread, so that review context is not lost.
21. As a developer, I want the findings tab to show all findings, so that I can triage efficiently.
22. As a developer, I want findings grouped by verified, needs triage, accepted, and dismissed, so that I understand review status.
23. As a developer, I want each finding to show severity and confidence, so that I can prioritize.
24. As a developer, I want to filter findings by severity, status, agent, and file, so that I can focus.
25. As a developer, I want a side preview when selecting a finding, so that I can inspect without leaving the table.
26. As a developer, I want a full finding detail view, so that I can inspect code, evidence, and reasoning deeply.
27. As a developer, I want the finding detail view to show changed code, so that I know what the PR changed.
28. As a developer, I want the finding detail view to show supporting evidence, so that I can verify the issue.
29. As a developer, I want the finding detail view to show counter-evidence, so that I can avoid overreacting.
30. As a developer, I want related tests shown, so that I know whether the issue is covered.
31. As a developer, I want agent consensus shown, so that I can judge reliability.
32. As a developer, I want to accept a finding, so that it can be copied or published.
33. As a developer, I want to dismiss a finding, so that noise is removed from the workflow.
34. As a developer, I want to update finding status manually, so that I can override the verifier.
35. As a developer, I want to copy a fix packet from the finding detail, so that I can paste it into my main coding agent.
36. As a developer, I want to open an evidence map, so that I can understand a finding without manually scouting the codebase.
37. As a developer, I want the evidence map to show code hierarchy, so that I can orient myself quickly.
38. As a developer, I want the evidence map to show graph relationships, so that I can see route/call/guard/test paths.
39. As a developer, I want the evidence map to highlight a missing guard, so that auth issues are visually obvious.
40. As a developer, I want the evidence map to show why the issue matters, so that I understand impact.
41. As a developer, I want evidence highlights, so that the core proof is easy to scan.
42. As a developer, I want suggested remediation in the evidence map, so that I know the likely fix.
43. As a developer, I want to open a mapped code node in my editor, so that I can inspect or fix quickly.
44. As a developer, I want to ask a chat question scoped to an evidence map, so that the answer uses only relevant context.
45. As a developer, I want to select accepted findings for publishing, so that I control what appears on GitHub.
46. As a developer, I want to preview GitHub comments, so that I can edit before publishing.
47. As a developer, I want to publish accepted comments, so that the PR receives actionable feedback.
48. As a developer, I want to save a review draft, so that I can return later.
49. As a developer, I want to copy selected findings from Publish, so that I can paste them into a coding agent instead of publishing.
50. As a developer, I want adapter settings, so that I can configure Codex CLI, Claude Code CLI, Gemini CLI, and OpenCode.
51. As a developer, I want adapter health checks, so that I know which CLIs are usable.
52. As a developer, I want adapter roles, so that agents are specialized.
53. As a developer, I want read-only mode settings, so that review agents do not modify code.
54. As a developer, I want environment allowlists, so that secrets are controlled.
55. As a developer, I want CLI output artifacts preserved, so that I can debug agent behavior.
56. As a developer, I want cocode to warn me when a future adapter is not implemented, so that the UI is honest.
57. As a developer, I want chat summaries maintained automatically, so that long threads remain usable.
58. As a developer, I want the app to avoid re-sending unnecessary code, so that reviews are faster and cheaper.
59. As a developer, I want local verifier results, so that some checks are deterministic.
60. As a developer, I want every accepted finding to have done criteria, so that a downstream coding agent knows when to stop.
61. As a developer, I want every review run to have traceable events, so that I can debug failures.
62. As a developer, I want partial results when an agent fails, so that one failed CLI does not waste the whole review.
63. As a developer, I want the system to show what it skipped, so that I understand limitations.
64. As a developer, I want prompt/preset versions stored, so that review runs are reproducible.
65. As a developer, I want dismissals to improve future reviews, so that repeated false positives decrease.
66. As a team lead, I want presets for security, performance, data integrity, and Postgres, so that reviewers can apply consistent standards.
67. As a team lead, I want review reports, so that I can compare review quality over time.
68. As a security reviewer, I want auth findings to include route, middleware, handler, and test evidence, so that access-control claims are verifiable.
69. As a database reviewer, I want query findings to include query shape and indexes, so that performance suggestions are grounded.
70. As an app builder, I want the adapter interface to support future Codex App Server and ACP drivers, so that Phase 1 work is not thrown away.

---

## Implementation Decisions

### Centralized chat decision

cocode will implement a centralized chat as a persisted domain model:

- `threads`
- `thread_messages`
- `chat_turns`
- `chat_turn_agent_runs`
- `message_context_refs`
- `thread_summaries`

The central chat will not depend on long-running external CLI sessions in Phase 1. Every CLI call receives enough context to answer the current turn. Where a CLI supports resume/session IDs, the adapter may store external session metadata, but cocode remains the source of truth.

### Orchestration decision

Use a hybrid pattern:

- deterministic workflows for review setup, context building, review execution, verification, evidence-map building, and publishing;
- conversation-driven UX for user interactions;
- orchestrator-mediated collaboration for agents.

### Adapter decision

Phase 1 supports CLI non-interactive mode. The code structure must use a `ConnectionDriver` abstraction so future drivers can support:

- Codex App Server,
- ACP stdio,
- MCP tools,
- A2A remote agents,
- provider SDKs.

### Chat-routing decision

Every chat turn goes through an intent/audience router. The router decides:

- whether local state can answer,
- whether one agent should answer,
- whether all agents should answer,
- whether outputs need synthesis,
- whether the turn should trigger an action such as copy packet, publish preview, evidence map, or finding status update.

### Evidence Map decision

Evidence Map is now part of MVP. It is not a decorative graph. It is a structured evidence artifact generated from findings, context, static analysis, and agent outputs.

### Mockup alignment decision

Adopt these mockup concepts:

- central sidebar with projects and threads,
- top project/branch controls,
- Chat/Findings/Publish tabs,
- setup review screen,
- findings table with side preview,
- finding detail screen,
- evidence map screen,
- adapter settings screen.

Do not treat mockup text as exact product truth. Use it as interaction intent and layout direction.

---

## Testing Decisions

Tests should validate behavior users observe, not private implementation details. Good tests confirm that:

- a review thread can be created,
- chat messages persist,
- a chat turn can route to one CLI,
- a chat turn can fan out to multiple CLIs,
- outputs stream to the UI,
- failed CLIs produce visible error messages,
- findings are created and linked from chat,
- evidence maps are generated and rendered,
- copy packets contain accepted findings,
- GitHub previews are correct.

Test modules:

- Thread and chat persistence
- Chat turn router
- CLI adapter
- SSE event stream
- Context bundle builder
- Finding normalizer/deduper
- Evidence map builder
- Copy packet renderer
- GitHub preview/publisher
- UI route integration
- Error/cancellation handling

---

## Out of Scope

The MVP will not include:

- true peer-to-peer agent chat;
- long-lived Codex App Server production driver;
- ACP production driver;
- MCP server hosting;
- A2A remote agents;
- in-app code fixing;
- automatic file edits;
- automatic GitHub publishing;
- team/cloud sync;
- multi-user collaboration.

---

## Further Notes

The centralized chat is worth adding because it makes cocode feel like a coherent assistant rather than a set of tools. The technical risk is manageable as long as the chat is modeled as cocode-owned persisted state and not delegated to external CLIs.

The most important implementation guardrail is to keep the central chat structured. Every conversational response should be linkable to review artifacts, findings, evidence, or agent runs. Otherwise the product will become a generic chat app and lose the review workflow’s value.

---

## Sources and Design Inputs

- User-provided latest mockups, May 2026.
- Existing cocode PRD/TDD/task breakdown documents.
- Victor Dibia, _Designing Multi-Agent Systems_, especially chapters on orchestration patterns, modern agent UIs, software engineering agents, context engineering, completion criteria, and distributed protocols.
- OpenAI Codex App Server documentation and app-server README.
- ACP transport documentation.
- Claude Code headless documentation.
- Gemini CLI headless and ACP documentation.
- OpenCode CLI and ACP documentation.
- MCP transport documentation.
