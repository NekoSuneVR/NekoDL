package boothengine

import (
	"encoding/json"
	"os"
)

// boothConfig mirrors BoothDownloader's own BDConfig.json schema exactly
// (BoothConfig.cs in github.com/Myrkie/BoothDownloader) — just Cookie and
// AutoZip, nothing more. Confirmed against a real BDConfig.json written by
// a real run of the tool, not just its README's example.
type boothConfig struct {
	Cookie  string `json:"Cookie"`
	AutoZip bool   `json:"AutoZip"`
}

// anonymousCookie must match BoothDownloader's own BoothConfig.AnonymousCookie
// constant exactly. Writing an empty string instead makes BoothDownloader
// think no cookie has ever been configured, and it falls into an
// interactive "paste your cookie" console prompt — confirmed live: run
// against a subprocess with no real terminal attached, that prompt reads
// from stdin forever with nothing to satisfy it. Always writing a real
// value (real cookie or this constant) keeps the subprocess non-interactive.
const anonymousCookie = "ANONYMOUS"

// writeConfig creates BoothDownloader's own per-run config file at path.
// The cookie is a real access token — it's written here (a 0o600 file)
// rather than ever passed as a CLI argument, so it doesn't show up in a
// process listing (`ps`, Task Manager, /proc/<pid>/cmdline, etc).
func writeConfig(path, cookie string, autoZip bool) error {
	if cookie == "" {
		cookie = anonymousCookie
	}
	data, err := json.Marshal(boothConfig{Cookie: cookie, AutoZip: autoZip})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
