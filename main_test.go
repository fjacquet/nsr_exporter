package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/fjacquet/nsr_exporter/internal/config"
	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsr"
)

// testHandler builds the real mux newServer() wires, so these tests assert the
// routes are actually registered — not merely that the handler funcs behave.
func testHandler(t *testing.T, store *nsr.SnapshotStore) http.Handler {
	t.Helper()
	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = "9447"
	cfg.Server.URI = "/metrics"
	return newServer(cfg, store, prometheus.NewRegistry()).Handler
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// collectedStore returns a store that has published one snapshot, i.e. the
// post-first-cycle state.
func collectedStore() *nsr.SnapshotStore {
	s := nsr.NewSnapshotStore()
	s.Swap(&models.Snapshot{
		Samples:   []models.Sample{{Name: "nsr_alerts_total", Value: 3}},
		Collected: time.Now(),
	})
	return s
}

// TestLivezAlwaysOK: /livez reads no state, so it answers 200 both before and
// after the first collection cycle. A probe wired here can never be the reason a
// healthy process is restarted.
func TestLivezAlwaysOK(t *testing.T) {
	for name, store := range map[string]*nsr.SnapshotStore{
		"before first snapshot": nsr.NewSnapshotStore(),
		"after first snapshot":  collectedStore(),
	} {
		t.Run(name, func(t *testing.T) {
			rec := get(t, testHandler(t, store), "/livez")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Body.String(); got != "ok" {
				t.Fatalf("body = %q, want %q", got, "ok")
			}
		})
	}
}

// TestReadyzAlwaysOK mirrors TestLivezAlwaysOK: /readyz is the same state-free
// handler, so readiness never depends on backend reachability either.
func TestReadyzAlwaysOK(t *testing.T) {
	for name, store := range map[string]*nsr.SnapshotStore{
		"before first snapshot": nsr.NewSnapshotStore(),
		"after first snapshot":  collectedStore(),
	} {
		t.Run(name, func(t *testing.T) {
			rec := get(t, testHandler(t, store), "/readyz")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Body.String(); got != "ok" {
				t.Fatalf("body = %q, want %q", got, "ok")
			}
		})
	}
}

// TestHealthReturns200BeforeFirstSnapshot pins the behaviour change: /health used
// to answer 503 during the startup window. It now answers 200 and puts the
// startup state in the body instead.
func TestHealthReturns200BeforeFirstSnapshot(t *testing.T) {
	rec := get(t, testHandler(t, nsr.NewSnapshotStore()), "/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "starting" {
		t.Fatalf("body = %q, want %q", got, "starting")
	}
}

func TestHealthReturns200AfterFirstSnapshot(t *testing.T) {
	rec := get(t, testHandler(t, collectedStore()), "/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("body = %q, want %q", got, "ok")
	}
}
