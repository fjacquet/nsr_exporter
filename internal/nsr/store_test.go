package nsr

import (
	"sync"
	"testing"

	"github.com/fjacquet/nsr_exporter/internal/models"
)

func TestSnapshotStore_SeededEmpty(t *testing.T) {
	s := NewSnapshotStore()
	if got := s.Load(); got == nil {
		t.Fatal("Load() before first Swap returned nil; store must seed an empty snapshot")
	}
	if n := len(s.Load().Samples); n != 0 {
		t.Fatalf("seeded snapshot should have 0 samples, got %d", n)
	}
}

func TestSnapshotStore_Swap(t *testing.T) {
	s := NewSnapshotStore()
	want := &models.Snapshot{Samples: []models.Sample{{Name: "nsr_alerts_total", Value: 3}}}
	s.Swap(want)
	if got := s.Load(); got != want {
		t.Fatalf("Load() = %p, want the swapped-in pointer %p", got, want)
	}
	if got := s.Load().Samples[0].Value; got != 3 {
		t.Fatalf("sample value = %v, want 3", got)
	}
}

// TestSnapshotStore_ConcurrentReadWrite exercises the RWMutex under the race
// detector: a writer swapping while many readers load must never tear.
func TestSnapshotStore_ConcurrentReadWrite(t *testing.T) {
	s := NewSnapshotStore()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(v float64) {
			defer wg.Done()
			s.Swap(&models.Snapshot{Samples: []models.Sample{{Name: "x", Value: v}}})
		}(float64(i))
		go func() {
			defer wg.Done()
			_ = s.Load().Samples
		}()
	}
	wg.Wait()
}
