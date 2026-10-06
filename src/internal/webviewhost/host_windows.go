//go:build windows

package webviewhost

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jchv/go-webview2/pkg/edge"
	"github.com/jchv/go-webview2/webviewloader"
)

// Host owns the WebView2 controller attached to a Walk window. All methods
// that touch Chromium must run on the window's UI thread.
type Host struct {
	view *edge.Chromium
	url  string
}

func CheckRuntime() error {
	version, err := webviewloader.GetInstalledVersion()
	if err != nil {
		return fmt.Errorf("无法检测 Microsoft Edge WebView2 Runtime: %w", err)
	}
	if version == "" {
		return fmt.Errorf("缺少 Microsoft Edge WebView2 Runtime。请重新运行本程序安装包补装运行时；离线环境请先安装微软 WebView2 Evergreen Standalone Installer（x64）")
	}
	return nil
}

func New(parent uintptr, url, profile string, onNavigationCompleted func()) (*Host, error) {
	if parent == 0 {
		return nil, fmt.Errorf("WebView2 父窗口尚未创建")
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("无法确定 WebView2 用户数据目录: %w", err)
	}
	dataDir := filepath.Join(cacheDir, "LSYL Tunnel", profile, "WebView2")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("无法创建 WebView2 用户数据目录: %w", err)
	}

	view := edge.NewChromium()
	view.DataPath = dataDir
	view.NavigationCompletedCallback = func(_ *edge.ICoreWebView2, _ *edge.ICoreWebView2NavigationCompletedEventArgs) {
		if onNavigationCompleted != nil {
			onNavigationCompleted()
		}
	}
	if !view.Embed(parent) {
		return nil, fmt.Errorf("WebView2 初始化失败，请检查运行时及用户数据目录: %s", dataDir)
	}
	if view.GetController() == nil {
		return nil, fmt.Errorf("WebView2 控制器未能创建")
	}
	settings, err := view.GetSettings()
	if err != nil {
		return nil, fmt.Errorf("读取 WebView2 设置失败: %w", err)
	}
	if err := settings.PutAreDefaultContextMenusEnabled(false); err != nil {
		return nil, fmt.Errorf("设置 WebView2 菜单失败: %w", err)
	}
	if err := settings.PutAreDevToolsEnabled(false); err != nil {
		return nil, fmt.Errorf("设置 WebView2 开发工具失败: %w", err)
	}
	if err := settings.PutIsStatusBarEnabled(false); err != nil {
		return nil, fmt.Errorf("设置 WebView2 状态栏失败: %w", err)
	}
	view.Resize()
	view.Navigate(url)
	return &Host{view: view, url: url}, nil
}

func (h *Host) Resize() {
	if h != nil {
		h.view.Resize()
	}
}

func (h *Host) Reload() {
	if h != nil {
		h.view.Navigate(h.url)
	}
}

func (h *Host) NotifyPositionChanged() {
	if h != nil {
		_ = h.view.NotifyParentWindowPositionChanged()
	}
}
