package nsr

import (
	"context"
	"strconv"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// poolsResponse wraps GET /pools: {"count":N,"pools":[...]}.
type poolsResponse struct {
	Pools []nwPool `json:"pools"`
}

// nwPool mirrors the swagger 19.13 Pool model. The type field is `poolType`; the
// Pool resource exposes NO capacity or volume-count fields, so this collector emits
// only an inventory info gauge.
type nwPool struct {
	Name     string `json:"name"`
	PoolType string `json:"poolType"`
	Enabled  bool   `json:"enabled"`
}

// PoolsCollector maps GET /pools to a pool inventory metric.
type PoolsCollector struct{}

// Name identifies the pools collector.
func (PoolsCollector) Name() string { return "pools" }

// Collect fetches GET /pools and maps pool inventory to samples.
func (PoolsCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp poolsResponse
	if err := c.Get(ctx, "/pools", nsrclient.QueryOpts{
		Fields: []string{"name", "poolType", "enabled"},
	}, &resp); err != nil {
		return nil, err
	}

	var b builder
	for _, p := range resp.Pools {
		if p.Name == "" {
			continue
		}
		b.gauge("nsr_pool_info", "A configured media pool (always 1).", 1,
			lbl("pool", p.Name),
			lbl("pool_type", p.PoolType),
			lbl("enabled", strconv.FormatBool(p.Enabled)),
		)
	}
	return b.out, nil
}
