# 0003 — Hand-rolled resty REST client

**Status**: Accepted

## Context

The family rule: use the official vendor Go SDK if it is available **and** useful
(modern auth, batched stats, models the exported objects, no regression). Otherwise
hand-roll a lean `resty/v2` client and record this ADR.

## Decision

Dell publishes **no Go SDK** for the NetWorker REST API (the official integration is the
`dell/ansible-networker` Python collection). Criterion 1 (available) fails outright, so we
hand-roll a lean `go-resty/resty/v2` client (`internal/nsrclient`).

The client targets `https://<host>:9090/nwrestapi/v3/global`, uses NetWorker's native
`fl=` field projection and `q=` filter query params, decodes the wrapped
`{"count":N,"<resource>":[…]}` envelopes, applies bounded retry that excludes 4xx, and
installs a credential-safe body-only trace hook. Endpoints and auth were validated against
`dell/ansible-networker` (`plugins/module_utils/*_api.py`, `plugins/modules/clients.py`).

**List responses are wrapped envelopes, not bare arrays.** Every NetWorker list endpoint
returns `{"count":N,"<resource>":[…]}` where `<resource>` is the endpoint's named
collection field (`clients`, `alerts`, `volumes`, …). The design spec's "decode JSON
arrays" is imprecise: decoders MUST unwrap the named field, never `json.Unmarshal` into a
`[]T` directly. Each collector declares the field name it expects and unwraps exactly that
key; a missing or renamed field yields an empty collection (and a per-target failure via
the graceful-degradation path in ADR-0001), never a decode panic.

## Consequences

- Full control over field projection and the `/backups` bounding strategy (ADR-0010) that
  no generic SDK would give.
- We own the request/response types and their tolerant parsing.
- No heavyweight or version-pinning SDK dependency.
