package httpengine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NekoSuneVR/NekoDL/core/internal/task"
)

// rangeServer serves a fixed byte payload and honors Range requests like a
// real static file server would (206 + Content-Range, or 200 for no Range header).
func rangeServer(t *testing.T, payload []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload)
			return
		}

		start, end, ok := parseTestRange(rangeHeader, len(payload))
		if !ok {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start : end+1])
	}))
}

func parseTestRange(header string, size int) (start, end int, ok bool) {
	header = strings.TrimPrefix(header, "bytes=")
	parts := strings.SplitN(header, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	if parts[1] == "" {
		end = size - 1
	} else {
		end, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, false
		}
	}
	return start, end, true
}

func waitForStatus(t *testing.T, tk *Task, want task.Status, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if tk.Status() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for status %s, got %s", want, tk.Status())
}

func TestDownloadSingleConnectionNoRangeSupport(t *testing.T) {
	payload := []byte("hello from a plain, non-ranged HTTP server")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	tk, err := New(Options{ID: "t1", URLs: []string{srv.URL}, DestPath: dest})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusComplete, 5*time.Second)

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("content mismatch: got %q want %q", got, payload)
	}
}

func TestDownloadSegmentedMultiConnection(t *testing.T) {
	payload := make([]byte, 3<<20) // 3 MiB — splits into 3 segments at maxConn=3
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	srv := rangeServer(t, payload)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	tk, err := New(Options{ID: "t2", URLs: []string{srv.URL}, DestPath: dest, MaxConnections: 3})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusComplete, 10*time.Second)

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if len(got) != len(payload) {
		t.Fatalf("length mismatch: got %d want %d", len(got), len(payload))
	}
	for i := range payload {
		if got[i] != payload[i] {
			t.Fatalf("first mismatch at byte %d: got %d want %d", i, got[i], payload[i])
		}
	}

	progress := tk.Progress()
	if progress.DownloadedBytes != int64(len(payload)) {
		t.Fatalf("progress DownloadedBytes = %d, want %d", progress.DownloadedBytes, len(payload))
	}
}

func TestResumeContinuesFromSavedOffset(t *testing.T) {
	payload := []byte("0123456789ABCDEFGHIJ") // 20 bytes
	var gotRangeHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRangeHeader = r.Header.Get("Range")
		start, end, ok := parseTestRange(gotRangeHeader, len(payload))
		if !ok {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start : end+1])
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")

	// Simulate a prior run that already fetched the first 12 bytes.
	if err := os.WriteFile(dest, payload[:12], 0o644); err != nil {
		t.Fatalf("seed dest: %v", err)
	}
	if err := saveProgress(progressSnapshot{
		URLs:            []string{srv.URL},
		Dest:            dest,
		Total:           int64(len(payload)),
		RangesSupported: true,
		Segments:        []segmentRange{{Start: 0, Current: 12, End: int64(len(payload) - 1)}},
	}); err != nil {
		t.Fatalf("seed progress: %v", err)
	}

	tk, err := New(Options{ID: "t3", URLs: []string{srv.URL}, DestPath: dest})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if p := tk.Progress(); p.DownloadedBytes != 12 {
		t.Fatalf("expected resumed progress of 12 bytes, got %d", p.DownloadedBytes)
	}

	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusComplete, 5*time.Second)

	if gotRangeHeader != "bytes=12-19" {
		t.Fatalf("expected resume to request bytes=12-19, server saw %q", gotRangeHeader)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("content mismatch: got %q want %q", got, payload)
	}
}

func TestChecksumMismatchFailsTask(t *testing.T) {
	payload := []byte("some file contents")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	tk, err := New(Options{
		ID:       "t4",
		URLs:     []string{srv.URL},
		DestPath: dest,
		Checksum: &Checksum{Algo: "sha256", Expected: "0000000000000000000000000000000000000000000000000000000000000"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusError, 5*time.Second)

	if tk.Err() == nil || !strings.Contains(tk.Err().Error(), "checksum mismatch") {
		t.Fatalf("expected a checksum mismatch error, got %v", tk.Err())
	}
}

func TestChecksumMatchSucceeds(t *testing.T) {
	payload := []byte("checksum me please")
	sum := sha256.Sum256(payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	tk, err := New(Options{
		ID:       "t5",
		URLs:     []string{srv.URL},
		DestPath: dest,
		Checksum: &Checksum{Algo: "sha256", Expected: hex.EncodeToString(sum[:])},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusComplete, 5*time.Second)
}

func TestMirrorFallbackToSecondURL(t *testing.T) {
	payload := []byte("served by the second mirror")

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer dead.Close()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer good.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	tk, err := New(Options{ID: "t6", URLs: []string{dead.URL, good.URL}, DestPath: dest, MaxRetries: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusComplete, 5*time.Second)

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("content mismatch: got %q want %q", got, payload)
	}
}

func TestRetrySucceedsAfterTransientFailure(t *testing.T) {
	payload := []byte("survives one flaky attempt")
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	tk, err := New(Options{ID: "t7", URLs: []string{srv.URL}, DestPath: dest, MaxRetries: 2})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusComplete, 5*time.Second)

	if attempts.Load() < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", attempts.Load())
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("content mismatch: got %q want %q", got, payload)
	}
}

func TestPauseStopsAndResumeContinues(t *testing.T) {
	const totalSize = 2 << 20 // 2 MiB, big enough to not finish instantly
	payload := make([]byte, totalSize)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	entered := make(chan struct{}, 1)
	release := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release // hold the response open until the test says go

		start, end, ok := parseTestRange(r.Header.Get("Range"), len(payload))
		if !ok {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start : end+1])
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	tk, err := New(Options{ID: "t8", URLs: []string{srv.URL}, DestPath: dest, MaxConnections: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("server handler was never entered")
	}

	if err := tk.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	close(release)

	if got := tk.Status(); got != task.StatusPaused {
		t.Fatalf("expected StatusPaused after Pause, got %s", got)
	}

	if err := tk.Resume(); err != nil {
		t.Fatalf("second Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusComplete, 10*time.Second)

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if len(got) != len(payload) {
		t.Fatalf("length mismatch: got %d want %d", len(got), len(payload))
	}
}
