# SelfTUI — Project Plan

> **SelfTUI** = **Self**-hosted + **TUI** (and "self" in the autonomous sense — it's an
> embedded coding agent). A visually appealing, responsive TUI to manage local/remote
> Ollama models, with an embedded AI coding-agent chat and a settings panel. Runs
> identically on PC (native terminal) and iPhone 16 Pro (SSH into the host, responsive to
> narrow screens).

> **Status (2026-09-06):** **v0.1.0 released 2026-09-04** (tag `v0.1.0`);
> **v0.1.1 audit-remediation hardening in progress** on
> `fix/v0.1.1-audit-remediation` (§12 has the current next action).
>
> **Release-hardening correction (2026-09-04, owner task — phase 7 of the v0.1
> hardening plan).** The public **v0.1 product contract** is: a **single-process
> Linux/WSL TUI for Ollama** (native Windows/macOS not supported); **chat is
> in-memory per process** — the per-process Markdown transcript under the XDG
> state dir survives exit as an append-only export but **cannot be resumed**
> (no reload/import path in v0.1); **workspace tools are off by default** and
> require an explicitly trusted project workspace root; **command execution is
> not shipped**; a bearer token on a **non-loopback host requires `https://`**.
> Planning-era prose above and milestone rows below predate this note: they
> are **historical records**, and where they conflict with this contract the
> contract wins. See `CHANGELOG.md` (`[v0.1.0] - 2026-09-04`) and the dated
> phase-7 `LEDGER.md` entry.

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
- **Repository:** standalone git repo (created 2026-09-03; rename/setup record
  in LEDGER). It lives outside the dotfiles tree, so the code never pollutes it.

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
          │  │  pull stream, │   │  read/grep/write/edit    │  │
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
| `agent.max_tool_iterations` | `12` | upper bound on tool calls per run |
| `agent.workspace_root` | cwd at launch | project root the agent operates on |
| `tools_enabled` | `false` | **Phase 4 (workspace tool trust):** arms the jailed workspace tools. Off = plain chat (the Ollama request carries no tools). Enabling requires `workspace_root` to be a real project directory — empty, `/` or the user's home directory is rejected. Sources: config file, `SELFTUI_TOOLS_ENABLED` (strconv.ParseBool), Settings → Agent → *Enable workspace tools*. |

### Ollama client (`internal/ollama`)
Thin typed wrapper over the REST API:
- `GET /api/tags` → model list (name, size, digest, parameter_size, quantization_level, modified_at)
- `POST /api/show` → full model/params/options/template details *(POST, not
  GET — the actual API; see LEDGER 2026-09-03)*
- `POST /api/pull` (stream) → `{status, digest, total, completed}` progress events
- `DELETE /api/delete` → remove a model
- `POST /api/chat` → supports `tools`, `stream` for the agent tool loop

**Confirmed stream behaviors (M1b, live-verified 2026-09-04):**
- Pull **errors arrive in-band as `{"error": …}` lines with HTTP 200** — stream
  clients must check the `error` field, not just the status code.
- Ollama **registers a model in `/api/tags` at ~50% of the download** — tag
  presence is NOT a pull-completion signal; trust the stream's `success` line.
- Pulls can run minutes: they use a client **without the 30s request timeout**
  (caller context = deadline); list/show/delete stay on the 30s client.
- **Streams are bounded (Phase 5, hardening):** chat and pull decode through
  the shared NDJSON reader (`internal/ollama/stream.go`) that caps one event
  at 4 MiB on the wire (`stream event exceeds 4194304 bytes`) and aborts a
  body silent for the 90s idle window (`stream idle timeout`). The timeout is
  idle-only — there is deliberately **no total request deadline**, so pulls
  keep running for minutes while progress lines keep arriving, and caller
  cancellation always wins over the idle watchdog. Chat additionally caps
  cumulative **raw** NDJSON bytes at 16 MiB (`chat stream exceeds 16777216
  bytes`) — counting JSON framing, content, thinking, and tool calls, so
  decoded tool arguments cannot slip past the cap (H-03); EOF before a
  terminal `done`/`success` event stays an error. The
  idle window defaults to 90s per client and is injectable in tests
  (`Client.streamIdle`); public `Client`/`Chat`/`ChatStream`/`Pull`
  signatures are unchanged.

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
 ├─ 0. Tools disabled (no ToolPolicy / NewRunner): plain chat only — the
 │     request carries no tools field and nothing can execute.
 ├─ 1. Build messages = system_prompt + conversation + tool results
 ├─ 2. POST /api/chat with { model, messages, tools, stream }  (armed runner)
 ├─ 3. Model responds:
 │     • finish_reason = "stop", content only → complete, emit final message
 │     • tool_calls present (native OR content-embedded) → validate → execute → append result → loop to 1
 │     • error / exhausted iterations          → surface message, fail gracefully
 └─ stop (hit max_tool_iterations, default 12)
