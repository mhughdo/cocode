# cocode MVP Task Breakdown

**Product:** cocode  
**Document type:** Implementation task breakdown  
**Scope:** MVP implementation with non-interactive CLI agents, Evidence Map, copy fix packets, GitHub publishing, Electron + Go/Gin + SQLite  
**Status baseline:** Tasks are planned unless explicitly marked done  
**Last updated:** 2026-05-04

---

## 0. How to Use This Document

Each task includes:

- **Task ID**: Stable identifier for dependency tracking.
- **Task name**: Short implementation name.
- **Description**: What must be built.
- **Status**: `Done`, `Not started`, `In progress`, `Blocked`, or `Deferred`.
- **Dependencies**: Task IDs that must finish first.
- **Parallelization**: What can be worked on at the same time.
- **Done criteria**: Concrete criteria for the task to be considered complete.

---

## 1. Global Definition of Done

A task is considered **done** only when all applicable criteria are satisfied:

1. **Behavior works**: The user-visible, API-visible, or system-visible behavior works in the intended flow.
2. **State is persisted**: Required DB rows, artifacts, events, or settings are saved correctly.
3. **Events are emitted**: Review-flow features emit events where expected.
4. **Errors are handled**: At least one relevant failure path is handled with typed errors and user-readable messages.
5. **Tests exist**: Unit, integration, E2E, or contract tests exist at the appropriate level.
6. **Security rules are respected**: Local auth, path sandboxing, secret redaction, and permission boundaries are not bypassed.
7. **Docs/comments exist**: Non-obvious behavior is documented in code comments, README, or docs.
8. **Integration is complete**: The task works with adjacent modules, not just in isolation.
9. **No hidden side effects**: The feature does not publish, modify files, or run risky commands without explicit user approval.

---

## 2. Critical Path

The MVP critical path is:

```text
T001-T003 docs
-> T010-T018 repo/app skeleton
-> T030-T039 backend skeleton
-> T050-T060 DB/artifacts
-> T070-T082 git/PR ingestion
-> T090-T107 CLI runtime
-> T140-T153 review orchestration/events
-> T170-T186 finding engine
-> T200-T218 verification/evidence/evidence map
-> T240-T266 core UI
-> T280-T299 copy/publish
-> T360-T390 tests/package
```

Large parallel workstreams:

| Workstream | Can start after | Notes |
|---|---|---|
| Frontend shell | T010, T014 | Can progress with mock APIs. |
| Backend DB/storage | T030 | Can progress before UI. |
| Git/PR ingestion | T050 | Independent of agent runtime. |
| Agent CLI runtime | T050 | Can use fake snapshots first. |
| Finding UI | T170 API contracts | Can use seeded data. |
| Evidence Map UI | T200 API contract | Can use mock graph JSON. |
| GitHub publish | T070, T280 | Can use fake GitHub server. |
| Tests/evals | Start early | Golden repos should be built before feature work finishes. |

---

## 3. Task Status Legend

| Status | Meaning |
|---|---|
| Done | Complete under done criteria. |
| Not started | No implementation started. |
| In progress | Active implementation. |
| Blocked | Cannot proceed due to dependency or decision. |
| Deferred | Intentionally pushed outside MVP. |

---

## 4. Documentation and Planning Tasks

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T001 | Update PRD | Produce updated PRD reflecting cocode, CLI-only MVP, Evidence Map, copy packets, and latest mockups. | Done | None | T002, T003 | PRD exists as Markdown; includes user stories, functional requirements, out of scope, testing, and source notes. |
| T002 | Update TDD | Produce comprehensive TDD with architecture, schemas, APIs, prompts, adapter strategy, Evidence Map, security, and tests. | Done | T001 | T003 | TDD exists as Markdown; includes Go/Gin/Electron structure, SQL schema, prompts, and future adapter plan. |
| T003 | Create task breakdown | Produce complete task breakdown with dependencies and done criteria. | Done | T001, T002 | None | This document exists as Markdown and every task has status, dependencies, and done criteria. |
| T004 | Define MVP release checklist | Create a concise release checklist derived from PRD/TDD. | Done | T001, T002, T003 | T010 | `docs/mvp_release_checklist.md` covers features, tests, security, packaging, and dogfood criteria. |
| T005 | Define risk register | Track top technical/product/security risks and mitigations. | Done | T001, T002 | T010 | `docs/risk_register.md` includes owner, severity, mitigation, review cadence, and status. |
| T006 | Define glossary | Define canonical terms: thread, review session, finding, evidence bundle, Evidence Map, adapter, connection driver, copy packet. | Done | T001 | T010 | `docs/glossary.md` defines canonical UI/API terms used by implementation tasks. |

---

## 5. Repository and Tooling Foundation

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T010 | Initialize monorepo | Create repository structure for desktop app, Go service, packages, docs, and testdata. | Done | T001 | T011, T030 | Repo builds empty projects; README explains structure; lint ignores generated/build files. |
| T011 | Configure package manager | Choose and configure pnpm/npm workspace for Electron/React packages. | Done | T010 | T012 | Install works on clean machine; lockfile committed; scripts documented. |
| T012 | Configure Go workspace | Initialize Go module for `cocoded` backend. | Done | T010 | T011, T030 | `go test ./...` runs; module naming is stable; tool versions documented. |
| T013 | Add shared schemas package | Create package for JSON schemas shared by backend/tests/docs. | Done | T010 | T050, T090 | Schemas are versioned and can be imported by tests/build scripts. |
| T014 | Configure Electron/Vite app | Create Electron main, preload, and renderer build pipeline. | Done | T010, T011 | T240 | Dev mode launches an empty window; production build creates app bundle. |
| T015 | Configure TypeScript strictness | Enable strict TS settings and path aliases. | Done | T014 | T241 | `tsc --noEmit` passes; aliases documented. |
| T016 | Configure Tailwind/shadcn | Install and configure shadcn/ui and Tailwind theme tokens. | Done | T014 | T242 | Base components render; theme matches mockup direction; dark-mode decision documented. |
| T017 | Configure Go lint/format/test scripts | Add `gofmt`, `go test`, vet/staticcheck if chosen. | Done | T012 | T030 | CI/local scripts run consistently; failures are clear. |
| T018 | Configure frontend lint/format/test scripts | Add ESLint/Prettier/TS test script. | Done | T014, T015 | T240 | Lint and format scripts run; rules do not fight shadcn patterns. |
| T019 | Add commit hooks | Add optional pre-commit hooks for formatting/linting. | Done | T017, T018 | T020 | Hooks are documented and can be bypassed for emergencies. |
| T020 | Add CI skeleton | Add CI workflow for backend tests, frontend typecheck, lint, and build smoke. | Done | T017, T018 | T360 | CI runs on PR; caches dependencies; failures are actionable. |

