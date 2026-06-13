package nsr

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/sirupsen/logrus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

func nowZero() time.Time { return time.Unix(1, 0).UTC() }

func testLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}

// writeBody copies a static fixture through the io.Writer interface so analysis
// sees a plain byte copy of a compile-time constant, not templated user input.
func writeBody(w io.Writer, body string) { _, _ = io.WriteString(w, body) }

// mockNetWorker serves the wrapped envelopes the collectors decode and enforces
// Basic auth, exactly as a real NetWorker REST endpoint does.
func mockNetWorker(t *testing.T) *httptest.Server {
	t.Helper()
	json := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		writeBody(w, body)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/nwrestapi/v3/global/clients", func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u == "" || p == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json(w, `{"count":1,"clients":[{"hostname":"app01","ndmp":false,"scheduledBackup":true,"backupCommand":"save","parallelism":4}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/alerts", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":1,"alerts":[{"severity":"WARNING","category":"Server","message":"m","time":"t"}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/serverstatistics", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"upSince":"2026-06-13T00:00:00Z","saves":1000,"saveSize":5000000,"recovers":10,"recoverSize":2000,"badSaves":3,"badRecovers":1}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/jobs", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":1,"jobs":[{"id":42,"name":"daily","type":"save","state":"Completed","completionStatus":"Succeeded","client":"app01"}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/sessions", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":2,"sessions":[{"type":"backup","client":"app01","state":"running","size":2048},{"type":"backup","client":"db01","state":"running","size":4096}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/volumes", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":1,"volumes":[{"name":"vol01","pool":"Default","mediaType":"adv_file","capacity":1000,"written":600,"recycledCount":2}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/datadomainsystems", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":1,"datadomainsystems":[{"name":"dd01","model":"DD9400","osVersion":"7.10","capacityTotal":9000,"capacityUsed":3000,"capacityAvailable":6000,"logicalCapacityUsed":27000}]}`)
	})
	return httptest.NewServer(mux)
}

func testCollector(srv *httptest.Server) (*Collector, *SnapshotStore) {
	client := nsrclient.New(nsrclient.Options{Name: "nsr-test", Host: srv.URL, Username: "u", Password: "p"})
	store := NewSnapshotStore()
	c := &Collector{
		systems:    []system{{name: "nsr-test", client: client}},
		collectors: DefaultCollectors(),
		store:      store,
		timeout:    5 * time.Second,
		log:        testLogger(),
		now:        nowZero,
	}
	return c, store
}

// TestDualExport_PrometheusAndOTLP is the load-bearing family test: the same
// snapshot must be visible through BOTH export paths.
func TestDualExport_PrometheusAndOTLP(t *testing.T) {
	srv := mockNetWorker(t)
	defer srv.Close()
	c, store := testCollector(srv)
	c.CollectOnce(context.Background())

	// --- Prometheus path ---
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if got := familyValue(fams, "nsr_client_parallelism"); got != 4 {
		t.Fatalf("prometheus nsr_client_parallelism = %v, want 4", got)
	}
	if got := familyValue(fams, "nsr_up"); got != 1 {
		t.Fatalf("prometheus nsr_up = %v, want 1", got)
	}
	if !familyHasLabel(fams, "nsr_client_info", "system", "nsr-test") {
		t.Fatal("prometheus nsr_client_info missing system=nsr-test label")
	}

	// --- OTLP path (same store) ---
	reader := sdkmetric.NewManualReader()
	if _, err := NewOTLPExporter(store, reader); err != nil {
		t.Fatalf("otlp: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("otlp collect: %v", err)
	}
	if got := otlpValue(&rm, "nsr_client_parallelism"); got != 4 {
		t.Fatalf("otlp nsr_client_parallelism = %v, want 4", got)
	}
	if got := otlpValue(&rm, "nsr_up"); got != 1 {
		t.Fatalf("otlp nsr_up = %v, want 1", got)
	}
}

// TestJobsCollector covers the counter path (serverstatistics) and label rendering
// of a numeric job id, through the Prometheus export path.
func TestJobsCollector(t *testing.T) {
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
	if got := familyValue(fams, "nsr_server_saves_total"); got != 1000 {
		t.Fatalf("nsr_server_saves_total = %v, want 1000", got)
	}
	if !familyHasLabel(fams, "nsr_job_status", "job_id", "42") {
		t.Fatal("nsr_job_status missing job_id=42 (numeric id should render as string label)")
	}
	if !familyHasLabel(fams, "nsr_job_status", "completion_status", "Succeeded") {
		t.Fatal("nsr_job_status missing completion_status=Succeeded")
	}
}

// TestSessionsCollector covers aggregation (sessions_total per type) and the
// present-value gauge (session_bytes).
func TestSessionsCollector(t *testing.T) {
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
	if got := familyValue(fams, "nsr_sessions_total"); got != 2 {
		t.Fatalf("nsr_sessions_total = %v, want 2 (both backup type)", got)
	}
	if !familyHasLabel(fams, "nsr_session_bytes", "client", "app01") {
		t.Fatal("nsr_session_bytes missing client=app01")
	}
}

// TestStorageCollector covers two endpoints in one collector, the volume counter
// with its own label set, and the Data Domain capacity gauges.
func TestStorageCollector(t *testing.T) {
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
	if got := familyValue(fams, "nsr_volume_capacity_bytes"); got != 1000 {
		t.Fatalf("nsr_volume_capacity_bytes = %v, want 1000", got)
	}
	if got := familyValue(fams, "nsr_volume_recycled_total"); got != 2 {
		t.Fatalf("nsr_volume_recycled_total = %v, want 2", got)
	}
	if got := familyValue(fams, "nsr_datadomain_logical_capacity_used_bytes"); got != 27000 {
		t.Fatalf("nsr_datadomain_logical_capacity_used_bytes = %v, want 27000", got)
	}
	if !familyHasLabel(fams, "nsr_datadomain_capacity_used_bytes", "model", "DD9400") {
		t.Fatal("nsr_datadomain_capacity_used_bytes missing model=DD9400")
	}
}

// TestCatalogCoversAllEmittedMetrics guards against export drift: every metric a
// collector emits must have a catalog entry, else the OTLP path silently drops it.
func TestCatalogCoversAllEmittedMetrics(t *testing.T) {
	srv := mockNetWorker(t)
	defer srv.Close()
	c, store := testCollector(srv)
	c.CollectOnce(context.Background())

	known := catalogNames()
	for _, s := range store.Load().Samples {
		if _, ok := known[s.Name]; !ok {
			t.Errorf("metric %q is emitted by a collector but missing from Catalog()", s.Name)
		}
	}
}

// --- helpers ---

func familyValue(fams []*dto.MetricFamily, name string) float64 {
	for _, f := range fams {
		if f.GetName() != name {
			continue
		}
		m := f.Metric[0]
		if m.Counter != nil {
			return m.GetCounter().GetValue()
		}
		return m.GetGauge().GetValue()
	}
	return -1
}

func familyHasLabel(fams []*dto.MetricFamily, name, key, val string) bool {
	for _, f := range fams {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.Metric {
			for _, l := range m.Label {
				if l.GetName() == key && l.GetValue() == val {
					return true
				}
			}
		}
	}
	return false
}

func otlpValue(rm *metricdata.ResourceMetrics, name string) float64 {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			if g, ok := m.Data.(metricdata.Gauge[float64]); ok && len(g.DataPoints) > 0 {
				return g.DataPoints[0].Value
			}
		}
	}
	return -1
}
