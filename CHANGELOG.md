# Changelog

All notable changes to SelfTUI are recorded here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
for tagged releases.

## [Unreleased]

### Changed

- Go module renamed from `selftui` to `github.com/MerverliPy/SelfTUI`
  (audit finding F-03): `go.mod` module line and all internal import paths
  updated, so `go install github.com/MerverliPy/SelfTUI/cmd/self-tui@latest`
  resolves once the next `v*` tag is cut. Runtime data paths
  (`~/.config/selftui/`, `$XDG_STATE_HOME/selftui/`) and the binary name are
  unchanged.

### Security

- Dependency hygiene: bumped `golang.org/x/net` v0.39.0 → v0.58.0 (and
  transitive `golang.org/x/text` → v0.41.0), clearing the full advisory set
  govulncheck reported against the old pin (GO-2026-5025…5030, 4440/4441,
  4918, 5942 — all in `x/net`, all unreachable from SelfTUI code in v0.2.0).
  `govulncheck ./...` now reports no vulnerabilities.

## [0.2.0] - 2026-09-07

### Added

- **V2d agent breadth — workspace context** (2026-09-07): with workspace tools
  enabled, every agent turn starts with a bounded (`8 KiB`) context system
  message: git branch, porcelain status, and the last 3 commits (fixed
  read-only `git` argv, host-side, `3s` timeout, gracefully omitted outside a
  git repo) plus a depth-capped (`4`) and entry-capped (`300`) project index
  with `.git` pruned. Plain chat never receives the block; the tool schema is
  unchanged.

- **V2c sandboxed `run_command`** (2026-09-07): workspace tools can now run
  an explicitly approved, argv-allowlisted `go` or read-only `git` command
  inside bubblewrap with no network, a scrubbed environment, bounded timeout
  and output, serialized execution, and process-group cancellation. The
  default engine fails closed when bubblewrap is unavailable; residual
  bubblewrap memory/CPU risk is documented in
  `docs/run-command-containment.md`.

### Security

- **Embedded tool JSON must be tool-framed.** Content-embedded tool calls are
  honored only when the model explicitly wraps them in a `{"tool_calls":[...]}`
  envelope (fenced or bare). Bare `{"name":...}` objects, `function` wrappers,
  and top-level call arrays are now rendered as prose instead of executing.
- **Removed the `-auth-token` flag.** A command-line secret appears in process
  listings and shell history; the flag was already documented as
  compatibility-only. Tokens remain supported via `SELFTUI_AUTH_TOKEN`, the
  0600 config file, and the Settings → Connection form.

### Changed

- Runbook `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md` closed: Tasks 17–22
  checklist entries ticked and a completion addendum appended (all 22 findings
  remediated, v0.1.1 gate green, release published 2026-09-07).

- **v0.1.1 audit-remediation hardening** (2026-09-05/06 on
  `fix/v0.1.1-audit-remediation`; findings in
  `SelfTUI-External-Audit-2026-09-04.md`, task blocks in
  `SelfTUI-Pi-Audit-Remediation-Runbook-2026-09-04.md`):
  - **Host and config trust.** Default XDG config loading is restored (H-01);
    workspace roots are validated canonically — `/` and the home directory are
    rejected as roots and recursive grep cannot observe a denied descendant
    (H-02).
  - **Bounded tools and streams.** A single raw NDJSON event over 4 MiB, or
    cumulative raw chat bytes over 16 MiB — counting JSON framing, content,
    thinking, and tool-call bytes — aborts the stream; a decoded tool-call
    argument over 1 MiB or a run-wide total of 64 tool calls is refused before
    execution (H-03). HTTP redirects are refused so a bearer token is never
    forwarded to another origin or downgraded to plain `http://` (M-08).
  - **Terminal and interaction safety.** One sanitization boundary strips
    terminal control sequences (OSC/CSI/C0/C1) from every remote-derived
    render path (H-05); tool approvals expire and modals keep keyboard focus
    (M-01); context trimming preserves atomic tool exchanges (M-02); stale
    model/host completions are ignored (M-03); cancellation propagates through
    tools and stream producers (M-06).
  - **Smoke and audit hygiene.** The live smoke test is non-destructive — it
    captures the host's state first and aborts when the target model already
    exists (H-04) — and writes its evidence to private unique `0700`/`0600`
    temp paths (M-11); audit packages are manifest-complete and verified
    (H-06).

### Fixed

- **Correctness cluster** (2026-09-06): context truncation stays on UTF-8
  rune boundaries; the Ollama client is rebuilt only when host/token change;
  `/export` echoes the real recorder failure instead of dead-end advice; an
  unsaved theme preview rolls back when the config write fails; stale model
  detail is dropped after a list reload; recorder shutdown is bounded so a
  wedged sink cannot hang exit; a missing explicit `-config` path hard-errors;
  config parse errors name the real `AGENT_` environment variable; a failed
  stream's error body is read under the idle watchdog rather than hanging the
  producer.
- Model deletes render an in-flight progress overlay (M-07) and each
  operation keeps exactly one spinner command chain (M-09).
