#!/usr/bin/env bash
# check-readme-status.sh — P0 doc-truth gate (2026-09-09, conclave P0):
# README.md's bold `Status:` line must announce exactly the release version
# being tagged, so the README can never ship announcing a stale release.
# (The drift this closes: README said "v0.2.0 released" while tags had
# reached v0.4.0.)
#
# Usage: scripts/check-readme-status.sh <VERSION>
#   <VERSION>  the release version, e.g. v0.1.0 (must match
#              v<major>.<minor>.<patch>)
#
# Exit codes: 0 = README status matches VERSION;
#             1 = mismatch (stale status line);
#             2 = cannot check (bad VERSION, missing README, or the status
#                 line is missing/ambiguous).
# This script never creates or pushes a git tag.
set -euo pipefail
cd "$(dirname "$0")/.."   # repo root

version="${1:-}"
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "check-readme-status: VERSION must match v<major>.<minor>.<patch> (got '${version:-<empty>}')" >&2
  echo "check-readme-status: e.g. scripts/check-readme-status.sh v0.4.0" >&2
  exit 2
fi

readme="README.md"
if [[ ! -f "$readme" ]]; then
  echo "check-readme-status: $readme not found at the repo root" >&2
  exit 2
fi

# The status line is the bold "Status:" line near the top of README.md:
#   **Status: v0.4.0 released 2026-09-08** (...)
status_lines="$(grep -nE '^\*\*Status: v[0-9]+\.[0-9]+\.[0-9]+ released ' "$readme" || true)"
count="$(printf '%s\n' "$status_lines" | grep -cE '^[0-9]+:' || true)"
if [[ "$count" -ne 1 ]]; then
  echo "check-readme-status: expected exactly one '**Status: vX.Y.Z released ...' line in $readme, found ${count}" >&2
  if [[ -n "$status_lines" ]]; then
    printf '%s\n' "$status_lines" >&2
  fi
  echo "check-readme-status: fix the Status line so the release version is unambiguous" >&2
  exit 2
fi

read_version="$(printf '%s\n' "$status_lines" \
  | sed -nE 's/^[0-9]+:\*\*Status: (v[0-9]+\.[0-9]+\.[0-9]+) released .*/\1/p' \
  | head -n1)"

if [[ "$read_version" == "$version" ]]; then
  echo "ok: $readme status announces $read_version"
  exit 0
fi

echo "FAIL: $readme status announces '${read_version}', want '${version}'" >&2
echo "FAIL: release gate requires README status == released version — update the Status line in the release-prep commit" >&2
exit 1
