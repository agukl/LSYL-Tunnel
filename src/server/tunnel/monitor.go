package tunnel

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
)

func (s *Server) startMonitor(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.status())
	})
	mux.HandleFunc("/security/unblock", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "method not allowed"})
			return
		}
		var req struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "invalid request"})
			return
		}
		ip := strings.TrimSpace(req.IP)
		if net.ParseIP(ip) == nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "invalid ip"})
			return
		}
		removed, err := s.fails.unblock(ip)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "unblocked", "removed": removed})
	})
	mux.HandleFunc("/security/permanent-block/delete", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "method not allowed"})
			return
		}
		var req struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "invalid request"})
			return
		}
		ip := strings.TrimSpace(req.IP)
		if net.ParseIP(ip) == nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "invalid ip"})
			return
		}
		removed, err := s.fails.unblockPermanent(ip)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "permanent block deleted", "removed": removed})
	})
	s.httpSrv = &http.Server{
		Addr:        s.cfg.MonitorAddr,
		BaseContext: func(net.Listener) context.Context { return ctx },
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !s.beginWorker() {
				http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
				return
			}
			defer s.lifecycle.workers.Done()
			mux.ServeHTTP(w, r)
		}),
	}
	s.startWorker(func() {
		err := s.httpSrv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log("monitor failed: %v", err)
		}
	})
}
