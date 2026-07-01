package httpengine

import (
	"context"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
)

const minSegmentSize = 1 << 20 // 1 MiB — don't bother splitting small files into many connections

// probe issues a Range: bytes=0-0 request to learn the file size, whether
// the server honors byte ranges at all, and — if present — the real
// filename from Content-Disposition. That header matters most for
// one-click-hoster resolvers (Google Drive, Dropbox, ...): their direct
// URLs are opaque (e.g. drive.google.com/uc?id=...), so a filename derived
// from the URL path alone is meaningless (confirmed live: Google Drive
// produced "<task-id>-uc" instead of the file's real name).
func probe(ctx context.Context, client *http.Client, url string) (total int64, rangesSupported bool, filename string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, false, "", err
	}
	req.Header.Set("Range", "bytes=0-0")

	resp, err := client.Do(req)
	if err != nil {
		return 0, false, "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	filename = contentDispositionFilename(resp.Header.Get("Content-Disposition"))

	switch resp.StatusCode {
	case http.StatusPartialContent:
		return parseContentRangeTotal(resp.Header.Get("Content-Range")), true, filename, nil
	case http.StatusOK:
		return resp.ContentLength, false, filename, nil
	default:
		return 0, false, "", &statusError{resp.StatusCode}
	}
}

// contentDispositionFilename extracts a server-suggested filename from a
// Content-Disposition header, preferring the RFC 5987 filename* (percent-
// encoded, charset-aware) form over plain filename= when both are present.
// Returns "" if the header is absent, unparseable, or names nothing usable.
func contentDispositionFilename(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	if star := params["filename*"]; star != "" {
		if name := decodeRFC5987(star); name != "" {
			return sanitizeFilename(name)
		}
	}
	return sanitizeFilename(params["filename"])
}

// decodeRFC5987 decodes the "charset'lang'percent-encoded-value" form used
// by filename*, e.g. "UTF-8''Terms%20of%20Use.pdf". Only UTF-8 is handled —
// the only charset any of NekoDL's resolvers have been observed to send.
func decodeRFC5987(v string) string {
	parts := strings.SplitN(v, "'", 3)
	if len(parts) != 3 {
		return ""
	}
	decoded, err := url.QueryUnescape(parts[2])
	if err != nil {
		return ""
	}
	return decoded
}

// sanitizeFilename defends against a malicious/misbehaving server naming a
// file "../../etc/passwd" or "..\\..\\windows\\win.ini" via
// Content-Disposition: strips any directory components (both slash
// styles — Content-Disposition is a wire value, not an OS path, so this
// uses "path" rather than the OS-specific "path/filepath") and leading
// dots, keeping just a plain basename.
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(path.Clean(name))
	name = strings.TrimLeft(name, ".")
	if name == "" || name == "." || name == "/" {
		return ""
	}
	return name
}

func parseContentRangeTotal(header string) int64 {
	idx := strings.LastIndex(header, "/")
	if idx == -1 {
		return -1
	}
	total, err := strconv.ParseInt(header[idx+1:], 10, 64)
	if err != nil {
		return -1
	}
	return total
}

// buildSegments divides [0, total-1] into up to maxConn contiguous ranges.
// If the server doesn't support ranges, or the size is unknown, there is
// exactly one unbounded segment and no mid-file resume is possible.
func buildSegments(total int64, maxConn int, rangesSupported bool) []segmentRange {
	if !rangesSupported || total <= 0 {
		return []segmentRange{{Start: 0, Current: 0, End: -1}}
	}
	if maxConn < 1 {
		maxConn = 1
	}

	n := maxConn
	if byPerSegmentSize := int(total / minSegmentSize); byPerSegmentSize < n {
		n = byPerSegmentSize
	}
	if n < 1 {
		n = 1
	}

	segments := make([]segmentRange, 0, n)
	size := total / int64(n)
	start := int64(0)
	for i := 0; i < n; i++ {
		end := start + size - 1
		if i == n-1 {
			end = total - 1
		}
		segments = append(segments, segmentRange{Start: start, Current: start, End: end})
		start = end + 1
	}
	return segments
}

func preallocate(path string, size int64) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Truncate(size)
}

type statusError struct{ code int }

func (e *statusError) Error() string {
	return "httpengine: unexpected HTTP status " + strconv.Itoa(e.code)
}
