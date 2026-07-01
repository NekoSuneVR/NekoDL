package torrentengine

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// defaultIPCheckURL is a plain-text public-IP echo service, used to compare
// the real IP against what's visible when dialing through a proxy.
const defaultIPCheckURL = "https://api.ipify.org"

// publicIP fetches the caller's apparent public IP as seen by ipCheckURL,
// using client to make the request (a proxied client sees the proxy's IP;
// http.DefaultClient sees the real one).
func publicIP(ctx context.Context, client *http.Client, ipCheckURL string) (string, error) {
	if ipCheckURL == "" {
		ipCheckURL = defaultIPCheckURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ipCheckURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("torrentengine: IP check returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// LeakCheckResult is the outcome of comparing direct vs. proxied public IP.
type LeakCheckResult struct {
	DirectIP  string
	ProxiedIP string
	// Leaked is true if the proxied request either failed (the proxy isn't
	// working) or returned the same IP as the direct request (the proxy is
	// reachable but isn't actually routing traffic) — either way, torrent
	// traffic through this proxy would be exposing the real IP.
	Leaked bool
	Reason string
}

// DetectLeak compares the public IP seen with and without proxyAddr. It's
// used both as a one-shot pre-flight check and, periodically, as the kill
// switch: if this ever reports Leaked, torrent traffic should stop.
func DetectLeak(ctx context.Context, proxyAddr, ipCheckURL string) (LeakCheckResult, error) {
	directIP, err := publicIP(ctx, http.DefaultClient, ipCheckURL)
	if err != nil {
		return LeakCheckResult{}, fmt.Errorf("torrentengine: checking direct IP: %w", err)
	}

	dialCtx, err := socks5HTTPDialContext(proxyAddr)
	if err != nil {
		return LeakCheckResult{}, err
	}
	proxiedClient := &http.Client{Transport: &http.Transport{DialContext: dialCtx}}

	proxiedIP, err := publicIP(ctx, proxiedClient, ipCheckURL)
	if err != nil {
		return LeakCheckResult{
			DirectIP: directIP,
			Leaked:   true,
			Reason:   fmt.Sprintf("proxy request failed: %v", err),
		}, nil
	}

	if proxiedIP == directIP {
		return LeakCheckResult{
			DirectIP:  directIP,
			ProxiedIP: proxiedIP,
			Leaked:    true,
			Reason:    "proxied request returned the same IP as the direct request — traffic is not actually being routed through the proxy",
		}, nil
	}

	return LeakCheckResult{DirectIP: directIP, ProxiedIP: proxiedIP, Leaked: false}, nil
}
