# SelfTUI v0.1.1 Audit Remediation — Pi Terminal Runbook

> **For agentic workers:** REQUIRED SUB-SKILL: use test-driven development for each task and execute only one task per fresh Pi session. Every task below is a complete paste-ready prompt.

**Goal:** Resolve every finding in `SelfTUI-External-Audit-2026-09-04.md` through small, reviewable patches and finish with a fully evidenced v0.1.1 release candidate.

**Architecture:** Preserve SelfTUI's single-process Bubble Tea ownership model, opt-in five-tool surface, Linux/WSL scope, and lack of command execution. Repair security boundaries at their source, add regression tests before implementation, and keep each finding in a separate commit unless a prompt explicitly says otherwise.

**Tech stack:** Go, Charm v2 (`bubbletea/v2`, `bubbles/v2`, `huh/v2`, `lipgloss/v2`, `glamour/v2`), Bash, Python standard library, Ollama HTTP/NDJSON APIs.

**Spec:** `SelfTUI-External-Audit-2026-09-04.md` plus the repository's `AGENTS.md`, `PLAN.md` §§10–12, `SECURITY.md`, and latest `LEDGER.md` entries.

## Global constraints

- Run every block from the SelfTUI repository root in a **fresh Pi session** and in the numbered order.
- Before editing, read `AGENTS.md`, the finding's cited source/tests, `PLAN.md` §§10–12, and the latest relevant `LEDGER.md` entries.
- At the start of every patch task, run `git status --short`. If it prints anything, stop and report the exact paths; never stash, reset, clean, or overwrite unrelated work.
- Work on branch `fix/v0.1.1-audit-remediation`. Task 00 creates it; every later task must confirm it is active.
- Use red-green TDD: add the narrow regression first, run it and confirm the expected pre-fix failure, implement the minimum fix, then rerun focused and broader gates.
- If a `[needs runtime verification]` finding does not reproduce, do not force a speculative patch. Append a `NOT_REPRODUCED` disposition with evidence to `LEDGER.md` and stop that task without a code commit.
- Preserve exact public errors unless the task explicitly defines a new stable error. Do not add shell execution, resumable sessions, native Windows/macOS support, or unrelated refactors.
- Do not run `make smoke` before Task 05. Do not run `VERSION=... make release-check` before Task 22; it deliberately recreates `dist/`.
- After a successful task, append the exact tests and result to `LEDGER.md`, run `git diff --check`, review `git diff --stat` and `git diff`, then commit only that task's allowed files.
- Never amend, force-push, move a tag, or publish a release from these tasks.

## Task/file map

| Task | Finding | Primary files |
|---:|---|---|
| 00 | Baseline | read-only checks, branch creation |
| 01 | H-01 | `cmd/self-tui/main.go`, config/startup tests |
| 02 | C-01 | `internal/agent/{runner,tools,toolpolicy}.go`, policy tests |
| 03 | H-02 | `internal/config/validate.go`, tools validation tests |
| 04 | H-03 | `internal/ollama/chat.go`, `internal/agent/runner.go`, tests |
| 05 | H-04 | `scripts/pull-delete-smoke.py`, script tests, README |
| 06 | H-05 | new UI sanitizer, Agent/Models rendering tests |
| 07 | H-06 | audit-pack creation/verification script |
| 08 | M-01 | agent confirmation and root modal routing |
| 09 | M-02 | context budgeting and tests |
| 10 | M-03 | Models/Agent request generations and tests |
| 11 | M-04 | asynchronous transcript recorder/lifecycle |
| 12 | M-05 | cell-/ANSI-aware wrapping and tests |
| 13 | M-06 | tool cancellation and context-aware channel sends |
| 14 | M-07 | delete busy overlay and geometry test |
| 15 | M-08 | redirect policy and HTTP tests |
| 16 | M-09 | single spinner command chain and scheduler test |
| 17 | M-10 | release reproducibility, toolchain, checksum paths |
| 18 | M-11 | private unique smoke captures |
| 19 | M-12 | SECURITY/README/CHANGELOG/PLAN alignment |
| 20 | L-01 | shared golden scenario construction |
| 21 | L-02 | complete clean target and timer artifact disposition |
| 22 | Final gate | full checks, audit-pack proof, release-candidate summary |

## Progress checklist

- [x] Task 00 — baseline and branch
- [x] Task 01 — H-01 default configuration
- [x] Task 02 — C-01 recursive grep policy
- [x] Task 03 — H-02 workspace canonicalization
- [x] Task 04 — H-03 tool byte/call budgets
- [x] Task 05 — H-04 smoke model safety
- [ ] Task 06 — H-05 terminal sanitization
- [ ] Task 07 — H-06 audit-package completeness
- [ ] Task 08 — M-01 approval/modal behavior
- [ ] Task 09 — M-02 context grouping
- [ ] Task 10 — M-03 async request generations
- [ ] Task 11 — M-04 transcript persistence
- [ ] Task 12 — M-05 terminal wrapping
- [ ] Task 13 — M-06 cancellation/backpressure
- [ ] Task 14 — M-07 delete overlay
- [ ] Task 15 — M-08 redirect policy
- [ ] Task 16 — M-09 spinner lifecycle
- [ ] Task 17 — M-10 release reproducibility
- [ ] Task 18 — M-11 private smoke captures
- [ ] Task 19 — M-12 documentation alignment
- [ ] Task 20 — L-01 golden coverage
- [ ] Task 21 — L-02 cleanup/hygiene
- [ ] Task 22 — final release-candidate gate

---

## Paste-ready Pi task blocks

### Task 00 — Establish the baseline and remediation branch

```text
TASK 00 — SELF TUI AUDIT REMEDIATION BASELINE

Work from the SelfTUI repository root. This task may create one branch but must not edit, commit, stash, reset, clean, tag, push, or publish anything.

1. Read AGENTS.md, PLAN.md §§10–12, SECURITY.md, the tail of LEDGER.md, and SelfTUI-External-Audit-2026-09-04.md.
2. Run and report exactly:
   git status --short
   git branch --show-current
   git rev-parse --short HEAD
   git log -1 --oneline
3. If git status is not empty, STOP. Preserve the exact output and ask the owner how to proceed.
4. If already on fix/v0.1.1-audit-remediation, keep it. Otherwise create it from the current clean reviewed branch with:
   git switch -c fix/v0.1.1-audit-remediation
5. Run the implementation baseline:
   make check
   make race
   make vuln
6. Do not run make smoke or release-check.
7. If any baseline command fails, STOP and report the exact command, exit status, and first actionable error. Do not patch during Task 00.
8. If all three pass, report BASELINE=PASS, branch, HEAD, Go version, govulncheck version, and elapsed time. Leave the worktree clean.
```

