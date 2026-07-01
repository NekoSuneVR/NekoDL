package boothengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NekoSuneVR/NekoDL/core/internal/task"
)

func TestNewTaskValidation(t *testing.T) {
	if _, err := NewTask(Options{DestDir: "/tmp/x", BinaryPath: "booth"}); err == nil {
		t.Fatal("expected an error when Input is empty")
	}
	if _, err := NewTask(Options{Input: "3807513", BinaryPath: "booth"}); err == nil {
		t.Fatal("expected an error when DestDir is empty")
	}
	if _, err := NewTask(Options{Input: "3807513", DestDir: "/tmp/x"}); err == nil {
		t.Fatal("expected an error when BinaryPath is empty (no default guess for this one)")
	}
}

func TestBuildArgsDefaults(t *testing.T) {
	tk, err := NewTask(Options{Input: "3807513", DestDir: "/tmp/x", BinaryPath: "booth"})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	args := tk.buildArgs("/tmp/x/BDConfig.json", "/tmp/x")
	want := []string{"--config", "/tmp/x/BDConfig.json", "--booth", "3807513", "--output-dir", "/tmp/x", "--max-retries", "3"}
	if len(args) != len(want) {
		t.Fatalf("got args %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("arg %d: got %q, want %q (full: %v)", i, args[i], want[i], args)
		}
	}
}

// findRealBoothDownloader locates a real BoothDownloader binary via an
// env var pointing at a real downloaded release, or skips — mirroring
// ytdlpengine's findRealYtDlp: a genuine live check where the tool is
// available (it was, when this was written, via a real downloaded
// release binary), not a permanent CI requirement everywhere else, since
// BoothDownloader has no package-manager install path to rely on.
func findRealBoothDownloader(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("NEKODL_TEST_BOOTHDOWNLOADER_BIN"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("BoothDownloader binary not available — set NEKODL_TEST_BOOTHDOWNLOADER_BIN to enable this test")
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
	t.Fatalf("timed out waiting for status %s, got %s (err=%v, warning=%q)", want, tk.Status(), tk.Err(), tk.Warning())
}

// TestRealAnonymousDownload runs the actual BoothDownloader binary against
// a real, small, free, long-standing public Booth item (its image gallery
// only — no purchase needed), anonymously (no cookie).
func TestRealAnonymousDownload(t *testing.T) {
	binary := findRealBoothDownloader(t)
	destDir := t.TempDir()

	tk, err := NewTask(Options{
		ID:         "real1",
		Input:      "3807513",
		DestDir:    destDir,
		BinaryPath: binary,
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	t.Cleanup(func() { tk.Cancel() })

	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusComplete, 60*time.Second)

	itemDir := filepath.Join(destDir, "3807513")
	entries, err := os.ReadDir(itemDir)
	if err != nil {
		t.Fatalf("expected item directory %s to exist: %v", itemDir, err)
	}
	if len(entries) == 0 {
		t.Fatal("expected real downloaded files, got an empty item directory")
	}
}

// TestRealWarningSurfacedForInvalidCookieCollection reproduces a real,
// confirmed BoothDownloader behavior: requesting a paid collection ("gifts")
// anonymously logs a real EROR line ("Cannot download Paid Items with
// invalid cookie.") but still exits 0. Confirms this wrapper surfaces that
// line via Warning() rather than silently reporting a clean success.
func TestRealWarningSurfacedForInvalidCookieCollection(t *testing.T) {
	binary := findRealBoothDownloader(t)
	destDir := t.TempDir()

	tk, err := NewTask(Options{
		ID:         "real2",
		Input:      "gifts",
		DestDir:    destDir,
		BinaryPath: binary,
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	t.Cleanup(func() { tk.Cancel() })

	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusComplete, 60*time.Second)

	if tk.Warning() == "" {
		t.Fatal("expected a non-empty warning for an anonymous request to a paid-only collection")
	}
}

func TestRealPauseKillsProcessAndResumeRestarts(t *testing.T) {
	binary := findRealBoothDownloader(t)
	destDir := t.TempDir()

	tk, err := NewTask(Options{
		ID:         "real3",
		Input:      "3807513",
		DestDir:    destDir,
		BinaryPath: binary,
		MaxRetries: 1,
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
		// A small real download can finish before Pause() lands — either
		// outcome means Pause() didn't leave the task stuck mid-transition.
		t.Fatalf("expected StatusPaused or StatusComplete, got %s", got)
	}

	if err := tk.Resume(); err != nil {
		t.Fatalf("second Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusComplete, 60*time.Second)
}

func TestRealCookieNeverAppearsInConfigAsCLIArg(t *testing.T) {
	tk, err := NewTask(Options{
		Input:      "3807513",
		DestDir:    t.TempDir(),
		BinaryPath: "booth",
		Cookie:     "super-secret-token",
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	args := tk.buildArgs("/tmp/x/BDConfig.json", "/tmp/x")
	for _, a := range args {
		if strings.Contains(a, "super-secret-token") {
			t.Fatalf("cookie leaked into a CLI argument: %v", args)
		}
	}
}