---

## 6. Electron Main/Preload Foundation

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T030 | Implement Electron main app lifecycle | Create window lifecycle, app ready, quit, crash logging. | Done | T014 | T031, T240 | App launches reliably; main process logs startup/shutdown. |
| T031 | Launch Go backend from Electron | Bundle/launch `cocoded` from main process in dev and prod modes. | Done | T030, T040 | T032 | Backend starts on app launch; port/token returned to renderer; shutdown kills process. |
| T032 | Generate local backend auth token | Generate per-launch high-entropy token and pass to renderer safely. | Done | T031 | T033, T104 | All renderer API calls include token; backend rejects missing/invalid token. |
| T033 | Implement preload API | Expose narrow `window.cocode` API for backend info, clipboard, repo picker, editor open. | Done | T030 | T034, T242 | Renderer cannot access raw Node/Electron APIs; IPC inputs are validated. |
| T034 | Implement clipboard bridge | Add secure clipboard write path for copy packets/comments. | Done | T033 | T290 | Copy works from renderer through main; oversized copy attempts are guarded/logged. |
| T035 | Implement repository picker | Add file dialog to select local repo directory. | Done | T033 | T070, T245 | User can select folder; result is passed to renderer; cancellation is handled. |
| T036 | Implement open external editor bridge | Open file/line in configured editor or OS fallback. | Done | T033 | T213, T263 | Works for supported editor command; unsupported editor shows clear error. |
| T037 | Add safe secret storage abstraction | Implement Electron-side secret storage wrapper. | Done | T030 | T330 | Can store/retrieve/delete test secret; does not expose secret to renderer. |
| T038 | Add crash/error log location | Define app log path and expose “open logs” action later. | Done | T030 | T360 | Main/backend logs are written to predictable local paths. |
| T039 | Harden Electron security defaults | Disable nodeIntegration, enable contextIsolation and sandbox, set CSP. | Done | T030 | T240, T330 | Security checklist passes; no raw ipcRenderer exposure. |

---

## 7. Go Backend Foundation

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T040 | Create Go backend entrypoint | Implement `cocoded` startup with config, logger, signal handling. | Done | T012 | T041, T050 | Binary starts locally; logs health info; graceful shutdown works. |
| T041 | Configure Gin router | Add base Gin router with middleware stack. | Done | T040 | T042 | `/api/health` returns success; request IDs and recovery middleware work. |
| T042 | Implement local auth middleware | Require per-launch auth token for all API routes except health/version if chosen. | Done | T032, T041 | T330 | Requests without token are rejected; tests cover success/failure. |
| T043 | Add response envelope | Standardize `{data,error,request_id}` responses. | Done | T041 | T044 | All handlers use envelope; errors are typed. |
| T044 | Add typed error package | Define error codes and mapping to HTTP status. | Done | T043 | All backend tasks | Tests cover representative error mappings. |
| T045 | Add backend config loader | Load app paths, DB path, artifact path, debug flags. | Done | T040 | T050 | Config works in dev/prod; invalid config fails clearly. |
| T046 | Add SSE helper | Implement reusable SSE stream handler with event IDs. | Done | T041 | T145 | SSE streams events; reconnect can resume from Last-Event-ID if implemented. |
| T047 | Add structured logger | Add JSON/dev logging with request ID and session ID fields. | Done | T040 | All backend tasks | Logs include request/session/agent fields where available. |
| T048 | Add backend version endpoint | Return build version, platform, DB path presence, and feature flags. | Done | T041 | T031 | Renderer can display backend version; tests cover endpoint. |

---

## 8. Database and Artifact Storage

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T050 | Add SQLite connection manager | Open DB, set WAL/foreign keys/busy timeout, close cleanly. | Done | T045 | T051 | DB opens in dev/prod path; pragmas applied; tests use temp DB. |
| T051 | Add migration runner | Implement migrations with version tracking. | Done | T050 | T052 | Fresh DB migrates; repeated migration is idempotent; migration failure is reported. |
| T052 | Implement schema v1 | Add core tables from TDD: workspaces, snapshots, sessions, agents, findings, evidence, graph, packets, publications. | Done | T051 | T053 | Schema applies cleanly; foreign keys enforced; indexes created. |
| T053 | Configure sqlc | Add sqlc config and initial query files. | Done | T052 | T054 | Generated Go code compiles; queries are type-safe. |
| T054 | Implement Workspace queries | CRUD/list workspace queries. | Done | T053 | T070 | Unit tests cover create/get/list/update. |
| T055 | Implement Snapshot queries | CRUD PR snapshot and changed-file queries. | Done | T053 | T075 | Tests cover snapshot creation and changed file lookup. |
| T056 | Implement ReviewSession queries | CRUD review sessions and status updates. | Done | T053 | T140 | Tests cover state transitions. |
| T057 | Implement Agent queries | CRUD agent configs/session agents/runs. | Done | T053 | T090 | Tests cover agent config and run lifecycle. |
| T058 | Implement Finding queries | CRUD candidates/findings/links/decisions. | Done | T053 | T170 | Tests cover candidate->finding relationships. |
| T059 | Implement Evidence Graph queries | CRUD evidence items, graphs, nodes, edges, call paths. | Done | T053 | T200 | Tests cover graph creation and retrieval. |
| T060 | Implement Artifact store | Store artifact files under app/workspace directory with metadata rows. | Done | T050, T052 | T090, T120 | Artifact save/read/delete works; hash/size stored; path traversal blocked. |
| T061 | Implement event store | Append events with monotonic per-session sequence. | Done | T056 | T145 | Events persist in order; duplicate sequence impossible; tests cover append/list. |
| T062 | Add FTS search tables | Add FTS for findings/evidence search and sync helpers. | Done | T052 | T255 | Search returns seeded findings; updates after finding changes. |
| T063 | Add DB backup/export dev command | Add developer command to dump local DB and artifacts metadata. | Done | T050 | T360 | Command runs safely and redacts secrets. |
| T064 | Add seeded dev data | Create seed script for UI development with sample sessions/findings/evidence maps. | Done | T052, T060 | T240 | Frontend can run against seeded data without agents. |

---