### Task 01 — H-01: Restore default XDG configuration loading

```text
TASK 01 — H-01 DEFAULT CONFIG LOAD/RESTART REGRESSION

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Task 00 passed. Confirm branch fix/v0.1.1-audit-remediation and a clean git status. Read cmd/self-tui/main.go, cmd/self-tui/main_test.go, internal/config/load.go, internal/config/config_test.go, internal/config/save_test.go, and audit finding H-01.

Allowed files: cmd/self-tui/main.go, cmd/self-tui/main_test.go, internal/config/config_test.go, LEDGER.md. Do not alter configuration precedence or file formats.

Implement with red-green TDD:
1. Add a regression proving an omitted -config flag produces ConfigPath=nil at the entrypoint boundary, while a non-empty explicit path remains a non-nil override.
2. Add or extend a config test proving Overrides{} resolves and loads the default XDG selftui/config.toml path. Isolate and restore all XDG-related test state; never touch the real user config.
3. Run the focused test and confirm it fails because the current entrypoint passes an empty-but-non-nil ConfigPath. If it does not fail for that reason, STOP and report the mismatch.
4. Implement the minimum fix: initialize config.Overrides without ConfigPath and assign ConfigPath only when *flagConfig is non-empty. A small pure helper is allowed if it makes entrypoint testing deterministic.
5. Prove explicit -config behavior and precedence are unchanged.
6. Run:
   go test -count=1 ./cmd/self-tui ./internal/config
   make check
7. Append H-01 evidence and exact commands to LEDGER.md.
8. Run git diff --check and review the full diff. Commit only the allowed files with:
   git commit -m "fix(config): load persisted default config on startup"

Report: commit SHA, failing-test evidence, passing commands, and changed files.
```

### Task 02 — C-01: Make recursive grep enforce the sensitive-path policy

```text
TASK 02 — C-01 POLICY-AWARE RECURSIVE GREP

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Task 01 committed; branch fix/v0.1.1-audit-remediation; clean worktree. Read internal/agent/runner.go, tools.go, toolpolicy.go, policy_test.go, runner_test.go, SECURITY.md, and audit finding C-01. Preserve the five-tool surface and .env.example carve-out.

Allowed files: internal/agent/runner.go, internal/agent/tools.go, internal/agent/toolpolicy.go, internal/agent/policy_test.go, internal/agent/runner_test.go, LEDGER.md.

Implement with red-green TDD:
1. Create a temporary workspace containing ordinary matching files plus matching secrets under .env, .env.local, .ssh, .gnupg, .aws, .azure, .kube, .config/gcloud, credentials, and credentials.json; include .env.example as an allowed control.
2. Drive the real runner/native grep tool with path ".". Assert denied contents and denied path names never appear, ordinary matches do appear, and .env.example follows the documented carve-out.
3. Run the focused regression and confirm the current code exposes at least one denied descendant. If not, STOP and explain the evidence conflict.
4. Change recursive grep so every discovered canonical workspace-relative file is authorized before opening. Prune denied directories before descent. Do not convert policy denials into a whole-operation failure: skip denied descendants deterministically while preserving real traversal/I/O errors.
5. Add context.Context to the grep boundary now so Task 13 will not redesign the API; check cancellation while walking and before scanning each file. Update all call sites/tests with an explicit allow-all callback only where no policy is intended.
6. Retain deterministic sorting and the existing result-size contract.
7. Run:
   go test -count=1 ./internal/agent -run 'Test.*(Grep|Policy|Sensitive)'
   go test -count=1 ./internal/agent
   make check
   make race
8. Append C-01 evidence to LEDGER.md, run git diff --check, review the diff, and commit:
   git commit -m "fix(agent): enforce policy during recursive grep"

Report the exact denied fixtures, test results, commit SHA, and whether any public function signature changed.
```

### Task 03 — H-02: Canonicalize workspace validation

```text
TASK 03 — H-02 CANONICAL WORKSPACE ROOT VALIDATION

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–02 committed; correct branch; clean status. Read internal/config/validate.go, internal/config/tools_test.go, internal/config/validate_test.go, internal/agent/tools.go canonicalRoot/securePath, README.md workspace contract, and H-02.

Allowed files: internal/config/validate.go, internal/config/tools_test.go, internal/config/validate_test.go, internal/config/load.go only if canonical persistence requires it, README.md only if behavior wording must be clarified, LEDGER.md.

Implement with red-green TDD:
1. Add table-driven tests for tools_enabled=true with: /tmp/.. resolving to /, a symlink to /, a symlink to the current home, a relative path that resolves to home when the test controls cwd, direct /, direct home, and a valid project directory.
2. Confirm the alias/symlink cases fail before the patch because Validate accepts them.
3. Introduce one config-local canonical-directory helper using filepath.Abs, filepath.EvalSymlinks, and directory validation. Canonicalize both candidate root and home before comparison.
4. Reject canonical / and canonical home with the existing stable error texts. Preserve tools_enabled=false behavior.
5. Ensure the runtime receives/stores the canonical accepted workspace, or prove validation and agent canonicalization cannot diverge. Do not import internal/agent into internal/config.
6. Run:
   go test -count=1 ./internal/config -run 'Test.*Tools.*Workspace'
   go test -count=1 ./internal/config
   make check
7. Append H-02 evidence, run git diff --check, review, and commit:
   git commit -m "fix(config): reject canonical root and home workspaces"

Report the alias cases, exact errors, commit SHA, and passing gates.
```

### Task 04 — H-03: Bound native tool streams and total executions

