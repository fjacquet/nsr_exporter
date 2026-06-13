package nsr

import (
	"context"
	"encoding/json"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// serverStatistics decodes GET /serverstatistics. Unlike the list endpoints this is
// a single object (no count envelope). All numeric fields are pointers so an
// absent/misnamed field yields no sample rather than a false 0 (ADR-0008).
//
// Field names are INFERRED from the metric spec and NetWorker camelCase convention
// — validate live with --once --debug --trace.
type serverStatistics struct {
	UpSince     string   `json:"upSince"`     // INFERRED — validate live
	Saves       *float64 `json:"saves"`       // INFERRED — validate live
	SaveSize    *float64 `json:"saveSize"`    // INFERRED — validate live
	Recovers    *float64 `json:"recovers"`    // INFERRED — validate live
	RecoverSize *float64 `json:"recoverSize"` // INFERRED — validate live
	BadSaves    *float64 `json:"badSaves"`    // INFERRED — validate live
	BadRecovers *float64 `json:"badRecovers"` // INFERRED — validate live
}

// jobsResponse wraps GET /jobs: {"count":N,"jobs":[...]}.
type jobsResponse struct {
	Jobs []nwJob `json:"jobs"`
}

type nwJob struct {
	ID               json.Number `json:"id"`               // may be numeric; rendered as string label
	Name             string      `json:"name"`             // INFERRED — validate live
	Type             string      `json:"type"`             // INFERRED — validate live
	State            string      `json:"state"`            // INFERRED — validate live
	CompletionStatus string      `json:"completionStatus"` // INFERRED — validate live
	Client           string      `json:"client"`           // INFERRED — validate live
	StartTime        string      `json:"startTime"`        // INFERRED — validate live; RFC3339
	EndTime          string      `json:"endTime"`          // INFERRED — validate live; RFC3339
	Group            string      `json:"group"`            // INFERRED — validate live
	Level            string      `json:"level"`            // INFERRED — validate live
}

// JobsCollector maps GET /serverstatistics and GET /jobs.
type JobsCollector struct{}

// Name identifies the jobs collector.
func (JobsCollector) Name() string { return "jobs" }

// Collect fetches server-wide statistics and individual job records.
func (JobsCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var b builder

	var stats serverStatistics
	if err := c.Get(ctx, "/serverstatistics", nsrclient.QueryOpts{}, &stats); err != nil {
		return nil, err
	}
	if ts, ok := parseTime(stats.UpSince); ok {
		b.gauge("nsr_server_up_since_timestamp_seconds", "NetWorker server start time (Unix seconds).", ts)
	}
	emitCounter(&b, "nsr_server_saves_total", "Cumulative backup attempts.", stats.Saves)
	emitCounter(&b, "nsr_server_save_size_bytes", "Cumulative bytes written by backups.", stats.SaveSize)
	emitCounter(&b, "nsr_server_recovers_total", "Cumulative recovery attempts.", stats.Recovers)
	emitCounter(&b, "nsr_server_recover_size_bytes", "Cumulative bytes restored by recoveries.", stats.RecoverSize)
	emitCounter(&b, "nsr_server_bad_saves_total", "Cumulative failed backup attempts.", stats.BadSaves)
	emitCounter(&b, "nsr_server_bad_recovers_total", "Cumulative failed recovery attempts.", stats.BadRecovers)

	var jobs jobsResponse
	if err := c.Get(ctx, "/jobs", nsrclient.QueryOpts{
		Fields: []string{"id", "name", "type", "state", "completionStatus", "client", "startTime", "endTime", "group", "level"},
	}, &jobs); err != nil {
		return nil, err
	}
	for _, j := range jobs.Jobs {
		b.gauge("nsr_job_status", "An individual NetWorker job (always 1).", 1,
			lbl("job_id", j.ID.String()),
			lbl("job_name", j.Name),
			lbl("job_type", j.Type),
			lbl("state", j.State),
			lbl("completion_status", j.CompletionStatus),
			lbl("client", j.Client),
			lbl("group", j.Group),
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
