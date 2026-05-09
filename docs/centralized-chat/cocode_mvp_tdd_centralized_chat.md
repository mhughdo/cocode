# cocode MVP TDD — Centralized Chat, CLI Agents, Findings, and Evidence Map

**Document status:** Updated for centralized chat mockups  
**App name:** cocode  
**Date:** 2026-05-06  
**Backend:** Go + Gin  
**Frontend:** Electron + React + TypeScript + shadcn/ui  
**Phase 1 adapter scope:** Non-interactive CLI agents only  
**Future adapter targets:** Codex App Server, ACP, MCP, A2A, provider SDKs

---

## Table of Contents

1. [Purpose](#purpose)
2. [Feasibility: Centralized Chat with Non-Interactive CLIs](#feasibility-centralized-chat-with-non-interactive-clis)
3. [Architecture Principles](#architecture-principles)
4. [Mockup Alignment](#mockup-alignment)
5. [System Architecture](#system-architecture)
6. [Project and Folder Architecture](#project-and-folder-architecture)
7. [Centralized Chat Architecture](#centralized-chat-architecture)
8. [Agent Runtime and Adapter Architecture](#agent-runtime-and-adapter-architecture)
9. [Review Workflow](#review-workflow)
10. [Context Management](#context-management)
11. [Database Schema](#database-schema)
12. [Artifact Store](#artifact-store)
13. [Backend API Contracts](#backend-api-contracts)
14. [SSE Event Model](#sse-event-model)
15. [Frontend Architecture](#frontend-architecture)
16. [Evidence Map Technical Design](#evidence-map-technical-design)
17. [Findings Technical Design](#findings-technical-design)
18. [Copy Packet Technical Design](#copy-packet-technical-design)
19. [GitHub Publishing](#github-publishing)
20. [Prompt System](#prompt-system)
21. [Failure Handling](#failure-handling)
22. [Security](#security)
23. [Testing Strategy](#testing-strategy)
24. [Implementation Setup](#implementation-setup)
25. [Definition of Done](#definition-of-done)
26. [Deferred Architecture Hooks](#deferred-architecture-hooks)

---

## Purpose

This TDD describes how to implement cocode’s MVP as a local-first multi-agent code review desktop app with a centralized chat. It includes the technical architecture, Go/Electron folder structure, core interfaces, database schema, prompt templates, event contracts, adapter design, error handling, and done criteria.

---

## Feasibility: Centralized Chat with Non-Interactive CLIs

### Decision

cocode can support centralized chat in Phase 1 even though external agents are only connected through non-interactive CLI mode.

The implementation model is:

```text
User message
  -> persisted as ThreadMessage
  -> turned into ChatTurn
  -> routed by cocode
  -> one or more CLI processes are invoked
  -> raw outputs are captured as AgentRuns and Artifacts
  -> outputs are normalized/synthesized
  -> clean assistant/system messages are appended to the central chat
  -> findings/evidence/publish/copy state is updated as needed
```

The central chat belongs to cocode. The CLIs are workers.

### Why this works

Most useful “chat” behavior does not require the external CLI to own the full thread. The user needs a coherent UI, preserved history, routed follow-up questions, visible progress, and agent outputs. cocode can provide that by persisting and curating context for each turn.

### What Phase 1 central chat is not

Phase 1 does not implement:

- peer-to-peer agent conversations,
- persistent live sessions with every CLI,
- full Codex App Server driver,
- full ACP driver,
- multi-user collaborative chat.

### Phase 1 runtime behavior

For each turn, cocode builds a `ChatContextBundle`:

```text
thread summary
recent messages
review source snapshot
selected findings
selected evidence map
changed files or relevant snippets
agent role/preset instructions
user question
expected output schema
```

It then calls one or more non-interactive CLIs using stdin/temp files/arguments. Results are stored and appended to the chat.

---

## Architecture Principles

1. **cocode is the source of truth.** External CLIs may have sessions, but cocode owns thread state, messages, findings, evidence, and decisions.
2. **Chat is an interface, not the data model.** Findings, evidence maps, artifacts, agent runs, and publications remain structured entities.
3. **Use explicit workflows for review.** Multi-agent chat is used for interaction; review execution remains deterministic and inspectable.
4. **Agents collaborate through cocode.** CLI agents do not talk directly to each other in Phase 1.
5. **Every agent output has provenance.** Store command, adapter, context bundle, stdout/stderr, exit code, artifacts, and normalized result.
6. **Every finding needs evidence.** A claim without evidence is downgraded or hidden.
7. **Every task needs done criteria.** Prompts and implementation tasks must define what “done” means.
8. **SSE first.** Use REST for commands and SSE for progress/events; WebSocket is deferred.
9. **Design adapter interfaces for transport evolution.** CLI now; Codex App Server/ACP later.
10. **Prefer local-first and human-gated actions.** No auto-publishing or file edits without approval.

---

## Mockup Alignment

| Mockup area                              |    Adopt for MVP? | Technical implication                                                         |
| ---------------------------------------- | ----------------: | ----------------------------------------------------------------------------- |
| Central chat as default review tab       |               Yes | Add `threads`, `thread_messages`, `chat_turns`, chat router, chat SSE events. |
| Chat input with ask target and responder |               Yes | Add audience/responder selection to chat turn request.                        |
| Review summary side cards                |               Yes | Derived from findings and activity events.                                    |
| Findings table with side preview         |               Yes | Add list/detail APIs and side-preview component.                              |
| Finding detail with thread area          |               Yes | Finding replies are messages in central thread with `finding_id` context.     |
| Evidence map screen                      |               Yes | Add graph schema, builder, API, React graph view.                             |
| Publish tab with copy packet             |               Yes | Publish and copy are peer actions.                                            |
| Settings / Adapters                      |               Yes | Phase 1 CLI configs; future rows disabled unless implemented.                 |
| User accounts/team avatar                | Local only in MVP | Store local profile settings only; no multi-user backend.                     |
| Agent labels such as “OpenCode API”      |            Adjust | Phase 1 treats OpenCode as CLI. Future drivers can be shown disabled.         |
| “Read & Write” tools in chat             |       Not default | Review mode is read-only. Write tools deferred.                               |

---

## System Architecture

```text
Electron Main
  - launches Go backend
  - manages local auth token
  - secure IPC and clipboard
  - file dialogs
  - safe credential storage

Electron Renderer
  - React + TypeScript + shadcn/ui
  - routes: Setup, Chat, Findings, Finding Detail, Evidence Map, Publish, Settings
  - REST commands
  - SSE event subscription
  - local UI state

Go Backend
  - Gin HTTP API
  - SQLite + sqlc
  - artifact store
  - git/GitHub clients
  - chat service
  - orchestrator
  - CLI runtime
  - context builder
  - finding engine
  - evidence map builder
  - copy packet exporter
  - GitHub publisher
```

---

## Project and Folder Architecture

```text
cocode/
  apps/
    desktop/
      electron/
        main/
          main.ts
          backend-process.ts
          ipc.ts
          clipboard.ts
          safe-storage.ts
        preload/
          index.ts
        renderer/
          src/
            app/
              routes.tsx
              providers.tsx
            components/
              layout/
              chat/
              findings/
              evidence-map/
              publish/
              adapters/
              code/
            features/
              threads/
              chat/
              findings/
              evidence-map/
              review-setup/
              publish/
              adapters/
            lib/
              api-client.ts
              sse-client.ts
              ids.ts
              clipboard.ts
            stores/
              thread-store.ts
              review-store.ts
              settings-store.ts
            styles/
              globals.css
      package.json
      electron.vite.config.ts

  services/
    cocoded/
      cmd/cocoded/main.go
      internal/
        app/
        httpapi/
        domain/
        db/
        git/
        github/
        orchestrator/
        chat/
        contextbuilder/
        agents/
        findings/
        evidencemap/
        exports/
        security/
        artifacts/
        telemetry/
        testutil/
      migrations/
      sqlc.yaml
      go.mod

  packages/
    presets/
      presets/
      roles/
      knowledge/
      schemas/
    shared-schemas/
      jsonschema/
      openapi/

  docs/
    prd.md
    tdd.md
    task-breakdown.md
    presets-roles.md
```

---

## Centralized Chat Architecture

### Domain Concepts

| Concept             | Definition                                                                     |
| ------------------- | ------------------------------------------------------------------------------ |
| Project             | Local repo/project root.                                                       |
| Thread              | A user-visible conversation/work item, usually tied to a PR or branch compare. |
| Review Source       | GitHub PR URL, local changes, or branch comparison.                            |
| Review Session      | One structured review run inside a thread.                                     |
| Thread Message      | Persisted message shown in chat.                                               |
| Chat Turn           | One user request and its execution lifecycle.                                  |
| Message Context Ref | Links a message to finding/evidence/artifact/file/review.                      |
| Agent Run           | One invocation of one CLI/driver.                                              |
| Synthesis           | A cocode-generated or agent-generated combined answer.                         |
| Thread Summary      | Compact summary used to keep long chat prompt sizes manageable.                |

### Message author types

```go
type MessageAuthorType string

const (
    AuthorUser         MessageAuthorType = "user"
    AuthorCocode       MessageAuthorType = "cocode"
    AuthorOrchestrator MessageAuthorType = "orchestrator"
    AuthorAgent        MessageAuthorType = "agent"
    AuthorSystem       MessageAuthorType = "system"
    AuthorVerifier     MessageAuthorType = "verifier"
)
```

### Message block types

Messages are not only text. Use structured blocks so the UI can render cards.

```go
type MessageBlockType string

const (
    BlockMarkdown        MessageBlockType = "markdown"
    BlockAgentRunCard    MessageBlockType = "agent_run_card"
    BlockFindingList     MessageBlockType = "finding_list"
    BlockFindingCard     MessageBlockType = "finding_card"
    BlockEvidenceMapCard MessageBlockType = "evidence_map_card"
    BlockCodeReference   MessageBlockType = "code_reference"
    BlockCopyPacketCard  MessageBlockType = "copy_packet_card"
    BlockPublishCard     MessageBlockType = "publish_card"
    BlockErrorCard       MessageBlockType = "error_card"
)
```

### Chat turn request

```go
type CreateChatTurnRequest struct {
    ThreadID        string              `json:"thread_id"`
    Body            string              `json:"body"`
    Mode            ChatMode            `json:"mode"`
    Audience        ChatAudience        `json:"audience"`
    Responder       ResponderSelection  `json:"responder"`
    ContextRefs     []ContextRef        `json:"context_refs"`
    Options         ChatTurnOptions     `json:"options"`
}

type ChatMode string

const (
    ChatModeReview     ChatMode = "review"
    ChatModeFollowUp   ChatMode = "follow_up"
    ChatModeEvidence   ChatMode = "evidence"
    ChatModePublish    ChatMode = "publish"
    ChatModeCopy       ChatMode = "copy"
    ChatModeSettings   ChatMode = "settings"
)

type ChatAudience string

const (
    AudienceOrchestrator ChatAudience = "orchestrator"
    AudienceAllAgents    ChatAudience = "all_agents"
    AudienceSelected     ChatAudience = "selected_agent"
    AudienceVerifier     ChatAudience = "verifier"
    AudienceLocalOnly    ChatAudience = "local_only"
)

type ResponderSelection struct {
    AdapterConfigID *string `json:"adapter_config_id,omitempty"`
    RoleID          *string `json:"role_id,omitempty"`
    ModelLabel      *string `json:"model_label,omitempty"`
}

type ContextRef struct {
    Type string `json:"type"` // review_session, finding, evidence_map, file, artifact, publish_draft
    ID   string `json:"id"`
}
```

### Chat turn lifecycle

```text
created
  -> routing
  -> context_building
  -> running
  -> synthesizing
  -> completed

or

created
  -> routing
  -> failed

or

running
  -> cancel_requested
  -> canceled
```

### Chat routing

The router chooses one of several execution plans.

```go
type ChatExecutionPlan struct {
    Kind           ChatExecutionKind
    AgentTargets   []AgentTarget
    NeedSynthesis  bool
    LocalAction    *LocalAction
    ContextPolicy  ContextPolicy
    OutputContract OutputContract
}

type ChatExecutionKind string

const (
    ExecLocalAnswer      ChatExecutionKind = "local_answer"
    ExecSingleAgent      ChatExecutionKind = "single_agent"
    ExecParallelAgents   ChatExecutionKind = "parallel_agents"
    ExecReviewWorkflow   ChatExecutionKind = "review_workflow"
    ExecEvidenceMapBuild ChatExecutionKind = "evidence_map_build"
    ExecCopyPacket       ChatExecutionKind = "copy_packet"
    ExecPublishPreview   ChatExecutionKind = "publish_preview"
)
```

Routing examples:

| User message                                    | Plan                                                           |
| ----------------------------------------------- | -------------------------------------------------------------- |
| “Run a deep review”                             | `ExecReviewWorkflow`                                           |
| “What are the top issues?”                      | `ExecLocalAnswer` if findings exist; otherwise review workflow |
| “Ask all agents if finding FND-42 is real”      | `ExecParallelAgents` + synthesis + `finding_id` context        |
| “Could auth still be applied by parent router?” | `ExecSingleAgent` or verifier + evidence context               |
| “Copy accepted findings”                        | `ExecCopyPacket`                                               |
| “Publish accepted findings”                     | `ExecPublishPreview`                                           |

### Central chat service flow

```go
func (s *ChatService) CreateTurn(ctx context.Context, req CreateChatTurnRequest) (*ChatTurn, error) {
    msg, err := s.messages.CreateUserMessage(ctx, req)
    if err != nil { return nil, err }

    turn, err := s.turns.Create(ctx, req.ThreadID, msg.ID)
    if err != nil { return nil, err }

    s.jobs.Enqueue(ChatTurnJob{TurnID: turn.ID})
    return turn, nil
}

func (r *ChatTurnRunner) Run(ctx context.Context, turnID string) error {
    turn := r.loadTurn(ctx, turnID)
    plan := r.router.Plan(ctx, turn)
    bundle := r.contextBuilder.BuildChatContext(ctx, turn, plan.ContextPolicy)

    switch plan.Kind {
    case ExecLocalAnswer:
        return r.answerFromLocalState(ctx, turn, bundle)
    case ExecSingleAgent:
        return r.runSingleAgent(ctx, turn, plan, bundle)
    case ExecParallelAgents:
        return r.runParallelAgents(ctx, turn, plan, bundle)
    case ExecReviewWorkflow:
        return r.reviewWorkflow.StartFromChat(ctx, turn, bundle)
    case ExecEvidenceMapBuild:
        return r.evidenceMapBuilder.BuildFromChat(ctx, turn, bundle)
    case ExecCopyPacket:
        return r.copyPacketExporter.ExportFromChat(ctx, turn, bundle)
    case ExecPublishPreview:
        return r.publisher.PreviewFromChat(ctx, turn, bundle)
    default:
        return ErrUnknownPlan
    }
}
```

### Ask all agents flow

```text
User asks all agents
  -> router selects enabled review agents
  -> context builder creates one shared context bundle
  -> adapter runtime fans out to CLIs in parallel
  -> each agent output becomes an AgentRun and raw artifact
  -> each response appears as expandable agent card
  -> synthesizer creates final cocode message
```

### Finding follow-up flow

A finding follow-up is not a separate chat. It is a normal thread message with context refs:

```json
{
  "context_refs": [
    { "type": "finding", "id": "fnd_01" },
    { "type": "evidence_map", "id": "evm_01" }
  ]
}
```

The UI may show it in the finding detail “Finding thread,” but the message remains part of the central review thread.

### Thread summary and compaction

Maintain summaries to prevent prompt bloat.

```go
type ThreadSummary struct {
    ID          string
    ThreadID    string
    UpToMessage string
    Summary     string
    FactsJSON   []byte
    CreatedAt   time.Time
}
```

Summary update triggers:

- every 20 messages,
- every completed review workflow,
- after large ask-all-agents turn,
- when accumulated prompt estimate exceeds a threshold.

Summary must preserve:

- review goal,
- source snapshot,
- accepted/dismissed findings,
- unresolved questions,
- important evidence,
- known constraints,
- prior agent conclusions.

---

## Agent Runtime and Adapter Architecture

### Phase 1: CLI-only non-interactive support

Supported initial drivers:

| Agent           | Phase 1 connection               | Notes                              |
| --------------- | -------------------------------- | ---------------------------------- |
| Codex CLI       | `codex exec`                     | Prefer JSONL when available.       |
| Claude Code CLI | `claude -p`                      | Prefer JSON or stream-json.        |
| Gemini CLI      | `gemini -p` or `gemini --prompt` | Prefer JSON output when available. |
| OpenCode        | `opencode run`                   | Prefer JSON events.                |
| Custom CLI      | command template                 | Text fallback.                     |

### Future drivers

Keep driver abstraction ready for:

| Driver           | Future use                                          |
| ---------------- | --------------------------------------------------- |
| Codex App Server | Long-lived JSON-RPC app-server integration.         |
| ACP stdio        | Gemini/OpenCode/Cursor-like agent session protocol. |
| MCP client       | Tool and context servers.                           |
| A2A client       | Remote agent task delegation.                       |
| Provider SDK     | Direct model/API structured calls.                  |

### Connection driver interface

```go
type ConnectionDriver interface {
    ID() string
    Kind() ConnectionKind
    HealthCheck(ctx context.Context, cfg AgentConfig) (*HealthResult, error)
    Start(ctx context.Context, cfg AgentConfig, session AgentExternalSession) (*DriverSession, error)
    RunTurn(ctx context.Context, sess *DriverSession, input DriverTurnInput) (<-chan DriverEvent, error)
    Cancel(ctx context.Context, sess *DriverSession, turnID string) error
    Close(ctx context.Context, sess *DriverSession) error
}

type ConnectionKind string

const (
    ConnectionCLI            ConnectionKind = "cli_non_interactive"
    ConnectionCodexAppServer ConnectionKind = "codex_app_server"
    ConnectionACPStdio       ConnectionKind = "acp_stdio"
    ConnectionMCP            ConnectionKind = "mcp"
    ConnectionA2A            ConnectionKind = "a2a"
    ConnectionProviderSDK    ConnectionKind = "provider_sdk"
)
```

### CLI driver implementation

```go
type CLIDriverConfig struct {
    Command           string
    ArgsTemplate      []string
    WorkingDirMode    string // project_root, temp_context_dir
    PromptMode        string // stdin, arg, temp_file
    OutputMode        string // text, json, jsonl
    TimeoutSeconds    int
    EnvAllowlist      []string
    ReadOnly          bool
    MaxStdoutBytes    int64
    MaxStderrBytes    int64
    SupportsResume    bool
    ResumeArgTemplate []string
}

func (d *CLIDriver) RunTurn(ctx context.Context, sess *DriverSession, input DriverTurnInput) (<-chan DriverEvent, error) {
    events := make(chan DriverEvent)
    go func() {
        defer close(events)
        cmd := exec.CommandContext(ctx, d.cfg.Command, renderArgs(d.cfg.ArgsTemplate, input)...)
        cmd.Dir = resolveWorkingDir(d.cfg, input)
        cmd.Env = buildEnv(os.Environ(), d.cfg.EnvAllowlist, input.Env)
        stdin, _ := cmd.StdinPipe()
        stdout, _ := cmd.StdoutPipe()
        stderr, _ := cmd.StderrPipe()

        // Start process, stream stdout/stderr, parse JSONL if configured,
        // emit DriverEvents, persist raw artifacts, enforce byte limits.
        _ = stdin
        _ = stdout
        _ = stderr
    }()
    return events, nil
}
```

### CLI execution rules

- Use args array, not shell strings.
- Prefer stdin or temp file for prompts.
- Store prompt as artifact.
- Store stdout/stderr separately.
- Treat stderr as logs unless exit code is non-zero.
- Enforce timeout and max output bytes.
- Kill process on cancellation.
- Never pass secrets unless explicitly allowed.
- Default review mode is read-only.
- Preserve raw output even if parsing fails.
- Use a normalizer if only text output is available.

### Codex App Server future driver

The future `CodexAppServerDriver` should implement the same `ConnectionDriver` interface. It will start or connect to `codex app-server`, initialize a JSON-RPC session, send turn requests, receive app-server events, handle approvals, and map native events into `DriverEvent`.

Do not hardcode Codex-specific objects into central chat. Map them into generic events:

```text
codex turn/started      -> AgentRunStarted
codex item/delta        -> AgentOutputDelta
codex approval/request  -> ToolApprovalRequested
codex turn/completed    -> AgentRunCompleted
codex error             -> AgentRunFailed
```

### ACP future driver

The future `ACPStdioDriver` should launch an ACP-compatible agent as a subprocess and exchange JSON-RPC over stdio. The same cocode chat thread remains the canonical state; ACP sessions are external session metadata.

### Agent collaboration model

Phase 1 collaboration patterns:

| Pattern                 | Supported? | Implementation                                                |
| ----------------------- | ---------: | ------------------------------------------------------------- |
| Parallel fan-out        |        Yes | Run selected CLIs concurrently.                               |
| Sequential handoff      |        Yes | Pass Agent A output as context to Agent B.                    |
| Verifier critique       |        Yes | Pass finding + evidence to verifier.                          |
| Synthesized consensus   |        Yes | Synthesize multiple outputs into one message/finding.         |
| Shared blackboard       |        Yes | Agents interact indirectly through cocode artifacts/findings. |
| Peer-to-peer group chat |         No | Deferred until richer protocol sessions exist.                |

---

## Review Workflow

### Review start from setup screen

```text
Set up review
  -> create thread
  -> create review source
  -> snapshot diff
  -> create review session
  -> append user setup message
  -> append orchestrator plan message
  -> start review workflow job
```

### Review workflow phases

```text
1. Ingest source
2. Snapshot diff
3. Build context
4. Run discovery/local indexing
5. Run reviewer CLIs in parallel
6. Normalize findings
7. Deduplicate findings
8. Verify findings
9. Build evidence maps for high-value findings
10. Draft copy packets and GitHub comments
11. Append review summary to chat
```

### Review workflow event checkpoints

Each phase emits events and can be resumed from persisted state.

```text
ReviewWorkflowStarted
ReviewSourceSnapshotted
ContextBuildStarted
ContextBundleCreated
AgentRunStarted
AgentRunCompleted
FindingCandidateCreated
FindingMerged
FindingVerified
EvidenceMapCreated
ReviewSummaryCreated
ReviewWorkflowCompleted
```

---

## Context Management

### Context bundle types

```text
ReviewContextBundle
ChatContextBundle
FindingContextBundle
EvidenceMapContextBundle
CopyPacketContextBundle
PublishContextBundle
```

### ChatContextBundle

```go
type ChatContextBundle struct {
    ThreadSummary       *ThreadSummary
    RecentMessages      []ThreadMessage
    ReviewSession       *ReviewSession
    ReviewSource        *ReviewSource
    Snapshot            *ReviewSnapshot
    ReferencedFindings  []Finding
    ReferencedMaps      []EvidenceMap
    RelevantArtifacts   []Artifact
    RelevantCode        []CodeSnippet
    UserQuestion        string
    PresetInstructions  string
    RoleInstructions    string
    OutputContract      OutputContract
    TokenEstimate       int
}
```

### Context selection rules

1. Always include the current user question.
2. Include thread summary before raw long history.
3. Include the last N recent messages, capped by token budget.
4. Include referenced finding/evidence artifacts exactly.
5. Include affected code snippets for finding follow-ups.
6. Include only relevant changed files for broad chat.
7. Include prior accepted/dismissed decisions when relevant.
8. Include the expected response schema.
9. Redact secrets before cloud/CLI prompts.
10. Record all included context item IDs.

### Context budget defaults

| Mode               |                      Budget |
| ------------------ | --------------------------: |
| Quick chat         |               8k–16k tokens |
| Finding follow-up  |              12k–25k tokens |
| Ask all agents     |    25k–50k tokens per agent |
| Deep review        | 50k+ depending on CLI/model |
| Evidence map build |                     25k–50k |

---

## Database Schema

SQLite is the Phase 1 database. Use WAL, foreign keys, and migration files.

```sql
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
```

### Projects and threads

```sql
CREATE TABLE projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  repo_path TEXT NOT NULL UNIQUE,
  provider TEXT,
  remote_url TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE threads (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  kind TEXT NOT NULL CHECK(kind IN ('review','general')),
  status TEXT NOT NULL CHECK(status IN ('draft','running','completed','failed','canceled','archived')),
  current_review_session_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX idx_threads_project_updated ON threads(project_id, updated_at DESC);
```

### Review sources and snapshots

```sql
CREATE TABLE review_sources (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK(kind IN ('github_pr','local_changes','branch_compare','commit_compare')),
  github_owner TEXT,
  github_repo TEXT,
  github_pr_number INTEGER,
  base_ref TEXT,
  head_ref TEXT,
  base_sha TEXT,
  head_sha TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE review_snapshots (
  id TEXT PRIMARY KEY,
  review_source_id TEXT NOT NULL REFERENCES review_sources(id) ON DELETE CASCADE,
  base_sha TEXT,
  head_sha TEXT,
  diff_artifact_id TEXT,
  changed_file_count INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE TABLE changed_files (
  id TEXT PRIMARY KEY,
  snapshot_id TEXT NOT NULL REFERENCES review_snapshots(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  old_path TEXT,
  status TEXT NOT NULL,
  additions INTEGER NOT NULL DEFAULT 0,
  deletions INTEGER NOT NULL DEFAULT 0,
  patch_artifact_id TEXT,
  is_excluded INTEGER NOT NULL DEFAULT 0
);
```

### Chat tables

```sql
CREATE TABLE thread_messages (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  parent_message_id TEXT REFERENCES thread_messages(id) ON DELETE SET NULL,
  author_type TEXT NOT NULL CHECK(author_type IN ('user','cocode','orchestrator','agent','system','verifier')),
  author_display_name TEXT NOT NULL,
  adapter_config_id TEXT,
  agent_run_id TEXT,
  body TEXT,
  blocks_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL CHECK(status IN ('pending','streaming','completed','failed','canceled')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX idx_thread_messages_thread_created ON thread_messages(thread_id, created_at);

CREATE TABLE message_context_refs (
  id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL REFERENCES thread_messages(id) ON DELETE CASCADE,
  ref_type TEXT NOT NULL CHECK(ref_type IN ('review_session','finding','evidence_map','artifact','file','publish_draft','copy_packet')),
  ref_id TEXT NOT NULL,
  label TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_message_context_refs_message ON message_context_refs(message_id);
CREATE INDEX idx_message_context_refs_ref ON message_context_refs(ref_type, ref_id);

CREATE TABLE chat_turns (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  user_message_id TEXT NOT NULL REFERENCES thread_messages(id) ON DELETE CASCADE,
  mode TEXT NOT NULL,
  audience TEXT NOT NULL,
  responder_adapter_config_id TEXT,
  execution_kind TEXT,
  status TEXT NOT NULL CHECK(status IN ('created','routing','context_building','running','synthesizing','completed','failed','cancel_requested','canceled')),
  context_bundle_id TEXT,
  error_code TEXT,
  error_message TEXT,
  started_at TEXT,
  completed_at TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX idx_chat_turns_thread_created ON chat_turns(thread_id, created_at);

CREATE TABLE chat_turn_agent_runs (
  chat_turn_id TEXT NOT NULL REFERENCES chat_turns(id) ON DELETE CASCADE,
  agent_run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  PRIMARY KEY(chat_turn_id, agent_run_id)
);

CREATE TABLE thread_summaries (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  up_to_message_id TEXT NOT NULL REFERENCES thread_messages(id) ON DELETE CASCADE,
  summary TEXT NOT NULL,
  facts_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
```

### Agent configs and runs

```sql
CREATE TABLE agent_configs (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  connection_kind TEXT NOT NULL,
  command TEXT,
  args_template_json TEXT NOT NULL DEFAULT '[]',
  working_dir_mode TEXT NOT NULL DEFAULT 'project_root',
  prompt_mode TEXT NOT NULL DEFAULT 'stdin',
  output_mode TEXT NOT NULL DEFAULT 'text',
  default_model_label TEXT,
  default_role_id TEXT,
  read_only INTEGER NOT NULL DEFAULT 1,
  env_allowlist_json TEXT NOT NULL DEFAULT '[]',
  timeout_seconds INTEGER NOT NULL DEFAULT 900,
  supports_resume INTEGER NOT NULL DEFAULT 0,
  health_status TEXT NOT NULL DEFAULT 'unknown',
  last_health_check_at TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE agent_external_sessions (
  id TEXT PRIMARY KEY,
  adapter_config_id TEXT NOT NULL REFERENCES agent_configs(id) ON DELETE CASCADE,
  thread_id TEXT REFERENCES threads(id) ON DELETE CASCADE,
  external_session_id TEXT,
  connection_kind TEXT NOT NULL,
  state_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE agent_runs (
  id TEXT PRIMARY KEY,
  thread_id TEXT REFERENCES threads(id) ON DELETE CASCADE,
  review_session_id TEXT,
  adapter_config_id TEXT NOT NULL REFERENCES agent_configs(id),
  external_session_id TEXT REFERENCES agent_external_sessions(id) ON DELETE SET NULL,
  role_id TEXT,
  status TEXT NOT NULL CHECK(status IN ('queued','running','completed','failed','canceled','timeout')),
  prompt_artifact_id TEXT,
  stdout_artifact_id TEXT,
  stderr_artifact_id TEXT,
  result_artifact_id TEXT,
  context_bundle_id TEXT,
  command_preview TEXT,
  exit_code INTEGER,
  duration_ms INTEGER,
  error_code TEXT,
  error_message TEXT,
  started_at TEXT,
  completed_at TEXT,
  created_at TEXT NOT NULL
);
```

### Review sessions, findings, evidence maps

```sql
CREATE TABLE review_sessions (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  review_source_id TEXT NOT NULL REFERENCES review_sources(id) ON DELETE CASCADE,
  snapshot_id TEXT REFERENCES review_snapshots(id) ON DELETE SET NULL,
  preset_ids_json TEXT NOT NULL DEFAULT '[]',
  focus_text TEXT,
  status TEXT NOT NULL CHECK(status IN ('draft','running','completed','failed','canceled')),
  total_findings INTEGER NOT NULL DEFAULT 0,
  verified_findings INTEGER NOT NULL DEFAULT 0,
  accepted_findings INTEGER NOT NULL DEFAULT 0,
  dismissed_findings INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE finding_candidates (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  agent_run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
  raw_claim TEXT NOT NULL,
  category TEXT,
  severity TEXT,
  confidence REAL,
  location_json TEXT NOT NULL DEFAULT '[]',
  evidence_json TEXT NOT NULL DEFAULT '[]',
  suggested_fix TEXT,
  draft_comment TEXT,
  raw_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE findings (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  canonical_claim TEXT NOT NULL,
  category TEXT NOT NULL,
  severity TEXT NOT NULL,
  confidence REAL NOT NULL,
  verification_status TEXT NOT NULL CHECK(verification_status IN ('unverified','verified','plausible','needs_triage','likely_false_positive','duplicate','not_actionable')),
  decision_status TEXT NOT NULL CHECK(decision_status IN ('undecided','accepted','dismissed','deferred','copied','published')),
  primary_path TEXT,
  primary_start_line INTEGER,
  primary_end_line INTEGER,
  evidence_summary TEXT,
  counter_evidence_summary TEXT,
  suggested_fix TEXT,
  draft_comment TEXT,
  fingerprint TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE finding_candidate_links (
  finding_id TEXT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
  candidate_id TEXT NOT NULL REFERENCES finding_candidates(id) ON DELETE CASCADE,
  PRIMARY KEY(finding_id, candidate_id)
);

CREATE TABLE evidence_items (
  id TEXT PRIMARY KEY,
  finding_id TEXT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK(kind IN ('supporting','counter','neutral')),
  evidence_type TEXT NOT NULL CHECK(evidence_type IN ('code','test','static_analysis','agent_claim','github_comment','config','query','migration','runtime_log')),
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  path TEXT,
  start_line INTEGER,
  end_line INTEGER,
  artifact_id TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE evidence_maps (
  id TEXT PRIMARY KEY,
  finding_id TEXT NOT NULL UNIQUE REFERENCES findings(id) ON DELETE CASCADE,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  interpretation TEXT,
  suggested_remediation TEXT,
  status TEXT NOT NULL CHECK(status IN ('draft','ready','failed')),
  graph_json_artifact_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE evidence_map_nodes (
  id TEXT PRIMARY KEY,
  evidence_map_id TEXT NOT NULL REFERENCES evidence_maps(id) ON DELETE CASCADE,
  node_key TEXT NOT NULL,
  node_type TEXT NOT NULL,
  label TEXT NOT NULL,
  path TEXT,
  start_line INTEGER,
  end_line INTEGER,
  artifact_id TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  UNIQUE(evidence_map_id, node_key)
);

CREATE TABLE evidence_map_edges (
  id TEXT PRIMARY KEY,
  evidence_map_id TEXT NOT NULL REFERENCES evidence_maps(id) ON DELETE CASCADE,
  source_node_key TEXT NOT NULL,
  target_node_key TEXT NOT NULL,
  edge_type TEXT NOT NULL,
  label TEXT,
  confidence REAL NOT NULL DEFAULT 1.0,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);
```

### Artifacts, copy packets, publishing

```sql
CREATE TABLE artifacts (
  id TEXT PRIMARY KEY,
  project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
  thread_id TEXT REFERENCES threads(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  content_type TEXT NOT NULL,
  storage_path TEXT NOT NULL,
  sha256 TEXT NOT NULL,
  byte_size INTEGER NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE copy_packets (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  finding_id TEXT REFERENCES findings(id) ON DELETE CASCADE,
  format TEXT NOT NULL CHECK(format IN ('markdown','xmlish','json','github_summary','compact')),
  content_artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
  finding_count INTEGER NOT NULL,
  copied_at TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE publish_drafts (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  body TEXT NOT NULL,
  comments_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL CHECK(status IN ('draft','ready','published','failed')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE github_publications (
  id TEXT PRIMARY KEY,
  publish_draft_id TEXT NOT NULL REFERENCES publish_drafts(id) ON DELETE CASCADE,
  github_review_id TEXT,
  github_comment_ids_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL CHECK(status IN ('pending','published','failed')),
  error_message TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

---

## Artifact Store

Use DB for metadata and filesystem for large contents.

```text
.cocode/
  artifacts/
    threads/{thread_id}/
      messages/{message_id}.json
      chat_turns/{turn_id}/
        context.md
        prompt.md
        stdout.txt
        stderr.txt
        result.json
      review_sessions/{review_session_id}/
        diff.patch
        context/
        findings/
        evidence_maps/
        copy_packets/
```

Artifact rules:

- content-address with SHA-256;
- never overwrite artifacts;
- store generated prompts;
- store raw CLI output;
- store parsed/normalized JSON separately;
- redact secrets before cloud/CLI prompts unless allowed.

---

## Backend API Contracts

### Thread APIs

```text
POST   /api/projects/open
GET    /api/projects
GET    /api/projects/:id

POST   /api/threads
GET    /api/threads?project_id=
GET    /api/threads/:id
PATCH  /api/threads/:id
POST   /api/threads/:id/archive
```

### Chat APIs

```text
GET    /api/threads/:id/messages
POST   /api/threads/:id/chat-turns
GET    /api/chat-turns/:id
POST   /api/chat-turns/:id/cancel
GET    /api/threads/:id/events
```

Request:

```json
{
  "body": "Ask all agents whether the auth finding is real.",
  "mode": "follow_up",
  "audience": "all_agents",
  "responder": {
    "adapter_config_id": null,
    "role_id": null
  },
  "context_refs": [{ "type": "finding", "id": "fnd_01" }],
  "options": {
    "include_evidence": true,
    "include_recent_messages": true
  }
}
```

Response:

```json
{
  "data": {
    "chat_turn_id": "ct_01",
    "user_message_id": "msg_01",
    "status": "created"
  },
  "error": null
}
```

### Review APIs

```text
POST   /api/review-sources
POST   /api/review-sources/:id/snapshot
POST   /api/review-sessions
POST   /api/review-sessions/:id/start
POST   /api/review-sessions/:id/cancel
GET    /api/review-sessions/:id
```

### Findings APIs

```text
GET    /api/review-sessions/:id/findings
GET    /api/findings/:id
PATCH  /api/findings/:id/decision
POST   /api/findings/:id/verify
POST   /api/findings/:id/evidence-map
```

### Evidence Map APIs

```text
GET    /api/evidence-maps/:id
POST   /api/findings/:id/evidence-map/build
GET    /api/evidence-maps/:id/graph
```

### Copy and Publish APIs

```text
POST   /api/findings/:id/copy-packet
POST   /api/review-sessions/:id/copy-packet
POST   /api/review-sessions/:id/publish-preview
POST   /api/publish-drafts/:id/publish-github
```

### Adapter APIs

```text
GET    /api/agent-configs
POST   /api/agent-configs
PATCH  /api/agent-configs/:id
POST   /api/agent-configs/:id/health-check
DELETE /api/agent-configs/:id
```

---

## SSE Event Model

Use SSE for thread-scoped updates.

```text
GET /api/threads/:id/events
```

Events:

```text
thread.message.created
thread.message.delta
thread.message.completed
chat_turn.created
chat_turn.routing
chat_turn.context_built
chat_turn.agent_run_started
chat_turn.agent_run_delta
chat_turn.agent_run_completed
chat_turn.synthesizing
chat_turn.completed
chat_turn.failed
review.workflow_started
review.context_built
review.finding_created
review.finding_verified
review.evidence_map_created
review.workflow_completed
finding.updated
copy_packet.created
publish_draft.created
github.publication_completed
error
```

SSE payload:

```json
{
  "event_id": "evt_01",
  "thread_id": "thr_01",
  "type": "chat_turn.agent_run_started",
  "created_at": "2026-05-06T10:15:00Z",
  "payload": {
    "chat_turn_id": "ct_01",
    "agent_run_id": "ar_01",
    "adapter_name": "Codex CLI"
  }
}
```

---

## Frontend Architecture

### Routes

```text
/projects/:projectId/threads/new
/projects/:projectId/threads/:threadId/chat
/projects/:projectId/threads/:threadId/findings
/projects/:projectId/threads/:threadId/findings/:findingId
/projects/:projectId/threads/:threadId/findings/:findingId/evidence-map
/projects/:projectId/threads/:threadId/publish
/settings/adapters
/settings/integrations
/settings/presets
```

### Core components

```text
AppShell
ProjectSidebar
ThreadSidebar
TopBar
ReviewSetupPage
ChatPage
ChatMessageList
ChatInputBar
AgentRunCard
FindingSummaryCard
ActivityCard
FindingsPage
FindingsTable
FindingPreviewPanel
FindingDetailPage
EvidenceMapPage
EvidenceGraph
CodeHierarchyTree
PublishPage
CopyPacketPanel
AdapterSettingsPage
```

### React state

Use a server-state client such as TanStack Query for REST data and a lightweight local store for selected UI state.

```text
Server state:
- projects
- threads
- messages
- findings
- evidence maps
- publish drafts
- adapter configs

Local UI state:
- selected finding
- selected filters
- input bar mode/audience/responder
- panel open/closed state
```

### SSE hook

```ts
export function useThreadEvents(threadId: string) {
  useEffect(() => {
    const es = new EventSource(`/api/threads/${threadId}/events`, {
      withCredentials: false,
    });

    es.addEventListener("thread.message.created", handleMessageCreated);
    es.addEventListener("thread.message.delta", handleMessageDelta);
    es.addEventListener("finding.updated", invalidateFindings);
    es.addEventListener("review.evidence_map_created", invalidateEvidenceMaps);
    es.onerror = () => markStreamDisconnected();

    return () => es.close();
  }, [threadId]);
}
```

### Central chat input behavior

The input bar sends:

```ts
type ChatInputSubmit = {
  body: string;
  mode: "review" | "follow_up" | "evidence" | "publish" | "copy";
  audience:
    | "all_agents"
    | "orchestrator"
    | "selected_agent"
    | "verifier"
    | "local_only";
  responder?: { adapterConfigId?: string; roleId?: string };
  contextRefs: Array<{ type: string; id: string }>;
};
```

When the user is on a finding detail or evidence map route, the UI automatically includes relevant `contextRefs`.

---

## Evidence Map Technical Design

### Purpose

Evidence Map lets the user validate a finding without manually scouting the codebase.

### Graph model

```go
type EvidenceGraph struct {
    Nodes []EvidenceNode `json:"nodes"`
    Edges []EvidenceEdge `json:"edges"`
}

type EvidenceNode struct {
    Key       string         `json:"key"`
    Type      string         `json:"type"`
    Label     string         `json:"label"`
    Path      string         `json:"path,omitempty"`
    StartLine *int           `json:"start_line,omitempty"`
    EndLine   *int           `json:"end_line,omitempty"`
    Status    string         `json:"status,omitempty"`
    Metadata  map[string]any `json:"metadata,omitempty"`
}

type EvidenceEdge struct {
    Source     string         `json:"source"`
    Target     string         `json:"target"`
    Type       string         `json:"type"`
    Label      string         `json:"label,omitempty"`
    Confidence float64        `json:"confidence"`
    Metadata   map[string]any `json:"metadata,omitempty"`
}
```

### Node types

```text
file
symbol
route
middleware
handler
test
query
migration
config
policy
external_service
```

### Edge types

```text
registers
calls
imports
guards
missing_guard
validates
covers
does_not_cover
reads
writes
queries
migrates
configures
depends_on
contradicts
```

### Evidence Map builder

Input:

```text
finding
evidence_items
changed files
related code snippets
static code search results
test references
agent outputs
```

Output:

```text
evidence_map
nodes
edges
interpretation
suggested remediation
```

### Graph construction algorithm

```go
func BuildEvidenceMap(ctx context.Context, finding Finding) (*EvidenceMap, error) {
    seeds := collectSeeds(finding)
    symbols := resolveSymbols(ctx, seeds)
    refs := findReferences(ctx, symbols)
    tests := findRelatedTests(ctx, symbols, finding)
    guards := detectGuardRelationships(ctx, symbols, refs)
    graph := assembleGraph(seeds, symbols, refs, tests, guards)
    graph = pruneGraph(graph, EvidenceMapPolicy{MaxNodes: 30, MaxEdges: 60})
    interpretation := explainGraph(ctx, finding, graph)
    return persistEvidenceMap(ctx, finding, graph, interpretation)
}
```

### UI behavior

- Left panel: code hierarchy and relevant files.
- Center: graph layout.
- Right panel: why this matters, evidence highlights, interpretation, suggested remediation.
- Nodes are clickable and open code references.
- Missing edges/guards use explicit edge type rather than relying only on color.

---

## Findings Technical Design

### Candidate normalization

Normalize all CLI output into `FindingCandidate`.

If the CLI output is unstructured text, use a local parser plus optional normalizer prompt.

```json
{
  "findings": [
    {
      "title": "Auth middleware skipped on billing route",
      "claim": "...",
      "category": "security",
      "severity": "high",
      "confidence": 0.92,
      "locations": [
        { "path": "src/routes/billing.ts", "start_line": 28, "end_line": 35 }
      ],
      "evidence": [],
      "suggested_fix": "...",
      "draft_comment": "..."
    }
  ]
}
```

### Deduplication

Use:

- normalized title/claim fingerprint,
- primary path/line overlap,
- category/severity similarity,
- embedding/similarity later,
- human merge overrides later.

### Verification

Verification status is assigned by:

- local code search,
- static analysis/test evidence,
- counter-evidence search,
- verifier agent if needed.

---

## Copy Packet Technical Design

### Default copy packet

```markdown
# cocode Fix Packet

Please fix ONLY the accepted findings below. Do not fix dismissed or unverified findings unless explicitly asked.

Repository:

- Project: {{project_name}}
- Review source: {{review_source}}
- Base SHA: {{base_sha}}
- Head SHA: {{head_sha}}

## Finding {{n}}: {{title}}

Severity: {{severity}}
Status: {{verification_status}}
Location: {{path}}:{{start_line}}-{{end_line}}

Claim:
{{canonical_claim}}

Evidence:
{{evidence_bullets}}

Counter-evidence:
{{counter_evidence_bullets}}

Suggested fix:
{{suggested_fix}}

Done criteria:

- The issue is fixed at the affected location.
- Relevant tests are added or updated.
- Existing tests still pass.
- No unrelated changes are made.
```

### Copy packet generation

Copy packets can be generated from:

- one finding,
- selected findings,
- all accepted findings,
- all verified high/medium findings.

---

## GitHub Publishing

Publishing is human-gated.

Flow:

```text
accepted findings
  -> create publish draft
  -> map each finding to diff
  -> preview comments
  -> user confirms
  -> submit GitHub review/comments
  -> store publication IDs
```

If diff mapping fails, show the issue and offer summary comment fallback.

---

## Prompt System

Prompts are versioned artifacts. Each prompt render must store:

- prompt template ID/version,
- role/preset IDs,
- context bundle ID,
- rendered prompt artifact ID,
- output schema.

### Orchestrator chat prompt

```text
You are cocode's review orchestrator.

Your job:
1. Understand the user's review request.
2. Decide whether to answer from existing cocode state or ask review agents.
3. Keep the response grounded in the current review thread.
4. When a finding, evidence map, file, or publish draft is relevant, reference it explicitly.
5. Do not claim a finding is verified unless verification evidence exists.
6. Prefer concise progress updates in chat; deep details belong in Findings and Evidence Map.

Available state:
{{THREAD_SUMMARY}}
{{REVIEW_SESSION_SUMMARY}}
{{RECENT_MESSAGES}}
{{REFERENCED_FINDINGS}}
{{REFERENCED_EVIDENCE_MAPS}}

User message:
{{USER_MESSAGE}}

Output requirements:
- Return Markdown for the user-facing answer.
- Include structured actions if applicable:
  - open_findings
  - open_evidence_map
  - create_copy_packet
  - create_publish_preview
  - ask_agents
```

### CLI review agent prompt

```text
You are a specialized code review agent running inside cocode.

Role:
{{ROLE_INSTRUCTIONS}}

Review source:
{{REVIEW_SOURCE}}

Focus:
{{FOCUS_TEXT}}

Context:
{{CONTEXT_BUNDLE}}

Your task:
1. Review the provided code and diff for issues matching your role.
2. Report only actionable findings.
3. Each finding must include exact file/line location when possible.
4. Each finding must include evidence and what would disprove it.
5. Do not make unsupported claims.
6. Do not edit files.
7. Do not publish comments.
8. Do not include dismissed findings unless asked.

Return JSON matching this schema:
{{FINDING_CANDIDATE_SCHEMA}}

Done criteria:
- You inspected all context items provided.
- You considered counter-evidence.
- You returned only actionable findings.
- You clearly state if no findings were found.
```

### Ask all agents synthesis prompt

```text
You are cocode's synthesis agent.

You will receive responses from multiple review agents. Your job is to produce one clear answer for the user.

User question:
{{USER_MESSAGE}}

Agent responses:
{{AGENT_RESPONSES}}

Current findings/evidence:
{{REFERENCED_CONTEXT}}

Rules:
1. Identify where agents agree.
2. Identify where agents disagree.
3. Prefer evidence over confidence.
4. Do not invent file references.
5. If evidence is insufficient, say what is missing.
6. Link the answer to findings or evidence maps when applicable.

Return:
- Short answer
- Agent consensus
- Evidence
- Open questions
- Recommended next action
```

### Finding verifier prompt

```text
You are cocode's evidence verifier.

Your job is to determine whether a finding is supported by the current codebase.

Finding:
{{FINDING}}

Evidence bundle:
{{EVIDENCE_ITEMS}}

Relevant code:
{{CODE_SNIPPETS}}

Question:
Is this finding verified, plausible, needs triage, likely false positive, duplicate, or not actionable?

Rules:
1. Use code evidence, not vibes.
2. Search for counter-evidence conceptually.
3. If a parent middleware, gateway policy, test, or config contradicts the finding, mention it.
4. Do not mark verified without specific evidence.
5. Suggest what additional evidence would be needed if uncertain.

Return JSON:
{
  "status": "verified | plausible | needs_triage | likely_false_positive | duplicate | not_actionable",
  "confidence": 0.0,
  "supporting_evidence": [],
  "counter_evidence": [],
  "summary": "",
  "recommended_action": ""
}
```

### Evidence Map builder prompt

```text
You are cocode's Evidence Map builder.

Build a graph that helps a developer verify this finding without manually scouting the codebase.

Finding:
{{FINDING}}

Evidence:
{{EVIDENCE_ITEMS}}

Relevant code:
{{CODE_SNIPPETS}}

Return JSON:
{
  "nodes": [
    {
      "key": "string",
      "type": "file|symbol|route|middleware|handler|test|query|migration|config|policy",
      "label": "string",
      "path": "string",
      "start_line": 0,
      "end_line": 0,
      "status": "supporting|counter|neutral|missing"
    }
  ],
  "edges": [
    {
      "source": "node_key",
      "target": "node_key",
      "type": "registers|calls|imports|guards|missing_guard|validates|covers|does_not_cover|reads|writes|queries|migrates|configures|depends_on|contradicts",
      "label": "string",
      "confidence": 0.0
    }
  ],
  "interpretation": "string",
  "suggested_remediation": "string",
  "evidence_highlights": []
}

Done criteria:
- Every node has a stable key.
- Every code node has path and line range when known.
- Missing relationships are represented explicitly with edge type "missing_guard" or similar.
- The interpretation explains why the graph supports or weakens the finding.
```

### Follow-up prompt

```text
You are answering a follow-up question in cocode.

User question:
{{USER_MESSAGE}}

Scoped context:
{{SCOPED_CONTEXT}}

Rules:
1. Answer only using the scoped evidence unless you explicitly say more context is needed.
2. If the user asks if a finding is true, discuss supporting and counter-evidence.
3. If the user asks what to fix, provide minimal remediation and done criteria.
4. If the user asks for uncertainty, explain what evidence is missing.
5. Do not modify code.

Return a concise but complete answer with citations to file/line references when available.
```

---

## Failure Handling

### Error categories

| Error                      | Handling                                                        |
| -------------------------- | --------------------------------------------------------------- |
| CLI missing                | Mark adapter unhealthy; show setup guidance.                    |
| CLI timeout                | Cancel process; store partial logs; show failed agent card.     |
| CLI non-zero exit          | Store stdout/stderr; show error card; keep other results.       |
| Invalid JSON               | Preserve raw output; attempt parser/normalizer once.            |
| Chat turn canceled         | Stop jobs/processes; append canceled system message.            |
| SSE disconnect             | Continue backend job; client can reconnect and reload messages. |
| Context build failure      | Mark turn failed; show files or refs that caused failure.       |
| Evidence map build failure | Finding remains usable; show map unavailable.                   |
| GitHub mapping failure     | Allow summary draft fallback.                                   |
| DB write failure           | Abort turn and surface durable error.                           |

### Partial results

If ask-all-agents runs four agents and one fails:

- show three successful agent cards,
- show one failed card,
- still run synthesis using available responses,
- include “one agent failed” in final answer.

### Cancellation

Use `context.Context` through Go job execution and `exec.CommandContext` for CLI processes. On cancellation:

1. mark chat turn `cancel_requested`;
2. cancel active contexts;
3. kill processes if necessary;
4. store partial logs;
5. append canceled message;
6. mark status `canceled`.

---

## Security

### Local backend

- Bind to `127.0.0.1`.
- Generate a per-launch auth token.
- Require token for all HTTP/SSE endpoints.
- Validate origins.
- Do not expose backend to LAN.
- Store secrets using OS-backed storage where possible.

### CLI execution

- Use argument arrays, not shell strings.
- Default to read-only review mode.
- Enforce working directory sandbox.
- Use env allowlist.
- Redact secrets in prompts.
- Cap stdout/stderr sizes.
- Require explicit approval for write mode later.

### Prompt injection

Treat source code, PR descriptions, comments, docs, and agent output as untrusted data. Prompts must separate instructions from code/data and remind agents that repository content is not allowed to override cocode instructions.

---

## Testing Strategy

### Unit tests

- chat router decisions
- context bundle builder
- thread summary builder
- CLI args renderer
- JSONL parser
- finding normalizer
- evidence map graph validator
- copy packet renderer
- diff mapper
- secret redactor

### Integration tests

- fake CLI success
- fake CLI timeout
- fake CLI invalid JSON
- ask all agents with one failure
- chat follow-up with finding context
- evidence map build
- publish preview
- SSE reconnect

### E2E tests

1. Open project.
2. Set up review.
3. Start review.
4. See chat progress.
5. Findings appear.
6. Open finding detail.
7. Ask follow-up from finding.
8. Open evidence map.
9. Copy fix packet.
10. Preview publish.
11. Restart app and resume thread.

---

## Implementation Setup

### Backend setup

```bash
cd services/cocoded
go mod init github.com/your-org/cocode/services/cocoded
go get github.com/gin-gonic/gin
go get github.com/mattn/go-sqlite3
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

### Frontend setup

```bash
cd apps/desktop
npm create electron-vite@latest
npm install
npm install @tanstack/react-query zustand react-router-dom
npx shadcn@latest init
```

### Local dev

```bash
# terminal 1
cd services/cocoded
go run ./cmd/cocoded

# terminal 2
cd apps/desktop
npm run dev
```

---

## Definition of Done

A feature is done when:

1. User-facing behavior works in the Electron UI.
2. Backend APIs are implemented and documented.
3. SQLite migrations and sqlc queries are added.
4. SSE events update the UI without manual refresh.
5. Error states are visible and recoverable.
6. Unit tests cover core logic.
7. Integration tests cover one success and one failure path.
8. Artifacts/provenance are persisted.
9. Security constraints are enforced.
10. The feature is represented in docs and task breakdown.

A chat feature is done when:

1. Message persists before execution.
2. Turn status transitions are stored.
3. Context bundle is persisted.
4. At least one CLI path works.
5. Failure produces visible message.
6. Cancellation works.
7. Resume/reload shows full history.
8. Relevant findings/evidence are linked.

An evidence-map feature is done when:

1. Map can be generated for a verified finding.
2. Nodes and edges are persisted.
3. UI renders graph and hierarchy.
4. Nodes open file/line references where known.
5. Chat can ask a question scoped to the map.
6. Failed map generation does not break finding detail.

---

## Deferred Architecture Hooks

Implement interfaces now, drivers later:

```text
ConnectionDriver
DriverSession
DriverEvent
ExternalSessionStore
ToolApprovalRequest
ProtocolCapability
```

Deferred drivers:

- Codex App Server driver
- ACP stdio driver
- MCP client driver
- A2A client driver
- provider SDK driver

The table-driven adapter settings UI can show disabled rows for future drivers, but Phase 1 should clearly mark them unavailable.
