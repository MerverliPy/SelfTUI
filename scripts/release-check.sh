#!/usr/bin/env bash
# release-check.sh — SelfTUI v0.1 release gate (hardening phase 8; M-10 fix:
# enforced toolchain pin, fixed archive member modes, flat SHA256SUMS).
#
# The full gate a versioned release must pass BEFORE the owner tags it.
# Usage:
#
#     VERSION=v0.1.0 scripts/release-check.sh      # or: VERSION=v0.1.0 make release-check
#
# Invariants:
#   * VERSION must match v<major>.<minor>.<patch> (no prerelease/build suffix).
#   * README.md's bold `Status:` line must announce exactly $VERSION - the
#     README must never ship announcing a stale release (P0 doc-truth gate,
#     2026-09-09; enforced via scripts/check-readme-status.sh).
#   * The git worktree must be clean - nothing staged, unstaged, or
#     untracked. Ignored build artifacts (bin/, dist/) don't count, so the
#     gate runs on exactly the commit that would be tagged.
#   * The local toolchain is enforced, not just documented: `go` must report
#     go1.27.1, `gofmt` must be the gofmt from that same distribution (gofmt
#     has no version flag, so the pin is by identity), and govulncheck must
#     report v1.7.0 - the same versions CI pins. Wrong or missing versions
#     fail fast (exit 2) before any slow gate and before dist/ is touched.
#     See CONTRIBUTING "prerequisites" and README "Release engineering".
#   * Archive members carry fixed modes (binary 0755, LICENSE/README 0644)
#     via install -m, so two builders or umasks produce byte-identical
#     archives.
#   * dist/SHA256SUMS is generated from inside dist/ and holds only the flat
#     archive names (no dist/ prefix, no absolute paths), so downloaded
#     GitHub Release assets verify beside the files with
#     `sha256sum -c SHA256SUMS`.
#
# Steps, in order:
#   0. toolchain pin               go 1.27.1 + same-distribution gofmt +
#                                  govulncheck v1.7.0 all on PATH
#   0b. README status gate         README.md's `Status:` line must announce
#                                  exactly $VERSION (doc truth; instant, so
#                                  it still fails fast before any slow gate)
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
#                                    fixed member modes (0755 binary, 0644 docs)
#  10. dist/SHA256SUMS               sha256 over both archives; entries flat
#                                    (no dist/ prefix) so a download
#                                    directory verifies with
#                                    `sha256sum -c SHA256SUMS`
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

# --- 0. toolchain pin (enforced): local gate == CI --------------------------
# CI pins Go 1.27.1 and govulncheck v1.7.0 (workflow env). The local gate
# must run on the same tools, so wrong or missing versions stop here - before
# the slow gates and before dist/ is touched. gofmt has no -version flag
# (verified against go1.27.1), so it is pinned by identity: the gofmt on PATH
# must be the gofmt shipping with the pinned `go` distribution.
want_go="go1.27.1"
want_govuln="v1.7.0"

if ! command -v go >/dev/null 2>&1; then
  echo "release-check: go not on PATH - install Go ${want_go#go} and put its bin directory first on PATH (see CONTRIBUTING 'prerequisites')" >&2
  exit 2
fi
go_out="$(go version 2>&1)" || true
go_ver="$(printf '%s\n' "$go_out" | awk 'NR==1 { print $3 }')"
if [[ "$go_ver" != "$want_go" ]]; then
  echo "release-check: go on PATH is '${go_ver:-<no version parsed from \`go version\`>}'; release gates need exactly ${want_go} (documented pin)" >&2
  echo "release-check: 'go version' output was: $(printf '%s' "$go_out" | head -1)" >&2
  echo "release-check: install Go ${want_go#go} and put its bin directory first on PATH (see CONTRIBUTING 'prerequisites')" >&2
  exit 2
fi

if ! command -v gofmt >/dev/null 2>&1; then
  echo "release-check: gofmt not on PATH - the Go ${want_go#go} distribution ships gofmt; put its bin directory first on PATH (see CONTRIBUTING 'prerequisites')" >&2
  exit 2
