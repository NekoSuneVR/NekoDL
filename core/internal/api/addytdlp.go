package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/NekoSuneVR/NekoDL/core/internal/scheduler"
	"github.com/NekoSuneVR/NekoDL/core/internal/ytdlpengine"
)

type addYtdlpRequest struct {
	URL            string `json:"url"`
	Format         string `json:"format"`
	Resolution     string `json:"resolution"`   // e.g. "1080" — ignored if Format is set
	AudioFormat    string `json:"audio_format"` // e.g. "mp3", "wav" — extracts audio only via yt-dlp's ffmpeg-backed -x
	NoPlaylist     bool   `json:"no_playlist"`
	Subtitles      bool   `json:"subtitles"`
	OutputTemplate string `json:"output_template"`

	// ProxyAddr is opt-in and unwarned, unlike torrents — see
	// ytdlpengine.Options.ProxyAddr's doc comment for why.
	ProxyAddr string `json:"proxy_addr"`

	// CookiesFileBase64 is a cookies.txt (Netscape format) file's contents,
	// base64-encoded since JSON can't carry raw bytes — same UX pattern as
	// the Booth engine's cookie/token input, for sites requiring login.
	CookiesFileBase64 string `json:"cookies_file_base64"`

	Priority int `json:"priority"`
}

func (s *Server) handleAddYtdlp(w http.ResponseWriter, r *http.Request) {
	var req addYtdlpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}

	id := newTaskID()
	destDir := filepath.Join(s.cfg.DataDir, "ytdlp", id)

	var cookiesFile string
	if req.CookiesFileBase64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(req.CookiesFileBase64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid cookies_file_base64"})
			return
		}
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		cookiesFile = filepath.Join(destDir, "cookies.txt")
		if err := os.WriteFile(cookiesFile, decoded, 0o600); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	tk, err := ytdlpengine.NewTask(ytdlpengine.Options{
		ID:             id,
		URL:            req.URL,
		DestDir:        destDir,
		Format:         req.Format,
		Resolution:     req.Resolution,
		AudioFormat:    req.AudioFormat,
		NoPlaylist:     req.NoPlaylist,
		Subtitles:      req.Subtitles,
		OutputTemplate: req.OutputTemplate,
		ProxyAddr:      req.ProxyAddr,
		CookiesFile:    cookiesFile,
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
