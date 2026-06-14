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

// nwSession mirrors the swagger 19.13 Session model. Activity type is `mode`
// (Saving/Recovering/Browsing); the client field is `clientHostname`; `size` and
// `transferRate` are unit/value objects, not scalars.
type nwSession struct {
	Mode           string     `json:"mode"`
	ClientHostname string     `json:"clientHostname"`
	Size           *nwSize    `json:"size"`
	TransferRate   *nwBitRate `json:"transferRate"`
}

// SessionsCollector maps GET /sessions to live-activity metrics.
type SessionsCollector struct{}

// Name identifies the sessions collector.
func (SessionsCollector) Name() string { return "sessions" }

// Collect fetches in-flight sessions.
func (SessionsCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp sessionsResponse
	if err := c.Get(ctx, "/sessions", nsrclient.QueryOpts{
		Fields: []string{"mode", "clientHostname", "size", "transferRate"},
	}, &resp); err != nil {
		return nil, err
	}

	var b builder
	byMode := make(map[string]float64)
	for _, s := range resp.Sessions {
		b.gauge("nsr_session_active", "An active NetWorker session (always 1).", 1,
			lbl("session_type", s.Mode), lbl("client", s.ClientHostname))
		// size and transferRate are {unit,value} objects; absent/unknown → no sample.
		if v, ok := s.Size.Bytes(); ok {
			b.gauge("nsr_session_bytes", "Bytes moved so far by an active session.", v,
				lbl("session_type", s.Mode), lbl("client", s.ClientHostname))
		}
		if v, ok := s.TransferRate.BytesPerSecond(); ok {
			b.gauge("nsr_session_transfer_bytes_per_second",
				"Live transfer rate of an active session (gauge; aggregate with sum/avg, never rate()).", v,
				lbl("session_type", s.Mode), lbl("client", s.ClientHostname))
		}
		byMode[s.Mode]++
	}
	for mode, n := range byMode {
		b.gauge("nsr_active_sessions", "Count of active sessions by mode.", n, lbl("session_type", mode))
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
