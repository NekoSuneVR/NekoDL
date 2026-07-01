package torrentengine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NekoSuneVR/NekoDL/core/internal/task"
)

// alternatingIPServer returns a different fake "public IP" on the first
// request than on every request after — so DetectLeak's two sequential
// requests (direct, then proxied) get genuinely different answers, letting
// the test exercise its real comparison logic instead of a canned result.
func alternatingIPServer(t *testing.T) *httptest.Server {
	t.Helper()
	var calls atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte("203.0.113.1")) // stands in for the "direct" IP
			return
		}
		_, _ = w.Write([]byte("198.51.100.2")) // stands in for the "proxied" IP
	}))
}

func fixedIPServer(t *testing.T, ip string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(ip))
	}))
}

func TestDetectLeakReportsNoLeakWhenIPsDiffer(t *testing.T) {
	echo := alternatingIPServer(t)
	defer echo.Close()
	proxyAddr := startTestSOCKS5Proxy(t)

	result, err := DetectLeak(context.Background(), proxyAddr, echo.URL)
	if err != nil {
		t.Fatalf("DetectLeak: %v", err)
	}
	if result.Leaked {
		t.Fatalf("expected Leaked=false when direct/proxied IPs differ, got %+v", result)
	}
	if result.DirectIP != "203.0.113.1" || result.ProxiedIP != "198.51.100.2" {
		t.Fatalf("unexpected IPs recorded: %+v", result)
	}
}

func TestDetectLeakReportsLeakWhenIPsMatch(t *testing.T) {
	echo := fixedIPServer(t, "203.0.113.1")
	defer echo.Close()
	proxyAddr := startTestSOCKS5Proxy(t)

	result, err := DetectLeak(context.Background(), proxyAddr, echo.URL)
	if err != nil {
		t.Fatalf("DetectLeak: %v", err)
	}
	if !result.Leaked {
		t.Fatal("expected Leaked=true when direct and proxied IPs are identical")
	}
	if result.Reason == "" {
		t.Fatal("expected a Reason explaining the leak")
	}
}

func TestDetectLeakReportsLeakWhenProxyIsDown(t *testing.T) {
	echo := fixedIPServer(t, "203.0.113.1")
	defer echo.Close()

	// Port 1 is reserved and never has anything listening on it — simulates
	// the kill switch's real trigger condition (proxy unreachable) without
	// needing to start and then tear down a real listener mid-test.
	const deadProxyAddr = "127.0.0.1:1"

	result, err := DetectLeak(context.Background(), deadProxyAddr, echo.URL)
	if err != nil {
		t.Fatalf("DetectLeak: %v", err)
	}
	if !result.Leaked {
		t.Fatal("expected Leaked=true when the proxy is unreachable")
	}
}

func TestKillSwitchStopsTaskWhenProxyDrops(t *testing.T) {
	seedDir := newSeedDir(t)
	sourceFile := filepath.Join(seedDir, "testfile.bin")
	writeRandomFile(t, sourceFile, 300*1024)

	_, _, mi := startSeeder(t, sourceFile)

	echo := fixedIPServer(t, "203.0.113.1") // same IP for direct+proxied => always "leaked"
	defer echo.Close()
	proxyAddr := startTestSOCKS5Proxy(t)

	leechDir := t.TempDir()
	tk, err := NewTask(Options{
		ID:                "killswitch1",
		MagnetURI:         mi.Magnet(nil, nil).String(),
		DestDir:           leechDir,
		DisableDHT:        true,
		DisablePEX:        true,
		ProxyAddr:         proxyAddr,
		LeakCheckInterval: 50 * time.Millisecond,
		IPCheckURL:        echo.URL,
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	t.Cleanup(func() { tk.Cancel() })

	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	// Deliberately not connected to the seeder as a peer — the kill switch
	// should trip on its own leak-check schedule regardless of transfer progress.

	waitForStatus(t, tk, task.StatusError, 5*time.Second)

	if tk.Err() == nil {
		t.Fatal("expected Err() to explain the kill switch trip")
	}
}
