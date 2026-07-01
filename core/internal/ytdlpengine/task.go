// Package ytdlpengine implements task.Task by wrapping yt-dlp as a managed
// subprocess. yt-dlp itself stays an external binary (see TODO.md Phase 4
// for the bundling/pinning/auto-update/patch-workflow decisions around
// that) — this package's job is just running it correctly and turning its
// own JSON progress output into NekoDL's task.Progress model.
package ytdlpengine

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/NekoSuneVR/NekoDL/core/internal/task"
)

// Options configure one yt-dlp download.
type Options struct {
	ID  string
	URL string

	DestDir string

	// BinaryPath is the yt-dlp executable to run. Empty means "yt-dlp"
	// resolved from PATH — the Docker image bundles a pinned one there
	// (see TODO.md Phase 4); a host install can point this wherever it likes.
	BinaryPath string

	Format         string // yt-dlp -f, e.g. "best", "bestvideo+bestaudio"
	NoPlaylist     bool   // --no-playlist: only download the single linked video, not its playlist
	Subtitles      bool   // --write-subs --write-auto-subs --sub-langs all
	OutputTemplate string // yt-dlp -o; defaults to "%(title)s.%(ext)s"

	// ProxyAddr routes this download through a proxy if set. Unlike
	// torrentengine, this has no "no proxy configured" warning — a yt-dlp
	// download isn't P2P and doesn't expose your IP to strangers the way
	// torrenting does, so there's nothing privacy-sensitive to warn about
	// by leaving it unset. Off by default.
	ProxyAddr string

	// CookiesFile is a path to a cookies.txt file (Netscape format, the
	// same format yt-dlp's own --cookies-from-browser export produces) for
	// sites that require login — same UX pattern as the Booth engine's
	// cookie/token input.
	CookiesFile string
}

// Task runs one yt-dlp subprocess. It implements task.Task.
//
// Pause/Resume works differently than the other engines: there's no
// pause primitive for a subprocess, so Pause() kills it and Resume()
// restarts it from scratch. yt-dlp resumes partially-downloaded files on
// its own (its default --continue behavior) when re-run with the same
// output template, so this often continues rather than re-downloading —
// but that's yt-dlp's own behavior, not something this wrapper guarantees.
type Task struct {
	opts Options

	mu      sync.Mutex
	status  task.Status
	lastErr error
	cmd     *exec.Cmd

	total      atomic.Int64 // -1 until known
	downloaded atomic.Int64
	speed      atomic.Int64

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewTask(opts Options) (*Task, error) {
	if opts.URL == "" {
		return nil, fmt.Errorf("ytdlpengine: URL is required")
	}
	if opts.DestDir == "" {
		return nil, fmt.Errorf("ytdlpengine: DestDir is required")
	}
	if opts.BinaryPath == "" {
		opts.BinaryPath = "yt-dlp"
	}
	if opts.OutputTemplate == "" {
		opts.OutputTemplate = "%(title)s.%(ext)s"
	}
	t := &Task{opts: opts, status: task.StatusPending}
	t.total.Store(-1)
	return t, nil
}

func (t *Task) ID() string     { return t.opts.ID }
func (t *Task) Engine() string { return "ytdlp" }

func (t *Task) Status() task.Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

func (t *Task) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastErr
}

func (t *Task) Progress() task.Progress {
	total := t.total.Load()
	if total < 0 {
		total = 0
	}
	return task.Progress{
		TotalBytes:      total,
		DownloadedBytes: t.downloaded.Load(),
		SpeedBytesPerS:  t.speed.Load(),
	}
}

func (t *Task) buildArgs() []string {
	args := []string{"--newline", "--progress-template", "download:%(progress)j"}
	if t.opts.Format != "" {
		args = append(args, "-f", t.opts.Format)
	}
	if t.opts.NoPlaylist {
		args = append(args, "--no-playlist")
	}
	if t.opts.Subtitles {
		args = append(args, "--write-subs", "--write-auto-subs", "--sub-langs", "all")
	}
	args = append(args, "-o", t.opts.OutputTemplate)
	if t.opts.ProxyAddr != "" {
		args = append(args, "--proxy", t.opts.ProxyAddr)
	}
	if t.opts.CookiesFile != "" {
		args = append(args, "--cookies", t.opts.CookiesFile)
	}
	args = append(args, t.opts.URL)
	return args
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
	cmd := exec.CommandContext(ctx, t.opts.BinaryPath, t.buildArgs()...)
	cmd.Dir = t.opts.DestDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.fail(fmt.Errorf("ytdlpengine: %w", err))
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.fail(fmt.Errorf("ytdlpengine: %w", err))
		return
	}

	t.mu.Lock()
	t.cmd = cmd
	t.mu.Unlock()

	if err := cmd.Start(); err != nil {
		if ctx.Err() != nil {
			return // Pause()/Cancel() fired before the process even started — not a real failure
		}
		t.fail(fmt.Errorf("ytdlpengine: starting yt-dlp: %w", err))
		return
	}

	var stderrLines []string
	var stderrWg sync.WaitGroup
	stderrWg.Add(1)
	go func() {
		defer stderrWg.Done()
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			stderrLines = append(stderrLines, s.Text())
		}
	}()

	scanProgress(stdout, func(p progressLine) {
		if p.TotalBytes != nil {
			t.total.Store(*p.TotalBytes)
		}
		t.downloaded.Store(p.DownloadedBytes)
		if p.Speed != nil {
			t.speed.Store(int64(*p.Speed))
		}
	})

	stderrWg.Wait()
	waitErr := cmd.Wait()

	if ctx.Err() != nil {
		return // Pause()/Cancel() already set the right status
	}

	if waitErr != nil {
		msg := waitErr.Error()
		if len(stderrLines) > 0 {
			msg = stderrLines[len(stderrLines)-1] // yt-dlp's own last error line is more useful than the exec error
		}
		t.fail(fmt.Errorf("ytdlpengine: %s", msg))
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
	return t.Cancel()
}

func (t *Task) fail(err error) {
	t.mu.Lock()
	t.status = task.StatusError
	t.lastErr = err
	t.mu.Unlock()
}
