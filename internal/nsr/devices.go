package nsr

import (
	"context"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// devicesResponse wraps GET /devices: {"count":N,"devices":[...]}.
type devicesResponse struct {
	Devices []nwDevice `json:"devices"`
}

type nwDevice struct {
	Name     string   `json:"name"`         // INFERRED — validate live
	Type     string   `json:"type"`         // INFERRED — validate live (tape/disk/adv_file)
	Status   string   `json:"status"`       // INFERRED — validate live (enabled/disabled/offline)
	Serial   string   `json:"serialNumber"` // INFERRED — validate live
	Capacity *float64 `json:"capacity"`     // INFERRED — validate live; bytes
}

// DevicesCollector maps GET /devices to device inventory metrics.
type DevicesCollector struct{}

// Name identifies the devices collector.
func (DevicesCollector) Name() string { return "devices" }

// Collect fetches GET /devices and maps device inventory to samples.
func (DevicesCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp devicesResponse
	if err := c.Get(ctx, "/devices", nsrclient.QueryOpts{
		Fields: []string{"name", "type", "status", "serialNumber", "capacity"},
	}, &resp); err != nil {
		return nil, err
	}

	var b builder
	for _, d := range resp.Devices {
		if d.Name == "" {
			continue
		}
		// nsr_device_info is an info gauge (always 1) carrying all label-only attributes.
		b.gauge("nsr_device_info", "A NetWorker backup device (always 1).", 1,
			lbl("device_name", d.Name),
			lbl("type", d.Type),
			lbl("status", d.Status),
			lbl("serial", d.Serial),
		)
		// Absent capacity yields no sample rather than a misleading 0 (ADR-0008).
		emitGauge(&b, "nsr_device_capacity_bytes", "Device storage capacity in bytes.",
			d.Capacity, lbl("device_name", d.Name))
	}
	return b.out, nil
}
