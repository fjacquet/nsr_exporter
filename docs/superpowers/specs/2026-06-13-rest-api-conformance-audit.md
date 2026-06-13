# NetWorker REST API Conformance Audit

**Date:** 2026-06-13
**Scope:** All 13 collectors in `internal/nsr/`
**Authoritative sources:**
1. `docs/pdf/n.txt` — Dell EMC NetWorker **19.2** REST API Reference Guide (field names, types, example JSON, Models). *Primary source for read-path field names and types.*
2. `/Users/fjacquet/Projects/ansible-networker` — Dell's official Ansible collection. Corroborates endpoint paths and write-side field names; thin pass-through, so it confirms names but rarely read-side types.

**Verdict:** Nearly every collector has at least one HIGH finding. Two systemic root causes dominate:
- **Size/BitRate values are JSON objects `{"unit","value"}`, not scalars.** Decoding them into `*float64` silently yields `nil` → the metric **never emits a single sample** and no error is raised.
- **The C5–C10 expansion collectors were scaffolded from inferred field names that were never validated.** Many reference fields — and in two cases entire endpoints — that do not exist in the 19.2 API.

---

## Cross-cutting themes

### T1 — `Size`/`BitRate` object decoded as scalar (HIGH, silent total data loss)
Spec `Size` model: `{"unit":"Byte"|"KB","value":<int64>}`. `BitRate`: `{"unit":"Byte/s"|"KB/s","value":<int64>}`. A `*float64`/`float64` target receives a JSON object → decoder leaves it `nil`/zero, no error. Affected:
- `sizing.go` `size` → every backup skipped by the `if bk.Size == nil { continue }` guard → **all six sizing metrics emit nothing.**
- `sessions.go` `size` → `nsr_session_bytes` never emits; `transferRate` (BitRate) not decoded at all → live-throughput gauge entirely missing.
- `jobs.go` serverstatistics `saveSize`, `recoverSize` → `nsr_server_save_size_bytes` / `_recover_size_bytes` never emit.
- `storage.go` volumes `capacity`, `written` → `nsr_volume_capacity_bytes` / `_written_bytes` never emit.

**Fix:** one shared helper, e.g. `internal/nsr/quantity.go`:
```go
type nwSize struct { Unit string `json:"unit"`; Value float64 `json:"value"` }
func (s *nwSize) Bytes() (float64, bool) { // (value, present)
    if s == nil { return 0, false }
    switch s.Unit { case "KB": return s.Value * 1024, true; default: return s.Value, true }
}
```
plus an analogous `nwBitRate.BytesPerSecond()` (`KB/s` → ×1024). Localizes the conversion in one place (consistent with the "tolerant parsing in one place" constraint).

### T2 — DataDomain capacities are human-readable strings, not numbers (HIGH)
`storage.go` `nwDataDomain` decodes capacity as `*float64`, but the spec returns **strings** like `"365 TB"`, `"129 GB"`, `"2852 GB"`. AND the field names are wrong (see T3). A string→bytes parser is required (split magnitude + IEC/SI suffix). Affected metrics never emit today.

