# cocode MVP Technical Design Document (TDD)

**Product:** cocode  
**Document type:** Comprehensive technical design for MVP implementation  
**Scope:** Local-first Electron app, Go + Gin backend, SQLite, non-interactive CLI agent adapters, Evidence Map, copy fix packets, GitHub review publishing  
**Status:** Updated after latest PRD, UI mockups, and adapter scope decisions  
**Last updated:** 2026-05-02

---

## 0. Executive Summary

cocode is a local-first desktop app that orchestrates multiple coding agents for PR/code review. The MVP runs CLI agents in non-interactive mode, normalizes their outputs into evidence-backed findings, verifies those findings against the local codebase, displays each finding through text and visual Evidence Map views, and lets the user copy accepted findings into an external coding agent or publish selected comments to GitHub.

The core technical bet is a stable internal domain model:

```text
ReviewSession -> AgentRun -> FindingCandidate -> CanonicalFinding -> EvidenceBundle -> EvidenceMap -> HumanDecision -> CopyPacket/GitHubPublication
```

External agent integrations are adapters. The MVP implements only **non-interactive CLI adapters**, but the architecture must be flexible enough to add persistent JSON-RPC connections such as Codex App Server and ACP later.

---

## 1. Design Principles

The design follows these principles:

1. **Finding-first, not transcript-first.** Raw agent output is provenance; canonical findings are the primary UI/domain object.
2. **Workflow-first, not autonomous-chat-first.** PR review should follow explicit stages for reliability and debuggability.
3. **Orchestrator-mediated collaboration.** Agents collaborate by producing artifacts that cocode passes to later agents/verifiers.
4. **Evidence before comments.** A finding should not be publishable unless it has location, evidence, and an actionable explanation.
5. **CLI-first MVP.** Use non-interactive CLI adapters now; design for app-server/protocol adapters later.
6. **Local-first and human-gated.** Source, artifacts, credentials, and decisions are local by default; external side effects require approval.
7. **Done means externally observable.** Each implementation task must define done criteria, tests, and integration expectations.

---

## 2. Source and Research Foundations

### 2.1 Designing Multi-Agent Systems book

The attached book provides several design constraints for cocode:

| Book concept | cocode application |
|---|---|
| Choose the simplest architecture that satisfies the problem | MVP uses explicit workflow orchestration with CLI adapters, not autonomous multi-agent chat. |
| Workflow patterns are reliable when execution path is known | PR review stages are fixed: ingest, context, review, normalize, verify, triage, copy/publish. |
| UX needs capability discovery, cost-aware delegation, provenance, interruptibility | UI shows presets, agents, runtime settings, evidence, event timeline, pause/cancel controls. |
| Context engineering is essential for software engineering agents | Context Builder sends bounded diff, related code, tests, and evidence bundles instead of whole repo. |
| Software engineering agents need tools, prompts, memory, and completion verification | cocode provides code search, artifacts, prompts, decision memory, verifier passes, and done criteria. |
| Distributed protocols require explicit context passing | cocode centralizes artifacts and passes references/outputs between agents. |

### 2.2 Codex App Server research note

OpenAI materials describe Codex App Server as a bidirectional JSON-RPC interface used by rich Codex surfaces such as the VS Code extension and desktop/web clients. The public README for `codex app-server` describes JSON-RPC 2.0-style messages over stdio JSONL by default, with experimental WebSocket support, and OpenAI's App Server blog describes local clients launching a long-running App Server child process and keeping a bidirectional stdio JSON-RPC channel open.

**MVP decision:** do not implement Codex App Server in Phase 1. Instead, design the adapter layer so a `JSONRPCStdioConnection` can be added later without changing review workflows or UI state.

### 2.3 ACP research note

ACP standardizes communication between coding agents and clients/editors using JSON-RPC over transports such as stdio. Gemini CLI's ACP mode is a concrete example of an agent process started with a flag and controlled over stdio JSON-RPC.

**MVP decision:** do not implement ACP in Phase 1. Implement a future-ready `ProtocolConnection` interface and keep connection type separate from agent role.

### 2.4 CLI research note

Claude Code and similar CLIs support programmatic non-interactive use. Claude Code supports `-p`/`--print`, structured output formats such as JSON and stream JSON, session metadata, allowed tools, and structured output schema support. This validates the MVP approach of treating CLI agents as black-box workers.

---

## 3. System Architecture

### 3.1 High-level architecture

```text
┌──────────────────────────────────────────────────────────────────┐
│ Electron Desktop App                                              │
│                                                                  │
│ Main Process                                                      │
│ - launches cocoded Go backend                                     │
│ - owns local auth token                                           │
│ - owns clipboard access                                           │
│ - owns file dialogs                                               │
│ - stores/retrieves secrets                                        │
│                                                                  │
│ Preload                                                           │
│ - exposes narrow window.cocode API                                │
│ - validates IPC input                                             │
│                                                                  │
│ Renderer: React + TypeScript + shadcn/ui                          │
│ - New Thread                                                      │
│ - Configure Review                                                │
│ - Review Running / Chat                                           │
│ - Findings Board                                                  │
│ - Finding Detail                                                  │
│ - Evidence Map                                                    │
│ - Follow-up                                                       │
│ - Publish / Copy                                                  │
└─────────────────────────────┬────────────────────────────────────┘
                              │ REST + SSE
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│ cocoded: Local Go + Gin Backend                                   │
│                                                                  │
│ HTTP API + SSE                                                    │
│ Review Orchestrator                                               │
│ Context Builder                                                   │
│ Agent Runtime                                                     │
│ CLI Adapter                                                       │
│ Finding Engine                                                    │
│ Verification Engine                                               │
│ Evidence Map Engine                                               │
│ Follow-up Engine                                                  │
│ Copy Packet Exporter                                              │
│ GitHub Publisher                                                  │
│ SQLite + Artifact Store                                           │
└─────────────────────────────┬────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│ Local / External Integrations                                     │
│                                                                  │
│ git CLI                                                           │
│ GitHub REST API                                                   │
│ CLI agents: Codex CLI, Claude Code, Gemini CLI, OpenCode, custom  │
│ Future: Codex App Server, ACP agents, MCP servers, A2A agents     │
└──────────────────────────────────────────────────────────────────┘
```

### 3.2 Key process boundaries

| Boundary | Direction | Protocol | Notes |
|---|---|---|---|
| Renderer -> Go backend | Command/query | REST | All data operations and review control. |
| Go backend -> Renderer | Progress/events | SSE | Review timeline, agent events, findings updates. |
| Renderer -> Electron main | OS actions | Secure IPC through preload | Clipboard, file picker, open external editor. |
| Go backend -> CLI agents | Process execution | stdin/stdout/stderr | MVP adapter path. |
| Go backend -> GitHub | External API | HTTPS REST | Only after user approval. |
| Future Go backend -> Codex App Server | Long-running child process | JSON-RPC over stdio JSONL | Designed but not implemented in MVP. |
| Future Go backend -> ACP agent | Long-running child process | JSON-RPC over stdio | Designed but not implemented in MVP. |

---

## 4. Technology Stack

| Layer | Choice | Notes |
|---|---|---|
| Desktop shell | Electron | Local repo access, process lifecycle, native clipboard, OS integration. |
| Frontend | React + TypeScript + Vite | Fast iteration and strong type boundaries. |
| UI | shadcn/ui + Tailwind CSS + Radix primitives | Minimal, clean, accessible component approach. |
| Graph UI | React Flow or equivalent | Evidence Map nodes/edges with custom node renderers. |
| Code view | Monaco Editor or lightweight code viewer | Diff/code display with line highlighting. |
| Backend | Go + Gin | Local HTTP API, SSE, process orchestration, filesystem/git integrations. |
| DB | SQLite | Local-first persistence. Enable WAL. |
| DB access | sqlc + migrations | Type-safe query methods and explicit schema control. |
| Git operations | git CLI wrapper | Safer than reimplementing all Git features. |
| Search | ripgrep CLI wrapper first | Fast local code search. Tree-sitter later for richer symbols. |
| CLI execution | Go `os/exec` with context cancellation | Subprocess control and timeout handling. |
| Streaming | SSE | Review progress is primarily backend-to-frontend. |
| Packaging | electron-vite + electron-builder | Cross-platform desktop packaging. |

