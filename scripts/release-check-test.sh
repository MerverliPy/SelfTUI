#!/usr/bin/env bash
# release-check-test.sh — regression suite for scripts/release-check.sh
# (runbook Task 17 / audit finding M-10: "release gate is not fully portable
# or cross-builder reproducible").
#
# Self-contained: builds disposable fixtures under `mktemp -d` and tears them
# down on exit. The harness NEVER runs release-check.sh against the real
# repository — every run happens inside a temp fixture repo whose `go`,
# `gofmt`, and `govulncheck` are fake scripts, so no real compile/test/gate
# work and no destructive dist/ handling ever touches this tree. No network.
# Run with:
#
#     bash scripts/release-check-test.sh
#
# Contract pinned here (the M-10 fix):
#   * wrong/missing toolchain versions fail fast (exit 2, stable actionable
#     error) BEFORE any slow gate and BEFORE dist/ is created — the local
#     release gate must not run unless the documented Go 1.27.1 and
#     govulncheck v1.7.0 are active, with gofmt coming from the same
#     distribution as the pinned `go`;
#   * archive members carry fixed modes (binary 0755, LICENSE/README 0644),
#     so two builders — or two umasks — produce byte-identical archives;
#   * dist/SHA256SUMS holds only flat archive names (no dist/ prefixes, no
#     absolute paths), generated from inside dist/, so downloaded assets
#     verify with `sha256sum -c SHA256SUMS` in a flat download directory.
#
# The fake `go` is deliberately umask-sensitive when it "builds" (-o): the
# file it emits inherits the caller's umask. The current cp-based staging
# then bakes that mode into the archive (members differ across umasks); the
# install -m 0755/0644 staging normalizes it away.
set -euo pipefail
cd "$(dirname "$0")/.."        # repo root

script="scripts/release-check.sh"
master_tool="$(mktemp -d)/master"   # one real path; per-case dirs copy subsets
base="$(mktemp -d)"
trap 'rm -rf "$(dirname "$master_tool")" "$base"' EXIT

checks=0
fails=0

pass() { checks=$((checks + 1)); echo "ok   $checks: $1"; }
fail() { fails=$((fails + 1)); echo "FAIL $fails: $1"; }

# expect_grep <desc> <text> <needle> — text must contain needle (fixed string).
expect_grep() {
  local desc="$1" text="$2" needle="$3"
  if printf '%s' "$text" | grep -Fq -- "$needle"; then
    pass "$desc"
  else
    fail "$desc (output lacks '$needle'; got: $(printf '%s' "$text" | head -c 400))"
  fi
}

# expect_empty <desc> <text>
expect_empty() {
  local desc="$1" text="$2"
  if [[ -z "$text" ]]; then
    pass "$desc"
  else
    fail "$desc (expected empty; got: $(printf '%s' "$text" | head -c 200))"
  fi
}

# --- fake toolchain ---------------------------------------------------------
# One canonical fake go/gofmt/govulncheck set, written once. Behaviour comes
# from environment variables at run time, so the same scripts serve every
# case; per-case PATH dirs hold copies of only the wanted tools.
fake_go="$(cat <<'GO'
#!/usr/bin/env bash
set -euo pipefail
line="${GO_VER_LINE:-go version go1.27.1 linux/amd64}"
goroot="${FAKE_GOROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
stamp="${FAKE_STAMP:-v0.1.1}"
log="${FAKE_LOG:-/dev/null}"
case "${1:-}" in
  version)
    if [ "$#" -gt 1 ]; then printf '%s: go1.27.1\n' "$2"; else printf '%s\n' "$line"; fi ;;
  env)
    shift; for k in "$@"; do
      case "$k" in
        GOROOT) printf '%s\n' "$goroot" ;;
        GOOS) printf 'linux\n' ;;
        GOARCH) printf 'amd64\n' ;;
        *) printf '\n' ;;
      esac
    done ;;
  *)
    printf 'go %s\n' "$*" >> "$log"
    # emulate `go build ... -o <path>`: emit an executable fake binary whose
    # content is the exact version stamp and whose mode follows the umask.
    prev=""
    for a in "$@"; do
      if [ "$prev" = "-o" ]; then
        mkdir -p "$(dirname "$a")"
        : > "$a"
        printf '#!/bin/sh\necho %q\n' "selftui $stamp" >> "$a"
        chmod u+x "$a"
      fi
      prev="$a"
    done
    exit 0 ;;
