# `run_command` containment (V2c)

**Status: shipped in v0.2 V2c (2026-09-07).** `run_command` is available only
when workspace tools are explicitly enabled. Its default engine is
[bubblewrap](https://github.com/containers/bubblewrap) (`bwrap`); if bwrap is
unavailable, the command fails closed. This is a Linux/WSL feature and is not a
shell escape hatch.

## Threat model and boundary

A cwd jail and argv filtering are not an OS sandbox. V2c adds a real OS
boundary before the existing executor guardrails:

- bwrap mounts `/usr` and `/lib` read-only, a selected Go toolchain and module
  cache read-only, and the configured workspace at `/workspace` read-write;
- `/home` is never mounted, `/tmp` is a private tmpfs, `/proc` and `/dev` are
  sandbox instances, and networking is disabled with `--unshare-net`;
- `--clearenv` is followed by a fixed environment allowlist. Host tokens,
  `OLLAMA_*`, `SSH_*`, cloud credentials, and arbitrary user environment values
  are not passed to the child;
- `--die-with-parent --new-session` plus the executor's process-group kill
  prevents a canceled or timed-out command from surviving its turn.

The workspace remains the only writable host mount. Commands cannot use a
shell or interpreter: the model supplies a fixed argv and the executor accepts
only the allowlisted `go` and read-only `git` subcommands.

## Command contract

`run_command` accepts:

```json
{"argv":["go","test","-count=1","./..."],"timeout":30}
```

The command is rejected when:

- argv is empty, exceeds 32 elements, or an element exceeds 4096 bytes;
- an element contains NUL/newline or shell metacharacters, an absolute path, or
  a path traversal (including option values such as `-modfile=../x`);
- the executable is not `go` or `git`;
- `go` is not followed by `test`, `vet`, `build`, `list`, `env`, or `version`;
- `git` is not followed by `status`, `diff`, `log`, `show`, `branch`,
  `rev-parse`, `ls-files`, or `grep`;
- dangerous Go execution flags or Git config/work-tree/directory overrides are
  present.

Every request reaches the normal in-TUI approval modal before execution. The
modal shows the tool name, canonical workspace, timeout, and raw argv JSON;
approval defaults to no and expires after the same bounded confirmation window.

## Hard limits

| Limit | Default / maximum | Enforcement |
|---|---:|---|
| command timeout | 30 s / 60 s | context deadline, SIGTERM, then group SIGKILL after 2 s |
| stdout | 256 KiB | bounded streaming writer; truncation marker |
| stderr | 256 KiB | bounded streaming writer; truncation marker |
| command concurrency | 1 | cancellation-aware single-flight slot |
| agent tool calls | 64 per run | runner batch gate |
| agent iterations | 12 by default | runner loop bound |

Output is streamed as sanitized `ToolOutputMsg` activity and returned to the
model as one bounded `ToolResultMsg`. A full result combines stdout and stderr
and is capped again by the agent result bound.

## Residual risk and alternatives

Bubblewrap does not provide CPU or memory limits. A hostile allowlisted Go test
can consume host memory until the timeout fires, creating possible host-wide
OOM pressure on WSL2. The 30-second default / 60-second maximum timeout,
output caps, one-command serialization, process-group cancellation, and
restricted argv are mandatory compensating controls and are covered by V2c
tests.

V2b also validated rootless Docker with `--network none`, a read-only root,
capability dropping, `no-new-privileges`, and an enforced memory limit. It
remains the stronger, slower alternative for a future selectable engine; the
current V2c binary deliberately uses bwrap as its default and only engine.

## Verification expectations

V2c tests cover argv rejection, environment scrubbing, output caps, explicit
confirmation, cancellation/group kill, and an offline `go test` workload
inside bwrap. The V2b gate evidence remains at
`docs/v2b-sandbox-gate-evidence.md`; it records the host-specific bwrap and
rootless-Docker probe outputs and the accepted WSL2 memory-limit residual.
