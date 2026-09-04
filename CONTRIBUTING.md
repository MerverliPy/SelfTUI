# Contributing

Thanks for considering a contribution to SelfTUI. v0.1 is a small,
tightly-scoped release; please read this before opening issues or PRs.

## Product contract (v0.1)

Public v0.1 is a **single-process Linux/WSL TUI for Ollama**. Chat is
in-memory (the Markdown transcript export survives exit but is not resumable);
workspace tools are disabled by default and require an explicit workspace;
command execution is not shipped; non-loopback tokens require HTTPS; native
Windows/macOS are not supported. Changes that silently walk any of these
lines back need an explicit owner decision first — see `PLAN.md` §11 and the
dated release-hardening note at the top.

## Repo rules

- `AGENTS.md` — project instructions and the binding one-fresh-session-per-step
  rule for agent sessions.
- `PLAN.md` — architecture, roadmap (§10), risks/owner decisions (§11).
- `LEDGER.md` — chronological, append-only work/decision log. Consult it
  before starting work; append to it when done; never rewrite history.
- `COUNCIL-MEMO.md` — the advisory audit that re-cut the roadmap (historical).

## Building and testing

Requires Go ≥ 1.25 (`GOTOOLCHAIN=auto` fetches it on demand).

```sh
make build     # bin/selftui
make test      # unit tests (uncached — golden fixture compares always run)
make lint      # go vet + gofmt check
make check     # canonical pre-commit gate: build + test + vet + gofmt
```

Race detector (always run it on UI/session changes):

```sh
go test -race -count=1 ./internal/ui ./internal/session
```

Golden render fixtures (testdata/golden) pin the shell at 72×30 and 120×40.
When a deliberate UI text/layout change moves them, regenerate and review the
diff — do not blanket-update without reading what changed:

```sh
go test ./internal/ui -run TestGoldenRender -update
```

## Style of contribution

- Test-first for behavior changes: a red test that pins the new contract, then
  the smallest implementation that turns it green. Table-driven tests for
  geometry/boundary logic, matching the existing patterns in `internal/ui`.
- Match the existing code layout and naming; keep changes minimal and
  focused. No speculative scaffolding.
- Keep the product contract accurate in user-facing copy: e.g. the transcript
  command is `/export` (flush + report the append-only Markdown path) — it
  must never claim the conversation can be resumed.

## Licensing

By contributing you agree that your contribution is licensed under the
Apache-2.0 terms in `LICENSE` (see the License's Contribution section).
