# SelfTUI — Project Plan

> **SelfTUI** = **Self**-hosted + **TUI** (and "self" in the autonomous sense — it's an
> embedded coding agent). A visually appealing, responsive TUI to manage local/remote
> Ollama models, with an embedded AI coding-agent chat and a settings panel. Runs
> identically on PC (native terminal) and iPhone 16 Pro (SSH into the host, responsive to
> narrow screens).

> **Status: PLANNING.** No implementation code yet.

---

## 1. Goals

| Goal | Detail |
|------|--------|
| **Manage Ollama models** | List, inspect, delete, and pull models against a configurable Ollama host. |
| **Chat & code agent** | A full project-aware AI coding agent (read/edit files, run commands) via the selected model. |
| **Settings panel** | Configure host, auth, theme, default model, and agent parameters. |
| **PC + iPhone compatible** | One binary. PC runs it in a real terminal; iPhone connects over SSH (Blink/Termius/iSH) and gets a layout that adapts to a narrow/tall window. |

### Non-goals (v1)
- No web server baked in (iPhone access is SSH-by-design per decision).
- No native iOS app.
- Packaging/installers beyond a single static binary + `Makefile`.

---

## 2. Confirmed technical decisions

- **Language:** Go 1.22+
- **TUI stack:** Charm.sh — **Bubble Tea** (models), **Lip Gloss** (styling/layout),
  **Huh** (settings forms), **Charmbracelet/log** (structured logs)
- **iPhone access:** SSH into the host running this TUI (Blink / Termius / iSH client).
- **Agent scope:** Full coding agent (project-aware, files + commands).
- **Ollama:** Default `http://localhost:11434`, but configurable base URL + optional auth
  so remote hosts (needed for iPhone-only workflows) are supported.
- **Markdown rendering:** `glamour` for GitHub-flavored markdown with syntax-highlighted
  code blocks in agent/chat output.
- **Repository:** standalone git repo at `/home/calvin/SelfTUI`; `/TUI` gitignored by the
  parent dotfiles repo so the code never pollutes it.

---

## 3. System architecture

```
                         ┌─────────────────────────────────────────────┐
                         │  Bubble Tea application (single model/Update)│
                         │   ┌───────────────────────────────────────┐ │
  ollama http ─────────▶ │   │  Views:                              │ │
  baseURL /api/...       │   │   • Models   (manage list)           │ │
                         │   │   • Agent    (chat + tools)          │ │
                         │   │   • Settings (huh forms)             │ │
                         │   │   • Status bar / tabs (responsive)   │ │
                         │   └───────────────────────────────────────┘ │
                         └─────────────────────────────────────────────┘
                                   ▲ events (tea.Msg)
          ┌────────────────────────┴─────────────────────────┐
          │               Background goroutines              │
          │  ┌──────────────┐   ┌──────────────────────────┐  │
          │  │ Ollama client │   │ Agent runner (tool loop)│  │
          │  │  /api/tags,   │   │  chat + tool_calls +    │  │
          │  │  pull stream, │   │  run_command/read/write │  │
          │  │  delete, show │   │                          │  │
          │  └──────────────┘   └──────────────────────────┘  │
          └───────────────────────────────┬────────────────────┘
                                          │ POST /api/chat        │ POST /api/chat
                                          │ (agent → Ollama host) │
                                          ▼
                                 ┌──────────────────┐
                                 │  Ollama host     │
                                 │ (local or remote)│
                                 └──────────────────┘
```

**Core principle:** *never block the Bubble Tea loop on long-running work.* Every
long operation (model pull, agent tool round-trip, command execution) runs in a
background goroutine that posts incremental `tea.Msg` events back to the UI through a
channel subscription (`tea.Cmd` + `waitForActivity`). The UI renders streaming progress
in real time without freezing.

---

## 4. Project layout

