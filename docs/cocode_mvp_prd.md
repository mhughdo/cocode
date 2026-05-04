# cocode MVP Product Requirements Document (PRD)

**Product name:** cocode  
**Product type:** Local-first desktop multi-agent code review cockpit  
**MVP focus:** PR/code review with evidence-backed findings, Evidence Map, follow-up Q&A, copy fix packets, and GitHub publishing  
**Primary agent integration for Phase 1:** Non-interactive CLI agents plus first Codex App Server and ACP stdio connectors
**Future agent integration readiness:** MCP, A2A, provider SDKs, richer protocol permissions/tooling
**Backend:** Go + Gin local service
**Desktop shell:** Electron + React + TypeScript + shadcn/ui
**Status:** Updated after latest UI mockups and scope decisions
**Last updated:** 2026-05-04

---

## 0. Executive Summary

cocode helps developers run a deep multi-agent code review without manually copying prompts into multiple CLIs, cross-checking every finding by hand, and rewriting useful findings for GitHub or a separate coding agent.

The MVP is intentionally narrow: it is a code-review cockpit, not a code-fixing IDE. It coordinates local CLI and stdio protocol agents, normalizes their outputs into evidence-backed findings, helps the user verify those findings through a Finding Detail screen and Evidence Map, then lets the user accept, dismiss, copy, or publish selected findings.

The most important UX object is the **finding**, not the raw agent transcript. The user should feel that cocode turns noisy agent outputs into a clean, trustworthy review queue.

---

## 1. Problem Statement

Developers already use several AI coding agents during code review, but the workflow is fragmented and repetitive.

From the user's perspective, the current workflow is:

1. Paste the same PR context and prompt into multiple CLIs or IDE agents.
2. Wait for each agent to produce independent review feedback.
3. Read each output manually.
4. Remove duplicates.
5. Check the codebase to determine whether each finding is true.
6. Ask follow-up questions in separate terminals or chats.
7. Decide which findings should be fixed or ignored.
8. Copy accepted findings into a main coding agent for fixing.
9. Manually convert accepted findings into GitHub PR comments.

This creates the following pain points:

| Pain point | User impact |
|---|---|
| Repetitive prompting | The same instructions and PR context must be pasted into multiple agents. |
| Scattered sessions | Agent outputs live in terminals, IDE panes, and chats instead of one review thread. |
| No unified review | Each agent reports findings in different formats with duplicates and conflicts. |
| Manual verification | The human still has to scout the repo to determine if findings are true. |
| Weak evidence | Many findings lack exact file/line evidence, call paths, tests, or counter-evidence. |
| Hard follow-up | Asking questions about a finding requires re-explaining context to another agent. |
| Awkward fix handoff | Useful findings must be rewritten before being pasted into the main coding agent. |
| GitHub friction | Publishing requires manual comment formatting and diff-line mapping. |
| Trust gap | Users cannot easily see what agents saw, where evidence came from, or why a claim was verified. |

The attached *Designing Multi-Agent Systems* book frames this as a systems-engineering problem, not merely an LLM prompting problem. Reliable multi-agent applications need task decomposition, explicit orchestration where predictability matters, context engineering, structured outputs, observability, interruptibility, evaluation, and human oversight.

---

## 2. Solution

cocode is a local-first desktop app that coordinates multiple CLI-based review agents and presents a unified, evidence-backed review experience.

From the user's perspective:

```text
Open PR or local diff
-> configure review scope, agents, and policies
-> run review
-> watch progress in a review thread
-> triage unified findings
-> open Finding Detail or Evidence Map to verify claims
-> ask follow-up questions scoped to a finding
-> accept, dismiss, defer, copy, or publish
```

The MVP does **not** modify code. It creates a smooth bridge from review to action through copyable fix packets and optional GitHub comments.

### 2.1 Primary user-facing capabilities

1. Create a review thread from a GitHub PR URL, local branch comparison, or local changes.
2. Configure which files, agents, presets, models, reasoning levels, and policies apply.
3. Run multiple non-interactive CLI agents concurrently.
4. Normalize raw CLI output into structured finding candidates.
5. Deduplicate and synthesize candidates into canonical findings.
6. Verify findings using local code search, deterministic checks, and verifier-agent passes.
7. Show each finding with evidence, counter-evidence, code snippets, affected lines, agent consensus, draft GitHub comment, and suggested fix.
8. Show an Evidence Map for each finding with code hierarchy, evidence graph, call path, and missing/contradicting relationships.
9. Let the user ask finding-scoped follow-up questions.
10. Let the user copy one finding, selected findings, or all accepted findings as a fix packet.
11. Let the user preview and publish selected GitHub comments.

### 2.2 Product positioning

cocode is not trying to be another single AI reviewer. Its differentiator is the orchestration and verification layer:

```text
multiple CLI agents
+ local context builder
+ evidence-backed finding engine
+ visual Evidence Map
+ human triage
+ copy/publish handoff
```

---

## 3. Goals

### 3.1 MVP goals

