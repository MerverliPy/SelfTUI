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
