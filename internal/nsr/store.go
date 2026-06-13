package nsr

import (
	"sync"

	"github.com/fjacquet/nsr_exporter/internal/models"
)

// SnapshotStore holds the most recent immutable Snapshot behind an RWMutex and
// serves it to both export paths via a cheap pointer-swap. Reads (every /metrics
// scrape and OTLP push) never block the collection loop for more than the swap,
// and the backend is never touched on scrape. See ADR-0001.
type SnapshotStore struct {
	mu      sync.RWMutex
	current *models.Snapshot
}

// NewSnapshotStore returns a store seeded with an empty snapshot so /metrics and
// /health respond sanely before the first collection cycle completes (ADR: serve
// HTTP before first collect).
func NewSnapshotStore() *SnapshotStore {
	return &SnapshotStore{current: &models.Snapshot{}}
}

// Swap atomically replaces the current snapshot. The collection loop calls this
// once per cycle with a freshly built, immutable snapshot.
func (s *SnapshotStore) Swap(snap *models.Snapshot) {
	s.mu.Lock()
	s.current = snap
	s.mu.Unlock()
}

// Load returns the current snapshot pointer. Callers must treat it as read-only.
func (s *SnapshotStore) Load() *models.Snapshot {
	s.mu.RLock()
	snap := s.current
	s.mu.RUnlock()
	return snap
}
