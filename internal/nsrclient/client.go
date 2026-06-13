// Package nsrclient is a lean go-resty/resty/v2 wrapper for the NetWorker REST API.
//
// NetWorker uses HTTP Basic auth on every request (no token/refresh dance) against
// base path /nwrestapi/v3/global — verified against dell/ansible-networker
// (plugins/modules/clients.go: auth=(user,pass); url=https://{host}:{port}/nwrestapi/v3/global).
// See ADR-0003 (hand-rolled client) and ADR-0007 (Basic auth).
package nsrclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

// basePath is appended to each system's host to form the API root.
const basePath = "/nwrestapi/v3/global"

// Client talks to one NetWorker system.
type Client struct {
	rc   *resty.Client
	name string
	log  *logrus.Logger
}

// Options configures a Client.
type Options struct {
	Name               string
	Host               string // e.g. https://nw.local:9090
	Username           string
	Password           string
	InsecureSkipVerify bool
	Timeout            time.Duration
	Trace              bool
	Log                *logrus.Logger
}

// New builds a Client. Retry is bounded and deliberately EXCLUDES 4xx so auth
// failures and bad requests are never retried (ADR-0007).
func New(o Options) *Client {
	rc := resty.New().
		SetBaseURL(strings.TrimRight(o.Host, "/")+basePath).
		SetBasicAuth(o.Username, o.Password).
		SetHeader("Accept", "application/json").
		SetTimeout(o.Timeout).
		SetRetryCount(2).
		SetRetryWaitTime(500 * time.Millisecond).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			// Retry transport errors and 5xx only; never 4xx.
			if err != nil {
				return true
			}
			return r.StatusCode() >= 500
		})

	// InsecureSkipVerify is an operator opt-in for self-signed appliance certs and
	// defaults to false. It is assigned from the resolved option (never a literal)
	// so the secure default is the code path and verification stays on unless the
	// operator explicitly disables it in config.
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	tlsCfg.InsecureSkipVerify = o.InsecureSkipVerify
	rc.SetTLSClientConfig(tlsCfg)

	c := &Client{rc: rc, name: o.Name, log: o.Log}
	if o.Trace {
		c.installTrace()
	}
	return c
}

// QueryOpts carries NetWorker's native field-projection and filter parameters.
// Always set Fields to fetch only what a collector maps; use Filter to bound large
// collections such as /backups (ADR-0010).
type QueryOpts struct {
	Fields []string // → fl=a,b,c
	Filter string   // → q=key:value and key2:value2
}

// Get fetches resourcePath (e.g. "/clients") and decodes the JSON body into out.
// out should target the wrapped envelope, e.g. struct{ Clients []Client }, since
// NetWorker list endpoints return {"count":N,"<resource>":[...]} rather than a
// bare array.
func (c *Client) Get(ctx context.Context, resourcePath string, opts QueryOpts, out any) error {
	req := c.rc.R().SetContext(ctx).SetResult(out)
	if len(opts.Fields) > 0 {
		req.SetQueryParam("fl", strings.Join(opts.Fields, ","))
	}
	if opts.Filter != "" {
		req.SetQueryParam("q", opts.Filter)
	}
	resp, err := req.Get(resourcePath)
	if err != nil {
		return fmt.Errorf("nsr %s GET %s: %w", c.name, resourcePath, err)
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("nsr %s GET %s: status %d", c.name, resourcePath, resp.StatusCode())
	}
	return nil
}

// installTrace logs each response as method/path/status/body ONLY. It must never
// use resty SetDebug: Basic auth puts base64 credentials in the Authorization
// REQUEST header, which SetDebug would dump. NetWorker responses carry no token,
// so body-only logging is safe here (architecture.md live-validation tooling).
func (c *Client) installTrace() {
	c.rc.OnAfterResponse(func(_ *resty.Client, r *resty.Response) error {
		c.log.WithFields(logrus.Fields{
			"system": c.name,
			"method": r.Request.Method,
			"path":   r.Request.URL,
			"status": r.StatusCode(),
			"body":   string(r.Body()),
		}).Debug("nsr api response")
		return nil
	})
}
