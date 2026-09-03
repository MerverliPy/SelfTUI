# run_command containment design (M0a Spike 2)

**Status:** design — gate evidence. Implementation ships inline with M3b (per
`COUNCIL-MEMO.md` finding C: safety controls land with the tool that exposes them).
**Drives the M0a go/no-go item "command containment".**

## Threat model (what containment must assume)

This is a **guardrail, not a sandbox** (council finding A). A cwd jail + timeout alone
do not stop a command from:

- reading credentials (`~/.ssh/*`, env secrets, config files outside the jail);
- hitting the network (exfil/SSRF against the Ollama host or LAN);
- writing absolute paths outside the workspace;
- spawning children that outlive the parent (daemonization);
- escaping via a shell or interpreter (sh, python, perl, node …).

Therefore **v1 `run_command` is a constrained executor, not a shell**:

> **argv allowlist · no shell or interpreter · scrubbed env · resource + output
> limits · process-group kill · per-call confirmation · cwd jail · timeout ·
> cancellation.** General shell stays disabled by default and is out of scope for
> v1 (a real OS/container sandbox would be required first).

## Design

### 1. Command model (what the model may request)

`run_command` takes a **fixed argv** from the agent (not a command string), e.g.
`["go", "test", "./..."]`. The runner:

1. **Rejects** argv containing any of: `sh`, `bash`, `zsh`, `dash`, `python*`,
   `perl`, `ruby`, `node*`, `deno`, `env`, `sudo`, `su`, `nohup`, `setsid`, or any
   argv element with shell metacharacters — at the *element* level (see below), so
   `run_command ["sh", "-c", "…"]` and `run_command ["echo", "x; rm -rf /"]` both
   fail with a readable reason. This is the "no shell/interpreter" rule.
2. **Allowlists** the executable: `internal/agent/commands.go` declares the v1 set
   (e.g. `go`, `git` (read-only subcommands first), `rg`/grep-equivalent, `make`
   only with an explicit target allowlist expansion later). Anything else →
   `ErrNotAllowed` surfaced to the model *and* in the confirmation prompt.
3. **Scrubs the environment**: passes only a fixed allowlist (e.g.
   `PATH`, `HOME`→jail, `TMPDIR`→jail, `GOCACHE`→jail, `GOPATH`→jail, `TERM`,
   `LANG`). Everything else (tokens, `OLLAMA_*`, `SSH_*`, `AWS_*`…) is stripped.
   Variables whose secrets the tool needs (Ollama auth) stay in the agent process,
   never the child.

### 2. Limits (hard, enforced by the runner, not the model)

| Limit | Default | Mechanism |
|-------|---------|-----------|
| timeout | 30 s (configurable, capped) | `context.WithTimeout` + SIGKILL after grace |
| output cap | 256 KB stdout + 256 KB stderr | bounded buffers; truncate with a marker line |
| concurrency | 1 (serialized with other Ollama jobs) | single-flight mutex (plan risk #5) |
| iterations | max tool loop 12 (global) | runner state machine |
| cwd | `workspace_root` (jail) | resolved symlinks, `..` escape rejected before exec |

### 3. Lifecycle (process-group kill + cancellation)

- The child starts in a **new process group** (`Setpgid`), so kill can target the
  whole tree (`kill(-pid)`) — a child cannot orphan grandchildren.
- **Timeout** → `SIGTERM` to the group, 2 s grace, then `SIGKILL`.
- **User cancel** (Esc / ctrl+c in the confirm UI, or app quit) uses the same
  group-kill path via the shared root context (`tea.WithContext`).
- **Streaming**: stdout/stderr are read incrementally into the bounded buffers and
  emitted as `ToolResultMsg` chunks + a live status line; nothing waits for EOF
  before the UI reacts.

### 4. Confirmation (per-call, irreversible-operations-only)

- **Read-only commands** (the M3a allowlist) run with a start-line notice but no
  prompt.
- **Mutation commands** (M3b allowlist: e.g. `go test -race` is read-only; applying
  a formatter or installer is not) require an explicit in-TUI confirm showing the
  resolved argv, the jail root, and the limits. Default = **no**; a
  `confirm-everything` toggle exists but has a distinct visual state.
- The confirmation UI and the allowlist **ship in the same change as the tool**
  (M3b), never later.

### 5. Test plan (lands with M3b)

1. argv rejection table: shell/interpreter binaries, metachars, absolute paths, `..`
   escapes, empty/oversized argv — table-driven unit tests.
2. env scrub: child sees only the allowlist (spawn `env` via the allowlist in tests).
3. output cap: flood 1 MB → exactly cap + truncation marker (can be ~256 KB).
4. timeout & group-kill: start `sleep 100` via the allowlist? (sleep isn't allowed —
   use `go test` with a slow test or a test double) → TIMEOUT path kills the group;
   assert no orphan process remains (`pgrep` in test).
5. cancellation: cancel mid-run → same group-kill evidence.
6. serialization: second command while first runs → queues, not interleave.
7. confirmation: refusals by default; ack only after explicit confirm (UI test).

## Open owner decisions

- Exact v1 allowlist breadth (which build/test/toolchain binaries) — propose
  `go`, `git`, `rg`/pure-Go grep, `make`; owner trims.
- Whether `git` non-read-only subcommands (commit/push) enter v1 at all, or wait.
- Config-file override scope for limits (per-host trust levels for the remote
  Ollama case).

## Gate bearing

Design satisfies the gate's "command containment" item: a constrained executor with
no shell/interpreter, scrub/limits/kill/cancel/confirm, each control inline at M3b.
**Verdict-eligible.** Anything looser (general shell) is explicitly out of v1.