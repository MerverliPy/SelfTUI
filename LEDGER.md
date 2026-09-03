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
> in a new `pi` session with cwd `/home/calvin/SelfTUI`.

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
