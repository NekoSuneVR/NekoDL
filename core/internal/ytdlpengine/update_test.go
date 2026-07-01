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

func TestRunPeriodicUpdateCheckStopsWithContext(t *testing.T) {
	binary := findRealYtDlp(t)

	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan struct{}, 1)

	done := make(chan struct{})
	go func() {
		RunPeriodicUpdateCheck(ctx, binary, 50*time.Millisecond, func(string, error) {
			select {
			case results <- struct{}{}:
			default:
			}
		})
		close(done)
	}()

	select {
	case <-results:
	case <-time.After(5 * time.Second):
		t.Fatal("expected at least one update check to run")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("expected RunPeriodicUpdateCheck to return promptly after ctx cancellation")
	}
}
