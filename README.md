# SelfTUI

[![ci](https://github.com/MerverliPy/SelfTUI/actions/workflows/ci.yml/badge.svg)](https://github.com/MerverliPy/SelfTUI/actions/workflows/ci.yml) [![release](https://img.shields.io/github/v/release/MerverliPy/SelfTUI)](https://github.com/MerverliPy/SelfTUI/releases) [![go](https://img.shields.io/github/go-mod/go-version/MerverliPy/SelfTUI)](go.mod) [![license](https://img.shields.io/github/license/MerverliPy/SelfTUI)](LICENSE)

> One binary. PC + phone. Manage Ollama models and chat with an embedded AI coding agent — natively on Linux or over SSH from your iPhone.

A visually appealing, responsive terminal UI for managing a local or remote
Ollama host and chatting with its models through an embedded AI coding agent.
**v0.1 is a single-process Linux/WSL TUI for Ollama**: it runs in a native
terminal on Linux (or Windows Subsystem for Linux), or over SSH from a phone
(Moshi, Blink, Termius, …) into that host, and the layout adapts to narrow
windows. Native Windows and native macOS are **not supported** in v0.1.

**Status: v0.2.0 released 2026-09-07** (signed-tag policy enforced for future
`v*` releases; see `CHANGELOG.md`). The Models tab lists live
models from the Ollama host (`/api/tags`) with selection + an inspect pane
(`/api/show`): key facts, parameters, template, modelfile, model info,
license — scrollable, side-by-side on wide screens and stacked
(enter-toggled) on narrow ones. **Delete with confirm (`x` → `y`/`esc`) and
streaming pull (`p` → name → spinner + progress; `esc` cancels)** work live;
pulls reload the list automatically. The Agent tab supports native or
content-embedded tool calls, explicit plain-chat fallback, and jailed
project-aware tools. The Settings tab (huh forms) edits the whole config
surface with in-session live apply.

## Install

Prebuilt static binaries ship with every release — no Go toolchain needed.
Grab the archive for your architecture from the
[releases page](https://github.com/MerverliPy/SelfTUI/releases/latest), verify
it against `SHA256SUMS`, and run it:

```sh
VER=v0.2.0    # always check the releases page for the latest tag
ARCH=amd64    # arm64 hosts (e.g. Raspberry Pi): ARCH=arm64
curl -LO "https://github.com/MerverliPy/SelfTUI/releases/download/${VER}/selftui-${VER}-linux-${ARCH}.tar.gz"
curl -LO "https://github.com/MerverliPy/SelfTUI/releases/download/${VER}/SHA256SUMS"
grep "linux-${ARCH}" SHA256SUMS | sha256sum -c -
tar -xzf "selftui-${VER}-linux-${ARCH}.tar.gz" && ./selftui
```

Got the Go toolchain? One command installs the same release:

```sh
go install github.com/MerverliPy/SelfTUI/cmd/self-tui@latest
```

Prefer to build from source? See
[Build & run](#build--run) — `make build` → `bin/selftui`.

## Features

- **Models** — list, inspect, pull (streamed progress), and delete Ollama models live.
- **Agent** — chat with streaming markdown + an embedded coding agent (jailed read/write/grep tools, explicit plain-chat fallback).
- **Settings** — huh forms over the whole config surface with live apply and esc-to-discard.
- **PC + phone** — one binary; the layout adapts from wide PC (120×40) down to a 72×30 phone over SSH (tmux reattach survives drops).

## Preview

Renders pinned by golden tests (`internal/ui/testdata/golden/`) — Models list + inspect side-by-side on wide PC:

```text
  Models    Agent    Settings
╭────────────────────────╮╭────────────────────────────────────────────────╮
│   Models               ││qwen3:8b                                        │
│                        ││size 4.8 GB · modified 2026-09-03 08:00         │
│  2 models              ││caps tools, vision                              │
│                        ││                                                │
││ qwen3:8b              ││── PARAMETERS                                   │
││ qwen3 · 8.2B · Q4_K_M ││num_ctx 4096                                    │
│  gemma3:12b            ││temperature 0.6                                 │
│  gemma3 · 12B · Q4_K_M ││── TEMPLATE                                     │
╰────────────────────────╯╰────────────────────────────────────────────────╯
```

Agent chat turn with per-turn meta on the assistant header:

```text
╭────────────────────────────────────────────────────────────────────────────────╮
│❯ you                                                                           │
│                                                                                │
│  explain this repo                                                             │
│                                                                                │
│◈ qwen3:8b                                                           0.4s · stop│
│                                                                                │
│  It is a mobile-first terminal UI for Ollama, styled like the opencode.ai TUI. │
╰────────────────────────────────────────────────────────────────────────────────╯
```

On a 72×30 phone the same tabs stack compactly (list fills the width, detail
toggles with enter) — see “Using SelfTUI from an iPhone (SSH)” below.

## Product contract

- **Platform:** a single-process TUI for Linux/WSL. Native Windows and macOS
  are not supported; phone use is SSH into a supported host, nothing runs on
  the phone itself.
- **Sessions are in-memory.** The conversation lives in the running process
  and ends with it. Every committed turn is mirrored to an **append-only
  Markdown transcript export** under the XDG state dir, so a chat survives
  exit as an inspectable file — and since **V2a (v0.2)** a saved transcript
  **can be reloaded** in a fresh process with `/resume` (see *Navigation*).
  v0.1 itself shipped no reload path; `/export` in the Agent input flushes
  and reports the transcript path.
- **Tools are disabled by default** and require an explicit workspace. The
  agent is plain chat (`tools off`) until you enable workspace tools
  (Settings → Agent → *Enable workspace tools*, `tools_enabled`, or
  `SELFTUI_TOOLS_ENABLED`) with a real project workspace root — never `/` or
  your home directory.
- **Command execution is a v0.2 capability.** The v0.1 release has no command
  execution. V2c adds `run_command` only when workspace tools are enabled,
  behind the Linux/WSL bubblewrap sandbox and an explicit per-call approval;
  see `docs/run-command-containment.md`.
- **Non-loopback tokens require HTTPS.** A bearer token over plain `http://`
  is accepted only for loopback hosts; pointing a token at any other host
  requires `https://` (enforced by config validation).

M7 polished the feel (opencode.ai TUI as reference): a slash-command menu over
the Agent input (`/clear`, `/model`, `/resume`, `/theme`, `/export`, `/help`,
`/refresh`) and a `ctrl+p` command palette reachable from any tab; stable
per-turn headers, a streaming caret (`▍`) that disappears at rest, elapsed +
stop-reason meta right-aligned on each assistant header, `pgup`/`pgdn` paging
and an `f` auto-follow toggle; the composer is opencode-style — a header row
with the model chip and a live context meter plus token usage
(`ctx ▓▓░░ 38% · 1.2k/3.1k`), an auto-growing prompt, and a statusline under
it that turns red at the truncation point and surfaces the omission marker in
the transcript. The model picker filters as you type and stars the configured
default. Golden render fixtures pin the shell — including every M7 overlay
and the composer layout — at the two canonical geometries, the measured
Moshi portrait device (72×30) and a wide PC window (120×40). The shell keeps
working down to a **40×12 minimum**; below that it shows a bounded
"terminal too small" notice with the current size and the 40×12 minimum
instead of rendering. **Auth/TLS**: https hosts (public CA, verified) +
bearer token are covered by tests on every endpoint; a settings save failure
surfaces inline with retry. `make smoke-reconnect` simulates an SSH drop
mid-generation and verifies clean process death, Ollama host recovery, and a
clean reconnect — see `docs/reconnect.md`.

## Build & run

```sh
make build     # bin/selftui
make run       # go run ./cmd/self-tui
make test      # unit tests (uncached)
make race      # full suite under the race detector
make lint      # go vet + gofmt check
make check     # canonical pre-commit gate: build + test + vet + gofmt
make vuln      # govulncheck ./... (needs govulncheck on PATH)
selftui -version  # print the build version ("dev" on dev builds)
```

Requires Go ≥ 1.25 for day-to-day development (`GOTOOLCHAIN=auto` fetches it
on demand). The **release gate enforces the pinned toolchain** — Go 1.27.1
and govulncheck v1.7.0, the same versions CI pins — and fails fast with the
exact fix before any gate work if `go version` is not go1.27.1, if the
`gofmt` on `PATH` is not the gofmt from that same Go distribution (gofmt has
no version flag, so the pin is enforced by identity), or if govulncheck is
missing or the wrong version. Before running the gate locally, put the
pinned Go distribution's `bin` first on `PATH`, e.g. from the repo root:

```sh
go version                                   # must report go1.27.1
# Go's own gofmt only resolves from its distribution's bin, so make PATH
# point at the distribution the `go` above actually runs:
export PATH="$(go env GOROOT)/bin:$PATH"     # gofmt now matches the pin
govulncheck -version                         # must report v1.7.0
```

`make vuln` additionally needs `govulncheck` on `PATH` (pinned install:
`go install golang.org/x/vuln/cmd/govulncheck@v1.7.0`). Logs go to
`$XDG_STATE_HOME/selftui/log.txt` (override with `-log-file /path/to/log`)
— never stderr, so the TUI stays clean over SSH. Every log line is redacted
at the sink: the configured auth token and `Bearer`-style credentials are
replaced with `[redacted]` before they reach the file or the ctrl+o logs
drawer.

## Release engineering

Release binaries are static, CGO-disabled Linux builds stamped with a version
(dev builds default to `dev`):

```sh
make build-linux-amd64                     # dist/selftui-linux-amd64
make build-linux-arm64                     # dist/selftui-linux-arm64
VERSION=v0.2.0 make release-check          # the full gate; never tags
```

`scripts/release-check.sh` (`VERSION=v0.2.0 make release-check`) is the gate
a release must pass before the owner tags it. It requires a clean worktree,
a `VERSION` of the form `v<major>.<minor>.<patch>`, and the enforced
toolchain pin above (fails fast, before any gate work, if go/gofmt/
govulncheck do not match the documented versions). It then runs `go mod
verify`, the gofmt check, `go vet`, uncached tests, race tests,
`govulncheck`, both Linux builds, a version-stamp check of each binary
(executed where the host can run it, otherwise the exact string `-X` linked
in), and writes deterministic archives plus `dist/SHA256SUMS`. The script
**never creates or pushes a git tag** — tagging and publishing is the
owner's step (`v0.1.0` was released this way on 2026-09-04 via `release.yml`;
`v0.2.0` followed on 2026-09-07). Since 2026-09-07 every
`v*` tag is GPG-signed (`git tag -s`; `release.yml` verifies the signature
before building anything — see CONTRIBUTING "Signed release tags"). Regression suite:
`bash scripts/release-check-test.sh` (fake go/gofmt/govulncheck fixtures
proving wrong versions fail fast, cross-umask byte-identical archives, and
flat-checksum verification — it never touches this tree's `dist/`).

Artifacts (all under the gitignored `dist/`):

- `dist/selftui-linux-amd64`, `dist/selftui-linux-arm64` — raw static binaries
- `dist/selftui-<version>-linux-<arch>.tar.gz` — deterministic archives
  (binary + `LICENSE` + `README.md`) with **fixed member modes** — binary
  0755, documents 0644 — so two builders or umasks produce byte-identical
  archives
- `dist/SHA256SUMS` — sha256 over both archives with **flat archive names**
  (no `dist/` prefix), generated from inside `dist/`, so verification works
  in a flat download directory:

```sh
cd dist && sha256sum -c SHA256SUMS     # repo-side verification
# …or beside downloaded GitHub Release assets (download the two archives +
# SHA256SUMS into one directory and run the same command there)
```

Audit packages (H-06):

```sh
make audit-pack                              # dist/selftui-audit-pack-<HEAD>.zip
```

`scripts/create-audit-pack.sh` produces a deterministic, **manifest-complete**
ZIP snapshot of the tracked tree (`git ls-files` is authoritative) — dotfiles
and `.github/workflows/*` included — so an external-audit package can never
again omit files its inventory promises. It requires a clean worktree,
snapshots the committed tree at `HEAD`, writes to the gitignored `dist/` by
default, and **refuses to overwrite an existing archive** (pass `--out` for
another path or `--force` to overwrite explicitly). Audit prompt/inventory
files (e.g. `PROMPT.md`, `FILE-INVENTORY.md`) are added only through explicit
`--extra TARGET=PATH` arguments (`AUDIT_PACK_EXTRAS="PROMPT.md=/path"` through
make) — never by silently substituting them for tracked files — and every
archived member is verified as a safe relative path. On success it prints the
archive path, tracked-file count, SHA256, and `MANIFEST_MATCH=PASS`; the
`verify` subcommand checks any ZIP against the tracked manifest in both
directions. Regression suite: `bash scripts/create-audit-pack-test.sh`.

CI and releases run on GitHub Actions (`.github/workflows/`): `ci.yml` runs
the local gate's checks minus the release-only steps (per-binary
version-stamp, archives, `SHA256SUMS`) on every pull request and push to
`main`; `release.yml` runs only on a pushed `v*` tag — it re-runs the
complete release gate, verifies the tag's GPG signature, verifies the tag is exactly the version stamped into
both binaries, uploads the two archives + `SHA256SUMS`, and publishes
release notes generated from `CHANGELOG.md`. Both workflows pin **Go 1.27.1**
and govulncheck **v1.7.0** as workflow configuration (env + `setup-go`); the
local gate now enforces the same versions itself, so CI configuration and the
documented local prerequisites cannot drift apart. Both workflows use only
GitHub's default `GITHUB_TOKEN` with least-privilege permissions — no
secrets.

## Config

Resolution order: **flags > env > config file > defaults**.

| Source | Examples |
|--------|----------|
| Flags | `selftui -host http://192.168.1.50:11434 -theme light -default-model qwen3:8b -temperature 0.4 -top-p 0.95 -num-ctx 8192 -max-tool-iterations 20 -workspace-root ~/proj -system-prompt "…" -verbose -log-file /tmp/selftui.log` |
| Env | `SELFTUI_HOST`, `SELFTUI_THEME`, `SELFTUI_DEFAULT_MODEL`, `SELFTUI_WORKSPACE_ROOT`, `SELFTUI_AUTH_TOKEN`, `SELFTUI_TOOLS_ENABLED`, `SELFTUI_AGENT_TEMPERATURE`, `SELFTUI_AGENT_TOP_P`, `SELFTUI_AGENT_NUM_CTX`, `SELFTUI_AGENT_SYSTEM_PROMPT`, `SELFTUI_AGENT_MAX_TOOL_ITERATIONS`, `SELFTUI_SESSION_DIR`, `SELFTUI_NO_SESSION` |
| File | `~/.config/selftui/config.toml` (`host`, `theme`, `default_model`, `auth_token`, `workspace_root`, `tools_enabled`, `[agent]` table) |

**Secrets (auth token):** set the token via `SELFTUI_AUTH_TOKEN` or put
`auth_token` in the config file (written 0600, directory 0700) — the Settings →
Connection form does this for you. There is deliberately no `-auth-token` flag:
a command-line secret shows up in process listings and shell history. A token is
only sent over `https://` unless the host is loopback
(`localhost`, `127.0.0.1`, `::1`) — a non-loopback host with a token must use
`https://`.

## Navigation

`tab` / `shift-tab`, or `1`/`2`/`3` from the Models tab, switch Models ·
Agent · Settings. `1`/`2`/`3` also switch from an empty Agent chat input; once
you start typing a prompt the digits become text (so “count to 300” never
jumps tabs). `ctrl+c` quits. `ctrl+o` toggles the read-only **logs drawer**
from any tab — debug traces (ollama requests, connection errors, agent-loop
decisions) with bearer tokens redacted; `pgup`/`pgdn` scroll, `esc` closes
(detail under Command palette below). `ctrl+p` opens the command palette.

**Models tab (M1b)**: `j`/`k` or arrows select · `enter` opens the inspect
pane (compact) or refreshes it (wide) · `esc` closes it · `u`/`d` scroll the
inspect pane · `r` reloads the model list · `x` deletes the selected model
(confirm with `y`, cancel with `n`/`esc`) · `p` pulls a model (type a name
like `qwen3:0.6b`, `enter` starts, `esc` cancels; progress + spinner show
while it streams, and the list reloads when it lands). Wide screens
auto-inspect the selected model.

**Agent tab (M2/M3b/M7)**: type a prompt · `enter` sends · `shift+enter` inserts a
newline · `m` opens the model picker (filter as you type; the configured
default is starred) · `r` reloads models · `u`/`d` scroll the transcript,
`pgup`/`pgdn` page it, and `f` toggles auto-follow (the stream auto-tails by
default; `d`/`pgdn` to the tail re-engages it). The bottom region is an
opencode-style composer: a header row showing the model chip and a live
context meter with token usage (`ctx ▓▓░░ 38% · 1.2k/3.1k`), an auto-growing
prompt (up to four rows), and below the box a statusline that shows the
running state with an **armed interrupt** (`esc` arms, `esc` again cancels —
a stray esc can't kill a run) or the key legend. A **`/`** in the prompt opens
the command menu — `/clear` (asks first), `/model`, `/resume` (reload a
saved transcript), `/undo` / `/redo` (revert a confirmed file change),
`/theme` (session toggle; save in Settings to keep it),
`/help` (command reference), `/refresh`, and
`/export` (flush the Markdown transcript and show its path) —
filtered as you type; arrows move and `enter` runs. Idle `esc` clears a
drafted prompt. Every finished assistant message shows elapsed time and why
it stopped (`· stop` / `· length` / `· stopped`) right-aligned on its header;
while a turn streams a `▍` caret rides the last line and vanishes at rest.
Once the conversation fills the context budget the meter turns red and a
visible truncation marker stays in the transcript until `/clear`.

**Chat is in-memory; the transcript is an export — and `/resume` reloads
it.** Committed turns are mirrored to a per-process Markdown file under
`$XDG_STATE_HOME/selftui/sessions/` (`chat-<timestamp>-<pid>.md`, 0600,
`## user (qwen3:8b) · time` / `## assistant (…) · elapsed · reason` blocks).
The file is append-only, survives exit, and is inspectable. Reopening
SelfTUI starts a fresh in-memory session; `/resume` (V2a) opens a picker of
the saved transcripts (newest first) and loads the chosen one into the live
conversation — asking first when the current chat already has history, and
refusing sends while a load is in flight. The saved file is never rewritten:
the resumed conversation continues in a fresh per-process transcript.
`/export` in the Agent input flushes and reports the path;
`SELFTUI_SESSION_DIR` overrides the directory and `SELFTUI_NO_SESSION=1`
turns recording off (chat then stays in-memory only, and `/resume` reports
"session recording is off — nothing to resume").

Tool-capable models may use jailed `read_file`, `list_dir`, `grep`,
`write_file`, and `edit_file`; every mutation opens a `y`/`enter` approve or
`n`/`esc` decline dialog. Confirmed file mutations are journaled per session
and revertible with the `/undo` and `/redo` commands. V2c also exposes `run_command`, which accepts only
allowlisted `go` and read-only `git` argv and runs it inside bubblewrap with
network disabled, a scrubbed environment, bounded timeout/output, and process
-group cancellation; it uses the same explicit approval dialog. **`run_command`
effects are never journaled and cannot be undone** — only file mutations
carry pre-images; a command's writes are protected from a later `/undo`
clobber by the journal's refuse-guards, never reverted. These tools
exist only when **workspace tools are enabled** (Settings → *Enable workspace
tools*, `tools_enabled`, or `SELFTUI_TOOLS_ENABLED`) with a project workspace
root; otherwise the agent is plain chat and the Ollama request carries no
tools. With tools enabled, requests to sensitive paths (`.ssh`, `.gnupg`, `.aws`,
`.azure`, `.kube`, `.config/gcloud`, or credential files like `.env`, `.env.local`,
`.env.production`, `credentials`, `credentials.json` — `.env.example` stays
allowed) are refused before the tool runs, on top of canonical workspace
containment. Every armed agent turn also starts with a **bounded workspace-context**
message (V2d): git branch/porcelain status/recent commits when the workspace is
a git repo (fixed read-only git argv, run by the app on the host — not through
the sandbox) plus a depth- and entry-capped project index (`.git` pruned).
Plain chat (`tools off`) never receives it. Models that reject
tools or return no tool call show an explicit plain-chat fallback.

**Command palette (M7)**: `ctrl+p` from any tab opens the command palette —
go to a tab, change model, clear the conversation, toggle the theme, refresh
models, open the logs drawer, or open the command list — filtered as you type
(`↑/↓` or `j/k` move, `enter` runs, `esc` closes). On a phone keyboard
without a ctrl key, the Agent tab's `/` menu is the equivalent path (Blink
maps ctrl to the `ctrl+p` shortcut). Palette and slash actions are
session-scoped; persistence is Settings → Theme.

**Logs drawer (N5)**: `ctrl+o` from any tab (or the palette's *Logs drawer*
command) toggles a read-only debug drawer over the view's bottom rows —
ollama request/response traces (method, path, status, size, duration),
connection errors, and agent-loop decisions (tool calls, context-budget
truncations, plain-chat fallbacks). `pgup`/`pgdn` or `↑/↓` scroll back
through the last 1000 entries (`end` returns to tail-follow, `home` jumps to
the top), `esc` or `ctrl+o` closes. The drawer and the log file are the same
redacted stream: bearer tokens are scrubbed from everything that flows
through the logger (the configured `auth_token` and generic `Bearer …`
credentials), so neither sink ever holds a secret. Redirect the file with
`-log-file /path/to/log` (default `$XDG_STATE_HOME/selftui/log.txt`); the
drawer always mirrors what is written there. Log verbosity is unchanged by
the drawer — `-verbose` still raises the level to debug.

**Settings tab (M4/Phase 4)**: a huh form over the config surface, in four sections —
Connection (host, auth token), Model defaults (default model, temperature,
top-p, context window), Theme, and Agent (system prompt, workspace root, max
tool iterations, and an *Enable workspace tools* toggle — off by default,
validated against the config policy). `enter`/`tab` advance fields,
`shift+tab` goes back, and
`esc` discards the whole edit (nothing is written; “revert”). Arrowing Dark/
Light previews the theme live; submitting on the last field writes the config
file and applies the change in-session (host/token swap the Ollama client and
reload both model lists; agent parameters apply to the next run). While the
form is open it owns the keyboard — tab/1/2/3 return once it is saved or
discarded; `ctrl+c` still quits.

Live behavior check: `make smoke` drives a real pull + delete against your
local Ollama host over a pty. It is **non-destructive**: the script captures
the host's state first and **refuses to run when the target model is already
installed** (it never deletes a model the run did not create). Use a
disposable model/tag — `make smoke-model MODEL=<name>` to override — or run
against an isolated Ollama store.

Smoke evidence never lands at a fixed public path: each run writes its TUI
capture into a private unique temp dir (`0700` dir, exclusive `0600` file),
retaining it on failure (its exact path is printed) and removing it after a
successful run unless `SMOKE_KEEP_CAPTURE=1` keeps it.

The layout reference geometry was measured on the real client (Moshi on an
iPhone 16 Pro, portrait, default font): **72 columns × 30 rows** — see
`docs/m0a-gate-evidence.md`. Golden render tests enforce this exact frame.
Terminals stay usable down to **40 columns × 12 rows**; anything smaller
shows a bounded "terminal too small" notice (with the current size and the
40×12 minimum) instead of the shell.

## Using SelfTUI from a phone (SSH)

SelfTUI runs on a **Linux or WSL host**; a phone is only an SSH client into
that host — nothing runs on the phone itself.

### On the computer (the host)

1. Install **Ollama** and SelfTUI (prebuilt binary — see
   [Install](#install) — or `make build` → `bin/selftui` / `make run` from
   source). The host must run Linux or WSL.
2. Make sure your SSH server is enabled and reachable from the phone
   (`systemctl status ssh` on Linux/WSL). Key-based login is easiest on a
   phone.
3. SelfTUI talks to Ollama on the **same machine** (`http://localhost:11434`
   default) — nothing else needs exposing to the network. For a *remote*
   Ollama, point at it with `-host https://…` and set
   `SELFTUI_AUTH_TOKEN` (or put `auth_token` in the config file), and set the
   same in Settings → Connection. A bearer token requires `https://` for any
   non-loopback host; plain-http bearer tokens are only accepted for
   localhost.

### On the phone

1. Install an SSH client: **Blink Shell**, **Termius**, or similar (Moshi is
   the client SelfTUI was measured on).
2. Add your computer as a host (`user@ip-or-name`) and connect. SSH key auth
   avoids typing passwords on the soft keyboard.
3. Run `selftui` (or `make run`). Optionally pass `-theme light` for bright
   rooms — or switch it live later in Settings → Theme (arrow over Light and
   watch it restyle; submit to keep).

### What you get on the small screen

Portrait with the default font lands in the **compact** layout (≤ 79 cols,
measured 72×30):

- **Models** — the list fills the screen; `enter` stacks the inspect pane
  below (the list shrinks to the top 40%), `esc` closes it, `u`/`d` scroll
  it. Wide-enough windows (≥ 90 cols) split list + detail side by side.
- **Agent** — chat fills the width, the opencode-style composer sits at the
  bottom (model chip + live ctx usage header, auto-growing prompt, statusline
  beneath); `m` selects a model, `esc` while running is an armed interrupt
  (`esc` again cancels). Mutation approvals (`y`/`enter` approve,
  `n`/`esc` decline) are height-capped so the decision row is always on
  screen, even for a huge payload.
- **Settings** — one form page per section (`enter` next); ≥ 120 cols turns
  it into two columns.

The status bar shows the live `WxH` and the active layout
(`compact`/`medium`/`wide`) — rotate or zoom the font and watch it adapt.
Everything is keyboard-driven: `tab`/`shift-tab` or `1`/`2`/`3` (Models tab,
or empty chat input) switch tabs; `esc` lives on the iOS keyboard toolbar (or
as a hardware key) in Blink and friends — it cancels pulls, stops agent
turns, and discards settings edits. Below **40×12** SelfTUI shows the bounded
"terminal too small" notice until the window grows back.

### If your SSH session drops

SelfTUI keeps your **settings** in a config file, but the conversation lives
in the running process (chat is in-memory; committed turns are mirrored to
the per-process Markdown transcript under the XDG state dir). What survives
a phone-side drop depends on the transport (verified live,
`docs/reconnect.md`):

- **Inside tmux (or mosh)** — the recommended setup — the app process
  survives: reconnect and re-attach and you get the **same screen back**
  (conversation + scroll intact).
- **Plain SSH (no tmux)** — the drop kills the app; reconnect and relaunch:
  clean boot at the negotiated geometry, config re-applied, and the aborted
  model job is cleaned up by Ollama (nothing is left stuck). The transcript
  file from the dead process is still on disk — relaunch and run `/resume`
  to load that chat back into the fresh session.

`make smoke-reconnect` exercises the plain-SSH path locally (SIGHUP on a
mid-generation drop, host recovery, clean fresh reconnect); its scratch
config/state and capture live in the same kind of private unique temp dir
and are removed after a pass unless `SMOKE_KEEP_CAPTURE=1`.

## Project docs

- `README.md` — this file (v0.1 product contract + usage)
- `PLAN.md` — architecture, decisions, roadmap §10, risks §11
- `LEDGER.md` — chronological work/decision log (historical records)
- `COUNCIL-MEMO.md` — advisory audit that re-cut the roadmap
- `AGENTS.md` — repo rules for agent sessions
- `CHANGELOG.md` — released and unreleased changes
- `CONTRIBUTING.md` — how to build, test, and contribute
- `SECURITY.md` — how to report a vulnerability
- `LICENSE` — Apache-2.0