## 9. Git and PR Ingestion

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T070 | Implement git repo validation | Validate selected path is a git repository and get root path. | Done | T054, T035 | T071 | Invalid repo shows typed error; valid repo creates workspace/repository. |
| T071 | Implement git command wrapper | Add safe wrapper for git commands with timeout/cwd/output limit. | Done | T070 | T072 | Tests use fake repo; command errors map to typed errors. |
| T072 | Parse GitHub PR URL | Extract owner, repo, and PR number from supported URL shapes. | Done | T044 | T073 | Tests cover valid/invalid GitHub URLs. |
| T073 | Implement GitHub auth config | Store/retrieve GitHub token reference and validate access. | Done | T037, T044 | T074 | Token is not stored in DB plaintext; missing token error is clear. |
| T074 | Fetch GitHub PR metadata | Fetch title, author, base/head refs, SHAs, and PR URL. | Done | T072, T073 | T075 | Integration test with fake GitHub server. |
| T075 | Fetch changed files from GitHub | Fetch changed file list, additions/deletions/status/patches. | Done | T074, T055 | T076 | Snapshot stores changed files and patches. |
| T076 | Fetch unified diff | Fetch PR diff artifact for diff mapping and reproducibility. | Done | T074, T060 | T079 | Diff stored as artifact; SHA metadata attached. |
| T077 | Implement local branch compare | Use git to compute diff between base/head branches. | Done | T071, T055 | T079 | Snapshot created from branch compare; tests with fixture repo. |
| T078 | Implement local changes snapshot | Use git diff for working tree/uncommitted changes. | Done | T071, T055 | T079 | Snapshot created; binary/generated files handled. |
| T079 | Implement diff parser | Parse unified diff into files, hunks, line mappings, changed line ranges. | Done | T076 or T077 | T170, T294 | Tests cover add/delete/rename/multiple hunks. |
| T080 | Detect generated/binary/excluded files | Identify generated/lock/vendor/binary files and support exclusion. | Done | T079 | T246 | UI receives flags; exclusions are persisted. |
| T081 | Implement changed-file API | Return changed file list with line ranges and exclusion flags. | Done | T055, T079 | T246 | API powers Configure Review changed-files card. |
| T082 | Implement snapshot creation API | Create snapshots from GitHub URL/local compare/local changes. | Done | T074-T081 | T245 | End-to-end API creates snapshot and returns ID. |
| T083 | Implement PR previous comments fetch | Fetch existing review comments/timeline comments for duplicate avoidance. | Done | T074 | T120, T299 | Comments stored as artifact/context items; optional if auth unavailable. |
| T084 | Implement CODEOWNERS/project rules discovery | Detect CODEOWNERS and common config files. | Done | T070 | T120 | Rules files discovered and added to context candidates. |
| T085 | Add PR ingestion UI smoke test | Simulate PR URL and verify snapshot creation flow. | Not started | T082, T245 | T360 | E2E test covers happy path and invalid URL. |

---

## 10. Agent Runtime and CLI Adapters

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T090 | Define AgentAdapter interfaces | Implement Go interfaces for AgentAdapter, ConnectionDriver, AgentTask, AgentEvent. | Done | T057, T013 | T091 | Interfaces compile; no workflow code depends directly on CLI implementation. |
| T091 | Define adapter capability model | Add capabilities such as supports_json, supports_streaming, supports_sessions, can_write, can_read. | Done | T090 | T092 | Capabilities are stored and returned by API. |
| T092 | Implement agent config API | CRUD agent configs and health test endpoint. | Done | T057, T090 | T248 | API tests cover create/update/test/delete. |
| T093 | Implement CommandOnceDriver | Spawn one process per task using exec with context cancellation. | Done | T090 | T094 | Can run fake CLI; cancel/timeout works; no shell by default. |
| T094 | Implement prompt delivery modes | Support prompt via stdin, arg, or temp file. | Done | T093, T060 | T095 | Tests cover all delivery modes and temp cleanup. |
| T095 | Implement stdout/stderr capture | Capture outputs separately with size limits and artifacts. | Done | T093, T060 | T096 | Raw artifacts stored; truncation marked in metadata. |
| T096 | Implement output parsers | Parse JSON, JSONL/NDJSON, and text fallback. | Done | T095, T013 | T170 | Parser tests cover valid/invalid/mixed outputs. |
| T097 | Implement CLI health check | Validate command exists, version if possible, and auth smoke prompt if enabled. | Done | T092, T093 | T248 | Health endpoint reports installed/missing/error states. |
| T098 | Implement agent run persistence | Create/update agent_runs around CLI execution. | Done | T057, T093 | T140 | Agent run rows include status, duration, artifacts, errors. |
| T099 | Implement CLI timeout policy | Enforce per-agent and per-review runtime limits. | Done | T093, T098 | T151 | Timeout results in typed error and preserved logs. |
| T100 | Implement single-agent cancellation | Cancel one running CLI without canceling session. | Done | T093, T098 | T151 | UI/API can cancel agent; process exits; session continues. |
| T101 | Implement bounded concurrency | Limit concurrent CLI processes per session/system. | Done | T098 | T147 | Concurrency limit tested with fake slow agents. |
| T102 | Implement fake JSON agent | Test fixture CLI that emits valid finding JSON. | Done | T093 | T170, T360 | Fixture documented and used in tests. |
| T103 | Implement fake malformed agent | Test fixture CLI that emits malformed output. | Done | T093 | T178, T360 | Fixture used for repair/error tests. |
| T104 | Add Codex CLI preset | Add configurable preset for Codex CLI non-interactive usage. | Done | T092, T093 | T105-T107 | Preset appears in UI; health check can validate command if installed. |
| T105 | Add Claude Code CLI preset | Add configurable preset for Claude `-p`/JSON mode. | Done | T092, T093 | T104,T106 | Preset appears in UI; docs explain required local auth. |
| T106 | Add Gemini CLI preset | Add configurable generic CLI preset for Gemini non-interactive mode; note future ACP. | Done | T092, T093 | T104,T105 | Preset appears; no ACP assumption in MVP. |
| T107 | Add custom CLI preset | User can create arbitrary CLI adapter config. | Done | T092 | T248 | Custom command can be saved, health checked, and used in review. |
| T107A | Add OpenCode CLI preset | Add configurable preset for OpenCode `run --format json` non-interactive usage. | Done | T092, T093 | T248 | Preset appears in UI; health check can validate local `opencode` if installed. |
| T108 | Add JSON-RPC stdio skeleton | Create disabled/future connection driver skeleton for Codex App Server/ACP. | Done | T090 | T109 | Interfaces compile; no MVP feature depends on it; documented as future. |
| T109 | Add Codex App Server adapter stub | Add feature-flagged stub mapping future Codex app-server events to AgentEvents. | Done | T108 | T110 | Stub is disabled; tests verify unsupported feature returns clear error. |
| T110 | Add ACP adapter stub | Add feature-flagged stub for future ACP agent integration. | Done | T108 | T111 | Stub is disabled; code structure is clear. |
| T111 | Document adapter extension guide | Write developer docs for adding new CLI or protocol adapter. | Done | T090-T110 | T006 | Guide includes config, parser, health check, tests, and security rules. |

