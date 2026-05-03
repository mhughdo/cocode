#!/bin/sh
set -eu

if [ "${1:-}" = "--version" ]; then
  printf 'cocode-fake-malformed-agent 0.1.0\n'
  exit 0
fi

/bin/cat >/dev/null || true

printf '%s\n' '{"summary":"This payload is intentionally malformed for repair tests.","findings":[{"claim":"A route appears to mutate state without validation.","category":"correctness","severity":"medium","confidence":0.73,"locations":[{"path":"apps/api/src/routes/repositories.ts","start_line":90,"end_line":96,"side":"RIGHT"}],"suggested_fix":"Add the missing validation before mutation.",}]}'