fi
goroot="$(go env GOROOT 2>&1)" || true
want_gofmt="$(readlink -f "$goroot/bin/gofmt" 2>/dev/null || true)"
have_gofmt="$(readlink -f "$(command -v gofmt)" 2>/dev/null || true)"
if [[ -z "$want_gofmt" || "$have_gofmt" != "$want_gofmt" ]]; then
  echo "release-check: gofmt on PATH is '$(command -v gofmt)'; the pinned Go ${want_go#go} distribution's gofmt is '$goroot/bin/gofmt'" >&2
  echo "release-check: gofmt has no version flag, so the pin is enforced by identity - put the pinned distribution's bin directory first on PATH:" >&2
  echo "  export PATH=\"$goroot/bin:\$PATH\"" >&2
  exit 2
fi

if ! command -v govulncheck >/dev/null 2>&1; then
  echo "release-check: govulncheck not on PATH - install the pinned version:" >&2
  echo "  go install golang.org/x/vuln/cmd/govulncheck@${want_govuln}" >&2
  exit 2
fi
gv_out="$(govulncheck -version 2>&1)" || true
gv_ver="$(printf '%s\n' "$gv_out" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || true)"
if [[ "$gv_ver" != "$want_govuln" ]]; then
  echo "release-check: govulncheck on PATH is '${gv_ver:-<no vX.Y.Z token in \`govulncheck -version\` output>}'; release gates need exactly ${want_govuln} (documented pin)" >&2
  echo "release-check: 'govulncheck -version' output was: $(printf '%s' "$gv_out" | head -1)" >&2
  echo "release-check: reinstall the pinned version: go install golang.org/x/vuln/cmd/govulncheck@${want_govuln}" >&2
  exit 2
fi

if ! command -v strings >/dev/null 2>&1; then
  echo "release-check: 'strings' (binutils) not on PATH - needed for the cross-arch version check" >&2
  exit 2
fi
echo "== toolchain pin: go ${want_go} + same-distribution gofmt + govulncheck ${want_govuln} =="

# --- 0b. README status gate (P0 doc truth, 2026-09-09) ----------------------
# The README must never ship announcing a stale release: its bold Status:
# line has to name exactly the version being tagged. Instant check, so it
# still fails fast before any slow gate and before dist/ is touched.
echo "== README status gate =="
scripts/check-readme-status.sh "$VERSION"

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
govulncheck ./...

# --- 7. CGO-disabled Linux builds ------------------------------------------
echo "== make build-linux-amd64 build-linux-arm64 (VERSION=$VERSION) =="
make build-linux-amd64 build-linux-arm64
echo "== Go toolchain of the built binaries =="
go version dist/selftui-linux-amd64 dist/selftui-linux-arm64

# --- 8. version-stamp checks -----------------------------------------------
# The exec-or-embedded-string logic lives in one place (shared with
# .github/workflows/release.yml) so local gate and CI cannot drift.
echo "== version stamp check (want: 'selftui $VERSION') =="
scripts/verify-binary-version.sh dist/selftui-linux-amd64 "$VERSION"
scripts/verify-binary-version.sh dist/selftui-linux-arm64 "$VERSION"

# --- 9. deterministic archives ---------------------------------------------
echo "== deterministic archives =="
for arch in amd64 arm64; do
  bin="dist/selftui-linux-$arch"
  archive="dist/selftui-$VERSION-linux-$arch.tar.gz"
  stage="dist/.stage-$arch"
  rm -rf "$stage"
  mkdir -p "$stage"
  # Fixed member modes: install -m forces 0755 (binary) / 0644 (documents)
  # regardless of the builder's umask or staging tool, so two builders (or
  # umasks) produce byte-identical archives.
  install -m 0755 "$bin" "$stage/selftui"
  install -m 0644 LICENSE "$stage/LICENSE"
  install -m 0644 README.md "$stage/README.md"
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
# Generated from inside dist/ so entries are the flat archive names that
# GitHub Release assets use; downloaders verify beside the downloaded assets
# with `sha256sum -c SHA256SUMS`. LC_ALL=C keeps member order and the
# manifest stable across locales and runs.
( cd dist && LC_ALL=C sha256sum selftui-"$VERSION"-linux-*.tar.gz > SHA256SUMS )
cat dist/SHA256SUMS

echo
echo "== release gate PASSED for $VERSION =="
echo "artifacts under dist/:"
ls -l dist/selftui-linux-* dist/selftui-"$VERSION"-linux-*.tar.gz dist/SHA256SUMS
