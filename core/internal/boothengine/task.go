// Package boothengine wraps Myrkie/BoothDownloader (Apache-2.0) as a managed
// subprocess, the same shell-out approach used for yt-dlp — see TODO.md
// Phase 0's integration decision and Phase 5 for what this covers and what
// it honestly doesn't.
package boothengine

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/NekoSuneVR/NekoDL/core/internal/task"
)

// Options configure one BoothDownloader run.
type Options struct {
	ID string

	// Input is passed through verbatim as BoothDownloader's own --booth
	// value: an item URL, a bare item ID, or one of its collection
	// keywords ("gifts", "orders", "owned") — space-separated combinations
	// are accepted too, exactly like typing them into BoothDownloader's own
	// console prompt. NekoDL does no parsing of its own here; confirmed by
	// reading BoothDownloader's actual argument-handling source that it
	// already accepts all of these natively. Must be non-empty — an empty
	// --booth makes BoothDownloader fall back to an interactive console
	// prompt that hangs forever against a subprocess with no real stdin.
	Input string

	DestDir string

	// BinaryPath is the BoothDownloader executable to run. Unlike yt-dlp,
	// there's no pip-style canonical install/PATH name for it, so this
	// must be configured explicitly — no default is guessed.
	BinaryPath string

	// Cookie is a Booth session token — it grants access to a real
	// account's purchases, so treat it like a secret. It's written to a
	// per-task config file on disk (0o600), never passed as a CLI
	// argument (which would be visible in a process listing) and never
	// logged. Empty means anonymous: image downloads still work, purchased
	// file downloads don't (BoothDownloader's own behavior, confirmed live).
	Cookie string

	// AutoZip mirrors BoothDownloader's own AutoZip config option: zip each
	// item's downloaded folder instead of leaving it as a plain directory.
	AutoZip bool

	// MaxRetries is BoothDownloader's own --max-retries for binary file
	// downloads. <=0 means BoothDownloader's own default (3).
	MaxRetries int
}

// Task runs one BoothDownloader subprocess. It implements task.Task.
//
// Progress is necessarily coarse: BoothDownloader renders per-file/per-item
// progress with a terminal progress-bar library (ShellProgressBar), not a
// machine-readable format, and confirmed live that none of that reaches a
// piped, non-TTY stdout anyway — so there is no byte-level progress to
// report, and Progress() always reports zero. Status is derived from the
// subprocess's own log lines and exit code — see Warning()'s doc comment
// for a real, confirmed limitation on how much that can catch.
type Task struct {
	opts Options

	mu      sync.Mutex
	status  task.Status
	lastErr error
	warning string
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewTask(opts Options) (*Task, error) {
	if strings.TrimSpace(opts.Input) == "" {
		return nil, fmt.Errorf("boothengine: Input is required")
	}
	if opts.DestDir == "" {
		return nil, fmt.Errorf("boothengine: DestDir is required")
	}
	if opts.BinaryPath == "" {
		return nil, fmt.Errorf("boothengine: BinaryPath is required (no default — BoothDownloader has no standard install location)")
	}
	return &Task{opts: opts, status: task.StatusPending}, nil
}

func (t *Task) ID() string     { return t.opts.ID }
func (t *Task) Engine() string { return "booth" }

func (t *Task) Status() task.Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// Err returns the error that caused the task to fail, if any — the
// optional errorProvider capability, same as httpengine.Task and others.
func (t *Task) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastErr
}

// Warning surfaces BoothDownloader's own most-relevant WARN/EROR log line
// verbatim (e.g. "Cookie is not valid...", "Cannot download Paid Items
// with invalid cookie.") — the optional warningProvider capability.
//
// Real, confirmed limitation from live testing: BoothDownloader exits 0
// even when it downloaded nothing (an invalid/deleted item ID, or an item
// you don't own, both silently produce an empty output folder/zip with no
// distinguishing error line at all — its per-item failure messages go
// through its progress-bar library's writer, not its logger, and were
// confirmed not to reach stdout in a piped/non-TTY run). So a task can
// report StatusComplete with an empty Warning() and still have downloaded
// nothing — this wrapper surfaces every real signal BoothDownloader
// actually gives, but "item not owned" specifically isn't one of them.
func (t *Task) Warning() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.warning
}

func (t *Task) Progress() task.Progress {
	return task.Progress{}
}

func (t *Task) buildArgs(configPath, destDir string) []string {
	maxRetries := t.opts.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return []string{
		"--config", configPath,
		"--booth", t.opts.Input,
		"--output-dir", destDir,
		"--max-retries", strconv.Itoa(maxRetries),
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

func (t *Task) run(ctx context.Context) {
	// BoothDownloader resets its own working directory to wherever its exe
	// lives on startup (confirmed live by reading its source and by an
	// actual test run: a relative --config/--output-dir silently resolved
	// against the *binary's* directory, not the launch directory, and an
	// unfound config with no Cookie set triggered exactly the interactive
	// hang described above). Absolute paths are required, not a style
	// preference — filepath.Abs guards against a caller passing relative ones.
	destDir, err := filepath.Abs(t.opts.DestDir)
	if err != nil {
		t.fail(fmt.Errorf("boothengine: resolving destination directory: %w", err))
		return
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.fail(fmt.Errorf("boothengine: creating destination directory: %w", err))
		return
	}

	configPath, err := filepath.Abs(filepath.Join(destDir, "BDConfig.json"))
	if err != nil {
		t.fail(fmt.Errorf("boothengine: resolving config path: %w", err))
		return
	}
	if err := writeConfig(configPath, t.opts.Cookie, t.opts.AutoZip); err != nil {
		t.fail(fmt.Errorf("boothengine: writing config: %w", err))
		return
	}

	cmd := exec.CommandContext(ctx, t.opts.BinaryPath, t.buildArgs(configPath, destDir)...)
	cmd.Dir = destDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.fail(fmt.Errorf("boothengine: %w", err))
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.fail(fmt.Errorf("boothengine: %w", err))
		return
	}

	t.mu.Lock()
	t.cmd = cmd
	t.mu.Unlock()

	if err := cmd.Start(); err != nil {
		if ctx.Err() != nil {
			return // Pause()/Cancel() fired before the process even started — not a real failure
		}
		t.fail(fmt.Errorf("boothengine: starting BoothDownloader: %w", err))
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

	var lastWarn, lastErrorLine string
	scanLogLines(stdout, func(l logLine) {
		switch strings.ToUpper(l.Level) {
		case "WARN":
			lastWarn = l.Message
		case "EROR", "ERROR":
			lastErrorLine = l.Message
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
			msg = stderrLines[len(stderrLines)-1]
		} else if lastErrorLine != "" {
			msg = lastErrorLine
		}
		t.fail(fmt.Errorf("boothengine: %s", msg))
		return
	}

	// BoothDownloader exits 0 even when it found/downloaded nothing (see
	// Warning()'s doc comment) — a real EROR/WARN line is surfaced as a
	// warning, not treated as task failure, since the subprocess itself
	// still completed and may have partially succeeded across a
	// multi-item --booth input (e.g. "gifts" with one broken item among many).
	t.mu.Lock()
	if lastErrorLine != "" {
		t.warning = lastErrorLine
	} else {
		t.warning = lastWarn
	}
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