---

## 5. Repository and Folder Structure

Recommended monorepo structure:

```text
cocode/
  apps/
    desktop/
      electron/
        main/
        preload/
      renderer/
        src/
          app/
          components/
          features/
          routes/
          lib/
          styles/
      package.json
      electron.vite.config.ts

  services/
    cocoded/
      cmd/
        cocoded/
          main.go
      internal/
        app/
        httpapi/
        domain/
        db/
        migrations/
        git/
        github/
        artifacts/
        contextbuilder/
        orchestrator/
        agents/
        findings/
        evidence/
        exports/
        security/
        telemetry/
      go.mod
      sqlc.yaml

  packages/
    schemas/
      finding.schema.json
      agent-output.schema.json
      copy-packet.schema.json
    prompts/
      review-agent.md
      verifier-agent.md
      evidence-map-builder.md
      follow-up.md
      comment-drafter.md
      copy-packet.md

  testdata/
    repos/
      go-api-auth-bug/
      generated-files-noise/
      github-diff-anchor-cases/
    fake-agents/
      json-agent.sh
      text-agent.sh
      malformed-agent.sh

  docs/
    prd.md
    tdd.md
    task-breakdown.md
```

### 5.1 Backend package responsibilities

| Package | Responsibility |
|---|---|
| `app` | Dependency wiring, config, lifecycle, startup/shutdown. |
| `httpapi` | Gin routes, handlers, middleware, SSE stream handlers. |
| `domain` | Core typed structs/enums independent of DB and HTTP. |
| `db` | SQLite connection, migrations, sqlc generated queries. |
| `git` | Local repo validation, diff, branch compare, file content, external editor path helpers. |
| `github` | PR URL parsing, API client, diff mapping, publish preview, publish submit. |
| `artifacts` | Stores prompts, raw outputs, diffs, snippets, generated packets. |
| `contextbuilder` | Builds bounded context bundles and evidence-map context. |
| `orchestrator` | Review workflow, agent scheduling, cancellation, checkpointing, event emission. |
| `agents` | Adapter interface, CLI adapter, future connection drivers. |
| `findings` | Normalization, repair, dedupe, ranking, comment drafting. |
| `evidence` | Verification, evidence bundles, Evidence Map graph construction. |
| `exports` | Copy packet renderers and GitHub summary exporters. |
| `security` | Local auth, path sandbox, secret redaction, permission policy. |
| `telemetry` | Structured logs, metrics, traces, event audit helpers. |

---

## 6. Domain Model

### 6.1 Core entities

```text
Workspace
Repository
PullRequestSnapshot
ChangedFile
ReviewSession
ReviewSessionAgent
AgentConfig
AgentRun
ContextBundle
ContextItem
Artifact
Event
FindingCandidate
Finding
EvidenceItem
EvidenceGraph
EvidenceNode
EvidenceEdge
CallPath
FindingThread
FindingThreadMessage
HumanDecision
CopyPacket
PublishDraft
GitHubPublication
CredentialRef
ReviewRule
```

### 6.2 Status enums

```text
review_session.status:
  draft | queued | running | paused | canceling | canceled | completed | failed

agent_run.status:
  queued | running | succeeded | failed | timed_out | canceled | output_invalid

finding.verification_status:
  unverified | verified | plausible | needs_human | likely_false_positive | duplicate | not_actionable

finding.decision_status:
  undecided | accepted | dismissed | deferred | copied | published

finding.severity:
  blocker | high | medium | low | nit

artifact.kind:
  diff | prompt | raw_output | parsed_output | context_bundle | evidence | evidence_graph | copy_packet | github_preview | log
```

---

## 7. SQLite Schema v1

This schema is intentionally explicit. It keeps large text in artifacts when possible, while keeping queryable metadata in normalized tables. All timestamps are ISO-8601 UTC strings.

