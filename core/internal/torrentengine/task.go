// Package torrentengine implements task.Task for BitTorrent downloads,
// using github.com/anacrolix/torrent. DHT, PEX, and tracker announces are
// the library's defaults — nothing extra is needed to enable them.
//
// Privacy is the point of this engine, not an afterthought: torrent traffic
// is the one protocol here that hands your real IP to strangers by design.
// If Options.ProxyAddr is set, every peer connection and tracker/webseed
// HTTP request for this task's torrent.Client routes through that SOCKS5
// proxy (see dialer.go), uTP is disabled (its UDP transport wouldn't go
// through the SOCKS5 CONNECT dialer, which is TCP-only — leaving it on
// would silently bypass the proxy), and a background leak/kill-switch
// check (ipcheck.go) periodically confirms traffic is actually being
// routed through the proxy, pausing the task (into StatusError, so the
// scheduler won't auto-resume it — see scheduler.go's rescheduleLocked) if
// it isn't. If ProxyAddr is empty, the task proceeds using the real IP —
// see Warning().
package torrentengine

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/NekoSuneVR/NekoDL/core/internal/task"
)

// Options configure one torrent download.
type Options struct {
	ID string

	// Exactly one of MagnetURI or TorrentBytes must be set.
	MagnetURI    string
	TorrentBytes []byte

	DestDir string

	// ProxyAddr, if set, routes this task's peer connections and
	// tracker/webseed HTTP requests through a SOCKS5 proxy at this address
	// (host:port, no auth). If empty, this task uses the real IP directly.
	ProxyAddr string

	DisableDHT bool
	DisablePEX bool
	Seed       bool

	// SeedRatioLimit stops uploading once bytes-uploaded/bytes-downloaded
	// reaches this ratio. SeedTimeLimit stops uploading once this long has
	// passed since the download completed. Zero means unlimited for that
	// dimension. Neither affects downloading — only DisallowDataUpload.
	SeedRatioLimit float64
	SeedTimeLimit  time.Duration

	MaxDownloadBps int64
	MaxUploadBps   int64

	// LeakCheckInterval controls how often the kill switch re-checks the
	// proxy while ProxyAddr is set. Defaults to 30s; tests override it to
	// something much shorter.
	LeakCheckInterval time.Duration
	// IPCheckURL overrides the public-IP echo service — for tests only.
	IPCheckURL string
}

// Task is a BitTorrent download/seed. It implements task.Task.
type Task struct {
	opts Options

	mu          sync.Mutex
	status      task.Status
	lastErr     error
	warning     string
	completedAt time.Time

	client *torrent.Client
	tor    *torrent.Torrent

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewTask validates opts and returns a Task that hasn't started yet — the
// torrent.Client and swarm connections aren't created until Resume().
func NewTask(opts Options) (*Task, error) {
	if opts.MagnetURI == "" && len(opts.TorrentBytes) == 0 {
		return nil, fmt.Errorf("torrentengine: either MagnetURI or TorrentBytes is required")
	}
	if opts.MagnetURI != "" && len(opts.TorrentBytes) != 0 {
		return nil, fmt.Errorf("torrentengine: only one of MagnetURI or TorrentBytes may be set")
	}
	if opts.DestDir == "" {
		return nil, fmt.Errorf("torrentengine: DestDir is required")
	}
	if opts.LeakCheckInterval <= 0 {
		opts.LeakCheckInterval = 30 * time.Second
	}

	t := &Task{opts: opts, status: task.StatusPending}
	if opts.ProxyAddr == "" {
		t.warning = "no proxy configured for this torrent — your real IP address is visible to peers and trackers"
	}
	return t, nil
}

func (t *Task) ID() string     { return t.opts.ID }
func (t *Task) Engine() string { return "torrent" }

func (t *Task) Status() task.Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// Err returns the error that caused the task to fail (including a
// kill-switch trip), if any. Picked up by the scheduler's optional
// errorProvider capability, same as httpengine.Task and megalink.Task.
func (t *Task) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastErr
}

// Warning returns a non-fatal caution about this task, if any — currently
// just the "no proxy configured" notice. Picked up by the scheduler the
// same way Err() is, via an optional capability interface.
func (t *Task) Warning() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.warning
}

func (t *Task) Progress() task.Progress {
	t.mu.Lock()
	tor := t.tor
	t.mu.Unlock()

	if tor == nil {
		return task.Progress{}
	}
	total := tor.Length() // -1 until metadata (GotInfo) is available
	if total < 0 {
		total = 0
	}
	return task.Progress{TotalBytes: total, DownloadedBytes: tor.BytesCompleted()}
}

