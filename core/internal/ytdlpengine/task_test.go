package ytdlpengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/NekoSuneVR/NekoDL/core/internal/task"
)

func TestNewTaskValidation(t *testing.T) {
	if _, err := NewTask(Options{DestDir: "/tmp/x"}); err == nil {
		t.Fatal("expected an error when URL is empty")
	}
	if _, err := NewTask(Options{URL: "https://example.com/x"}); err == nil {
		t.Fatal("expected an error when DestDir is empty")
	}
}

func TestBuildArgs(t *testing.T) {
	tk, err := NewTask(Options{
		URL:        "https://example.com/video",
		DestDir:    "/tmp/x",
		Format:     "best",
		NoPlaylist: true,
		ProxyAddr:  "127.0.0.1:9050",
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	args := tk.buildArgs()

	want := []string{"--newline", "--progress-template", "download:%(progress)j", "-f", "best", "--no-playlist", "-o", "%(title)s.%(ext)s", "--proxy", "127.0.0.1:9050", "https://example.com/video"}
	if len(args) != len(want) {
		t.Fatalf("got args %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("arg %d: got %q, want %q (full: %v)", i, args[i], want[i], args)
		}
	}
}

// findRealYtDlp locates a real yt-dlp binary on this machine, or skips the
// test if none is found — this is a genuine live check where the tool is
// available (which it was when this was written), not a permanent
// network-dependent CI requirement everywhere else.
func findRealYtDlp(t *testing.T) string {
	t.Helper()
	if path, err := exec.LookPath("yt-dlp"); err == nil {
		return path
	}
	t.Skip("yt-dlp not found on PATH — skipping live subprocess test")
	return ""
}

func waitForStatus(t *testing.T, tk *Task, want task.Status, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if tk.Status() == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for status %s, got %s (err=%v)", want, tk.Status(), tk.Err())
}

// TestRealDownload actually runs yt-dlp against a real, small, stable,
// long-standing public test video (the first video ever uploaded to
// YouTube — famously tiny and used in yt-dlp's own test suite too) and
// confirms a real file lands on disk with real progress reported along the way.
func TestRealDownload(t *testing.T) {
	binary := findRealYtDlp(t)
	destDir := t.TempDir()

	tk, err := NewTask(Options{
		ID:         "real1",
		URL:        "https://www.youtube.com/watch?v=jNQXAC9IVRw",
		DestDir:    destDir,
		BinaryPath: binary,
		Format:     "worst", // keep the real download tiny
		NoPlaylist: true,
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	t.Cleanup(func() { tk.Cancel() })

	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusComplete, 60*time.Second)

	progress := tk.Progress()
	if progress.DownloadedBytes == 0 {
		t.Fatal("expected DownloadedBytes > 0 after a real completed download")
	}
	if progress.TotalBytes == 0 {
		t.Fatal("expected TotalBytes to have been learned from real progress output")
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	found := false
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".part" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a real downloaded file in %s, got entries: %v", destDir, entries)
	}
}

func TestPauseKillsProcessAndResumeRestarts(t *testing.T) {
	binary := findRealYtDlp(t)
	destDir := t.TempDir()

	tk, err := NewTask(Options{
		ID:         "real2",
		URL:        "https://www.youtube.com/watch?v=jNQXAC9IVRw",
		DestDir:    destDir,
		BinaryPath: binary,
		Format:     "worst",
		NoPlaylist: true,
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	t.Cleanup(func() { tk.Cancel() })

	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if err := tk.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if got := tk.Status(); got != task.StatusPaused && got != task.StatusComplete {
		// The tiny test video can finish before Pause() lands — either
		// outcome means Pause() didn't leave the task stuck mid-transition.
		t.Fatalf("expected StatusPaused or StatusComplete, got %s", got)
	}

	if err := tk.Resume(); err != nil {
		t.Fatalf("second Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusComplete, 60*time.Second)
}

func TestFailsOnInvalidURL(t *testing.T) {
	binary := findRealYtDlp(t)
	tk, err := NewTask(Options{
		ID:         "real3",
		URL:        "https://example.com/definitely-not-a-real-video-nekodl-test",
		DestDir:    t.TempDir(),
		BinaryPath: binary,
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	t.Cleanup(func() { tk.Cancel() })

	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusError, 30*time.Second)

	if tk.Err() == nil {
		t.Fatal("expected Err() to report a reason")
	}
}
