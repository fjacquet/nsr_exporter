package nsr

import (
	"context"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// storagenodesResponse wraps GET /storagenodes: {"count":N,"storagenodes":[...]}.
type storagenodesResponse struct {
	StorageNodes []nwStorageNode `json:"storagenodes"`
}

type nwStorageNode struct {
	Name        string   `json:"name"`        // INFERRED — validate live
	Status      string   `json:"status"`      // INFERRED — validate live (enabled/disabled)
	DeviceCount *float64 `json:"deviceCount"` // INFERRED — validate live
}

// StorageNodesCollector maps GET /storagenodes to storage-node metrics.
type StorageNodesCollector struct{}

// Name identifies the storagenodes collector.
func (StorageNodesCollector) Name() string { return "storagenodes" }

// Collect fetches GET /storagenodes and maps storage-node inventory to samples.
func (StorageNodesCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp storagenodesResponse
	if err := c.Get(ctx, "/storagenodes", nsrclient.QueryOpts{
		Fields: []string{"name", "status", "deviceCount"},
	}, &resp); err != nil {
		return nil, err
	}

	var b builder
	for _, n := range resp.StorageNodes {
		if n.Name == "" {
			continue
		}
		// nsr_storagenode_info is an info gauge (always 1).
		b.gauge("nsr_storagenode_info", "A NetWorker storage node (always 1).", 1,
			lbl("node", n.Name),
			lbl("status", n.Status),
		)
		// Absent device count yields no sample (ADR-0008).
		emitGauge(&b, "nsr_storagenode_device_count", "Number of devices attached to this storage node.",
			n.DeviceCount, lbl("node", n.Name))
	}
	return b.out, nil
}
