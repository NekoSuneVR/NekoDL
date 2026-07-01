package ytdlpengine

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// CheckForUpdate runs yt-dlp's own self-update (-U) once. It's meant to be
// called from a periodic background goroutine (see RunPeriodicUpdateCheck),
// never from inside an active download — updating yt-dlp mid-download could
// replace the binary a running subprocess is using.
func CheckForUpdate(ctx context.Context, binaryPath string) (output string, err error) {
	if binaryPath == "" {
		binaryPath = "yt-dlp"
	}
	out, err := exec.CommandContext(ctx, binaryPath, "-U").CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("ytdlpengine: update check failed: %w", err)
	}
	return string(out), nil
}

// RunPeriodicUpdateCheck calls CheckForUpdate on a schedule until ctx is
// done, reporting each result via onResult (typically just a log line).
// It never runs concurrently with a download by design: it's independent
// of task lifecycles entirely, not synchronized with them, and yt-dlp's own
// -U replaces the binary file on disk atomically, so it doesn't corrupt a
// subprocess that's already running with the old binary loaded into memory —
// only subsequent invocations pick up the new version.
func RunPeriodicUpdateCheck(ctx context.Context, binaryPath string, interval time.Duration, onResult func(output string, err error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			output, err := CheckForUpdate(ctx, binaryPath)
			onResult(output, err)
		}
	}
}
