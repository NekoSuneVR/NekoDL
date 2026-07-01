package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// requireAuth wraps a handler so it 401s unless the request presents the
// configured API token as "Authorization: Bearer <token>". If no token is
// configured (Config.APIToken == ""), auth is disabled and every request passes.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.APIToken == "" {
			next(w, r)
			return
		}

		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		provided := strings.TrimPrefix(header, prefix)
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.APIToken)) != 1 {
			http.Error(w, "invalid bearer token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}
