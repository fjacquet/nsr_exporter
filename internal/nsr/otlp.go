package nsr

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/fjacquet/nsr_exporter/internal/models"
)

// OTLPExporter is the second export path. It pre-registers one observable
// instrument per catalog metric and a single callback that, on each metric read,
// loads the latest snapshot and observes every Sample — reading the same immutable
// snapshot the Prometheus path does, never the backend.
type OTLPExporter struct {
	provider *sdkmetric.MeterProvider
}

// NewOTLPExporter wires the OTLP path over the snapshot store. The reader is
// injectable: a PeriodicReader (with the otlpmetricgrpc exporter) in production, a
// ManualReader in tests.
func NewOTLPExporter(store *SnapshotStore, reader sdkmetric.Reader) (*OTLPExporter, error) {
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("github.com/fjacquet/nsr_exporter")

	instByName := make(map[string]metric.Float64Observable, len(Catalog()))
	observables := make([]metric.Observable, 0, len(Catalog()))
	for _, m := range Catalog() {
		inst, err := newObservable(meter, m)
		if err != nil {
			return nil, err
		}
		instByName[m.Name] = inst
		observables = append(observables, inst)
	}

	_, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		snap := store.Load()
		for _, s := range snap.Samples {
			inst, ok := instByName[s.Name]
			if !ok {
				continue // not in catalog → skip (the drift test catches this)
			}
			o.ObserveFloat64(inst, s.Value, metric.WithAttributes(toAttributes(s.Labels)...))
		}
		return nil
	}, observables...)
	if err != nil {
		return nil, err
	}
	return &OTLPExporter{provider: provider}, nil
}

// Shutdown flushes and stops the meter provider.
func (e *OTLPExporter) Shutdown(ctx context.Context) error {
	return e.provider.Shutdown(ctx)
}

func newObservable(meter metric.Meter, m MetricMeta) (metric.Float64Observable, error) {
	if m.Type == models.Counter {
		return meter.Float64ObservableCounter(m.Name, metric.WithDescription(m.Help))
	}
	return meter.Float64ObservableGauge(m.Name, metric.WithDescription(m.Help))
}

func toAttributes(labels []models.Label) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, len(labels))
	for i, l := range labels {
		attrs[i] = attribute.String(l.Key, l.Value)
	}
	return attrs
}
