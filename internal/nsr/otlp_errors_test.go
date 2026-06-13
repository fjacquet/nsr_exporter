package nsr

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// errExporter is a sdkmetric.Exporter that always fails — used to force the error
// counter to increment in tests.
type errExporter struct{}

func (e *errExporter) Temporality(k sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (e *errExporter) Aggregation(k sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.AggregationDefault{}
}

func (e *errExporter) Export(_ context.Context, _ *metricdata.ResourceMetrics) error {
	return errors.New("simulated export failure")
}

func (e *errExporter) ForceFlush(_ context.Context) error { return nil }
func (e *errExporter) Shutdown(_ context.Context) error   { return nil }

// TestOTLPExportErrorsTotal verifies the counter is registered and increments on
// export failure.
func TestOTLPExportErrorsTotal(t *testing.T) {
	reg := prometheus.NewRegistry()
	counter := NewOTLPErrorCounter(reg)
	if counter == nil {
		t.Fatal("NewOTLPErrorCounter returned nil")
	}

	// Verify the counter is registered by gathering.
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if !containsFamily(fams, "nsr_otlp_export_errors_total") {
		t.Fatal("nsr_otlp_export_errors_total not registered on prometheus registry")
	}

	// Force an increment and verify.
	counter.Inc()
	fams, err = reg.Gather()
	if err != nil {
		t.Fatalf("gather after Inc: %v", err)
	}
	if v := familyCounterValue(fams, "nsr_otlp_export_errors_total"); v != 1 {
		t.Fatalf("nsr_otlp_export_errors_total = %v, want 1", v)
	}
}

// TestCountingExporter verifies that CountingExporter increments the counter
// when the underlying exporter returns an error.
func TestCountingExporter(t *testing.T) {
	reg := prometheus.NewRegistry()
	counter := NewOTLPErrorCounter(reg)

	exp := &CountingExporter{inner: &errExporter{}, errors: counter}
	err := exp.Export(context.Background(), &metricdata.ResourceMetrics{})
	if err == nil {
		t.Fatal("expected error from errExporter, got nil")
	}

	fams, err2 := reg.Gather()
	if err2 != nil {
		t.Fatalf("gather: %v", err2)
	}
	if v := familyCounterValue(fams, "nsr_otlp_export_errors_total"); v != 1 {
		t.Fatalf("nsr_otlp_export_errors_total = %v after export error, want 1", v)
	}
}

// --- helpers ---

func containsFamily(fams []*dto.MetricFamily, name string) bool {
	for _, f := range fams {
		if f.GetName() == name {
			return true
		}
	}
	return false
}

func familyCounterValue(fams []*dto.MetricFamily, name string) float64 {
	for _, f := range fams {
		if f.GetName() != name {
			continue
		}
		if len(f.Metric) == 0 {
			return 0
		}
		m := f.Metric[0]
		if m.Counter != nil {
			return m.GetCounter().GetValue()
		}
		if m.Gauge != nil {
			return m.GetGauge().GetValue()
		}
	}
	return -1
}
