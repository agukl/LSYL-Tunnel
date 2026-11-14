//go:build windows

package gui

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
)

const (
	minDialTimeoutSec = 5
	maxDialTimeoutSec = 60
)

func (a *App) handleDialTimeout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		a.writeJSON(w, apiResult{Message: "请求方法不支持"})
		return
	}
	if a.panelURL != "" && "http://"+r.Host+"/" != a.panelURL {
		w.WriteHeader(http.StatusForbidden)
		a.writeJSON(w, apiResult{Message: "请求来源不允许"})
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
		w.WriteHeader(http.StatusForbidden)
		a.writeJSON(w, apiResult{Message: "请求来源不允许"})
		return
	}
	defer r.Body.Close()
	var req struct {
		Seconds *int `json:"dial_timeout_sec"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || req.Seconds == nil {
		a.writeJSON(w, apiResult{Message: "请求格式不正确"})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		a.writeJSON(w, apiResult{Message: "请求格式不正确"})
		return
	}
	if *req.Seconds < minDialTimeoutSec || *req.Seconds > maxDialTimeoutSec {
		a.writeJSON(w, apiResult{Message: "拨号超时应为 5–60 秒"})
		return
	}
	if !a.loginMu.TryLock() {
		a.writeJSON(w, apiResult{Message: "正在连接，请稍后再修改拨号超时"})
		return
	}
	defer a.loginMu.Unlock()
	if a.connecting.Load() {
		a.writeJSON(w, apiResult{Message: "正在连接，请稍后再修改拨号超时"})
		return
	}
	cfg, err := readClientConfigRaw(a.configPath)
	if err != nil {
		log.Print("读取拨号超时配置失败: " + err.Error())
		a.writeJSON(w, apiResult{Message: "无法读取当前客户端配置，请检查配置文件。"})
		return
	}
	if cfg.Connection.DialTimeoutSec != *req.Seconds {
		cfg.Connection.DialTimeoutSec = *req.Seconds
		if err := a.saveClientConfig(cfg); err != nil {
			log.Print("保存拨号超时失败: " + err.Error())
			a.writeJSON(w, apiResult{Message: "无法保存拨号超时，请检查配置文件权限。"})
			return
		}
	}
	a.writeJSON(w, apiResult{OK: true, Message: "拨号超时已保存，下次连接生效", State: ptrAPIState(a.currentAPIState())})
}
