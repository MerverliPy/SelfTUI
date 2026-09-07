#!/usr/bin/env bash
# create-audit-pack.sh — manifest-complete, deterministic source snapshot for
# external audits (remediation of audit finding H-06).
#
# Produces a ZIP whose tracked members are EXACTLY `git ls-files` — dotfiles
# and .github/workflows included — so an audit package can never again omit
# files its inventory promises (the historical H-06 failure dropped
# .github/workflows/ci.yml, .github/workflows/release.yml, .gitignore, and
# .gitattributes). The tracked set and the archived content both come from
# the committed tree at HEAD, and every archived member is verified to be a
# safe relative path.
#
# Audit prompt/inventory files (e.g. PROMPT.md, FILE-INVENTORY.md) are added
# ONLY through explicit --extra TARGET=PATH arguments. A target that is a
# tracked path (which would silently substitute different content for a real
# file), duplicates another extra, or is not a safe relative path aborts the
# run before anything is written.
#
# Usage (run from anywhere inside the repository you want to snapshot):
#
#     scripts/create-audit-pack.sh                                # create, default output
#     scripts/create-audit-pack.sh --out pack.zip --force \
#         --extra PROMPT.md=/path/PROMPT.md \
#         --extra FILE-INVENTORY.md=/path/FILE-INVENTORY.md
#     scripts/create-audit-pack.sh verify <zip> [--extra TARGET]...
#
# Default output (create): dist/selftui-audit-pack-<HEAD>.zip — under the
# gitignored dist/ dir. An existing archive is NEVER overwritten silently:
# the script aborts unless --force is given (or --out names another path).
#
# Determinism: the archive is byte-identical for a given commit (fixed member
# order, entry timestamps fixed to the commit time, fixed deflate). On success
# the create form prints the archive path, tracked-file count, SHA256, and
# MANIFEST_MATCH=PASS; the verify form prints the manifest comparison.
#
# Env overrides (the `make audit-pack` target passes these through):
#     AUDIT_PACK_OUT      default archive path for create
#     AUDIT_PACK_EXTRAS   space-separated TARGET=PATH list appended to --extra
#
# Exit codes: 0 = created/verified; 1 = manifest or content mismatch;
# 2 = usage or precondition error. Tracked symlinks are unsupported (a symlink
# member has no content to archive) — the script refuses them rather than
# silently archiving nothing.
set -euo pipefail

# --- locate the repository (cwd may be anywhere inside it) -----------------
if ! repo="$(git rev-parse --show-toplevel 2>/dev/null)"; then
  echo "create-audit-pack: not inside a git repository (run from the repo to snapshot)" >&2
  exit 2
fi

