# Project Ledger — SelfTUI

The persistent **work/decision log** for this project. `LEDGER.md` is the **past-facing**
record (what happened, why, what you hit, what happens next) that complements:

- **`PLAN.md`** — future-facing: architecture, re-cut ship-gated roadmap (`§10`), risks & owner decisions (`§11`).
- **`COUNCIL-MEMO.md`** — the advisory audit verdict that re-cut the roadmap.
- **`LEDGER.md`** (this file) — a chronological, append-only log for session efficiency and continuity.

**Rules:** consult at session start → append at session end. Never overwrite history;
always append. Keep the append template filled (work done → commands + exit codes →
decisions/blockers → next action).

> **One fresh session per step (binding — see `AGENTS.md`).** A "step" = one milestone
> (`PLAN.md` §10) or one owner-assigned task. Do not chain a second step in the same
> session. Finish the chosen step → append this file → commit → stop; start the next step
> in a new `pi` session with cwd at this repository's root.

> **Historical record (label added 2026-09-04).** Every `##`/`###` entry below
> is a dated, append-only record of work and claims **as they stood when the
> entry was written**. Old product claims retained inside entries — e.g. the
> transcript command's earlier name, pre-release version strings, the removed
> `run_command` behavior, planning-era "chat sessions persist" framing, and
> personal local paths — are **historical**: superseded where they conflict
> with the 2026-09-04 v0.1 product contract in `PLAN.md` and the dated
> release-hardening entry appended at the bottom of this log.

---

## Ledger conventions

- **Once section per session**, newest last, fixed `## YYYY-MM-DD` header. Record the
  milestone being worked (e.g. `M0`, `M0a`, `M1a`).
- Use the append template below; leave placeholders explicit (`TBD`) rather than empty.
- Canonical commands & artifacts live in the footer (speed cheatsheet) — update them as
  the build adds scripts/tests.
- Record **exit codes for every command run** so a later session never re-guesses.

---

## Session log

### 2026-09-03 — Planning & council audit (no code yet)
**Milestone:** Pre-M0 (planning) · **Result:** Recommendations captured; roadmap re-cut.

**Work done**
- Created the full project plan (`PLAN.md`): goals, confirmed decisions, architecture,
  project layout, domain/data flow, coding-agent design, responsive layout, settings,
  dependencies, roadmap, risks, next step.
- Named the project **SelfTUI** (self-hosted + TUI; module id `selftui`, binary `self-tui`).
- Resolved the four design forks with the owner (Go + Bubble Tea; SSH-into-host on
  iPhone; full coding agent; configurable local/remote Ollama) + git location
  (standalone repo in `/home/calvin/TUI`) + glamour markdown.
- Ran a **3-advisor council audit** (`council-architect`, `council-operator`,
  `council-skeptic`), 2 passes, converged. Verdict: reshape-and-proceed with a hard
  `M0a` spike gate before agent work. Wrote `COUNCIL-MEMO.md`.

**Commands + exit codes**
- `pwd` `0` · `ls -la` `0` · `git rev-parse --is-inside-work-tree` `0` ·
  tooling check (`which go/rustc/cargo/node/python3`, versions) `0`
- Go 1.22.2 / Rust 1.96 / Node v22.22 / Python 3 detected. No code compiled this session.
- `git status --short` `0` (clean; TUI dir tracked by parent dotfiles repo — pending `/TUI` gitignore).

**Decisions / lines to respect (from council + owner)**
- Re-cut roadmap: M0a hard gate before agent; split M1a/M1b; M3a read-only → M3b mutation;
  inline safety; alpha after M2; M6 = release acceptance (hardening pushed inline).
- `run_command` v1 = **argv-allowlist, no shell/interpreter**, scrubbed env, limits,
  process-kill, per-call confirm; else disabled/dropped. Confining file paths ≠ sandbox.
- Responsive/breakpoints lead in M0, driven by **measured** SSH geometry, not assumed 88-col.
- Log **to a file**, not stderr (stderr corrupts alt-screen over SSH).
- Pin **Charm as one v1-or-v2 set** after a compile spike; never mix majors.
- No-tool-model behavior: **never silent fallback**.
- Buy into `LEDGER.md` (this file) for all future session handoffs.

**Blockers / open owner decisions (carry to next session)**
- ✅ **Repo setup done** — dir renamed `/home/calvin/TUI` → `/home/calvin/SelfTUI`; `git init` run; initial commit `dda790a` on `main`; dotfiles ignores it automatically (its `.gitignore` uses `*` default-ignore, so a `/TUI` entry was unnecessary).
  - *Harness note:* the old session cwd `/home/calvin/TUI` was re-created as a thin `REDIRECT-NOTE.md` stub so the shell tool works; delete it once a new session opens on `/home/calvin/SelfTUI`.
- 6 owner decisions still unresolved: Charm v1 vs v2; concurrency primitive (channel vs
  `tea.Program.Send`) + nested models vs god `Update`; no-tool-model behavior; `rg` vs
  pure-Go grep; serialize Ollama jobs; M0a go/no-go gate criteria.
- `go.mod` not yet created (planning stage; module `selftui` suggested).

**Next action**
- On green-light: run **M0** (repo setup, pin Charm set after compile spike, skeleton +
  responsive shell, cancellation plumbing), then **M0a** (measure SSH geometry; Spike 1 =
  Ollama tool-calling on target model(s); Spike 2 = `run_command` containment design;
  define + pass the go/no-go gate).

---

### 2026-09-03 — Session infra diagnosis (tooling, not project code)
**Milestone:** n/a · **Result:** both flagged issues diagnosed; Flag 2 remediated.

**Flag 1 — cwd stub:** renaming `TUI`→`SelfTUI` mid-session stranded the session cwd, breaking
bash. Re-created `/home/calvin/TUI/REDIRECT-NOTE.md` stub to satisfy the cwd pointer. **Fix:**
open a new session with cwd `/home/calvin/SelfTUI`, then `rm -r /home/calvin/TUI`. (Owner chose
to keep the stub for this session.)

**Flag 2 — subagent "Insufficient Balance":** `deepseek/*` provider credits exhausted (HTTP 402).
`settings.json` pinned worker/delegate/scout/researcher/polisher to it, so those failed. Also
found: `oracle`/`council-architect` pinned nonexistent `grok-4.5`.

**Commands + exit codes:** diagnosis via `git status`, `subagent models`, `rg` (hidden). `0`

**Fix applied (owner approved):** `~/.pi/agent/settings.json` overrides → `opencode-go/*`;
`council-architect.md` `grok-4.5`→`opencode-go/grok-4.6`; `council-operator.md`
`deepseek/*`→`opencode-go/deepseek-v4-pro`. Verified: settings.json valid JSON; `subagent`
`models` resolves all to opencode-go. `deepseek` still listed in registry but unfunded — avoid it.

---

### 2026-09-03 — M0a Spike 1: Ollama native tool-calling probe (LINCHPIN)
**Milestone:** M0a gate · **Result:** PARTIAL PASS — native tool_calls work on some models,
but NOT on the primary coding model ⇒ agent needs a content-based tool fallback.

**Method:** throwaway probe `/tmp/spike1.py` (kept out of repo) POSTs `/api/chat` with an
Ollama `tools` schema (`read_file`, `echo_text`) and drives a real 2-step round trip
(model emits tool_call → we inject tool result → model must summarize) on each installed
model. Tested Ollama server **0.33.1**. Live models: qwen2.5-coder:14b, gemma3:12b,
qwen3-vl:8b, qwen2.5:1.5b (+ `-pi` variants).

**Results (compact):**
- `qwen2.5-coder:14b` / `-pi` — `Capabilities: tools` TRUE, but emits the call as plain
  JSON in `message.content`, `message.tool_calls` EMPTY → **Ollama did not convert**. Not
  usable for a native tool loop.
- `gemma3:12b` / `-pi` — HTTP 400 `does not support tools` → unusable.
- `qwen3-vl:8b` — ✅ full 2-step PASS: proper `tool_calls`, args auto-parsed, streamed over
  ~376 lines; after injecting the read_file result, produced a correct final summary.
- `qwen2.5:1.5b` — emits tool_calls (PASS syntactically) but too small / wrong answer.

**Commands + exit codes:** `ollama --version` (0.33.1) `0`; probe runs `0`.

