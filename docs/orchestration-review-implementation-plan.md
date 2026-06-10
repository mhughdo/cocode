# Cocode Orchestration Review Implementation Plan

**Status:** Draft  
**Created:** 2026-06-10  
**Owner:** Cocode local review workflow  
**Source review:** User-provided orchestration review of `cocode`  
**Primary goal:** Make local AI code review higher-signal, traceable, resilient, and efficient before expanding CI automation.

## How To Track Progress

Use this file as the living implementation tracker.

Progress states:

- `todo`: not started.
- `doing`: actively being implemented.
- `blocked`: cannot progress without a decision or external dependency.
- `done`: implemented, tested, and verified.
- `deferred`: intentionally postponed, with reason recorded.

Update rules:

1. Change task status in the task tables as work lands.
2. Add a line to the progress ledger for every merged implementation batch.
3. Keep the review coverage matrix current; every audit item should map to at least one task ID.
4. Do not mark a task `done` unless the listed verification has been run or explicitly replaced.
5. If scope changes, add a decision note rather than silently deleting the task.

## Progress Ledger

| Date | Change | Tasks | Verification | Notes |
| --- | --- | --- | --- | --- |
| 2026-06-10 | Initial implementation plan created. | all | not run | Planning only. |
| 2026-06-10 | Expanded audit coverage and moved read-only hardening into the first batch. | `SAFE-*`, coverage matrix | not run | Planning only. |
| 2026-06-10 | Fixed persisted severity priority for verifier and curator ordering. | `ORCH-01` | `go test ./internal/orchestrator -run 'TestSeverityPriorityUsesPersistedFindingScale|TestPrioritizedVerifierFindingsRanksBlockerBeforeHigh|TestCurationCandidateScoreRanksBlockerBeforeHigh'` | `blocker` now ranks above `high`. |
| 2026-06-10 | Preserved agent-targeted centralized chat answers on reload. | `CHAT-01` | `go test ./internal/chat -run 'TestShouldHideReviewAgentRunFromChatSkipsInternalWorkflowRuns|TestRemoveHiddenReviewAgentRunMessagesDeletesPersistedInternalCards'` | `role=chat` messages are no longer deleted as hidden workflow cards. |
| 2026-06-10 | Included requested project rules in standard context and expanded rule discovery. | `CTX-01`, `CTX-02` | `go test ./internal/contextbundle ./internal/projectrules` | Standard reviews keep project rule items; discovery includes AGENTS, CLAUDE, CONTRIBUTING, and Cursor rules. |
| 2026-06-10 | Enforced read-only runtime settings for review agents. | `SAFE-01`, `SAFE-03` | `go test ./internal/agents ./internal/agentpreset ./internal/agentrun ./internal/db ./internal/httpapi -run 'ReviewMode|CodexCLI|Antigravity|JSONRPC|PromotesDefaultCodex|AgentPresets'` | Codex CLI/App Server default to read-only; stale Codex configs are repaired; risky review-mode runtime args fail before adapter open. |
| 2026-06-10 | Made normalize and dedupe writes restart-safe. | `ORCH-02`, `ORCH-03` | `go test ./internal/db -run 'Finding|Migration|Apply'`; `go test ./internal/orchestrator -run 'TestNormalizeAgentOutputsIsRestartSafe|TestDeduplicateFindingsIsRestartSafe'` | Candidate, finding, and candidate-link writes are idempotent across phase retries. |
| 2026-06-10 | Added workflow invariant and mixed-output preservation tests. | `BASE-01`, `BASE-02` | `go test ./internal/agentoutput ./internal/orchestrator` | Phase order/checkpoint events are encoded; structured, embedded, and malformed CLI output preservation is covered. |
| 2026-06-10 | Added embedded reviewer prompt rendering, role overlays, output contract enums, prompt artifacts, and schema-sync tests. | `PROMPT-01`, `PROMPT-02`, `PROMPT-03`, `PROMPT-04`, `PROMPT-05`, `PROMPT-06`, `PROMPT-08`, `PROMPT-09`, `PROMPT-10` | `go test ./internal/reviewprompt`; `go test ./internal/agentoutput -run 'TestParserKnownEnumsMatchPackageSchemas|TestParsePreservesMixedCLIOutputContracts|TestFindingCandidateSchema|TestFakeJSONAgentMatchesReviewAgentOutputSchema'`; `go test ./internal/orchestrator -run 'TestDefaultPromptTemplateDoesNotInjectSetupFocusLabels|TestWorkflowRunsFakeAgentEndToEnd|TestWorkflowPhaseCheckpointOrderInvariant|TestWorkflowPersistsStructuredFindingCandidates|TestWorkflowPersistsDelimitedFindingCandidateEvents|TestWorkflowPersistsMalformedOutputAsRawParsedArtifact'` | Runtime prompt source moved out of `service.go`; selected roles alter prompts/hashes; run metadata and artifacts record prompt provenance; reviewer/curator taxonomies now share canonical categories. |
| 2026-06-10 | Added deterministic changed-code anchor validation for local verifier and curator output. | `VERIFY-01`, `VERIFY-02` | `go test ./internal/evidence ./internal/orchestrator` | Curator `verified` status and primary-location overrides are gated by changed-hunk/source-line validation; invalid curator anchors fall back to representative anchors or downgrade with event provenance. |
| 2026-06-10 | Added quoted-code freshness checks and deterministic counter-evidence classification. | `VERIFY-03`, `VERIFY-04` | `go test ./internal/evidence ./internal/orchestrator` | Local verification now rejects stale quoted code observations, records matched code quotes, and can reach `likely_false_positive` from direct counter-evidence without an LLM verifier. |
| 2026-06-10 | Added consensus confidence and verifier disagreement preservation. | `VERIFY-05`, `VERIFY-06` | `go test ./internal/evidence ./internal/findingengine ./internal/orchestrator` | Distinct agent runs now boost merged confidence, and conflicting verifier outputs are stored as explicit disagreement evidence instead of being overwritten by the last writer. |
| 2026-06-10 | Documented local-first orchestration contracts for CI/adapters. | `BASE-03` | Documentation review | CI and GitHub automation are defined as triggers/reporting adapters over the same local review session, finding, evidence, chat, decision, and publish draft contracts. |
| 2026-06-10 | Added cross-session dismissal memory, transport partial-failure coverage, timeout-output preservation, and real draft comment generation. | `VERIFY-07`, `VERIFY-08`, `VERIFY-09`, `VERIFY-10` | `go test ./internal/orchestrator -run 'TestWorkflowCarriesDismissedFindingFingerprintAcrossSessions|TestWorkflowContinuesWhenOneAgentTransportOpenFails|TestWorkflowKeepsParseableTimedOutAgentOutput|TestDraftFindingCommentsCreatesMissingDrafts|TestWorkflowAgentTimeoutKeepsOtherFindings|TestWorkflowContinuesWhenOneAgentFails'` | Repository-scoped prior dismissals now carry forward by fingerprint; transport-open failures preserve successful agents; parseable timed-out output can produce candidates with provenance; `draft_comments` fills missing finding draft comments deterministically. |
| 2026-06-10 | Centralized untrusted-context prompt guidance across review, curator, verifier, chat, and follow-up prompts. | `PROMPT-07` | `go test ./internal/reviewprompt ./internal/chat ./internal/followup`; `go test ./internal/orchestrator -run 'TestWorkflowUsesSelectedOrchestratorForDedupeCuration|TestVerifyFindingsRunsVerifierCLIWithFindingContext|TestWorkflowRunsFakeAgentEndToEnd'` | Prompt builders now consume one shared instruction; the embedded reviewer prompt uses a placeholder expanded by the renderer, and prompt tests assert the exact shared wording. |
| 2026-06-10 | Split centralized chat turn creation from execution and enforced turn transitions. | `CHAT-02`, `CHAT-16` | `go test ./internal/chat`; `go test ./internal/httpapi -run 'TestReviewSessionChatThreadEndpointSeedsAndAnswers|TestReviewSessionChatThreadEndpointUsesOrchestratorResponder'` | Chat POST now returns `202 Accepted` with a created turn while execution runs from persisted state in the background; allowed turn transitions are tested. |
| 2026-06-10 | Added centralized chat turn cancellation endpoint and Stop-button wiring. | `CHAT-03` | `go test ./internal/chat ./internal/httpapi -run 'TestCancelTurnMarksRequestAndRunExitsCanceled|TestReviewSessionChatThreadEndpointSeedsAndAnswers|TestReviewSessionChatThreadEndpointUsesOrchestratorResponder'`; `pnpm --filter @cocode/desktop typecheck` | Stop now marks turns `cancel_requested`, cancels matching active chat runs when possible, finalizes canceled turns in the worker, and keeps the composer in Stop mode after async turn creation. |
| 2026-06-10 | Shared centralized chat context across all-agent fan-out and ran reviewers concurrently. | `CHAT-04`, `CHAT-05` | `go test ./internal/chat`; `go test ./internal/httpapi -run 'TestReviewSessionChatThreadEndpointSeedsAndAnswers|TestReviewSessionChatThreadEndpointUsesOrchestratorResponder'` | All-agent chat now builds one safe context bundle, prefers an external-visibility recipient when needed, and executes reviewer fan-out with bounded parallelism before synthesis. |