---

## 11. Context Builder

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T120 | Define ContextBundle model | Implement domain/API models for bundles and items. | Done | T058, T060 | T121 | Model maps to DB; artifacts can store rendered bundle. |
| T121 | Build diff context items | Convert changed file hunks into context items. | Done | T079, T120 | T122 | Context includes file/path/line/content; tests cover multiple hunks. |
| T122 | Add changed-file full/slice context | Include full small files or slices around changed lines. | Done | T071, T121 | T123 | Respects size/token budgets and exclusions. |
| T123 | Add related call-site search | Use ripgrep/code search to find references to changed symbols/paths. | Done | T071, T121 | T124 | Returns bounded related items with file/line refs. |
| T124 | Add related test discovery | Find likely tests by path/name/import references. | Done | T071, T121 | T125 | Test files added when found; absence recorded. |
| T125 | Add project convention discovery | Include CODEOWNERS, lint/config, README snippets, package/build config. | Done | T084, T120 | T126 | Project rules context is bounded and auditable. |
| T126 | Add prior comments context | Include previous PR comments when available. | Done | T083, T120 | T127 | Duplicate avoidance context stored as items. |
| T127 | Add prior decision memory | Include prior dismissals/rules from review_rules. | Done | T058, T120 | T128 | Dismissal memory can be queried and included. |
| T128 | Add token estimation | Estimate tokens per item and bundle. | Done | T120 | T129 | Estimates appear in API/UI; tests cover known samples. |
| T129 | Add context budgeter | Select/drop items according to review depth and budget. | Done | T128 | T130 | Budgeting is deterministic and records dropped reasons. |
| T130 | Add secret redaction | Redact token-like strings, private keys, env values before cloud-backed CLI context. | Done | T120 | T333 | Tests cover secret fixtures; redaction report artifact created. |
| T131 | Store rendered context bundle artifacts | Persist final prompt context sent to each agent. | Done | T120, T060 | T140 | Agent run links to bundle; user can audit context. |
| T132 | Build review context API | Endpoint/service to build and preview context for a session. | Done | T120-T131 | T247 | API returns bundle metadata, items, token estimate. |
| T133 | Build finding-scoped context | Build context for one finding and evidence bundle. | Done | T186, T204 | T280 | Follow-up uses finding-scoped bundle by default. |
| T134 | Build Evidence Map context | Build graph-specific context from finding/evidence/code relationships. | Done | T133 | T210 | Evidence Map builder receives bounded relevant context. |
| T135 | Add context debug viewer API | Return context items and artifacts for developer/provenance panel. | Done | T131 | T260 | UI can show what each agent saw. |

---

## 12. Review Orchestration and Events

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T140 | Implement review session create API | Create session from snapshot, preset, agents, and policies. | Done | T056, T082, T092 | T245 | API creates draft session and session agents. |
| T141 | Implement review start API | Transition session to queued/running and start workflow. | Done | T140, T132 | T142 | Start endpoint returns quickly; background workflow begins. |
| T142 | Implement workflow runner | Execute phases: context, agents, normalize, dedupe, verify, graph, draft. | Done | T141, T101 | T143 | Runner executes fake-agent review end-to-end. |
| T143 | Implement checkpointing | Persist phase progress and allow inspecting partially completed sessions. | Done | T142, T061 | T144 | Crash/restart can load last persisted state. |
| T144 | Implement session status transitions | Enforce valid transitions and timestamps. | Done | T056, T142 | T151 | Invalid transitions rejected; tests cover matrix. |
| T145 | Implement event bus | Append DB events and broadcast to SSE subscribers. | Done | T061, T046 | T146 | UI receives live events; DB retains event log. |
| T146 | Implement SSE endpoint | Stream session events with sequence IDs. | Done | T145 | T254 | Browser receives real-time events and reconnects. |
| T147 | Implement parallel agent scheduling | Run selected review agents in parallel with bounded concurrency. | Done | T101, T142 | T148 | Multiple fake agents run; ordering/events deterministic enough for tests. |
| T148 | Implement Local Verifier scheduling | Run deterministic verifier as part of workflow. | Done | T142, T200 | T149 | Verifier produces evidence/status for seeded findings. |
| T149 | Implement partial failure handling | Continue workflow when one agent fails if policy allows. | Done | T142, T098 | T150 | Failed agent emits error; other findings survive. |
| T150 | Implement workflow cancel | Cancel all running agents and mark session canceled. | Done | T100, T144 | T254 | Cancel stops processes and preserves partial results. |
| T151 | Implement pause/resume skeleton | Pause new phases and resume where safe. | Done | T143, T144 | T254 | MVP can mark pause/resume; complex active process pausing documented. |
| T152 | Implement early findings emission | Emit finding events before full workflow completes. | Done | T170, T145 | T254 | Early findings appear in UI event stream. |
| T153 | Implement run summary stats | Compute progress %, files scanned, active agents, finding counts. | Done | T145, T056 | T254 | Review Running screen receives summary model. |

---

