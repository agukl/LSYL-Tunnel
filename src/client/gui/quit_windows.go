//go:build windows

package gui

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

const clientQuitTimeout = 5 * time.Second

func requestQuitRunningInstance() error {
	if !clientSingleInstanceRunning() {
		return nil
	}

	app := NewApp()
	panelURL, err := readClientPanelURL(app.workspace)
	if err != nil {
		if !clientSingleInstanceRunning() {
			return nil
		}
		return fmt.Errorf("客户端正在运行，但无法定位退出接口，请先从托盘退出后重试")
	}
	if err := postClientQuit(panelURL); err != nil {
		if !clientSingleInstanceRunning() {
			return nil
		}
		return fmt.Errorf("无法请求客户端退出，请先从托盘退出后重试: %w", err)
	}
	if waitForClientInstanceExit(clientQuitTimeout) {
		return nil
	}
	return fmt.Errorf("客户端仍在运行，请先从托盘退出后重试")
}

func readClientPanelURL(workspace string) (string, error) {
	data, err := os.ReadFile(filepath.Join(appTmpDir(workspace), "gui", "client-gui.url"))
	if err != nil {
		return "", err
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return "", fmt.Errorf("empty client gui url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" {
		return "", fmt.Errorf("unsupported client gui url scheme")
	}
	host := u.Hostname()
	if !isLocalClientHost(host) {
		return "", fmt.Errorf("client gui url is not local")
	}
	u.Path = "/api/quit"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func postClientQuit(panelQuitURL string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest(http.MethodPost, panelQuitURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("quit endpoint returned %s", resp.Status)
	}
	return nil
}

func waitForClientInstanceExit(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !clientSingleInstanceRunning() {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return !clientSingleInstanceRunning()
}

func clientSingleInstanceRunning() bool {
	h, acquired, err := acquireClientSingleInstanceMutex()
	if h != 0 {
		if acquired {
			_ = windows.ReleaseMutex(h)
		}
		_ = windows.CloseHandle(h)
	}
	if err != nil {
		return false
	}
	return !acquired
}

func acquireClientSingleInstanceMutex() (windows.Handle, bool, error) {
	name, err := windows.UTF16PtrFromString(clientSingleInstanceMutexName)
	if err != nil {
		return 0, false, err
	}
	h, err := windows.CreateMutex(nil, true, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		return h, false, nil
	}
	if err != nil {
		if h != 0 {
			_ = windows.CloseHandle(h)
		}
		return 0, false, err
	}
	return h, true, nil
}

func isLocalClientHost(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