## Assumptions And Boundaries

Assumptions:

- Local review quality and follow-up UX are the immediate priority; CI/GitHub automation remains informed by `docs/ai-code-review-design.md`.
- Cocode remains the source of truth for threads, findings, evidence maps, decisions, and artifacts.
- External CLIs are workers. They may own transport-specific sessions later, but Cocode owns persisted state.
- Schema changes must be migration-backed and idempotent.

Always do:

- Preserve existing review session data with migrations.
- Add focused backend tests for orchestration, chat, evidence, and context changes.
- Keep prompts traceable by version/hash once prompt work begins.
- Keep human-gated behavior for publishing or workspace writes.

Ask first:

- Enabling write-capable agent tools during review.
- Replacing the current checkpoint model wholesale.
- Removing existing adapter support.

Never do:

- Let untrusted repository content override system/developer review instructions.
- Mark an LLM-curated finding verified without deterministic anchor/content checks.
- Hide or delete persisted user-visible chat answers as a rendering shortcut.

## Target Architecture

Cocode keeps the existing deterministic 8-phase workflow:

```text
build_review_context
  -> risk_scout
  -> run_review_agents
  -> normalize_outputs
  -> deduplicate_findings
  -> verify_findings
  -> build_evidence_maps
  -> draft_comments
```

The target change is not a new workflow. It is stronger contracts between phases:

- Review agents receive differentiated role overlays and exact output contracts.
- Normalize and dedupe phases are restart-safe.
- Curator and verifier agents can propose conclusions, but deterministic checks gate final status.
- Evidence maps include enough supporting and counter-evidence for a user to confirm the issue quickly.
- Centralized chat becomes an asynchronous, persistent, cancelable Cocode workflow rather than a blocking per-agent CLI loop.

## Local-First Orchestration Contracts

Local Cocode state remains the product contract:

- `review_sessions`, `findings`, `evidence_items`, `evidence_graphs`, `chat_threads`, `chat_messages`, `human_decisions`, `publish_drafts`, and artifacts are the source of truth.
- CI and GitHub automation should create, resume, inspect, and report these same records instead of owning a parallel finding or evidence schema.
- External CLI, App Server, and ACP sessions are worker/session metadata attached to agent runs or chat turns; they are not the authoritative review state.
- Human decision memory is carried by stable finding fingerprints inside a repository, with event provenance when a prior dismissal affects a later run.
- Publishing and GitHub comments consume accepted local findings and publish drafts; they do not bypass verification, decision, or evidence-map state.

