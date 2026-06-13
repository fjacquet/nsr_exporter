# nsr_exporter — family-standard Makefile. CI reproduces locally: everything CI
# runs is a target here.

BINARY      := nsr_exporter
PKG         := github.com/fjacquet/nsr_exporter
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)
GO          ?= go

.DEFAULT_GOAL := sure

.PHONY: all tools fmt fmt-check vet lint test test-race test-coverage vuln ci sure \
        cli sbom release release-snapshot docker run-cli clean

tools: ## install pinned dev tools
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	$(GO) install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest

fmt: ## format
	gofmt -w .

fmt-check: ## fail if unformatted
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "unformatted:"; echo "$$out"; exit 1; fi

vet:
	$(GO) vet ./...

lint:
	golangci-lint run ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-coverage:
	$(GO) test -coverprofile=coverage.out ./... && $(GO) tool cover -html=coverage.out -o coverage.html

vuln:
	govulncheck ./...

# The CI gate.
ci: fmt-check vet lint test-race vuln

# Local convenience.
sure: fmt vet test cli lint

# Full local pipeline: CI gate + every build artifact (binary, SBOM, image).
all: ci cli sbom docker

cli: ## build bin/nsr_exporter
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

sbom: ## CycloneDX module SBOM
	cyclonedx-gomod mod -licenses -json -output sbom.json

release: ## GoReleaser full release
	goreleaser release --clean

release-snapshot: ## GoReleaser local dry-run (no publish)
	goreleaser release --snapshot --clean

docker:
	docker build -t $(BINARY):$(VERSION) .

run-cli: cli ## build and run against config.yaml
	./bin/$(BINARY) --config config.yaml

clean:
	rm -rf bin coverage.out coverage.html sbom.json dist site
