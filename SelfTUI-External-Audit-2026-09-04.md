# SelfTUI External Audit and Optimization Plan

**Snapshot:** claimed commit `e6ef11b` on `main`, dated 2026-09-04  
**Review mode:** static, read-only source review; no builds, tests, vulnerability scans, or live Ollama operations were run

## 1. Executive summary

SelfTUI has a stronger-than-average small-project baseline: tools are opt-in, the tool dispatch surface is closed, command execution is intentionally absent, common traversal and static symlink escapes are blocked, streams have per-event/idle bounds, persisted secrets use restrictive permissions, and the TUI has substantial geometry and cancellation coverage.

The release should nevertheless be treated as needing a focused security/correctness patch. The top security defect is that recursive `grep` authorizes only the requested root and then reads denied descendants such as `.env` or `.ssh/*`. The top product defect is even broader in reach: normal startup always supplies an empty `ConfigPath` override, so the default XDG config is not loaded and Settings changes do not survive an ordinary restart. Workspace-root aliases, unbounded native tool batches, a destructive smoke script, and unsanitized terminal control sequences are the other highest-priority risks.

One audit limitation is material: the archive claims to contain 97 tracked files but omits both GitHub Actions workflows plus `.gitignore` and `.gitattributes`. CI/release workflow behavior therefore could not be independently audited.

## 2. Findings table

| ID | Severity | Area | Primary location | Summary |
|---|---|---|---|---|
| C-01 | Critical | Security | `internal/agent/runner.go:319-333`; `internal/agent/tools.go:116-175` | Recursive `grep` bypasses the sensitive-path denylist. |
| H-01 | High | Correctness/config | `cmd/self-tui/main.go:46,66`; `internal/config/load.go:61-79` | Normal startup skips the persisted default config. |
| H-02 | High | Security/config | `internal/config/validate.go:63-77`; `internal/agent/tools.go:240-259` | Aliases of `/` or the home directory pass workspace validation. |
| H-03 | High | Security/resource bounds | `internal/ollama/chat.go:145-190`; `internal/agent/runner.go:167-245` | Native tool arguments and parallel calls escape cumulative limits. |
| H-04 | High | Release/smoke safety | `scripts/pull-delete-smoke.py:93-100,210-213` | `make smoke` can permanently delete a pre-existing model. |
| H-05 | High | Terminal security | `internal/ui/agent_view.go:337-347,1155-1171`; `internal/ui/models_view.go:670-706` | Untrusted Ollama text can reach the terminal with control sequences. `[needs runtime verification]` |
| H-06 | High | Audit completeness | `FILE-INVENTORY.md:3,13,56,77,91-92` | Four promised tracked files are absent from the ZIP; workflows are unauditable. |
| M-01 | Medium | Correctness/UX | `internal/agent/runner.go:385-403`; `internal/ui/app.go:165-188` | Approval has no real timeout, and Tab can hide the modal. |
| M-02 | Medium | Agent protocol | `internal/agent/context.go:13-61` | Context eviction can orphan tool results and still exceed the budget. |
| M-03 | Medium | Async correctness | `internal/ui/models_view.go:224-233,330-341,508-515,1034-1046` | Stale model responses overwrite a newer selection or host. `[needs runtime verification]` |
| M-04 | Medium | UI responsiveness | `internal/ui/agent_view.go:700-753`; `internal/session/session.go:31-69,92-147` | Transcript filesystem work blocks the Bubble Tea update loop. `[needs runtime verification]` |
| M-05 | Medium | Terminal correctness | `internal/ui/models_view.go:953-979` | Wrapping measures cells but slices bytes, corrupting Unicode/ANSI. |
| M-06 | Medium | Cancellation/concurrency | `internal/agent/runner.go:295-375`; `internal/ui/agent_view.go:267-275`; `internal/ui/models_view.go:274-284` | Filesystem work and channel sends cannot be canceled under load. `[needs runtime verification]` |
| M-07 | Medium | Models UX | `internal/ui/models_view.go:539-545,599-612,754-767` | The in-flight delete state suppresses the promised busy overlay. |
| M-08 | Medium | HTTP/token safety | `internal/ollama/client.go:41-47,65-76`; `internal/ollama/stream.go:48-64` | Redirects are not constrained by the non-loopback HTTPS token policy. `[needs runtime verification]` |
| M-09 | Medium | Async performance | `internal/ui/models_view.go:290-291,343-367` | Pull progress can multiply spinner command chains. `[needs runtime verification]` |
| M-10 | Medium | Release engineering | `scripts/release-check.sh:14-16,62-71,123-140` | The release gate does not fully deliver portable, cross-builder artifacts. |
| M-11 | Medium | Script security | `scripts/pull-delete-smoke.py:30,86-89`; `scripts/reconnect-smoke.py:41,163-167` | Predictable `/tmp` captures risk disclosure and symlink-following overwrites. |
| M-12 | Medium | Docs/contract | `SECURITY.md:45-49`; `README.md:10`; `CHANGELOG.md:8,60-63` | Security and release-state claims are broader or older than reality. |
| L-01 | Low | Test quality | `internal/ui/golden_test.go:457-494` | The light-theme “every frame” loop renders many named scenarios as the default Models view. |
| L-02 | Low | Repository hygiene | `Makefile:36-40,63-75`; `internal/ui/timer.sh:1-2` | `make clean` is incomplete and a generated timer fixture appears tracked. |

