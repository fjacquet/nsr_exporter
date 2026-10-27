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

	// Concurrent sessions on one host share (client, mode); aggregate per pair so
	// each series is emitted once (duplicates fail the whole /metrics scrape).
	type key struct{ mode, client string }
	type agg struct {
		n          float64
		bytes, bps float64
		hasB, hasR bool
	}
	groups := make(map[key]*agg)
	var order []key
	byMode := make(map[string]float64)
	for _, s := range resp.Sessions {
		k := key{s.Mode, s.ClientHostname}
		g := groups[k]
		if g == nil {
			g = &agg{}
			groups[k] = g
			order = append(order, k)
		}
		g.n++
		// size and transferRate are {unit,value} objects; unparseable → not summed.
		if v, ok := s.Size.Bytes(); ok {
			g.bytes += v
			g.hasB = true
		}
		if v, ok := s.TransferRate.BytesPerSecond(); ok {
			g.bps += v
			g.hasR = true
		}
		byMode[s.Mode]++
	}

	var b builder
	for _, k := range order {
		g := groups[k]
		l := []models.Label{lbl("session_type", k.mode), lbl("client", k.client)}
		b.gauge("nsr_session_active", "Count of active NetWorker sessions for a client and mode.", g.n, l...)
		// no parseable value in the group → no sample, never a false 0 (ADR-0008).
		if g.hasB {
			b.gauge("nsr_session_bytes", "Bytes moved so far, summed over a client's active sessions.", g.bytes, l...)
		}
		if g.hasR {
			b.gauge("nsr_session_transfer_bytes_per_second",
				"Live transfer rate summed over a client's active sessions (gauge; aggregate with sum/avg, never rate()).", g.bps, l...)
		}
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