| Goal | Description |
|---|---|
| Make multi-agent review easy | Configure once, run many agents, see one unified review. |
| Reduce false-positive effort | Evidence panels and Evidence Map make it easier to verify findings. |
| Support existing workflows | Copy fix packets into the user's main coding agent instead of forcing in-app fixing. |
| Preserve human control | No code changes or GitHub comments happen without explicit user action. |
| Start with simple integrations | Use non-interactive CLI adapters first. |
| Keep architecture extensible | Prepare for Codex App Server, ACP, MCP, A2A, and SDK integrations later. |
| Stay local-first | Source code, artifacts, sessions, and decisions are stored locally by default. |

### 3.2 Non-goals for MVP

| Non-goal | Reason |
|---|---|
| In-app code editing/fixing | The MVP should focus on review quality and handoff first. |
| Fully autonomous agent-to-agent chat | Too hard to debug and not necessary for code review MVP. |
| Cloud team runner | Adds infra, security, and tenant complexity too early. |
| Support every CLI/provider | Start with a generic CLI adapter, first stdio protocol connectors, and a few curated presets. |
| Protocol tool/permission callbacks | Keep agent protocol runs read-only until a permission UI exists. |
| Auto-publish GitHub comments | User trust requires explicit review before external side effects. |

---

## 4. Personas

### 4.1 Primary persona: AI-assisted code reviewer

The primary user already uses coding agents while reviewing PRs. They want multiple opinions but do not want to manually coordinate the agents. They care about depth, trust, and speed.

### 4.2 Secondary persona: solo developer using a main coding agent

This user wants cocode to review changes and produce fix packets they can paste into a running coding agent such as Codex CLI, Claude Code, Cursor, OpenCode, or another terminal-based agent.

### 4.3 Secondary persona: team lead or maintainer

This user wants consistent reviews, fewer false positives, auditable decisions, and clean GitHub comments that do not annoy contributors.

### 4.4 Secondary persona: power user / agent integrator

This user wants to configure custom CLI commands, output parsers, and later richer connections such as Codex App Server or ACP.

---

## 5. Core User Journeys

### 5.1 New thread

1. User opens cocode.
2. User chooses a source:
   - Paste GitHub PR URL.
   - Review local changes.
   - Compare branches.
3. User sees suggested setup:
   - Codex.
   - Claude Code.
   - Gemini.
   - Local Verifier.
   - Security-sensitive preset.
4. User optionally describes what to focus on.
5. User clicks **Continue to configure review**.

### 5.2 Configure review

1. User sees changed files and can include/exclude them.
2. User sees available agents and toggles them.
3. User selects review preset.
4. User configures context policies:
   - Include changed code.
   - Include related call sites.
   - Include related tests.
   - Include project conventions.
   - Redact secrets.
   - Keep selected files local-only.
5. User configures runtime/model/reasoning defaults per agent where supported.
6. User sets a runtime limit.
7. User starts the review.

### 5.3 Review running / chat

1. User sees the review thread.
2. User sees progress, phase, files scanned, findings count, high severity count, active agents, and estimated remaining time.
3. User sees per-agent status cards.
4. User sees early findings.
5. User can pause the review.
6. User can cancel one agent.
7. User can ask a high-level follow-up in the thread.

### 5.4 Findings board

1. User opens Findings tab.
2. User sees summary cards:
   - Total findings.
   - Verified.
   - Needs triage.
   - Accepted.
   - Dismissed.
3. User searches or filters findings.
4. User selects a finding.
5. User sees a right-side preview with code, claim, consensus, and actions.
6. User can copy, accept, dismiss, open full detail, or open Evidence Map.

### 5.5 Finding detail

1. User opens one finding.
2. User sees title, severity, verification status, and affected location.
3. User sees changed code, surrounding code, and related tests.
4. User sees evidence cards with citations to files and lines.
5. User sees counter-evidence if available.
6. User sees agent consensus.
7. User sees finding metadata.
8. User sees a draft GitHub comment.
9. User sees suggested fix summary.
10. User can copy fix packet, copy comment, accept, dismiss, ask follow-up, or open Evidence Map.

### 5.6 Evidence Map

1. User opens Evidence Map for a finding.
2. User sees code hierarchy with relevant files and line ranges.
3. User sees an evidence graph with nodes such as:
   - Router setup.
   - Changed route.
   - Middleware or guard.
   - Handler.
   - Related test.
   - Config/gateway.
4. User sees edges such as:
   - calls.
   - mounts.
   - protects.
   - tests.
   - supports.
   - contradicts.
   - missing guard.
5. User sees a call path strip, for example:

```text
router/setup.go L34 -> routes/billing.go L132 -> handlers/payouts.go L210
```

6. User sees a right-side finding panel with:
   - Finding title.
   - Severity.
   - Why it matters.
   - Evidence checklist.
   - Ask verifier action.
   - Finding metadata.
7. User can click graph nodes to open code.
8. User can ask the verifier to re-check a relationship.
9. User can return to finding detail.

