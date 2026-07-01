// Package api exposes NekoDL's HTTP API: task CRUD, an SSE progress feed,
// and (later) the aria2-compatible RPC shim. See TODO.md Phase 1/12.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/NekoSuneVR/NekoDL/core/internal/config"
	"github.com/NekoSuneVR/NekoDL/core/internal/scheduler"
)

type Server struct {
	cfg       config.Config
	scheduler *scheduler.Scheduler
	mux       *http.ServeMux
}

func New(cfg config.Config, sched *scheduler.Scheduler) *Server {
	s := &Server{cfg: cfg, scheduler: sched, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	// Liveness check stays unauthenticated so external health monitors don't need the API token.
	s.mux.HandleFunc("/health", s.handleHealth)

	s.mux.HandleFunc("GET /api/v1/tasks", s.requireAuth(s.handleListTasks))
	s.mux.HandleFunc("GET /api/v1/tasks/{id}", s.requireAuth(s.handleGetTask))
	s.mux.HandleFunc("POST /api/v1/tasks/{id}/pause", s.requireAuth(s.handlePauseTask))
	s.mux.HandleFunc("POST /api/v1/tasks/{id}/resume", s.requireAuth(s.handleResumeTask))
	s.mux.HandleFunc("POST /api/v1/tasks/{id}/cancel", s.requireAuth(s.handleCancelTask))
	s.mux.HandleFunc("DELETE /api/v1/tasks/{id}", s.requireAuth(s.handleRemoveTask))
	s.mux.HandleFunc("GET /api/v1/events", s.requireAuth(s.handleEvents))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
