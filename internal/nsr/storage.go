package nsr

import (
	"context"
	"strings"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// volumesResponse wraps GET /volumes: {"count":N,"volumes":[...]}.
type volumesResponse struct {
	Volumes []nwVolume `json:"volumes"`
}

// nwVolume mirrors the swagger 19.13 Volume model: media class is `type`, capacity
// and written are Size objects, the recycle count is `recycled`, and there is no
// scalar status — only a `states` array.
type nwVolume struct {
	Name     string   `json:"name"`
	Pool     string   `json:"pool"`
	Type     string   `json:"type"`
	States   []string `json:"states"`
	Capacity *nwSize  `json:"capacity"`
	Written  *nwSize  `json:"written"`
	Recycled *float64 `json:"recycled"`
}

// datadomainsResponse wraps GET /datadomainsystems: {"count":N,"dataDomainSystems":[...]}.
type datadomainsResponse struct {
	Systems []nwDataDomain `json:"dataDomainSystems"`
}

// nwDataDomain mirrors the swagger 19.13 DataDomainSystem model. Capacities are
// human-readable strings (e.g. "112 GB"), not numbers — parsed via parseHumanSize.
type nwDataDomain struct {
	Name              string `json:"name"`
	Model             string `json:"model"`
	OSVersion         string `json:"osVersion"`
	TotalCapacity     string `json:"totalCapacity"`
	UsedCapacity      string `json:"usedCapacity"`
	AvailableCapacity string `json:"availableCapacity"`
	UsedLogical       string `json:"usedLogicalCapacity"`
}

// StorageCollector maps GET /volumes and GET /datadomainsystems.
type StorageCollector struct{}

// Name identifies the storage collector.
func (StorageCollector) Name() string { return "storage" }

// Collect fetches media volumes and Data Domain capacity.
func (StorageCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var b builder

	var vols volumesResponse
	if err := c.Get(ctx, "/volumes", nsrclient.QueryOpts{
		Fields: []string{"name", "pool", "type", "states", "capacity", "written", "recycled"},
	}, &vols); err != nil {
		return nil, err
	}
	for _, v := range vols.Volumes {
		vl := []models.Label{lbl("volume_name", v.Name), lbl("pool", v.Pool), lbl("type", v.Type)}
		if bytes, ok := v.Capacity.Bytes(); ok {
			b.gauge("nsr_volume_capacity_bytes", "Volume capacity in bytes.", bytes, vl...)
		}
		if bytes, ok := v.Written.Bytes(); ok {
			b.gauge("nsr_volume_written_bytes", "Bytes written to the volume.", bytes, vl...)
		}
		// Recycled count carries only volume_name (its own consistent label set).
		if v.Recycled != nil {
			b.counter("nsr_volume_recycled_total", "Times the volume has been recycled.", *v.Recycled,
				lbl("volume_name", v.Name))
		}
		// nsr_volume_status is an info gauge (always 1); the states array (e.g.
		// Recyclable/WORM/Archive) is joined into one status label. Empty → no sample.
		if len(v.States) > 0 {
			b.gauge("nsr_volume_status", "Volume state(s) as an info gauge (always 1).", 1,
				lbl("volume_name", v.Name),
				lbl("pool", v.Pool),
				lbl("status", strings.Join(v.States, ",")),
			)
		}
	}

	var dds datadomainsResponse
	if err := c.Get(ctx, "/datadomainsystems", nsrclient.QueryOpts{
		Fields: []string{"name", "model", "osVersion", "totalCapacity", "usedCapacity", "availableCapacity", "usedLogicalCapacity"},
	}, &dds); err != nil {
		return nil, err
	}
	for _, d := range dds.Systems {
		dl := []models.Label{lbl("dd_name", d.Name), lbl("model", d.Model), lbl("os_version", d.OSVersion)}
		emitHumanSize(&b, "nsr_datadomain_capacity_total_bytes", "Target Data Domain total size.", d.TotalCapacity, dl...)
		emitHumanSize(&b, "nsr_datadomain_capacity_used_bytes", "Data Domain physical capacity used.", d.UsedCapacity, dl...)
		emitHumanSize(&b, "nsr_datadomain_capacity_available_bytes", "Data Domain free space.", d.AvailableCapacity, dl...)
		emitHumanSize(&b, "nsr_datadomain_logical_capacity_used_bytes", "Pre-deduplication size stored on target.", d.UsedLogical, dl...)
	}
	return b.out, nil
}

// emitHumanSize parses a NetWorker human-readable capacity string ("112 GB") into
// bytes and emits a gauge; empty/malformed input yields no sample (ADR-0008).
func emitHumanSize(b *builder, name, help, s string, labels ...models.Label) {
	if v, ok := parseHumanSize(s); ok {
		b.gauge(name, help, v, labels...)
	}
}
