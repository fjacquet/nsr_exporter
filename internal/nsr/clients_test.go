package nsr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// TestClientsCollector_DuplicateHostname reproduces issue #36: a single NetWorker
// host registered as multiple distinct client RESOURCES (e.g. several parallel
// backup configs for the same hostname, each with its own resourceId) must not
// collide on /metrics. Before the fix, nsr_client_info/nsr_client_parallelism were
// labeled only by client_name, so two resources sharing a hostname produced two
// identical series and prometheus/client_golang's Registry.Gather rejected the
// scrape with "collected metric ... was collected before with the same name and
// label values" (a 500 on every scrape).
func TestClientsCollector_DuplicateHostname(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/nwrestapi/v3/global/clients", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":2,"clients":[
			{"hostname":"bbps001","ndmp":false,"scheduledBackup":true,"backupCommand":"save","parallelism":4,"os":"Linux","resourceId":{"id":"146.0.0.0.0.0.0.0.1(1)"}},
			{"hostname":"bbps001","ndmp":false,"scheduledBackup":true,"backupCommand":"save","parallelism":8,"os":"Linux","resourceId":{"id":"146.0.0.0.0.0.0.0.2(1)"}}
		]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := nsrclient.New(nsrclient.Options{Name: "nsr-test", Host: srv.URL, Username: "u", Password: "p"})
	store := NewSnapshotStore()
	c := &Collector{
		systems:    []system{{name: "nsr-test", client: client}},
		collectors: []ResourceCollector{ClientsCollector{}},
		store:      store,
		timeout:    5 * time.Second,
		log:        testLogger(),
		now:        nowZero,
	}
	c.CollectOnce(context.Background())

	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v (duplicate client_name series were not disambiguated)", err)
	}

	if !familyHasLabel(fams, "nsr_client_info", "resource_id", "146.0.0.0.0.0.0.0.1(1)") {
		t.Fatal("nsr_client_info missing resource_id=146.0.0.0.0.0.0.0.1(1)")
	}
	if !familyHasLabel(fams, "nsr_client_info", "resource_id", "146.0.0.0.0.0.0.0.2(1)") {
		t.Fatal("nsr_client_info missing resource_id=146.0.0.0.0.0.0.0.2(1)")
	}
	if got := familyMetricCount(fams, "nsr_client_parallelism"); got != 2 {
		t.Fatalf("nsr_client_parallelism series count = %d, want 2", got)
	}
}

func familyMetricCount(fams []*dto.MetricFamily, name string) int {
	for _, f := range fams {
		if f.GetName() == name {
			return len(f.Metric)
		}
	}
	return 0
}
