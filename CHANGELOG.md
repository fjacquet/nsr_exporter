# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Versions predating this file are reconstructed from git history; their entries
summarize each release at the level the commit messages support.

## [Unreleased]

### Added

- `/livez` and `/readyz` endpoints, both always answering `200 OK` with an `ok`
  body and reading no collection state whatsoever (ADR-0012).
- `HEALTHCHECK` against `/livez` in `Dockerfile` and `Dockerfile.goreleaser`, and
  a matching `healthcheck:` in `docker-compose.yml` and `docker-compose.ghcr.yml`.
- `main_test.go` — this repo's first server test, covering the probes and
  `/health` both before and after the first snapshot.

### Changed

- `/health` no longer returns `503` during the startup window before the first
  collection cycle. It always returns `200`; the startup state moved into the
  body (`starting` / `ok`). Anything scripting against the `503` must now read
  the body instead.
- The Helm chart's `livenessProbe` and `readinessProbe` default to `/livez` and
  `/readyz` instead of `/health`.
- Container base image is `alpine:latest`, unpinned, matching the rest of the
  exporter family; the previous `alpine:3.24` pin is dropped.

## [0.12.4] - 2026-07-12

### Changed

- Dependency bump only: Go toolchain updated to 1.26.5, plus corresponding
  module updates.

## [0.12.3] - 2026-07-10

### Added

- Multi-arch GHCR image publishing restored, alongside the Go 1.26.5 bump.
- MkDocs site now uses the brand icon as favicon and logo.

### Changed

- GitHub Actions bumps: `actions/checkout` to 7.0.0, `azure/setup-helm` to 5.0.1.

## [0.12.2] - 2026-06-20

### Changed

- Fleet health refresh across CI configuration.
- CI security workflow made advisory, matching the family's central default.
- CI migrated to the `fjacquet/ci` reusable, Makefile-based workflows.

## [0.12.1] - 2026-06-16

### Added

- Helm chart, with a lockstep publishing workflow alongside container releases.

## [0.12.0] - 2026-06-14

### Breaking

- Canonical metrics port assigned as 9447 (previously a different port).

### Added

- Node Exporter Full (dashboard 1860) companion Grafana dashboard.

## [0.11.0] - 2026-06-14

### Added

- Grafana dashboard refinements: units, thresholds, a NOC overview, and
  freshness/forecast panels, per an accompanying design spec.

## [0.10.2] - 2026-06-14

### Added

- ADR-0004 documenting the dual export model, including degradation behaviour
  and wrapped envelopes.

## [0.10.1] - 2026-06-14

### Added

- Windows binaries added to the GoReleaser build matrix.

## [0.10.0] - 2026-06-14

### Fixed

- Case-insensitive priority matching in the Grafana overview alert panels.
- REST API field extraction corrected across all collectors.

## [0.9.1] - 2026-06-13

### Changed

- ADR navigation in the docs site made dynamic via the `awesome-pages` MkDocs
  plugin; CI bumped to Node 24.

## [0.9.0] - 2026-06-13

### Added

- Six additional resource collectors (C1-C10 series): client `lastBackupTime`
  and `operatingSystem`, volume status, alerts-acknowledged label, jobs timing
  and grouping, plus further collector fields.
- OTLP push export path: configurable `opentelemetry` block, gRPC transport
  moved into `internal/nsr`, and an `nsr_otlp_export_errors_total` health
  metric.
- `docker-compose` + Prometheus + Grafana observability stack, with new
  dashboards (`nsr-devices`, `nsr-protection`) and extended alert rules
  covering devices, protection, and VMware.
- ADRs 0002 (modular resource collectors), 0005 (config hot reload), 0006
  (label-key consistency invariant), 0008 (absent-never-zero parsing), 0009
  (metric naming & units), and 0011 (supply-chain / release hardening).
- `make all` target (ci + cli + sbom + docker).

### Fixed

- CodeRabbit review findings addressed across two rounds, including a
  base-scoped fix.

## [0.1.1] - 2026-06-13

### Fixed

- Homebrew cask token now uses the plain `{{ .Env.X }}` form in the release
  config.

## [0.1.0] - 2026-06-13

### Added

- Initial `nsr_exporter` scaffold: snapshot model, dual (Prometheus + OTLP)
  export, alerts collector, and CLI.
- Jobs, sessions, storage, and sizing collectors.
- CI workflow trio, GoReleaser release pipeline, Dependabot, and Dockerfiles.
- README (with badges), MkDocs Material documentation site, and MIT license.

### Changed

- Alpine base image bumped from 3.21 to 3.24.
- Various CI action version bumps (docker/build-push-action, goreleaser-action,
  actions/deploy-pages, actions/upload-artifact, actions/setup-python,
  actions/setup-go).