```text
TASK 04 — H-03 NATIVE TOOL BYTE/CALL BUDGETS

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–03 committed; correct branch; clean status. Read internal/ollama/chat.go, stream.go, types.go, stream_test.go, chat_test.go, internal/agent/runner.go, runner_test.go, context.go, and H-03.

Allowed files: internal/ollama/chat.go, internal/ollama/stream.go, internal/ollama/types.go, internal/ollama/stream_test.go, internal/ollama/chat_test.go, internal/agent/runner.go, internal/agent/runner_test.go, LEDGER.md.

Required limits: retain the existing 16 MiB cumulative chat-stream ceiling, but count complete raw NDJSON event bytes including JSON framing and tool calls. Add a 1 MiB decoded argument limit per tool call and a 64-call maximum per Runner.Run across all model iterations. Reject a batch that would cross the call limit before executing any call in that batch.

Implement with red-green TDD:
1. Add a stream test emitting repeated sub-4 MiB tool-argument events whose cumulative raw bytes cross 16 MiB. Assert the crossing event is not delivered and the stable chat-stream-too-large error is returned.
2. Add runner tests for a single >1 MiB argument and a batch/run exceeding 64 calls. Assert zero calls from the crossing batch execute.
3. Add adversarial merge tests for two simultaneous calls, repeated complete calls, fragmented arguments, same-name calls, and ambiguous fragments. If the wire type provides no stable ID/index, reject ambiguous fragmentation instead of guessing.
4. Run the new tests and confirm the current code fails by accepting/unboundedly accumulating or executing the payload.
5. Implement cumulative raw-byte accounting before callback delivery, per-call argument validation before execution, and a run-wide call counter.
6. Preserve normal single/parallel calls below the limits and existing public errors where possible; define and test stable new errors for argument/call limits.
7. Run:
   go test -count=1 ./internal/ollama -run 'TestChat.*(Cumulative|Tool|Oversized)'
   go test -count=1 ./internal/agent -run 'Test.*(Tool|Call|Bound|Merge)'
   go test -count=1 ./internal/ollama ./internal/agent
   make check
   make race
8. Append H-03 evidence, run git diff --check, review, and commit:
   git commit -m "fix(agent): bound native tool streams and executions"

Report exact limits, errors, red/green evidence, commit SHA, and gates.
```

### Task 05 — H-04: Make the live smoke test non-destructive

```text
TASK 05 — H-04 NON-DESTRUCTIVE PULL/DELETE SMOKE

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–04 committed; correct branch; clean status. Do not contact a live Ollama host until fake-host tests pass. Read scripts/pull-delete-smoke.py, Makefile smoke targets, README.md live behavior text, and H-04.

Allowed files: scripts/pull-delete-smoke.py, a new scripts/pull_delete_smoke_test.py, README.md, Makefile only if a safe test target is justified, LEDGER.md.

Implement with red-green TDD using only Python's standard library:
1. Refactor the smallest import-safe preflight/cleanup functions needed for unittest.mock; preserve command-line behavior under if __name__ == "__main__".
2. Add a fake-host test where the target model exists initially. Assert the script aborts before any DELETE or pull/TUI action and returns a clear nonzero result.
3. Add a test where the target is absent. Assert cleanup deletes only after this run created the model, including failure cleanup.
4. Confirm the pre-existing-model test fails against current behavior because api_delete occurs before state capture.
5. Implement initial-state capture before mutation. Never restore by re-pulling a tag. Abort safely if the target exists; cleanup only resources created by this run.
6. Update README: make smoke refuses an already-installed target and users should select a disposable model/tag or isolated Ollama store.
7. Run:
   python3 -m unittest -v scripts/pull_delete_smoke_test.py
   python3 -m py_compile scripts/pull-delete-smoke.py scripts/pull_delete_smoke_test.py
   make check
8. Do not run make smoke in this task unless the owner separately confirms the target model is disposable and the host is isolated.
9. Append H-04 evidence, run git diff --check, review, and commit:
   git commit -m "fix(smoke): preserve pre-existing Ollama models"

Report fake-host call order, exact safe-abort message, commit SHA, and gates.
```

### Task 06 — H-05: Sanitize untrusted terminal output

```text
TASK 06 — H-05 TERMINAL CONTROL-SEQUENCE BOUNDARY

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–05 committed; correct branch; clean status. This finding requires reproduction. Read all remote-derived render paths in internal/ui/agent_view.go and models_view.go, existing ANSI helpers/tests, go.mod, and H-05. Do not strip SelfTUI's own styles after they are applied.

Allowed files: create internal/ui/sanitize.go and internal/ui/sanitize_test.go; modify internal/ui/agent_view.go, internal/ui/models_view.go, related focused tests, go.mod/go.sum only if an already-pinned Charm ANSI utility must become direct, LEDGER.md.

Implement with red-green TDD:
1. Add tests injecting OSC 52, CSI clear-screen/cursor movement, DCS, BEL, carriage return, and C1 controls into chat tokens, model names, API errors, tool inputs/results, and Markdown-render failure fallback.
2. Assert rendered output contains no unsafe control sequences/bytes while preserving newline, tab, ordinary Unicode, and intended Markdown content.
3. Confirm at least the raw fallback currently fails. Also record whether Glamour's successful path preserves or strips each payload.
4. If no unsafe sequence reaches any final View output, mark H-05 NOT_REPRODUCED in LEDGER.md and stop without speculative code.
5. Otherwise create sanitizeTerminalText(string) string as the single pre-style boundary. Reuse an already-pinned Charm ANSI parser/stripper if it removes OSC/CSI/DCS safely; do not add a new dependency merely for convenience. Filter remaining unsafe C0/C1 controls, retaining only \n and \t.
6. Apply sanitization to every remote-derived value before SelfTUI styling/render caching. Avoid double-sanitizing user-authored local Markdown unless required by the same terminal threat model; document the chosen boundary in code.
7. Run:
   go test -count=1 ./internal/ui -run 'Test.*(Sanitize|Control|OSC|CSI)'
   go test -count=1 ./internal/ui
   make check
   make race
   make vuln
8. Update LEDGER.md, run git diff --check, review, and commit:
   git commit -m "fix(ui): sanitize untrusted terminal control sequences"

Report reproduction bytes, sanitization rules, dependency impact, commit SHA, and gates.
```

### Task 07 — H-06: Make audit-pack creation manifest-complete

