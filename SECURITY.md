# Security Policy

## Supported scope

SelfTUI v0.1 is a single-process Linux/WSL TUI for Ollama. The security-relevant
boundaries of that release:

- chat sessions are **in-memory**; the Markdown transcript export is an
  append-only file that survives exit but **cannot be resumed**;
- workspace tools are **disabled by default** and require an explicitly
  configured project workspace root;
- **command execution is not shipped** (no shell, no interpreters, no
  subprocess tools);
- a bearer token for a **non-loopback host requires `https://`**;
- native Windows and macOS are **not supported** in v0.1.

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
  a sensitive-path denylist (`.ssh`, `.gnupg`, `.aws`, `.azure`, `.kube`,
  `.config/gcloud`, `.env*`, `credentials*`).
- Ollama streams are bounded (per-event and cumulative caps, idle timeout);
  pull/chat bodies cannot grow without limit.
- See `docs/run-command-containment.md` for the dated record of why command
  execution was deferred rather than shipped.
