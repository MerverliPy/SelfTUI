#!/usr/bin/env bash
# verify-binary-version.sh — assert a release binary carries a version stamp.
#
# Single source of truth for the per-binary version check, shared by
# scripts/release-check.sh (step 8) and .github/workflows/release.yml so the
# exec-or-embedded-string logic cannot drift between the local gate and CI.
#
# Usage: scripts/verify-binary-version.sh <binary> <VERSION>
#   <binary>   path to the built release binary
#   <VERSION>  expected stamp, e.g. v0.1.0   (the binary must report
#              exactly "selftui <VERSION>")
#
# Exit codes: 0 = verified; 1 = mismatch; 2 = cannot verify (tool missing).
# This script never creates or pushes a git tag.
set -euo pipefail

bin="$1"
version="$2"
expect="selftui $version"

# Native path: execute -version and compare the full output. The output
# contract ("selftui <VERSION>") is pinned by cmd/self-tui/main_test.go.
if out="$("$bin" -version 2>&1)"; then
  if [[ "$out" == "$expect" ]]; then
    echo "ok: $bin -> $out (executed)"
    exit 0
  fi
  echo "FAIL: $bin reports '$out', want '$expect'" >&2
  exit 1
fi

case "$out" in
  *"Exec format error"*|*"cannot execute binary file"*)
    # A cross-compiled binary cannot execute on a foreign host (arm64 on an
    # amd64 machine) without qemu-user/binfmt. Fall back to the bytes the
    # linker wrote: -X replaced the whole Version value, so the exact
    # version must appear as its own isolated string, and printVersion's
    # format literal must still read "selftui %s" (checked as a substring,
    # because Go packs rodata strings without separators). A missing stamp
    # or a changed format then fails loudly instead of passing silently.
    if ! command -v strings >/dev/null 2>&1; then
      echo "FAIL: cannot verify $bin without 'strings' (binutils)" >&2
      exit 2
    fi
    if strings "$bin" | grep -F -- "selftui %s" >/dev/null \
        && strings "$bin" | grep -Fx -- "$version" >/dev/null; then
      echo "ok: $bin -> $expect (embedded strings; cannot exec $(go env GOOS)/$(go env GOARCH) on this host)"
      exit 0
    fi
    echo "FAIL: $bin does not embed '$version' alongside the 'selftui %s' format" >&2
    exit 1
    ;;
  *)
    echo "FAIL: unexpected failure running '$bin -version': $out" >&2
    exit 1
    ;;
esac
