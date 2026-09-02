//go:build windows

package gui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"lsyltunnel/src/server/tunnel"
)

type monitorStatus struct {
	Service                 string                  `json:"service"`
	ListenAddr              string                  `json:"listen_addr"`
	UptimeSec               int64                   `json:"uptime_sec"`
	ActiveConnections       int                     `json:"active_connections"`
	ConnectionsRejected     int64                   `json:"connections_rejected"`
	ConnectionLimits        map[string]any          `json:"connection_limits,omitempty"`
	ConnectionRejections    map[string]any          `json:"connection_rejections,omitempty"`
	ActiveStreams           int64                   `json:"active_streams"`
	TotalStreams            int64                   `json:"total_streams"`
	UserStreamLimits        map[string]any          `json:"user_stream_limits,omitempty"`
	UserStreamLimitRejected int64                   `json:"user_stream_limit_rejected"`
	AuthOK                  int64                   `json:"auth_ok"`
	AuthFailed              int64                   `json:"auth_failed"`
	PolicyRejected          int64                   `json:"policy_rejected"`
	DialFailed              int64                   `json:"dial_failed"`
	BytesUp                 int64                   `json:"bytes_up"`
	BytesDown               int64                   `json:"bytes_down"`
	EntrySecurity           map[string]any          `json:"entry_security,omitempty"`
	FailureTracker          map[string]any          `json:"failure_tracker,omitempty"`
	EntryTrafficLog         map[string]any          `json:"entry_traffic_log,omitempty"`
	BlockedIPs              []tunnel.BlockedIPState `json:"blocked_ips,omitempty"`
	RecentEvents            []tunnel.RuntimeEvent   `json:"recent_events,omitempty"`
}

type unblockIPRequest struct {
	IP string `json:"ip"`
}

func loadMonitorStatus(addr string) (*monitorStatus, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, nil
	}
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	resp, err := client.Get("http://" + addr + "/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("monitor returned %s", resp.Status)
	}
	var status monitorStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}

func unblockIPViaMonitor(addr, ip string) (bool, error) {
	addr = strings.TrimSpace(addr)
	ip = strings.TrimSpace(ip)
	if addr == "" {
		return false, fmt.Errorf("monitor address is empty")
	}
	body, err := json.Marshal(unblockIPRequest{IP: ip})
	if err != nil {
		return false, err
	}
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	resp, err := client.Post("http://"+addr+"/security/unblock", "application/json; charset=utf-8", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var result struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
		Removed bool   `json:"removed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !result.OK {
		if result.Message == "" {
			result.Message = fmt.Sprintf("monitor returned %s", resp.Status)
		}
		return false, fmt.Errorf("%s", result.Message)
	}
	return result.Removed, nil
}

func deletePermanentBlockViaMonitor(addr, ip string) (bool, error) {
	addr = strings.TrimSpace(addr)
	ip = strings.TrimSpace(ip)
	if addr == "" {
		return false, fmt.Errorf("monitor address is empty")
	}
	body, err := json.Marshal(unblockIPRequest{IP: ip})
	if err != nil {
		return false, err
	}
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	resp, err := client.Post("http://"+addr+"/security/permanent-block/delete", "application/json; charset=utf-8", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var result struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
		Removed bool   `json:"removed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !result.OK {
		if result.Message == "" {
			result.Message = fmt.Sprintf("monitor returned %s", resp.Status)
		}
		return false, fmt.Errorf("%s", result.Message)
	}
	return result.Removed, nil
}
