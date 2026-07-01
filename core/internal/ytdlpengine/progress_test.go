package ytdlpengine

import (
	"strings"
	"testing"
)

// These fixture lines are copied verbatim from a real
// `yt-dlp --newline --progress-template "download:%(progress)j"` run
// against a real video, not hand-written — see TODO.md Phase 4.
const realProgressFixture = `[youtube] Extracting URL: https://www.youtube.com/watch?v=jNQXAC9IVRw
[youtube] jNQXAC9IVRw: Downloading webpage
[info] jNQXAC9IVRw: Downloading 1 format(s): 18
[download] Destination: test_video.mp4
{"status": "downloading", "downloaded_bytes": 1024, "total_bytes": 629172, "tmpfilename": "test_video.mp4.part", "filename": "test_video.mp4", "eta": null, "speed": null, "elapsed": 0.126}
{"status": "downloading", "downloaded_bytes": 261120, "total_bytes": 629172, "tmpfilename": "test_video.mp4.part", "filename": "test_video.mp4", "eta": 0, "speed": 7646398.94, "elapsed": 0.16}
{"status": "downloading", "downloaded_bytes": 629172, "total_bytes": 629172, "tmpfilename": "test_video.mp4.part", "filename": "test_video.mp4", "eta": 0, "speed": 7951891.7, "elapsed": 0.205}
{"downloaded_bytes": 629172, "total_bytes": 629172, "filename": "test_video.mp4", "status": "finished", "elapsed": 0.207, "speed": 3042523.26}
`

func TestScanProgressParsesRealFixture(t *testing.T) {
	var updates []progressLine
	scanProgress(strings.NewReader(realProgressFixture), func(p progressLine) {
		updates = append(updates, p)
	})

	if len(updates) != 4 {
		t.Fatalf("expected 4 progress updates, got %d", len(updates))
	}

	first := updates[0]
	if first.Status != "downloading" || first.DownloadedBytes != 1024 {
		t.Fatalf("unexpected first update: %+v", first)
	}
	if first.TotalBytes == nil || *first.TotalBytes != 629172 {
		t.Fatalf("expected TotalBytes=629172, got %+v", first.TotalBytes)
	}
	if first.Speed != nil {
		t.Fatalf("expected a nil Speed for the first sample (yt-dlp emits JSON null until it has enough data), got %v", *first.Speed)
	}

	last := updates[3]
	if last.Status != "finished" || last.DownloadedBytes != 629172 {
		t.Fatalf("unexpected final update: %+v", last)
	}
}

func TestScanProgressIgnoresNonJSONLines(t *testing.T) {
	var count int
	scanProgress(strings.NewReader("[youtube] just some status text\nnot json either\n"), func(progressLine) {
		count++
	})
	if count != 0 {
		t.Fatalf("expected 0 progress updates from non-JSON lines, got %d", count)
	}
}

func TestScanProgressIgnoresMalformedJSONLine(t *testing.T) {
	var count int
	scanProgress(strings.NewReader(`{"status": "downloading", not valid json`+"\n"), func(progressLine) {
		count++
	})
	if count != 0 {
		t.Fatalf("expected malformed JSON to be skipped, not crash, got %d updates", count)
	}
}
