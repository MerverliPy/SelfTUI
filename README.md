# SelfTUI

A visually appealing, responsive terminal UI to manage local/remote Ollama
models, with an embedded AI coding-agent chat and a settings panel. Runs
identically on a PC (native terminal) and over SSH on a phone (Moshi;
Blink/Termius similar) — layout adapts to narrow windows.

**Status: M7 — pre-v0.1 UX polish done.** The Models tab lists live models from the Ollama host (`/api/tags`) with selection + an inspect pane (`/api/show`): key facts, parameters, template, modelfile, model info, license — scrollable, side-by-side on wide screens and stacked (enter-toggled) on the phone. **Delete with confirm (`x` → `y`/`esc`) and streaming pull (`p` → name → spinner + progress; `esc` cancels)** work live; pulls reload the list automatically. The Agent tab supports native or content-embedded tool calls, explicit plain-chat fallback, jailed read/write/edit tools, and a constrained confirmed command executor. It is a guardrail rather than an OS sandbox; general shell and interpreters remain disabled. M7 polished the feel (opencode.ai TUI as reference): a slash-command menu over the Agent input (`/clear`, `/model`, `/theme`, `/help`, `/refresh`) and a `ctrl+p` command palette reachable from any tab; stable per-turn headers, a streaming caret (`▍`) that disappears at rest, elapsed + stop-reason footers, `pgup`/`pgdn` paging and an `f` auto-follow toggle; and a live context meter (`ctx ▓▓░░░ 38%`) that goes red at the truncation point and surfaces the omission marker in the transcript. The model picker filters as you type and stars the configured default. The Settings tab (huh forms) edits the whole config surface and writes it back to the config file with in-session live apply. Golden render fixtures pin the shell — including every M7 overlay — at the two canonical geometries, the measured Moshi portrait device (72×30) and a wide PC window (120×40). **Auth/TLS**: https hosts (public CA, verified) + bearer token are covered by tests on every endpoint; a settings save failure surfaces inline with retry. `make smoke-reconnect` simulates an SSH drop mid-generation and verifies clean process death, Ollama host recovery, and a clean reconnect — see `docs/reconnect.md`.

## Build & run

```sh
make build     # bin/selftui
make run       # go run ./cmd/self-tui
make test      # unit tests
make lint      # go vet + gofmt check
selftui -version  # print the build version
```

Requires Go ≥ 1.25 (`GOTOOLCHAIN=auto` fetches it on demand). Logs go to
`$XDG_STATE_HOME/selftui/log.txt` — never stderr, so the TUI stays clean over
SSH.

## Config

Resolution order: **flags > env > config file > defaults**.

| Source | Examples |
|--------|----------|
| Flags | `selftui -host http://192.168.1.50:11434 -theme light -default-model qwen3:8b -temperature 0.4 -top-p 0.95 -num-ctx 8192 -max-tool-iterations 20 -workspace-root ~/proj -auth-token tok -system-prompt "…"` |
| Env | `SELFTUI_HOST`, `SELFTUI_THEME`, `SELFTUI_DEFAULT_MODEL`, `SELFTUI_WORKSPACE_ROOT`, `SELFTUI_AUTH_TOKEN`, `SELFTUI_AGENT_TEMPERATURE`, `SELFTUI_AGENT_TOP_P`, `SELFTUI_AGENT_NUM_CTX`, `SELFTUI_AGENT_SYSTEM_PROMPT`, `SELFTUI_AGENT_MAX_TOOL_ITERATIONS` |
| File | `~/.config/selftui/config.toml` (`host`, `theme`, `default_model`, `auth_token`, `workspace_root`, `[agent]` table) |

## Navigation

`tab` / `shift-tab`, or `1`/`2`/`3` from the Models tab, switch Models ·
Agent · Settings. `1`/`2`/`3` also switch from an empty Agent chat input; once
you start typing a prompt the digits become text (so “count to 300” never
jumps tabs). `ctrl+c` quits.

**Models tab (M1b)**: `j`/`k` or arrows select · `enter` opens the inspect
pane (compact) or refreshes it (wide) · `esc` closes it · `u`/`d` scroll the
inspect pane · `r` reloads the model list · `x` deletes the selected model
(confirm with `y`, cancel with `n`/`esc`) · `p` pulls a model (type a name
like `qwen3:0.6b`, `enter` starts, `esc` cancels; progress + spinner show
while it streams, and the list reloads when it lands). Wide screens
auto-inspect the selected model.

