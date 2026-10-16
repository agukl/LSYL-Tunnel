//go:build windows

package gui

import (
	"encoding/json"
	"net/http"
	"time"

	"lsyltunnel/src/client/tunnel"
	appversion "lsyltunnel/src/internal/version"
)

type apiState struct {
	Autostart     bool               `json:"autostart"`
	Connecting    bool               `json:"connecting"`
	Running       bool               `json:"running"`
	Version       string             `json:"version"`
	ServerVersion string             `json:"server_version,omitempty"`
	Config        loginForm          `json:"config"`
	Profiles      []clientProfile    `json:"profiles"`
	Route         string             `json:"route"`
	Stats         tunnel.ClientStats `json:"stats"`
	RunStatus     string             `json:"run_status"`
	HasPassword   bool               `json:"has_password"`
	Notice        string             `json:"notice,omitempty"`
	NoticeBad     bool               `json:"notice_bad,omitempty"`
	WindowHidden  bool               `json:"window_hidden,omitempty"`
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

type profileSwitchRequest struct {
	ID string `json:"id"`
}

func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, a.currentAPIState())
}

func (a *App) currentAPIState() apiState {
	status := a.runtimeStatus()
	stats := a.clientStats()
	notice, noticeBad := a.noticeSnapshot()
	return apiState{
		Autostart:     a.autostartEnabled(),
		Connecting:    a.connecting.Load() || (a.autostartLaunch && !a.autoLoginDone.Load()),
		Running:       a.isRunning(),
		Version:       appversion.AppVersion,
		ServerVersion: stats.ServerVersion,
		Config:        a.currentLoginForm(),
		Profiles:      a.clientProfiles(),
		Route:         a.routeSummary(),
		Stats:         stats,
		RunStatus:     status,
		HasPassword:   a.hasPasswordState(),
		Notice:        notice,
		NoticeBad:     noticeBad,
		WindowHidden:  a.windowHidden.Load(),
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
		a.detachClient(client, status)
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
	a.writeJSON(w, a.login(r.Context(), &form))
}

func (a *App) handleStop(w http.ResponseWriter, r *http.Request) {
	if err := a.stopClient(); err != nil {
		a.writeJSON(w, apiResult{OK: false, Message: friendlyError(err)})
		return
	}
	a.writeJSON(w, apiResult{OK: true, Message: "连接已停止"})
}

func (a *App) handleProfileSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeJSON(w, apiResult{OK: false, Message: "请求方法不支持", State: ptrAPIState(a.currentAPIState())})
		return
	}
	defer r.Body.Close()
	var req profileSwitchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeJSON(w, apiResult{OK: false, Message: "请求格式不正确", State: ptrAPIState(a.currentAPIState())})
		return
	}
	if !a.loginMu.TryLock() {
		a.writeJSON(w, apiResult{Message: "正在连接，请稍候再切换配置。"})
		return
	}
	defer a.loginMu.Unlock()
	if a.isRunning() {
		a.writeJSON(w, apiResult{OK: false, Message: "请先断开当前连接，再切换客户端配置。", State: ptrAPIState(a.currentAPIState())})
		return
	}
	if err := a.switchClientProfile(req.ID); err != nil {
		a.appendLog("切换客户端配置失败: " + err.Error())
		a.writeJSON(w, apiResult{OK: false, Message: profileSwitchFriendlyError(err), State: ptrAPIState(a.currentAPIState())})
		return
	}
	a.setNotice("已切换客户端配置，请确认账号密码后连接。", false)
	a.appendLog("已切换客户端配置")
	a.writeJSON(w, apiResult{OK: true, Message: "已切换客户端配置", State: ptrAPIState(a.currentAPIState())})
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
