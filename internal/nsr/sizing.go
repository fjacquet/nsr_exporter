package nsr

import (
	"context"
	"strings"
	"time"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// backupsResponse wraps GET /backups: {"count":N,"backups":[...]}.
type backupsResponse struct {
	Backups []nwBackup `json:"backups"`
}

type nwBackup struct {
	Client        string   `json:"client"`        // INFERRED — validate live
	Name          string   `json:"name"`          // INFERRED — validate live (saveset)
	Level         string   `json:"level"`         // INFERRED — validate live (full/incr/1..9)
	Size          *float64 `json:"size"`          // INFERRED — validate live (bytes)
	SaveTime      string   `json:"saveTime"`      // confirmed in design spec §5.6
	RetentionTime string   `json:"retentionTime"` // confirmed in design spec §5.6
	Pool          string   `json:"pool"`          // INFERRED — validate live
	Duration      *float64 `json:"duration"`      // INFERRED — validate live (seconds, optional)
}

// SizingCollector maps the bounded GET /backups to capacity-forecasting metrics.
// It is constructed with a lookback window and clock so the query is always bounded
// (ADR-0010) — never fetch the full catalog.
type SizingCollector struct {
	Window time.Duration
	Now    func() time.Time
}

// Name identifies the sizing collector.
func (SizingCollector) Name() string { return "sizing" }

// Collect fetches a bounded slice of the backup catalog and aggregates FETB,
// change size, retention, and throughput per client/saveset.
func (s SizingCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp backupsResponse
	if err := c.Get(ctx, "/backups", nsrclient.QueryOpts{
		Filter: backupWindowFilter(s.Now(), s.Window),
		Fields: []string{"client", "name", "level", "size", "saveTime", "retentionTime", "pool", "duration"},
	}, &resp); err != nil {
		return nil, err
	}

	// Aggregate to ONE series per key so multiple save-set instances sharing the
	// same client/saveset never produce duplicate label sets. Sizes max per
	// client+saveset; retention max per client+saveset+pool; throughput keeps the
	// fastest instance that carried a duration.
	fullMax := make(map[ckey]float64)
	incrMax := make(map[ckey]float64)
	retMax := make(map[retKey]float64)
	jobBest := make(map[ckey]jobAgg)

	for _, bk := range resp.Backups {
		if bk.Size == nil {
			continue
		}
		k := ckey{bk.Client, bk.Name}
		if isFullLevel(bk.Level) {
			putMax(fullMax, k, *bk.Size)
		} else {
			putMax(incrMax, k, *bk.Size)
		}

		if save, ok1 := parseTime(bk.SaveTime); ok1 {
			if ret, ok2 := parseTime(bk.RetentionTime); ok2 && ret > save {
				putMax(retMax, retKey{bk.Client, bk.Name, bk.Pool}, ret-save)
			}
		}

		if bk.Duration != nil && *bk.Duration > 0 {
			bps := *bk.Size / *bk.Duration
			if cur, ok := jobBest[k]; !ok || bps > cur.bps {
				jobBest[k] = jobAgg{bps: bps, dur: *bk.Duration}
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
			lbl("client", k.client), lbl("saveset_name", k.saveset), lbl("pool", k.pool))
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
type retKey struct{ client, saveset, pool string }
type jobAgg struct{ bps, dur float64 }

// putMax stores v under k when it exceeds the current value (or k is absent).
func putMax[K comparable](m map[K]float64, k K, v float64) {
	if cur, ok := m[k]; !ok || v > cur {
		m[k] = v
	}
}

// isFullLevel buckets a NetWorker backup level into Full vs incremental. Level
// vocabulary is INFERRED (full / incr / 1..9 / manual) — validate live.
func isFullLevel(level string) bool {
	return strings.EqualFold(strings.TrimSpace(level), "full")
}

// backupWindowFilter builds the NetWorker q= filter bounding /backups to save sets
// newer than now-window. The savetime filter SYNTAX is INFERRED — validate live;
// it is isolated here so a correction touches exactly one function (ADR-0010).
func backupWindowFilter(now time.Time, window time.Duration) string {
	since := now.Add(-window).UTC().Format("01/02/2006 15:04:05")
	return "savetime>'" + since + "'"
}
