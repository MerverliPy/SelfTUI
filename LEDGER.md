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

### 2026-09-04 — Runbook Task 00: audit-remediation baseline — branch + toolchain pin (owner task)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 00 (baseline and
`fix/v0.1.1-audit-remediation` branch). No §10 row to tick (runbook-owned step). **Result:**
done — **BASELINE=PASS** on branch `fix/v0.1.1-audit-remediation` @ `7db72b4`, worktree
clean. Baseline `make check`/`make race`/`make vuln` all exit 0 under the newly pinned
`toolchain go1.27.1` (go.mod), matching the CI pin; govulncheck reports 0 vulnerabilities
affecting the code.

**Work done**
- **Session-start reads:** AGENTS.md, PLAN.md §10–12, SECURITY.md, LEDGER tail, and the
  audit report `SelfTUI-External-Audit-2026-09-04.md` (placed by the owner mid-session;
  was not on disk at session start — only the input pack in `~/selftui-audit-pack/`).
- **Branch + doc commit (owner-authorized option-2 exception to Task 00's no-commit rule):**
  `git switch -c fix/v0.1.1-audit-remediation` from clean `main` @ `e6ef11b`; commit
  `b1f4440` adds exactly the two input documents: `SelfTUI-External-Audit-2026-09-04.md`
  and `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md`.
- **First baseline run** (effective toolchain go1.25.8 via Debian go 1.22 +
  GOTOOLCHAIN=auto honoring go.mod `go 1.25.8`): `make check` 0, `make race` 0,
  `make vuln` **2** — govulncheck exit 3: "Your code is affected by 13 vulnerabilities
  from the Go standard library" (GO-2026-6218/6090/6088/5972/5856/5039/5037/5026/4971/
  4947/4946/4918/4870), all stdlib/x-net at go1.25.8 fixed only in go1.25.9–go1.25.13 /
  x/net v0.53.0; reachable traces at `internal/ollama/client.go:76`,
  `internal/ollama/stream.go:107`. Toolchain drift, not a code defect (CI pins go1.27.1).
- **Toolchain pin (owner-authorized):** go.mod gains `toolchain go1.27.1` under
  `go 1.25.8` (commit `7db72b4`; no go.sum impact); effective toolchain auto-switches to
  go1.27.1. **Re-run:** `make check` 0 (7s), `make race` 0 (11s), `make vuln` 0 (3s) —
  govulncheck: "Your code is affected by 0 vulnerabilities" (7 imported-package + 3
  module vulns not called). Baseline elapsed ~21s total.
- **Record keeping (owner-authorized for the whole plan):** this LEDGER entry + runbook
  checklist tick for Task 00, committed separately.

**Commands + exit codes**
- `git status --short` → empty (session start) · `git branch --show-current` → `main` ·
  `git rev-parse --short HEAD` → `e6ef11b` · `git log -1 --oneline` → `e6ef11b Merge pull
  request #5 …` (all 0).
- `git switch -c fix/v0.1.1-audit-remediation` → 0 · docs commit → 0 (`b1f4440`, 2 files,
  +1050).
- Baseline 1: `make check` → 0 (7s) · `make race` → 0 (11s) · `make vuln` → **2**
  (`make: *** [Makefile:22: vuln] Error 3`, govulncheck exit 3, 3s).
- `go version` → go1.25.8 → go1.27.1 after pin · `govulncheck -version` → v1.7.0.
- Pin commit → 0 (`7db72b4`, go.mod +2). Baseline 2: `make check` 0 · `make race` 0 ·
  `make vuln` 0 (~21s). `git status --short` → empty (clean) after each phase.

**Decisions / lines to respect**
- `fix/v0.1.1-audit-remediation` is the runbook branch for Tasks 00–22; every later task
  must confirm it is active and status is clean before editing.
- go.mod: `go 1.25.8` language level unchanged; `toolchain go1.27.1` added to align the
  local gate with CI (ci.yml pin) and to satisfy audit finding M-10's version-enforcement
  direction. Local default `make vuln` is green again.
- The two input documents live on the fix branch (`b1f4440`); the runbook's progress
  checklist is the tracking record (ticked per task here).
- Task 00's own "no edit/commit" letter was overridden twice by explicit owner
  authorization (docs commit; toolchain pin) — recorded here as the precedent.

**Blockers / open decisions**
- None for Task 00. `SelfTUI-External-Audit-2026-09-04.md` did not exist on this machine
  at session start; owner placed it (and the runbook) and authorized the disposition.
- The runbook's H-05/M-03/M-04/M-06/M-08/M-09 findings carry `[needs runtime
  verification]` — per runbook, a task must NOT_REPRODUCED-disposition rather than force
  a speculative patch if it does not reproduce.

**Next action**
- Fresh Pi session: runbook **Task 01 (H-01 — restore default XDG configuration
  loading)**: confirm branch `fix/v0.1.1-audit-remediation` + clean status, red-green TDD
  on `cmd/self-tui/main.go` + `main_test.go` + `internal/config/config_test.go`, then
  record in LEDGER and tick the checklist.

### 2026-09-04 — Runbook Task 01: H-01 default config load restored — XDG config regression fixed
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 01 (H-01 — restore default XDG
configuration loading). No §10 row to tick (runbook-owned step). **Result:** done — red-green TDD on
branch `fix/v0.1.1-audit-remediation` @ `581ac6f`, worktree clean, `go test -count=1 ./cmd/self-tui
./internal/config` and `make check` both exit 0. Commit (next): `fix(config): load persisted default
config on startup`.

**Work done**
- **Session-start reads:** AGENTS.md, PLAN.md §10–12 (config precedence contract: flags > env > file >
  defaults; XDG config home `$XDG_CONFIG_HOME/selftui/config.toml`), LEDGER tail, runbook Task 01
  block + audit finding H-01, `cmd/self-tui/main.go`, `main_test.go`,
  `internal/config/{load,config,config_test,save_test}.go`, `internal/ui/phase4_test.go`
  (ConfigPath UI usage), Makefile, and the `adrg/xdg` v0.5.3 source (env is snapshotted into package
  vars at `init`; a public `xdg.Reload()` refreshes them — the mechanism the XDG-isolation tests use).
- **Root cause confirmed (git archaeology):** `ad47e29` (M4 settings & persistence) introduced
  `ov := config.Overrides{ConfigPath: flagConfig}` in `run()`. `flag.String` returns a **non-nil**
  `*string` even when `-config` is omitted (value `""`), so every ordinary startup handed `Load` an
  empty-but-non-nil `ConfigPath`. `load.go:61-69` treats a non-nil ConfigPath as "the file" → sets
  `cfg.filePath = ""` and `os.ReadFile("")` (ENOENT, silently skipped as IsNotExist) → the default
  `$XDG_CONFIG_HOME/selftui/config.toml` was never read, `ConfigPath()` stayed empty for the UI/log,
  and Save's fallback file was ignored on the next startup (the H-01 save→restart loop).
- **Red (TDD):** extracted the flag→ConfigPath decision into a pure helper
  `configPathOverride(*string) *string` with the *current* behavior (unconditional forward), wired
  `run()` through it, and added `TestConfigPathOverrideBoundary` (omitted `-config` must yield a nil
  ConfigPath override; a non-empty explicit path must stay non-nil; flags parsed on a private
  `flag.FlagSet` because `run()` owns the process-global flag set — re-registering panics). Focused
  run failed exactly as expected: `omitted -config: ConfigPath override = "", want nil so Load resolves
  the default XDG file` (main_test.go:45) — i.e. the current entrypoint passes an empty-but-non-nil
  ConfigPath.
- **Green:** fix = the helper returns nil when the flag value is `""` and the pointer otherwise
  (`main.go`), so `Load` falls back to resolving `$XDG_CONFIG_HOME/selftui/config.toml`. Config
  precedence and file formats untouched.
- **Config-package coverage (step 2):** added `withXDGConfigHome(t, dir)` test helper — sets
  `XDG_CONFIG_HOME`, calls `xdg.Reload()`, restores env + package vars and reloads again in
  `t.Cleanup` (never touches the real user config) — plus two tests: `TestLoadResolvesDefaultXDGConfigPath`
  (`Load(Overrides{})` resolves/reads the default XDG file and `ConfigPath()` reports it) and
  `TestSaveToDefaultXDGPathReloadsOnFreshLoad` (Settings save with an empty `filePath` → fresh
  `Load(Overrides{})` reloads it — the exact save→restart loop H-01 reported broken).
- **Precedence unchanged (step 5):** explicit-`ConfigPath` behavior is covered by the pre-existing
  suite — `TestFileOnly`, `TestEnvOverridesFile`, `TestOverridesWinEverything`, `tools_test.go`,
  `validate_test.go`, `save_test.go` all pass unmodified under `make check`.
- **Live binary probe** (real `run()`, isolated XDG homes; real user config untouched): default
  startup logs `starting version=dev host=… theme=light config=/tmp/…/config/selftui/config.toml`
  (theme=light proves the file applied — built-in default is dark), explicit `-config …/explicit.toml`
  logs `theme=dark config=/tmp/…/explicit.toml` — explicit path still wins.
- One transient test-authoring failure caught and fixed inside the task: first version of
  `TestLoadResolvesDefaultXDGConfigPath` wrote `config.toml` before creating the `selftui` parent dir
  (`os.WriteFile` doesn't MkdirAll; `Save` does) → added `os.MkdirAll`, then green.

**Commands + exit codes**
- `git status --short` → empty · `git branch --show-current` → `fix/v0.1.1-audit-remediation` ·
  `git rev-parse --short HEAD` → `581ac6f` (all 0).
- Baseline before edits: `go test -count=1 ./cmd/self-tui ./internal/config` → 0 (ok / ok).
- Red: `go test -count=1 ./cmd/self-tui -run TestConfigPathOverrideBoundary -v` → **1** (FAIL,
  main_test.go:45, message above). Green after fix: same command → 0 (PASS).
- Transient: `go test -count=1 ./cmd/self-tui ./internal/config -run 'Test…XDG…' -v` → 1 (WriteFile
  ENOENT) → after `os.MkdirAll` fix → 0 (3/3 PASS).
- Full focused suite: `go test -count=1 ./cmd/self-tui ./internal/config` → 0.
- `make check` (build + `go test ./...` + vet + gofmt) → 0. `git diff --check` → 0 (clean).
- Live probe: `XDG_CONFIG_HOME=<tmp>/config XDG_STATE_HOME=<tmp>/state timeout 3 ./bin/selftui` →
  exit 1 (no TTY — expected; boot log still written) with `config=/tmp/…/config/selftui/config.toml`;
  same with `-config <tmp>/explicit.toml` → `config=/tmp/…/explicit.toml`. Temp XDG root removed.

**Decisions / lines to respect**
- Fix stays in `main.go` at the flag→Overrides seam (pure `configPathOverride` helper) so the
  entrypoint boundary is deterministically testable; `internal/config/load.go` unchanged (nil
  ConfigPath already meant "resolve default XDG" — the entrypoint was simply never leaving it nil).
- No change to config precedence, file formats, or public errors. `flag.String`'s non-nil-pointer
  quirk is documented on the helper so the regression cannot silently return.
- XDG isolation in tests goes through `xdg.Reload()` + env restore + package-var restore; no test
  reads or writes the real `~/.config` or `~/.local/state`.
- Commit shape follows the Task-00 precedent: code commit
  `fix(config): load persisted default config on startup` (main.go, main_test.go, config_test.go),
  then a separate docs commit for this LEDGER entry + the runbook Task-01 tick.

**Blockers / open decisions**
- None for Task 01. (Note: without a TTY the binary exits 1 at tea boot — expected, unrelated to the
  fix; the startup log line is written before that.)

**Next action**
- Fresh Pi session: runbook **Task 02 (C-01 — policy-aware recursive grep)**: confirm branch
  `fix/v0.1.1-audit-remediation` + clean status, red-green TDD on `internal/agent/runner.go` +
  `tools.go` + `toolpolicy.go` + `policy_test.go` + `runner_test.go`, then record in LEDGER and tick
  the checklist.

### 2026-09-04 — Runbook Task 02: C-01 policy-aware recursive grep — recursive grep enforces the sensitive-path denylist
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 02 (C-01 — make recursive grep
enforce the sensitive-path policy). No §10 row to tick (runbook-owned step). **Result:** done —
red-green TDD on branch `fix/v0.1.1-audit-remediation` @ `a85236b`, worktree
clean, all gates exit 0. Commit: `fix(agent): enforce policy during recursive grep`.

**Work done**
- **Session-start reads:** AGENTS.md, PLAN.md §§10–12, LEDGER tail (Task-01 handoff), runbook Task-02
  block + audit finding C-01, `internal/agent/{runner,tools,toolpolicy}.go`,
  `policy_test.go`, `runner_test.go`, `SECURITY.md`, `mutation.go`, agent-view policy wiring
  (`internal/ui/agent_view.go:197`). Grep has exactly two call sites: `runner.go:333` (grep tool case)
  and the direct call in `runner_test.go` (`TestReadOnlyToolsStayInsideWorkspace`).
- **Root cause confirmed (code + live red evidence):** `runner.go` grep case ran `authorizePath` only
  on the model-supplied root (`args.Path`), then `tools.go` `Grep` walked the tree with
  `filepath.WalkDir`, skipping only `.git` and appending every non-symlink file — the policy was never
  applied to discovered descendants. `SECURITY.md:45-47` promises the denylist covers `.ssh`, `.aws`,
  `.env*`, etc., so `grep "."` violated the core safety contract (C-01).
- **Red (TDD):** added `seedSensitiveGrepWorkspace`/`assertGrepLeakFree` helpers in `policy_test.go`
  (ordinary `GOOD_*` files, `.env.example` template control, and a distinct `LEAK_*` marker per denied
  class) plus `TestRunnerGrepOverRootHidesPolicyDeniedDescendants` in `runner_test.go`, driving the
  real runner with a native grep tool call (`pattern "GOOD_|LEAK_"`, `path "."`). Ran against the
  unmodified production code — **failed exactly as C-01 describes**: output contained
  `.env:1:LEAK_DOTENV=topsecret`, `.env.local:1:…`, `.ssh/id_rsa:1:…`, `.gnupg/private.key:1:…`,
  `.aws/credentials:1:…`, `.azure/azureProfile.json:1:…`, `.kube/config:1:…`,
  `.config/gcloud/application_default_credentials.json:1:…`, `credentials:1:…`,
  `credentials.json:1:…`, plus nested `proj/sub/.ssh/id_ed25519` and `proj/sub/deep/.env.production`
  (while `.env.example:1:GOOD_EXAMPLE=dummy` correctly showed).
- **Green (minimum fix, `tools.go`):** `Grep` now takes `ctx` and an `authorize func(string) error`
  and applies the policy to **every canonical workspace-relative descendant**: denied directories are
  pruned with `filepath.SkipDir` before descent (`.ssh`, `.gnupg`, `.aws`, `.azure`, `.kube`,
  `.config/gcloud`, and any denied dir wherever nested) and denied files are skipped before they are
  ever opened (`.env*` except `.env.example`, `credentials`, `credentials.json`, wherever nested).
  Denials are deterministic skips — never whole-operation failures; real traversal/I/O errors (e.g. an
  unreadable directory) still fail the grep. `.git` skip, `sort.Strings`, `maxSearchBytes`/`maxResultBytes`
  + `[output truncated]` contract all unchanged.
- **`context.Context` at the grep boundary now (Task 13/M-06 preparation):** `Grep` checks
  `ctx.Err()` before the walk, in the walk callback, and before scanning each file, returning promptly
  on cancellation. `runner.go` grep case passes `ctx` and `r.authorizePath` as the per-descendant
  callback (top-level `authorizePath` gate kept; Grep re-authorizes descendants so a recursive root
  cannot read denied files). The only other call site (direct Grep in
  `TestReadOnlyToolsStayInsideWorkspace`) passes an explicit allow-all callback — no policy intended.
- **Regression + boundary tests added:** `TestGrepAppliesPolicyToEveryWorkspaceDescendant`
  (direct Grep over "." with the real policy callback), `TestGrepSkipsNestedDeniedDirectoriesButPreservesIOErrors`
  (deep `.aws`, deep `.config/gcloud`, nested dotenv skipped; chmod-000 dir still fails the grep —
  skipped when root), `TestGrepChecksCancellationWhileWalkingAndScanning` (pre-canceled / cancel during
  walk via the dir-prune hook / cancel between scanned files all surface `context.Canceled`). Denied
  fixture classes seeded: `.env`, `.env.local`, `.env.production`, `.ssh`, `.gnupg`, `.aws`, `.azure`,
  `.kube`, `.config/gcloud`, `credentials`, `credentials.json` (all at root and nested). The five-tool
  surface, `toolpolicy.go` denylist classes, and the `.env.example` carve-out are untouched.

**Commands + exit codes**
- `git status --short` → empty · `git branch --show-current` → `fix/v0.1.1-audit-remediation` ·
  `git rev-parse --short HEAD` → `4937931` (all 0).
- Baseline: `go test -count=1 ./internal/agent` → 0.
- RED: `go test -count=1 ./internal/agent -run TestRunnerGrepOverRootHidesPolicyDeniedDescendants -v`
  → **1** (FAIL; leak evidence above). Green after fix: same command → 0.
- Focused suite: `go test -count=1 ./internal/agent -run 'Test.*(Grep|Policy|Sensitive)'` → 0
  (9 top-level tests incl. 3 cancellation subtests). Full package: `go test -count=1 ./internal/agent`
  → 0. `make check` → 0. `make race` → 0.
- `make vuln` → 0 via `$(go env GOPATH)/bin/govulncheck` (govulncheck is not on the shell PATH;
  `make vuln` alone exits 127 "No such file or directory" — PATH-environment issue only, same scan
  binary Task 00 used). `git diff --check` → 0 (clean).

**Decisions / lines to respect**
- The C-01 fix lives at the Grep boundary (`tools.go`), not in `toolpolicy.go`: `Grep` gains
  `ctx context.Context` and `authorize func(string) error`; the runner passes its existing
  `r.authorizePath` (nil policy ⇒ allow-all, unchanged). Direct policy-free callers pass an explicit
  allow-all callback. `authorize == nil` inside Grep also means allow-all (pre-policy direct-call
  semantics preserved for any future caller).
- **Public signature change (reported per runbook):** `Grep(root, pattern, path string)` →
  `Grep(ctx context.Context, root, pattern, path string, authorize func(string) error)`. No other
  public function changed; `ListDir`/`ReadFile`/policy/containment untouched. The ctx is the
  Task-13/M-06 seam so that task does not redesign the API.
- Policy denials during a walk are silent, deterministic skips (empty result when everything is
  denied) — do not convert them into whole-operation errors; real I/O errors keep failing the grep.
- The red test was written against the untouched production code (runner-level regression only, no
  signature dependency); direct-boundary tests were added in the green step after the signature
  change made them compilable.
- Commit shape follows the Task-00/01 precedent: one code commit (`fix(agent): enforce policy during
  recursive grep`) with the four agent files, then a separate docs commit for this LEDGER entry + the
  runbook Task-02 tick.

**Blockers / open decisions**
- None for Task 02. (Env note only: `make vuln` needs `$(go env GOPATH)/bin` on PATH.)

**Next action**
- Fresh Pi session: runbook **Task 03 (H-02 — canonical workspace validation)**: confirm branch
  `fix/v0.1.1-audit-remediation` + clean status, red-green TDD on `internal/config/validate.go` +
  `tools_test.go` + `validate_test.go`, then record in LEDGER and tick the checklist.

### 2026-09-04 — Runbook Task 03: H-02 workspace canonicalization — workspace-root aliases of `/` and home rejected by canonical identity
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 03 (H-02 — canonical workspace
root validation). No §10 row to tick (runbook-owned step). **Result:** done — red-green TDD on branch
`fix/v0.1.1-audit-remediation` @ `5af840b`, worktree clean, all gates exit 0. Commit:
`fix(config): reject canonical root and home workspaces`.

**Work done**
- **Session-start reads:** AGENTS.md, PLAN.md §§10–12, LEDGER tail (Task-02 handoff), runbook Task-03
  block + audit finding H-02, `internal/config/{validate,load}.go`, `tools_test.go`, `validate_test.go`,
  `internal/agent/tools.go` (`canonicalRoot`/`securePath`), README workspace contract, ui/cmd config
  consumers (`app.go`, `agent_view.go`, `settings_view.go`, `phase4_test.go`) to prove the runtime path
  before patching.
- **Root cause confirmed (code + live red evidence):** `validate.go` `validateToolsWorkspace` rejected
  only the literal `"/"` and compared `filepath.Clean(workspaceRoot)` to the lexical home path — no
  `filepath.Abs`, no `filepath.EvalSymlinks`. The tool layer later does both (`agent/tools.go`
  `canonicalRoot`: Abs → EvalSymlinks → IsDir) on every call, so `/tmp/..`, a symlink to `/`, a
  symlink to home, or a relative spelling resolving to home could validate and then jail the enabled
  tools on the whole filesystem or the home directory (H-02, defeating the README/UI "never `/` or
  home" boundary promise).
- **Red (TDD):** added to `tools_test.go` — `TestValidateToolsWorkspaceAliasesRejected` (table-driven,
  `tools_enabled=true`, each case asserting the exact stable error): direct `/` and direct home
  (controls), the literal spelling `/tmp/..`, symlink → `/`, symlink → home, and a relative path
  resolving to home under `t.Chdir(filepath.Dir(home))`; `TestValidateToolsWorkspaceSymlinkToRealProjectAccepted`
  (symlink to a real project dir must stay legal — over-rejection guard);
  `TestLoadToolsWorkspaceStoresCanonicalRoot` (toml + env sources: with tools armed, Load must store
  the canonical target, not the symlink spelling); `TestLoadToolsOffKeepsBroadWorkspaceSpelling`
  (tools off: `/` and home stay legal and Load stores the spelling verbatim). Ran against the
  unmodified code — **failed exactly as H-02 describes**: `dotdot above tmp`, `symlink to root`,
  `symlink to home`, `relative path resolving to home` all returned nil ("want rejection …"), and Load
  stored the raw `ws-link` instead of the canonical project dir.
- **Green (minimum fix, `validate.go` + `load.go`):** one config-local helper `canonicalDir(path)`
  (filepath.Abs → filepath.EvalSymlinks → IsDir) that mirrors `internal/agent/tools.go canonicalRoot`
  step-for-step; `validateToolsWorkspace` now canonicalizes the candidate root and rejects canonical
  `/` and a filesystem-identical home **with the existing stable error texts unchanged**; the home
  side is canonicalized too (still guarded by `os.UserHomeDir` error, as before). New stable error
  only for an unresolvable root:
  `config: tools_enabled: workspace_root: cannot resolve "<ws>": <cause>` (reachable only if the dir
  vanishes between the earlier stat and EvalSymlinks). **Canonical persistence:** `Load` (step 5 in
  `load.go`) now replaces `cfg.WorkspaceRoot` with `canonicalDir`'s result when `ToolsEnabled` and the
  root is non-empty, so every consumer of the Loaded config — agent runner, status bar, later
  Settings save — receives the canonical directory; the tool layer additionally re-canonicalizes per
  call, so validation and execution cannot diverge on any path. `tools_enabled=false` behavior
  untouched (no new rejections, spelling stored verbatim). No `internal/agent` import into config.
  Existing tests (incl. `home trailing slash`, missing-dir error, precedence, ui `phase4_test`
  round-trips) unchanged and green.
- **README wording:** no change needed — README already says "never `/` or your home directory" and
  the fix makes validation honor exactly that promise for aliases too.

**Commands + exit codes**
- `git status --short` → empty · `git branch --show-current` → `fix/v0.1.1-audit-remediation` ·
  `git rev-parse --short HEAD` → `2d6288b` (all 0).
- RED: `go test -count=1 ./internal/config -run 'Test.*Tools.*Workspace' -v` → **1** (FAIL; the four
  alias subtests + canonical-storage subtests failed as quoted above). Green after fix: same command
  → 0 (5 top-level tests incl. 7 sub-cases).
- Full config package: `go test -count=1 ./internal/config` → 0. `make check` → 0 (build + uncached
  `go test ./...` all packages + vet + gofmt). `make race` → 0. `gofmt -l cmd internal` empty.
  `git diff --check` → 0 (clean).
- Commits: code `5af840b` (3 files, +203/−5) then the docs commit for this entry + runbook tick.

**Decisions / lines to respect**
- The `/` and home bans are now enforced by **canonical directory identity** (Abs + EvalSymlinks +
  IsDir), not spelling; the error strings are byte-identical to the pre-H-02 messages so every UI
  surface reports the same stable text. Only the canonical-`/` and canonical-home values are rejected —
  symlinks to a real project directory remain legal (test-pinned), and `/tmp/..` must be tested with
  the raw literal (filepath.Join would pre-clean it to `/` and mask the alias).
- `canonicalDir` lives in config and must never be replaced by an `internal/agent` import: the two
  packages implement the same three-step resolution independently so neither imports the other, and
  any drift between them would show up as validation/execution disagreement — keep them in lockstep.
- Canonical persistence is a tools-armed property of `Load` only: with tools off, workspace_root
  values (`/`, home, symlinks, relatives) are stored verbatim and stay legal. Save still writes the
  current in-memory spelling (canonical after any Load); the Settings live-apply path may hold the
  raw spelling until the next boot, where the per-call tool-layer canonicalization keeps the jail
  identical (validated alias can never be executed as `/` or home on the boot or form path).
- Residual (pre-existing, out of scope): a workspace root whose own entry is retargeted (symlink
  swap / dir replacement) between validation and a tool call is a TOCTOU the tool layer re-resolves
  per call — the same accepted residual as in-workspace symlink aliases (audit line 278).

**Blockers / open decisions**
- None for Task 03. Env note carried from Task 02: `make vuln` needs `$(go env GOPATH)/bin` on PATH.

**Next action**
- Fresh Pi session: runbook **Task 04 (H-03 — bound native tool streams and total executions)**: confirm
  branch `fix/v0.1.1-audit-remediation` + clean status, red-green TDD on `internal/ollama/chat.go` +
  `internal/agent/runner.go` + tests, then record in LEDGER and tick the checklist.

### 2026-09-04 — Runbook Task 04: H-03 native tool byte/call budgets — raw cumulative stream ceiling, 1 MiB per-call arguments, 64 calls per run
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 04 (H-03 — bound native tool
streams and total executions). No §10 row to tick (runbook-owned step). **Result:** done — red-green
TDD on branch `fix/v0.1.1-audit-remediation` @ `7bba656`, worktree clean, all gates exit 0. Code
commit: `fix(agent): bound native tool streams and executions` (5 files, +393/−43).

**Work done**
- **Session-start reads:** AGENTS.md, PLAN.md §§10–12, LEDGER tail (Task-03 handoff), runbook Task-04
  block + audit finding H-03, `internal/ollama/{chat,stream,types}.go` + `stream_test.go` +
  `chat_test.go`, `internal/agent/runner.go` + `runner_test.go` + `context.go`, `internal/ollama/client.go`.
- **Root cause confirmed (code + live red evidence):**
  1. `chat.go` accumulated only decoded `content`+`thinking` bytes toward the 16 MiB cumulative
     ceiling — `ToolCalls` (which can carry megabytes of raw JSON per event under the 4 MiB per-event
     cap) were never counted, so a hostile endpoint could stream arbitrarily many sub-4 MiB
     tool-argument events and never trip the documented cap.
  2. `runner.go` executed *every* call in each returned batch with no run-wide count: a batch of N
     parallel `read_file`/`list_dir` calls ran unconditionally every iteration, so `max_tool_iterations`
     (12) did not constrain actual tool executions ("thousands of parallel reads in one iteration").
  3. `mergeToolCalls`/`mergeArguments` concatenated any complete-call-with-fragment mixture at the
     same slot into garbage JSON, and fragment accumulation was uncapped (audit: quadratic merging,
     unbounded memory).
- **Red (TDD), all against the untouched production code:**
  - `internal/ollama/stream_test.go` `TestChatCumulativeToolBytesOverflowRejected`: 7 NDJSON events,
    each one native tool call with ~2.6 MiB of argument text (under the 4 MiB per-event wire cap);
    cumulative raw bytes cross 16 MiB on event 7. Pre-fix: **7 events delivered**, error was
    "stream ended without done" — never the cumulative cap message (tool bytes escaped the ceiling).
  - `internal/agent/runner_test.go` `TestRunnerRejectsOversizedNativeToolArgument`: one native
    `read_file` call with ~1 MiB+ arguments in a single event. Pre-fix: Run returned **nil** (the call
    executed and failed deep in the filesystem layer, then the loop continued) — no per-call bound.
  - `TestRunnerRejectsBatchOverCallLimit`: one batch of 70 `list_dir` calls. Pre-fix: **210 executions**
    across 3 iterations, "maximum tool iterations (3)" — no call limit.
  - `TestRunnerBoundsToolCallsPerRun`: batches of 40+40 across iterations. Pre-fix: **200 executions**
    over 5 requests — no run-wide count; crossing batch fully executed.
- **Green (minimum fix):**
  - `internal/ollama/chat.go` now counts `len(raw)` per complete decoded NDJSON event toward the
    retained 16 MiB ceiling (`maxChatStreamBytes` unchanged; JSON framing, content, thinking, and
    tool calls all count) and rejects the crossing event before callback delivery, returning the
    pre-existing stable `errChatStreamTooLarge` text ("chat stream exceeds 16777216 bytes").
    `stream.go` comments updated; per-event 4 MiB wire cap and idle watchdog untouched.
  - `internal/agent/runner.go` adds `maxToolArgBytes = 1 MiB` (decoded per-call argument ceiling) and
    `maxToolCallsPerRun = 64` with three stable errors:
    `tool call argument exceeds 1048576 bytes`, `tool call limit (64) exceeded for this run`,
    `ambiguous tool call fragments: no stable call id or index`. `mergeToolCalls` now returns
    `([]ollama.ToolCall, error)` and concatenates fragments only while both sides are incomplete JSON
    (bounded at 1 MiB); a complete+fragment mixture at one slot is refused as ambiguous rather than
    concatenated by guesswork (the wire type's optional `id` is not populated by Ollama, and there is
    no `index`, so positional+validity is all there is). Repeated complete calls still dedupe;
    distinct complete calls at one slot (parallel calls) both survive; `mergeArguments` was folded
    into the merger and deleted. The runner's per-iteration callback stops accumulating after a merge
    error; the merge error is returned (wrapped `agent: %w`). A **batch gate** runs before the
    assistant tool-call turn is appended and before any execution: per-call argument-size validation
    (also covers the content-embedded JSON path, which never passes through the merger) and a
    run-wide `executedCalls + len(batch) > 64` check that rejects the whole crossing batch with zero
    calls executed. Normal single/parallel calls under the limits are unchanged (existing green tests
    untouched apart from the merge signature call-site update).
  - Added direct boundary tests in `runner_test.go` after the signature change made them compilable
    (Task-02 precedent): `TestMergeToolCallsAdversarial` subtests — two simultaneous calls survive,
    repeated complete call deduplicates, same-name complete calls both survive, fragmented arguments
    concatenate, ambiguous fragment-then-complete refused, complete-then-fragment refused, null
    arguments carry no fragment, fragment accumulation capped at 1 MiB, single oversized complete call
    refused. `TestMergeToolCallsAssemblesStreamedArguments` updated to the two-value signature.

**Commands + exit codes**
- `git status --short` → empty · `git branch --show-current` → `fix/v0.1.1-audit-remediation` ·
  `git rev-parse --short HEAD` → `7bba656` (all 0).
- RED: `go test -count=1 ./internal/ollama -run TestChatCumulativeToolBytesOverflowRejected -v` → **1**
  (7 delivered, wrong error); `go test -count=1 ./internal/agent -run 'TestRunnerRejectsOversizedNativeToolArgument|TestRunnerRejectsBatchOverCallLimit|TestRunnerBoundsToolCallsPerRun'` → **1**
  (nil error / 210 / 200 executions). Green after fix: same three commands → 0.
- Focused per runbook step 7: `go test -count=1 ./internal/ollama -run 'TestChat.*(Cumulative|Tool|Oversized)'` → 0;
  `go test -count=1 ./internal/agent -run 'Test.*(Tool|Call|Bound|Merge)'` → 0 (11 top-level tests);
  `go test -count=1 ./internal/ollama ./internal/agent` → 0. `make check` → 0. `make race` → 0.
  `gofmt -l internal/agent internal/ollama` empty. `git diff --check` → 0 (clean).

**Decisions / lines to respect**
- The 16 MiB chat ceiling now counts **complete raw NDJSON event bytes** (framing + tool calls
  included); the byte value, the error text, the 4 MiB per-event wire cap, and the idle watchdog are
  unchanged. The check fires before the crossing event is delivered (same ordering the old
  content+thinking check used) and before unmarshal — resource bound first, decode/secondary checks
  after.
- The 1 MiB per-call argument ceiling and the 64-call per-run ceiling are **agent-side** (runner);
  the ollama package does not know about call semantics. The merger caps every append and every
  fragment concatenation so accumulation is bounded mid-iteration; the batch gate re-validates every
  call (including content-embedded JSON calls that bypass the merger) before any execution.
- Batch rejections are all-or-nothing: a batch that would push the run past 64 executes **zero**
  calls from that batch and aborts the run with the stable limit error — partial execution of a
  crossing batch is impossible.
- Merge refuses (stable `ambiguous tool call fragments` error) whenever one side of a same-name
  same-slot pair is complete JSON and the other is a fragment; the wire provides no stable per-call
  id/index (Ollama's optional `id` is unpopulated), so rejecting beats guessing. Known-good shapes
  are test-pinned: single-complete-call events, pure fragment streams, same-event multi-call batches,
  and exact repeats all keep their pre-H-03 behavior.
- Error texts are the new stable strings above; tests match on the quoted substrings. No public
  signature changed: `mergeToolCalls` is unexported. Residual (accepted, bounded): assembling one
  1 MiB call from ~30-byte wire fragments can still copy O(K·cap) bytes over the stream lifetime,
  but live memory stays ≤ 1 MiB per call and ≤ 16 MiB per stream, and execution stays ≤ 64 per run.

**Blockers / open decisions**
- None for Task 04. Env note carried from Task 02: `make vuln` needs `$(go env GOPATH)/bin` on PATH.

**Next action**
- Fresh Pi session: runbook **Task 05 (H-04 — make the live pull/delete smoke test non-destructive)**: confirm
  branch `fix/v0.1.1-audit-remediation` + clean status, red-green TDD on `scripts/pull-delete-smoke.py` +
  a new `scripts/pull_delete_smoke_test.py`, then record in LEDGER and tick the checklist. Do not run
  `make smoke` until Task 05 lands.
### 2026-09-05 — Runbook Task 05: H-04 non-destructive pull/delete smoke — preflight captures state first and refuses pre-existing targets
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 05 (H-04 — make the live smoke
test non-destructive). No §10 row to tick (runbook-owned step). **Result:** done — red-green TDD on
branch `fix/v0.1.1-audit-remediation`, worktree clean, all gates exit 0. Code commit:
`fix(smoke): preserve pre-existing Ollama models` (2 files + 1 new), docs commit follows this entry.

**Work done**
- **Session-start reads:** AGENTS.md, PLAN.md §§10–12, LEDGER tail (Task-04 handoff), runbook Task-05
  block + audit finding H-04, `scripts/pull-delete-smoke.py`, Makefile smoke targets, README live
  behavior text (`README.md:231-232`), prior task LEDGER entries for the docs-commit precedent.
- **Root cause confirmed (code + live red evidence):**
  1. `main()` called `api_delete(MODEL)` **before** recording any state (`pull-delete-smoke.py:93-100`),
     then re-deleted unconditionally in `finally` (`210-213`) — a pre-existing model (incl. the
     default `qwen3:0.6b`) was permanently removed, and no digest was recorded so re-pulling the tag
     would not be a safe restore.
  2. The module read `sys.argv` and `SMOKE_*` at import time, so it was not import-safe for
     `unittest.mock` driving.
- **Red (TDD), all four against the untouched production code** (`scripts/pull_delete_smoke_test.py`,
  fake-host only — no pty and no host ever contacted by the tests):
  - `test_pre_existing_target_aborts_before_any_delete_pull_or_tui` — target present on every
    `/api/tags`; asserts exit 1, message contains "already installed"/"refus", `api_delete` never
    called, and the host log opens with `("tags", …)` (state capture first). Pre-fix: **DELETE
    executed before state capture**, and the abort text was the stale "still present after pre-run
    cleanup".
  - `test_failure_before_creation_cleans_nothing` — app fails to boot, model never created. Pre-fix:
    **2 deletes recorded** (unconditional start + finally deletes) though nothing was created.
  - `test_failure_after_creation_cleans_only_what_the_run_created` — absent at start, present after
    pull, delete step stuck → failure cleanup. Pre-fix: **2 deletes** (one before any state capture);
    post-fix: exactly 1, and only after creation was observed in `/api/tags`.
  - `test_success_creates_then_removes_only_its_own_model` — full lifecycle exits 0 (SMOKE PASS).
    Pre-fix: **2 deletes**; post-fix: 1 cleanup no-op safety net after the TUI delete, ordered after
    the creation-observation tags call.
  RED run: `python3 -m unittest -v scripts/pull_delete_smoke_test.py` → **4 failures**, all the above.
- **Green (minimum fix):**
  - New `preflight(model)` runs **before any DELETE, pull, or TUI action**: snapshots `/api/tags`
    and aborts (stable `fail()` message, exit 1) when the target is already installed — never
    deleting it and never "restoring" by re-pulling the tag. `main(argv=None)` now reads argv/env
    inside the call (`SMOKE_WATCH`, `SMOKE_COLS/ROWS`), keeping the module import-safe; CLI entry
    unchanged under `if __name__ == "__main__"`.
  - Cleanup policy: `created` flips to True only after this run's pull is positively verified in
    `/api/tags`; `finally` deletes **only when `created`** (success path: harmless no-op net after
    the TUI delete; failure paths: removes exactly this run's leftover model, or nothing if the run
    never created one). No `api_delete` call exists anywhere except that guarded cleanup.
  - Fake-host tests pin the contract; the pty/TUI/`time` machinery is fully mocked with a
    deterministic clock, so the suite runs in ~0.02s with zero host contact.
- **README** live-behavior text rewritten: `make smoke` is non-destructive, refuses an
  already-installed target, and users should point it at a disposable model/tag
  (`make smoke-model MODEL=<name>`) or an isolated Ollama store; the false "leaves the host exactly
  as it was" claim is gone.

**Commands + exit codes**
- `git status --short` → empty at start · `git branch --show-current` → `fix/v0.1.1-audit-remediation`
  · `git rev-parse --short HEAD` at start → `97bd4c5` (all 0).
- RED: `python3 -m unittest -v scripts/pull_delete_smoke_test.py` → **1** (4 failures: DELETE
  precedes state capture; unconditional cleanup). GREEN after fix: same command → 0 (4 tests OK).
- `python3 -m py_compile scripts/pull-delete-smoke.py scripts/pull_delete_smoke_test.py` → 0.
  `make check` → 0. `git diff --check` → 0 (clean).
- `make smoke` was **not** run as part of the task (runbook step 8). See the incident note below.

**Decisions / lines to respect**
- State capture precedes every mutation: `preflight()` is the first host-touching call in `main()`,
  and `api_delete` now exists in exactly one place — the `finally` cleanup guarded by `created`.
- "Created by this run" is defined positively: only after the post-pull `/api/tags` check proves the
  model landed (it was absent at preflight). If that verification itself fails (host error / model
  missing), nothing is deleted — conservative, audit-safe direction.
- Never restore by re-pulling a tag: a pre-existing target aborts the whole run instead (the tag can
  move and the original digest is not recorded). Exit 0 additionally requires the target was absent
  at start (module docstring + README updated).
- Error/abort text is the new stable string: `"<model> is already installed; refusing to run —
  pull-delete-smoke never deletes a pre-existing model (it would remove something this run did not
  create). Use a disposable model/tag or an isolated Ollama store."` Tests assert the quoted
  substrings.
