//go:build windows

package gui

import (
	"net/http"
	"strings"

	frontassets "lsyltunnel/src/server/front"
)

func (a *App) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/state", a.handleAdminState)
	mux.HandleFunc("/api/config", a.handleAdminConfig)
	mux.HandleFunc("/api/password/hash", a.handleAdminPasswordHash)
	mux.HandleFunc("/api/service/restart", a.handleAdminServiceRestart)
	mux.HandleFunc("/api/security/unblock", a.handleAdminUnblockIP)
	mux.HandleFunc("/api/security/permanent-block/delete", a.handleAdminDeletePermanentBlock)
	mux.HandleFunc("/api/log-analysis", a.handleAdminLogAnalysis)
	mux.Handle("/", noStore(http.FileServer(http.FS(frontassets.FS))))
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

func (a *App) adminHandler() http.Handler {
	// Only hand this handler to Wails' in-process asset server, never net/http.Serve.
	mux := http.NewServeMux()
	a.registerAdminRoutes(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if r.Host != "wails.localhost" ||
				(r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://wails.localhost") ||
				(r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin") {
				http.Error(w, "invalid panel origin", http.StatusForbidden)
				return
			}
			if r.ContentLength > 1<<20 {
				http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		}
		mux.ServeHTTP(w, r)
	})
}