- Transcript persistence moved off the UI update loop (M-04); long lines wrap
  by terminal display cell instead of by byte (M-05).

### Changed

- **Reproducible release tooling** (M-10): the release gate and audit pack
  produce byte-identical artifacts under any umask — fixed archive member
  modes and flat `SHA256SUMS` entries — and `go.mod` pins the Go 1.27.1
  toolchain to match CI.

## [v0.1.0] - 2026-09-04

### Changed

- **Product contract aligned for v0.1** (`hardening/v0.1`, 2026-09-04): public
  docs now state that v0.1 is a single-process **Linux/WSL** TUI for Ollama,
  that **chat sessions are in-memory** with the Markdown transcript export
  surviving exit but **not resumable**, that **workspace tools are disabled by
  default** and require an explicit workspace, that **command execution is not
  shipped**, that **non-loopback tokens require HTTPS**, and that native
  **Windows and macOS are not supported** in v0.1. Stale planning-era claims
  were removed or explicitly labeled historical.
- `-version` no longer reports a hard-coded pre-release build tag: `Version`
  is now a `var` defaulting to `dev` (builds stamp their own value), with a
  test pinning the `selftui <value>` output format.
- The Agent slash command that flushes the transcript was renamed to
  **`/export`**: it flushes and reports the Markdown transcript path and
  never claims the conversation can be resumed. Menus, help, hints, tests,
  and golden fixtures updated. Session files remain append-only Markdown
  exports.
- Below **40 columns × 12 rows** SelfTUI now renders a deterministic bounded
  "terminal too small" message (current dimensions + `40x12` minimum) instead
  of the shell; table-driven geometry tests cover the boundary values and
  Unicode content.

### Added

- **Chat session resume (v0.2 V2a, 2026-09-07).** The `/resume` slash command
  opens a picker over the saved per-process transcripts under
  `$XDG_STATE_HOME/selftui/sessions/` (newest first, humanized mtime + size)
  and reloads the chosen one into the live Agent conversation: committed
  user/assistant turns re-enter as plain history with their historical model
  chips and `elapsed · reason` meta, the render cache and context meter
  rebuild, and auto-follow re-engages. Safe import: transcripts record only
  committed turns (never tool-call state), the truncation flag recomputes at
  import time (an over-budget transcript shows its truncation marker
  immediately), sends are refused while a transcript load is in flight (a
  mid-load message can never be wiped by the import), and the next send
  keeps the currently selected model (picker rows never switch it). The
  parser matches header lines under the writer's exact grammar (body
  headings like `## user story` stay content), preserves leading blank
  lines of recorded content verbatim, and list ordering is deterministic
  even when two files share an mtime; imported model/meta strings and the
  resume notice are sanitized like any other display text. Resuming over a
  live conversation asks first (y/esc, same guard as `/clear`); listing and
  parse failures surface one notice and never destroy the conversation; the
  new run's transcript stays append-only for post-resume turns.
- `LICENSE` (Apache-2.0), `SECURITY.md` (private vulnerability reporting via
  GitHub's Security tab), `CONTRIBUTING.md`, and this changelog with an
  Unreleased section.
- **Reproducible CI + release gates (v0.1 hardening phase 8, 2026-09-04).**
  New Makefile targets: `race` (full suite under the race detector), `vuln`
  (`govulncheck ./...`), `build-linux-amd64`/`build-linux-arm64` (CGO-disabled
  static Linux release binaries stamped with `-X main.Version=$(VERSION)`),
  and `release-check` (runs `scripts/release-check.sh`). The gate script
  requires a clean worktree and `VERSION=v<major>.<minor>.<patch>`, then runs
  `go mod verify`, the gofmt check, `go vet`, uncached tests, race tests,
  `govulncheck`, both Linux builds, per-binary version-stamp verification,
  and writes deterministic release archives plus `dist/SHA256SUMS` under the
  gitignored `dist/`; it never creates or pushes a git tag. GitHub Actions
  workflows were added: `ci.yml` (every pull request + push to `main`) and
  `release.yml` (`v*` tag pushes — complete release gate, tag-vs-binary-
  version verification, archive + `SHA256SUMS` upload, release notes from
  `CHANGELOG.md`). Both pin **Go 1.27.1** (current official stable release,
  2026-09-04) and govulncheck **v1.7.0**, and use only the default
  least-privilege `GITHUB_TOKEN` — no secrets. The first `make vuln` run
  surfaced two reachable advisories in indirect dependencies — goldmark
  (GO-2026-5320, XSS in the markdown render path) and x/text (GO-2026-5970,
  infinite loop) — fixed by bumping to goldmark v1.7.17 and x/text v0.39.0.

### Security

- Earlier v0.1 hardening removed command execution, made workspace tools
  opt-in with a real workspace root, validated/atomically saved config,
  bounded Ollama streams, and envelope-routed async UI events; those changes
  are documented in `LEDGER.md` and shipped in the v0.1.0 release
  (2026-09-04).
