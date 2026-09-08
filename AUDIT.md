# SelfTUI — Public-GitHub Readiness Audit

**Date:** 2026-09-08 · **Auditor:** read-only agent audit · **HEAD:** `c3c9330` (main, 44 commits past v0.2.0)
**Scope:** whole repo at working tree; no files modified. The only artifact created is this report.
**Method:** every finding below cites a command actually run in this environment or live GitHub API evidence (`gh`). Checks that could not be run are listed under "Unknowns requiring confirmation" — nothing is inferred.

---

## 1. Executive summary

SelfTUI is a genuinely well-engineered repository: the test suite (unit + race + 25 golden render fixtures) passes green in this environment, CI is green on main with govulncheck/gitleaks/actionlint steps succeeding, branch protection is real, the v0.2.0 release pipeline produced signed-tag-gated binaries with a SHA256SUMS manifest, and secrets handling (https-only token transport, removed `-auth-token` flag, redacted log sink) is better than most public Go TUIs.

The gaps are **adoption and community-shaped, not engineering-shaped**: there is no prebuilt-binary quick-start in the README, the module path (`module selftui`) blocks `go install`, the README status line is one release behind, the LICENSE copyright placeholder was never filled in, and CODE_OF_CONDUCT / issue templates are absent. Adoption evidence matches: 0 stars, 0 forks, 1 download per release asset.

**Readiness score: 82 / 100** (detailed breakdown at the end).

---

## 2. Findings

Severity scale: Critical / High / Medium / Low. "Pos." marks a verified strength recorded for completeness.

### F-01 · No prebuilt-binary quick-start install path — **High**

- **File:** `README.md` (§ "Build & run", line 114; § "Product contract", line 385)
- **Evidence:** `rg -n -i 'download|release asset|go install|install' README.md` shows the only user-facing install is `make build → bin/selftui` (line 385: "Install Ollama and SelfTUI (`make build` → `bin/selftui` …"). Release assets (`selftui-v0.2.0-linux-{amd64,arm64}.tar.gz`, `SHA256SUMS`) exist — confirmed via `gh release view v0.2.0 --json assets` — but are referenced only inside "Release engineering" (line 194), a maintainer-oriented section. A newcomer must install Go ≥ 1.25 + make to try a product whose tagline is "**One binary.**"
- **Severity:** High (largest single adoption friction)
- **Fix:** Add an "Install" section immediately after the badges: two `curl -LO` + `tar -xzf` + `chmod +x` one-liners for amd64/arm64, plus `sha256sum -c SHA256SUMS`, linking the releases page. Keep `make build` as the "from source" path.
- **Verification:** `rg -n 'releases/download' README.md` finds the quick-start block; follow the commands on a clean machine.

### F-02 · README status is one release stale — **Medium**

- **File:** `README.md:13` (and heading `README.md:68` "## v0.1 product contract")
- **Evidence:** Line 13 says "**Status: v0.1.1 released 2026-09-07**". `gh release list` shows **SelfTUI v0.2.0 — Latest, published 2026-09-07T19:38Z**; `git describe --tags` → `v0.2.0-44-gc3c9330`. The body does document v0.2 features (V2a/V2c at lines 76, 85–87, 306–320), so the status line and "v0.1" heading framing contradict the release metadata and CHANGELOG.md (`[0.2.0] - 2026-09-07`).
- **Severity:** Medium (a stale status line is the cheapest way to look abandoned)
- **Fix:** Update line 13 to `Status: v0.2.0 released 2026-09-07 (…)`. Retitle line 68 to "## Product contract" and drop per-version framing inside it (keep the version caveats inline where they matter).
- **Verification:** `rg -n 'Status: v0\.' README.md` shows v0.2.0; `gh release list` agrees.

### F-03 · `module selftui` blocks `go install` and godoc — **Medium**

- **File:** `go.mod:1`
- **Evidence:** `head -1 go.mod` → `module selftui`. This is not an importable path, so `go install github.com/MerverliPy/SelfTUI/cmd/self-tui@latest` cannot work for any user, and the package is invisible on pkg.go.dev. All internal imports (`selftui/internal/...`, 79 .go files) hang off this path.
- **Severity:** Medium (blocks the standard Go CLI adoption path)
- **Fix:** Rename module to `github.com/MerverliPy/SelfTUI` and update import paths (mechanical: `gofmt -w -r` or a scripted `sed` + `go mod tidy`); tag the next release after the rename so `go install …/cmd/self-tui@latest` resolves. Low-risk: `go test ./...` after the rename must be green.
- **Verification:** `go install github.com/MerverliPy/SelfTUI/cmd/self-tui@<new-tag>` in a scratch `GOBIN`, then `selftui -version`.

