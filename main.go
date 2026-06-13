// Command nsr_exporter is a Prometheus + OTLP exporter for Dell EMC NetWorker.
//
// It runs one background loop that polls every configured NetWorker system on an
// interval, publishes an immutable snapshot, and serves /metrics (and OTLP push)
// from that snapshot so scrapes never reach the backend.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/fjacquet/nsr_exporter/internal/config"
	"github.com/fjacquet/nsr_exporter/internal/logging"
	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsr"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var (
		cfgPath string
		debug   bool
		once    bool
		trace   bool
	)
	root := &cobra.Command{
		Use:     "nsr_exporter",
		Short:   "Prometheus + OTLP exporter for Dell EMC NetWorker",
		Version: version,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(cfgPath, debug, once, trace)
		},
	}
	root.Flags().StringVar(&cfgPath, "config", "config.yaml", "path to config.yaml")
	root.Flags().BoolVar(&debug, "debug", false, "enable debug logging")
	root.Flags().BoolVar(&once, "once", false, "run a single collection cycle and exit")
	root.Flags().BoolVar(&trace, "trace", false, "log each API response (method/path/status/body) — never headers")

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(cfgPath string, debug, once, trace bool) error {
	config.LoadDotEnv(cfgPath)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	log, err := logging.Setup(cfg.Server.LogName, debug || trace)
	if err != nil {
		return err
	}

	store := nsr.NewSnapshotStore()
	collector := nsr.NewCollector(cfg, store, log, trace)

	// --once: a single cycle, optional sample dump for live validation, then exit.
	if once {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Collection.Timeout+10*time.Second)
		defer cancel()
		collector.CollectOnce(ctx)
		if debug {
			dumpSamples(store.Load().Samples)
		}
		return nil
	}

	// Register the Prometheus export path.
	reg := prometheus.NewRegistry()
	reg.MustRegister(nsr.NewPromCollector(store))

	// OTLP export path (push) only when an endpoint is configured.
	otlpShutdown := setupOTLP(store, log)
	defer otlpShutdown()

	// Serve HTTP BEFORE the first collection cycle: the first poll can exceed the
	// collection timeout and must not stall /metrics or /health (ADR: serve first).
	srv := newServer(cfg, store, reg)
	go func() {
		log.WithField("addr", srv.Addr).Info("serving metrics")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("http server failed")
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go collector.Run(ctx)
	watchReload(ctx, cfgPath, log)

	<-ctx.Done()
	log.Info("shutting down")
	shctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shctx)
}

func newServer(cfg *config.Config, store *nsr.SnapshotStore, reg *prometheus.Registry) *http.Server {
	mux := http.NewServeMux()
	mux.Handle(cfg.Server.URI, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		// Healthy once we have ever published a snapshot.
		if store.Load().Collected.IsZero() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("starting"))
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	return &http.Server{
		Addr:              cfg.Server.Host + ":" + cfg.Server.Port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

// setupOTLP wires the OTLP push path when OTEL_EXPORTER_OTLP_ENDPOINT is set;
// otherwise it is a no-op so the exporter runs Prometheus-only out of the box.
func setupOTLP(store *nsr.SnapshotStore, log *logrus.Logger) func() {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func() {}
	}
	ctx := context.Background()
	exp, err := otlpGRPCExporter(ctx)
	if err != nil {
		log.WithError(err).Warn("OTLP endpoint set but exporter init failed; continuing Prometheus-only")
		return func() {}
	}
	reader := sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(30*time.Second))
	otlp, err := nsr.NewOTLPExporter(store, reader)
	if err != nil {
		log.WithError(err).Warn("OTLP exporter wiring failed; continuing Prometheus-only")
		return func() {}
	}
	log.Info("OTLP push export enabled")
	return func() { _ = otlp.Shutdown(context.Background()) }
}

// watchReload installs SIGHUP-triggered config reload. A full file-watch is a
// follow-up; SIGHUP covers the operational reload path today (ADR-0005).
func watchReload(ctx context.Context, cfgPath string, log *logrus.Logger) {
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-hup:
				if _, err := config.Load(cfgPath); err != nil {
					log.WithError(err).Warn("SIGHUP reload: config invalid, keeping running config")
					continue
				}
				log.Info("SIGHUP reload: config is valid (live swap is a follow-up)")
			}
		}
	}()
}

// dumpSamples prints every sample in exposition-like form, sorted, for diffing
// against docs/metrics.md during live validation (--once --debug).
func dumpSamples(samples []models.Sample) {
	lines := make([]string, 0, len(samples))
	for _, s := range samples {
		labels := append([]models.Label(nil), s.Labels...)
		sort.Slice(labels, func(i, j int) bool { return labels[i].Key < labels[j].Key })
		parts := make([]string, len(labels))
		for i, l := range labels {
			parts[i] = fmt.Sprintf("%s=%q", l.Key, l.Value)
		}
		lines = append(lines, fmt.Sprintf("%s{%s} %g", s.Name, strings.Join(parts, ","), s.Value))
	}
	sort.Strings(lines)
	for _, ln := range lines {
		fmt.Println(ln)
	}
}