## Preserve Existing Strengths

The review called out several parts of the architecture as fundamentally sound. These are constraints, not work items to replace:

- Keep deterministic checkpointed orchestration as the backbone; do not turn the whole workflow into an opaque LLM chat.
- Keep tolerant multi-format parsing and raw artifact preservation.
- Keep union-find deterministic dedupe as the fallback path under LLM curation.
- Keep local anchor validation against changed hunks, but strengthen it with quoted-code checks.
- Keep Cocode-owned thread/finding/evidence state even when adapter sessions are added.
- Keep CI/GitHub automation as an adapter over local review concepts, not a forked review system.

## Implementation Phases

### Phase 0: Workflow Invariants And Audit Baseline

These tasks make sure future refactors preserve the good parts the audit says are already working.

| ID | Status | Task | Acceptance Criteria | Verification | Likely Files |
| --- | --- | --- | --- | --- | --- |
| BASE-01 | done | Add workflow invariant tests for the 8-phase checkpoint order. | The expected phase sequence is encoded in tests and fails if phases are reordered or skipped accidentally. | `go test ./internal/orchestrator -run 'TestWorkflowPhaseCheckpointOrderInvariant'` from `services/cocoded`. | `services/cocoded/internal/orchestrator/service.go` |
| BASE-02 | done | Add parser preservation tests for mixed CLI output. | Structured JSON, JSON embedded in text, and raw malformed output are preserved with artifacts and parse status. | `go test ./internal/agentoutput -run 'TestParsePreservesMixedCLIOutputContracts'`; `go test ./internal/orchestrator -run 'TestWorkflowPersistsStructuredFindingCandidates|TestWorkflowPersistsDelimitedFindingCandidateEvents|TestWorkflowPersistsMalformedOutputAsRawParsedArtifact'` from `services/cocoded`. | `services/cocoded/internal/agentoutput`, `services/cocoded/internal/orchestrator` |
| BASE-03 | done | Document local-first orchestration contracts in this plan or an ADR. | Future CI mode is described as a trigger/reporting adapter over local session/finding/evidence contracts. | Documentation review. | `docs/orchestration-review-implementation-plan.md`, optional ADR |

### Phase 1: Correctness And Persistence Quick Wins

These are small or medium changes that remove silent correctness failures before larger architecture work.

| ID | Status | Task | Acceptance Criteria | Verification | Likely Files |
| --- | --- | --- | --- | --- | --- |
| ORCH-01 | done | Align severity ranking with persisted severity enum. | `blocker > high > medium > low > nit`; verifier and curator ordering use the same scale as finding dedupe. | `go test ./internal/orchestrator -run 'TestSeverityPriorityUsesPersistedFindingScale|TestPrioritizedVerifierFindingsRanksBlockerBeforeHigh|TestCurationCandidateScoreRanksBlockerBeforeHigh'` from `services/cocoded`. | `services/cocoded/internal/orchestrator/verifier_agent.go`, `services/cocoded/internal/orchestrator/curation_agent.go`, `services/cocoded/internal/findingengine/dedupe.go` |
| ORCH-02 | done | Make `normalize_outputs` restart-safe. | Re-running the phase after partial completion does not duplicate candidates or fail on existing rows. | `go test ./internal/db -run 'Finding|Migration|Apply'`; `go test ./internal/orchestrator -run 'TestNormalizeAgentOutputsIsRestartSafe'` from `services/cocoded`. | `services/cocoded/internal/orchestrator/service.go`, `services/cocoded/internal/db` |
| ORCH-03 | done | Make `deduplicate_findings` restart-safe. | Re-running after a mid-phase crash does not fail on `UNIQUE(review_session_id, fingerprint)` and does not duplicate findings. | `go test ./internal/db -run 'Finding|Migration|Apply'`; `go test ./internal/orchestrator -run 'TestDeduplicateFindingsIsRestartSafe'` from `services/cocoded`. | `services/cocoded/internal/orchestrator/service.go`, `services/cocoded/internal/db/schema.go` |
| CHAT-01 | done | Stop deleting persisted chat answers. | Agent-targeted chat messages with role `chat` survive thread reload and remain prompt-visible history. | `go test ./internal/chat -run 'TestShouldHideReviewAgentRunFromChatSkipsInternalWorkflowRuns|TestRemoveHiddenReviewAgentRunMessagesDeletesPersistedInternalCards'` from `services/cocoded`. | `services/cocoded/internal/chat/service.go` |
| CTX-01 | done | Include project rules at standard context depth. | Project conventions included when policy requests them in standard reviews. | `go test ./internal/contextbundle ./internal/projectrules` from `services/cocoded`. | `services/cocoded/internal/contextbundle/budget.go` |
| CTX-02 | done | Expand project rule discovery. | `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md`, and `.cursor/rules` are discovered where present. | `go test ./internal/contextbundle ./internal/projectrules` from `services/cocoded`. | `services/cocoded/internal/projectrules/discovery.go` |

Checkpoint after Phase 1:

- `go test ./internal/orchestrator ./internal/findingengine ./internal/chat ./internal/contextbundle ./internal/projectrules` from `services/cocoded`.
- Desktop smoke: start a review, ask a follow-up, reload/switch tabs, confirm chat answer remains.

### Phase 2: Prompt Contracts And Role Specialization

This phase turns the multi-agent workflow from duplicate reviewers into differentiated specialists.

