# Changelog

All notable changes to SelfTUI are recorded here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
for tagged releases.

## [Unreleased]

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

- `LICENSE` (Apache-2.0), `SECURITY.md` (private vulnerability reporting via
  GitHub's Security tab), `CONTRIBUTING.md`, and this changelog with an
  Unreleased section.

### Security

- Earlier v0.1 hardening already removed command execution, made workspace
  tools opt-in with a real workspace root, validated/atomically saved config,
  bounded Ollama streams, and envelope-routed async UI events; those changes
  are documented in `LEDGER.md` and will ship in the v0.1.0 release notes.
