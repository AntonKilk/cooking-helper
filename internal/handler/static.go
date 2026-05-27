package handler

import (
	"io/fs"
	"net/http"
)

// StaticFiles serves the embedded front-end assets (CSS, vendored HTMX, icons)
// under the /static/ prefix.
func StaticFiles(fsys fs.FS) http.Handler {
	return http.StripPrefix("/static/", http.FileServerFS(fsys))
}

// ServiceWorker serves sw.js from the application root so its scope covers the
// whole origin. It is served separately from /static/ for that reason.
func ServiceWorker(fsys fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(fsys, "sw.js")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(data)
	}
}

// Manifest serves the PWA manifest with an explicit Content-Type, since
// .webmanifest is not in Go's default MIME database.
func Manifest(fsys fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(fsys, "manifest.webmanifest")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/manifest+json")
		_, _ = w.Write(data)
	}
}
