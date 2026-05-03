# cocode Glossary

Use these terms consistently in API names, UI copy, docs, and tests.

| Term | Definition |
|---|---|
| Adapter | Backend integration that turns a configured agent or tool into cocode `AgentEvent` output. MVP adapters are CLI-first; future adapters may use JSON-RPC or other protocols. |
| Agent config | Saved local configuration for one reviewer/verifier/fixer-style agent, including command, args, role, capabilities, and environment allowlist. |
| Agent run | One execution of an agent against a specific task and context bundle. Runs persist status, timing, raw artifacts, and parsed output. |
| Artifact | App-managed file stored outside normalized DB rows, such as diffs, prompts, raw outputs, context bundles, evidence graphs, copy packets, and GitHub previews. |
| Call path | Ordered explanation of how code flows between Evidence Map nodes when cocode can infer or observe a useful path. |
| Changed file | File included in a snapshot diff, with status, line ranges, generated/binary flags, and optional patch artifact. |
| Connection driver | Low-level process or protocol transport used by an adapter, such as one-shot CLI execution or future JSON-RPC stdio. |
| Context bundle | Bounded, persisted set of context items prepared for an agent, verifier, follow-up question, or Evidence Map task. |
| Copy packet | Deterministic handoff generated from selected findings so the user can paste accepted review work into a coding agent. |
| Decision status | Current human triage state of a finding: undecided, accepted, dismissed, deferred, copied, or published. |
| Evidence bundle | The finding-scoped evidence available to a verifier or follow-up answer, including supporting, counter, missing, test, and search evidence items. |
| Evidence item | One concrete piece of support, contradiction, missing context, test coverage, search result, or agent-provided evidence attached to a finding. |
| Evidence Map | Visual graph for one finding that shows changed code, related evidence, edges, call path, missing reasons, and deep links. |
| Finding | Canonical review issue produced after candidate normalization and dedupe. Findings are the primary object users triage, copy, and publish. |
| Finding candidate | Raw or lightly normalized issue emitted by one agent before dedupe into a canonical finding. |
| Follow-up thread | Persistent chat thread scoped to one finding. It stores user questions, agent/verifier answers, evidence references, and quick-action messages. |
| GitHub preview | Persisted publish draft showing review body, inline comments, anchor warnings, and publish checklist before any external side effect. |
| Local Verifier | Deterministic Go verification path that searches local code and applies rule profiles without sending code to an external agent. |
| Pull request snapshot | Immutable local representation of a GitHub PR, branch compare, commit compare, or local changes diff used for a review session. |
| Review session | One cocode review workflow over one snapshot. It owns agents, context, findings, evidence, events, decisions, and export/publish artifacts. |
| Thread | User-facing conversation surface. In MVP this usually means a review thread or a finding-scoped follow-up thread. |
| Verification status | cocode assessment of whether evidence supports a finding: unverified, verified, plausible, needs_human, likely_false_positive, duplicate, or not_actionable. |
| Workspace | Local root selected by the user that contains one or more repositories plus app settings and persisted cocode state. |
