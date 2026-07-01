package resolver

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// GoogleDrive resolves a shared Google Drive file link to a direct-download
// URL. Confirmed live (see TODO.md Phase 2) that drive.google.com/uc?export=
// download&id={id} 303-redirects to drive.usercontent.google.com/download —
// Go's http.Client follows that automatically, so this covers small files
// with no extra code.
//
// NOT handled: Google's "can't scan this file for viruses, download anyway?"
// interstitial for large files, which needs a confirm-token exchange. No
// Google account/browser was available to trigger and verify that path live,
// so it's left unimplemented rather than guessed at — see TODO.md.
type GoogleDrive struct{}

func (GoogleDrive) Name() string { return "google-drive" }

func (GoogleDrive) CanResolve(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if strings.ToLower(u.Hostname()) != "drive.google.com" {
		return false
	}
	_, ok := googleDriveFileID(u)
	return ok
}

func (g GoogleDrive) Resolve(_ context.Context, rawURL string) (Result, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return Result{}, err
	}
	id, ok := googleDriveFileID(u)
	if !ok {
		return Result{}, fmt.Errorf("resolver: google-drive: could not find a file ID in %q", rawURL)
	}
	return Result{URLs: []string{"https://drive.google.com/uc?export=download&id=" + id}}, nil
}

var googleDriveFileDPattern = regexp.MustCompile(`^/file/d/([^/]+)`)

// googleDriveFileID handles both link shapes Google Drive hands out:
// /file/d/{id}/view and /uc?id={id} (or /open?id={id}).
func googleDriveFileID(u *url.URL) (string, bool) {
	if m := googleDriveFileDPattern.FindStringSubmatch(u.Path); m != nil {
		return m[1], true
	}
	if id := u.Query().Get("id"); id != "" {
		return id, true
	}
	return "", false
}