### F-04 · LICENSE copyright placeholder never filled in — **Medium**

- **File:** `LICENSE:189`
- **Evidence:** `grep -n 'Copyright' LICENSE` → `189:   Copyright [yyyy] [name of copyright owner]`. The Apache-2.0 appendix explicitly instructs fill-in of this line. GitHub still detects Apache-2.0 (`gh repo view` → `licenseInfo: apache-2.0`), but the un-substituted template looks unfinished to contributors and in license-scanning tools.
- **Severity:** Medium
- **Fix:** Replace with `Copyright 2026 <legal name or "the SelfTUI authors">`.
- **Verification:** `grep -n 'Copyright \[' LICENSE` returns nothing.

### F-05 · No CODE_OF_CONDUCT.md — **Medium**

- **File:** missing (repo root)
- **Evidence:** `for f in … CODE_OF_CONDUCT.md …; do [ -e "$f" ] … done` → `MISSING: CODE_OF_CONDUCT.md`. CONTRIBUTING.md (118 lines) and SECURITY.md (85 lines) both exist and are substantive; CoC is the community-standards gap, and it is a component of GitHub's community-profile checklist.
- **Severity:** Medium
- **Fix:** Add the Contributor Covenant v2.1 (`CODE_OF_CONDUCT.md`), with a contact channel (GitHub private contact or an issue label policy).
- **Verification:** `[ -f CODE_OF_CONDUCT.md ] && echo ok`; repo Community profile shows "Code of conduct: ✓".

### F-06 · No issue or PR templates — **Low**

- **File:** `.github/` (directory listing)
- **Evidence:** `ls -la .github/` shows only `workflows/`; `ls .github/ISSUE_TEMPLATE/` → `No such file or directory`.
- **Severity:** Low (matters more as external traffic arrives)
- **Fix:** Add `.github/ISSUE_TEMPLATE/bug_report.md` (include Ollama host version, terminal emulator/geometry, OS) and `feature_request.md`; optionally a short `.github/PULL_REQUEST_TEMPLATE.md` pointing at `make check`.
- **Verification:** `ls .github/ISSUE_TEMPLATE/` lists the templates.

### F-07 · `internal/ui/agent_view.go` is a 3,483-line god file — **Medium**

- **File:** `internal/ui/agent_view.go`
- **Evidence:** `fd -e go -E _test.go . | xargs wc -l | sort -rn` → `3483 internal/ui/agent_view.go` (next largest non-test file: `models_view.go` at 1,415). `rg -c '^func '` → **132 functions** in one file. Function granularity itself is fine (names like `mergeStreamDeltas`, `slashDraftKey`, `enqueueSessionTurn` are coherent), but the file spans composer, transcript rendering, session recording/export, tool-call display, modal routing, and live-stream plumbing.
- **Severity:** Medium (maintainability + first-contribution barrier; review surface for every UI change)
- **Fix:** Split along existing seams into sibling files in the same package (no API change): `agent_composer.go`, `agent_transcript.go`, `agent_session.go`, `agent_tools_display.go`. Golden tests + 91% package coverage make this a safe mechanical refactor.
- **Verification:** `go test -race -count=1 ./internal/ui/` green before and after; `wc -l internal/ui/agent_view*.go` shows all files < ~1,000 lines.

### F-08 · govulncheck cannot be reproduced locally with the repo's own pinned command on an older base Go — **Low**

- **File:** `Makefile:50` (`vuln:` target), `README.md:145`, `.github/workflows/ci.yml` (GOVULNCHECK_VERSION: "v1.7.0")
- **Evidence:** Running the documented `go install golang.org/x/vuln/cmd/govulncheck@v1.7.0` in this environment (base `go` on PATH is 1.22.2, `GOTOOLCHAIN=auto`) produced a govulncheck built with the go1.26.8 toolchain, which then fails against this module's go1.27.1 stdlib: `file requires newer Go version go1.27 (application built with go1.26)` for multiple files. In CI the same step **succeeds** (verified: `gh run view 34200563710` → step `govulncheck (make vuln)` conclusion `success`), because CI's setup-go provides a real 1.27.1 toolchain. So this is a contributor-reproducibility nuance, not a CI defect.
- **Severity:** Low
- **Fix:** Add one sentence to CONTRIBUTING "Building and testing": when the `go` binary on PATH is older than 1.27.1, build govulncheck with the pinned toolchain (`GOTOOLCHAIN=go1.27.1 go install golang.org/x/vuln/cmd/govulncheck@v1.7.0`).
- **Verification:** Repeat the documented install on a base-Go-1.22 machine; the modified instruction completes and `govulncheck ./...` runs.

