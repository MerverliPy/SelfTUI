# SelfTUI

A visually appealing, responsive terminal UI to manage local/remote Ollama
models, with an embedded AI coding-agent chat and a settings panel. Runs
identically on a PC (native terminal) and over SSH on a phone (Moshi;
Blink/Termius similar) — layout adapts to narrow windows.

**Status: M4 — Settings & persistence landed.** The Models tab lists live models from the Ollama host (`/api/tags`) with selection + an inspect pane (`/api/show`): key facts, parameters, template, modelfile, model info, license — scrollable, side-by-side on wide screens and stacked (enter-toggled) on the phone. **Delete with confirm (`x` → `y`/`esc`) and streaming pull (`p` → name → spinner + progress; `esc` cancels)** work live; pulls reload the list automatically. The Agent tab supports native or content-embedded tool calls, explicit plain-chat fallback, jailed read/write/edit tools, and a constrained confirmed command executor. It is a guardrail rather than an OS sandbox; general shell and interpreters remain disabled. The Settings tab (huh forms) edits the whole config surface and writes it back to the config file with in-session live apply.

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
| Flags | `selftui -host http://192.168.1.50:11434 -theme light -default-model qwen3:8b -temperature 0.4 -top-p 0.95 -num-ctx 8192 -max-tool-iterations 20 -workspace-root ~/proj -auth-token tok -system-prompt "…"` |
| Env | `SELFTUI_HOST`, `SELFTUI_THEME`, `SELFTUI_DEFAULT_MODEL`, `SELFTUI_WORKSPACE_ROOT`, `SELFTUI_AUTH_TOKEN`, `SELFTUI_AGENT_TEMPERATURE`, `SELFTUI_AGENT_TOP_P`, `SELFTUI_AGENT_NUM_CTX`, `SELFTUI_AGENT_SYSTEM_PROMPT`, `SELFTUI_AGENT_MAX_TOOL_ITERATIONS` |
| File | `~/.config/selftui/config.toml` (`host`, `theme`, `default_model`, `auth_token`, `workspace_root`, `[agent]` table) |

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

## Project docs

- `PLAN.md` — architecture, decisions, roadmap §10, risks §11
- `LEDGER.md` — chronological work/decision log
- `COUNCIL-MEMO.md` — advisory audit that re-cut the roadmap
- `AGENTS.md` — repo rules for agent sessions

The SSH-on-iPhone guide (Blink/Termius) ships with M5 acceptance.