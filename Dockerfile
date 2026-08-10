# Local/dev multi-stage build. The release image is built by GoReleaser via
# Dockerfile.goreleaser. Non-root USER is mandatory (CI + semgrep enforce it).
FROM docker.io/library/golang:1.26-alpine AS builder
WORKDIR /src
# Cache deps first.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/nsr_exporter .

# Unpinned per the family standard (spec decision 5, ADR-0012): all fifteen
# exporter repos share alpine:latest. This is the one build input whose contents
# can change between two builds of the same commit; uniformity was chosen over
# reproducibility, and revisiting it is a family-wide decision.
FROM docker.io/library/alpine:latest
# Copy the CA bundle from the Debian-based builder rather than `apk add
# ca-certificates`: apk fetches the index over TLS from the Alpine CDN, which fails
# behind a corporate MITM proxy because the bare alpine image has no CA bundle yet
# to validate the proxy cert (chicken-and-egg). adduser is a busybox builtin.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN adduser -D -u 10001 nsr
COPY --from=builder /out/nsr_exporter /usr/local/bin/nsr_exporter
USER 10001
EXPOSE 9447

# Probes /livez, never /metrics: rendering the full exposition on every probe
# tick is needless load and can block behind a slow collection cycle (ADR-0012).
# 127.0.0.1, never localhost — busybox wget tries ::1 first and the exporter
# binds IPv4 only, so a localhost check fails with connection refused.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9447/livez || exit 1

ENTRYPOINT ["/usr/local/bin/nsr_exporter"]
CMD ["--config", "/etc/nsr_exporter/config.yaml"]
