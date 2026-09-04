# run_command containment design — DEFERRED (not shipped in v0.1)

**Status: historical deferred-design record (2026-09-03, v0.1 hardening) —
retained for reference; not current product behavior.**

The executor described on this page is **not shipped in v0.1**. Per the public
v0.1 security decision, SelfTUI exposes no command execution: `run_command` was
removed from the tool schema, its dispatch case and executor were deleted from
`internal/agent`, and the command-output UI path was dropped. The v0.1 tool
surface is project-aware `read_file`/`list_dir`/`grep` plus confirmed
`write_file`/`edit_file` only.

**Why it is deferred:** a workspace `cwd` jail plus argv filtering is **not an
OS sandbox**. A constrained executor still cannot stop a command from reading
credentials, hitting the network, writing absolute paths, spawning children
that outlive it, or escaping through its own bugs. Reintroducing command
execution in a future release requires a real OS/container sandbox first.

This page preserves the design that was built and evaluated during M0a–M3b, as
the reference for that future work. It is a record, not a current capability.

---

## Threat model (what containment must assume)

This is a **guardrail, not a sandbox** (council finding A). A cwd jail + timeout alone
do not stop a command from:

- reading credentials (`~/.ssh/*`, env secrets, config files outside the jail);
- hitting the network (exfil/SSRF against the Ollama host or LAN);
- writing absolute paths outside the workspace;
- spawning children that outlive the parent (daemonization);
- escaping via a shell or interpreter (sh, python, perl, node …).

Therefore the (deferred) `run_command` was designed as a **constrained executor,
not a shell**:

> **argv allowlist · no shell or interpreter · scrubbed env · resource + output
> limits · process-group kill · per-call confirmation · cwd jail · timeout ·
> cancellation.** General shell stays disabled by default and is out of scope
> (a real OS/container sandbox would be required first).

## Design (recorded for future reference)

### 1. Command model (what the model may request)

`run_command` takes a **fixed argv** from the agent (not a command string), e.g.
`["go", "test", "./..."]`. The runner:

1. **Rejects** argv containing shells/interpreters or shell metacharacters at the
   *element* level.
2. **Allowlists** the executable: `internal/agent/command.go` permitted only `go`
   (`test`, `vet`, `build`, `list`, `env`, `version`) and read-only `git`
   (`status`, `diff`, `log`, `show`, `branch`, `rev-parse`, `ls-files`, `grep`).
3. **Scrubs the environment**: a fixed allowlist only (`PATH`,
   `HOME`/`TMPDIR`/`GOCACHE`/`GOPATH` jailed under the workspace, `TERM`, `LANG`);
   tokens and `OLLAMA_*`/`SSH_*`/`AWS_*` secrets never reach the child.

### 2. Limits (hard, enforced by the runner, not the model)

| Limit | Default | Mechanism |
|-------|---------|-----------|
| timeout | 30 s (configurable, capped at 60 s) | `context.WithTimeout` + SIGKILL after grace |
| output cap | 256 KB stdout + 256 KB stderr | bounded buffers; truncate with a marker line |
| concurrency | 1 (serialized with other Ollama jobs) | single-flight mutex |
| iterations | max tool loop 12 (global) | runner state machine |
| cwd | `workspace_root` (jail) | resolved symlinks, `..` escape rejected before exec |

### 3. Lifecycle (process-group kill + cancellation)

- Child starts in a **new process group** (`Setpgid`), so kill targets the whole
  tree (`kill(-pid)`).
- **Timeout** → `SIGTERM` to the group, 2 s grace, then `SIGKILL`.
- **User cancel** uses the same group-kill path via the shared root context.
- **Streaming**: stdout/stderr read incrementally into bounded buffers and
  emitted to the UI (the removed `ToolOutputMsg` path).

### 4. Confirmation (per-call)

Every `run_command` call required an explicit in-TUI confirmation showing the
resolved argv, jail root, and timeout; default = **no**. Confirmation stays for
the mutation tools that did ship (`write_file`, `edit_file`).

## Resolved owner decisions (historic)

- v1 (as designed) permitted the bounded `go` and read-only `git` subcommand sets;
  `make`, `rg`, and all git mutations waited for a later decision.
- Limits were fixed (30 s default, 60 s maximum; 256 KiB per stream) rather than
  config-overridable trust levels.

## Superseded by the v0.1 security decision

**2026-09-03:** the owner decided public v0.1 must not expose or retain
`run_command`; the code was removed rather than shipped behind the guardrails
above, because the guardrails are not an OS sandbox. Removal commit message:
`security: remove command execution from v0.1`.
