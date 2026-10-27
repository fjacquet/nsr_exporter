package nsr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// helpStub emits one sample whose help text is whatever the backend answers, so
// each system can be made to disagree with the other.
type helpStub struct{}

func (helpStub) Name() string { return "help" }
func (helpStub) Collect(ctx context.Context, c *nsrclient.Client) ([]models.Sample, error) {
	var out struct {
		Help string `json:"help"`
	}
	if err := c.Get(ctx, "/x", nsrclient.QueryOpts{}, &out); err != nil {
		return nil, err
	}
	return []models.Sample{{Name: "nsr_session_active", Help: out.Help, Type: models.Gauge, Value: 1}}, nil
}

func helpServer(help string, delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(delay)
		w.Header().Set("Content-Type", "application/json")
		writeBody(w, `{"help":"`+help+`"}`)
	}))
}

// The first system finishes last; config order must still decide which sample wins.
func TestCollectOnce_CrossSystemMergeIsConfigOrder(t *testing.T) {
	a, b := helpServer("A", 20*time.Millisecond), helpServer("B", 0)
	defer a.Close()
	defer b.Close()
	mk := func(name string, srv *httptest.Server) system {
		return system{name: name, client: nsrclient.New(nsrclient.Options{Name: name, Host: srv.URL, Username: "u", Password: "p"})}
	}
	store := NewSnapshotStore()
	c := &Collector{
		systems:    []system{mk("sysa", a), mk("sysb", b)},
		collectors: []ResourceCollector{helpStub{}},
		store:      store, timeout: 5 * time.Second, log: testLogger(), now: nowZero,
	}
	for i := 0; i < 50; i++ {
		c.CollectOnce(context.Background())
		out := store.Load().Samples
		for _, s := range out {
			if s.Name == "nsr_session_active" && s.Help != "A" {
				t.Fatalf("run %d: surviving help = %q, want A", i, s.Help)
			}
		}
		if guardCount(out, "sysa", "help_mismatch") != 0 || guardCount(out, "sysb", "help_mismatch") != 1 {
			t.Fatalf("run %d: help_mismatch attributed to wrong system", i)
		}
	}
}
