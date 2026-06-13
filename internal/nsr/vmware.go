package nsr

import (
	"context"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// vmwaresResponse wraps GET /vmwares: {"count":N,"vmwares":[...]}.
type vmwaresResponse struct {
	VMwares []nwVMware `json:"vmwares"`
}

type nwVMware struct {
	Name             string `json:"name"`             // INFERRED — validate live; vCenter hostname
	Version          string `json:"version"`          // INFERRED — validate live
	ConnectionStatus string `json:"connectionStatus"` // INFERRED — validate live (connected/disconnected)
}

// VMwareCollector maps GET /vmwares to VMware vCenter inventory metrics.
type VMwareCollector struct{}

// Name identifies the vmware collector.
func (VMwareCollector) Name() string { return "vmware" }

// Collect fetches GET /vmwares and maps VMware vCenter info to samples.
func (VMwareCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp vmwaresResponse
	if err := c.Get(ctx, "/vmwares", nsrclient.QueryOpts{
		Fields: []string{"name", "version", "connectionStatus"},
	}, &resp); err != nil {
		return nil, err
	}

	var b builder
	for _, v := range resp.VMwares {
		if v.Name == "" {
			continue
		}
		// nsr_vmware_info is an info gauge (always 1) carrying all label-only attributes.
		b.gauge("nsr_vmware_info", "A registered VMware vCenter (always 1).", 1,
			lbl("vcenter", v.Name),
			lbl("version", v.Version),
			lbl("status", v.ConnectionStatus),
		)
	}
	return b.out, nil
}
