package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"path/filepath"

	"github.com/NekoSuneVR/NekoDL/core/internal/httpengine"
	"github.com/NekoSuneVR/NekoDL/core/internal/resolver"
	"github.com/NekoSuneVR/NekoDL/core/internal/scheduler"
)

type addTaskRequest struct {
	URL            string           `json:"url"`
	MaxConnections int              `json:"max_connections"`
	Priority       int              `json:"priority"`
	Checksum       *checksumRequest `json:"checksum,omitempty"`
}

type checksumRequest struct {
	Algo     string `json:"algo"`
	Expected string `json:"expected"`
}

// handleAddTask creates and starts an HTTP download. If the URL belongs to
// a known one-click hoster (Dropbox, Pixeldrain, ...) it's resolved to a
// real direct-download URL first; otherwise it's downloaded as-is.
func (s *Server) handleAddTask(w http.ResponseWriter, r *http.Request) {
	var req addTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}

	urls := []string{req.URL}
	if s.resolvers != nil {
		result, err := s.resolvers.Resolve(r.Context(), req.URL)
		switch {
		case err == nil:
			urls = result.URLs
		case err == resolver.ErrNotSupported:
			// Not a known hoster — fetch the URL as given.
		default:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
	}

	id := newTaskID()
	dest := filepath.Join(s.cfg.DataDir, "downloads", taskFilename(id, urls[0]))

	var checksum *httpengine.Checksum
	if req.Checksum != nil {
		checksum = &httpengine.Checksum{Algo: req.Checksum.Algo, Expected: req.Checksum.Expected}
	}

	tk, err := httpengine.New(httpengine.Options{
		ID:             id,
		URLs:           urls,
		DestPath:       dest,
		MaxConnections: req.MaxConnections,
		Checksum:       checksum,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.scheduler.Enqueue(tk, scheduler.Options{Priority: req.Priority})
	if err := tk.Resume(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func newTaskID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf) // crypto/rand.Read on a fixed-size buffer never returns an error in practice
	return hex.EncodeToString(buf)
}

// taskFilename derives a destination filename from the URL's last path
// segment, prefixed with the task ID so two tasks downloading same-named
// files never collide.
func taskFilename(id, rawURL string) string {
	name := "download"
	if u, err := url.Parse(rawURL); err == nil {
		if base := path.Base(u.Path); base != "" && base != "/" && base != "." {
			name = base
		}
	}
	return id + "-" + name
}
