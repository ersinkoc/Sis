package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var assets embed.FS

// Handler serves the embedded WebUI assets with SPA fallback behavior.
func Handler() http.Handler {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := path.Clean("/" + r.URL.Path)
		if shouldServeIndex(cleanPath, sub) {
			w.Header().Set("Cache-Control", "no-cache")
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		applyCacheHeaders(w, cleanPath)
		fileServer.ServeHTTP(w, r)
	})
}

func shouldServeIndex(requestPath string, dist fs.FS) bool {
	if requestPath == "/" {
		return true
	}
	if strings.HasPrefix(requestPath, "/assets/") {
		return false
	}
	if path.Ext(requestPath) != "" {
		return false
	}
	name := strings.TrimPrefix(requestPath, "/")
	if name != "" {
		if _, err := fs.Stat(dist, name); err == nil {
			return false
		}
	}
	return true
}

func applyCacheHeaders(w http.ResponseWriter, requestPath string) {
	if strings.HasPrefix(requestPath, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
}