### T3 — Wrong / phantom field names
| Collector | Code field | Real field (n.txt / ansible) | Verdict |
|---|---|---|---|
| alerts | `severity` | `priority` | RENAME |
| alerts | `time` | `timestamp` | RENAME (struct tag `json:"time"` never decodes) |
| alerts | `acknowledged` | — | NOT IN SPEC (always false) |
| clients | `lastBackupTime` | — | NOT IN SPEC → `nsr_client_last_backup_timestamp_seconds` never emits |
| clients | `operatingSystem` | — | NOT IN SPEC (label always "") — consider `backupType`/`networkerVersion` |
| jobs | `client` | `clientHostname` | RENAME |
| jobs | `group` | — | NOT IN SPEC (label always "") |
| jobs | `level` | — | NOT IN SPEC on Job (it's a Backup attr) |
| sessions | `type` | `mode` (Saving/Recovering/Browsing) | RENAME |
| sessions | `client` | `clientHostname` | RENAME |
| sessions | `state` | — (use `completed`/`stopped` bools) | NOT IN SPEC |
| sizing/backups | `client` | `clientHostname` | RENAME |
| sizing/backups | `pool` | — (not on Backup; only Volume/Session) | NOT IN SPEC |
| sizing/backups | `duration` | — (derive `completionTime − saveTime`) | NOT IN SPEC |
| volumes | `mediaType` (tag) | `type` | RENAME (reversed from intuition) |
| volumes | `recycledCount` | `recycled` | RENAME |
| volumes | `status` | `states` (array[String]) | NOT IN SPEC as scalar |
| pools | `type` | `poolType` | RENAME |
| pools | `capacityTotal`,`capacityUsed`,`volumeCount` | — | NOT IN SPEC (no capacity on Pool) |
| devices | `type` | `mediaType` / `mediaFamily` | RENAME |
| devices | `serialNumber` | `deviceSerialNumber` | RENAME |
| devices | `capacity` | — | NOT IN SPEC (no capacity on Device) |
| storagenodes | `status` | `enabled` (bool) | RENAME/RETYPE |
| storagenodes | `deviceCount` | `numberOfDevices` | RENAME |
| policies | `enabled` (policy top-level) | — (lives on `workflows[].enabled`) | NOT IN SPEC → `nsr_policy_enabled` always 0 |
| policies | `clientCount` | — | NOT IN SPEC |
| groups | `policy` | — (inverse: `workflow.protectionGroups[]`) | NOT IN SPEC |
| groups | `clientCount` | — | NOT IN SPEC |

### T4 — Wrong endpoints (HIGH, collector entirely dead)
- **`vmware.go` calls `/vmwares`** — does not exist. Real: `GET /vmware/vcenters`, wrapper `vCenters`, identity field `hostname` (ansible `vmware_api.py:206`, `get_v_center` at `:143`). `version`/`connectionStatus` do not exist on a vCenter. Every poll 404s.
- **`queues.go` calls `/queues`** — does not exist in 19.2. ansible-networker only has `/queue/{id}` (singular, by numeric task id) and its own `queues.py` module is broken (imports AlertsApi). There is **no enumerable queue list**. `nsr_queue_depth`/`nsr_queue_wait_seconds` can never emit.

### T5 — Wrapper-key case (LOW; Go's decoder is case-insensitive, but `fl=` projection sent to server uses wrong case)
`storagenodes` (`storagenodes`→`storageNodes`), `policies` (`protectionpolicies`→`protectionPolicies`), `groups` (`protectiongroups`→`protectionGroups`), `datadomainsystems` (→`dataDomainSystems`). Decode survives; the wire-level `fl=` field names may be silently ignored if the server is case-sensitive.

### T6 — `/backups` q= window filter unconfirmed (HIGH risk per ADR-0010)
`sizing.go` sends `savetime>'MM/DD/YYYY HH:MM:SS'`. Neither source documents the `>` operator or that date format for the REST `q=`. The spec documents only exact-match `AttributeName:Value`; ansible builds `q=key: value and …` (equality only). If `>` is rejected/ignored, the query may return the **entire catalog unbounded** — the exact failure ADR-0010 exists to prevent. **Must validate live** before trusting; consider a documented epoch/RFC3339 form. Field case is also `saveTime` (model) vs `savetime` (code).

### T7 — Prometheus naming: `_total` on gauges (LOW)
`nsr_alerts_total`, `nsr_sessions_total` are live counts (gauges) but the `_total` suffix implies a counter; `rate()` users will be misled. Rename to `nsr_alerts_active` / `nsr_active_sessions` or reclassify.

### T8 — `level` bucketing too narrow (LOW)
`sizing.go isFullLevel` treats only `"full"` as Full; spec enum has 16 values (`Full`, `Incr`, `SynthFull`, `IncrSynthFull`, `Consolidate`, `Migration`, `1`–`9`, `Manual`, `Skip`, `TxnLog`). `SynthFull`/`IncrSynthFull`/`Consolidate` are misbucketed as incremental.

---

## Per-collector severity summary

| Collector | HIGH | MED | LOW | Net status |
|---|---|---|---|---|
| alerts.go | `time`→`timestamp` tag (timestamp label always ""); `severity`→`priority` (severity breakdown collapses to one ""-series) | `priority` not projected | `acknowledged` phantom | Emits, but severity dimension + timestamp broken |
| clients.go | `lastBackupTime` phantom → timestamp metric dead | `operatingSystem` phantom (empty label) | — | Core info OK; 1 metric dead, 1 empty label |
| jobs.go | `saveSize`/`recoverSize` Size-object → 2 metrics dead; `client`→`clientHostname`; `group`/`level` phantom (empty labels) | `currentSaves`/`currentRecovers`/`maxSaves` not collected | `version` not exposed; int-as-float | Partially working; size metrics dead, several empty labels |
| sessions.go | `size` Size-object → `nsr_session_bytes` dead; `type`/`client`/`state` all phantom → all 3 labels "" | `transferRate` BitRate gauge missing | `nsr_sessions_total` collapses on ""-type | Structurally hollow |
| storage.go | volumes `capacity`/`written` Size-object dead; 4 DD capacity fields wrong-name+string-typed → all dead; `mediaType`→`type`; `recycledCount`→`recycled` dead | `fl=` names wrong | DD wrapper case | Most storage metrics dead |
| sizing.go | `size` Size-object → **all 6 metrics dead**; `duration` phantom → throughput/duration dead; `client`→`clientHostname` | `pool` phantom; q= filter unverified (T6) | level bucketing | Collector emits nothing today |
| pools.go | `capacityTotal`/`capacityUsed`/`volumeCount` phantom → all 3 metrics dead; `type`→`poolType` | label-key inconsistency (latent) | poolType enum comment wrong | Only name/poolType salvageable |
| queues.go | `/queues` endpoint does not exist → collector dead | — | — | Remove or re-scope |
| catalog.go | (registry) entries for dead metrics above; `_total` gauges | — | — | Mirror fixes |
| vmware.go | `/vmwares` wrong endpoint → dead; `vmwares`→`vCenters`; `name`→`hostname`; `version`/`connectionStatus` phantom | — | — | Rebuild against `/vmware/vcenters` |
| storagenodes.go | wrapper `storagenodes`→`storageNodes` (fl= wire); `deviceCount`→`numberOfDevices` dead; `status`→`enabled` | missing rich fields | — | Device-count dead, status empty |
| devices.go | `capacity` phantom → metric dead; `type`→`mediaType`/`mediaFamily`; `serialNumber`→`deviceSerialNumber` | — | status enum comment (`Service` not `offline`) | Capacity dead, labels empty |
| policies.go | `enabled` phantom → `nsr_policy_enabled` always 0; `clientCount` phantom (policy+group) → 2 metrics dead; `group.policy` phantom | wrapper case | — | Needs redesign (read `workflows[]`) |

---

## Recommended fix plan (sequenced)

**Phase A — kill the systemic Size bug (unblocks the most metrics):**
1. Add `internal/nsr/quantity.go` with `nwSize`/`nwBitRate` + unit-aware `Bytes()`/`BytesPerSecond()` and a `parseHumanSize("365 TB")` helper for DataDomain. TDD with fixtures.
2. Apply to `sizing.go`, `sessions.go`, `jobs.go` (serverstats), `storage.go` (volumes). Update fixtures in `internal/nsr/testdata/` to the real object/string shapes from the spec example JSON.

**Phase B — field-name corrections (renames + drops):** alerts, clients, jobs, sessions, sizing, volumes, pools, devices, storagenodes per the T3 table. Drop phantom metrics (`nsr_pool_*`, `nsr_device_capacity_bytes`, `nsr_*_client_count`, `nsr_client_last_backup_timestamp_seconds`) from both the collector and `catalog.go`, or re-source them (notes below). Fix wrapper-key case and `fl=` lists.

**Phase C — broken endpoints:**
- Rebuild `vmware.go` against `GET /vmware/vcenters` (`vCenters`/`hostname`); decide whether VM-level metrics come from `/vmware/vcenters/{hostname}/vms`.
- Remove `queues.go` + its catalog entries (no list endpoint), or gate behind a future-version note.

**Phase D — re-sourcing & semantics:**
- Derive `nsr_job_duration_seconds`/`nsr_job_bytes_per_second` from `completionTime − saveTime` on the Backup, not a phantom `duration`.
- Pool capacity, if wanted, must aggregate `Volume.capacity` grouped by `pool` from `/volumes`.
- `nsr_policy_enabled` from `workflows[].enabled`; group→policy via inverse `workflow.protectionGroups[]`.
- Add `currentSaves`/`currentRecovers`/`maxSaves` (real live gauges) to jobs.
- Rename `_total` gauges (T7); widen `isFullLevel` (T8).

**Phase E — live validation (cannot be settled from docs):**
- The `/backups` `q=` save-time window operator + date format (T6) — the single biggest open risk. Validate with `bin/nsr_exporter --once --trace` against a real appliance before trusting the bound.
- Confirm `Size.unit` values actually returned (Byte vs KB) and DataDomain capacity string grammar.

---

## Confidence & limits
- **High confidence:** field-name renames and phantom-field findings — corroborated by both the spec Models/example JSON and (where surfaced) ansible-networker argument specs.
- **Medium:** exact read-side types for resources where ansible is pure pass-through (sessions, jobs, backups) rest on the spec example JSON alone — solid but worth a live trace.
- **Open (needs live appliance):** T6 q= filter syntax; exact `Size.unit` distribution; DataDomain capacity string format. Version skew: target spec is 19.2; CLAUDE.md mentions 19.x/19.7 docs also present — a 19.7 appliance may add fields (e.g. the singular `/queue`).
