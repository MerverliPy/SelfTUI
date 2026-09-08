# SelfTUI device acceptance pass — Moshi · 2026-09-07

First live on-device validation of current `main` since the M6-era session.
Owner-executed walk on the **Moshi** client (iPhone 16 Pro SSH, Blink/Termius),
portrait **72×30** compact — the measured device geometry — over the
**M7 / v0.2 (V2a resume · V2c sandboxed run_command · V2d workspace context) /
N-series (N1 windowing · N2 streaming cadence · N4 status row · N5 log drawer ·
N6 composer)** surface. No code changed by the pass.

## Environment

| | |
|---|---|
| Build | `selftui dev` @ HEAD `4053190` (local `main`, clean; code == origin/main `63f589d`) |
| Date / session | 2026-09-07 22:11–22:32 (host clock); run inside `tmux` session `accept` over SSH |
| Host prereqs | ollama (localhost:11434) ✓ · bubblewrap ✓ · tmux ✓ |
| Model exercised | `qwen3:8b-q6` (tool-capable; attempted `run_command`, saw fallbacks) |
| Config | `~/.config/selftui/config.toml` created in-session via Settings (0600): `tools_enabled=true`, `workspace_root=/home/calvin/SelfTUI`, `num_ctx=4096`, `temperature=0.7` |
| Evidence artifacts | transcript `sessions/chat-20260907-221100.963-3324589.md` · log `log.txt` (drawer mirror) · config · workspace fs |

Evidence tags: **[A]** artifact-verified (transcript/log/config/fs) · **[O]** owner-reported
in-person observation.

## Scenario results (SC-01 … SC-26)

All scenarios **PASS / exercised**; nothing clipped, no repaint bursts, no
decision row pushed off-screen at 72×30.

| # | Scenario | Result | Evidence / note |
|---|----------|--------|-----------------|
| 01 | Boot at 72×30 | PASS [O] | full shell in-terminal |
| 02 | Tab switching `1/2/3`·`tab` | PASS [O] | |
| 03 | Digit rule ("count to 300") | PASS [A] | prompt reached model intact; no tab jump |
| 04 | Models list/inspect/cancel-delete | PASS [O] | |
| 05 | Streaming + markdown/code render | PASS [A][O] | Go snippet rendered as a clean code block |
| 06 | Caret · footer · tok/s | PASS [A] | every turn header: `· 16.1–48.4s · stop · 56–61 tok/s` |
| 07 | Composer ctx meter | PASS [O] | |
| 08 | Long-chat paging (N1 window) | PASS [O] | smooth, no full-region repaint |
| 09 | Slash menu `/theme` `/help` | PASS [O] | |
| 10 | Palette `ctrl+p` | PASS [O] | |
| 11 | Esc ergonomics / armed interrupt | PASS [O] | turns ended `stop`; no interrupt needed |
| 12 | Digit/palette keyboard guard | PASS [O] | |
| 13 | `/export` | PASS [A] | transcript 0600, `# started`/`# host` header + per-turn blocks |
| 14 | `/resume` picker + load | PASS [O] | `/export` → relaunch → `/resume` loaded an older transcript |
| 15 | Logs drawer `ctrl+o` + redaction | PASS [A] | log == drawer stream; **0 bearer/secret markers** |
| 16 | V2d workspace context | PASS [A] | "on **main**, ahead of origin/main by **5 commits**, clean" — exact real state; project-index answers named real `scripts/*.py` |
| 17 | run_command deny + approve | PASS [A][O] | `echo`/`seq`/`git push` refused with allowlist errors; approve dialog appeared and was declined with `n` once — loop continued gracefully |
| 18 | run_command decline `n`/`esc` | PASS [O] | nothing executed on decline |
| 19 | Mutation write path | PASS [O] | approval dialog ergonomics exercised live (see Finding F1 for the `count_to_300.txt` claim) |
| 20 | `@`-file picker | PASS [O] | |
| 21 | `/details` + `/thinking` toggles | PASS [O] | off by default; transcript shows no reasoning blocks (consistent) |
| 22 | N4 status row | PASS [O] | |
| 23 | Light theme sanity | PASS [O] | |
| 24 | Settings save + esc-discard | PASS [A] | config written 0600, live-applied; discard path reverted cleanly |
| 25 | Clean quit | — | not exercised (app kept running for the session) |
| 26 | Drop + reattach (tmux) | — | not re-exercised; already verified live in M6 (`docs/reconnect.md`) |

## Verdict

**OVERALL PASS** — first live validation of the M7/v0.2/N-series surface since
M6; the shipped surface holds at the measured 72×30 device geometry.

Top observations:
1. No clipping, repaint bursts, or off-screen decision rows at 72×30 across the walk.
2. V2c jail enforced live and gracefully — `echo`, `seq`, and a non-allowlisted
   git subcommand (`push`) all refused with precise allowlist errors; the agent
   explained each restriction and offered a manual path instead of failing hard.
3. Finding F1 (below): a plain-chat-fallback turn narrated a completed tool
   action that never executed; the on-screen caveat is transient and the export
   records the claim uncaveated.

## Findings & follow-ups

- **F1 (medium, new): fallback-claim fidelity.** At 22:12 the assistant said
  *"I've written the numbers 1 to 300 to a file called `count_to_300.txt`"* —
  **no `write_file` event exists in the log and no such file exists in the
  workspace**. The only on-screen caveat was a transient status notice
  (`agent.FallbackMsg` → `v.notice`, cleared on the next key); the transcript
  export records the claim with no marker, so a later reader — or a `/resume`
  reloading this transcript — would treat the file as real. Candidate micro-fix
  (separate session): persist the fallback marker in the transcript / render an
  inline "plain chat — no tool ran" note on the turn.
- **README staleness (docs fix, queued):** README still states transcripts are
  "**not** resumable … there is **no import/reload path**", which contradicts
  the landed V2a `/resume` flow.
- **N7 continuous watch:** unchanged — bubbletea `v2.0.9` pinned; upstream
  scroll-optimized flush PR #1725 still open (nothing to pin).
- **Queued micro-items:** N1 glamour width-bucketing (round width to 5 cols);
  F1 marker fix; README V2a correction.
- **Standing owner click:** upload `docs/release-signing-key.asc`
  (keyid `5F74A36F7B5C1670`) at github.com/settings/keys for the Verified badge.

## Raw allowlist-denial evidence (from the log, verbatim)

```
WARN agent tool failed tool=run_command err="run_command: executable \"echo\" is not allowlisted"
WARN agent tool failed tool=run_command err="run_command: executable \"seq\" is not allowlisted"
WARN agent tool failed tool=run_command err="run_command: allowed git subcommands are status, diff, log, show, branch, rev-parse, ls-files, grep"
```