esac
GO
)"

fake_gofmt="$(cat <<'GOFMT'
#!/usr/bin/env bash
exit 0
GOFMT
)"

fake_govuln="$(cat <<'GV'
#!/usr/bin/env bash
set -euo pipefail
line="${GOVULN_VER_LINE:-govulncheck v1.7.0}"
rc="${GOVULN_RC:-0}"
log="${FAKE_LOG:-/dev/null}"
case "${1:-}" in
  -version) printf '%s\n' "$line"; exit "$rc" ;;
  *) printf 'govulncheck %s\n' "$*" >> "$log"; exit 0 ;;
esac
GV
)"

mkdir -p "$master_tool/bin"
printf '%s' "$fake_go" > "$master_tool/bin/go"
printf '%s' "$fake_gofmt" > "$master_tool/bin/gofmt"
printf '%s' "$fake_govuln" > "$master_tool/bin/govulncheck"
chmod +x "$master_tool/bin/go" "$master_tool/bin/gofmt" "$master_tool/bin/govulncheck"

# mktool <dir> <names...> — a PATH dir holding copies of the named fakes.
mktool() {
  local d="$1"; shift
  mkdir -p "$d"
  for n in "$@"; do cp "$master_tool/bin/$n" "$d/$n"; chmod +x "$d/$n"; done
}

# --- fixture repo ------------------------------------------------------------
# A minimal clean git repo mirroring the real release-check inputs: the live
# scripts + Makefile are copied in, LICENSE/README are real archive members.
make_repo() {
  local d="$1"
  mkdir -p "$d/scripts"
  cp scripts/release-check.sh "$d/scripts/release-check.sh"
  cp scripts/verify-binary-version.sh "$d/scripts/verify-binary-version.sh"
  cp scripts/check-readme-status.sh "$d/scripts/check-readme-status.sh"
  cp Makefile "$d/Makefile"
  printf 'SelfTUI fixture license text\n' > "$d/LICENSE"
  printf '# Fixture README\n\n**Status: v0.1.1 released 2026-09-09**\n\nfixture content for the deterministic archive\n' > "$d/README.md"
  printf 'module fixture\n\ngo 1.25.8\n\ntoolchain go1.27.1\n' > "$d/go.mod"
  printf '/dist/\n/bin/\n' > "$d/.gitignore"
  git -C "$d" init -q
  git -C "$d" add -A
  git -C "$d" -c user.name=release-test -c user.email=release-test@example.invalid \
    commit -qm "fixture"
}

# --- fixture PATH hygiene ---------------------------------------------------
# $sys is a dir of symlinks to the real system tools release-check needs
# (git/make/tar/...), deliberately WITHOUT go/gofmt/govulncheck. Fixture runs
# use PATH="<fake tools>:$sys" so the real toolchain can never leak in and
# a "missing X" case is genuinely missing.
sys="$base/sys"
mkdir -p "$sys"
for t in env bash sh git make tar gzip sha256sum install readlink strings awk \
         grep sed head tail cat ls cp mkdir rm mv dirname basename wc tr cut \
         sort find date touch chmod stat xargs; do
  if real="$(command -v "$t" 2>/dev/null)" && [[ -n "$real" ]]; then
    ln -sf "$real" "$sys/$t"
  fi
done

# run_gate <repo> <extra_path> <umask> [env pairs...] — run the fixture's
# release-check.sh with PATH=$extra_path:$sys, VERSION=v0.1.1 and the given
# env, under <umask>. Captures combined output into $r_out and the exit code
# into $r_rc.
r_out=""
r_rc=0
run_gate() {
  local repo="$1" extra="$2" um="$3"
  shift 3
  set +e
  r_out="$(
    cd "$repo" || exit 99
    umask "$um"
    env PATH="$extra:$sys" VERSION=v0.1.1 "$@" bash scripts/release-check.sh 2>&1
  )"
  r_rc=$?
  set -e
}

# sha_of <file>
sha_of() { sha256sum "$1" | awk '{print $1}'; }

# tar_mode <archive> <member> — first field of the member's tar -tvzf line.
tar_mode() {
  tar -tvzf "$1" 2>/dev/null | awk -v m="$2" '$NF == m { print $1; exit }'
}