| ID | Status | Task | Acceptance Criteria | Verification | Likely Files |
| --- | --- | --- | --- | --- | --- |
| PROMPT-01 | done | Move live reviewer prompt to a single embedded markdown source. | Runtime prompt source is not a divergent inline Go string; prompt rendering has a version/hash. | `go test ./internal/reviewprompt` from `services/cocoded`. | `packages/prompts/review-agent.md`, `services/cocoded/internal/orchestrator/service.go` |
| PROMPT-02 | done | Persist rendered prompt artifacts. | Each agent run records prompt version/hash and enough rendered prompt metadata to trace findings. | `go test ./internal/orchestrator -run 'TestWorkflowRunsFakeAgentEndToEnd'` from `services/cocoded`. | `services/cocoded/internal/agents`, `services/cocoded/internal/orchestrator`, `services/cocoded/internal/db` |
| PROMPT-03 | done | Add exact output enums, confidence scale, JSON-only rule, and worked example. | Reviewer prompt lists valid severities/categories and says to output exactly one JSON object. Unknown enum coercion is reduced to fallback behavior only. | `go test ./internal/reviewprompt`; `go test ./internal/agentoutput -run 'TestParserKnownEnumsMatchPackageSchemas'` from `services/cocoded`. | `services/cocoded/internal/agentoutput/candidates.go`, `packages/prompts/review-agent.md` |
| PROMPT-04 | done | Add severity rubric, stop conditions, and finding cap. | Agents have behavior-mapped severity definitions, `findings: []` guidance, and an explicit maximum finding count. | `go test ./internal/reviewprompt` from `services/cocoded`. | `packages/prompts/review-agent.md` |
| PROMPT-05 | done | Add role/preset overlays to review prompts. | Each configured role contributes role objective, focus areas, boundaries, and `do not flag` guidance to the prompt. | `go test ./internal/reviewprompt` from `services/cocoded`. | `services/cocoded/internal/orchestrator/service.go`, `services/cocoded/internal/agentpreset`, `docs/presets-and-roles.md` |
| PROMPT-06 | done | Align reviewer and curator taxonomies. | Curator categories map to the same canonical category enum used by parsed candidates. | `go test ./internal/agentoutput -run 'TestParserKnownEnumsMatchPackageSchemas'`; `go test ./internal/orchestrator -run 'TestWorkflowUsesSelectedOrchestratorForDedupeCuration|TestWorkflowRunsFakeAgentEndToEnd'` from `services/cocoded`. | `services/cocoded/internal/orchestrator/curation_agent.go`, `services/cocoded/internal/agentoutput/candidates.go` |
| PROMPT-07 | done | Centralize injection-defense text. | Review, curator, verifier, evidence, and chat prompts share consistent untrusted-context guidance. | `go test ./internal/reviewprompt ./internal/chat ./internal/followup`; `go test ./internal/orchestrator -run 'TestWorkflowUsesSelectedOrchestratorForDedupeCuration|TestVerifyFindingsRunsVerifierCLIWithFindingContext|TestWorkflowRunsFakeAgentEndToEnd'` from `services/cocoded`. | `services/cocoded/internal/orchestrator`, `services/cocoded/internal/chat`, `packages/prompts` |
| PROMPT-08 | done | Wire and test `PromptTemplate` override behavior. | Runtime prompt override has a deliberate API, is tested, and is included in prompt hash/provenance. | `go test ./internal/reviewprompt -run 'TestRenderReviewPromptTracksTemplateOverride'` from `services/cocoded`. | `services/cocoded/internal/httpapi/router.go`, `services/cocoded/internal/orchestrator/service.go` |
| PROMPT-09 | done | Add prompt output-contract schema tests. | Prompt examples and parser schema stay in sync; schema drift fails tests. | `go test ./internal/reviewprompt`; `go test ./internal/agentoutput -run 'TestParserKnownEnumsMatchPackageSchemas'` from `services/cocoded`. | `packages/prompts`, `services/cocoded/internal/agentoutput` |
| PROMPT-10 | done | Add review-relevant project-rules filtering guidance. | Agents are told how to use project rules without treating setup/style-only text as review truth. | `go test ./internal/reviewprompt` from `services/cocoded`. | `packages/prompts/review-agent.md`, `services/cocoded/internal/contextbundle` |

Checkpoint after Phase 2:

- Prompt snapshot tests pass.
- A local test review shows materially different prompts for security, performance, and migration reviewers.
- Findings include prompt version/hash provenance.

### Phase 3: Verification, Aggregation, And Evidence Quality

This phase makes “verified” mean more than “the file and line exist.”

