# SelfTUI
GO      ?= go
BIN     := bin/selftui
# Release version stamp. Release builds inject $(VERSION) via -X; the
# release-check gate additionally requires VERSION=v<major>.<minor>.<patch>
# (e.g. VERSION=v0.1.0) and a clean worktree.
VERSION ?= dev

.PHONY: build test race vuln lint vet fmt run check clean probe probe-build probe-raw probe-local \
	release-check build-linux-amd64 build-linux-arm64 smoke smoke-model smoke-reconnect audit-pack \
	secret-scan actionlint gitleaks

build: ## compile the self-tui binary
	$(GO) build -o $(BIN) ./cmd/self-tui

test: ## run all unit tests (uncached: golden fixture compares must always execute)
	$(GO) test -count=1 ./...

race: ## run the full suite under the race detector (uncached)
	$(GO) test -race -count=1 ./...

vuln: ## scan the module and its dependencies for known vulnerabilities (needs govulncheck on PATH)
	govulncheck ./...

vet: ## static analysis
	$(GO) vet ./...

fmt: ## check gofmt; exits nonzero if any file needs formatting
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "files need gofmt:"; echo "$$out"; exit 1; fi

lint: vet fmt ## alias: vet + format check (golangci-lint can be added later)

run: ## run directly from source
	$(GO) run ./cmd/self-tui

probe-build: ## compile the size probe binary
	$(GO) build -o bin/size-probe ./cmd/size-probe

probe: probe-build ## interactive size probe (measure WindowSizeMsg live)
	./bin/size-probe

smoke: build ## live M1b smoke: pull + delete against the local Ollama host
	python3 scripts/pull-delete-smoke.py

smoke-model: build ## live smoke pulling a specific model instead of the default
	python3 scripts/pull-delete-smoke.py $(MODEL)

smoke-reconnect: build ## M6 reconnect smoke: SSH-drop simulation + fresh reconnect
	python3 scripts/reconnect-smoke.py

probe-raw: probe-build ## size probe as CSV lines (harness/script friendly)
	./bin/size-probe -mode raw

probe-local: probe-build ## local pty-based measurement at several sizes + mid-run resize
	bash scripts/probe-local.sh

check: build test lint ## canonical pre-commit gate

# --- supply-chain security (P0) ---
secret-scan: ## run gitleaks against the working tree and history
	@echo "== gitleaks secret scan =="
	gitleaks detect --config .gitleaks.toml --redact --verbose

actionlint: ## lint GitHub Actions workflows
	@echo "== actionlint =="
	actionlint .github/workflows/*.yml

gitleaks: secret-scan ## alias for gitleaks scan

# --- v0.1 release tooling --------------------------------------------------
# Static, CGO-disabled Linux release binaries stamped with $(VERSION), plus
# the full release gate. Artifacts land in dist/ (gitignored). None of these
# targets ever creates or pushes a git tag - tagging is the owner's step.
build-linux-amd64: ## CGO-disabled static Linux/amd64 release binary (stamped with $(VERSION))
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "-s -w -X main.Version=$(VERSION)" -o dist/selftui-linux-amd64 ./cmd/self-tui

build-linux-arm64: ## CGO-disabled static Linux/arm64 release binary (stamped with $(VERSION))
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "-s -w -X main.Version=$(VERSION)" -o dist/selftui-linux-arm64 ./cmd/self-tui

release-check: ## full release gate; run as: VERSION=v0.1.0 make release-check
	scripts/release-check.sh

# --- audit packaging (H-06 remediation) -----------------------------------
# scripts/create-audit-pack.sh snapshots the tracked tree (git ls-files,
# dotfiles and .github/workflows included) into a deterministic, manifest-
# complete ZIP for external audits; it refuses to overwrite an existing
# archive and never runs on a dirty worktree. Default output is the
# gitignored dist/selftui-audit-pack-<HEAD>.zip; override with AUDIT_PACK_OUT,
# and add audit prompt/inventory files with AUDIT_PACK_EXTRAS="PROMPT.md=/path"
# (space-separated TARGET=PATH) or by calling the script's --extra directly.
audit-pack: ## manifest-complete deterministic source snapshot for external audits
	scripts/create-audit-pack.sh

clean:
	rm -rf $(BIN) bin/size-probe dist
