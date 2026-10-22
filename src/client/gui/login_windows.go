//go:build windows

package gui

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"lsyltunnel/src/client/tunnel"
)

var errNoSavedLogin = errors.New("未保存登录凭据，请先输入密码连接成功后再使用自动连接")

func (a *App) beginLogin(parent context.Context) (context.Context, func(), bool) {
	if !a.loginMu.TryLock() {
		return nil, nil, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing.Load() || a.connecting.Load() {
		a.loginMu.Unlock()
		return nil, nil, false
	}
	ctx, cancel := context.WithCancel(parent)
	a.loginCancel = cancel
	a.connecting.Store(true)
	return ctx, func() {
		cancel()
		a.mu.Lock()
		a.loginCancel = nil
		a.connecting.Store(false)
		a.mu.Unlock()
		a.loginMu.Unlock()
	}, true
}

func (a *App) cancelPendingLogin() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closing.Store(true)
	if a.loginCancel != nil {
		a.loginCancel()
	}
}

func (a *App) savedLoginConfig() (tunnel.Config, error) {
	cfg, err := readClientConfigRaw(a.configPath)
	if err != nil {
		return cfg, fmt.Errorf("read client config: %w", err)
	}
	if strings.TrimSpace(cfg.SavedCredential.Ciphertext) == "" || strings.TrimSpace(cfg.Username) == "" {
		return cfg, errNoSavedLogin
	}
	// Autostart uses only the saved sealed credential, never another password source.
	cfg.Password, cfg.PasswordEnv, cfg.PasswordFile = "", "", ""
	return cfg, nil
}

// A nil form selects the current profile's saved credential for autostart.
func (a *App) login(parent context.Context, form *loginForm) apiResult {
	ctx, done, ok := a.beginLogin(parent)
	if !ok {
		return apiResult{Message: "正在连接或退出，请稍候"}
	}
	defer done()
	if a.isRunning() {
		return apiResult{OK: true, Message: "已经连接，正在后台值守"}
	}
	a.setNotice("", false)
	var rawCfg tunnel.Config
	var err error
	if form == nil {
		rawCfg, err = a.savedLoginConfig()
	} else {
		rawCfg, err = a.prepareLoginConfig(*form)
	}
	if err != nil {
		log.Print("配置准备失败: " + err.Error())
		if errors.Is(err, errNoSavedLogin) {
			return apiResult{Message: err.Error()}
		}
		return apiResult{Message: friendlyError(err)}
	}
	cfg, err := runtimeClientConfigFromRaw(a.configPath, rawCfg)
	if err != nil {
		log.Print("配置校验失败: " + err.Error())
		return apiResult{Message: friendlyError(err)}
	}
	loginTimeout := time.Duration(cfg.Connection.DialTimeoutSec+3) * time.Second
	if loginTimeout < 5*time.Second {
		loginTimeout = 5 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, loginTimeout)
	resp, err := tunnel.CheckLoginResponse(probeCtx, cfg)
	cancel()
	if err != nil {
		log.Print("登录探测失败: " + err.Error())
		if shouldClearSavedPasswordState(err) {
			a.clearSavedPasswordStateAfterAuthFailure()
		}
		return apiResult{Message: friendlyError(err)}
	}
	if ctx.Err() != nil {
		return apiResult{Message: "连接已取消"}
	}
	if rawCfg.Password != "" {
		if resp.CredentialKey != nil {
			sealed, err := tunnel.SealSavedCredential(*resp.CredentialKey, cfg, rawCfg.Password)
			if err != nil {
				log.Print("保存本地登录凭据失败: " + err.Error())
				return apiResult{Message: friendlyError(err)}
			}
			rawCfg.SavedCredential = sealed
			cfg.SavedCredential = sealed
			cfg.Password = ""
		} else {
			log.Print("服务端未提供本地凭据密封公钥，本次不会保存密码")
		}
		rawCfg.Password = ""
	}
	if form != nil {
		if err := a.saveClientConfig(rawCfg); err != nil {
			log.Print("保存配置失败: " + err.Error())
			return apiResult{Message: friendlyError(err)}
		}
	}
	if ctx.Err() != nil {
		return apiResult{Message: "连接已取消"}
	}
	if err := a.startClient(cfg, resp.ServerVersion); err != nil {
		log.Print("启动连接失败: " + err.Error())
		return apiResult{Message: friendlyError(err)}
	}
	log.Print("登录成功，隧道已启动并进入后台值守")
	return apiResult{OK: true, Message: "登录成功，已在后台值守"}
}

func (a *App) autoConnect() {
	a.setNotice("正在使用已保存凭据自动连接", false)
	result := a.login(context.Background(), nil)
	a.autoLoginDone.Store(true)
	if a.closing.Load() {
		return
	}
	if !result.OK {
		a.setNotice("自动连接未完成："+result.Message, true)
		a.showWindow()
		return
	}
	a.setNotice("", false)
	a.runOnUI(func() {
		if a.closing.Load() || !a.isRunning() || a.mw == nil {
			return
		}
		if a.ensureTrayVisibleNow() {
			a.mw.Hide()
			a.windowHidden.Store(true)
		}
	})
}