```sql
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA busy_timeout = 5000;

CREATE TABLE workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  root_path TEXT NOT NULL UNIQUE,
  default_repo_id TEXT,
  settings_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE repositories (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  owner TEXT,
  remote_url TEXT,
  local_path TEXT NOT NULL,
  default_branch TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(workspace_id, local_path)
);

CREATE TABLE pull_request_snapshots (
  id TEXT PRIMARY KEY,
  repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
  source_type TEXT NOT NULL CHECK(source_type IN ('github_pr','branch_compare','commit_compare','local_changes')),
  provider TEXT,
  owner TEXT,
  repo TEXT,
  pr_number INTEGER,
  pr_title TEXT,
  pr_url TEXT,
  base_ref TEXT,
  head_ref TEXT,
  base_sha TEXT,
  head_sha TEXT,
  diff_artifact_id TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE changed_files (
  id TEXT PRIMARY KEY,
  snapshot_id TEXT NOT NULL REFERENCES pull_request_snapshots(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  old_path TEXT,
  status TEXT NOT NULL,
  additions INTEGER NOT NULL DEFAULT 0,
  deletions INTEGER NOT NULL DEFAULT 0,
  is_binary INTEGER NOT NULL DEFAULT 0,
  is_generated INTEGER NOT NULL DEFAULT 0,
  is_excluded INTEGER NOT NULL DEFAULT 0,
  line_ranges_json TEXT NOT NULL DEFAULT '[]',
  patch_artifact_id TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE review_sessions (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
  snapshot_id TEXT NOT NULL REFERENCES pull_request_snapshots(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  review_depth TEXT NOT NULL DEFAULT 'standard',
  focus_prompt TEXT,
  preset TEXT,
  runtime_limit_seconds INTEGER NOT NULL DEFAULT 1800,
  context_policy_json TEXT NOT NULL DEFAULT '{}',
  started_at TEXT,
  completed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE agent_configs (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  role TEXT NOT NULL,
  adapter_kind TEXT NOT NULL CHECK(adapter_kind IN ('cli_noninteractive','jsonrpc_stdio','acp_stdio','mcp','a2a','provider_api','local_verifier')),
  command TEXT,
  args_json TEXT NOT NULL DEFAULT '[]',
  cwd_mode TEXT NOT NULL DEFAULT 'repo_root',
  env_allowlist_json TEXT NOT NULL DEFAULT '[]',
  output_mode TEXT NOT NULL DEFAULT 'text',
  model_label TEXT,
  reasoning_label TEXT,
  capabilities_json TEXT NOT NULL DEFAULT '{}',
  settings_json TEXT NOT NULL DEFAULT '{}',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE review_session_agents (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  agent_config_id TEXT NOT NULL REFERENCES agent_configs(id) ON DELETE RESTRICT,
  role TEXT NOT NULL,
  run_order INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  settings_override_json TEXT NOT NULL DEFAULT '{}',
  UNIQUE(review_session_id, agent_config_id)
);

CREATE TABLE artifacts (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  review_session_id TEXT REFERENCES review_sessions(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  relative_path TEXT NOT NULL,
  content_type TEXT NOT NULL DEFAULT 'text/plain',
  size_bytes INTEGER NOT NULL DEFAULT 0,
  sha256 TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE events (
  id TEXT PRIMARY KEY,
  review_session_id TEXT REFERENCES review_sessions(id) ON DELETE CASCADE,
  agent_run_id TEXT,
  type TEXT NOT NULL,
  level TEXT NOT NULL DEFAULT 'info',
  sequence INTEGER NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  UNIQUE(review_session_id, sequence)
);

CREATE TABLE context_bundles (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  agent_config_id TEXT REFERENCES agent_configs(id) ON DELETE SET NULL,
  scope TEXT NOT NULL CHECK(scope IN ('review','finding','evidence_map','follow_up')),
  token_estimate INTEGER NOT NULL DEFAULT 0,
  item_count INTEGER NOT NULL DEFAULT 0,
  artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  policy_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE context_items (
  id TEXT PRIMARY KEY,
  context_bundle_id TEXT NOT NULL REFERENCES context_bundles(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  path TEXT,
  start_line INTEGER,
  end_line INTEGER,
  title TEXT,
  content_artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  token_estimate INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE agent_runs (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  agent_config_id TEXT NOT NULL REFERENCES agent_configs(id) ON DELETE RESTRICT,
  context_bundle_id TEXT REFERENCES context_bundles(id) ON DELETE SET NULL,
  status TEXT NOT NULL,
  role TEXT NOT NULL,
  started_at TEXT,
  completed_at TEXT,
  duration_ms INTEGER,
  exit_code INTEGER,
  stdout_artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  stderr_artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  parsed_output_artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  error_code TEXT,
  error_message TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE finding_candidates (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  agent_run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
  raw_artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  category TEXT NOT NULL,
  severity TEXT NOT NULL,
  confidence REAL NOT NULL DEFAULT 0.5,
  claim TEXT NOT NULL,
  primary_path TEXT,
  primary_start_line INTEGER,
  primary_end_line INTEGER,
  locations_json TEXT NOT NULL DEFAULT '[]',
  evidence_json TEXT NOT NULL DEFAULT '[]',
  suggested_fix TEXT,
  draft_comment TEXT,
  fingerprint TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE findings (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  canonical_claim TEXT NOT NULL,
  category TEXT NOT NULL,
  severity TEXT NOT NULL,
  confidence REAL NOT NULL DEFAULT 0.5,
  verification_status TEXT NOT NULL DEFAULT 'unverified',
  decision_status TEXT NOT NULL DEFAULT 'undecided',
  primary_path TEXT,
  primary_start_line INTEGER,
  primary_end_line INTEGER,
  evidence_summary TEXT,
  counter_evidence_summary TEXT,
  suggested_fix TEXT,
  draft_comment TEXT,
  fingerprint TEXT NOT NULL,
  merged_from_count INTEGER NOT NULL DEFAULT 1,
  introduced_in_sha TEXT,
  first_seen_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(review_session_id, fingerprint)
);

CREATE TABLE finding_candidate_links (
  finding_id TEXT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
  finding_candidate_id TEXT NOT NULL REFERENCES finding_candidates(id) ON DELETE CASCADE,
  relation TEXT NOT NULL DEFAULT 'merged',
  PRIMARY KEY(finding_id, finding_candidate_id)
);

CREATE TABLE evidence_items (
  id TEXT PRIMARY KEY,
  finding_id TEXT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK(kind IN ('supporting','counter','neutral','missing','test','search','agent','static_analysis')),
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  path TEXT,
  start_line INTEGER,
  end_line INTEGER,
  artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  confidence REAL NOT NULL DEFAULT 0.5,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE evidence_graphs (
  id TEXT PRIMARY KEY,
  finding_id TEXT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'ready',
  layout_json TEXT NOT NULL DEFAULT '{}',
  summary TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(finding_id)
);

CREATE TABLE evidence_nodes (
  id TEXT PRIMARY KEY,
  evidence_graph_id TEXT NOT NULL REFERENCES evidence_graphs(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK(kind IN ('changed_code','related_code','middleware','guard','handler','test','config','counter_evidence','missing_guard','unknown')),
  label TEXT NOT NULL,
  path TEXT,
  symbol TEXT,
  start_line INTEGER,
  end_line INTEGER,
  evidence_item_id TEXT REFERENCES evidence_items(id) ON DELETE SET NULL,
  confidence REAL NOT NULL DEFAULT 0.5,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE evidence_edges (
  id TEXT PRIMARY KEY,
  evidence_graph_id TEXT NOT NULL REFERENCES evidence_graphs(id) ON DELETE CASCADE,
  source_node_id TEXT NOT NULL REFERENCES evidence_nodes(id) ON DELETE CASCADE,
  target_node_id TEXT NOT NULL REFERENCES evidence_nodes(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK(kind IN ('calls','mounts','protects','tests','supports','contradicts','missing_guard','imports','reads','writes','unknown')),
  status TEXT NOT NULL DEFAULT 'observed',
  label TEXT,
  confidence REAL NOT NULL DEFAULT 0.5,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE call_paths (
  id TEXT PRIMARY KEY,
  evidence_graph_id TEXT NOT NULL REFERENCES evidence_graphs(id) ON DELETE CASCADE,
  label TEXT,
  confidence REAL NOT NULL DEFAULT 0.5,
  created_at TEXT NOT NULL
);

CREATE TABLE call_path_steps (
  id TEXT PRIMARY KEY,
  call_path_id TEXT NOT NULL REFERENCES call_paths(id) ON DELETE CASCADE,
  step_index INTEGER NOT NULL,
  node_id TEXT REFERENCES evidence_nodes(id) ON DELETE SET NULL,
  path TEXT,
  start_line INTEGER,
  end_line INTEGER,
  label TEXT NOT NULL,
  UNIQUE(call_path_id, step_index)
);

CREATE TABLE finding_threads (
  id TEXT PRIMARY KEY,
  finding_id TEXT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(finding_id)
);

CREATE TABLE finding_thread_messages (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL REFERENCES finding_threads(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK(role IN ('user','assistant','system','agent')),
  agent_config_id TEXT REFERENCES agent_configs(id) ON DELETE SET NULL,
  content TEXT NOT NULL,
  evidence_refs_json TEXT NOT NULL DEFAULT '[]',
  artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE human_decisions (
  id TEXT PRIMARY KEY,
  finding_id TEXT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  decision TEXT NOT NULL CHECK(decision IN ('accepted','dismissed','deferred','copied','published','edited')),
  reason TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE copy_packets (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  finding_id TEXT REFERENCES findings(id) ON DELETE CASCADE,
  format TEXT NOT NULL CHECK(format IN ('markdown','xmlish','json','compact','github_summary')),
  content_artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
  finding_count INTEGER NOT NULL,
  token_estimate INTEGER NOT NULL DEFAULT 0,
  copied_at TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE publish_drafts (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  provider TEXT NOT NULL DEFAULT 'github',
  status TEXT NOT NULL DEFAULT 'draft',
  review_event TEXT CHECK(review_event IN ('COMMENT','REQUEST_CHANGES','APPROVE')),
  body TEXT,
  comments_json TEXT NOT NULL DEFAULT '[]',
  artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE github_publications (
  id TEXT PRIMARY KEY,
  review_session_id TEXT NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
  publish_draft_id TEXT REFERENCES publish_drafts(id) ON DELETE SET NULL,
  github_review_id TEXT,
  github_comment_ids_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL,
  error_message TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE credential_refs (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  display_name TEXT NOT NULL,
  storage_provider TEXT NOT NULL,
  storage_key TEXT NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE review_rules (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  scope TEXT NOT NULL,
  rule_type TEXT NOT NULL,
  content TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

### 7.1 Indexes

```sql
CREATE INDEX idx_changed_files_snapshot ON changed_files(snapshot_id);
CREATE INDEX idx_review_sessions_workspace ON review_sessions(workspace_id, created_at DESC);
CREATE INDEX idx_events_session_sequence ON events(review_session_id, sequence);
CREATE INDEX idx_agent_runs_session ON agent_runs(review_session_id, status);
CREATE INDEX idx_candidates_session ON finding_candidates(review_session_id);
CREATE INDEX idx_candidates_fingerprint ON finding_candidates(review_session_id, fingerprint);
CREATE INDEX idx_findings_session_status ON findings(review_session_id, decision_status, verification_status);
CREATE INDEX idx_findings_path ON findings(review_session_id, primary_path);
CREATE INDEX idx_evidence_finding ON evidence_items(finding_id, kind);
CREATE INDEX idx_evidence_nodes_graph ON evidence_nodes(evidence_graph_id, kind);
CREATE INDEX idx_evidence_edges_graph ON evidence_edges(evidence_graph_id, kind);
CREATE INDEX idx_decisions_finding ON human_decisions(finding_id, created_at DESC);
CREATE INDEX idx_copy_packets_session ON copy_packets(review_session_id, created_at DESC);
```

### 7.2 FTS tables

Use SQLite FTS5 for local search across findings and evidence:

```sql
CREATE VIRTUAL TABLE finding_search USING fts5(
  finding_id UNINDEXED,
  claim,
  evidence_summary,
  suggested_fix,
  draft_comment
);

