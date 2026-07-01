package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/NekoSuneVR/NekoDL/core/internal/scheduler"
	"github.com/NekoSuneVR/NekoDL/core/internal/torrentengine"
)

type addTorrentRequest struct {
	MagnetURI         string `json:"magnet_uri"`
	TorrentFileBase64 string `json:"torrent_file_base64"`

	// ProxyAddr, if set, routes this torrent through a SOCKS5 proxy
	// (host:port, no auth). If empty, the torrent uses the real IP
	// directly — the response includes a warning when that happens.
	ProxyAddr string `json:"proxy_addr"`

	MaxDownloadBps int64 `json:"max_download_bps"`
	MaxUploadBps   int64 `json:"max_upload_bps"`
	Seed           bool  `json:"seed"`
	Priority       int   `json:"priority"`
}

// handleAddTorrent creates and starts a BitTorrent download from a magnet
// URI or an uploaded .torrent file's bytes (base64-encoded, since JSON
// can't carry raw binary). See torrentengine's package doc for the privacy
// model (SOCKS5 binding, kill switch) this wires up.
func (s *Server) handleAddTorrent(w http.ResponseWriter, r *http.Request) {
	var req addTorrentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	var torrentBytes []byte
	if req.TorrentFileBase64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(req.TorrentFileBase64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid torrent_file_base64"})
			return
		}
		torrentBytes = decoded
	}

	id := newTaskID()
	destDir := filepath.Join(s.cfg.DataDir, "torrents", id)

	tk, err := torrentengine.NewTask(torrentengine.Options{
		ID:             id,
		MagnetURI:      req.MagnetURI,
		TorrentBytes:   torrentBytes,
		DestDir:        destDir,
		ProxyAddr:      req.ProxyAddr,
		MaxDownloadBps: req.MaxDownloadBps,
		MaxUploadBps:   req.MaxUploadBps,
		Seed:           req.Seed,
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

	resp := map[string]string{"id": id}
	if warning := tk.Warning(); warning != "" {
		resp["warning"] = warning
	}
	writeJSON(w, http.StatusCreated, resp)
}
