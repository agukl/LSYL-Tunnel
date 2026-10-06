//go:build windows

package gui

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

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

func (a *App) startAdminServer() (*http.Server, error) {
	mux := http.NewServeMux()
	a.registerAdminRoutes(mux)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	a.panelURL = "http://" + ln.Addr().String() + "/"
	tmpDir := filepath.Join(appTmpDir(a.workspace), "gui")
	_ = os.MkdirAll(tmpDir, 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "server-gui.url"), []byte(a.panelURL), 0o644)

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			a.appendLog("管理台服务异常: " + serveErr.Error())
		}
	}()
	return srv, nil
}