| ID | Status | Task | Acceptance Criteria | Verification | Likely Files |
| --- | --- | --- | --- | --- | --- |
| VERIFY-01 | done | Gate curator-asserted verification status. | Curator may propose `verified`, but persisted status cannot exceed `locally_supported` unless deterministic anchor/content checks pass. | `go test ./internal/evidence ./internal/orchestrator` from `services/cocoded`. | `services/cocoded/internal/orchestrator/service.go`, `services/cocoded/internal/evidence/service.go` |
| VERIFY-02 | done | Require deterministic anchor validation for curated location overrides. | Curated primary location must point to changed code or be downgraded/rejected with provenance. | `go test ./internal/evidence ./internal/orchestrator` from `services/cocoded`. | `services/cocoded/internal/orchestrator/service.go` |
| VERIFY-03 | done | Add quoted-code matching for finding and evidence locations. | Evidence status accounts for whether quoted/observed code still matches file content. | `go test ./internal/evidence ./internal/orchestrator` from `services/cocoded`. | `services/cocoded/internal/evidence`, `services/cocoded/internal/orchestrator/verifier_agent.go` |
| VERIFY-04 | done | Make deterministic counter-evidence classification reachable. | Local verifier can classify concrete contradictions as counter-evidence and mark likely false positives when appropriate. | `go test ./internal/evidence ./internal/orchestrator` from `services/cocoded`. | `services/cocoded/internal/evidence/service.go` |
| VERIFY-05 | done | Add consensus confidence across distinct source agents. | Merged findings combine confidence from independent agents using a documented formula, not only a tiebreaker. | `go test ./internal/evidence ./internal/findingengine ./internal/orchestrator` from `services/cocoded`. | `services/cocoded/internal/findingengine`, `services/cocoded/internal/orchestrator` |
| VERIFY-06 | done | Preserve verifier disagreement explicitly. | Conflicting verifier outputs are stored/displayed instead of last-writer-wins. | `go test ./internal/evidence ./internal/findingengine ./internal/orchestrator` from `services/cocoded`. | `services/cocoded/internal/orchestrator/verifier_agent.go`, `services/cocoded/internal/evidence` |
| VERIFY-07 | done | Reuse prior dismissals across review sessions. | Stable fingerprints can suppress or downgrade previously dismissed findings in later sessions. | `go test ./internal/orchestrator -run 'TestWorkflowCarriesDismissedFindingFingerprintAcrossSessions'` from `services/cocoded`. | `services/cocoded/internal/db`, `services/cocoded/internal/findingengine`, `services/cocoded/internal/orchestrator` |
| VERIFY-08 | done | Handle partial agent failures without failing the whole review phase. | One reviewer transport failure records a warning and preserves successful agents unless policy requires fail-fast. | `go test ./internal/orchestrator -run 'TestWorkflowContinuesWhenOneAgentTransportOpenFails|TestWorkflowContinuesWhenOneAgentFails'` from `services/cocoded`. | `services/cocoded/internal/orchestrator/service.go` |
| VERIFY-09 | done | Preserve parsed output from timed-out runs when safe. | If an agent times out after emitting parseable findings, usable output is retained with timeout provenance. | `go test ./internal/orchestrator -run 'TestWorkflowKeepsParseableTimedOutAgentOutput|TestWorkflowAgentTimeoutKeepsOtherFindings'` from `services/cocoded`. | `services/cocoded/internal/agents`, `services/cocoded/internal/orchestrator` |
| VERIFY-10 | done | Decide whether `draft_comments` is real or removed from progress. | Phase either generates meaningful draft artifacts or no longer inflates progress as a no-op. | `go test ./internal/orchestrator -run 'TestDraftFindingCommentsCreatesMissingDrafts'` from `services/cocoded`. | `services/cocoded/internal/orchestrator/service.go` |

Checkpoint after Phase 3:

- Evidence map for a seeded false-positive candidate includes supporting and counter-evidence.
- “Verified” findings cite changed lines and matching code text.
- Duplicate and consensus behavior is visible in finding detail.

### Phase 4: Centralized Chat And Follow-Up Rework

This phase implements the architecture already sketched in `docs/centralized-chat/cocode_mvp_tdd_centralized_chat.md`.

| ID | Status | Task | Acceptance Criteria | Verification | Likely Files |
| --- | --- | --- | --- | --- | --- |
| CHAT-02 | done | Convert chat ask endpoint to async turn creation. | POST returns `202` with turn ID; work proceeds through persisted turn states. | `go test ./internal/chat`; `go test ./internal/httpapi -run 'TestReviewSessionChatThreadEndpointSeedsAndAnswers|TestReviewSessionChatThreadEndpointUsesOrchestratorResponder'` from `services/cocoded`. | `services/cocoded/internal/httpapi/chat.go`, `services/cocoded/internal/chat/service.go` |
| CHAT-03 | done | Add cancel endpoint and real cancellation handling. | Stop button marks turn `cancel_requested`/`canceled` and cancels active work where supported. | `go test ./internal/chat ./internal/httpapi -run 'TestCancelTurnMarksRequestAndRunExitsCanceled|TestReviewSessionChatThreadEndpointSeedsAndAnswers|TestReviewSessionChatThreadEndpointUsesOrchestratorResponder'`; `pnpm --filter @cocode/desktop typecheck` from repo root. | `services/cocoded/internal/httpapi/chat.go`, `services/cocoded/internal/chat/service.go`, `apps/desktop/src/renderer/src/app/chat/centralized-chat-screen.tsx`, `apps/desktop/src/renderer/src/lib/api.ts` |
| CHAT-04 | done | Build one shared context bundle per chat turn. | Multi-agent chat fan-out reuses one bundle instead of rebuilding/persisting per agent. | `go test ./internal/chat` from `services/cocoded`. | `services/cocoded/internal/chat/service.go`, `services/cocoded/internal/contextbundle` |
| CHAT-05 | done | Parallelize all-agent fan-out. | Asking all reviewers runs independent agent calls concurrently with bounded concurrency. | `go test ./internal/chat`; `go test ./internal/httpapi -run 'TestReviewSessionChatThreadEndpointSeedsAndAnswers|TestReviewSessionChatThreadEndpointUsesOrchestratorResponder'` from `services/cocoded`. | `services/cocoded/internal/chat/service.go` |
| CHAT-06 | todo | Add chat context caching by session/policy/snapshot. | Repeated follow-ups reuse compatible context artifacts where safe. | Cache key/unit tests. | `services/cocoded/internal/chat`, `services/cocoded/internal/contextbundle`, `services/cocoded/internal/db` |
| CHAT-07 | todo | Add thread summaries and compaction. | Long chats include compact summaries and recent turns instead of unbounded raw history. | Chat prompt context tests around summary threshold. | `services/cocoded/internal/chat` |
| CHAT-08 | todo | Add SSE `thread.message.delta` / turn events. | Renderer receives incremental updates without refetching the full thread on every event. | HTTP/SSE tests and desktop e2e. | `services/cocoded/internal/httpapi`, `apps/desktop/src/renderer/src` |
| CHAT-09 | todo | Debounce or remove full-thread refresh loops. | Chat UI no longer does high-frequency full refetches during streaming. | Playwright trace or e2e assertion. | `apps/desktop/src/renderer/src` |
| CHAT-10 | todo | Unify finding follow-ups into centralized chat. | Finding-scoped questions are normal central chat turns with finding/evidence context refs and prior history. | Follow-up e2e from finding detail and chat tab. | `services/cocoded/internal/followup`, `services/cocoded/internal/chat`, `apps/desktop/src/renderer/src` |
| CHAT-11 | todo | Include prior messages in finding-scoped context. | Follow-up answers are not cold-started unless explicitly requested. | Follow-up service test with prior turn dependency. | `services/cocoded/internal/followup/question.go`, `services/cocoded/internal/chat` |
| CHAT-12 | todo | Use `chat_message_context_refs` for review/finding/evidence/file context. | Context refs are persisted and used to build follow-up prompts instead of ad hoc prompt text only. | DB/API tests for message context refs. | `services/cocoded/internal/chat`, `services/cocoded/internal/db/sql/schema.sql` |
| CHAT-13 | todo | Make thread read APIs side-effect-free and efficient. | Loading a thread does not perform server-side read-modify-write cleanup or N+1 sync work. | HTTP API test plus query-count style unit/fake test where practical. | `services/cocoded/internal/chat/service.go`, `services/cocoded/internal/httpapi/chat.go` |
| CHAT-14 | todo | Replace client-faked SSE previews with persisted message deltas. | UI renders streamed content from persisted/delta events and does not depend on truncated preview reconstruction. | SSE/chat e2e with long answer over preview-size threshold. | `services/cocoded/internal/httpapi`, `services/cocoded/internal/chat`, `apps/desktop/src/renderer/src` |
| CHAT-15 | todo | Reconcile abandoned running turns on startup/load. | Client aborts or app restarts do not leave `chat_turns` permanently stuck in `running`. | Startup/reconcile service test. | `services/cocoded/internal/chat`, `services/cocoded/internal/app` |
| CHAT-16 | done | Define chat turn state machine. | Allowed transitions for `created`, `routing`, `context_building`, `running`, `synthesizing`, `completed`, `failed`, `cancel_requested`, and `canceled` are enforced. | `go test ./internal/chat` from `services/cocoded`. | `services/cocoded/internal/chat`, `services/cocoded/internal/db/sql/schema.sql` |

