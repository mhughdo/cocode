# Fake CLI Agents

Deterministic shell fixtures used by backend and integration tests. They avoid
network access and should run with a plain POSIX shell.

- `json-agent.sh` emits a valid structured review payload with one finding.
- `malformed-agent.sh` emits intentionally broken JSON for repair/error tests.
