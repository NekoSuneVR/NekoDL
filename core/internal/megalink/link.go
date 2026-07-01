// Package megalink downloads files from public mega.nz share links.
//
// This doesn't fit the resolver.Resolver pattern used for Dropbox/Pixeldrain:
// MEGA's temporary download URL serves AES-encrypted ciphertext, so
// decryption has to happen as part of streaming the download, not before it
// (there's no "direct URL" to just hand to the HTTP engine). See TODO.md
// Phase 2 for the full reasoning, what's verified vs. not, and why no
// existing Go library was used (the actively-maintained one doesn't support
// public links at all; the ones that do haven't been touched since 2019-2021).
//
// The key-splitting/CTR/attribute-decryption scheme and the MEGA API
// request shape here are transcribed from a long-standing, widely-used
// reference implementation (github.com/juanriaza/python-mega), not
// reverse-engineered from scratch or from memory. The crypto mechanics are
// unit-tested against Go's own stdlib AES implementation as an independent
// check. What is NOT independently verified is whether the real mega.nz API
// still matches that request/response shape today — no MEGA account or safe
// public test file was available to check live when this was written.
package megalink

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseLink extracts the file handle and base64 key from a public mega.nz
// file link, supporting both the modern (/file/{handle}#{key}) and legacy
// (#!{handle}!{key}) URL formats.
func ParseLink(rawURL string) (handle, keyB64 string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}

	host := strings.ToLower(u.Hostname())
	if !strings.HasSuffix(host, "mega.nz") && !strings.HasSuffix(host, "mega.co.nz") {
		return "", "", fmt.Errorf("megalink: %q is not a mega.nz URL", rawURL)
	}

	frag := u.Fragment
	if strings.HasPrefix(frag, "!") {
		parts := strings.SplitN(strings.TrimPrefix(frag, "!"), "!", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("megalink: malformed legacy link fragment in %q", rawURL)
		}
		return parts[0], parts[1], nil
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) == 2 && segments[0] == "file" && segments[1] != "" && frag != "" {
		return segments[1], frag, nil
	}

	return "", "", fmt.Errorf("megalink: could not find a file handle and key in %q", rawURL)
}

// CanResolve reports whether rawURL looks like a mega.nz file link at all,
// without fully validating it — mirrors resolver.Resolver.CanResolve so
// callers can use the same "does anything claim this URL" dispatch pattern.
func CanResolve(rawURL string) bool {
	_, _, err := ParseLink(rawURL)
	return err == nil
}
