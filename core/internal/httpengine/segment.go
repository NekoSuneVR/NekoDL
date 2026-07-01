package httpengine

import (
	"context"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const minSegmentSize = 1 << 20 // 1 MiB — don't bother splitting small files into many connections

// probe issues a Range: bytes=0-0 request to learn the file size and
// whether the server honors byte ranges at all.
func probe(ctx context.Context, client *http.Client, url string) (total int64, rangesSupported bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, false, err
	}
	req.Header.Set("Range", "bytes=0-0")

	resp, err := client.Do(req)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	switch resp.StatusCode {
	case http.StatusPartialContent:
		return parseContentRangeTotal(resp.Header.Get("Content-Range")), true, nil
	case http.StatusOK:
		return resp.ContentLength, false, nil
	default:
		return 0, false, &statusError{resp.StatusCode}
	}
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
