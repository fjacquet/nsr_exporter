# syntax=docker/dockerfile:1
# Build context is the repo root: docker build -f cmd/mocknw/Dockerfile .
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/mocknw ./cmd/mocknw

FROM alpine:3.21
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN adduser -D -u 10001 mocknw
COPY --from=build /out/mocknw /usr/local/bin/mocknw
USER 10001
EXPOSE 9090
ENTRYPOINT ["/usr/local/bin/mocknw"]