echo "== release-check-test =="

# --- 0. the script exists, is executable, and is syntactically valid ---------
if [[ -f "$script" ]] && [[ -x "$script" ]] && bash -n "$script"; then
  pass "release-check.sh exists, is executable, and bash -n is clean"
else
  fail "release-check.sh missing / not executable / syntax error"
fi

status_script="scripts/check-readme-status.sh"
if [[ -f "$status_script" ]] && [[ -x "$status_script" ]] && bash -n "$status_script"; then
  pass "check-readme-status.sh exists, is executable, and bash -n is clean"
else
  fail "check-readme-status.sh missing / not executable / syntax error"
fi

# The canonical good toolchain and fixture repo, shared by the fail-fast cases.
good_tool="$base/tool"
mktool "$good_tool/bin" go gofmt govulncheck
full_path="$good_tool/bin"

# --- 1. wrong/missing toolchains fail fast, before slow gates ---------------
# Each of these must exit 2 (not run to completion), print a stable
# actionable error, never reach a slow-gate/go-build invocation (FAKE_LOG
# stays empty), and leave the fixture's dist/ untouched.

# 1a. missing go entirely.
repo="$base/r-missgo"
make_repo "$repo"
missgo_tool="$base/tool-missgo"
mktool "$missgo_tool/bin" gofmt govulncheck
run_gate "$repo" "$missgo_tool/bin" 0022
if [[ $r_rc -eq 2 ]]; then pass "missing go: exits 2"; else fail "missing go: rc=$r_rc (want 2)"; fi
expect_grep "missing go: stable error names the pin" "$r_out" "go not on PATH - install Go 1.27.1"
expect_grep "missing go: actionable install hint" "$r_out" "install Go 1.27.1 and put its bin directory first on PATH"
if [[ ! -e "$repo/dist" ]]; then pass "missing go: dist/ never created"; else fail "missing go: dist/ was created before the toolchain gate"; fi

# 1b. go present but the wrong/newer version.
repo="$base/r-wronggo"
make_repo "$repo"
run_gate "$repo" "$full_path" 0022 GO_VER_LINE="go version go1.28.0 linux/amd64" FAKE_LOG="$base/log-wronggo.txt"
if [[ $r_rc -eq 2 ]]; then pass "go go1.28.0: exits 2"; else fail "go go1.28.0: rc=$r_rc (want 2)"; fi
expect_grep "go go1.28.0: error names the offending version" "$r_out" "go on PATH is 'go1.28.0'"
expect_grep "go go1.28.0: error names the pin" "$r_out" "release gates need exactly go1.27.1"
expect_empty "go go1.28.0: no slow gate reached" "$(cat "$base/log-wronggo.txt" 2>/dev/null || true)"
if [[ ! -e "$repo/dist" ]]; then pass "go go1.28.0: dist/ never created"; else fail "go go1.28.0: dist/ was created before the toolchain gate"; fi

# 1c. patch drift counts as wrong too (the documented pin is exactly 1.27.1).
run_gate "$repo" "$full_path" 0022 GO_VER_LINE="go version go1.27.2 linux/amd64"
if [[ $r_rc -eq 2 ]]; then pass "go go1.27.2: exits 2"; else fail "go go1.27.2: rc=$r_rc (want 2)"; fi
expect_grep "go go1.27.2: rejected as patch drift" "$r_out" "go on PATH is 'go1.27.2'"

# 1d. an older go also fails the pin.
run_gate "$repo" "$full_path" 0022 GO_VER_LINE="go version go1.25.8 linux/amd64"
if [[ $r_rc -eq 2 ]]; then pass "go go1.25.8: exits 2"; else fail "go go1.25.8: rc=$r_rc (want 2)"; fi
expect_grep "go go1.25.8: rejected" "$r_out" "go on PATH is 'go1.25.8'"

# 1e. unparseable go version output is rejected defensively.
run_gate "$repo" "$full_path" 0022 GO_VER_LINE="go version not-a-version linux/amd64"
if [[ $r_rc -eq 2 ]]; then pass "garbage go version: exits 2"; else fail "garbage go version: rc=$r_rc (want 2)"; fi
expect_grep "garbage go version: rejected" "$r_out" "go on PATH is 'not-a-version'"

