# SelfTUI — Project Instructions & Session Rule

This file governs all work in this repo. It is read at the start of every session and
takes precedence over the global agent rules for anything it specifies.

## Binding session rule — one fresh session per step

**"Step" is defined as:** one milestone from `PLAN.md` §10 (M0, M0a, M1a, M1b, M2,
M3a, M3b, M4, M5, M6) **or** one clearly-scoped task the owner assigns in a message.

1. **One step per session (binding).** Never start a new step in the same session that
   completed a step. When a step is done (changes committed), the session STOPS. The
   next step is begun only by opening a **new `pi` session with cwd at this
   repository's root**.
2. **Session-start ritual (read-only, in this order):**
   `AGENTS.md` → `PLAN.md` §10 (roadmap) + §11 (risks/owner decisions) → **the tail of
   `LEDGER.md`** (latest handoff). Then choose **exactly one** step. Do not re-plan the
   whole build or chain steps.
3. **Session-end ritual:** complete the chosen step → append a handoff entry to
   `LEDGER.md` (work done · commands+exit codes · decisions · blockers · next action) →
   tick the milestone's exit condition in `PLAN.md` §10 → commit → **stop**. Do not open
   the next milestone in this session.
4. **Evidence over assertion.** Run the canonical checks (build/test/lint once they
   exist — see `make` targets as they are added) and report green only from real output.
5. **Consult & append, never rewrite.** `LEDGER.md` is the past/handoff source of truth;
   `PLAN.md` is the future. Append new entries; do not rewrite history.
6. **Scope discipline.** No unplanned scope, no speculative scaffolding. Match existing
   patterns. If a step is ambiguous or blocking, stop and ask rather than guessing.
7. **Safety (carried from the council audit).** Safety controls ship inline with the tool
   that exposes them; no mutation/`run_command` tool ships before its workspace jail,
   confirmation, timeout, cancellation, and tests exist.
8. **Model reality (spike-verified).** Default agent model is `qwen3:8b` (native
   `tool_calls` works). The agent tool layer must support dual dispatch — native
   `message.tool_calls` **and** tool-JSON parsed from `content` — and must handle qwen3's
   `thinking` phase. See `PLAN.md` §6.

## Where the truth lives
- `PLAN.md` — future: architecture, roadmap (§10), risks/owner decisions (§11).
- `LEDGER.md` — past: chronological handoff log per session.
- `COUNCIL-MEMO.md` — the advisory verdict that re-cut the roadmap.

## Agent skills

### Issue tracker

Issues live in this repo's GitHub Issues (github.com/MerverliPy/SelfTUI), used via the
`gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles use their default label strings: `needs-triage`,
`needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See
`docs/agents/triage-labels.md`.

### Domain docs

Single-context layout: one `CONTEXT.md` at the repo root plus `docs/adr/`. See
`docs/agents/domain.md`.