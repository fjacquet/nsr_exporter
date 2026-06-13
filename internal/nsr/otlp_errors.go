package nsr

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// NewOTLPErrorCounter registers and returns the nsr_otlp_export_errors_total counter
// on the given Prometheus registry. The counter is incremented each time an OTLP push
// fails, making OTLP outages alertable via Prometheus.
func NewOTLPErrorCounter(reg prometheus.Registerer) prometheus.Counter {
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "nsr_otlp_export_errors_total",
		Help: "Total number of OTLP metric push failures.",
	})
	reg.MustRegister(c)
	return c
}

// CountingExporter wraps an sdkmetric.Exporter and increments the error counter
// whenever the inner exporter returns a non-nil error. This surfaces OTLP push
// failures as a Prometheus metric without relying on the SDK's internal error handlers.
type CountingExporter struct {
	inner  sdkmetric.Exporter
	errors prometheus.Counter
}

// NewCountingExporter wraps exp to increment errCounter on every export error.
func NewCountingExporter(exp sdkmetric.Exporter, errCounter prometheus.Counter) *CountingExporter {
	return &CountingExporter{inner: exp, errors: errCounter}
}

// Temporality delegates to the inner exporter.
func (c *CountingExporter) Temporality(k sdkmetric.InstrumentKind) metricdata.Temporality {
	return c.inner.Temporality(k)
}

// Aggregation delegates to the inner exporter.
func (c *CountingExporter) Aggregation(k sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return c.inner.Aggregation(k)
}

// Export delegates to the inner exporter and increments the error counter on failure.
func (c *CountingExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	err := c.inner.Export(ctx, rm)
	if err != nil {
		c.errors.Inc()
	}
	return err
}

// ForceFlush delegates to the inner exporter.
func (c *CountingExporter) ForceFlush(ctx context.Context) error {
	return c.inner.ForceFlush(ctx)
}

// Shutdown delegates to the inner exporter.
func (c *CountingExporter) Shutdown(ctx context.Context) error {
	return c.inner.Shutdown(ctx)
}
