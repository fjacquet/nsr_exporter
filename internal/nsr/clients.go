package nsr

import (
	"context"
	"strconv"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// clientsResponse is the wrapped envelope returned by GET /clients:
// {"count":N,"clients":[...]}. NetWorker never returns a bare array.
type clientsResponse struct {
	Clients []nwClient `json:"clients"`
}

type nwClient struct {
	Hostname        string `json:"hostname"`
	NDMP            bool   `json:"ndmp"`
	ScheduledBackup bool   `json:"scheduledBackup"`
	BackupCommand   string `json:"backupCommand"`
	Parallelism     *int   `json:"parallelism"` // pointer: absent → no sample, never 0
}

// ClientsCollector maps GET /clients to client inventory metrics.
type ClientsCollector struct{}

// Name identifies the clients collector.
func (ClientsCollector) Name() string { return "clients" }

// Collect fetches GET /clients and maps client inventory to samples.
func (ClientsCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp clientsResponse
	err := c.Get(ctx, "/clients", nsrclient.QueryOpts{
		Fields: []string{"hostname", "ndmp", "scheduledBackup", "backupCommand", "parallelism"},
	}, &resp)
	if err != nil {
		return nil, err
	}

	var b builder
	for _, cl := range resp.Clients {
		if cl.Hostname == "" {
			continue
		}
		b.gauge("nsr_client_info", "Metadata about a configured backup client (always 1).", 1,
			lbl("client_name", cl.Hostname),
			lbl("ndmp", strconv.FormatBool(cl.NDMP)),
			lbl("scheduled_backup", strconv.FormatBool(cl.ScheduledBackup)),
			lbl("backup_command", cl.BackupCommand),
		)
		// Absent parallelism yields no sample rather than a misleading 0 (ADR-0008).
		if cl.Parallelism != nil {
			b.gauge("nsr_client_parallelism", "Configured backup stream limit per client.",
				float64(*cl.Parallelism), lbl("client_name", cl.Hostname))
		}
	}
	return b.out, nil
}
