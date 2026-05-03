#!/usr/bin/env sh
set -eu

target="${1:-.}"
files="$(gofmt -l "$target")"

if [ -n "$files" ]; then
  printf '%s\n' "Go files need gofmt:"
  printf '%s\n' "$files"
  exit 1
fi
