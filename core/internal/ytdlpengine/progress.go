package ytdlpengine

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// progressLine is yt-dlp's own JSON progress schema, captured from a real
// run (`yt-dlp --newline --progress-template "download:%(progress)j"`) —
// not guessed from docs. total_bytes/speed are pointers because yt-dlp
// emits a literal JSON null for both until it has enough samples to know
// them (confirmed in the first line of a real run), which would otherwise
// silently unmarshal to a misleading 0.
type progressLine struct {
	Status          string   `json:"status"`
	DownloadedBytes int64    `json:"downloaded_bytes"`
	TotalBytes      *int64   `json:"total_bytes"`
	Speed           *float64 `json:"speed"`
	Filename        string   `json:"filename"`
}

// scanProgress reads r line by line, forwarding parsed progress updates to
// onProgress and returning once r is exhausted (the subprocess exited) or
// ctx-driven cancellation closes the pipe out from under it. Lines that
// aren't a progress JSON object (yt-dlp's normal human-readable status
// lines, e.g. "[youtube] Extracting URL: ...") are ignored, not errors.
func scanProgress(r io.Reader, onProgress func(progressLine)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var p progressLine
		if err := json.Unmarshal([]byte(line), &p); err != nil {
			continue // a stray non-progress line that happened to start with '{' — ignore rather than fail the whole task over it
		}
		onProgress(p)
	}
}
