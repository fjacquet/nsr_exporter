package nsr

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// serverStatistics decodes GET /serverstatistics — a single object (no count
// envelope). Counts are int64-valued; saveSize/recoverSize are Size objects
// ({"unit","value"}), not scalars. Pointers/objects absent → no sample (ADR-0008).
type serverStatistics struct {
	UpSince         string   `json:"upSince"`
	Saves           *float64 `json:"saves"`
	SaveSize        *nwSize  `json:"saveSize"`
	Recovers        *float64 `json:"recovers"`
	RecoverSize     *nwSize  `json:"recoverSize"`
	BadSaves        *float64 `json:"badSaves"`
	BadRecovers     *float64 `json:"badRecovers"`
	CurrentSaves    *float64 `json:"currentSaves"`
	CurrentRecovers *float64 `json:"currentRecovers"`
	MaxSaves        *float64 `json:"maxSaves"`
	MaxRecovers     *float64 `json:"maxRecovers"`
}

// jobsResponse wraps GET /jobs: {"count":N,"jobs":[...]}.
type jobsResponse struct {
	Jobs []nwJob `json:"jobs"`
}

// nwJob mirrors the swagger 19.13 Job model. The client field is `clientHostname`
// (not `client`); `level` exists in 19.13 (absent in 19.2 → parsed tolerantly).
// There is no `group` field on Job.
type nwJob struct {
	ID               json.Number `json:"id"` // may be numeric; rendered as string label
	Name             string      `json:"name"`
	Type             string      `json:"type"`
	State            string      `json:"state"`
	CompletionStatus string      `json:"completionStatus"`
	ClientHostname   string      `json:"clientHostname"`
	StartTime        string      `json:"startTime"` // RFC3339
	EndTime          string      `json:"endTime"`   // RFC3339
	Level            string      `json:"level"`     // 19.13+ (absent on older servers)
}

// JobsCollector maps GET /serverstatistics and GET /jobs. Window bounds /jobs to
// jobs started within the lookback; Now is the clock for the client-side cut
// (nil means time.Now).
type JobsCollector struct {
	Window time.Duration
	Now    func() time.Time
}

// Name identifies the jobs collector.
func (JobsCollector) Name() string { return "jobs" }

// jobWindowFilter builds the NetWorker q= filter bounding /jobs to jobs started
// within the lookback window. The swagger 19.13 time-range form
// AttributeName:["N hours"] is the documented syntax; live-server validation of
// startTime on /jobs is pending, so a correction touches only this function.
func jobWindowFilter(window time.Duration) string {
	hours := int(window.Hours())
	if hours < 1 {
		hours = 1
	}
	return `startTime:["` + strconv.Itoa(hours) + ` hours"]`
}

// Collect fetches server-wide statistics and individual job records.
func (jc JobsCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var b builder

	var stats serverStatistics
	if err := c.Get(ctx, "/serverstatistics", nsrclient.QueryOpts{}, &stats); err != nil {
		return nil, err
	}
	if ts, ok := parseTime(stats.UpSince); ok {
		b.gauge("nsr_server_up_since_timestamp_seconds", "NetWorker server start time (Unix seconds).", ts)
	}
	emitCounter(&b, "nsr_server_saves_total", "Cumulative backup attempts.", stats.Saves)
	emitSizeCounter(&b, "nsr_server_save_size_bytes", "Cumulative bytes written by backups.", stats.SaveSize)
	emitCounter(&b, "nsr_server_recovers_total", "Cumulative recovery attempts.", stats.Recovers)
	emitSizeCounter(&b, "nsr_server_recover_size_bytes", "Cumulative bytes restored by recoveries.", stats.RecoverSize)
	emitCounter(&b, "nsr_server_bad_saves_total", "Cumulative failed backup attempts.", stats.BadSaves)
	emitCounter(&b, "nsr_server_bad_recovers_total", "Cumulative failed recovery attempts.", stats.BadRecovers)
	emitGauge(&b, "nsr_server_current_saves", "Saves currently running on the server.", stats.CurrentSaves)
	emitGauge(&b, "nsr_server_current_recovers", "Recoveries currently running on the server.", stats.CurrentRecovers)
	emitGauge(&b, "nsr_server_max_saves", "Maximum concurrent saves the server allows.", stats.MaxSaves)
	emitGauge(&b, "nsr_server_max_recovers", "Maximum concurrent recoveries the server allows.", stats.MaxRecovers)

	var jobs jobsResponse
	if err := c.Get(ctx, "/jobs", nsrclient.QueryOpts{
		Filter: jobWindowFilter(jc.Window),
		Fields: []string{"id", "name", "type", "state", "completionStatus", "clientHostname", "startTime", "endTime", "level"},
	}, &jobs); err != nil {
		return nil, err
	}
	now := time.Now
	if jc.Now != nil {
		now = jc.Now
	}
	cutoff := float64(now().Add(-jc.Window).Unix())
	for _, j := range jobs.Jobs {
		// Defense in depth if the server ignores q=. A job with an absent or
		// unparseable startTime is kept (ADR-0008). A still-running job that began
		// before the window is dropped too (accepted).
		if ts, ok := parseTime(j.StartTime); ok && ts < cutoff {
			continue
		}
		b.gauge("nsr_job_status", "An individual NetWorker job (always 1).", 1,
			lbl("job_id", j.ID.String()),
			lbl("job_name", j.Name),
			lbl("job_type", j.Type),
			lbl("state", j.State),
			lbl("completion_status", j.CompletionStatus),
			lbl("client", j.ClientHostname),
			lbl("level", j.Level),
		)
		// Absent or unparseable timestamps yield no sample (ADR-0008).
		if ts, ok := parseTime(j.StartTime); ok {
			b.gauge("nsr_job_start_timestamp_seconds",
				"Unix timestamp when the job started.", ts,
				lbl("job_id", j.ID.String()),
				lbl("job_name", j.Name),
			)
		}
		if ts, ok := parseTime(j.EndTime); ok {
			b.gauge("nsr_job_end_timestamp_seconds",
				"Unix timestamp when the job ended.", ts,
				lbl("job_id", j.ID.String()),
				lbl("job_name", j.Name),
			)
		}
	}
	return b.out, nil
}

// emitCounter appends a counter sample only when the source value is present.
func emitCounter(b *builder, name, help string, v *float64, labels ...models.Label) {
	if v != nil {
		b.counter(name, help, *v, labels...)
	}
}

// emitSizeCounter appends a counter from a Size object, converting to bytes; an
// absent object or unknown unit yields no sample (ADR-0008).
func emitSizeCounter(b *builder, name, help string, s *nwSize, labels ...models.Label) {
	if v, ok := s.Bytes(); ok {
		b.counter(name, help, v, labels...)
	}
}
