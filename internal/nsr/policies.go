package nsr

import (
	"context"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// protectionPoliciesResponse wraps GET /protectionpolicies:
// {"count":N,"protectionpolicies":[...]}.
type protectionPoliciesResponse struct {
	Policies []nwProtectionPolicy `json:"protectionpolicies"`
}

type nwProtectionPolicy struct {
	Name        string   `json:"name"`        // INFERRED — validate live
	Enabled     bool     `json:"enabled"`     // INFERRED — validate live
	ClientCount *float64 `json:"clientCount"` // INFERRED — validate live
}

// protectionGroupsResponse wraps GET /protectiongroups:
// {"count":N,"protectiongroups":[...]}.
type protectionGroupsResponse struct {
	Groups []nwProtectionGroup `json:"protectiongroups"`
}

type nwProtectionGroup struct {
	Name        string   `json:"name"`        // INFERRED — validate live
	Policy      string   `json:"policy"`      // INFERRED — validate live; parent policy name
	ClientCount *float64 `json:"clientCount"` // INFERRED — validate live
}

// PoliciesCollector maps GET /protectionpolicies and GET /protectiongroups.
type PoliciesCollector struct{}

// Name identifies the policies collector.
func (PoliciesCollector) Name() string { return "policies" }

// Collect fetches protection policies and groups, correlating them in-process to
// avoid N+1 calls per cycle.
func (PoliciesCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var b builder

	var policies protectionPoliciesResponse
	if err := c.Get(ctx, "/protectionpolicies", nsrclient.QueryOpts{
		Fields: []string{"name", "enabled", "clientCount"},
	}, &policies); err != nil {
		return nil, err
	}
	for _, p := range policies.Policies {
		if p.Name == "" {
			continue
		}
		enabled := 0.0
		if p.Enabled {
			enabled = 1.0
		}
		b.gauge("nsr_policy_enabled", "1 if the protection policy is enabled, else 0.", enabled,
			lbl("policy", p.Name))
		// Absent client count yields no sample (ADR-0008).
		emitGauge(&b, "nsr_policy_client_count", "Number of clients covered by this policy.",
			p.ClientCount, lbl("policy", p.Name))
	}

	var groups protectionGroupsResponse
	if err := c.Get(ctx, "/protectiongroups", nsrclient.QueryOpts{
		Fields: []string{"name", "policy", "clientCount"},
	}, &groups); err != nil {
		return nil, err
	}
	for _, g := range groups.Groups {
		if g.Name == "" {
			continue
		}
		// nsr_group_client_count correlates group and policy in-process (no extra requests).
		emitGauge(&b, "nsr_group_client_count", "Number of clients in this protection group.",
			g.ClientCount, lbl("group", g.Name), lbl("policy", g.Policy))
	}
	return b.out, nil
}