### 5.7 Finding-scoped follow-up

1. User opens follow-up on a finding.
2. User sees the evidence bundle in use.
3. User asks a focused question.
4. Selected agent answers using only the finding context unless more context is requested.
5. Answer cites files, line ranges, and evidence items.
6. User can ask for counter-evidence.
7. User can accept, dismiss, copy, or return to finding.

### 5.8 Publish and copy

1. User opens Publish tab.
2. User sees accepted findings and can select which ones to include.
3. User sees GitHub review preview with inline comments and diff context.
4. User sees Copy Fix Packet preview.
5. User can choose format:
   - Markdown.
   - XML-ish.
   - JSON.
   - Compact.
   - GitHub summary.
6. User can copy selected findings, accepted findings, or one comment.
7. User can save as draft.
8. User can publish to GitHub.

---

## 6. User Stories

### 6.1 Workspace and PR setup

1. As a developer, I want to open a local repository, so that cocode can inspect my code locally.
2. As a developer, I want cocode to validate that the selected folder is a git repository, so that I avoid setup errors.
3. As a developer, I want to paste a GitHub PR URL, so that cocode can fetch PR metadata and diff information.
4. As a developer, I want cocode to parse owner, repo, and PR number from a GitHub URL, so that I do not have to enter metadata manually.
5. As a developer, I want to compare two branches, so that I can review a feature branch before opening a PR.
6. As a developer, I want to review local uncommitted changes, so that I can catch issues early.
7. As a developer, I want cocode to snapshot base SHA, head SHA, changed files, and diff, so that review runs are reproducible.
8. As a developer, I want to exclude files from review, so that generated files and lockfiles do not waste agent time.
9. As a developer, I want cocode to detect large diffs, so that I can decide whether to run a deep review.
10. As a developer, I want a new-thread flow that starts with “What should we review?”, so that setup feels simple.

### 6.2 Review configuration

11. As a developer, I want a Configure Review screen, so that I can tune the review before agents run.
12. As a developer, I want to select participating CLI agents, so that I can choose which opinions to include.
13. As a developer, I want to configure runtime/model/reasoning per agent where supported, so that I can balance quality and speed.
14. As a developer, I want to use presets such as Security & Auth Focus, Performance Deep Dive, Refactor Readiness, and Release Readiness, so that I can start with useful defaults.
15. As a developer, I want to include related call sites, so that agents can review impact beyond the diff.
16. As a developer, I want to include related tests, so that findings can be checked against coverage.
17. As a developer, I want to include project conventions, so that review comments match the repository style.
18. As a developer, I want to redact secrets before context is passed to agents, so that sensitive values are protected.
19. As a developer, I want to mark files as local-only, so that they are never sent to cloud-backed CLIs.
20. As a developer, I want to set a runtime limit, so that review does not run indefinitely.
21. As a developer, I want review settings saved per thread, so that follow-up questions use consistent defaults.

### 6.3 CLI agent execution

22. As a developer, I want cocode to run CLI agents non-interactively, so that it can automate agents without opening interactive terminals.
23. As a developer, I want to configure CLI command, arguments, cwd, env allowlist, timeout, and output mode, so that custom agents can be added.
24. As a developer, I want cocode to pass prompts through stdin or a temp prompt file, so that long context bundles are supported.
25. As a developer, I want cocode to capture stdout, stderr, exit code, duration, and cancellation reason, so that agent runs are auditable.
26. As a developer, I want cocode to support JSON, JSONL/NDJSON, and text output parsing, so that different CLIs can be integrated.
27. As a developer, I want cocode to store raw CLI output as an artifact, so that I can debug parsing problems.
28. As a developer, I want to cancel one running CLI process, so that one slow agent does not block the review.
29. As a developer, I want cocode to keep partial results if one CLI fails, so that completed work remains useful.
30. As a developer, I want cocode to run multiple agents concurrently with bounded concurrency, so that the review is faster but not chaotic.
31. As a developer, I want agent availability checks, so that missing commands or auth issues appear before the run starts.
32. As a developer, I want the code structure to support future persistent JSON-RPC and ACP connectors, so that the MVP does not paint us into a corner.

### 6.4 Review running and observability

33. As a developer, I want review events streamed live, so that I can see progress without refreshing.
34. As a developer, I want a review status panel, so that I can see phase, files scanned, findings, active agents, and estimated time.
35. As a developer, I want per-agent cards, so that I can understand what each agent is doing.
36. As a developer, I want early findings to appear before the run finishes, so that I can start triage sooner.
37. As a developer, I want pause and cancellation controls, so that I can interrupt expensive or irrelevant runs.
38. As a developer, I want raw prompts and artifacts available in debug mode, so that I can inspect provenance.
39. As a developer, I want cost and token estimates where available, so that I can make informed tradeoffs.
40. As a developer, I want review sessions to survive app restart, so that long-running work can be resumed or inspected.

### 6.5 Context building