# --- helpers ----------------------------------------------------------------
# tmp for tar/tracked intermediates; cleaned on exit.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# unsafe_path <path> — echoes a reason and returns 0 when path is NOT a safe
# relative archive path (absolute, empty, dot/empty components, traversal,
# backslash, colon components). Returns 1 when safe. The canonical check also
# runs inside the python verifier over every archived member; this shell copy
# gives fast pre-build failures for --extra targets.
unsafe_path() {
  local p="$1" part
  if [[ -z "$p" ]]; then echo "empty path"; return 0; fi
  if [[ "$p" == /* ]]; then echo "absolute path"; return 0; fi
  if [[ "$p" == *\\* ]]; then echo "backslash in path"; return 0; fi
  IFS='/' read -ra parts <<< "$p"
  for part in "${parts[@]}"; do
    if [[ -z "$part" ]]; then echo "empty component"; return 0; fi
    if [[ "$part" == "." || "$part" == ".." ]]; then echo "dot component '$part'"; return 0; fi
    if [[ "$part" == *:* ]]; then echo "colon component '$part'"; return 0; fi
  done
  return 1
}

# require_clean — the manifest is `git ls-files`; a dirty worktree would make
# ls-files disagree with the committed tree being archived.
require_clean() {
  if [[ -n "$(git -C "$repo" status --porcelain)" ]]; then
    echo "create-audit-pack: worktree is not clean - the tracked manifest must equal HEAD:" >&2
    git -C "$repo" status --porcelain >&2
    exit 2
  fi
}

head_sha="$(git -C "$repo" rev-parse HEAD)"
head_short="$(git -C "$repo" rev-parse --short HEAD)"
commit_epoch="$(git -C "$repo" log -1 --format=%ct HEAD)"

# tracked_manifest <file> — write sorted `git ls-files` (authoritative set).
tracked_manifest() {
  git -C "$repo" ls-files | LC_ALL=C sort -u > "$1"
}

# python_verify <tracked-file> <zip> [extra targets...] — the one canonical
# manifest comparator, shared by the `verify` form and create's self-check.
# Enforces safe-relative member paths, then compares tracked-vs-archived in
# BOTH directions (nothing missing, nothing unexpected). Extras must be
# declared explicitly. Prints one line per problem and ends with
# MANIFEST_MATCH=PASS or =FAIL; exit 0/1.
python_verify() {
  python3 - "$@" <<'PY'
import sys, zipfile

tracked_file, zip_path = sys.argv[1], sys.argv[2]
extras = sys.argv[3:]

def unsafe(name):
    n = name[:-1] if name.endswith("/") else name
    if not n:
        return "empty path"
    if n.startswith("/"):
        return "absolute path"
    if "\\" in n:
        return "backslash in path"
    for part in n.split("/"):
        if part == "":
            return "empty component"
        if part in (".", ".."):
            return "dot component %r" % part
        if ":" in part:
            return "colon component %r" % part
    return None

with open(tracked_file, encoding="utf-8", errors="surrogateescape") as fh:
    tracked = [ln.rstrip("\n") for ln in fh if ln.rstrip("\n")]

bad = []
try:
    names = zipfile.ZipFile(zip_path).namelist()
except (zipfile.BadZipFile, OSError) as exc:
    print("cannot read archive: %s: %s" % (zip_path, exc))
    print("MANIFEST_MATCH=FAIL")
    sys.exit(1)

members = []
for n in names:
    why = unsafe(n)
    if why:
        bad.append("unsafe archive path: %s (%s)" % (n, why))
        continue
    if not n.endswith("/"):
        members.append(n)

seen = set()
for t in extras:
    why = unsafe(t)
    if why:
        bad.append("invalid extra target: %s (%s)" % (t, why))
    elif t in seen:
        bad.append("duplicate extra target: %s" % t)
    seen.add(t)

tset = set(tracked)
mset = set(members)
eset = set(extras)
missing = sorted(tset - mset)
unexpected = sorted(mset - tset - eset)
for m in missing:
    bad.append("missing tracked: %s" % m)
for u in unexpected:
    bad.append("unexpected member: %s" % u)

for line in bad:
    print(line)
print("manifest: %d tracked, %d members, %d extras - missing %d, unexpected %d" %
      (len(tset), len(mset), len(eset), len(missing), len(unexpected)))
if bad:
    print("MANIFEST_MATCH=FAIL")
    sys.exit(1)
print("MANIFEST_MATCH=PASS")
PY
}

# python_build <tar> <tracked-file> <out.zip> <commit-epoch> [TARGET SOURCE]...
# Snapshot the committed tree into a deterministic zip. Validates BEFORE
# writing: tracked set vs archived tree in both directions, no tracked
# symlinks, extras safe/unique/non-shadowing. Deterministic: members written
# in sorted order with timestamps fixed to the commit epoch.
python_build() {
  python3 - "$@" <<'PY'
import os, sys, tarfile, time, zipfile

tar_path, tracked_path, out_path = sys.argv[1], sys.argv[2], sys.argv[3]
epoch = int(sys.argv[4])
pairs = sys.argv[5:]          # interleaved target, source
assert len(pairs) % 2 == 0

with open(tracked_path, encoding="utf-8", errors="surrogateescape") as fh:
    tracked = [ln.rstrip("\n") for ln in fh if ln.rstrip("\n")]

def fail(msg):
    print("audit pack build: %s" % msg)
    sys.exit(1)

# ---- load the archived tree -----------------------------------------------
tar = tarfile.open(tar_path, "r")
content = {}                  # name -> (bytes, mode)
for member in tar.getmembers():
    if member.isfile():
        fh = tar.extractfile(member)
        content[member.name] = (fh.read(), member.mode)
    elif member.isdir():
        continue
    elif member.issym() or member.islnk():
        fail("tracked symlink member %r has no archivable content" % member.name)
    else:
        continue  # devices/fifos never appear from git archive

# ---- tracked vs archived tree, both directions -----------------------------
tree_names = set(content)
tset = set(tracked)
if tree_names != tset:
    for m in sorted(tset - tree_names):
        print("archive missing tracked file: %s" % m)
    for m in sorted(tree_names - tset):
        print("unexpected archive member: %s" % m)
    fail("tracked set differs from the HEAD tree")

# ---- extras: safe, unique, never shadowing a tracked path ------------------
extra_src = {}
for i in range(0, len(pairs), 2):
    target, source = pairs[i], pairs[i + 1]
    if target in tset:
        fail("extra target %r shadows a tracked file" % target)
    if target in extra_src:
        fail("duplicate extra target %r" % target)
    if not os.path.isfile(source) or not os.access(source, os.R_OK):
        fail("extra source is not a readable regular file: %s" % source)
    with open(source, "rb") as fh:
        extra_src[target] = (fh.read(), 0o644)

# ---- deterministic write ---------------------------------------------------
dt = time.gmtime(epoch)
zip_dt = (dt.tm_year, dt.tm_mon, dt.tm_mday, dt.tm_hour, dt.tm_min, dt.tm_sec)
with zipfile.ZipFile(out_path, "w", zipfile.ZIP_DEFLATED) as z:
    # All members (tracked then extras), each written in sorted order; the
    # member set and timestamps are fixed per commit, so output is
    # byte-identical across runs.
    for name in sorted(tset) + sorted(extra_src):
        data, mode = content[name] if name in content else extra_src[name]
        zi = zipfile.ZipInfo(name, zip_dt)
        zi.compress_type = zipfile.ZIP_DEFLATED
        zi.external_attr = (mode & 0xFFFF) << 16
        zi.create_system = 3
        z.writestr(zi, data)
PY
}

# --- argument parsing -------------------------------------------------------
mode="create"
out=""
extras=()
force=0
positional=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    create) mode="create"; shift ;;
    verify)
      if [[ "$mode" != "create" ]]; then echo "create-audit-pack: duplicate subcommand" >&2; exit 2; fi
      mode="verify"; shift
      ;;
    --out)
      if [[ $# -lt 2 ]]; then echo "create-audit-pack: --out needs a path" >&2; exit 2; fi
      out="$2"; shift 2
      ;;
    --extra)
      if [[ $# -lt 2 ]]; then echo "create-audit-pack: --extra needs TARGET[=PATH]" >&2; exit 2; fi
      extras+=("$2"); shift 2
      ;;
    --force) force=1; shift ;;
    --) shift; positional+=("$@"); break ;;
    -*) echo "create-audit-pack: unknown option $1" >&2; exit 2 ;;
    *) positional+=("$1"); shift ;;
  esac
done

if [[ "$mode" == "create" && ${#positional[@]} -gt 0 ]]; then
  echo "create-audit-pack: unexpected positional argument(s): ${positional[*]}" >&2
  exit 2
fi

# Env overrides (documented defaults for `make audit-pack`).
if [[ "$mode" == "create" && -z "$out" && -n "${AUDIT_PACK_OUT:-}" ]]; then
  out="$AUDIT_PACK_OUT"
fi
if [[ -n "${AUDIT_PACK_EXTRAS:-}" ]]; then
  # shellcheck disable=SC2206
  extras+=($AUDIT_PACK_EXTRAS)
fi

if [[ "$mode" == "verify" ]]; then
  if [[ ${#positional[@]} -ne 1 ]]; then
    echo "create-audit-pack: verify needs exactly one <zip> argument" >&2
    exit 2
  fi
  case "${positional[0]}" in
    /*) zip_file="${positional[0]}" ;;
    *) zip_file="$repo/${positional[0]#./}" ;;
  esac
  if [[ ! -f "$zip_file" ]]; then
    echo "create-audit-pack: no such archive: $zip_file" >&2
    exit 2
  fi
  require_clean
  tracked_file="$tmp/tracked"
  tracked_manifest "$tracked_file"
  echo "== audit pack verify =="
  echo "archive: $zip_file"
  echo "HEAD:    $head_short (git ls-files, clean worktree)"
  # --extra TARGET[=PATH]: only the TARGET matters for membership here.
  extra_targets=()
  for e in "${extras[@]}"; do
    extra_targets+=("${e%%=*}")
  done
  python_verify "$tracked_file" "$zip_file" "${extra_targets[@]}"
  exit 0
fi

# --- create ----------------------------------------------------------------
require_clean
tracked_file="$tmp/tracked"
tracked_manifest "$tracked_file"
tracked_count="$(wc -l < "$tracked_file")"

# Tracked symlinks cannot be archived (no content) - refuse loudly.
if symlinks="$(git -C "$repo" ls-files -s | awk '$1 == "120000" {print $4}')" && [[ -n "$symlinks" ]]; then
  echo "create-audit-pack: tracked symlinks are not supported in audit packs:" >&2
  echo "$symlinks" >&2
  exit 2
fi

if [[ -z "$out" ]]; then
  out="dist/selftui-audit-pack-$head_short.zip"
fi
case "$out" in
  /*) out_abs="$out" ;;
  *) out_abs="$repo/$out" ;;
esac
if [[ -e "$out_abs" && $force -eq 0 ]]; then
  echo "create-audit-pack: refusing to overwrite existing archive: $out_abs" >&2
  echo "create-audit-pack: pass --out with a new path, or --force to overwrite explicitly" >&2
  exit 2
fi
mkdir -p "$(dirname "$out_abs")"

# Parse extras: each must be TARGET=PATH with real content to add.
extra_pairs=()
extra_targets=()
for e in "${extras[@]}"; do
  target="${e%%=*}"
  source="${e#*=}"
  if [[ "$e" != *=* || -z "$target" || -z "$source" ]]; then
    echo "create-audit-pack: --extra must be TARGET=PATH (got '$e')" >&2
    exit 2
  fi
  if reason="$(unsafe_path "$target")"; then
    echo "create-audit-pack: unsafe extra target '$target' ($reason)" >&2
    exit 2
  fi
  case "$source" in
    /*) source_abs="$source" ;;
    *) source_abs="$repo/$source" ;;
  esac
  if [[ ! -f "$source_abs" || ! -r "$source_abs" ]]; then
    echo "create-audit-pack: extra source is not a readable regular file: $source_abs" >&2
    exit 2
  fi
  extra_pairs+=("$target" "$source_abs")
  extra_targets+=("$target")
done
for target in "${extra_targets[@]}"; do
  if grep -Fxq -- "$target" "$tracked_file"; then
    echo "create-audit-pack: extra target '$target' is a tracked file - refusing to shadow it" >&2
    exit 2
  fi
done

# Snapshot the committed tree to a tar (preserves dotfiles, modes, and the
# exact committed bytes), then build the deterministic zip in one python pass.
tar_file="$tmp/tree.tar"
git -C "$repo" archive --format=tar -o "$tar_file" HEAD

if ! build_out="$(python_build "$tar_file" "$tracked_file" "$out_abs" "$commit_epoch" "${extra_pairs[@]}" 2>&1)"; then
  printf '%s\n' "$build_out"
  echo "create-audit-pack: archive build failed - nothing written to $out_abs" >&2
  exit 1
fi

echo "== audit pack: manifest-complete source snapshot =="
echo "repo:     $repo"
echo "HEAD:     $head_sha"
echo "tracked:  $tracked_count files (git ls-files)"
if [[ ${#extra_targets[@]} -gt 0 ]]; then
  echo "extras:   ${extra_targets[*]}"
else
  echo "extras:   none (add audit prompt/inventory files with --extra TARGET=PATH)"
fi
echo "archive:  $out_abs"
echo "sha256:   $(sha256sum "$out_abs" | awk '{print $1}')  $(basename "$out_abs")"

# Post-build self-check through the SAME canonical comparator as `verify`.
if ! python_verify "$tracked_file" "$out_abs" "${extra_targets[@]}"; then
  rm -f "$out_abs"
  echo "create-audit-pack: manifest check failed - removed $out_abs" >&2
  exit 1
fi
exit 0
