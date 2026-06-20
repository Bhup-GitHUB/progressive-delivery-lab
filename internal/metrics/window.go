package metrics

import (
	"sort"
	"sync"
	"time"
)

type Snapshot struct {
	Requests  int     `json:"requests"`
	Errors    int     `json:"errors"`
	ErrorRate float64 `json:"errorRate"`
	P99MS     int64   `json:"p99LatencyMs"`
}

type Window struct {
	mu        sync.Mutex
	limit     int
	requests  int
	errors    int
	latencies []time.Duration
}

func NewWindow(limit int) *Window {
	if limit <= 0 {
		limit = 1000
	}
	return &Window{limit: limit}
}

func (w *Window) Record(latency time.Duration, failed bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.requests++
	if failed {
		w.errors++
	}

	w.latencies = append(w.latencies, latency)
	if len(w.latencies) > w.limit {
		copy(w.latencies, w.latencies[len(w.latencies)-w.limit:])
		w.latencies = w.latencies[:w.limit]
	}
}

func (w *Window) Snapshot() Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()

	snapshot := Snapshot{
		Requests: w.requests,
		Errors:   w.errors,
	}
	if w.requests > 0 {
		snapshot.ErrorRate = float64(w.errors) / float64(w.requests)
	}
	if len(w.latencies) > 0 {
		values := append([]time.Duration(nil), w.latencies...)
		sort.Slice(values, func(i, j int) bool {
			return values[i] < values[j]
		})
		index := int(float64(len(values))*0.99) - 1
		if index < 0 {
			index = 0
		}
		if index >= len(values) {
			index = len(values) - 1
		}
		snapshot.P99MS = values[index].Milliseconds()
	}
	return snapshot
}