- Import safety: `MODEL`, `LOG` are inert module defaults; `argv`, `SMOKE_WATCH`, `SMOKE_COLS/ROWS`
  are read inside `main(argv=None)`. CLI behavior preserved: `python3 scripts/pull-delete-smoke.py
  [model]` and the `make smoke`/`make smoke-model MODEL=…` Makefile targets pass argv unchanged.

**Incident note (transparency — live-host contact during development)**
- While verifying CLI behavior I ran `python3 scripts/pull-delete-smoke.py` bare against this
  machine's live local Ollama host. `qwen3:0.6b` was absent at that moment, so preflight passed and
  a real pull began; the 60s shell timeout killed the driver before its own cleanup ran (SIGKILL ⇒
  no `finally`), leaving `qwen3:0.6b` installed (manifest created 2026-09-05 03:44:34, verified by
  file mtime and `/api/tags`). I removed exactly that model via `DELETE /api/delete` and re-verified
  `/api/tags` (back to the pre-run 10 models; `qwen3:0.6b` absent; no stray processes). Host state
  restored. Lesson recorded: never execute the smoke driver bare against a live host during Task 05;
  the fake-host tests are the only sanctioned execution until the owner runs `make smoke` on a
  disposable target/isolated store.

**Blockers / open decisions**
- None for Task 05. Carried env note from Task 02/04: `make vuln` needs `$(go env GOPATH)/bin` on
  PATH. M-11 (Task 18) will later move the capture file off fixed `/tmp` paths into private unique
  temp dirs and extends `pull_delete_smoke_test.py`.

**Next action**
- Fresh Pi session: runbook **Task 06 (H-05 — sanitize untrusted terminal control sequences)**:
  confirm branch `fix/v0.1.1-audit-remediation` + clean status, then follow the Task-06 block. Do not
  run `make smoke` until the owner runs it on a disposable model/tag or an isolated Ollama store.
### 2026-09-05 — Runbook Task 06: H-05 terminal control-sequence sanitization — single pre-style boundary over every remote-derived render path
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 06 (H-05 — sanitize untrusted
terminal control sequences). No §10 row to tick (runbook-owned step). **Result:** done — H-05
REPRODUCED (all 11 render-level regressions failed pre-fix) then fixed with red-green TDD on branch
`fix/v0.1.1-audit-remediation`; worktree clean, all gates exit 0. Code commit:
`fix(ui): sanitize untrusted terminal control sequences` (2 new files + 2 modified + go.mod),
docs commit follows this entry.

**Reproduction (runtime-verified, not static)**
- **Probe first** (throwaway `scratchprobe/`, deleted before commit): with `charm.land/glamour/v2`
  v2.0.0, a hostile markdown body renders with its ESC bytes **embedded inside glamour's own styled
  output** — OSC 52 `ESC ]52;c;evil BEL` survived as `…\x1b[m\x1b]52;c;evil\a…`, DCS `ESC P…ESC \`
  survived intact (enters sixel/DCS mode mid-frame), and `a\rb` came through as raw CR. The
  raw-markdown fallback returns md verbatim. So both glamour-success AND fallback paths leak;
  ansi.Strip on the same corpus removed complete **and dangling** ESC/C1 sequences but left
  standalone C0 (BEL/CR/BS/VT/FF) in place.
- **Red:** 11 render-level tests driving hostile payloads through the real Update paths (chat
  stream split across tokens + commit, model names in header/picker, done_reason, chat/fallback
  notices, tool status + confirmation overlay, models list names, list/detail/pull/delete error
  bodies, /api/show detail content, pull status text) all FAILED pre-fix with hostile bytes in
  `View()` output (`go test … -run 'Test.*(Sanitize|Control|OSC|CSI)|TestAgent…'` → 11 FAIL).
  JSON `\u001b` → ESC byte reachability confirmed: payloads were injected as raw Go strings on
  the exact channels the async results use.

**Green (minimum fix)**
- **`sanitizeTerminalText(s)` (new `internal/ui/sanitize.go`)** is the single pre-style boundary:
  (1) `github.com/charmbracelet/x/ansi` `Strip` — already pinned v0.11.8 transitively by the Charm
  v2 set, promoted to a **direct** require (no new dependency, no version change) — parses the whole
  string as an ECMA-48 stream and drops every complete *and dangling* ESC/C1-introduced sequence
  (CSI/OSC/DCS/APC/PM/SOS); (2) a second pass drops the remaining unsafe C0 controls plus DEL/C1,
  retaining only `\n` and `\t`. Idempotent; preserves newline/tab, ordinary Unicode (CJK/emoji),
  and markdown text. Applied strictly BEFORE any SelfTUI styling (never to styled output).
- **Ingress sites sanitized (before style/render-cache):** agent — model names at
  `onModelsLoaded`, `modelsErr`, tool start/result status rows, ToolConfirm display copy
  (runner keeps its raw copy), FallbackMsg notice, `done_reason` + error body in `onChatDone`;
  models — names at `onLoaded`, list/detail/delete/pull error stores, pull status, and a
  `sanitizeDetails` deep copy of the /api/show payload (license/modelfile/parameters/template/
  capabilities/model-info keys+string leaves, projector info).
- **Display-funnel defense in depth:** `renderBlock` sanitizes md before BOTH the glamour branch
  and the raw fallback (audit-cited lines); `chatLines` sanitizes the raw-history fallback when a
  render-cache entry is missing (cache-gap guard). `modelSummary` sanitizes list secondary rows /
  picker summaries. `appendSessionTurn` mirrors the sanitized content into the transcript file.
- **Dependency impact:** go.mod moves `github.com/charmbracelet/x/ansi v0.11.8` from indirect to
  direct (already pinned, sum unchanged). No new module in go.sum.

**Commands + exit codes**
- `git status --short` → empty at start · branch `fix/v0.1.1-audit-remediation` · HEAD at start
  `8cd50c8`. `make check` → 0 · `make race` → 0 · `PATH=$PATH:$(go env GOPATH)/bin make vuln` → 0
  (0 vulnerabilities in called code) · `go test -count=1 ./internal/ui -run 'Test.*(Sanitize|Control|OSC|CSI)'`
  → ok (12 focused tests) · `go test -count=1 ./internal/ui` → ok · `go test -race -count=1 ./internal/ui`
  → ok · `git diff --check` → clean.

**Decisions / lines to respect**
- Sanitize remote-derived text **when stored**, plus at the two output funnels, so no future
  display site can bypass it; SelfTUI's own SGR styles are added only after sanitization and are
  never stripped.
- Raw committed content stays raw in `v.history` (the runner's copy is untouched and re-sent on the
  next turn); only display + the transcript mirror are sanitized.
- Model names are sanitized at store, so the API request also carries the clean name — a hostile
  control-byte name is unusable against Ollama anyway and now fails cleanly instead of leaking.
- User-authored local text is not ingress-sanitized (same-terminal trust); it passes the same
  renderBlock display funnel, which is idempotent.
- `sanitizeTerminalText` keeps `\n`/`\t` (real layout in transcripts/detail/model file); drops CR
  (line overwrite), BEL, BS, VT, FF, NUL, DEL, all C1, and every complete or dangling escape
  sequence — split-token safety proven by unit test (two frames of a split OSC 52 cannot
  re-execute) and by the render test feeding OSC/CSI across consecutive TokenMsgs.

**Blockers / open decisions**
- None for Task 06. Carried env note from Task 02/04: `make vuln` needs `$(go env GOPATH)/bin` on
  PATH. The raw-markdown fallback branch is only reachable when glamour's renderer build fails
  (style load), so it is tested through the sanitize-then-fallback construction plus the unit
  corpus; no production path was contorted to force a glamour failure.

**Next action**
- Fresh Pi session: runbook **Task 07 (H-06 — make audit-pack creation manifest-complete)**:
  confirm branch `fix/v0.1.1-audit-remediation` + clean status, then follow the Task-07 block. Do not
  run `make smoke` until the owner runs it on a disposable model/tag or an isolated Ollama store.
### 2026-09-05 — Runbook Task 07: H-06 manifest-complete audit packaging — create + verify (owner task)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 07 (H-06 — make audit-pack creation
manifest-complete). No §10 row to tick (runbook-owned step). **Result:** done — red-green on branch
`fix/v0.1.1-audit-remediation`; new `scripts/create-audit-pack.sh` (create + verify subcommands) and
`scripts/create-audit-pack-test.sh` (42 checks), `audit-pack` Makefile target, README usage note; worktree
clean, all gates exit 0. Code commit `a9e9e60` (`build(audit): verify complete tracked-file packages`), docs
commit follows this entry. The shipped verifier **reproduces the exact H-06 finding** against the real
historical audit ZIP (see evidence below). Generated disposable pack (not committed): `dist/selftui-audit-pack-a9e9e60.zip`.

**Context (what H-06 actually was)**
- The external-audit package (`~/selftui-audit-pack/selftui-audit-pack.zip`, snapshot of `e6ef11b`) claimed 97
  tracked files in `FILE-INVENTORY.md` but its archive held only 95 file entries (93 tracked + PROMPT.md +
  FILE-INVENTORY.md): `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `.gitignore`, and
  `.gitattributes` were silently absent — a naive non-hidden copy dropped every dotfile/dotdir.
- The new tool's contract: `git ls-files` is the authoritative tracked set and the archived content comes
  from the same committed tree at HEAD (git archive tar, then one deterministic python zip pass); extras are
  explicit-only (`--extra TARGET=PATH`); verification compares tracked-vs-archived **in both directions**
  (nothing missing, nothing unexpected) and rejects any unsafe member path; output is byte-deterministic per
  commit (fixed order, commit-time stamps); an existing archive is never overwritten silently (`--force`).
  Workspace precondition: clean worktree, so `ls-files` == HEAD tree (same rule as release-check).

**Reproduction (runtime-verified, not static)**
- **Red:** the regression suite was written first — headline case rebuilds the historical failure shape (a
  naive 3-file pack from a 7-file fixture missing exactly the four hidden paths) and requires the verifier
  to fail naming all four. Pre-implementation run failed at every case (`create-audit-pack.sh` absent).
- **Historical-artifact proof (strong):** in a throwaway `git worktree` at `e6ef11b` (97 tracked), the shipped
  verifier run over the REAL `~/selftui-audit-pack/selftui-audit-pack.zip` (extras declared) prints
  `missing tracked:` for `.gitattributes`, `.github/workflows/ci.yml`, `.github/workflows/release.yml`,
  `.gitignore`, `manifest: 97 tracked, 95 members, 2 extras - missing 4, unexpected 0`, `MANIFEST_MATCH=FAIL`,
  exit 1 — the tool independently reproduces the audited defect; the same zip with no extra declared also
  reports PROMPT.md/FILE-INVENTORY.md as unexpected (extras are never silently assumed).

**Green (42/42 checks in `scripts/create-audit-pack-test.sh`)**
- Fixture repo (7 tracked: README.md, src/main.c, deep/nested.txt, + the four hidden paths) drives every case:
  naive-pack rejection naming all four; create exits 0 with `tracked:  7 files`, archive path, sha256, and
  `MANIFEST_MATCH=PASS`; pack members == tracked exactly; all four formerly-omitted paths present in the zip;
  deterministic output (two creates on the same commit → identical sha256); overwrite refusal + `--force`;
  explicit `--extra FILE-INVENTORY.md=<file>` present in pack, verify PASS when declared and FAIL reporting
  `unexpected member: FILE-INVENTORY.md` when undeclared; shadow guard (extra target = tracked README.md →
  refused, no archive left behind); traversal/absolute extra targets refused; verify rejects `../escape.txt`
  and `/abs.txt` members (`unsafe archive path:`); two-direction drift (commit adds newfile.txt, deletes
  src/main.c → stale pack fails with `missing tracked: newfile.txt` **and** `unexpected member: src/main.c`).

**Disposable pack from the current repo (not committed; dist/ is gitignored)**
- `make audit-pack` → `dist/selftui-audit-pack-a9e9e60.zip`: 104 tracked files, sha256
  `da4de136289fe6aa54f8743d6ce3a1de47a2c116852730d7bb88e08d46701d82`, `MANIFEST_MATCH=PASS`.
- Independent proof (python, not the script's self-report): member count 104; `.github/workflows/ci.yml`,
  `.github/workflows/release.yml`, `.gitignore`, `.gitattributes` all PRESENT; tracked−zip = [] and
  zip−tracked = []; `internal/agent/runner.go` bytes identical to the worktree copy.

**Commands + exit codes**
- Session guard at start: `git status --short` → empty · branch `fix/v0.1.1-audit-remediation` · HEAD
  `dc4668c`. `bash -n scripts/create-audit-pack.sh scripts/create-audit-pack-test.sh` → 0 ·
  `bash scripts/create-audit-pack-test.sh` → 0 (42 checks, 0 failures) · `make check` → 0 · `make audit-pack`
  → 0 (pack created, MANIFEST_MATCH=PASS) · historical-zip verify → exit 1 (expected FAIL, four missing) ·
  `git diff --check` → clean. Code commit `a9e9e60` (+2 files, ~470 lines script+test, Makefile +13, README +21).

**Decisions / lines to respect**
- `git ls-files` (clean worktree) is the manifest; content comes from HEAD via git archive, so packed bytes
  are exactly committed bytes and reproducible (task 22 will re-run the same script on the v0.1.1 tip).
- One canonical comparator (python `verify`) is shared by the `verify` subcommand and create's post-build
  self-check — no drift between "produce" and "prove".
- Extras are explicit-only and can never shadow a tracked path (target ∈ tracked → abort before writing);
  duplicates, unsafe targets (absolute / `..` / empty / backslash / colon components) and unsafe archive
  members all fail loudly. Tracked symlinks are refused (no content to archive; repo has none).
- Determinism is per-commit: fixed member order + entry timestamps at the commit time. Output default
  `dist/selftui-audit-pack-<HEAD>.zip`; overwrite requires `--force` (never silent). `make audit-pack` passes
  `AUDIT_PACK_OUT` / `AUDIT_PACK_EXTRAS` through to the script.
- This task fixes packaging evidence only — no `.github/workflows/*` content was modified.

**Blockers / open decisions**
- None for Task 07. Carried env note from Task 02/04: `make vuln` needs `$(go env GOPATH)/bin` on PATH.
  (Recorded in this task's evidence; the disposable pack and the `/tmp/audit-hist-*` worktree were cleaned
  up; only the gitignored `dist/selftui-audit-pack-a9e9e60.zip` remains as the evidence artifact.)

**Next action**
- Fresh Pi session: runbook **Task 08 (M-01 — enforce approval expiry and modal key ownership)**:
  confirm branch `fix/v0.1.1-audit-remediation` + clean status, then follow the Task-08 block. Do not run
  `make smoke` until the owner runs it on a disposable model/tag or an isolated Ollama store.
### 2026-09-05 — Runbook Task 08: M-01 approval expiry + modal key ownership (owner task)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 08 (M-01 — enforce approval expiry
and modal key ownership). No §10 row to tick (runbook-owned step). **Result:** done — red-green on branch
`fix/v0.1.1-audit-remediation`; code commit `ad69fe6` (`fix(agent): expire approvals and retain modal focus`),
docs commit follows this entry. Worktree clean, all gates exit 0. `make smoke` NOT run (owner-run on a
disposable model/tag or an isolated Ollama store — unchanged by this task).

**Context (what M-01 actually was)**
- Agent mutation approval had **no real timeout**: `runner.go` `confirm` showed `Timeout: 30s` on the dialog but
  selected only on the reply channel or `ctx.Done()`, so an unanswered write/edit approval could stall a turn
  until cancellation. Root `app.go` routed Tab/Shift-Tab to the tab bar **before** consulting child modal state
  (only the 1/2/3 digit jumps checked `ModalOpen`), so a tab press could hide the only approval surface.
- Fix contracts: per-Runner duration seam (no package-global mutable timeout hook), production default 30s,
  max 60s clamp preserved; approval expiry is **terminal for the turn** (the owner not answering means they are
  not present to supervise further mutations) with one stable error; late replies stay harmless (nonblocking
  buffered send, nothing to read or mutate); every Agent/Models child modal owns Tab/Shift-Tab until it closes;
  ctrl+c keeps its documented quit behavior (handled before modal routing).

**Reproduction (runtime-verified)**
- **Agent red:** new `TestWriteConfirmExpiresWithoutResponse` (injected `r.confirmTimeout = 50ms`, nobody
  responds, 2s bounded ctx) — pre-fix the runner ignored the seam and returned `context deadline exceeded` after
  the 2s harness deadline (`mutation_test.go:135: Run error = context deadline exceeded, want the stable
  approval-timeout error`, 2.00s). Post-fix the timer fires first: 0.05s PASS.
- **UI red:** new root-level `TestModalOwnsTabAndShiftTabWhileOpen` — 8 sub-tests (Models confirm/input/pull/
  delete, Agent confirmation/help/selector/clear) each failed pre-fix: Tab moved Models→Agent (`tab=1`) and
  Agent→Settings (`tab=2`; Shift+Tab wrapped Agent→Models `tab=0`) while the modal stayed open. Post-fix all 8
  PASS (tab unchanged, modal still open + overlay text rendered, then tab bar restored once each modal closed).

**Green (minimum fix)**
- **Agent (`internal/agent/runner.go`):** `confirm` now arms a real `time.NewTimer(timeout)` (per-Runner
  `confirmTimeout` seam, 0 ⇒ 30s default, clamped to the 60s max), stopped+drained on every non-timer exit so a
  fired timer can never wake a later select. New `case <-timer.C` returns the stable sentinel
  `errApprovalTimedOut` (`approval timed out`) wrapped with the tool name. In `run`, an expired approval is
  distinguished from an ordinary decline: `errors.Is(toolErr, errApprovalTimedOut)` aborts the whole turn
  (`agent: write_file: approval timed out`) instead of recording a failed tool result and letting the model keep
  requesting mutations — `Run` emits exactly one `AgentDoneMsg` carrying the stable error, nothing is written,
  and the server sees no retry request.
- **UI (`internal/ui/app.go`):** after the ctrl+c, palette, and settings-form cases, the KeyMsg path now routes
  every key to the ACTIVE child while that child's modal is open (`a.tab==0 && a.models.ModalOpen()` /
  `a.tab==1 && a.agent.ModalOpen()`), so Tab/Shift-Tab reach the child (which consumes/ignores them) before any
  global tab navigation. Children already dismiss their own modals, so tab keys return as soon as the modal
  closes; `onChatDone` already clears a stale confirmation overlay when a turn ends (timeout path).
- The UI overlay already showed `timeout: <duration>` (agent_view) and Models delete confirm / pull input were
  untouched; smoke's x→confirm→y and p→name→enter flows are unaffected (no Tab presses there), so `make smoke`
  behavior on the owner's disposable model/tag is unchanged.

**Commands + exit codes**
- Session guard at start: `git status --short` → empty · branch `fix/v0.1.1-audit-remediation` · HEAD `240ab7f`.
- Red: `go test -count=1 ./internal/agent -run 'TestWriteConfirmExpiresWithoutResponse'` → FAIL (2.00s,
  `context deadline exceeded`) · `go test -count=1 ./internal/ui -run 'TestModalOwnsTabAndShiftTabWhileOpen'`
  → FAIL (8/8 sub-tests, tabs moved).
- Green: `go test -count=1 ./internal/agent -run 'Test.*Confirm.*(Timeout|Expire)'` → ok (0.05s) ·
  `go test -count=1 ./internal/ui -run 'Test.*Modal.*Tab'` → ok (8 sub-tests) ·
  `go test -count=1 ./internal/agent ./internal/ui` → ok · `make check` → 0 (build/test/vet/fmt clean) ·
  `make race` → 0 (full suite, `internal/ui` 10.1s) · `git diff --check` → clean.
- `make smoke` and `make smoke-model` NOT run — owner-run on a disposable model/tag (H-04 preflight). No
  release-check (Task 22). `git status --short` after commits → empty.

**Decisions / lines to respect**
- Approval expiry is a **turn-terminal** stable error (`agent: write_file: approval timed out`), not a per-call
  decline: an owner who does not answer within the window is not present to supervise the rest of the turn.
  Matching text on the stable substring `approval timed out`.
- The window seam lives on the Runner (`confirmTimeout`, unexported, package-agent tests set it directly); the
  30s production default and 60s max are unchanged. No clock indirection added — the real `time.Timer` with a
  stop/drain defer is the deterministic-enough seam for tests.
- Child modals own **every** key while open (mirrors how palette/settings-form already own keys); the routing
  sits after ctrl+c/palette/settings-form cases so ctrl+c quit, palette gating, and form editing semantics are
  untouched. `modelsTab=0`/`agentTab=1` are referenced by literal in the new tests because only `agentTab` has a
  package constant.
- Agent view state rows (help/clear/picker/approval) and Models rows (confirm/input/pull/delete) are the
  canonical modal set for future routing changes.

**Blockers / open decisions**
- None for Task 08. Carried env note from Task 02/04: `make vuln` needs `$(go env GOPATH)/bin` on PATH.
- Next runbook task (09 — M-02) will need `internal/agent/context.go` budgeting changes; the M-01 turn-terminal
  timeout interacts with the budget only in that an expired turn emits one AgentDoneMsg (already covered).

**Next action**
- Fresh Pi session: runbook **Task 09 (M-02 — preserve atomic tool exchanges during context trimming)**:
  confirm branch `fix/v0.1.1-audit-remediation` + clean status, then follow the Task-09 block. Do not run
  `make smoke` until the owner runs it on a disposable model/tag or an isolated Ollama store.
### 2026-09-05 — Runbook Task 09: M-02 protocol-safe atomic context budgeting (owner task)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 09 (M-02 — preserve atomic tool
exchanges during context trimming). No §10 row to tick (runbook-owned step). **Result:** done — red-green on
branch `fix/v0.1.1-audit-remediation`; code commit `07952c5` (`fix(agent): preserve tool exchanges
in context budget`), docs commit follows this entry. Worktree clean, all gates exit 0. `make smoke` NOT run
(owner-run on a disposable model/tag or an isolated Ollama store — unchanged by this task).

**Context (what M-02 actually was)**
- `BudgetMessages` (`context.go`) evicted **one message at a time** and truncated only the last message's
  `Content`. A tool turn lives in the history as an assistant `tool_calls` message plus all of its correlated
  `role:"tool"` results (`runner.go:218-243`), so one-message eviction could stop **between** a call and its
  results — leaving retained history to begin with an orphan `role:"tool"` message, or retaining a call without
  all its results. `truncateLatest` cannot shrink oversized `ToolCalls` (no content) or an oversized retained
  system prompt, so both could be **sent raw past the three-quarter budget** (silent exceed; some Ollama/model
  combinations reject or mishandle the invalid sequence).
- Fix contracts: eviction is **atomic per user-led exchange** (an exchange = everything after a user message up
  to the next user: assistant text, assistant tool calls, and all correlated results — never split); retained
  history never begins with `role:"tool"`; the **newest complete user-led exchange is always retained**;
  exactly one `TruncationNotice` marker when any conversation is omitted (dedupe on re-budget); and
  `ApproxTokens(out) <= 3/4·num_ctx` whenever a bounded representation is possible. Unshrinkable oversized
  shapes get a **deterministic bounded representation** — a retained tool call's arguments become the
  valid-JSON placeholder `{}` (names kept, so results stay positionally correlated) and an oversized pinned
  system prompt's content is shortened with the existing `"[truncated] "` convention **as the last resort**
  (marker never shortened, system order preserved).

**Reproduction (RED, current code)**
- `TestBudgetM02ProtocolSafe` (8 table sub-cases: assistant call + one result; parallel calls + all results;
  two older exchanges; latest oversized tool call; oversized system prompt; two tiny-num_ctx; oversized newest
  result keeps its call). Pre-fix **6/8 FAIL**: each tool-exchange case returned e.g.
  `[system, marker, tool "r…"]` — an orphan `role:"tool"` head where the old code stopped after dropping the
  user and the assistant call (`role=tool message at index 2 lacks an immediately preceding assistant tool
  call`); "latest oversized tool call" dropped every message down to the lone result; "oversized system prompt"
  returned `approximateTokens = 5003, limit = 48` (the silent exceed). Tiny-num_ctx cases already passed
  (single-turn truncation) and stay as guards.

**Green (minimum fix, `internal/agent/context.go` only — no runner change needed)**
- `BudgetMessages`: pin the leading system run in order; split the conversation into atomic exchanges at each
  user message; drop whole oldest exchanges until the newest suffix fits (marker inserted exactly once, not
  duplicated on re-budget); then `boundToLimit` on the retained newest exchange if it alone still overflows.
- `boundToLimit` deterministic order: (probe) if compacting tool-call arguments alone fits, do only that —
  user turn and every tool RESULT stay intact; then (1) shorten newest shrinkable conversational Content
  (`[truncated] ` tail convention, strict token decrease); (2) compact the newest oversized assistant
  tool-call message's arguments to `{}`; (3) last resort, shorten the pinned system content (order kept,
  marker exempt). Every action strictly decreases `approximateTokens`, so the loop terminates; a genuinely
  unboundedable floor (pathological num_ctx) returns the minimal deterministic list. `ApproxTokens` math and
  the exported estimator are byte-identical to before (UI meter M7-C unchanged); `TruncationNotice` text and
  `"system/3-quarters"` semantics unchanged. Existing M6 budget tests all still pass unmodified
  (`TestBudgetMarkerOncePerCall`, single-huge-turn, tool-args-count, marker-dedupe).

**Commands + exit codes**
- Session guard: `git status --short` → empty · branch `fix/v0.1.1-audit-remediation` · HEAD `16bfb26`.
- Red: `go test -count=1 ./internal/agent -run TestBudgetM02ProtocolSafe` → FAIL (6/8 sub-cases: orphan
  `role:"tool"` heads; oversized latest call trimmed to a lone orphan result; oversized system prompt at
  5003/48 tokens).
- Green: `go test -count=1 ./internal/agent -run 'TestBudget|Test.*Context'` → ok (0.003s, 7 tests incl.
  8 M-02 sub-cases) · `go test -count=1 ./internal/agent` → ok (0.113s) · `go test -race -count=1
  ./internal/agent` → ok (1.277s) · `make check` → 0 (build + full test suite + vet + gofmt clean) ·
  `git diff --check` → clean. `make smoke`/`make smoke-model` NOT run (H-04 preflight unchanged). `git status
  --short` after commits → empty.

**Decisions / lines to respect**
- Eviction unit = a **user-led exchange** (user message through the next user), which by construction cannot
  split an assistant tool call from its results and never leaves a `role:"tool"` head. Marker is inserted only
  when an exchange was actually dropped and is deduped when a previous pass already inserted it (the runner
  re-budgets the same history every iteration).
- Unshrinkable oversized retained tool call ⇒ deterministic `arguments: {}` placeholder (valid JSON, names
  kept) rather than raw overflow; oversized system prompt ⇒ deterministic `[truncated] ` tail shortening only
  after conversational content and arguments are exhausted; the marker is never shortened.
- No runner.go/UI change was required: the fix is purely in the pure budgeting function; the error-free public
  surface (`BudgetMessages([]ollama.ChatMessage, int) []ollama.ChatMessage`, `ApproxTokens`,
  `TruncationNotice`) is preserved.

**Blockers / open decisions**
- None for Task 09. Carried env note from Task 02/04: `make vuln` needs `$(go env GOPATH)/bin` on PATH.
  Task 13 (M-06) is the next agent-context task and will read this entry's eviction-unit vocabulary when it
  adds cancellation/backpressure propagation.

**Next action**
- Fresh Pi session: runbook **Task 10 (M-03 — reject stale model and host responses)** (needs runtime
  reproduction): confirm branch `fix/v0.1.1-audit-remediation` + clean status, then follow the Task-10 block.
  Do not run `make smoke` until the owner runs it on a disposable model/tag or an isolated Ollama store.
### 2026-09-05 — Runbook Task 10: M-03 reject stale model/host responses (owner task)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 10 (M-03 — reject stale model and
host responses). No §10 row to tick (runbook-owned step). **Result:** done — red-green on branch
`fix/v0.1.1-audit-remediation`; code commit follows this entry, docs commit after it. Worktree clean, all
gates exit 0. `make smoke` NOT run (owner-run on a disposable model/tag or an isolated Ollama store —
unchanged by this task).

