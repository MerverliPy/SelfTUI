#!/usr/bin/env bash
# create-audit-pack-test.sh — regression suite for scripts/create-audit-pack.sh
# (runbook Task 07 / audit finding H-06: "manifest-complete audit packaging").
#
# Self-contained: builds disposable git fixtures under `mktemp -d` and tears
# them down on exit. No network, no live Ollama, no writes outside the temp
# dir, and nothing here is ever committed — run with:
#
#     bash scripts/create-audit-pack-test.sh
#
# The headline case reproduces the exact H-06 failure shape: a "naive
# non-hidden copy" (shell globs skip dotfiles) drops .github/workflows/ci.yml,
# .github/workflows/release.yml, .gitignore, and .gitattributes — and the
# shipped verifier must reject that pack. The remaining cases pin the
# contract: extras only via explicit arguments, no shadowing of tracked
# files, safe-relative member paths, deterministic output, refusal to
# overwrite an archive silently, and bidirectional manifest comparison.
set -euo pipefail
cd "$(dirname "$0")/.."        # repo root

script="scripts/create-audit-pack.sh"
script_abs="$(cd "$(dirname "$0")" && pwd)/create-audit-pack.sh"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT

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
    fail "$desc (output lacks '$needle'; got: $(printf '%s' "$text" | head -c 600))"
  fi
}

# expect_not_grep <desc> <text> <needle>
expect_not_grep() {
  local desc="$1" text="$2" needle="$3"
  if printf '%s' "$text" | grep -Fq -- "$needle"; then
    fail "$desc (output unexpectedly contains '$needle')"
  else
    pass "$desc"
  fi
}

# zip_members <zip> — prints the non-directory member names of a zip.
zip_members() {
  python3 - "$1" <<'PY'
import sys, zipfile
z = zipfile.ZipFile(sys.argv[1])
print("\n".join(n for n in z.namelist() if not n.endswith("/")))
PY
}

# make_zip <out.zip> <dir> <file>... — tiny deterministic zip of the given
# member names. Members that name no real file under <dir> (e.g. traversal
# probes like ../escape.txt) get placeholder bytes — the point of those is the
# member PATH, not its content.
make_zip() {
  local out="$1" dir="$2"
  shift 2
  python3 - "$out" "$dir" "$@" <<'PY'
import sys, os, zipfile
out, root = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
    for name in sys.argv[3:]:
        zi = zipfile.ZipInfo(name, (2000, 1, 1, 0, 0, 0))
        zi.compress_type = zipfile.ZIP_DEFLATED
        src = os.path.join(root, name)
        if os.path.isfile(src):
            with open(src, "rb") as fh:
                data = fh.read()
        else:
            data = b"probe member content\n"
        z.writestr(zi, data)
PY
}

# make_repo <dir> — fixture repo: three ordinary files plus the four paths the
# H-06 pack omitted (.github/workflows/ci.yml, .github/workflows/release.yml,
# .gitignore, .gitattributes) = 7 tracked files, all committed.
make_repo() {
  local d="$1"
  mkdir -p "$d/src" "$d/deep" "$d/.github/workflows"
  printf 'fixture readme\n' > "$d/README.md"
  printf 'int main(void){return 0;}\n' > "$d/src/main.c"
  printf 'nested content\n' > "$d/deep/nested.txt"
  printf '# ci fixture\n' > "$d/.github/workflows/ci.yml"
  printf '# release fixture\n' > "$d/.github/workflows/release.yml"
  printf '/bin/\n' > "$d/.gitignore"
  printf '* -whitespace\n' > "$d/.gitattributes"
  git init -q "$d"
  git -C "$d" add -A
  git -C "$d" -c user.name=audit-test -c user.email=audit-test@example.invalid \
    commit -qm "fixture init"
}

# run_script <workdir> <args...> — captures stdout+stderr and the exit code of
# the shipped script run from <workdir> into $r_out / $r_rc.
r_out=""
r_rc=0
run_script() {
  local dir="$1"
  shift
  set +e
  r_out="$(cd "$dir" && bash "$script_abs" "$@" 2>&1)"
  r_rc=$?
  set -e
}

fixture="$test_root/repo"
make_repo "$fixture"
f_pack="$test_root/pack.zip"

echo "== create-audit-pack-test =="

# --- 0. the script exists and is syntactically valid -----------------------
if [[ -f "$script" ]] && [[ -x "$script" ]] && bash -n "$script"; then
  pass "create-audit-pack.sh exists, is executable, and bash -n is clean"