```text
TASK 07 — H-06 MANIFEST-COMPLETE AUDIT PACKAGING

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–06 committed; correct branch; clean status. Read FILE-INVENTORY.md, the audit prompt's package contract, existing scripts, .gitignore, .gitattributes, and H-06. This fixes packaging evidence, not CI workflow behavior.

Allowed files: create scripts/create-audit-pack.sh and scripts/create-audit-pack-test.sh; modify Makefile, README.md or CONTRIBUTING.md for usage, LEDGER.md. Do not modify workflow contents in this task.

Implement with red-green verification:
1. Write a shell test that creates a disposable git fixture containing ordinary files plus .github/workflows/ci.yml, .github/workflows/release.yml, .gitignore, and .gitattributes. Prove a naive non-hidden copy fails the manifest comparison.
2. Implement scripts/create-audit-pack.sh using git ls-files as the authoritative tracked set and an archive/copy mechanism that preserves dotfiles. Add supplied prompt/inventory files only through explicit arguments, never by silently substituting them for tracked files.
3. After creation, compare sorted tracked paths against sorted archived tracked paths in both directions. Abort nonzero on missing or unexpected tracked content; verify every archived member is a safe relative path.
4. Make output deterministic where practical and print archive path, tracked-file count, SHA256, and MANIFEST_MATCH=PASS.
5. Add a Makefile audit-pack target with explicit input/output arguments or documented defaults that cannot overwrite an existing archive silently.
6. Run:
   bash -n scripts/create-audit-pack.sh scripts/create-audit-pack-test.sh
   bash scripts/create-audit-pack-test.sh
   make check
7. Generate one disposable audit pack from the current repository and prove all four formerly omitted paths are present. Do not commit the generated ZIP.
8. Append H-06 evidence, run git diff --check, review, and commit:
   git commit -m "build(audit): verify complete tracked-file packages"

Report archive count, four-path proof, SHA256, commit SHA, and gates.
```

### Task 08 — M-01: Enforce approval expiry and modal key ownership

```text
TASK 08 — M-01 APPROVAL TIMEOUT AND HARD MODAL ROUTING

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–07 committed; correct branch; clean status. Read internal/agent/runner.go, mutation_test.go, internal/ui/app.go, app_test.go, agent_view.go/tests, models_view.go/tests, routing_regression_test.go, and M-01.

Allowed files: internal/agent/runner.go, internal/agent/mutation_test.go, internal/ui/app.go, internal/ui/app_test.go, internal/ui/agent_view_test.go, internal/ui/models_view_test.go, internal/ui/routing_regression_test.go, LEDGER.md.

Implement with red-green TDD:
1. Add an agent test with an injected short confirmation duration that never responds. Assert the runner returns a stable timeout error, emits exactly one terminal done result, and writes/edits nothing.
2. Avoid package-global mutable timeout hooks. Add a per-Runner duration/clock seam whose production default remains 30s and whose maximum remains 60s.
3. Confirm the current test hangs or exceeds its bounded test deadline; keep the test harness itself time-bounded.
4. Implement a timer in confirm; stop/drain it correctly. A late Respond must remain nonblocking and must not mutate.
5. Add root-level Tab and Shift-Tab tests for Agent confirmation/help/selector/clear and Models confirm/input/pull/delete states. Assert the active tab does not change while a modal owns input.
6. Route child modal keys before global tab navigation while keeping Ctrl+C's documented behavior.
7. Run:
   go test -count=1 ./internal/agent -run 'Test.*Confirm.*(Timeout|Expire)'
   go test -count=1 ./internal/ui -run 'Test.*Modal.*Tab'
   go test -count=1 ./internal/agent ./internal/ui
   make check
   make race
8. Append M-01 evidence, run git diff --check, review, and commit:
   git commit -m "fix(agent): expire approvals and retain modal focus"

Report timeout/error text, modal states tested, commit SHA, and gates.
```

### Task 09 — M-02: Preserve atomic tool exchanges during context trimming

```text
TASK 09 — M-02 PROTOCOL-SAFE CONTEXT BUDGETING

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–08 committed; correct branch; clean status. Read internal/agent/context.go, context_test.go, runner.go tool-history construction, Ollama message types, and M-02.

Allowed files: internal/agent/context.go, internal/agent/context_test.go, internal/agent/runner.go only if correlation metadata is required, LEDGER.md.

Implement with red-green TDD:
1. Add table tests for: assistant tool call plus one result; one call plus multiple results; two older exchanges; a latest oversized tool call; an oversized system prompt; and a tiny num_ctx.
2. Required invariants: system messages retain their intended order; no retained role=tool message lacks its preceding assistant tool call; an assistant call is not retained without all correlated results; the newest complete user-led exchange is preferred; exactly one truncation marker is used; ApproxTokens(output) <= limit whenever a bounded representation is possible.
3. Run focused tests and confirm current one-message eviction or Content-only truncation violates at least the orphan/latest-call cases.
4. Group protocol messages into atomic eviction units. For a single oversized unshrinkable call/system message, return a deterministic bounded representation or explicit runner error; do not silently exceed the budget.
5. Preserve the public TruncationNotice text and the UI estimator's use of the same token math.
6. Run:
   go test -count=1 ./internal/agent -run 'TestBudget|Test.*Context'
   go test -count=1 ./internal/agent
   make check
7. Append M-02 evidence, run git diff --check, review, and commit:
   git commit -m "fix(agent): preserve tool exchanges in context budget"

Report every invariant, red/green evidence, commit SHA, and gates.
```

### Task 10 — M-03: Reject stale model and host responses

