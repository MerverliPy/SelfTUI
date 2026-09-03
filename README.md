# SelfTUI

A visually appealing, responsive terminal UI to manage local/remote Ollama
models, with an embedded AI coding-agent chat and a settings panel. Runs
identically on a PC (native terminal) and over SSH on a phone (Moshi;
Blink/Termius similar) — layout adapts to narrow windows.

**Status: M0 — skeleton.** The app boots, tabs work, the responsive layout
system and cancellation plumbing are in place, and the Charm dependency set is
pinned (v2 line). Models list, chat/agent, and settings land in later
milestones (see `PLAN.md` §10).

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
`ctrl+c` quits. (Full key map lands with the real views.)

## Project docs

- `PLAN.md` — architecture, decisions, roadmap §10, risks §11
- `LEDGER.md` — chronological work/decision log
- `COUNCIL-MEMO.md` — advisory audit that re-cut the roadmap
- `AGENTS.md` — repo rules for agent sessions

The SSH-on-iPhone guide (Blink/Termius) ships with M5 acceptance.