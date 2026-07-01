package boothengine

import (
	"strings"
	"testing"
)

// Both fixtures below are real console output captured from real runs of
// the actual BoothDownloader-Window-64.exe v10.0.8 binary, not hand-written
// guesses at its format.

const realAnonymousDownloadFixture = `[12:14:08.235] [INFO] BoothDownloader: Booth Downloader - V10.0.8
[12:14:08.695] [WARN] BoothDownloader: Using anonymous cookie - Purchased file downloads will not function.
[12:14:08.705] [INFO] BoothDownloader: Grabbing the following booth Ids: 3807513

[12:14:10.476] [INFO] BoothDownloader: Downloading 3807513
[12:14:10.476] [INFO] BoothDownloader: Writing _BoothPage.json
[12:14:12.463] [INFO] BoothDownloader: ENVFileDIR: ./out\3807513
`

const realInvalidCookieGiftsFixture = `[12:17:18.677] [INFO] BoothDownloader: Booth Downloader - V10.0.8
[12:17:19.149] [WARN] BoothDownloader: Using anonymous cookie - Purchased file downloads will not function.
[12:17:19.185] [EROR] BoothDownloader: Cannot download Paid Items with invalid cookie.
[12:17:19.186] [INFO] BoothDownloader: Continuing in 5 seconds...
[12:17:24.187] [EROR] BoothDownloader: No items found to download, exiting in 5 seconds
`

func TestScanLogLinesAnonymousDownload(t *testing.T) {
	var lines []logLine
	scanLogLines(strings.NewReader(realAnonymousDownloadFixture), func(l logLine) {
		lines = append(lines, l)
	})

	if len(lines) != 6 {
		t.Fatalf("got %d lines, want 6 (blank separator line should be skipped): %+v", len(lines), lines)
	}
	if lines[1].Level != "WARN" || !strings.Contains(lines[1].Message, "Using anonymous cookie") {
		t.Fatalf("unexpected warn line: %+v", lines[1])
	}
	if lines[3].Level != "INFO" || lines[3].Message != "Downloading 3807513" {
		t.Fatalf("unexpected info line: %+v", lines[3])
	}
}

func TestScanLogLinesSurfacesErrorLevel(t *testing.T) {
	var errLines []string
	scanLogLines(strings.NewReader(realInvalidCookieGiftsFixture), func(l logLine) {
		if l.Level == "EROR" {
			errLines = append(errLines, l.Message)
		}
	})

	if len(errLines) != 2 {
		t.Fatalf("got %d EROR lines, want 2: %v", len(errLines), errLines)
	}
	if !strings.Contains(errLines[0], "Cannot download Paid Items") {
		t.Fatalf("unexpected first error line: %q", errLines[0])
	}
	if !strings.Contains(errLines[1], "No items found to download") {
		t.Fatalf("unexpected second error line: %q", errLines[1])
	}
}
