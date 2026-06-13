package nsr

import (
	"context"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// sessionsResponse wraps GET /sessions: {"count":N,"sessions":[...]}. Sessions are
// live, in-flight backup/recover/clone activity — a real-time signal /jobs lacks.
type sessionsResponse struct {
	Sessions []nwSession `json:"sessions"`
}

type nwSession struct {
	Type   string   `json:"type"`   // INFERRED — validate live (backup/recover/clone)
	Client string   `json:"client"` // INFERRED — validate live
	State  string   `json:"state"`  // INFERRED — validate live
	Bytes  *float64 `json:"size"`   // INFERRED — validate live (bytes moved so far)
}

// SessionsCollector maps GET /sessions to live-activity metrics.
type SessionsCollector struct{}

// Name identifies the sessions collector.
func (SessionsCollector) Name() string { return "sessions" }

// Collect fetches in-flight sessions.
func (SessionsCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp sessionsResponse
	if err := c.Get(ctx, "/sessions", nsrclient.QueryOpts{
		Fields: []string{"type", "client", "state", "size"},
	}, &resp); err != nil {
		return nil, err
	}

	var b builder
	byType := make(map[string]float64)
	for _, s := range resp.Sessions {
		b.gauge("nsr_session_active", "An active NetWorker session (always 1).", 1,
			lbl("session_type", s.Type), lbl("client", s.Client), lbl("state", s.State))
		emitGauge(&b, "nsr_session_bytes", "Bytes moved so far by an active session.", s.Bytes,
			lbl("session_type", s.Type), lbl("client", s.Client))
		byType[s.Type]++
	}
	for typ, n := range byType {
		b.gauge("nsr_sessions_total", "Count of active sessions by type.", n, lbl("session_type", typ))
	}
	return b.out, nil
}

// emitGauge appends a gauge sample only when the source value is present (absent →
// no sample, never a false 0; ADR-0008).
func emitGauge(b *builder, name, help string, v *float64, labels ...models.Label) {
	if v != nil {
		b.gauge(name, help, *v, labels...)
	}
}