else
  fail "create-audit-pack.sh missing / not executable / syntax error"
fi

# --- 1. H-06 headline: a naive non-hidden copy fails the manifest -----------
# Reproduce the historical failure: a pack built by ordinary non-hidden copy
# (globs skip dotfiles) carries only README.md, src/main.c, deep/nested.txt.
naive="$test_root/naive.zip"
make_zip "$naive" "$fixture" README.md src/main.c deep/nested.txt
run_script "$fixture" verify "$naive"
if [[ $r_rc -ne 0 ]]; then pass "naive non-hidden copy: verify exits nonzero"; else fail "naive non-hidden copy: verify exits 0"; fi
expect_grep "naive pack: ci.yml reported missing" "$r_out" "missing tracked: .github/workflows/ci.yml"
expect_grep "naive pack: release.yml reported missing" "$r_out" "missing tracked: .github/workflows/release.yml"
expect_grep "naive pack: .gitignore reported missing" "$r_out" "missing tracked: .gitignore"
expect_grep "naive pack: .gitattributes reported missing" "$r_out" "missing tracked: .gitattributes"
expect_grep "naive pack: FAIL token" "$r_out" "MANIFEST_MATCH=FAIL"

# --- 2. create produces a manifest-complete pack ---------------------------
run_script "$fixture" --out "$f_pack"
if [[ $r_rc -eq 0 ]]; then pass "create exits 0"; else fail "create exits $r_rc: $(printf '%s' "$r_out" | head -c 600)"; fi
expect_grep "create reports tracked count" "$r_out" "tracked:  7 files"
expect_grep "create reports archive path" "$r_out" "archive:"
expect_grep "create reports sha256" "$r_out" "sha256:"
expect_grep "create self-verify PASS" "$r_out" "MANIFEST_MATCH=PASS"

members="$(zip_members "$f_pack")"
tracked_now="$(git -C "$fixture" ls-files | LC_ALL=C sort -u)"
if [[ "$(printf '%s' "$members" | LC_ALL=C sort -u)" == "$tracked_now" ]]; then
  pass "pack members == tracked files exactly"
else
  fail "pack members != tracked files
  members: $(printf '%s' "$members" | tr '\n' ' ')
  tracked: $(printf '%s' "$tracked_now" | tr '\n' ' ')"
fi
# The four formerly omitted paths must be present in the created pack.
for need in ".github/workflows/ci.yml" ".github/workflows/release.yml" ".gitignore" ".gitattributes"; do
  if printf '%s' "$members" | grep -Fxq -- "$need"; then
    pass "created pack contains $need"
  else
    fail "created pack missing $need"
  fi
done
# Explicit standalone verify of the good pack agrees (PASS, no extras).
run_script "$fixture" verify "$f_pack"
if [[ $r_rc -eq 0 ]]; then pass "verify of complete pack exits 0"; else fail "verify of complete pack exits $r_rc"; fi
expect_grep "verify of complete pack PASS" "$r_out" "MANIFEST_MATCH=PASS"

# --- 3. deterministic: same commit => byte-identical archives ---------------
f_pack2="$test_root/pack2.zip"
run_script "$fixture" --out "$f_pack2"
sha1="$(sha256sum "$f_pack" | awk '{print $1}')"
sha2="$(sha256sum "$f_pack2" | awk '{print $1}')"
if [[ "$sha1" == "$sha2" ]]; then
  pass "deterministic output ($sha1)"
else
  fail "archives differ for the same commit ($sha1 vs $sha2)"
fi

# --- 4. an existing archive is never overwritten silently -------------------
run_script "$fixture" --out "$f_pack"
if [[ $r_rc -ne 0 ]]; then pass "second create with same --out exits nonzero"; else fail "second create with same --out exits 0"; fi
expect_grep "overwrite refusal names the archive" "$r_out" "refusing to overwrite existing archive"
run_script "$fixture" --out "$f_pack" --force
if [[ $r_rc -eq 0 ]]; then pass "--force overwrites explicitly"; else fail "--force create exits $r_rc"; fi

