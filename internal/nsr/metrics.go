package nsr

import (
	"context"
	"time"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// systemLabel is the identity label carried by every metric so one process can
// serve many NetWorker systems (architecture.md: one process, many targets).
const systemLabel = "system"

// ResourceCollector maps one NetWorker domain (clients, alerts, jobs, …) to
// Samples. Implementations live in their own file (clients.go, alerts.go, …),
// fetch via the client, unwrap the named collection field, project with fl=, and
// tolerantly parse — an unparseable value yields an absent sample, never a zero
// (ADR-0008).
type ResourceCollector interface {
	// Name identifies the collector in logs and the _up metric.
	Name() string
	// Collect fetches and maps this domain for a single system. The returned
	// Samples must NOT carry the system label; the loop appends it uniformly.
	Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error)
}

// builder accumulates Samples for one collector, keeping construction terse.
type builder struct {
	out []models.Sample
}

func (b *builder) gauge(name, help string, value float64, labels ...models.Label) {
	b.out = append(b.out, models.Sample{Name: name, Help: help, Type: models.Gauge, Value: value, Labels: labels})
}

func (b *builder) counter(name, help string, value float64, labels ...models.Label) {
	b.out = append(b.out, models.Sample{Name: name, Help: help, Type: models.Counter, Value: value, Labels: labels})
}

// lbl is a terse label constructor.
func lbl(key, value string) models.Label { return models.Label{Key: key, Value: value} }

// parseTime parses a NetWorker timestamp into Unix seconds. NetWorker emits
// RFC3339 strings (e.g. "2026-06-13T08:00:00Z"); some fields may already be epoch
// numbers handled at the field level. An unparseable value returns ok=false so the
// caller emits no sample rather than a misleading zero (ADR-0008).
func parseTime(s string) (seconds float64, ok bool) {
	if s == "" {
		return 0, false
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return float64(t.Unix()), true
		}
	}
	return 0, false
}

// withSystem returns a copy of s with the system identity label appended. Append
// (not prepend) keeps a stable canonical order; the system key is uniform across
// every series so the label-key invariant holds (ADR-0006).
func withSystem(s models.Sample, system string) models.Sample {
	labels := make([]models.Label, 0, len(s.Labels)+1)
	labels = append(labels, s.Labels...)
	labels = append(labels, lbl(systemLabel, system))
	s.Labels = labels
	return s
}
