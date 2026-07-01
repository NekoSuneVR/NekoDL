package httpengine

import (
	"sync"
	"time"
)

// rateTracker computes a rough bytes/sec figure from periodic samples of a
// cumulative byte counter. It intentionally doesn't try to be more precise
// than that — Progress() just needs something reasonable to show a user.
type rateTracker struct {
	mu        sync.Mutex
	lastTime  time.Time
	lastBytes int64
	rate      int64
}

func (r *rateTracker) sample(totalBytes int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if !r.lastTime.IsZero() {
		if elapsed := now.Sub(r.lastTime).Seconds(); elapsed >= 0.2 {
			r.rate = int64(float64(totalBytes-r.lastBytes) / elapsed)
			r.lastTime = now
			r.lastBytes = totalBytes
		}
		return
	}
	r.lastTime = now
	r.lastBytes = totalBytes
}

func (r *rateTracker) Rate() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rate
}