# 1f. gofmt missing from PATH.
repo="$base/r-missgofmt"
make_repo "$repo"
missgofmt_tool="$base/tool-missgofmt"
mktool "$missgofmt_tool/bin" go govulncheck
run_gate "$repo" "$missgofmt_tool/bin" 0022
if [[ $r_rc -eq 2 ]]; then pass "missing gofmt: exits 2"; else fail "missing gofmt: rc=$r_rc (want 2)"; fi
expect_grep "missing gofmt: stable error" "$r_out" "gofmt not on PATH"
if [[ ! -e "$repo/dist" ]]; then pass "missing gofmt: dist/ never created"; else fail "missing gofmt: dist/ was created before the toolchain gate"; fi

# 1g. gofmt on PATH but from a different distribution than the pinned go.
repo="$base/r-wronggofmt"
make_repo "$repo"
decoy="$base/decoy"
mktool "$decoy" gofmt
run_gate "$repo" "$decoy:$full_path" 0022 FAKE_LOG="$base/log-wronggofmt.txt"
if [[ $r_rc -eq 2 ]]; then pass "foreign gofmt: exits 2"; else fail "foreign gofmt: rc=$r_rc (want 2)"; fi
expect_grep "foreign gofmt: error names the PATH gofmt" "$r_out" "gofmt on PATH is"
expect_grep "foreign gofmt: actionable fix" "$r_out" "bin directory first on PATH"
expect_empty "foreign gofmt: no slow gate reached" "$(cat "$base/log-wronggofmt.txt" 2>/dev/null || true)"
if [[ ! -e "$repo/dist" ]]; then pass "foreign gofmt: dist/ never created"; else fail "foreign gofmt: dist/ was created before the toolchain gate"; fi

# 1h. govulncheck missing.
repo="$base/r-missgv"
make_repo "$repo"
missgv_tool="$base/tool-missgv"
mktool "$missgv_tool/bin" go gofmt
run_gate "$repo" "$missgv_tool/bin" 0022
if [[ $r_rc -eq 2 ]]; then pass "missing govulncheck: exits 2"; else fail "missing govulncheck: rc=$r_rc (want 2)"; fi
expect_grep "missing govulncheck: install hint names the pin" "$r_out" "govulncheck@v1.7.0"
if [[ ! -e "$repo/dist" ]]; then pass "missing govulncheck: dist/ never created"; else fail "missing govulncheck: dist/ was created before the toolchain gate"; fi

# 1i. govulncheck present but the wrong version.
repo="$base/r-wronggv"
make_repo "$repo"
run_gate "$repo" "$full_path" 0022 GOVULN_VER_LINE="govulncheck v1.6.0" FAKE_LOG="$base/log-wronggv.txt"
if [[ $r_rc -eq 2 ]]; then pass "govulncheck v1.6.0: exits 2"; else fail "govulncheck v1.6.0: rc=$r_rc (want 2)"; fi
expect_grep "govulncheck v1.6.0: error names the pin" "$r_out" "release gates need exactly v1.7.0"
expect_grep "govulncheck v1.6.0: reinstall hint" "$r_out" "reinstall the pinned version"
expect_empty "govulncheck v1.6.0: only -version was invoked" "$(cat "$base/log-wronggv.txt" 2>/dev/null || true)"
if [[ ! -e "$repo/dist" ]]; then pass "govulncheck v1.6.0: dist/ never created"; else fail "govulncheck v1.6.0: dist/ was created before the toolchain gate"; fi

# 1j. newer govulncheck patch also rejected.
run_gate "$repo" "$full_path" 0022 GOVULN_VER_LINE="govulncheck v1.7.1"
if [[ $r_rc -eq 2 ]]; then pass "govulncheck v1.7.1: exits 2"; else fail "govulncheck v1.7.1: rc=$r_rc (want 2)"; fi
expect_grep "govulncheck v1.7.1: rejected" "$r_out" "release gates need exactly v1.7.0"

# 1k. govulncheck output with no version token (defensive parse).
run_gate "$repo" "$full_path" 0022 GOVULN_VER_LINE="govulncheck: unknown flag; usage: govulncheck [flags] ./..."
if [[ $r_rc -eq 2 ]]; then pass "govulncheck no version token: exits 2"; else fail "govulncheck no version token: rc=$r_rc (want 2)"; fi
expect_grep "govulncheck no version token: output echoed" "$r_out" "govulncheck -version' output was"