CREATE VIRTUAL TABLE evidence_search USING fts5(
  evidence_item_id UNINDEXED,
  title,
  summary,
  path
);
```

---

## 8. Backend HTTP API

### 8.1 API envelope

All API responses use one shape:

```json
{
  "data": {},
  "error": null,
  "request_id": "req_01J..."
}
```

Error:

```json
{
  "data": null,
  "error": {
    "code": "AGENT_TIMEOUT",
    "message": "Claude Code reviewer timed out after 30 minutes.",
    "details": {
      "agent_run_id": "ar_01J..."
    }
  },
  "request_id": "req_01J..."
}
```

### 8.2 Route groups

```text
GET    /api/health
GET    /api/version

POST   /api/workspaces/open
GET    /api/workspaces
GET    /api/workspaces/:id
PATCH  /api/workspaces/:id/settings

POST   /api/pr-snapshots/from-github-url
POST   /api/pr-snapshots/from-local-compare
POST   /api/pr-snapshots/from-local-changes
GET    /api/pr-snapshots/:id
GET    /api/pr-snapshots/:id/changed-files

POST   /api/review-sessions
GET    /api/review-sessions
GET    /api/review-sessions/:id
POST   /api/review-sessions/:id/context-bundles/preview
GET    /api/review-sessions/:id/context-bundles
POST   /api/review-sessions/:id/start
GET    /api/review-sessions/:id/checkpoint
GET    /api/review-sessions/:id/summary
POST   /api/review-sessions/:id/pause
POST   /api/review-sessions/:id/resume
POST   /api/review-sessions/:id/cancel
GET    /api/review-sessions/:id/events
GET    /api/review-sessions/:id/findings

GET    /api/findings/:id
PATCH  /api/findings/:id/decision
PATCH  /api/findings/:id/draft-comment
GET    /api/findings/:id/evidence
GET    /api/review-sessions/:id/findings/:finding_id
GET    /api/review-sessions/:id/findings/:finding_id/evidence
GET    /api/review-sessions/:id/findings/:finding_id/evidence-map
POST   /api/review-sessions/:id/findings/:finding_id/evidence-map/rebuild
POST   /api/review-sessions/:id/findings/:finding_id/context-bundles/preview
POST   /api/review-sessions/:id/findings/:finding_id/evidence-map/context-bundles/preview
POST   /api/review-sessions/:id/findings/:finding_id/decision
PATCH  /api/review-sessions/:id/findings/:finding_id/draft-comment
POST   /api/findings/:id/question
POST   /api/review-sessions/:id/findings/:finding_id/question
GET    /api/findings/:id/thread
GET    /api/review-sessions/:id/findings/:finding_id/thread
GET    /api/findings/:id/evidence-map
POST   /api/findings/:id/evidence-map/rebuild
POST   /api/findings/:id/context-bundles/preview
POST   /api/findings/:id/evidence-map/context-bundles/preview

POST   /api/review-sessions/:id/export/copy-packet
POST   /api/findings/:id/export/copy-packet

POST   /api/review-sessions/:id/github/preview
POST   /api/review-sessions/:id/github/publish

GET    /api/agents/configs
POST   /api/agents/configs
PATCH  /api/agents/configs/:id
POST   /api/agents/configs/:id/test
DELETE /api/agents/configs/:id
```

### 8.3 Create review session request

```json
{
  "snapshot_id": "prs_01J...",
  "title": "PR #482 billing auth guard",
  "review_depth": "standard",
  "preset": "security_sensitive",
  "focus_prompt": "Focus on auth, billing, and data integrity.",
  "agent_config_ids": ["agent_codex", "agent_claude", "agent_gemini", "agent_local_verifier"],
  "runtime_limit_seconds": 1800,
  "context_policy": {
    "include_changed_code": true,
    "include_related_call_sites": true,
    "include_related_tests": true,
    "include_project_conventions": true,
    "redact_secrets": true,
    "local_only_paths": ["config/policy.yaml", "internal/store/ledger.go"]
  }
}
```

### 8.4 SSE event stream

Endpoint:

```text
GET /api/review-sessions/:id/events
```

Example event:

```text
id: 42
event: review.event
data: {"id":"evt_01J...","type":"AgentRunStarted","sequence":42,"payload":{"agent_run_id":"ar_01J..."}}
```

The SSE `id` is the per-session event sequence. Reconnect with `Last-Event-ID`
or `after_sequence` to replay only later events.

Event types:

```text
ReviewSessionStarted
ContextBuildStarted
ContextBundleCreated
AgentRunQueued
AgentRunStarted
AgentProgress
AgentRunSucceeded
AgentRunFailed
AgentRunCanceled
FindingCandidateCreated
FindingNormalized
FindingMerged
VerificationStarted
EvidenceAttached
EvidenceGraphBuilt
FindingVerified
HumanDecisionRecorded
CopyPacketCreated
GitHubPreviewCreated
GitHubPublicationSucceeded
GitHubPublicationFailed
ReviewSessionCompleted
ReviewSessionFailed
ReviewSessionCanceled
```

---

## 9. Review Workflow

### 9.1 Workflow phases

```text
Draft
-> IngestSnapshot
-> BuildReviewContext
-> RunReviewAgentsInParallel
-> NormalizeOutputs
-> DeduplicateFindings
-> VerifyFindings
-> BuildEvidenceMaps
-> DraftComments
-> PrepareCopyPackets
-> AwaitHumanTriage
-> CopyAndOrPublish
```

### 9.2 Workflow state machine

```text
draft -> queued -> running -> completed
                 -> paused -> running
                 -> canceling -> canceled
                 -> failed
```

### 9.3 Orchestrator pseudocode

```go
type ReviewWorkflow struct {
    Snapshots      SnapshotService
    ContextBuilder ContextBuilder
    AgentRuntime   AgentRuntime
    Findings       FindingService
    Evidence       EvidenceService
    Exports        ExportService
    Events         EventBus
}

func (w *ReviewWorkflow) Run(ctx context.Context, sessionID string) error {
    session := w.loadSession(ctx, sessionID)
    w.Events.Emit(ctx, sessionID, "ReviewSessionStarted", nil)

    snapshot := w.Snapshots.Get(ctx, session.SnapshotID)

    reviewBundle := w.ContextBuilder.BuildReviewBundle(ctx, session, snapshot)
    w.Events.Emit(ctx, sessionID, "ContextBundleCreated", map[string]any{
        "context_bundle_id": reviewBundle.ID,
    })

    runs := w.AgentRuntime.RunParallel(ctx, session, reviewBundle)

    candidates := w.Findings.NormalizeRuns(ctx, session, runs)
    findings := w.Findings.Deduplicate(ctx, session, candidates)

    for _, finding := range findings {
        w.Evidence.Verify(ctx, session, finding)
        w.Evidence.BuildEvidenceMap(ctx, session, finding)
    }

    w.Findings.DraftComments(ctx, session)
    w.Exports.PrepareDefaultPackets(ctx, session)

    w.Events.Emit(ctx, sessionID, "ReviewSessionCompleted", nil)
    return nil
}
```

---

## 10. Agent Runtime and Adapter Design

### 10.1 MVP constraint

Only non-interactive CLI execution is implemented in MVP.

That means each agent run is:

```text
render prompt/context
-> spawn command
-> write prompt to stdin or temp file
-> capture stdout/stderr
-> wait for exit or timeout/cancel
-> store artifacts
-> parse output
-> emit events
```

### 10.2 Future-ready connection model

Do not hardcode CLI execution into review workflows. Use an adapter and connection-driver split:

```go
type AgentAdapter interface {
    ID() string
    Kind() AdapterKind
    HealthCheck(ctx context.Context) (*AgentHealth, error)
    Capabilities(ctx context.Context) (*AgentCapabilities, error)
    RunTask(ctx context.Context, task AgentTask) (<-chan AgentEvent, error)
    Cancel(ctx context.Context, runID string) error
}

type ConnectionDriver interface {
    Open(ctx context.Context, cfg ConnectionConfig) (Connection, error)
}

