#!/bin/sh
set -eu

if [ "${1:-}" = "--version" ]; then
  printf 'cocode-fake-json-agent 0.1.0\n'
  exit 0
fi

# Consume prompt/context so tests exercise stdin delivery without echoing input.
/bin/cat >/dev/null || true

cat <<'JSON'
{
  "summary": "Found one deterministic fixture issue.",
  "findings": [
    {
      "claim": "Repository settings updates can run without an admin permission check.",
      "category": "security",
      "severity": "high",
      "confidence": 0.91,
      "locations": [
        {
          "path": "apps/api/src/routes/repositories.ts",
          "start_line": 87,
          "end_line": 112,
          "side": "RIGHT"
        }
      ],
      "evidence": [
        {
          "title": "Mutation route reaches updateSettings after member authentication only",
          "summary": "The route updates repository settings without requiring workspace admin privileges.",
          "path": "apps/api/src/routes/repositories.ts",
          "start_line": 87,
          "end_line": 112
        }
      ],
      "counter_evidence_request": "Show an upstream admin-only guard that always runs before this route.",
      "suggested_fix": "Mount requireWorkspaceAdmin before updateRepositorySettings and add a member-denied regression test.",
      "draft_comment": "Please require workspace admin permission before mutating repository settings."
    }
  ]
}
JSON

