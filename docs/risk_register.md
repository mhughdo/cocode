# cocode MVP Risk Register

Review this register weekly during MVP buildout and before every release candidate. Severity uses `High`, `Medium`, or `Low`; owner is the team role responsible for keeping the mitigation current.

| ID | Risk | Severity | Owner | Mitigation | Review Cadence | Status |
|---|---|---:|---|---|---|---|
| R001 | Large PRs exceed context, memory, or UI rendering budgets. | High | Backend | Keep context selection streaming/bounded, cap snippets, store raw artifacts once, and prefer indexed lookups over repeated full scans. | Weekly and before large-diff dogfood | Open |
| R002 | GitHub inline comments anchor to the wrong diff line. | High | Backend | Use stored diff artifacts, deterministic line/side mapping, unanchored warnings, and summary-only fallback. | Each publish change | Mitigated |
| R003 | CLI agents receive secrets or local-only files. | High | Security | Redact context bundles, enforce local-only exclusions, track provider visibility, and allowlist environment variables. | Weekly until T330 passes | Open |
| R004 | CLI presets drift from locally installed tool behavior. | Medium | Backend | Health-check command availability/version and keep presets editable instead of hardcoding provider assumptions. | Each adapter change | Open |
| R005 | Agent output triggers unsafe side effects. | High | Backend | Treat output as untrusted, parse/verify before persistence, and keep publish/write actions human-gated. | Each workflow change | Open |
| R006 | Desktop security regression exposes Node or backend token to renderer content. | High | Desktop | Keep narrow preload API, sandbox renderer, validate IPC inputs, and test local auth/origin behavior. | Each Electron change | Open |
| R007 | SQLite migrations break existing local workspaces. | Medium | Backend | Keep migrations idempotent, add migration tests, and document backup/export command. | Each schema change | Open |
| R008 | Evidence Map becomes visually impressive but not trustworthy. | Medium | Product/Frontend | Always display source file/line, missing reasons, confidence, and counter-evidence instead of inferred-only graph nodes. | Each Evidence Map change | Open |
| R009 | Publish flow duplicates comments across reruns. | Medium | Backend | Check published finding state, fingerprint, and primary path/line before creating preview/publish payloads. | Each publish change | Mitigated |
| R010 | Packaged app cannot find or launch the backend binary. | Medium | Desktop | Test dev and packaged launch paths, log backend startup, and document recovery location. | Each packaging change | Open |
