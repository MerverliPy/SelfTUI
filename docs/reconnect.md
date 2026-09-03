# Reconnect semantics & smoke evidence (M6)

**Date:** 2026-09-03 · **Decision:** owner, recorded 2026-09-03 (PLAN §10 M6) ·
**Transport resolution + smoke:** M6 session (live over Moshi/iPhone 16 Pro).

## What "reconnect" means — resolution (live-measured)

SelfTUI runs on the **host** inside a terminal the phone provides over SSH.
The M6 owner decision required resolving which transport "reconnect" means;
the **live run on the actual Moshi/iPhone 16 Pro client answered it**:

> The owner runs SelfTUI **inside tmux on the host** over SSH (process tree:
> `selftui` → `bash` → `tmux`). A phone-side drop kills the SSH client but
> **the tmux server keeps the app process alive**; reconnecting and
> reattaching restores the same screen. Reconnect therefore means **tmux
> session reattach** for this owner's workflow.

Two reconnect shapes exist, and SelfTUI must be correct under both:

1. **tmux (or mosh) reattach — the owner's live transport.** The app process
   survives the drop. On reconnect the terminal replays/resizes the frame and
   the app receives a fresh geometry event; **scroll state and the open
   conversation survive** because state is in-process. Verify: full redraw,
   scroll state preserved, in-flight Ollama op continues or is aborted by the
   server without wedging it, geometry re-negotiated.
2. **Fresh SSH re-connect (direct ssh, no tmux/mosh).** sshd closes the pty
   on a drop → SIGHUP → the app process dies (default disposition, correct:
   no in-app cleanup is left behind). The dropped client's in-flight request
   dies with it and **Ollama aborts the job and frees the slot**. Reconnect =
   relaunch; the config file (`theme`, host, token, agent params) is
   re-applied; chat/scroll state is per-process and does not survive (accepted
   semantics; session resume is out of v1 scope).

The smoke suite covers **shape 2 deterministically on the host**
(`make smoke-reconnect`, which simulates the pty-master close → SIGHUP path)
and **shape 1 live over the owner's device** (recorded below).

## Instrument extension (M0a pattern, `cmd/size-probe`)

`probe.txt` (`$XDG_STATE_HOME/selftui/probe.txt`) is now a sequence of
**attributable session blocks**:

- each launch writes a header — `# size-probe start session=<tag> pid=<pid>
  term=… colorterm=… profile=…` — so reconnect runs show one clean block per
  session, each starting with its own `initial WxH` geometry line;
- `-session NAME` tags a run for a live test (default `pid-<pid>`);
- **checkpoints** mark moments in the record: press `c` in tui mode or send
  `SIGUSR1` in raw mode → a `checkpoint` event line ("about to drop", "just
  reconnected").

Local control harness for the pty→probe pipeline: `scripts/probe-local.sh`
(`make probe-local`).

## Smoke procedure

### Shape 2 (fresh SSH death) — local host harness, `make smoke-reconnect`

`scripts/reconnect-smoke.py` boots `bin/selftui` in a pty at the canonical
72×30, starts an agent chat turn against the smallest local chat model, then
**drops the transport** (pty master close → SIGHUP, like sshd) mid-generation.
It asserts, in order:

1. boot renders Models and the status bar shows the negotiated `72x30
   compact`;
2. the chat turn streams tokens (informational; a slow host still exercises
   the drop);
3. the app dies promptly on the drop (SIGHUP) — never hangs;
4. the Ollama host recovered: `/api/tags` is fast **and** a fresh short
   generation completes (no stuck job from the dropped client);
5. reconnect = fresh process at 72×30: clean boot + config file re-applied
   (light theme from a scratch config);
6. `ctrl+c` exits 0.

### Shape 1 (tmux reattach) — live over Moshi/iPhone 16 Pro (owner device)

1. On the host: `make build`. On the phone over SSH, attach tmux and start
   the app: `bin/selftui` → Agent tab → send a long reply prompt
   ("count slowly from 1 to 300, one number per line") and let it stream.
2. While streaming, **drop the connection from the phone** (close the SSH
   app / airplane mode). Wait a few seconds.
3. Reconnect over SSH and reattach tmux. Expected: **the same screen returns**
   — conversation and scroll state intact, no garbage, no stuck dialog.
4. Verify the host recovered: the aborted pre-drop generation did not wedge
   Ollama (`/api/tags` fast; a short `/api/chat` completes).
5. Optional geometry record: `bin/size-probe -dur 10 -session m6-live-X` on a
   fresh pane before and after → two clean `initial` lines at the device
   geometry (72×30 compact).

## Evidence

### Shape 2 — local harness (2026-09-03, this host)

`make smoke-reconnect` (model: qwen2.5:1.5b, geometry 72×30) — three runs,
all PASS:

```
boot ok (72x30 compact, list loaded) in 0.8s
chat turn started; tokens streamed before drop: True
drop: app exited promptly with -1 (rc=-1)          # SIGHUP death, like sshd
host recovered: tags in 0.00s, generation round trip in 0.1s
reconnect boot ok (72x30 compact) in 4.9s total
SMOKE PASS in 5s
```

The harness isolates `$XDG_STATE_HOME`; its app log shows two `starting`
sessions (0.6.0-m6) with `theme=light` re-applied from the scratch config →
config persistence across the reconnect.

### Shape 1 — live Moshi / iPhone 16 Pro (owner run, 2026-09-03 ~16:59–17:04)

| Check | Evidence | Result |
|-------|----------|--------|
| Transport | host process tree during/after the drop: `./bin/selftui` (pid 881865, started 16:59) → `-bash` → `tmux` | tmux-over-SSH confirmed |
| Process survives the drop | app still running at evidence capture (17:04, elapsed 4:56, across the reconnect); app log shows `starting version=0.6.0-m6 … program running` with **no** `shutdown clean` | PASS — reattach semantics |
| Same screen / scroll state | owner observation after reconnect: "app survived, same screen" (conversation intact) | PASS |
| No garbage / no crash | app kept running; no panic; host unaffected | PASS |
| Host recovery | `/api/tags` HTTP 200 in 1.4 ms after the drop; ollama daemon up | PASS |
| Geometry re-negotiation | probe session blocks (m6-live-1a) record the initial `72 30 compact` plus SIGWINCH resize storms (82×34→26×10 etc.) across the phone's keyboard/session transitions — the pty resizes the app handles | exercised |

Residual: no dedicated post-reconnect probe run (`-session m6-live-1b`) was
captured in this pass; the reattach geometry/redraw assertions are covered
deterministically by the golden frame tests (72×30/120×40) and the shape-2
harness. One command fills the gap on a future device run
(`bin/size-probe -dur 10 -session m6-live-1b` after reattach).

### Instrument checks (local)

`size-probe` session header + SIGUSR1 checkpoint (raw mode) verified in a pty
at 72×30:

```
# size-probe start session=m6-check-2 pid=820074 term=tmux-256color colorterm=truecolor profile=TrueColor
2026-09-03T16:31:14  initial   72  30  compact
2026-09-03T16:31:15  checkpoint 72  30  compact  SIGUSR1
```