# 1l. stale README status fails fast, before slow gates (P0 doc-truth gate).
repo="$base/r-stale-status"
make_repo "$repo"
printf '# Fixture README\n\n**Status: v0.1.0 released 2026-09-01**\n\nstale fixture content\n' > "$repo/README.md"
git -C "$repo" add -A
git -C "$repo" -c user.name=release-test -c user.email=release-test@example.invalid commit -qm "stale status"
run_gate "$repo" "$full_path" 0022
if [[ $r_rc -eq 1 ]]; then pass "stale README status: exits 1"; else fail "stale README status: rc=$r_rc (want 1)"; fi
expect_grep "stale README status: names the drift" "$r_out" "status announces 'v0.1.0', want 'v0.1.1'"
expect_grep "stale README status: update hint" "$r_out" "update the Status line"
if [[ ! -e "$repo/dist" ]]; then pass "stale README status: dist/ never created"; else fail "stale README status: dist/ was created before the doc gate"; fi

# 1m. README with no Status line cannot pass the doc gate.
repo="$base/r-no-status"
make_repo "$repo"
printf '# Fixture README\n\nfixture content without a status line\n' > "$repo/README.md"
git -C "$repo" add -A
git -C "$repo" -c user.name=release-test -c user.email=release-test@example.invalid commit -qm "no status"
run_gate "$repo" "$full_path" 0022
if [[ $r_rc -eq 2 ]]; then pass "missing README status: exits 2"; else fail "missing README status: rc=$r_rc (want 2)"; fi
expect_grep "missing README status: explains" "$r_out" "expected exactly one"
if [[ ! -e "$repo/dist" ]]; then pass "missing README status: dist/ never created"; else fail "missing README status: dist/ was created before the doc gate"; fi

# --- 2. correct toolchain: the gate runs to a stubbed PASS in the fixture ---
repo="$base/r-good"
make_repo "$repo"
run_gate "$repo" "$full_path" 0022 FAKE_LOG="$base/log-good.txt"
if [[ $r_rc -eq 0 ]]; then pass "correct versions: release gate passes (stubbed checkpoint)"; else fail "correct versions: rc=$r_rc (want 0): $(printf '%s' "$r_out" | tail -c 300)"; fi
expect_grep "correct versions: gate PASSED line" "$r_out" "release gate PASSED for v0.1.1"
expect_grep "correct versions: version-stamp check ran" "$r_out" "ok: dist/selftui-linux-amd64 -> selftui v0.1.1"
for a in "selftui-v0.1.1-linux-amd64.tar.gz" "selftui-v0.1.1-linux-arm64.tar.gz"; do
  if [[ -f "$repo/dist/$a" ]]; then pass "correct versions: produced $a"; else fail "correct versions: missing dist/$a"; fi
done

# --- 3. SHA256SUMS carries only flat archive names --------------------------
sums="$repo/dist/SHA256SUMS"
bad_lines="$(awk '$2 !~ /^selftui-v0\.1\.1-linux-(amd64|arm64)\.tar\.gz$/ || $2 ~ /\// {print}' "$sums" 2>/dev/null || true)"
bad_count="$(printf '%s\n' "$bad_lines" | sed '/^$/d' | wc -l | tr -d ' ')"
nlines="$(wc -l < "$sums" | tr -d ' ')"
if [[ -f "$sums" ]] && [[ "$nlines" -eq 2 ]] && [[ "$bad_count" -eq 0 ]]; then
  pass "SHA256SUMS: exactly two flat archive entries, no path prefixes"
else
  fail "SHA256SUMS malformed (lines=$nlines, bad=$bad_count): $(cat "$sums" 2>/dev/null | head -c 300)"
fi
expect_grep "SHA256SUMS: amd64 flat entry" "$(cat "$sums")" "selftui-v0.1.1-linux-amd64.tar.gz"
expect_grep "SHA256SUMS: arm64 flat entry" "$(cat "$sums")" "selftui-v0.1.1-linux-arm64.tar.gz"

# --- 4. checksums verify from inside dist and from a flat download dir ------
if (cd "$repo/dist" && sha256sum -c SHA256SUMS >/dev/null 2>&1); then
  pass "sha256sum -c SHA256SUMS verifies from inside dist/"
