package torrentengine

import (
	"testing"
	"time"

	"github.com/anacrolix/torrent"
)

// statsWithBytes builds a TorrentStats with just BytesWrittenData/BytesReadData
// set, using Count.Add since Count's fields are unexported.
func statsWithBytes(written, read int64) torrent.TorrentStats {
	var s torrent.TorrentStats
	s.BytesWrittenData.Add(written)
	s.BytesReadData.Add(read)
	return s
}

func TestShouldStopSeedingRatioLimit(t *testing.T) {
	now := time.Now()

	// 50 uploaded / 100 downloaded = 0.5 ratio, limit is 1.0 — not yet.
	if shouldStopSeeding(statsWithBytes(50, 100), 1.0, 0, time.Time{}, now) {
		t.Fatal("expected not to stop below the ratio limit")
	}
	// 150/100 = 1.5 ratio, limit is 1.0 — should stop.
	if !shouldStopSeeding(statsWithBytes(150, 100), 1.0, 0, time.Time{}, now) {
		t.Fatal("expected to stop once the ratio limit is reached")
	}
	// Exactly at the limit should also stop (>=).
	if !shouldStopSeeding(statsWithBytes(100, 100), 1.0, 0, time.Time{}, now) {
		t.Fatal("expected to stop exactly at the ratio limit")
	}
}

func TestShouldStopSeedingIgnoresRatioBeforeAnyDownload(t *testing.T) {
	now := time.Now()
	// BytesReadData == 0 would divide by zero — must not spuriously trigger.
	if shouldStopSeeding(statsWithBytes(500, 0), 1.0, 0, time.Time{}, now) {
		t.Fatal("expected no ratio-based stop when nothing has been downloaded yet")
	}
}

func TestShouldStopSeedingTimeLimit(t *testing.T) {
	completedAt := time.Now().Add(-2 * time.Hour)

	if shouldStopSeeding(statsWithBytes(0, 0), 0, 3*time.Hour, completedAt, time.Now()) {
		t.Fatal("expected not to stop before the time limit elapses")
	}
	if !shouldStopSeeding(statsWithBytes(0, 0), 0, 1*time.Hour, completedAt, time.Now()) {
		t.Fatal("expected to stop once the time limit has elapsed")
	}
}

func TestShouldStopSeedingTimeLimitIgnoredBeforeCompletion(t *testing.T) {
	// completedAt is the zero value — task hasn't finished downloading yet.
	if shouldStopSeeding(statsWithBytes(0, 0), 0, time.Nanosecond, time.Time{}, time.Now()) {
		t.Fatal("expected no time-based stop before completedAt is set")
	}
}

func TestShouldStopSeedingUnlimited(t *testing.T) {
	now := time.Now()
	if shouldStopSeeding(statsWithBytes(1_000_000, 1), 0, 0, now.Add(-time.Hour), now) {
		t.Fatal("expected no stop when both limits are zero (unlimited)")
	}
}
