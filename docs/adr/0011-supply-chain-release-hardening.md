# 0011 — Supply-chain / release hardening

**Status**: Accepted

## Context

The initial CI gate ran only `gofmt`, `go vet`, race tests, and a build — no vulnerability
scanning, no SAST, no linter. All GitHub Actions across `ci.yml` and `release.yml` were
referenced by **mutable tags** (`@v4`, `@v3`, …). A re-pointed tag or repo compromise would
run unreviewed code with the workflow token and secrets. Known-CVE dependencies or insecure
code patterns could merge unflagged. The release pipeline lacked a CycloneDX SBOM,
checksums, and reproducible-build flags. The container image ran as root.

## Decision

The following hardening is applied in the v0.1 release:

1. **CGO off**: all builds set `CGO_ENABLED=0` so the binary is statically linked and the
   minimal Docker image requires no C runtime.
2. **Non-root container image**: the Dockerfile uses a multi-stage build; the final stage
   copies only the binary and CA certificates from the builder, and runs as `USER 10001`
   (never root). CA certs are copied from `ca-certificates` in the build stage — not
   installed at container runtime with `apk add` (which would add download-time supply-chain
   risk and require network access).
3. **SHA-pinned Actions**: every GitHub Action is pinned to a full commit SHA with a
   trailing `# vX.Y.Z` comment. A `.github/dependabot.yml` covers ecosystems
   `github-actions`, `gomod`, and `docker`, with `persist-credentials: false` in all
   workflows so the workflow token is not exposed to checked-out code.
4. **GoReleaser + CycloneDX SBOM**: `.goreleaser.yaml` (schema v2) cross-compiles
   `linux,darwin × amd64,arm64`, produces `tar.gz` archives with bundled `LICENSE`/
   `README.md`/`config.yaml`, a `checksums.txt`, a CycloneDX SBOM via `cyclonedx-gomod`
   (consistent with `make sbom`), and reproducible-build flags (`-trimpath`). The
   multi-arch GHCR image uses `docker/metadata-action` for tags and builds with max-mode
   provenance attestation.
5. **`make ci` gate**: `gofmt` check, `go vet`, `golangci-lint`, `go test -race`,
   `govulncheck` — all must pass before merge. `make sure` reproduces this locally.

## Consequences

- Workflows execute only immutable, reviewed Action code; tag-repoint attacks are
  neutralised. Dependabot keeps pins fresh via reviewable PRs.
- CVE-bearing dependencies and insecure code patterns fail CI before merge.
- The container is non-root by default, reducing the blast radius of any container escape.
- CA cert copying (no `apk add`) means the final image has no package manager and no
  extra network calls at container start.
- `make release-snapshot` reproduces the GoReleaser pipeline locally without a tag push.
- The injected `main.version` uses GoReleaser's `{{ .Version }}` (no `v` prefix).