### F-09 · Release tags v0.1.0/v0.1.1 are unsigned — **Low (informational)**

- **File:** git tags
- **Evidence:** `git cat-file tag v0.1.0|v0.1.1` → no `BEGIN PGP SIGNATURE` block; `v0.2.0` → 1 signature block. This matches the documented policy ("signed-tag policy enforced for **future** `v*` releases", README line 13; signing key published at `docs/release-signing-key.asc`, verified to be a `PGP PUBLIC KEY BLOCK`).
- **Severity:** Low — expected, but the published v0.1.x release notes could state that those tags predate the signing policy so verifiers aren't surprised.
- **Fix:** One line in v0.1.x release notes (or leave as-is; v0.2.0 onward is covered).
- **Verification:** `git cat-file tag v0.2.0 | grep -c -e 'BEGIN PGP SIGNATURE'` → 1.

### F-10 · Local branch/worktree debris — **Low (not publicly visible)**

- **File:** git refs
- **Evidence:** `git branch -a` → 22 local branches, many merged (e.g. `feat/n6-composer`, `release/v0.2.0`); `git worktree list` → two leftover audit-squad worktrees under `/tmp` (`audit-squad/cA`, `audit-squad/cB`). The tracked tree itself is clean (`git status --porcelain` empty).
- **Severity:** Low
- **Fix:** `git remote prune origin && git branch --merged main | grep -v main | xargs -r git branch -d`; `git worktree remove` the /tmp leftovers.
- **Verification:** `git worktree list` shows only the main checkout; `git branch` count drops.

### F-11 · `cmd/size-probe` has no tests; `cmd/self-tui` coverage 12.3% — **Low**

- **File:** `cmd/size-probe/`, `cmd/self-tui/`
- **Evidence:** `go test -cover ./...` → `selftui/cmd/size-probe: coverage: 0.0%`, `selftui/cmd/self-tui: coverage: 12.3%` while all `internal/*` packages are 83–93%. This is a deliberate architecture (logic lives in tested `internal` packages; `main.go` is glue, and what little it owns — e.g. `configPathOverride` — is extracted and tested, `cmd/self-tui/main.go:37`). size-probe is a measurement instrument.
- **Severity:** Low
- **Fix:** Optional: one smoke test for `run()` flag-parse failure paths; otherwise no action.
- **Verification:** `go test -cover ./cmd/...` unchanged; no regression.

---

### Verified strengths (Pos. — evidence recorded, no action)

