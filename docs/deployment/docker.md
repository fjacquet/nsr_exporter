# Docker Deployment

`nsr_exporter` ships as a minimal Alpine-based Docker image (non-root `USER 10001`).
Use the compose stacks for a complete observability environment, or pull the image
standalone for integration into an existing stack.

## Image locations

| Use case | Image |
|---|---|
| Local build (dev) | Built by `docker compose up --build` from `./Dockerfile` |
| Published release | `ghcr.io/fjacquet/nsr_exporter:<tag>` |

## Quick start — build from source

```bash
# Clone and copy quickstart credentials
git clone https://github.com/fjacquet/nsr_exporter.git
cd nsr_exporter
cp .env.example .env
# Edit .env: set NSR1_USERNAME, NSR1_PASSWORD, and optionally GF_ADMIN_PASSWORD.

docker compose up --build
```

Open Grafana at <http://localhost:3000> (default admin/admin).
Prometheus UI is at <http://localhost:9090>.
The exporter metrics endpoint is at <http://localhost:9097/metrics>.

## Quick start — published image (no build)

```bash
cp .env.example .env
# Edit .env with real credentials, then:

NSR1_PASSWORD='your-password' docker compose -f docker-compose.ghcr.yml up -d
```

Pin a specific release:

```bash
NSR_TAG=0.2.0 NSR1_PASSWORD='...' docker compose -f docker-compose.ghcr.yml up -d
```

## Configuration

The exporter reads `/etc/nsr_exporter/config.yaml` inside the container. The compose
stacks mount `./config.demo.yaml` there (so the exporter scrapes the bundled mock);
mount your own `./config.yaml` for a real target. See `config.yaml` for the full schema; key fields:

| Field | Default | Purpose |
|---|---|---|
| `server.port` | `9097` | `/metrics` listen port |
| `collection.interval` | `5m` | Background poll frequency |
| `collection.timeout` | `60s` | Per-system API deadline |
| `collection.backupWindow` | `24h` | Bound for `/backups` queries (ADR-0010) |
| `systems[].insecureSkipVerify` | `false` | Skip TLS verification (dev only) |

`${ENV_VAR}` references in `host`, `username`, and `password` expand at load time;
an unset variable causes a fail-fast startup error. Use `passwordFile` to supply
secrets out-of-band without embedding them in the compose environment.

## OTLP push export (optional)

The compose stacks include an `otel-collector` service. The collector exposes two ports:

| Port | Protocol | Purpose |
|---|---|---|
| `4317` | gRPC | Receives OTLP metrics pushed by the exporter |
| `8889` | HTTP | Prometheus scrape endpoint — the collector re-exposes pushed metrics here |

`config.demo.yaml` ships with the OTLP endpoint pre-configured so the bundled
`docker compose up` stack automatically pushes metrics to the collector.

To enable OTLP in your own `config.yaml`:

```yaml
opentelemetry:
  endpoint: "otel-collector:4317"
  insecure: true       # use false and configure TLS for production collectors
  pushInterval: "30s"  # how often to push; default is 30s
```

Or set the standard OTEL env var (takes precedence over the config file):

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317
```

Once running, verify the collector is receiving metrics:

```bash
curl -s http://localhost:8889/metrics | grep '^nsr_'
```

The `nsr_otlp_export_errors_total` counter on the exporter's own `/metrics` increments
whenever a push fails — use it to alert on OTLP outages:

```promql
increase(nsr_otlp_export_errors_total[5m]) > 0
```

## Health check

```bash
curl -s http://localhost:9097/metrics | grep '^nsr_up'
```

A `1` means the NetWorker system responded in the last collection cycle.

## Production notes

- Change `GF_ADMIN_PASSWORD` before exposing Grafana externally.
- Set `GF_SERVER_ROOT_URL` in `.env` so share links resolve correctly.
- Pin `NSR_TAG` to a release version instead of `:latest` for reproducible deploys.
- Run behind a TLS-terminating reverse proxy (nginx, Caddy, Traefik) for external access.
- Never commit `.env` or `config.yaml` containing real credentials to version control.
