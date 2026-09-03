# Project Ledger — SelfTUI

The persistent **work/decision log** for this project. `LEDGER.md` is the **past-facing**
record (what happened, why, what you hit, what happens next) that complements:

- **`PLAN.md`** — future-facing: architecture, re-cut ship-gated roadmap (`§10`), risks & owner decisions (`§11`).
- **`COUNCIL-MEMO.md`** — the advisory audit verdict that re-cut the roadmap.
- **`LEDGER.md`** (this file) — a chronological, append-only log for session efficiency and continuity.

**Rules:** consult at session start → append at session end. Never overwrite history;
always append. Keep the append template filled (work done → commands + exit codes →
decisions/blockers → next action).

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