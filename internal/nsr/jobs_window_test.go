package nsr

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

var windowNow = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

func windowNowFn() time.Time { return windowNow }

// jobsServer serves /serverstatistics and /jobs only; it records the q parameter
// the exporter sent on /jobs and answers with status/body.
func jobsServer(t *testing.T, status int, body string, gotQ *string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/nwrestapi/v3/global/serverstatistics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeBody(w, `{"saves":1}`)
	})
	mux.HandleFunc("/nwrestapi/v3/global/jobs", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if gotQ != nil {
			*gotQ = r.URL.Query().Get("q")
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		writeBody(w, body)
	})
	return httptest.NewServer(mux)
}

func jobJSON(id int, start time.Time, state string) string {
	st := ""
	if !start.IsZero() {
		st = start.Format(time.RFC3339)
	}
	return fmt.Sprintf(`{"id":%d,"name":"j%d","type":"save","state":%q,"completionStatus":"Succeeded","clientHostname":"c","startTime":%q,"level":"Full"}`, id, id, state, st)
}

// gatherJobs reads job series through both export paths.
func gatherJobs(t *testing.T, store *SnapshotStore) (statusIDs map[string]bool, startSeries int, otlpStatus int) {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	statusIDs = map[string]bool{}
	for _, f := range fams {
		for _, m := range f.Metric {
			switch f.GetName() {
			case "nsr_job_status":
				for _, l := range m.Label {
					if l.GetName() == "job_id" {
						statusIDs[l.GetValue()] = true
					}
				}
			case "nsr_job_start_timestamp_seconds":
				startSeries++
			}
		}
	}
	reader := sdkmetric.NewManualReader()
	if _, err := NewOTLPExporter(store, reader); err != nil {
		t.Fatalf("otlp: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("otlp collect: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if g, ok := m.Data.(metricdata.Gauge[float64]); ok && m.Name == "nsr_job_status" {
				otlpStatus = len(g.DataPoints)
			}
		}
	}
	return statusIDs, startSeries, otlpStatus
}

func windowCollector(srv *httptest.Server, window time.Duration) (*Collector, *SnapshotStore) {
	c, store := testCollector(srv)
	c.collectors = []ResourceCollector{JobsCollector{Window: window, Now: windowNowFn}}
	return c, store
}

func TestJobWindowFilter(t *testing.T) {
	if got, want := jobWindowFilter(24*time.Hour), `startTime:["24 hours"]`; got != want {
		t.Fatalf("jobWindowFilter(24h) = %q, want %q", got, want)
	}
	if got, want := jobWindowFilter(10*time.Minute), `startTime:["1 hours"]`; got != want {
		t.Fatalf("jobWindowFilter(10m) = %q, want %q", got, want)
	}
	if got, want := jobWindowFilter(90*time.Minute), `startTime:["2 hours"]`; got != want {
		t.Fatalf("jobWindowFilter(90m) = %q, want %q", got, want)
	}
}

func TestJobs_SendsWindowFilter(t *testing.T) {
	var q string
	srv := jobsServer(t, 200, `{"count":0,"jobs":[]}`, &q)
	defer srv.Close()
	c, _ := windowCollector(srv, 24*time.Hour)
	c.CollectOnce(context.Background())
	if want := jobWindowFilter(24 * time.Hour); q != want {
		t.Fatalf("q = %q, want %q", q, want)
	}
}

func TestJobs_ClientSideCut(t *testing.T) {
	body := `{"count":3,"jobs":[` +
		jobJSON(1, windowNow.Add(-1*time.Hour), "Completed") + "," +
		jobJSON(2, windowNow.Add(-23*time.Hour), "Completed") + "," +
		jobJSON(3, windowNow.Add(-48*time.Hour), "Completed") + `]}`
	srv := jobsServer(t, 200, body, nil)
	defer srv.Close()
	c, store := windowCollector(srv, 24*time.Hour)
	c.CollectOnce(context.Background())
	ids, starts, otlpN := gatherJobs(t, store)
	if len(ids) != 2 || !ids["1"] || !ids["2"] || ids["3"] {
		t.Fatalf("job ids = %v, want {1,2}", ids)
	}
	if starts != 2 || otlpN != 2 {
		t.Fatalf("start series = %d, otlp status points = %d, want 2 and 2", starts, otlpN)
	}
}

func TestJobs_UnparseableStartTimeKept(t *testing.T) {
	srv := jobsServer(t, 200, `{"count":1,"jobs":[`+jobJSON(7, time.Time{}, "Completed")+`]}`, nil)
	defer srv.Close()
	c, store := windowCollector(srv, 24*time.Hour)
	c.CollectOnce(context.Background())
	ids, starts, otlpN := gatherJobs(t, store)
	if !ids["7"] || otlpN != 1 {
		t.Fatalf("job 7 dropped: ids=%v otlp=%d", ids, otlpN)
	}
	if starts != 0 {
		t.Fatalf("start series = %d, want 0 (absent, never zero)", starts)
	}
}

// Accepted behavior: the cut is on startTime only, so a still-running job that
// began before the window disappears from nsr_job_status.
func TestJobs_RunningJobBeforeWindowDropped(t *testing.T) {
	srv := jobsServer(t, 200, `{"count":1,"jobs":[`+jobJSON(9, windowNow.Add(-30*time.Hour), "Running")+`]}`, nil)
	defer srv.Close()
	c, store := windowCollector(srv, 24*time.Hour)
	c.CollectOnce(context.Background())
	ids, _, _ := gatherJobs(t, store)
	if len(ids) != 0 {
		t.Fatalf("running job started before window was kept: %v", ids)
	}
}

func TestJobs_FilterRejected400(t *testing.T) {
	bad := jobsServer(t, 400, `{"message":"bad q"}`, nil)
	defer bad.Close()
	good := jobsServer(t, 200, `{"count":0,"jobs":[]}`, nil)
	defer good.Close()

	mk := func(name string, srv *httptest.Server) system {
		return system{name: name, client: nsrclient.New(nsrclient.Options{Name: name, Host: srv.URL, Username: "u", Password: "p"})}
	}
	jc := JobsCollector{Window: 24 * time.Hour, Now: windowNowFn}
	badSys, goodSys := mk("bad", bad), mk("good", good)
	if _, err := jc.Collect(context.Background(), badSys.client); err == nil {
		t.Fatal("Collect returned nil error on HTTP 400")
	}

	c, store := windowCollector(good, 24*time.Hour)
	c.systems = []system{badSys, goodSys}
	c.CollectOnce(context.Background())

	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	got := map[string]float64{}
	for _, f := range fams {
		if f.GetName() != "nsr_up" {
			continue
		}
		for _, m := range f.Metric {
			for _, l := range m.Label {
				if l.GetName() == "system" {
					got[l.GetValue()] = m.GetGauge().GetValue()
				}
			}
		}
	}
	if got["bad"] != 0 || got["good"] != 1 {
		t.Fatalf("nsr_up = %v, want bad=0 good=1", got)
	}
}
