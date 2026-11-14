//go:build windows

package gui

import (
	"net/http"

	"lsyltunnel/src/server/tunnel"
)

type ipv6TunnelView struct {
	Addresses        []string `json:"addresses"`
	MonitorAvailable bool     `json:"monitor_available"`
	Listening        bool     `json:"listening"`
}

func (a *App) handleAdminIPv6Tunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResult{OK: false, Message: "请求方法不支持"})
		return
	}
	view := ipv6TunnelView{Addresses: tunnel.CurrentGlobalIPv6Addresses()}
	if cfg, err := readRawServerConfig(a.configPath); err == nil {
		if monitor, err := loadMonitorStatus(cfg.MonitorAddr); err == nil && monitor != nil {
			view.MonitorAvailable = true
			view.Listening = monitor.IPv6TunnelListening
		}
	}
	writeJSON(w, http.StatusOK, view)
}