## 13. Finding Engine

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T170 | Define finding schemas | Create JSON schemas for agent output and finding candidate. | Done | T013 | T171 | Schemas validate sample outputs; versioned. |
| T171 | Implement structured output parser | Parse valid JSON agent output into candidates. | Done | T096, T170 | T172 | Valid fake JSON agent produces candidate rows. |
| T172 | Implement JSONL/NDJSON parser | Parse streaming/event outputs into final candidate set. | Done | T096, T170 | T173 | Tests cover line-by-line agent output. |
| T173 | Implement text output normalizer | Convert text agent output into candidates using deterministic heuristics and optional repair prompt. | Done | T096, T170 | T174 | Text fixture produces reasonable candidates or clear low-confidence output. |
| T174 | Implement malformed output repair | Attempt one repair pass for malformed structured output. | Done | T173, T103 | T175 | Broken JSON fixture repaired or marked invalid with raw artifact. |
| T175 | Implement location normalization | Normalize path/line ranges and map to changed files where possible. | Done | T079, T171 | T176 | Invalid paths flagged; line ranges validated. |
| T176 | Implement candidate persistence | Store FindingCandidate rows with raw artifact links. | Done | T058, T171 | T177 | Candidates persist with agent provenance. |
| T177 | Implement finding fingerprinting | Compute stable fingerprints for duplicate detection. | Done | T176 | T178 | Similar samples produce same/near fingerprints. |
| T178 | Implement dedupe exact/overlap | Merge candidates by fingerprint and line overlap. | Done | T177 | T179 | Duplicate candidates merge into one finding. |
| T179 | Implement optional LLM dedupe hook | Add interface for future/optional LLM dedupe when deterministic merge is uncertain. | Done | T178 | T180 | Hook is feature-flagged, validates refined clusters, and default deterministic path works. |
| T180 | Implement canonical finding creation | Create Finding rows and candidate links. | Done | T178, T058 | T181 | Canonical findings include merged count and provenance. |
| T181 | Implement severity/category normalization | Normalize agent-specific labels into app enum values. | Done | T171 | T182 | Unknown labels map to safe defaults. |
| T182 | Implement finding ranking | Sort by severity, verification, confidence, agent agreement. | Done | T180 | T256 | API returns stable sort order. |
| T183 | Implement finding list API | Return findings with filters/search/status counts. | Done | T180, T062 | T256 | UI can filter all/verified/needs triage/accepted/dismissed. |
| T184 | Implement finding detail API | Return finding, candidates/provenance, evidence, code snippets, draft comment. | Done | T180, T204 | T260 | Finding Detail screen has all needed data. |
| T185 | Implement decision API | Accept, dismiss, defer, copied, published decisions. | Done | T058, T180 | T256,T290 | Decision updates finding status and appends human_decisions row. |
| T186 | Implement dismissal reasons | Capture dismissal reason and optional rule-memory suggestion. | Done | T185 | T335 | Dismissal reason persists and can be queried later. |
| T187 | Implement draft comment storage | Store/edit per-finding draft GitHub comment. | Done | T180 | T296 | User edits persist and are used in GitHub preview. |

---

## 14. Verification, Evidence, and Evidence Map

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T200 | Define EvidenceItem model | Implement domain/API models for supporting/counter/neutral/missing evidence. | Done | T059, T180 | T201 | Evidence item schema maps to DB/API. |
| T201 | Implement code search service | Wrapper around ripgrep/git grep with path sandbox, timeout, output limit. | Done | T071, T044 | T202 | Tests cover search hits, no hits, timeout, path restriction. |
| T202 | Implement primary location evidence | Attach changed code evidence for finding location. | Done | T175, T200 | T203 | Every located finding gets primary evidence item. |
| T203 | Implement counter-evidence search | Search likely guard/config/test paths for contradiction. | Done | T201, T200 | T204 | Counter-evidence items created when found. |
| T204 | Implement verification status assignment | Assign verified/plausible/needs_human/false_positive/not_actionable. | Done | T202, T203 | T205 | Seeded findings produce expected statuses. |
| T205 | Implement Local Verifier rules | Deterministic checks for auth guard, webhook validation, tests, idempotency basics. | Done | T201, T204 | T206 | Golden repo auth bug is verified by local verifier. |
| T206 | Implement verifier agent prompt runner | Optional CLI verifier task using finding-scoped context. | Done | T133, T090, T204 | T207 | Verifier CLI can update evidence/status; failures do not block local evidence. |
| T207 | Implement evidence API | Return evidence items grouped by support/counter/test/search. | Done | T200-T204 | T260 | Finding Detail evidence cards load correctly. |
| T208 | Implement evidence summaries | Generate concise evidence_summary and counter_evidence_summary. | Done | T204 | T184 | Finding cards/details show summaries. |
| T209 | Define Evidence Graph view model | Define API types for hierarchy, nodes, edges, call path, legend. | Done | T059 | T210 | Types documented and used by frontend mock data. |
| T210 | Implement graph node builder | Create graph nodes from finding, evidence items, code context. | Done | T134, T209 | T211 | Graph includes primary node and evidence nodes. |
| T211 | Implement graph edge builder | Create observed and missing edges: calls, mounts, protects, tests, supports, contradicts, missing_guard. | Done | T210 | T212 | Edges reference valid nodes; missing_guard visual status represented. |
| T212 | Implement call path builder | Build readable call path from graph/code relationships. | Done | T210, T211 | T213 | Call path appears for auth golden repo; unavailable reason stored if missing. |
| T213 | Implement node deep-link data | Add file/line references for nodes and external editor payloads. | Done | T210, T036 | T263 | Clicking node can open code view or editor. |
| T214 | Persist evidence graph | Store graph, nodes, edges, and call path. | Done | T210-T212, T059 | T215 | Graph can be reloaded after app restart. |
| T215 | Implement Evidence Map API | Return complete graph view model for finding. | Done | T214 | T263 | API returns hierarchy, nodes, edges, call path, right-panel data. |
| T216 | Implement graph rebuild API | Rebuild Evidence Map for a finding. | Done | T215 | T263 | Rebuild updates graph and emits event. |
| T217 | Handle incomplete graph fallback | Return partial graph with missing reasons instead of failure. | Done | T215 | T263 | UI can render partial/unavailable states. |
| T218 | Implement Ask Verifier from Evidence Map | Create follow-up/verifier task scoped to graph context. | Done | T206, T215, T280 | T264 | User can ask verifier about current graph path through Evidence Map routes; response persists in the finding thread. |
| T219 | Evidence Map tests | Unit/integration tests for graph creation and API response. | Done | T210-T217 | T360 | Golden auth repo produces expected nodes/edges/call path. |

---