```text
TASK 10 — M-03 VERSION ASYNCHRONOUS MODEL REQUESTS

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–09 committed; correct branch; clean status. This finding requires reproduction. Read internal/ui/models_view.go/tests, agent_view.go/tests, app.go routing, Settings client-apply flow, and M-03.

Allowed files: internal/ui/models_view.go, internal/ui/models_view_test.go, internal/ui/agent_view.go, internal/ui/agent_view_test.go, internal/ui/app.go, internal/ui/app_test.go or phase4_test.go, LEDGER.md.

Implement with red-green TDD:
1. Build controlled clients where old-host/list or model-A/show blocks while new-host/list or model-B/show completes first.
2. Add tests proving: A's late detail cannot appear under selected B; old-host results cannot replace new-host state after ApplyClient; Agent model reload ignores an obsolete result; a selection changed during loading eventually requests the latest model.
3. Confirm at least one current test fails by accepting stale state. If none reproduces, record M-03 NOT_REPRODUCED with the exact schedule and stop without code changes.
4. Add monotonically increasing client generation and request IDs to asynchronous result messages. Capture IDs in each command and ignore mismatched completions in Update.
5. Cancel obsolete request contexts when practical. After an inspection finishes, start the current selection's request if it differs from the completed name.
6. Keep all model mutation on the Bubble Tea update loop.
7. Run:
   go test -count=1 ./internal/ui -run 'Test.*(Stale|Generation|Selection|ApplyClient)'
   go test -count=1 ./internal/ui
   make check
   make race
8. Append M-03 evidence, run git diff --check, review, and commit:
   git commit -m "fix(ui): ignore stale model request results"

Report the controlled completion order, stale-result disposition, commit SHA, and gates.
```

### Task 11 — M-04: Move transcript persistence off the update loop

```text
TASK 11 — M-04 NONBLOCKING ORDERED TRANSCRIPT PERSISTENCE

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–10 committed; correct branch; clean status. This finding requires latency reproduction. Read internal/ui/agent_view.go, session_ui_test.go, internal/session/session.go/tests, cmd/self-tui/main.go, and M-04. Preserve append-only, non-resumable Markdown semantics and exact 0600/0700 permissions.

Allowed files: internal/session/session.go, internal/session/session_test.go, create internal/session/recorder.go and recorder_test.go if an actor is selected, internal/ui/agent_view.go, internal/ui/session_ui_test.go, internal/ui/cancellation_test.go, cmd/self-tui/main.go and main_test.go for shutdown lifecycle, LEDGER.md.

Implement with red-green TDD:
1. Add an injectable blocking writer/filesystem seam. Prove that committing a user/assistant turn and invoking /export do not block AgentView.Update while persistence is stalled.
2. Add order tests for user→assistant→user turns, one-error-only reporting, disabled recording, and explicit flush/close on normal shutdown.
3. Confirm current Update blocks in the controlled test. If not reproducible, record M-04 NOT_REPRODUCED and stop without speculative restructuring.
4. Implement the smallest ordered background boundary. Prefer one recorder actor/queue owning the Log over independent concurrent commands, because transcript order must remain deterministic.
5. UI Update may enqueue immutable work and process completion/error messages, but it must not call Open, Append, Sync, or Close directly. Cancellation/shutdown must not strand the worker.
6. Keep /export semantics: flush all earlier enqueued turns before reporting the exact path. Keep session recording optional.
7. Run:
   go test -count=1 ./internal/session ./internal/ui -run 'Test.*(Session|Transcript|Recorder|Export)'
   go test -count=1 ./cmd/self-tui ./internal/session ./internal/ui
   make check
   make race
8. Append M-04 evidence, run git diff --check, review, and commit:
   git commit -m "fix(session): persist transcripts off the UI loop"

Report measured blocked/nonblocked behavior, lifecycle design, ordering proof, commit SHA, and gates.
```

### Task 12 — M-05: Replace byte-based line wrapping

```text
TASK 12 — M-05 CELL-AND-ANSI-AWARE WRAPPING

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–11 committed; correct branch; clean status. Read internal/ui/models_view.go wrapLines, components.go, truncate_test.go, models_view_test.go, small_terminal_test.go, go.mod, and M-05.

Allowed files: internal/ui/models_view.go, internal/ui/components.go only for a shared helper, internal/ui/models_view_test.go, internal/ui/small_terminal_test.go, internal/ui/truncate_test.go, go.mod/go.sum only if promoting an already-pinned Charm utility, LEDGER.md.

Implement with red-green TDD:
1. Extend TestWrapLines with over-width CJK, emoji, a ZWJ sequence, combining marks, a word wider than the limit, and ANSI-styled content.
2. For every output row assert utf8.ValidString(row), lipgloss.Width(row) <= width, visible text is neither lost nor duplicated, and no ANSI opener is split from its parameters/reset.
3. Confirm current byte slicing fails or corrupts at least one case.
4. Replace len/slice wrapping with a display-cell/grapheme-aware algorithm that tokenizes or preserves ANSI sequences. Reuse a pinned Charm width/ANSI facility where it meets all tests; do not introduce a parallel width model.
5. Preserve ASCII word-boundary behavior from the existing tests and deterministic output.
6. Run:
   go test -count=1 ./internal/ui -run 'TestWrapLines|Test.*Unicode|TestTruncateToWidth'
   go test -count=1 ./internal/ui
   make check
7. Update only golden fixtures whose semantic output intentionally changes; inspect every golden diff manually.
8. Append M-05 evidence, run git diff --check, review, and commit:
   git commit -m "fix(ui): wrap text by terminal display cells"

Report cases, chosen width primitive, golden changes, commit SHA, and gates.
```

### Task 13 — M-06: Propagate cancellation through tools and UI delivery

```text
TASK 13 — M-06 CANCELLABLE TOOLS AND CHANNEL DELIVERY

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Task 02 already made grep context-aware; Tasks 01–12 committed; correct branch; clean status. This finding requires saturation reproduction. Read internal/agent runner/tools and tests, internal/ui agent/models producers, cancellation_test.go, and M-06.

Allowed files: internal/agent/runner.go, internal/agent/tools.go, internal/agent/runner_test.go, internal/ui/agent_view.go, internal/ui/models_view.go, internal/ui/cancellation_test.go, related focused tests, LEDGER.md.

Implement with red-green TDD:
1. Add an agent test that cancels during a large/blocked filesystem operation and asserts prompt return with context.Canceled or context.DeadlineExceeded.
2. Add Agent and Pull tests that generate more than 64 events, stop draining, cancel the parent, and use explicit producer-done channels to prove goroutine termination. Do not use runtime.NumGoroutine as the only oracle.
3. Confirm current unconditional sends or tool work exceed the bounded deadline. If not reproducible, record the exact schedule as NOT_REPRODUCED and stop the unproven portion.
4. Pass context through every filesystem tool boundary. Check before/after single-file operations and repeatedly during directory walk/scan. Add deterministic total-file/total-byte/deadline limits only where needed; preserve existing output caps.
5. Replace producer sends with one context-aware helper: delivery succeeds in order or cancellation wins. Ensure exactly one terminal UI state is produced when delivery remains possible; never send on a closed channel.
6. Preserve the single-owner Bubble Tea update model and FIFO event ordering.
7. Run:
   go test -count=1 ./internal/agent -run 'Test.*Cancel.*Tool'
   go test -count=1 ./internal/ui -run 'Test.*(Saturation|Backpressure|Cancellation)'
   go test -count=1 ./internal/agent ./internal/ui
   make check
   make race
8. Append M-06 evidence, run git diff --check, review, and commit:
   git commit -m "fix(runtime): propagate cancellation through tools and streams"

Report event counts, deadlines, producer completion evidence, commit SHA, and gates.
```