## 3. Detailed findings

### Critical

#### C-01 — Recursive `grep` bypasses the sensitive-path denylist

**Evidence.** `internal/agent/runner.go:319-333` checks only the model-supplied root — `if err := r.authorizePath(args.Path); err != nil` — and then calls `Grep(root, args.Pattern, args.Path)`. When that path is a directory, `internal/agent/tools.go:137-151` executes `filepath.WalkDir`; it skips only `.git` and appends every non-symlink file. `ToolPolicy.AuthorizePath` is never applied to discovered descendants. Yet `SECURITY.md:45-47` promises that workspace access is “gated by a sensitive-path denylist” covering `.ssh`, `.aws`, `.env*`, and similar paths.

**Why it matters.** `grep` on `.` is both allowed and normal. In a typical project it can read `.env`, `.aws/credentials`, `.ssh/*`, `.config/gcloud/*`, or `credentials.json`, then return matches to the configured local or remote model. Tools being off by default limits exposure, but once a user deliberately enables them this violates the core safety contract.

**Suggested fix.** Make traversal policy-aware: prune denied directories, reject denied basenames before opening files, and authorize each canonical workspace-relative descendant. Preserve the `.env.example` carve-out explicitly. Add regression tests that seed every sensitive class beneath a root, run `grep(".")`, and prove neither matching content nor path names escape. Also test errors and nested denied directories.

**Implementation-time verification.** Run focused agent tests, then `make check`, `make race`, and `make vuln`.

### High

#### H-01 — Normal startup skips the persisted default config

**Evidence.** `cmd/self-tui/main.go:46` creates `flagConfig := flag.String("config", "", ...)`; line 66 always assigns that non-nil pointer to `Overrides{ConfigPath: flagConfig}`. `internal/config/load.go:61-69` resolves `$XDG_CONFIG_HOME/selftui/config.toml` only when `ov.ConfigPath == nil`. With the ordinary no-flag invocation, `Load` therefore sets `cfg.filePath` to `""` and calls `os.ReadFile("")`. `config.Save` later falls back to the XDG path (`internal/config/load.go:239-246`), so Settings can write the default file that the next ordinary startup again ignores.

**Why it matters.** Host, token, theme, model, workspace, tool enablement, and agent settings appear to save but do not persist across normal restarts. This affects essentially every user of the Settings tab and also leaves `ConfigPath()` empty for UI/status reporting.

**Suggested fix.** Populate `ov.ConfigPath` only when `*flagConfig != ""`, ideally via a small pure flag-to-overrides helper. Add an entrypoint-level test with an isolated XDG config home proving (1) no flag loads the default file, (2) `-config` loads the explicit file, and (3) a Settings save is loaded by a fresh default invocation.