```

### Execution of tool calls (everything async, in goroutine, blocking-safe ops)
| Tool | Signature | Notes |
|------|-----------|-------|
| `read_file` | path | assessor-resolved within workspace root; policy-gated |
| `write_file` | path, content | atomic write, no overwrite unless `overwrite:true`; policy-gated before confirmation |
| `edit_file` | path, old, new | exact-match replace, verify applied; policy-gated before confirmation |
| `list_dir` | path | shallow dir listing; policy-gated |
| `grep` | pattern, path | rg-backed over project; policy-gated |
| `run_command` | argv, timeout | **V2c (v0.2):** bwrap-sandboxed allowlisted `go`/read-only `git`; no shell/interpreter; scrubbed env; timeout/output caps; process-group kill; single-flight; per-call confirmation. See `docs/run-command-containment.md`. v0.1 did not ship this tool. |

### Safety rules
- **Workspace tools are opt-in (Phase 4).** `NewRunner` (the compatibility
  constructor) sends no tool definitions and behaves as plain chat;
  production arms the tools through `NewRunnerWithPolicy` with a
  `ToolPolicy`, driven by `config.ToolsEnabled` (default `false`).
- All file access **jailed to `workspace_root`** (resolve symlinks, reject `..` escapes).
- **Sensitive-path policy (`ToolPolicy.AuthorizePath`), applied before every
  tool executes on top of containment (Phase 4):** a requested path
  containing a component named `.ssh`, `.gnupg`, `.aws`, `.azure`, `.kube`,
  or the `.config/gcloud` composite, or whose basename is `.env`/`.env.*`
  (except `.env.example`), `credentials`, or `credentials.json` is refused
  outright — credential trees are never worth touching even when they
  resolve inside the workspace.
- Enabling tools with `workspace_root` empty, `/`, or the user's home
  directory is rejected at config validation: a model with file tools must
  not be pointed at the whole filesystem or at the directory adjacent to
  the user's credentials.
- The UI shows the canonical workspace and `tools off`/`tools on` in the
  status bar and Agent statusline; tools enabled against a non-loopback
  host shows a persistent warning that workspace content may be sent to
  that host. No onboarding wizard in v0.1.
- **V2c command safety:** `run_command` adds a bubblewrap OS boundary to the
  existing guardrails: selective read-only system/toolchain/module-cache
  mounts, workspace-only host write access, private `/tmp`, no network,
  scrubbed environment, and parent/descendant teardown. Bubblewrap has no
  CPU/memory cap, so the residual WSL2 host-OOM risk remains documented in
  `docs/run-command-containment.md`; timeout (30s default/60s cap), output
  caps, serialization, process-group kill, argv allowlist, and confirmation
  are mandatory and tested. v0.1 intentionally omitted command execution.
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

> **Phase 6 (2026-09-04): every async child result crosses the App shell in
> one envelope per child.** All asynchronous results — the Agent's model-list
> fetches and every chat activity-channel event, and the Models tab's
> list/show/delete/pull results plus its dialog spinner ticks — are produced
> wrapped in `agentEventMsg`/`modelsEventMsg` (payload `tea.Msg`).
> `App.Update` has exactly one case per child (unwrap + delegate), so a newly
> added async result can never be dropped at the shell again (the
> 2026-09-06 `ToolConfirmMsg` routing bug). Root-owned messages
> (`settingsThemeMsg`, `agentThemeMsg`, `settingsSaveDoneMsg`, `WindowSizeMsg`,
> `KeyMsg`) stay root messages.

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

**M0 — Repo + skeleton + minimal config.** ✅ *done 2026-09-03 — see LEDGER*
standalone git repo (setup record in LEDGER 2026-09-03); `go.mod`, Charm
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

**M1b — Delete + streamed pull.** ✅ *done 2026-09-04 — live smoke on the local host:* `DELETE` w/ confirm (x → y/esc, error-inline retry); streaming `POST /api/pull` (in-band errors, context-cancel via esc) with bubbles spinner + progress bar + name input; activity-channel plumbing (resubscribed `waitPullCmd`); modal guard so digits in names can't trigger tab jumps; `scripts/pull-delete-smoke.py` + `make smoke` pty harness; 55 tests green. ✅ *Exit: delete & pull work live. → **ALPHA candidate 1.***

**M2 — Chat + plain-chat path.** non-tool streaming chat in Agent view, glamour markdown
rendering + syntax-highlight code blocks, input, model selector, graceful errors;
no-tool-model → explicit fallback (never silent). ✅ *Exit: chat renders nicely.*
→ **Ship ALPHA** (reliable model mgmt + plain chat) at both narrow & wide geometries.

**M3a — Read-only agent.** ✅ *done 2026-09-03:* tool-loop state machine with
capability probe + explicit plain-chat fallback; native and content-embedded tool
calls; qwen3 thinking suppression; jailed read_file/list_dir/grep; bounded loop;
streaming `Msg`s; cancellation. **Exit: agent reads/lists/greps a project live; no
mutation surfaced.**

**M3b — Mutation agent (jailed).** ✅ *done 2026-09-05:* jailed atomic
`write_file`/exact `edit_file`, plus confirmed `run_command` constrained to approved
`go` and read-only `git` argv (no shell/interpreters), scrubbed environment, 30s
default/60s cap, 256 KiB per stream, process-group cancellation, serialization,
context budgeting, and focused UI/executor tests. ✅ *Exit: agent writes/edits files
and runs allowed commands, all gated + tested.* *(Historical milestone record:
`run_command` — including the read-only `git` argv — was removed from v0.1 on
2026-09-03; see the removal note below and the product contract at the top.)*

> **2026-09-03 — v0.1 hardening: command execution removed.** Public v0.1 does not
> expose or retain `run_command`. The executor, its tool schema, and its dispatch case
> were deleted; `ToolOutputMsg` and the command-only UI routing went with them. v0.1
> ships project-aware `read_file`/`list_dir`/`grep` plus confirmed `write_file`/
> `edit_file` only. cwd + argv filtering is not an OS sandbox — see
> `docs/run-command-containment.md`, now a dated deferred-design record.

**M4 — Settings & persistence.** ✅ *done 2026-09-06:* huh (v2-aligned `charm.land/huh/v2 v2.0.3`) forms over the full config surface in four sections (Connection / Model defaults / Theme / Agent), two-column on wide screens (LayoutColumns), single-column pages otherwise; field validation (http(s) host, numeric ranges); esc-discard = nothing written (revert); submit writes the config file off-loop (`config.Save`, 0600 perms) and live-applies in-session — theme swap restyles the whole shell, host/token rebuild the Ollama client and reload both model lists, and agent params/default model/workspace/system prompt apply to the next agent run; Theme select previews live while arrowing (rolls back on discard). New flags/env for every config key. **Exit: settings persist & revert.**

**M5 — Responsive completion + iPhone path.** ✅ *done 2026-09-06:* golden render tests pin the full shell at the two canonical geometries — the measured Moshi portrait device (72×30, `docs/m0a-gate-evidence.md`) and a wide PC window (120×40) — across seven scenarios (models list/inspect compact & wide, agent, settings editing single- & two-column), fixtures in `internal/ui/testdata/golden/` (regenerate with `go test ./internal/ui -run TestGoldenRender -update`); every frame is guard-asserted to stay inside its terminal (no row wider than the terminal, no view taller than the screen, frame-filling views land on exactly h rows). Approval ergonomics: every overlay body (agent confirm, models delete/pull/input) is height-capped via `fitContent` so a multi-KB mutation payload can no longer push the `y/enter approve` decision row — or the status bar — off a 30-row phone screen; a middle “… (N more lines)” marker shows what was cut. Settings form height budget reserves huh's footer row (was 1 row over at 72×30). Light theme: verified as a genuinely different palette (fg/bg/accent/error) with light renders across every tab at both geometries and a light-glamour chat round-trip; active-tab chip in light now uses light text on the violet accent for contrast. README ships the SSH-on-iPhone guide (Blink/Termius; measured geometry, compact behaviors, esc/key tips, remote-host config). ✅ *Exit: golden renders at both widths; README iPhone guide; light theme contrast-checked; approval dialogs bounded.*

**M6 — Release acceptance.** unit+golden tests throughout; auth/TLS; error surfacing;
context-truncation edges; binary/reconnect smoke test; docs; release acceptance.
✅ *done 2026-09-03 — evidence in docs/reconnect.md + LEDGER.*
**Reconnect semantics resolved live** on the owner's actual Moshi/iPhone 16 Pro
client: the owner runs SelfTUI inside **tmux over SSH**, so a phone drop =
**tmux reattach** — the app process survives (verified: pid survived the drop,
"same screen" observation, app log shows no shutdown) and the host Ollama
recovers (tags 200 in 1.4 ms). The **fresh-SSH death** shape (plain ssh: drop
→ SIGHUP → process dies; config persists, chat is per-process) is covered
deterministically by the new `make smoke-reconnect` harness
(`scripts/reconnect-smoke.py`): 72×30 boot, mid-generation drop → SIGHUP
death (rc=-1), host recovery (generation round-trip 0.1 s), clean fresh
reconnect with config re-applied — PASS ×3. *(Chat is per-process: the live
conversation lives in the process; since the 2026-09-06 persistence task,
committed turns are also mirrored to a per-process session file under the
XDG state dir so a dropped/quit process leaves a recoverable transcript.)* `cmd/size-probe` extended with
session headers (pid + `-session` tag) and checkpoints (`c` / SIGUSR1) so
probe.txt is attributable session blocks. Auth/TLS: bearer-token tests on the
stream endpoints and real-TLS tests (trusted handshake over https on JSON +
stream endpoints; untrusted cert rejected). Error surfacing: settings save
failure now tested end-to-end (read-only dir → error panel with cause → retry
after fix succeeds). Context-truncation edges: fixed + tested — a giant *first*
message is now bounded (was sent raw past numCtx), the plain-chat fallback
honors the budget, repeated budgeting inserts the marker exactly once,
tool-call args count toward the budget. Bug found by the smoke: **digits typed
in the Agent chat input switched tabs mid-prompt** — fixed: digits jump only
from Models / an empty chat input (regression test). Dead `compactToolResult`
helper removed. `selftui -version` prints `0.6.0-m6` and is logged at startup.
`make check` + `go test -race` green; README ships the reconnect guidance.
*(Historical record: that version string is what the M6 build printed; current
builds default `Version` to `dev` — see the 2026-09-04 product-contract note.)*
*(Hardening was pushed inline into each tool's milestone, so M6 is acceptance, not the
first safety gate.)*

**M7 — UX polish, pre-v0.1 (owner scope 2026-09-03; opencode.ai TUI as the
reference for feel).** ✅ *done 2026-09-06 — evidence: 40+ new M7 tests in
`internal/ui/m7_test.go`, 10 new golden frames (17 total) at 72×30/120×40,
`make check` + `go test -race` green.*
- **A — Composer + commands:** slash-command menu over the Agent input with a
  live filter (`/clear` with y/n confirm, `/model`, `/theme`, `/help`,
  `/refresh`); unmatched slash drafts are ordinary prose; `esc` clears a
  drafted prompt when idle; the input placeholder reads “/ for commands”; a
  `ctrl+p` command palette from any tab (go to tab, change model, clear
  conversation, toggle theme, refresh models, command list) with live filter;
  session-scope theme actions show a status-bar toast cleared on the next
  key. `esc`-while-draft, palette modal guard, and composer empty-input
  letter-command rules all preserved (digit typing tests still pass).
- **B — Transcript feel:** stable per-block headers with the model chip,
  streaming caret “▍” that disappears at rest, per-turn meta right-aligned on
  the assistant header (elapsed + terminal reason — `· stop`/`· length`/
  `· stopped` — surfaced through `AgentDoneMsg.Reason`), `pgup`/`pgdn`
  paging, and an `f` auto-follow toggle (d/pgdn back to the tail re-engages
  follow).
- **C — Context meter + picker upgrade:** live meter (`ctx ▓▓░░░ 38%`) in the
  Agent hint row over system+history+draft, reusing `agent.ApproxTokens`;
  red at 100% with a visible truncation marker in the transcript head until
  `/clear`; model picker filters as you type (name/family/size/quant,
  j/k+arrows nav) and stars the configured default model.
Constraints respected: every overlay and menu is height-capped/width-fitted
(fitContent + renderCenteredOverlay + wrapLines now ANSI-width aware; guard
frame rows ≤ terminal at both geometries), letter commands are empty-input
only, digits are text while composing, phone-first compact geometry held.
*Owner decisions recorded in LEDGER: palette/slash keybind set (ctrl+p on
any tab + “/” menu as the phone path) and the meter lives in the Agent hint
row, not the shared status bar.* → **v0.1 next (tag + release notes).**

**M7 follow-up — opencode composer (owner task 2026-09-06, after M7).**
Re-shaped the Agent tab's chat box toward the opencode.ai TUI after a live
reference pass over its footer source: the bottom is now one **composer
block** (header row: model chip left, live ctx meter + numeric token usage
right; auto-growing prompt 1–4 rows) with an opencode-style **statusline**
under it (running state · armed interrupt, key legend); per-turn meta moved
from a footer row onto the assistant header's right side; `esc` while
running is now an **armed interrupt** (first esc warns “esc again to
interrupt”, second cancels). Goldens grew to 19 frames; `make check` +
`go test -race` green. *Still next: v0.1 (tag + release notes).*

**M7 follow-up 2 — chat-session persistence (owner task 2026-09-06).**
Chat stays in-memory, but committed turns now mirror to a per-process
markdown transcript (`internal/session`, lazy open on the first message):
files under `$XDG_STATE_HOME/selftui/sessions/chat-<ts>-<pid>.md` (0600),
`## user/assistant (model) · time · meta` blocks with raw content, so a
conversation survives exit and is inspectable (the owner asked for this so
past chats are recoverable). New `/save` slash command flushes + reveals the
path; `SELFTUI_SESSION_DIR` overrides the dir, `SELFTUI_NO_SESSION=1`
disables; errors disable once with one notice. *(Historical record: the
`/save` command was renamed `/export` in the 2026-09-04 release-hardening
pass; the transcript remains an append-only export that cannot be resumed.)*
Slash menu grew to six
commands (menu cap 6). Goldens 19 frames; `make check` + `go test -race`
green. *Still next: v0.1 (tag + release notes).*