```
TUI/
├── go.mod                        # module selftui / self-tui (or local path)
├── Makefile                      # build, test, lint, run (tailscale/SSH note)
├── README.md                     # usage, SSH-on-iPhone guide
├── PLAN.md                       # this file
├── LEDGER.md                     # persistent work/decision log
├── COUNCIL-MEMO.md               # advisory audit verdict
├── cmd/
│   └── self-tui/
│       └── main.go               # entrypoint: flags, config load, tea.NewProgram
├── internal/
│   ├── config/
│   │   ├── config.go             # Config struct + defaults
│   │   ├── load.go               # env vars, flags, config file merge
│   │   └── config_test.go
│   ├── ollama/
│   │   ├── client.go             # Ollama HTTP client (base URL, auth, timeout)
│   │   ├── tags.go               # list models
│   │   ├── pull.go               # streaming pull (progress events)
│   │   ├── delete.go             # delete model
│   │   ├── show.go               # model details/inspect
│   │   ├── chat.go               # chat + tool_calls request/response types
│   │   └── ollama_test.go
│   ├── agent/
│   │   ├── runner.go             # the async tool-loop state machine
│   │   ├── tools.go              # tool definitions (schema for Ollama)
│   │   ├── exec.go               # read_file / write_file / edit_file / list_dir / grep
│   │   ├── command.go            # run_command with streaming + timeout + cwd sandbox
│   │   ├── context.go            # context window budgeting / truncation
│   │   └── runner_test.go
│   └── ui/
│       ├── app.go                # root Bubble Tea model (view switching, tabs)
│       ├── styles.go             # Lip Gloss theme (light/dark), reuse for pc+mobile
│       ├── layout.go             # responsive breakpoints / panel stacking
│       ├── models_view.go        # model list + actions + pull progress
│       ├── agent_view.go         # chat transcript + input box
│       ├── settings_view.go      # huh form(s)
│       ├── components/
│       │   ├── tabs.go
│       │   ├── statusbar.go
│       │   └── spinner.go
│       └── *_test.go             # golden render tests at two widths
```

---

## 5. Domain model / data flow

### Config (`internal/config`)
Resolved in priority order: **flags > env vars > config file (`~/.config/selftui/config.toml`) > defaults.**

| Key | Default | Meaning |
|-----|---------|---------|
| `host` | `http://localhost:11434` | Ollama base URL |
| `auth.token` | unset | optional Bearer token for remote hosts |
| `default_model` | first entry of `/api/tags` | model auto-selected for agent chat |
| `theme` | `dark` | `dark` or `light` |
| `agent.temperature` | `0.7` | chat sampling |
| `agent.top_p` | `0.9` | nucleus sampling |
| `agent.num_ctx` | `4096` | context window |
| `agent.system_prompt` | built-in default | agent persona/system instructions |
| `agent.workspace_root` | cwd at launch | project root the agent operates on |

### Ollama client (`internal/ollama`)
Thin typed wrapper over the REST API:
- `GET /api/tags` → model list (name, size, digest, parameter_size, quantization_level, modified_at)
- `POST /api/show` → full model/params/options/template details *(POST, not
  GET — the actual API; see LEDGER 2026-09-03)*
- `POST /api/pull` (stream) → `{status, digest, total, completed}` progress events
- `DELETE /api/delete` → remove a model
- `POST /api/chat` → supports `tools`, `stream` for the agent tool loop

### Model list item
```go
type Model struct {
    Name          string
    ParameterSize string // e.g. "7B"
    Quantization  string // e.g. "Q4_K_M"
    Family        string // e.g. "llama"
    SizeBytes     int64
    ModifiedAt    time.Time
}
```

---

## 6. The AI coding agent (the hard part)

Architecture: **Ollama native tool/function calling** + an async tool-execution loop in a
background goroutine. No external agent framework.

> **Spike-verified (M0a, 2026-09-03):** native `message.tool_calls` work only on some
> installed models. **`qwen3:8b` is the confirmed tool-capable default** (full 2-step PASS,
> `tools`+`thinking` capabilities). `qwen2.5-coder:14b` streams the call as JSON in
> `message.content` with `tool_calls` empty, and `gemma3:12b` rejects tools (HTTP 400).
> **Therefore the tool layer must support two dispatch paths:** (a) native
> `message.tool_calls`, and (b) detect → parse → validate a
> tool-JSON emitted in `content`, feeding the same executor. qwen3 also emits a `thinking`
> phase — the runner must consume/ignore reasoning messages so they don't corrupt the tool
> loop or get shown as final output. This makes the "explicit fallback, never silent" owner
> decision an **empirical requirement**.

### Tool loop (state machine in `internal/agent/runner.go`)
```
start
 ├─ 1. Build messages = system_prompt + conversation + tool results
 ├─ 2. POST /api/chat with { model, messages, tools, stream }
 ├─ 3. Model responds:
 │     • finish_reason = "stop", content only → complete, emit final message
 │     • tool_calls present (native OR content-embedded) → validate → execute → append result → loop to 1
 │     • error / exhausted iterations          → surface message, fail gracefully
 └─ stop (hit max_tool_iterations, default 12)
```