### Task 14 — M-07: Render the in-flight delete overlay

```text
TASK 14 — M-07 DELETE BUSY OVERLAY

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–13 committed; correct branch; clean status. Read internal/ui/models_view.go, models_view_test.go, golden_test.go, canonical geometry helpers, and M-07.

Allowed files: internal/ui/models_view.go, internal/ui/models_view_test.go, internal/ui/golden_test.go, relevant internal/ui/testdata/golden files only if intentionally adding the state, LEDGER.md.

Implement with red-green TDD:
1. Extend TestModelsViewDeleteConfirmFlow so it renders immediately after y and before the delete command completes.
2. Assert the frame contains a deleting title/spinner and exact target model, ignores input, fits 72x30 and 120x40, and does not show the interactive model list as the active body.
3. Confirm the test fails because View has no deleting branch.
4. Add the minimum deleting overlay branch, reusing confirmLines or a clearly named deleteProgressLines helper. Do not re-enable cancellation unless the HTTP/delete contract supports it safely.
5. Prove success and error completion close the busy state and surface the existing notice/error.
6. Run:
   go test -count=1 ./internal/ui -run 'TestModelsViewDelete|TestGoldenFramesFitTerminal'
   go test -count=1 ./internal/ui
   make check
7. Inspect all golden diffs, append M-07 evidence, run git diff --check, and commit:
   git commit -m "fix(models): render delete progress overlay"

Report both geometry measurements, changed fixtures, commit SHA, and gates.
```

### Task 15 — M-08: Enforce a redirect-safe bearer-token policy

```text
TASK 15 — M-08 REDIRECT-SAFE OLLAMA CLIENT

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–14 committed; correct branch; clean status. This finding requires HTTP reproduction. Read internal/config host/token validation, internal/ollama/client.go, stream.go, ollama_test.go, stream_test.go, SECURITY.md, and M-08.

Allowed files: internal/ollama/client.go, internal/ollama/stream.go, internal/ollama/ollama_test.go, internal/ollama/stream_test.go, internal/config tests only if shared policy changes, SECURITY.md only for exact redirect behavior, LEDGER.md.

Safety decision: Ollama API clients must not follow redirects automatically. Return a stable error identifying the original API operation and refusal; do not forward Authorization to any redirect target.

Implement with red-green TDD:
1. Create TLS/HTTP test servers for same-host, cross-host, subdomain-equivalent where practical, and HTTPS→HTTP redirects. Capture every received Authorization header.
2. Confirm current default client follows at least one redirect or otherwise demonstrate the pinned Go behavior. If no credential-bearing redirect can occur, record M-08 NOT_REPRODUCED and keep only documentation/tests needed to pin that behavior.
3. Configure both finite and streaming http.Client instances with the same CheckRedirect refusal policy.
4. Test that the client returns the stable redirect-refused error, the target receives no bearer token, ordinary non-redirect requests still work, and response bodies/connections close correctly.
5. Do not weaken initial non-loopback token/HTTPS validation.
6. Run:
   go test -count=1 ./internal/ollama -run 'Test.*Redirect|TestHTTPS|Test.*Bearer'
   go test -count=1 ./internal/ollama ./internal/config
   make check
   make race
7. Append M-08 evidence, run git diff --check, review, and commit if a patch is required:
   git commit -m "fix(ollama): refuse API redirects carrying credentials"

Report observed Go redirect behavior, header captures, commit SHA or NOT_REPRODUCED disposition, and gates.
```

### Task 16 — M-09: Maintain exactly one spinner command chain

```text
TASK 16 — M-09 SINGLE SPINNER SUBSCRIPTION

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–15 committed; correct branch; clean status. This finding requires pinned-dependency verification. Read internal/ui/models_view.go spinner flow/tests, the exact pinned bubbles/v2 spinner API source/docs available locally, routing_regression_test.go, and M-09.

Allowed files: internal/ui/models_view.go, internal/ui/models_view_test.go, internal/ui/routing_regression_test.go, LEDGER.md.

Implement with red-green TDD:
1. Build a deterministic test seam/counter around spinner scheduling. Enter pulling, deliver several progress messages and spinner ticks, and assert only one successor tick remains scheduled per consumed tick.
2. Confirm current progress events increase independent scheduled chains. If the pinned spinner API disproves this behavior, record M-09 NOT_REPRODUCED with source/API evidence and stop without patching.
3. Seed one spinner command only when transitioning into pulling or deleting.
4. On spinner.TickMsg, capture the command returned by v.spinner.Update(msg), wrap that command's eventual message in modelsEventMsg, and schedule it only while the spinner state remains active.
5. Progress handlers must resubscribe only to pull activity; unrelated events must not create spinner chains. Completion must stop rescheduling.
6. Run:
   go test -count=1 ./internal/ui -run 'Test.*Spinner|TestModelsViewPull|TestModelsViewDelete'
   go test -count=1 ./internal/ui
   make check
   make race
7. Append M-09 evidence, run git diff --check, review, and commit if reproduced:
   git commit -m "fix(models): keep one spinner command chain"

Report scheduler counts before/after, dependency API evidence, commit SHA or NOT_REPRODUCED, and gates.
```

### Task 17 — M-10: Make the release gate portable and reproducible

