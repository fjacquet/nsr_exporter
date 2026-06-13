package nsr

import (
	"context"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// protectionPoliciesResponse wraps GET /protectionpolicies:
// {"count":N,"protectionPolicies":[...]} (camelCase wrapper, swagger 19.13).
type protectionPoliciesResponse struct {
	Policies []nwProtectionPolicy `json:"protectionPolicies"`
}

// nwProtectionPolicy mirrors the swagger 19.13 Policy model. There is no top-level
// `enabled` or `clientCount`; enablement lives on each workflow, so policy-enabled
// is derived as "any workflow enabled".
type nwProtectionPolicy struct {
	Name      string `json:"name"`
	Workflows []struct {
		Enabled bool `json:"enabled"`
	} `json:"workflows"`
}

// protectionGroupsResponse wraps GET /protectiongroups:
// {"count":N,"protectionGroups":[...]} (camelCase wrapper, swagger 19.13).
type protectionGroupsResponse struct {
	Groups []nwProtectionGroup `json:"protectionGroups"`
}

// nwProtectionGroup mirrors the swagger 19.13 ProtectionGroup model. There is no
// `policy` back-reference or `clientCount`; the useful field is `workItemType`.
type nwProtectionGroup struct {
	Name         string `json:"name"`
	WorkItemType string `json:"workItemType"`
}

// PoliciesCollector maps GET /protectionpolicies and GET /protectiongroups.
type PoliciesCollector struct{}

// Name identifies the policies collector.
func (PoliciesCollector) Name() string { return "policies" }

// Collect fetches protection policies and groups.
func (PoliciesCollector) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var b builder

	var policies protectionPoliciesResponse
	if err := c.Get(ctx, "/protectionpolicies", nsrclient.QueryOpts{
		Fields: []string{"name", "workflows"},
	}, &policies); err != nil {
		return nil, err
	}
	for _, p := range policies.Policies {
		if p.Name == "" {
			continue
		}
		// A policy has no top-level enabled flag; treat it as enabled when any of its
		// workflows is enabled.
		enabled := 0.0
		for _, w := range p.Workflows {
			if w.Enabled {
				enabled = 1.0
				break
			}
		}
		b.gauge("nsr_policy_enabled", "1 if any workflow in the protection policy is enabled, else 0.", enabled,
			lbl("policy", p.Name))
	}

	var groups protectionGroupsResponse
	if err := c.Get(ctx, "/protectiongroups", nsrclient.QueryOpts{
		Fields: []string{"name", "workItemType"},
	}, &groups); err != nil {
		return nil, err
	}
	for _, g := range groups.Groups {
		if g.Name == "" {
			continue
		}
		b.gauge("nsr_group_info", "A protection group (always 1).", 1,
			lbl("group", g.Name),
			lbl("work_item_type", g.WorkItemType),
		)
	}
	return b.out, nil
}
