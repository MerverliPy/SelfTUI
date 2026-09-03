# SelfTUI
GO      ?= go
BIN     := bin/selftui

.PHONY: build test lint vet fmt run check clean probe probe-build probe-raw probe-local

build: ## compile the self-tui binary
	$(GO) build -o $(BIN) ./cmd/self-tui

test: ## run all unit tests (uncached: golden fixture compares must always execute)
	$(GO) test -count=1 ./...

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

clean:
	rm -rf $(BIN)