Checkpoint after Phase 4:

- Follow-up chat survives reload, supports cancel, streams deltas, and preserves message ordering.
- Asking all agents does not rebuild identical context N times.
- Finding follow-up and central chat share the same message timeline.

### Phase 5: Session Reuse And Adapter Evolution

This phase reduces latency/cost for capable adapters while preserving Cocode as the state owner.

| ID | Status | Task | Acceptance Criteria | Verification | Likely Files |
| --- | --- | --- | --- | --- | --- |
| ADAPT-01 | todo | Remove centralized-chat hard rejection of non-CLI adapters once adapter contracts are ready. | Chat can route to compatible App Server/ACP adapters behind capability checks. | Adapter capability tests. | `services/cocoded/internal/chat/service.go`, `services/cocoded/internal/agents` |
| ADAPT-02 | todo | Implement Codex App Server session reuse for follow-ups. | Follow-ups can resume an existing thread/session when the selected adapter supports it. | Fake app-server adapter test plus manual local run. | `services/cocoded/internal/agents`, `services/cocoded/internal/chat` |
| ADAPT-03 | todo | Implement ACP session reuse where available. | ACP adapters can load/resume sessions for follow-up context. | ACP adapter test/fake driver. | `services/cocoded/internal/agents`, `services/cocoded/internal/chat` |
| ADAPT-04 | todo | Add routing rule: session resume vs fresh cheap call. | Explain-only turns can use compact finding slice; investigation turns can resume agent session. | Chat router tests. | `services/cocoded/internal/chat` |
| ADAPT-05 | todo | Replace disabled App Server stub with explicit capability/health state. | UI/backend can distinguish unsupported, disabled, unhealthy, and ready adapters. | Adapter health tests and settings UI smoke. | `services/cocoded/internal/agents`, `services/cocoded/internal/httpapi/agent_configs.go`, `apps/desktop/src/renderer/src` |
| ADAPT-06 | todo | Store external session metadata per adapter/run/turn. | Session reuse has durable IDs, expiry, provenance, and invalidation behavior. | DB and adapter session tests. | `services/cocoded/internal/db`, `services/cocoded/internal/agents`, `services/cocoded/internal/chat` |

Checkpoint after Phase 5:

- Compatible adapters reuse session context for investigation follow-ups.
- CLI adapters still work with the async/cached fallback path.

### Phase 6: Read-Only Safety Hardening

This phase is part of the first implementation batch. It closes the gap where review mode is read-only by instruction but not consistently enforced by adapter runtime settings.

| ID | Status | Task | Acceptance Criteria | Verification | Likely Files |
| --- | --- | --- | --- | --- | --- |
| SAFE-01 | done | Replace instruction-only read-only mode with enforced read-only execution. | Review-mode agents cannot write to the working tree even if prompted by untrusted repo content. | `go test ./internal/agents ./internal/agentpreset ./internal/agentrun ./internal/db ./internal/httpapi -run 'ReviewMode|CodexCLI|Antigravity|JSONRPC|PromotesDefaultCodex|AgentPresets'` from `services/cocoded`. | `services/cocoded/internal/agents`, `services/cocoded/internal/agentpreset/presets.go` |
| SAFE-02 | todo | Evaluate ephemeral worktree execution for adapters that need filesystem access. | Agents can inspect code in an isolated copy without mutating the user workspace. | Integration test with temporary worktree cleanup. | `services/cocoded/internal/agents`, `services/cocoded/internal/git` |
| SAFE-03 | done | Align Codex App Server sandbox defaults with review mode. | App Server review sessions do not default to `workspace-write` when the task is read-only. | `go test ./internal/agents ./internal/agentpreset ./internal/agentrun ./internal/db ./internal/httpapi -run 'ReviewMode|CodexCLI|Antigravity|JSONRPC|PromotesDefaultCodex|AgentPresets'` from `services/cocoded`. | `services/cocoded/internal/agents/jsonrpc_stdio.go` |

Checkpoint after Phase 6:

- Prompt-injection fixture cannot cause a workspace write during review.
- Workspace diff remains clean after hostile review fixture.

