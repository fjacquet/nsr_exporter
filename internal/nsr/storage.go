package nsr

import (
	"context"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// volumesResponse wraps GET /volumes: {"count":N,"volumes":[...]}.
type volumesResponse struct {
	Volumes []nwVolume `json:"volumes"`
}

type nwVolume struct {
	Name     string   `json:"name"`          // INFERRED — validate live
	Pool     string   `json:"pool"`          // INFERRED — validate live
	Type     string   `json:"mediaType"`     // INFERRED — validate live (tape/disk)
	Status   string   `json:"status"`        // INFERRED — validate live (appendable/full/recyclable)
	Capacity *float64 `json:"capacity"`      // INFERRED — validate live
	Written  *float64 `json:"written"`       // INFERRED — validate live
	Recycled *float64 `json:"recycledCount"` // INFERRED — validate live
}

// datadomainsResponse wraps GET /datadomainsystems: {"count":N,"datadomainsystems":[...]}.
type datadomainsResponse struct {
	Systems []nwDataDomain `json:"datadomainsystems"`
}

type nwDataDomain struct {
	Name              string   `json:"name"`                // INFERRED — validate live
	Model             string   `json:"model"`               // INFERRED — validate live
	OSVersion         string   `json:"osVersion"`           // INFERRED — validate live
	CapacityTotal     *float64 `json:"capacityTotal"`       // INFERRED — validate live
	CapacityUsed      *float64 `json:"capacityUsed"`        // INFERRED — validate live
	CapacityAvailable *float64 `json:"capacityAvailable"`   // INFERRED — validate live
	LogicalUsed       *float64 `json:"logicalCapacityUsed"` // INFERRED — validate live
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
		Fields: []string{"name", "pool", "mediaType", "status", "capacity", "written", "recycledCount"},
	}, &vols); err != nil {
		return nil, err
	}
	for _, v := range vols.Volumes {
		vl := []models.Label{lbl("volume_name", v.Name), lbl("pool", v.Pool), lbl("type", v.Type)}
		emitGauge(&b, "nsr_volume_capacity_bytes", "Formatted volume capacity.", v.Capacity, vl...)
		emitGauge(&b, "nsr_volume_written_bytes", "Bytes written to the volume.", v.Written, vl...)
		// Recycled count carries only volume_name (its own consistent label set).
		if v.Recycled != nil {
			b.counter("nsr_volume_recycled_total", "Times the volume has been recycled.", *v.Recycled,
				lbl("volume_name", v.Name))
		}
		// nsr_volume_status is an info gauge (always 1); status absent → no sample.
		if v.Status != "" {
			b.gauge("nsr_volume_status", "Volume status as an info gauge (always 1).", 1,
				lbl("volume_name", v.Name),
				lbl("pool", v.Pool),
				lbl("status", v.Status),
			)
		}
	}

	var dds datadomainsResponse
	if err := c.Get(ctx, "/datadomainsystems", nsrclient.QueryOpts{
		Fields: []string{"name", "model", "osVersion", "capacityTotal", "capacityUsed", "capacityAvailable", "logicalCapacityUsed"},
	}, &dds); err != nil {
		return nil, err
	}
	for _, d := range dds.Systems {
		dl := []models.Label{lbl("dd_name", d.Name), lbl("model", d.Model), lbl("os_version", d.OSVersion)}
		emitGauge(&b, "nsr_datadomain_capacity_total_bytes", "Target Data Domain total size.", d.CapacityTotal, dl...)
		emitGauge(&b, "nsr_datadomain_capacity_used_bytes", "Data Domain physical capacity used.", d.CapacityUsed, dl...)
		emitGauge(&b, "nsr_datadomain_capacity_available_bytes", "Data Domain free space.", d.CapacityAvailable, dl...)
		emitGauge(&b, "nsr_datadomain_logical_capacity_used_bytes", "Pre-deduplication size stored on target.", d.LogicalUsed, dl...)
	}
	return b.out, nil
}