41. As a developer, I want cocode to build context bundles automatically, so that I do not gather files manually.
42. As a developer, I want context bundles to include changed code, surrounding code, related call sites, related tests, and project conventions, so that agents receive useful evidence.
43. As a developer, I want context bundles to include PR metadata and review focus, so that agents understand intent.
44. As a developer, I want cocode to estimate token size, so that context stays within useful limits.
45. As a developer, I want cocode to track which context items were sent to each agent, so that results are auditable.
46. As a developer, I want finding-scoped context for follow-up questions, so that answers stay focused.
47. As a developer, I want context bundles to include evidence-map-specific items, so that graph generation has the necessary code relationships.

### 6.6 Findings

48. As a developer, I want raw CLI output normalized into finding candidates, so that different agents can feed one review board.
49. As a developer, I want finding candidates validated against a schema, so that malformed outputs do not pollute the UI.
50. As a developer, I want one repair pass for malformed JSON/text outputs, so that minor formatting failures can be recovered.
51. As a developer, I want similar findings deduplicated, so that the board is not noisy.
52. As a developer, I want one canonical finding per issue, so that I can make one decision.
53. As a developer, I want provenance from all candidate findings, so that I know which agents contributed.
54. As a developer, I want severity, category, confidence, and verification status, so that I can prioritize.
55. As a developer, I want decision states such as accepted, dismissed, deferred, copied, and published, so that review progress is clear.
56. As a developer, I want to record dismissal reasons, so that future reviews can reduce similar false positives.
57. As a developer, I want finding fingerprints, so that reruns can identify repeated findings.

### 6.7 Verification and evidence

58. As a developer, I want cocode to verify findings using local code search and deterministic checks, so that unsupported claims are downgraded.
59. As a developer, I want supporting evidence attached to each finding, so that I can understand why it is true.
60. As a developer, I want counter-evidence attached where found, so that I can detect false positives.
61. As a developer, I want findings marked verified, plausible, needs human, likely false positive, duplicate, or not actionable, so that review decisions are explicit.
62. As a developer, I want evidence cards in Finding Detail, so that I can inspect files and line ranges quickly.
63. As a developer, I want Local Verifier as a first-class agent, so that deterministic checks can complement LLM agents.
64. As a developer, I want verifier output to be visible, so that I know why a finding was upgraded or downgraded.

### 6.8 Evidence Map

65. As a developer, I want to open an Evidence Map from a finding, so that I can verify claims visually.
66. As a developer, I want Evidence Map to show code hierarchy with relevant files and line ranges, so that I can orient myself quickly.
67. As a developer, I want Evidence Map to show graph nodes for changed code, related code, guard/middleware, handler, tests, config, and counter-evidence, so that relationships are visible.
68. As a developer, I want Evidence Map to show graph edges for calls, mounts, protects, tests, missing guard, contradicts, and supports, so that the reasoning path is explicit.
69. As a developer, I want Evidence Map to show a call path strip, so that I can understand execution flow at a glance.
70. As a developer, I want Evidence Map nodes to deep-link to file and line, so that I can open exact code quickly.
71. As a developer, I want Evidence Map to open files in my external editor, so that I can inspect or fix the code in my preferred environment.
72. As a developer, I want the right panel to show why the issue matters, evidence checklist, actions, and metadata, so that I can decide without leaving the screen.
73. As a developer, I want to ask verifier using current Evidence Map context, so that a specific path or relationship can be rechecked.
74. As a developer, I want Evidence Map to handle incomplete graphs gracefully, so that the feature remains useful even when full static analysis is not possible.

### 6.9 Follow-up Q&A

75. As a developer, I want to ask a follow-up question scoped to a finding, so that I can resolve uncertainty.
76. As a developer, I want follow-up to use the evidence bundle by default, so that answers do not drift.
77. As a developer, I want to select runtime/model/reasoning for follow-up, so that I can choose speed or depth.
78. As a developer, I want follow-up answers to cite files and lines where possible, so that they are verifiable.
79. As a developer, I want follow-up messages persisted, so that review decisions remain auditable.
80. As a developer, I want quick actions such as ask for counter-evidence, accept finding, and dismiss finding, so that follow-up flows into triage.

### 6.10 Copy Fix Packet

81. As a developer, I want to copy one finding, so that I can paste it into a running coding agent.
82. As a developer, I want to copy selected findings, so that I can split fix work into logical groups.
83. As a developer, I want to copy all accepted findings, so that I can ask my main agent to fix everything I approved.
84. As a developer, I want packet formats such as Markdown, XML-ish, JSON, compact, and GitHub summary, so that I can match the receiving agent.
85. As a developer, I want a copy packet to include repo snapshot, base/head SHA, accepted findings, evidence, suggested fixes, and acceptance criteria, so that the receiving agent has enough context.
86. As a developer, I want copy packets to exclude dismissed/deferred findings unless explicitly selected, so that the receiving agent does not fix noise.
87. As a developer, I want cocode to write the packet to clipboard through a secure Electron preload/main bridge, so that the renderer does not get broad system access.
88. As a developer, I want generated copy packets stored as artifacts, so that I can re-copy or audit them later.
89. As a developer, I want token estimates for packets, so that I can keep the receiving agent within context limits.
90. As a developer, I want copied findings marked as copied, so that I know what I already handed off.

