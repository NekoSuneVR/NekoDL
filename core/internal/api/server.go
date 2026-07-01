// Package api exposes NekoDL's HTTP API: task CRUD, an SSE progress feed,
// and (later) the aria2-compatible RPC shim. See TODO.md Phase 1/12.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/NekoSuneVR/NekoDL/core/internal/config"
	"github.com/NekoSuneVR/NekoDL/core/internal/resolver"
	"github.com/NekoSuneVR/NekoDL/core/internal/scheduler"
)

type Server struct {
	cfg       config.Config
	scheduler *scheduler.Scheduler
	resolvers *resolver.Registry
	mux       *http.ServeMux
}

// New builds the API server. resolvers may be nil to disable one-click-hoster resolution.
func New(cfg config.Config, sched *scheduler.Scheduler, resolvers *resolver.Registry) *Server {
	s := &Server{cfg: cfg, scheduler: sched, resolvers: resolvers, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	// Liveness check stays unauthenticated so external health monitors don't need the API token.
	s.mux.HandleFunc("/health", s.handleHealth)

	s.mux.HandleFunc("POST /api/v1/tasks", s.requireAuth(s.handleAddTask))
	s.mux.HandleFunc("POST /api/v1/torrents", s.requireAuth(s.handleAddTorrent))
	s.mux.HandleFunc("GET /api/v1/tasks", s.requireAuth(s.handleListTasks))
	s.mux.HandleFunc("GET /api/v1/tasks/{id}", s.requireAuth(s.handleGetTask))
	s.mux.HandleFunc("POST /api/v1/tasks/{id}/pause", s.requireAuth(s.handlePauseTask))
	s.mux.HandleFunc("POST /api/v1/tasks/{id}/resume", s.requireAuth(s.handleResumeTask))
	s.mux.HandleFunc("POST /api/v1/tasks/{id}/cancel", s.requireAuth(s.handleCancelTask))
	s.mux.HandleFunc("DELETE /api/v1/tasks/{id}", s.requireAuth(s.handleRemoveTask))
	s.mux.HandleFunc("GET /api/v1/events", s.requireAuth(s.handleEvents))

	if s.cfg.StaticDir != "" {
		s.mux.Handle("/", staticHandler(s.cfg.StaticDir))
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
