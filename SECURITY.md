# Security Policy

## Supported scope

SelfTUI **v0.1.x** is a single-process Linux/WSL TUI for Ollama — **v0.1.0
released 2026-09-04**, with **v0.1.1 hardening in progress** (see
`CHANGELOG.md`). The security-relevant
boundaries of that release:

- chat sessions are **in-memory**; the Markdown transcript export is an
  append-only file that survives exit but **cannot be resumed**;
- workspace tools are **disabled by default** and require an explicitly
  configured project workspace root (`/` and the home directory are rejected
  as roots);
- v0.1.x has no command execution; v0.2 V2c adds only the sandboxed,
  allowlisted `run_command` tool when workspace tools are explicitly enabled;
- a bearer token for a **non-loopback host requires `https://`**;
- native Windows and macOS are **not supported** in v0.1.x.

## Reporting a vulnerability

Please do **not** open a public issue for a security vulnerability.

Report it through **GitHub's private vulnerability reporting** feature:

1. Go to the repository's **Security** tab:
   <https://github.com/MerverliPy/SelfTUI/security>
2. Click **Report a vulnerability** (or **New advisory**) and fill in the
   details — what the issue is, how to reproduce it, and what impact you
   believe it has.

We do not operate a public security contact email address, so please use the
GitHub feature above rather than trying to reach us off-platform. If the
repository is not yet public and GitHub's private vulnerability reporting is
therefore unavailable, file an issue on the private repository titled
`[SECURITY] …` — only repository collaborators can see it.

Reports are acknowledged and triaged as soon as possible; please include as
much reproduction detail as you can (terminal geometry/OS, Ollama host
configuration, the input that triggered it, and any log excerpt from
`$XDG_STATE_HOME/selftui/log.txt`).

## Security-relevant behaviors worth knowing

- Config files are written `0600` (directory `0700`); auth tokens are never
  logged and never accepted over plain `http://` to a non-loopback host.
- Transcript files under the XDG state dir are written `0600`.
- Workspace tool access is jailed to the resolved workspace root and gated by
  a lexical sensitive-path policy. A requested path is refused when any of
  its components is a sensitive dot-directory — `.ssh`, `.gnupg`, `.aws`,
  `.azure`, `.kube`, or the adjacent `.config`/`gcloud` pair — or when its
  final component is a credential file: the dotenv family (`.env` and any
  `.env.*`, e.g. `.env.local` or `.env.production`) or exactly `credentials`
  or `credentials.json`. The carve-out is exact too: `.env.example`, the
  dotenv template, stays readable and writable, and a name merely prefixed
  with `credentials` (e.g. `credentials.json.backup`) is not in the
  denylist.
- Ollama streams are bounded. Every NDJSON stream (pull and chat) aborts when
  a single event exceeds 4 MiB of raw JSON or when the body delivers no bytes
  for the 90s idle window — there is no total request deadline, so a long
  generation or download with steady deltas keeps running. Chat additionally
  caps cumulative raw NDJSON bytes at 16 MiB per request, counting JSON
  framing, content, thinking, and tool calls, so tool arguments cannot slip
  past the cap. Pull (model downloads) deliberately has no cumulative cap;
  its per-event cap and idle watchdog still apply. Non-stream responses are
  read capped at 64 MiB and ordinary requests time out at 30 s; error bodies
  from failed streams are read under the same idle watchdog.
- No HTTP redirect is ever followed (both the finite and the streaming
  client refuse), so a bearer token cannot be forwarded to a different
  origin or downgraded to plain `http://` by a redirecting Ollama host.
- Agent runs are budgeted: one decoded tool-call argument may not exceed
  1 MiB, a run executes at most 64 tool calls across its iterations, and the
  model loop runs at most 12 iterations by default (raise the iteration cap
  with `-max-tool-iterations` / `SELFTUI_AGENT_MAX_TOOL_ITERATIONS`).
- V2c `run_command` uses bubblewrap by default: `/home` is not mounted,
  networking is disabled, the workspace is the only writable host mount,
  and the child receives a fixed scrubbed environment. Only `go` (test, vet,
  build, list, env, version) and read-only `git` subcommands are accepted;
  shell/interpreter argv, path escapes, and dangerous config/execution flags
  are refused. Each call requires approval and is bounded to 30 seconds by
  default / 60 seconds maximum, 256 KiB per output stream, one concurrent
  command, and process-group termination on cancel/timeout. Bubblewrap has no
  CPU or memory cap, so a hostile allowlisted Go workload can still create
  host-wide OOM pressure until the timeout; see
  `docs/run-command-containment.md` for the residual-risk analysis.