# --- 5. extras are explicit-only; shadowing a tracked file is refused -------
inv="$test_root/FILE-INVENTORY.md"
printf 'inventory fixture\n' > "$inv"
x_pack="$test_root/x.zip"
run_script "$fixture" --out "$x_pack" --extra FILE-INVENTORY.md="$inv"
if [[ $r_rc -eq 0 ]]; then pass "create with explicit extra exits 0"; else fail "extra create exits $r_rc"; fi
xmembers="$(zip_members "$x_pack")"
if printf '%s' "$xmembers" | grep -Fxq "FILE-INVENTORY.md"; then
  pass "extra FILE-INVENTORY.md present in pack"
else
  fail "extra FILE-INVENTORY.md absent"
fi
# Verify agrees only when the extra is declared.
run_script "$fixture" verify "$x_pack" --extra FILE-INVENTORY.md
if [[ $r_rc -eq 0 ]]; then pass "verify PASS when extra declared"; else fail "verify with declared extra exits $r_rc"; fi
expect_grep "verify PASS token with declared extra" "$r_out" "MANIFEST_MATCH=PASS"
run_script "$fixture" verify "$x_pack"
if [[ $r_rc -ne 0 ]]; then pass "verify FAIL when extra undeclared"; else fail "verify without declaring extra exits 0"; fi
expect_grep "undeclared extra reported as unexpected" "$r_out" "unexpected member: FILE-INVENTORY.md"
# An extra whose target is a tracked path would silently substitute content.
s_pack="$test_root/s.zip"
run_script "$fixture" --out "$s_pack" --extra README.md="$inv"
if [[ $r_rc -ne 0 ]]; then pass "extra target shadowing a tracked file is refused"; else fail "shadowing extra exits 0"; fi
expect_grep "shadow refusal names the tracked target" "$r_out" "README.md"
if [[ ! -e "$s_pack" ]]; then pass "no archive left behind by refused create"; else fail "refused create still wrote $s_pack"; fi

# --- 6. unsafe extra targets are refused ------------------------------------
run_script "$fixture" --out "$test_root/u1.zip" --extra ../evil.txt="$inv"
if [[ $r_rc -ne 0 ]]; then pass "traversal extra target refused"; else fail "traversal extra target exits 0"; fi
run_script "$fixture" --out "$test_root/u2.zip" --extra /abs.txt="$inv"
if [[ $r_rc -ne 0 ]]; then pass "absolute extra target refused"; else fail "absolute extra target exits 0"; fi

# --- 7. unsafe members inside an archive are rejected on verify -------------
evil1="$test_root/evil1.zip"
evil2="$test_root/evil2.zip"
make_zip "$evil1" "$fixture" ../escape.txt README.md
make_zip "$evil2" "$fixture" /abs.txt README.md
run_script "$fixture" verify "$evil1"
if [[ $r_rc -ne 0 ]]; then pass "verify rejects ../ member"; else fail "verify accepts ../ member"; fi
expect_grep "../ member named as unsafe" "$r_out" "unsafe archive path: ../escape.txt"
run_script "$fixture" verify "$evil2"
if [[ $r_rc -ne 0 ]]; then pass "verify rejects absolute member"; else fail "verify accepts absolute member"; fi

# --- 8. bidirectional drift: add one tracked file, delete another -----------
drifty="$test_root/drifty"
make_repo "$drifty"
drift_pack="$test_root/drift-pack.zip"
run_script "$drifty" --out "$drift_pack"
if [[ $r_rc -eq 0 ]]; then pass "drift fixture v1 pack created"; else fail "drift v1 create exits $r_rc"; fi
printf 'new tracked file\n' > "$drifty/newfile.txt"
rm "$drifty/src/main.c"
git -C "$drifty" add -A
git -C "$drifty" -c user.name=audit-test -c user.email=audit-test@example.invalid \
  commit -qm "drift: add newfile.txt, drop src/main.c"
run_script "$drifty" verify "$drift_pack"
if [[ $r_rc -ne 0 ]]; then pass "stale pack fails verification after drift"; else fail "stale pack passes after drift"; fi
expect_grep "drift: new tracked file reported missing" "$r_out" "missing tracked: newfile.txt"
expect_grep "drift: removed tracked file reported unexpected" "$r_out" "unexpected member: src/main.c"
expect_grep "drift: FAIL token" "$r_out" "MANIFEST_MATCH=FAIL"

# --- summary ----------------------------------------------------------------
echo
if [[ $fails -eq 0 ]]; then
  echo "create-audit-pack-test: $checks checks, 0 failures — PASS"
  exit 0
fi
echo "create-audit-pack-test: $checks checks, $fails failures — FAIL" >&2
exit 1
