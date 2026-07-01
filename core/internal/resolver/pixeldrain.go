package resolver

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Pixeldrain has a real public API for downloads (confirmed against its own
// docs, see TODO.md Phase 2): GET /api/file/{id} returns the raw file with
// no auth and byte-range support — no scraping needed.
type Pixeldrain struct{}

func (Pixeldrain) Name() string { return "pixeldrain" }

func (Pixeldrain) CanResolve(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if strings.ToLower(u.Hostname()) != "pixeldrain.com" {
		return false
	}
	_, ok := pixeldrainFileID(u)
	return ok
}

func (p Pixeldrain) Resolve(_ context.Context, rawURL string) (Result, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return Result{}, err
	}
	id, ok := pixeldrainFileID(u)
	if !ok {
		return Result{}, fmt.Errorf("resolver: pixeldrain: could not find a file ID in %q", rawURL)
	}
	return Result{URLs: []string{"https://pixeldrain.com/api/file/" + id}}, nil
}

// pixeldrainFileID extracts the ID from a share link like
// pixeldrain.com/u/{id} or an already-direct pixeldrain.com/api/file/{id}.
func pixeldrainFileID(u *url.URL) (string, bool) {
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch {
	case len(parts) == 2 && parts[0] == "u":
		return parts[1], true
	case len(parts) == 3 && parts[0] == "api" && parts[1] == "file":
		return parts[2], true
	default:
		return "", false
	}
}
