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

Requires Go ≥ 1.25 for day-to-day development (`GOTOOLCHAIN=auto` fetches it
on demand). CI and the release gates pin **Go 1.27.1** (current official
stable at phase-8 time). The release gate **enforces** the local
prerequisites rather than trusting the docs: it fails fast (exit 2, before
any gate work) unless the `go` on PATH reports `go1.27.1`, the `gofmt` on
PATH is the gofmt shipping with that same distribution (`$(go env
GOROOT)/bin/gofmt` — gofmt has no version flag, so the pin is by identity),
and govulncheck reports `v1.7.0` (`govulncheck -version`). CI configures the
same versions in the workflows (`setup-go` + a pinned `go install`); run the
gate locally with the pinned distribution's `bin` first on PATH so local ==
CI. This is a local prerequisite only — `.github/workflows/` needs no change
when you install a different toolchain elsewhere.

```sh
make build     # bin/selftui
make test      # unit tests (uncached — golden fixture compares always run)
make race      # full suite under the race detector
make lint      # go vet + gofmt check
make check     # canonical pre-commit gate: build + test + vet + gofmt
make vuln      # govulncheck ./... (install: go install golang.org/x/vuln/cmd/govulncheck@v1.7.0)
```

Race detector (or `make race`):

```sh
go test -race -count=1 ./...
```

Golden render fixtures (testdata/golden) pin the shell at 72×30 and 120×40.
When a deliberate UI text/layout change moves them, regenerate and review the
diff — do not blanket-update without reading what changed:

```sh
go test ./internal/ui -run TestGoldenRender -update
```

## Releases

v0.1.x releases are gated end-to-end by `scripts/release-check.sh`
(`VERSION=v0.1.1 make release-check`): it demands a clean worktree, a
`VERSION` matching `v<major>.<minor>.<patch>`, and the enforced toolchain
pin above (go 1.27.1 + same-distribution gofmt + govulncheck v1.7.0 — fails
fast before any slow step), then runs module verification, gofmt, vet,
uncached tests, race tests, `govulncheck`, both CGO-disabled Linux builds,
per-binary version-stamp checks, deterministic archives with fixed member
modes (binary 0755, documents 0644), and a flat-named `SHA256SUMS` manifest
under `dist/` (gitignored). Verify an archive set with
`cd dist && sha256sum -c SHA256SUMS` — the manifest's flat entries are what
downloaders see beside GitHub Release assets. Regression suite:
`bash scripts/release-check-test.sh`. The gate **never tags**;
pushing a `v*` tag is the owner's step and triggers `.github/workflows/
release.yml`, which re-runs the gate, verifies the tag against both binaries'
stamped versions, uploads the archives + `SHA256SUMS`, and generates release
notes from `CHANGELOG.md`. See README "Release engineering (v0.1)".

### Signed release tags (owner policy, 2026-09-07)

Every `v*` release tag is GPG-signed (`v0.1.0`/`v0.1.1` predate the policy
and are unsigned). The signing key is Ed25519, no passphrase, living in the
release host's GPG home; the public half is committed at
`docs/release-signing-key.asc` so `release.yml` can verify every pushed tag
(`git verify-tag` — unsigned or lightweight tags fail the release before
any asset is built). Local repo config signs tags by default
(`git config --local tag.gpgsign true` + `user.signingkey`); that config is
machine-local and does not transfer with a clone. Tagging practice:

```sh
git tag -s vX.Y.Z -m "SelfTUI vX.Y.Z"  # signed; -a/-m without -s fails CI
git verify-tag vX.Y.Z                    # Good signature before pushing
git push origin vX.Y.Z
```

Back up the private key (`gpg --export-secret-keys <keyid>`) somewhere the
repo never sees (private key material must never be committed —
`secret-scan` guards this). For the green "Verified" badge on GitHub,
the owner also uploads the public key under github.com/settings/keys.

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
