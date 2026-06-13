package nsr

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// backupsResponse wraps GET /backups: {"count":N,"backups":[...]}.
type backupsResponse struct {
	Backups []nwBackup `json:"backups"`
}

// nwBackup mirrors the swagger 19.13 Backup model. The client field is
// `clientHostname`; `size` is a Size object; there is no `duration` or `pool`
// field — duration is derived from completionTime − saveTime.
type nwBackup struct {
	ClientHostname string  `json:"clientHostname"`
	Name           string  `json:"name"` // saveset path/name
	Level          string  `json:"level"`
	Size           *nwSize `json:"size"`
	SaveTime       string  `json:"saveTime"`
	RetentionTime  string  `json:"retentionTime"`
	CompletionTime string  `json:"completionTime"`
}

// SizingCollector maps the bounded GET /backups to capacity-forecasting metrics.
// It is constructed with a lookback window so the query is always bounded via the
// server-relative saveTime offset filter (ADR-0010) — never fetch the full catalog.
type SizingCollector struct {
	Window time.Duration
}

// Name identifies the sizing collector.
func (SizingCollector) Name() string { return "sizing" }

// Collect fetches a bounded slice of the backup catalog and aggregates FETB,
// change size, retention, and throughput per client/saveset.
func (s SizingCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp backupsResponse
	if err := c.Get(ctx, "/backups", nsrclient.QueryOpts{
		Filter: backupWindowFilter(s.Window),
		Fields: []string{"clientHostname", "name", "level", "size", "saveTime", "retentionTime", "completionTime"},
	}, &resp); err != nil {
		return nil, err
	}

	// Aggregate to ONE series per key so multiple save-set instances sharing the
	// same client/saveset never produce duplicate label sets. Sizes max per
	// client+saveset; retention max per client+saveset; throughput keeps the
	// fastest instance that carried a derivable duration.
	fullMax := make(map[ckey]float64)
	incrMax := make(map[ckey]float64)
	retMax := make(map[ckey]float64)
	jobBest := make(map[ckey]jobAgg)

	for _, bk := range resp.Backups {
		size, ok := bk.Size.Bytes()
		if !ok {
			continue
		}
		k := ckey{bk.ClientHostname, bk.Name}
		if isFullLevel(bk.Level) {
			putMax(fullMax, k, size)
		} else {
			putMax(incrMax, k, size)
		}

		save, okSave := parseTime(bk.SaveTime)
		if okSave {
			if ret, okRet := parseTime(bk.RetentionTime); okRet && ret > save {
				putMax(retMax, k, ret-save)
			}
		}

		// duration is not a field; derive it from completionTime − saveTime.
		if comp, okComp := parseTime(bk.CompletionTime); okComp && okSave {
			if dur := comp - save; dur > 0 {
				bps := size / dur
				if cur, exists := jobBest[k]; !exists || bps > cur.bps {
					jobBest[k] = jobAgg{bps: bps, dur: dur}
				}
			}
		}
	}

	var b builder
	for k, size := range fullMax {
		b.gauge("nsr_backup_source_size_bytes", "FETB: largest Full backup per client/saveset.", size,
			lbl("client", k.client), lbl("saveset_name", k.saveset), lbl("level", "Full"))
	}
	for k, size := range incrMax {
		b.gauge("nsr_backup_change_size_bytes", "Largest incremental change per client/saveset.", size,
			lbl("client", k.client), lbl("saveset_name", k.saveset), lbl("level", "Incr"))
	}
	for k, secs := range retMax {
		b.gauge("nsr_backup_retention_seconds", "Retention period of a save set (seconds).", secs,
			lbl("client", k.client), lbl("saveset_name", k.saveset))
	}
	for k, j := range jobBest {
		b.gauge("nsr_job_duration_seconds", "Elapsed backup time for a save set.", j.dur,
			lbl("client", k.client), lbl("job_name", k.saveset))
		b.gauge("nsr_job_bytes_per_second", "Ingest throughput of a backup action.", j.bps,
			lbl("client", k.client), lbl("job_name", k.saveset))
	}
	return b.out, nil
}

type ckey struct{ client, saveset string }
type jobAgg struct{ bps, dur float64 }

// putMax stores v under k when it exceeds the current value (or k is absent).
func putMax[K comparable](m map[K]float64, k K, v float64) {
	if cur, ok := m[k]; !ok || v > cur {
		m[k] = v
	}
}

// fullLevels are the swagger 19.13 Backup.level enum values that represent a full
// (or synthesized-full) image rather than an incremental change.
var fullLevels = map[string]struct{}{
	"full": {}, "synthfull": {}, "incrsynthfull": {}, "consolidate": {},
}

// isFullLevel buckets a NetWorker backup level into Full vs incremental.
func isFullLevel(level string) bool {
	_, ok := fullLevels[strings.ToLower(strings.TrimSpace(level))]
	return ok
}

// backupWindowFilter builds the NetWorker q= filter bounding /backups to save sets
// within the lookback window. The swagger 19.13 q= documents an offset range form
// — saveTime:["N hours"] / ["N days"] — which the server applies relative to now,
// so no client clock is needed (ADR-0010). Isolated here so a live-validation
// correction touches exactly one function.
func backupWindowFilter(window time.Duration) string {
	hours := int(window.Hours())
	if hours < 1 {
		hours = 1
	}
	return `saveTime:["` + strconv.Itoa(hours) + ` hours"]`
}