#### H-02 — Workspace-root aliases bypass the `/` and home bans

**Evidence.** `internal/config/validate.go:66-76` rejects only the literal `"/"` and compares `filepath.Clean(workspaceRoot)` to the lexical home path. It does not call `filepath.Abs` or `filepath.EvalSymlinks`. The tool layer later does both in `internal/agent/tools.go:240-259`. Values such as `/tmp/..`, a relative path resolving to home, or `/tmp/project -> /` can therefore validate and subsequently become the entire filesystem or home directory.

**Why it matters.** The README’s “never `/` or your home directory” boundary can be defeated by ordinary path aliases, handing an enabled agent much broader filesystem visibility than the user interface and validation promise.

**Suggested fix.** Resolve both the candidate and home through one shared absolute/canonical routine before comparison; reject a canonical `/` and filesystem-identical home. Pass/store the canonical root so validation and execution cannot disagree. Add lexical-alias, relative-CWD, and symlink-root tests.

#### H-03 — Native tool batches escape stream-size and tool-count bounds

**Evidence.** `internal/ollama/chat.go:173-175` increments `contentBytes` only for content and thinking; decoded `Message.ToolCalls` at lines 155-182 are omitted. `internal/agent/runner.go:167` bounds model round-trips, but lines 223-243 execute every call in a returned batch, with no run-wide call count. `mergeToolCalls` at lines 411-448 repeatedly copies/concatenates argument fragments.

**Why it matters.** A misbehaving or malicious Ollama endpoint can send arbitrarily many sub-4 MiB argument events or thousands of parallel read calls in one iteration. Memory and CPU can grow without the documented 16 MiB cumulative cap, fragment merging can become quadratic, and `max_tool_iterations` does not constrain actual tool executions.

**Suggested fix.** Prefer a cumulative raw-NDJSON byte cap, then add explicit maximum tool calls per event/run and per-call argument limits. Correlate fragments by protocol ID/index rather than only slice position/name, and accumulate with bounded linear storage. Reject the crossing event before callback delivery. Add fragmented-argument and multi-call adversarial tests.

#### H-04 — The live smoke test can delete a pre-existing model permanently

**Evidence.** The script promises “The host is left exactly as found” at `scripts/pull-delete-smoke.py:5-9`, and README repeats it at `README.md:231-232`. But `main` unconditionally calls `api_delete(MODEL)` before recording state (`scripts/pull-delete-smoke.py:93-100`) and deletes it again in `finally` (`210-213`).

**Why it matters.** Running `make smoke`, including with the default `qwen3:0.6b`, removes an already-installed model and never restores it. Re-pulling would not be a safe restoration because a tag can move and the original digest is not recorded.

**Suggested fix.** Query initial state before any mutation and abort if the target already exists, or run against an isolated Ollama model store. Cleanup only a model proven to have been created by this run. Correct the README claim and add a no-mutation precondition test using a fake API.

#### H-05 — Untrusted Ollama text can inject terminal control sequences `[needs runtime verification]`

**Evidence.** Remote chat tokens are appended verbatim at `internal/ui/agent_view.go:337-347`; `renderBlock` falls back to raw Markdown at `1155-1171`. Remote error bodies are rendered directly at `internal/ui/models_view.go:670-706`, and model identifiers are interpolated into styled terminal strings. JSON `\u001b` decodes to an ESC byte. No sanitization boundary was found before these strings reach rendering.

**Why it matters.** A malicious remote host/model can attempt OSC/CSI/DCS operations such as clipboard writes, screen clearing, title changes, or deceptive cursor movement. The raw fallback is a proven unsanitized path; whether Glamour’s successful-render path preserves every sequence should be verified at implementation time.

**Suggested fix.** Centralize sanitization for all remote-derived model names, details, errors, statuses, tool text, and chat text. Strip unsafe C0/C1 controls and terminal sequences while preserving intended newline/tab semantics; apply SelfTUI’s own styles only after sanitization. Add OSC 52, CSI, carriage-return, and split-sequence tests for both successful Markdown rendering and fallback.