### 6.11 GitHub publishing

91. As a developer, I want to preview GitHub review comments, so that I know what will be posted.
92. As a developer, I want to select findings to publish, so that only accepted issues reach GitHub.
93. As a developer, I want cocode to map finding locations to GitHub diff positions/lines, so that inline comments are accurate.
94. As a developer, I want cocode to warn when a line cannot be anchored, so that comments are not misplaced.
95. As a developer, I want to publish selected inline comments, so that the PR receives actionable feedback.
96. As a developer, I want to publish a summary-only review, so that I can avoid over-commenting.
97. As a developer, I want to save a pending/draft review, so that I can submit later.
98. As a developer, I want to submit COMMENT or REQUEST_CHANGES, so that the GitHub review event matches my intent.
99. As a developer, I want GitHub review/comment IDs stored, so that cocode can avoid duplicates and show publication status.
100. As a developer, I want publishing to always require explicit user approval, so that AI never speaks for me unexpectedly.

### 6.12 Security and settings

101. As a developer, I want credentials stored securely, so that tokens are not leaked in project files.
102. As a developer, I want cocode to prefer each CLI's own auth store, so that credentials are not duplicated unnecessarily.
103. As a developer, I want a local backend token, so that random local web pages cannot call cocode APIs.
104. As a developer, I want cocode to bind only to localhost, so that the backend is not exposed to the network.
105. As a developer, I want path sandboxing, so that agents cannot read outside the workspace by accident.
106. As a developer, I want secret redaction reports, so that I know what was removed before agent calls.
107. As a developer, I want write actions denied during review mode, so that review agents cannot modify code.
108. As a developer, I want a settings screen for agents, CLIs, GitHub, presets, and permissions, so that setup is manageable.
109. As a developer, I want custom CLI adapter config, so that I can bring my own agent.
110. As a developer, I want adapter health checks, so that broken integrations are obvious.

### 6.13 Future extensibility

111. As a developer, I want a future Codex App Server connection type, so that cocode can eventually use richer bidirectional JSON-RPC sessions instead of only one-shot CLI runs.
112. As a developer, I want a future ACP connection type, so that ACP-compatible agents such as Gemini CLI ACP mode can be integrated cleanly.
113. As a developer, I want a future MCP client layer, so that approved tools and context servers can be exposed consistently.
114. As a developer, I want a future A2A layer, so that remote opaque agents can be delegated work later.
115. As a developer, I want all future connection types mapped into the same AgentAdapter interface, so that the UI and review workflow do not need to change.

---

## 7. Functional Requirements

### 7.1 Workspace and PR ingestion

| ID | Requirement | Priority |
|---|---|---|
| FR-001 | User can open a local repository. | P0 |
| FR-002 | App validates selected path is a git repo. | P0 |
| FR-003 | User can paste a GitHub PR URL. | P0 |
| FR-004 | App parses owner, repo, and PR number from URL. | P0 |
| FR-005 | App fetches PR metadata, base SHA, head SHA, title, author, and changed files. | P0 |
| FR-006 | App can compare local branches. | P0 |
| FR-007 | App can review local uncommitted changes. | P1 |
| FR-008 | App snapshots diff and metadata for reproducibility. | P0 |
| FR-009 | App detects changed files and line ranges. | P0 |
| FR-010 | App lets user exclude files from review. | P0 |

### 7.2 Review configuration

| ID | Requirement | Priority |
|---|---|---|
| FR-011 | User can choose review preset. | P0 |
| FR-012 | User can toggle participating CLI agents. | P0 |
| FR-013 | User can configure runtime/model/reasoning where supported. | P0 |
| FR-014 | User can configure review timeout. | P0 |
| FR-015 | User can toggle context policies. | P0 |
| FR-016 | User can mark files local-only. | P0 |
| FR-017 | User can enable secret redaction. | P0 |
| FR-018 | User can enter custom review focus. | P0 |

### 7.3 CLI agent execution

| ID | Requirement | Priority |
|---|---|---|
| FR-019 | MVP supports non-interactive CLI execution. | P0 |
| FR-020 | App can configure CLI command, args, cwd, env allowlist, timeout, and output mode. | P0 |
| FR-021 | App can pass prompt via stdin or temp prompt file. | P0 |
| FR-022 | App captures stdout, stderr, exit code, duration, and cancellation status. | P0 |
| FR-023 | App supports JSON, JSONL/NDJSON, and text output parsing. | P0 |
| FR-024 | App stores raw CLI output as artifact. | P0 |
| FR-025 | App can cancel one running CLI process. | P0 |
| FR-026 | App keeps partial review results if one CLI fails. | P0 |
| FR-027 | App can run multiple CLI agents concurrently with bounded concurrency. | P0 |
| FR-028 | App displays agent availability and setup problems. | P0 |
| FR-029 | App records CLI session IDs when available. | P1 |
| FR-030 | Code structure supports future JSON-RPC and ACP connectors. | P0 architecture |