**v0.2 roadmap — scoped 2026-09-07 (owner-selected, small focused release).**
Sequential ship gates, one per session per the binding session rule; the v0.2
tag cuts when the owner-selected set lands (owner may cut earlier).

**V2a — Chat session resume.** ✅ *done 2026-09-07 — see LEDGER:* Reload a saved per-process transcript
(`internal/session` markdown under `$XDG_STATE_HOME/selftui/sessions/`) into a
live Agent conversation: picker over saved sessions, safe import (tool-armed
runner state, context budgeting re-applied on load), meta/model handling,
tests. ✅ *Exit: a saved chat resumes live at both canonical geometries.*

**V2b — GATE: sandbox spike.** ✅ *done 2026-09-07 — verdict **GO**, evidence in
`docs/v2b-sandbox-gate-evidence.md` + LEDGER (independent reality-checker audit
reproduced 4/4 load-bearing probes, ENDORSE GO).* Evaluated bubblewrap 0.9.0,
`systemd-run` slices, and rootless containers (Docker 29.6.0 **is** rootless on
the release host) against `docs/run-command-containment.md`. bwrap and rootless
Docker both pass the full threat-model matrix and the real offline `go test`
workload; `systemd-run` **fails** as a containment layer on this WSL2 host
(`IPAddressDeny` and `MemoryMax` silently unenforced — recorded host quirk).
GO is conditional on V2c shipping/testing the full mitigation stack (timeout,
output caps, serialization, process-group kill, argv allowlist, per-call
confirm); rootless Docker stays the documented opt-in engine (`--memory`
enforced; bwrap has no CPU/memory caps). ✅ *Exit: evidence doc + verdict GO →
V2c.*

