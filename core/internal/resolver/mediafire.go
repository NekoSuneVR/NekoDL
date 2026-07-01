package resolver

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Mediafire has no public API for arbitrary files, so this scrapes the
// share page's HTML for its download button's href — the same category of
// approach as a yt-dlp site extractor, and just as fragile: it breaks
// whenever Mediafire changes their page markup.
//
// NOT independently live-verified: the only real Mediafire files findable
// during research were malware-flagged (an "unlock tool" and a modded APK),
// and Mediafire requires an account for uploads, so there was no safe way
// to fetch a real share page here. This is tested against a constructed
// mock page matching the documented pattern instead — see TODO.md.
type Mediafire struct {
	// Client is used to fetch the share page. Defaults to http.DefaultClient.
	Client *http.Client
}

func (Mediafire) Name() string { return "mediafire" }

func (Mediafire) CanResolve(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "mediafire.com" || host == "www.mediafire.com"
}

func (m Mediafire) Resolve(ctx context.Context, rawURL string) (Result, error) {
	client := m.Client
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}

	downloadURL, ok := extractMediafireDownloadURL(string(body))
	if !ok {
		return Result{}, fmt.Errorf("resolver: mediafire: could not find a download link on %q", rawURL)
	}
	return Result{URLs: []string{downloadURL}}, nil
}

var (
	mediafireButtonTag = regexp.MustCompile(`<a[^>]*id="downloadButton"[^>]*>`)
	hrefAttr           = regexp.MustCompile(`href="([^"]+)"`)
)

// extractMediafireDownloadURL matches the <a id="downloadButton" href="...">
// pattern regardless of attribute order within the tag.
func extractMediafireDownloadURL(page string) (string, bool) {
	tag := mediafireButtonTag.FindString(page)
	if tag == "" {
		return "", false
	}
	m := hrefAttr.FindStringSubmatch(tag)
	if m == nil {
		return "", false
	}
	return html.UnescapeString(m[1]), true
}
