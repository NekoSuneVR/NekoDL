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
	return s.requireAuthToken(next, func(r *http.Request) string {
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) {
			return ""
		}
		return strings.TrimPrefix(header, prefix)
	})
}

// requireAuthWS is requireAuth, plus accepting the token as a "?token="
// query parameter. Browsers' native WebSocket API can't set an Authorization
// header on the handshake request at all, so the events endpoint needs this
// fallback — every other endpoint stays header-only (a token in a URL query
// string can end up in logs/history, so this isn't extended beyond the one
// route that genuinely needs it).
func (s *Server) requireAuthWS(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuthToken(next, func(r *http.Request) string {
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if strings.HasPrefix(header, prefix) {
			return strings.TrimPrefix(header, prefix)
		}
		return r.URL.Query().Get("token")
	})
}

func (s *Server) requireAuthToken(next http.HandlerFunc, extract func(*http.Request) string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.APIToken == "" {
			next(w, r)
			return
		}

		provided := extract(r)
		if provided == "" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.APIToken)) != 1 {
			http.Error(w, "invalid bearer token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}
