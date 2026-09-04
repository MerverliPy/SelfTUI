# Council Audit Memo — SelfTUI Plan

**Date:** 2026-09-03 · **Audited artifact:** the pre-M0 plan document (then at a
local path under the owner's home dir; since renamed/moved — see LEDGER
2026-09-03) · **Status:** CONVERGED

> **Historical advisory record (2026-09-03).** This memo audited the
> *planning-era* roadmap and is retained verbatim as history. Product claims
> it repeats (full coding agent incl. commands; PC + iPhone framing) predate
> the v0.1 hardening — see the 2026-09-04 product contract in `PLAN.md`.

---

## Question and scope

Independently audit the **SelfTUI** plan (a self-hosted Ollama TUI; Go + Bubble Tea; manage
Ollama models, an
embedded full AI coding-agent, settings panel; PC + iPhone-via-SSH responsive) and
identify what can be improved or addressed. Scope: plan quality — architecture
soundness, scope/phasing realism, risk exposure. **Not** implementation. No project
files were modified by any advisor.

## Recommendation (converged)

**Proceed — do not halt the whole project — but re-cut the roadmap into ship gates
with a hard, evidence-based spike gate *before* any agent (mutation) work.** The halt
posture was rejected for a personal/local tool: blanket demand-discovery is not
justified. In its place, all three advisors converged on: build model management +
plain chat on a revised roadmap, but **do not begin agent implementation until an
`M0a` technical/security/device spike passes** (see gates below). Model management and
plain chat may proceed regardless; the *agent* scope is what is gated.

**Single condition that would force a full halt/drop of agent scope:** failure of the
designated release target model to complete repeatable end-to-end tool-call coding
tasks on the intended Ollama configuration/version. Models management and chat would
still ship.

## Accepted feedback (with reasons)

