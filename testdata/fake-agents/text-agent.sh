#!/bin/sh
set -eu

if [ "${1:-}" = "--version" ]; then
  printf 'cocode-fake-text-agent 0.1.0\n'
  exit 0
fi

/bin/cat >/dev/null || true

cat <<'TEXT'
Finding: Repository settings can be changed without proving workspace admin permission.
Category: security
Severity: medium
Confidence: 0.62
Location: apps/api/src/routes/repositories.ts:87-112
Evidence: The route updates repository settings after member authentication but before any admin-only guard.
Suggested fix: Require workspace admin permission before mutating repository settings.
Draft comment: Please require workspace admin permission before this settings mutation.
TEXT