## 15. Frontend Core UI

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T240 | Build app shell | Layout with left sidebar, top nav, content area matching mockups. | Done | T014, T016 | T241 | Renderer shell uses a reusable sidebar/header/content/detail-pane chrome, responsive desktop grid constraints, and placeholder review data matching the Codex Desktop design direction. |
| T241 | Implement API client | Typed REST client with auth token, response envelope, error handling. | Done | T032, T043 | T242 | Renderer API client injects local auth, decodes response envelopes, surfaces typed errors, and exposes loading/success/error state helpers. |
| T242 | Implement shared UI components | Buttons, cards, badges, tabs, dropdowns, command/search, empty/error states. | Done | T016 | All UI tasks | shadcn primitives cover buttons, cards, badges, tabs, dropdowns, command/search, inputs, scroll areas, tooltips, separators, skeletons, alerts, and empty states; app chrome helpers reuse them for shell/sidebar/pane/state UI. |
| T243 | Implement left workspace/thread sidebar | Workspace selector, threads list, saved presets, settings, agents online. | Done | T240, T054 | T244 | Sidebar loads workspaces and recent review sessions from authenticated APIs, bounds visible thread rendering for large histories, highlights active workspace/thread state, and exposes open-repo/search/settings actions. |
| T244 | Implement top nav | Repo selector, PR title, search, ask all agents, open repo, new thread, notifications/user. | Done | T240 | T245 | Top nav renders active repository/session context from API state, provides search, ask-all-agents, open-repo, commit, notifications, and overflow controls, and disables repository opening while a picker request is in progress. |
| T245 | Implement New Thread screen | Source selector for PR URL, local changes, branch compare, suggested setup, composer. | Done | T082, T243 | T246 | New Thread screen supports local changes, branch compare, and GitHub PR sources, validates required source fields, creates snapshots through typed APIs, and continues to Configure Review on success. |
| T246 | Implement Configure Review screen | Changed files, agents, policies/context, runtime table, start review. | Done | T081, T092, T140 | T247 | Configure Review shows bounded changed-file previews for large diffs, review-safe agent selection, runtime/depth controls, context policy toggles, focus prompt, and create/start review API flow. |
| T247 | Implement context policy UI | Toggles for changed code, call sites, tests, conventions, redaction, local-only files. | Done | T132, T246 | T248 | Configure Review sends the strict backend `context_policy` JSON, including prompt material, changed code, related code/tests/conventions, prior context, redaction, token/item budgets, and changed-file local-only paths. |
| T248 | Implement agent settings UI | Agent list, CLI config, health check, custom CLI creation. | Done | T092, T097 | T246 | Agent settings loads presets/configs from API, supports create-from-preset including OpenCode/custom CLI, edits command/args/env/output/runtime settings, saves through typed CRUD helpers, and runs health checks. |
| T249 | Implement review thread tabs | Chat, Review details, Findings, Publish tabs. | Done | T240, T140 | T250 | Review thread renders Chat, Review details, Findings, and Publish tabs, and keeps live review state scoped to the active session. |
| T250 | Implement review running screen | Status panel, progress bar, agent cards, early findings, pause/cancel controls. | Done | T146, T153 | T251 | Review screen loads summary data, consumes authenticated SSE events, shows bounded event/finding lists, and exposes session pause/resume/cancel controls. |
| T251 | Implement chat composer | Review/follow-up composer with runtime/model/reasoning/tool/permission controls. | In progress | T249, T092 | T280 | Composer controls render, but submit remains disabled until the UI is wired to review-level or finding-scoped question endpoints. |
| T252 | Implement event timeline/debug panel | Show event log/provenance in Review details. | Done | T146, T135 | T253 | Review details tab shows a bounded live event timeline with event level, type, sequence, agent run, artifact, timestamp, and payload preview. |
| T253 | Implement early findings list | Show early findings in review running screen. | Done | T152, T183 | T256 | Review running screen and Findings tab load early/canonical findings from the findings API and keep them refreshed during live review updates. |
| T254 | Implement review controls UI | Pause, cancel review, cancel one agent. | In progress | T150, T151, T100 | T250 | Session-level pause/resume/cancel controls call APIs and reflect state; per-agent cancel still needs an exposed agent-run cancel API route before completion. |
| T255 | Implement finding search/filter UI | Search and filter controls for findings board. | Done | T183, T062 | T256 | Findings Board calls the list API with debounced search plus status/severity filters and preserves the selected finding preview when filters change. |
| T256 | Implement Findings Board screen | Summary cards, list, selected finding preview, copy/accept/dismiss actions. | Done | T183, T185, T242 | T257 | Board renders summary cards, bounded finding list, selected preview, accept/dismiss decision updates, and local copy action against seeded/API data. |
| T257 | Implement finding card | Reusable card with severity/status/agents/location/actions. | Done | T256 | T258 | Reusable finding card supports selected, hover, loading, accept, and copy states with bounded preview text. |
| T258 | Implement agent consensus component | Show agent agreement/disagreement icons and summaries. | Done | T184 | T260 | Selected finding detail renders candidate-derived agreement, confidence, severity, and per-agent signal rows from provenance data. |
| T259 | Implement code viewer/diff component | Show code snippets with line numbers and added/removed highlights. | Done | T184, T242 | T260 | Detail code tab renders bounded evidence snippets with line numbers and a local copy-path action for long findings. |
| T260 | Implement Finding Detail screen | Changed code tabs, evidence list, consensus panel, draft comment, suggested fix, actions. | Done | T184, T207, T259 | T261 | Finding Detail is implemented as the selected detail pane in the Findings Board with overview, code, evidence, draft, consensus, and accept/dismiss/copy actions. |
| T261 | Implement evidence cards | Evidence list with file/line links, status, details, counter-evidence. | Done | T207, T260 | T263 | Evidence tab renders prioritized evidence cards with kind badges, file/line locations, and bounded summaries. |
| T262 | Implement draft comment editor | Show/edit draft GitHub comment. | Done | T187, T260 | T296 | Draft tab edits and persists the draft GitHub comment through the draft-comment API and refreshes finding detail/list state. |
| T263 | Implement Evidence Map screen | Code hierarchy, graph, right panel, call path, legend, back/open editor actions. | Done | T215, T242 | T264 | Evidence Map renders as a review subview with hierarchy, bounded SVG graph layout, call path rail, legend, partial-state badges, and selected context panel. |
| T264 | Implement Evidence Map interactions | Node click, edge detail, ask verifier, open in editor. | Done | T218, T213, T263 | T280 | Node/edge/call-path selection updates the panel, rebuild reloads graph state, ask verifier posts graph refs, and open-in-editor uses deep-link data through the Electron bridge. |
| T265 | Implement Follow-up screen | Evidence bundle chips, chat messages, finding summary, selected agents, quick actions. | Done | T280, T251 | T266 | Follow-up opens from selected findings, loads/persists the finding thread, shows summary/evidence bundle/agent selector, supports scoped questions, and runs counter/accept/copy/dismiss quick actions. |
| T266 | Implement Publish screen | Accepted findings selector, GitHub preview, copy fix packet preview, checklist. | Done | T290, T296 | T267 | Publish tab loads accepted findings, builds GitHub preview/checklist, renders copy-packet previews, and copies selected packets through the Electron clipboard bridge; direct GitHub submit remains disabled until an HTTP publish route is exposed. |
| T267 | Implement loading/error/empty states | Add robust state components across all screens. | Not started | T240-T266 | T360 | Common failure states are understandable and tested. |

---