type Connection interface {
    SendTask(ctx context.Context, task AgentTask) (<-chan AgentEvent, error)
    Close(ctx context.Context) error
}
```

MVP implementation:

```text
AgentAdapter = CLIReviewAgentAdapter
ConnectionDriver = CommandOnceDriver
Connection = CommandOnceConnection
```

Future implementations:

```text
CodexAppServerAdapter -> JSONRPCStdioDriver
ACPAgentAdapter       -> JSONRPCStdioDriver
MCPToolAdapter        -> MCPClientDriver
A2ARemoteAgentAdapter -> HTTPJSONRPCDriver
ProviderSDKAdapter    -> ProviderClientDriver
```

### 10.3 Adapter kinds

```go
type AdapterKind string

const (
    AdapterCLINonInteractive AdapterKind = "cli_noninteractive"
    AdapterJSONRPCStdio      AdapterKind = "jsonrpc_stdio"      // future
    AdapterACPStdio          AdapterKind = "acp_stdio"          // future
    AdapterMCP               AdapterKind = "mcp"                // future
    AdapterA2A               AdapterKind = "a2a"                // future
    AdapterProviderAPI       AdapterKind = "provider_api"       // future
    AdapterLocalVerifier     AdapterKind = "local_verifier"
)
```

### 10.4 CLI adapter config

```go
type CLIAdapterConfig struct {
    Command              string            `json:"command"`
    ArgsTemplate         []string          `json:"args_template"`
    WorkingDirectoryMode string            `json:"working_directory_mode"`
    EnvAllowlist         []string          `json:"env_allowlist"`
    PromptDelivery       PromptDelivery    `json:"prompt_delivery"`
    OutputMode           OutputMode        `json:"output_mode"`
    TimeoutSeconds       int               `json:"timeout_seconds"`
    MaxStdoutBytes       int64             `json:"max_stdout_bytes"`
    MaxStderrBytes       int64             `json:"max_stderr_bytes"`
    Metadata             map[string]string `json:"metadata"`
}

type PromptDelivery string
const (
    PromptViaStdin    PromptDelivery = "stdin"
    PromptViaArg      PromptDelivery = "arg"
    PromptViaTempFile PromptDelivery = "temp_file"
)

type OutputMode string
const (
    OutputText   OutputMode = "text"
    OutputJSON   OutputMode = "json"
    OutputJSONL  OutputMode = "jsonl"
    OutputNDJSON OutputMode = "ndjson"
)
```

### 10.5 CLI execution rules

Do:

```text
- Use exec.CommandContext for cancellation.
- Pass args as arrays, not shell strings.
- Prefer stdin or temp prompt file for long prompts.
- Capture stdout and stderr separately.
- Limit output size.
- Store raw outputs as artifacts.
- Parse structured output first.
- Fall back to text normalization.
- Emit events for queued, started, progress, completed, failed, canceled.
```

Avoid:

```text
- Running through shell by default.
- Passing secrets in command-line args.
- Letting review-mode CLIs write files.
- Assuming session state persists between one-shot runs.
- Depending on CLI-specific output without raw artifact fallback.
```

### 10.6 First-party CLI presets

| Agent | Adapter kind | Example strategy |
|---|---|---|
| Codex CLI | `cli_noninteractive` | User-configured non-interactive command now; future App Server connector later. |
| Claude Code | `cli_noninteractive` | `claude -p` with JSON output where configured. |
| Gemini CLI | `cli_noninteractive` initially | Future ACP mode using `gemini --acp`. |
| OpenCode | `cli_noninteractive` initially | Use run mode; future ACP/server mode. |
| Local Verifier | `local_verifier` | Deterministic Go code search/static checks, no external LLM required. |
| Custom CLI | `cli_noninteractive` | User-defined command template. |

### 10.7 Codex App Server future plan

Add later:

```go
type JSONRPCStdioConnection struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout *bufio.Scanner
    stderr *bufio.Scanner
    pending map[string]chan JSONRPCResponse
    events  chan AgentEvent
}
```

Future `CodexAppServerAdapter` will:

1. Launch `codex app-server` as long-running child process.
2. Initialize JSON-RPC session.
3. Create/resume thread.
4. Send prompt/turn.
5. Stream server notifications into cocode events.
6. Map approval requests into cocode permission UI.
7. Store Codex thread IDs in `agent_runs.metadata_json`.

The key is that this future adapter still emits the same internal `AgentEvent` and writes the same `FindingCandidate` artifacts as the CLI adapter.

### 10.8 ACP future plan

Future `ACPAgentAdapter` will:

1. Launch ACP-compatible agent as a subprocess.
2. Speak JSON-RPC over stdio.
3. Initialize/authenticate.
4. Create/load session.
5. Send prompt.
6. Consume progress/update notifications.
7. Map ACP outputs/diffs/messages to cocode artifacts.

The shared `JSONRPCStdioConnection` should support both Codex App Server and ACP. Protocol-specific logic should live above the transport.

---

## 11. Prompt Pack

All prompts are versioned artifacts. Prompt templates live in `packages/prompts` and are rendered with structured inputs. Every rendered prompt is stored as an artifact.

### 11.1 Shared review output schema

Every review agent should be instructed to return JSON when possible:
The versioned contracts live in `packages/schemas/review-agent-output.schema.json` and `packages/schemas/finding-candidate.schema.json`; backend tests validate fake agent output and normalized candidate samples against those files.

```json
{
  "summary": "short review summary",
  "findings": [
    {
      "claim": "clear issue statement",
      "category": "security|correctness|testing|reliability|performance|maintainability|api|docs|style|other",
      "severity": "blocker|high|medium|low|nit",
      "confidence": 0.0,
      "locations": [
        {
          "path": "relative/file/path.go",
          "start_line": 1,
          "end_line": 2,
          "side": "RIGHT|LEFT|UNKNOWN"
        }
      ],
      "evidence": [
        {
          "title": "evidence title",
          "summary": "why this supports the claim",
          "path": "relative/file/path.go",
          "start_line": 1,
          "end_line": 2
        }
      ],
      "counter_evidence_request": "what would disprove this claim",
      "suggested_fix": "concrete fix direction",
      "draft_comment": "concise PR comment"
    }
  ]
}
```

### 11.2 Review agent prompt

```markdown
# Role
You are a senior code reviewer participating in a multi-agent PR review.

# Task
Review the provided PR context and identify actionable findings.

# Review focus
{{review_focus}}

# Rules
- Report only issues that are actionable.
- Prefer correctness, security, reliability, data integrity, tests, and API compatibility over style.
- Do not invent files, line numbers, or behavior.
- If uncertain, lower confidence and say what evidence would be needed.
- Do not include hidden chain-of-thought. Provide concise evidence summaries only.
- Each finding must include exact file paths and line ranges when possible.
- Do not suggest fixes for dismissed or unrelated findings.

# Done criteria
Your review is done only when:
1. You inspected the changed files in the context.
2. You considered related call sites/tests included in the context.
3. You produced structured findings or an empty findings array.
4. Each finding has evidence and a suggested fix.
5. You clearly marked uncertainty when evidence is incomplete.

# Output
Return JSON matching the provided schema. No markdown outside JSON.

# PR Context
{{context_bundle}}
```

### 11.3 Security reviewer additions

```markdown
Focus on:
- authentication and authorization bypass
- missing tenant/user scoping
- secrets and sensitive data exposure
- injection risks
- unsafe deserialization
- webhook verification
- permission/role checks
- audit logging gaps

Raise severity based on exploitability and impact. If the issue depends on deployment assumptions, mark it needs-human or plausible instead of verified.
```

### 11.4 Testing reviewer additions

```markdown
Focus on:
- missing regression tests for changed behavior
- weak assertions
- tests that do not fail before the fix
- missing negative tests
- untested edge cases
- test names that do not match behavior

Do not ask for tests just for coverage. Tie each test recommendation to a concrete risk.
```

### 11.5 Finding normalizer prompt

Used only if CLI output is text or malformed.

```markdown
# Role
You convert raw code review text into structured finding candidates.

# Rules
- Do not create new findings that are not present in the raw output.
- Preserve uncertainty.
- Use exact file/line locations only if present or obvious from cited snippets.
- If a finding lacks location, include it with low confidence and location unknown.
- Return JSON only.

# Schema
{{finding_schema}}

# Raw output
{{raw_output}}
```

### 11.6 Deduper/synthesizer prompt

```markdown
# Role
You merge duplicate finding candidates into canonical findings.