### 7.4 Context building

| ID | Requirement | Priority |
|---|---|---|
| FR-031 | App builds context bundles from diff, changed files, surrounding code, related call sites, tests, and project conventions. | P0 |
| FR-032 | App estimates token size for context bundles. | P0 |
| FR-033 | App redacts secrets before sending context to cloud-backed CLIs where possible. | P0 |
| FR-034 | App tracks which context items were sent to each agent. | P0 |
| FR-035 | App stores context bundle artifacts. | P0 |
| FR-036 | App supports finding-scoped context for follow-up questions. | P0 |
| FR-037 | App supports evidence-map-specific context items. | P0 |

### 7.5 Findings

| ID | Requirement | Priority |
|---|---|---|
| FR-038 | App normalizes CLI output into FindingCandidate records. | P0 |
| FR-039 | App validates finding schema. | P0 |
| FR-040 | App attempts one repair pass for malformed agent output. | P0 |
| FR-041 | App deduplicates similar findings. | P0 |
| FR-042 | App produces CanonicalFinding records. | P0 |
| FR-043 | App stores provenance from all candidate findings. | P0 |
| FR-044 | App assigns severity, category, confidence, and verification status. | P0 |
| FR-045 | App supports accept, dismiss, defer, copied, and published decision states. | P0 |
| FR-046 | User can record dismissal reason. | P1 |

### 7.6 Verification and evidence

| ID | Requirement | Priority |
|---|---|---|
| FR-047 | App verifies findings using local code search and deterministic checks. | P0 |
| FR-048 | App attaches supporting evidence. | P0 |
| FR-049 | App attaches counter-evidence where found. | P0 |
| FR-050 | App marks findings verified, plausible, needs human, likely false positive, duplicate, or not actionable. | P0 |
| FR-051 | App shows evidence cards in finding detail. | P0 |
| FR-052 | App supports Local Verifier as a first-class agent. | P0 |

### 7.7 Evidence Map

| ID | Requirement | Priority |
|---|---|---|
| FR-053 | User can open Evidence Map from a finding. | P0 |
| FR-054 | Evidence Map shows code hierarchy with relevant files and line ranges. | P0 |
| FR-055 | Evidence Map shows graph nodes for changed code, related code, guard/middleware, handler, tests, config, and counter-evidence. | P0 |
| FR-056 | Evidence Map shows graph edges for calls, mounts, protects, tests, missing guard, contradicts, and supports. | P0 |
| FR-057 | Evidence Map shows a call path strip. | P0 |
| FR-058 | Evidence Map can deep-link to file and line. | P0 |
| FR-059 | Evidence Map can open a file in external editor. | P1 |
| FR-060 | Evidence Map right panel shows why it matters, evidence checklist, actions, and finding metadata. | P0 |
| FR-061 | User can ask verifier using current Evidence Map context. | P1 |
| FR-062 | Evidence Map handles incomplete graphs gracefully. | P0 |

### 7.8 Follow-up Q&A

| ID | Requirement | Priority |
|---|---|---|
| FR-063 | User can ask a follow-up question scoped to a finding. | P0 |
| FR-064 | Follow-up uses evidence bundle by default. | P0 |
| FR-065 | User can select agent/runtime/model/reasoning for follow-up. | P0 |
| FR-066 | Follow-up answers cite files and lines where possible. | P0 |
| FR-067 | Follow-up messages are persisted. | P0 |

### 7.9 Copy Fix Packet

| ID | Requirement | Priority |
|---|---|---|
| FR-068 | User can copy one finding. | P0 |
| FR-069 | User can copy selected findings. | P0 |
| FR-070 | User can copy all accepted findings. | P0 |
| FR-071 | User can choose packet format: Markdown, XML-ish, JSON, compact, GitHub summary. | P0 |
| FR-072 | Copy packet includes repo snapshot, base/head SHA, accepted findings, evidence, suggested fixes, and acceptance criteria. | P0 |
| FR-073 | Copy packet excludes dismissed/deferred findings unless explicitly selected. | P0 |
| FR-074 | App writes copy packet to clipboard through secure Electron preload/main bridge. | P0 |
| FR-075 | App stores generated copy packet as artifact. | P1 |
| FR-076 | App shows estimated token count for packet. | P1 |

### 7.10 GitHub publishing

| ID | Requirement | Priority |
|---|---|---|
| FR-077 | User can preview GitHub review. | P0 |
| FR-078 | User can select findings to publish. | P0 |
| FR-079 | App maps finding locations to GitHub diff positions/lines. | P0 |
| FR-080 | App warns when a line cannot be anchored. | P0 |
| FR-081 | User can publish selected inline comments. | P0 |
| FR-082 | User can publish summary-only review. | P1 |
| FR-083 | User can save pending/draft review. | P1 |
| FR-084 | User can submit COMMENT or REQUEST_CHANGES review event. | P0 |
| FR-085 | App tracks GitHub review/comment IDs. | P0 |
| FR-086 | App avoids duplicate publishing on rerun. | P0 |

