# SelfTUI
GO      ?= go
BIN     := bin/selftui

.PHONY: build test lint vet fmt run check clean

build: ## compile the self-tui binary
	$(GO) build -o $(BIN) ./cmd/self-tui

test: ## run all unit tests
	$(GO) test ./...

vet: ## static analysis
	$(GO) vet ./...

fmt: ## check gofmt; exits nonzero if any file needs formatting
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "files need gofmt:"; echo "$$out"; exit 1; fi

lint: vet fmt ## alias: vet + format check (golangci-lint can be added later)

run: ## run directly from source
	$(GO) run ./cmd/self-tui

check: build test lint ## canonical pre-commit gate

clean:
	rm -rf $(BIN)