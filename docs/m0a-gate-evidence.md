# M0a gate evidence — go/no-go

**Date:** 2026-09-03 · **Gate:** PLAN §10 M0a + owner decision #6 (council finding D/B/A).
The gate gates the **agent scope**; model management + plain chat may proceed
regardless. Verdict at the bottom.

> **Historical evidence record (2026-09-03; label added 2026-09-04).** Gate
> measurement evidence for M0a: device references (Moshi/iPhone 16 Pro) and
> measured geometries are as-recorded. Where this file touches product scope
> (agent tool breadth, the `run_command` containment design), the 2026-09-04
> v0.1 product contract in `PLAN.md` governs — command execution does not ship.

## Gate definition (owner-fixed criteria)

1. **one validated target workflow** on the target SSH client;
2. **supported model / tool-loop compatibility** on the named release model;
3. **command containment** (run_command design);
4. **observed mobile usability**.

---

## Instrument & method

- `cmd/size-probe` — two independent reporters of the same pty:
  - **tui mode**: Bubble Tea `WindowSizeMsg` viewer (alt screen) — logs initial
    size, every resize, key delivery (code/text/mod), color profile;
  - **raw mode**: standalone `TIOCGWINSZ` + SIGWINCH reporter, pure CSV, no
    Bubble Tea — a cross-check that tui and raw must agree.
- Every event appends to `$XDG_STATE_HOME/selftui/probe.txt` (durable evidence).
- Local control harness: `scripts/probe-local.sh` (`make probe-local`) — runs the
  probe in a pty at fixed winsizes and mid-run resize; **5/5 PASS** on
  `2026-09-03` (50x100, 88x44, 120x40, 160x50 negotiated exactly; 88x44→100x50
  resize streamed live). This validates the whole pty → `WindowSizeMsg` pipeline
  locally so the device run is meaningful.

## Measured data

| Source | Geometry | Notes |
|--------|----------|-------|
| Local control (pty harness) | 50x100, 88x44, 120x40, 160x50 | exact negotiation PASS x4; mid-run resize PASS |
| **Moshi / iPhone 16 Pro (device run)** | **72 x 30 → compact** | TERM=tmux-256color, COLORTERM=truecolor → profile TrueColor; keys `j` (code 106), DEL (127) delivered with correct codes; no modifier loss |
| Local pty (dev session) | 88x44 → 100x50 resize | WindowSizeMsg streams size changes live |
| Edge observed | WindowSizeMsg 0x0 | before a pty negotiates a size (script w/o parent tty); app must tolerate a zero-size frame — probe now filters; app stores it harmlessly |

**Read-out:** Moshi's default portrait width is 72 cols, height 30 rows. Current
breakpoints (`compact ≤79`, `medium ≤119`, `wide ≥120`) hold it — recalibrated
constants carry the measurement (`devicePortraitCols = 72`), and layout.go now
cites the evidence. Landscape geometry was not captured this session (see
Residuals).

---

## Gate items

### 1. Validated target workflow — PASS (measured)

The measurement workflow over **Moshi** (ssh → host pty → Bubble Tea) is validated
end-to-end: pty negotiation delivers the client's real 72x30, resize plumbing works
(local control), keys arrive with correct codes/transforms, truecolor is
negotiated. The same pty path is what the M0 app shell uses (M0 was already
pty-boot-verified). Residual: reconnect behavior not exercised (deferred, M5/M6).

### 2. Model / tool-loop compatibility — PASS (spike 1 + OD3, prior sessions)

- **`qwen3:8b` is the release default**: native `tools` capability confirmed, 2-step
  tool-call PASS (args parsed, correct summary) — a repeated end-to-end tool-call
  coding round-trip on the target Ollama config (risk #4, council finding B
  single-condition test).
- **Dual dispatch is an empirical requirement, not a preference**: native
  `message.tool_calls` (qwen3/qwen3-vl) AND content-embedded tool-JSON
  (qwen2.5-coder:14b emits the call in `content`); `gemma3:12b` rejects tools
  (HTTP 400) → explicit plain-chat fallback, never silent.
- **qwen3 `thinking` phase** must be consumed by the runner (not shown as final
  output) — recorded for M2/M3 implementation.
- Material: LEDGER spike-1 + OD3 entries, commit `7ef1158`/`b07542c`.

### 3. Command containment — PASS (design, `docs/run-command-containment.md`)

argv allowlist, no shell/interpreter (argv-level rejection incl. embedded
metachars), scrubbed env, resource+output caps, process-group kill, per-call
confirmation, cwd jail, timeout, cancellation, serialization; controls ship inline
with the tool at M3b (council finding C). General shell is explicitly out of v1.

### 4. Observed mobile usability — PASS (measured, one geometry)

On the pty the app will actually run on (Moshi), the default portrait geometry is
72x30 — **narrow and short**, comfortably inside the compact stacked layout;
keyboard input and truecolor rendering are confirmed. App renders at 72x30 with no
panics (M0 shell; golden rendering at two widths already tested). One geometry
sampled; landscape and reconnect are residuals, not failures.

---

## Verdict — **GO** (agent scope proceeds)

All four gate criteria have evidence: validated target workflow (PASS), model /
tool-loop compatibility for `qwen3:8b` (PASS), command containment design (PASS),
observed mobile usability (PASS). No halt condition triggered (target model
repeatedly completes tool-call tasks).

## Residuals / carry to later milestones

- **Landscape geometry + rotation events** on the phone: not captured this run
  (only one portrait initial). Re-run `bin/size-probe -dur 30` in Moshi landscape
  or after font changes; expected ~150 cols (Wide) at default font.
- **Reconnect behavior** (SSH drops/redraw) — planned M5/M6 smoke.
- **Height-driven layout**: device height = 30 rows; models/agent views (M1a/M2)
  should respect height for stacking, not just width.
- **Scroll behavior** measured when the first real scrolling views land (M1a/M2).
- Landscape/large-font data will refine `mediumSplitMin` (currently 90, 1.25x the
  measured portrait width).