```text
TASK 17 — M-10 RELEASE TOOLCHAIN, ARCHIVE MODES, AND CHECKSUM PATHS

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–16 committed; correct branch; clean status. Read scripts/release-check.sh, verify-binary-version.sh, Makefile, README release engineering, CONTRIBUTING.md, go.mod, and M-10. Do not run the full release-check in this task.

Allowed files: scripts/release-check.sh, create scripts/release-check-test.sh, Makefile, README.md, CONTRIBUTING.md, LEDGER.md.

Required contract: local release-check must fail fast unless the documented Go 1.27.1 and govulncheck v1.7.0 are active; archive members must have fixed modes (binary 0755, LICENSE/README 0644); SHA256SUMS must contain flat archive names usable from a download directory.

Implement with red-green tests:
1. Create a shell harness using temporary fake go/gofmt/govulncheck binaries to prove wrong/missing versions fail before slow gates and correct versions proceed to a safely stubbed checkpoint. Never invoke destructive dist handling in the harness outside its temp fixture.
2. Create two staging/archive fixtures under umask 0002 and 0022; confirm current member modes/hashes differ, then require identical hashes and exact modes.
3. In the v0.1.1 fixture, assert SHA256SUMS contains only selftui-v0.1.1-linux-amd64.tar.gz and selftui-v0.1.1-linux-arm64.tar.gz, never dist/ prefixes or absolute paths. Keep production generation driven by the validated VERSION variable.
4. Add explicit version checks with stable actionable errors. Parse actual command outputs defensively and test the exact accepted/rejected forms.
5. Stage with install -m 0755 for binaries and install -m 0644 for documents, or equivalent deterministic tar mode normalization.
6. Generate checksums from inside dist so downloaded assets verify with sha256sum -c SHA256SUMS.
7. Update README/CONTRIBUTING to distinguish enforced local prerequisites from CI configuration.
8. Run:
   bash -n scripts/release-check.sh scripts/release-check-test.sh
   bash scripts/release-check-test.sh
   make check
9. Do not run VERSION=... make release-check until Task 22 and a clean worktree.
10. Append M-10 evidence, run git diff --check, review, and commit:
   git commit -m "fix(release): enforce reproducible portable artifacts"

Report version-check cases, cross-umask hashes, tar modes, checksum entries, commit SHA, and gates.
```

### Task 18 — M-11: Use private unique smoke captures

```text
TASK 18 — M-11 PRIVATE UNIQUE SMOKE CAPTURES

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Task 05 completed the non-destructive smoke refactor; Tasks 01–17 committed; correct branch; clean status. Read both Python smoke scripts/tests and M-11. Do not contact live services.

Allowed files: scripts/pull-delete-smoke.py, scripts/pull_delete_smoke_test.py, scripts/reconnect-smoke.py, create scripts/reconnect_smoke_test.py, README.md if capture behavior is documented, LEDGER.md.

Implement with red-green standard-library tests:
1. Pre-place symlinks at the old fixed /tmp paths and prove the refactored scripts never open or modify their targets.
2. Assert each run gets a unique 0700 temporary directory and 0600 exclusive capture file. Verify failure retains the capture and prints its exact path; success removes it by default unless an explicit keep-capture option/environment variable is set.
3. Confirm the current fixed-path code fails the symlink/uniqueness test.
4. Use tempfile.TemporaryDirectory/mkdtemp and os.open with O_CREAT|O_EXCL plus 0600 where explicit files are needed. Never follow a caller-controlled fixed symlink.
5. Ensure reconnect's existing private scratch config/state remains private and is cleaned predictably.
6. Run:
   python3 -m unittest -v scripts/pull_delete_smoke_test.py scripts/reconnect_smoke_test.py
   python3 -m py_compile scripts/pull-delete-smoke.py scripts/reconnect-smoke.py scripts/pull_delete_smoke_test.py scripts/reconnect_smoke_test.py
   make check
7. Append M-11 evidence, run git diff --check, review, and commit:
   git commit -m "fix(smoke): store captures in private temp paths"

Report modes, symlink test, success/failure retention behavior, commit SHA, and gates.
```

### Task 19 — M-12: Align security and release documentation

```text
TASK 19 — M-12 CONTRACT AND RELEASE-DOC ALIGNMENT

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: All behavior patches through Task 18 committed; correct branch; clean status. Read SECURITY.md, README.md, CHANGELOG.md, PLAN.md §§10–13, latest LEDGER.md entries, ToolPolicy implementation/tests, stream limits, and M-12. Documentation must describe the code now present on this branch, not the pre-audit snapshot.

Allowed files: SECURITY.md, README.md, CHANGELOG.md, PLAN.md, CONTRIBUTING.md only if needed for one consistent release statement, LEDGER.md.

Implement as an evidence-backed documentation patch:
1. Enumerate the final sensitive-path policy exactly, including .env.example and exact credential basename behavior. Do not use wildcard language broader than tests.
2. Describe stream limits exactly: per-event raw cap, cumulative raw chat cap, argument/call caps, idle/header/redirect behavior as actually implemented.
3. Change README from pre-release hardening to the accurate v0.1.0-published/v0.1.1-hardening state.
4. Cut [v0.1.0] - 2026-09-04 from the existing Unreleased material without losing entries; leave a fresh [Unreleased] section for v0.1.1 changes.
5. Reconcile PLAN §12 so it has one current next action and repair the visibly truncated §13 sentence. Preserve historical detail in LEDGER.md rather than rewriting history.
6. Mention the already-queued gitleaks-in-CI, actionlint, Node action updates, signed-tag decision, and public-visibility decision without claiming absent workflow work is complete.
7. Run factual searches proving no stale claims remain:
   rg -n 'release hardening|will ship|credentials\*|\.env\*|pull/chat bodies cannot grow without limit' README.md CHANGELOG.md SECURITY.md PLAN.md
   make check
8. Review every remaining match and explain why it is current or remove/correct it.
9. Append M-12 evidence, run git diff --check, review, and commit:
   git commit -m "docs: align security and v0.1 release state"

Report corrected claims, remaining intentional matches, commit SHA, and gates.
```

### Task 20 — L-01: Make light-theme golden coverage real

