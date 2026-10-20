# portcheck — developer task runner.
# Run `make help` to list targets.

SHELL         := /bin/bash
.SHELLFLAGS   := -eu -o pipefail -c
.DEFAULT_GOAL := help

MODULE   := github.com/roystondwork-dev/portcheck
BIN      := bin/portcheck
TOOL     := go tool -modfile=tools/go.mod
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w \
	-X $(MODULE)/internal/buildinfo.version=$(VERSION) \
	-X $(MODULE)/internal/buildinfo.commit=$(COMMIT) \
	-X $(MODULE)/internal/buildinfo.date=$(DATE)

COVER_PROFILE := coverage.out
FUZZTIME      ?= 30s

##@ General

.PHONY: help
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n"} \
		/^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Development

.PHONY: build
build: ## Build the portcheck binary into ./bin.
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/portcheck

.PHONY: run
run: ## Run portcheck with ARGS="...".
	go run ./cmd/portcheck $(ARGS)

.PHONY: fmt
fmt: ## Format code (gofmt + goimports via golangci-lint).
	golangci-lint fmt ./...

.PHONY: tidy
tidy: ## Tidy go.mod files.
	go mod tidy
	cd tools && go mod tidy

##@ Quality

.PHONY: lint
lint: ## Run golangci-lint.
	golangci-lint run ./...

.PHONY: vuln
vuln: ## Scan dependencies for known vulnerabilities.
	$(TOOL) govulncheck ./...

.PHONY: test
test: ## Run unit tests with the race detector.
	$(TOOL) gotestsum --format pkgname -- -race -shuffle=on -count=1 ./...

.PHONY: test-integration
test-integration: ## Run unit + integration tests (real sockets/processes).
	$(TOOL) gotestsum --format pkgname -- -race -count=1 -tags integration ./...

.PHONY: cover
cover: ## Run all tests with coverage and write coverage.out.
	go test -race -count=1 -tags integration -covermode=atomic -coverpkg=./... -coverprofile=$(COVER_PROFILE) ./...

.PHONY: cover-check
cover-check: cover ## Enforce coverage thresholds from .testcoverage.yml.
	$(TOOL) go-test-coverage --config=.testcoverage.yml

.PHONY: cover-html
cover-html: cover ## Open an HTML coverage report.
	go tool cover -html=$(COVER_PROFILE)

.PHONY: fuzz
fuzz: ## Run every fuzz target for FUZZTIME each.
	@for pkg in $$(go list ./...); do \
		for f in $$(go test -list '^Fuzz' $$pkg | grep '^Fuzz' || true); do \
			echo "==> $$pkg $$f"; go test -run=^$$ -fuzz=^$$f$$ -fuzztime=$(FUZZTIME) $$pkg; \
		done; \
	done

.PHONY: mutation
mutation: ## Run mutation testing on the public library.
	$(TOOL) gremlins unleash ./pkg/...

.PHONY: bench
bench: ## Run benchmarks.
	go test -run=^$$ -bench=. -benchmem ./...

.PHONY: check
check: lint vuln cover-check ## Everything CI runs: lint, vuln, tests, coverage gate.

##@ Docs & release

.PHONY: docs-cli
docs-cli: ## Regenerate CLI reference markdown + man pages.
	go run ./internal/cli/gendocs -out docs/commands -man man

.PHONY: docs
docs: docs-cli ## Build the documentation site (requires mkdocs-material).
	mkdocs build --strict

.PHONY: snapshot
snapshot: ## Build a local, unpublished release with GoReleaser.
	goreleaser release --snapshot --clean

.PHONY: clean
clean: ## Remove build and test artifacts.
	rm -rf bin dist site man $(COVER_PROFILE) coverage.html
