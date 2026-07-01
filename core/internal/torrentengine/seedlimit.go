package torrentengine

import (
	"context"
	"time"

	"github.com/anacrolix/torrent"
)

// shouldStopSeeding is the seeding-limit decision, pulled out as a pure
// function so it's testable with synthetic stats rather than only via a
// full real seed/leech integration test.
func shouldStopSeeding(stats torrent.TorrentStats, ratioLimit float64, timeLimit time.Duration, completedAt time.Time, now time.Time) bool {
	if ratioLimit > 0 && stats.BytesReadData.Int64() > 0 {
		ratio := float64(stats.BytesWrittenData.Int64()) / float64(stats.BytesReadData.Int64())
		if ratio >= ratioLimit {
			return true
		}
	}
	if timeLimit > 0 && !completedAt.IsZero() && now.Sub(completedAt) >= timeLimit {
		return true
	}
	return false
}

// runSeedLimiter periodically checks the configured ratio/time limits and
// stops uploading (without otherwise disturbing the task) once either is
// hit. A zero limit means "unlimited" for that dimension.
func (t *Task) runSeedLimiter(ctx context.Context, tor *torrent.Torrent) {
	if t.opts.SeedRatioLimit <= 0 && t.opts.SeedTimeLimit <= 0 {
		return
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		t.mu.Lock()
		completedAt := t.completedAt
		t.mu.Unlock()

		if shouldStopSeeding(tor.Stats(), t.opts.SeedRatioLimit, t.opts.SeedTimeLimit, completedAt, time.Now()) {
			tor.DisallowDataUpload()
			return
		}
	}
}