### 7.11 Observability and control

| ID | Requirement | Priority |
|---|---|---|
| FR-087 | App streams review events to UI. | P0 |
| FR-088 | App shows live progress and per-agent status. | P0 |
| FR-089 | User can pause review. | P1 |
| FR-090 | User can cancel entire review. | P0 |
| FR-091 | User can cancel one agent. | P0 |
| FR-092 | App stores event log for replay/debugging. | P0 |
| FR-093 | App shows partial results after failures. | P0 |

---

## 8. Implementation Decisions

### 8.1 Architecture decisions

| Decision | Rationale |
|---|---|
| Local-first Electron app | Local repo access, native clipboard, CLI execution, and privacy. |
| Go + Gin local backend | Strong fit for process orchestration, local APIs, streaming, git integration, and the user's Go expertise. |
| SQLite local DB | Embedded durable storage for sessions, findings, artifacts, and decisions. |
| REST + SSE | REST for commands; SSE for review progress/events. |
| Finding-first domain model | Findings are user-facing objects; raw transcripts are provenance. |
| CLI-first phase | Fastest path to real user value and simplest adapter surface. |
| Extensible connection abstraction | Codex App Server, ACP, MCP, A2A, and SDKs should not require UI rewrite. |
| Evidence Map as MVP | Directly addresses the pain point of manually scouting the codebase. |
| Copy Fix Packet as MVP | Directly supports the user's current workflow of fixing in a separate main coding agent. |

### 8.2 Agent collaboration decision

CLI agents do not need to talk directly to each other.

cocode uses an orchestrator-mediated blackboard model:

1. cocode builds a context bundle.
2. CLI Agent A receives a review task and context.
3. CLI Agent A returns raw output.
4. cocode normalizes output into finding candidates.
5. CLI Agent B or Local Verifier receives selected findings and evidence requests.
6. Verifier checks/supports/refutes each claim.
7. cocode deduplicates and synthesizes canonical findings.
8. UI shows a unified board and Evidence Map.

This follows the central-hub pattern: the orchestrator owns state and passes artifacts between agents instead of requiring peer-to-peer agent communication.

### 8.3 Future adapter design decision

MVP implementation must define interfaces that can later support these connection types:

| Connection type | MVP support | Future support |
|---|---:|---:|
| One-shot non-interactive CLI | Yes | Yes |
| Streaming CLI JSON/JSONL | Partial | Yes |
| Persistent JSON-RPC stdio | First stdio connector | Yes |
| Codex App Server | Ephemeral thread + streaming turns | Resume + richer client callbacks |
| ACP stdio agent | Session prompt + streaming chunks | Richer client callbacks |
| MCP tool/context server | Interface only | Yes |
| A2A remote agent | No | Later |
| Provider SDK/API | No | Later |

### 8.4 Evidence Map implementation decision

Evidence Map is not a generic architecture diagram. It is a finding-specific verification surface. It should be generated from stored evidence items, local code-map data, route/symbol relationships, test references, and verifier notes.

MVP graph quality should be pragmatic:

- Full language-server accuracy is not required.
- Static/deterministic relationships are preferred where available.
- AI may summarize relationships, but graph nodes/edges must reference real files and lines where possible.
- Missing relationships, such as missing middleware guard, are represented explicitly as negative evidence.

### 8.5 Copy packet implementation decision

Copy packet generation is part of the core MVP, not an export afterthought.

Default packet format: Markdown.

Default packet includes:

- PR/repo snapshot.
- Base/head SHA.
- Selected accepted findings.
- Evidence and counter-evidence.
- Suggested fix direction.
- Acceptance criteria.
- Explicit instruction not to fix dismissed/deferred/unverified findings.

### 8.6 Done criteria decision

Every implementation task must define **done criteria**. A task is not done merely because code exists. It is done when:

1. The user-visible behavior works.
2. Data is persisted or emitted as required.
3. Error cases are handled.
4. Tests exist at the appropriate level.
5. Security/privacy constraints are respected.
6. Documentation or comments are added where future maintainers need them.
7. The feature integrates with the surrounding workflow.

The Task Breakdown document includes per-task done criteria.

---

## 9. Testing Decisions

### 9.1 Testing philosophy

Tests should validate external behavior rather than private implementation details.

Good tests answer questions like:

- Can a user open a repo and create a review session?
- Can the backend run a fake CLI and convert output into findings?
- Can a finding be verified and displayed with evidence?
- Can the Evidence Map display nodes, edges, and call path for a finding?
- Can a user accept findings and copy a fix packet?
- Can GitHub preview/publish payloads be generated correctly?
- Can errors preserve partial results?

Avoid tests that assert exact LLM prose unless the prose is part of a fixed template.

### 9.2 Test modules

