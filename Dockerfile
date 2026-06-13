# Local/dev multi-stage build. The release image is built by GoReleaser via
# Dockerfile.goreleaser. Non-root USER is mandatory (CI + semgrep enforce it).
FROM golang:1.26-alpine AS builder
WORKDIR /src
# Cache deps first.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/nsr_exporter .

FROM alpine:3.21
# Copy the CA bundle from the Debian-based builder rather than `apk add
# ca-certificates`: apk fetches the index over TLS from the Alpine CDN, which fails
# behind a corporate MITM proxy because the bare alpine image has no CA bundle yet
# to validate the proxy cert (chicken-and-egg). adduser is a busybox builtin.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN adduser -D -u 10001 nsr
COPY --from=builder /out/nsr_exporter /usr/local/bin/nsr_exporter
USER 10001
EXPOSE 9097
ENTRYPOINT ["/usr/local/bin/nsr_exporter"]
CMD ["--config", "/etc/nsr_exporter/config.yaml"]
