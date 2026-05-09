# cocode Review Presets and Roles — Centralized Chat Update

**Date:** 2026-05-06  
**Status:** Updated for centralized chat and Evidence Map MVP

---

## Table of Contents

1. [Preset Model](#preset-model)
2. [How Presets Interact With Central Chat](#how-presets-interact-with-central-chat)
3. [Recommended MVP Presets](#recommended-mvp-presets)
4. [Role Catalog](#role-catalog)
5. [Preset YAML Shape](#preset-yaml-shape)
6. [Role Prompt Shape](#role-prompt-shape)
7. [Evidence Map Requirements](#evidence-map-requirements)
8. [Chat Behavior by Role](#chat-behavior-by-role)
9. [Done Criteria for Agent Outputs](#done-criteria-for-agent-outputs)

---

## Preset Model

A cocode preset is a declarative package:

```text
preset =
  review goal
  + selected roles
  + role instructions
  + knowledge packs
  + context policy
  + verification policy
  + evidence-map policy
  + output policy
  + chat behavior policy
```

Presets are data, not hardcoded logic.

---

## How Presets Interact With Central Chat

Central chat does not remove presets. Instead, presets inform:

- how the initial review workflow runs;
- what agents are selected by default;
- what context is gathered;
- how follow-up questions are routed;
- how evidence maps are built;
- how copy packets are written.

Example:

```text
User: "Could this auth bug be a false positive?"
Context: finding_id=fnd_auth_01
Preset: Security & Auth Focus

Router:
  -> local verifier first
  -> ask AuthZ/Tenant Isolation reviewer if uncertainty remains
  -> optionally ask Counter-Evidence Skeptic
  -> synthesize answer into central chat
```

---

## Recommended MVP Presets

| Preset                             | Purpose                                               | Default roles                                                                          |
| ---------------------------------- | ----------------------------------------------------- | -------------------------------------------------------------------------------------- |
| Standard PR Review                 | Everyday balanced review                              | Context Discovery, Go Correctness, Security, Testing, Evidence Verifier, Synthesizer   |
| Security & Auth Focus              | Auth, middleware, route protection, data exposure     | Security Reviewer, AuthZ/Tenant Isolation, Counter-Evidence Skeptic, Evidence Verifier |
| Go Performance Deep Dive           | CPU, memory, allocations, hot paths, I/O, concurrency | Go Performance, Go Concurrency, Postgres Query Performance, Evidence Verifier          |
| PostgreSQL Query Performance       | Queries, indexes, pagination, N+1, scans              | Postgres Query Performance, Data Integrity, Evidence Verifier                          |
| PostgreSQL Migration Safety        | Schema changes, lock risk, backfills, compatibility   | Migration Safety, Data Integrity, Query Performance, Evidence Verifier                 |
| Data Integrity & Transactions      | Money, ledgers, orders, idempotency, transactions     | Data Integrity, Postgres Migration Safety, Go Correctness, Evidence Verifier           |
| Reliability & Production Readiness | Timeouts, retries, workers, external services         | Reliability, Go Concurrency, Observability, Testing                                    |
| Testing & Regression Coverage      | Missing tests and weak regression coverage            | Testing, Go Correctness, Security when relevant                                        |
| API Compatibility & Client Impact  | Breaking API behavior, response contracts             | API Compatibility, Testing, Evidence Verifier                                          |
| Privacy & Sensitive Data           | PII, logging, exports, secrets                        | Security, Privacy, Observability, Evidence Verifier                                    |

---

## Role Catalog

### Context Discovery Agent

Goal: build neutral context for reviewers.

Must find:

- changed files,
- relevant call sites,
- related tests,
- route/middleware relationships,
- query/migration relationships,
- config/policy files,
- prior comments,
- prior dismissals.

Output:

- context item list,
- evidence seeds,
- uncertainty list.

### Go Correctness & Idioms Reviewer

Checks:

- error handling,
- nil handling,
- context cancellation,
- resource cleanup,
- API correctness,
- goroutine lifecycle,
- panic/recover misuse,
- Go idioms.

### Go Performance Reviewer

Checks:

- allocations,
- unnecessary conversions,
- large copies,
- hot path regressions,
- inefficient serialization,
- repeated work,
- DB calls in loops,
- caching risks.

### Go Concurrency Reviewer

Checks:

- race risks,
- goroutine leaks,
- channel deadlocks,
- lock ordering,
- context propagation,
- worker shutdown.

### PostgreSQL Query Performance Reviewer

Checks:

- missing/wrong indexes,
- N+1 queries,
- unbounded scans,
- OFFSET pagination,
- expensive aggregates,
- over-fetching,
- query builder regressions,
- plan stability.

### PostgreSQL Migration Safety Reviewer

Checks:

- table rewrite risk,
- blocking locks,
- concurrent index needs,
- backfill strategy,
- constraint validation,
- rollback compatibility,
- app/schema compatibility during deploy.

### Security Reviewer

Checks:

- auth/authz,
- injection,
- secrets,
- SSRF,
- command injection,
- path traversal,
- logging exposure,
- unsafe deserialization,
- webhook validation.

### AuthZ & Tenant Isolation Reviewer

Checks route-to-data path:

```text
router -> middleware -> handler -> service -> query -> response
```

Must verify:

- caller identity,
- ownership checks,
- tenant scoping,
- admin bypass rules,
- test coverage.

### Testing & Regression Reviewer

Checks:

- missing regression tests,
- negative tests,
- boundary tests,
- concurrency tests,
- migration tests,
- security tests.

### Evidence Verifier

Checks whether each finding is true using evidence and counter-evidence.

### Counter-Evidence Skeptic

Actively attempts to disprove high-impact findings before they are marked verified.

### Finding Synthesizer

Merges findings, rewrites them clearly, ranks them, and prepares user-facing summaries.

### Copy Fix Packet Writer

Converts accepted findings into paste-ready instructions for an external coding agent.

---

## Preset YAML Shape

```yaml
id: security-auth-focus
name: Security & Auth Focus
version: 0.2
status: builtin
description: Deep review of auth, authorization, route protection, secrets, and data exposure.

roles:
  - context-discovery
  - security-reviewer
  - authz-tenant-isolation-reviewer
  - counter-evidence-skeptic
  - evidence-verifier
  - finding-synthesizer
  - copy-fix-packet-writer

context_policy:
  include_changed_code: true
  include_surrounding_code: true
  include_related_call_sites: true
  include_related_tests: true
  include_routes: true
  include_middleware: true
  include_config: true
  include_prior_comments: true
  redact_secrets: true

verification_policy:
  require_exact_location: true
  require_counter_evidence_search: true
  require_evidence_map_for_high: true
  min_confidence_to_surface: 0.55
  min_confidence_to_mark_verified: 0.80

chat_policy:
  default_audience: all_agents
  follow_up_primary_responder: evidence-verifier
  ask_all_roles:
    - security-reviewer
    - authz-tenant-isolation-reviewer
    - counter-evidence-skeptic
  local_first_for_status_questions: true

output_policy:
  generate_findings: true
  generate_evidence_maps: true
  generate_copy_packets: true
  generate_github_comments: true
```

---

## Role Prompt Shape

```markdown
---
id: authz-tenant-isolation-reviewer
name: AuthZ & Tenant Isolation Reviewer
version: 0.2
---

You are an authorization and tenant-isolation reviewer inside cocode.

Review goal:
Find access-control issues where users may access or mutate data they should not.

Focus areas:

- route registration
- middleware and guard application
- caller identity source
- tenant/account/user ownership checks
- admin bypasses
- query filters
- response data exposure
- tests for unauthorized access

Required method:

1. Trace route -> middleware -> handler -> service -> query -> response.
2. Identify where identity is established.
3. Identify where authorization is enforced.
4. Check whether tenant/user/account IDs come from trusted identity or request input.
5. Check tests for allowed and denied access.
6. Search for counter-evidence before reporting.

Output:
Return findings in cocode FindingCandidate JSON.

Done criteria:

- Every reported finding has exact file/line evidence.
- You checked for parent middleware or upstream guard before claiming a missing guard.
- You checked whether the handler/service enforces authorization.
- You checked for tests or noted that tests were unavailable.
- You reported no finding if evidence is insufficient.
```

---

## Evidence Map Requirements

Roles should emit graph-friendly evidence:

```json
{
  "evidence_map_seeds": [
    {
      "node_type": "route",
      "path": "src/routes/billing.ts",
      "start_line": 28,
      "end_line": 35,
      "label": "Billing route"
    },
    {
      "node_type": "middleware",
      "path": "src/middleware/auth.ts",
      "start_line": 22,
      "end_line": 48,
      "label": "RequireAuth middleware"
    }
  ],
  "relationships": [
    {
      "source": "Billing route",
      "target": "RequireAuth middleware",
      "type": "missing_guard",
      "evidence": "Route is registered without RequireAuth."
    }
  ]
}
```

---

## Chat Behavior by Role

| Role                       | Best chat use                                       |
| -------------------------- | --------------------------------------------------- |
| Orchestrator               | Planning, status, deciding which agents to call.    |
| Context Discovery          | “What files matter for this?”                       |
| Security Reviewer          | “What security risks exist?”                        |
| AuthZ/Tenant Isolation     | “Is this authorization issue real?”                 |
| Go Performance             | “Will this slow down hot paths?”                    |
| Postgres Query Performance | “Is this query/index change risky?”                 |
| Migration Safety           | “Can this migration lock or rewrite a large table?” |
| Evidence Verifier          | “Is this finding true?”                             |
| Counter-Evidence Skeptic   | “Try to disprove this finding.”                     |
| Synthesizer                | “Summarize what agents agree on.”                   |
| Copy Packet Writer         | “Prepare this for my coding agent.”                 |

---

## Done Criteria for Agent Outputs

A review agent output is done when:

1. It follows the requested schema.
2. It includes no unsupported claims.
3. Every finding has a concrete location or explains why no location is possible.
4. Every high/medium finding includes evidence.
5. Every high-severity finding includes counter-evidence search notes.
6. It distinguishes verified, plausible, and uncertain claims.
7. It states “no findings” when appropriate.
8. It does not propose code edits in review mode.
9. It does not publish comments.
10. It preserves enough evidence for Evidence Map generation.
