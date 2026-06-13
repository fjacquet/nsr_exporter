package nsr

import (
	"context"
	"strconv"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// alertsResponse wraps GET /alerts: {"count":N,"alerts":[...]}.
type alertsResponse struct {
	Alerts []nwAlert `json:"alerts"`
}

type nwAlert struct {
	Severity     string `json:"severity"`
	Category     string `json:"category"`
	Message      string `json:"message"`
	Time         string `json:"time"`
	Acknowledged bool   `json:"acknowledged"` // INFERRED — validate live
}

// AlertsCollector maps GET /alerts to active-alert metrics.
type AlertsCollector struct{}

// Name identifies the alerts collector.
func (AlertsCollector) Name() string { return "alerts" }

// Collect fetches GET /alerts and maps active alerts to samples.
func (AlertsCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp alertsResponse
	err := c.Get(ctx, "/alerts", nsrclient.QueryOpts{
		Fields: []string{"severity", "category", "message", "time", "acknowledged"},
	}, &resp)
	if err != nil {
		return nil, err
	}

	var b builder
	bySeverity := make(map[string]float64)
	for _, a := range resp.Alerts {
		b.gauge("nsr_alert_info", "An active NetWorker alert (always 1).", 1,
			lbl("severity", a.Severity),
			lbl("category", a.Category),
			lbl("message", a.Message),
			lbl("timestamp", a.Time),
			lbl("acknowledged", strconv.FormatBool(a.Acknowledged)),
		)
		bySeverity[a.Severity]++
	}
	for sev, n := range bySeverity {
		b.gauge("nsr_alerts_total", "Count of active alerts by severity.", n, lbl("severity", sev))
	}
	return b.out, nil
}
