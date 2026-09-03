#!/usr/bin/env bash
# probe-local.sh — M0a measurement harness.
#
# Validates the size-probe -> pty -> BubbleTea WindowSizeMsg pipeline at
# several controlled geometries and during a mid-run resize. This is the
# LOCAL control; the same probe is then run over real SSH clients (Blink /
# Termius) from the phone per docs/m0a-gate-evidence.md.
#
# Every run appends its session to $XDG_STATE_HOME/selftui/probe.txt, so this
# harness doubles as a regression record that pty negotiation works.
set -u
cd "$(dirname "$0")/.."

GO_BIN=bin/size-probe
MEASURE=(50x100 88x44 120x40 160x50) # narrow-tall portrait .. wide PC

pass=0; fail=0
out=$(mktemp)
trap 'rm -f "$out"' EXIT

# extract: pull size-probe event lines out of a script capture. awk splits
# on real tabs (grep -E won't interpret \t), so this is robust even when
# renderer escape bytes glue onto the same line.
extract() {
  awk -F'\t' 'NF >= 6 && $2 ~ /^(initial|resize)$/ && $3 ~ /^[0-9]+$/ { print }' "$1"
}

echo "== initial-size negotiation (raw -once at fixed winsize) =="
for dim in "${MEASURE[@]}"; do
  cols=${dim%x*}; rows=${dim#*x}
  script -qefc "stty cols $cols rows $rows; $GO_BIN -mode raw -once" "$out" >/dev/null 2>&1
  got=$(extract "$out" | tail -1 | awk -F'\t' '{print $3"x"$4}')
  if [ "$got" = "${cols}x${rows}" ]; then
    echo "  PASS  ${cols}x${rows} negotiated -> probe saw $got"
    pass=$((pass+1))
  else
    echo "  FAIL  ${cols}x${rows} negotiated -> probe saw '${got:-<none>}'"
    fail=$((fail+1))
  fi
  : >"$out"
done

echo "== mid-run resize (initial + resize events on the same session) =="
# Probe runs in the same pty process group (no job control), so a foreground
# stty resizes the pty -> SIGWINCH to that group -> WindowSizeMsg arrives live.
script -qefc "stty cols 88 rows 44; $GO_BIN -mode raw -dur 3 & sleep 1; stty cols 100 rows 50; wait" "$out" >/dev/null 2>&1
events=$(extract "$out")
echo "$events"
n=$(printf '%s\n' "$events" | grep -c resize)
if echo "$events" | grep -q $'88\t44\t' && echo "$events" | grep -q $'100\t50\t' && [ "$n" -ge 1 ]; then
  echo "  PASS  saw both 88x44 and 100x50 with $n resize event(s)"
  pass=$((pass+1))
else
  echo "  FAIL  expected 88x44 then 100x50, saw:"
  echo "$events"
  fail=$((fail+1))
fi

echo
echo "== result: $pass passed, $fail failed =="
[ "$fail" -eq 0 ]