## 16. Follow-up, Copy Packets, and GitHub Publishing

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T280 | Implement finding thread service | Create/load finding-scoped follow-up thread. | Done | T133, T185 | T281 | Thread exists per finding and persists messages. |
| T281 | Implement follow-up API | Submit question to selected CLI/local verifier with finding-scoped context. | Done | T280, T090, T133 | T282 | Fake agent answer persists and cites evidence refs. |
| T282 | Implement follow-up message persistence | Store user/assistant messages with evidence refs and artifacts. | Done | T280 | T265 | Reloading finding shows thread history. |
| T283 | Implement quick actions from follow-up | Ask counter-evidence, accept, dismiss, copy. | Done | T281, T185 | T265 | Quick actions update finding state. |
| T290 | Implement copy packet renderer | Render Markdown/XML-ish/JSON/compact/GitHub summary. | Done | T184, T185, T060 | T291 | Snapshot + selected findings render accurately. |
| T291 | Implement copy packet API | Generate packet, store artifact, return content/token estimate. | Done | T290, T060 | T292 | API works for single/selected/accepted findings. |
| T292 | Implement clipboard UI action | Use Electron bridge to copy packet/comment. | Done | T034, T291 | T266 | Publish screen copy-packet action writes generated packet content through the Electron clipboard bridge and reports success/failure inline. |
| T293 | Mark findings copied | Record copied decisions and packet metadata. | Done | T291, T185 | T266 | Copied state appears on findings/publish screen. |
| T294 | Implement GitHub diff mapper | Map finding file/line to GitHub diff position/line/side. | Done | T079, T076 | T295 | Tests cover add/remove/context/multi-hunk cases. |
| T295 | Implement GitHub review preview service | Build review body and comments JSON from selected findings. | Done | T187, T294 | T296 | Preview returns comment list and anchor warnings. |
| T296 | Implement GitHub preview API | Endpoint returns publish preview artifact and checklist status. | Done | T295 | T266 | Publish screen loads preview with seeded findings. |
| T297 | Implement GitHub publish service | Submit COMMENT or REQUEST_CHANGES review/comments. | Done | T073, T296 | T298 | Fake GitHub server test validates payload. |
| T298 | Track publication state | Store GitHub review/comment IDs and update finding decisions. | Done | T297, T185 | T299 | Published findings show status and avoid republish. |
| T299 | Implement duplicate publish prevention | Detect findings already published for same snapshot/location. | Done | T298 | T266 | Rerun warns/prevents duplicate comments. |
| T300 | Implement summary-only review | Publish only review body without inline comments. | Done | T297 | T266 | Summary-only path works and is tested. |
| T301 | Implement pending/draft review path | Save pending review if using GitHub pending review API. | Done | T297 | T266 | GitHub client can create pending reviews without `event` and submit them later through the review events endpoint. |

---

## 17. Security, Privacy, and Settings

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T320 | Implement path sandbox | Ensure all file reads/artifacts stay inside workspace/app dirs. | Done | T060, T071 | T321 | Shared sandbox helpers guard artifact and repo read/write choke points; traversal and symlink escape tests fail safely. |
| T321 | Implement command safety policy | Block shell execution by default and restrict dangerous commands. | Done | T093 | T322 | CLI commands stay arg-array based; inline shell syntax and risky command names are blocked unless config explicitly opts into `allow_risky_command`. |
| T322 | Implement permission model | Define read/search/test/shell/write/publish risk levels. | Done | T321 | T323 | Permission actions map to risk levels and runner metadata records approved/denied decisions consistently. |
| T323 | Add review-mode write denial | Ensure review agents cannot modify files through cocode-managed tools. | Done | T322 | T090 | Review-mode sessions, follow-up agents, and runtime runs reject write-capable configs before launching. |
| T324 | Implement env allowlist | Only pass explicitly allowed env vars to CLI agents. | Done | T093 | T330 | Shared env-name validation and allowlist resolution are enforced for review, follow-up, and command health checks. |
| T325 | Implement local-only file enforcement | Exclude local-only files from external/cloud-backed CLI context. | Done | T130, T247 | T326 | External agent context excludes local-only changed files/items before reads and records omissions. |
| T326 | Implement provider visibility metadata | Track which files/context items were sent to which agent. | Done | T131, T325 | T252 | Context artifacts, preview responses, debug responses, and agent runs record recipient provider/egress and sent item counts. |
| T327 | Implement secret redaction UI | Show redaction status/report in configure/review detail. | Not started | T130, T247 | T333 | User can inspect redaction summary. |
| T328 | Implement credentials settings | Add GitHub token and optional CLI credential refs/settings. | Not started | T037, T073 | T248 | Secrets are stored outside DB plaintext. |
| T329 | Implement backend Origin checks | Reject suspicious browser-origin requests to local backend. | Done | T042 | T330 | Origin middleware accepts loopback/file/app origins, rejects non-local browser origins, and handles local CORS preflight without auth. |
| T330 | Security smoke tests | Test local auth, Origin, path sandbox, env allowlist, secret redaction. | Done | T320-T329 | T360 | `internal/securitysmoke` covers local auth, Origin, path sandbox, env allowlist, and secret redaction, and CI runs it explicitly. |
| T331 | Add prompt-injection guardrails | Wrap repo/PR content as untrusted data in prompts. | Done | T120, T170 | T332 | Rendered context and review/verifier/follow-up prompts label repo/PR/agent content as untrusted evidence; Markdown fences expand around nested fences. |
| T332 | Add agent output trust rules | Treat agent output as untrusted until parsed/verified. | Done | T170 | T204 | Parsed output artifacts/events mark agent output untrusted; copy packets carry trust boundaries; GitHub preview/publication require accepted findings before publish side effects. |
| T333 | Add privacy indicators | UI shows cloud/local status and local-only restrictions. | Not started | T326, T247 | T248 | User sees which agents may receive code. |
| T334 | Implement audit log viewer | Surface events/actions/approvals in Review details. | Not started | T252, T061 | T360 | User can inspect decisions, copies, publish actions. |
| T335 | Implement review rule memory | Convert dismissal reasons into optional local rules. | Not started | T186, T127 | T336 | User can enable/disable stored rules. |
| T336 | Implement settings export/import | Export non-secret settings, presets, and rules. | Not started | T248, T335 | T390 | Export does not include secrets; import validates schema. |

---

## 18. Testing, Evaluation, and Packaging

