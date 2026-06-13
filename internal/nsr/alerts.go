package nsr

import (
	"context"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// alertsResponse wraps GET /alerts: {"count":N,"alerts":[...]}.
type alertsResponse struct {
	Alerts []nwAlert `json:"alerts"`
}

// nwAlert mirrors the swagger 19.13 Alert model: category, message, priority,
// timestamp. There is no severity or acknowledged field.
type nwAlert struct {
	Priority  string `json:"priority"`
	Category  string `json:"category"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// AlertsCollector maps GET /alerts to active-alert metrics.
type AlertsCollector struct{}

// Name identifies the alerts collector.
func (AlertsCollector) Name() string { return "alerts" }

// Collect fetches GET /alerts and maps active alerts to samples.
func (AlertsCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp alertsResponse
	err := c.Get(ctx, "/alerts", nsrclient.QueryOpts{
		Fields: []string{"priority", "category", "message", "timestamp"},
	}, &resp)
	if err != nil {
		return nil, err
	}

	var b builder
	byPriority := make(map[string]float64)
	for _, a := range resp.Alerts {
		b.gauge("nsr_alert_info", "An active NetWorker alert (always 1).", 1,
			lbl("priority", a.Priority),
			lbl("category", a.Category),
			lbl("message", a.Message),
			lbl("timestamp", a.Timestamp),
		)
		byPriority[a.Priority]++
	}
	for prio, n := range byPriority {
		b.gauge("nsr_alerts_active", "Count of active alerts by priority.", n, lbl("priority", prio))
	}
	return b.out, nil
}
