#!/usr/bin/env bash
# release-check.sh — SelfTUI v0.1 release gate (hardening phase 8).
#
# The full gate a versioned release must pass BEFORE the owner tags it.
# Usage:
#
#     VERSION=v0.1.0 scripts/release-check.sh      # or: VERSION=v0.1.0 make release-check
#
# Invariants:
#   * VERSION must match v<major>.<minor>.<patch> (no prerelease/build suffix).
#   * The git worktree must be clean - nothing staged, unstaged, or
#     untracked. Ignored build artifacts (bin/, dist/) don't count, so the
#     gate runs on exactly the commit that would be tagged.
#   * The gate uses whatever `go`/`gofmt`/`govulncheck` the caller put on
#     PATH. CI and the documented local gate pin Go 1.27.1 (current official
#     stable) and govulncheck v1.7.0 - see README "Release engineering".
#
# Steps, in order:
#   1. go mod verify                 module graph + go.sum integrity
#   2. gofmt check                   no file needs formatting
#   3. go vet ./...                  static analysis
#   4. go test -count=1 ./...        uncached unit/golden tests
#   5. go test -race -count=1 ./...  full suite under the race detector
#   6. govulncheck ./...             known vulnerabilities in module + build
#   7. make build-linux-amd64 + build-linux-arm64
#                                    CGO-disabled static binaries stamped
#                                    -X main.Version=$VERSION
#   8. version-stamp check           every binary reports `selftui $VERSION`
#                                    (executed where the host can run it,
#                                    otherwise the exact string linked in)
#   9. deterministic archives        dist/selftui-$VERSION-linux-{amd64,arm64}.tar.gz
#  10. dist/SHA256SUMS               sha256 over both archives, entries
#                                    prefixed dist/ so
#                                    `sha256sum -c dist/SHA256SUMS` verifies
#                                    from the repo root
#
# This script NEVER creates or pushes a git tag; tagging stays the owner's
# separate release step, done only once this gate is green.
set -euo pipefail
cd "$(dirname "$0")/.."   # repo root

VERSION="${VERSION:-}"
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "release-check: VERSION must match v<major>.<minor>.<patch> (got '${VERSION}')" >&2
  echo "release-check: e.g. VERSION=v0.1.0 scripts/release-check.sh" >&2
  exit 2
fi
echo "== release-check for $VERSION =="

# --- clean worktree --------------------------------------------------------
if [[ -n "$(git status --porcelain)" ]]; then
  echo "release-check: worktree is not clean - commit or stash first:" >&2
  git status --porcelain >&2
  exit 2
fi

# dist/ is owned by this gate: start from a fresh, empty artifact dir so no
# stale file can leak into the archives or the checksum manifest.
rm -rf dist
mkdir -p dist

# --- 1. go mod verify ------------------------------------------------------
echo "== go mod verify =="
go mod verify

# --- 2. gofmt check --------------------------------------------------------
echo "== gofmt check =="
out="$(gofmt -l .)"
if [[ -n "$out" ]]; then
  echo "release-check: files need gofmt:" >&2
  echo "$out" >&2
  exit 1
fi

# --- 3. go vet -------------------------------------------------------------
echo "== go vet ./... =="
go vet ./...

# --- 4. uncached tests -----------------------------------------------------
echo "== go test -count=1 ./... =="
go test -count=1 ./...

# --- 5. race tests ---------------------------------------------------------
echo "== go test -race -count=1 ./... =="
go test -race -count=1 ./...

# --- 6. govulncheck --------------------------------------------------------
echo "== govulncheck ./... =="
if ! command -v govulncheck >/dev/null 2>&1; then
  echo "release-check: govulncheck not on PATH - install the pinned version:" >&2
  echo "  go install golang.org/x/vuln/cmd/govulncheck@v1.7.0" >&2
  exit 2
fi
govulncheck ./...

# --- 7. CGO-disabled Linux builds ------------------------------------------
echo "== make build-linux-amd64 build-linux-arm64 (VERSION=$VERSION) =="
make build-linux-amd64 build-linux-arm64
echo "== Go toolchain of the built binaries =="
go version dist/selftui-linux-amd64 dist/selftui-linux-arm64

# --- 8. version-stamp checks -----------------------------------------------
echo "== version stamp check (want: 'selftui $VERSION') =="
expect="selftui $VERSION"
for bin in dist/selftui-linux-amd64 dist/selftui-linux-arm64; do
  if out="$("$bin" -version 2>&1)"; then
    if [[ "$out" == "$expect" ]]; then
      echo "ok: $bin -> $out (executed)"
    else
      echo "release-check: $bin reports '$out', want '$expect'" >&2
      exit 1
    fi
  else
    # A cross-compiled binary cannot execute on a foreign host (arm64 on an
    # amd64 machine) without qemu-user/binfmt, so fall back to the value the
    # linker actually wrote: -X replaces the whole string, so the exact
    # version must appear as its own standalone string in the ELF image.
    case "$out" in
      *"Exec format error"*|*"cannot execute binary file"*)
        if ! command -v strings >/dev/null 2>&1; then
          echo "release-check: cannot verify $bin: needs 'strings' (binutils)" >&2
          exit 2
        fi
        if strings "$bin" | grep -Fx -- "$VERSION" >/dev/null; then
          echo "ok: $bin -> $expect (linked-in string; cannot exec $(go env GOOS)/$(go env GOARCH) on this host)"
        else
          echo "release-check: $bin does not embed the string '$VERSION'" >&2
          exit 1
        fi
        ;;
      *)
        echo "release-check: unexpected failure running '$bin -version': $out" >&2
        exit 1
        ;;
    esac
  fi
done

# --- 9. deterministic archives ---------------------------------------------
echo "== deterministic archives =="
for arch in amd64 arm64; do
  bin="dist/selftui-linux-$arch"
  archive="dist/selftui-$VERSION-linux-$arch.tar.gz"
  stage="dist/.stage-$arch"
  rm -rf "$stage"
  mkdir -p "$stage"
  cp "$bin" "$stage/selftui"
  cp LICENSE "$stage/LICENSE"
  cp README.md "$stage/README.md"
  # Fixed member order and mtimes, owner/group 0, and gzip without header
  # name/mtime stamp, so identical inputs yield byte-identical archives.
  tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner \
    -C "$stage" -cf "$stage/bundle.tar" selftui LICENSE README.md
  gzip -n -c "$stage/bundle.tar" > "$archive"
  rm -rf "$stage"
  echo "created $archive"
done

# --- 10. SHA256SUMS --------------------------------------------------------
echo "== dist/SHA256SUMS =="
# Entries are dist/-prefixed so `sha256sum -c dist/SHA256SUMS` verifies from
# the repo root (the documented local gate). LC_ALL=C keeps member order and
# the manifest stable across locales and runs.
LC_ALL=C sha256sum dist/selftui-"$VERSION"-linux-*.tar.gz > dist/SHA256SUMS
cat dist/SHA256SUMS

echo
echo "== release gate PASSED for $VERSION =="
echo "artifacts under dist/:"
ls -l dist/selftui-linux-* dist/selftui-"$VERSION"-linux-*.tar.gz dist/SHA256SUMS
