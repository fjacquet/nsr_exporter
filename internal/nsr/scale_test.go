package nsr

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// Default benchmark sizes: the spec's starting guess for a 5,000-client server.
const (
	scaleClients  = 5000
	scaleSessions = 10000
	scaleJobs     = 50000
)

func TestCollectionDurationMetric(t *testing.T) {
	srv := mockNetWorker(t)
	defer srv.Close()
	c, store := testCollector(srv)
	c.CollectOnce(context.Background())

	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if !familyHasLabel(fams, "nsr_collection_duration_seconds", "system", "nsr-test") {
		t.Fatal("prometheus nsr_collection_duration_seconds{system=nsr-test} missing")
	}
	if got := familyValue(fams, "nsr_collection_duration_seconds"); got < 0 {
		t.Fatalf("prometheus duration = %v, want >= 0", got)
	}

	reader := sdkmetric.NewManualReader()
	if _, err := NewOTLPExporter(store, reader); err != nil {
		t.Fatalf("otlp: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("otlp collect: %v", err)
	}
	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "nsr_collection_duration_seconds" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("otlp nsr_collection_duration_seconds missing")
	}
	if got := otlpValue(&rm, "nsr_collection_duration_seconds"); got < 0 {
		t.Fatalf("otlp duration = %v, want >= 0", got)
	}
}

// scaleMock serves synthetic /clients, /sessions and /jobs of the given sizes and
// minimal valid bodies for every other endpoint so nsr_up stays 1.
func scaleMock(t testing.TB, clients, sessions, jobs int) *httptest.Server {
	t.Helper()
	const p = "/nwrestapi/v3/global/"
	static := map[string]string{
		"alerts":             `{"count":0,"alerts":[]}`,
		"serverstatistics":   `{"upSince":"2026-06-13T00:00:00Z","saves":1,"saveSize":{"unit":"KB","value":1},"recovers":1,"recoverSize":{"unit":"Byte","value":1},"badSaves":0,"badRecovers":0,"currentSaves":1,"currentRecovers":1,"maxSaves":32,"maxRecovers":16}`,
		"volumes":            `{"count":0,"volumes":[]}`,
		"datadomainsystems":  `{"count":0,"dataDomainSystems":[]}`,
		"devices":            `{"count":0,"devices":[]}`,
		"storagenodes":       `{"count":0,"storageNodes":[]}`,
		"pools":              `{"count":0,"pools":[]}`,
		"vmware/vcenters":    `{"count":0,"vCenters":[]}`,
		"protectionpolicies": `{"count":0,"protectionPolicies":[]}`,
		"protectiongroups":   `{"count":0,"protectionGroups":[]}`,
		"backups":            `{"count":0,"backups":[]}`,
	}
	mux := http.NewServeMux()
	serve := func(path, body string) {
		mux.HandleFunc(p+path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			writeBody(w, body)
		})
	}
	for path, body := range static {
		serve(path, body)
	}

	var b strings.Builder
	fmt.Fprintf(&b, `{"count":%d,"clients":[`, clients)
	for i := 0; i < clients; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"hostname":"client%05d.example.com","resourceId":{"id":"1.0.%d.1","sequence":1},"ndmp":false,"scheduledBackup":true,"backupCommand":"save","parallelism":4,"os":"Linux"}`, i, i)
	}
	b.WriteString(`]}`)
	serve("clients", b.String())

	b.Reset()
	fmt.Fprintf(&b, `{"count":%d,"sessions":[`, sessions)
	for i := 0; i < sessions; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		mode := "Saving"
		if i%10 == 0 {
			mode = "Recovering"
		}
		fmt.Fprintf(&b, `{"mode":%q,"clientHostname":"client%05d.example.com","size":{"unit":"KB","value":%d},"transferRate":{"unit":"KB/s","value":%d}}`, mode, i%max(clients, 1), 1000+i, 10+i%50)
	}
	b.WriteString(`]}`)
	serve("sessions", b.String())

	// startTime spread evenly over the last 72h; computed once at fixture build.
	now := time.Now().UTC()
	b.Reset()
	fmt.Fprintf(&b, `{"count":%d,"jobs":[`, jobs)
	for i := 0; i < jobs; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		start := now.Add(-time.Duration(i) * 72 * time.Hour / time.Duration(max(jobs, 1)))
		fmt.Fprintf(&b, `{"id":%d,"name":"policy%d","type":"save","state":"Completed","completionStatus":"Succeeded","clientHostname":"client%05d.example.com","startTime":%q,"endTime":%q,"level":"Full"}`,
			i+1, i%20, i%max(clients, 1), start.Format(time.RFC3339), start.Add(30*time.Minute).Format(time.RFC3339))
	}
	b.WriteString(`]}`)
	serve("jobs", b.String())

	return httptest.NewServer(mux)
}

func TestScaleMockKeepsSystemUp(t *testing.T) {
	srv := scaleMock(t, 20, 40, 100)
	defer srv.Close()
	c, store := testCollector(srv)
	c.CollectOnce(context.Background())
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if got := familyValue(fams, "nsr_up"); got != 1 {
		t.Fatalf("nsr_up = %v, want 1", got)
	}
}

func scaleBench(b *testing.B) (*Collector, *prometheus.Registry) {
	b.Helper()
	srv := scaleMock(b, scaleClients, scaleSessions, scaleJobs)
	b.Cleanup(srv.Close)
	c, store := testCollector(srv)
	c.timeout = 5 * time.Minute
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	return c, reg
}

func render(b *testing.B, reg *prometheus.Registry) int {
	b.Helper()
	rec := httptest.NewRecorder()
	promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		b.Fatalf("/metrics status %d", rec.Code)
	}
	return rec.Body.Len()
}

func BenchmarkCollectAndGather(b *testing.B) {
	c, reg := scaleBench(b)
	b.ReportAllocs()
	b.ResetTimer()
	var size int
	for i := 0; i < b.N; i++ {
		c.CollectOnce(context.Background())
		if _, err := reg.Gather(); err != nil {
			b.Fatal(err)
		}
		size = render(b, reg)
	}
	b.ReportMetric(float64(size), "body_bytes")
}

func BenchmarkGatherOnly(b *testing.B) {
	c, reg := scaleBench(b)
	c.CollectOnce(context.Background())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := reg.Gather(); err != nil {
			b.Fatal(err)
		}
	}
}