**V2c — Sandboxed run_command (only on V2b GO).** ✅ *done 2026-09-07:* command
execution reinstated behind bubblewrap (default engine) plus the containment
design: argv allowlist, no shell/interpreter, scrubbed environment, private
filesystem/network boundary, timeout/output limits, single-flight
serialization, process-group kill, and per-call confirmation. Tests cover the
full mitigation stack and an offline `go test` workload inside bwrap. See
`docs/run-command-containment.md` and `docs/v2c-sandbox-evidence.md` plus the
V2c ledger entry. ✅ *Exit: agent runs allowed commands inside the sandbox, all
gated + tested.*

**V2d — Agent breadth.** Git-awareness / multi-file edits / project indexing
(risk #3); the exact cut is decided at that session's start after V2a–V2c.
✅ *done 2026-09-07 — cut owner-selected in-session: git-awareness + project
indexing (multi-file edits and mutation undo/redo stay out of this cut).*
Every armed agent turn now starts with a bounded workspace-context system
message (`internal/agent/workspace.go`): git branch/porcelain status/last 3
commits (fixed read-only host-side `git` argv, 3s timeout, omitted outside a
repo) plus a depth-4/entry-300 project index with `.git` pruned. Plain chat
never receives it; the closed tool schema is unchanged. ✅ *Exit: per its own
scoped exit criteria — context injected + bounded + tested (unit, git-repo,
boundedness, depth-cap, cancellation, wire-shape tests), `make check` and
`go test -race` green.*

---

## 11. Risks & open questions

| # | Item | Notes / needed decision |
|---|------|--------------------------|
| 1 | **Git ownership** ✅ *decided + set up* | Standalone repo; `git init` done (setup record in LEDGER 2026-09-03). |
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

**M1a — model list/show landed 2026-09-03**, **M1b — delete + streamed pull landed
2026-09-04**, **M2 — chat + plain-chat landed 2026-09-03**, **M3a — read-only agent
landed 2026-09-05**, **M3b — jailed mutation agent landed 2026-09-05**,
**M4 — Settings & persistence landed 2026-09-06**, and
**M5 — Responsive completion + iPhone path landed 2026-09-06** (golden render
fixtures at 72×30 and 120×40 across all three tabs, height-capped approval
dialogs, light-theme verification, README SSH-on-iPhone guide).
**M6 — Release acceptance landed 2026-09-03**: reconnect semantics resolved live
on the Moshi/iPhone 16 Pro client (tmux-over-SSH reattach is the owner's
transport; fresh-SSH death covered by the local smoke), `make smoke-reconnect`
harness, size-probe session/checkpoint instrumentation, auth/TLS acceptance
tests, settings save-error + retry test, context-truncation edge fixes
(single-huge-turn bound, plain-chat fallback budget, marker idempotence, tool
args counted), digit-tab-jump bug fix + regression test, `-version` flag,
release docs (`docs/reconnect.md`, README). `make check` and `go test -race`
green.
**V2c — sandboxed `run_command` landed 2026-09-07:** bwrap is the default
and fail-closed engine for the allowlisted, confirmed `go`/read-only `git`
command tool; the workspace is the only writable host mount, network is
unshared, environment is scrubbed, and timeout/output/serialization/group-kill
mitigations are tested. Residual bwrap memory/CPU risk and the validated
rootless-Docker alternative are documented in `docs/run-command-containment.md`.
**M7 — UX polish landed 2026-09-06** (opencode.ai TUI as the feel reference):
slash-command menu + `ctrl+p` palette (A), transcript feel — caret, turn
footers with elapsed + stop reason — later moved onto the assistant header's
right side when the opencode-composer follow-up landed (see §10 M7 note),
pgup/pgdn + `f` follow (B), context meter
+ filter-as-you-type model picker with the default starred (C); `make check`
and `go test -race` green, 10 new golden frames (17 total) at 72×30/120×40.
**v0.1 hardening phases 1–7 landed 2026-09-03/04** on `hardening/v0.1`
(root cancellation, command execution removed, config validated + atomically
saved, explicit workspace tool trust, bounded streams, enveloped UI events,
product-contract docs) — see LEDGER for each dated phase entry.
**Phase 8 — reproducible CI + release gates landed 2026-09-04**: Makefile
targets (`race`, `vuln`, `release-check`, `build-linux-amd64`/
`build-linux-arm64` with `CGO_ENABLED=0`/`GOOS=linux`/`-X main.Version`),
`scripts/release-check.sh` (full gate, never tags) + shared
`scripts/verify-binary-version.sh`, `.github/workflows/ci.yml` + `release.yml`
(pinned Go 1.27.1, govulncheck v1.7.0), and dependency bumps closing two
reachable advisories (goldmark v1.7.17 GO-2026-5320, x/text v0.39.0
GO-2026-5970). Two code-review lanes: 0 hard violations, findings fixed in
the separate commits listed in the LEDGER phase-8 entry; spec verdict
`V0_1_RELEASE_CANDIDATE_READY`. The v0.1.0 release then landed the same day
(runbook steps 4–6 below): tag `v0.1.0` + release notes, in a fresh session
per the runbook rule "do not tag in a hardening phase".
**Release runbook step 4 landed 2026-09-04**: `hardening/v0.1` merged to
`main` via **PR #1** (commit `1a45554`, merge commit, branch kept); CI
bring-up fixed three latent defects found by the first remote runs — job
`name:` used the `env` context (invalid in GitHub's parser; both workflows
fixed to static names in `9bd1e0c`), a same-clock-tick transcript-file reuse
race in `internal/session` (`5ad160a`, O_EXCL + retry), and the settings-test
`drive` driver dropping any command slower than 100 ms — which silently lost
the off-loop config write (the "writing config…" flake family; fixed with
domain-driven hop waits in `settings_view_test.go`). Branch
protection enforced on `main` (required ci status check, PR flow,
`enforce_admins: true`, zero required approvals). Full local release gate +
CI both green on the merged commit (`VERSION=v0.1.0 make release-check`
PASSED, 0 vulnerabilities, binaries stamp `selftui v0.1.0`).
**Runbook step 5 landed 2026-09-04 — v0.1.0 published**: annotated tag
`v0.1.0` created and pushed on the `main` tip; the first `release.yml` run
**failed the gate** on a rare test-harness deadlock (see LEDGER 2026-09-04
step-5 entry) — `TestRunnerCancellationReturnsPromptly` parked its test
server handler forever (net/http arms client-close detection only after the
request body reaches EOF; the write-path drain is not guaranteed), hanging
the `internal/agent` race suite for the full 10-minute package timeout on the
2-vCPU runner. Root cause fixed in **PR #3** (`fix/cancel-test-hang`, merged
`c70bf89`): the four write-then-park test handlers now drain the request
body before parking, deterministically arming the background read. Flake
reproduced locally (`GOMAXPROCS=2 go test -race -count=200`) and proven
green with the fix; tag moved pre-release to `c70bf89`. Second `release.yml`
run **SUCCESS** — gate passed at the tag, release **SelfTUI v0.1.0** created
with both Linux archives + SHA256SUMS; assets independently downloaded and
verified (hashes OK, `selftui v0.1.0` stamps). CI green on merged main.
**Runbook step 6 landed 2026-09-04 — history audit clean**: gitleaks
**v8.30.1** (checksum-verified release binary, `~/go/bin/gitleaks`) over the
full reachable history (`--all --full-history`: 45 commits, all refs incl.
tag `v0.1.0` + `hardening/v0.1`) and the working tree found **0 leaks**;
unreachable objects (stash entries, superseded tag, orphan blobs) scanned as
supplementary evidence — also 0. `SECRET_HISTORY_SCAN=RESOLVED`. No
remediation or code changes needed.
**M-12 documentation alignment (runbook Task 19, 2026-09-06):** the changelog
cut landed — the `[Unreleased]` material became `## [v0.1.0] - 2026-09-04`
and a fresh `[Unreleased]` now carries the v0.1.1 audit-remediation work —
and `SECURITY.md`/`README.md` state the exact sensitive-path policy, stream
limits, and the v0.1.0-published/v0.1.1-hardening state (see the LEDGER
Task-19 entry).
**Task 22 — final release-candidate gate ✅ (2026-09-06):** `VERSION=v0.1.1 make release-check` PASSED (go mod verify, gofmt, vet, test, race, govulncheck v1.7.0, both Linux builds, version-stamp, deterministic archives, SHA256SUMS); `scripts/release-check-test.sh` 52/52 PASS; `scripts/create-audit-pack-test.sh` 42/42 PASS; `make audit-pack` produced manifest-complete `dist/selftui-audit-pack-9c5039f.zip` (107 tracked, 107 members, MANIFEST_MATCH=PASS); `scripts/create-audit-pack.sh verify` PASS. Full finding matrix at `dist/v0.1.1-finding-matrix.md`. All 22 external-audit findings (C-01 through L-02) remediated. **v0.1.1 is now ready for the owner to tag and publish.**