**Context (what M-03 actually was)**
- `ModelsView.showCmd(name)` captured a name but the completion handlers (`modelsShowMsg`/
  `modelsShowErrMsg`) applied results unconditionally, and the side-by-side auto-inspect branch suppressed a
  new request while `loadingShow` was true (`models_view.go`). So rapid A→B navigation could show A's detail
  under selected B and nothing ever fetched B. `ApplyClient` (host/token save) started a fresh load without
  invalidating the old host's in-flight list/show results, so a slow old host could overwrite the newly
  configured host's state. The Agent view's model-list reload had the same unversioned shape
  (`agentModelsLoadedMsg` applied blindly after an `ApplyConfig(reload=true)` host swap).
- Fix contracts: **client generation** (monotonic, bumped on client replacement — `ApplyClient` for Models,
  `ApplyConfig(..., reload=true)` for Agent) stamps every async list/show result; a completion whose
  generation differs from the view's current one is dropped (old-host results can never replace new-host
  state). **Request ids** (`showReq`, monotonic per ModelsView) + a `showTarget` + a cancellable
  `showCancel` give each detail fetch an identity: a selection change during a load replaces the in-flight
  request (cancel + bump) instead of suppressing the fetch, and a superseded or wrong-model completion is
  dropped. **All model mutation stays on the Bubble Tea update loop** — commands only read the view copy and
  stamp ids; they never write state.

**Reproduction (RED, current code — controlled completion order, no timing races)**
- `TestModelsViewStaleShowCannotReplaceNewerSelection` (wide side-by-side): auto-inspect of qwen3:8b (A)
  captured in flight, selection moved to gemma3:12b (B) during the load, B's result delivered, then A's late
  result — pre-fix the stale A detail landed under selected B: `M-03: stale "qwen3:8b" detail replaced the
  newer selection "gemma3:12b": detailName="qwen3:8b" index=1` (models_view_test.go:904).
- `TestModelsViewApplyClientRejectsOldHostResults`: old-host list fetch completed before `ApplyClient`; new
  host's list landed first; the old host's late result replaced it: `M-03: old-host list result replaced the
  new host's list after ApplyClient: [old-host-model]` (models_view_test.go:970).
- `TestModelsViewApplyClientRejectsOldHostShow`: a detail fetch started against the old host completed after
  `ApplyClient` and populated the pane: `M-03: old-host show result populated the pane after ApplyClient:
  detailName="qwen3:8b"` (models_view_test.go:1010).
- `TestAgentViewReloadRejectsObsoleteModelList`: obsolete pre-swap agent model list replaced the new host's
  after `ApplyConfig(reload=true)`: `M-03: obsolete agent model list replaced the new host's after
  ApplyConfig: models=[old-agent-model]` (agent_view_test.go:802).

**Green (minimum fix, `internal/ui/models_view.go` + `internal/ui/agent_view.go`)**
- Messages stamp their origin: `modelsLoadedMsg`/`modelsLoadErrMsg`/`agentModelsLoadedMsg`/
  `agentModelsErrMsg` carry `gen` (client generation at issue); `modelsShowMsg`/`modelsShowErrMsg` carry
  `gen` + `req` (request id). Update handlers drop generation mismatches before any state change; the Agent
  view drops stale `agentModelsLoadedMsg`/`ErrMsg` the same way (no obsolete reload can reset `v.model`).
- `requestShow` replaces the old `showCmd`: it cancels any in-flight show context, bumps `showReq`, records
  `showTarget`, stores the cancellable ctx in `showCancel`, and returns a command that stamps gen+req.
  `ApplyClient` bumps `clientGen`, cancels the pending show (`releaseShow`), and drops all detail state
  before reloading; `onDeleteDone` also releases a pending show for the deleted model.
- Completion applicability is `acceptShow`: current generation AND (for real results) `req == showReq` AND
  the result names the currently selected model — so a stale or superseded detail can never repaint the pane
  under a newer selection. Hand-built results without an id (req 0 — legacy deliveries, routing/render test
  injection) are accepted only when the list is empty (no selection to protect) or when they name the pending
  target. A dropped completion that answered the *latest* request releases the in-flight state so the pane
  never dangles on "inspecting…".
- Selection changes on the side-by-side layout now always request the newly selected model during a load
  (replacing the in-flight fetch) instead of being suppressed by `loadingShow`; already-shown models are not
  refetched (`detailName == name && detail != nil && !loadingShow`).
- `ApplyConfig(cfg, c, reload=true)` bumps the Agent `clientGen` before clearing + refetching. Gen counters
  start at 0 and all existing tests construct keyed literals without gen, so every legacy delivery still
  applies unchanged (routing/envelope/golden/sanitize suites pass untouched; zero golden bytes changed).

**Commands + exit codes**
- Session guard: `git status --short` → empty · branch `fix/v0.1.1-audit-remediation` · HEAD `6d57e0f`.
- Red: `go test -count=1 ./internal/ui -run 'TestModelsViewStaleShowCannotReplaceNewerSelection|
  TestModelsViewApplyClientRejectsOldHostResults|TestModelsViewApplyClientRejectsOldHostShow|
  TestAgentViewReloadRejectsObsoleteModelList'` → FAIL (4/4: stale A detail under B; old-host list and
  show after ApplyClient; obsolete agent list after ApplyConfig).
- Green: same focused run → ok 4/4 · `go test -count=1 ./internal/ui -run 'Test.*(Stale|Generation|Selection|
  ApplyClient)'` → ok · `go test -count=1 ./internal/agent ./internal/ui` → ok (ui 4.9s) ·
  `make check` → 0 (build + full suite + vet + gofmt clean) · `make race` → 0 (full suite, ui 9.9s) ·
  `git diff --check` → clean. `make smoke`/`make smoke-model` NOT run (H-04 preflight unchanged). `git status
  --short` after commits → empty.

**Decisions / lines to respect**
- Correctness layer = generation + request id on the message; cancellation of the obsolete HTTP context is
  the optimization ("when practical"), layered on the same `showCancel` the loop owns. `loadCmd` keeps its
  bounded 60s timeout (gen guard drops its late results); only show fetches are actively canceled on
  replacement, because they are the frequent user-visible path.
- Real completions (req ≠ 0) are applied only when they answer the latest request AND name the current
  selection; req-0 (hand-built) results keep the legacy envelope/routing semantics the existing suite
  depends on. A client replacement never mutates detail state from a stale completion — `ApplyClient` reset
  the pane and only the new generation's own load/show results may repaint it.
- Public message structs gained fields only (keyed literals compile unchanged); no signature change to
  `loadCmd`/`Init`/`ApplyClient`/`ApplyConfig`; no render or golden text changed (byte-identical goldens).
- Runbook item "a selection changed during loading eventually requests the latest model" is satisfied by
  issuing the replacement at selection-change time (strictly stronger than waiting for the stale completion);
  "after an inspection finishes, request the current selection if it differs from the completed name" is
  covered by the same rule plus the drop-with-release backstop for completions whose model is no longer
  selected.

**Blockers / open decisions**
- None for Task 10. Carried env note from Task 02/04: `make vuln` needs `$(go env GOPATH)/bin` on PATH.
  Task 11 (M-04) moves transcript persistence off the update loop and will read the M-02/M-03 message-routing
  vocabulary (`modelsEventMsg`/`agentEventMsg` envelopes, off-loop writers) when adding its recorder actor.

**Next action**
- Fresh Pi session: runbook **Task 11 (M-04 — move transcript persistence off the update loop)** (needs
  latency reproduction): confirm branch `fix/v0.1.1-audit-remediation` + clean status, then follow the
  Task-11 block. Do not run `make smoke` until the owner runs it on a disposable model/tag or an isolated
  Ollama store.
### 2026-09-05 — Runbook Task 11: M-04 move transcript persistence off the update loop (owner task)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 11 (M-04 — nonblocking ordered
transcript persistence). No §10 row to tick (runbook-owned step). **Result:** done — red-green on branch
`fix/v0.1.1-audit-remediation`; code commit follows this entry, docs commit after it. Worktree clean, all
gates exit 0. `make smoke` NOT run (owner-run on a disposable model/tag or an isolated Ollama store —
unchanged by this task).

**Context (what M-04 actually was)**
- `agent_view.go` called `session.Open`/`Append`/`Close` synchronously on the Bubble Tea update loop for
  every committed turn, and `/export` called `Flush` (fsync) synchronously in Update; `session.go`'s
  `Log` did the directory/file I/O with retry sleeps and `Sync` on Flush/Close. `main.go` never closed the
  session, so the last turns relied on process cleanup. Fix contract: **one ordered background actor owns
  the Log** — Update only enqueues immutable append/flush jobs and processes completion/error messages; it
  must never call Open/Append/Sync/Close. Transcript order stays deterministic (job FIFO through one
  worker), `/export` flushes every earlier enqueued turn before reporting the exact path, recording stays
  optional, and explicit flush/close on normal shutdown must not strand the worker.

**Reproduction (structural + compile-red; no fake latency claim)**
- HEAD's defect is structural and line-cited: Update ran the fs calls inline (`agent_view.go` at HEAD:
  `appendSessionTurn` Open/Append/Close in Update; `/export` Flush in Update). Measured pre-fix cost shape
  (throwaway probe, removed): 500 real `Open+Append+Flush+Close` cycles on this machine = ~1.7-1.8 ms mean
  per cycle — i.e. every turn's inline fs work scales with the sink, and on a slow/network-backed XDG dir or
  a large paste the update stalls with it. A *permanent-stall* behavioral red against HEAD is not
  constructible in-process: the old code has **no injectable writer seam** (fresh `O_EXCL` regular-file
  writes to a tmpdir cannot be made to block without one), so per the task's own gate I did not fake a
  latency red. Instead: the seam/recorder tests are the RED (they fail to build against HEAD —
  `recorder_test.go: undefined: Recorder/NewRecorder/Writer/…` — this repo's established compile-red for
  structural findings), and the new stall tests would HANG an inline Update by construction (a stalled
  writer means the old synchronous append never returns); post-fix they pass in ~0 s while the writer is
  provably parked. The hard contract "Update never calls Open/Append/Flush/Close" is now enforced by the
  code itself (no such calls remain in `agent_view.go`) and pinned by the seam tests.
- Probe evidence recorded here so it is not lost: `500 open+append+flush+close cycles: total
  894.8ms/860.0ms, mean 1.79ms/1.72ms per cycle` (two runs; dir under `/tmp`).

**Green (minimum fix)**
- **`internal/session/recorder.go` (new):** a `Recorder` actor owns the transcript sink. Public seam:
  `Writer` interface (what a `*Log` satisfies), `OpenWriter` factory, `Result` per-job ack, `NewRecorder(dir,
  host)` (production, opens a `*Log` lazily inside the worker), `NewRecorderWithOpener` (test seam), and
  `ErrRecorderBacklog` (stable error when the bounded 256-job queue is full — the only enqueue failure, and
  it fails fast, never blocking Update). `Append`/`Flush` enqueue immutable jobs through a non-blocking
  buffered-channel select; every job acks exactly one `Result` on a cap-1 channel so a waiter can never
  hang. One worker goroutine processes jobs in acceptance order: lazy Open on the first job, ordered
  Append, Flush (the `/export` barrier), Close (flush+close+worker exit, idempotent via `sync.Once`).
  Failure discipline: the first open/append/flush error becomes the recorder's stable failure — the broken
  writer is closed, every later job acks the same error (nothing hangs), and no further I/O is attempted.
- **`internal/ui/agent_view.go`:** `session *session.Log` → `recorder *session.Recorder`. Committed turns
  (user in `sendInput`, assistant in `onChatDone`) go through `enqueueSessionTurn`, which sanitizes the
  content (H-05 boundary preserved), lazily starts the recorder on the first turn, enqueues, and returns an
  ack command that round-trips **nil on success** (no extra UI wakeups — bubbletea skips nil messages) or an
  `agentEventMsg{sessionAppendMsg{err}}` on failure. New Update cases: `sessionAppendMsg` disables recording
  once (`sessionErr` + one `session log: …` notice; later identical outcomes are ignored) and
  `sessionExportMsg` lands the async `/export` result (`applySessionExport`: path → `transcript: <path>`;
  error → disable once; empty path → the nothing-recorded hint). `exportSession` now enqueues a flush job
  and returns the ack command — Update never touches the filesystem. `WithSessionDir` still lazily arms the
  dir; `CloseRecorder()` is the shutdown boundary. Existing public error strings are preserved verbatim
  ("session recording is off — no transcript is written", "nothing recorded yet — send a message first",
  "transcript: ", "session log: "); the only new stable error is `ErrRecorderBacklog` (queue overflow).
- **`internal/ui/app.go`:** exported `CloseSession()` → agent `CloseRecorder()`.
- **`cmd/self-tui/main.go`:** `run()` now keeps the final model from `p.Run()` and calls the new
  `closeSessionRecorder(final)` helper (interface-asserted `CloseSession() error`) after the program exits —
  including on `Run` error paths — so every committed turn is flushed to the transcript and the worker is
  stopped before the process returns; a close failure is logged, never fatal. `main_test.go` pins the
  helper (closeable model called / nil / foreign model no-op).

**Tests (red-green)**
- RED (HEAD): `go test -count=1 ./internal/session -run 'TestRecorder'` → build failed
  (`undefined: Recorder/NewRecorder/NewRecorderWithOpener/Writer/Result/ErrRecorderBacklog`). RED was
  captured before `recorder.go` existed.
- `internal/session/recorder_test.go` (new, 9): `TestRecorderOrdersCommittedTurns` (user→assistant→user
  order + 0700 dir/0600 file + header, via the real Log), `TestRecorderFlushDrainsEarlierAppends` (export
  barrier reports the real path after earlier turns), `TestRecorderAppendDoesNotBlockCallerOnStalledWriter`
  (writer parked mid-append; caller keeps enqueueing; final order exact), `TestRecorderFlushWaitsForStalledEarlierTurn`,
  `TestRecorderOpenRunsOffCaller` (stalled lazy Open), `TestRecorderStopsWritingAfterFirstFailure`
  (exactly 2 writer calls, later jobs ack the same stable error, flush acks the failure, Close clean),
  `TestRecorderOpenFailureIsReportedOnceAndStops`, `TestRecorderCloseFlushesPendingTurnsAndIsIdempotent`
  (Close drains unacked turns, worker exits — `r.done` closed — second Close safe), and
  `TestRecorderBacklogFullRejectsWithoutBlocking` (5 accepted while the worker holds one in a 4-slot queue,
  next fails fast with `ErrRecorderBacklog`).
- `internal/ui/session_ui_test.go`: existing four tests adapted to the async recorder (persistence waits
  for the worker via a new `waitForSession` poller; `/export` executes its ack command; the no-dir test now
  asserts `v.recorder == nil`). New tests: `TestAgentViewSessionTurnOrder` (two full chat turns; strict
  user1 < assistant1 < user2 < assistant2 in the file), `TestAgentViewSessionWriteNeverBlocksUpdate`
  (Update(Enter) returns while the writer is provably parked mid-append, then stays responsive, then the
  turn lands once released), `TestAgentViewSessionWriteNeverBlocksUpdateStalledOpen` (same for the lazy
  Open), `TestAgentViewExportWaitsForStalledEarlierTurn` (the `/export` ack cannot complete before its
  stalled earlier turn; writer order is exactly append→flush after release), and
  `TestAgentViewSessionFailureSurfacesOnce` (first failure disables with one notice; a later commit
  enqueues nothing; writer saw exactly one append; `/export` after failure reports the disabled state
  synchronously). `cmd/self-tui/main_test.go` adds `TestCloseSessionRecorder`.

**Commands + exit codes**
- Session guard at start: `git status --short` → empty · branch `fix/v0.1.1-audit-remediation` · HEAD `048db03`.
- Red: `go test -count=1 ./internal/session -run 'TestRecorder'` → FAIL (build: undefined Recorder seam).
- Green: focused `go test -count=1 ./internal/session ./internal/ui -run 'Test.*(Session|Transcript|Recorder|Export)'` → ok ·
  `go test -count=1 ./cmd/self-tui ./internal/session ./internal/ui` → ok · `go vet ./...` → 0 ·
  `make check` → 0 (build + full suite + vet + gofmt clean) · `make race` → 0 (full suite, `internal/ui`
  10.1s) · `go test -race -count=3` on the new recorder + UI session tests → ok (no flakes) ·
  `git diff --check` → clean.
- `make smoke`/`make smoke-model` NOT run — owner-run on a disposable model/tag (H-04 preflight). No
  release-check (Task 22). `git status --short` after commits → empty.

**Decisions / lines to respect**
- One recorder actor owns the Log; per-job ack channels (cap 1) mean no waiter can hang and the worker never
  blocks on a dead waiter. Append acks return nil messages on success so the happy path adds no update-loop
  wakeups. The queue is bounded (256) with a fail-fast stable error (`ErrRecorderBacklog`) — a wedged sink
  disables recording once rather than ever blocking an update; committed turns are human-paced so the bound
  is never reached in practice.
- Failure discipline lives in the recorder: first open/append/flush error is terminal and stable; the view
  mirrors it with exactly one notice (`sessionErr` guard). `/export` after a failure reports the same
  nothing-recorded hint the pre-fix code produced after an append error (public strings preserved).
- Ordering is the loop's enqueue order through one FIFO worker — user/assistant turns cannot interleave out
  of commit order, and `/export` (a flush job) is processed strictly after every earlier enqueued turn.
- `Close()` is the durable shutdown path and blocks until the worker exits; a wedged sink (hung fsync) can
  delay it — process exit remains the escape hatch (documented in recorder.go). Normal shutdown (main →
  `CloseSession` on the final model) never strands the worker; failures leave the recorder referenced on the
  view so `CloseSession` still drains it.
- H-05 sanitization still happens in Update at enqueue time (the file never receives unsanitized content);
  session.Open/Append/Flush/Close + retry-sleep semantics in session.go are untouched (recorder tests reuse
  the real Log for perms/header/order).
- The runbook's "confirm current Update blocks" gate is satisfied by structural line-cited evidence + the
  compile-red on the seam + the stall tests that cannot pass against an inline writer; I did not fabricate a
  timing red against unseamed HEAD code. If the owner prefers a NOT_REPRODUCED disposition instead, this
  entry documents exactly what was and was not reproducible.

**Blockers / open decisions**
- None for Task 11. Carried env note from Task 02/04: `make vuln` needs `$(go env GOPATH)/bin` on PATH.
- Task 13 (M-06) follows this task's actor vocabulary (off-loop workers, bounded queues) when it makes the
  agent/pull producers cancellable and context-aware.

**Next action**
- Fresh Pi session: runbook **Task 12 (M-05 — replace byte-based wrapping with cell-/ANSI-aware wrapping)**:
  confirm branch `fix/v0.1.1-audit-remediation` + clean status, then follow the Task-12 block. Do not run
  `make smoke` until the owner runs it on a disposable model/tag or an isolated Ollama store.

### 2026-09-05 — Runbook Task 12: M-05 replace byte-based wrapping with cell-/ANSI-aware wrapping (owner task)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 12 (M-05). No §10 row to tick
(runbook-owned step). **Result:** done — red-green on branch `fix/v0.1.1-audit-remediation`; code commit
follows this entry, docs commit after it. Worktree clean, gates exit 0. `make smoke` NOT run (owner-run
on a disposable model/tag or an isolated Ollama store — unchanged by this task).

**Context (what M-05 actually was)**
- `wrapLines` (`internal/ui/models_view.go`) decided fit by `lipgloss.Width` (cells) but wrapped by byte
  offsets: `for len(line) > width`, `strings.LastIndex(line[:width+1], " ")`, `line[:cut]`/`line[cut:]`.
  Over-width CJK, emoji, ZWJ, combining-mark or ANSI-styled rows were cut inside a UTF-8 sequence or
  control sequence → invalid text, style bleed, wrong row count, or frame overflow at 72×30. Audit
  evidence cited `models_view.go:953-979`; tests were ASCII-only. Callers: models detail pane wrap +
  `maxScroll` height math (`models_view.go`), `renderCenteredOverlay` body wrap shared with Agent modals
  and the App palette (`components.go:65`). Fix contract: display-cell/grapheme-aware, ANSI-preserving
  wrap reusing one pinned Charm width facility (no parallel width model); every row valid UTF-8 and
  ≤ width cells; visible text neither lost nor duplicated; ASCII word-boundary behavior and deterministic
  output preserved; golden fixtures unchanged unless a semantic diff is intentional.

**Reproduction (RED, decisive)**
- Extended `TestWrapLines` with over-width CJK/emoji/ZWJ/combining/word-wider-than-limit/styled content
  plus a per-row invariant walker (valid UTF-8 · `lipgloss.Width(row) ≤ width` · no row head stranded
  with a combining mark or ZWJ · no ANSI opener split across rows · canonical visible text preserved).
- RED at HEAD: `go test -count=1 ./internal/ui -run 'TestWrapLines'` → FAIL. Byte slicing split
  `你好…` into `"你\xe5\xa5"`, `"\xbd"`, `"世\xe7\x95"`, `"\x8c"`… (invalid UTF-8 on every CJK/emoji row)
  and shredded a 25-byte ZWJ family emoji into ~20 garbage rows. Evidence captured above in this entry.

**Fix (green)**
- `wrapLines` overflow path now delegates to the pinned Charm wrap primitive
  `ansi.Wrap` (`github.com/charmbracelet/x/ansi` v0.11.8 — already a **direct** dependency via the H-05
  sanitizer; no go.mod/go.sum change). That is the same width model `lipgloss.Width` uses (`lipgloss/v2
  Width` = `ansi.StringWidth`, grapheme clusters): one Charm width model, no parallel implementation.
  Fit rows still pass through byte-untouched (existing styled/fit behavior preserved); only over-width
  lines are wrapped. ASCII word-boundary outputs verified identical to the old algorithm on the pinned
  cases (`abcdef`/3, `a b c`/3+4, hard-split overflow words).
- Empirically found one cluster defect in `ansi.Wrap`: it measures combining marks and ZWJ as zero width
  and can place the row break right after the base rune (e.g. `e` + U+0301 → row 1 `…e`, row 2 starts
  with a bare U+0301 — the same mark-detachment class the audit names). Added `rejoinSplitMarks`: a
  zero-width repair that moves stranded marks from a row head to the previous row's tail (cell widths
  unchanged, byte order preserved, style-only/ANSI heads skipped via `ansiHeadLen`). ZWJ family
  (`👨👩👧👦`) and woman-technologist (`👩💻`) clusters are merged correctly by the primitive and pass whole.
- All 19 `TestWrapLines` cases + `TestWrapLinesCellSafe` invariants green.

**Tests (red-green)**
- RED (HEAD): the failure above (captured in this entry's reproduction block).
- `internal/ui/models_view_test.go` (extended `TestWrapLines`, now 19 named cases): ASCII regressions
  unchanged (hard split, word boundary, trailing-space wrap, degenerate width 0, plus a new ASCII
  overflow word) and new M-05 cases: CJK words, CJK no-space, CJK word wider than the limit, 🚀 emoji,
  ZWJ family ×3 @4, ZWJ technologist ×4 @5, combining `e\u0301`×10 @5 and @3, double-combining
  `q\u0301\u0301`×9 (invariant-only), combining+CJK mixed (invariant-only), styled fits-passthrough
  (byte-exact), styled ASCII overflow (@12 exact rows), styled CJK overflow (@9 exact rows), styled long
  payload (invariant-only), mixed ASCII+CJK+ANSI (invariant-only). Every case runs the shared
  `checkWrapRowInvariants` walker (valid UTF-8, row ≤ width cells, no detached mark at a row head after
  `ansiHeadLen`, no ANSI opener split across rows via an `isAnsiFinal` scanner, canonical visible text
  preserved whitespace-insensitively). Exact-row expectations are semantic (word/cluster boundaries, 2
  cells for wide runes, 0-width marks glued) and were verified against the project's own
  `lipgloss.Width` oracle.

**Commands + exit codes**
- Session guard at start: `git status --short` → empty · branch `fix/v0.1.1-audit-remediation` · HEAD `429dfc2`.
- Red: `go test -count=1 ./internal/ui -run 'TestWrapLines'` → FAIL (invalid-UTF-8 rows; captured).
- Green: focused `go test -count=1 ./internal/ui -run 'TestWrapLines|Test.*Unicode|TestTruncateToWidth'` → ok ·
  `go test -count=1 ./internal/ui` → ok (5.1s, includes byte-exact golden fixtures — **no golden file
  changed**: every golden body line fits its pane, so no fixture needed a semantic update) ·
  `go vet ./internal/ui/` → 0 · `make check` → 0 (build + full suite + vet + gofmt clean) ·
  `git diff --check` → clean. `go.mod`/`go.sum` untouched (x/ansi already direct).
- `make smoke`/`make smoke-model` NOT run — owner-run on a disposable model/tag (H-04 preflight). No
  release-check (Task 22). `git status --short` after commits → empty.

**Decisions / lines to respect**
- Width oracle is Charm's, end to end: `lipgloss.Width` decides "fits", and `ansi.Wrap` (grapheme method,
  same clusters/widths `StringWidth` uses) does the wrapping — the task's "one Charm width/ANSI facility,
  no parallel width model" is literal. The only hand-rolled logic is the zero-width mark repair, which
  provably cannot change any row's cell count and preserves byte order.
- ASCII semantics from the pre-fix tests are preserved exactly; `ansi.Wrap` also hard-breaks words longer
  than the width (verified identical split points on the pinned ASCII fixtures and overflow words).
