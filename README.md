# SelfTUI

A visually appealing, responsive terminal UI to manage local/remote Ollama
models, with an embedded AI coding-agent chat and a settings panel. Runs
identically on a PC (native terminal) and over SSH on a phone (Moshi;
Blink/Termius similar) — layout adapts to narrow windows.

**Status: M3b — jailed mutation agent landed.** The Models tab lists live models from the Ollama host (`/api/tags`) with selection + an inspect pane (`/api/show`): key facts, parameters, template, modelfile, model info, license — scrollable, side-by-side on wide screens and stacked (enter-toggled) on the phone. **Delete with confirm (`x` → `y`/`esc`) and streaming pull (`p` → name → spinner + progress; `esc` cancels)** work live; pulls reload the list automatically. The Agent tab supports native or content-embedded tool calls, explicit plain-chat fallback, jailed read/write/edit tools, and a constrained confirmed command executor. It is a guardrail rather than an OS sandbox; general shell and interpreters remain disabled.

## Build & run

```sh
make build     # bin/selftui
make run       # go run ./cmd/self-tui
make test      # unit tests
make lint      # go vet + gofmt check
```

Requires Go ≥ 1.25 (`GOTOOLCHAIN=auto` fetches it on demand). Logs go to
`$XDG_STATE_HOME/selftui/log.txt` — never stderr, so the TUI stays clean over
SSH.

## Config

Resolution order: **flags > env > config file > defaults**.

| Source | Examples |
|--------|----------|
| Flags | `selftui -host http://192.168.1.50:11434 -theme light` |
| Env | `SELFTUI_HOST`, `SELFTUI_THEME`, `SELFTUI_DEFAULT_MODEL`, `SELFTUI_WORKSPACE_ROOT` |
| File | `~/.config/selftui/config.toml` (`host`, `theme`, `auth_token`, …) |

## Navigation

`tab` / `shift-tab` or `1`/`2`/`3` switch Models · Agent · Settings.
`ctrl+c` quits.

**Models tab (M1b)**: `j`/`k` or arrows select · `enter` opens the inspect
pane (compact) or refreshes it (wide) · `esc` closes it · `u`/`d` scroll the
inspect pane · `r` reloads the model list · `x` deletes the selected model
(confirm with `y`, cancel with `n`/`esc`) · `p` pulls a model (type a name
like `qwen3:0.6b`, `enter` starts, `esc` cancels; progress + spinner show
while it streams, and the list reloads when it lands). Wide screens
auto-inspect the selected model.

**Agent tab (M2/M3b)**: type a prompt · `enter` sends · `shift+enter` inserts a
newline · `m` selects a model · `esc` cancels a turn. Tool-capable models may
use jailed `read_file`, `list_dir`, `grep`, `write_file`, and `edit_file`;
every mutation opens a `y`/`enter` approve or `n`/`esc` decline dialog. The
only command executor accepts a fixed argv for approved `go` subcommands and
read-only `git` subcommands; it has a scrubbed environment, 30s default/60s
maximum timeout, 256 KiB cap per output stream, process-group cancellation,
and no shell or interpreter. Models that reject tools or return no tool call
show an explicit plain-chat fallback.

Live behavior check: `make smoke` drives a real pull + delete against your
local Ollama host over a pty (leaves the host exactly as it was).

## Project docs

- `PLAN.md` — architecture, decisions, roadmap §10, risks §11
- `LEDGER.md` — chronological work/decision log
- `COUNCIL-MEMO.md` — advisory audit that re-cut the roadmap
- `AGENTS.md` — repo rules for agent sessions

The SSH-on-iPhone guide (Blink/Termius) ships with M5 acceptance.