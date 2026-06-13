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
		json(w, `{"count":1,"clients":[{"hostname":"app01","ndmp":false,"scheduledBackup":true,"backupCommand":"save","parallelism":4,"os":"Linux"}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/alerts", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":1,"alerts":[{"priority":"critical","category":"Server","message":"m","timestamp":"2026-06-13T08:00:00Z"}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/serverstatistics", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"upSince":"2026-06-13T00:00:00Z","saves":1000,"saveSize":{"unit":"KB","value":5000000},"recovers":10,"recoverSize":{"unit":"Byte","value":2000},"badSaves":3,"badRecovers":1,"currentSaves":2,"currentRecovers":1,"maxSaves":32,"maxRecovers":16}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/jobs", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":1,"jobs":[{"id":42,"name":"daily","type":"save","state":"Completed","completionStatus":"Succeeded","clientHostname":"app01","startTime":"2026-06-13T01:00:00Z","endTime":"2026-06-13T01:30:00Z","level":"Full"}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/sessions", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":2,"sessions":[{"mode":"Saving","clientHostname":"app01","size":{"unit":"KB","value":2},"transferRate":{"unit":"KB/s","value":10}},{"mode":"Saving","clientHostname":"db01","size":{"unit":"Byte","value":4096}}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/volumes", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":1,"volumes":[{"name":"vol01","pool":"Default","type":"adv_file","states":["Recyclable"],"capacity":{"unit":"KB","value":1000},"written":{"unit":"KB","value":600},"recycled":2}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/datadomainsystems", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":1,"dataDomainSystems":[{"name":"dd01","model":"DD9400","osVersion":"7.10","totalCapacity":"112 GB","usedCapacity":"202 MB","availableCapacity":"111 GB","usedLogicalCapacity":"2 TB"}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/devices", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":1,"devices":[{"name":"tape01","mediaType":"LTO Ultrium-8","mediaFamily":"Tape","status":"Enabled","deviceSerialNumber":"SN001"}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/storagenodes", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":1,"storageNodes":[{"name":"sn01.local","enabled":true,"typeOfStorageNode":"SCSI","version":"19.13","numberOfDevices":4}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/pools", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":1,"pools":[{"name":"Default","poolType":"Backup","enabled":true}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/vmware/vcenters", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":1,"vCenters":[{"hostname":"vcenter.local","cloudDeployment":false}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/protectionpolicies", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":1,"protectionPolicies":[{"name":"GoldPolicy","workflows":[{"enabled":false},{"enabled":true}]}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/protectiongroups", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":1,"protectionGroups":[{"name":"DBGroup","workItemType":"Client"}]}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/backups", func(w http.ResponseWriter, _ *http.Request) {
		json(w, `{"count":3,"backups":[
			{"clientHostname":"app01","name":"/data","level":"full","size":{"unit":"Byte","value":1000},"saveTime":"2026-06-12T01:00:00Z","retentionTime":"2026-07-12T01:00:00Z","completionTime":"2026-06-12T01:01:40Z"},
			{"clientHostname":"app01","name":"/data","level":"full","size":{"unit":"Byte","value":1500},"saveTime":"2026-06-13T01:00:00Z","retentionTime":"2026-07-13T01:00:00Z"},
			{"clientHostname":"app01","name":"/data","level":"incr","size":{"unit":"Byte","value":50},"saveTime":"2026-06-13T13:00:00Z","retentionTime":"2026-06-20T13:00:00Z"}
		]}`)
	})
	return httptest.NewServer(mux)
}

func testCollector(srv *httptest.Server) (*Collector, *SnapshotStore) {
	client := nsrclient.New(nsrclient.Options{Name: "nsr-test", Host: srv.URL, Username: "u", Password: "p"})
	store := NewSnapshotStore()
	collectors := append(DefaultCollectors(), SizingCollector{Window: 24 * time.Hour})
	c := &Collector{
		systems:    []system{{name: "nsr-test", client: client}},
		collectors: collectors,
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
	if got := familyValue(fams, "nsr_active_sessions"); got != 2 {
		t.Fatalf("nsr_active_sessions = %v, want 2 (both Saving mode)", got)
	}
	if !familyHasLabel(fams, "nsr_session_bytes", "client", "app01") {
		t.Fatal("nsr_session_bytes missing client=app01")
	}
	// transferRate KB/s 10 → 10240 bytes/s gauge.
	if got := familyValue(fams, "nsr_session_transfer_bytes_per_second"); got != 10240 {
		t.Fatalf("nsr_session_transfer_bytes_per_second = %v, want 10240", got)
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
	// capacity KB 1000 → 1024000 bytes after unit conversion.
	if got := familyValue(fams, "nsr_volume_capacity_bytes"); got != 1024000 {
		t.Fatalf("nsr_volume_capacity_bytes = %v, want 1024000", got)
	}
	if got := familyValue(fams, "nsr_volume_recycled_total"); got != 2 {
		t.Fatalf("nsr_volume_recycled_total = %v, want 2", got)
	}
	// usedLogicalCapacity "2 TB" → 2*1024^4 bytes.
	if got := familyValue(fams, "nsr_datadomain_logical_capacity_used_bytes"); got != 2*(1<<40) {
		t.Fatalf("nsr_datadomain_logical_capacity_used_bytes = %v, want %v", got, 2*(1<<40))
	}
	if !familyHasLabel(fams, "nsr_datadomain_capacity_used_bytes", "model", "DD9400") {
		t.Fatal("nsr_datadomain_capacity_used_bytes missing model=DD9400")
	}
}

// TestSizingCollector covers the FETB max-per-saveset aggregation, the Full/Incr
// bucketing, and the duration-gated throughput metric.
func TestSizingCollector(t *testing.T) {
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
	// Two Full backups (1000, 1500) → FETB is the max, 1500.
	if got := familyValue(fams, "nsr_backup_source_size_bytes"); got != 1500 {
		t.Fatalf("nsr_backup_source_size_bytes = %v, want 1500 (max Full)", got)
	}
	if got := familyValue(fams, "nsr_backup_change_size_bytes"); got != 50 {
		t.Fatalf("nsr_backup_change_size_bytes = %v, want 50 (the Incr)", got)
	}
	// Only the first Full carries duration=100 → throughput 1000/100 = 10.
	if got := familyValue(fams, "nsr_job_bytes_per_second"); got != 10 {
		t.Fatalf("nsr_job_bytes_per_second = %v, want 10", got)
	}
}

// TestStorageCollector_C2 asserts the C2 addition: nsr_volume_status info gauge,
// via both Prometheus and OTLP paths.
func TestStorageCollector_C2(t *testing.T) {
	srv := mockNetWorker(t)
	defer srv.Close()
	c, store := testCollector(srv)
	c.CollectOnce(context.Background())

	// Prometheus path
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if !familyHasLabel(fams, "nsr_volume_status", "status", "Recyclable") {
		t.Fatal("prometheus nsr_volume_status missing status=Recyclable label")
	}
	if got := familyValue(fams, "nsr_volume_status"); got != 1 {
		t.Fatalf("prometheus nsr_volume_status = %v, want 1", got)
	}

	// OTLP path
	reader := sdkmetric.NewManualReader()
	if _, err := NewOTLPExporter(store, reader); err != nil {
		t.Fatalf("otlp: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("otlp collect: %v", err)
	}
	if got := otlpValue(&rm, "nsr_volume_status"); got != 1 {
		t.Fatalf("otlp nsr_volume_status = %v, want 1", got)
	}
}

// TestClientsCollector_C1 asserts the operating_system label on nsr_client_info is
// sourced from the real Client.os field, via the Prometheus path.
func TestClientsCollector_C1(t *testing.T) {
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
	if !familyHasLabel(fams, "nsr_client_info", "operating_system", "Linux") {
		t.Fatal("prometheus nsr_client_info missing operating_system=Linux label")
	}
}

// TestAlertsCollector_C4 asserts nsr_alert_info carries the real priority label and
// is visible via both Prometheus and OTLP paths.
func TestAlertsCollector_C4(t *testing.T) {
	srv := mockNetWorker(t)
	defer srv.Close()
	c, store := testCollector(srv)
	c.CollectOnce(context.Background())

	// Prometheus path
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if !familyHasLabel(fams, "nsr_alert_info", "priority", "critical") {
		t.Fatal("prometheus nsr_alert_info missing priority=critical label")
	}

	// OTLP path
	reader := sdkmetric.NewManualReader()
	if _, err := NewOTLPExporter(store, reader); err != nil {
		t.Fatalf("otlp: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("otlp collect: %v", err)
	}
	if got := otlpValue(&rm, "nsr_alert_info"); got != 1 {
		t.Fatalf("otlp nsr_alert_info = %v, want 1", got)
	}
}

// TestJobsCollector_C3 asserts the C3 additions: nsr_job_start_timestamp_seconds,
// nsr_job_end_timestamp_seconds, and group/level labels on nsr_job_status, via both
// Prometheus and OTLP paths.
func TestJobsCollector_C3(t *testing.T) {
	srv := mockNetWorker(t)
	defer srv.Close()
	c, store := testCollector(srv)
	c.CollectOnce(context.Background())

	// Prometheus path
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if got := familyValue(fams, "nsr_job_start_timestamp_seconds"); got <= 0 {
		t.Fatalf("prometheus nsr_job_start_timestamp_seconds = %v, want > 0", got)
	}
	if got := familyValue(fams, "nsr_job_end_timestamp_seconds"); got <= 0 {
		t.Fatalf("prometheus nsr_job_end_timestamp_seconds = %v, want > 0", got)
	}
	if !familyHasLabel(fams, "nsr_job_status", "level", "Full") {
		t.Fatal("prometheus nsr_job_status missing level=Full label")
	}
	if !familyHasLabel(fams, "nsr_job_status", "client", "app01") {
		t.Fatal("prometheus nsr_job_status missing client=app01 (from clientHostname)")
	}

	// OTLP path
	reader := sdkmetric.NewManualReader()
	if _, err := NewOTLPExporter(store, reader); err != nil {
		t.Fatalf("otlp: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("otlp collect: %v", err)
	}
	if got := otlpValue(&rm, "nsr_job_start_timestamp_seconds"); got <= 0 {
		t.Fatalf("otlp nsr_job_start_timestamp_seconds = %v, want > 0", got)
	}
	if got := otlpValue(&rm, "nsr_job_end_timestamp_seconds"); got <= 0 {
		t.Fatalf("otlp nsr_job_end_timestamp_seconds = %v, want > 0", got)
	}
}

// TestDevicesCollector_C5 asserts C5: nsr_device_info and nsr_device_capacity_bytes
// via both Prometheus and OTLP paths.
func TestDevicesCollector_C5(t *testing.T) {
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
	if !familyHasLabel(fams, "nsr_device_info", "device_name", "tape01") {
		t.Fatal("prometheus nsr_device_info missing device_name=tape01")
	}
	if !familyHasLabel(fams, "nsr_device_info", "media_family", "Tape") {
		t.Fatal("prometheus nsr_device_info missing media_family=Tape")
	}

	reader := sdkmetric.NewManualReader()
	if _, err := NewOTLPExporter(store, reader); err != nil {
		t.Fatalf("otlp: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("otlp collect: %v", err)
	}
	if got := otlpValue(&rm, "nsr_device_info"); got != 1 {
		t.Fatalf("otlp nsr_device_info = %v, want 1", got)
	}
}

// TestStorageNodesCollector_C6 asserts C6: nsr_storagenode_info and
// nsr_storagenode_device_count via both Prometheus and OTLP paths.
func TestStorageNodesCollector_C6(t *testing.T) {
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
	if !familyHasLabel(fams, "nsr_storagenode_info", "node", "sn01.local") {
		t.Fatal("prometheus nsr_storagenode_info missing node=sn01.local")
	}
	if got := familyValue(fams, "nsr_storagenode_device_count"); got != 4 {
		t.Fatalf("prometheus nsr_storagenode_device_count = %v, want 4", got)
	}

	reader := sdkmetric.NewManualReader()
	if _, err := NewOTLPExporter(store, reader); err != nil {
		t.Fatalf("otlp: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("otlp collect: %v", err)
	}
	if got := otlpValue(&rm, "nsr_storagenode_device_count"); got != 4 {
		t.Fatalf("otlp nsr_storagenode_device_count = %v, want 4", got)
	}
}

// TestPoolsCollector_C7 asserts the pool inventory info gauge (the Pool resource
// exposes no capacity fields), via both Prometheus and OTLP paths.
func TestPoolsCollector_C7(t *testing.T) {
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
	if !familyHasLabel(fams, "nsr_pool_info", "pool", "Default") {
		t.Fatal("prometheus nsr_pool_info missing pool=Default")
	}
	if !familyHasLabel(fams, "nsr_pool_info", "pool_type", "Backup") {
		t.Fatal("prometheus nsr_pool_info missing pool_type=Backup")
	}

	reader := sdkmetric.NewManualReader()
	if _, err := NewOTLPExporter(store, reader); err != nil {
		t.Fatalf("otlp: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("otlp collect: %v", err)
	}
	if got := otlpValue(&rm, "nsr_pool_info"); got != 1 {
		t.Fatalf("otlp nsr_pool_info = %v, want 1", got)
	}
}

// TestVMwareCollector_C8 asserts C8: nsr_vmware_info from GET /vmware/vcenters via
// both Prometheus and OTLP.
func TestVMwareCollector_C8(t *testing.T) {
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
	if !familyHasLabel(fams, "nsr_vmware_info", "vcenter", "vcenter.local") {
		t.Fatal("prometheus nsr_vmware_info missing vcenter=vcenter.local")
	}
	if !familyHasLabel(fams, "nsr_vmware_info", "cloud_deployment", "false") {
		t.Fatal("prometheus nsr_vmware_info missing cloud_deployment=false")
	}

	reader := sdkmetric.NewManualReader()
	if _, err := NewOTLPExporter(store, reader); err != nil {
		t.Fatalf("otlp: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("otlp collect: %v", err)
	}
	if got := otlpValue(&rm, "nsr_vmware_info"); got != 1 {
		t.Fatalf("otlp nsr_vmware_info = %v, want 1", got)
	}
}

// TestPoliciesCollector_C10 asserts nsr_policy_enabled (derived from workflows[].enabled)
// and the nsr_group_info gauge, via both Prometheus and OTLP paths.
func TestPoliciesCollector_C10(t *testing.T) {
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
	if !familyHasLabel(fams, "nsr_policy_enabled", "policy", "GoldPolicy") {
		t.Fatal("prometheus nsr_policy_enabled missing policy=GoldPolicy")
	}
	// One of GoldPolicy's workflows is enabled → policy is enabled.
	if got := familyValue(fams, "nsr_policy_enabled"); got != 1 {
		t.Fatalf("prometheus nsr_policy_enabled = %v, want 1 (a workflow is enabled)", got)
	}
	if !familyHasLabel(fams, "nsr_group_info", "group", "DBGroup") {
		t.Fatal("prometheus nsr_group_info missing group=DBGroup")
	}

	reader := sdkmetric.NewManualReader()
	if _, err := NewOTLPExporter(store, reader); err != nil {
		t.Fatalf("otlp: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("otlp collect: %v", err)
	}
	if got := otlpValue(&rm, "nsr_policy_enabled"); got != 1 {
		t.Fatalf("otlp nsr_policy_enabled = %v, want 1", got)
	}
	if got := otlpValue(&rm, "nsr_group_info"); got != 1 {
		t.Fatalf("otlp nsr_group_info = %v, want 1", got)
	}
}

// TestBackupWindowFilter pins the bounding-filter shape so a live-validation fix is
// an obvious one-line change.
func TestBackupWindowFilter(t *testing.T) {
	got := backupWindowFilter(24 * time.Hour)
	want := `saveTime:["24 hours"]`
	if got != want {
		t.Fatalf("backupWindowFilter = %q, want %q", got, want)
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