| ID | Task | Description | Status | Dependencies | Parallelization | Done criteria |
|---|---|---|---|---|---|---|
| T360 | Create backend unit test suite | Unit tests for parsers, diff mapping, redaction, packet rendering, verification rules. | Not started | Feature packages | T361 | `go test ./...` covers core pure logic. |
| T361 | Create backend integration test harness | Temp SQLite DB + fake repo + fake CLI agents. | Not started | T050, T060, T093 | T362 | Integration tests run locally/CI without real providers. |
| T362 | Create fake GitHub server | Simulate PR metadata/files/reviews endpoints. | Not started | T073, T297 | T363 | GitHub ingestion and publish tests do not hit real GitHub. |
| T363 | Create golden repo: auth bug | Fixture repo matching Evidence Map auth/middleware example. | Not started | T010 | T219 | Review/verification/evidence map tests use it. |
| T364 | Create golden repo: webhook validation | Fixture for missing webhook signature validation. | Not started | T010 | T205 | Verifier detects expected finding. |
| T365 | Create golden repo: generated-file noise | Fixture for file exclusion and context budget behavior. | Not started | T010 | T080, T129 | Generated files excluded by default. |
| T366 | Create frontend component tests | Tests for finding card, evidence card, graph node, copy buttons. | Not started | T242, T257, T261 | T367 | Component tests cover loading/error/action states. |
| T367 | Create E2E test harness | Launch Electron with fake backend or seeded DB. | Not started | T014, T040 | T368 | E2E can navigate major screens. |
| T368 | E2E: New thread to configure | Test PR URL/local branch flow to Configure Review. | Not started | T245, T246, T367 | T369 | Test passes with seeded/fake data. |
| T369 | E2E: Run fake review | Start review with fake agents and see findings. | Not started | T142, T250, T367 | T370 | Test verifies progress and findings board. |
| T370 | E2E: Finding detail and Evidence Map | Open finding detail and Evidence Map. | Not started | T260, T263, T367 | T371 | Test verifies graph/call path render. |
| T371 | E2E: Copy packet | Accept findings and copy selected packet. | Not started | T291, T292, T367 | T372 | Clipboard bridge mocked/verified. |
| T372 | E2E: GitHub preview | Preview publish payload with fake GitHub. | Not started | T296, T367 | T373 | Preview comments and checklist render. |
| T373 | Reliability test: agent timeout | Slow fake agent times out without losing other findings. | Not started | T099, T149, T361 | T374 | Session completes partially with clear error. |
| T374 | Reliability test: app restart | Restart app/backend and reload previous session. | Not started | T143, T031, T361 | T375 | Findings/evidence/decisions persist. |
| T375 | Evaluation harness v1 | Run reviews on golden repos and measure expected findings. | Not started | T363-T365, T142 | T376 | Outputs metrics: precision-ish accepted/expected, false positives, cost/time where available. |
| T376 | Add dogfood checklist | Manual test checklist for real PRs. | Not started | Core MVP | T390 | Checklist covers setup, run, triage, evidence map, copy, publish. |
| T380 | Configure electron-builder | Build dev/prod app package for macOS first, then Windows/Linux if possible. | Not started | T014, T031 | T381 | Packaged app launches backend and UI. |
| T381 | Add app signing/notarization notes | Document signing steps even if not fully automated in MVP. | Not started | T380 | T382 | Release docs explain platform requirements. |
| T382 | Add update strategy note | Decide no auto-update vs future auto-update. | Not started | T380 | T390 | MVP release behavior documented. |
| T383 | Add first-run setup guide | UI/docs for connecting GitHub token and CLI agents. | Not started | T248, T328 | T390 | New user can configure app without developer help. |
| T384 | Add troubleshooting guide | Missing CLI, auth errors, invalid output, timeouts, GitHub anchor failures. | Not started | T044, T092, T297 | T390 | Errors link to guide or show relevant advice. |
| T390 | MVP release candidate review | Verify all P0 tasks, tests, security, packaging, and docs. | Not started | All P0 tasks | None | Release checklist passes; known issues documented; no auto-publish/write behavior. |

---

## 19. Deferred / Post-MVP Tasks

| ID | Task | Reason deferred | Dependency to revisit | Done criteria when resumed |
|---|---|---|---|---|
| F001 | Full Codex App Server connector | MVP starts with non-interactive CLI; app-server adds protocol complexity. | T108, T109 | Can create/resume Codex threads and stream events through cocode. |
| F002 | Full ACP connector | MVP starts with CLI; ACP requires session lifecycle and protocol event mapping. | T108, T110 | Gemini ACP or another ACP agent runs through same review flow. |
| F003 | MCP tool server support | Requires permission UI and tool registry. | T322 | Agents can use approved MCP tools with audit events. |
| F004 | A2A remote agent support | Useful later for remote opaque agents, not local MVP. | Protocol layer | Can discover remote agent card and delegate a task. |
| F005 | In-app fixing | User prefers copy-to-main-agent for MVP. | Findings/copy mature | Worktree, patch preview, tests, and user approval implemented. |
| F006 | Cloud/team runner | Adds major infra/security complexity. | Local MVP proven | GitHub App + container worker + tenant isolation. |
| F007 | GitLab/Bitbucket support | GitHub first. | Publisher abstraction | Similar preview/publish UX works for another provider. |
| F008 | Real cost accounting | CLI cost visibility varies by provider. | Agent metadata | Cost shown accurately for supported agents. |

---

## 20. MVP Completion Criteria

The MVP is complete when:

1. A user can open a local repo and create a PR/local snapshot.
2. A user can configure and run at least two fake/real CLI agents plus Local Verifier.
3. The review workflow produces canonical findings from CLI output.
4. Findings can be verified and shown with evidence.
5. Evidence Map shows hierarchy, graph, and call path for at least one golden repo finding.
6. User can ask a finding-scoped follow-up.
7. User can accept/dismiss/defer findings.
8. User can copy one, selected, and accepted findings as Markdown fix packets.
9. User can preview GitHub comments and publish selected findings to a fake or real test PR.
10. App handles missing CLI, invalid output, timeout, cancellation, and GitHub anchor failure gracefully.
11. Local security controls pass: auth token, localhost binding, path sandbox, env allowlist, no raw renderer Node access.
12. Packaged app launches on at least one target OS.
13. E2E tests cover New Thread -> Configure -> Run -> Findings -> Evidence Map -> Copy -> Publish Preview.
14. Dogfood review on a real PR is possible without developer intervention.


---

## 21. Reference URLs

- OpenAI Codex App Server blog: https://openai.com/index/unlocking-the-codex-harness/
- OpenAI Codex App Server README: https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md
- Agent Client Protocol transports: https://agentclientprotocol.com/protocol/transports
- Claude Code programmatic/headless CLI: https://code.claude.com/docs/en/headless
- Gemini CLI ACP mode: https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/acp-mode.md
- GitHub pull request reviews REST API: https://docs.github.com/rest/pulls/reviews
- Electron security tutorial: https://www.electronjs.org/docs/latest/tutorial/security
- Gin documentation: https://gin-gonic.com/en/docs/
- SQLite WAL documentation: https://www.sqlite.org/wal.html
- sqlc documentation: https://sqlc.dev/
