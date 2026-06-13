package nsr

import (
	"context"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// poolsResponse wraps GET /pools: {"count":N,"pools":[...]}.
type poolsResponse struct {
	Pools []nwPool `json:"pools"`
}

type nwPool struct {
	Name          string   `json:"name"`          // INFERRED — validate live
	Type          string   `json:"type"`          // INFERRED — validate live (Backup/Clone/Archive)
	CapacityTotal *float64 `json:"capacityTotal"` // INFERRED — validate live; bytes
	CapacityUsed  *float64 `json:"capacityUsed"`  // INFERRED — validate live; bytes
	VolumeCount   *float64 `json:"volumeCount"`   // INFERRED — validate live
}

// PoolsCollector maps GET /pools to pool-capacity metrics.
type PoolsCollector struct{}

// Name identifies the pools collector.
func (PoolsCollector) Name() string { return "pools" }

// Collect fetches GET /pools and maps pool capacity to samples.
func (PoolsCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp poolsResponse
	if err := c.Get(ctx, "/pools", nsrclient.QueryOpts{
		Fields: []string{"name", "type", "capacityTotal", "capacityUsed", "volumeCount"},
	}, &resp); err != nil {
		return nil, err
	}

	var b builder
	for _, p := range resp.Pools {
		if p.Name == "" {
			continue
		}
		pl := []models.Label{lbl("pool", p.Name), lbl("type", p.Type)}
		// Absent numeric fields yield no sample (ADR-0008).
		emitGauge(&b, "nsr_pool_capacity_bytes", "Total pool capacity in bytes.", p.CapacityTotal, pl...)
		emitGauge(&b, "nsr_pool_used_bytes", "Used pool capacity in bytes.", p.CapacityUsed,
			lbl("pool", p.Name))
		emitGauge(&b, "nsr_pool_volume_count", "Number of volumes in the pool.", p.VolumeCount,
			lbl("pool", p.Name))
	}
	return b.out, nil
}
