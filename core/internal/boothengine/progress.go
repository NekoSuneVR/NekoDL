package boothengine

import (
	"bufio"
	"io"
	"regexp"
)

// logLinePattern matches BoothDownloader's own Serilog console output
// template (LoggerHelper.cs: "[{Timestamp:HH:mm:ss.fff}] [{Level:u4}] "),
// confirmed against real runs of the actual binary, e.g.:
//
//	[12:14:08.695] [WARN] BoothDownloader: Using anonymous cookie - ...
//	[12:17:19.185] [EROR] BoothDownloader: Cannot download Paid Items with invalid cookie.
var logLinePattern = regexp.MustCompile(`^\[[\d:.]+\]\s+\[(\w+)\]\s+BoothDownloader:\s+(.*)$`)

type logLine struct {
	Level   string
	Message string
}

// scanLogLines reads BoothDownloader's stdout, calling onLine for every
// recognized log line. Lines that don't match (the blank separators it
// prints between phases, or a progress-bar line if one ever reaches stdout
// in a piped/non-TTY context — none were observed to in real testing) are
// silently skipped rather than erroring: this line format isn't a
// documented, stable contract, just an observed one, so unrecognized
// output degrades to "no extra info" instead of failing the task.
func scanLogLines(r io.Reader, onLine func(logLine)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		m := logLinePattern.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}
		onLine(logLine{Level: m[1], Message: m[2]})
	}
}
