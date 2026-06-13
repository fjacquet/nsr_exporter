package nsr

import (
	"context"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// queuesResponse wraps GET /queues: {"count":N,"queues":[...]}.
type queuesResponse struct {
	Queues []nwQueue `json:"queues"`
}

type nwQueue struct {
	Name     string   `json:"name"`     // INFERRED — validate live
	Depth    *float64 `json:"depth"`    // INFERRED — validate live; number of pending items
	WaitTime *float64 `json:"waitTime"` // INFERRED — validate live; seconds
}

// QueuesCollector maps GET /queues to queue depth and wait metrics.
type QueuesCollector struct{}

// Name identifies the queues collector.
func (QueuesCollector) Name() string { return "queues" }

// Collect fetches GET /queues and maps queue depth/wait to samples.
func (QueuesCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp queuesResponse
	if err := c.Get(ctx, "/queues", nsrclient.QueryOpts{
		Fields: []string{"name", "depth", "waitTime"},
	}, &resp); err != nil {
		return nil, err
	}

	var b builder
	for _, q := range resp.Queues {
		if q.Name == "" {
			continue
		}
		// Absent depth or waitTime yields no sample (ADR-0008).
		emitGauge(&b, "nsr_queue_depth", "Number of pending items in the queue.",
			q.Depth, lbl("queue", q.Name))
		emitGauge(&b, "nsr_queue_wait_seconds", "Current wait time in the queue (seconds).",
			q.WaitTime, lbl("queue", q.Name))
	}
	return b.out, nil
}