### Phase 7: Observability, Evaluation, And Trust Metrics

The review points to Cloudflare/DoorDash-style production signals: accepted findings, duplicate rate, suppression rate, cache hit rate, latency, and cost. Cocode already has local eval concepts; this phase makes them part of the orchestration plan before CI expansion.

| ID | Status | Task | Acceptance Criteria | Verification | Likely Files |
| --- | --- | --- | --- | --- | --- |
| OBS-01 | todo | Emit review orchestration metrics for phase latency and outcomes. | Each phase records duration, status, failure reason, and finding counts. | Metrics unit tests or structured event assertions. | `services/cocoded/internal/orchestrator`, `services/cocoded/internal/httpapi/audit_log.go` |
| OBS-02 | todo | Track token/context/cache efficiency. | Agent runs record context size, prompt hash, cache hit/miss where available, and output size. | Agent run metadata tests. | `services/cocoded/internal/agents`, `services/cocoded/internal/contextbundle`, `services/cocoded/internal/chat` |
| OBS-03 | todo | Track trust metrics from human decisions. | Accepted, dismissed, suppressed, not-actionable, duplicate, stale, and publishable outcomes are queryable per session/repo. | Eval harness tests and audit API tests. | `services/cocoded/internal/evalharness`, `services/cocoded/internal/httpapi/findings.go` |
| OBS-04 | todo | Add dogfood eval gates for prompt/orchestration changes. | A small local eval suite can compare precision-ish, false positives, accepted expected findings, and suppression rate before/after changes. | `go test ./internal/evalharness` plus documented eval command. | `services/cocoded/internal/evalharness`, `testdata` |
| OBS-05 | todo | Add duplicate/noise regression tracking. | Multi-agent duplicate rate is measured so role overlays can be evaluated against the "N copies of same agent" failure mode. | Eval fixture with duplicate candidate clusters. | `services/cocoded/internal/findingengine`, `services/cocoded/internal/evalharness` |

Checkpoint after Phase 7:

- A local dogfood review can report latency, candidate count, dedupe rate, verified count, accepted/dismissed decisions, and duplicate/noise rate.
- Prompt and orchestration changes can be compared with before/after eval reports.

## Parallelization Plan

Safe to parallelize:

- `ORCH-01`, `CTX-01`, and `CTX-02`.
- Prompt snapshot work (`PROMPT-01` to `PROMPT-04`) after agreeing on prompt file ownership.
- Evidence verification tasks (`VERIFY-03` to `VERIFY-06`) once deterministic status gates are defined.
- Frontend SSE/render work after `CHAT-02` and `CHAT-08` API contracts are drafted.
- Observability/eval tasks after phase events and finding decision states are stable.

Must be sequential:

- `ORCH-02` before `ORCH-03` if candidate/finding writes share transaction patterns.
- `PROMPT-01` before prompt provenance and role overlays.
- `CHAT-02` before cancellation, deltas, and UI follow-up changes.
- `ADAPT-*` after async chat contracts are stable.

## Verification Command Set

Backend focused checks:

```bash
cd services/cocoded
go test ./internal/orchestrator ./internal/findingengine ./internal/evidence ./internal/chat ./internal/contextbundle ./internal/projectrules ./internal/httpapi
```

Desktop checks:

```bash
pnpm --filter @cocode/desktop typecheck
pnpm --filter @cocode/desktop build
pnpm --filter @cocode/desktop exec playwright test -c playwright.config.ts e2e/app-smoke.spec.ts
```

End-to-end manual checks:

- Start a review with at least two different reviewer roles and confirm prompts differ.
- Force one agent failure and confirm successful agents still produce review state.
- Ask a follow-up, switch tabs, reload the app, and confirm the answer remains in chat.
- Open a finding and confirm evidence map includes changed-code anchor, supporting call sites, and counter-evidence when present.

## Review Coverage Matrix