# Rules
- Merge candidates that describe the same underlying issue, even if wording differs.
- Preserve all agent provenance.
- Prefer the strongest evidence and most precise location.
- Do not merge separate issues just because they are in the same file.
- Return canonical findings as JSON.

# Candidates
{{finding_candidates}}
```

### 11.7 Verifier prompt

```markdown
# Role
You are a verifier. Your job is to decide whether a finding is supported by the provided code evidence.

# Task
Verify or refute the finding using only the provided evidence bundle and allowed local search results.

# Possible statuses
- verified: evidence supports the claim.
- plausible: claim may be true but proof is incomplete.
- needs_human: depends on product intent or external assumptions.
- likely_false_positive: evidence contradicts the claim.
- not_actionable: too vague or not useful.

# Rules
- Cite files and line ranges.
- Look for counter-evidence.
- Do not rely only on another agent's claim.
- Do not include hidden chain-of-thought.
- Provide concise reasoning summary.

# Done criteria
The verification is done only when:
1. You checked the primary location.
2. You checked at least one likely counter-evidence path if available.
3. You assigned exactly one status.
4. You returned supporting and counter-evidence lists.

# Finding
{{finding}}

# Evidence bundle
{{evidence_bundle}}
```

### 11.8 Evidence Map builder prompt

```markdown
# Role
You create an evidence graph for a code review finding.

# Task
From the finding and evidence bundle, create nodes and edges that help a developer verify the claim visually.

# Node kinds
changed_code, related_code, middleware, guard, handler, test, config, counter_evidence, missing_guard, unknown

# Edge kinds
calls, mounts, protects, tests, supports, contradicts, missing_guard, imports, reads, writes, unknown

# Rules
- Every node should reference a real file and line range when possible.
- Use missing_guard only when absence of a relationship is central to the claim.
- Use contradicts for counter-evidence.
- Include a call path when possible.
- If the graph is incomplete, mark missing pieces clearly instead of inventing nodes.
- Return JSON only.

# Done criteria
The graph is done only when:
1. It contains the primary changed code node.
2. It contains at least one supporting or counter-evidence node when available.
3. It contains edges explaining the relationship between nodes.
4. It includes a call path or a reason why no call path is available.

# Finding
{{finding}}

# Evidence bundle
{{evidence_bundle}}
```

### 11.9 Follow-up prompt

```markdown
# Role
You answer follow-up questions about one code review finding.

# Grounding
Use the evidence bundle first. Do not use unrelated repository assumptions.

# Rules
- Answer the user's question directly.
- Cite files and lines where possible.
- Say when evidence is insufficient.
- Suggest a verifier/search action if more context is needed.
- Do not modify code.

# Finding
{{finding}}

# Evidence bundle in use
{{evidence_bundle}}

# User question
{{question}}
```

The MVP follow-up foundation creates exactly one thread per finding with `finding_threads.UNIQUE(finding_id)`. `GET /api/findings/:id/thread` and the review-session-scoped equivalent create the thread on first load, return the finding summary plus ordered messages, and preserve existing messages across reloads. Message rows keep role, optional agent config, optional artifact, and JSON evidence references.

`POST /api/findings/:id/question` appends the user question, builds a persisted finding-scoped context bundle, runs a selected `cli_noninteractive` agent or deterministic `local_verifier`, and stores the assistant answer with cited evidence refs plus the stdout artifact for CLI runs. The endpoint accepts an optional context policy override and rejects unsupported/disabled agent configs before execution.

### 11.10 GitHub comment drafter prompt

```markdown
# Role
You draft concise GitHub PR review comments.

# Rules
- One issue per comment.
- Be professional and specific.
- Explain why it matters.
- Include a concrete fix direction.
- Avoid overclaiming.
- Do not mention internal agent names unless useful.
- Do not include dismissed, false-positive, or unverified findings.

# Finding
{{finding}}

# Evidence
{{evidence_bundle}}
```

### 11.11 Copy fix packet prompt/template

Copy packets can be generated deterministically from templates, not by an LLM. Default Markdown template:

```markdown
# Fix accepted PR review findings

You are working in the same repository. Fix ONLY the accepted findings below.
Do not address dismissed, deferred, unverified, or likely-false-positive findings.
Prefer minimal, idiomatic changes. Add or update tests when the finding asks for it.

Repository snapshot:
- Repository: {{repo_full_name}}
- PR: {{pr_number_or_compare}}
- Base SHA: {{base_sha}}
- Head SHA: {{head_sha}}
- Review session: {{review_session_id}}

