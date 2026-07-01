// Package api exposes NekoDL's HTTP API. It's a thin skeleton for now —
// just a health check — that later phases extend with task CRUD, WebSocket
// progress events, and the aria2-compatible RPC shim.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/NekoSuneVR/NekoDL/core/internal/config"
)

type Server struct {
	cfg config.Config
	mux *http.ServeMux
}

func New(cfg config.Config) *Server {
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
