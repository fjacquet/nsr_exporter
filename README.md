# nsr_exporter

[![CI](https://github.com/fjacquet/nsr_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/fjacquet/nsr_exporter/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/fjacquet/nsr_exporter?include_prereleases&sort=semver)](https://github.com/fjacquet/nsr_exporter/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/fjacquet/nsr_exporter)](https://goreportcard.com/report/github.com/fjacquet/nsr_exporter)
[![Go](https://img.shields.io/github/go-mod/go-version/fjacquet/nsr_exporter)](go.mod)
[![License](https://img.shields.io/github/license/fjacquet/nsr_exporter)](LICENSE)
[![Docs](https://img.shields.io/badge/docs-mkdocs-blue)](https://fjacquet.github.io/nsr_exporter/)

A **Prometheus + OTLP** metrics exporter for **Dell EMC NetWorker** backup servers.

A single process polls every configured NetWorker system on a background interval,
caches an immutable snapshot, and serves `/metrics` (and an OTLP push) instantly from
cache — so scrapes never reach the backend. Ships as a static, dependency-free binary.

## Features

- **Decoupled snapshot model** — backend API load is constant regardless of scraper count.
- **Dual export** — Prometheus `/metrics` and OTLP push, from the same snapshot.
- **Multi-system** — one process serves many servers; every metric carries `system="<name>"`.
- **Safe on large catalogs** — the `/backups` sizing query is time-bounded (never the full catalog).
- **Operable** — `--once --debug` sample dump, credential-safe `--trace`, SIGHUP reload, `/health`.

## Quick start

```bash
# Build
make cli

# Try it offline against the bundled mock NetWorker server
go run ./cmd/mocknw &                 # listens on :9090
cp config.yaml my.yaml                # point a system at http://127.0.0.1:9090
NSR1_USERNAME=admin NSR1_PASSWORD=test ./bin/nsr_exporter --config my.yaml --once --debug
```

Then scrape `http://localhost:9097/metrics`. See [`docs/metrics.md`](docs/metrics.md) for
the metric catalog and [`config.yaml`](config.yaml) for configuration.

## Configuration

`config.yaml` is the source of truth; `${ENV_VAR}` references in host/username/password
expand at load (fail-fast on unset), and `passwordFile` supplies secrets out-of-band. A
`.env` beside the config is a quickstart convenience and never overrides real env vars.

## Authentication

NetWorker uses **HTTP Basic auth** on every request against `/nwrestapi/v3/global`
(no token dance). `insecureSkipVerify` is an operator opt-in for self-signed appliance
certificates. See [ADR-0007](docs/adr/0007-http-basic-auth.md).

## Development

```bash
make ci        # the gate: fmt-check, vet, lint, test-race, govulncheck
make sure      # local convenience: fmt, vet, test, build, lint
make test      # unit tests (httptest mock; no appliance needed)
```

> **Field-name validation.** Collectors for `serverstatistics`, `jobs`, `sessions`,
> `volumes`, `datadomainsystems` use field names inferred from the NetWorker REST
> convention (marked `// INFERRED` in code). Confirm them against a live appliance with
> `nsr_exporter --config real.yaml --once --debug --trace 2>trace.log | sort > samples.txt`;
> a wrong guess surfaces as a *missing* metric, never a false zero.

## License

[MIT](LICENSE)