#### H-06 — The audit package is incomplete (coverage blocker, not a proven repository defect)

**Evidence.** `FILE-INVENTORY.md:3` claims 97 tracked files and lines 13, 56, 77, 91-92 list `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `.gitignore`, and `.gitattributes`. None is present in the ZIP. The archive contains 93 of the 97 listed tracked files, plus `PROMPT.md` and `FILE-INVENTORY.md`; present listed files matched their recorded sizes.

**Why it matters.** Workflow triggers, permissions, action references, tag/version guards, release-note extraction, checkout depth, ignore rules, and golden-file line-ending policy cannot be independently checked. This does not prove the live repository lacks those files.

**Suggested fix.** Regenerate with `git archive` or an explicit `git ls-files` copy that preserves dotfiles. Make packaging fail when archive paths and the manifest differ in either direction.

### Medium

#### M-01 — Mutation approval has no real timeout, and Tab can hide it

**Evidence.** `internal/agent/runner.go:385-403` displays `Timeout: timeout` but selects only on the reply channel or `ctx.Done()`; there is no timer case. `internal/ui/agent_view.go:1542-1548` visibly says `timeout: 30s`. The root router handles Tab/Shift-Tab before consulting child modal state at `internal/ui/app.go:165-188`; only digit tab jumps check `ModalOpen()`.

**Why it matters.** An unanswered mutation can stall a turn indefinitely, while Tab hides the only approval surface. The UI claims a safety/liveness property that is not enforced.

**Suggested fix.** Add a stopped/drained `time.Timer`, auto-decline with a stable timeout result, and keep late replies harmless. Route all keys to an active child modal before global tab navigation. Test no-response expiry and Tab/Shift-Tab for every Models/Agent modal.

#### M-02 — Context eviction can orphan tool results and still exceed its limit

**Evidence.** `internal/agent/context.go:39-45` inserts a marker and removes one message at a time. A tool turn is stored as an assistant `tool_calls` message plus one or more `role:"tool"` messages (`internal/agent/runner.go:218-243`). `truncateLatest` at `internal/agent/context.go:48-61` truncates only the last message’s `Content`; it cannot reduce oversized `ToolCalls` or an oversized retained system prompt.

**Why it matters.** Budgeting can send an orphan tool result, misassociate a result, or exceed the advertised three-quarter context budget. Some Ollama/model combinations may reject or mishandle the invalid protocol sequence.

**Suggested fix.** Budget atomic exchanges: preserve assistant tool calls with all correlated results, never begin retained conversational history with `role:"tool"`, and explicitly handle an oversized system prompt/latest tool call. Enforce `ApproxTokens(out) <= limit` as a postcondition. Add low-`num_ctx`, multi-tool, giant-system, and giant-latest-call tests.

#### M-03 — Stale asynchronous model responses overwrite current state `[needs runtime verification]`

**Evidence.** `showCmd(name)` captures a name (`internal/ui/models_view.go:224-233`), but completion at lines 330-341 is applied without checking the current selection. Selection changes suppress a new request while `loadingShow` is true (`508-515`). `ApplyClient` starts a new load without invalidating an older host’s in-flight results (`1034-1046`). Messages contain no request generation.

**Why it matters.** Rapid A→B navigation can show A’s details under B, and a slow response from an old host can overwrite a newly configured host’s list. The same unversioned load shape exists for Agent models.

**Suggested fix.** Add monotonically increasing client/request generations, cancel obsolete contexts, ignore mismatched completions, and request the latest selection after an older inspection finishes. Test slow-old/fast-new host swaps and selection changes during an in-flight show.

#### M-04 — Transcript persistence blocks the Bubble Tea update loop `[needs runtime verification]`

**Evidence.** `internal/ui/agent_view.go:700-723` explicitly performs `session.Open`, `Append`, and error-path `Close` on the update loop; `/export` calls `Flush` synchronously at `739-753`. `internal/session/session.go:31-69` performs directory/file I/O and retry sleeps, while `Flush`/`Close` call `Sync` at `119-147`.

**Why it matters.** A slow/full/network-backed XDG directory or large pasted message can freeze input and rendering, delay stream consumption, and amplify channel backpressure. Normal shutdown also discards the final model (`cmd/self-tui/main.go:154-159`), so the session relies on process cleanup instead of an explicit close/error path.

**Suggested fix.** Use a serialized writer worker or ordered `tea.Cmd` protocol with immutable payloads and completion messages. Preserve user/assistant ordering, surface one stable error, and explicitly flush/close on normal shutdown.

#### M-05 — Wrapping measures display cells but slices bytes

**Evidence.** `internal/ui/models_view.go:953-979` first checks `lipgloss.Width(line)` but then loops on `len(line)`, searches `line[:width+1]`, and emits `line[:cut]`. Tests at `internal/ui/models_view_test.go:331-347` use ASCII only.

**Why it matters.** Long CJK, emoji, combining/ZWJ text, or ANSI-styled rows can be cut inside a UTF-8 sequence or control sequence, producing invalid text, style bleed, incorrect height, or frame overflow at the 72×30 target.

**Suggested fix.** Wrap by grapheme/display cells with ANSI token awareness, preferably using one Charm-aligned width/wrap primitive throughout. Test over-width CJK, emoji/ZWJ, combining marks, and styled lines; assert valid UTF-8 and `lipgloss.Width(row) <= width`.

#### M-06 — Filesystem work and producer sends cannot be canceled under load `[needs runtime verification]`

**Evidence.** `executeTool(ctx, ...)` receives context but invokes `ReadFile`, `ListDir`, and `Grep` without it (`internal/agent/runner.go:295-333`). `Grep` collects and sorts the whole path list before scanning (`internal/agent/tools.go:136-168`); its result cap does not bound no-match traversal. Agent and pull producers use unconditional sends into 64-slot channels (`internal/ui/agent_view.go:267-275`; `internal/ui/models_view.go:262-284`).

**Why it matters.** Esc or process cancellation can remain stuck in a large/network filesystem walk. If the UI stops consuming or is blocked by disk I/O, a full channel can strand the producer because cancellation cannot win the send.

**Suggested fix.** Thread context through tools, check it during traversal/scanning, stream instead of collecting all paths, and impose total file/byte/time budgets. Send with `select { case ch <- msg: case <-ctx.Done(): }`. Add >64-event saturation/quit tests with explicit producer-done channels and cancellation during a large grep.

#### M-07 — The in-flight delete state suppresses its own busy overlay

**Evidence.** Approval sets `confirmDelete=false` and `deleting=true` at `internal/ui/models_view.go:539-545`. `View` handles confirm, input, and pull states at `599-612`, but not `deleting`. The deleting-spinner branch exists only inside `confirmLines` at `754-767`, which is unreachable after approval. Tests assert state/command but do not render between approval and completion (`internal/ui/models_view_test.go:414-436`).

**Why it matters.** During a delete request the UI falls back to the ordinary list while ignoring keys, appearing frozen instead of showing the promised progress state.

**Suggested fix.** Render a delete overlay when `deleting` is true and test its title, spinner, target model, and bounded geometry before delivering completion.

#### M-08 — Redirects are not constrained by the HTTPS token policy `[needs runtime verification]`

**Evidence.** Configuration validates only the initial URL, while `internal/ollama/client.go:41-47` creates default `http.Client`s with no `CheckRedirect`. Authorization is attached before `Do` at `65-76`; streaming does the same at `internal/ollama/stream.go:48-64`.

**Why it matters.** The documented rule says a bearer token is never accepted over non-loopback plain HTTP, but a permitted HTTPS endpoint can redirect a request. The exact header forwarding behavior should be pinned against the project’s Go version, especially for same-host/subdomain and HTTPS→HTTP redirects.

**Suggested fix.** Reject redirects for API calls, or validate every destination and refuse/strip credentials unless it remains HTTPS and the intended same origin. Add downgrade, cross-host, and subdomain redirect tests.

#### M-09 — Pull progress can multiply spinner command chains `[needs runtime verification]`

**Evidence.** `internal/ui/models_view.go:290-291` wraps a fresh `v.spinner.Tick()` command. The update path discards the command returned by `v.spinner.Update(msg)` at `343-344`, then appends a new `spinnerTick()` after every message while pulling/deleting at `363-367`. Every pull-progress event therefore seeds another spinner sequence rather than maintaining one subscription.

**Why it matters.** A long pull with many progress events can accumulate timer/message chains and unnecessary UI wakeups. The precise cadence depends on the pinned Bubbles v2 implementation and should be observed at implementation time.

**Suggested fix.** Seed one spinner on state transition, then wrap/reschedule only the command returned from `spinner.Update`; progress handlers should resubscribe only to pull activity. Add a deterministic scheduler-count test.

#### M-10 — The release gate is not fully portable or cross-builder reproducible

**Evidence.** `scripts/release-check.sh:14-16` explicitly uses whatever `go`, `gofmt`, and `govulncheck` are on `PATH`; lines 62-71 check presence, not versions, while README claims gate pinning. Archive creation at `123-130` normalizes order, time, owner, and group but not member modes. Finally, lines 135-140 write `dist/selftui-...` into `SHA256SUMS`, although GitHub Release assets are downloaded flat; the ledger itself records needing to strip that prefix.

**Why it matters.** Different toolchains can yield different results, different umasks can change tar member modes/hashes, and end users cannot directly run `sha256sum -c SHA256SUMS` beside flat downloaded assets.

**Suggested fix.** Enforce or containerize the documented Go/govulncheck versions; stage via `install -m 0755/0644` or normalize tar modes; generate the manifest inside `dist` with flat names. Add a cross-umask reproducibility check and document flat-directory verification.

#### M-11 — Predictable `/tmp` captures risk disclosure and symlink-following overwrites

**Evidence.** `scripts/pull-delete-smoke.py:30,86-89,206-209` writes a full TUI capture to `/tmp/selftui-smoke.log`; `scripts/reconnect-smoke.py:41,163-167,271-274` does the same at `/tmp/selftui-reconnect.log` using ordinary `open(..., "w")`.

**Why it matters.** On a shared host another process can pre-place a symlink, causing a same-user file overwrite; normal umasks may also expose prompts, model output, paths, or errors.

**Suggested fix.** Use a unique 0700 temporary directory and an exclusive 0600/no-follow file. Retain the capture only on failure or when explicitly requested, and print its path.

#### M-12 — Security and release-state documentation overstate reality

**Evidence.** `SECURITY.md:45-47` describes `.env*` and `credentials*`, but `internal/agent/toolpolicy.go:41-48,74-83` intentionally permits `.env.example` and blocks only exact `credentials`/`credentials.json`; tests allow broader credential-like names. `SECURITY.md:48-49` broadly claims cumulative stream caps although tool-call arguments are excluded. `README.md:10` still says “v0.1 release hardening,” and `CHANGELOG.md:8,60-63` contains only `[Unreleased]` and says changes “will ship,” while `PLAN.md:591-605` says v0.1.0 was published.

**Why it matters.** Users may rely on protections that do not exist, and contributors cannot tell whether they are preparing or maintaining the release.

**Suggested fix.** After code fixes, state the exact denylist and `.env.example` carve-out, narrow stream claims to enforced fields, cut `[v0.1.0] - 2026-09-04`, update README status, and collapse PLAN §12 to one current next action.

### Low

#### L-01 — Light-theme “every frame” coverage renders the wrong scenarios

**Evidence.** `internal/ui/golden_test.go:457-494` iterates all `goldenFrames`, but its switch constructs only three Models cases, base Agent, and Settings. Names for turn states, palette, picker, slash/help/clear, and other frames hit no case and remain the default Models view; the final assertion checks only that the tab bar exists.

**Why it matters.** The test name and loop imply comprehensive light-theme state coverage while many rows repeatedly test the same screen.

**Suggested fix.** Extract scenario builders shared by dark and light tests and assert the intended active tab/modal before geometry checks.

#### L-02 — Cleanup is incomplete and a generated timer fixture appears tracked

**Evidence.** `Makefile:36-40,63-75` creates `bin/size-probe` and `dist/*`, but `clean` removes only `bin/selftui`. `internal/ui/timer.sh:1-2` is just a ten-second sleep, while the routing regression test creates that fixture in `t.TempDir()`.

**Why it matters.** Contributors can mistake a partial cleanup for a clean build tree, and the stray script adds provenance noise.

**Suggested fix.** Remove only explicit owned outputs (`bin/selftui`, `bin/size-probe`, `dist/`) and delete the tracked timer if no external purpose exists.

## 4. Optimization plan

### Quick wins (hours)

1. Fix `ConfigPath` construction in `cmd/self-tui/main.go` and add a default-XDG startup regression test (H-01).
2. Enforce approval expiry, block Tab/Shift-Tab while any child modal is active, and add the two negative tests (M-01).
3. Make `pull-delete-smoke.py` abort when the target pre-exists; move both smoke captures into private unique temp directories (H-04, M-11).
4. Add the missing `deleting` render branch and a between-approval-and-completion geometry test (M-07).
5. Generate flat checksum entries and normalize staged archive modes (part of M-10).

### Short term — this release cycle

| Work item | Effort × impact | Files/areas | Exit condition |
|---|---|---|---|
| Close the recursive policy gap and canonicalize workspace validation | M × L | `internal/agent/{runner,tools,toolpolicy}.go`, `internal/config/validate.go`, tests | Root grep cannot observe any denied descendant; aliases of `/`/home are rejected. |
| Bound the native tool protocol | M × L | `internal/ollama/chat.go`, `internal/agent/runner.go`, stream/runner tests | Raw cumulative bytes, calls/run, and args/call are capped; overflow is rejected before execution. |
| Establish one terminal-sanitization boundary | M × L | `internal/ui`, Ollama-to-UI adapters, terminal regression tests | OSC/CSI/C0/C1 payloads cannot reach terminal output; valid Unicode/Markdown remains intact. |
| Make context and cancellation protocol-safe | M × M | `internal/agent/context.go`, filesystem tools, UI producer channels | Tool call/results stay atomic; filesystem work and full-channel sends stop on cancellation. |
| Version asynchronous UI requests | M × M | Models/Agent loading and Settings-save messages | Old host/selection/save completions cannot overwrite newer state. |
| Replace byte slicing with cell-aware wrapping | S–M × M | `internal/ui/models_view.go`, shared render helper, golden/unit tests | Long Unicode and styled lines remain valid and bounded at 72×30 and 120×40. |
| Repair release/package evidence | M × M | audit packaging, release script, README/CHANGELOG/PLAN | Manifest-complete audit ZIP; cross-umask identical archives; flat checksum verification; current docs. |

At implementation time, run `make check`, `make race`, `make vuln`, `go test -race -count=1 ./...`, and `VERSION=v0.1.1 make release-check` on a clean worktree. The last command is destructive to `dist/` by design; run it only in the intended repository checkout.

### Medium term

- Build an adversarial protocol harness that fragments two or more simultaneous tool calls, floods >64 UI events, stalls before HTTP headers, redirects credentials, and cancels during a large filesystem walk. This would cover the state-machine gaps behind H-03, M-03, M-06, M-08, and M-09.
- Move transcript persistence behind a single ordered writer with explicit startup/shutdown lifecycle and fault injection. Measure key-to-render latency with blocked writes rather than relying only on content assertions.
- Make all golden scenarios data-driven across both themes and both canonical geometries; retain the existing byte-exact ANSI-stripped snapshots and raw width/height guards.
- For a stronger filesystem boundary, evaluate directory-FD-relative operations and a commit-time no-replace primitive. `securePath` currently validates a pathname before it is later opened/renamed, and `overwrite=false` is checked before `os.Rename`; a concurrent local filesystem race remains `[needs runtime verification]`. Prioritize this after the direct recursive/alias violations because it requires a realistic portability design.
- Add dependency-update automation and `go mod tidy -diff` to the maintenance gate after the absent workflows are restored to the audit input. Keep `govulncheck`; do not replace it with version scanning alone.

### Long term / out of scope

- **Do not add shell/command execution to fix tool limitations.** Its absence is a documented v0.1 security decision. Better bounded read/write tools solve the current defects without expanding authority.
- **Do not build resumable chat storage yet.** Append-only Markdown export and in-memory sessions are explicit v0.1 boundaries; first make the existing writer nonblocking and reliably closed.
- **Do not widen native-platform scope.** Linux/WSL plus SSH-from-phone is explicit. Native Windows/macOS work would dilute effort from security and correctness fixes.
- **Do not replace Bubble Tea or split the process solely because `App.Update` is large.** The single-owner update architecture is a recorded choice and generally sound; request generations, one spinner chain, and bounded commands address the concrete failures.
- A module-path migration from `module selftui` to the public repository path is useful before advertising versioned `go install`, but it should be a deliberate compatibility task rather than an opportunistic audit fix.

### Alignment with the existing queue

`PLAN.md:613-616` and the latest ledger already queue the v0.1.0 changelog cut, gitleaks-in-CI, actionlint in the local gate, Node action major updates, a signed-tag decision, and a future public-visibility decision. Fold M-10/M-12 and the regenerated audit package into that same v0.1.1/release-hardening tranche. Do not duplicate the gitleaks or actionlint work. Because the workflow files were absent, this review cannot confirm whether any queued workflow work has already landed or whether private vulnerability reporting is enabled; verify those directly before public visibility.

## 5. Appendix: assumptions, limits, and preserved strengths

### Review limitations

- This was a static review of the supplied text snapshot. No canonical checks, binaries, network services, or live terminal sessions were run.
- The four missing dotfiles/workflows prevented CI/release workflow review. No claim here implies that the live repository lacks them.
- Findings explicitly marked `[needs runtime verification]` have a concrete static risk path but require an implementation-time reproducer or pinned-dependency behavior check before claiming observed impact.
- The snapshot is labeled 2026-09-04, while PLAN/LEDGER include later-dated completed entries and non-monotonic chronology. Treat those histories as decision context, not independent provenance proof.
- Dependency versions are exactly recorded in `go.mod`/`go.sum`, and the documents say `govulncheck` was green; that result was not independently rerun.

### Controls worth preserving

- Tools are genuinely off by default, and the disabled runner neither advertises nor executes tool calls.
- The dispatch set is closed; `run_command` is intentionally absent and should not be reported as an incomplete feature.
- Common `..` and static symlink escapes outside a legitimate workspace are blocked; mutations require explicit approval and use same-directory temp-file/rename commits.
- Finite API responses, stream events, chat content/thinking, and read/tool results have useful bounds; stream body reads have an idle watchdog.
- Config and transcript creation use private permissions; tokens are not intentionally logged.
- The test suite has substantial byte-exact geometry coverage at 72×30 and 120×40, a >3 KiB approval-payload frame, focused routing tests, and HTTP cancellation tests.
- Non-resumable transcripts, Linux/WSL support, lack of a total pull deadline while progress continues, and absence of command execution are documented design choices, not findings.
- The ledger records in-workspace symlink aliases to forbidden files as an accepted residual. This report does not relabel that accepted choice; C-01 is a different defect because an ordinary recursive root request reads lexical denied descendants without naming or aliasing them.

## Top 5 — do these first

1. Fix default config loading and add a save→restart regression test (H-01).
2. Stop recursive `grep` from opening any policy-denied descendant (C-01).
3. Canonicalize workspace-root validation before comparing against `/` and home (H-02).
4. Cap cumulative native tool bytes, argument size, and total calls per run (H-03).
5. Make the smoke test refuse pre-existing models and clean up only what it created (H-04).