| Module | Test focus |
|---|---|
| Workspace Manager | Repo open, invalid repo, dirty state handling. |
| Git/PR Ingestion | URL parsing, diff fetch, changed files, snapshotting. |
| Context Builder | Context item selection, line ranges, token estimates, redaction. |
| CLI Adapter | Command invocation, stdin/file prompt, stdout/stderr capture, timeouts, cancellation. |
| Agent Runtime | Concurrency, partial failure, event emission. |
| Finding Engine | Schema validation, repair, dedupe, ranking, provenance. |
| Verification Engine | Code search, counter-evidence, status assignment. |
| Evidence Map Engine | Node/edge creation, call path, incomplete graph fallback. |
| Follow-up Engine | Finding-scoped context, answers persisted. |
| Copy Packet Exporter | Markdown/XML-ish/JSON/compact output, clipboard flow. |
| GitHub Publisher | Diff mapping, preview payloads, publish state, duplicate prevention. |
| Security Engine | Local auth, path sandboxing, secret redaction, permission checks. |
| UI | Setup flow, running state, findings board, finding detail, Evidence Map, follow-up, publish/copy. |

### 9.3 Acceptance tests

1. Given a local git repo, the app opens it and stores a workspace.
2. Given a GitHub PR URL, the app creates a PR snapshot with correct metadata and changed files.
3. Given a fake CLI agent that outputs valid JSON, the app creates finding candidates.
4. Given two agents with duplicate findings, the app creates one canonical finding with provenance from both.
5. Given a finding, the app verifies it with local code search and attaches evidence.
6. Given a verified finding, the Finding Detail screen shows changed code, evidence, consensus, draft comment, and copy actions.
7. Given a verified finding, the Evidence Map shows relevant hierarchy, graph nodes, graph edges, and call path.
8. Given a finding follow-up question, the answer is persisted and cites evidence where possible.
9. Given accepted findings, the app generates a Markdown copy packet and writes it to clipboard.
10. Given accepted findings, the app creates a GitHub publish preview with correctly mapped line comments.
11. Given one CLI agent times out, other completed findings remain available.
12. Given app restart, previous sessions, findings, evidence, and decisions are still visible.

---

## 10. Out of Scope

The following are out of scope for MVP:

- In-app code editing or automatic fixing.
- Automatic commit creation or push.
- Auto-publishing GitHub comments.
- Cloud-hosted review runner.
- Team/org administration.
- Full A2A remote-agent integration.
- Full MCP tool marketplace.
- Full ACP client-side permission/tool/file callback implementation.
- Full Codex App Server thread resume and client-side permission/tool callback implementation.
- Real-time peer-to-peer conversations among CLI agents.
- GitLab, Bitbucket, Azure DevOps, Gerrit.
- Fine-tuning models.
- Training on user code or decisions without explicit consent.

---

## 11. Further Notes

The MVP should deliberately choose reliability over autonomy. The attached book emphasizes using the simplest architecture that satisfies the problem and choosing workflow patterns when reliability and debuggability matter. cocode should therefore start with a deterministic review workflow:

```text
ingest -> context -> parallel agent review -> normalize -> dedupe -> verify -> evidence map -> human triage -> copy/publish
```

The initial protocol support should stay read-only and review-focused: CLI execution remains the simplest adapter, while Codex App Server and ACP stdio add streaming/session coverage without granting tools or write permissions.

The Evidence Map is a major product differentiator. It directly addresses the pain point: “I have to scout the codebase myself to see if the finding is true.” If Evidence Map works well, cocode becomes more than a prompt runner; it becomes a trust and verification layer.

---

## 12. Source Notes

This PRD was grounded in:

- The user-provided `Designing-Multi-Agent-Systems.pdf`, especially chapters on multi-agent patterns, UX principles, workflow building, modern agent UIs, evaluation, distributed protocols, and software-engineering agents.
- OpenAI Codex App Server materials describing JSON-RPC/stdio integration and long-running App Server clients.
- Agent Client Protocol documentation describing JSON-RPC and stdio client-agent transport.
- Claude Code and OpenCode/Gemini CLI automation documentation for non-interactive or protocol-driven CLI integrations.
- GitHub REST API documentation for pull request reviews and review comments.

---

## 13. Reference URLs

- OpenAI Codex App Server blog: https://openai.com/index/unlocking-the-codex-harness/
- OpenAI Codex App Server README: https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md
- Agent Client Protocol transports: https://agentclientprotocol.com/protocol/transports
- Agent Client Protocol overview: https://agentclientprotocol.com/protocol/overview
- Claude Code programmatic/headless CLI: https://code.claude.com/docs/en/headless
- Gemini CLI ACP mode: https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/acp-mode.md
- GitHub pull request reviews REST API: https://docs.github.com/rest/pulls/reviews
- GitHub pull request review comments REST API: https://docs.github.com/en/rest/pulls/comments
- Electron security tutorial: https://www.electronjs.org/docs/latest/tutorial/security
- Gin documentation: https://gin-gonic.com/en/docs/
- SQLite WAL documentation: https://www.sqlite.org/wal.html
- sqlc documentation: https://sqlc.dev/