### Execution of tool calls (everything async, in goroutine, blocking-safe ops)
| Tool | Signature | Notes |
|------|-----------|-------|
| `read_file` | path | assessor-resolved within workspace root |
| `write_file` | path, content | atomic write, no overwrite unless `overwrite:true` |
| `edit_file` | path, old, new | exact-match replace, verify applied |
| `list_dir` | path | shallow dir listing |
| `grep` | pattern, path | rg-backed over project |
| `run_command` | argv, timeout | **v1: argv allowlist, no shell/interpreter**; scrubbed env, resource/output limits, process-group kill, per-call confirmation. General shell = labeled dangerous opt-in behind a real OS/container sandbox; otherwise disabled. Streaming stdout/stderr; cancellation. |

### Safety rules
- All file access **jailed to `workspace_root`** (resolve symlinks, reject `..` escapes).
- **These are guardrails, not a sandbox.** A workspace cwd + timeout + denylist do *not* stop a
  command from reading credentials, hitting the network, writing absolute paths, spawning
  children, or escaping via interpreters. Per council audit (finding A): v1 `run_command` is
  an **argv allowlist with no shell/interpreter**, scrubbed env, resource + output limits,
  process-group kill, cancellation, and per-call confirmation. If real isolation is
  unavailable, `run_command` is **disabled by default / dropped**. Cwd confinement alone is
  insufficient.
- The safety controls for each tool ship **inline with that tool** in its milestone (not
  deferred to a final hardening milestone).
- Max tool iterations and max tokens bound each run.

### Context budgeting (`internal/agent/context.go`)
Track approximate token count per message; when `num_ctx` is approached, truncate the
oldest assistant/user tool-result pairs (summarize->drop strategy) to keep the window
bounded.

### Streaming to UI
The runner emits an ordered stream of `tea.Msg`s:
- `TokenMsg{ text }` — streamed tokens append to the live transcript
- `ToolStartMsg{ name, input }` — show "⚙ running read_file …"
- `ToolResultMsg{ ok, summary }` — show result (truncated) inline
- `AgentDoneMsg{ err }` — finalize
The UI just reacts to messages; it never runs the loop.

---

## 7. Views & responsive layout (PC vs iPhone)

### Root: tab bar + active view
`Models | Agent | Settings` navigation via `<1>/<2>/<3>` and `tab`, mirrored in a
status bar. Works at any width.

### Breakpoint policy (`internal/ui/layout.go`)
Lip Gloss reacts to `tea.WindowSizeMsg`. Define two layouts:

| Breakpoint | Portrait iPhone 16 Pro (≈ 88 cols) | Wide PC (≥ 120 cols) |
|-----------|------------------------------------|----------------------|
| **Models** | list fills width; detail pane stacked below & toggled | list (40%) + detail pane (60%) side-by-side |
| **Agent** | chat fills width; input full-width at bottom; file tree hidden behind a fold key | chat (flex) + file tree panel (right) + input |
| **Settings** | single-column form, scrollable | two-column form sections |

Everything uses the **same styles/theme objects**; only *layout/geometry* differs by
width. This keeps the "iPhone compatible" promise cheap and tested.

### Theme
Lip Gloss palette centralized in `styles.go`, supporting `dark` (default) and `light`.
Focus on contrast so it reads on the smaller mobile display in both light & dark
terminal themes.

---

## 8. Settings panel

Built with **Huh** (form library). One form per section:
1. **Connection** — host base URL, auth token (masked)
2. **Model defaults** — default model, temperature, top_p, num_ctx
3. **Theme** — dark/light live toggle
4. **Agent** — system prompt editor, workspace root, max tool iterations

Each save writes config back to the config file (and supports an in-session live apply).

---

## 9. Dependencies (pin as one compatible set at impl time)

| Package | Purpose |
|---------|---------|
| `github.com/charmbracelet/bubbletea` | TUI engine |
| `github.com/charmbracelet/lipgloss` | styling/layout/width    |
| `github.com/charmbracelet/huh` | settings forms |
| `github.com/charmbracelet/bubbles` | reusable components (list, table, textarea) |
| `github.com/charmbracelet/log` | structured logs (**to a file**, not stderr — stderr corrupts the alt-screen over SSH) |
| `github.com/adrg/xdg` | config file path resolution |
| `github.com/charmbracelet/glamour` | markdown rendering for agent/chat output |

