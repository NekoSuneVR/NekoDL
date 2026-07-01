package torrentengine

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NekoSuneVR/NekoDL/core/internal/task"
)

func writeRandomFile(t *testing.T, path string, size int) []byte {
	t.Helper()
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return data
}

func waitForStatus(t *testing.T, tk *Task, want task.Status, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if tk.Status() == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for status %s, got %s (err=%v)", want, tk.Status(), tk.Err())
}

// TestDownloadOverLoopback is the flagship test: a real seeding
// torrent.Client serves a real (small) file over a real BitTorrent wire
// protocol connection on loopback, with no tracker or DHT — the two
// clients are introduced to each other directly via AddClientPeer, which
// the library documents as exactly this: a way to connect in-process test
// clients without needing a network-visible tracker.
func TestDownloadOverLoopback(t *testing.T) {
	seedDir := newSeedDir(t)
	sourceFile := filepath.Join(seedDir, "testfile.bin")
	want := writeRandomFile(t, sourceFile, 300*1024) // a few pieces at 64 KiB each

	seedClient, seedTorrent, mi := startSeeder(t, sourceFile)

	leechDir := t.TempDir()
	tk, err := NewTask(Options{
		ID:         "loopback1",
		MagnetURI:  mi.Magnet(nil, nil).String(),
		DestDir:    leechDir,
		DisableDHT: true,
		DisablePEX: true,
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	t.Cleanup(func() { tk.Cancel() })

	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	// Give the leecher's torrent a moment to exist, then introduce the seeder.
	deadline := time.Now().Add(5 * time.Second)
	for tk.tor == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if tk.tor == nil {
		t.Fatal("leecher torrent was never created")
	}
	n := tk.tor.AddClientPeer(seedClient)
	t.Logf("AddClientPeer added %d addr(s); seeder BytesCompleted=%d Length=%d", n, seedTorrent.BytesCompleted(), seedTorrent.Length())

	waitForStatus(t, tk, task.StatusComplete, 20*time.Second)
	t.Logf("leecher reached StatusComplete")

	got, err := os.ReadFile(filepath.Join(leechDir, "testfile.bin"))
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("downloaded content does not match the seeded file")
	}
}

func TestPauseAndResumeOverLoopback(t *testing.T) {
	seedDir := newSeedDir(t)
	sourceFile := filepath.Join(seedDir, "testfile.bin")
	want := writeRandomFile(t, sourceFile, 300*1024)

	seedClient, _, mi := startSeeder(t, sourceFile)

	leechDir := t.TempDir()
	tk, err := NewTask(Options{
		ID:         "loopback2",
		MagnetURI:  mi.Magnet(nil, nil).String(),
		DestDir:    leechDir,
		DisableDHT: true,
		DisablePEX: true,
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
	if got := tk.Status(); got != task.StatusPaused {
		t.Fatalf("expected StatusPaused, got %s", got)
	}

	if err := tk.Resume(); err != nil {
		t.Fatalf("second Resume: %v", err)
	}
	if tk.tor == nil {
		t.Fatal("torrent was torn down by Pause — it shouldn't be, only download should stop")
	}
	tk.tor.AddClientPeer(seedClient)

	waitForStatus(t, tk, task.StatusComplete, 20*time.Second)

	got, err := os.ReadFile(filepath.Join(leechDir, "testfile.bin"))
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("downloaded content does not match the seeded file")
	}
}

func TestNewTaskValidation(t *testing.T) {
	if _, err := NewTask(Options{DestDir: "/tmp/x"}); err == nil {
		t.Fatal("expected an error when neither MagnetURI nor TorrentBytes is set")
	}
	if _, err := NewTask(Options{MagnetURI: "magnet:?xt=x", TorrentBytes: []byte{1}, DestDir: "/tmp/x"}); err == nil {
		t.Fatal("expected an error when both MagnetURI and TorrentBytes are set")
	}
	if _, err := NewTask(Options{MagnetURI: "magnet:?xt=x"}); err == nil {
		t.Fatal("expected an error when DestDir is empty")
	}
}

func TestNoProxyProducesWarning(t *testing.T) {
	tk, err := NewTask(Options{MagnetURI: "magnet:?xt=x", DestDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	if tk.Warning() == "" {
		t.Fatal("expected a warning when no proxy is configured")
	}
}

func TestProxyConfiguredProducesNoWarning(t *testing.T) {
	tk, err := NewTask(Options{MagnetURI: "magnet:?xt=x", DestDir: t.TempDir(), ProxyAddr: "127.0.0.1:1"})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	if tk.Warning() != "" {
		t.Fatalf("expected no warning when a proxy is configured, got %q", tk.Warning())
	}
}
