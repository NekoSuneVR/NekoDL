package resolver

import (
	"context"
	"net/url"
	"strings"
)

// Dropbox turns a shared Dropbox link into a direct-download link. Dropbox
// share URLs already support this via a query parameter — no scraping needed,
// see TODO.md Phase 2.
type Dropbox struct{}

func (Dropbox) Name() string { return "dropbox" }

func (Dropbox) CanResolve(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "dropbox.com" || host == "www.dropbox.com"
}

func (d Dropbox) Resolve(_ context.Context, rawURL string) (Result, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return Result{}, err
	}
	q := u.Query()
	q.Set("dl", "1")
	u.RawQuery = q.Encode()
	return Result{URLs: []string{u.String()}}, nil
}
