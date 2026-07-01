// Package httpengine implements task.Task for plain HTTP/HTTPS downloads:
// segmented multi-connection fetching with resume, checksum verification,
// per-segment retry/backoff, and mirror/fallback URLs.
package httpengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NekoSuneVR/NekoDL/core/internal/task"
)

// Options configure one download.
type Options struct {
	ID       string
	URLs     []string // tried in order; later ones are mirrors/fallbacks
	DestPath string

	MaxConnections int // segments per download; <=1 means single-connection
	MaxRetries     int // per-segment retry attempts before trying the next mirror

	Checksum *Checksum // nil disables verification
	Client   *http.Client
}

// renamedForFilenameHint recomputes DestPath's basename using a
// Content-Disposition-supplied filename, preserving DestPath's directory
// and its existing "<id>-" collision-safe prefix.
func renamedForFilenameHint(dest, filename string) string {
	dir, base := filepath.Split(dest)
	prefix := base
	if idx := strings.Index(base, "-"); idx != -1 {
		prefix = base[:idx]
	}
	return filepath.Join(dir, prefix+"-"+filename)
}

// Task is an HTTP/HTTPS download. It implements task.Task.
type Task struct {
	id       string
	urls     []string
	dest     string
	maxConn  int
	maxRetry int
	checksum *Checksum
	client   *http.Client

	mu              sync.Mutex
	status          task.Status
	total           int64
	rangesSupported bool
	segments        []segmentRange
	lastErr         error

	downloaded atomic.Int64
	rate       *rateTracker

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a Task. If a progress sidecar file already exists for
// opts.DestPath, its resume state is loaded automatically.
func New(opts Options) (*Task, error) {
	if len(opts.URLs) == 0 {
		return nil, errors.New("httpengine: at least one URL is required")
	}
	if opts.DestPath == "" {
		return nil, errors.New("httpengine: destination path is required")
	}

	maxConn := opts.MaxConnections
	if maxConn <= 0 {
		maxConn = 1
	}
	maxRetry := opts.MaxRetries
	if maxRetry <= 0 {
		maxRetry = 3
	}
	client := opts.Client
	if client == nil {
		client = http.DefaultClient
	}

	t := &Task{
		id:       opts.ID,
		urls:     opts.URLs,
		dest:     opts.DestPath,
		maxConn:  maxConn,
		maxRetry: maxRetry,
		checksum: opts.Checksum,
		client:   client,
		status:   task.StatusPending,
		rate:     &rateTracker{},
	}

	snap, err := loadProgress(opts.DestPath)
	if err != nil {
		return nil, fmt.Errorf("httpengine: loading resume state: %w", err)
	}
	if snap != nil {
		t.total = snap.Total
		t.rangesSupported = snap.RangesSupported
		t.segments = snap.Segments
		var downloaded int64
		for _, s := range t.segments {
			downloaded += s.Current - s.Start
		}
		t.downloaded.Store(downloaded)
	}

	return t, nil
}

func (t *Task) ID() string     { return t.id }
func (t *Task) Engine() string { return "http" }

func (t *Task) Status() task.Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// Err returns the error that caused the task to fail, if any. Not part of
// task.Task — callers that want the reason behind task.StatusError use this.
func (t *Task) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastErr
}

func (t *Task) Progress() task.Progress {
	t.mu.Lock()
	total := t.total
	t.mu.Unlock()

	return task.Progress{
		TotalBytes:      total,
		DownloadedBytes: t.downloaded.Load(),
		SpeedBytesPerS:  t.rate.Rate(),
	}
}

func (t *Task) Resume() error {
	t.mu.Lock()
	switch t.status {
	case task.StatusActive, task.StatusComplete, task.StatusCancelled:
		t.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	t.status = task.StatusActive
	t.mu.Unlock()

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.run(ctx)
	}()
	return nil
}

func (t *Task) Pause() error {
	t.mu.Lock()
	if t.status != task.StatusActive {
		t.mu.Unlock()
		return nil
	}
	cancel := t.cancel
	t.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	t.wg.Wait()

	t.mu.Lock()
	if t.status == task.StatusActive {
		t.status = task.StatusPaused
	}
	t.mu.Unlock()
	return nil
}

func (t *Task) Cancel() error {
	t.mu.Lock()
	cancel := t.cancel
	t.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	t.wg.Wait()

	t.mu.Lock()
	t.status = task.StatusCancelled
	t.mu.Unlock()
	return nil
}

func (t *Task) Remove() error {
	if err := t.Cancel(); err != nil {
		return err
	}
	_ = os.Remove(progressPath(t.dest))
	_ = os.Remove(t.dest)
	return nil
}

// run drives one Resume() cycle to completion, pause, or failure.
func (t *Task) run(ctx context.Context) {
	if err := os.MkdirAll(filepath.Dir(t.dest), 0o755); err != nil {
		t.fail(fmt.Errorf("httpengine: creating destination directory: %w", err))
		return
	}

	if err := t.ensureSegments(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		t.fail(err)
		return
	}

	var lastErr error
	for _, url := range t.urls {
		err := t.downloadSegments(ctx, url)
		if err == nil {
			if err := t.finalize(); err != nil {
				t.fail(err)
			}
			return
		}
		if errors.Is(err, context.Canceled) {
			return // Pause() or Cancel() already set the right status
		}
		lastErr = err
	}
	t.fail(fmt.Errorf("httpengine: download failed on all mirrors: %w", lastErr))
}

