# V2e — Multi-file edit batches + undo/redo (design gate, 2026-09-08)

Status: **DESIGN GATE — not implemented.** This doc is the V2b-style gate for the
V2d-deferred agent-breadth items (multi-file edits, mutation undo/redo). It records the
design, the evidence it rests on, the constraints it inherits, and the open owner
decisions. Implementation is a separate session, only on an owner GO.

Evidence inputs: scout recon of the current mutation surface
(`internal/agent` + `internal/ui` + `internal/session`, file:line refs) and a web
research brief on 2025–2026 agent undo/multi-file prior art (aider, Claude Code,
opencode, Codex CLI; `/tmp/selftui-v2e-research.md`, run `9b469831`). Load-bearing
seams were verified by direct read, then **grep-re-verified against HEAD after the
reality-check pass** (§8) corrected three drifted refs and one overstated invariant.

---

## 1. Goal and non-goals

**Goal.** Let the agent propose one coherent change across several files and let the
owner review it as one unit before anything is written; give the owner undo/redo of
confirmed agent mutations.

**Non-goals for this cut (v1):**
- No `delete_file`/`rename` primitives (new closed-schema surface; separate gate if wanted).
- No binary-file edits (existing regular-file checks stay; NUL-sniff refusal on edit ops).
- No journaling of `run_command` effects — sandboxed commands can touch anything the
  allowlist permits; their side effects are out of undo scope and the UI states this
  (same documented gap class as Claude Code's "bash mutations are not tracked").
- No git auto-commit / no writes to the user's git history (see §4; future owner opt-in).
- No per-file accept/reject inside a batch review (all-or-nothing v1; precedent §3).

## 2. Current surface this builds on (verified seams)

| Seam | Fact | Ref |
|---|---|---|
| Closed tool schema | six tools (`tools.go:44-79`); a new tool must enter `AgentTools()` + the `executeTool` switch | tools.go:30-80, runner.go:497-628 |
| Per-call confirm | `ToolConfirmMsg{Name,Input,Workspace,Timeout,reply}` (struct runner.go:86-95), built in `confirm()` (runner.go:630-670); 30 s default / 60 s cap (runner.go:20-22); expiry ends the turn | runner.go:86-95, 630-670 |
| Atomic write | temp + chmod + fsync + rename, per file; parent dir NOT fsynced after rename (post-crash rename durability best-effort); no transaction across files | mutation.go:100-136 |
| edit_file determinism | `old` must match exactly once, against the tree **at apply time** | mutation.go:61 |
| Sensitive paths | `AuthorizePath` refuses components `.ssh/.gnupg/.aws/.azure/.kube` + composite `.config/gcloud`, basenames `.env*` (except `.env.example`) and exact `credentials`/`credentials.json` (exact-map lookup, no prefix matching) before any dialog | toolpolicy.go:33-84; pre-dialog order runner.go:553-556, 581-584 |
| **`.git` write refusal — ABSENT** | no `.git` refusal exists in the mutation path today: read-side prunes only (tools.go:197 grep, workspace.go:107/256 walks); `secureWritePath`/`securePath` enforce containment only — a model-requested `write_file` into `.git/hooks/pre-commit` passes both gates and reaches the approval dialog (dialog-only protection). **V2e adds a lexical `.git` component refusal for all mutation tools at proposal + apply (§4.2 step 4), closing this gap for the existing single-file tools too.** | tools.go:197, workspace.go:107/256, mutation.go:67-102 |
| H-03 budgets | 1 MiB per tool-call arg, 64 calls/run, 12 iterations default; reject-in-full before any dialog | runner.go:16, 32-33, 373-389 |
| Wire hygiene | context budgeting compacts oversized tool args/results — batch payloads must stay lean | context.go |
| Non-git workspaces | supported and silent (V2d context omitted outside a repo); `.git` never writable | workspace.go |
| UI confirm modal | `v.confirmation` blocks input; y/enter approve · n/esc decline; height-capped overlays are the established pattern | agent_view.go:120, agent_composer.go:163-176, agent_paint.go |

**The gap.** Today every approved `write_file`/`edit_file` commits immediately; a
crash or cancel between two calls of a multi-file turn leaves a partial change-set,
and nothing records pre-images, so there is nothing to undo.

## 3. Prior art (research brief, 2025–2026)

- **aider** — every edit format is inherently multi-file (one response, many fenced
  blocks); `/undo` is strictly single-level: selective `git checkout HEAD~1 -- <paths>`
  + `git reset --soft`, guarded by refuse-checks (HEAD not an aider commit from this
  session, merge commit, pushed, file now dirty, file absent in parent). Dirty
  pre-existing work is committed separately first so human state is never bundled.
- **Claude Code** — checkpoints captured **before each prompt**, survive `/resume`,
  bounded (100 most recent per session, ~30-day sweep); documented gaps: bash
  mutations, subagent edits, symlinked/hardlinked paths skipped; "not a replacement
  for version control".
- **opencode** — approvals are allow/ask/deny per tool+pattern with once/always/reject;
  session snapshots live in a **separate internal git object DB** (never the user's
  history), capture tracked + non-ignored untracked files, skip >2 MiB files; revert
  is staged-and-reversible (`unrevert`). Bug #38672: in non-git directories `/undo`
  reports success but does not revert — a git-dependent substrate silently fails our
  requirement.
- **Codex CLI** — no native rollback; untracked files created by the agent are
  unrecoverable. The cautionary pole.
- **No shipped tool implements redo**, and no surveyed tool documents multi-file
  batch review at 72×30; narrow-terminal diff conventions (one-column unified diff,
  per-file one-row summaries, collapsed unchanged context) transfer as inference.

Synthesis: SelfTUI's substrate is a **touched-files pre-image snapshot store** — the
proven Claude-Code-family pattern (capture-before-mutate), scoped per change-set
instead of per prompt — plus aider-style refuse-guards and opencode-style bounds. It
works outside git, never touches the user's history, and avoids the unproven
reverse-diff-journal family and the shadow-git store's lifecycle complexity.

## 4. Design

### 4.1 The batch tool (one new closed-schema entry)

`write_files` — `{ops: [op…], note?: string}` where each op is
`{path: string, kind: "create"|"overwrite"|"edit", content?: string, old?: string, new?: string}`:

- `create` → existing `WriteFile` semantics with `overwrite=false`;
  `overwrite` → `WriteFile` with `overwrite=true`;
  `edit` → existing `EditFile` semantics (exact-one-match replace).
- Caps (hard proposal ceiling, checked before any dialog): ops ≤ **16**; per-op
  content/old/new ≤ 256 KiB; whole-call args ≤ the existing 1 MiB H-03 cap. A batch
  over any cap is rejected **in full** before confirmation (existing H-03 pattern,
  runner.go:250-266).
- Model-facing result: compact per-file status lines only (`created X`, `edited Y`,
  or one failure line) — never the full diff — so the transcript stays inside the
  context budget.

### 4.2 Propose → review → apply (all-or-nothing)

1. **Propose.** Model emits one `write_files` call. The raw payload stays in the
   runner; the UI receives a new `BatchReviewMsg` (summary rows + rendered per-file
   unified diffs), not a generic confirm.
2. **Review (new UI stage, replaces the raw-args modal for this tool).** Height-capped
   overlay (established `fitContent` pattern): one-column unified diff, one summary row
   per file (`M internal/agent/runner.go  +18 −6`), collapsed unchanged context as
   `··· N unchanged lines ···`, `… (N more lines)` truncation marker as in M5. Keys:
   `y`/`enter` approve all, `n`/`esc` decline. Sensitive paths never reach this stage
   (`AuthorizePath` runs per op at validation, before the dialog — current behavior).
3. **Review window.** The 30/60 s mutation window is a mismatch for reading a diff;
   the batch stage gets its own window: **120 s default / 300 s hard cap**
   (`confirmTimeout` seam reused with a batch-specific bound; owner decision §6.2).
   Nothing is applied pre-approval, so expiry stays safe: decline semantics, turn ends.
4. **Validate-all at apply time.** On approval, every op re-runs `AuthorizePath` + a
   new lexical **`.git` component refusal** + containment + (`edit`) exact-one-match
   against the **current** tree — TOCTOU between proposal and approval fails the whole
   batch with a per-op reason. The `.git` refusal joins `AuthorizePath` itself so the
   single-file `write_file`/`edit_file` tools inherit the same fix (see the §2 gap
   row). Then the M-06 `ctx.Err()` gate.
5. **Journal write-ahead.** For each op, capture the pre-image (current bytes, mode,
   existed-flag; refusal if a pre-existing file exceeds the per-file journal cap, §6.4)
   and fsync the journal entry **before** the first `atomicWrite` — capture-before-mutate
   ordering (Claude Code's documented best practice). Durability caveat: `atomicWrite`
   fsyncs the temp file but not the parent directory after rename, so post-crash rename
   durability is best-effort — acceptable for session-scoped crash-recovery artifacts
   (§6.7), and stated as such rather than as an absolute crash-safety guarantee.
6. **Apply sequentially.** Per-op `atomicWrite` (existing primitive, unchanged).
7. **Failure ⇒ compensating rollback.** Any mid-apply failure (validation race, disk
   error, cancellation) rolls back already-applied files from the journal pre-images
   (all-or-nothing; a created file is removed). The turn continues with a per-op error
   result; nothing is half-applied. Cancellation mid-apply = rollback + journal entry
   discarded (owner decision §6.5). **If the rollback itself fails** (e.g. ENOSPC while
   restoring a pre-image): stop immediately, leave the tree and the surviving journal
   entry untouched, and fail loudly naming the affected files — the entry stays so a
   later `/undo` can retry (owner decision §6.8); the journal is never deleted on a
   failed rollback.

### 4.3 Undo / redo

- **Scope:** every confirmed mutation **and** every applied batch journals (uniform —
  single `write_file`/`edit_file` calls are just 1-op change-sets; owner decision §6.3).
- **Mechanics:** journal entries live under `$XDG_STATE_HOME/selftui/undo/` (0600):
  per-file pre-image blob + `{path, mode, existed}` metadata + post-apply content hash
  per file. `undo` pops the newest entry; **refuse-guards (aider-style) run first**:
  each file's current hash must equal the entry's post-hash — any mismatch (external
  edit since) refuses the whole undo with a visible notice naming the file. Revert =
  restore pre-image via `atomicWrite` (or delete a `created` file). `redo` is the
  symmetric operation (post-image captured at undo time); no shipped agent tool has
  redo — flagged as beyond-precedent but near-free here (owner decision §6.6).
- **In-process, never sandboxed:** undo shells nothing out (sandboxed git is
  read-only by design); it is plain Go over the journal, host-side.
- **UX:** `/undo` and `/redo` slash commands + palette entries (discoverable path per
  M7; no new leader chords), each guarded by the standard y/esc confirm; result as a
  status-bar notice.
- **Bounds & lifecycle:** ≤ **25** entries, ≤ **32 MiB** total, per-file pre-image cap
  **8 MiB** (larger ⇒ batch refused at proposal, opencode's >2 MiB exclusion made
  strict); LRU eviction; entries are **session-scoped** (stacks die with the process;
  disk blobs are crash-recovery artifacts, GC'd at clean exit — owner decision §6.7).
- **Restarts/crashes:** a crash mid-apply leaves fsynced pre-images; on next start the
  stale entry is detected (files at pre-image state) and reported, not auto-reverted.

### 4.4 What the model sees

The system prompt tool description states: batch is all-or-nothing, exact-one-match is
validated at apply time, refuses pre-images over the cap, and `run_command` effects are
not undoable. qwen3-class models handled existing tools fine (M3a/OD3); the batch shape
is a strict superset of `write_file`/`edit_file` semantics, so plain-chat fallback rules
are unchanged.

## 5. Test strategy (gate exit criteria for the implementation session)

- **Unit:** batch validation (caps, per-op authorize/contain), TOCTOU reject-at-apply,
  mid-apply disk-failure rollback leaves the tree exactly at pre-state, journal
  write-ahead ordering (fsync before first write — fault-injected), undo/redo
  round-trips, refuse-guard on external change, bounds/eviction, crash-recovery
  detection.
- **Wire-shape:** closed-schema addition only; result lines compact; budget/compaction
  unaffected.
- **UI:** review overlay at 72×30 and 120×40 (guard: frame within terminal, footer
  visible); decline/timeout paths; `/undo`-`/redo` confirms; new golden fixtures only
  for the new overlay (existing frames byte-identical).
- **Regression:** all existing suites green — `make check`, `go test -race ./...`,
  goldens unchanged.

## 6. Owner decisions (the gate click)

> **Resolved 2026-09-08 (owner click, this session):** #1 **GO** — implementation next
> session; #2 **120 s default / 300 s cap**; #3 **all confirmed mutations** journal;
> #4 defaults (25 entries / 32 MiB / 8 MiB per file); #5 **full rollback** on mid-apply
> cancellation; #6 **include `/redo`**; #7 **session-scoped** journal; #8 **stop +
> retain entry** on rollback failure.

1. **GO / NO-GO** on the design as a whole.
2. **Batch review window:** 120 s default / 300 s cap (rec) vs keep 30/60 s.
3. **Undo scope:** all confirmed mutations (rec, uniform) vs batches only.
4. **Journal bounds:** 25 entries / 32 MiB total / 8 MiB per-file pre-image (rec).
5. **Mid-apply cancellation:** full rollback (rec) vs finish-in-flight-file-then-stop.
6. **Redo:** include (rec, near-free) vs undo-only (SOTA parity).
7. **Journal lifecycle:** session-scoped (rec) vs cross-session retention.
8. **Rollback-failure semantics:** stop + loud per-file error + journal entry retained
   for a later `/undo` retry (rec) vs any alternative the owner prefers.

## 7. Residual risks (carried to implementation)

- Model reliability emitting valid batch JSON (superset of known-good single-file
  shapes; DisallowUnknownFields decoding gives clean per-op errors; 12-iteration loop
  bounds retry cost).
- Review overlay density at 72×30 for a 16-op batch (mitigated by collapsed context +
  per-file summary rows + `… (N more lines)`; worst case is bounded by the 1 MiB arg cap).
- Undo after an intervening `run_command` that touched the same files: the hash
  refuse-guard makes this a visible refusal, not silent clobber (the documented-gap
  pattern Claude Code also uses for bash).
- Residual TOCTOU between approval and pre-image capture: an external writer's change
  in that window becomes the captured pre-image (consistent with today's single-tool
  posture, where `write_file` clobbers post-approval); the microsecond capture-to-rename
  window is unclosable without file locking — refuse-guards cover divergence at
  undo-time, not this window.
- Rollback-failure (§6.8): all-or-nothing is unachievable if the compensating restore
  itself fails; the stop-and-retain semantics bound the damage to the files already
  written, with the journal entry as the recovery path.

## 8. Gate record

- **Inputs:** scout recon (`/tmp/selftui-v2e-scout-context.md`, run `8092459a`,
  completed; its claim ".git is never a writable target" was WRONG — caught by the
  review pass below); researcher brief (`/tmp/selftui-v2e-research.md`, run
  `9b469831`, completed after one same-protocol retry — the first attempt ran as a
  foreground child without web-tool extensions).
- **Independent review:** reality-checker run `2d15ad0e` — verdict **NEEDS WORK**
  (confidence high): 3 wrong §2 refs, the unsubstantiated ".git never writable"
  invariant, unpinned rollback-failure semantics, "crash-safe" overclaim,
  `credentials*` phrasing, residual-TOCTOU note. All six findings parent-verified
  against HEAD (greps recorded in the session LEDGER) and applied as the §2/§4/§6/§7
  fixes above; the re-check trigger (refreshed §2 grep-verified, `.git` gate added,
  rollback-failure decision recorded) is closed parent-side.
- **Owner decision: GO (2026-09-08)** — the §6 click resolved all eight decisions
> (above); implementation is the next session, owner-assigned per the one-step rule,
> with §5 as its exit gate.