**Decision implications (see PLAN §6 / risk #4):**
1. **Agent tool-dispatch must support BOTH** native `message.tool_calls` (qwen3-vl) **and**
   content-embedded tool-JSON parsing+validation (qwen2.5-coder) — otherwise the primary
   coding model can't drive the agent. This makes the "explicit fallback, never silent"
   owner decision an **empirical requirement**, not a preference.
2. Native tool loop is **confirmed feasible** (qwen3-vl 2-step pass) → M3 scope stays.
3. **Model recommendation for the coding agent:** prefer a genuinely tool-capable coding
   model (e.g., a qwen3 non-VL coder, or qwen3-vl) OR implement the content-JSON parser so
   `qwen2.5-coder:14b` works. Owner to pick (owner decision OD3).

**Follow-up — OD3 RESOLVED (owned):** pulled + validated **`qwen3:8b`** — Capabilities:
`tools` + `thinking`; full 2-step native tool PASS (args parsed, correct summary). This is
now the recommended default agent coding model. Dual-dispatch (native + content-JSON) is
still required for backward/pick-any-model behavior. **Note:** qwen3 emits a `thinking`
phase — the agent loop must handle/omitt thinking messages appropriately (see PLAN §6).

---

## Appendix — canonical commands (update as build grows)

| Task | Command |
|------|---------|
| Plan | `cat PLAN.md` · `cat COUNCIL-MEMO.md` |
| Ledger | `cat LEDGER.md` |
| Build (future) | `make build` (TBD) |
| Test (future) | `make test` (TBD) |
| Lint (future) | `make lint` (TBD) |

---

## Append template (copy for a new session)

```
### YYYY-MM-DD — <short scope; milestone>
**Milestone:** <e.g. M0a — gate> · **Result:** <done / blocked / partial>

**Work done**
- <bullet per meaningful change>

**Commands + exit codes**
- `<command>` `0` · `<command>` `1` (note why)

**Decisions / lines to respect**
- <new decisions made this session>

**Blockers / open decisions (carry to next session)**
- <open owner decisions or blocking issues>

**Next action**
- <first concrete step for the next session>
```
### 2026-09-03 — M0: repo skeleton + Charm v1-v2 compile spike + pinned set (DONE)
**Milestone:** M0 · **Result:** done — app boots (pty-verified), tabs work, clean build,
Charm set pinned (v2 line), tests green.

**Work done**
- **Charm v1-v2 compile spike** (throwaway, kept in `/tmp/spike-v1`, `/tmp/spike-v2`):
  headless harness (no tty; drives `tea.Model` via direct `Update`/`View` calls) exercising
  bubbletea core (WindowSizeMsg, tab keys, quit), bubbles (list/viewport/textarea/spinner/table),
  huh (construct+bind+validate), glamour, log, xdg, toml.
  - **v1 set** (bubbletea v1.3.10, lipgloss v1.1.1-pseudo, huh v1.0.0, bubbles v1.0.0, glamour
    v1.0.0): **19/19 PASS**; caveat — glamour pins lipgloss to a *dev pseudo-version*
    (`v1.1.1-0.20250404…`), and this line's newest patches also require Go ≥1.24.
  - **v2 set** (`charm.land/…/v2`: bubbletea v2.0.9, lipgloss v2.0.6, huh v2.0.3, bubbles
    v2.2.1, glamour v1.0.0, log v1.0.0): **18/18 PASS**. Key findings:
    1. v2 modules self-declare as **`charm.land/…`** (moved off github.com) and require
       **go ≥1.25**.
    2. glamour v1.0.0 drags `x/cellbuf@v0.0.13` into the graph, which is incompatible with
       the `x/ansi v0.11.8` that lipgloss/v2 v2.0.6 requires → **cellbuf must be pinned
       ≥v0.0.15** (applies the moment glamour joins at M2).
    3. v2 API deltas vs v1: `View() tea.View` (struct, use `tea.NewView`), `KeyMsg` is an
       interface (`KeyPressMsg`/`KeyReleaseMsg`, `Key{Code,Text,Mod}`), `KeyMod.Contains`,
       `tea.Quit()` is a Msg-returning func, `lipgloss.Color` is now `func(string) color.Color`,
       `Italic(bool)` explicit, `table/table.New(opts...)` + `WithColumns/WithRows/WithHeight/WithWidth`,
       viewport `New(options…)`.
- **OD removal — Charm version set (plan risk #7), OWNER decision resolved by spike evidence:**
  pinned the **v2 aligned set**. Rationale: v2 is Charm's current maintained major; v1's own
  latest needs ≥1.24 anyway, so there is no toolchain-compatibility reason to stay on v1;
  v2's context-native program (`tea.WithContext`) + layered `View` fit M0 cancellation and the
  iPhone path. `go.mod` declares `go 1.25.0` — within plan §2's "Go 1.22+" floor; `GOTOOLCHAIN=auto`
  fetches the toolchain. "Never mix majors" honored: our direct imports are 100% v2 line;
  glamour/log's transitive lipgloss-v1 is inert (no cross-major type sharing).
- **M0 code** (repo root = module `selftui`): `go.mod` pinned set (`charm.land/bubbletea/v2 v2.0.9`,
  `charm.land/lipgloss/v2 v2.0.6`, `github.com/adrg/xdg v0.5.3`, `github.com/charmbracelet/log v1.0.0`,
  `github.com/pelletier/go-toml/v2 v2.4.3`, + `x/cellbuf v0.0.15` pin); `cmd/self-tui/main.go`
  (flags `-host/-theme/-config/-verbose`, config chain, file logger via `xdg.StateFile`, cancellation:
  `signal.NotifyContext` → `tea.WithContext`, clean quit path); `internal/config` (struct §5 + defaults,
  load chain flags > env > file > defaults, `Overrides` struct, TOML file; config_test.go ×6);
  `internal/ui` (root `App` model with Tab/ShiftTab/1-2-3/ctrl+c keys + `WindowSizeMsg`;
  `styles.go` centralized dark/light theme; `layout.go` breakpoint system `Compact(≤79)/Medium(≤119)/Wide`
  + `ForModels` geometry — thresholds centralized for M0a measurement; `components.go` TabBar/StatusBar;
  placeholders per tab; `app_test.go` headless boot/tabs/quit/two-widths/layout-boundary tests =
  10 assertions).
- **Tooling:** `Makefile` (`build/test/lint(=vet+fmt)/run/check`); `README.md` (status, build, config
  sources, nav); logs never touch stderr (risk #10).

**Commands + exit codes**
- `go version` (1.22.2; GOTOOLCHAIN auto) `0`
- spikes: `go build` v1 `0` · run `0` (19/19) · v2 `0` (18/18); toolchains auto-fetched go1.24.2/go1.25.8/go1.26.8
- `make check` (build+test+vet+fmt) `0` — all tests pass
- pty boot smoke `script -qefc "timeout 2 ./bin/selftui" /tmp/boot.out` → exit `124` (ran until kill,
  no panic; bubbletea v2 init + Kitty protocol confirmed)
- non-tty run fails cleanly: `selftui: tui: bubbletea: could not open TTY` `1` (expected)

**Decisions / lines to respect**
- Charm **v2 aligned set pinned** (see OD above); cellbuf ≥v0.0.15 pin is a permanent go.mod line.
- assert in LEDGER: when glamour is added (M2), cellbuf pin is already in place.
- Breakpoint thresholds (79/119) are placeholders pending M0a real-width measurement.
- Config: only host/theme/default_model/workspace_root wire through file+env+flags in M0; agent params
  stay defaults until M4. Logs to `$XDG_STATE_HOME/selftui/log.txt`.

**Blockers / open decisions (carry to next session)**
- None blocking. M0a (gate) is next: measure real SSH widths, Ollama tool spikes (already partially
  done via earlier spike1 + OD3), run_command containment design.

**Next action**
- Begin **M0a** in a fresh session: measure WindowSizeMsg on real SSH clients (Blink/Termius),
  recalibrate `layout.go` thresholds, complete the go/no-go gate evidence.

### 2026-09-03 — M0a: GATE — SSH measurement, layout recalibration, go/no-go evidence (DONE — GO)
**Milestone:** M0a — gate · **Result:** done — **GO**; all 4 gate items evidenced; next is M1a.

**Work done**
- **`cmd/size-probe`** — two-mode measurement instrument: **tui** = Bubble Tea
  `WindowSizeMsg` viewer (alt screen via `View.AltScreen`, v2 idiom), **raw** = standalone
  `TIOCGWINSZ` + SIGWINCH CSV reporter with *no* Bubble Tea (deliberate: two independent
  reporters of the same pty cross-check each other). Every event → screen + durable log
  `$XDG_STATE_HOME/selftui/probe.txt`. Filters `WindowSizeMsg` 0x0 (a pty can deliver a
  zero-size frame before negotiating a size — app must tolerate it; observed under
  `script` without a parent tty).
- **`scripts/probe-local.sh` + Makefile targets** (`probe`, `probe-raw`, `probe-local`,
  `probe-build`): local control harness, **5/5 PASS** — exact negotiation at 50x100 /
  88x44 / 120x40 / 160x50, plus live 88x44→100x50 mid-run resize. Bugs fixed along the
  way: POSIX gives background jobs stdin=/dev/null, so raw mode now ioctls `/dev/tty`
  (controlling terminal); tab-splitting must use awk (grep -E won't interpret `\t`).
- **Owner decision — target client = Moshi** (custom answer; plan said Blink/Termius).
  **Device run captured:** initial **72x30 → compact**, TERM=tmux-256color +
  COLORTERM=truecolor (TrueColor), keys `j`(106)/DEL(127) delivered with correct codes.
  Landscape not rotated during the run (residual).
- **`internal/ui/layout.go` recalibration** — constants now evidence-anchored:
  `devicePortraitCols = 72` (measured), `compactMax = 79`, `mediumMax = 119`,
  `mediumSplitMin = 90` (replaces magic 90; 60/40 split keeps detail ≥50 cols).
  Comments cite `docs/m0a-gate-evidence.md`. Measured 72 sits inside compact with margin;
  landscape ≈150 cols (Wide) at default font.
- **`app_test.go`** — `TestMeasuredDeviceWidthIsCompact` pins the measurement.
- **`docs/run-command-containment.md`** (Spike 2) — argv allowlist, no shell/interpreter
  (incl. metachar rejection), scrubbed env, output/resource caps, process-group kill,
  per-call confirm, cwd jail, timeout, cancel, serialization; ships inline at M3b.
- **`docs/m0a-gate-evidence.md`** — the four gate items + verdict **GO** + residuals.
- PLAN §10 M0a tick, §11#6 (Moshi + instrument), §12 next-step refresh; README Moshi.

**Commands + exit codes**
- `make probe-local` `0` (5/5) · `make check` `0` (build+test+vet+fmt) · `gofmt -l .` empty
- probe smoke under `script` / with `stty`: tui `0`, raw `0`
- `ssh localhost …` `255` (no pubkey — unnecessary; pty→WindowSizeMsg path validated by harness)

**Decisions / lines to respect**
- **Gate = GO** — agent scope (M3a/M3b) proceeds; model mgmt + chat proceed regardless.
- Thresholds compact ≤79 / medium 80–119 (split ≥90) / wide ≥120 are
  measurement-backed; revisit when landscape/large-font data lands.
- Moshi is the primary mobile client; Blink/Termius/iSH are similar but *unmeasured*.
- `qwen3:8b` remains the release agent-model default (spike 1 + OD3); dual dispatch
  stays an implementation requirement.

**Blockers / open decisions (carry to next session)**
- None blocking. **Residuals:** landscape geometry + rotation events (owner: run
  `./bin/size-probe -dur 30` rotated, or after font change); reconnect behavior
  (M5/M6 smoke); height-aware layout (device height 30 rows — stack by height for
  M1a/M2 views, not just width); scrolling measurement when scrollable views land.

**Next action**
- Fresh session: **M1a** — ollama client `tags`/`show` + Models view selection &
  inspect pane (exit: list + inspect models live).

### 2026-09-03 — M1a: Model list/show — ollama tags/show + Models list/inspect pane (DONE)
**Milestone:** M1a · **Result:** done — list + inspect **live** (local host smoke; wide + compact). Next is M1b.

**Work done**
- **`internal/ollama/`** — thin typed client (PLAN §4 tree): `client.go` (base URL
  trim, optional Bearer token, 30s timeout, body cap, `{"error":…}` surfacing on
  non-2xx), `tags.go` (`List` → GET /api/tags → []Model per §5 domain struct),
  `show.go` (`Show` → POST /api/show → Details incl. license/modelfile/parameters/
  template/capabilities/model_info), `types.go`. **11 tests**: parse, bearer header,
  no-token, show request body + mixed-type model_info, empty-name reject, API error
  surfaced, HTTP status fallback, refused connect, timeout, context cancel,
  trailing-slash normalization (all httptest-based, no live server).
- **`internal/ui/models_view.go`** — the Models tab: **bubbles list** component
  (`charm.land/bubbles/v2 v2.2.1` added to the pinned set; needs bubbletea ≥2.0.8,
  we hold 2.0.9) with theme-matched delegate (violet accent selection), filtering
  off, quit keybindings off (default binds `v`→quit — must disable!), `u`/`d`
  unbound from paging so they scroll the inspect pane. Stacked on compact
  (enter opens/closes pane, esc closes), auto-inspect on wide/medium-split when the
  pane is persistently visible. Detail = key facts + sections (parameters,
  template, modelfile, model info, license), word-wrapped, scroll-clamped both in
  the handler and at render. `humanBytes`, `wrapLines` helpers.
- **App wiring** — `App` owns `ModelsView`; `Init` fetches tags at boot; keys go to
  the Models tab only when active; async msgs always routed; `main.go` builds
  `ollama.New(cfg.Host, cfg.AuthToken)`; `Styles` gained `Pane` + `Error`.
- **Live smokes (exit criterion — list + inspect live):** local Ollama running
  `qwen3:8b`/`qwen2`×3/`qwen3vl`/`gemma3`. Wide 120x40 pty capture: list rendered +
  **auto-inspect populated live** (family/param/quant, `caps completion, tools,
  thinking`, PARAMETERS incl. temperature 0.6, template). Compact **72x30** (the
  measured Moshi geometry): stacked list, no detail pane, no panic.

**Commands + exit codes**
- `go get charm.land/bubbles/v2@v2.2.1` `0` (go.mod MVS: bubbles pulls
  x/ansi v0.11.7/runewidth v0.0.27→v0.0.24 net upgrade but still ≥ pins)
- `make check` (build+test+vet+fmt) `0` — all green; `gofmt -l` empty
- `go test ./...` `0` (config cached, ollama 11, ui 16 incl. wrap/bytes units)
- wide smoke `script … cols 120 rows 40; timeout 4 ./bin/selftui` → 124 (timeout
  kill as designed), capture greps: live list + inspect text present
- compact smoke 72x30 → app data captured cleanly (wrapper script lingered on the
  small pty — harness quirk, not app); list + description rows present

**Decisions / lines to respect**
- **`GET /api/show` in PLAN §5 is wrong — the API is POST.** Client + §5 table
  corrected; Evidence = the running server + API docs.
- **bubbles default KeyMap claims `v` (quit), `u`/`d` (paging), `/` (filter).**
  Disabled quit + filtering, removed `u`/`d` from paging. `DisableQuitKeybindings`
  is mandatory or the app quits on `v`.
- Delegate height must be ≥2 for the description line to render (desc loop is
  `i < height-1`); SetHeight(1)+ShowDescription=true renders zero desc lines.
- List scroll state: clamped in the key handler AND at render via shared
  `detailPaneDims`/`maxScroll`; `SetSize` happens on WindowSizeMsg (+ the render
  copy), paginator sizes stay coherent.
- Errors surface inline (red) with an `r` retry hint; loading/empty states render
  as hints — never silent.
- Auto-inspect triggers once per selection change (dedup via `detailName`) and on
  load for side-by-side only (compact stays quiet until enter).

**Blockers / open decisions (carry to next session)**
- None blocking. **Residuals for M1b/M5:** spinner component (bubbles ships one —
  reuse for pull progress); pull/delete confirmation UX; height-aware layout
  already exercised (detail scroll); landscape/reconnect still unmeasured (M5).
  Bubbles list brings `textinput`/`key`/`paginator` deps into go.mod (fine — one
  aligned set, M6 audit).

**Next action**
- Fresh session: **M1b** — `DELETE /api/delete` w/ confirm + streaming `POST
  /api/pull` with bubbles spinner + progress; then ALPHA candidate 1.

### 2026-09-04 — M1b (delete w/ confirm + streamed pull w/ spinner)
**Milestone:** M1b · **Result:** ✅ done — exit criterion met live; **ALPHA candidate 1** ready.

**Work done**
- **`internal/ollama/delete.go`** — `Delete` → `DELETE /api/delete` with `{"name"}` body;
  errors surface via the shared `apiError` path (verified live: 404 + `{"error":"model 'x' not found"}`).
- **`internal/ollama/pull.go`** — `Pull(ctx, name, onProgress)` → streaming `POST /api/pull`
  (NDJSON): per-line `{status,digest,total,completed}` → `PullProgress` callbacks; **in-band
  `{"error":…}` with HTTP 200 is the error channel** (verified live); `success` line ends the
  stream. Pull uses a new `Client.stream` http.Client **without the 30s request timeout** —
  the caller's context is the deadline (cancellation, Ctrl+C). `Client` gains the `stream`
  field (`client.go`).
- **Models view (M1b UX)** — `internal/ui/models_view.go`:
  - `x` → centered confirm dialog "Delete <name>? (size) · y confirm · esc cancel";
    `y` runs the DELETE, failure keeps the dialog open with the error inline for retry,
    success clears the stale detail + reloads the list, notice "deleted <name>".
  - `p` → name-entry dialog (bubbles textinput, placeholder `qwen3:0.6b`); `enter` starts
    the pull, `esc` aborts, empty name ignored.
  - Pull dialog: bubbles spinner + phase status + progress bar (`progress.ViewAs`) + bytes
    when the server reports a layer size; **`esc` cancels** the in-flight pull (context
    cancel → surfaced "context canceled" error; stale `⚠ pull failed` clears on reload).
  - Activity channel pattern (PLAN §8): pull goroutine → `pullCh chan tea.Msg` (buffered 64),
    UI resubscribes `waitPullCmd` on every progress message; spinner tick keeps animating.
  - **Modal guard:** open dialogs swallow every key (nav + global `1/2/3` tab jump via new
    `ModelsView.ModalOpen()` on `App`) — otherwise typing a model name containing a digit
    jumped tabs (found via smoke).
  - Compact (no-pane) layout reserves a hint row: "x delete · p pull · r refresh" (or the
    transient notice / pull error); wide/medium puts notice/error atop the detail pane.
  - Empty state updated: "no models installed — press p to pull one".
- **Tests (55 total, all green):** ollama +11 (delete posts name/surfaces err/rejects empty;
  pull streams phases+layer totals/in-band error/empty-name/context-cancel). ui +17
  (delete confirm flow, esc/n cancel, error-inline retry, empty-list x, modal swallows nav,
  pull input→progress render→drain→reload, empty name ignored, esc-cancel, in-band error
  surfaced, legend hint, ModalOpen blocks tab keys).
- **`scripts/pull-delete-smoke.py` + `make smoke`** — pty-driven live harness: boots the
  binary at 110x36, pulls a real model through the UI, waits for completion, navigates to
  it, deletes it with confirm, verifies via `/api/tags`, quits on ctrl+c, always restores
  the host. Also used at 72x30 (compact dialogs render, no panic).

**Commands + exit codes**
- `go build ./...` `0` · `go vet ./...` `0` · `gofmt -l .` empty `0` · `make check` `0`
- `go test ./...` `0` — config cached, **ollama 15, ui 33** (+11/+17 this step)
- Live smoke `make smoke` — **SMOKE PASS in 106s**: pulled qwen3:0.6b through the TUI
  (~100s, rendered "pulling manifest"/"pulling 7f4030…", 102 %-frames, byte lines),
  "pulled qwen3:0.6b" notice, dialog exit, delete confirm "Delete qwen3:0.6b?" + y,
  model absent from `/api/tags` afterwards; host left with its original 10 models.

**Decisions / findings (respect these)**
- **Ollama registers a model in `/api/tags` at ~50% of the download**, long before the pull
  stream ends — do NOT use tag-presence as a pull-completion signal; use the UI's own
  completion (dialog exit / "pulled" notice) or the stream's `success` line.
- **Ollama pull errors arrive in-band as `{"error":…}` lines with HTTP 200** — check the
  stream, not just the status code (verified live).
- **bubbletea v2 redraws incrementally (per-line diffs)** — unchanged lines are not
  re-emitted, so "did the dialog close" cannot be inferred from the absence of its text in
  a recent capture window; watch for the constant-update lines falling quiet instead.
- Custom tags like `qwen3:0.6b:smoke123` are **invalid** (`:` is not allowed in a tag);
  smoke harness therefore re-pulls the plain model and cleans up via the API before/after.
- The 30s request timeout does NOT apply to pulls (multi-minute); `Client.stream` exists
  for that. Deletes stay on the 30s client.
- Oops + guardrail: my compact quick-check sent `y` on the selected (real) qwen3:8b and
  deleted it; restored via API pull (5.23GB, confirmed). Smoke harness always targets the
  model it pulled and API-cleans before/after — keep it that way.

**Blockers / next action**
- None blocking. README updated (M1b keys + `make smoke`). PLAN §10 M1b ticked; §12 next
  step = M2. Next fresh session: **M2 — chat + plain-chat path** (streaming chat in the
  Agent view, glamour markdown, input, graceful errors) → **Ship ALPHA**.

### 2026-09-03 — M2 — Chat + plain-chat path (DONE)
**Milestone:** M2 · **Result:** ✅ done — non-tool streaming chat path in Agent view works live with markdown rendering and explicit no-tool fallback.

**Work done**
- Added `internal/ollama/chat.go` with typed `/api/chat` request/response:
  - `Role` / `ChatMessage` / `ChatOptions` / `ChatRequest`
  - streaming `POST /api/chat` with NDJSON decode
  - per-event in-band `error` handling (HTTP 200 stream errors)
  - model/message/stream validation, context cancellation, and body cap via `maxBodyBytes`
- Added `internal/ollama/chat_test.go`:
  - payload fields
  - streaming delta aggregation
  - in-band and HTTP errors
  - empty input guards
  - bad JSON / missing done
  - context cancellation, token auth, options omission when zero
- Added `internal/ui/agent_view.go` (M2 Agent tab):
  - model loading/default selection (`/api/tags` + config default fallback)
  - markdown-rendered transcript with `glamour`
  - textarea-driven input with plain enter to send, shift+enter newline
  - `m` selector overlay (model switch), `r` refresh, `u`/`d` scrolling, `esc` stop
  - streaming async channel plumbing with resubscribed command (`waitChatCmd`)
  - stop-aware notice state (`stopped`) and graceful inline error/notice/hint lines
  - scroll math fixed to count rendered lines (not block entries)
- Wired Agent tab into `internal/ui/app.go`:
  - App owns both `models` and `agent`
  - dual init (`models.Init` + `agent.Init`)
  - key routing with modal guards for tab jumps
  - async chat events routed to Agent view
- Added `internal/ui/agent_view_test.go` for model loading, send/commit flow, live streaming,
  retry/error paths, selector, cancellation, scroll/resizes at multiple widths.

**Commands + exit codes**
- `go test ./...` `0`
- `go test ./internal/ollama ./internal/ui -count=1` `0`
- `go test ./internal/ui -run TestAgentViewScrollAndResize` `0`
- `go test ./internal/ui/` `0`
- `make check` `0` (`go build -o bin/selftui ./cmd/self-tui`; `go test ./...`; `go vet ./...`)

**Decisions / notes**
- Kept M2 strictly non-tool: plain chat prompt always sends to `/api/chat` and commits streamed
  assistant content without tool calls.
- `renderChatPane` scroll now uses flattened rendered lines and explicit follow offset, fixing early
  failures where scrolling and content visibility were wrong on multi-line markdown blocks.
- Stop handling marks `stopped` even if stream naturally finishes while cancellation was requested,
  matching current UX expectations.

**Blockers / open decisions**
- None.

**Next action**
- Fresh session: begin **M3a — Read-only agent tool loop** (state machine + `read_file` / `list_dir` / `grep` path).

### 2026-09-03 — M3a — Read-only agent tool loop (DONE)
**Milestone:** M3a · **Result:** ✅ done — read-only agent loop and explicit fallback path implemented and tested.

**Work done**
- Added `internal/agent/runner.go`: bounded 12-iteration state machine, ordered
  streaming `Msg`s (`TokenMsg`, `ToolStartMsg`, `ToolResultMsg`, `AgentDoneMsg`), native
  `message.tool_calls` dispatch, content-embedded JSON dispatch, streamed argument
  assembly, qwen3 thinking suppression, context cancellation, and HTTP-400 explicit
  plain-chat fallback.
- Added `internal/agent/tools.go`: only `read_file`, `list_dir`, and pure-Go `grep`;
  canonical workspace/symlink jail, regular-file checks, and bounded read/search/list
  results. No mutation or command tool is exposed.
- Extended `internal/ollama/chat.go` with typed tool definitions/messages and `ChatStream`
  event decoding while preserving the M2 `Chat` callback API.
- Wired `AgentView` to the runner and activity channel; tool activity is shown in the
  hint row, unsupported/no-tool models announce their plain-chat fallback, and the root
  App routes all agent events.
- Added focused filesystem, symlink, native/content dispatch, thinking/fallback,
  iteration-bound, cancellation, API transport, and UI tool-loop tests.
- Updated `README.md` and ticked M3a in `PLAN.md`; next milestone remains M3b, where
  mutation tools ship with jail/confirmation/timeout/cancel controls.

**Commands + exit codes**
- `go test ./... -count=1 -timeout=60s` `0`
- `go test -race ./... -count=1 -timeout=120s` `0`
- `make check` `0` (build + tests + vet + gofmt)
- `ollama list | head -8` `0` — local release model `qwen3:8b` present; no destructive
  live mutation smoke was run in this milestone.

**Decisions / findings**
- Capability probing uses the actual `/api/chat` tool request, matching the observed
  Ollama behavior: native tool support, content-embedded calls, and HTTP-400 rejection.
- The compatibility constructor used by legacy M2 tests omits the system prompt;
  production `NewAgentViewWithWorkspace` supplies the configured workspace and prompt.
- `grep` is pure Go to preserve the single-binary claim; `.git` directories and symlink
  entries are skipped during recursive search.

**Blockers / next action**
- None. Next fresh session: **M3b — mutation agent**, with write/edit and constrained
  `run_command` safety controls implemented inline and tested before exposure.

### 2026-09-05 — M3b — Mutation agent (DONE)
**Milestone:** M3b · **Result:** ✅ done — jailed mutations and a constrained confirmed command executor are implemented and tested.

**Work done**
- Added atomic, workspace/symlink-jail-checked `write_file` (explicit overwrite only) and
  exact-one-match `edit_file`; both now require an in-TUI per-call approval dialog.
- Added `run_command`: fixed argv only; approved `go` and read-only `git` subcommands;
  no shells/interpreters; scrubbed HOME/TMP/GOPATH/GOCACHE environment; 30s default / 60s
  maximum timeout; 256 KiB stdout and stderr caps; process-group TERM/KILL cancellation;
  serialized execution and streaming output events.
- Added the runner confirmation/output events, context-window budgeting with explicit
  truncation marker, and Agent UI confirmation modal (`y`/enter approve; `n`/esc decline).
- Added executor, filesystem jail, command validation/env/output/cancellation, context,
  runner, and UI refusal tests. Updated README, containment design, and PLAN §10/§12.

**Commands + exit codes**
- `go test ./internal/agent -run 'Test(WriteAndEdit|MutationTools|DeclinedMutation|RunCommand|Budget)' -count=1` `1` — expected RED before implementation (missing mutation APIs).
- `go test ./internal/agent -count=1` `0`
- `go test ./internal/ui -run 'TestAgentView(DeclinesMutation|ReadOnly)' -count=1` `0`
- `go test -race ./... -count=1 -timeout=120s` `0`
- `make check` `0` (build + tests + vet + gofmt)
- `git diff --check` `0`

**Decisions / findings**
- Owner chose the v1 allowlist: bounded `go` plus read-only `git`; `make`, `rg`, git
  mutations, general shells, and interpreters remain disabled.
- The executor is explicitly a guardrail, not an OS/container sandbox; confirmations,
  jail validation, env scrubbing, limits, and process-group kill reduce risk but cannot
  isolate malicious code run by an approved `go test`.

**Blockers / next action**
- None. M3b is committed; per the session rule, stop. Next fresh session: **M4 — Settings & persistence**.

### 2026-09-06 — M4 — Settings & persistence (DONE)
**Milestone:** M4 · **Result:** ✅ done — huh settings forms persist to the config file and live-apply in-session; esc discards (revert).

**Work done**
- **Config write-back (`internal/config`):** added `Save(Config)` (full-file TOML rewrite, dir 0700 / file 0600), kept load chain flags > env > file > defaults untouched; env+flag overrides now cover every §5 key (`SELFTUI_AGENT_*`, auth token, …). Round-trip + precedence tests extended.
- **CLI (`cmd/self-tui`):** flags for the full config surface (`-auth-token -default-model -workspace-root -temperature -top-p -num-ctx -max-tool-iterations -system-prompt`), parsed/validated before load.
- **Settings tab (`internal/ui/settings_view.go`, new):** embedded `charm.land/huh/v2` v2.0.3 (v2-aligned set per risk #7; never mix majors) with four sections matching PLAN §8: Connection (host + masked auth token), Model defaults (default model, temperature, top-p, num_ctx with range validation), Theme (dark/light select), Agent (system-prompt editor, workspace root, max tool iterations). Wide (≥120) = `LayoutColumns(2)` two-column sections; narrow = one group/page, scrollable. Esc (rebound form Quit) = discard; ctrl+c stays quit-app at the shell.
- **Form ownership model:** while the Settings form is open it is a modal — enter/tab advance, shift+tab back, esc discards; the shell tab bar + 1/2/3 jumps are inert until save/discard (entering Settings always rebuilds a fresh form from the current config). The root App forwards all leftover messages (huh internal next/prev field/group + re-fed commands) to the form while editing; a handled nav key is never re-typed into the form (ordering bug found + fixed via tests).
- **Live theme preview:** huh pushes the Theme selection into the bound accessor as the user arrows → `settingsThemeMsg` → App re-themes the shell + all child views immediately (list chrome/delegate/spinner accent, glamour renderer rebuild + cache re-render, form palette). Preview never touches `cfg.Theme`; discard rolls back to the start theme.
- **Submit = persist + live apply:** form completion snapshots the bound values and runs `config.Save` off-loop (`settingsSaveDoneMsg`); on success the App replaces the live config, re-themes when the theme changed, and — when host/token changed — rebuilds the `ollama.Client`, clears+reloads both model lists, and rebuilds the agent runner (new client/workspace/system prompt/iteration cap). Agent params/default model apply to the next send; an in-flight turn keeps its own runner (safe mid-stream swap). Write failure shows an error panel and applies nothing.
- **Bindings pattern:** form `Value(&v.field)` bindings live on one shared `*settingsValues` so the view and the form read the same data (enables preview diff + snapshot without form accessors).
- Tests: form seeding, key-ownership modal, esc discard, live theme message emission (march Dark→Light→Dark), submit→file write + saved panel, injected save error (no apply), live apply of host/theme/agent params without mutating the source config, config round-trip. Headless driver replays bubbletea cmd→msg chains with a grace window so cursor-blink `tea.Tick` cmds don't stall tests.

**Commands + exit codes**
- `go build ./...` `0`
- `go vet ./...` `0` · `gofmt -l cmd internal` clean
- `go test ./... -count=1 -timeout=180s` `0`
- `go test -race ./... -count=1 -timeout=180s` `0`
- `make check` `0` (build + tests + vet + gofmt)
- `git diff --check` `0`

**Decisions / lines to respect**
- Settings editing is intentionally modal: leaving the Settings tab requires save (last-field submit) or discard (esc). This follows huh conventions; hint text on the result panels says "enter edit again · tab switch tabs · ctrl+c quit". Flagged for owner: if mid-form tab-away is wanted later, it needs field-level prev/next rebinding.
- Save writes the *whole effective config* to the file (env/flag sources are re-loadable on next boot; nothing is preserved as "file-only").
- Theme preview is the only live-apply during editing; host/token/model/agent apply on submit ("live-apply where cheap").
- huh v2 is embedded as an internal `*huh.Form` (Init/Update/View called by the view; init cmds dropped — focus is synchronous, blink ticks not routed), which keeps the v2 `compat` adapter out of the app.

**Blockers / next action**
- None. M4 is committed; per the session rule, stop. Next fresh session: **M5 — Responsive completion + iPhone path** (golden render tests at two widths, SSH-on-iPhone guide, light theme polish, mobile approval ergonomics).

### 2026-09-06 — M5 — Responsive completion + iPhone path (DONE)
**Milestone:** M5 · **Result:** ✅ done — golden render tests at the two canonical geometries, height-capped approval dialogs, light-theme verification, and the README SSH-on-iPhone guide.

**Work done**
- **Golden render harness (`internal/ui/golden_test.go`, new):** renders the full shell (tab bar + body + status bar) in seven deterministic scenarios at the two canonical geometries — the measured Moshi portrait device **72×30** (`docs/m0a-gate-evidence.md`) and a wide PC window **120×40** — and compares each against a checked-in ANSI-stripped fixture under `testdata/golden/` (`go test ./internal/ui -run TestGoldenRender -update` regenerates). Scenarios: models list (compact), models stacked inspect (compact), models side-by-side inspect (wide), agent (compact + wide), settings editing single-column (compact) and two-column (wide).
- **Frame-fit guards (`TestGoldenFramesFitTerminal`):** no rendered row wider than the terminal, no view taller than the screen, frame-filling views land on exactly `h` rows (models/agent/settings-compact), every scenario non-empty.
- **Mobile approval ergonomics:** every overlay body (agent confirm/selector via `renderOverlayTitle`, models delete/pull/input via `renderOverlay`) is now capped at `bodyH-4` rows through a shared `fitContent` helper that keeps the head and the final action legend and replaces the dropped middle with a “… (N more lines)” marker. Verified by `TestApprovalOverlayFitsDevice`, which drives a real `write_file` approval with a >3 KiB payload at 72×30 and 120×40 — before the fix that dialog rendered 40 rows in a 30-row terminal and hid the `y / enter approve` row.
- **Settings 1-row overflow fix:** the huh form’s height budget now reserves its footer row (`h-3`, was `h-2`), which rendered the compact Settings form one row too tall at 72×30 (measured with a probe; frame now lands on exactly 30 rows). Same budget in `buildForm` and `resize`.
- **Light theme:** verified as a genuinely distinct palette (`TestThemePalettesDiffer`: fg/bg/muted/accent/error all differ), light renders of every populated tab at both geometries inside the frame (`TestLightThemeRendersEveryTab`), and one markdown chat round-trip through the light glamour renderer at 72×30 (`TestLightThemeAgentChatRenders`). Light active-tab chip now uses light text on the violet accent (the body-black foreground vanished on it).
- **README:** M5 status line plus “Using SelfTUI from an iPhone (SSH)” — host setup (SSH server, local Ollama default; remote host/token flags for a remote Ollama), client setup (Blink/Termius/Moshi), measured geometry, per-tab compact behavior, esc/key/theme tips, live width/breakpoint readout.
- **Makefile:** `make test` now runs `go test -count=1` so the golden fixture compare can never be masked by a stale test cache.
- Ticked M5 in `PLAN.md` §10 and rolled §12 to **M6 — Release acceptance**.

**Commands + exit codes**
- probe (temporary, removed) measured: settings compact overflow 31/30 rows → fixed to 30; agent/models frames exact at both geometries `0`
- `go test ./... -count=1 -timeout=180s` `0`
- `go test -race ./... -count=1 -timeout=240s` `0`
- `go test ./internal/ui -run TestGoldenRender -count=3` `0` (fixtures deterministic)
- `go test ./internal/ui -run TestGoldenRender -update` `0` (fixture generation; second run compares clean)
- `make check` `0` (build + uncached tests + vet + gofmt)
- `gofmt -l cmd internal` clean · `git diff --check` `0`

**Decisions / lines to respect**
- Golden fixtures store ANSI-stripped text: layout/geometry/content drift fails the compare; palette drift is covered by the dedicated theme tests (a color-only change never churns fixtures). Regeneration is `go test ./internal/ui -run TestGoldenRender -update`.
- `fitContent` semantics: keep the head and the last two wrapped lines (the decision legend is always last, so approve/cancel keys stay on screen), replace the dropped middle with one marker row. Overlays never exceed the body height.
- The huh form height budget is `h-3` (not `h-2`) because huh draws a footer row below the field area when a group overflows (measured at 72×30).
- Settings-wide renders shorter than the screen by design (two-column content is ~22 rows at 40); the frame-fit guard allows it, all other scenarios must fill exactly `h` rows.
- No palette/ANSI changes landed that affect dark fixtures; fixtures are environment-stable because compares run in the same env that generated them.

**Blockers / next action**
- None. M5 is committed; per the session rule, stop. Next fresh session: **M6 — Release acceptance** (unit+golden tests throughout; auth/TLS; error surfacing; context-truncation edges; binary/reconnect smoke test; docs; release acceptance).

### 2026-09-06 — Owner decision for M6 (recorded between sessions)
- **M6's reconnect smoke runs LIVE over the owner's actual Moshi/iPhone 16 Pro client**
  (the measured 72×30 device), not only the local pty harness. Evidence recorded in
  `docs/` following the M0a pattern (`cmd/size-probe` + probe record at
  `$XDG_STATE_HOME/selftui/probe.txt`).
- Open point for the M6 session to resolve first: what "reconnect" means on the
  transport (mosh reattach vs SSH re-connect vs fresh client after a drop) — then
  verify geometry, scroll state, and in-flight cancellation recovery after reconnect.
- Recorded in `PLAN.md` §10 under M6 so the roadmap carries it.
### 2026-09-03 — M6: Release acceptance (DONE)
**Milestone:** M6 · **Result:** ✅ done — reconnect semantics resolved LIVE on the
owner's Moshi/iPhone 16 Pro client (tmux-over-SSH reattach is the real
transport), `make smoke-reconnect` harness + size-probe instrumentation,
auth/TLS + context-truncation + error-surfacing acceptance work, one real UI
bug found by the smoke and fixed, release docs. `make check` and
`go test -race ./...` green.

**Work done**
- **Reconnect semantics (owner decision, resolved live):** asked → owner runs
  SelfTUI **inside tmux over SSH**; the live drop test showed the app process
  **survives** (pid 881865 alive across the drop/reconnect, app log shows no
  shutdown clean) with the same screen restored (owner observation) and host
  Ollama healthy (tags HTTP 200 in 1.4 ms). Canonical live shape = **tmux
  reattach**; the **fresh-SSH death** shape (SIGHUP → process dies, config
  persists, chat is per-process) is covered by the local harness.
  `docs/reconnect.md` records the resolution, instrument spec, both procedures,
  and the evidence (local + live); README ships a user-facing "if your SSH
  session drops" note.
- **`cmd/size-probe` extension:** session headers `# size-probe start
  session=<tag> pid=<pid> term=…` (new `-session` flag, default `pid-<pid>`),
  checkpoints (`c` key in tui / SIGUSR1 in raw → `checkpoint` event line).
  probe.txt is now attributable session blocks (verified at 72×30 in a pty).
- **`scripts/reconnect-smoke.py` + `make smoke-reconnect`:** boots selftui in a
  pty at 72×30 (child = own session with the pty as controlling terminal, so
  closing the master delivers SIGHUP like sshd), starts a chat turn, drops
  mid-generation, asserts SIGHUP death (rc=-1) + host recovery (fast tags +
  complete short generation) + clean fresh reconnect at 72×30 with the config
  file re-applied (light theme) + ctrl+c exit 0. **PASS ×4 runs** (~5–13 s).
- **M6 acceptance test additions:** bearer token on the Pull stream endpoint;
  real-TLS pair — trusted-cert https handshake carrying the token on a JSON +
  a stream endpoint, and untrusted-cert rejection with a surfaced certificate
  error; settings save-failure surfaced end-to-end (chmod 0500 dir → error
  panel names the cause → fix + retry succeeds); context-truncation edges in
  new `internal/agent/context_test.go`.
- **Context-truncation fixes (found by the new tests):** (a) a giant *first*
  message was sent raw past num_ctx — `BudgetMessages` early-returned on
  `len<3`; now only degenerate/empty lists skip budgeting and `truncateLatest`
  bounds a lone overflowing turn (marker-free, `[truncated]`-prefixed tail);
  (b) the plain-chat fallback (`runPlainChat`) bypassed the budget — now calls
  `BudgetMessages`; (c) repeated budgeting inserts the truncation marker
  exactly once (idempotence pinned by test); (d) tool-call argument JSON is
  counted toward the budget (tested). Dead `compactToolResult` helper removed
  (per-stream caps already bound results).
- **Real UI bug found by the smoke:** digits typed in the Agent chat input
  switched tabs mid-prompt ("1 to 300" → 1 and 3 jump). Fixed in `app.go`:
  digit tab-jumps are disabled while the Agent input is **composing** (empty
  input still jumps; new `AgentView.composing()` helper); regression test
  `TestAgentTabDigitsTypeNotJump` covers composing + empty-input + enter;
  `TestAgentViewModalBlocksTabJump` still passes (empty input → digits jump
  after esc).
- **Release identity:** `selftui -version` → `selftui 0.6.0-m6`; version logged
  at startup (app log evidence now attributable).
- **PLAN/README:** §10 M6 ticked with the resolved semantics + evidence; §12
  rolled to "v0.1 release"; README status → M6, nav note (digits = text while
  composing), iPhone drop guidance, `-version`.

**Commands + exit codes**
- `make check` `0` (build + uncached tests + vet + gofmt)
- `go test -race ./... -count=1 -timeout=240s` `0`
- `make smoke-reconnect` `0` ×4 (PASS in 5–13 s; SIGHUP rc=-1; host recovery
  round-trip 0.1–2.2 s)
- `make probe-local` `0` (5/5 PASS incl. 88×44→100×50 mid-run resize)
- `gofmt -l cmd internal` clean · `git diff --check` (below)
- live evidence: probe.txt session blocks `m6-live-1a` at 72×30; app log
  shows 0.6.0-m6 sessions; `ps` tree selftui → bash → tmux

**Decisions / lines to respect**
- Reconnect canonical for THIS owner = **tmux-over-SSH reattach** (process
  survives; scroll/chat state preserved). Fresh-SSH death is the documented
  no-tmux shape, covered by the local harness — not a separate live gate.
- Chat/scroll state is per-process by design; session resume is out of v1.
- `BudgetMessages` may now truncate a lone overflowing turn (tail kept,
  `[truncated]` prefix) — never sends past the reserved 3/4·num_ctx budget.
- Digit tab-jumps yield to chat composition; README nav updated.

**Blockers / next action**
- Residual (not blocking): no dedicated post-reconnect `-session m6-live-1b`
  probe block was captured on the device this pass; one command fills it on a
  future device run (documented in `docs/reconnect.md`). Per the session rule,
  stop here. Next fresh session: **v0.1 release** (tag the M6 build, release
  notes) or any owner-assigned follow-up.
### 2026-09-03 — Owner decision for M7 (recorded between sessions)
- **Pre-v0.1 UX polish step added (M7)**, scoped by the owner with opencode.ai's
  TUI as the reference for feel/smoothness (researched: leader keys, command
  palette ctrl+p, slash commands, filter-as-you-type pickers, per-turn
  status/stop reasons, context visibility).
- Selected packages: **A — composer + slash commands + ctrl+p palette**,
  **B — transcript feel** (streaming caret, model chips, elapsed + stop
  reason, paging/auto-follow), **C — context meter + filterable model
  picker**. Package **D** (help/onboarding overlay) explicitly dropped for v0.1.
- Per the binding one-step-per-session rule this starts in a **fresh session**
  (cwd /home/calvin/SelfTUI); scope + constraints recorded in `PLAN.md` §10
  (M7) and §12. Order within M7 is up to the executing session; each package
  is one scoped build step with golden-test acceptance at 72×30/120×40.
- Open for the M7 session to resolve: exact keybind set for palette/slash
  (phone keyboards: no ctrl+p hardware? Blink maps ctrl; verify) and whether
  the context meter lives in the hint row or the status bar.

### 2026-09-06 — M7: UX polish, pre-v0.1 — composer/palette, transcript feel, context meter + picker (DONE)
**Milestone:** M7 (owner-scoped packages A + B + C, opencode.ai TUI as the feel
reference) · **Result:** done — `make check` and `go test -race` green; 10 new
golden frames (17 total) at 72×30/120×40; README/PLAN updated; v0.1 is next.

**Work done**
- **A — Composer + commands.**
  - Slash-command menu over the Agent input: typing "/" shows the command
    list and the draft filters it live; arrows steer the highlight, enter
    runs, esc drops the whole draft. Commands: `/clear` (y/n confirm dialog;
    notice on empty), `/model` (opens the picker), `/theme` (sends
    `agentThemeMsg` to the root — session-scoped shell-wide toggle),
    `/help` (compact reference overlay; full onboarding stays out of v0.1),
    `/refresh`. An unmatched slash draft ("/this file" chat) is ordinary
    prose — no menu, sends normally. Input placeholder now reads
    "/ for commands, or chat…". Idle `esc` clears a drafted prompt.
  - `ctrl+p` command palette on any tab (palette.go): go to Models/Agent/
    Settings, change model, clear conversation, toggle theme, refresh models,
    open the command list — all filtered as you type; runs synchronously
    against the App value (no async plumbing). Guarded against opening over
    another modal or mid-stream; owns every key while open. Theme actions
    (palette + /theme) show a status-bar toast cleared on the next keypress.
  - `applyTheme` now tracks `curTheme` (session theme) so session toggles
    never stick on the stale `cfg.Theme`; digits-are-text-while-composing and
    empty-input letter-command rules verified intact by the M6 regression
    tests.
- **B — Transcript feel.** Streaming caret "▍" rides the live block and
  disappears at rest; each finished assistant turn gains a muted footer with
  elapsed time + terminal reason — `AgentDoneMsg` now carries the final
  stream's `done_reason` (stop/length/tool_calls) out of the runner;
  `turnMeta` parallels history (user placeholder) so geometry re-renders keep
  footers; pgup/pgdn page the transcript, `f` toggles auto-follow, and d/pgdn
  back to the tail re-engages it.
- **C — Context meter + picker.** Meter in the Agent hint row (composing/
  streaming always, idle once the conversation has ≥1% weight): same 4-chars-
  per-token estimator as the runner via the new `agent.ApproxTokens`, over the
  exact next payload (system prompt + history + in-flight/draft). Red at 100%,
  and once a send exceeds the budget the transcript head shows the omission
  marker (`agent.TruncationNotice`, exported from the same const the runner
  inserts) until /clear. Model picker filters as you type across
  name/family/size/quant with j/k+arrows navigation, stars the configured
  default model, and shows a no-match row.
- **Overlay safety:** `renderCenteredOverlay` (shared by Agent modals + the
  palette) replaces the duplicate overlay titles; `wrapLines` is now
  ANSI-width aware (styled rows no longer byte-split mid-sequence); hint rows
  width-fit by dropping lowest-priority legend segments, meter last. New
  golden frames: agent-picker/slash/help/clear-confirm + palette at both
  72×30 and 120×40 — every frame passes the fit-terminal guard.
- README: status line, Agent-tab keys, command palette + slash commands
  section. PLAN.md §10 M7 ticked + §12 next-step rewritten (v0.1 next).

**Commands + exit codes**
- `go build ./...` `0` · `go vet ./...` `0` · `make check` `0`
- `go test -race ./...` `0` (agent/config/ollama/ui)
- `go test ./internal/ui -run TestGoldenRender -update` `0` (fixtures
  regenerated: 17 frames under testdata/golden/)
- `gofmt -l .` empty (clean)

**Decisions / lines to respect**
- **Palette/slash keybind set (M7 open question):** ctrl+p from any tab +
  the "/" menu in the Agent input as the phone path (Blink maps ctrl for
  ctrl+p; soft keyboards get the slash menu). Slash menu navigates with
  arrows only (typed letters filter; j/k type into the draft). Palette and
  picker filters reserve j/k for navigation.
- **Context meter placement (M7 open question):** the Agent hint row, not the
  shared status bar (status shows host/geometry shell chrome; the meter is
  conversation-local). Shown while composing/streaming and, idle, once ≥1%
  used; truncation shows "ctx full — /clear" + the transcript marker.
- `/theme` and palette theme toggles are **session-scoped** (toast says
  "save in Settings to keep it"); persistence stays a Settings action.
- `esc` semantics: stops a stream, clears a drafted prompt when idle, closes
  overlays/menus — never quits.
- The user header stays plain "❯ you" (chip lives on assistant blocks).

**Blockers / open decisions (carry to next session)**
- None. v0.1 release (tag `v0.1.0`, release notes) is the next step.

**Next action**
- Fresh session: tag v0.1.0 + release notes (PLAN §10 says "tag + release
  notes"; decide version string behavior — `-version` prints 0.6.0-m6 today).

### 2026-09-06 — M7 follow-up: opencode-style chat box (owner task)
**Milestone:** owner task on top of M7 · **Result:** done — composer block +
statusline, auto-growing prompt, numeric ctx usage, armed interrupt,
header-right turn meta; 19 golden frames; `make check` + `go test -race` green.

**Work done** (scope decided with the owner: "Whole Agent tab"; layout =
composer block + statusline; behaviors = auto-grow + numeric usage + armed
interrupt; model chip as a display chip, picker trigger stays `m`)
- Grounded the reference by reading opencode's own footer source
  (`footer.view.tsx`/`footer.prompt.tsx`): composer on top, a one-row
  statusline under it — mode/status · interrupt · usage · model.
- **Composer block** replaces the old hint row + input box: one bordered
  pane whose header row shows the model chip left and the live ctx meter +
  numeric token usage right (`ctx ▓▓░░ 38% · 1.2k/3.1k`; red "ctx full —
  /clear" at 100%). The prompt textarea auto-grows 1→4 rows as multi-line
  text is drafted (`composerRowsFor` counts wrapped rows at the pane width;
  capped, re-fit on every keystroke/resize/reset). Idle-empty still shows
  the full usage row (opencode keeps activity visible).
- **Statusline** under the composer: error (red, "enter to retry"), running
  state (`running…`/tool status + `esc interrupt`), transient notice, else
  the width-fitted key legend (composing vs idle). The slash menu keeps
  floating between transcript and composer; statusline row counts are
  dynamic so the chat pane always absorbs the composer's growth.
- **Armed interrupt**: while running the first `esc` only arms (statusline
  flips to `esc again to interrupt` in red); the second cancels — a stray
  esc can no longer kill a long run. `stopArmed` resets on start/done.
  Existing stop tests updated to the two-press contract.
- **Transcript**: per-turn meta (elapsed + reason) moved from a separate
  muted footer row onto the assistant header line, right-aligned to the pane
  margin (`assistantHeaderRow`); saves a row per turn and reads like
  opencode's part headers. `turnMeta` stays parallel for geometry re-renders.
- New golden frames: `agent-turn-*` (committed turn with meta header) — 19
  total. `/help` + README updated for the composer/statusline/armed-esc.

**Commands + exit codes**
- `go build ./...` `0` · `go vet ./...` `0` · `gofmt -l .` empty
- `make check` `0` · `go test -race ./...` `0`
- `go test ./internal/ui -run TestGoldenRender -update` `0` (19 frames)
- one expected mid-work hang traced to the old single-esc stop tests under
  the new armed semantics (tests updated, not the code)

**Decisions / lines to respect**
- Follow-up overrides two M7 details: turn meta now lives on the assistant
  header's right side (not a footer row), and `esc` while running is an
  armed interrupt (first press warns, second cancels). Idle esc still clears
  a drafted prompt; esc never quits.
- The ctx meter + usage live in the composer header (conversation-local);
  the app status bar below remains host/geometry shell chrome.
- Composer auto-grow caps at 4 rows; wrapping math treats the first line as
  shortened by the "❯ " prompt.

**Blockers / open decisions**
- None. v0.1 release (tag + release notes; `-version` still prints 0.6.0-m6)
  is next.

**Next action**
- Fresh session: tag v0.1.0 + release notes.

### 2026-09-06 — Chat-session persistence (owner task: "do that" on recoverable chats)
**Milestone:** owner task · **Result:** done — committed turns now mirror to a
per-process markdown transcript; `make check` + `go test -race` green.

**Work done**
- `internal/session`: append-only transcript writer. `Open(dir, host)` makes
  the sessions dir (0700) + `chat-<ts>-<pid>.md` (0600, append mode) with a
  small header (# SelfTUI chat session / started / host); `Append(role,
  model, content, meta, at)` writes readable `## user/assistant (model) ·
  HH:MM:SS · meta` blocks preserving content verbatim; Flush/Close. Unit
  tests: perms, header, block format incl multi-line content, fresh file per
  run (sub-second + pid name), nil-safety, unknown-role error.
- UI wiring: AgentView holds `sessionDir/sessionHost/session` and lazily
  opens the log on the first committed turn (`appendSessionTurn` on the user
  append in sendInput and on the assistant commit in onChatDone, with the
  elapsed·reason meta). A write failure disables the log once and shows one
  notice — chat never blocks or breaks for the transcript. `WithSessionDir`
  on AgentView and App; main resolves the dir (default
  `$XDG_STATE_HOME/selftui/sessions` via xdg.StateHome, `SELFTUI_SESSION_DIR`
  override, `SELFTUI_NO_SESSION=1` disable) and logs it at startup.
- `/save` slash command: flush + notice with the transcript path (also
  explains the off state and the nothing-recorded-yet state). Slash menu is
  now six commands — menu cap raised 5→6 so the whole set fits; help overlay
  + README updated (location, env knobs, `/save`).

**Commands + exit codes**
- `go build ./...` `0` · `go vet ./...` `0` · `gofmt -l .` empty
- `go test ./... -count=1` `0` (agent/config/ollama/session/ui/cmd)
- `go test -race ./...` `0` · `make check` `0`
- `go test ./internal/ui -run TestGoldenRender -update` `0` (help/slash
  fixtures regenerated for /save + 6-row menu; 19 frames total)

**Decisions / lines to respect**
- Recording is ON by default (owner asked for recoverable chats); env knobs:
  `SELFTUI_SESSION_DIR` to relocate, `SELFTUI_NO_SESSION=1` to disable.
- One file per process run (chat is per-process); /clear within a run does
  not start a new file. Files are plain markdown (0600) — inspectable by the
  owner or by tooling, e.g. `rg '^## ' ~/.local/state/selftui/sessions/`.
- Transcript writes happen on the update loop but are tiny and synchronous;
  errors degrade to one notice, never an error state.

**Blockers / open decisions**
- None. v0.1 release (tag + release notes) still next.

**Next action**
- Fresh session: tag v0.1.0 + release notes.

### 2026-09-06 — Polish fixes found live in the owner's session (follow-up)
**Milestone:** owner task (their pasted 82x34 session) · **Result:** done —
truncation now shows "…", no phantom caret during tool/thinking phases.

**Work done**
- `truncateToWidth` silently cut text without an ellipsis (lipgloss MaxWidth
  truncates and fits, so the fallback never ran). Rewritten: rune-wise trim
  to maxW-1 columns + "…", ANSI sequences copied whole and never split, wide
  runes counted by lipgloss.Width. New unit tests (ellipsis, pass-through,
  styled ANSI survival, wide runes, maxW=1).
- Live assistant block in chatLines was gated on `streaming || text`; during
  a tool run or qwen3 thinking it streamed no text yet still drew an empty
  header + caret (seen as "◈ qwen3:8b▍" in the paste). Now the block renders
  only when streamText is non-empty; the tool/thinking state lives on the
  statusline. New test: no caret before text, caret rides text, gone at rest.

**Commands + exit codes**
- `go test ./... -count=1` `0` · `go test -race ./...` `0` · `make build` `0`
- goldens unchanged (no regeneration needed)

**Decisions**
- Cut text always shows "…"; the transcript never invents an assistant block
  that has no content.

**Next action**
- Fresh session: v0.1 tag + release notes (unchanged).

### 2026-09-06 — Root cause: mutation dialogs dropped at the App shell (owner bug)
**Milestone:** owner bug fix · **Result:** done — write_file/edit_file/run_command
approvals now reach the UI in the real binary.

**Symptom (owner-reported):** asking the agent to write a script hangs forever —
statusline stuck at "⚙ write_file … esc again to interrupt", no "Allow
write_file?" dialog ever appears, no file is created. Reproduced live: runner
and AgentView flows both pass against qwen3:8b, so the failure was upstream.

**Root cause:** App.Update's forwarded-case list for agent messages omitted
`agent.ToolConfirmMsg` and `agent.ToolOutputMsg`. Every mutation test drove
AgentView.Update directly (or pumped its chatCh into the view, bypassing the
shell), so the gap never surfaced: in the real app the runner emits
ToolConfirmMsg, the root App drops it, the runner blocks forever on the
approval channel, and the UI shows a perpetually "running/armed" tool line
with no way to approve.

**Fix**
- Added `agent.ToolConfirmMsg` and `agent.ToolOutputMsg` to App.Update's
  agent case list (app.go).
- New App-level regression tests (routing_regression_test.go) that drive the
  whole flow through App.Update: write_file confirm appears as the overlay,
  y approves → file written + final reply; n declines → no file. These tests
  would hang/fail on the old routing.
- Note for future tests: fake NDJSON tool events must nest the function
  object exactly like the agent package's toolEvent helper (fmt.Sprintf with
  a separate `{"function":…}` arg), or Ollama decode fails and the run errors
  before any tool call.

**Commands + exit codes**
- `make check` `0` · `go test -race ./...` `0` · `make build` `0`
- live repros (temporary, removed): runner + UI two-turn timer against the
  real qwen3:8b — both PASS (25.3s / 10.9s), confirming tool writes + edits

**Next action**
- Fresh session: v0.1 tag + release notes (unchanged).

### 2026-09-06 — Remote repo created (owner task)
**Milestone:** owner task · **Result:** done — private GitHub remote, main pushed.

**Work done**
- Created **https://github.com/MerverliPy/SelfTUI** (PRIVATE, account MerverliPy —
  no orgs; verified via `gh auth status`). Branch: `main` at `b6bd9a0`.
- `origin` = https fetch/push, tracking set, HEAD pushed.
- Hygiene checked before push: 78 tracked files; no config.toml/*.log/bin/
  sessions/probe.txt tracked (all ignored); no secrets.
- Owner decided: private, and keep the internal docs (PLAN/LEDGER/COUNCIL-MEMO/
  AGENTS) in the remote as-is.
- Note: LEDGER is a personal session journal — it is now in the private
  remote; flip visibility (`gh repo edit --visibility public`) only after
  deciding whether to keep it there.

**Commands + exit codes**
- `gh repo create SelfTUI --private --source . --remote origin --push` `0`
- verified: `git ls-remote --heads origin` shows main at b6bd9a0

**Next action**
- Fresh session: v0.1 tag + release notes (unchanged).

### 2026-09-03 — v0.1 hardening, phase 3: validate + atomically save config (owner task)
**Milestone:** owner-assigned step on `hardening/v0.1` (phase 3 of the v0.1
hardening plan; branch tip was `6903003`) · **Result:** done — see commit
"security: validate and atomically save configuration". No §10 milestone row to
tick (hardening phases are owner-assigned steps, not PLAN.md §10 milestones).

**Work done**
- `config.Validate(Config) error` (new `internal/config/validate.go`) is now the
  single configuration policy, with stable field-prefixed errors
  (`config: host: …`, `config: agent: …`, `config: theme: …`,
  `config: workspace_root: …`). Rules: host scheme exactly http/https, hostname
  present, userinfo/query/fragment rejected, bearer token over plain http only
  for localhost/127.0.0.1/::1; theme dark|light; temperature 0–2; top_p 0–1;
  num_ctx 128–1,048,576; max_tool_iterations 1–100; non-empty workspace_root
  must be an existing directory. First violation wins, in a fixed order.
- `config.Load` calls `Validate` after all sources are applied (defaults → file
  → env → overrides) and returns the error verbatim; `config.Save` validates
  before marshaling. Every surface reports the identical message regardless of
  the offending source.
- `config.Save` now writes atomically via `writeFileAtomic`: same-directory
  0600 temp file (`.selftui-config-*.tmp`) → write → Sync → Close → Chmod(0600)
  → rename over target → Chmod(0600) on the final path; the temp file is
  removed on every failure (deferred cleanup); created config directories are
  0700 (pre-existing dirs untouched).
- Settings form no longer keeps its own validation policy — it delegates every
  field check to `config.Validate` (`policyValidator` in settings_view.go),
  which also fixes a real drift: the form allowed max tool iterations up to 256
  while the policy is 1–100. Fields keep only parse-shape checks (number /
  integer) plus token and workspace-root checks that previously had none.
- Tests (all table-driven where the task asked): every invalid rule is rejected
  with the exact same stable error through each of TOML / env / Overrides
  (15 rules × 3 sources + boundary/loopback acceptance matrix + host-level unit
  cases + NaN); an existing 0644 config becomes 0600 after Save; a failed
  validation leaves the old file byte-identical with no temp litter; a
  mid-write failure (read-only dir) keeps the old file; a rename failure
  cleans the temp file; a successful save reloads identically (every field).
  UI test drives the form to max tool iterations, types "0" onto the seeded
  "12" ("120": inside the old 1..256 band, outside the new 1..100) and asserts
  config.Validate's stable message inline, with nothing written.
- `-auth-token` removed from README examples; README + flag help now recommend
  `SELFTUI_AUTH_TOKEN` or the 0600 config file and warn the flag (kept only for
  compatibility) can leak argv secrets into process listings / shell history.
- Existing priority tests updated to policy-valid sample values (env/flag/file
  hosts now carry a scheme; token fixtures sit on https or loopback hosts;
  TestSaveWritesConfig uses a real workspace dir).

**Commands + exit codes**
- RED: `go test -count=1 ./internal/config` → build fail `undefined: Validate`;
  `go test ./internal/ui -run TestSettingsFieldValidationReusesConfigPolicy`
  → FAIL (old policy accepted "120", saved). Green after implementation.
- `go test -count=1 ./internal/config ./internal/ui ./cmd/self-tui` `0`
- `go test -race -count=1 ./internal/config ./internal/ui` `0`
- `make check` `0` (build + `go test -count=1 ./...` + vet + gofmt) · `make fmt` `0`
- live boot checks: `SELFTUI_THEME=pink|SELFTUI_HOST=ftp://…|SELFTUI_AUTH_TOKEN`
  over http remote → `selftui: load config: config: …` exit 1; `-version` `0`.

**Decisions / lines to respect**
- Validate errors flow **verbatim** out of Load and Save (no extra wrapping) so
  any caller sees the exact stable message; main prints
  `selftui: load config: config: host: …`.
- Loopback = `localhost`, `127.0.0.1`, `::1` (case-insensitive hostname).
  Empty `workspace_root` stays allowed (= cwd); non-empty must exist as a dir.
- Created config parent dirs are 0700; a pre-existing parent is left alone
  (never chmod a user's `-config` directory out from under them).
- The Settings form shows config.Validate errors only when they belong to the
  field being edited (matched by the stable marker substring); Save
  re-validates as the backstop, surfacing any miss in the existing error panel.

**Blockers / open decisions**
- None. v0.1 (tag + release notes) still next; further hardening phases per
  owner as assigned.

**Next action**
- Fresh session: next owner-assigned step (v0.1 tag/release notes or the next
  hardening phase).

### 2026-09-03 — v0.1 hardening, phase 4: require explicit workspace tool trust (owner task)
**Milestone:** owner-assigned step on `hardening/v0.1` (phase 4 of the v0.1
hardening plan; branch tip was `a98f1d8`) · **Result:** done — see commit
"security: require explicit workspace tool trust". No §10 milestone row to
tick (hardening phases are owner-assigned steps, not PLAN.md §10 milestones).

**Work done**
- `config.ToolsEnabled bool`, TOML `tools_enabled`, env `SELFTUI_TOOLS_ENABLED`
  via `strconv.ParseBool` (garbage → `parse SELFTUI_TOOLS_ENABLED="…"` load
  error; env beats file, so `false` can disable a file-enabled tools), default
  `false`, persisted by `Save`. `config.Validate` now rejects
  `tools_enabled=true` when `workspace_root` is empty (`config: tools_enabled:
  workspace_root is required when tools are enabled`), `/` (`…must not be /…`)
  or exactly the current user's home directory (`…must not be your home
  directory…`, compared after `filepath.Clean` so a trailing slash cannot
  dodge it); tools-off keeps those roots legal. Settings → Agent gained an
  *Enable workspace tools* confirm toggle (11th field) whose validator is the
  config policy (stable `config: tools_enabled:` marker), so enabling against
  an unsafe root fails inline with the same message `Save`/`Load` produce.
- `agent.ToolPolicy` (new `toolpolicy.go`): `Tools() []ollama.ToolDefinition`
  returns the five Phase-1 v0.1 tools (read_file/list_dir/grep +
  confirmed write_file/edit_file; closed — no run_command); `AuthorizePath`
  rejects any requested path containing a component `.ssh`/`.gnupg`/`.aws`/
  `.azure`/`.kube` or the `.config/gcloud` composite, and basenames `.env`/
  `.env.*` except exactly `.env.example`, plus `credentials`/
  `credentials.json`. Lexical and additive to the existing canonical
  containment (`securePath`/`canonicalRoot`); `.`/empty-component paths pass
  to containment (a model legitimately asks `list_dir "."`).
- `Runner.policy *ToolPolicy`; `NewRunner` stays the compatibility wrapper
  with tools DISABLED (nil policy → plain chat, no tools field on the wire —
  proven by a test that inspects the raw JSON body, and by a test where a
  hostile endpoint replies with a tool_calls event: nothing executes, no
  confirmation, no file). Production wiring is `NewRunnerWithPolicy`;
  `runPlainChat` now records the terminal done_reason so the tools-off footer
  keeps its `· stop/· length` meta. Each of the five `executeTool` cases calls
  `authorizePath(args.Path)` before its own validation/confirm/executor, so a
  sensitive request never even surfaces an approval dialog.
- UI: the Agent statusline (idle legend) and the root status bar both show the
  canonical workspace (real path, symlinks resolved — `canonicalWorkspaceLabel`,
  empty root → cwd) and `tools off`/`tools on`; the bar drops the workspace
  (then the warning) under width pressure, host+tools survive. When tools are
  enabled against a non-loopback host (`config.LoopbackHost`, exported and
  reused by validateHost) the Agent statusline shows a persistent red
  `⚠ tools on — workspace content may be sent to <host>` row, and the status
  bar appends `⚠ workspace content may be sent to the remote host` when it
  fits. Agent view/compat constructors default to tools off; the App passes
  `cfg.ToolsEnabled`/`cfg.Host` through `newAgentView` and `ApplyConfig`
  rebuilds the runner on a settings save. No onboarding wizard built (v0.1
  stays out of that scope).
- Golden fixtures regenerated (19 frames; only the status rows changed —
  `· tools off · /home/calvin/SelfTUI/internal/ui` in the agent frames/bar).
  Note: fixtures embed the test cwd as the canonical workspace (empty
  workspace_root → cwd is the real default behavior), so they are
  machine-path dependent like the existing behavior always was.
- README (env/file config rows, agent tools = opt-in, Settings toggle,
  sensitive-path refusal) and PLAN §5/§6 (tools_enabled row, policy, safety
  rules, tool-table policy notes) updated.

**Commands + exit codes**
- RED: `go test -count=1 ./internal/config -run 'TestTools|…'` → build fail
  `undefined: ToolsEnabled`; `go test ./internal/agent -run 'TestToolPolicy|…'`
  → `undefined: ToolPolicy/NewRunnerWithPolicy`; `go vet ./internal/ui` →
  `too many arguments in call to newAgentView` (all intended).
- Green: `go test -count=1 ./internal/config ./internal/agent ./internal/ui` `0`
- `go test -race -count=1 ./internal/config ./internal/agent ./internal/ui` `0`
- `make check` `0` (build + `go test -count=1 ./...` + vet + gofmt); full
  `go test -race -count=1 ./...` `0`; goldens regenerated via
  `go test ./internal/ui -run TestGoldenRender -update` (diff reviewed: only
  the status rows).

**Decisions / lines to respect**
- ToolPolicy zero value = fully armed; "enabled" is expressed by which
  constructor arms it (nil policy vs `&ToolPolicy{}`), mirroring
  NewRunner = disabled default.
- AuthorizePath is lexical on the requested path, deliberately in addition to
  containment; an in-workspace symlink alias to a forbidden file is out of
  scope (the agent cannot create symlinks, so only a pre-existing user-made
  alias could matter).
- `.env.*`-prefixed basenames are refused except exactly `.env.example`
  (future `.env` variants are credential files too).
- The v0.1 tools stay the same closed five; nothing was added to the tool
  surface, only an explicit trust gate in front of it.

**Blockers / open decisions**
- The referenced `docs/superpowers/plans/2026-09-04-v0.1-release-hardening.md`
  does not exist in the repo (any branch) or on disk, as in phases 0–3; the
  inline phase spec was treated as operative, and ToolPolicy's API was derived
  from it + the codebase. If the owner's plan named different fields/methods,
  the divergence is isolated to `internal/agent/toolpolicy.go`.

**Next action**
- Fresh session: next owner-assigned step (v0.1 tag + release notes or the
  next hardening phase).

### 2026-09-03 — v0.1 hardening, phase 5: bound and time out Ollama streams (owner task)
**Milestone:** owner-assigned step on `hardening/v0.1` (phase 5 of the v0.1
hardening plan; branch tip was `7fea64f`) · **Result:** done — see commit
"fix: bound and time out Ollama streams". No §10 milestone row to tick
(hardening phases are owner-assigned steps, not PLAN.md §10 milestones).

**Work done**
- New shared internal NDJSON stream decoder `internal/ollama/stream.go`, used
  by both `ChatStream` and `Pull`: frames events line-by-line over a 32 KiB
  bufio buffer and (a) rejects any single event whose raw wire bytes exceed
  4 MiB (`stream event exceeds 4194304 bytes`), checked while accumulating so
  memory stays ≤ cap; (b) enforces a per-byte **idle** timeout — an
  `idleReader` wraps the response body, bounds every read with the idle
  window, and on expiry cancels a *child* request context so net/http tears
  down the blocked read; (c) honors caller cancellation throughout (the child
  context derives from the caller's, so cancels propagate with the same
  `context canceled` errors as before). No total request deadline exists: a
  body delivering bytes at any cadence inside the window runs indefinitely.
  Idle default 90s per client (`Client.streamIdle`, set by `New`, injectable
  in tests); zero means default.
- `ChatStream` tracks cumulative decoded `message.content` + `message.thinking`
  + top-level `thinking` bytes and rejects > 16 MiB with `chat stream exceeds
  16777216 bytes` before delivering the crossing event; the terminal `done`
  requirement (EOF without `done` → `stream ended without done`) is unchanged.
- `Pull` uses the same per-event + idle protections but keeps no cumulative
  budget, so multi-minute downloads survive as long as progress lines keep
  arriving (a test streams 25 events across 3+ idle windows and completes).
- `postStream` helper (shared request build: child ctx + stream client)
  deduplicates the chat/pull POST path; both decode loops now `json.Unmarshal`
  per framed event, preserving every pre-existing error message shape
  (`decode stream: …`, in-band `{"error": …}`, `stream ended without
  done/success`, HTTP-error body parsing, Bearer header, non-2xx read). Public
  `Client`, `Chat`, `ChatStream`, `Pull` signatures untouched; `Chat` wrapper
  unchanged.
- New `internal/ollama/stream_test.go` (10 httptest tests, short injected
  idle): one oversized chat event, cumulative chat content+thinking overflow
  (5 × ~4 MiB events; thinking event crosses the 16 MiB budget), one oversized
  pull event, stalled chat body, stalled pull body (idle fires at 60 ms),
  steady pull progress outliving the idle window (no total deadline), caller
  cancellation beating a 5 s idle in both chat and pull, malformed pull NDJSON
  (`decode stream`), pull EOF without `success`. The first five went
  genuinely red (old code: no caps, no idle) then green; the last five guard
  existing behavior through the rewrite (red-able only by regression), and the
  pre-existing chat malformed/EOF/cancel tests stay green on the new decoder.
- PLAN §5 "Confirmed stream behaviors" gained the phase-5 bounds bullet.

**Commands + exit codes**
- RED: focused run (`-run 'Test(Chat|Pull)(Oversized…|…)'`) → first compile
  fail (`streamIdle` seam missing) → seam only → 5 behavioral FAILs (cap/idle
  messages absent) `1`.
- GREEN: focused run → 10/10 PASS `0`; `go test -count=1 ./internal/ollama`
  `0`; `go test -race -count=1 ./internal/ollama` `0` (6/6 repeat runs clean);
  `make check` `0` (build + `go test -count=1 ./...` + vet + gofmt).
- Environmental note: this host runs a localhost port prober (observed as
  `moshi-hook`; reproduced standalone with a raw `net.Listen` and zero client
  traffic) that sends stray `GET /` ~0.6–1.1 s after a new 127.0.0.1 port
  binds. It intermittently failed `TestChatOversizedEventRejected` /
  `TestChatCumulativeOverflowRejected` under `-race` (they hold listeners
  ~100 ms+ serving multi-MiB bodies and used the strict `fakeChatServer`
  helper that `t.Errorfs` on non-POST requests). Those two tests now use
  probe-tolerant inline handlers (stray requests get a silent 404; a genuine
  client bug still fails via the client's own error). Not a code defect.

**Decisions / lines to respect**
- The idle watchdog is per-received-byte, not per-request: a read that
  delivers nothing for the window aborts via child-context cancellation; a
  read that delivers (any amount, however slowly) resets the window. The
  90 s default is a constant; tests inject per client.
- Per-event cap counts raw wire bytes of the JSON line (strictly stronger
  than decoded size and the actual memory bound); the chat cumulative cap
  counts decoded content+thinking across events.
- The decoder returns clean `io.EOF` at an event boundary; "ended without
  done/success" is the caller's decision, so chat and pull keep their own
  terminal semantics on one shared framing path.
- Large streaming bodies are read through a 32 KiB bufio fill, keeping the
  per-read watchdog goroutine cheap on multi-GiB pulls.

**Blockers / open decisions**
- None. v0.1 (tag + release notes) still next; further hardening phases per
  owner as assigned.

**Next action**
- Fresh session: next owner-assigned step (v0.1 tag/release notes or the
  next hardening phase).
### 2026-09-04 — v0.1 hardening, phase 6: envelope asynchronous UI events (owner task)
**Milestone:** owner-assigned step on `hardening/v0.1` (phase 6 of the v0.1
hardening plan; branch tip was `43698da`) · **Result:** done — commit
"refactor: envelope asynchronous UI events". No §10 milestone row to tick
(hardening phases are owner-assigned steps, not PLAN.md §10 milestones).

**Work done**
- New per-child envelopes `agentEventMsg{ msg tea.Msg }` (agent_view.go) and
  `modelsEventMsg{ msg tea.Msg }` (models_view.go): the single message shape
  the root App accepts for each child's asynchronous results.
- Producer-side wrapping, so routing coverage is structural, not a per-type
  case list: the Agent model-list loader and the chat activity goroutine post
  `agentEventMsg` (model-list results + every chat-channel event —
  Token/ToolStart/ToolResult/ToolConfirm/Fallback/AgentDone + legacy
  agentTokenMsg/agentDoneMsg); the Models load/show/delete cmds, the pull
  goroutine (progress + completion), and the pull dialog's spinner ticks all
  post `modelsEventMsg`.
- `App.Update` (app.go): the two concrete child case lists are replaced by
  exactly one case per child that unwraps and delegates to
  `ModelsView.Update`/`AgentView.Update`; bare `spinner.TickMsg` routing to
  ModelsView is gone. Root-owned `settingsThemeMsg`, `agentThemeMsg`,
  `settingsSaveDoneMsg`, `WindowSizeMsg`, `KeyMsg` remain root messages.
- Each view keeps its concrete cases and adds a one-line unwrap for its own
  envelope, so a standalone view (or a test that runs the view's commands
  directly) is self-consistent: wrapped results that come straight back are
  re-dispatched to the same switch.
- Tests: new `TestAppEnvelopeRoutingTable` (18 rows — every currently defined
  async payload, agent and models, injected through its envelope at the root
  with per-row state assertions proving it reached the intended child,
  including an enveloped spinner tick advancing the Models dialog spinner);
  `TestUnrelatedSpinnerTickNotRoutedToModels` (a bare tick is dropped, not
  silently animating ModelsView); explicit root-flow regressions
  `TestAppRoutingChatCompletesTurn`, `TestAppRoutingPullCompletesAndReloads`,
  `TestAppRoutingChatErrorSurfaced`; the existing tool-confirmation and
  declined-write routing regressions were preserved (envelope-adapted
  injections; `pumpAgent` unchanged — the chat channel now carries
  envelopes, which is exactly what the shell routes). Root-level test
  injections (app/agent_view/m7/golden) now use the envelopes; view-level
  concrete injections and channel pumps are untouched; direct `cmd()` result
  assertions (deleteResultFromCmd, show result, canceled list) unwrap the
  envelope. PLAN §6 "Streaming to UI" gained the phase-6 envelope bullet.

**Commands + exit codes**
- RED: focused `-run 'TestAppEnvelopeRoutingTable|TestUnrelatedSpinnerTickNotRoutedToModels'`
  → behavioral FAILs `1` (envelopes dropped at the shell; raw spinner tick
  silently routed to ModelsView — frame advanced).
- GREEN: same focused run → `ok` `0`; `go test -count=1 ./internal/ui` `0`;
  `go test -race -count=1 ./internal/ui` `0` (8.98s, clean); `make check` `0`
  (build + `go test -count=1 ./...` all packages ok + vet + gofmt).

**Decisions / lines to respect**
- The envelope is applied at the producer boundary (cmds return it; chat/pull
  goroutines post it), and the App shell is the only unwrap point in
  production; the views' own unwrap case exists purely so a standalone view
  remains a coherent tea.Model when its own wrapped results return to it.
- Root-bound messages produced by children (agentThemeMsg; settings'
  settingsThemeMsg/settingsSaveDoneMsg; huh form internals) are deliberately
  NOT wrapped — the shell routes them itself.
- Channels stay typed `chan tea.Msg`; the payload is structural, so future
  payload types are routed by construction.

**Blockers / open decisions**
- None. v0.1 (tag + release notes) still next; further hardening phases per
  owner as assigned.

**Next action**
- Fresh session: next owner-assigned step (v0.1 tag/release notes or the
  next hardening phase).

### 2026-09-04 — v0.1 hardening, phase 7: align the product contract (owner task)
**Milestone:** owner-assigned step on `hardening/v0.1` (phase 7 of the v0.1
hardening plan; branch tip was `4ebd1fa`) · **Result:** done — commit
"docs: align product contract for v0.1". No §10 milestone row to tick
(hardening phases are owner-assigned steps, not PLAN.md §10 milestones).

**Work done**
- **Version identity.** `const Version = "0.6.0-m6"` → `var Version = "dev"`
  in `cmd/self-tui/main.go`; the `-version` output is produced by a single
  `printVersion(w io.Writer)` seam, and a new test temporarily sets `Version`
  (`dev`, `1.2.3-rc1`, `0.7.0`) and proves the formatting stays exactly
  `selftui <value>`. Live check: `go run ./cmd/self-tui -version` and
  `bin/selftui -version` both print `selftui dev`. **No tag was created** in
  this phase (v0.1.0 tagging stays a separate owner step).
- **`/save` → `/export` rename.** The Agent slash command that flushes the
  transcript is now `/export` ("flush + reveal the transcript file path"):
  slash-command list, help overlay, dispatch case, notice prefix
  (`session:` → `transcript:`), comments, tests, and the four golden fixtures
  (agent-help compact/wide, agent-slash compact/wide) all updated. The
  success notice reports the Markdown transcript path; the test now also
  asserts the notice never claims the conversation can be resumed. Session
  files remain append-only Markdown exports (`internal/session` untouched
  beyond comments).
- **Deterministic small-terminal state.** New App gate: when a `WindowSizeMsg`
  arrives below `minTermW=40` or `minTermH=12` (layout.go), the shell stores
  the geometry and does **not** forward a sub-minimum size to the children
  (their layouts assume ≥40x12), keys are inert (only `ctrl+c` still quits),
  and `App.View()` renders a bounded placeholder naming `terminal too small`,
  the current dimensions, and `minimum: 40x12` — rows truncated to the window
  width and capped at the window height, so it cannot overflow even at 1x1.
  A zero-size frame (no pty size negotiated yet) is explicitly **not** "too
  small" (M0a edge note preserved). New `small_terminal_test.go`: a
  table-driven boundary suite (39x12 / 40x11 / 39x11 / 39x100 / 200x11 /
  1x30 / 80x1 / 1x1 / 40x12 / 41x12 / 40x13 / 72x30 / 120x40) asserting the
  message contract, the normal shell at/above the minimum, and frame
  boundedness (no row wider than the window, no view taller), plus a Unicode
  content case (CJK, box drawing, block shading, emoji, combining accent in
  the Agent transcript) at 40x12/41x12/40x13/72x30/39x12/40x11.
- **Public docs rewritten to the v0.1 contract** (README, PLAN §top +
  §12): v0.1 is a single-process Linux/WSL TUI for Ollama; sessions are
  in-memory with the Markdown transcript export surviving exit but **not**
  resumable; tools are off by default and require an explicit workspace;
  command execution is not shipped; non-loopback tokens require HTTPS;
  native Windows/macOS not supported. README gains a "v0.1 product contract"
  section and documents the 40x12 minimum + `/export`.
- **Historical evidence preserved with a dated correction.** LEDGER.md gains
  this entry + an explicit "Historical record" label at the top (all dated
  entries below are as-written history; superseded claims are governed by the
  PLAN contract note). PLAN.md's stale top-level "PLANNING. No implementation
  code yet." was removed and replaced by a dated release-hardening correction;
  the remaining old strings in PLAN.md (M3b `run_command`/read-only git row,
  M6 `0.6.0-m6`, M7-follow-up-2 `/save`) are each annotated "(Historical
  record…)". docs/reconnect.md and docs/m0a-gate-evidence.md carry explicit
  historical-evidence labels (version strings there are as-captured);
  docs/run-command-containment.md is now "historical deferred-design record".
  COUNCIL-MEMO.md is labeled a historical advisory record and its audited
  artifact path scrubbed.
- **Personal absolute paths scrubbed** from README (none), the new
  CONTRIBUTING/SECURITY, PLAN.md (repo path removed from §2, §10 M0, §11 #1),
  AGENTS.md (session-ritual cwd now "this repository's root"), COUNCIL-MEMO.md,
  and the LEDGER preamble (same reword); historical entries inside LEDGER keep
  them only under the top historical label.
- **New repo files:** `LICENSE` (Apache-2.0 — no recorded owner decision
  specified another license), `SECURITY.md` (private vulnerability reporting
  via GitHub's Security tab; no invented email), `CONTRIBUTING.md`, and
  `CHANGELOG.md` (Unreleased section). README Project docs list updated.

**Commands + exit codes**
- `rg -n '0\.6\.0-m6|Chat sessions persist|/save|read-only git|PLANNING\. No
  implementation' .` → every remaining hit is **historical and explicitly
  labeled**: dated LEDGER entries (this one included, which quotes the gate
  patterns), docs/reconnect.md (historical evidence label), and PLAN.md
  inline "(Historical record…)" annotations; outside those, no hits for any
  of the five patterns — README/CHANGELOG/docs carry none.
- RED→GREEN per slice: version test (compile red → green `0`); `/export`
  tests (behavioral red → green `0`); small-terminal tests (red → green `0`,
  including 40x12 wide-rune frames with no overflow).
- `go test -count=1 ./cmd/self-tui ./internal/ui ./internal/session` → ok `0`.
- `go test -race -count=1 ./internal/ui ./internal/session` → ok `0`.
- `make check` (build + `go test -count=1 ./...` all packages ok + vet +
  gofmt) → exit `0`.
- Golden fixtures regenerated with `go test ./internal/ui -run TestGoldenRender
  -update`; `git diff` of testdata/golden touches only the four /export
  fixture lines.
- Environmental note: two cold-start full `internal/ui` runs failed before any
  edit (no test named; view-frame output), then passed 10+ consecutive clean
  runs incl. `-race` — consistent with the documented localhost port-prober
  flake on strict fake-host tests (see phase-5 LEDGER note), not a code
  defect; final gate evidence below is from clean runs.

**Decisions / lines to respect**
- `Version` is a `var` (default `dev`); release tagging stays a separate owner
  step — **no `v0.1.0` tag in this phase**.
- The transcript command is `/export` and its copy never claims resumability;
  "session" naming survives only in internal package/field names.
- The small-terminal floor is 40x12; sub-minimum sizes never reach child
  views; zero-size frames are not "too small".
- Old strings that remain anywhere are historical and explicitly labeled;
  the v0.1 product contract in PLAN.md/README.md governs current claims.
- Apache-2.0 LICENSE added (no other recorded owner license decision).

**Blockers / open decisions**
- None. v0.1 tag + release notes still next; further hardening phases per
  owner as assigned.

**Next action**
- Fresh session: next owner-assigned step (v0.1 tag/release notes or the
  next hardening phase).

### 2026-09-04 — v0.1 hardening, phase 8: reproducible CI + release gates (owner task)
**Milestone:** owner-assigned step on `hardening/v0.1` (phase 8 of the v0.1
hardening plan; branch tip at start was `22271da`) · **Result:** done —
commits listed below. No §10 milestone row to tick (hardening phases are
owner-assigned steps, not PLAN.md §10 milestones). **No tag was created or
pushed** — v0.1.0 tagging stays a separate owner step.

**Work done**
- **Toolchain pin (checked against the live official source).** Current
  official stable Go on 2026-09-04 is **1.27.1** (go.dev/dl JSON). CI and the
  release gates pin Go **1.27.1** (recorded in both workflows + README +
  CONTRIBUTING); the local gate evidence below was produced under the same
  toolchain (`GOTOOLCHAIN=go1.27.1`, its `bin` on PATH) so local == CI. The
  module's `go 1.25.8` directive stays the language floor (no go.mod bump —
  out of scope).
- **govulncheck pin = v1.7.0.** The GitHub "latest release" endpoint
  misleadingly reports v1.1.4; that release **panics** under Go 1.27.1
  (`unexpected expr: *ast.KeyValueExpr` — its x/tools v0.29 SSA predates
  Go 1.27 stdlib syntax). v1.7.0 (newest tag per the Go module proxy) scans
  cleanly. Pinned in Makefile docs, README, CONTRIBUTING, both workflows,
  and the release-check install hint.
- **`make vuln` surfaced two reachable advisories (first run, v1.7.0):**
  goldmark **GO-2026-5320** (XSS in the glamour markdown render path, trace
  through `agent_view.go` renderBlock) and x/text **GO-2026-5970** (infinite
  loop). Fixed by bumping the indirect deps to goldmark v1.7.17 and
  golang.org/x/text v0.39.0 in their own commit; golden renders unchanged
  after the bump. govulncheck now reports 0 affecting (7 in imported
  packages + 3 in required modules remain, none reachable — non-blocking).
- **Makefile.** New `VERSION ?= dev`; new targets `race`
  (`go test -race -count=1 ./...`), `vuln` (`govulncheck ./...`),
  `build-linux-amd64`/`build-linux-arm64` (`CGO_ENABLED=0 GOOS=linux GOARCH
  <exact> go build -trimpath -ldflags "-s -w -X main.Version=$(VERSION)" -o
  dist/selftui-linux-<arch>`), and `release-check` (runs
  `scripts/release-check.sh`); smoke targets added to `.PHONY` (review fix).
- **scripts/release-check.sh (new).** `set -euo pipefail`; requires
  `VERSION` matching `^v[0-9]+\.[0-9]+\.[0-9]+$` and a clean worktree
  (`git status --porcelain` empty; ignored `bin/`/`dist/` don't count), then:
  go mod verify → gofmt check → go vet → `go test -count=1 ./...` → `go test
  -race -count=1 ./...` → govulncheck → both Linux builds → per-binary
  version-stamp check → deterministic archives → `dist/SHA256SUMS` (entries
  `dist/`-prefixed so `sha256sum -c dist/SHA256SUMS` works from the root).
  `dist/` is rebuilt fresh each run. **The script never creates or pushes a
  git tag.** Two non-obvious engineering decisions, both verified: (1)
  deterministic archives via `tar --sort=name --mtime=@0 --owner=0 --group=0
  --numeric-owner` + `gzip -n` (two full gate runs produced byte-identical
  SHA256SUMS); (2) the cross-arch (arm64-on-amd64) `-version` check cannot
  exec without qemu/binfmt, and `go version -m` does not record `-ldflags`,
  so the check falls back to the bytes the linker wrote — the exact version
  as an isolated string plus the `selftui %s` format literal (Go packs
  rodata without separators, so the format is a substring `-F` match).
- **scripts/verify-binary-version.sh (new, review fix).** The exec-or-
  embedded-string ladder was duplicated between release-check.sh step 8 and
  release.yml; it now lives in one shared script both call.
- **.github/workflows/ci.yml (new).** Triggers: pull_request + push to main.
  Least privilege (`contents: read`, no secrets). Steps: go mod verify, make
  fmt, make vet, make test, make race, govulncheck (pinned v1.7.0 install),
  CGO-disabled Linux builds. Concurrency cancel-in-progress.
- **.github/workflows/release.yml (new).** Triggers only on pushed `v*`
  tags. `VERSION` = the tag. Runs the complete release gate
  (`make release-check`), a dedicated step verifying the tag equals the
  version stamped into both binaries (via the shared helper), generates
  release notes from the CHANGELOG section for the version (tag or bare),
  falling back to `[Unreleased]`, then to `gh release create --generate-
  notes`; uploads the two `selftui-<version>-linux-<arch>.tar.gz` archives +
  `SHA256SUMS`. Least privilege (`contents: write`, default GITHUB_TOKEN, no
  secrets).
- **.gitignore.** `/bin/` and `/dist/` were already ignored (verified with
  `git check-ignore`); no change was needed.
- **Docs.** README gained a "Release engineering (v0.1)" section (targets,
  gate contract, artifacts, no-tag rule, Go/govulncheck pins, workflows);
  CONTRIBUTING and CHANGELOG updated; a review nit (README overclaiming
  ci.yml parity) and a CHANGELOG duplicate `### Added` heading were fixed in
  a separate docs commit.

**Code review (requested, complete `main..HEAD` diff).** Two parallel
read-only reviewer lanes (standards + spec, fresh contexts):
- Standards lane: **0 hard violations, 7 judgement calls** — the four
  substantive ones were fixed in separate commits: duplicated version-check
  ladder (extracted to `scripts/verify-binary-version.sh`), asymmetric
  cross-arch fallback (now also requires the `selftui %s` format), govulncheck
  probe after the expensive steps (moved up front), and the smoke `.PHONY`
  omission. Supply-chain note on mutable action refs (@v4/@v5) left as-is
  (no repo rule; documented).
- Spec lane (Phase-8 brief as spec): all requirements present — the five
  Makefile targets with the exact recipes, release-check step order +
  invariants + no-tag guarantee, ci.yml trigger/check set + Go version in
  workflow and README, release.yml v*-only trigger + tag-vs-version
  verification + full gate + archive/SHA256SUMS upload + release notes +
  least privilege/no secrets; `-trimpath` noted as an unasked-but-benign
  addition (reproducibility). Verdict: **`V0_1_RELEASE_CANDIDATE_READY`**.
- Both reviewers independently confirmed no local tag exists.

**Commands + exit codes (all under Go 1.27.1; govulncheck v1.7.0)**
- Pre-commit gates at the phase-8 commit: `git diff --check` → 0;
  `make check` → 0; `make race` → 0; `make vuln` → 0 (0 affecting).
- Full release gate before review: `VERSION=v0.1.0 make release-check` → 0
  twice, with **byte-identical** `dist/SHA256SUMS` across runs
  (reproducibility proven). One earlier run failed at the arm64 stamp check
  (rc=1): `grep -Fxq` under `set -o pipefail` exits on first match and
  SIGPIPEs `strings`, so the pipeline rc was 141 even on success — fixed by
  reading the full stream (`grep -Fx … >/dev/null`); regression-tested
  positive and negative.
- Full release gate after the review fixes (final gate, clean tree at code
  HEAD): `VERSION=v0.1.0 make release-check` → **0**; `git diff --check` → 0;
  `make check` → 0; `make race` → 0; `make vuln` → 0;
  `./dist/selftui-linux-amd64 -version` → `selftui v0.1.0` (rc 0);
  `sha256sum -c dist/SHA256SUMS` → both OK (rc 0).
- Helper checks: `scripts/verify-binary-version.sh dist/selftui-linux-{amd64,
  arm64} v0.1.0` → ok (executed / embedded strings); negative test with
  v9.9.9 → rc 1 as designed.
- Artifacts at the final gate: `dist/selftui-linux-amd64`
  `22bb92a6ea03ec3121d81f7fb579cb737fcb7a9ccefc3798a11d7426e3d1b092`,
  `dist/selftui-linux-arm64`
  `7766b6a19a6c6f8d4635f33b7524ec402bbd2eb1344f58e5a8594c1b58ccde86`,
  `dist/selftui-v0.1.0-linux-amd64.tar.gz`
  `c33c1c4829f538c8538a76eed6c8d7662d4d7114b5744fe2c3cbbec6d1ee3def`,
  `dist/selftui-v0.1.0-linux-arm64.tar.gz`
  `73510c83c78a9d50b35c4099833fa86ff592cb63db1aa634b3093b07192729ba`,
  `dist/SHA256SUMS` (over the two archives).
- Environmental note: two `internal/ui` cold-start runs failed before any
  edit (settings-form tests stuck on "writing config…"), then passed 3/3 in
  isolation and 2× full-suite — the documented localhost/timing flake
  (phase-5 LEDGER note), not a code defect; all final-gate evidence is from
  clean consecutive runs.

**Decisions / lines to respect**
- Pinned toolchain **Go 1.27.1** (current official stable, 2026-09-04) and
  govulncheck **v1.7.0** govern CI + the release gate; the module floor stays
  `go 1.25.8`.
- `release-check` and both workflows **never tag**; `release.yml` only reacts
  to a tag the owner pushes. No push/merge happened in this phase.
- `-trimpath` and deterministic-archive flags are deliberate
  reproducibility additions beyond the brief's literal recipes.
- The cross-arch version check intentionally uses embedded-string evidence
  (documented in the script header) because `go version -m` does not record
  `-X`.
- Mutable action refs (`actions/checkout@v4`, `setup-go@v5`) are an accepted
  trade-off (no SHA pinning requirement documented); revisit if supply-chain
  posture tightens.

**Blockers / open decisions**
- None. v0.1 tag + release notes remain the next owner step (fresh session);
  when the owner tags `v0.1.0`, `release.yml` re-runs this exact gate and
  uploads the artifacts + CHANGELOG-derived notes.

**Next action**
- Fresh session: owner pushes tag `v0.1.0` (after this gate is green at that
  commit) and publishes the release; or the next owner-assigned step.

### 2026-09-04 — v0.1 release-candidate audit fixes: portable goldens, whitespace carve-out, probe-tolerant fake hosts (owner task)
**Milestone:** owner-assigned step on `hardening/v0.1` (follow-up to the read-only
RC audit of the same branch) · **Result:** done — F1, F2, F3 fixed and verified;
F4/F5 are environment-only (no repo defect, recorded below). No §10 milestone row
to tick (owner-assigned step). **No tag was created or pushed.**

**Work done**
- **F1 (blocker): golden fixtures are now checkout-path independent.** The
  fixtures byte-compare full shell renders whose status rows show the canonical
  workspace; with the default empty `workspace_root` that label is the process
  cwd, so the suite only passed from `/home/calvin/SelfTUI` — reproduced by
  running `./internal/ui` from a `/tmp` copy (drift on every path-bearing
  frame). Post-render cwd normalization alone was insufficient: status-row
  layout depends on label *length*, so a different-length checkout reflows
  padding before any token substitution. Fix: golden frames now build the App
  through a new `goldenApp` helper with a fixed workspace root `/tmp` (exists on
  every Linux/WSL host, passes config validation, stable length 4), and
  `normalizeWorkspace` strips any accidental cwd text before store/compare as a
  guard. 19 fixtures regenerated; the diff is workspace-label/padding only.
- **F2 (minor): `git diff --check` is green again.** `internal/ui/testdata/golden/*.txt`
  rows are padded to the full frame width, so trailing spaces are load-bearing
  fixture content; a repo-root `.gitattributes` exempts exactly the
  trailing-space checks for those files. Range check `main...HEAD` now exits 0
  (was 2, 22 findings).
- **F3 (environmental flake): fake-host HTTP helpers are probe-tolerant.** This
  host's `moshi-hook` localhost port prober (reproduced live: a bare listener
  on a fresh 127.0.0.1 port received `GET / HTTP/1.1` ~2 s after bind) made
  strict fake hosts fail under `-race` intermittently (~1 in 3 full-suite runs;
  observed on `TestAppRoutingPullCompletesAndReloads`, geometry, and chat
  tests). Converted every handler-side mismatch error to a silent 404 across
  `internal/ui` (`fakeShowServer`, `fakeDeleteServer`, `fakePullServer`,
  `fakeOllamaUI`) and `internal/ollama` (List/Bearer/NoToken/Show/
  HostTrailingSlash/Delete/Pull/chat servers). Genuine client mistakes still
  fail through the client's own 404 error, and dedicated client-side
  method/path assertions (e.g. `TestDeletePostsName`) are unchanged.
- **F4/F5 (environment, no code change):** `govulncheck` was not on PATH —
  installed the repo-pinned v1.7.0 out-of-repo (GOPATH) on go1.27.1 to run the
  mandated gates. `gitleaks` is not installed → `SECRET_HISTORY_SCAN=UNRESOLVED`
  (not installed, per audit instruction); a supplementary `git log --all -p`
  scan over high-signal secret patterns found 0 matches.

**Commands + exit codes**
- `gofmt -l internal/ui internal/ollama` → empty `0`; `go vet ./internal/ui ./internal/ollama` `0`.
- Golden regen `go test ./internal/ui -run TestGoldenRender -update` `0`; fixture diff reviewed (19 files, workspace-label/padding only).
- `make test` `0` (uncached `./...` all ok); golden stability re-run `0`.
- Portability proof: full `go test -count=1 ./...` (go1.27.1) from a fresh
  `/tmp/selftui-verify.*` copy → all ok `0` (failed before the fix).
- `go test -race -count=1 ./...` ×3 → `0` each (prober still running on the host;
  previously ~1-in-3 full-suite runs flaked).
- `git diff --check` (worktree) `0`; `git diff --check main...HEAD` `0`.
- `make check` `0`; `make vuln` (govulncheck v1.7.0) `0` (0 reachable).
- `VERSION=v0.1.0 make release-check` → gate runs on the clean commit (see below).

**Decisions / lines to respect**
- Golden frames pin layout for a fixed workspace root (`/tmp`), not the default
  empty-root→cwd identity; the compact/wide presence of the label in some frames
  shifted accordingly (shorter label fits more rows) — deterministic everywhere.
- `.gitattributes` scopes the trailing-space exemption to
  `internal/ui/testdata/golden/*.txt` only; all other whitespace checks stay on.
- Silent-404 fake hosts keep real client bugs detectable via the client's own
  error; positive client-side method/path assertions were preserved, not
  weakened.

**Blockers / open decisions**
- None (code). `SECRET_HISTORY_SCAN=UNRESOLVED` until gitleaks is run on the
  repo (owner decision — nothing installed automatically). GitHub CI has never
  run on the remote (`gh run list` empty; only `main` is pushed); the first push
  of `hardening/v0.1` will exercise `ci.yml` for the first time.

**Next action**
- Owner (fresh session): push `hardening/v0.1`, watch the first CI run go green,
  then tag `v0.1.0` and publish — release.yml re-runs this gate at the tag.

### 2026-09-04 — v0.1 runbook step 4: PR #1 merged to main; CI bring-up found two latent defects; branch protection enforced; gate green on merged main (owner task)
**Milestone:** owner-assigned step (release runbook §4 "Push a Pull Request" —
push branch, open PR vs `main`, require CI, review diff, merge, rerun the full
release gate on the merged commit). No §10 row to tick (owner-assigned step);
§12 tail updated. **Result:** done — `hardening/v0.1` merged into `main`
(commit `1a45554`), first-ever remote CI runs green (PR + merged main), branch
protection enforced on `main`, full release gate PASSED on the merged commit.
**No tag was created or pushed** (runbook step 5 stays for a fresh session).

**Work done**
- Pushed `hardening/v0.1` (14 commits, +5,619/−813) and opened **PR #1**
  (`v0.1 hardening: security, reproducible CI/release gates, RC fixes`) vs
  `main` with a theme-grouped body; no PR template exists in the repo. This
  was the repo's **first remote CI run ever** (LEDGER predicted this).
- **CI bring-up found two latent defects** (both fixed on the branch before
  merge; neither was catchable by the local release gate):
  - **F1 — workflow parse error (both files).** `ci.yml` and `release.yml`
    used the `env` context inside job `name:` — GitHub's parser rejects it
    (job-name context is limited to github/inputs/matrix/needs/strategy/
    vars), so every dispatch died in 0 s with "workflow file issue" and no
    check ever attached to the PR. Caught with `actionlint` v1.7.7 (installed
    out-of-repo). Fix `9bd1e0c`: static job names — which also keeps the
    required-status-check context stable across Go version bumps. First remote
    parse of these files; the local gate does not lint workflow YAML
    (candidate addition for v0.1.1: run actionlint in ci.yml/`make check`).
  - **F2 — session transcript same-tick reuse.** `TestAppendModeContinuesAfter
    Reopen` failed on the runner: the transcript name is
    `chat-<millisecond>-<pid>.md`, so a second `Open` in the same process
    landing within one clock tick collides and `O_CREATE|O_APPEND` silently
    reopened the previous run's file. Local runs (slower fsync) never hit the
    ~1 ms window. Fix `5ad160a`: `O_EXCL` + retry on the next tick — a fresh
    per-run file is now a guarantee, clock-granularity agnostic (plain
    `UnixNano` would not help on a coarse VM clock). Regression test occupies
    the current instant's candidate name and asserts Open never reuses it.
    150× repeated + race ×5 green locally.
  - **F3 — settings-save flake (test harness, surfaced by the docs PR #2's
    CI run).** The `drive`/`execHop` test driver dropped any command slower
    than 100 ms. The settings form runs `config.Save` off-loop and reports
    back with `settingsSaveDoneMsg`; when a write (temp file + fsync)
    exceeded 100 ms under load — the "writing config…" family documented at
    phase 5 and in the RC audit — the driver silently lost the message and
    three settings tests failed together (stuck at `settingsSaving`), twice
    on this host and once on the runner. Fix (`internal/ui/settings_view_test.go`):
    hop policy is now domain-driven — while the form is editing, pending
    commands are animation noise (the form's cursor restarts a 530 ms blink
    chain per keypress; bubbles spinner ticks reschedule) and keep the short
    grace, but once the form completes (`settingsSaving`/`settingsSaved`)
    the pending command is real work and is awaited up to a 2 s deadlock-
    guard cap. Naively raising the cap made every blink hop pay its full
    interval (settings suite ballooned to 134 s), so the state-based split
    is what keeps it fast AND correct: 50× stress of the flaky family +
    race green, full suite back to ~4.8 s. Test-only change; no production
    code touched.
- **CI on PR #1: green** (run `33875771938`, check "Go fmt · vet · test ·
  race · vuln · cross-build" reported on head `5ad160a`).
- **Branch protection on `main`** (PUT `branches/main/protection`):
  required status check `Go fmt · vet · test · race · vuln · cross-build`
  (strict false); required PR with `required_approving_review_count: 0`
  (solo: the owner cannot approve their own PR, so enforcement comes from the
  required check + PR flow, not approvals); **`enforce_admins: true`** so the
  owner's merges are gated too (this is the binding part); no restrictions.
  Applied while PR #1 was open → mergeStateStatus CLEAN, merged through the
  gate. Owner retains the settings-level escape hatch (reversible).
- **Merged PR #1 with a merge commit** (not squash — the 15-commit hardening
  history is the auditable record), commit `1a45554`; branch
  `hardening/v0.1` **kept** (not deleted), per the runbook.
- **Full release gate on merged `main` (`1a45554`), pinned toolchain:**
  `VERSION=v0.1.0 make release-check` → **PASSED** (0 reachable
  vulnerabilities under go1.27.1; both binaries stamp `selftui v0.1.0`;
  deterministic archives + SHA256SUMS). `make check` → 0; `make race` → 0;
  `git diff --check` → 0; `./dist/selftui-linux-amd64 -version` →
  `selftui v0.1.0`; `sha256sum -c dist/SHA256SUMS` → OK ×2.
- **CI on merged `main` (push trigger): SUCCESS** (run `33876257480`) — first
  CI run on the default branch.
- New artifact hashes (differ from the phase-8 set because commits `9bd1e0c`
  and `5ad160a` changed the tree): `dist/selftui-v0.1.0-linux-amd64.tar.gz`
  `2d389a12fec85049fe0bad9f6406bf68bf4ca322af7790cd30bf25129af281be`,
  `dist/selftui-v0.1.0-linux-arm64.tar.gz`
  `b26e04bdef7d012b10775dc1e7a10e998f065e145f82d4bbe4208c299f1fd04d`.

**Commands + exit codes (all on merged `main` content)**
- `git push -u origin hardening/v0.1` 0 · `gh pr create …` → PR #1 ·
  `actionlint v1.7.7` on both workflows → found F1 (2 findings); clean after
  fix · `gh pr checks 1 --watch` → green · `go test ./internal/session
  -count=150` 0 · `go test -race ./internal/session -count=5` 0 ·
  `go test -count=1 ./...` 0 · PUT branch protection → 200.
- `gh pr merge 1 --merge` → merged (`1a45554`) · `git checkout main && git
  pull --ff-only` 0 · tree(main) == tree(hardening/v0.1) (`726a203…`).
- With `GOTOOLCHAIN=go1.27.1` + module-cached toolchain bin + GOPATH on PATH:
  `VERSION=v0.1.0 make release-check` 0 · `make check` 0 · `make race` 0 ·
  `git diff --check` 0 · `sha256sum -c dist/SHA256SUMS` 0 · `gh run watch
  33876257480` → success.
- First gate attempt WITHOUT the pinned toolchain failed (govulncheck under
  the shell's default go1.25.8 reported 13 stdlib advisories) — a toolchain
  artifact of the run environment, not a repo defect; CI (go1.27.1) and the
  pinned local gate are both green.

**Decisions / lines to respect**
- Enforcement model: required CI check + PR flow via branch protection with
  `enforce_admins: true` and zero required approvals; branch protection
  "require PR" for the owner comes from protection itself (direct pushes to
  `main` are now rejected) — the docs commit for this entry lands via PR #2.
- Job names in both workflows are now static; the required-check context
  string is `Go fmt · vet · test · race · vuln · cross-build` and must stay
  in sync if the job is ever renamed (else every PR blocks).
- Release gate and CI must run under the pinned toolchain
  (`GOTOOLCHAIN=go1.27.1`, govulncheck v1.7.0); the module floor `go 1.25.8`
  is unchanged. Running govulncheck under an older Go reports stdlib
  advisories that are environmental, not repo defects.
- `actions/checkout@v4` / `actions/setup-go@v5` now emit a Node-20
  deprecation warning (GitHub 2025-09-19, forced to Node 24) — still
  functional; mutable action refs remain an accepted trade-off (recorded
  phase 8); revisit action majors in v0.1.1.
- Branch `hardening/v0.1` intentionally kept; delete after the v0.1.1 patch
  milestone or once the owner has finished diff-reviewing merged PR #1.

**Blockers / open decisions**
- None (code/CI). `SECRET_HISTORY_SCAN=UNRESOLVED` persists (gitleaks never
  installed) — required before the repo goes public (runbook step 5/6).
- Owner review of the merged PR #1 diff in GitHub remains worthwhile (runbook
  item 4) — the PR page is the auditable record; branch kept for that.

**Next action**
- Fresh session (runbook step 5): merged `main` passes the gate (this entry)
  → create an annotated tag `v0.1.0` on the `main` tip, push it, watch
  `release.yml` re-run the gate at the tag and publish assets; independently
  download + verify SHA256SUMS and `selftui v0.1.0` version strings; then the
  history audit (gitleaks) before any public-visibility change.

### 2026-09-04 — v0.1 runbook step 5: v0.1.0 published; release gate flake (test-harness deadlock) found by the first tag run and fixed via PR #3 (owner task)
**Milestone:** owner-assigned step (release runbook §5 "Tag + publish" — create
the annotated tag on the merged `main` tip, let `release.yml` re-run the gate
at the tag and publish, then independently verify the released assets). No §10
row to tick (owner-assigned step); §12 tail updated. **Result:** done —
**SelfTUI v0.1.0 released** (2026-09-04T14:59:47Z, 3 assets: both Linux
archives + SHA256SUMS, not draft/prerelease). The first tag run **failed the
gate** on a rare test-harness deadlock; root cause fixed in **PR #3** (merged
`c70bf89`) and the tag moved pre-release to the fixed tip before publishing.

**Work done**
- **Local gate at merged `main` (`1bf98bb`) passed**, pinned toolchain
  (`GOTOOLCHAIN=go1.27.1`, `~/go/bin` on PATH for govulncheck v1.7.0):
  `VERSION=v0.1.0 make release-check` → PASSED, 0 reachable
  vulnerabilities; both binaries stamp `selftui v0.1.0`. Release-notes
  fallback confirmed: no `## [v0.1.0]` changelog section yet, so the
  workflow's `[Unreleased]` fallback supplies the notes body (55 lines).
- **Created + pushed annotated tag `v0.1.0`** (message "SelfTUI v0.1.0",
  unsigned — no signing key configured) at `1bf98bb`.
- **First release run FAILED the gate** (run `33884427515`, `release.yml`,
  step "Run the complete release gate"): `selftui/internal/agent` hung
  `go test -race` for the full 600.052s package timeout on the 2-vCPU
  runner. `TestRunnerCancellationReturnsPromptly` (added with the M3a
  cancellation work) never completed.
- **Root cause (diagnosed from the goroutine dump + Go 1.27.1 sources +
  deterministic repro):** test servers that stream a response and then park
  on `<-r.Context().Done()` without reading the request body can deadlock in
  cleanup. net/http only arms its client-close-detection **background read**
  once the request body reaches EOF (`registerOnHitEOF`/`startBackgroundRead`,
  `net/http/server.go` `conn.serve`); `httptest.Server.Close` deliberately
  does **not** close `StateActive` connections, so the parked handler is
  released only when the server notices the client disconnect. The
  write-path drain that normally consumes small Content-Length bodies
  (`server.go` ~1447, Issue 15527 deadlock guard) is **not guaranteed** —
  skipped when `closeAfterReply` is set or raced — so the background read
  was never armed (the CI dump shows only the handler, `srv.Close`, and the
  alarm goroutine alive). Deterministic repro of the mechanism: an
  unterminated chunked-body request + a parking handler never releases, even
  after `CloseClientConnections`.
- **Flake reproduced locally** at the runner's shape:
  `GOMAXPROCS=2 go test -race -count=200 -timeout 90s -run
  'TestRunnerCancellationReturnsPromptly|TestChatContextCancelled|
  TestPullContextCancelled' ./internal/agent ./internal/ollama` → agent
  package hung to the 90s timeout (same deadlock shape).
- **Fix (test-only, PR #3 `fix/cancel-test-hang`, 4 files / +20):** each
  write-then-park test handler now drains the request body first
  (`io.Copy(io.Discard, r.Body)`), which deterministically arms the
  background read before the handler parks — `internal/agent/runner_test.go`
  (TestRunnerCancellationReturnsPromptly, the CI hang), `internal/ollama/
  stream_test.go` (stallServer), `internal/ollama/chat_test.go`
  (TestChatContextCancelled), `internal/ollama/ollama_test.go`
  (TestPullContextCancelled). The pre-canceled-context test that never opens
  a connection is untouched. No production code touched.
- **Fix validated:** the same 200-run adversarial workload is green in 1.7s;
  full agent+ollama suites ×30 under `GOMAXPROCS=2 -race` green (2.3s/68s);
  `VERSION=v0.1.0 make release-check` PASSED at `e53a931`.
- **PR #3**: CI green (run `33886703899`), merged with a merge commit
  `c70bf89`; branch `fix/cancel-test-hang` deleted.
- **Tag moved pre-release** `1bf98bb` → `c70bf89` (nothing had been published
  — the first run failed before the publish step; private repo). Deleted
  local + remote `v0.1.0`, re-created the annotated tag at the new tip,
  pushed.
- **Second release run SUCCESS** (run `33886825863`): gate passed at the tag
  (make release-check + per-binary version verification), release notes
  generated from the CHANGELOG `[Unreleased]` fallback, release created —
  **SelfTUI v0.1.0**, 3 assets. `ci.yml` on merged `main` also green (run
  `33886814058`).
- **Independent verification (fresh dir `/tmp/v010-verify`, CI-published
  assets only, never the local `dist/`):** `sha256sum -c` OK for both
  archives against the published manifest (amd64 `e6d947aa…`, arm64
  `ea12dce4…` — CI-built hashes, distinct from the local builds as
  expected); amd64 binary executes and prints exactly `selftui v0.1.0`;
  arm64 binary (cross-arch, cannot exec here) embeds the isolated `v0.1.0`
  string + the `selftui %s` format literal per the documented
  exec-or-embedded check; both archives contain exactly `selftui` + LICENSE
  + README.md.

**Commands + exit codes**
- `git tag -a v0.1.0 -m "SelfTUI v0.1.0"` 0 · `git push origin v0.1.0` 0
  (first push, at `1bf98bb`).
- First release run watched: `gh run watch 33884427515 --exit-status` → 1
  (gate step failed; exit 2 in the step, test timeout 600.052s).
- `GOMAXPROCS=2 go test -race -count=200 -timeout 90s … ./internal/agent
  ./internal/ollama` (pre-fix) → agent FAIL 90.030s (repro); post-fix → 0
  (1.686s / 10.143s).
- `GOMAXPROCS=2 go test -race -count=30 -timeout 120s ./internal/agent
  ./internal/ollama` (post-fix) → 0 (2.255s / 67.931s).
- `VERSION=v0.1.0 make release-check` at `1bf98bb` 0, at `e53a931` 0 ·
  `gofmt -l internal/agent internal/ollama` → clean.
- PR #3: `gh pr checks 3 --watch` → green · `gh pr merge 3 --merge` 0 ·
  `git push origin --delete fix/cancel-test-hang` 0.
- Tag move: `git tag -d v0.1.0` 0 · `git push origin :v0.1.0` 0 ·
  `git tag -a v0.1.0 -m "SelfTUI v0.1.0"` 0 · `git push origin v0.1.0` 0.
- Second release run: `gh run watch 33886825863 --exit-status` → 0
  (success) · `ci.yml` run `33886814058` conclusion success.
- Independent verify: `gh release download -R MerverliPy/SelfTUI v0.1.0` 0 ·
  `sha256sum -c` (manifest with `dist/` prefix stripped) → OK ×2 ·
  `./amd/selftui -version` → `selftui v0.1.0` · `strings` checks on arm64 → 0.

**Decisions / lines to respect**
- The release gate failing at a tag is a **blocking release defect**: the
  gate is the release's own binding check, and this class of test-harness
  deadlock (parking handlers + unconsumed request bodies) can flake any
  future PR/CI run. Fixed at the root rather than re-run-and-hope — same
  standard as the F1/F2/F3 fixes in step 4.
- A tag whose release published nothing (failed gate) was **moved**, not
  version-bumped: `v0.1.0` now names the fixed commit `c70bf89`; the product
  tree at the tag is unchanged except test files, and the version stamp
  contract (`selftui v0.1.0`) still holds. Nothing was ever visible outside
  the private repo.
- Release notes for v0.1.0 come from the CHANGELOG `[Unreleased]` section
  (no `[v0.1.0]` section exists yet) — the workflow's documented fallback.
  Cut `[Unreleased]` → `[v0.1.0] - 2026-09-04` at the next release, not
  after the fact (the published notes already carry the content).
- The annotated tag is **unsigned** (no GPG key configured on this host);
  commits in this repo are unsigned too. Revisit if the owner wants
  signed tags/releases.
- Actionlint in the local gate remains a candidate (step-4 note); the
  Node-20 deprecation warning on checkout/setup-go actions persists
  (v0.1.1-era action-major bump).

**Blockers / open decisions**
- None (release published and verified). `SECRET_HISTORY_SCAN=UNRESOLVED`
  persists (gitleaks never installed) — the **next step** before any
  public-visibility change; the repo stays private until it is resolved.

**Next action**
- Fresh session (runbook step 6, if the runbook so orders): **history audit
  (gitleaks)** over the full git history before any public-visibility change,
  then whatever the runbook's public step requires. v0.1.1-era follow-ups
  queued: changelog cut, actionlint in the gate, Node-20 action bumps,
  signed-tag decision.

### 2026-09-04 — v0.1 runbook step 6: gitleaks history audit — SECRET_HISTORY_SCAN resolved (clean) (owner task)
**Milestone:** owner-assigned step (release runbook §6 "history audit before
any public-visibility change" — resolve `SECRET_HISTORY_SCAN=UNRESOLVED`, the
last outstanding release-gate item; the repo stays private until then). No §10
row to tick (owner-assigned step); §12 tail updated. **Result:** done —
**`SECRET_HISTORY_SCAN=RESOLVED`**, audit clean: gitleaks v8.30.1 over the
full reachable history and the working tree found **0 leaks**; unreachable
objects scanned as supplementary evidence (also 0). No remediation needed; no
code or workflow files changed.

**Work done**
- **Installed gitleaks pinned v8.30.1** (official release binary, not
  `go install` — the Go proxy's module path for v8.30.1 declares
  `github.com/zricethezav/gitleaks/v8` while required as
  `github.com/gitleaks/gitleaks/v8`, a constraint conflict). Downloaded
  `gitleaks_8.30.1_linux_x64.tar.gz` + `gitleaks_8.30.1_checksums.txt` from
  the v8.30.1 GitHub release, verified the checksum (sha256
  `551f6fc8…`, OK), extracted to `~/go/bin/gitleaks` (on PATH; same
  GOPATH convention as govulncheck v1.7.0). `gitleaks version` → `8.30.1`.
- **Full reachable-history scan (the gate):** `gitleaks git --log-opts="--all
  --full-history" --redact` with a JSON report → **45 commits scanned,
  ~962 KB, no leaks found** (exit 0). Scope covered every ref:
  `refs/heads/main`, `refs/heads/hardening/v0.1`, both `origin/*` refs, and
  tag `v0.1.0`. 49 reachable commits = 45 scanned + 4 merge commits (merges
  carry no independent diff; their content was scanned in the source
  commits). `--redact` kept any potential match out of stdout.
- **Working-tree scan:** `gitleaks detect --source . --redact` → **no leaks
  found** (exit 0).
- **Supplementary completeness — unreachable objects** (never transmitted by
  a push, scanned anyway as evidence): `git fsck --unreachable --no-reflogs`
  lists 3 blobs / 4 commits / 10 trees / 1 tag — the 4 commits are two
  pre-merge duplicates of the phase-8 CI commit (`a367327d`, `9f5f2cde`), the
  two `git stash` entries from the command-execution removal
  (`6309bcec` WIP, `5b9f5f97` index on `hardening/v0.1`), and the tag is the
  superseded pre-move `v0.1.0` object at `1bf98bb` (its target content is in
  main's history and was scanned there). Extracted each snapshot
  (`git archive`) + the 3 orphan blobs to `/tmp/gl-unreach` and ran
  `gitleaks detect` on each: **no leaks found** (exit 0 ×6). These objects
  are unreachable garbage (tag move + stash) and are not sent on any push;
  `git gc` will collect them — no action taken.
- Cross-checked against the earlier manual `git log --all -p` high-signal
  pattern scan (step-4 entry, 0 matches): two independent methods agree.
  Default gitleaks ruleset used (no custom `.gitleaks.toml`); the repo has
  no pre-existing gitleaks config, Makefile target, or CI job.

**Commands + exit codes**
- `go install github.com/gitleaks/gitleaks/v8@v8.30.1` → 1 (module-path
  mismatch, see above) · `gh release download v8.30.1 -R gitleaks/gitleaks
  -p 'gitleaks_8.30.1_linux_x64.tar.gz' -p 'gitleaks_8.30.1_checksums.txt'` 0
  · `sha256sum -c` (linux_x64 line) → OK · `gitleaks version` → `8.30.1` ·
  `install -m 0755 gitleaks ~/go/bin/gitleaks` 0.
- `gitleaks git --log-opts="--all --full-history" --redact --report-format
  json --report-path /tmp/gl-history.json .` → 0 (45 commits, 0 leaks) ·
  `gitleaks detect --source . --redact …` → 0 (0 leaks) · unreachable
  snapshots ×4 + orphan blobs `gitleaks detect` → 0 each (0 leaks).
- `git fsck --unreachable --no-reflogs` → 0 (3 blob/4 commit/10 tree/1 tag) ·
  `git rev-list --all | wc -l` → 49 · `git log --all --merges --oneline | wc
  -l` → 4 (45 + 4 = 49 ✓).

**Decisions / lines to respect**
- **`SECRET_HISTORY_SCAN=RESOLVED`** — the release-gate item is closed on
  the evidence above (gitleaks v8.30.1, full history + tree + unreachable
  extras, all 0). The **public-visibility decision itself is the owner's**
  next call; nothing in this audit blocks or forces it, and the repo remains
  private until the owner acts.
- gitleaks pinned at **v8.30.1** (release binary, checksum-verified) — record
  the same version in any future CI/pre-commit integration for consistency.
- No code/workflow changes this session: gitleaks integration (CI job or
  pre-commit hook) was **not** added — scope discipline. Recommended as a
  v0.1.1-era item so the secret baseline cannot regress before the repo goes
  public (see open decisions).

**Blockers / open decisions**
- None (audit clean and resolved).
- **Recommended follow-up (owner):** add a recurring gitleaks check (CI job on
  `ci.yml` — e.g. `gitleaks/gitleaks-action` at v8.30.1 — or a `make` target
  in the local gate) so future commits can't introduce secrets between this
  audit and the public-visibility change. Queued with the other v0.1.1-era
  hygiene items rather than built here.

**Next action**
- Owner (fresh session): the **public-visibility step** the runbook orders
  next (make the repo public), now unblocked by the audit, then the
  v0.1.1-era queue: changelog cut (`[Unreleased]` → `[v0.1.0] - 2026-09-04`),
  gitleaks-in-CI (recommended above), actionlint in the local gate, Node-20
  action bumps, signed-tag decision.
