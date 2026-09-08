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

## Git option policy (deny-by-default)

"Read-only" is an intent, not a git guarantee, so the validator enforces a
per-subcommand option allowlist for the eight git subcommands instead of
passing options through. Each subcommand accepts only the exact option
spellings listed for it in `internal/agent/command.go`; everything else is
rejected before the sandbox runs:

- **write sinks are closed:** `--output[=<file>]` — honored by the diff
  machinery shared by `diff`, `log`, and `show`, which writes the patch to a
  file under `/workspace` — is never allowed. Because git accepts any unique
  prefix of a long option, abbreviations such as `--out=...` are rejected the
  same way: the allowlist matches exact spellings only;
- **external-helper execution is closed, including git's default-on
  drivers:** `--ext-diff` and `--textconv` (which make git run helpers defined
  in repository config/attributes) are rejected — and because git enables
  textconv filters (by default for `diff` and `log`) and external diff
  drivers (for `diff`) without any flag when a repository's `.gitattributes`
  and `.git/config` define a driver, `run_command` additionally **forces
  `--no-textconv --no-ext-diff`** right after the subcommand for the whole
  diff family (`diff`, `log`, `show`). An approved read-only argv therefore
  executes no repository-configured helper even when the argv itself names no
  helper option (git-diff(1): "textconv filters are enabled by default only
  for git-diff and git-log");
- **config and work-tree isolation is retained:** `-c`, `--config-env`,
  `--git-dir`, `--work-tree`, and their `=`-attached spellings are rejected
  wherever they appear (git only honors them before the subcommand, and the
  subcommand slot itself is allowlisted); the sandbox additionally scrubs
  config with `GIT_CONFIG_GLOBAL=/dev/null` and `GIT_CONFIG_SYSTEM=/dev/null`;
- operands (revisions such as `HEAD~1`, pathspecs such as `HEAD:README.md` or
  `internal/`, and values of listed value options) still pass as plain
  operands, so the ordinary read-only diagnostics keep working: `git status
  --short`, `git log --oneline -n 5`, `git diff --stat HEAD~1 HEAD`, `git
  rev-parse HEAD`, `git ls-files`, path-scoped `git grep`, and `git show
  HEAD:README.md` are all accepted.

To request a new option, add its exact spelling to the subcommand's list in
`internal/agent/command.go` (`gitOptionFlags` for options without a value,
`gitOptionValues` for options that take one) and cover it in the validator
tests in `internal/agent/command_test.go`. The option must not open a write or
external-helper-execution sink, and abbreviated spellings are never inferred.

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

**Command effects are never journaled.** `run_command` is outside the undo
journal (`/undo` / `/redo` in the Agent tab): a sandboxed command can touch
anything the allowlist permits, so its file effects are not tracked and
cannot be reverted by the journal. This is the same documented-gap pattern
Claude Code uses for bash mutations. The journal's refuse-guards still
protect the files it *does* track — if an approved command changes a file a
later mutation recorded, `/undo` of that mutation refuses loudly instead of
clobbering the command's result (the refusal names the file and suggests an
approved `run_command` or the user's own edit as the cause).

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
