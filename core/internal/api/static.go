package api

import (
	"net/http"
	"os"
	"path/filepath"
)

// staticHandler serves a built SPA (web/dist) from dir, falling back to
// index.html for any path that doesn't match a real file — standard SPA
// client-side-routing behavior. Registered on "/", which Go's ServeMux
// only reaches for requests that don't match a more specific registered
// pattern (like "/api/v1/..." or "/health"), so it can't shadow the API.
func staticHandler(dir string) http.Handler {
	fileServer := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
