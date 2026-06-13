# 0007 — HTTP Basic authentication

**Status**: Accepted

## Context

The family default auth pattern is a modern bearer + token-refresh flow (e.g. PowerFlex
`/rest/auth/login` + `/rest/auth/update-token`). This ADR records where NetWorker
deliberately deviates.

## Decision

The NetWorker REST API authenticates with **HTTP Basic on every request** — no login
endpoint, no token, no refresh. Confirmed against `dell/ansible-networker`
(`plugins/modules/clients.py:382` `auth=(user,password)`, passed to every request). The
client therefore sets `SetBasicAuth(user, pass)` once and reuses it.

Retry policy still **excludes 4xx**: a 401/403 is a credential problem, never retried.
TLS minimum version is 1.2; `insecureSkipVerify` is an operator opt-in for self-signed
appliance certificates and defaults to off.

Because Basic auth places base64 credentials in the **request** `Authorization` header,
the `--trace` hook logs response **body only** and never headers, and must never use resty
`SetDebug` (which would dump the header).

## Consequences

- Connection management is far simpler than the Data Domain / PowerFlex token dance.
- Credentials travel on every request — TLS verification matters; document the
  `insecureSkipVerify` risk.
- Deviates from the family bearer+refresh norm by backend design, hence this ADR.