| Audit Item | Covered By | Status | Notes |
| --- | --- | --- | --- |
| Architecture is fundamentally sound and should stay deterministic/checkpointed. | `BASE-01`, `BASE-03` | done | Protects current backbone instead of replacing it. |
| Parsing is robust and should be preserved. | `BASE-02` | done | Regression tests cover tolerant parsing and raw artifacts. |
| Cocode owns state; CLIs are workers. | `BASE-03`, `CHAT-12`, `ADAPT-06` | doing | Local-first contract is documented; structured chat refs and external session metadata remain open. |
| H1: Roles/presets never reach prompts. | `PROMPT-05` | done | Selected role overlays alter prompts and prompt hashes. |
| H2: `blocker` severity gets worst verification priority. | `ORCH-01` | done | Fixed in verifier prioritization and curator candidate scoring. |
| H3: Resume after mid-phase crash can permanently fail session. | `ORCH-02`, `ORCH-03` | done | Candidate, finding, and candidate-link writes are idempotent across retries. |
| H4: Chat answers are deleted from persistence. | `CHAT-01` | done | `role=chat` agent messages now survive hidden workflow cleanup. |
| H5: Curator can mint verified status without deterministic backstop. | `VERIFY-01`, `VERIFY-02` | done | Curator primary anchors now pass changed-hunk/source-line validation or downgrade/fallback with provenance. |
| Missing output enums and examples. | `PROMPT-03` | done | Prompt includes exact severity/category enum and JSON-only rule. |
| Missing severity rubric, stop conditions, finding cap. | `PROMPT-04` | done | Rubric and caps reduce noisy findings and overclaiming. |
| Prompt sprawl and drift. | `PROMPT-01`, `PROMPT-02`, `PROMPT-07` | done | Single source, version/hash, artifacts, and shared injection-defense wording are now covered by prompt tests. |
| `PromptTemplate` override not wired/traceable. | `PROMPT-02`, `PROMPT-08` | done | Override behavior is traced in prompt hash/provenance. |
| Prompt examples and parser schema can drift. | `PROMPT-09` | done | Schema/prompt consistency tests guard drift. |
| Chat prompt omits full injection-defense guidance. | `PROMPT-07` | done | Centralized chat consumes the shared untrusted-context instruction and has prompt coverage. |
| Project rules dropped at default depth. | `CTX-01` | done | Standard reviews include requested rules. |
| Rule discovery misses common instruction files. | `CTX-02` | done | Adds `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md`, `.cursor/rules`. |
| Project rules may be noisy if used naively. | `PROMPT-10` | done | Reviewers are told how to treat project rules as guidance rather than review truth. |
| Read-only is instruction-only. | `SAFE-01`, `SAFE-02`, `SAFE-03` | doing | `SAFE-01` and `SAFE-03` are complete; disposable worktree isolation remains open as `SAFE-02`. |
| Verified means file/line exists, not code content match. | `VERIFY-03` | done | Local verifier checks matchable quoted code against the cited source window and flags stale observations. |
| Deterministic counter-evidence path is unreachable/weak. | `VERIFY-04` | done | Direct counter-evidence heuristics can now produce `likely_false_positive` without an LLM verifier. |
| No consensus signal across agents. | `VERIFY-05` | done | Distinct-agent confidence is combined using a simple independence-style formula. |
| Dismissals do not survive re-review. | `VERIFY-07` | done | Prior repository-scoped dismissals carry forward by stable fingerprint with event provenance. |
| One agent transport error can fail phase. | `VERIFY-08` | done | Transport-open failure fixture preserves successful agents and emits partial-failure warning. |
| Parsed output from timed-out runs is discarded. | `VERIFY-09` | done | Parseable timed-out output can create candidates with timeout provenance. |
| Verifier disagreement is last-writer-wins. | `VERIFY-06` | done | Conflicting verifier outputs now emit explicit disagreement evidence and reconcile conservatively. |
| `draft_comments` is checkpointed no-op. | `VERIFY-10` | done | The phase now fills missing finding draft comments and emits preparation events. |
| Chat POST is synchronous/blocking. | `CHAT-02` | done | POST now creates a turn, returns `202`, and runs the turn from persisted state in the background. |
| Chat has no real cancel. | `CHAT-03` | done | Stop button calls a cancel endpoint; backend marks cancel state and cancels matching active chat agent runs. |
| Chat rebuilds context per agent. | `CHAT-04`, `CHAT-06` | doing | All-agent fan-out now shares one bundle; durable cache key remains open. |
| Chat fan-out is serial. | `CHAT-05` | done | All-agent fan-out uses bounded parallel execution. |
| Chat sends very large prompt/context every turn. | `CHAT-06`, `CHAT-07`, `ADAPT-04` | todo | Cache, summarize, and route to compact finding context when enough. |
| Chat turn states exist but are underused. | `CHAT-02`, `CHAT-16` | done | Turn creation/execution now uses and validates the persisted state machine. |
| `chat_message_context_refs` exists but is unused. | `CHAT-12` | todo | Uses structured refs for finding/evidence/file context. |
| Chat answer survives only via client-side preview reconstruction. | `CHAT-01`, `CHAT-14` | todo | Persist full answer and stream durable deltas. |
| Renderer refetches full thread too often. | `CHAT-08`, `CHAT-09` | todo | SSE deltas and UI debounce. |
| Thread GET does cleanup/sync work and can cause N+1 behavior. | `CHAT-13` | todo | Makes reads cheap and side-effect-free. |
| Client abort leaves turns stuck running. | `CHAT-03`, `CHAT-15` | todo | Adds cancel and reconcile paths. |
| Finding follow-ups are separate/cold. | `CHAT-10`, `CHAT-11` | todo | Unified central thread with history/context refs. |
| App Server/ACP session reuse unavailable for chat. | `ADAPT-01`, `ADAPT-02`, `ADAPT-03`, `ADAPT-04` | todo | Structural follow-up efficiency work. |
| App Server adapter is a disabled stub / adapter readiness is unclear. | `ADAPT-05` | todo | Capability and health state should be explicit. |
| External session IDs are not durable enough for reuse. | `ADAPT-06` | todo | Adds durable metadata and invalidation rules. |
| Need Cloudflare/DoorDash-style trust and efficiency metrics. | `OBS-01`, `OBS-02`, `OBS-03`, `OBS-04`, `OBS-05` | todo | Tracks quality, latency, token/cache, and duplicate/noise outcomes. |
| CI design should build on local contracts, not fork them. | `BASE-03`, `OBS-01` | doing | Local-first contract is documented; phase metrics remain open. |

## Initial Implementation Batch

Recommended first batch:

1. `ORCH-01`: severity ranking.
2. `CHAT-01`: preserve chat answers.
3. `SAFE-01`, `SAFE-02`, and `SAFE-03`: enforce read-only review execution.
4. `CTX-01` and `CTX-02`: include and discover project rules.
5. `ORCH-02` and `ORCH-03`: idempotent normalize/dedupe writes.

Why this batch:

- It fixes silent correctness issues before prompt and UI changes.
- It has focused tests and low frontend blast radius.
- It prepares the workflow for safer resume/retry behavior before async chat and session reuse.
- It makes prompt-injection safety an enforced runtime property, not just prompt text.

## Decision Notes

1. Prompt contract checks currently live in Go tests against the embedded prompt source and parser schemas; add package-level prompt tests only if non-Go prompt tooling becomes authoritative.
2. Cross-session dismissal memory is repository-scoped by stable finding fingerprint. Branch or base/head lineage can be added later if false carryover appears in dogfood data.
3. `draft_comments` remains a dedicated workflow phase and now fills missing finding draft comments deterministically; GitHub preview and copy packets remain separate product surfaces.

## Open Questions

1. Which adapter should be first for durable session reuse: Codex App Server or ACP?
2. What minimum dogfood eval set should gate prompt/orchestration changes before CI rollout?