else
  fail "sha256sum -c SHA256SUMS fails from inside dist/"
fi
dl="$base/download"
mkdir -p "$dl"
cp "$repo/dist"/selftui-v0.1.1-linux-*.tar.gz "$repo/dist/SHA256SUMS" "$dl/"
if (cd "$dl" && sha256sum -c SHA256SUMS >/dev/null 2>&1); then
  pass "downloaded flat assets verify with sha256sum -c SHA256SUMS"
else
  fail "flat-download sha256sum -c failed (entries: $(cd "$dl" && sed 's/^[^ ]*  //' SHA256SUMS | tr '\n' ' '))"
fi

# --- 5. cross-umask reproducibility + fixed member modes ---------------------
# Two identical fixtures built under umask 0002 and 0022 must produce
# byte-identical archives with exact member modes (binary 0755, docs 0644).
tmpl="$base/tmpl"
make_repo "$tmpl"
repo_a="$base/umask-a"
repo_b="$base/umask-b"
cp -a "$tmpl" "$repo_a"
cp -a "$tmpl" "$repo_b"
tool_a="$base/tool-a"
tool_b="$base/tool-b"
mktool "$tool_a/bin" go gofmt govulncheck
mktool "$tool_b/bin" go gofmt govulncheck
run_gate "$repo_a" "$tool_a/bin" 0002
a_rc=$r_rc
a_out=$r_out
run_gate "$repo_b" "$tool_b/bin" 0022
b_rc=$r_rc
b_out=$r_out
if [[ $a_rc -eq 0 ]] && [[ $b_rc -eq 0 ]]; then
  pass "umask 0002 and 0022: both gates pass"
else
  fail "umask runs failed (0002 rc=$a_rc, 0022 rc=$b_rc)"
fi
umask_ok=1
umask_detail=""
for arch in amd64 arm64; do
  ha="$(sha_of "$repo_a/dist/selftui-v0.1.1-linux-$arch.tar.gz" 2>/dev/null || true)"
  hb="$(sha_of "$repo_b/dist/selftui-v0.1.1-linux-$arch.tar.gz" 2>/dev/null || true)"
  if [[ -n "$ha" && "$ha" == "$hb" ]]; then
    pass "umask reproducibility: $arch archive byte-identical under 0002 and 0022 ($ha)"
  else
    umask_ok=0
    fail "umask reproducibility: $arch differs (0002=$ha 0022=$hb)"
  fi
done
sa="$(sha_of "$repo_a/dist/SHA256SUMS" 2>/dev/null || true)"
sb="$(sha_of "$repo_b/dist/SHA256SUMS" 2>/dev/null || true)"
if [[ -n "$sa" && "$sa" == "$sb" ]]; then
  pass "umask reproducibility: SHA256SUMS identical under 0002 and 0022"
else
  umask_ok=0
  fail "umask reproducibility: SHA256SUMS differs (0002=$sa 0022=$sb)"
fi
# Exact member modes in the 0022 archive (and the 0002 archive must agree).
modes_ok=1
for repo in "$repo_a" "$repo_b"; do
  ar="$repo/dist/selftui-v0.1.1-linux-amd64.tar.gz"
  bm="$(tar_mode "$ar" selftui)"
  lm="$(tar_mode "$ar" LICENSE)"
  rm_="$(tar_mode "$ar" README.md)"
  if [[ "$bm" == "-rwxr-xr-x" ]] && [[ "$lm" == "-rw-r--r--" ]] && [[ "$rm_" == "-rw-r--r--" ]]; then
    pass "member modes in $(basename "$repo"): selftui=$bm LICENSE=$lm README.md=$rm_"
  else
    modes_ok=0
    fail "member modes in $(basename "$repo") wrong: selftui=$bm LICENSE=$lm README.md=$rm_ (want -rwxr-xr-x / -rw-r--r-- / -rw-r--r--)"
  fi
done

# --- summary ----------------------------------------------------------------
echo
if [[ $fails -eq 0 ]]; then
  echo "release-check-test: $checks checks, 0 failures — PASS"
  exit 0
fi
echo "release-check-test: $checks checks, $fails failures — FAIL" >&2
exit 1