| # | Finding | Disposition | Action |
|---|---------|-------------|--------|
| A | **`run_command` controls are guardrails, not a sandbox.** A cwd + timeout + denylist + confirmation don't stop a command reading creds, hitting the network, writing absolute paths, spawning children, or escaping via interpreters. | **accepted (all 3)** | v1: disable by default, or an **argv allowlist / no shell or interpreter** runner with scrubbed env, resource + output limits, process-group kill, cancellation, and per-call confirmation. General shell = labeled dangerous opt-in, only behind a real OS/container sandbox. Cwd confinement + denylist are insufficient. |
| B | **Agent tool-calling feasibility is the linchpin** (streamed valid `tool_calls`, multi-tool-turn assembly, 4K context, ~12 iterations). | **accepted (all 3)** | Early **spike** (`M0a`) on named supported models: streamed arg assembly, malformed/parallel calls, tool-result correlation, retries, cancellation, context exhaustion, iteration exhaustion. M3 must not start until one release model passes defined coding scenarios. |
| C | **Safety gates deploy too late** — M3 grants mutation/exec while confirmation + hardening sit in M6. | **accepted (all 3)** | Move each tool's safety controls (jail, confirm, timeout, cancellation, tests) *into the milestone that exposes it*. Keep hardening continuous; reserve the last milestone for release acceptance, docs + smoke tests. |
| D | **Responsive/iPhone work and the 88-col assumption deferred/unevidenced.** | **accepted (all 3)** | Measure real `WindowSizeMsg` width/height, resize, key delivery, color, scrolling, reconnect from the *actual supported SSH client(s)*; derive ranges from measurement (don't treat 88 as fact). Pull a minimal responsive shell + breakpoint system into **M0** so feature views ship at both narrow and wide geometries — prevent a late M5 retrofit. |
| E | **M3 is oversized** (runner + 6 tools + jail + command streaming + context budgeting + UI streaming = one solo-slice). | **accepted (architect, operator)** | Re-cut: **M3a read-only agent** (capability check, read/list/grep, bounded loop) → **M3b mutation** (write/edit/command) only with jail + confirm + timeout + cancel + tests. Ship an **alpha after M2** (reliable model management + plain chat). |
| F | **M1 oversized** (4 API ops + streamed pull + detail + confirm + spinner + progress bar). | **accepted (operator)** | Split M1: list/show first; then destructive delete + streamed pull. |
| G | **"Pinned dependencies" is unverified** (section 9 lists no versions) and the **single-binary story conflicts with `rg`-backed grep**; logs-on-stderr corrupt the alt-screen over SSH. | **accepted (all 3)** | Pin **Charm v1 or v2 as one compatible set** (after a compile spike); resolve `rg` prerequisite vs pure-Go grep; **log to a file**, not stderr. |
| H | **Concurrency/streaming sketch is incomplete** ("waitForActivity" ad-hoc, no nested models, single god `Update`). | **accepted (architect)** | Resubscribed activity channel **or** `tea.Program.Send`; token coalescing + context cancel; per-view nested `tea.Models`; ordered events; cancellation/teardown/race tests. |

## Rejected / refined feedback (with reasons)

- **Halt-and-spike for the whole product (skeptic Pass 1)** — *rejected / refined.* The
  other advisors rightly noted viability does not require freezing implementation; for
  a personal tool, blanket demand validation is over-cautious. Assistant posture
  (all 3, Pass 2): reversible M0–M2 work proceeds; the **hard gate applies to the agent
  scope**, not model management/chat.

## Owner decisions (not settable by advisor evidence — owner must choose)

1. **Charm version set** — v1 vs v2 as one pinned, compatible set (after compile spike).
2. **Concurrency + model decomposition** — activity channel vs `tea.Program.Send`;
   nested per-view `tea.Models` vs god `Update`. Either design must keep ordered events +
   nonblocking `Update`, with cancel/coalesce/teardown/race tests.
3. **No-tool model behavior** — hard-disable Agent tools vs *explicit* plain-chat
   fallback. **Never silent** (silent misrepresents agent capability).
4. **Grep** — declared `rg` runtime prerequisite vs pure-Go implementation (weighs
   effort vs the single-binary story).
5. **Ollama job serialization** — allow concurrent pull+agent (GPU contention) or
   serialize/no-pull-during-agent.
6. **Go-forward gate criteria** — the owner defines the `M0a` gate: one validated target
   workflow, model/tool-loop compatibility, command containment, observed mobile
   usability.

## Evidence and run IDs

Pass 1 (independent reports): skeptic `b363661f…245`, architect `47197693…07d`, operator `ef39bbc0…46f`.
Pass 2 (cross-exam): skeptic `ac754678…9cc`, architect `650472f4…1bc`, operator `68c435c2…858`.

## Confidence and what would change the decision

**Confidence: high** on the direction (reshape-and-go with a hard agent gate; safety
inline; responsive leads; M-split). This rests on plan-level reasoning, not a compile
or a live probe, so runtime details (exact Charm versions, real Ollama tool behavior,
measured SSH geometry) remain open — which is precisely why the `M0a` spike is the
gate. What would change the decision: the target model failing repeatable tool-call
tasks, or measured mobile geometry/ergonomics proving unusable for agent approval.

## Roster, passes, fallbacks, context modes

- Advisors: `council-architect` (grok-4.6), `council-operator` (gpt-5.6-sol),
  `council-skeptic` (gpt-5.6-sol). All fresh context, read-only.
- Passes: 2 (independent, then one cross-exam). Converged at pass cap; no pass 3.
- **Run failures/fallbacks:** architect's first launch failed on an invalid pinned
  model (`grok-4.5` not in registry) → relaunched on `opencode-go/grok-4.6`. The
  operator's profile model (`deepseek/deepseek-v4-pro`) twice returned empty structured
  output → relaunched on `openai-codex/gpt-5.6-sol`. All three advisors' Pass 2 resumed
  their own prior sessions. These were tooling recoveries, not scope changes.