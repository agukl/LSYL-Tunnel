//go:build windows

package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"
)

const clientAutostartKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const clientAutostartValue = "LSYLTunnelClient"

type clientAutostart struct {
	root       registry.Key
	key        string
	executable string
}

func newClientAutostart(executable string) *clientAutostart {
	return &clientAutostart{root: registry.CURRENT_USER, key: clientAutostartKey, executable: executable}
}

func isAutostartLaunch(args []string) bool {
	return len(args) == 1 && args[0] == "--autostart"
}

func autostartWorkspace(executable string) string {
	dir := filepath.Dir(executable)
	if root := findWorkspaceFrom(dir); root != "" {
		return root
	}
	if strings.EqualFold(filepath.Base(dir), "bin") {
		return filepath.Dir(dir)
	}
	return dir
}

func (s *clientAutostart) command() (string, error) {
	if s == nil || !filepath.IsAbs(s.executable) || strings.ContainsAny(s.executable, "\"\x00\r\n") {
		return "", fmt.Errorf("无法确定客户端完整路径，请从安装目录启动客户端")
	}
	command := `"` + filepath.Clean(s.executable) + `" --autostart`
	if len(utf16.Encode([]rune(command))) > 260 {
		return "", fmt.Errorf("客户端安装路径过长，无法设置开机自启")
	}
	return command, nil
}

func (s *clientAutostart) enabled() (bool, error) {
	command, err := s.command()
	if err != nil {
		return false, err
	}
	k, err := registry.OpenKey(s.root, s.key, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer k.Close()
	value, _, err := k.GetStringValue(clientAutostartValue)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	return strings.EqualFold(value, command), err
}

func (s *clientAutostart) setEnabled(enabled bool) error {
	command, err := s.command()
	if err != nil {
		return err
	}
	if !enabled {
		k, err := registry.OpenKey(s.root, s.key, registry.QUERY_VALUE|registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer k.Close()
		value, _, err := k.GetStringValue(clientAutostartValue)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		// Do not remove another installation's startup entry.
		if !strings.EqualFold(value, command) {
			return nil
		}
		return k.DeleteValue(clientAutostartValue)
	}
	k, _, err := registry.CreateKey(s.root, s.key, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(clientAutostartValue, command)
}

func (a *App) autostartEnabled() bool {
	if a.autostart == nil {
		return false
	}
	enabled, err := a.autostart.enabled()
	return err == nil && enabled
}

func (a *App) handleAutostart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		a.writeJSON(w, apiResult{Message: "请求方法不支持"})
		return
	}
	// Reject cross-origin requests before changing this user's startup settings.
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
		Enabled *bool `json:"enabled"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || req.Enabled == nil {
		a.writeJSON(w, apiResult{Message: "请求格式不正确"})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		a.writeJSON(w, apiResult{Message: "请求格式不正确"})
		return
	}
	if err := a.autostart.setEnabled(*req.Enabled); err != nil {
		log.Print("设置开机自启失败: " + err.Error())
		a.writeJSON(w, apiResult{Message: "无法修改开机自启设置，请检查当前用户权限或系统启动项限制。", State: ptrAPIState(a.currentAPIState())})
		return
	}
	message := "已关闭开机自启"
	if *req.Enabled {
		message = "已开启开机自启并连接"
	}
	a.writeJSON(w, apiResult{OK: true, Message: message, State: ptrAPIState(a.currentAPIState())})
}
