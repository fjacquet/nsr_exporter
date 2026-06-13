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

// nwDevice mirrors the swagger 19.13 Device model: media class is `mediaType` +
// `mediaFamily` (Tape/Disk/Cloud/Logical), the serial is `deviceSerialNumber`, and
// `status` is an enum (Enabled/Disabled/Service). There is no device-level capacity
// field, so this collector emits only an inventory info gauge.
type nwDevice struct {
	Name        string `json:"name"`
	MediaType   string `json:"mediaType"`
	MediaFamily string `json:"mediaFamily"`
	Status      string `json:"status"`
	Serial      string `json:"deviceSerialNumber"`
}

// DevicesCollector maps GET /devices to device inventory metrics.
type DevicesCollector struct{}

// Name identifies the devices collector.
func (DevicesCollector) Name() string { return "devices" }

// Collect fetches GET /devices and maps device inventory to samples.
func (DevicesCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp devicesResponse
	if err := c.Get(ctx, "/devices", nsrclient.QueryOpts{
		Fields: []string{"name", "mediaType", "mediaFamily", "status", "deviceSerialNumber"},
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
			lbl("media_type", d.MediaType),
			lbl("media_family", d.MediaFamily),
			lbl("status", d.Status),
			lbl("serial", d.Serial),
		)
	}
	return b.out, nil
}