**Pin Choice (council finding G):** select **Charm v1 or v2 as one aligned set** across
bubbletea/lipgloss/huh/bubbles/glamour *after a compile spike* — never mix majors.
Also resolve: `rg` runtime prerequisite vs pure-Go grep (see Risks #3/#4), and pick a
TOML parser for the config file (`github.com/pelletier/go-toml/v2`).

---

## 10. Implementation roadmap — re-cut into ship gates (council audit, converged)

Re-cut per `COUNCIL-MEMO.md`. Principle: **each milestone is a ship gate**, safety
controls ship inline with the tool that exposes them, and the **agent scope is gated on
an evidence spike — model management + plain chat are not.**

**M0 — Repo + skeleton + minimal config.** ✅ *done 2026-09-03 — see LEDGER* standalone git repo at `/home/calvin/SelfTUI`
(dotfiles ignores it by default, so no `.gitignore` step needed there); `go.mod`, Charm
dependency set pinned; root model + tab/status bar;
config load; **responsive shell + breakpoint system at measured sizes** (not an assumed
88-col); cancellation plumbing. ✅ *Exit: app boots, tabs work, clean build, Charm set compiles.*

**M0a — GATE: measurement + technical/security/device spikes** ✅ *done 2026-09-03 —
GO, evidence in `docs/m0a-gate-evidence.md` + LEDGER*. Measure real `WindowSizeMsg`
width/height, resize, key delivery, color, scrolling, reconnect on the actual SSH
client(s). **Spike 1:** Ollama tool-calling on named release-target models — streamed arg
assembly, malformed/parallel calls, multi-tool-turn correlation, retries, cancellation,
context exhaustion, iteration exhaustion. **Spike 2:** `run_command` containment design
(argv-allowlist, no-shell, scrubbed env, limits, process-kill). ✅ *Exit: predefined
go/no-go gate passes = one validated target workflow + supported model/tool-loop
compatibility + command containment + observed mobile usability.* — **All four items
passed 2026-09-03:** measured Moshi (owner client, iPhone 16 Pro) 72x30 portrait /
truecolor / key delivery (`cmd/size-probe` + `scripts/probe-local.sh` harness 5/5);
`qwen3:8b` native tool loop PASS (spike 1 + OD3); containment design in
`docs/run-command-containment.md`; verdict **GO**. Residuals carried: landscape
geometry, reconnect, height-aware layout, scrolling.

**M1a — Model list/show.** ✅ *done 2026-09-03 — live smoke on the local host (qwen3:8b et al.):* ollama client `tags`/`show`; `internal/ollama` (client.go/tags.go/show.go + 11 tests); Models view selection + inspect pane in `internal/ui/models_view.go` (bubbles v2 list, theme-matched), stacked on compact (enter-toggled, u/d scroll) and auto-inspect side-by-side on wide; `charm.land/bubbles/v2` added to the pinned set. ✅ *Exit: list + inspect models live.*

**M1b — Delete + streamed pull.** `DELETE` w/ confirm; streaming `pull` w/ spinner +
progress. ✅ *Exit: delete & pull work live. → **ALPHA candidate 1.***

**M2 — Chat + plain-chat path.** non-tool streaming chat in Agent view, glamour markdown
rendering + syntax-highlight code blocks, input, model selector, graceful errors;
no-tool-model → explicit fallback (never silent). ✅ *Exit: chat renders nicely.*
→ **Ship ALPHA** (reliable model mgmt + plain chat) at both narrow & wide geometries.

**M3a — Read-only agent.** the tool-loop state machine; capability check;
read_file/list_dir/grep; bounded loop; streaming `Msg`s; cancellation. ✅ *Exit: agent
reads/lists/greps a project live; no mutation surfaced.*

**M3b — Mutation agent (jailed).** write_file/edit_file + `run_command` per constrained
design (argv-allowlist, no shell) with **jail + confirm + timeout + cancel + tests
inline**; context budgeting; job serialization vs Ollama. ✅ *Exit: agent writes/edits
files and runs allowed commands, all gated + tested.*

**M4 — Settings & persistence.** huh forms wired to config file + env + flags; theme
toggle; live-apply where cheap. ✅ *Exit: settings persist & revert.*

**M5 — Responsive completion + iPhone path.** golden render tests at two widths; README
SSH-on-iPhone guide (Blink/Termius); light theme; mobile approval ergonomics. (Much
responsive shell already landed in M0.)

**M6 — Release acceptance.** unit+golden tests throughout; auth/TLS; error surfacing;
context-truncation edges; binary/reconnect smoke test; docs; release acceptance.
*(Hardening was pushed inline into each tool's milestone, so M6 is acceptance, not the
first safety gate.)*

---

## 11. Risks & open questions

| # | Item | Notes / needed decision |
|---|------|--------------------------|
| 1 | **Git ownership** ✅ *decided + set up* | Standalone repo at `/home/calvin/SelfTUI`; `git init` done (dotfiles `.gitignore` uses `*` default-ignore, so no extra step was needed). |
| 2 | **Markdown rendering** ✅ *decided* | Use `glamour` for GitHub-flavored markdown. |
| 3 | **Agent tool breadth** | "Full coding agent" is large. v1 tool set is bounded by the read-only (M3a) then mutation (M3b) split. Confirm whether git-awareness/project-indexing/multi-file apply belong in v1 or later. |
| 4 | **Coding model + dispatch** | *OD3 resolved:* default agent model = **`qwen3:8b`** (native tool PASS). Dual dispatch (native `tool_calls` + content-embedded tool-JSON) stays required for pick-any-model (`qwen2.5-coder` content-JSON; `gemma3` → 400 → explicit non-agent fallback). Agent loop must handle qwen3 `thinking` phase. |
| 5 | **Remote host auth + job serialization** | Basic/bearer depends on what the remote exposes — verify the concrete setup. *Owner decision:* serialize Ollama jobs (no pull during agent) vs allow overlap on one GPU. |
| 6 | **iPhone terminal width** | **Measure**, don't assume 88-col. Confirm actual cols/rows + key behavior + reconnect **per Moshi (owner's client; Blink/Termius/iSH similar)** during M0a, and drive breakpoint ranges from that measurement. *M0a update:* measurement instrument = `cmd/size-probe` (+ `make probe-local` harness); owner device run records into `$XDG_STATE_HOME/selftui/probe.txt`.
| 7 | **Charm version set** | *Owner decision:* pick **v1 or v2 as one aligned set** (bubbletea/lipgloss/huh/bubbles/glamour) after a compile spike; never mix majors. |
| 8 | **Concurrency + model structure** | *Owner decision:* activity channel (resubscribed `tea.Cmd`) vs `tea.Program.Send`; token coalescing + context cancel; nested per-view `tea.Models` vs god `Update`. Ordered events + nonblocking `Update` required; race/teardown tests. |
| 9 | **Grep dependency** | *Owner decision:* declared `rg` runtime prerequisite vs pure-Go grep. *Pure-Go preferred* to preserve the single-binary claim unless `rg` speed is required. |
| 10 | **Logging transport** | Log **to a file** — stderr corrupts the alt-screen over SSH. |
| 11 | **M0a go/no-go gate** | *Owner defines* the gate: one validated target workflow, supported model/tool-loop compatibility, command containment, observed mobile usability. |
| 12 | **Project ledger** | `LEDGER.md` is the persistent work/decision log for session continuity — consult & append each session. See §13. |

---

## 12. Next step

**M1a — model list/show landed 2026-09-03** (live list + inspect on the local
host; compact 72x30 and wide 120x40 smokes).
Next milestone: **M1b — delete + streamed pull**: `DELETE /api/delete` with a
confirmation; streaming `POST /api/pull` (`{status,digest,total,completed}`
progress events) with a spinner + progress bar in the Models tab; spinner
component; errors and cancellation surfaced. → **ALPHA candidate 1** after M1b.

---

## 13. Project ledger (`LEDGER.md`)

`LEDGER.md` is the persistent work/decision log that keeps sessions efficient and
high-performant. It is the **past-facing** record (what happened, why, what you hit, what
happens next) that complements `PLAN.md` (future-facing) and `COUNCIL-MEMO.md` (this

---

## 14. Session rule (binding)

**One fresh session per step.** A "step" = one milestone (`§10`) or one owner-assigned task.
Never chain a second step in the same session. Session-start ritual: `AGENTS.md` →
`PLAN.md` §10+§11 → tail of `LEDGER.md` → choose exactly one step. Session-end ritual:
complete it → append `LEDGER.md` → tick the `§10` exit → commit → **stop**. The full rule
and the self-hosted tooling reality notes are in `AGENTS.md`.
