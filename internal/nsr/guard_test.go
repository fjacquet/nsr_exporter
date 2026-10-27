package nsr

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/fjacquet/nsr_exporter/internal/models"
)

func gs(name, help string, t models.MetricType, v float64, labels ...models.Label) models.Sample {
	return models.Sample{Name: name, Help: help, Type: t, Value: v, Labels: labels}
}

// guardCount reads the self-metric for (system, reason); -1 if absent.
func guardCount(out []models.Sample, system, reason string) float64 {
	for _, s := range out {
		if s.Name != guardMetric {
			continue
		}
		var sys, rea string
		for _, l := range s.Labels {
			switch l.Key {
			case systemLabel:
				sys = l.Value
			case "reason":
				rea = l.Value
			}
		}
		if sys == system && rea == reason {
			return s.Value
		}
	}
	return -1
}

func countName(out []models.Sample, name string) int {
	n := 0
	for _, s := range out {
		if s.Name == name {
			n++
		}
	}
	return n
}

func TestGuard_DropsLaterDuplicate(t *testing.T) {
	in := []models.Sample{
		gs("m", "h", models.Gauge, 1, lbl("a", "1"), lbl("b", "2"), lbl("system", "a")),
		gs("m", "h", models.Gauge, 2, lbl("system", "a"), lbl("b", "2"), lbl("a", "1")),
	}
	out := guardSamples(in, []string{"a"}, testLogger())
	if countName(out, "m") != 1 || out[0].Value != 1 {
		t.Fatalf("want first kept, got %+v", out)
	}
	if got := guardCount(out, "a", "duplicate"); got != 1 {
		t.Fatalf("duplicate = %v", got)
	}
}

func TestGuard_SameSeriesDifferentSystemIsNotDuplicate(t *testing.T) {
	in := []models.Sample{
		gs("m", "h", models.Gauge, 1, lbl("system", "a")),
		gs("m", "h", models.Gauge, 1, lbl("system", "b")),
	}
	out := guardSamples(in, []string{"a", "b"}, testLogger())
	if countName(out, "m") != 2 || guardCount(out, "a", "duplicate") != 0 || guardCount(out, "b", "duplicate") != 0 {
		t.Fatalf("got %+v", out)
	}
}

func TestGuard_TypeMismatchDropsLater(t *testing.T) {
	in := []models.Sample{
		gs("m", "h", models.Gauge, 1, lbl("system", "a")),
		gs("m", "h", models.Counter, 2, lbl("x", "y"), lbl("system", "a")),
	}
	out := guardSamples(in, []string{"a"}, testLogger())
	if countName(out, "m") != 1 || guardCount(out, "a", "type_mismatch") != 1 {
		t.Fatalf("got %+v", out)
	}
}

func TestGuard_HelpMismatchRewritesHelp(t *testing.T) {
	in := []models.Sample{
		gs("m", "first", models.Gauge, 1, lbl("k", "1"), lbl("system", "a")),
		gs("m", "second", models.Gauge, 2, lbl("k", "2"), lbl("system", "a")),
	}
	out := guardSamples(in, []string{"a"}, testLogger())
	if countName(out, "m") != 2 || out[1].Help != "first" || guardCount(out, "a", "help_mismatch") != 1 {
		t.Fatalf("got %+v", out)
	}
	if in[1].Help != "second" {
		t.Fatal("input mutated")
	}
}

func TestGuard_InvalidLabelName(t *testing.T) {
	in := []models.Sample{
		gs("m", "h", models.Gauge, 1, lbl("bad-key", "v"), lbl("system", "a")),
		gs("m", "h", models.Gauge, 1, lbl("k", "1"), lbl("k", "2"), lbl("system", "a")),
		gs("m", "h", models.Gauge, 1, lbl("ok", "1"), lbl("system", "a")),
	}
	out := guardSamples(in, []string{"a"}, testLogger())
	if countName(out, "m") != 1 || guardCount(out, "a", "invalid_label") != 2 {
		t.Fatalf("got %+v", out)
	}
}

func TestGuard_NonUTF8LabelValue(t *testing.T) {
	in := []models.Sample{
		gs("m", "h", models.Gauge, 1, lbl("k", "bad\xff"), lbl("system", "a")),
		gs("m", "h", models.Gauge, 1, lbl("k", "ok"), lbl("system", "a")),
	}
	out := guardSamples(in, []string{"a"}, testLogger())
	if countName(out, "m") != 1 || guardCount(out, "a", "invalid_label") != 1 {
		t.Fatalf("got %+v", out)
	}
}

func TestGuard_LabelKeySetMismatchUntouched(t *testing.T) {
	in := []models.Sample{
		gs("m", "h", models.Gauge, 1, lbl("k", "1"), lbl("system", "a")),
		gs("m", "h", models.Gauge, 1, lbl("other", "1"), lbl("system", "a")),
	}
	out := guardSamples(in, []string{"a"}, testLogger())
	if countName(out, "m") != 2 {
		t.Fatalf("got %+v", out)
	}
	for _, r := range guardReasons {
		if guardCount(out, "a", string(r)) != 0 {
			t.Fatalf("reason %s nonzero", r)
		}
	}
}

func TestGuard_EmitsZeroForEveryReasonAndSystem(t *testing.T) {
	out := guardSamples(nil, []string{"a", "b"}, testLogger())
	for _, sys := range []string{"a", "b"} {
		for _, r := range guardReasons {
			if got := guardCount(out, sys, string(r)); got != 0 {
				t.Fatalf("%s/%s = %v, want 0", sys, r, got)
			}
		}
	}
}

func TestGuard_GatherSucceedsViaPromAndOTLP(t *testing.T) {
	sys := lbl("system", "a")
	in := []models.Sample{
		gs("good", "h", models.Gauge, 7, sys),
		gs("dup", "h", models.Gauge, 1, sys),
		gs("dup", "h", models.Gauge, 2, sys),
		gs("typ", "h", models.Gauge, 1, sys),
		gs("typ", "h", models.Counter, 1, lbl("x", "y"), sys),
		gs("hlp", "h1", models.Gauge, 1, lbl("k", "1"), sys),
		gs("hlp", "h2", models.Gauge, 1, lbl("k", "2"), sys),
		gs("lab", "h", models.Gauge, 1, lbl("bad-key", "v"), sys),
		gs("lab", "h", models.Gauge, 1, lbl("k", "1"), lbl("k", "2"), sys),
	}
	store := NewSnapshotStore()
	store.Swap(&models.Snapshot{Samples: guardSamples(in, []string{"a"}, testLogger())})

	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if got := familyValue(fams, "good"); got != 7 {
		t.Fatalf("good = %v", got)
	}
	if got := familyValue(fams, "dup"); got != 1 {
		t.Fatalf("dup = %v", got)
	}

	reader := sdkmetric.NewManualReader()
	if _, err := NewOTLPExporter(store, reader); err != nil {
		t.Fatalf("otlp: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("otlp collect: %v", err)
	}
	counts := map[string]float64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			g, ok := m.Data.(metricdata.Gauge[float64])
			if !ok || m.Name != guardMetric {
				continue
			}
			for _, dp := range g.DataPoints {
				r, _ := dp.Attributes.Value("reason")
				counts[r.AsString()] = dp.Value
			}
		}
	}
	want := map[string]float64{"duplicate": 1, "type_mismatch": 1, "help_mismatch": 1, "invalid_label": 2}
	for r, v := range want {
		if counts[r] != v {
			t.Fatalf("otlp %s = %v, want %v (all %v)", r, counts[r], v, counts)
		}
	}
}
