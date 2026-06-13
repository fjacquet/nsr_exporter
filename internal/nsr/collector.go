package nsr

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/fjacquet/nsr_exporter/internal/config"
	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsrclient"
)

// maxConcurrentSystems caps how many NetWorker systems are polled in parallel per
// cycle so a large fleet can't exhaust connections or memory.
const maxConcurrentSystems = 8

// system pairs a configured target with its client.
type system struct {
	name   string
	client *nsrclient.Client
}

// Collector runs the background collection loop and publishes snapshots.
type Collector struct {
	systems    []system
	collectors []ResourceCollector
	store      *SnapshotStore
	interval   time.Duration
	timeout    time.Duration
	log        *logrus.Logger
	now        func() time.Time // injectable for tests
}

// NewCollector wires a Collector from config. The default collector set covers the
// five spec domains plus live sessions.
func NewCollector(cfg *config.Config, store *SnapshotStore, log *logrus.Logger, trace bool) *Collector {
	systems := make([]system, 0, len(cfg.Systems))
	for _, s := range cfg.Systems {
		client := nsrclient.New(nsrclient.Options{
			Name:               s.Name,
			Host:               s.Host,
			Username:           s.Username,
			Password:           s.Password,
			InsecureSkipVerify: s.InsecureSkipVerify,
			Timeout:            cfg.Collection.Timeout,
			Trace:              trace,
			Log:                log,
		})
		systems = append(systems, system{name: s.Name, client: client})
	}
	collectors := DefaultCollectors()
	// SizingCollector is stateful (lookback window + clock), so it is appended here
	// rather than in the stateless DefaultCollectors set.
	collectors = append(collectors, SizingCollector{Window: cfg.Collection.BackupWindow, Now: time.Now})

	return &Collector{
		systems:    systems,
		collectors: collectors,
		store:      store,
		interval:   cfg.Collection.Interval,
		timeout:    cfg.Collection.Timeout,
		log:        log,
		now:        time.Now,
	}
}

// DefaultCollectors is the canonical set of stateless resource collectors. The
// stateful SizingCollector is added separately by NewCollector.
func DefaultCollectors() []ResourceCollector {
	return []ResourceCollector{
		AlertsCollector{},
		ClientsCollector{},
		JobsCollector{},
		SessionsCollector{},
		StorageCollector{},
		DevicesCollector{},
		StorageNodesCollector{},
		PoolsCollector{},
		VMwareCollector{},
		QueuesCollector{},
		PoliciesCollector{},
	}
}

// Run drives the ticker loop until ctx is cancelled. It collects once immediately
// so the first snapshot is fresh, then on every interval tick.
func (c *Collector) Run(ctx context.Context) {
	c.CollectOnce(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.CollectOnce(ctx)
		}
	}
}

// CollectOnce runs one full collection cycle across all systems and publishes the
// resulting immutable snapshot. A per-system failure degrades gracefully: that
// system's nsr_up is 0 and the cycle continues (architecture.md).
func (c *Collector) CollectOnce(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var mu sync.Mutex
	all := make([]models.Sample, 0, 256)

	g, gctx := errgroup.WithContext(cctx)
	g.SetLimit(maxConcurrentSystems)
	for _, sys := range c.systems {
		sys := sys
		g.Go(func() error {
			samples, healthy := c.collectSystem(gctx, sys)
			up := 0.0
			if healthy {
				up = 1.0
			}
			samples = append(samples, models.Sample{
				Name: "nsr_up", Help: "1 if the system was reachable this cycle, else 0.",
				Type: models.Gauge, Value: up,
			})
			tagged := make([]models.Sample, len(samples))
			for i, s := range samples {
				tagged[i] = withSystem(s, sys.name)
			}
			mu.Lock()
			all = append(all, tagged...)
			mu.Unlock()
			return nil // never fail the group; degradation is per-system
		})
	}
	_ = g.Wait()

	c.store.Swap(&models.Snapshot{Samples: all, Collected: c.now()})
}

// collectSystem runs every resource collector against one system. It returns the
// samples gathered and whether the system was fully healthy (no collector errored).
func (c *Collector) collectSystem(ctx context.Context, sys system) (samples []models.Sample, healthy bool) {
	healthy = true
	for _, rc := range c.collectors {
		out, err := rc.Collect(ctx, sys.client)
		if err != nil {
			healthy = false
			c.log.WithFields(logrus.Fields{
				"system": sys.name, "collector": rc.Name(), "error": err,
			}).Warn("collector failed; degrading this system")
			continue
		}
		samples = append(samples, out...)
	}
	return samples, healthy
}
