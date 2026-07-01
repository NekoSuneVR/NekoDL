package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// handleEvents streams the current task list as Server-Sent Events once a
// second. This is a stand-in for a real WebSocket feed: implementing RFC 6455
// by hand, or vendoring a WebSocket library, wasn't worth the risk without a
// Go toolchain available to verify it (see TODO.md Phase 1). SSE is push-based
// and stdlib-only, which covers "live updates" for now.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			data, err := json.Marshal(s.scheduler.Records())
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
