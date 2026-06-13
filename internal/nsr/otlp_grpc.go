package main

import (
	"context"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// otlpGRPCExporter builds the production OTLP/gRPC metric exporter. Endpoint and
// TLS are configured via the standard OTEL_EXPORTER_OTLP_* environment variables.
func otlpGRPCExporter(ctx context.Context) (sdkmetric.Exporter, error) {
	return otlpmetricgrpc.New(ctx)
}
