//go:build windows

package gui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"lsyltunnel/src/client/tunnel"
)

type apiState struct {
	Running     bool               `json:"running"`
	Config      loginForm          `json:"config"`
	Route       string             `json:"route"`
	Stats       tunnel.ClientStats `json:"stats"`
	RunStatus   string             `json:"run_status"`
	HasPassword bool               `json:"has_password"`
	Notice      string             `json:"notice,omitempty"`
	NoticeBad   bool               `json:"notice_bad,omitempty"`
}

type apiResult struct {
	OK      bool      `json:"ok"`
	Message string    `json:"message"`
	State   *apiState `json:"state,omitempty"`
}

type loginForm struct {
	ServerAddr string `json:"server_addr"`
	Username   string `json:"username"`
	Password   string `json:"password"`
}

func (a *App) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, clientHTML)
}

func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, a.currentAPIState())
}

func (a *App) currentAPIState() apiState {
	status := a.runtimeStatus()
	stats := a.clientStats()
	notice, noticeBad := a.noticeSnapshot()
	return apiState{
		Running:     a.isRunning(),
		Config:      a.currentLoginForm(),
		Route:       a.routeSummary(),
		Stats:       stats,
		RunStatus:   status,
		HasPassword: a.hasPasswordState(),
		Notice:      notice,
		NoticeBad:   noticeBad,
	}
}

func (a *App) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	client := a.client()
	if client == nil {
		a.writeJSON(w, apiResult{OK: false, Message: "客户端未运行", State: ptrAPIState(a.currentAPIState())})
		return
	}
	status := client.CheckHealthNow(r.Context())
	if status.Terminal {
		a.detachClient(client, terminalDisconnectMessage(tunnel.ClientStats{Health: status}), true)
		a.writeJSON(w, apiResult{OK: false, Message: status.Message, State: ptrAPIState(a.currentAPIState())})
		return
	}
	ok := status.State == tunnel.HealthOK
	message := status.Message
	if ok {
		summary := client.CheckForwardsNow(r.Context())
		message = tunnel.ForwardCheckMessage(summary)
		ok = summary.Rejected == 0 && summary.Failed == 0
	}
	if message == "" {
		message = "状态已刷新"
	}
	a.writeJSON(w, apiResult{OK: ok, Message: message, State: ptrAPIState(a.currentAPIState())})
}

func (a *App) handleStart(w http.ResponseWriter, r *http.Request) {
	form, err := decodeLoginForm(r)
	if err != nil {
		a.writeJSON(w, apiResult{OK: false, Message: "请求格式不正确"})
		return
	}
	rawCfg, err := a.prepareLoginConfig(form)
	a.setNotice("", false)
	if err != nil {
		a.appendLog("配置准备失败: " + err.Error())
		a.writeJSON(w, apiResult{OK: false, Message: friendlyError(err)})
		return
	}
	if a.isRunning() {
		a.writeJSON(w, apiResult{OK: true, Message: "已经连接，正在后台值守"})
		return
	}
	cfg, err := runtimeClientConfigFromRaw(a.configPath, rawCfg)
	if err != nil {
		a.appendLog("配置校验失败: " + err.Error())
		a.writeJSON(w, apiResult{OK: false, Message: friendlyError(err)})
		return
	}
	loginTimeout := time.Duration(cfg.Connection.DialTimeoutSec+3) * time.Second
	if loginTimeout < 5*time.Second {
		loginTimeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), loginTimeout)
	defer cancel()
	resp, err := tunnel.CheckLoginResponse(ctx, cfg)
	if err != nil {
		a.appendLog("登录探测失败: " + err.Error())
		if shouldClearSavedPasswordState(err) {
			a.clearSavedPasswordStateAfterAuthFailure()
		}
		a.writeJSON(w, apiResult{OK: false, Message: friendlyError(err)})
		return
	}
	if rawCfg.Password != "" {
		if resp.CredentialKey != nil {
			sealed, err := tunnel.SealSavedCredential(*resp.CredentialKey, cfg, rawCfg.Password)
			if err != nil {
				a.appendLog("保存本地登录凭据失败: " + err.Error())
				a.writeJSON(w, apiResult{OK: false, Message: friendlyError(err)})
				return
			}
			rawCfg.SavedCredential = sealed
			cfg.SavedCredential = sealed
			cfg.Password = ""
		} else {
			a.appendLog("服务端未提供本地凭据密封公钥，本次不会保存密码")
		}
		rawCfg.Password = ""
	}
	if err := a.saveClientConfig(rawCfg); err != nil {
		a.appendLog("保存配置失败: " + err.Error())
		a.writeJSON(w, apiResult{OK: false, Message: friendlyError(err)})
		return
	}
	if err := a.startClient(cfg); err != nil {
		a.appendLog("启动连接失败: " + err.Error())
		a.writeJSON(w, apiResult{OK: false, Message: friendlyError(err)})
		return
	}
	a.appendLog("登录成功，隧道已启动并进入后台值守")
	a.writeJSON(w, apiResult{OK: true, Message: "登录成功，已在后台值守"})
}

func (a *App) handleStop(w http.ResponseWriter, r *http.Request) {
	if err := a.stopClient(); err != nil {
		a.writeJSON(w, apiResult{OK: false, Message: friendlyError(err)})
		return
	}
	a.writeJSON(w, apiResult{OK: true, Message: "连接已停止"})
}

func (a *App) handleHide(w http.ResponseWriter, r *http.Request) {
	a.hideToTray("LSYL Tunnel 已隐藏到托盘，连接会继续保持")
	a.writeJSON(w, apiResult{OK: true, Message: "已隐藏到托盘"})
}

func (a *App) handleQuit(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, apiResult{OK: true, Message: "正在退出"})
	go func() {
		time.Sleep(150 * time.Millisecond)
		if a.mw != nil {
			a.mw.Synchronize(func() { a.exitApp() })
		}
	}()
}

func (a *App) writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func decodeLoginForm(r *http.Request) (loginForm, error) {
	defer r.Body.Close()
	var form loginForm
	if err := json.NewDecoder(r.Body).Decode(&form); err != nil {
		return form, err
	}
	return form, nil
}
