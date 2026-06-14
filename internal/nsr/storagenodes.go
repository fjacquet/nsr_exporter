package nsr

import (
	"context"
	"strconv"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// storagenodesResponse wraps GET /storagenodes: {"count":N,"storageNodes":[...]}.
// The wrapper key is camelCase `storageNodes` (swagger 19.13).
type storagenodesResponse struct {
	StorageNodes []nwStorageNode `json:"storageNodes"`
}

// nwStorageNode mirrors the swagger 19.13 StorageNode model: availability is the
// boolean `enabled` (no scalar status), and the device count is `numberOfDevices`.
type nwStorageNode struct {
	Name            string   `json:"name"`
	Enabled         bool     `json:"enabled"`
	Type            string   `json:"typeOfStorageNode"`
	Version         string   `json:"version"`
	NumberOfDevices *float64 `json:"numberOfDevices"`
}

// StorageNodesCollector maps GET /storagenodes to storage-node metrics.
type StorageNodesCollector struct{}

// Name identifies the storagenodes collector.
func (StorageNodesCollector) Name() string { return "storagenodes" }

// Collect fetches GET /storagenodes and maps storage-node inventory to samples.
func (StorageNodesCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp storagenodesResponse
	if err := c.Get(ctx, "/storagenodes", nsrclient.QueryOpts{
		Fields: []string{"name", "enabled", "typeOfStorageNode", "version", "numberOfDevices"},
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
			lbl("enabled", strconv.FormatBool(n.Enabled)),
			lbl("type", n.Type),
			lbl("version", n.Version),
		)
		// Absent device count yields no sample (ADR-0008).
		emitGauge(&b, "nsr_storagenode_device_count", "Number of devices attached to this storage node.",
			n.NumberOfDevices, lbl("node", n.Name))
	}
	return b.out, nil
}
