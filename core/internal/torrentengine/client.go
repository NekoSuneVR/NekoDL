package torrentengine

import (
	"github.com/anacrolix/torrent"
	"golang.org/x/time/rate"
)

// buildClientConfig turns Options into a torrent.ClientConfig. DHT and PEX
// are left at their library defaults (both enabled) unless explicitly
// disabled — NekoDL doesn't need to do anything extra for those beyond not
// turning them off.
func buildClientConfig(opts Options) (*torrent.ClientConfig, error) {
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = opts.DestDir
	cfg.Seed = opts.Seed
	// NekoDL runs one torrent.Client per Task (see task.go), so every task
	// needs its own port — the library's fixed default (42069) would make
	// every task after the first fail to bind. Let the OS pick a free one.
	cfg.ListenPort = 0

	if opts.DisableDHT {
		cfg.NoDHT = true
	}
	if opts.DisablePEX {
		cfg.DisablePEX = true
	}

	if opts.MaxDownloadBps > 0 {
		cfg.DownloadRateLimiter = rate.NewLimiter(rate.Limit(opts.MaxDownloadBps), int(max(opts.MaxDownloadBps, 16*1024)))
	}
	if opts.MaxUploadBps > 0 {
		cfg.UploadRateLimiter = rate.NewLimiter(rate.Limit(opts.MaxUploadBps), int(max(opts.MaxUploadBps, 16*1024)))
	}

	if opts.ProxyAddr != "" {
		// uTP is UDP-based and wouldn't go through the SOCKS5 TCP CONNECT
		// dialer added to the *Client below — leaving it on would silently
		// bypass the proxy for some peer connections, so it's disabled
		// outright whenever a proxy is configured.
		cfg.DisableUTP = true

		httpDial, err := socks5HTTPDialContext(opts.ProxyAddr)
		if err != nil {
			return nil, err
		}
		cfg.HTTPDialContext = httpDial
	}

	return cfg, nil
}
