//go:build windows

package gui

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	frontassets "lsyltunnel/src/client/front"
)

func (a *App) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/state", a.handleState)
	mux.HandleFunc("/api/health/check", a.handleHealthCheck)
	mux.HandleFunc("/api/start", a.handleStart)
	mux.HandleFunc("/api/stop", a.handleStop)
	mux.HandleFunc("/api/profile/switch", a.handleProfileSwitch)
	mux.HandleFunc("/api/mobile/export", a.handleMobileExport)
	mux.HandleFunc("/api/hide", a.handleHide)
	mux.HandleFunc("/api/quit", a.handleQuit)
	mux.HandleFunc("/api/window/minimize", a.handleWindowMinimize)
	mux.HandleFunc("/api/window/close", a.handleWindowClose)
	mux.HandleFunc("/api/window/drag", a.handleWindowDrag)
	mux.Handle("/", noStore(http.FileServer(http.FS(frontassets.FS))))
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
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
	_ = os.WriteFile(filepath.Join(tmpDir, "client-gui.url"), []byte(a.panelURL), 0o644)

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			a.appendLog("界面服务异常: " + serveErr.Error())
		}
	}()
	return srv, nil
}
