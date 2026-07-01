package httpengine

import (
	"encoding/json"
	"os"
)

// segmentRange tracks one byte range of a download. Current is the resume
// point: the next byte still needed. End == -1 means "unbounded" (server
// didn't support Range requests, so there's exactly one segment and no
// mid-file resume is possible).
type segmentRange struct {
	Start   int64 `json:"start"`
	Current int64 `json:"current"`
	End     int64 `json:"end"`
}

// progressSnapshot is the sidecar file (<dest>.nekodl-progress.json) that
// lets a download resume after a restart instead of starting over.
type progressSnapshot struct {
	URLs            []string       `json:"urls"`
	Dest            string         `json:"dest"`
	Total           int64          `json:"total"`
	RangesSupported bool           `json:"ranges_supported"`
	Segments        []segmentRange `json:"segments"`
}

func progressPath(dest string) string {
	return dest + ".nekodl-progress.json"
}

func saveProgress(snap progressSnapshot) error {
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(progressPath(snap.Dest), data, 0o644)
}

// loadProgress returns (nil, nil) if no snapshot exists yet — that's the
// normal case for a brand new download, not an error.
func loadProgress(dest string) (*progressSnapshot, error) {
	data, err := os.ReadFile(progressPath(dest))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var snap progressSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}