func (t *Task) ensureSegments(ctx context.Context) error {
	t.mu.Lock()
	already := len(t.segments) > 0
	t.mu.Unlock()
	if already {
		return nil
	}

	var lastErr error
	for _, url := range t.urls {
		total, ranged, filename, err := t.probeWithRetry(ctx, url)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			lastErr = err
			continue
		}

		t.mu.Lock()
		if filename != "" {
			t.dest = renamedForFilenameHint(t.dest, filename)
		}
		t.total = total
		t.rangesSupported = ranged
		t.segments = buildSegments(total, t.maxConn, ranged)
		snap := t.snapshotLocked()
		dest := t.dest
		t.mu.Unlock()

		if ranged && total > 0 {
			if err := preallocate(dest, total); err != nil {
				return err
			}
		}
		return saveProgress(snap)
	}
	return fmt.Errorf("httpengine: probing all mirrors failed: %w", lastErr)
}

func (t *Task) downloadSegments(ctx context.Context, url string) error {
	file, err := os.OpenFile(t.dest, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	t.mu.Lock()
	n := len(t.segments)
	t.mu.Unlock()

	var (
		wg       sync.WaitGroup
		errOnce  sync.Once
		firstErr error
	)

	for i := 0; i < n; i++ {
		t.mu.Lock()
		pending := t.segments[i].End < 0 || t.segments[i].Current <= t.segments[i].End
		t.mu.Unlock()
		if !pending {
			continue
		}

		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := t.downloadSegmentWithRetry(ctx, file, url, i); err != nil {
				errOnce.Do(func() { firstErr = err })
			}
		}()
	}
	wg.Wait()
	return firstErr
}

func (t *Task) downloadSegmentWithRetry(ctx context.Context, file *os.File, url string, index int) error {
	err := retryWithBackoff(ctx, t.maxRetry, func() error {
		return t.fetchSegment(ctx, file, url, index)
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("segment %d: %w", index, err)
	}
	return err
}

// probeWithRetry retries transient probe failures the same way segment
// downloads are retried — a flaky connection on the very first request
// shouldn't fail the whole task immediately.
func (t *Task) probeWithRetry(ctx context.Context, url string) (total int64, ranged bool, filename string, err error) {
	err = retryWithBackoff(ctx, t.maxRetry, func() error {
		var e error
		total, ranged, filename, e = probe(ctx, t.client, url)
		return e
	})
	return total, ranged, filename, err
}

// retryWithBackoff calls fn until it succeeds, ctx is cancelled, or it has
// failed maxRetries+1 times in total, waiting attempt*500ms between tries.
func retryWithBackoff(ctx context.Context, maxRetries int, fn func() error) error {
	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if errors.Is(err, context.Canceled) {
			return err
		}
		if attempt >= maxRetries {
			return err
		}

		backoff := time.Duration(attempt+1) * 500 * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
}

func (t *Task) fetchSegment(ctx context.Context, file *os.File, url string, index int) error {
	t.mu.Lock()
	ranged := t.rangesSupported
	if !ranged {
		// Can't resume without server range support — every attempt restarts from 0.
		t.segments[index].Current = 0
		t.downloaded.Store(0)
	}
	seg := t.segments[index]
	t.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if ranged {
		rangeHeader := "bytes=" + strconv.FormatInt(seg.Current, 10) + "-"
		if seg.End >= 0 {
			rangeHeader += strconv.FormatInt(seg.End, 10)
		}
		req.Header.Set("Range", rangeHeader)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	wantStatus := http.StatusOK
	if ranged {
		wantStatus = http.StatusPartialContent
	}
	if resp.StatusCode != wantStatus {
		return &statusError{resp.StatusCode}
	}

	offset := seg.Current
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := file.WriteAt(buf[:n], offset); werr != nil {
				return werr
			}
			offset += int64(n)
			total := t.downloaded.Add(int64(n))
			t.rate.sample(total)

			t.mu.Lock()
			t.segments[index].Current = offset
			snap := t.snapshotLocked()
			t.mu.Unlock()
			_ = saveProgress(snap)
		}
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return readErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func (t *Task) finalize() error {
	if t.checksum != nil {
		if err := verifyChecksum(t.dest, *t.checksum); err != nil {
			return err
		}
	}
	_ = os.Remove(progressPath(t.dest))

	t.mu.Lock()
	t.status = task.StatusComplete
	t.mu.Unlock()
	return nil
}

func (t *Task) fail(err error) {
	t.mu.Lock()
	t.status = task.StatusError
	t.lastErr = err
	t.mu.Unlock()
}

// snapshotLocked builds a progressSnapshot from current state. Callers must hold t.mu.
func (t *Task) snapshotLocked() progressSnapshot {
	return progressSnapshot{
		URLs:            t.urls,
		Dest:            t.dest,
		Total:           t.total,
		RangesSupported: t.rangesSupported,
		Segments:        append([]segmentRange(nil), t.segments...),
	}
}
