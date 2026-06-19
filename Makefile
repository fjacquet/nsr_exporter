# nsr_exporter — fjacquet/ci standard Makefile. CI reproduces locally: everything
# CI runs is a target here.

BINARY      := nsr_exporter
PKG         := github.com/fjacquet/nsr_exporter
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)
GO          ?= go
DIST        ?= dist
COVER       ?= coverage.out

# Pinned tool versions (installed by `make tools`).
GOLANGCI_VERSION   ?= v2.12.2
GORELEASER_VERSION ?= v2.16.0

.DEFAULT_GOAL := sure

.PHONY: all clean install tools fmt fmt-check vet lint format test test-race \
        test-coverage vuln ci sure sbom security docs coverage-upload release \
        release-snapshot build cli docker run-cli

# ── canonical targets (fjacquet/ci interface) ────────────────────────────────

all: clean lint test build

clean:
	rm -rf bin $(DIST) site $(COVER) *.sarif coverage.html

install:
	$(GO) mod download

tools: ## install pinned dev tools
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	$(GO) install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest
	$(GO) install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)

lint:
	golangci-lint run --timeout=5m

format:
	golangci-lint fmt

test:
	$(GO) test -race -coverprofile=$(COVER) -covermode=atomic ./...

build:
	$(GO) build -v ./...

vuln:
	govulncheck ./...

sbom: ## CycloneDX module SBOM
	mkdir -p $(DIST)
	cyclonedx-gomod mod -licenses -json -output $(DIST)/sbom.cdx.json
	@echo "wrote $(DIST)/sbom.cdx.json"

security:
	uvx semgrep scan --config auto --error --skip-unknown-extensions

docs:
	uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict --site-dir site

coverage-upload:
	uvx --from codecov-cli codecov upload-process --file $(COVER) || true

release: ## GoReleaser full release
	goreleaser release --clean

ci: lint test build vuln

# ── repo-specific targets (preserved) ────────────────────────────────────────

fmt: ## format
	gofmt -w .

fmt-check: ## fail if unformatted
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "unformatted:"; echo "$$out"; exit 1; fi

vet:
	$(GO) vet ./...

test-race:
	$(GO) test -race ./...

test-coverage:
	$(GO) test -coverprofile=coverage.out ./... && $(GO) tool cover -html=coverage.out -o coverage.html

# Local convenience.
sure: fmt vet test cli lint

cli: ## build bin/nsr_exporter
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

release-snapshot: ## GoReleaser local dry-run (no publish)
	goreleaser release --snapshot --clean

docker:
	docker build -t $(BINARY):$(VERSION) .

run-cli: cli ## build and run against config.yaml
	./bin/$(BINARY) --config config.yaml