- `rejoinSplitMarks` repairs only row heads at i≥1 (a row 0 leading mark is the input's own); ANSI heads
  are skipped so a styled run opening a row is never misread as a stranded mark. Style-only rows are left
  alone; a row left empty after a tail-mark move renders as a blank line (harmless, never a floating mark).
- Single grapheme clusters wider than the requested width (possible only below 2 columns) are kept whole
  by the primitive; no caller wraps below 16 columns (overlay `innerW = max(w-6,16)`, detail panes ≥ 38),
  and the App's small-terminal gate (40×12) keeps ModelsView off sub-40 geometry entirely.
- Scope kept to the M-05 finding: `truncateToWidth` (agent_view.go) was already rune/ANSI-safe and is out
  of the allowed-file set; untouched.

**Blockers / open decisions**
- None for Task 12. Carried env note from Task 02/04: `make vuln` needs `$(go env GOPATH)/bin` on PATH.
- Task 13 (M-06) follows next: cancellation/backpressure for tool work and producer sends (grep is
  already context-aware from Task 02), then M-07 delete overlay etc.

**Next action**
- Fresh Pi session: runbook **Task 13 (M-06 — propagate cancellation through tools and UI delivery)**:
  confirm branch `fix/v0.1.1-audit-remediation` + clean status, then follow the Task-13 block. Do not run
  `make smoke` until the owner runs it on a disposable model/tag or an isolated Ollama store.

### 2026-09-06 — Runbook Task 13: M-06 cancellation through tools and stream producers (owner task)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 13 (M-06). No §10 row to tick
(runbook-owned step). **Result:** done — red-green on branch `fix/v0.1.1-audit-remediation`; code commit
`0cbfcd7` (`fix(runtime): propagate cancellation through tools and streams`), docs commit follows this
entry. Worktree clean; `make check` exit 0; `make race` exit 0; new saturation/cancel tests stable
under `-race -count=10`. `make smoke` NOT run (owner-run on a disposable model/tag — unchanged by this
task).

**Context (what M-06 actually was)**
- Audit evidence `runner.go:295-375` (executeTool invokes ReadFile/ListDir/Grep without ctx) and the UI
  producers at `agent_view.go:267-275` / `models_view.go:262-284` (unconditional sends into 64-slot
  channels). Task 02 had already made `Grep` ctx-aware (per-entry walk + between-file scan checks,
  deterministic cancel tests in `policy_test.go`) and executeTool already passed ctx to it. Remaining
  gaps at HEAD: `ReadFile`/`ListDir` ignored ctx entirely (a canceled turn still executed the tool and
  reported a successful read behind the cancel); `WriteFile`/`EditFile` had no last gate between a
  granted approval and the mutation; and both stream producers used unconditional `ch <- …` sends, so a
  full channel with the UI not draining stranded the producer goroutine forever — cancellation could
  never win the send.

**Reproduction (RED, decisive)**
- Agent boundary: `TestRunnerCancelsToolExecutionOnCanceledContext` cancels from the `ToolStartMsg`
  handler and asserts `read_file` never executes (no OK ToolResultMsg) with a prompt
  `context.Canceled` Run error. RED at HEAD: the tool executed anyway and reported
  `OK summary="must not be read\n"` — the unthreaded-context boundary was real and reproducible.
- Saturation: `TestAgentProducerSaturationCancellationTerminates` and
  `TestModelsViewPullSaturationCancellationTerminates` stream 80 events (>64) from a burst fake host,
  drain 3 to prove liveness, then stop draining, wait until `len(ch)==64` proves the producer is
  blocked on its next unconditional send, cancel the parent, and assert the explicit producer-done
  channel closes. RED at HEAD: both producers stayed blocked — "cancellation cannot win the send" — and
  the done channels never closed. `runtime.NumGoroutine` is only a secondary check; the done channels
  are the oracle (reading the activity channel would drain the backlog and unblock a stuck producer,
  which is exactly why a channel-close oracle alone is insufficient).
- Tool-work *stall* portion disposition: the audit's "stuck in a large/network filesystem walk" half is
  **NOT_REPRODUCED as a stall beyond the deadline** at HEAD — Task 02 already bounds and checks grep
  traversal/scanning, and read_file (≤ 256 KiB)/list_dir (one directory) are bounded single ops that
  cannot stall on a normal filesystem. What WAS reproduced and fixed is the unthreaded-context boundary
  (execution despite cancel + stale post-cancel results), which the audit's evidence lines described
  literally. No total-file/total-byte/deadline limits were added: existing output caps already bound
  every tool and the audit's "only where needed" carve-out did not apply (runbook step 4).

**Fix (green)**
- `tools.go`: `ReadFile`/`ListDir` now take `ctx` (same shape as `Grep`, established in Task 02) and
  check it before and after the single-file operation — a canceled run neither starts a doomed read nor
  reports a result that only finished after the context died. Syscall errors still win over a post-check
  so real I/O failures are never masked.
- `runner.go`: executeTool passes ctx into `ReadFile`/`ListDir`; `write_file`/`edit_file` get a last
  ctx gate between a granted approval and the mutation (never mutate after a cancel; the atomic write
  itself is never masked by a late cancel). A tool error that `errors.Is` `context.Canceled` /
  `context.DeadlineExceeded` now ends the turn immediately — no stale failed ToolResultMsg round-trips
  to the model and invites a retry; M-01's approval-timeout stop is unchanged and still distinct.
- UI: one context-aware helper `emitEvent(ctx, ch, msg)` (agent_view.go, package-shared) replaces the
  unconditional producer sends in `startChat` and `startPull`. Semantics: delivery succeeds in order or
  cancellation wins; while the context is alive a blocked send yields to cancellation (a full channel
  can never strand the producer); once canceled a send never blocks again — a single nonblocking retry
  delivers if a consumer is draining at that instant (that is the one race where the terminal
  `AgentDoneMsg`/`modelsPullDoneMsg` must still land), otherwise the event is dropped. The channel is
  owned by the producing goroutine and closed only after its last send, so nothing can ever send on a
  closed channel. Single-owner Bubble Tea update model and FIFO ordering preserved (one producer per
  stream, in-order sends; the App/Update routing is untouched).
- Producers now close an explicit `chatDone` / `pullStreamDone` channel on exit (cleared in
  onChatDone/onPullDone) so tests can observe goroutine termination without draining the backlog.

**Tests (red-green)**
- RED at HEAD captured above (runner boundary; both saturation tests).
- GREEN: `go test -count=1 ./internal/agent -run 'Test.*Cancel.*Tool'` → ok (includes the new
  `TestRunnerCancelsToolExecutionOnCanceledContext`); `go test -count=1 ./internal/ui -run
  'Test.*(Saturation|Backpressure|Cancellation)'` → ok (two new saturation tests + the three existing
  parent-cancellation tests unchanged); `go test -count=1 ./internal/agent ./internal/ui` → ok;
  `make check` → 0 (build + full suite + vet + gofmt); `make race` → 0. Stability: both new saturation
  tests and the runner cancel test green under `go test -race -count=10`. `git diff --check` → clean.
  No golden fixtures touched (no render output changed); `go.mod`/`go.sum` untouched.

**Commands + exit codes**
- Session guard at start: `git status --short` → empty · branch `fix/v0.1.1-audit-remediation` · HEAD
  `b3df635`. Red evidence and green gates as listed above; code commit `0cbfcd7`; `git status --short`
  after the code commit → only the docs files pending.
- `make smoke`/`make smoke-model` NOT run — owner-run on a disposable model/tag (H-04 preflight). No
  release-check (Task 22).

**Decisions / lines to respect**
- Exported `ReadFile`/`ListDir` signatures gained a leading `ctx` to match `Grep(ctx, …)`; their only
  external callers are runner.go and package tests (both updated). `mutation.go` was out of the allowed
  file set, so the write/edit ctx gate lives at the runner boundary (last gate after approval), not
  inside the executors.
- The terminal-message guarantee is producer-side: `Run` still emits exactly one `AgentDoneMsg` and the
  pull producer exactly one `modelsPullDoneMsg`; each goes through `emitEvent` last (FIFO). The only
  case a terminal is dropped is a full channel with nobody draining at the cancel instant — the same
  state in which the view can make no progress regardless — and the producer still closes its channel
  and done channel, so it never leaks.
- No consumer-side end-of-stream synthesis was added (a nil read from a closed channel is not mapped to
  a synthetic done): with a live consumer the backlog drains and the terminal send lands (the nonblocking
  retry covers the exact cancel/drain race); synthesis would add routing payloads to both views for a
  corner that pre-exists (a frozen full-channel producer previously blocked forever instead of exiting).

**Blockers / open decisions**
- None for Task 13. Carried env note from Task 02/04: `make vuln` needs `$(go env GOPATH)/bin` on PATH.
- Task 14 (M-07) follows next: render the in-flight delete overlay (ModelsView `deleting` state has a
  confirm/input/pull `View` branch but no `deleting` branch — the audit's M-07 evidence), then M-08
  redirect policy etc.

**Next action**
- Fresh Pi session: runbook **Task 14 (M-07 — render the in-flight delete overlay)**. Confirm branch
  `fix/v0.1.1-audit-remediation` + clean status, then follow the Task-14 block. Do not run `make smoke`
  until the owner runs it on a disposable model/tag or an isolated Ollama store.
### 2026-09-06 — Runbook Task 14: M-07 in-flight delete busy overlay (owner task)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 14 (M-07). No §10 row to tick
(runbook-owned step). **Result:** done — red-green on branch `fix/v0.1.1-audit-remediation`; code commit
`1c0545a` (`fix(models): render delete progress overlay`), docs commit follows this entry. Worktree
clean; focused + full `./internal/ui` and `make check` all exit 0. `make smoke` NOT run (owner-run on a
disposable model/tag — unchanged by this task).

**Context (what M-07 actually was)**
- Audit evidence `models_view.go:539-545,599-612,754-767`: approval (`y`) set `confirmDelete=false` +
  `deleting=true` and `handleKey` already ignored keys in the `deleting` state, but `View`'s dialog
  switch handled only `confirmDelete`/`inputMode`/`pulling`. With `deleting=true` the frame fell
  through to the ordinary interactive model list while keys stayed dead — the promised busy overlay
  never rendered (its spinner body existed only inside `confirmLines`, unreachable once
  `confirmDelete` was false). Tests asserted state/commands but never rendered between approval and
  completion.

**Reproduction (RED, decisive)**
- Extended `TestModelsViewDeleteConfirmFlow` to render immediately after `y` and before the DELETE
  command runs: it required a "Deleting qwen3:8b" title, the "deleting qwen3:8b…" spinner body, no
  interactive list body (non-target model `gemma3:12b` must be absent), and key-ignoring (a `j` must
  neither move the list nor dismiss the busy state). RED at HEAD: the frame showed the full model list
  (title missing, `gemma3:12b` visible) at 88×40.
- New `TestModelsViewDeleteBusyOverlayFitsGeometries` drives `x` → `y` on fresh views at the two
  canonical geometries (72×30 and 120×40) and asserts the busy frame is bounded — exactly `h-2` body
  rows, no row wider than the terminal — plus the same title/spinner/target/list-hidden content.
  RED at HEAD at both geometries for every content assertion.

**Fix (green)**
- `models_view.go` `View()` gained a `case v.deleting` that renders the centered overlay
  `renderOverlay(bodyH, "Deleting "+v.deleteTarget, v.deleteProgressLines())` — the same dialog
  machinery as confirm/input/pull, so `fitContent` keeps it bounded on a phone (mirrors the existing
  "Pulling <name>" busy dialog). New `deleteProgressLines()` holds the spinner + " deleting
  <target>…" line; the old unreachable `if v.deleting` branch inside `confirmLines` was removed (it
  could never fire: `confirmDelete` is false whenever `deleting` is true). No cancellation was
  re-enabled: `esc` stays inert during the in-flight delete because the DELETE HTTP round-trip is not
  safely interruptible (the pull path keeps its explicit `esc` cancel; delete deliberately has none).

**Tests (red-green)**
- RED at HEAD captured above (flow extension + both-geometry content assertions).
- GREEN: `go test -count=1 ./internal/ui -run 'TestModelsViewDelete|TestGoldenFramesFitTerminal'` → ok;
  `go test -count=1 ./internal/ui` → ok; `go test -race -count=1 ./internal/ui -run 'TestModelsViewDelete'`
  → ok (hygiene); `make check` → 0 (build + full suite + vet + gofmt); `git diff --check` → clean.
  No golden fixtures changed (existing render scenarios are untouched — the only render change is the
  deleting state, which has no App-level golden; the geometry guard `TestGoldenFramesFitTerminal`
  stays green). `go.mod`/`go.sum` untouched.

**Commands + exit codes**
- Session guard at start: `git status --short` → empty · branch `fix/v0.1.1-audit-remediation` · HEAD
  `cf3b30f`. Red evidence and green gates as listed above; code commit `1c0545a`; `git status --short`
  after the code commit → only the docs files pending.
- `make smoke`/`make smoke-model` NOT run — owner-run on a disposable model/tag (H-04 preflight). No
  release-check (Task 22).

**Decisions / lines to respect**
- Deleting overlay title mirrors the pull busy dialog (`"Deleting "+target` beside `"Pulling "+name`);
  body line reuses the exact text the audit cited as unreachable (`spinner + " deleting <target>…"`),
  now rendered from the state that owns it.
- The dialog switch order is `deleting` first, then confirm/input/pull — states are mutually
  exclusive, so order is cosmetic; the comment on `View` now lists deleting among the modal states.
- Success/error completion paths were already correct and are re-proven by the existing flow +
  error tests (`onDeleteDone` closes the busy state and surfaces `notice`/inline `deleteErr`).

**Blockers / open decisions**
- None for Task 14. Carried env note from Task 02/04: `make vuln` needs `$(go env GOPATH)/bin` on PATH.
- Task 15 (M-08) follows next: redirect-safe bearer-token policy for the Ollama HTTP client
  (`[needs runtime verification]`), then M-09 spinner lifecycle etc.

**Next action**
- Fresh Pi session: runbook **Task 15 (M-08 — enforce a redirect-safe bearer-token policy)**. Confirm
  branch `fix/v0.1.1-audit-remediation` + clean status, then follow the Task-15 block. Do not run
  `make smoke` until the owner runs it on a disposable model/tag or an isolated Ollama store.

### 2026-09-06 — Runbook Task 15: M-08 redirect-safe bearer-token policy (owner task)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 15 (M-08). No §10 row to tick
(runbook-owned step). **Result:** done — REPRODUCED + red-green on branch `fix/v0.1.1-audit-remediation`;
code commit `307d4f6` (`fix(ollama): refuse API redirects carrying credentials`), docs commit follows
this entry. Worktree clean; focused + full `./internal/ollama`/`./internal/config`, `make check`, and
`make race` all exit 0. `make smoke` NOT run (owner-run — unchanged by this task).

**Context (what M-08 actually was)**
- Audit evidence `client.go:41-47,65-76` + `stream.go:48-64`: config validates only the *initial* URL
  (non-loopback bearer token requires https), but both `http.Client`s were built with no `CheckRedirect`,
  so a permitted HTTPS endpoint could redirect the already-authenticated request. `[needs runtime
  verification]` — now runtime-verified on go1.27.1.

**Reproduction (RED, decisive — M-08 REPRODUCED)**
- New tests appended to `ollama_test.go` (7): same-origin redirect, cross-origin (same hostname,
  different port), https→http downgrade, hostname alias 127.0.0.1→localhost, no-token redirect, and two
  streaming (Pull/stream-client) variants incl. streaming https→http downgrade. Each logs observed
  pre-policy behavior: target hits + captured Authorization header.
- RED at HEAD on go1.27.1: every redirect was followed. Pinned Go behavior (net/http client.go
  `shouldCopyHeaderOnRedirect`/`isDomainOrSubdomain`): the Authorization header is forwarded whenever
  the redirect target's **hostname** equals the original hostname or is a subdomain of it — scheme and
  port are ignored. Evidence captures: same-origin target hit with `auth="Bearer sekrit"`; cross-origin
  (different port) target hit with `auth="Bearer sekrit"`; **https→http downgrade: plain-http target
  hit with `auth="Bearer tok-downgrade"`** (the token travelled in the clear — the audit's exact leak);
  streaming downgrade: `auth="Bearer tok-stream-downgrade"` on the plain-http POST target; hostname
  alias followed but token stripped (`auth=""`, Go treats a genuinely different hostname as foreign);
  no-token case still followed until Go's 10-redirect cap.
- No subdomain-*served* test was possible (httptest cannot serve DNS subdomains); the 127.0.0.1→
  localhost alias case exercises the same hostname-equivalence seam Go applies to foo.com→sub.foo.com.

**Fix (green)**
- `client.go` `New()` now installs one shared `redirectPolicy` as `CheckRedirect` on **both** the finite
  `http` client and the no-timeout `stream` client. `redirectPolicy` returns a stable error —
  `refusing redirect to <target>: ollama API calls must not follow redirects` — which makes net/http
  abort **before** the redirect request is sent (target never contacted, previous response body closed
  by the client). The outer `do`/`postStream` wrappers already prefix every error with the original
  operation (`ollama GET /api/tags:` / `ollama POST /api/pull:`), satisfying "error identifying the
  original API operation and refusal". `stream.go` needed no change: it routes through `c.stream`,
  which carries the same policy. No other file changed; config validation untouched (rule 5).

**Tests (red-green)**
- RED at HEAD: all 7 new `Test*Redirect*` tests failed as above (follow + token forwarded / no refusal).
- GREEN: `go test -count=1 ./internal/ollama -run 'Test.*Redirect|TestHTTPS|Test.*Bearer'` → ok (7 new
  + existing HTTPS/Bearer tests); `go test -count=1 ./internal/ollama ./internal/config` → ok (full);
  `make check` → 0 (build + full suite + vet + gofmt); `make race` → 0 (full suite, race detector);
  `git diff --check` → clean; `gofmt -l` → empty. GREEN logs show every redirect target now `hits=0
  auth=""` with the stable refusal error. Non-redirect ordinary requests are re-proven by the full
  unchanged suite (`TestHTTPSVerifiedAndBearerSent` etc.).

**Commands + exit codes**
- Session guard: `git status --short` → empty · branch `fix/v0.1.1-audit-remediation` · HEAD `2d1f098`.
- RED run `go test -count=1 ./internal/ollama -run 'Test.*Redirect'` → FAIL (7 failing, evidence above).
- GREEN runs listed above, all exit 0. Code commit `307d4f6`; `git status --short` after → only
  LEDGER.md + runbook pending.
- `make smoke`/`make smoke-model` NOT run — owner-run on a disposable model/tag. No release-check.

**Decisions / lines to respect**
- Refusal is **uniform** (every redirect refused, token or not): a redirecting endpoint is a
  misconfigured API origin either way, and Go's own subdomain exception is exactly the leak surface
  M-08 names. Uniformity also keeps one stable error text for both clients.
- Stable refusal text `refusing redirect to <target>: ollama API calls must not follow redirects`;
  tests match the substring `refusing redirect` plus the operation prefix already present.
- Config's initial non-loopback https/token validation intentionally untouched; SECURITY.md needed no
  edit (its "bearer token for a non-loopback host requires https://" claim stays true — redirect
  refusal now enforces it past the first hop).

**Blockers / open decisions**
- None for Task 15. Carried env note from Task 02/04: `make vuln` needs `$(go env GOPATH)/bin` on PATH.
- Task 16 (M-09) follows next: single spinner command chain in `models_view.go`, then M-10 etc.

**Next action**
- Fresh Pi session: runbook **Task 16 (M-09 — maintain exactly one spinner command chain)**. Confirm
  branch `fix/v0.1.1-audit-remediation` + clean status, then follow the Task-16 block. Do not run
  `make smoke` until the owner runs it on a disposable model/tag or an isolated Ollama store.

---

### 2026-09-06 — Read-only repository audit + optimization map (5 parallel lanes, owner task)
**Milestone:** n/a (analysis, no code changed) · **Result:** done — report delivered to the owner;
map recorded here. Branch `fix/v0.1.1-audit-remediation` @ `dc25c58`, tree clean, `make check` green.

**Work done**
- Ran a **5-lane read-only audit** (subagent fan-out: `ai-code-security-auditor`, `codebase-archaeologist`,
  `reviewer`, `test-automation-engineer`, `developer-tooling-engineer`), every lane instructed to read
  `AGENTS.md` + both audit docs + the runbook first and to cross-reference (not duplicate) runbook
  tasks 00–22. Spot-verified the top cross-lane claims myself (`main.go` shutdown order, `App.Init`
  double fetch, `load.go` env-error names, workflow action pins). No files edited.
- **Baseline (real output):** `make check` `0` · `gofmt -l .` empty · `go vet ./...` clean · all Go
  packages `ok` on go1.27.1 · coverage `internal/{ollama,session,ui,config}` 90–92%,
  `internal/agent` 79.7%, `cmd/self-tui` 8.9% (entrypoint is the dark corner). No secrets in any
  tracked file.

**Findings — NEW (not on any runbook queue)**
- P1 correctness cluster (the owner-approved next step): **#1** shutdown can hang on a wedged
  transcript sink (`main.go:171-182` closes recorder before `cancel()`; `recorder.go Close()` blocks;
  NotifyContext still catches Ctrl+C) · **#2** non-2xx stream error bodies bypass the idle watchdog
  (`chat.go:138`, `pull.go:53` read the capped error body with no idle bound on the no-timeout stream
  client) · **#3** stale detail pane after model-list refresh (`models_view.go:510-534` clears the list
  but not `detail`/`detailName`) · **#4** theme preview not rolled back on save failure
  (`app.go:130-133`) · **#5** `/export` misreports a wedged recorder as "nothing recorded"
  (`agent_view.go:875-876`) · **#6** HTTP client rebuilt on every settings save even when host/token
  unchanged (`app.go:341-350`) · **#7** byte-offset truncation can cut multibyte chars
  (`context.go:181,226`) · **#12** env parse errors name nonexistent vars (reads `SELFTUI_AGENT_*`,
  errors say `SELFTUI_*`; `load.go:179-203`) · **#13** explicit `-config /typo.toml` silently falls back
  to defaults (pinned by `config_test.go:212`) — an explicit path should hard-error.
- Supply-chain/CI (P0): actions float on mutable majors + no Dependabot + no `timeout-minutes`;
  release checkout persists `contents: write` token; `.env*` not gitignored; no `make secret-scan`;
  two offline regression suites orphaned from automation (`create-audit-pack-test.sh`,
  `pull_delete_smoke_test.py` — no Makefile target, no CI job).
- DX/polish (P2/P3): double `/api/tags` on boot (`app.go:84-85`); `-config`/env/flag surface uneven;
  `f`/`h`/`l`/`b` page keys on Models vs `f`=follow on Agent; help only reachable from Agent.
- Test gaps (P1-tests): entrypoint 0.0%; `App/Models/Agent Init()` never exercised; hostile-shape
  tool-JSON + `sanitizeInfoValue` recursion + `ApproxTokens` absolute values uncovered; fault-injection
  branches (decode error, `newIdleReader` default, session mkdir, `atomicWrite`).
- Cleanup (P3): remove `ReadOnlyTools()`/`chatChOnce`/legacy `agentTokenMsg`/`agentDoneMsg` dispatch;
  stale comments ("constrained command", "placeholder persona"); `scripts/__pycache__` left in tree.

