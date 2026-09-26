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
	Hostname        string         `json:"hostname"`
	NDMP            bool           `json:"ndmp"`
	ScheduledBackup bool           `json:"scheduledBackup"`
	BackupCommand   string         `json:"backupCommand"`
	Parallelism     *int           `json:"parallelism"` // pointer: absent → no sample, never 0
	OS              string         `json:"os"`          // swagger 19.13 Client.os (the real OS field)
	ResourceID      *nwClientResID `json:"resourceId"`  // uniquely identifies the client RESOURCE (see resolveResourceID)
}

// nwClientResID mirrors swagger's ResourceId: {id, sequence}. The id attribute
// "uniquely identifies a client resource instance" (swagger-v3.yaml) — distinct
// from hostname, which NetWorker allows multiple client resources to share (e.g.
// several parallel backup configs for one host). Issue #36.
type nwClientResID struct {
	ID string `json:"id"`
}

// resolveResourceID returns the client resource's unique identifier, or "" if the
// backend omitted it (older NetWorker versions). An empty value is a label, not a
// numeric sample, so ADR-0008 (absent, never zero) does not apply here.
func resolveResourceID(cl nwClient) string {
	if cl.ResourceID == nil {
		return ""
	}
	return cl.ResourceID.ID
}

// ClientsCollector maps GET /clients to client inventory metrics.
type ClientsCollector struct{}

// Name identifies the clients collector.
func (ClientsCollector) Name() string { return "clients" }

// Collect fetches GET /clients and maps client inventory to samples.
func (ClientsCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp clientsResponse
	err := c.Get(ctx, "/clients", nsrclient.QueryOpts{
		Fields: []string{"hostname", "ndmp", "scheduledBackup", "backupCommand", "parallelism", "os", "resourceId"},
	}, &resp)
	if err != nil {
		return nil, err
	}

	var b builder
	for _, cl := range resp.Clients {
		if cl.Hostname == "" {
			continue
		}
		resourceID := resolveResourceID(cl)
		// resource_id disambiguates client resources that share a hostname (NetWorker
		// allows several client resources per host, e.g. parallel backup configs with
		// different save sets/tags — issue #36). Without it, two such resources yield
		// identical nsr_client_info/nsr_client_parallelism series and the Prometheus
		// registry rejects the scrape as a duplicate metric.
		b.gauge("nsr_client_info", "Metadata about a configured backup client (always 1).", 1,
			lbl("client_name", cl.Hostname),
			lbl("resource_id", resourceID),
			lbl("ndmp", strconv.FormatBool(cl.NDMP)),
			lbl("scheduled_backup", strconv.FormatBool(cl.ScheduledBackup)),
			lbl("backup_command", cl.BackupCommand),
			lbl("operating_system", cl.OS),
		)
		// Absent parallelism yields no sample rather than a misleading 0 (ADR-0008).
		if cl.Parallelism != nil {
			b.gauge("nsr_client_parallelism", "Configured backup stream limit per client.",
				float64(*cl.Parallelism), lbl("client_name", cl.Hostname), lbl("resource_id", resourceID))
		}
	}
	return b.out, nil
}
