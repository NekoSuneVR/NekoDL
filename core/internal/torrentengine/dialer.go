package torrentengine

import (
	"context"
	"fmt"
	"net"

	"github.com/anacrolix/torrent/dialer"
	"golang.org/x/net/proxy"
)

// socks5Dialer adapts golang.org/x/net/proxy's SOCKS5 client dialer to
// anacrolix/torrent's dialer.T interface, so peer connections (not just
// tracker HTTP requests) go through the proxy.
type socks5Dialer struct {
	inner   proxy.ContextDialer
	network string
}

func (d *socks5Dialer) Dial(ctx context.Context, addr string) (net.Conn, error) {
	return d.inner.DialContext(ctx, d.network, addr)
}

func (d *socks5Dialer) DialerNetwork() string { return d.network }

// newSOCKS5Dialer builds a dialer.T that routes connections through a
// SOCKS5 proxy at proxyAddr (host:port, no auth).
func newSOCKS5Dialer(proxyAddr string) (dialer.T, error) {
	base, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("torrentengine: creating SOCKS5 dialer: %w", err)
	}
	cd, ok := base.(proxy.ContextDialer)
	if !ok {
		// Not expected in practice — x/net/proxy's SOCKS5 dialer implements
		// this — but fail clearly rather than silently dialing without a
		// context (which would ignore cancellation/timeouts).
		return nil, fmt.Errorf("torrentengine: SOCKS5 dialer does not support context-aware dialing")
	}
	return &socks5Dialer{inner: cd, network: "tcp"}, nil
}

// socks5HTTPDialContext returns a DialContext func suitable for
// ClientConfig.HTTPDialContext / http.Transport.DialContext, routing
// tracker/webseed HTTP requests through the same SOCKS5 proxy.
func socks5HTTPDialContext(proxyAddr string) (func(ctx context.Context, network, addr string) (net.Conn, error), error) {
	base, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("torrentengine: creating SOCKS5 HTTP dialer: %w", err)
	}
	cd, ok := base.(proxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("torrentengine: SOCKS5 dialer does not support context-aware dialing")
	}
	return cd.DialContext, nil
}
