package nsr

import (
	"context"
	"strconv"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// vcentersResponse wraps GET /vmware/vcenters: {"count":N,"vCenters":[...]}. The
// wrapper key is camelCase `vCenters` (swagger 19.13).
type vcentersResponse struct {
	VCenters []nwVCenter `json:"vCenters"`
}

// nwVCenter mirrors the swagger 19.13 VCenter model. The identity field is
// `hostname`; there are no `version` or `connectionStatus` fields on a vCenter.
type nwVCenter struct {
	Hostname        string `json:"hostname"`
	CloudDeployment bool   `json:"cloudDeployment"`
}

// VMwareCollector maps GET /vmware/vcenters to VMware vCenter inventory metrics.
type VMwareCollector struct{}

// Name identifies the vmware collector.
func (VMwareCollector) Name() string { return "vmware" }

// Collect fetches GET /vmware/vcenters and maps vCenter info to samples.
func (VMwareCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var resp vcentersResponse
	if err := c.Get(ctx, "/vmware/vcenters", nsrclient.QueryOpts{
		Fields: []string{"hostname", "cloudDeployment"},
	}, &resp); err != nil {
		return nil, err
	}

	var b builder
	for _, v := range resp.VCenters {
		if v.Hostname == "" {
			continue
		}
		// nsr_vmware_info is an info gauge (always 1) carrying all label-only attributes.
		b.gauge("nsr_vmware_info", "A registered VMware vCenter (always 1).", 1,
			lbl("vcenter", v.Hostname),
			lbl("cloud_deployment", strconv.FormatBool(v.CloudDeployment)),
		)
	}
	return b.out, nil
}
