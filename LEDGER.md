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
- ✅ **Repo setup done** — dir renamed `/home/calvin/TUI` → `/home/calvin/SelfTUI`; `git init` run;
  dotfiles ignores it automatically (its `.gitignore` uses `*` default-ignore, so the `/TUI`
  entry was unnecessary).
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