**P0 supply-chain — landed 2026-09-07**:
**gitleaks-in-CI** (added to `.github/workflows/ci.yml` + `make secret-scan`),
**actionlint in the local gate** (`make actionlint` + CI step),
**action SHA-pinning** (`actions/checkout` → `11d5960a326750d5838078e36cf38b85af677262`,
`actions/setup-go` → `40f1582b2485089dde7abd97c1529aa768e1baff`,
both workflows),
**timeout-minutes** (15 CI / 30 release),
**`.env*` in `.gitignore`**,
**`.gitleaks.toml`** and **`.actionlintrc`** config files.

**Owner decision (2026-09-07):** future release tags `v*` will be GPG-signed. Implementation (key setup + signing practice + doc note) is deferred to a follow-up step — this session only ships the P0 code + records the decision.
**The public-visibility decision stays the owner's call** — the repo
may now go public at the owner's discretion.

**P1 correctness** ✅ *all resolved* — findings #1–#7, #12, #13 from the
5-lane read-only audit were fixed in 9 commits on
`fix/v0.1.1-audit-remediation` (merged to main via PR #6). No remaining
correctness blockers.

---

## 13. Project ledger (`LEDGER.md`)

`LEDGER.md` is the persistent work/decision log that keeps sessions efficient and
high-performant. It is the **past-facing** record (what happened, why, what you hit, what
happens next) that complements `PLAN.md` (future-facing) and `COUNCIL-MEMO.md`
(the 2026-09-03 advisory audit that re-cut the roadmap; retained verbatim as
history — see the product contract at the top of this file for the current
line).

---

## 14. Session rule (binding)

**One fresh session per step.** A "step" = one milestone (`§10`) or one owner-assigned task.
Never chain a second step in the same session. Session-start ritual: `AGENTS.md` →
`PLAN.md` §10+§11 → tail of `LEDGER.md` → choose exactly one step. Session-end ritual:
complete it → append `LEDGER.md` → tick the `§10` exit → commit → **stop**. The full rule
and the self-hosted tooling reality notes are in `AGENTS.md`.

**v0.1.1 published (2026-09-07):** annotated tag `v0.1.1` created at `eacb522`;
`release.yml` ran successfully (run `34070705981`, 1m15s); 3 assets published (amd64/arm64
archives + SHA256SUMS); independently downloaded and verified (`sha256sum -c` OK). The
503 error observed was a transient GitHub API blip at asset-upload time, not a repo defect.

**fix/v0.1.1-audit-remediation merged to main via PR #6 (2026-09-07):**
`gh pr merge 6 --merge` → merge commit `ba6e098`. Branch protection enforced
(required check `Go fmt · vet · test · race · vuln · cross-build` PASSED, 1m11s).
`main` now at `ba6e098`; v0.1.1 tag (`eacb522`) is reachable from `main`. Branch
`fix/v0.1.1-audit-remediation` kept (auditable history, same as `hardening/v0.1`).

**GPG signed-tag implementation landed 2026-09-07** (branch `signed-tags`,
merged to `main` via PR): Ed25519 no-passphrase key generated on the release
host (owner choice; keyid `5F74A36F7B5C1670`, expires 2028-09-06), repo-local
`tag.gpgsign` + `user.signingkey` configured, public key committed at
`docs/release-signing-key.asc`, and `release.yml` now fails the release on
an unsigned tag (`git verify-tag`) before building anything. Same session
fixed a P0 regression: commit `a05e89f` had dropped the whole top-level
`env:` block from `release.yml`, which would have failed the next release
(`VERSION` empty) — block restored. Practice documented in CONTRIBUTING
("Signed release tags") + README. Pre-policy tags `v0.1.0`/`v0.1.1` stay
unsigned. `make actionlint` + `make secret-scan` (0 leaks) + `make check`
green. Remaining owner-optional: upload the public key at
github.com/settings/keys for the green Verified badge.

**Resolved — was queued, now landed:** ~~gitleaks-in-CI, actionlint in the
local gate, the signed-tag decision~~ (all on `main` as of 2026-09-07; the
workflows pin action SHAs, not `@v` majors). **The public-visibility decision
stays the owner's call** — the repo may now go public at the owner's
discretion.

**v0.2 scope (2026-09-07, owner-selected via chat):** **chat session resume**,
**sandboxed command execution**, and **agent breadth** — sequenced as
V2a→V2b→V2c→V2d (§10) as a **small focused release** (one gate per session;
V2a, V2b, and V2c are landed; v0.2 tags when the set lands).
**Excluded from v0.2 (not owner-selected):** mobile residuals (landscape/
rotation geometry measurement, post-reconnect probe block `m6-live-1b`).
**v0.2.0 tagged 2026-09-07** (signed annotated tag after the V2d session
landed the set; see LEDGER). Remaining owner click: upload the GPG public
key at github.com/settings/keys for the green Verified badge.

## 12. Next-level TUI plan — performance · usability · visibility (PROPOSAL, planning-only, 2026-09-07)

Status: **proposal, not committed scope.** Owner-selected v0.2 (V2a–V2d) stays
first in line (V2a–V2c are landed; V2d cut is decided at that session's start). This §12
is the planning phase the owner requested 2026-09-07 ("take the TUI to the next
level in performance, usability, visibility") — an N-series cut for the owner to
sequence as v0.3 (or interleave) after v0.2 leftovers. Evidence base: web research
brief (state of the art 2025–2026, Bubble Tea v2 / Crush / opencode / Claude Code
statusline; artifact: subagent research.md d7577a9c) + repo recon against the D4
benchmarks. Claims below were verified against the pinned tree where marked ✅v.

### N1 — Render windowing (P, highest value, baseline pinned) — **LANDED 2026-09-07**
Landed: O(visible) window via `chatLineCount` + `chatWindowTotal`;
equivalence pinned against a frozen naive copy; golden frames byte-identical.
Gate result: `ChatWindow100x/tail` ≈111 µs / 14 allocs vs pinned
`ChatLines100x` ≈754 µs / 2 018 allocs (~6.6×, −450× bytes); `ChatPane100x/tail`
1.4 → 0.92 ms. See LEDGER 2026-09-07.
The known hotspot: `chatLines` rebuilds the **O(total cached lines)** join every
frame (D4: ChatPane100x ≈1.5 ms/op · 14k allocs; ChatLines100x ≈0.87 ms/op).
Plan: render only the visible window — slice from the cached per-turn blocks and
join O(visible) lines; finalized turns stay frozen (Crush pattern: per-width render
cache + versioned invalidation + `Finished()` freeze; SelfTUI already has the
per-width cache, so the delta is the window slice). Gate: new bench must beat the
pinned ChatLines100x baseline; golden frames unchanged (72×30 + 120×40 still
byte-identical). ✅v baseline verified in `internal/ui/agent_view_bench_test.go`.

### N2 — Streaming repaint discipline (P)
During a live stream, re-render only the active block, not the whole pane; cache
thinking/content sections separately so stream deltas don't invalidate rendered
neighbors (Crush pattern); batch deltas to a repaint tick instead of per-token
frames. Keep the "▍" caret + follow behavior intact (M7-B pins the UX).

### N3 — tok/s + exact token counts (V+U, cheap, high value)
✅v `eval_count` / `prompt_eval_duration` from Ollama's final stream chunk are **not
parsed today**; the ctx meter runs on `agent.ApproxTokens`. Plan: parse the final
chunk in `internal/ollama`, surface per-turn `model · 3.4s · stop · 41 tok/s` in the
existing M7-B turn footer, and upgrade the M7-C ctx meter with measured prompt
tokens when a turn completes (ApproxTokens stays for live drafting).

### N4 — Status bar as observability row (V)
Extend the persistent bottom row to `model · ctx bar · tok/s · host` with an **amber
tier** (~80% of num_ctx) before today's red-100% tier (meter is a correctness
feature — over num_ctx silently truncates). Add background-job pills (pull progress,
queued turns) in Crush style. All content lives in the already-cached status row, so
frame cost ≈ 0.

### N5 — Debug/log drawer (V)
Keybind-toggled drawer over `charmbracelet/log`: ollama request/response traces,
reconnect events, agent loop decisions (tool calls, budget, truncation markers).
k9s-style pattern; ships with a `selftui --log-file` flag so drawer + file share one
sink. Read-only; no secrets (redact bearer tokens).

### N6 — Chat composer upgrades (U)
- `@`-file fuzzy reference in the agent input (opencode pattern): pick a workspace
  file, inline it into the draft/agent context (SelfTUI's jailed read_file already
  defines the path safety rules).
- `/details` + `/thinking` toggles to gate tool-output and reasoning blocks
  (qwen3 thinking is suppressed in the loop; this surfaces it on demand).
- Amber/red ctx-tier already in N4; palette (`ctrl+p`) remains the discoverable
  path — a leader key is deliberately **not** adopted (72×30 phone: discoverable >
  muscle-memory chords).

### N7 — Upstream tracking (P, no code now)
✅v Pinned `bubbletea v2.0.9` does **NOT** have `WithScrollOptimization` (research
flagged release status unconfirmed; verified absent). When Charm ships the
scroll-optimized flush (#1725/#1761) + event-driven rendering (#1776) in a release,
pin it and re-run the D4 bench + a 72×30 scroll-frame bench. Track glamour
width-bucketing (round width to 5 cols so resize jitter doesn't rebuild the
renderer) as a micro-item under N1.

### N8 — Spike (device test, owner-run): native scrollback via tea.Println
Charm's chat-history pattern (discussion #1482): print *finalized* turns to the
terminal's native scrollback (`tea.Println`) so old turns cost zero bytes over SSH;
TUI owns only input + streaming area. Biggest open trade-off: iPhone SSH clients
(Blink/Termius) may capture gestures / behave oddly with native scrollback —
**on-device spike before any commitment**. Cheap alternative if N1 windowing lands:
stay in altscreen; N8 is optional.

### Explicitly rejected / deferred
- Leader-key two-stroke chords (discoverability at 72×30; palette wins).
- Mouse capture (keep off; preserve native selection/scroll).
- Undo/redo of agent mutations (touches the V2c jail; revisit with V2d, owner
  decision — aider/opencode precedent noted but out of this cut).

### Sequencing sketch (owner to confirm)
N1 → N3 → N4 → N2 → N6 → N5 → N7(continuous) → N8(spike). Each N-item = one
session per the binding session rule; N1 first (benchmark baseline exists and any
windowing work "must beat" it per the D4 ledger entry).
