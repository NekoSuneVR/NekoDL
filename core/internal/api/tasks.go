package api

import (
	"encoding/json"
	"net/http"

	"github.com/NekoSuneVR/NekoDL/core/internal/scheduler"
)

func (s *Server) handleListTasks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.scheduler.Records())
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	record, err := s.scheduler.Get(r.PathValue("id"))
	if err != nil {
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handlePauseTask(w http.ResponseWriter, r *http.Request) {
	s.taskAction(w, r, s.scheduler.Pause)
}

func (s *Server) handleResumeTask(w http.ResponseWriter, r *http.Request) {
	s.taskAction(w, r, s.scheduler.Resume)
}

func (s *Server) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	s.taskAction(w, r, s.scheduler.Cancel)
}

func (s *Server) handleRemoveTask(w http.ResponseWriter, r *http.Request) {
	s.taskAction(w, r, s.scheduler.Remove)
}

func (s *Server) taskAction(w http.ResponseWriter, r *http.Request, action func(string) error) {
	if err := action(r.PathValue("id")); err != nil {
		writeTaskError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeTaskError(w http.ResponseWriter, err error) {
	if err == scheduler.ErrTaskNotFound {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
