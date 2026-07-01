// Package task defines the common interface every download engine
// (HTTP, BitTorrent, yt-dlp, Booth, Plex ripper, ...) implements, so the
// core scheduler and API can manage them all the same way.
package task

// Status is the lifecycle state of a task.
type Status string

const (
	StatusPending   Status = "pending"
	StatusActive    Status = "active"
	StatusPaused    Status = "paused"
	StatusComplete  Status = "complete"
	StatusError     Status = "error"
	StatusCancelled Status = "cancelled"
)

// Progress is a point-in-time snapshot of a task's transfer state.
type Progress struct {
	TotalBytes      int64
	DownloadedBytes int64
	SpeedBytesPerS  int64
}

// Task is implemented by every download engine's task type.
type Task interface {
	ID() string
	Engine() string
	Pause() error
	Resume() error
	Cancel() error
	Remove() error
	Progress() Progress
	Status() Status
}
