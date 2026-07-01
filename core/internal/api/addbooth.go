package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/NekoSuneVR/NekoDL/core/internal/boothengine"
	"github.com/NekoSuneVR/NekoDL/core/internal/scheduler"
)

type addBoothRequest struct {
	// Input is BoothDownloader's own --booth syntax verbatim: an item URL,
	// a bare item ID, or "gifts"/"orders"/"owned" (space-separated
	// combinations allowed) — see boothengine.Options.Input's doc comment.
	Input string `json:"input"`

	// Cookie is a Booth session token — a real account credential, treated
	// as a secret. Never logged; written to a 0o600 per-task config file,
	// never a CLI argument. Empty means anonymous (image downloads only).
	Cookie     string `json:"cookie"`
	AutoZip    bool   `json:"auto_zip"`
	MaxRetries int    `json:"max_retries"`
	Priority   int    `json:"priority"`
}

// handleAddBooth creates and starts a BoothDownloader run. Disabled with a
// clear error if the server has no BoothBinaryPath configured, rather than
// guessing at an install location the way ytdlpengine does for "yt-dlp" —
// BoothDownloader has no package-manager convention to fall back to.
func (s *Server) handleAddBooth(w http.ResponseWriter, r *http.Request) {
	if s.cfg.BoothBinaryPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "the Booth engine is not configured on this server (booth_binary_path is empty)",
		})
		return
	}

	var req addBoothRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Input == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input is required"})
		return
	}

	id := newTaskID()
	destDir := filepath.Join(s.cfg.DataDir, "booth", id)

	tk, err := boothengine.NewTask(boothengine.Options{
		ID:         id,
		Input:      req.Input,
		DestDir:    destDir,
		BinaryPath: s.cfg.BoothBinaryPath,
		Cookie:     req.Cookie,
		AutoZip:    req.AutoZip,
		MaxRetries: req.MaxRetries,
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
