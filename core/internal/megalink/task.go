package megalink

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/NekoSuneVR/NekoDL/core/internal/task"
)

// Task implements task.Task for a single public mega.nz file download.
//
// Unlike httpengine.Task, this is single-shot: MEGA's temporary URLs
// weren't confirmed to support resumable Range requests, so Pause() stops
// the transfer but Resume() after a pause restarts it from the beginning
// rather than continuing partway through — a real limitation, not an
// oversight, documented here and in TODO.md.
type Task struct {
	id      string
	rawURL  string
	destDir string
	dl      *Downloader

	mu      sync.Mutex
	status  task.Status
	lastErr error
	destPath string

	total      int64
	downloaded atomic.Int64

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewTask creates a Task that will download rawURL into destDir once
// started. The final filename isn't known until MEGA's metadata resolves,
// so it's decided at Resume() time, not here.
func NewTask(id, rawURL, destDir string, dl *Downloader) *Task {
	if dl == nil {
		dl = &Downloader{}
	}
	return &Task{id: id, rawURL: rawURL, destDir: destDir, dl: dl, status: task.StatusPending}
}

func (t *Task) ID() string     { return t.id }
func (t *Task) Engine() string { return "mega" }

func (t *Task) Status() task.Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// Err returns the error that caused the task to fail, if any. Not part of
// task.Task — mirrors httpengine.Task.Err, and the scheduler picks it up
// the same way via the optional errorProvider capability.
func (t *Task) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastErr
}

func (t *Task) Progress() task.Progress {
	t.mu.Lock()
	total := t.total
	t.mu.Unlock()
	return task.Progress{TotalBytes: total, DownloadedBytes: t.downloaded.Load()}
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

func (t *Task) run(ctx context.Context) {
	meta, err := t.dl.Metadata(ctx, t.rawURL)
	if err != nil {
		t.fail(err)
		return
	}

	t.mu.Lock()
	t.total = meta.Size
	name := meta.Name
	if name == "" {
		name = t.id
	}
	dest := filepath.Join(t.destDir, t.id+"-"+name)
	t.destPath = dest
	t.mu.Unlock()

	if err := os.MkdirAll(t.destDir, 0o755); err != nil {
		t.fail(err)
		return
	}

	_, err = t.dl.DownloadWithProgress(ctx, t.rawURL, dest, func(n int64) {
		t.downloaded.Store(n)
	})
	if err != nil {
		if ctx.Err() != nil {
			return // Pause() or Cancel() already set the right status
		}
		t.fail(err)
		return
	}

	t.mu.Lock()
	t.status = task.StatusComplete
	t.mu.Unlock()
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
	// A paused MEGA download restarts from scratch on Resume() — reset the
	// progress counter so that's reflected immediately rather than showing
	// a stale byte count until the restart catches up.
	t.downloaded.Store(0)
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
	t.mu.Lock()
	dest := t.destPath
	t.mu.Unlock()
	if dest != "" {
		_ = os.Remove(dest)
	}
	return nil
}

func (t *Task) fail(err error) {
	t.mu.Lock()
	t.status = task.StatusError
	t.lastErr = err
	t.mu.Unlock()
}