| # | Area | Evidence |
|---|------|----------|
| P-1 | **Tests green, uncached + race** | `go test -count=1 ./...` → all 7 test-bearing packages `ok` (8.7s); `go test -race -count=1 ./...` → all `ok` (17.3s). Coverage: agent 83.3%, config 92.5%, logsink 93.3%, ollama 92.5%, session 92.3%, ui 91.0%. 25 golden fixtures in `internal/ui/testdata/golden/`. |
| P-2 | **Build + mod verify** | `CGO_ENABLED=0 go build -o /tmp/audit-selftui ./cmd/self-tui` → 26.7 MB binary, no errors; `go mod verify` → "all modules verified". |
| P-3 | **CI green and well-built** | `gh run list` → latest main run `34200563710` success (1m15s). ci.yml: pinned-action SHAs (checkout/setup-go), `permissions: contents: read`, concurrency cancel, `timeout-minutes: 15`, static job name used as required-check context, Go 1.27.1 + govulncheck v1.7.0 pinned in both workflows and README. |
| P-4 | **Branch protection real** | `gh api …/branches/main/protection` → requires status check `Go fmt · vet · test · race · vuln · cross-build`, `enforce_admins: true`, force-push/deletion disabled. (PR approving-review count is 0 — solo-appropriate; noted in Unknowns.) |
| P-5 | **Release pipeline end-to-end** | `gh release view v0.2.0` → published release with `selftui-v0.2.0-linux-amd64.tar.gz` (6.6 MB), `…-arm64.tar.gz` (6.0 MB), `SHA256SUMS`; release.yml re-runs the full gate, verifies tag signature and tag==stamped version; `make release-check` builds deterministic archives and refuses dirty worktrees. Release gate has its own regression suite (`scripts/release-check-test.sh`). |
| P-6 | **Secrets hygiene** | `rg` for hardcoded secret-shaped literals over tracked files (excluding tests/docs) → no hits; CI gitleaks step green; `.gitleaks.toml` present with justified allowlists; token transport restricted to https except loopback (`internal/config/validate.go:141` area); `-auth-token` flag removed in v0.2.0 (CHANGELOG Security section); log sink redacts bearer tokens (`internal/logsink/logsink.go:21`); `.gitignore` blocks `.env`. |
| P-7 | **Failure handling** | `internal/ollama/client.go:21` `requestTimeout = 30s` on the API client; streaming client deliberately has no request timeout but a 90s idle timeout with a typed `errIdleTimeout` (`internal/ollama/stream.go:27,39`) and context cancellation; panic/`log.Fatal` scan of library code → zero hits. |
| P-8 | **No code debt markers** | `rg -n -i 'TODO|FIXME|HACK|XXX' -g '*.go'` → only test fixtures for `git grep TODO` itself; no real TODO debt. No duplicate function names across non-test files (`rg '^func ' | sort | uniq -c` → none repeated). |
| P-9 | **Docs depth** | README 465 lines: badges, tagline, features, pinned golden-render previews, product contract, build & run with pinned-toolchain instructions, release engineering, full config table (flags > env > file), navigation, iPhone/SSH guide measured at 72×30, project docs index. CONTRIBUTING documents test-first policy, golden-fixture workflow, signed-tag release policy, DCO-style Apache contribution clause. SECURITY.md routes to GitHub private vulnerability reporting. CHANGELOG follows Keep-a-Changelog (0.1.0, 0.1.1, 0.2.0). |
| P-10 | **Repo metadata** | `gh repo view` → description set, 8 topics (ai-agent, bubbletea, go, llm, ollama, ssh, terminal, tui), issues enabled, Apache-2.0 detected. |
| P-11 | **Healthy process velocity** | 5 PRs merged 2026-09-08 alone (#22–#26) with visible external-style review cycles (PR #22 Codex review fixes 7/7). |

---

## 3. Inspection areas (map of evidence)

1. **Problem / target user** — One-binary Linux TUI for Ollama model management + embedded coding agent; target: self-hosters on Linux/WSL, iPhone-over-SSH users. Clear, differentiated, honest about platform limits (README:5–11). ✔
2. **README clarity / first-use** — Strong content and previews; stale status line (F-02); no prebuilt quick-start (F-01). Build & run and config sections are precise and testable. ◐
3. **Installation reliability** — Source build verified working (P-2); binary install path undocumented (F-01); `go install` blocked by module path (F-03). ◐
4. **Repository structure** — Clean: `cmd/self-tui`, `cmd/size-probe`, `internal/{agent,config,logsink,ollama,session,ui}`, `scripts/` (each smoke/gate script has a paired test), `docs/` evidence files, `.github/workflows/`, `Makefile` with help comments. ✔
5. **Code quality / duplication** — go vet clean, gofmt clean, no TODO debt, no duplicate function names, panic-free library code; god file F-07. ◐
6. **Tests / lint / build** — All green locally (uncached, race, vet, gofmt, build) and in CI (P-1, P-3). ✔
7. **Security / secrets / dependencies / permissions** — No secrets found; gitleaks + govulncheck + actionlint in CI; least-privilege workflows; https-only token transport; sandboxed `run_command` (bubblewrap, fail-closed, documented in `docs/run-command-containment.md`); `go mod verify` green. ✔
8. **GitHub Actions / release** — ci.yml + release.yml both verified green; signed v0.2.0 tag; assets + SHA256SUMS published (P-3, P-5; F-09 informational). ✔
9. **LICENSE / CONTRIBUTING / SECURITY / templates** — LICENSE present but placeholder unfilled (F-04); CONTRIBUTING ✔; SECURITY ✔; CoC missing (F-05); issue/PR templates missing (F-06). ◐
10. **Mobile usability / accessibility** — Layout breakpoints driven by *measured* device geometry (72×30) and pinned by 25 golden fixtures; "terminal too small" floor (<40×12); keyboard-only (no mouse dependency — `rg -i mouse internal/ui` → none), light theme contrast-checked per PLAN §10 M5; screen-reader support is inherently limited for TUIs. Independent device verification not possible in this environment (Unknowns). ◐
11. **Performance / failure handling** — v0.2 perf items landed (N1 width-bucketing PR #26, N2 streaming, N3 tokens via PR #21); timeouts/idle-timeouts verified (P-7); flaky-transport hardening (chatStub) in latest PR. ✔
12. **Adoption / contribution blockers** — 0 stars/forks, 1 download/asset, discussions disabled; the blockers are F-01/F-03 (install path), F-02 (credibility), F-04/05/06 (community signals), F-07 (contribution surface). ◐

---

## 4. Readiness score: **82 / 100**

| Dimension | Weight | Score |
|---|---|---|
| Problem clarity & differentiation | 10 | 9 |
| README & first-use docs | 15 | 12 |
| Installation reliability | 15 | 9 |
| Repo structure & code quality | 15 | 13 |
| Tests, lint, build | 15 | 14 |
| Security & supply chain | 10 | 10 |
| CI & release process | 10 | 9 |
| Community files & signals | 10 | 6 |
| **Total** | **100** | **82** |

The engineering axes are near-ceiling; every lost point traces to a specific finding above, all of which are cheap to fix.

## 5. Top five blockers

1. **No prebuilt-binary quick start** (F-01) — the tagline says "One binary," but the README only teaches `make build`.
2. **Stale README status line** (F-02) — "v0.1.1" against a shipped v0.2.0 reads as an unmaintained repo.
3. **`module selftui` blocks `go install`** (F-03) — the standard Go CLI discovery path doesn't exist.
4. **Community-profile gaps** (F-04, F-05, F-06) — placeholder copyright, no CoC, no issue templates.
5. **3,483-line `agent_view.go`** (F-07) — the file a new contributor must read to touch the flagship feature.

## 6. Prioritized implementation plan (awaiting your approval — nothing implemented)

**P0 — credibility & first-click (≤ half a day)**
1. README: add "Install" section (curl/tar + checksum verify + releases link) — F-01.
2. README: status line → v0.2.0; retitle "v0.1 product contract" → "Product contract" — F-02.
3. LICENSE: fill in the copyright appendix line — F-04.
4. Add `CODE_OF_CONDUCT.md` (Contributor Covenant) — F-05.
5. Add `.github/ISSUE_TEMPLATE/{bug_report.md,feature_request.md}` (+ optional PR template pointing at `make check`) — F-06.

**P1 — Go-native install path (1 day)**
6. Rename module to `github.com/MerverliPy/SelfTUI`; update imports; `go mod tidy`; full gate green; land **before** the next tag so `go install …@latest` works — F-03.
7. Local hygiene: prune merged branches/worktrees — F-10.

**P2 — contributor experience (1–2 days)**
8. Split `internal/ui/agent_view.go` along existing seams (same package, no API change; golden tests are the safety net) — F-07.
9. CONTRIBUTING: one-line govulncheck toolchain note for base-Go < 1.27.1 contributors — F-08.
10. v0.1.x release notes: note unsigned tags predate the signing policy — F-09.

**P3 — optional**
11. Enable GitHub Discussions (Q&A for a phone-SSH product is high-value).
12. Consider an install script (`curl … | sh`) or Homebrew tap once P1 lands.
13. Optional smoke test for `cmd/self-tui` run() error paths — F-11.

## 7. Recommended files to add / remove

**Add:** `CODE_OF_CONDUCT.md`; `.github/ISSUE_TEMPLATE/bug_report.md`; `.github/ISSUE_TEMPLATE/feature_request.md`; `.github/PULL_REQUEST_TEMPLATE.md` (optional); (post-P1) nothing else required — the docs set is already unusually complete.

**Remove:** nothing is harmful. Optional tidy: move `SelfTUI-External-Audit-2026-09-04.md` under `docs/` to declutter the root (update the reference in README "Project docs" if moved). Local-only cleanup (not a repo change): merged branches + `/tmp` audit-squad worktrees (F-10).

## 8. Unknowns requiring confirmation

- **govulncheck locally (F-08):** could not complete in this environment because the base `go` on PATH is 1.22.2 and the pinned install builds a go1.26.8-embedded binary that rejects the go1.27.1 stdlib. CI's identical step is green (verified via `gh run view 34200563710`), so the module itself is not implicated — but the exact P1-fix wording should be validated on a second machine.
- **actions/checkout & setup-go pinned SHAs:** CI runs them green, so the SHAs resolve and execute; I did not independently verify which checkout/setup-go releases those SHAs correspond to.
- **Device usability on iPhone (Moshi/Blink/Termius):** measured on the owner's device and recorded (`docs/device-acceptance-2026-09-07.md`, `docs/m0a-gate-evidence.md`) and pinned via golden fixtures, but not independently reproducible in this audit environment.
- **Live Ollama interaction:** the smoke suites (`make smoke`, `make smoke-model`, `make smoke-reconnect`) require a live Ollama host + SSH-drop simulation; not executed here. Unit/integration coverage of the same code paths is green.
- **Branch protection "required approvals = 0":** intentional for a solo project; confirm whether you want ≥1 external review required once contributors arrive.
- **Discussions disabled / 0 stars:** adoption state is current as of today; no historical traffic data was available to this audit.

### Resolution addendum — debug pass 2026-09-08T16:03Z (parent-local, zero agents; read-only)

1. **govulncheck (F-08) — RESOLVED, fix validated.** `GOBIN=<tmp> GOTOOLCHAIN=go1.27.1 go install golang.org/x/vuln/cmd/govulncheck@v1.7.0` exits 0 and produces a scanner reporting `Go: go1.27.1 / Scanner: govulncheck@v1.7.0`; the exact command proposed in F-08 works verbatim on a base-Go-1.22 machine. Scan of this module: **0 reachable vulnerabilities** (7 in imported packages + 3 in required modules, all unreachable per call-graph). All 10 trace to one indirect dep: `golang.org/x/net@v0.39.0` (GO-2026-5025…5030, 4440/4441, 4918, 5942), fixed in x/net v0.53.0–v0.56.0 — a single `go get golang.org/x/net@latest && go mod tidy` clears the board. Suggested as a low-priority dependency-hygiene item; not a blocker.
2. **Pinned action SHAs — RESOLVED.** `git ls-remote --tags` proves both pins are release commits: actions/checkout `11d5960a…` = **v4.4.0** (commit 2026-07-16, "backport fixes to releases-v4 #2524", also the `v4` moving tag); actions/setup-go `40f1582b…` = **v5.6.0** (commit 2025-12-15, "Fall back to downloading from go.dev/dl #689", also the `v5` moving tag). Supply-chain pin provenance is now first-party verified.
3. **Device usability — evidence verified, execution stays owner-only.** `docs/device-acceptance-2026-09-07.md` (6.8 KB) is a real, dated artifact: 26 scenarios (SC-01…SC-26) on Moshi/iPhone 16 Pro SSH at the measured 72×30 portrait over the M7/v0.2 surface, with explicit **[A] artifact-verified vs [O] owner-reported** epistemic tags; `docs/m0a-gate-evidence.md` pins the geometry read-out ("Moshi's default portrait width is 72 cols, height 30 rows") and documents the `WindowSizeMsg 0x0` pty edge case as handled. No phone in this environment — independent reproduction remains impossible here; the evidence artifact is coherent and honest about its own limits.
4. **Live Ollama — host IS live.** Read-only `GET http://localhost:11434/api/version` → **Ollama 0.33.1**; `GET /api/tags` → models present (incl. `qwen3:0.6b`). The precondition the audit called missing now exists, so `make smoke` / `make smoke-model` are runnable on owner approval — deliberately **not** auto-run: the pull/delete smoke mutates the host's model store (human-gated per safety rules). Liveness is recorded as evidence; full smoke remains an owner-executed step.
5. **Branch protection — refreshed, unchanged (2026-09-08T16:03Z):** required check `Go fmt · vet · test · race · vuln · cross-build`, `enforce_admins: true`, `required_reviews: 0`. Still an owner decision, not a defect.
6. **Adoption state — refreshed, unchanged (2026-09-08T16:03Z):** stars 0, forks 0, discussions disabled, repo public. Matches §1; no further action.

---

*Audit performed read-only. Commands of record: `fd`, `rg`, `git log/status/tag/worktree`, `go build/test/vet/gofmt/mod verify/-cover`, `make` target inspection, `gh repo view / run list / run view / release list / release view / api …protection`. Exit codes: all inspection commands returned 0 except the intentionally-exploratory rg patterns documented inline (F-08 environment failure; secret-pattern probe rerun after a shell-quoting error).*