```text
TASK 20 — L-01 SHARED GOLDEN SCENARIO BUILDERS

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–19 committed; correct branch; clean status. Read internal/ui/golden_test.go, every golden fixture name, scenario-driving helpers, and L-01. Do not redesign production UI in this task.

Allowed files: internal/ui/golden_test.go, test-only helpers in internal/ui/*_test.go if necessary, intentional golden fixture updates only, LEDGER.md.

Implement with red-green test refactoring:
1. Add an assertion mapping every goldenFrames name to an explicit scenario builder. The test must fail if a name falls through to a default screen.
2. Extract/reuse one scenario builder per frame that accepts theme and geometry, and use it for both dark golden comparison and light geometry rendering.
3. Before width/height assertions, verify the intended active tab, modal/state marker, and representative content for that frame.
4. Confirm the old light loop misconstructs at least agent-turn, palette, picker, slash/help/clear cases.
5. Preserve the 72x30 and 120x40 raw display-width/height guards and existing byte-exact dark fixtures.
6. Run:
   go test -count=1 ./internal/ui -run 'TestGolden|TestLightTheme'
   go test -count=1 ./internal/ui
   make check
7. Inspect every fixture diff. Do not accept bulk golden updates without explaining each semantic change.
8. Append L-01 evidence, run git diff --check, review, and commit:
   git commit -m "test(ui): share complete golden frame scenarios"

Report scenario count/mapping, corrected false cases, fixture changes, commit SHA, and gates.
```

### Task 21 — L-02: Complete cleanup and resolve the timer artifact

```text
TASK 21 — L-02 OWNED ARTIFACT CLEANUP

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–20 committed; correct branch; clean status. Read Makefile, .gitignore, internal/ui/timer.sh, routing_regression_test.go, git history/ledger references available locally, and L-02.

Allowed files: Makefile, .gitignore only if owned output rules need correction, internal/ui/timer.sh only for deletion if proven unused, related tests/docs, LEDGER.md.

Implement with evidence first:
1. Prove bin/size-probe and dist release outputs are created by repository targets and are intended owned artifacts. Add a safe cleanup verification using disposable sentinel files: clean may remove only known owned outputs and must leave unrelated files outside owned directories untouched.
2. Search every tracked file and available history/ledger reference for internal/ui/timer.sh. If it has no runtime, fixture, packaging, or documentation consumer, delete it. If a consumer exists, retain it and document the reason; do not delete based only on appearance.
3. Update clean to remove bin/selftui, bin/size-probe, and the repository-owned dist directory using explicit repository-relative paths. Do not use unresolved environment variables, broad globs, HOME, or paths outside the repository.
4. Run:
   make check
   make probe-build
   test -f bin/size-probe
   make clean
   test ! -e bin/selftui
   test ! -e bin/size-probe
   test ! -e dist
   make check
5. Record whether timer.sh was deleted or retained and why.
6. Append L-02 evidence, run git diff --check, review, and commit:
   git commit -m "chore: clean all repository-owned build artifacts"

Report before/after artifact list, timer disposition, commit SHA, and gates.
```

### Task 22 — Final verification and release-candidate evidence

```text
TASK 22 — FINAL V0.1.1 AUDIT REMEDIATION GATE

Session guard: run git status --short and git branch --show-current. If status is nonempty or the branch is not fix/v0.1.1-audit-remediation, STOP without editing. Read AGENTS.md, PLAN.md §§10–12, and the tail of LEDGER.md before work.

Prerequisite: Tasks 01–21 have a recorded PASS, NOT_REPRODUCED disposition, or explicit owner-approved deferral; correct branch; clean status. This task may generate ignored artifacts under dist/ and one audit ZIP, but must not tag, push, publish, or contact a live Ollama host.

1. Read AGENTS.md, the full external audit, every Task 01–21 LEDGER entry, PLAN §12 tail, SECURITY.md, README.md, CHANGELOG.md, both GitHub workflows, release scripts, and the diff from the branch base.
2. Build a finding matrix C-01 through L-02 with columns: status, commit, regression test, focused gate, residual risk. No finding may disappear. H-06 must distinguish audit-package completeness from repository behavior.
3. Confirm clean tracked state before the release gate. Resolve the actual branch base without a placeholder:
   git status --short
   base_commit="$(git merge-base main HEAD)"
   test -n "$base_commit"
   git log --oneline --decorate --no-merges "$base_commit"..HEAD
   git diff --check "$base_commit"..HEAD
   git diff --stat "$base_commit"..HEAD
4. Run fresh, complete gates and preserve exact output/exit status:
   make check
   make race
   make vuln
   go test -race -count=1 ./...
5. Confirm the documented Go and govulncheck versions are active. Then, on the clean worktree and using the intended v0.1.1 value, run:
   VERSION=v0.1.1 make release-check
6. Verify dist/SHA256SUMS from inside dist, inspect archive members/modes for both architectures, and prove a second clean release build produces identical hashes. Do not delete evidence until hashes are recorded.
7. Do not run make smoke against the user's normal Ollama store. If live smoke is required, STOP and request explicit confirmation of an isolated host/model store and disposable model.
8. Use the Task 07 packaging script to generate a new audit package. Compare its tracked manifest to git ls-files in both directions and explicitly prove inclusion of:
   .github/workflows/ci.yml
   .github/workflows/release.yml
   .gitignore
   .gitattributes
9. Perform a read-only review of workflow permissions, triggers, action refs, tag/version/ancestry guard, fetch depth, release-note extraction, and asset paths. Mark anything requiring GitHub settings as NEEDS_CONFIRMATION.
10. Append one final LEDGER entry with the full finding matrix, gate outputs, artifact hashes, package SHA256, residual risks, and RELEASE_CANDIDATE_READY=YES or NO. Commit only the ledger/docs change if tracked state changed:
    git commit -m "docs: record v0.1.1 audit remediation evidence"
11. Finish with git status --short. Report the exact status; do not claim clean if it is not empty. Do not tag, push, open a PR, or publish.

Final response must include: branch/HEAD, all finding statuses, all commits, full gate results, artifact hashes, audit-package manifest proof, unresolved GitHub-setting checks, and the single next owner decision.
```

## Completion policy

A finding is complete only when its regression failed before the patch for the expected reason, passed after the patch, the requested broader gates passed freshly, the diff was reviewed, and the result was recorded in `LEDGER.md`. For a runtime-dependent finding, `NOT_REPRODUCED` with a deterministic test and exact evidence is an acceptable disposition; an untested assumption is not.

Do not collapse tasks merely because they touch the same file. The numbered commits are intentional review and rollback boundaries.