func (t *Task) Resume() error {
	t.mu.Lock()
	switch t.status {
	case task.StatusActive, task.StatusComplete, task.StatusCancelled:
		t.mu.Unlock()
		return nil
	}
	alreadyStarted := t.client != nil
	t.mu.Unlock()

	if alreadyStarted {
		t.mu.Lock()
		t.tor.AllowDataDownload()
		t.status = task.StatusActive
		t.mu.Unlock()
		return nil
	}

	return t.start()
}

// start creates the torrent.Client and adds the torrent — done once, the
// first time this task is resumed. Later Pause()/Resume() calls just
// toggle AllowDataDownload/DisallowDataDownload rather than recreating
// anything, which keeps peer connections and DHT state alive across a pause.
func (t *Task) start() error {
	cfg, err := buildClientConfig(t.opts)
	if err != nil {
		t.fail(err)
		return err
	}

	client, err := torrent.NewClient(cfg)
	if err != nil {
		t.fail(err)
		return err
	}

	if t.opts.ProxyAddr != "" {
		peerDialer, err := newSOCKS5Dialer(t.opts.ProxyAddr)
		if err != nil {
			client.Close()
			t.fail(err)
			return err
		}
		client.AddDialer(peerDialer)
	}

	tor, err := t.addTorrent(client)
	if err != nil {
		client.Close()
		t.fail(err)
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())

	t.mu.Lock()
	t.client = client
	t.tor = tor
	t.cancel = cancel
	t.status = task.StatusActive
	t.mu.Unlock()

	if t.opts.ProxyAddr != "" {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.runKillSwitch(ctx)
		}()
	}

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.runDownload(ctx, tor)
	}()

	if t.opts.SeedRatioLimit > 0 || t.opts.SeedTimeLimit > 0 {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.runSeedLimiter(ctx, tor)
		}()
	}

	return nil
}

func (t *Task) addTorrent(client *torrent.Client) (*torrent.Torrent, error) {
	if t.opts.MagnetURI != "" {
		return client.AddMagnet(t.opts.MagnetURI)
	}
	mi, err := metainfo.Load(bytes.NewReader(t.opts.TorrentBytes))
	if err != nil {
		return nil, fmt.Errorf("torrentengine: parsing .torrent file: %w", err)
	}
	return client.AddTorrent(mi)
}

// runDownload waits for the torrent's metadata before calling DownloadAll —
// calling it any earlier panics inside the library (Info is nil for a
// magnet link until a peer sends it), then waits for completion.
func (t *Task) runDownload(ctx context.Context, tor *torrent.Torrent) {
	select {
	case <-ctx.Done():
		return
	case <-tor.GotInfo():
	}

	tor.DownloadAll()

	select {
	case <-ctx.Done():
		return
	case <-tor.Complete().On():
	}
	t.mu.Lock()
	if t.status == task.StatusActive {
		t.status = task.StatusComplete
	}
	t.completedAt = time.Now()
	t.mu.Unlock()
}

// runKillSwitch periodically confirms torrent traffic is actually routed
// through the proxy. If it ever isn't, it stops the torrent immediately
// (rather than waiting for the current check interval's data to already be
// exposed) and marks the task as errored so the scheduler won't auto-resume it.
func (t *Task) runKillSwitch(ctx context.Context) {
	ticker := time.NewTicker(t.opts.LeakCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		result, err := DetectLeak(ctx, t.opts.ProxyAddr, t.opts.IPCheckURL)
		if err != nil {
			continue // transient checker failure (e.g. IP-echo service down) — don't trip on this alone
		}
		if result.Leaked {
			t.mu.Lock()
			if t.tor != nil {
				t.tor.DisallowDataDownload()
			}
			t.status = task.StatusError
			t.lastErr = fmt.Errorf("torrentengine: kill switch triggered — %s", result.Reason)
			t.mu.Unlock()
			return
		}
	}
}

func (t *Task) Pause() error {
	t.mu.Lock()
	if t.status != task.StatusActive || t.tor == nil {
		t.mu.Unlock()
		return nil
	}
	t.tor.DisallowDataDownload()
	t.status = task.StatusPaused
	t.mu.Unlock()
	return nil
}

func (t *Task) Cancel() error {
	t.mu.Lock()
	cancel := t.cancel
	client := t.client
	t.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	t.wg.Wait()
	if client != nil {
		client.Close()
	}

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