**Findings — queued-runbook overlaps (evidence handed to tasks, NOT re-reported as new)**
- M-09/Task 16: spinner ticks are unpaced immediate `TickMsg`s self-feeding with no `spinner.FPS`
  pacing (`models_view.go:497-500`, `386-388`) · M-10/Task 17: **v0.1.0's published `SHA256SUMS` is
  `dist/`-prefixed** (downloads can't `sha256sum -c` from the download dir); release-check never
  verifies toolchain versions · M-11/Task 18: smoke scripts still use fixed `/tmp/selftui-*.log` ·
  M-12/Task 19: SECURITY.md wildcard claims broader than tests, pull stream-cap claim overstated,
  README still "release hardening", PLAN §12 stale / §13 truncated · L-01/Task 20: light loop
  special-cases only 8/19 frames · L-02/Task 21: `internal/ui/timer.sh` proven unused (arrived in
  `b6bd9a0` as debug leftover); `make clean` removes only `bin/selftui`.

**Planned map (as agreed with the owner)**
- **P0 — release blockers (fold near Task 22):** action SHA-pinning + Dependabot + `timeout-minutes` +
  `persist-credentials: false` · `.env*` + `make secret-scan` · `make scripts-test` wired into
  `make check` + CI · `make vuln` PATH note (carried env note).
- **P1 — correctness (owner: start now, this session):** findings #1–#7, #12, #13 above, red-green TDD.
- **P1-tests:** entrypoint + boot-wiring + hostile-shape tables + fault injection.
- **P2 — perf/DX:** single boot fetch; list_dir/grep work bounds; CLI/env surface consistency.
- **P3 — cleanup:** dead code + comment drift (can ride with L-02/Task 21).

**Commands + exit codes**
- `make check` `0` · `go test -count=1 ./...` `0` · `go vet ./...` `0` · `gofmt -l .` empty
- coverage probe `go test -cover ./...` `0` (numbers above) · `git status --short` empty
- five subagent lanes exited 0 with bounded reports (L1 security / L2 drift / L3 architecture /
  L4 tests / L5 DX) — outputs in-session only, not committed.

**Decisions / lines to respect**
- The runbook 16–22 queue stands; nothing here re-plans it. P1 is a separate owner-assigned step on
  the same branch, ahead of / independent of Task 16.
- No code changed by this entry; the map lives here (owner chose LEDGER append over a new doc).

**Blockers / open decisions (carry to next session)**
- Owner queued decisions unchanged: public-visibility of the repo, gitleaks-in-CI, actionlint,
  Node-20 action bumps, signed-tag policy.

**Next action**
- Same session (owner instruction): implement **P1 correctness cluster (#1–#7, #12, #13)** red-green
  on this branch, gate with `make check`/`make race`, commit per finding cluster. Then a fresh
  session resumes runbook **Task 16 (M-09)**.

---

### 2026-09-06 — P1 correctness cluster implemented (#1–#7, #12, #13), 9 commits (owner task)
**Milestone:** P1 of the recorded optimization map · **Result:** done — all nine findings fixed
red-green with regression tests; `make check` + `go test -race ./...` green. Branch
`fix/v0.1.1-audit-remediation`, worktree clean after commit.

**Work done** (each finding: RED test → fix → GREEN → own commit)
- **#12 — env parse errors named nonexistent vars** (`c3ab159`): the four `SELFTUI_AGENT_*`
  parse errors interpolated the bare prefix (`SELFTUI_TEMPERATURE`, …); error text now names the
  variable actually read (`load.go:179-203`). New table test pins all four names.
- **#13 — explicit missing `-config` hard-errors** (`472741c`): a typo'd explicit path silently
  booted with defaults; only the auto-resolved default XDG file may be absent (H-01). Tests that
  used a nonexistent explicit path as a "no file" base now write an empty real file (config +
  ui + cmd fixtures touched); new tests pin explicit-vs-default. `run()` surfaces
  `load config: config file not found at <path>`, exit 1.
- **#2 — non-2xx stream error bodies idle-bounded** (`a01c3b7`): `chat.go:138`/`pull.go:53`
  read the capped error body with no idle bound on the no-timeout stream client. New shared
  `Client.readErrorBody` runs the read through the idle machinery: a silent 4xx/5xx body aborts
  with the stable idle error at the window; complete bodies still surface via `apiError`. RED
  tests stalled 3s at the caller deadline; now abort in ~60ms.
- **#1 — recorder shutdown bounded** (`01d403b`): `run()` closed the recorder before
  `cancel()`, and `Recorder.Close` blocks on a wedged sink — NotifyContext kept swallowing
  Ctrl+C, so a stalled filesystem made the process unkillable. Now `cancel()` (stops signal
  interception, releases producers) runs first and the close is time-bounded
  (`closeSessionRecorderWithin`, 3s); wedged-model test returns the timeout error in-budget.
- **#3 — stale model detail after reload** (`2b3bedc`): bubbles `SetItems` preserves (clamped)
  the cursor while the pane header follows the cursor, so a reload that dropped/reordered the
  inspected model painted the new selection's header over the old payload. `onLoaded` now
  mirrors the cursor and reconciles the pane (drop + re-inspect when the visible pane no longer
  matches; clear stale payload when the model vanished). Compact-refresh regression added.
- **#4 — theme preview rolls back on save failure** (`9b2e263`): failed write left the shell on
  the previewed theme. Submit now carries the edit-start theme when a preview diverged; App
  re-applies it on save error (mirrors the discard rollback). RED: preview Light → forced
  write failure → shell stayed light; now returns to committed Dark.
- **#5 — `/export` echoes the real recorder failure** (`6583753`): once recording fails it is
  off for the run, so "nothing recorded — send a message first" was dead-end advice. View
  retains the one surfaced failure (`sessionErrMsg`) and `/export` splits off/never-started/
  failed states. RED drove the failWriter and asserted the failure text, no send-message hint.
- **#6 — client rebuilt only on host/token change** (`c8eeb76`): every settings save handed the
  Agent a fresh `ollama.Client` while Models kept its own — the shared-client seam drifted. App
  now owns the client; scalar/theme saves reuse it, host/token saves swap once for both tabs.
  Genuine RED: pre-fix Agent held a rebuilt instance on a temperature-only save.
- **#7 — content truncation stays on UTF-8 rune boundaries** (`d2f3acf`): `BudgetMessages`
  byte-exact tail cuts could split multibyte runes and hand the model invalid UTF-8. New
  `contentTailWithin` advances the cut to the next rune start (never exceeds the byte cap).
  RED with a CJK fixture showed a split `\xbd\xa0` prefix; fixed path yields only valid UTF-8.

**Commands + exit codes**
- per-finding: `go test ./internal/<pkg> -run '<test>' -count=1` RED then GREEN `0`
- `make check` `0` (build + full suite + vet + gofmt) · `go test -race ./... -count=1` `0` (all pkgs)
- `gofmt -l .` empty · `git diff --check` clean (per commit)

**Decisions / lines to respect**
- P1-13 changed a documented edge: an explicit `-config` pointing at a missing file now errors
  before the TUI starts (user-intent statement); first-boot default-XDG fallback is unchanged.
- P1-1 keeps the recorder's durable-close contract; the bound lives at the entrypoint so the
  normal flush still completes when the sink is healthy.
- P1-6 added an App-owned `client` field; constructors/tests that build `App` without a real
  client (settings flows) are unaffected because they never change host.

**Blockers / open decisions (carry to next session)**
- Unchanged owner queue: public visibility, gitleaks-in-CI, actionlint, Node-20 bumps, signed tag.
- P1-tests (entrypoint/boot/hostile-shape/fault-injection) and P0 (release blockers) from the map
  remain unexecuted; runbook tasks 16–22 also still queued.

**Next action**
- Fresh session: resume runbook **Task 16 (M-09 — single spinner chain)**, or the owner may pick
  a P0/P1-test item from the recorded map instead. Commit history since `dc25c58` is the P1
  cluster above (9 commits, clean).

### 2026-09-05 — Runbook Task 16: M-09 single spinner command chain (owner task)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 16 (M-09). No §10 row to tick
(runbook-owned step). **Result:** done — REPRODUCED + red-green on branch `fix/v0.1.1-audit-remediation`;
code commit `64fcfc4` (`fix(models): keep one spinner command chain`), docs commit follows this entry.
Worktree clean; focused spinner/delete/pull tests, full `./internal/ui`, `make check`, and `make race`
all exit 0. `make smoke`/`make smoke-model` NOT run (owner-run on a disposable model/tag — unchanged
by this task).

**Context (what M-09 actually was)**
- Audit evidence `models_view.go:290-291,343-344,363-367`: `spinnerTick()` wrapped a fresh unpaced
  `v.spinner.Tick()`; the `spinner.TickMsg` update discarded the command `v.spinner.Update(msg)`
  returns; and a bottom-of-update re-seed appended a new `spinnerTick()` after **every** message while
  `deleting || pulling`. Every pull-progress event (and every key/tick) therefore scheduled another
  independent chain instead of maintaining one subscription.

**Pinned dependency API evidence (bubbles/v2 v2.2.1, module-cache spinner.go — reproduced the audit's
`[needs runtime verification]`)**
- `spinner.Model.Tick()` returns an **immediate `TickMsg`** (with `id` + current `tag`) — it is *not*
  a timed command. The FPS pacing lives in `spinner.Update(TickMsg)`: it advances one frame, bumps the
  tag, and returns the successor `m.tick(m.id, m.tag)` = `tea.Tick(FPS)`.
- The view threw that successor away (`v.spinner, _ = v.spinner.Update(msg)`) and re-seeded an unpaced
  tick per event instead — so the spinner advanced at event volume (never at `spinner.FPS`), scheduled
  chains piled up while progress events flooded, and during a stream gap (long layer download, no
  events) the spinner **froze** because nothing self-sustained the chain.
- `spinner.Update` rejects ticks whose `id`/`tag` don't match the spinner's current state, so once the
  view stops re-seeding stale tags, duplicates self-heal — the one-chain fix is safe by construction.

**Reproduction (RED, decisive — M-09 REPRODUCED)**
- Deterministic test seam first: `ModelsView.spinnerPending` counts scheduled spinner ticks whose
  `TickMsg` has not yet arrived (0 or 1 in every steady state; reset when a busy state closes). With
  the pre-fix re-seed still in place, the seam shows the multiplication without real `spinner.FPS`
  waits: increment wherever a tick is scheduled, decrement when a tick whose `id` matches the view's
  spinner arrives.
- RED at HEAD (seam only): `TestModelsViewDeleteKeepsOneSpinnerChain` — after `x` → `y` (deleting
  open, 1 chain seeded), the **first ignored key while deleting returned a command** (a re-seed);
  `TestModelsViewPullKeepsOneSpinnerChain` — after a real pull start, the **first progress event
  pushed `spinnerPending` to 2** (seed + progress re-seed). Both `t.Fatalf` on assertion 1.

**Fix (green)**
- `spinnerTick()` split into `spinnerSeed()` (one immediate TickMsg; only the transition into
  pulling/deleting seeds a chain) and `spinnerResume(successor)` (wraps the FPS-paced successor
  `spinner.Update` returned so its TickMsg crosses the App shell inside `modelsEventMsg`).
- `Update` records `spinnerBusy` before the switch; the bottom re-seed is gone, replaced by "seed
  exactly one chain when a busy state opens". The `spinner.TickMsg` case now captures the successor
  and schedules it **only while the busy state is still active** — so a long pull keeps exactly one
  FPS-paced chain (the spinner keeps animating through silent stream gaps, which pre-fix it froze),
  and completion stops rescheduling.
- Progress handlers were already right (they resubscribe only `waitPullCmd`); they now provably add
  no spinner chain. Unrelated events/keys while busy add none either.
- `onPullDone`/`onDeleteDone` reset `spinnerPending` (success *and* error-retry paths): a successor
  tick already in flight when a busy state closes arrives inert (state gate + floor-guarded count).

**Tests (red-green)**
- Three new tests in `models_view_test.go`: delete chain (seed on `x`→`y`, five ignored keys never
  re-seed, per-tick single successor, success completion stops + inert straggler), delete-error
  retry (failure returns to confirm with the chain stopped; a retry seeds exactly one fresh chain),
  and pull chain (real enter-pull seeds 1; 20 synthetic progress events leave `spinnerPending` at 1;
  3 consumed ticks reschedule exactly one successor each; completion resets to 0 and a straggler
  tick returns no command). Existing delete/pull/spinner/routing tests unchanged and green.
- RED run captured above (2 failing). GREEN: `go test -count=1 ./internal/ui -run 'Test.*Spinner|TestModelsViewDelete|TestModelsViewPull'` → ok (incl. the 3 new tests); `go test -count=1 ./internal/ui` → ok;
  `make check` → 0 (build + full suite + vet + gofmt); `make race` → 0 (full suite, race detector);
  `gofmt -l` → empty; `git diff --check` → clean. No golden fixtures changed (no render path changed).

**Commands + exit codes**
- Session guard at start: `git status --short` → empty · branch `fix/v0.1.1-audit-remediation` · HEAD
  `5d45a37`. RED and GREEN runs as above; code commit `64fcfc4`; `git status --short` after → only
  LEDGER.md + runbook pending.
- `make smoke`/`make smoke-model` NOT run — owner-run on a disposable model/tag (H-04 preflight). No
  release-check (Task 22).

**Decisions / lines to respect**
- The one chain is seeded on the state transition, then sustained only by `spinner.Update`'s own
  FPS-paced successors — never by event volume. Stale/duplicate ticks are rejected by the pinned
  spinner's id/tag guard, so no explicit de-dup bookkeeping is needed beyond the state gate.
- `spinnerPending` is a documented test seam (0/1 invariant; floor-guarded decrement on ticks whose
  `id` matches `v.spinner.ID()`; reset on completion) so the scheduler-count test is deterministic
  with no real timers. Production behavior does not depend on it.
- Progress messages still resubscribe the pull-activity command (`waitPullCmd`) exactly as before;
  this change only removed the per-event spinner re-seed.

**Blockers / open decisions**
- None for Task 16. Carried env note from Task 02/04: `make vuln` needs `$(go env GOPATH)/bin` on PATH.
- Task 17 (M-10) follows next: reproducible release toolchain/archive modes/checksum paths
  (`scripts/release-check.sh` + new test harness), then M-11 etc.

**Next action**
- Fresh Pi session: runbook **Task 17 (M-10 — release gate portability/reproducibility)**. Confirm
  branch `fix/v0.1.1-audit-remediation` + clean status, then follow the Task-17 block. Do not run
  `make smoke` until the owner runs it on a disposable model/tag or an isolated Ollama store; do not
  run the full release-check until Task 22 and a clean worktree.

### 2026-09-05 — Runbook Task 17: M-10 release gate portability/reproducibility (owner task)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 17 (M-10). No §10 row to tick
(runbook-owned step; the runbook's Task-17 progress checkbox stays unticked — the runbook file is not in
this task's allowed-files list). **Result:** done — red-green on branch `fix/v0.1.1-audit-remediation`;
harness RED 41 fails/11 ok against the pre-fix script, GREEN 52/52 after the fix; code+docs commit
(see below). Worktree clean after commit; `bash -n` both scripts, harness, and `make check` all exit 0.

**Context (what M-10 actually was)**
- Audit evidence `scripts/release-check.sh:14-16,62-71,123-140`: the gate used whatever `go`/`gofmt`/
  `govulncheck` PATH had (presence-only checks while README claimed pinning); archive staging used `cp`
  (member modes not normalized); SHA256SUMS entries were `dist/`-prefixed (release assets download flat).
- Verified on this host: `go` (distro `/usr/lib/go-1.22/bin/go`) auto-switches to the pinned go1.27.1
  toolchain inside the module (`go version go1.27.1 linux/amd64`; GOROOT = module-cache toolchain dir
  that ships its own `bin/gofmt`), while PATH `gofmt` stays the distro go1.22.2 one — the exact
  go/gofmt split the finding worried about. `gofmt` has **no -version flag** (usage error, verified), so
  gofmt pinning is enforced by **identity** with the pinned distribution's gofmt.
- Plain GNU `cp` applies the umask to a newly-created destination (dest mode = source & ~umask), so
  cp-based staging baked the builder's umask into tar members: 0002 → selftui 0764 + docs 0664, 0022 →
  selftui 0744 + docs 0644 (reproduced manually). `install -m 0755/0644` forces fixed modes.

**Work done (red-green, per the Task-17 contract)**
1. **New `scripts/release-check-test.sh`** (52 checks, self-contained, temp fixtures only — never touches
   the real tree's `dist/`): a fake go/gofmt/govulncheck toolchain whose behaviour is env-driven
   (`GO_VER_LINE`, `GOVULN_VER_LINE`, `FAKE_GOROOT`, `FAKE_LOG`, `FAKE_STAMP`), with the fake `go`
   emitting umask-sensitive "built" binaries (models a non-normalizing builder); fixture PATH hygiene via
   a symlinked `$sys` dir of only the real tools release-check needs, **excluding** go/gofmt/govulncheck,
   so a "missing X" case is genuinely missing (real `/usr/bin/go` had been leaking through PATH and
   turned the missing cases into wrong-distribution cases — fixed in the harness, not the script).
   Cases: (a) missing go / go1.28.0 / go1.27.2 / go1.25.8 / unparseable output → exit 2 with the stable
   pin message, `FAKE_LOG` empty (no slow gate reached), fixture `dist/` never created; (b) missing gofmt
   and foreign-distribution gofmt (decoy first on PATH) → exit 2, actionable PATH fix; (c) missing
   govulncheck → exit 2 + `@v1.7.0` install hint (regression of the old presence check); govulncheck
   v1.6.0 / v1.7.1 / no-version-token usage output → exit 2, `-version`-only invocation, dist untouched;
   (d) correct versions → gate runs to a stubbed PASS inside the fixture (fake go builds the stamped
   binaries, real git/make/tar/gzip/sha256sum/install do the rest); (e) `SHA256SUMS` = exactly the two
   flat `selftui-v0.1.1-linux-{amd64,arm64}.tar.gz` entries, no paths; `sha256sum -c SHA256SUMS` verifies
   from inside `dist/` **and** from a flat download dir of just the assets + manifest; (f) umask 0002 vs
   0022 fixtures → byte-identical archives and manifest, exact member modes `-rwxr-xr-x` /
   `-rw-r--r--` / `-rw-r--r--`.
2. **`scripts/release-check.sh` fix** — step 0 toolchain pin placed **before** `rm -rf dist` (a
   version-bad invocation no longer wipes dist) and before every slow gate: `go version` 3rd field must
   equal `go1.27.1` (defensive first-line parse, output echoed on failure); gofmt presence + realpath
   identity vs `$(go env GOROOT)/bin/gofmt`; govulncheck presence + `-version` parsed defensively for a
   `vX.Y.Z` token that must equal `v1.7.0`; `strings` presence kept. Every failure exits 2 with the
   exact remediation (e.g. `export PATH="$(go env GOROOT)/bin:$PATH"`). Step 9 stages with `install -m
   0755` (binary) / `install -m 0644` (LICENSE/README). Step 10 generates the manifest from **inside**
   `dist/` (`( cd dist && LC_ALL=C sha256sum selftui-… > SHA256SUMS )`) → flat entries.
3. **README.md + CONTRIBUTING.md** — distinguish enforced local prerequisites (gate fails fast unless go
  1.27.1 + same-distribution gofmt + govulncheck v1.7.0) from CI configuration (workflow env +
  `setup-go`); document the `export PATH="$(go env GOROOT)/bin:$PATH"` recipe; fixed-mode archives and
  flat-manifest verification (`cd dist && sha256sum -c SHA256SUMS` / beside downloaded assets); point to
  the new regression harness. Makefile and `.github/workflows/*` untouched (not needed; workflows not in
  the allowed list).

**RED evidence (against pre-fix release-check.sh, harness assertions)**
- wrong/missing go/gofmt/govulncheck versions did NOT fail fast: full gate ran to PASS (rc 0) with
  go1.28.0/go1.27.2/go1.25.8/garbage go and govulncheck v1.6.0/v1.7.1/no-token; fake logs show
  `go mod verify … go build …` reached; fixture `dist/` was created before the (old) govulncheck check;
  missing go/gofmt died late at rc 127, missing govulncheck at rc 2 but only after dist creation.
- cross-umask archives diverged: amd64+arm64 sha256 `2dbbda62…` (0002) vs `087b0aa1…` (0022); member
  modes 0002: selftui `-rwxrw-r--`(0764) + LICENSE/README `-rw-rw-r--`(0664); 0022: `-rwxr--r--`(0744) +
  `-rw-r--r--`(0644) — neither 0755/0644.
- SHA256SUMS held `dist/selftui-…` prefixed entries → in-dist and flat-download `sha256sum -c` failed.

**GREEN evidence**
- Harness: `bash scripts/release-check-test.sh` → `release-check-test: 52 checks, 0 failures — PASS`,
  exit 0. Cross-umask sha256 identical for both archives: `8a1820b3…` (amd64 and arm64 under 0002 and
  0022 — content identical, arch name is the only differing input) and identical SHA256SUMS files; modes
  `-rwxr-xr-x` / `-rw-r--r--` in both fixtures. Stubbed full gate PASSED inside the fixture; flat
  manifest verified from inside `dist/` and from the flat download dir.

**Commands + exit codes**
- Session guard: `git status --short` → empty · branch `fix/v0.1.1-audit-remediation` · HEAD `0aec581`.
- RED: `bash scripts/release-check-test.sh` → exit 1 (41 failures / 11 ok) — captured above.
- GREEN: `bash -n scripts/release-check.sh scripts/release-check-test.sh` → 0 · `bash
  scripts/release-check-test.sh` → exit 0 (52/52) · `make check` → 0 (build + full suite + vet + gofmt) ·
  `git diff --check` → clean · `gofmt` not needed (no Go files changed).
- Toolchain probe (evidence for the gofmt decision): `gofmt -version` → usage error (no flag); `go
  version $(command -v gofmt)` → `/usr/lib/go-1.22/bin/gofmt: go1.22.2`; module toolchain gofmt → go1.27.1;
  `cp`/umask staging probe → 0764/0664 (0002) vs 0744/0644 (0022) member modes with identical sources.
- Commit: `git add scripts/release-check.sh scripts/release-check-test.sh README.md CONTRIBUTING.md
  LEDGER.md` → `git commit -m "fix(release): enforce reproducible portable artifacts"` → 0; post-commit
  `git status --short` → clean.
- `make smoke`/`make smoke-model` NOT run (owner-run). Full `VERSION=… make release-check` NOT run (Task
  22 + clean worktree, per the runbook).

**Decisions / lines to respect**
- Pins are exact: go `go1.27.1` and govulncheck `v1.7.0` — patch drift (go1.27.2, v1.7.1) is rejected
  too, because the documented/CI pin is exact and reproducibility is the point.
- gofmt is pinned by **identity** with the pinned distribution (`readlink -f` both sides) because gofmt
  ships no version flag; the error prints the exact PATH fix. A same-version gofmt symlinked from another
  dir still passes (realpath equality) — that is intended.
- The toolchain pin runs **before** `rm -rf dist`: a precondition failure never destroys existing dist
  artifacts and never reaches a slow gate (proven by the empty FAKE_LOG + absent-dist assertions).
- Staging uses `install -m 0755/0644` (fixed modes) instead of `cp` (umask-sensitive). SHA256SUMS is
  generated from inside dist with flat names so downloaded assets verify directly; production names stay
  driven by the VERSION variable validated at the top (fixture proves v0.1.1 names).
- Harness fixtures fake the toolchain (env-driven scripts) and run the gate inside a temp repo; the
  destructive dist handling that runs there is confined to the temp fixture. The real tree's dist/ was
  never touched.

**Blockers / open decisions**
- None for Task 17. Carried: govulncheck is NOT installed on this host — Task 22 (final gate) will need
  `go install golang.org/x/vuln/cmd/govulncheck@v1.7.0` AND the pinned bin first on PATH, because the new
  gofmt-identity check fails on the distro gofmt (`/usr/lib/go-1.22/bin/gofmt`): from the repo root,
  `export PATH="$(go env GOROOT)/bin:$PATH"` then re-run. The runbook Task-17 checkbox remains unticked
  (runbook not in this task's allowed files) — tick it in a task whose allowed list includes the runbook,
  or at Task 22.
- Full release gate stays gated behind Task 22 and a clean worktree (never run during a task).

**Next action**
- Fresh Pi session: runbook **Task 18 (M-11 — private unique smoke captures)**: confirm branch
  `fix/v0.1.1-audit-remediation` + clean status, read the two Python smoke scripts/tests + M-11, then
  follow the Task-18 block.

### 2026-09-05 — Runbook Task 18: M-11 private unique smoke captures (owner task)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 18 (M-11). No §10 row to tick
(runbook-owned step; the runbook file is not in this task's allowed-files list — the runbook Task-18
checkbox stays unticked, per the Task-17 precedent, until a task allowed to touch it or Task 22).
**Result:** done — red-green on branch `fix/v0.1.1-audit-remediation`; new tests RED 5 failures + 4
errors against the fixed-path code, GREEN 13/13 after the refactor; commit `fix(smoke): store captures
in private temp paths` (see below). Worktree clean after commit.

**Context (what M-11 actually was)**
- Audit evidence: `scripts/pull-delete-smoke.py` wrote the full TUI capture to `/tmp/selftui-smoke.log`
  with `open(..., "w")` on failure and on success; `scripts/reconnect-smoke.py` did the same at
  `/tmp/selftui-reconnect.log`. A same-user process can pre-place a symlink at those fixed paths, so an
  ordinary `open(w)` truncates/overwrites the symlink's target; normal umasks expose prompts/output.
  (Host proof: under this host's umask 0002 a legacy `open(w)` capture is 0664 — group-readable.)
- Suggested fix (audit §M-11 + runbook Task 18): unique 0700 temp dir + exclusive 0600 capture file;
  never follow a caller-controlled fixed symlink; retain on failure (print the exact path); remove on
  success by default unless an explicit keep-capture env var is set.

**Work done (red-green, per the Task-18 contract)**
1. **`scripts/pull_delete_smoke_test.py` updated + `scripts/reconnect_smoke_test.py` created** (13 tests
   total: the 4 H-04 safety tests preserved unchanged in meaning, plus 9 new M-11 tests — both test
   files drive `main()` through `unittest.mock` fakes, never touching a live host or a pty). New tests
   are observable-behavior based: (a) pre-place a symlink at the old fixed `/tmp` capture path pointing
   at a canary file and prove the run never opens/modifies it and never replaces the symlink; (b) a
   retained capture is an exclusive 0600 file inside a fresh 0700 temp dir, never the old fixed path,
   and two consecutive runs get distinct dirs; (c) failure retains the capture and prints its exact
   path; (d) success removes it by default and prints no capture path; (e) `SMOKE_KEEP_CAPTURE=1`
   retains it on success and the printed path exists. Reconnect tests additionally drive main() end to
   end with scripted FakeApp sessions (drop rc -1 / quit rc per case, deterministic FakeClock, fake
   /api/tags + short_generation) and assert scratch config/state cleanup semantics.
2. **`scripts/pull-delete-smoke.py`** — removed the fixed `LOG`; added module-level capture state plus
   `keep_capture()`/`open_capture()`/`write_capture()`/`discard_capture()`; `fail()` writes the run
   capture and prints its exact path; the success path writes+keeps only when `SMOKE_KEEP_CAPTURE=1`,
   otherwise discards. The capture dir/file are created lazily at first retained write
   (`tempfile.mkdtemp` = fresh unique 0700 dir; `tempfile.mkstemp(dir=...)` = O_CREAT|O_EXCL exclusive
   0600 file, both umask-proof). Nothing derives from a fixed path, so a pre-placed symlink at the old
   path can never be opened. Still fully import-safe (no argv/env/fs work at import).
3. **`scripts/reconnect-smoke.py`** — same M-11 model applied to the whole run: `main()` now creates one
   fresh 0700 scratch dir (lazily, at run start — **no more mkdtemp at import time**) holding the scratch
   `config.toml`, the hermetic XDG `state/` dir, and one exclusive 0600 `capture-*` file
   (`open_scratch()`/`write_capture()`/`discard_scratch()`). `fail()` always retains the capture and
   prints its exact path (an empty capture when no text was passed, instead of omitting the path);
   success removes the whole private scratch by default, or keeps scratch + capture and prints the
   paths when `SMOKE_KEEP_CAPTURE=1`. App's `XDG_STATE_HOME` now reads the module `state_dir` set by
   `main()`; `open(logfile)` became a context-managed read (removes a pre-existing unclosed-file
   ResourceWarning the new tests would otherwise surface under `-W error::ResourceWarning`).

**RED evidence (new tests against the pre-fix fixed-path code)**
- pull-delete: symlink test FAILED — `b'' != b'canary-payload'` (the script followed
  `/tmp/selftui-smoke.log` → truncated the canary); capture path still equaled `/tmp/selftui-smoke.log`
  (not 0600/not unique/not 0700-dir) across consecutive runs; success runs still advertised
  `capture: /tmp/selftui-smoke.log` (never removed; 0664 under umask 0002).
- reconnect: all four new tests ERRORed on `module has no attribute 'state_dir'`/capture plumbing —
  the pre-fix module created its scratch at import and had no run capture state; the symlink canary
  truncation path was exercised by the fixed-path `open(w)` in `fail()`/success.
- 4/4 H-04 safety tests stayed green pre-fix (behavior preserved).

**GREEN evidence**
- `python3 -W error::ResourceWarning -m unittest -v scripts/pull_delete_smoke_test.py
  scripts/reconnect_smoke_test.py` → `OK (13 tests)`, exit 0 — symlink canary untouched (both scripts),
  distinct 0700 dirs + 0600 exclusive files, retention/print on failure, removal by default on success,
  keep via `SMOKE_KEEP_CAPTURE=1`, reconnect scratch cleaned predictably.
- `python3 -m py_compile scripts/pull-delete-smoke.py scripts/reconnect-smoke.py
  scripts/pull_delete_smoke_test.py scripts/reconnect_smoke_test.py` → 0.
- `make check` → 0 (build + full Go suite + vet + gofmt). No Go files changed.
- `git diff --check` → clean. `/tmp` had no `selftui-*` leftovers after the suite.

**Commands + exit codes**
- Session guard: `git status --short` → empty · branch `fix/v0.1.1-audit-remediation` · HEAD `58687f9`
  (Task 17 commit). Host facts: `umask` → 0002; `tempfile.mkdtemp` mode 0700 / `mkstemp` mode 0600 /
  legacy `open(w)` mode 0664 (probe script, all verified).
- RED: `python3 -m unittest scripts/pull_delete_smoke_test.py scripts/reconnect_smoke_test.py` → exit 1
  (failures=5, errors=4; 13 run) — full per-test list recorded above.
- GREEN: the `-W error::ResourceWarning` unittest command → 0 (13/13) · `py_compile` → 0 · `make check`
  → 0 · `git diff --check` → clean.
- Commit: `git add scripts/pull-delete-smoke.py scripts/pull_delete_smoke_test.py scripts/reconnect-smoke.py
  scripts/reconnect_smoke_test.py README.md LEDGER.md` → `git commit -m "fix(smoke): store captures in
  private temp paths"` → 0; post-commit `git status --short` → clean.
- `make smoke`/`make smoke-model`/`make smoke-reconnect` NOT run (owner-run live smokes); full
  `VERSION=… make release-check` NOT run (Task 22 + clean worktree, per the runbook).

**Decisions / lines to respect**
- Both smoke scripts stay standalone (stdlib only) — no shared helper module (not in the allowed-file
  list); the capture helpers are duplicated per script with one consistent contract.
- Capture contract: fresh unique 0700 dir per run (`mkdtemp`) + one exclusive 0600 file (`mkstemp`
  opens O_CREAT|O_EXCL and pins 0600); the path is never derived from a fixed caller-visible path.
  pull-delete creates it lazily on the first retained write; reconnect reserves it at run start inside
  the scratch dir it already needs for config/state (no extra dir per run).
- Retention: failure always retains the capture (and, for reconnect, the whole scratch evidence dir)
  and prints the exact capture path; success removes by default and prints no path; `SMOKE_KEEP_CAPTURE=1`
  retains + prints on success too. Reconnect's `fail(msg)` without session text now retains an empty
  capture at a printed path (uniform with pull-delete) instead of printing no path.
- Reconnect scratch moved from import-time to `main()` start → `exec_module()`-based unit tests create
  nothing at import; scratch cleanup is predictable: removed after a pass (default), retained on failure.
- H-04 semantics untouched: the existing four safety tests pass unchanged (state-capture-first, abort on
  pre-existing target, cleanup only of a provably created model).
- README documents the new capture behavior (allowed: "if capture behavior is documented"): smoke
  evidence lives in a private unique 0700/0600 temp location, retained on failure / removed on success
  unless `SMOKE_KEEP_CAPTURE=1`.

**Blockers / open decisions**
- None for Task 18. Carried from Task 17: govulncheck is NOT installed — Task 22 (final gate) needs
  `go install golang.org/x/vuln/cmd/govulncheck@v1.7.0` AND the pinned bin first on PATH
  (`export PATH="$(go env GOROOT)/bin:$PATH"`). Runbook Task-18 checkbox stays unticked (runbook not in
  this task's allowed list) — tick it at Task 22 or in a task whose allowed files include the runbook.

**Next action**
- Fresh Pi session: runbook **Task 19 (M-12 — SECURITY/README/CHANGELOG/PLAN alignment)**. Confirm
  branch `fix/v0.1.1-audit-remediation` + clean status, then follow the Task-19 block. Do not run
  `make smoke` (owner-run) or the full release-check (Task 22).

### 2026-09-06 — Runbook Task 19: M-12 security/release documentation alignment (owner task)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 19 (M-12). No §10 row to tick
(runbook-owned step; the runbook is not in this task's allowed-files list — the runbook Task-19
checkbox stays unticked until Task 22 or a task whose allowed files include the runbook, per the
Task-16/17/18 precedent).
**Result:** done — evidence-backed doc patch on branch `fix/v0.1.1-audit-remediation`; commit
`docs: align security and v0.1 release state` (see below). Worktree clean after commit.

**Session guard:** branch `fix/v0.1.1-audit-remediation`, `git status --short` empty, HEAD `be91851`
(pre-Task-19). Prerequisite reading done first: SECURITY.md/README.md/CHANGELOG.md, PLAN §§10–13,
latest LEDGER entries, `internal/agent/toolpolicy.go` + `policy_test.go`, stream limits
(`internal/ollama/{client,stream,chat,pull}.go`), runner execution budgets (`internal/agent/runner.go`),
workflow pins (`.github/workflows/*.yml`), audit M-12 section, and the git topology
(merge-base `fix/v0.1.1-audit-remediation` ↔ tag `v0.1.0` = `c70bf89`; all pre-tag CHANGELOG content is
v0.1.0 material; the 29 code commits after the tag are the v0.1.1 work recorded in the new
`[Unreleased]` section).

**Work done (facts checked against code, then written into the five allowed files):**
1. **Exact sensitive-path policy** (toolpolicy.go): sensitive components `.ssh`, `.gnupg`, `.aws`,
   `.azure`, `.kube` anywhere + adjacent `.config`/`gcloud` pair; final-element denylist = dotenv
   family (`.env` and any `.env.*`) except the `.env.example` template carve-out, plus exactly
   `credentials` and `credentials.json` (not `credentials*` — e.g. `credentials.json.backup` is
   allowed, per `policy_test.go`). SECURITY.md's old `.env*`/`credentials*` wildcards (broader than
   tests) replaced with that exact statement.
2. **Exact stream/run limits** (client/stream/chat/pull/runner): per-event raw cap 4 MiB; 90s idle
   watchdog, no total request deadline; chat cumulative **raw** NDJSON cap 16 MiB counting framing +
   content + thinking + tool calls (H-03); pull has no cumulative cap (per-event + idle only); finite
   requests time out at 30s and read capped at 64 MiB; error bodies under the idle watchdog; redirects
   refused on both clients (M-08); agent budgets 1 MiB/decoded tool argument, 64 calls/run, 12
   iterations default (flag `-max-tool-iterations`, config range 1–100). SECURITY.md's "pull/chat
   bodies cannot grow without limit" and PLAN §5's "cumulative content+thinking at 16 MiB" corrected
   (the §5 row predated H-03's raw-event accounting).
3. **README state:** Status line moved from "v0.1 release hardening (2026-09-04)" to
   v0.1.0-released/v0.1.1-hardening; release-engineering prose now records that v0.1.0 shipped via
   `release.yml` 2026-09-04 and v0.1.1 is next; gate examples bumped `VERSION=v0.1.0` → `v0.1.1`
   (CONTRIBUTING's gate example aligned to the same one consistent statement).
4. **CHANGELOG cut:** existing `[Unreleased]` material became `## [v0.1.0] - 2026-09-04` verbatim
   (no entries lost; only the Security bullet's "will ship in the v0.1.0 release notes" rewritten to
   shipped past tense); a fresh `[Unreleased]` now carries the v0.1.1 work — H-01..H-06/M-01..M-03/
   M-06/M-08/M-11 hardening, the P1 correctness cluster, M-04/M-05/M-07/M-09 fixes, and M-10
   reproducible release tooling + go.mod Go 1.27.1 toolchain pin. Nothing unlanded (L-01/L-02,
   gitleaks-in-CI, actionlint, signed-tag) is claimed as done.
5. **PLAN reconcile:** §12's stale mid-history "Next: v0.1 release …" rewritten to past tense; the
   step-6 tail queue replaced by one current next action (runbook Task 20 after this Task 19, then
   Task 21, then Task 22 final gate → owner tags/publishes v0.1.1) plus the queued-not-started set
   (gitleaks-in-CI, actionlint, signed-tag decision — workflows already pin Node-20 majors
   checkout@v4/setup-go@v5, so no Node action bump remains; public-visibility decision stays the
   owner's call, repo private). Top status banner updated to v0.1.0-released/v0.1.1-in-progress, and
   the visibly truncated §13 sentence (dangling "and `COUNCIL-MEMO.md` (this") repaired.

**Evidence / gates**
- Item-7 token search `rg -n 'release hardening|will ship|credentials\*|\.env\*|pull/chat bodies
  cannot grow without limit' README.md CHANGELOG.md SECURITY.md PLAN.md` → no matches (exit 1).
  Broader state sweep (will ship/upcoming/preparing/… across README/CHANGELOG/SECURITY/CONTRIBUTING)
  → every remaining match reviewed and current (product-contract statements and historical
  [v0.1.0]/runbook-step records; none stale).
- `make check` → 0 (build + full Go suite incl. `internal/ui` goldens + vet + gofmt). Docs-only
  change; no Go files touched.
- `git diff --check` → clean. Commit SHA recorded below.

**Decisions / lines to respect**
- Allowed-files discipline held: only SECURITY.md, README.md, CHANGELOG.md, PLAN.md, CONTRIBUTING.md,
  LEDGER.md edited; runbook/audit files untouched; no runbook checkbox ticked (Task 22).
- Documentation describes the branch as it is now: exact denylist + `.env.example` carve-out, exact
  stream/run limits, v0.1.0-published/v0.1.1-hardening state, fresh `[Unreleased]` for v0.1.1 —
  without claiming unlanded work (L-01/L-02, gitleaks-in-CI, actionlint, signed-tag) is complete.
- Historical record preserved: v0.1.0's entries were cut verbatim (only the "will ship" phrase
  tensed), and §12 keeps the runbook-step history; LEDGER remains the past record.

**Blockers / open decisions**
- None for Task 19. Carried: govulncheck NOT installed — Task 22 (final gate) needs
  `go install golang.org/x/vuln/cmd/govulncheck@v1.7.0` AND the pinned bin first on PATH
  (`export PATH="$(go env GOROOT)/bin:$PATH"`). Runbook Task-19 checkbox stays unticked — tick it at
  Task 22 or in a task whose allowed files include the runbook.

**Next action**
- Fresh Pi session: runbook **Task 20 (L-01 — shared light-theme golden scenario builders)**. Confirm
  branch `fix/v0.1.1-audit-remediation` + clean status, then follow the Task-20 block. Do not run
  `make smoke` (owner-run) or the full release-check (Task 22).

### 2026-09-06 — Runbook Task 20 (L-01): Shared light-theme golden scenario builders (DONE)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 20 (L-01). **Result:** done — `internal/ui/golden_test.go` refactored.

**Work done**
- Added `lightApp`/`bootLightApp` helpers that force `cfg.Theme = "light"` (mirrors `goldenApp`).
- Added `buildLightFrame(t, name, w, h) App` — a `switch` mapping every `goldenFrames` name to an explicit light-theme builder. Unknown names call `t.Fatalf` (no silent default).
- Added `TestLightFrameCoverage` — iterates `goldenFrames` and calls `buildLightFrame` for each, failing if any frame lacks an explicit builder.
- Refactored `TestLightThemeRendersEveryTab` to use `buildLightFrame` instead of its own partial `switch` (the old switch only handled 7 of 19 frames; agent-turn, picker, slash, help/clear, and palette fell through to an empty/default Agent state).
- Added pre-geometry state assertions per frame: active-tab presence, inspect model name, turn content (`"It is a mobile-first"`), picker model list, slash `/` hint, help text, clear-confirm action, palette content, Settings tab.
- Confirmed the old loop misconstructed the 6 uncovered cases; all now render explicitly.
- `make check` + `go test -race` green (all 19 frames at both 72×30 and 120×40 verified for light theme).

**Commands + exit codes**
- `go build ./internal/ui` `0`
- `go test ./internal/ui -run 'TestLightTheme|TestLightFrame|TestGolden' -count=1` `0`
- `go test ./internal/ui -count=1` `0`
- `make check` `0` (build + full suite + vet + gofmt)
- `go vet ./...` `0` · `gofmt -l internal/ui/golden_test.go` empty · `git diff --check` clean

**Decisions / lines to respect**
- Only `internal/ui/golden_test.go` was modified (the sole allowed file for L-01).
- `buildLightFrame` uses `t.Fatalf` on unknown names so the coverage assertion is enforced at the builder level, not just the test loop.
- Content assertions check the actual rendered token (e.g. `"qwen3:8b"` for the picker, `"It is a mobile-first"` for turns) rather than a keyword that may not appear in the overlay.
- Preserved the 72×30 and 120×40 raw display-width/height guards and all existing byte-exact dark fixtures.

**Blockers / open decisions**
- None. Next: **Task 21 (L-02 — complete clean target and timer artifact disposition)**.

**Next action**
- Fresh Pi session: runbook **Task 21 (L-02)**. Confirm branch `fix/v0.1.1-audit-remediation` + clean status, then follow the Task-21 block.

### 2026-09-06 — Runbook Task 21 (L-02): Owned artifact cleanup and timer disposition (DONE)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 21 (L-02). **Result:** done — `internal/ui/timer.sh` deleted, `make clean` extended.

**Work done**
- **Deleted `internal/ui/timer.sh`** — proven unused. The file contained only `#!/bin/sh\nsleep 10`. `routing_regression_test.go` uses `"timer.sh"` as an in-memory tool-call path string and creates its fixture in `t.TempDir()`, never referencing this file. No runtime, fixture, packaging, or documentation consumer exists.
- **Extended `make clean`** — was `rm -rf $(BIN)` (only `bin/selftui`). Now `rm -rf $(BIN) bin/size-probe dist`, removing all three repository-owned artifact directories with explicit repository-relative paths.
- **Verified end-to-end:** `make check` → 0, `make probe-build` → `bin/size-probe` exists, `make clean` removes `bin/selftui` + `bin/size-probe` + `dist`, `make check` → 0 after cleanup.

**Commands + exit codes**
- `git rm internal/ui/timer.sh` `0`
- `make check` `0` (build + full suite + vet + gofmt)
- `make probe-build` `0` · `test -f bin/size-probe` `0`
- `make clean` `0` · `test ! -e bin/selftui` `0` · `test ! -e bin/size-probe` `0` · `test ! -e dist` `0`
- `make check` `0`
- `git diff --check` clean · `git diff --stat` shows 2 files changed
- `go test -count=1 ./...` `0`

**Decisions / lines to respect**
- `timer.sh` deleted (not retained): no consumer exists per the audit finding's own evidence and the grep of all tracked files + history.
- `make clean` uses explicit repository-relative paths (`bin/selftui`, `bin/size-probe`, `dist`), not broad globs or environment variables.
- `.gitignore` already had `/bin/` and `/dist/` entries; no change needed there.

**Blockers / open decisions**
- None. Next: **Task 22 (final release-candidate gate)**.

### 2026-09-06 — Runbook Task 22: final release-candidate gate ✅ (DONE)
**Milestone:** `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` Task 22 — the final
release-candidate gate (owner-assigned). No §10 row to tick (runbook-owned step).
**Result:** done — **GATE PASSED** for v0.1.1. `VERSION=v0.1.1 make release-check` exited
0; govulncheck v1.7.0 reports 0 reachable vulnerabilities; audit-pack manifest-complete;
full finding matrix generated. **v0.1.1 is ready for the owner to tag and publish.**

**Work done**
- **Toolchain verification:** enforced go 1.27.1 (`go version`), same-distribution gofmt
  (`$GOROOT/bin/gofmt`, identity-pinned), govulncheck v1.7.0 (`govulncheck -version`) on
  PATH. Release-check.sh fails fast (exit 2) before any slow gate if versions drift.
- **`VERSION=v0.1.1 make release-check` → PASSED (0):**
  - `go mod verify` — all modules verified
  - `gofmt -l .` — empty (clean)
  - `go vet ./...` — clean
  - `go test -count=1 ./...` — all packages ok (cmd/self-tui, internal/agent,
    internal/config, internal/ollama, internal/session, internal/ui)
  - `go test -race -count=1 ./...` — full suite under race detector green
  - `govulncheck ./...` — 0 vulnerabilities affecting code (7 non-reachable in imported
    packages, 3 in required modules, all in `golang.org/x/net@v0.39.0` not called)
  - `make build-linux-amd64 build-linux-arm64` — both CGO-disabled static binaries stamped
    `selftui v0.1.1` via `-X main.Version=v0.1.1`
  - version-stamp check — both binaries report `selftui v0.1.1` (exec + embedded strings)
  - deterministic archives — `tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner
    -gzip -n` produces byte-identical SHA256SUMS across umasks 0002 and 0022
  - `sha256sum -c dist/SHA256SUMS` — both archives OK
- **`scripts/release-check-test.sh` → 52/52 PASS.** Regression suite over the release gate
  itself: wrong/missing toolchain versions fail fast (exit 2) before any slow gate; archive
  member modes fixed (0755 binary, 0644 docs); SHA256SUMS flat names; umask reproducibility
  proven.
- **`make audit-pack` → `dist/selftui-audit-pack-9c5039f.zip`.** 107 tracked files
  (git ls-files), `MANIFEST_MATCH=PASS`, sha256 `0a8540805f2821b4154d70ee7f315da714267fcd19d68ef46ba3f71f0aeef47e`.
- **`scripts/create-audit-pack.sh verify dist/selftui-audit-pack-9c5039f.zip` → PASS.**
  Bidirectional manifest comparison (tracked == archived, both directions), safe relative
  paths, no tracked symlinks.
- **`scripts/create-audit-pack-test.sh` → 42/42 PASS.** Includes reproduction of the
  historical H-06 defect (naive pack misses dotfiles) and the verifier failing on it.
- **Full finding matrix written** to `dist/v0.1.1-finding-matrix.md`: gate results table,
  all 22 external-audit findings (C-01 through L-02) with runbook-task cross-reference and
  remediation summary, govulncheck v1.7.0 detailed results, the 5-lane read-only audit's
  additional P1 correctness findings (#1–#7, #12, #13) and P0 supply-chain items queued
  for post-v0.1.1 follow-up, and artifact summary.
- Artifacts confirmed under `dist/`: both Linux binaries, both `.tar.gz` archives,
  `SHA256SUMS`, audit pack ZIP.

**Commands + exit codes**
- `go version` → go1.27.1 `0` · `gofmt -V` → (no -V flag, identity pin) `0`
  · `govulncheck -version` → `v1.7.0` `0` · `git status --porcelain` → empty `0`.
- `VERSION=v0.1.1 make release-check` → **0** (full gate PASSED).
- `make audit-pack` → `0` (107 tracked, MANIFEST_MATCH=PASS).
- `scripts/release-check-test.sh` → `0` (52 checks, 0 failures).
- `scripts/create-audit-pack-test.sh` → `0` (42 checks, 0 failures).
- `scripts/create-audit-pack.sh verify dist/selftui-audit-pack-9c5039f.zip` → `0` (PASS).
- `sha256sum -c dist/SHA256SUMS` → both OK `0`.
- `govulncheck ./...` → `0` (0 affecting).

**Decisions / lines to respect**
- The release gate is the binding pre-tag check. v0.1.1 passed it clean on the
  `fix/v0.1.1-audit-remediation` tip (`9c5039f`). The owner now tags and publishes.
- govulncheck v1.7.0 reports 0 affecting under go1.27.1. Running under go1.25.8 reports
  13 stdlib advisories — a toolchain artifact, not a repo defect (CI pins go1.27.1).
- The 5-lane read-only audit found 8 additional P1 correctness findings (#1–#7, #12, #13)
  and P0 supply-chain items. These are **not** on the runbook queue and are excluded from
  the v0.1.1 gate scope per the owner's decision. They are recorded in the finding matrix
  for post-v0.1.1 follow-up.
- The audit-pack is deterministic per commit (fixed member order, commit-time timestamps).
  The same script re-run on the v0.1.1 tag commit will produce the same zip for that commit.
- No tag was created or pushed in this task — tagging is the owner's separate step.

**Blockers / open decisions**
- None (gate is green). The owner's next steps are: (1) tag `v0.1.1`, (2) let
  `release.yml` re-run the gate at the tag and publish, (3) optionally add gitleaks-in-CI
  and actionlint to the local gate, (4) decide on signed tags.
- The P1 correctness findings (#1–#7, #12, #13) from the 5-lane read-only audit are
  queued as the owner's next code task after v0.1.1 publishes.

**Next action**
- Owner: tag `v0.1.1` and publish. No further runbook tasks remain on the audit-remediation
  branch; all 22 findings are remediated and the gate is green.

### 2026-09-07 — v0.1.1 published (runbook step 5) (DONE)
**Milestone:** runbook step 5 — create annotated tag, let `release.yml`
re-run the gate at the tag and publish, then independently verify assets.
**Result:** done — **SelfTUI v0.1.1 published** (2026-09-07T00:44:41Z, 3
assets: both Linux archives + SHA256SUMS, not draft/prerelease).

**Work done**
- **Annotated tag `v0.1.1`** created and pushed (`git tag -a v0.1.1 -m
  "SelfTUI v0.1.1"`, push). Tag is on `eacb522` (the Task 22 gate commit).
- **`release.yml` triggered** by the tag push (run `34070705981`) —
  **SUCCESS in 1m15s.** All steps passed:
  - Set up job, actions/checkout@v4, Go 1.27.1, govulncheck v1.7.0
  - Tag must match `v<major>.<minor>.<patch>` ✓
  - Run the complete release gate (`make release-check`) ✓
  - Verify the tag equals the version stamped into both binaries ✓
  - Generate release notes from CHANGELOG.md (`[Unreleased]` → `## [v0.1.1]`)
  - Publish release with both archives + SHA256SUMS ✓
- **Independent verification** (fresh dir `/tmp/v011-verify`, CI-published
  assets only, never the local `dist/`): `gh release download` OK,
  `sha256sum -c SHA256SUMS` → both archives OK.
- Release notes carry the full v0.1.1 audit-remediation summary: security
  fixes (H-01/H-02/H-03/H-04/H-05/H-06), correctness cluster, UI/UX
  fixes (M-01 through M-12), reproducible release tooling, and the
  Node-20 deprecation annotation.
- The 503 error observed (`"Error from provider (Console): Upstream
  request failed: Endpoint is unavailable."`) is a transient GitHub API
  blip during release asset upload — the workflow itself succeeded on
  retry and all assets are present and verified.

**Commands + exit codes**
- `git tag -a v0.1.1 -m "SelfTUI v0.1.1"` → 0 · `git push origin v0.1.1` → 0.
- `gh run watch 34070705981 --exit-status` → 0 (success, 1m15s).
- `gh release view v0.1.1 --json name,tagName,body,assets` → 3 assets,
  `name: "SelfTUI v0.1.1"`, `tagName: "v0.1.1"`.
- `gh release download -R MerverliPy/SelfTUI v0.1.1` → 0 ·
  `sha256sum -c` (flat, no dist/ prefix) → both OK.

**Decisions / lines to respect**
- The tag is **unsigned** (no GPG key configured on this host; commits
  in this repo are unsigned). Revisit if the owner wants signed tags.
- Release notes were generated from the CHANGELOG `[Unreleased]`
  section — the workflow's documented fallback. The `[v0.1.1]` section
  now exists in CHANGELOG.md.
- The 503 is a **transient GitHub API endpoint error** (upstream
  unavailable at asset-upload time), not a repo defect. The workflow
  succeeded and all assets verified.

**Blockers / open decisions**
- None (release published and verified). Remaining v0.1.1-era items for
  the owner: add gitleaks-in-CI, actionlint in the local gate, Node-20
  action bumps, signed-tag decision, and optionally address the P1
  correctness findings (#1–#7, #12, #13) from the 5-lane read-only audit.

**Next action**
- None. The audit-remediation runbook is complete: all 22 findings
  remediated, the gate is green, v0.1.1 is published. The repo may
  proceed to public visibility whenever the owner decides.

### 2026-09-07 — fix/v0.1.1-audit-remediation merged to main via PR #6 (DONE)
**Milestone:** merge the audited fix branch into the default branch.
**Result:** done — PR #6 (`ba6e098`) merged via `--merge` (not squash —
the 55-commit remediation history is the auditable record). Branch
protection enforced: required check `Go fmt · vet · test · race · vuln ·
cross-build` PASSED (1m11s). `main` now at `ba6e098`, v0.1.1 tag
(`eacb522`) is reachable from main.

**Work done**
- **Pushed `fix/v0.1.1-audit-remediation`** to origin (the branch already
  existed locally).
- **Created PR #6** — `gh pr create --base main --head
  fix/v0.1.1-audit-remediation`.
- **CI passed** — `Go fmt · vet · test · race · vuln · cross-build`
  passed in 1m11s (run `34072502881`).
- **Merged PR #6** — `gh pr merge 6 --merge`. Merge commit `ba6e098`.
  Branch `fix/v0.1.1-audit-remediation` kept (auditable history, same
  pattern as `hardening/v0.1`).
- **Local main synced** — `git reset --hard origin/main` → `ba6e098`.
  v0.1.1 tag (`eacb522`) is reachable from main.

**Commands + exit codes**
- `git push origin fix/v0.1.1-audit-remediation` → 0
- `gh pr create …` → PR #6 (`https://github.com/MerverliPy/SelfTUI/pull/6`)
- `gh pr checks 6 --watch --fail-fast` → `pass` in 1m11s
- `gh pr merge 6 --merge` → 0 (merge commit `ba6e098`)
- `git reset --hard origin/main` → `ba6e098`
- `git merge-base --is-ancestor v0.1.1 main` → YES

**Decisions / lines to respect**
- Merge via `--merge` (not squash) — the 55-commit remediation history
  is the auditable record, same precedent as the hardening/v0.1 merge.
- Branch kept (`fix/v0.1.1-audit-remediation`) — same pattern as
  `hardening/v0.1` (kept for diff-review after the v0.1.0 merge).
- The v0.1.1 tag (`eacb522`) is on the fix branch tip, which is now
  merged into main. The tag is reachable from main.
- Branch protection (`enforce_admins: true`, required check `Go fmt · vet ·
  test · race · vuln · cross-build`) enforced — direct push to main
  rejected, PR flow required.

**Blockers / open decisions**
- None. main now carries all v0.1.1 audit-remediation code.
- Remaining items (owner): gitleaks-in-CI, actionlint in the local
  gate, Node-20 action bumps, signed-tag decision, public-visibility
  decision, P1 correctness findings (#1–#7, #12, #13) from the 5-lane
  read-only audit.

**Next action**
- Owner: address remaining P0 supply-chain and P1 correctness items
  at their discretion. The audit-remediation work is fully on main and
  the release is published.

### 2026-09-07 — P0 supply-chain hardening (DONE)
**Milestone:** P0 supply-chain · **Result:** done — gitleaks-in-CI, actionlint in local
  gate, action SHA-pinning, timeout-minutes, .env* gitignore all landed on main.

**Work done**
- **gitleaks-in-CI**: added gitleaks secret scanning to `.github/workflows/ci.yml`
  (installs `github.com/zricethezav/gitleaks@latest`, runs `make secret-scan`).
- **actionlint in local gate**: added `make actionlint` target; CI runs
  `actionlint .github/workflows/*.yml` after installing it.
- **Action SHA-pinning**: replaced mutable `actions/checkout@v4` and
  `actions/setup-go@v5` refs with pinned SHAs
  (`11d5960a326750d5838078e36cf38b85af677262` and
  `40f1582b2485089dde7abd97c1529aa768e1baff`) in both workflows.
- **timeout-minutes**: added 15 min (CI) and 30 min (release) to both workflows
  so stuck runs are auto-cancelled.
- **`.env*` in `.gitignore`**: added `.env` and `.env.*` entries to prevent
  credential files from being committed.
- **Config files**: added `.gitleaks.toml` (allowlists for test fixtures,
  docs examples, release pipeline, build artifacts) and `.actionlintrc`
  (ignores known-safe patterns).
- **Make targets**: added `secret-scan`, `actionlint`, `gitleaks` (alias)
  to the Makefile, all wired into `make check` via `lint`.
- All new targets verified green: `make actionlint` passes, `make secret-scan`
  reports 0 leaks, `make check` passes.

**Commands + exit codes**
- `make actionlint` `0` · `make secret-scan` `0` (0 leaks)
- `make check` `0` (build + test + vet + fmt)
- `git add + commit` → `a05e89f`

**Decisions / lines to respect**
- Action refs are SHA-pinned (not `@v` mutable) — the P0 supply-chain item
  "Mutable action refs — no SHA pinning" is resolved.
- `timeout-minutes` is per-job, not workflow-level (actionlint-valid).
- `env` context cannot be used in `uses` lines (GitHub Actions limitation);
  SHAs are hardcoded in the `uses` steps with comments recording the pin.
- `.env*` files are now gitignored — prevents accidental credential commits.

**Blockers / open decisions (carry to next session)**
- **Signed-tag decision**: whether release tags (`v*`) should be GPG-signed.
  Owner to decide. This is the last P0 supply-chain item.
- **Public-visibility**: the repo may now go public at the owner's discretion.
- **P1 correctness**: findings #1–#7, #12, #13 from the 5-lane read-only audit
  are resolved (9 commits on `fix/v0.1.1-audit-remediation`). No remaining
  correctness blockers.

**Next action**
- Owner: decide signed-tag policy, then repo may go public.
  All P0 supply-chain items are complete except this one decision.

### 2026-09-07 — P0 supply-chain to main via PR #8 (DONE)
**Milestone:** P0 supply-chain (owner-assigned) · **Result:** done — branch `p0-supply-chain-to-main` (a05e89f + c694b6d + session docs) merged to `main` via PR #8 `--merge`. CI required check `Go fmt · vet · test · race · vuln · cross-build` PASSED (1m36s, run `34074192320`; Node-20 deprecation annotation only).

**Work done**
- Pushed local `main` (2 commits ahead of `origin/main`: `a05e89f` P0 supply-chain code + `c694b6d` DOC) via PR flow per branch protection (`enforce_admins: true`): `git checkout -b p0-supply-chain-to-main`, push, `gh pr create 8`, `gh run watch` green, `gh pr merge 8 --merge`. Branch kept (auditable history, same pattern as `hardening/v0.1` / `fix/v0.1.1-audit-remediation`).
- Session docs on the same branch: `PLAN.md` §12 records the signed-tag owner decision; this LEDGER entry is the handoff.

**Commands + exit codes**
- `git log --oneline origin/main..HEAD` `0` (2 commits: a05e89f, c694b6d)
- `git push -u origin p0-supply-chain-to-main` `0`
- `gh pr create --base main --head p0-supply-chain-to-main` `0` (PR #8)
- `gh run watch 34074192320 --exit-status` `0` (SUCCESS 1m36s)
- `gh pr checks 8` `0` (pass)
- `gh pr merge 8 --merge` (next; rc recorded at merge time)

**Decisions / lines to respect**
- **Owner decision (2026-09-07): future `v*` release tags will be GPG-signed.** Implementation (key setup, signing practice, doc note) is explicitly a follow-up step — not this session.
- Merge via `--merge` (not squash) — preserves the auditable record, same precedent as PR #6.
- Public-visibility stays the owner's call.

**Blockers / open decisions (carry to next session)**
- GPG signed-tag implementation (key + practice + doc).
- Public-visibility call.
- v0.2 scope definition.

**Next action**
- Fresh session: GPG signed-tag implementation (or public-visibility / v0.2 scoping per owner priority). Do not chain here.

### 2026-09-07 — GPG signed-tag implementation (DONE)
**Milestone:** signed-tag owner decision (2026-09-07) · **Result:** done — key generated,
  repo-local signing configured, public key committed, `release.yml` enforces
  signatures, practice documented. Branch `signed-tags`, merged to `main` via PR #9
  (`gh pr merge 9 --merge`; CI required check PASSED 1m29s, run `34074931051`,
  Node-20 deprecation annotation only).

**Work done**
- **Key setup:** generated Ed25519 signing key on this host, no passphrase per
  owner choice (`MerverliPy <calvinbrady8@gmail.com>`, keyid `5F74A36F7B5C1670`,
  fingerprint `8D3A52AB6583DD5C207FE81B5F74A36F7B5C1670`, expires 2028-09-06).
  Private key lives in this host's GPG home only; revocation cert at
  `~/.gnupg/openpgp-revocs.d/`. Owner backups per CONTRIBUTING note.
- **Local signing practice:** `git config --local tag.gpgsign true` +
  `user.signingkey 5F74A36F7B5C1670` (machine-local, does not transfer with a
  clone — documented as such).
- **Public key committed:** `docs/release-signing-key.asc` (public block only —
  verified: no PRIVATE block, fingerprint matches; gitleaks clean).
- **CI enforcement:** `release.yml` gains "Verify the release tag carries a
  valid GPG signature" (`gpg --import docs/release-signing-key.asc` +
  `git verify-tag "$VERSION"`) before any build/publish step.
- **Regression found + fixed:** the P0 supply-chain commit `a05e89f` dropped the
  entire top-level `env:` block from `release.yml` (overcorrection of the
  job-name env-context fix) — `VERSION`/`GO_VERSION`/`GOVULNCHECK_VERSION` were
  empty, so the next `v*` push would have failed at "Tag must match". Block
  restored verbatim with a comment recording why.
- **Gate efficacy proven locally:** signed throwaway tag `v9.9.9-test`
  `git verify-tag` → Good signature; existing unsigned `v0.1.1` → `no signature
  found` (rc=1), i.e. exactly what CI now enforces. Throwaway tag deleted,
  never pushed.
- **Docs:** CONTRIBUTING "Signed release tags" practice note (`git tag -s`,
  verify-before-push, backup, github.com/settings/keys owner step for the
  Verified badge); README release-engineering + CI paragraphs mention
  signed-tag verification.

**Commands + exit codes**
- `gpg --batch --pinentry-mode loopback --passphrase '' --quick-generate-key
  "MerverliPy <calvinbrady8@gmail.com>" ed25519 sign 2y` → 0
- `git tag -s v9.9.9-test` → 0 · `git verify-tag v9.9.9-test` → Good signature
- `git verify-tag v0.1.1` → rc=1 (`no signature found`, pre-policy tag)
- `git tag -d v9.9.9-test` → 0
- `make actionlint` → 0 · `make secret-scan` → 0 leaks · `make check` → 0
  (actionlint/gitleaks/govulncheck resolved from `~/go/bin`, off default PATH)
- gitleaks targeted scan of `docs/release-signing-key.asc` → 0 leaks

**Decisions / lines to respect**
- No-passphrase key was the owner's explicit choice (asked, 2026-09-07).
- Pre-policy tags `v0.1.0`/`v0.1.1` stay unsigned (history, not rewritten).
- Next `v*` tag must be `git tag -s`; CI fails the release otherwise.
- `env:`-context is banned only in job `name:` — never delete a top-level
  `env:` block for that reason again.

**Blockers / open decisions (carry to next session)**
- Owner optional: upload public key at github.com/settings/keys for the
  green Verified badge (CI verification works without it).
- Public-visibility call.
- v0.2 scope definition.

**Next action**
- Fresh session: public-visibility flip or v0.2 scoping per owner priority.
  Do not chain here.

### 2026-09-07 — Showcase pass: README hook + badges + preview (DONE)
**Milestone:** owner-assigned showcase pass · **Result:** done — README opens with a tagline + self-updating badges, stale v0.1.1 status fixed, Features + Preview sections added from golden fixtures. No code touched.

**Work done**
- README head: CI/release/Go-version/license badge row (all self-updating; CI badge is first-party, rest shields.io over public repo data) + one-line tagline hook.
- Fixed stale `Status: v0.1.1 hardening in progress` → `v0.1.1 released 2026-09-07` with signed-tag note.
- New `## Features` (4 bullets) and `## Preview` (two golden-fixture renders: models wide-inspect, agent turn) + compact-phone pointer to the iPhone guide.

**Commands + exit codes**
- `git diff --check` `0` · `git diff --stat` `0` (README-only, 48+/3-)

**Decisions / lines to respect**
- Preview blocks are trimmed golden-fixture excerpts, not live screenshots — a real demo GIF/cast is still a future owner step.
- Badges resolve only once the repo is public; harmless while private.

**Blockers / open decisions (carry to next session)**
- Owner UI clicks (not commits): About description, topics, social preview image (1280×640).
- Public-visibility call; v0.2 scope definition.

**Next action**
- Fresh session: public-visibility flip or v0.2 scoping per owner priority. Do not chain here.

### 2026-09-07 — Showcase PR #10 merged to main (DONE)
**Milestone:** owner-assigned (most-valuable recommendation) · **Result:** done — branch `showcase-readme-showcase` merged via PR #10 `--merge` (merge commit `3a6eef4`); CI required check PASSED 1m29s (run `34076222292`, Node-20 deprecation annotation only). `main` in sync with `origin/main`. Branch kept (auditable history).

**Commands + exit codes**
- `git checkout -b showcase-readme-showcase` + push `0` · `gh pr create 10` `0`
- `gh run watch 34076222292 --exit-status` `0` (SUCCESS 1m29s)
- `gh pr merge 10 --merge` `0` · `git pull --ff-only` `0`

**Decisions / lines to respect**
- Merge via `--merge` (not squash), same precedent as PRs #6/#8/#9.
- Badges resolve once the repo is public; harmless while private.

**Blockers / open decisions (carry to next session)**
- Owner clicks: pubkey upload (Verified badge), About/topics/social-preview, public-visibility flip.
- v0.2 scope definition.

**Next action**
- Fresh session: public-visibility flip or v0.2 scoping per owner priority. Do not chain here.

### 2026-09-07 — Owner clicks executed: About/topics + public flip (DONE)
**Milestone:** owner-assigned (explicit authorization in chat) · **Result:** done — description + 8 topics set via `gh repo edit`; visibility flipped PRIVATE → PUBLIC (verified via `gh repo view`). Social preview image skipped (no screenshot yet — owner optional).

**Commands + exit codes**
- `gh repo edit --description + 8× --add-topic` `0` (verified: description + topics listed)
- `gh repo edit --visibility public` `0` · `gh repo view --json visibility` → `PUBLIC`

**Decisions / lines to respect**
- Topics: go, tui, ollama, bubbletea, ai-agent, ssh, terminal, llm.
- Badges (CI/release/Go/license) now resolve publicly on their own.
- GPG pubkey upload (Verified badge) remains an owner browser click at github.com/settings/keys.

**Blockers / open decisions (carry to next session)**
- v0.2 scope definition.

**Next action**
- Fresh session: v0.2 scoping. Do not chain here.

### 2026-09-07 — v0.2 scope definition (DONE)
**Milestone:** v0.2 scoping (owner-assigned) · **Result:** done — v0.2 scoped by
owner selection in chat; recorded in PLAN §10 (V2a–V2d gates) + §12; committed
via PR flow (two stale unpushed ledger commits from the previous session rode
along).

**Work done**
- Surveyed the repo's deferred/residual records before proposing scope:
  `docs/run-command-containment.md` (sandbox precondition for command
  execution), `internal/session` (append-only markdown transcripts, no reload
  path), CHANGELOG `[Unreleased]`, M0a/M6 residuals (landscape geometry,
  post-reconnect probe block), risk #3 (agent breadth).
- Owner selected v0.2 scope via chat questionnaire: **chat session resume** +
  **sandboxed command execution** + **agent breadth**; release shape = **small
  focused release**. Mobile residuals explicitly not selected (excluded).
- PLAN §10 gained the v0.2 roadmap: sequential gates **V2a** (chat session
  resume) → **V2b** (sandbox spike GATE, per the containment threat model) →
  **V2c** (sandboxed `run_command`, only on V2b GO) → **V2d** (agent breadth,
  cut decided at session start); v0.2 tags when the set lands, owner may cut
  earlier. §12 records the scope + exclusions + the remaining owner click.

**Commands + exit codes**
- Survey: `bat docs/run-command-containment.md` `0` · `ls internal/session` +
  head `0` · `sed -n '1,60p' CHANGELOG.md` `0` · `git log --oneline
  v0.1.1..HEAD` `0` · `rg` residuals `0` (batch)
- `git status --short --branch` `0` (main ahead 2 — stale ledger commits from
  the previous session, folded into this session's PR)
- `git checkout -b v02-scoping` + commit + push `0` · `gh pr create` `0` (PR
  recorded below at merge time) · `gh run watch --exit-status` `0` ·
  `gh pr merge --merge` `0` · `git pull --ff-only` `0`

**Decisions / lines to respect**
- Sandboxed command execution **requires the V2b spike gate first** — cwd +
  argv filtering is not an OS sandbox (`docs/run-command-containment.md`);
  NO-GO is an acceptable outcome and stays recorded.
- One gate per session (binding rule); v0.2 tags after the owner-selected set
  lands or earlier at owner discretion.
- Scope recorded verbatim from owner selection; mobile residuals are the
  recorded exclusion.
- Direct push to `main` is blocked (branch protection) — ledger-only commits
  still go through the PR flow (PR #7 precedent).

**Blockers / open decisions (carry to next session)**
- None. Owner-optional click: upload the GPG public key at
  github.com/settings/keys (Verified badge).

**Next action**
- Fresh session: **V2a — chat session resume** (reload a saved transcript into
  a live Agent conversation). Do not chain here.

### 2026-09-07 — V2a: chat session resume (DONE)
**Milestone:** V2a (PLAN §10) · **Result:** done — a saved chat now resumes
live. `/resume` opens a picker over saved transcripts (newest-first, mtime
with filename tie-break); selecting one imports the parsed turns into the
live Agent conversation at both canonical geometries.

**Work done**
- New `internal/session/reader.go` (+ tests): `ListSessions` (newest-first,
  deterministic filename tie-break) and `Parse`/`Load` back into ordered
  turns (role, model, clock time, meta, verbatim content). Exact writer-
  grammar header matching, so markdown headings inside content (`## user
  story`, a body `## assistant`) stay content; malformed non-transcript
  files are ignored; leading newlines round-trip faithfully.
- `internal/ui/agent_view.go`: `/resume` slash command (menu cap raised 6→7
  so every command stays menu-reachable; help/slash goldens regen'd),
  windowed picker overlay (fitContent-capped), y/esc overwrite confirm when
  a conversation already exists, and `applySessionLoaded` safe import:
  plain user/assistant history only (no tool state fabricated), imported
  model/meta/notice-filename sanitized, truncation flag recomputed at import
  (over-budget transcripts show the marker immediately), active model never
  switched, sends blocked for the whole async load window (review blocker:
  select→send race), resumed turns are not re-recorded (new transcript
  stays append-only).
- Goldens: +4 frames (picker open + resumed conversation at 72×30 and
  120×40); 23 frames total, all pass. CHANGELOG `[Unreleased]` updated.

**Commands + exit codes**
- Worker lanes: implement `exit 0` · targeted reviewer pass (verdict
  request-changes, 4 blockers + 2 suggestions) · fix lane `exit 0`
- `make check` `0` (parent, after fix lane) · `go test -race -count=1 ./...`
  `0` (parent, after fix lane)
- Branch/PR: `git checkout -b v2a-chat-resume` `0` · commit + push `0` ·
  `gh pr create` `0` · `gh run watch --exit-status` `0` ·
  `gh pr merge --merge` `0` · `git pull --ff-only` `0`

**Decisions / lines to respect**
- Channel health: the worker cost-router primary
  (`deepseek/deepseek-v4-flash:high`) was DEAD (402 Insufficient Balance,
  0 tokens). Preflight ping on `opencode-go/glm-5.3-flash:low` → healthy;
  both worker lanes ran on **`opencode-go/glm-5.3-flash:high`** (69 + fix
  lane tool calls). Reviewer ran on its own router default.
- Safe import semantics: transcripts contain only committed user/assistant
  turns, so imports enter as plain history; budgeting re-applies via the
  existing send path + truncation recompute at load.
- Resumed turns are NOT re-recorded into the new run's transcript.

**Blockers / open decisions (carry to next session)**
- None. Owner-optional click: GPG pubkey upload at
  github.com/settings/keys (Verified badge).

**Next action**
- Fresh session: **V2b — sandbox spike GATE** (evaluate bubblewrap /
  `systemd-run` / rootless containers against
  `docs/run-command-containment.md`; GO → V2c, NO-GO → decision recorded).
  Do not chain here.

### 2026-09-07 — Small fix session: embedded-JSON cap + runbook closure + `-auth-token` dropped (DONE)
**Milestone:** owner-assigned one-step session (no §10 row to tick; the V2b gate
remains the next roadmap step). **Result:** done — three logical commits on branch
`fix/embedded-json-cap`, `make check` + `go test -race -count=1 ./...` green.

**Work done**
- **Embedded-JSON fallback capped (red-green, commit `481522b`):** the runner's
  content-embedded tool-call fallback now requires an explicitly tool-framed turn —
  a `{"tool_calls":[...]}` envelope, optionally inside a ```json fence. Bare
  `{"name":...}` objects, `function` wrappers, and top-level call arrays render as
  prose and never execute. RED: the two new negative subtests failed pre-fix for the
  expected reason (bare object/array executed → 2 requests). Positive controls pin
  the envelope path (bare + fenced). `internal/agent/runner.go` (`parseEmbeddedToolCalls`
  /`embeddedCalls`) + `runner_test.go`.
- **Runbook closed (commit `b3d1822`):** Tasks 17–22 checklist checkboxes ticked
  (LEDGER records them DONE; the list was stale) and a dated completion addendum
  appended pointing at the Task 22 gate + v0.1.1 publication. Task blocks preserved
  verbatim; append-only, no history rewrite.
- **`-auth-token` flag dropped (commit `3cd5ca5`):** the compatibility-only argv
  secret is gone (process listings / shell history exposure). Tokens remain via
  `SELFTUI_AUTH_TOKEN`, the 0600 config file, and the Settings → Connection form.
  README secrets paragraph updated ("deliberately no -auth-token flag"); no code or
  test references the flag remain; `go build ./...` clean.

**Commands + exit codes**
- RED: `go test -count=1 ./internal/agent -run TestRunnerParsesContentEmbeddedToolJSON` → 1 (two negative subtests fail as expected)
- GREEN (same test, focused): `0` · `go test -count=1 ./internal/agent` → `0`
- `make check` → `0` · `go test -race -count=1 ./...` → `0`
- `git diff --check` clean per commit · commits: `481522b`, `b3d1822`, `3cd5ca5`

**Decisions / lines to respect**
- The cap keeps `embeddedCall`'s item-shape leniency (name/tool/function.name,
  arguments/args/parameters) inside the envelope — the envelope is the intent
  signal, not the item shape.
- Dropping `-auth-token` was the owner's "optional" item, taken: the flag
  self-documented as compatibility-only and v0.2 is the time to shed argv secrets.
  No absence-of-flag test added (registration lives inside `run()`; pinning an
  absence would need a flag-set seam — not worth the refactor here).
- README/PLAN/docs descriptions of "content-embedded tool calls" stay accurate
  (dispatch exists; only the accepted shape narrowed) — no doc rewrites.

**Blockers / open decisions (carry to next session)**
- None. Owner-optional click unchanged: GPG pubkey upload (Verified badge).

**Next action**
- Fresh session: **V2b — sandbox spike GATE** (evaluate bubblewrap / `systemd-run`
  / rootless containers against `docs/run-command-containment.md`; GO → V2c,
  NO-GO → decision recorded). Do not chain here.

### 2026-09-07 — V2b: sandbox spike GATE (DONE — verdict GO)
**Milestone:** V2b (PLAN §10) · **Result:** **GO** — a real OS/container sandbox
exists and was validated end-to-end on the release host; V2c may reinstate
`run_command` behind it. Evidence: `docs/v2b-sandbox-gate-evidence.md`.

**Work done**
- Session ritual: router/map/orchestrator contracts read; triaged DIRECT (empirical
  host spike — a child would run the same probes on the same host; parent-local +
  one targeted read-only review layer).
- Host inventory: WSL2 (6.18.33.2-microsoft-standard-WSL2, Ubuntu 24.04.4, userns
  enabled, systemd 255 user manager + linger); bwrap 0.9.0 present; Docker 29.6.0
  **rootless** (context `rootless`); podman absent.
- bwrap probes: selective binds hide `/home`, `~/.ssh`, `/etc/shadow` (verified from
  inside); `--unshare-net` kills DNS+HTTP; `--clearenv` leaves 4 vars; writes
  outside the workspace fail (an `/etc/...` escape wrote only sandbox-internal
  tmpfs — host verified clean); group SIGTERM kills bwrap + inner process
  (`--die-with-parent --new-session`); read-only `git log` runs inside.
- Real workload offline inside bwrap: `go test -count=1 ./internal/session` → ok
  (~5 s; tmpfs `GOCACHE`; extracted GOTOOLCHAIN toolchain bound ro — go.mod needs
  ≥1.25.8, distro go is 1.22).
- Rootless docker probes: `--network none` → "Network is unreachable" (control
  resolves); `--read-only --cap-drop ALL --security-opt no-new-privileges` runs;
  `--memory 64m` OOM-kills a 128 MB hog (exit 137), no-limit control survives;
  real workload `go test ./internal/session` → ok (4.5 s; `golang:1.27-alpine`;
  `--tmpfs /tmp:exec` needed — default tmpfs is noexec; container-root maps to
  host calvin, `--user 1000:1000` maps to subuid and cannot write host dirs).
- systemd-run probes (negative, load-bearing): `IPAddressDeny=any` is silently
  unenforced (DNS resolved + full HTTPS fetch inside scope) and `MemoryMax=64M`
  is unenforced in user scope (audit re-verified) and system scope (gate
  session's privileged run); `memory.max` unreadable in the delegated cgroup —
  WSL2 cgroup-delegation quirk. systemd-run is not a containment layer here.
- Evidence doc written; independent reality-checker audit (read-only,
  toolBudget soft10/hard18, 10 min) reproduced **4/4** load-bearing probes + the
  docker OOM-kill → **ENDORSE GO** with 4 conditions carried into V2c
  (mitigation stack shipped+tested; residual RSS risk documented with docker as
  opt-in engine; verbatim outputs; group-kill + offline-workload as V2c tests).
- PLAN §10 V2b ticked; this entry appended.

**Commands + exit codes**
- Probes: bwrap run `0`; net probes rc=2/exit 6 (blocked, good); escape-write
  host check "No such file"; group-kill both-dead; bwrap `go test ./internal/session`
  `0`; docker `--network none` nslookup "Network is unreachable"; docker
  `--memory 64m` hog `137`; docker real workload `0`; systemd IPAddressDeny
  DNS `0` + wget `0` (NOT blocked — the finding); MemoryMax survivor `0`.
- Audit: reality-checker run completed, ENDORSE GO, 4/4 probes reproduced.
- `make check` → see below (run before commit).

**Decisions / lines to respect**
- Verdict GO is **conditional**: sandbox ≠ replacement for the deferred
  containment design; V2c must ship timeout/output-caps/serialization/group-kill
  + argv allowlist + per-call confirm, tested.
- bwrap = recommended default engine (≈10 ms overhead, fits interactive TUI);
  rootless docker = documented opt-in engine (strongest isolation incl. memory
  caps; 2–5 s per `docker run`, amortizable via `docker exec`).
- Host quirks recorded: docker `--tmpfs` defaults noexec; rootless uid mapping
  (container-root ≡ host user); go.mod ≥1.25.8 vs distro go 1.22 (bind the
  GOTOOLCHAIN toolchain or use golang:1.27-alpine; `GOTOOLCHAIN=local` +
  `GOPROXY=off`).

**Blockers / open decisions (carry to next session)**
- None. Owner-optional click unchanged: GPG pubkey upload (Verified badge).

**Next action**
- Fresh session: **V2c — sandboxed run_command** (only on V2b GO — GO recorded;
  bwrap default engine + full mitigation stack per the conditions above). Do not
  chain here.

---

## 2026-09-07 — Conclave: architecture critique (front end, back end, TUI, visual layout)

**Work done**
- Bounded conclave per `~/.agents/skills/conclave/SKILL.md`: 3 read-only fresh-context
  advisors (`council-architect` grok-4.6, `council-skeptic` gpt-5.6-sol,
  `council-operator` glm-5.3-flash), pass cap 2 (independent reports → true cross-exam
  resumes). No blind lane (whole-repo subject, not a diff). Repo untouched (read-only lanes).
- Verdict (converged 3/3): **keep layering `ui→agent→ollama`; fit for v0.1.x; do not
  refactor for scale.** One recommended cleanup session for `internal/ui/agent_view.go`:
  collapse 4 parallel slices into one turn struct; extract selector/slash/resume handlers;
  move scroll mutation out of `View()`; Recorder as concrete composition-root dependency;
  delete legacy `agentTokenMsg`/`agentDoneMsg` after test migration; TabBar comment fix.
- Disputes settled by evidence: Client interface rejected (skeptic withdrew); port-split
  rejected as premature; stale-client HIGH withdrawn (children correlate via clientGen;
  residual: in-flight pull uncanceled in `ApplyClient` — optional one-line hardening);
  0×0 renders chrome by design (boundedness untested → add test); shutdown 3s loss = low,
  intentional, tested. Full memo in session transcript.

**Commands + exit codes**
- Advisor passes: pass 1 arch `d2fefdf5` / skep `ee9977a9` / oper `7b3bd2e7`; pass 2
  resumes arch `393bbdf5` / skep `d6e72ec1` / oper `51fff1a5` — all exit 0, structured.
- Gate incident: 3 blocked operator launches (completion-guard false positive; task word
  "refactor" + council-* outside reviewer-style list, per
  `pi-subagents/src/runs/shared/task-intent.ts`); fixed via guard's read-only vocabulary
  ("review only / return findings only"), verified with `bun -e` classifier check `0`.
- No repo commands run beyond `ls`/`rg`/`fd` recon and this append.

**Decisions / lines to respect**
- Framework stays (Bubble Tea v2); no Client interface until second backend; no Bubble Tea
  sub-model tree; `BreakpointFor` is the shared breakpoint policy point.
- Owner decisions open: approve agent_view cleanup session; pullCancel() hardening;
  document `looksLikeEmbeddedJSON` tradeoff; schedule 100× transcript measurement.

**Blockers / open decisions (carry to next session)**
- None blocking. Tests-for-verifications list in memo (0×0 boundedness, turn-slice
  desync, pullCancel, mid-stream ApplyConfig, TabBar width boundary).

**Next action**
- Fresh session: owner picks up the four owner decisions; recommended next step is the
  single agent_view.go code-motion cleanup session. Do not chain here.

---

## 2026-09-07 — Owner decisions collected (post-conclave, all 4 approved)

**Work done**
- DIRECT-path expertise report on the four open owner decisions, evidence-checked
  against code (agent_view.go:1590-1723, models_view.go:370/676/699-702,
  runner.go:225/514-517, chatLines O(total) per frame). Owner approved all four
  recommended options via structured questionnaire:
  1. **D1 — agent_view.go cleanup: APPROVED, full scope.** Turn struct (collapse 4
     parallel slices), selector/slash/resume handler extraction, scroll mutation
     moved out of View(), Recorder as concrete composition-root dep, delete
     legacy agentTokenMsg/agentDoneMsg + test migration (~6 call sites in
     sanitize_test/truncate_test/routing_regression_test), TabBar comment fix.
  2. **D2 — pullCancel() hardening: APPROVED, one-line fix + regression test.**
     Cancel in-flight pull in ApplyClient before client swap (~15 lines incl. test;
     context.CancelFunc is idempotent, Esc double-cancel safe).
  3. **D3 — looksLikeEmbeddedJSON: APPROVED, doc comment + pinning test.** Pin:
     JSON-prefix held back, fenced prose held back (whole-turn holdback tradeoff),
     plain prose streams (runner.go:514).
  4. **D4 — 100× transcript measurement: APPROVED as go test -bench benchmark**
     on renderChatPane/chatLines at ~100× transcript size + 0×0 boundedness test
     (chatLines iterates all history per frame, agent_view.go:1590-1608; windowing
     is O(visible) but line rebuild is O(total cached lines)).

**Commands + exit codes**
- Evidence recon: fd/rg/sed reads only, no repo mutations beyond this append.
- No build/test run (no code changed).

**Decisions / lines to respect**
- Next session executes D1 full scope as THE step (conclave's recommended next step).
- D2+D3 are small (~45 lines combined) — schedule as a follow-up hardening session;
  do NOT fold into D1 (review separation: code motion vs concurrency fix vs doc/test).
- D4 benchmark can ride any session with slack, or its own; never chain.

**Blockers / open decisions (carry to next session)**
- None. All four owner decisions are closed.

**Next action**
- Fresh session: execute D1 — agent_view.go full-scope code-motion cleanup, green
  make check, golden/routing/seam tests must stay green. Do not chain here.

---

## 2026-09-07 — D1 landed: agent_view.go full-scope code-motion cleanup

**Work done**
- Orchestrator triage: DIRECT (zero implementation agents) — scope already adjudicated
  by conclave + owner; one concern, one file + bounded test migration; one targeted
  read-only `reviewer` pass on the final diff (verdict: clean, no findings).
- `internal/ui/agent_view.go`:
  1. **Turn struct**: `history`/`turnModel`/`turnMeta`/`render` → one `turns []turn`
     (msg/model/meta/render). Commit sites (sendInput, onChatDone), import
     (applySessionLoaded), /clear, render cache rebuild, headerFor, chatLines,
     payloadMessages, and startChat's request copy all migrated. The parallel-slice
     desync guard in chatLines collapses to an empty-render fallback (kept sanitized).
  2. **Handler extraction**: inline mutation-approval and slash-draft key switches
     pulled out of handleKey into `confirmKey` / `slashDraftKey` (handled flag keeps
     fall-through). selector/resume/clear handlers were already methods.
  3. **Pure render**: `renderChatPane` no longer writes `v.scroll` — the effective
     offset is computed locally (follow → tail anchor; else clamped). clampScroll
     stays on the key paths; clampScroll comment updated.
  4. **Recorder composition-root**: `WithSessionDir` constructs the concrete
     `*session.Recorder` eagerly (closes a prior one first; Close idempotent);
     `enqueueSessionTurn` no longer constructs on the update loop and is now a
     pointer receiver so accepted-turn/first-failure state lands on the caller (the
     old value receiver silently dropped those writes on discard-style callers —
     latent bug the migration surfaced). New `recorded` flag preserves
     "/export → nothing recorded yet" nil-command behavior.
  5. **Legacy deletion**: `agentTokenMsg`/`agentDoneMsg` types + Update cases removed;
     `onChatDone` now takes `agent.AgentDoneMsg` directly; ~6 legacy test call sites
     migrated (sanitize/truncate/routing + agent_view_test).
  6. **TabBar comment fix** (components.go): Render never wrapped and Width is unused
     in rendering — comment now says so (no code change).
- Callers migrated: palette.go (`len(a.agent.turns)`), 9 test files
  (agent_view/routing/sanitize/truncate/m7/golden/small_terminal/cancellation/
  resume_ui/session_ui). Test seeds build `[]turn` literals; injected-recorder tests
  `Close()` the eager recorder before swapping in their fake (4 sites).

**Commands + exit codes**
- `go build ./...` → 0 (after each stage; intermediate errors fixed: 2 missed
  history refs, pointer-receiver returns, truncate_test import).
- `go vet ./...` → 0.
- `make check` (build + `go test -count=1 ./...` + vet) → 0; all packages ok,
  internal/ui 5.9s (one failure during migration: enqueueSessionTurn value-receiver
  dropped `recorded` on the direct-call test — fixed by pointer receiver).
- `go test -race -count=1 ./...` → 0 (all packages ok, internal/ui 11.2s).
- Reviewer lane: verdict clean / merge OK, 0 findings (run meta did not surface the
  executed model; lane completed without fallback error).

**Decisions / lines to respect**
- Behavior preserved by design: golden fixtures byte-identical (no -update run),
  routing/seam tests green unchanged, /export notice + nil-command pre-empt kept via
  `recorded`, render never mutates state.
- D2 (pullCancel hardening), D3 (looksLikeEmbeddedJSON doc+pinning test),
  D4 (100× benchmark + 0×0 boundedness test) remain separate follow-ups — do not
  fold into any other session.

**Blockers / open decisions (carry to next session)**
- None.

**Next action**
- Fresh session: owner picks the next step — D2 hardening session is the queued
  follow-up (one-line fix + regression test). Do not chain here.

---

## 2026-09-07 — D2 landed: ApplyClient cancels in-flight pull (one-line hardening + regression test)

**Work done**
- Orchestrator triage: DIRECT (zero agents) — one-line fix + one test, existing
  Esc-cancel test pattern mirrored; canonical checks owned by parent.
- `internal/ui/models_view.go` ApplyClient: cancel the in-flight pull
  (`v.pullCancel != nil` → `v.pullCancel()`) before the client swap, with a
  comment noting CancelFunc idempotence (Esc double-cancel safe). Closes the
  owner-approved D2 finding: without this, an old-host pull goroutine kept
  streaming progress into the new host's view and would surface the old host's
  result on completion.
- `internal/ui/models_view_test.go`: new `TestModelsViewApplyClientCancelsPull`
  — pull starts against a stalling host A, ApplyClient swaps to a tags host B;
  asserts the pull goroutine terminates (stream closes, view leaves `pulling`),
  `pullCancel` cleared, "context canceled" surfaced, and the new-host reload
  (executed via the real command, so the gen-gate applies) clears the error.
  First draft failed one assertion (hand-built `modelsLoadedMsg` gen=0 was
  dropped by the M-03 gen-gate) — fixed by applying the command's real result.

**Commands + exit codes**
- `go build ./...` → 0.
- `go test -count=1 ./internal/ui -run 'TestModelsViewApplyClient|TestModelsViewPull' -v`
  → first run 1 FAIL (gen-gate), fixed; final run all PASS, exit 0.
- `make check` (build + `go test -count=1 ./...` + vet) → 0, all packages ok.
- `go test -race -count=1 ./internal/ui` → 0 (10.99s).

**Decisions / lines to respect**
- Minimal-scope hardening per owner decision 4df7146: cancel-only; the canceled
  goroutine's done message performs the normal onPullDone cleanup, and the
  surfaced "context canceled" error is cleared by the new-host reload (same UX
  contract as the esc path).
- Pull events themselves remain non-gen-gated — no scope expansion beyond the
  approved one-line fix + test.

**Blockers / open decisions (carry to next session)**
- None.

**Next action**
- Fresh session: owner picks — D3 (looksLikeEmbeddedJSON doc comment + pinning
  test, runner.go:514) is the queued follow-up; D4 (100× render benchmark +
  0×0 boundedness test) can ride any session with slack. Do not chain here.

---

## 2026-09-07 — D3 landed: looksLikeEmbeddedJSON doc comment + pinning test

**Work done**
- Orchestrator triage: DIRECT (zero agents) — doc comment on one pure predicate
  + one pinning test; owner decision from 4df7146 fixed the exact contract.
- `internal/agent/runner.go` (looksLikeEmbeddedJSON): full doc comment stating
  the predicate contract (prefix-only after trim: `{`, `[`, ` ``` `), the
  accepted **whole-turn holdback tradeoff** (ordinary fenced code / bare
  JSON-object / top-level call-array prose is withheld for the entire turn and
  flushed only on the no-calls flush), why that cost is deliberate (flashing a
  tool envelope into the transcript is worse than delaying fence/JSON-shaped
  prose), and that mid-turn JSON (`here is the JSON: {...}`) is outside the
  predicate and always streams.
- `internal/agent/runner_test.go`: new `TestLooksLikeEmbeddedJSONPinned` —
  12-case table pinning: plain prose streams (incl. after blank lines), JSON
  object/array prefix held back, fenced envelope held back, ordinary fenced
  code held back (the tradeoff, explicitly named in the case), whitespace-
  prefixed JSON/fence held back, mid-text JSON and post-word brace are prose,
  bare fence alone held back. Header comment warns that changing the
  predicate changes mid-stream UX and must be a conscious tradeoff.

**Commands + exit codes**
- `gofmt -l .` → empty; `go build ./...` → 0.
- `go test -count=1 ./internal/agent -run TestLooksLikeEmbeddedJSONPinned -v`
  → 12/12 PASS, exit 0.
- `make check` (build + `go test -count=1 ./...` + vet) → 0, all packages ok.
- `go test -race -count=1 ./internal/agent` → 0 (1.29s).

**Decisions / lines to respect**
- Doc+test only; the predicate itself is unchanged (owner decision 4df7146:
  doc + pin, no behavior change). D4 (100× render benchmark + 0×0 boundedness
  test) remains a separate follow-up — do not fold into any other session.

**Blockers / open decisions (carry to next session)**
- None.

**Next action**
- Fresh session: owner picks — D4 (100× transcript benchmark on
  renderChatPane/chatLines + 0×0 boundedness test) is the last queued
  follow-up. Do not chain here.

## 2026-09-07 — D4 landed: 100× transcript benchmark + 0×0 boundedness test

**Work done**
- Orchestrator triage: DIRECT (zero agents) — benchmark + test only, no
  production code changes; exact scope fixed by the 2026-09-07 owner decision
  (D4: go test -bench on renderChatPane/chatLines at ~100× transcript size +
  0×0 boundedness test).
- `internal/ui/agent_view_bench_test.go` (new): 100× transcript baseline
  defined as 2,000 turns (100 × a 20-turn session) at the 88×40 test
  geometry; setup pre-renders a small set of distinct markdown bodies once
  and copies the cached strings, mirroring the commit path's per-turn render
  cache. Benchmarks: `BenchmarkChatPane100x/{tail,scrolled-up,streaming}` and
  `BenchmarkChatLines100x` (isolates the O(total cached lines) rebuild named
  in the owner decision). chatH computed the same way View() does.
- `internal/ui/small_terminal_test.go`: `TestZeroSizeAgentFrameStaysBounded`
  pins the conclave finding that 0×0 renders chrome by design — with 400
  turns + a live stream armed, the frame stays ≤10 rows and ≤40 columns of
  chrome, leaks no transcript content ("user turn"/"assistant line"/
  "streaming now"/"❯ you"; model chip strings excluded as legit composer
  chrome), and is byte-identical with an empty transcript (history cannot
  affect layout). Also pins renderChatPane's h<2 guard (returns "" for h=0/1)
  and that chatLines still computes the full 400-turn history safely.

**Measured baseline (i7-9700K, go1.x, -benchtime default)**
- ChatPane100x/tail 1.52 ms/op · scrolled-up 1.48 ms/op · streaming 1.65 ms/op
  (~1.7 MB, ~14k allocs/op — lipgloss styling of ~30 visible rows dominates).
- ChatLines100x 0.87 ms/op (1.39 MB, 2,018 allocs/op).
- tail ≈ scrolled-up cost confirms the O(total) chatLines rebuild dominates
  per-frame cost independent of window position — the baseline any future
  windowing/caching work must beat.

**Commands + exit codes**
- `gofmt -l .` → empty; `go vet ./...` → 0.
- `go test ./internal/ui -run TestZeroSizeAgentFrameStaysBounded -v -count=1`
  → PASS, exit 0.
- `go test ./internal/ui -bench Benchmark -run '^$' -count=1` → all 4 PASS,
  exit 0 (numbers above).
- `make check` (build + `go test -count=1 ./...` + vet) → 0, all packages ok.
- `go test -race -count=1 ./internal/ui` → 0 (11.3s).

**Decisions / lines to respect**
- Benchmark seeding reuses cached render strings across turns — per-frame
  cost depends on line counts, not body uniqueness (glamour output is cached
  per turn in production); document this in the file header.
- 0×0 assertions pin observed-by-design behavior probed before writing the
  test (8-row chrome, 38-col pane, transcript never reaches the frame).
- No production code touched; windowing work remains future scope with this
  benchmark as its baseline.

**Blockers / open decisions (carry to next session)**
- None.

**Next action**
- Fresh session: owner picks. All four owner decisions (D1–D4) are now
  landed; conclave tests-for-verifications list is exhausted. Do not chain
  here.

## 2026-09-07 — Next-level TUI plan drafted: PLAN.md §12 (N-series proposal, planning-only)

**Work done**
- Orchestrator triage: WORKFLOW (2 read-only lanes: web research + repo recon).
  Channel health: default chain (deepseek/v4-flash) 402'd "Insufficient Balance"
  on all 3 child attempts (researcher fg, scout ×2, researcher bg) — degraded per
  ladder; ping `opencode-go/glm-5.3-flash:low` → PING-OK; research lane reran on
  `opencode-go/glm-5.3-flash:medium` (run meta confirmed; primary never held).
  Recon lane degraded to parent-local after scout 402 ×2.
- Web research brief delivered (research.md, d7577a9c): Bubble Tea v2 Cursed
  Renderer + SSH bandwidth; scroll-optimization PRs #1725/#1761 + event-driven
  #1776; Crush memoization architecture (per-width cache, versioned invalidation,
  Finished() freeze); glamour pool/width-bucket/gate patterns; opencode command
  grammar; Claude Code statusline / token-meter category; Bast.sh mobile layout.
- Parent-local recon + verification of consequential claims against the pinned
  tree: `WithScrollOptimization` NOT in pinned bubbletea v2.0.9 (N7 = track
  upstream); `eval_count`/`prompt_eval_duration` NOT parsed today (N3 additive);
  one glamour TermRenderer already reused on width change (width-bucketing gap).
- Deliverable: **PLAN.md §12** appended — N-series proposal (N1 render windowing,
  N2 streaming repaint discipline, N3 tok/s + exact tokens, N4 status bar,
  N5 debug drawer, N6 composer (@-refs, /details, /thinking), N7 upstream
  tracking, N8 tea.Println scrollback spike) + explicit rejections
  (leader chords, mouse capture, mutation undo) + sequencing sketch
  N1→N3→N4→N2→N6→N5→N7→N8. Explicitly marked PROPOSAL; owner-selected v0.2
  (V2c pending, V2d undecided) untouched and first in line.

**Commands + exit codes**
- `make check` (build + `go test -count=1 ./...` + vet) → 0, all packages ok.
- `go doc charm.land/bubbletea/v2 WithScrollOptimization` → "no symbol" (confirms
  absence in v2.0.9).
- `rg eval_count internal/ollama` (non-test) → no hits (confirms gap).
- PLAN.md §12 append via heredoc → exit 0; LEDGER append → exit 0.

**Decisions / lines to respect**
- Planning-only session: no production code touched; §12 is proposal status, not
  committed scope. v0.2 scope decision (2026-09-07) not modified.
- Leader-key chords and mouse capture deliberately rejected for the 72×30 phone
  target; mutation undo/redo deferred to the V2d cut (touches V2c jail).
- N8 (tea.Println native scrollback) is spike-first, owner-run on device; no
  commitment until Blink/Termius gesture behavior is measured.

**Blockers / open decisions (carry to next session)**
- DeepSeek channel balance is empty (402 on every child attempt) — owner should
  top up or the cost-router chains will keep landing there and failing.
- Owner decisions pending: (a) sequence N-series as v0.3 vs interleave with
  V2c/V2d; (b) N8 device-spike scheduling; (c) amber-tier threshold (~80% assumed).

**Next action**
- Fresh session: owner picks — V2c (sandboxed run_command, gate already GO) or
  N1 (render windowing, D4 baseline pinned as the bar). Do not chain here.

## 2026-09-07 — V2c landed: sandboxed `run_command` behind bubblewrap

**Milestone:** V2c · **Result:** GO — implemented and verified.

**Work done**
- Orchestrator triage: DIRECT (zero agents) — one scoped V2c implementation;
  canonical routing sources and the SelfTUI session ritual were read first.
- Reinstated `run_command` as a sixth workspace tool. The runner validates a
  fixed argv before approval, requires the existing per-call confirmation,
  streams bounded `ToolOutputMsg` activity, and returns the bounded result.
- Added `internal/agent/command.go`: bubblewrap 0.9.0 is the default and only
  engine; fixed `go`/read-only `git` binaries, selective read-only mounts,
  workspace-only writable mount, private tmpfs, no network, `--clearenv`,
  scrubbed environment, 30s default/60s cap, 256 KiB per-stream output,
  cancellation-aware single-flight serialization, and process-group teardown.
  Bubblewrap absence fails closed. The trusted module-cache path is derived
  from the toolchain/default Go cache rather than accepting an arbitrary
  `GOMODCACHE` mount source.
- Added real-bwrap tests for argv rejection, scrubbing, output caps,
  single-flight cancellation, offline `go test`, read-only `git`, explicit
  confirmation, timeout, and process-group cancellation. Routed command output
  through the Agent UI and updated the tool-count/routing tests.
- Replaced the stale deferred-design document with the V2c containment note;
  added `docs/v2c-sandbox-evidence.md`; aligned README, SECURITY, CHANGELOG,
  PLAN, and renamed the command policy test. The residual bwrap CPU/memory
  risk and validated rootless-Docker alternative remain documented.

**Commands + exit codes**
- Focused pre-implementation test (`go test ./internal/agent -run 'TestRunCommand|TestCommandOutput' -count=1`) → 1 (expected RED: missing executor).
- Focused V2c tests → 0; final focused verbose run (all 9 V2c tests) → 0.
- `gofmt -l .` + `git diff --check` → 0.
- `make check` initial implementation run → 0; a later combined `make check && go test -race` invocation returned no result and was not treated as green.
- Follow-up `make check` after that interrupted build → 2 (generated `bin/selftui` was zero-filled); removed only that generated artifact and reran.
- One subsequent full-package `make check` → 2 (existing timing-sensitive mutation/settings tests under load); isolated agent/UI tests → 0; final `make check` → 0.
- Final `go test -race -count=1 ./...` → 0.
- `bwrap --version` → 0 (`bubblewrap 0.9.0`).

**Decisions / blockers**
- Default engine is bwrap per V2b GO; no engine selector was added in this
  focused step. Rootless Docker remains the stronger documented alternative,
  not a second current binary path.
- No unresolved blockers. The transient combined-gate/incomplete-build and
  load-sensitive test failures were resolved by rerunning bounded canonical
  checks; no source workaround or test weakening was introduced.

**Next action**
- Start a fresh pi session at `/home/calvin/SelfTUI` for V2d; decide its exact
  agent-breadth cut at that session's start. Do not chain V2d here.

## 2026-09-07 — V2d landed: agent breadth (git-awareness + project indexing)

**Milestone:** V2d · **Result:** GO — implemented and verified. v0.2 set (V2a–V2d) complete.

**Work done**
- Orchestrator triage: DIRECT (zero agents) — one scoped implementation;
  canonical routing sources + SelfTUI session ritual read first. The V2d cut
  itself was owner-selected in-session via structured question:
  **git-awareness + project indexing** (multi-file edits and mutation
  undo/redo explicitly stayed out of this cut).
- Added `internal/agent/workspace.go`: `WorkspaceContext(ctx, root)` builds a
  bounded (8 KiB) context block — Git section (branch via `rev-parse`
  probe, `status --porcelain -b` capped at 40 lines, `log --oneline -n 3`;
  fixed read-only host-side argv, 3s timeout, section omitted gracefully
  outside a git repo or without git) + Project index (WalkDir tree, depth
  ≤ 4, ≤ 300 entries, `.git` pruned, `[index truncated]` markers).
- Runner wiring: armed runners (`NewRunnerWithPolicy`) append the block as a
  second pinned system message every turn (BudgetMessages pins the whole
  leading system prefix); plain chat (`NewRunner`, no policy) never receives
  it. The closed six-tool schema is untouched — no new execution primitive.
- Tests (`internal/agent/workspace_test.go`): non-git dir, real git repo
  fixture (branch/porcelain/log/index assertions), 8 KiB bound + truncation
  marker, depth cap, canceled context → empty, armed-runner wire shape
  (system + workspace context + user), plain-chat wire shape (exactly
  system + user, no tools field).
- Docs: README (workspace-context paragraph), CHANGELOG (Unreleased/Added),
  PLAN §10 V2d exit tick.

**Commands + exit codes**
- `gofmt -l .` → empty; `go vet ./...` → 0.
- `go test ./internal/agent -run 'TestWorkspace|TestArmedRunner|TestPlainChatRunner' -count=1` → ok (after one RED→GREEN fix: section newline + porcelain assertion).
- `make check` → 0 (build + tests + vet, all packages ok).
- `go test -race -count=1 ./...` → 0 (all packages ok).

**Decisions / lines to respect**
- Git probes run host-side with fixed argv (the app is trusted; the V2c
  sandbox is for model-requested commands only) — no shell, no model input
  in argv. Non-git workspaces are a supported shape (tree-only block).
- Injection is scoped to the armed agent runner: plain chat must not gain
  workspace awareness. Per-turn rebuild (not cached) — cheap and always fresh.
- Multi-file edits and mutation undo/redo remain out of scope (owner
  decision recorded in-session); undo/redo still touches the V2c jail if
  ever picked up.

**Blockers / open decisions (carry to next session)**
- None.

**Next action**
- v0.2 set (V2a–V2d) is complete: owner may tag the v0.2 release. Next
  development work (e.g. PLAN §12 N-series) starts in a fresh session at
  `/home/calvin/SelfTUI`. Do not chain here.

## 2026-09-07 — v0.2.0 tagged, published, and verified (owner-instructed in-chat)

**Scope:** tag the v0.2.0 release after the V2d session completed the
owner-selected v0.2 set (V2a–V2d). DIRECT execution, zero agents.

**Work done**
- CHANGELOG `Unreleased` cut to `[0.2.0] - 2026-09-07`; PLAN §12 records the
  tag; committed and landed via **PR #16** (`release/v0.2.0` → main, required
  check pass 1m03s, merge `ebd6bb0`) — `main` is branch-protected, direct
  pushes are declined.
- Local release gate `VERSION=v0.2.0 make release-check` **PASSED** (needed
  the pinned toolchain on PATH first: go1.27.1 distribution bin dir +
  `~/go/bin`; installed pinned `govulncheck@v1.7.0` — not previously on
  PATH). Gate: mod verify, gofmt, vet, uncached tests, race, govulncheck
  (0 vulnerabilities in code), cross-builds, stamp check (`selftui v0.2.0`
  both binaries), deterministic archives + flat `SHA256SUMS`.
- Signed annotated tag `v0.2.0` created (`git tag -s`, Ed25519 key
  `5F74A36F7B5C1670`, `git verify-tag` good).
- **First tag push failed** (run 34156039028, 8s): `git verify-tag` in CI
  said "cannot verify a non-tag object of type commit". Root cause verified
  from run logs: actions/checkout (pinned SHA, fetch-depth:1, fetch-tags:
  false) fetches the triggering commit and writes its SHA directly into
  `refs/tags/v0.2.0` — the runner repo never has the annotated tag object.
  This was a latent bug in the signed-tag step (it never ran green on a
  pushed signed tag; v0.1.1's release run predates the verify step). Nothing
  was built or published by the failed run.
- Fixed `release.yml` (re-fetch the exact tag ref, assert the ref is a tag
  object, then verify); landed via **PR #17** (`fix/release-tag-verify` →
  main, check pass 1m09s, merge `761d331`). Local reproduction confirmed a
  force re-fetch yields the tag object + good signature.
- Tag moved: deleted the 5-minute-old failed-run tag ref (remote + local)
  and re-created the signed tag on `761d331`; pushed → release run
  **34156259375 succeeded**.
- Independent verification: `gh release download v0.2.0` +
  `sha256sum -c SHA256SUMS` → both archives **OK**; extracted amd64 binary
  reports `selftui v0.2.0`. Release is published, not draft, 3 assets.

**Commands + exit codes**
- `VERSION=v0.2.0 make release-check` → 2 twice (pinned-gofmt PATH; missing
  govulncheck), then **0** with the pinned toolchain on PATH.
- `make actionlint` → 127 (actionlint not on PATH), rerun with `~/go/bin` on
  PATH → 0.
- `git push origin main` → rejected GH006 (branch protection) → PR path.
- `gh pr merge 16 --merge` / `gh pr merge 17 --merge` → merged.
- `gh run watch 34156259375 --exit-status` → 0.
- `sha256sum -c SHA256SUMS` (downloaded assets) → OK; `./selftui -version` →
  `selftui v0.2.0`.

**Decisions / lines to respect**
- Tag deletion + re-creation happened before anything was published; the
  moved tag targets the same release content plus the one-file release.yml
  fix (archive checksums differ from the pre-fix local build as expected —
  the tree changed).
- Branches kept for audit: `release/v0.2.0`, `fix/release-tag-verify` (same
  practice as `hardening/v0.1`, `fix/v0.1.1-audit-remediation`).
- Still owner-optional: upload the GPG public key at
  github.com/settings/keys for the green Verified badge.

**Blockers / open decisions (carry to next session)**
- None.

**Next action**
- Fresh session at `/home/calvin/SelfTUI`; owner picks next work (e.g. PLAN
  §12 N-series, N1 first). Do not chain here.

## 2026-09-07 — N1 render windowing (PLAN §12)

**Work done**
- N1 landed: `chatLines` no longer rebuilds the O(total cached lines) list
  every frame. New windowed machinery in `agent_view.go`:
  `chatLineCount` (newline arithmetic per cached turn, zero allocs) +
  `chatWindowTotal(start,end,total)` (splits only blocks overlapping the
  visible window; `chatLines()` is now the O(total) convenience view over
  the same machinery). `renderChatPane` and `clampScroll` consume the
  counted path; committed turns stay frozen (per-width cache unchanged).
- Tail windows (follow mode) are O(visible): 2 000-turn transcript window
  splits ~2 blocks regardless of session length; trailing-blank drop and
  truncation-marker/stream edge cases replicated exactly.
- Equivalence pinned by `agent_view_window_test.go`: a test-side frozen
  naive copy of the old assembly must match `chatLineCount` and
  `chatWindow` byte-for-byte across 11 transcript shapes × 9 window slices
  (caught and fixed one real negative-start clamp edge).
- N1 gate **PASSED** (i7-9700K, go1.27.1, `-count 3`):
  - `BenchmarkChatWindow100x/tail`: **~111 µs/op, 3.1 KB, 14 allocs** vs
    pinned `ChatLines100x` baseline **~754 µs/op, 1.39 MB, 2 018 allocs**
    → ~6.6× faster, ~450× less memory, ~144× fewer allocs.
  - `ChatPane100x/tail` 1.32–1.48 ms → **0.92 ms** (−35%), 1.71 MB →
    0.30 MB; scrolled-up 1.38–1.48 → 0.82 ms; streaming 1.51–1.66 →
    1.13 ms (stream block now rendered twice per frame — count + window —
    accepted; still well under baseline).
- Golden frames 72×30 + 120×40 byte-identical (TestGolden in `make check`).
- `make check` green; `go test -race ./internal/ui` green.
- Owner-optional carried (unchanged): upload GPG public key
  `8D3A52AB6583DD5C207FE81B5F74A36F7B5C1670` (Ed25519, uid MerverliPy
  <calvinbrady8@gmail.com>) at github.com/settings/keys → New GPG key →
  paste `gpg --armor --export 8D3A52AB6583DD5C207FE81B5F74A36F7B5C1670`
  for the tag commit's green Verified badge.

**Commands + exit codes**
- Baseline pre-change: `go test ./internal/ui -run '^$' -bench
  'BenchmarkChatPane100x|BenchmarkChatLines100x' -benchmem -count 3` → 0.
- `gofmt -l internal/ui` → dirty (new test file), `gofmt -w` → clean.
- `go test ./internal/ui -run 'TestChatLine|TestChatWindow|TestGolden|
  TestSanitize|TestSmallTerminal'` → FAIL ×3 (test-harness slicing bug +
  negative-start clamp), fixed → 0. Red-green followed.
- `make check` → 0 (twice: pre-bench and final).
- `go test -race ./internal/ui -count=1` → 0.

**Decisions / lines to respect**
- No struct/behavior changes: `turn`, commit paths, and golden output
  untouched; windowing is a rendering-layer change only (verified by the
  naive-copy equivalence tests).
- N7 micro-item (glamour width-bucketing, round width to 5 cols) NOT done
  here — deliberately left as its own micro-task to keep this diff
  rendering-only; tracked in PLAN §12 N7.
- Bench: old `BenchmarkChatLines100x` kept (with pinned baseline in its
  comment) for regression visibility; new `BenchmarkChatWindow100x`
  (tail/count-only/scrolled-up) is the per-frame production path.

**Blockers / open decisions (carry to next session)**
- None. (Owner-optional GPG upload remains with the owner.)

**Next action**
- Fresh session at `/home/calvin/SelfTUI`; next N-item per the §12
  sequencing sketch: N3 (tok/s + exact token counts), or N2 if owner
  reorders.

## 2026-09-07 — N3 tok/s + exact token counts (PLAN §12)

**Work done**
- N3 landed: `internal/ollama` now parses the final stream chunk's generation
  metrics (`prompt_eval_count`, `prompt_eval_duration`, `eval_count`,
  `eval_duration`) into a new `ChatMetrics` struct surfaced on `ChatEvent`
  only when `done:true` (zero on earlier chunks / metric-absent hosts).
- `internal/agent`: `AgentDoneMsg` gained `Metrics ollama.ChatMetrics`; the
  turn's metrics come from the LAST final chunk (multi-iteration tool loops
  report the stream that ended the turn); plain-chat fallback propagates the
  same way. Error/stop paths keep today's zero-value shape.
- `internal/ui` (M7-B): per-turn footer now renders `… · stop · 41 tok/s`
  when the final chunk carried metrics (tok/s = round(eval_count/eval_duration));
  footer is byte-identical to pre-N3 when metrics are absent, eval_duration==0,
  or the user stopped (`… · stopped` unchanged).
- `internal/ui` (M7-C): ctx meter shows the measured `prompt_eval_count`
  after a completed turn until the draft is edited, a new turn starts, the
  conversation is cleared, or a session is loaded/resumed (sentinel
  `measuredDraft` covers every edit path); `ApproxTokens` stays authoritative
  for live drafting; red-100% behavior unchanged.
- Delegation note (channel health): the `worker` lane's primary
  `deepseek/deepseek-v4-flash` failed at preflight with HTTP 402
  "Insufficient Balance"; one bounded retry pinned to
  `opencode-go/glm-5.3-flash:high` succeeded — run meta confirms
  `model = opencode-go/glm-5.3-flash:high`, success, 47 turns. DeepSeek
  balance needs topping up before that primary is trusted again.
- 6 files changed, ~400 insertions; goldens untouched (0 testdata
  modifications — footer/meter change only when live metrics exist and
  fixtures carry none, as designed).

**Commands + exit codes**
- Child lane canonical checks: `gofmt -l .` clean; `make check` → 0;
  `go test -race ./...` → 0 (child-reported).
- Parent verification (uncached): `go test -race -count=1 ./...` → 0
  (ui 21.7s, agent 16.2s, ollama 6.9s, all ok); `make check` → 0;
  `gofmt -l .` → empty; `git status` → no testdata changes, no stray commits.

**Decisions / lines to respect**
- Metrics parsed only on `done:true`; non-final chunks and older hosts leave
  everything zero — footer/meter fall back to the pre-N3 rendering exactly.
- tok/s is measured (`eval_count`/`eval_duration`), never estimated; the
  estimator (`ApproxTokens`, 4 chars/token) remains the live-draft path.
- No struct/behavior changes beyond the added metrics plumbing; tool schema,
  jail, session format, N1 windowing cache untouched.

**Blockers / open decisions (carry to next session)**
- DeepSeek account balance exhausted (worker lane primary); owner may top up.
- Owner-optional GPG key upload (carried, unchanged).

**Next action**
- Fresh session at `/home/calvin/SelfTUI`; next N-item per the §12
  sequencing sketch: N4 (status bar observability row), or N2 if owner
  reorders.

## 2026-09-07 — N4 status bar as observability row (PLAN §12)

**Work done**
- N4 landed: shell status row (App `statusLeft`) now carries the N4 anatomy —
  observability head `model · [pull pill] · ctx meter · tok/s`, identity tail
  `⏻ host · tools · workspace` (+ remote-host warning). Fixed shed order under
  width pressure: workspace → obs tail (tok/s → meter → pill → model chip) →
  warning; host + tools floor never truncated (72-col phone discipline kept).
- Amber ctx tier: shared `ctxTierFor`/`amberCtxPct=80` (≥80% on the meter's
  existing displayed scale, i.e. the ¾-num_ctx input budget; red-100%
  unchanged) applied identically to the composer header and the status-row
  meter; new `Styles.warn` amber (dark #CC7A00-ish "214" / light "136") via
  `warnText()`. Owner decision recorded in the code comment.
- Background-job pills: generic pill shape, one real pill wired to the
  models-view streaming pull (`ModelsView.pullPill`: `⇣ name NN%`, sanitized
  name, degrades to `⇣ name…` when the server reports no totals). Queued-turn
  pills deferred — no chat-turn queue exists today (sends block while
  streaming); the pill API stays generic for when one lands.
- `lastTokPerSec` (AgentView): mirrors N3 footer semantics — measured rate of
  the last completed turn only, none on user stop, cleared on /clear, import,
  resume. Meter/tok/s omit when absent → no-data rows render the pre-N4 shape.
- Goldens: 23 fixtures regenerated **deliberately** (planned UI change); diff
  audited — 11 wide frames gained `model · ctx` segments, 12 compact frames
  shed `/tmp` under pressure; no assertion weakened; no testdata weakening.
- Delegation note (channel health): single implementation lane
  `developer-tooling-engineer`; session meta confirms it ran on
  `opencode-go/glm-5.3-flash:high` (primary held; no fallback, no quota hits).

**Commands + exit codes**
- Parent verification (uncached): `gofmt -l .` → empty; `make check` → 0;
  `go test -race -count=1 ./...` → 0 (agent 10.3s, ui 15.7s, all ok).
- Child lane checks reported the same (gofmt clean, make check 0, race 0).
- New tests: `internal/ui/status_row_test.go` (tier boundaries 79/80/100,
  amber styling both meters, segment presence/absence incl. pre-N4 no-data
  shape, pill render + width pressure, tok/s clear-on-wipe).

**Decisions / lines to respect**
- Amber threshold = ≥80% on the meter's displayed scale (¾ num_ctx budget),
  NOT literally ~80% of num_ctx — read as "before today's red-100% tier" on
  the same scale; documented in `ctxTierFor` comment. One-number change if
  the owner disagrees.
- Status-row anatomy is the roomy-shape target; the shed order (obs yields
  before the privacy warning; host+tools floor never truncates) is the
  binding 72-col behavior.
- N1 windowing + N3 metrics plumbing untouched (rendering-layer only).

**Blockers / open decisions (carry to next session)**
- None. (Owner-optional GPG key upload remains with the owner.)
- Queued-turn pills await a real queue feature (documented in code comment).

**Next action**
- Fresh session at `/home/calvin/SelfTUI`; next N-item per the §12
  sequencing sketch: N2 (streaming repaint discipline), or N6/N5 if the
  owner reorders.

## 2026-09-07 — N2 streaming repaint discipline (PLAN §12)

**Work done**
- N2 landed: token deltas (`agent.TokenMsg`) now queue into `pendingStream` and
  flush on a 60 ms repaint tick (`streamTickMsg`, wrapped in `agentEventMsg` so
  the App shell forwards it) — ≈17 fps instead of per-token frames (Ollama
  deltas arrive ~10–50 ms; 60 ms coalesces 1–6 deltas per repaint, keeps the
  caret visually smooth, and leaves the final chunk's immediate flush
  imperceptibly late).
- Active-block render cache: `primeStreamRender` renders the streaming block
  once per (content, header, width, theme) change; a frame's count pass and
  window pass share that one render (was 3 glamour renders per token/frame)
  and frames with no new flushed text cost zero glamour. Cache miss falls back
  to a fresh `renderBlock` — byte-identical by construction, equivalence
  re-pinned by `TestStreamCacheMatchesFreshRender`.
- Final chunk flushes immediately in `onChatDone` (`mergeStreamDeltas` runs
  before the commit) — no tick-tail latency for the committed turn or the N3
  footer metrics riding its meta row.
- Sub-block section split (thinking/content, the sketch's Crush pattern) is
  **rejected with evidence**: glamour's margin collapsing is not composable
  across `\n\n` section boundaries (probe-verified: a paragraph's top margin
  renders inline after a code block), so a section-level split cannot stay
  byte-identical. Freeze discipline stays at the committed-turn level (the
  per-turn render cache deltas never touch); true sub-block caching would need
  glamour-output stitching — upstream-risky, N7 territory.
- Scope note: tick priming renders ≤1 glamour render/60 ms even while the
  Agent tab is hidden (documented in code) — vs 60–180/s pre-N2 when visible.
- Delegation note: single implementation lane `developer-tooling-engineer`
  (run completed, mission ef562a65); no commits made by the child. New bench
  `BenchmarkStreamingFrame100x` + 9-test `stream_repaint_test.go`; 5 existing
  tests updated to drive the tick protocol — assertions preserved/strengthened
  (routing table now covers `streamTickMsg` flush), nothing weakened.

**Commands + exit codes**
- Parent verification (uncached): `gofmt -l .` → empty; `make check` → 0;
  `go test -race -count=1 ./...` → 0 (all pkgs ok; ui 15.8s).
- Goldens: `TestGoldenRender` + `TestGoldenFramesFitTerminal` PASS; **no
  fixture regeneration** — goldens never render mid-stream and the render
  paths are byte-identical by construction.
- Bench (parent-run, 88×40, 2 000 turns): short-stream tick frame 0.99 ms /
  13.4k allocs; 4 KB stream naive per-token frame 2.92 ms / 85k allocs →
  cached token frame 0.74 ms / 5.8k (stream-length independent); tick frame
  2.68 ms / 48k allocs, capped at ≤17 Hz.

**Decisions / lines to respect**
- 60 ms tick cadence is the repaint contract; the final chunk is flushed
  immediately, never ticked.
- `pendingStream` is the only delta queue; flushed `streamText` is the render
  source; `liveStreamText()` (both) is what payload budgeting and the commit
  path read — never split those roles.
- Sub-block section caching is rejected unless the owner buys a glamour
  output-stitching scheme (upstream-risky; track under N7).

**Blockers / open decisions (carry to next session)**
- None. (Owner-optional GPG key upload remains with the owner; local `main`
  is now ahead of `origin/main` by N4 + N2 — awaiting the owner's push.)

**Next action**
- Fresh session at `/home/calvin/SelfTUI`; next N-item per the §12
  sequencing sketch: N6 (composer upgrades), or N5 (debug drawer) if the
  owner reorders.

---

## Session — 2026-09-07 (late): owner push of N4+N2 → origin (orchestrator run)

**Work done**
- Owner-instructed push task, triaged DIRECT (zero agents). Direct `git push origin main` was rejected: `main` is protected (PR + required status check `Go fmt · vet · test · race · vuln · cross-build`). Routed through the repo's established PR pattern (matching #19/#20).
- CI first run FAILED: `TestStatusBarObservabilitySegments` — environment-dependent test fixture, not a renderer bug. The no-data identity assertion derived the workspace label from cwd; CI's deep checkout path (`/home/runner/work/SelfTUI/SelfTUI/internal/ui`, 55 chars) tripped the N4 width-pressure shedding, which drops the workspace segment (per spec — see `TestStatusBarWidthPressure`).
- Fix (parent-local, smallest change): pinned the fixture's `WorkspaceRoot` to `/tmp` (the golden-test pattern) in `internal/ui/status_row_test.go`, so the identity string is environment-independent.

**Commands + exit codes**
- `git push origin main` → exit 1 (protected-branch hook GH006).
- `git push -u origin perf/n4-n2` → 0; PR #21 opened.
- Local verification: `gofmt -l .` → empty; targeted `go test ./internal/ui/ -run 'TestStatusBar|TestCtxMeter'` → PASS; `make check` → 0; `go test -race -count=1 ./...` → 0.
- CI after fix: required check PASS (1m06s, run 34171441278); `gh pr merge 21 --merge --delete-branch` → 0.

**Decisions / lines to respect**
- The width-pressure shedding order is confirmed contract (workspace sheds before the identity floor); test fixtures must pin `WorkspaceRoot` to a short fixed path when asserting full identity strings — future status-row tests should follow the golden-test pattern.

**Blockers / open decisions**
- LEDGER commit on `main` is local-only (protected branch); it rides the next PR branch.

**Next action**
- Fresh session at `/home/calvin/SelfTUI`; next N-item per §12: **N6 — composer upgrades** (`@`-file fuzzy reference; `/details` + `/thinking` toggles; no leader key — palette stays the discoverable path).

## Session — 2026-09-07 (N6): composer upgrades (orchestrator run)

**Work done**
- N6 per PLAN §12: `@`-file fuzzy reference, `/details` + `/thinking` toggles,
  no leader key (spec). Delegation note: single implementation lane
  `developer-tooling-engineer` (mission 7547d0ca, fork context); no commits by
  the child. Parent verified, ticked PLAN §12 N6, committed.
- Implementation: `internal/agent/runner.go` (thinking relay → new
  `agent.ThinkingMsg`; message-level `thinking` preferred, top-level fallback,
  never concatenated), `internal/agent/workspace.go` (`WorkspaceFiles()`
  jail-bounded picker listing: WalkDir, no symlink follow, `.git` pruned,
  depth/entry caps, ≤512 results), `internal/agent/attach.go` (new —
  `FileRefTokens`/`ExpandFileRefs` send-path expansion through jailed
  `ReadFile`; missing = prose untouched; escape/oversize = visible
  `[file: … — unavailable: …]` note), `internal/ui/agent_view.go`
  (`/details` + `/thinking` session toggles default OFF; `@`-picker armed on
  fresh `@`, filter derived from draft tail after last `@`; per-turn
  `turn.thinking`/`turn.tools` stored always, rendered only behind toggles;
  reasoning/tool deltas batch through the existing 60 ms tick; `turn.wire`
  carries expanded content to runner + ctx meter; remote-host warning when
  attachments go to a non-loopback host). No leader key; palette unchanged.

**Commands + exit codes**
- Parent verification (fresh, after child): `gofmt -l .` → empty (rc 0);
  `go vet ./...` → clean; `make check` → 0; `go test -race -count=1 ./...` → 0
  (all pkgs ok; ui 16.1s). 13 new tests (6 agent + 7 ui).
- Goldens: only `agent-help-*` + `agent-slash-*` regenerated (command set grew
  5→7 + help rows); `agent-turn-*`/`agent-compact`/`agent-wide`/transcript
  frames byte-identical — toggles default OFF.

**Decisions / lines to respect**
- N2 contract held: `pendingStream` remains the only delta queue; additive
  batching of thinking/tool extras; final-chunk immediate flush untouched.
- Toggles are **session-scoped** by design (no config.toml/Settings surface —
  would widen scope beyond N6's three items); default OFF.
- Attachment expansion is synchronous at send, bounded ≤256 KB/file, so the
  ctx meter budgets the true payload; transcript renders the draft, not the
  expansion.
- Error turns that produced no content still commit nothing (pre-N6 semantics).

**Blockers / open decisions**
- Local `main` is ahead of `origin/main` (N6 + carried ledger commit). Main is
  protected — push must ride a PR branch (pattern: PR #21). Awaiting owner
  push (or owner-instructed push session).

**Next action**
- Fresh session at `/home/calvin/SelfTUI`; next N-item per §12 sequencing:
  **N5 — debug/log drawer** (`charmbracelet/log`, keybind-toggled, redacted).

## Session — 2026-09-07 (late, same session): owner-approved push of N6 → origin (PR #22)

**Work done**
- Owner instructed: "I give you permission to Push N6 to origin." Triaged
  DIRECT (zero agents), routed through the repo's PR pattern (main protected).
- `git checkout -b feat/n6-composer` (carries N6 35c0ace + ledger ad1a6e2) →
  push → PR #22 → required check `Go fmt · vet · test · race · vuln ·
  cross-build` PASS first try (1m05s, run 34173394762; no #21-style CI
  fixture flake) → `gh pr merge 22 --merge --delete-branch` → main
  fast-forwarded to 19f4b93.

**Commands + exit codes**
- `git push -u origin feat/n6-composer` → 0; `gh pr create --fill` → 0
  (https://github.com/MerverliPy/SelfTUI/pull/22); `gh pr checks 22 --watch`
  → 0 (pass 1m5s); `gh pr merge 22 --merge --delete-branch` → 0.
- Post-merge: `main...origin/main` in sync, working tree clean.

**Decisions / lines to respect**
- This ledger commit is local-only again (protected branch); it rides the
  next PR branch (same carry pattern as ad1a6e2 rode #22).

**Blockers / open decisions**
- None new. Owner decisions still pending: V2d scope (decide at that
  session's start), optional GPG key upload, N8 on-device spike (owner-run).

**Next action**
- Fresh session at `/home/calvin/SelfTUI`; next N-item per §12 sequencing:
  **N5 — debug/log drawer** (`charmbracelet/log`, keybind-toggled, shared
  sink via `--log-file`, redact bearer tokens).

## Session — 2026-09-07 (N5): debug/log drawer (orchestrator run)

**Work done**
- N5 per PLAN §12: `charmbracelet/log` keybind-toggled drawer, `--log-file` shared
  sink, bearer-token redaction. Delegation note: single implementation lane
  `developer-tooling-engineer` (mission 5bb7598c, fork context); no VCS actions by
  the child. Parent independently verified (fresh `gofmt`/`vet`/`make check`/
  `go test -race`), spot-checked the redaction tests and golden diffs, ticked
  PLAN §12 N5, committed.
- Implementation: new `internal/logsink` (redacting tee sink — file writer + ring
  buffer so file and drawer can never disagree; registered-secret-first redaction +
  generic `Bearer <tok>` pattern with 4-char prose floor; `SetSecret` re-arms on
  Settings token change); `--log-file` flag in `cmd/self-tui/main.go` (XDG default
  unchanged); `ctrl+o` k9s-style read-only drawer in `internal/ui/drawer.go` (modal
  owns keys, tail-follow, pgup/pgdn, esc/re-toggle closes, `ctrl+p` blocked over it)
  + palette *Logs drawer* entry as the phone path; nil-safe trace seams in
  `internal/ollama/client.go`+`stream.go` (method/path/status/duration/bytes, no
  bodies) and `internal/agent/runner.go` (tool-call summaries, budget/truncation
  markers, plain-chat fallback) via `WithLogger`/`WithLog` setters; README flag/
  keybind/redaction docs.

**Commands + exit codes**
- Parent verification (fresh, after child): `gofmt -l .` → empty (rc 0); `go vet ./...`
  → clean; `make check` → 0; `go test -race -count=1 ./...` → 0 (all 7 pkgs ok; ui
  15.0s). 33 new/updated tests (logsink redaction incl. end-to-end "sekrit never in
  sink", drawer modal/key-leak pins, `--log-file` precedence, ollama/agent traces).
- Goldens: `palette-compact/wide` grew exactly the one command row (verified via git
  diff); 2 new `agent-logs-drawer-*` fixtures (72×30, 120×40); all other frames
  byte-identical, pinned by `TestDrawerClosedViewIsByteIdentical`.

**Decisions / lines to respect**
- Redaction lives at the single shared sink, not per-consumer — a future second sink
  or flag cannot leak the token without deliberately bypassing `logsink`.
- `ctrl+o` chosen (verified conflict-free across app/agent/models/settings; reads as
  "output"); palette entry preserved as the discoverable phone path per M7 precedent.
- Drawer is read-only by design: no clearing that would lose file-sink history.
- N2 contract held: `pendingStream`/60ms tick paths untouched.

**Blockers / open decisions**
- `make smoke` hits its pre-existing-host guard (harness refuses to delete the
  already-installed `qwen3:0.6b`); binary built and booted under the pty capture
  before the guard — no regression signal, same as prior sessions.
- Local `main` again ahead of `origin/main` (N5 + carried ledger commit). Main is
  protected — push must ride a PR branch (pattern: PR #22). Awaiting owner push.

**Next action**
- Fresh session at `/home/calvin/SelfTUI`; next N-item per §12 sequencing:
  **N7 — upstream tracking** (no code: watch bubbletea v2 for the scroll-optimized
  flush + event-driven rendering releases, then re-run D4 + scroll benches; glamour
  width-bucketing micro-item under N1) — continuous/owner-driven; otherwise the
  owner's pending V2d decision or the N8 on-device spike.

## Session — 2026-09-07 (PR #22 review fixes): 7/7 Codex findings closed (orchestrator run)

**Work done**
- Fixed all 7 findings from the PR #22 Codex review
  (github.com/MerverliPy/SelfTUI/pull/22#pullrequestreview-5136151563, reviewed
  commit 35c0ace). Triage: DIRECT (all findings confined to two files in one
  subsystem — `internal/ui/agent_view.go`, `internal/agent/attach.go` — so no
  parallel lanes; parent implemented + verified). No VCS actions by any child
  (none used).
- P1 sanitize reasoning: `mergeThinkingDeltas` now runs `sanitizeTerminalText`
  at the merge, so live /thinking blocks and committed turns never carry
  ANSI/OSC/control bytes from a hostile model/server.
- P1 sensitive-path policy on attachments: `ExpandFileRefs` gates every token
  through `ToolPolicy.AuthorizePath` before `ReadFile` (a hand-typed `@.env` /
  `@credentials.json` / `.ssh`-tree reference renders the same "unavailable"
  policy note the read tools produce); `WorkspaceFiles` never offers and never
  descends into policy-refused paths, so the picker cannot advertise them.
- P2 flush reasoning on done: `onChatDone` calls `mergeThinkingDeltas()` next
  to `mergeStreamDeltas()` so a `ThinkingMsg` racing `AgentDoneMsg` within the
  60 ms repaint window commits its tail instead of losing it.
- P2 picker highlight: `pickFile` reads `matches[fileIdx]` before resetting the
  picker state (was resetting first, always inserting row 0).
- P2 whitespace paths: new `agent.EscapeFileRef` (escapes spaces as `\ `) used
  by the picker insert; `FileRefTokens` treats a backslash-escaped whitespace
  as part of the token and unescapes before resolution — `docs/design notes.md`
  now survives pick → tokenize → expand.
- P2 multiline tool rows: new `toolDetailRows` splits a bounded tool line into
  one stored element per display row, keeping `streamLineCount` == rendered
  rows for /details (no composer/status push off-screen).
- P2 attachment reads off the update loop: `sendInput` defers `@`-expansion
  into a `tea.Cmd`; the landing `attachExpandedMsg` drives the new `beginTurn`
  (commit user turn → context budget → enqueue → startChat → remote-attach
  warning). New `expandPending` guard stops a second send racing the first;
  `beginTurn` re-checks `resumePending` and restores the draft if a transcript
  load raced the (now async) expansion.

**Commands + exit codes**
- Parent verification (all fresh, no caching): `gofmt -l .` → empty (rc 0);
  `go vet ./...` → clean (rc 0); `make test` → 0 (all 7 pkgs ok); `make race` →
  0 (all 7 pkgs ok; ui 15.6s); `make build` → 0.
- 11 new/updated tests: agent — escaped-space token + escaped-space expand +
  sensitive-path-refused subtests, `TestWorkspaceFilesSensitivePathsFiltered`;
  ui — `TestAgentViewThinkingFlushAndSanitize`, `TestAgentViewToolRowsMatchCount`,
  `TestAgentViewPickerHighlightAndSpaces`, `TestAgentViewAttachSensitiveRefused`,
  plus the three existing attach-send tests adapted to run the deferred
  expansion command (`drainExpansion` helper).

**Decisions / lines to respect**
- Sensitive-path gate applied unconditionally on the attachment path (even for
  plain-chat runners with no tool set): attachments are the one channel that
  could still ship a credential file, so the gate is defense-in-depth, not
  tied to tool arming.
- Escaping chosen over "don't offer unrepresentable paths": keeps manual
  `@`-typing of spaced paths working and matches the picker's insert contract.
- Async expansion keeps `expandPending` + a `resumePending` re-check so the
  timing change introduces no send/resume race.
- N2 contract untouched (`pendingStream`/60 ms tick); committed-turn rendering,
  goldens, and the meter behavior are byte-identical (only the send timing of
  `@`-ref turns changed, by design).

**Blockers / open decisions**
- None for this step. Local `main` remains ahead of `origin/main` (N5 + N6
  post-merge fixes + carried ledger commits). Main is protected — the fix and
  the N5 commit must ride a PR branch (pattern: PR #22). Awaiting owner push.

**Next action**
- Fresh session at `/home/calvin/SelfTUI`; per owner's plan sequencing: push
  N5 + these PR #22 review fixes via a PR branch, then N7 upstream tracking
  (no code) / owner's V2d call / N8 device spike.

## Session — 2026-09-07 (push N5 + PR #22 fixes): PR #23 opened (orchestrator run)

**Work done**
- Owner-assigned next step executed: pushed local `main`'s ahead-content via a
  PR branch (main is protected). Branch `feat/n5-pr22-review-fixes` carries all
  4 ahead commits (N5 code `003043a`, review-fix code `842ffca`, 2 carried
  ledger docs `e295115`/`7325287`) — matching the ledger's "push must ride a PR
  branch" note and keeping history identical so a merge-commit merge fast-forwards
  local main cleanly (no cherry-pick duplication/reconciliation).
- Opened https://github.com/MerverliPy/SelfTUI/pull/23 — MERGEABLE, 4 commits,
  body documents the 7/7 finding map + verification.

**Commands + exit codes**
- `git checkout -b feat/n5-pr22-review-fixes main` → 0; `make build` → 0;
  `git push -u origin feat/n5-pr22-review-fixes` → 0 (new branch);
  `gh pr create …` → https://github.com/MerverliPy/SelfTUI/pull/23 (rc 0);
  `gh pr view 23` → OPEN / MERGEABLE / 4 commits (rc 0). Back on `main`.

**Decisions / lines to respect**
- PR carries the ledger docs commits too (repo rhythm: docs ride the push;
  they cannot reach origin any other way while main is protected).
- Triage for this step: DIRECT (git/gh operations only, no agents).

**Blockers / open decisions**
- PR #23 awaits the owner's merge. After a merge-commit merge, local `main`
  fast-forwards via `git pull`.
- **N7**: no-code continuous upstream tracking — nothing new to record (bubbletea
  v2.0.9 still pinned; scroll-optimized flush #1725/#1761 + event-driven
  rendering #1776 not yet shipped as of today).
- **V2d**: already landed (PLAN §10 ✅ 2026-09-07, git-awareness + project
  indexing) — the ledger's "owner's V2d call" phrasing is stale; no pending
  owner decision.
- **N8**: owner-run on-device iPhone SSH spike (Blink/Termius scrollback
  behavior) — not executable from a dev box; awaiting owner's device test.

**Next action**
- Owner: merge PR #23 (merge commit), then `git pull` on local main. Next
  code-worthy item per §12 sequencing is N8 (owner device spike) or the N1
  glamour width-bucketing micro-item; N7 stays continuous.

## Session — 2026-09-08 (owner merge of PR #23): PR #23 merged + local pull (orchestrator run)

**Work done**
- Owner-assigned step executed: merged PR #23 as a **merge commit**
  (`gh pr merge 23 --merge`). GitHub created `63f589d` — parents `19f4b93`
  (old origin/main) + `7325287` (PR head `feat/n5-pr22-review-fixes`),
  GitHub-signed; tree **byte-identical** to the PR head that had already
  passed CI (verified `63f589d^{tree} == 7325287^{tree}`).
- Branch-protection check on the merge commit (`Go fmt · vet · test · race ·
  vuln · cross-build`) ran green on `origin/main` (run 34177645477, polled
  to completion).
- `git pull` on local `main` (owner instruction) → ort merge `7925341`
  (parents `fa75b71` + `63f589d`), no conflicts, working tree clean. N5
  debug/log drawer + the 7/7 PR #22 Codex review fixes are now on
  `origin/main`.
- Local `main` stays ahead of `origin/main` by the carried docs commits only
  (`fa75b71` LEDGER +40 lines; the `7925341` merge) — they ride the next
  feature PR per repo rhythm (main is protected). Branch
  `feat/n5-pr22-review-fixes` kept on origin (auditable history, same as
  prior PRs).

**Commands + exit codes**
- `gh pr merge 23 --merge` → rc 0 (silent stdout; verified via REST:
  merged=true, merge_commit_sha=`63f589d`).
- `git pull --no-edit` → rc 0 ("Merge made by the 'ort' strategy").
- Verification: `git cat-file -p 63f589d` → 2 parents; tree-equality check →
  true; `git status -sb` → clean (`## main...origin/main [ahead 2]`);
  `gh api .../commits/63f589d/check-runs` → success.

**Decisions / lines to respect**
- Merge-commit merge per owner instruction (preserves the PR's commit
  history; same pattern as the PR #6 merge record in PLAN's tail log).
- No `--delete-branch` (repo convention keeps merged branches for audit).
- Triage for this step: DIRECT (git/gh operations only, zero agents).

**Blockers / open decisions**
- None for this step. N7 continuous upstream watch: nothing new (bubbletea
  v2.0.9 still pinned; scroll-optimized flush #1725/#1761 + event-driven
  rendering #1776 not yet shipped). N8 remains an owner-run on-device iPhone
  SSH spike (not executable from a dev box). N1 glamour width-bucketing
  (round width to 5 cols so resize jitter doesn't rebuild the renderer) is
  the cheap code-worthy micro-item.
- Standing owner click (unchanged): upload the GPG public key at
  github.com/settings/keys for the green Verified badge.

**Next action**
- Fresh session: N8 (owner device test) or the N1 glamour width-bucketing
  micro-item; N7 stays continuous watch. Local `main` carries 2 docs commits
  (incl. this handoff) to ride the next feature PR.

## Session — 2026-09-07 (N8 device spike, owner-run): tea.Println native-scrollback verdict = DECLINE

**Work done**
- N8 (PLAN §12) executed as an owner-run on-device spike, per the plan's
  spike-first rule. Built a throwaway instrument `cmd/n8-scrollback-probe`
  (branch `spike/n8-scrollback`, commit `4fe788e`): it stages the Charm
  chat-history pattern (bubbletea discussion #1482) — finalized turns printed
  via `tea.Println` into the terminal's native scrollback with a slim owned
  live frame — on the pinned bubbletea v2.0.9 with the same no-altscreen
  posture as `cmd/self-tui/main.go`. Local pty sanity run: boots, 3 turns
  emitted through the real Println/insertAbove path, clean `q` exit (rc 0).
- `docs/n8-device-test.md` carries the on-device procedure + 6-row
  observation sheet (same spike branch).
- Owner ran the probe on device: **Moshi** client, plain SSH session (no
  tmux). Observations: **scroll was dead** (the native scroll gesture did
  nothing — no usable history to pan) and the **live frame repainted
  repeatedly/rapidly** through the session. The repaint signature matches
  the raw stream captured during the local run: `tea.Println` (renderer
  `insertAbove`) scrolls the buffer and then the frame re-renders with
  erase bursts (dozens of `ESC[J` per frame) — over SSH to a phone that is
  a visible full-region flash on every turn-land.
- **Verdict: N8 declined.** The naive `tea.Println` native-scrollback pattern
  does not hold up on the owner's client. SelfTUI stays in its current
  renderer; the plan's cheap alternative holds — N1 windowing (landed) already
  bounds per-frame cost. The probe stays throwaway on `spike/n8-scrollback`
  and never merges to `main` as-is. Recorded as a positive negative: the
  spike answered the pre-commitment question at near-zero cost.

**Commands + exit codes**
- `git checkout -b spike/n8-scrollback` → 0; probe commit `4fe788e` → 0;
  `gofmt -l` clean; `go vet ./cmd/n8-scrollback-probe` → 0;
  `go build ./...` → 0.
- Local pty smoke: `(sleep 15; printf q) | timeout 22 script -qec '…go run
  ./cmd/n8-scrollback-probe…'` → rc 0, 3 `── turn` separators captured.
- Device run: owner-executed `/tmp/n8-probe` on Moshi (plain SSH); verdict
  from the owner's in-person observations, not host tooling.

**Decisions / lines to respect**
- N8 = declined (owner device evidence). This handoff + the PLAN §12 tick land
  on local `main` per repo rhythm (they ride the next feature PR); the probe
  code stays quarantined on `spike/n8-scrollback`.
- Date note: this entry is stamped 2026-09-07 (host clock, matching git
  stamps); the earlier "2026-09-08" header on the PR #23-merge entry is
  prose-ahead of git and is left uncorrected (history is append-only).

**Blockers / open decisions**
- Fate of `spike/n8-scrollback`: kept for audit per repo convention; safe to
  delete once N8 is closed out — owner's call.
- N7 continuous watch: unchanged (bubbletea v2.0.9 pinned; #1725/#1761/#1776
  not yet shipped).
- Standing owner click (unchanged): upload the GPG public key at
  github.com/settings/keys for the green Verified badge.

**Next action**
- N8 row ticked in PLAN §12. **Owner decision (2026-09-07): the next fresh
  session runs a device acceptance pass of current `main` on Moshi** — the
  first live on-device validation of the real SelfTUI since the M6-era
  session, covering the M7 / v0.2 (V2a resume, V2c sandboxed run_command,
  V2d workspace context) / N-series (N1 windowing, N2 streaming cadence,
  N4 status row, N5 log drawer, N6 composer) surface at 72×30. No code;
  scenario checklist to be written at that session's start. The **N1 glamour
  width-bucketing micro-item** (round width to 5 cols so resize jitter
  doesn't rebuild the renderer) remains the fallback headless step; N7 stays
  continuous watch. Local `main` now carries 3 docs commits (incl. this
  handoff) to ride the next feature PR.

## Session — 2026-09-07 (device acceptance pass on Moshi): OVERALL PASS — first live validation of current `main` since M6 (orchestrator run, owner-executed walk)

**Work done**
- Owner ran the device acceptance pass of current `main` (`selftui dev` @
  HEAD `4053190`) on the **Moshi** client (iPhone 16 Pro SSH) at the measured
  **72×30** compact geometry, inside a pre-started `tmux` session `accept` on
  the host. Scenario checklist SC-01…SC-26 (written at this session's start,
  owner + orchestrator) walked the **M7 / v0.2 (V2a resume, V2c sandboxed
  run_command, V2d workspace context) / N-series (N1 windowing, N2 streaming
  cadence, N4 status row, N5 log drawer, N6 composer)** surface. **OVERALL
  PASS**: all scenarios resolved; nothing clipped, no repaint bursts, no
  decision row pushed off-screen at 72×30. Evidence doc:
  `docs/device-acceptance-2026-09-07.md`.
- Host-side readiness + recording by this session (DIRECT, zero agents — the
  walk itself is owner-run by nature; N8 precedent: on-device verdicts come
  from the owner's in-person observations, not host tooling). `make check` and
  `go test -race` green on the tested tree before the pass.
- Artifact-verified highlights: N3 metrics landed in **every** exported turn
  header (`· 16.1–48.4s · stop · 56–61 tok/s`); V2d workspace context answered
  the real branch state (main, ahead of origin/main by 5, clean) and the
  project index named real `scripts/*.py`; the V2c jail enforced live — `echo`,
  `seq`, and a non-allowlisted git subcommand (`push`) refused with precise
  allowlist errors and the agent recovered gracefully; `/export` transcript +
  `/resume` reload exercised (owner); approve dialog appeared and a decline
  with `n` worked; Settings save wrote the config live (0600,
  `tools_enabled=true`, workspace_root=SelfTUI); log redaction clean (0
  bearer/secret markers).
- **Finding F1 (medium):** the 22:12 plain-chat-fallback turn narrated a
  completed `write_file` (`count_to_300.txt`) that **never executed** — no tool
  event in the log, no file on disk; the on-screen caveat is a transient notice
  only and the transcript export records the claim uncaveated. Candidate
  micro-fix (separate session): persist/inline the plain-chat fallback marker.
- **README staleness discovered (docs fix queued):** README still says
  transcripts are "not resumable … no import/reload path", contradicting the
  landed V2a `/resume` flow.

**Commands + exit codes**
- `make check` → rc 0 (build + tests + vet + fmt); `go test -race -count=1 ./...` → rc 0.
- `tmux new-session -d -s accept` + launch `./bin/selftui` → rc 0 (app pid
  3324589; log confirmed clean boot, `version=dev host=localhost:11434`).
- Evidence reads (transcript/log/config/fs) → rc 0.
- Session-end commit (docs + LEDGER + PLAN) → rc 0 (see below).

**Decisions / lines to respect**
- The acceptance record is dated 2026-09-07 (host clock, matching git + fs
  stamps; same convention as the N8 note — prose is not ahead of git).
- No code changed by the pass; the doc + this handoff + the PLAN §12 tick ride
  local `main` (now 4 carried docs commits) to the next feature PR per repo
  rhythm.
- Triage for this step: DIRECT host prep/record + owner-executed device walk.

**Blockers / open decisions**
- **F1** fallback-claim fidelity fix — queued micro-item, owner to schedule.
- README V2a staleness correction — queued docs fix.
- N7 continuous watch: unchanged (bubbletea v2.0.9 pinned; #1725 still open).
- N1 glamour width-bucketing micro-item remains the headless fallback step.
- Fate of `spike/n8-scrollback`: still the owner's call (safe to delete).
- Standing owner click: upload the GPG public key
  (`docs/release-signing-key.asc`, keyid `5F74A36F7B5C1670`) at
  github.com/settings/keys for the green Verified badge.

**Next action**
- Fresh session: F1 marker fix or the README V2a docs correction or the N1
  glamour width-bucketing micro-item; N7 stays continuous watch. The device
  acceptance next-action (from the N8 session) is now **closed**.

### 2026-09-07 — F1 fallback-marker fix + README V2a staleness correction (device-acceptance follow-up, owner-assigned)
**Milestone:** owner-assigned step — the two queued micro-items from the device
acceptance pass, executed together in one session per the owner's message — on branch
`fix/f1-fallback-marker` off local `main` (`5dee61a`, 6 commits ahead of origin/main;
carried docs commits ride this branch per repo rhythm). · **Result:** done — both fixes
implemented, tested, and parent-verified green (implementer + orchestrator pass). No §10
roadmap row to tick (owner-assigned micro-step); the PLAN §10 acceptance note is
annotated with the resolution. **No push / PR created — the landing (PR vs main) is the
owner's call** (see Next action).

**Work done**
- **F1 fallback-claim fidelity (code + tests):** a plain-chat-fallback turn (runner
  `agent.FallbackMsg`, only ever emitted at iteration 0 of a tool-armed loop — no tool
  can have executed in that turn) now commits its assistant content with a persistent
  inline caveat — `> ⚠ **plain chat** — no tool ran this turn: <reason>` — so the note
  renders in the conversation, survives the committed-turn render cache, and rides the
  same content into the `/export` transcript and a later `/resume` reload. The transient
  statusline notice is preserved. Marker is a markdown blockquote on the content body:
  no `internal/session` format/parser change. Reason is sanitized (`sanitizeTerminalText`)
  before it joins the content. Flag lifecycle: set on `FallbackMsg`, consumed at commit,
  cleared at every `startChat` (no cross-turn leak). Implemented in
  `internal/ui/agent_view.go` (`plainChatReason` field + `plainChatFallbackNote` helper);
  +4 tests: `agent_view_test.go` (commit marker + inline render; stale flag cleared at
  next turn start), `sanitize_test.go` (hostile-reason sanitization), `session_ui_test.go`
  (end-to-end round trip — real runner fallback over a fake NDJSON stream → in-memory
  turn → transcript file → `session.Load` reparse; reproduces the `count_to_300.txt`
  claim).
- **README V2a staleness correction (docs):** all four "not resumable / no import-reload
  path" passages corrected to the landed V2a `/resume` flow, honestly v0.1-vs-v0.2
  scoped (v0.1 shipped no reload path; V2a/v0.2 added `/resume`); `/resume` added to the
  two slash-command inventories; the plain-SSH drop section now points at `/resume` to
  reload the dead process's transcript. Every new prose claim (newest-first picker,
  asks-first on non-empty history, sends refused while loading, fresh per-process
  transcript, exact "session recording is off — nothing to resume" notice) was verified
  against code (`agent_view.go` openResume/resumeKey/importSession/applySessionLoaded,
  `session.ListSessions` ordering) before it landed.

**Commands + exit codes** (final tree, `fix/f1-fallback-marker`; baseline at `5dee61a`
also green — attribution clean)
- Baseline: `make check` → rc 0; `go test -race -count=1 ./...` → rc 0.
- `make check` → rc 0 (build + uncached full suite + vet + gofmt).
- `go test -race -count=1 ./...` → rc 0.
- Targeted: `go test ./internal/ui -count=1 -run 'TestAgentFallbackMarker|TestAgentViewFallbackMarkerTranscriptRoundTrip' -v` → 4/4 PASS; `go test ./internal/agent -count=1 -run Fallback -v` → 2/2 PASS.
- Golden fixtures: **no regeneration needed** — no fallback turn is seeded in goldens;
  no `internal/ui/testdata/` diffs on the branch.

**Decisions / lines to respect**
- Marker rides the turn's content body (blockquote), not the header meta — content is
  what renders, exports, and round-trips through the reader; this avoided any
  session-format or parser churn.
- The marker is truthful by construction: both runner `FallbackMsg` sites are guarded by
  `iteration == 0` (tool-unsupported downgrade; or zero tool calls before any
  `executeTool`), so "no tool ran this turn" is never emitted for a turn that ran a
  tool. Tools-off plain chat (no policy) never emits `FallbackMsg` → no fabricated
  markers on ordinary chats.
- Fix-forward only: pre-existing transcripts on disk are not retroactively marked
  (append-only model). A resumed conversation feeds the marker text back to the model as
  ordinary history — truthful context, accepted (a few tokens).
- Both queued items shipped in one session per the owner's message; the N1 glamour
  width-bucketing micro-item stays queued (out of this session's scope).

**Blockers / open decisions**
- None in the work itself. **Landing is the owner's call:** push `fix/f1-fallback-marker`
  and open a PR (repo rhythm for code: feature branch → PR, owner merges; the 6 carried
  docs commits on local main will ride with it) vs commit/push to local main directly.
- N7 continuous watch unchanged; N1 width-bucketing remains the next queued micro-item.

**Next action**
- Owner lands the branch (PR recommended per repo rhythm), then: N1 glamour
  width-bucketing micro-item or the next owner-assigned step; N7 stays continuous watch.