{{#findings}}
## Finding {{index}}: {{title}}

Severity: {{severity}}
Status: {{verification_status}}
Category: {{category}}
Location: {{primary_path}}:{{primary_start_line}}-{{primary_end_line}}

Claim:
{{canonical_claim}}

Evidence:
{{evidence_items}}

Counter-evidence:
{{counter_evidence_items}}

Expected fix:
{{suggested_fix}}

Acceptance criteria:
- The finding is fixed at the affected location or a better equivalent location.
- Relevant existing tests still pass.
- Add or update regression tests if the finding is about behavior or safety.
- Do not introduce unrelated refactors.
{{/findings}}
```

---

## 12. Context Builder

### 12.1 Input sources

| Source | Purpose |
|---|---|
| PR diff | Primary review target. |
| Full changed files | Needed when changed file is small or hunk context is insufficient. |
| Surrounding code | Avoid diff-only false positives. |
| Related call sites | Show downstream impact. |
| Related tests | Determine coverage and behavior. |
| Project conventions | Align with repo style. |
| Prior comments | Avoid duplicates. |
| Prior decisions | Reduce repeated false positives. |
| Static analysis/test output | Deterministic signals. |

### 12.2 Context bundle scopes

```text
review: initial multi-agent review
finding: finding detail/follow-up
evidence_map: graph generation and verifier re-check
follow_up: user question scoped to one finding
```

### 12.3 Token budget rules

Start simple:

| Review depth | Approx budget strategy |
|---|---|
| Quick | Diff + hunk context only. |
| Standard | Diff + changed files + related tests/call sites. |
| Deep | Standard + broader references + project conventions + prior comments. |

Avoid whole-repo context by default. Store every context item and bundle so users can audit what each agent saw.

---

## 13. Finding Engine

### 13.1 Normalization pipeline

```text
RawAgentOutput
-> structured parser if JSON/JSONL
-> text normalizer if needed
-> schema validation
-> location normalization
-> fingerprint generation
-> FindingCandidate records
```

The MVP implementation persists structured JSON and JSONL/NDJSON candidates during the `normalize_outputs` phase. Each candidate keeps the agent run ID plus the raw stdout artifact as provenance, while `FindingCandidateCreated` events allow the running review stream to surface candidates before the whole workflow completes.
When output is not structured, candidate extraction first applies one deterministic repair pass for simple malformed JSON such as trailing commas. If repair fails, it falls back to a conservative text candidate with low confidence and raw-output evidence.
Candidate persistence normalizes paths against the snapshot changed-file list, records changed-file IDs and validity messages in `locations_json`, and computes an app-owned fingerprint. The dedupe phase currently merges exact fingerprints and overlapping same-category candidates with similar claim terms, then materializes canonical `findings` rows and candidate links.

### 13.2 Fingerprint strategy

Fingerprint input:

```text
normalized category
normalized claim key terms
primary path
primary line range bucket
suggested issue type
```

Do not use exact prose as fingerprint. Different agents will describe the same issue differently.

### 13.3 Deduplication strategy

MVP dedupe:

1. Exact fingerprint match.
2. Same file and overlapping line ranges.
3. Similar normalized claims using token similarity.
4. Optional LLM dedupe pass for ambiguous cases.

### 13.4 Ranking

Ranking score:

```text
severity_weight
+ verification_weight
+ agent_agreement_weight
+ evidence_count_weight
- false_positive_risk
- nit_penalty
```

---

## 14. Verification and Evidence Engine

### 14.1 Verification steps

For each finding:

1. Load primary changed file and hunk.
2. Search for cited symbols/guards/functions.
3. Search for counter-evidence.
4. Attach evidence items.
5. Run Local Verifier rules where applicable.
6. Optionally ask a CLI verifier agent.
7. Assign verification status.
8. Build Evidence Map.

The MVP implementation runs a deterministic local verifier during the `verify_findings` workflow phase. It rebuilds cocode-owned evidence items for each canonical finding, reads the primary changed-code snippet inside the repository sandbox, searches likely guard/config/test paths with a bounded `rg --json` wrapper, stores supporting/counter/missing/test evidence rows, and updates `verification_status`, `evidence_summary`, and `counter_evidence_summary`. The local verifier starts with rule profiles for auth guards, webhook validation, test coverage, and idempotency so search terms and evidence metadata are predictable.

Finding-scoped and Evidence Map-scoped context bundles reuse the same context item model with `scope` set to `finding` or `evidence_map`. They include finding prompt material, bounded evidence rows, scoped changed-code snippets, related tests/search hits when available, and optionally a compact Evidence Map graph summary. Scoped bundles cap tokens/items below the broad review defaults so follow-up/verifier tasks stay fast on large PRs.

The verifier agent runner is an optional extension after deterministic local verification. It selects enabled `cli_noninteractive` agent configs whose role is `verifier`, builds persisted finding-scoped bundles, and asks the CLI to return a single JSON verification result. The runner caps verifier configs and finding count for large diffs, stores verifier-provided evidence with `producer=verifier_agent`, and may update `verification_status`, `evidence_summary`, and `counter_evidence_summary`. CLI failures, invalid verifier output, and per-finding context failures are recorded as warning events and do not remove or block local verifier evidence.

### 14.2 Local verifier examples

| Finding type | Deterministic checks |
|---|---|
| Missing auth middleware | Search route setup, parent groups, middleware registration, handler auth calls. |
| Missing webhook validation | Search signature verification, event type validation, test coverage. |
| Missing tests | Search related test files and assertion patterns. |
| Potential nil dereference | Search guard clauses and call paths. |
| Duplicate idempotency risk | Search idempotency keys and storage constraints. |

### 14.3 Verification status assignment

| Status | Criteria |
|---|---|
| verified | Supporting evidence exists and counter-evidence does not refute it. |
| plausible | Some evidence exists but important context is missing. |
| needs_human | Depends on product intent, external contract, or business rule. |
| likely_false_positive | Counter-evidence contradicts the claim. |
| duplicate | Same issue already represented by another finding. |
| not_actionable | Too vague, no location, no concrete fix, or pure preference. |

The app stores the canonical false-positive-like status as `likely_false_positive` so the UI and seed data use one stable enum value.

---

## 15. Evidence Map Design

### 15.1 UI layout

Evidence Map uses the latest mockup structure:

```text
Left panel: Code hierarchy
Center panel: Evidence graph
Right panel: Finding claim/evidence/action panel
Bottom strip: Call path
```

### 15.2 Node kinds

| Kind | Example |
|---|---|
| changed_code | `routes/billing.go L118-L142` |
| related_code | `router/setup.go L34-L67` |
| middleware | `middleware/auth.go L22-L48` |
| guard | `RequireAuth` |
| handler | `handlers/payouts.go L210-L268` |
| test | `tests/billing_routes_test.go L45-L102` |
| config | `config/gateway.yaml L42-L48` |
| counter_evidence | Gateway-level auth rule. |
| missing_guard | Red dashed missing guard relationship. |

### 15.3 Edge kinds

| Kind | Visual treatment | Meaning |
|---|---|---|
| calls | Solid arrow | Execution or function call path. |
| mounts | Solid arrow | Router/group setup. |
| protects | Solid green edge | Guard/middleware protects target. |
| tests | Solid yellow edge | Test covers code path. |
| supports | Solid/green relation | Evidence supports claim. |
| contradicts | Orange/red relation | Counter-evidence weakens claim. |
| missing_guard | Red dashed edge with X | Expected protection edge is missing. |

### 15.4 Graph generation algorithm

```text
Input: Finding + EvidenceBundle + ContextItems

1. Create primary changed-code node from finding location.
2. Add evidence item nodes from supporting evidence.
3. Add counter-evidence nodes.
4. Use local code-map/search to add related setup, middleware, handler, tests.
5. Create observed relationship edges.
6. Create missing relationship edges only when absence is the claim.
7. Build shortest readable call path.
8. Persist graph nodes/edges/call path.
9. Return graph view model to UI.
```

The backend graph builder keeps the first response bounded for large PRs: evidence items are prioritized by kind/confidence, graph evidence nodes are capped, omitted counts are recorded in `layout_json`, and raw snippets stay on evidence-item detail APIs instead of being duplicated into every graph node.

### 15.5 Evidence Map API response

```json
{
  "finding": {
    "id": "f_01J...",
    "canonical_claim": "Auth middleware skipped on billing route",
    "severity": "high",
    "verification_status": "verified"
  },
  "graph": {
    "id": "evidence_graph_...",
    "status": "ready",
    "summary": "Evidence map for auth middleware skipped..."
  },
  "hierarchy": [
    {
      "path": "api/routes/billing.go",
      "start_line": 118,
      "end_line": 142,
      "kind": "changed_code",
      "node_ids": ["n1"]
    }
  ],
  "nodes": [
    {
      "id": "n1",
      "kind": "changed_code",
      "label": "Billing route",
      "path": "api/routes/billing.go",
      "start_line": 118,
      "end_line": 142,
      "deep_link": {
        "kind": "file",
        "path": "api/routes/billing.go",
        "start_line": 118,
        "end_line": 142
      }
    }
  ],
  "edges": [
    {
      "id": "e1",
      "source": "n1",
      "target": "n2",
      "kind": "missing_guard",
      "status": "missing",
      "label": "RequireAuth not attached"
    }
  ],
  "call_path": [
    {
      "path": "router/setup.go",
      "start_line": 34,
      "label": "router setup"
    },
    {
      "path": "routes/billing.go",
      "start_line": 132,
      "label": "billing route"
    },
    {
      "path": "handlers/payouts.go",
      "start_line": 210,
      "label": "billing handler"
    }
  ],
  "call_path_unavailable_reason": "",
  "missing_reasons": [],
  "panel": {
    "claim": "Auth middleware skipped on billing route",
    "evidence_counts": {
      "supporting": 1,
      "counter": 0,
      "test": 1
    }
  }
}
```

### 15.6 Evidence Map done criteria

Evidence Map is done when:

1. It can load for any finding, even if graph data is incomplete.
2. It shows at least the primary location node when a location exists.
3. It shows supporting and counter-evidence nodes when evidence exists.
4. It shows a call path or a clear “call path unavailable” reason.
5. It deep-links nodes to file/line views.
6. It visually distinguishes missing, supporting, and contradicting relationships.
7. It has test coverage for graph response mapping and UI rendering states.

---

## 16. Copy Fix Packet

### 16.1 Formats

| Format | Purpose |
|---|---|
| Markdown | Default paste into coding agent. |
| XML-ish | More structured prompt for agents that follow tags well. |
| JSON | Machine-readable handoff. |
| Compact | Lower-token chat windows. |
| GitHub summary | Manual PR review summary. |

### 16.2 API

```text
POST /api/review-sessions/:id/export/copy-packet
POST /api/findings/:id/export/copy-packet
```

Request:

```json
{
  "format": "markdown",
  "finding_ids": ["f_01J...", "f_01K..."],
  "include_code_snippets": true,
  "include_evidence": true,
  "include_counter_evidence": true,
  "target_agent": "codex_cli"
}
```

Response:

```json
{
  "data": {
    "copy_packet_id": "cp_01J...",
    "content": "# Fix accepted PR review findings\n...",
    "finding_count": 3,
    "token_estimate": 2800
  },
  "error": null,
  "request_id": "req_01J..."
}
```

### 16.3 Clipboard flow

```text
Renderer requests packet from backend
-> backend renders packet and stores artifact
-> renderer sends text to preload API
-> Electron main writes to clipboard
-> backend marks copy_packet copied_at and finding decision copied
```

---

## 17. GitHub Publisher

### 17.1 Publishing modes

| Mode | Behavior |
|---|---|
| Preview | Build review payload only. |
| Save pending draft | Create pending review if implemented. |
| Publish comment review | Submit selected comments with COMMENT. |
| Request changes | Submit selected comments with REQUEST_CHANGES. |
| Summary only | Publish review body without inline comments. |

### 17.2 Diff mapping

GitHub inline comments require mapping to PR diff lines/positions. Store diff snapshot and changed file patches so mappings are reproducible.

If mapping fails:

1. Mark comment as unanchored.
2. Show warning in Publish tab.
3. Offer summary-only fallback.
4. Do not publish misplaced comments silently.

---

## 18. Frontend Architecture

### 18.1 Renderer routes

```text
/workspaces
/thread/new
/threads/:threadId
/threads/:threadId/configure
/threads/:threadId/chat
/threads/:threadId/findings
/threads/:threadId/findings/:findingId
/threads/:threadId/findings/:findingId/evidence-map
/threads/:threadId/findings/:findingId/follow-up
/threads/:threadId/publish
/settings/agents
/settings/github
/settings/security
```

### 18.2 Feature modules

```text
features/workspaces
features/thread
features/configure-review
features/review-running
features/findings
features/evidence-map
features/follow-up
features/publish
features/agents
features/settings
```

### 18.3 State management

Use React Query for server state. Use lightweight local state for UI-only state.

```text
Server state:
- workspaces
- snapshots
- review sessions
- events
- findings
- evidence maps
- publish previews

Local UI state:
- selected tabs
- expanded panels
- graph layout positions
- unsaved comment edits
- selected findings for copy/publish
```

### 18.4 Screens and components

| Screen | Key components |
|---|---|
| New Thread | Source selector, PR URL input, branch compare form, suggested setup, prompt composer. |
| Configure Review | Changed files card, agents card, policies/context card, CLI runtime table, start button. |
| Review Running | Progress summary, agent cards, early findings, chat composer, pause/cancel controls. |
| Findings Board | Summary stats, filters, finding list, selected finding preview, actions. |
| Finding Detail | Code tabs, evidence list, consensus panel, comment draft, suggested fix, copy/accept/dismiss. |
| Evidence Map | Code hierarchy, React Flow graph, right panel, call path strip. |
| Follow-up | Evidence bundle chips, chat messages, selected agents, quick actions. |
| Publish | Accepted finding selector, GitHub preview, Copy Fix Packet preview, final checklist. |

---

## 19. Electron Security Design

### 19.1 BrowserWindow defaults

```ts
new BrowserWindow({
  webPreferences: {
    preload: preloadPath,
    nodeIntegration: false,
    contextIsolation: true,
    sandbox: true,
  },
})
```

### 19.2 Preload API

Expose only narrow APIs:

```ts
contextBridge.exposeInMainWorld("cocode", {
  copyText: (text: string) => ipcRenderer.invoke("clipboard:writeText", text),
  selectRepository: () => ipcRenderer.invoke("dialog:selectRepository"),
  openExternalEditor: (payload: OpenEditorPayload) => ipcRenderer.invoke("editor:open", payload),
  getBackendInfo: () => ipcRenderer.invoke("backend:getInfo"),
})
```

Never expose raw `ipcRenderer`.

### 19.3 Backend local security

```text
- Bind to 127.0.0.1 only.
- Generate per-launch auth token.
- Require auth token on every request.
- Validate Origin for browser-origin requests.
- Reject non-localhost requests.
- Scope filesystem reads to workspace.
- Store artifacts under app-managed directory.
```

---

## 20. Error Handling

### 20.1 Error code taxonomy

```text
WORKSPACE_NOT_FOUND
WORKSPACE_INVALID_GIT_REPO
GIT_COMMAND_FAILED
PR_URL_INVALID
PR_FETCH_FAILED
CONTEXT_BUILD_FAILED
CONTEXT_BUDGET_EXCEEDED
AGENT_NOT_CONFIGURED
AGENT_HEALTHCHECK_FAILED
AGENT_PROCESS_FAILED
AGENT_TIMEOUT
AGENT_CANCELED
AGENT_OUTPUT_INVALID
FINDING_NORMALIZATION_FAILED
VERIFICATION_FAILED
EVIDENCE_MAP_FAILED
COPY_PACKET_FAILED
GITHUB_AUTH_FAILED
GITHUB_ANCHOR_FAILED
GITHUB_PUBLISH_FAILED
PERMISSION_DENIED
SECRET_DETECTED
LOCAL_BACKEND_AUTH_FAILED
```

### 20.2 Recovery behavior

| Error | Recovery |
|---|---|
| One agent fails | Preserve other agent results and mark agent failed. |
| Invalid output | Attempt one repair; if failed, store raw output and mark invalid. |
| Timeout | Cancel process, store partial logs, keep session usable. |
| Evidence Map fails | Show textual evidence and “graph unavailable” state. |
| GitHub anchor fails | Offer summary comment fallback. |
| Copy fails | Show packet text in modal for manual copy. |
| PR changed | Warn and offer refresh snapshot. |
| Secret detected | Redact and show redaction report. |

---

## 21. Testing Strategy

### 21.1 Test levels

| Level | Purpose |
|---|---|
| Unit | Pure logic: diff mapping, parsing, schema validation, redaction, copy rendering. |
| Integration | Backend with fake repo, fake CLI, fake GitHub server. |
| Contract | Adapter contracts, API payloads, Evidence Map response shape, GitHub publish payload. |
| E2E | Electron app flow from new thread to copy/publish with fake agents. |
| Evaluation | Golden repos and historical PRs to measure finding quality. |

### 21.2 Golden repos

```text
go-api-auth-bug
webhook-validation-bug
missing-test-bug
large-diff-context-budget
generated-files-noise
github-diff-anchor-cases
```

### 21.3 Agent testing

Use fake CLI agents:

| Fake agent | Behavior |
|---|---|
| json-agent | Returns valid schema. |
| text-agent | Returns human prose requiring normalization. |
| malformed-agent | Returns broken JSON to test repair. |
| slow-agent | Sleeps to test timeout/cancel. |
| stderr-agent | Writes logs to stderr while succeeding. |
| failing-agent | Exits non-zero. |

### 21.4 Done criteria for implementation tasks

A task is done only when:

1. The user-visible or API-visible behavior works.
2. The relevant DB state or artifact is persisted.
3. Expected events are emitted if the feature participates in review flow.
4. Error states are handled.
5. Tests cover success and at least one failure path.
6. Security constraints are respected.
7. Documentation or inline comments exist for non-obvious behavior.
8. It integrates with dependent screens/workflows.

---

## 22. MVP Milestones

```text
M0: Repo skeleton and local backend launch
M1: Workspace, PR ingestion, DB, artifacts
M2: CLI agent runtime and review orchestration
M3: Finding normalization, dedupe, verification
M4: Findings Board and Finding Detail UI
M5: Evidence Map engine and screen
M6: Follow-up Q&A and copy fix packets
M7: GitHub preview/publish
M8: Security, testing, packaging, dogfood eval
```

---

## 23. Future Extensions

| Extension | Dependency |
|---|---|
| Codex App Server connector | `JSONRPCStdioConnection`, adapter capability mapping, approval event mapping. |
| ACP connector | `JSONRPCStdioConnection`, ACP session lifecycle, event mapping. |
| MCP tool servers | Permission UI, tool registry, MCP client lifecycle. |
| A2A remote agents | HTTP JSON-RPC/SSE client, task lifecycle, auth model. |
| In-app fixing | Worktree management, patch preview, tests, file write permissions. |
| Cloud/team mode | GitHub App, containerized workers, tenant isolation, centralized secrets. |

---

## 24. Source Notes

This TDD is grounded in:

- `Designing-Multi-Agent-Systems.pdf`, especially chapters on multi-agent patterns, UX principles, workflow building, modern agent UIs, evaluation, distributed protocols, and software-engineering agents.
- OpenAI Codex App Server blog and open-source README for future JSON-RPC app-server direction.
- Agent Client Protocol documentation for future editor/agent protocol direction.
- Claude Code, Gemini CLI, OpenCode, and related CLI automation documentation for CLI adapter assumptions.
- GitHub REST API documentation for PR review publishing.
- Electron security documentation for local desktop process isolation.
- SQLite, sqlc, and Gin documentation for local backend implementation choices.

---

## 25. Reference URLs

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