**Agent tab (M2/M3b/M7)**: type a prompt · `enter` sends · `shift+enter` inserts a
newline · `esc` stops a turn or clears a drafted prompt · `m` opens the model
picker (filter as you type; the configured default is starred) · `r` reloads
models · `u`/`d` scroll the transcript, `pgup`/`pgdn` page it, and `f`
toggles auto-follow (the stream auto-tails by default; `d`/`pgdn` to the
tail re-engages it). A **`/`** in the input opens the command menu — `/clear`
(asks first), `/model`, `/theme` (session toggle; save in Settings to keep it),
`/help` (command reference), `/refresh` — filtered as you type; arrows move
and `enter` runs. Every finished turn gets a footer with its elapsed time and
why it stopped (`· stop` / `· length` / `· stopped`); while a turn streams a
`▍` caret rides the last line and vanishes at rest. The hint row carries a
live context meter (`ctx ▓▓░░░ 38%`) that turns red at the truncation point
and leaves the omission marker visible in the transcript until `/clear`.
Tool-capable models may use jailed `read_file`, `list_dir`, `grep`,
`write_file`, and `edit_file`; every mutation opens a `y`/`enter` approve or
`n`/`esc` decline dialog. The only command executor accepts a fixed argv for
approved `go` subcommands and read-only `git` subcommands; it has a scrubbed
environment, 30s default/60s maximum timeout, 256 KiB cap per output stream,
process-group cancellation, and no shell or interpreter. Models that reject
tools or return no tool call show an explicit plain-chat fallback.

**Command palette (M7)**: `ctrl+p` from any tab opens the command palette —
go to a tab, change model, clear the conversation, toggle the theme, refresh
models, or open the command list — filtered as you type (`↑/↓` or `j/k`
move, `enter` runs, `esc` closes). On a phone keyboard without a ctrl key,
the Agent tab's `/` menu is the equivalent path (Blink maps ctrl to the
`ctrl+p` shortcut). Palette and slash actions are session-scoped; persistence
is Settings → Theme.

**Settings tab (M4)**: a huh form over the config surface, in four sections —
Connection (host, auth token), Model defaults (default model, temperature,
top-p, context window), Theme, and Agent (system prompt, workspace root, max
tool iterations). `enter`/`tab` advance fields, `shift+tab` goes back, and
`esc` discards the whole edit (nothing is written; “revert”). Arrowing Dark/
Light previews the theme live; submitting on the last field writes the config
file and applies the change in-session (host/token swap the Ollama client and
reload both model lists; agent parameters apply to the next run). While the
form is open it owns the keyboard — tab/1/2/3 return once it is saved or
discarded; `ctrl+c` still quits.

Live behavior check: `make smoke` drives a real pull + delete against your
local Ollama host over a pty (leaves the host exactly as it was).

The layout reference geometry was measured on the real client (Moshi on an
iPhone 16 Pro, portrait, default font): **72 columns × 30 rows** — see
`docs/m0a-gate-evidence.md`. Golden render tests enforce this exact frame.

## Using SelfTUI from an iPhone (SSH)

SelfTUI is a plain TUI over SSH — nothing runs on the phone itself.

### On the computer (the host)

1. Install **Ollama** and SelfTUI (`make build` → `bin/selftui`, or run from
   source with `make run`).
2. Make sure your SSH server is enabled and reachable from the phone
   (`systemctl status ssh`, or macOS → System Settings → Sharing → Remote
   Login). Key-based login is easiest on a phone.
3. SelfTUI talks to Ollama on the **same machine** (`http://localhost:11434`
   default) — nothing else needs exposing to the network. For a *remote*
   Ollama, point at it with `-host http://…` plus `-auth-token` (or
   `SELFTUI_HOST`/`SELFTUI_AUTH_TOKEN`), and set the same in Settings →
   Connection.

### On the iPhone

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
- **Agent** — chat fills the width, input sits at the bottom; `m` selects a
  model, `esc` stops a turn. Mutation approvals (`y`/`enter` approve,
  `n`/`esc` decline) are height-capped so the decision row is always on
  screen, even for a huge payload.
- **Settings** — one form page per section (`enter` next); ≥ 120 cols turns
  it into two columns.

The status bar shows the live `WxH` and the active layout
(`compact`/`medium`/`wide`) — rotate or zoom the font and watch it adapt.
Everything is keyboard-driven: `tab`/`shift-tab` or `1`/`2`/`3` (Models tab,
or empty chat input) switch tabs; `esc` lives on the iOS keyboard toolbar (or
as a hardware key) in Blink and friends — it cancels pulls, stops agent
turns, and discards settings edits.

### If your SSH session drops

SelfTUI keeps your **settings** in a config file, but the conversation lives
in the running process. What survives a phone-side drop depends on the
transport (verified live, `docs/reconnect.md`):

- **Inside tmux (or mosh)** — the recommended setup — the app process
  survives: reconnect and re-attach and you get the **same screen back**
  (conversation + scroll intact).
- **Plain SSH (no tmux)** — the drop kills the app; reconnect and relaunch:
  clean boot at the negotiated geometry, config re-applied, and the aborted
  model job is cleaned up by Ollama (nothing is left stuck).

`make smoke-reconnect` exercises the plain-SSH path locally (SIGHUP on a
mid-generation drop, host recovery, clean fresh reconnect).

## Project docs

- `PLAN.md` — architecture, decisions, roadmap §10, risks §11
- `LEDGER.md` — chronological work/decision log
- `COUNCIL-MEMO.md` — advisory audit that re-cut the roadmap
- `AGENTS.md` — repo rules for agent sessions
