package ytdlpengine

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCheckForUpdateAgainstRealBinary(t *testing.T) {
	binary := findRealYtDlp(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	output, err := CheckForUpdate(ctx, binary)
	if err != nil {
		t.Fatalf("CheckForUpdate: %v (output: %s)", err, output)
	}
	if !strings.Contains(strings.ToLower(output), "yt-dlp is up to date") && !strings.Contains(strings.ToLower(output), "updated") {
		t.Fatalf("unexpected update output: %s", output)
	}
}

// TestRunPeriodicUpdateCheckStopsWithContext exercises the ticker/cancel
// loop itself with a fake, instant checkFn — not a real "yt-dlp -U" call.
// That real call was observed live to take anywhere from ~1s to well over
// a minute depending on GitHub API conditions on the day, which made an
// earlier version of this test (calling the real binary through
// RunPeriodicUpdateCheck directly) flaky for reasons entirely unrelated to
// whether this loop's logic is correct. See TestCheckForUpdateAgainstRealBinary
// for the real-binary coverage.
func TestRunPeriodicUpdateCheckStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan struct{}, 1)

	done := make(chan struct{})
	go func() {
		runPeriodicCheck(ctx, 5*time.Millisecond, func(string, error) {
			select {
			case results <- struct{}{}:
			default:
			}
		}, func(context.Context) (string, error) {
			return "fake output", nil
		})
		close(done)
	}()

	select {
	case <-results:
	case <-time.After(2 * time.Second):
		t.Fatal("expected at least one update check to run")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("expected RunPeriodicUpdateCheck to return promptly after ctx cancellation")
	}
}

// TestRunPeriodicUpdateCheckWiresRealCheckForUpdate confirms
// RunPeriodicUpdateCheck's public entry point actually calls the real
// CheckForUpdate (not just runPeriodicCheck's own already-tested loop
// logic) — one real, generously-timed network call, not a tight bound.
func TestRunPeriodicUpdateCheckWiresRealCheckForUpdate(t *testing.T) {
	binary := findRealYtDlp(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan struct{}, 1)

	go RunPeriodicUpdateCheck(ctx, binary, 10*time.Millisecond, func(string, error) {
		select {
		case results <- struct{}{}:
		default:
		}
	})

	select {
	case <-results:
	case <-time.After(90 * time.Second):
		t.Fatal("expected at least one real update check to complete")
	}
}
