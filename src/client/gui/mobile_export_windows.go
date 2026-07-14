//go:build windows

package gui

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
	"lsyltunnel/src/client/mobileprofile"
)

func (a *App) handleMobileExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeJSON(w, apiResult{OK: false, Message: "请求方法不支持"})
		return
	}
	target, err := a.exportMobileProfileToDownloads()
	if err != nil {
		a.appendLog("导出移动端配置失败: " + err.Error())
		a.writeJSON(w, apiResult{OK: false, Message: mobileExportFriendlyError(err), State: ptrAPIState(a.currentAPIState())})
		return
	}
	a.appendLog("已导出移动端配置: " + target)
	a.writeJSON(w, apiResult{OK: true, Message: "移动端配置已导出到系统下载目录：" + target, State: ptrAPIState(a.currentAPIState())})
}

func (a *App) exportMobileProfileToDownloads() (string, error) {
	if !a.isRunning() {
		return "", errors.New("请先登录成功后再导出移动端配置")
	}
	cfg, err := readClientConfigRaw(a.configPath)
	if err != nil {
		return "", fmt.Errorf("读取客户端配置失败: %w", err)
	}
	certFile := cfg.TLS.CACertFile
	if certFile == "" {
		certFile = defaultCACertFile(a.configPath)
	}
	certFile = resolveLocalConfigPath(filepath.Dir(a.configPath), certFile)
	fileName, err := mobileprofile.FileNameFromConfig(cfg)
	if err != nil {
		return "", err
	}
	downloads, err := systemDownloadsDir()
	if err != nil {
		return "", err
	}
	target := filepath.Join(downloads, fileName)
	result, err := mobileprofile.Export(a.configPath, certFile, target, true)
	if err != nil {
		return "", err
	}
	return result.Path, nil
}

func systemDownloadsDir() (string, error) {
	if dir, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, windows.KF_FLAG_CREATE); err == nil && dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法定位系统下载目录: %w", err)
	}
	return filepath.Join(home, "Downloads"), nil
}

func mobileExportFriendlyError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	switch {
	case containsAnyText(text, "请先登录成功"):
		return text
	case containsAnyText(text, "saved_credential"):
		return "当前登录凭据不可用于移动端导出，请重新输入密码连接成功后再试。"
	case containsAnyText(text, "username is required"):
		return "客户端配置缺少用户名，请重新输入账号并连接成功后再导出。"
	case containsAnyText(text, "server_addr is invalid"):
		return "客户端配置中的服务端地址格式不正确，应填写为 地址:端口。"
	case containsAnyText(text, "mobile export only supports client_to_server"):
		return "移动端只支持正向转发，当前配置包含反向转发或 IP 接管规则，无法导出。"
	case containsAnyText(text, "at least one client_to_server forward is required"):
		return "当前配置没有可导出的正向转发规则。"
	case containsAnyText(text, "below 1024"):
		return "移动端本地端口必须大于等于 1024，请联系管理员调整端口后重新连接。"
	case containsAnyText(text, "127.0.0.1 for mobile"):
		return "移动端本地监听只能使用 127.0.0.1，请联系管理员调整配置。"
	case containsAnyText(text, "listen_addr is invalid"):
		return "移动端本地监听地址格式不正确，应填写为 127.0.0.1:端口。"
	case containsAnyText(text, "server_target is invalid"):
		return "转发目标地址格式不正确，应填写为 地址:端口。"
	case containsAnyText(text, "duplicate forward name"):
		return "存在重名的转发规则，请联系管理员修改规则 name。"
	case containsAnyText(text, "duplicate mobile listen address"):
		return "存在重复的移动端监听端口，请联系管理员调整配置。"
	case containsAnyText(text, "insecure_skip_verify"):
		return "移动端不允许跳过证书校验，请联系管理员重新下发证书配置。"
	case containsAnyText(text, "requires tls.min_version 1.3"):
		return "移动端要求使用 TLS 1.3，请联系管理员更新客户端配置。"
	case containsAnyText(text, "server certificate", "no pem certificate"):
		return "服务端信任证书无效或缺失，请联系管理员重新下发客户端证书。"
	case containsAnyText(text, "read client config", "parse client config", "invalid client config"):
		return "无法读取客户端配置，请联系管理员检查配置文件。"
	case containsAnyText(text, "无法定位系统下载目录"):
		return "无法定位系统下载目录，请检查当前 Windows 用户目录。"
	default:
		return friendlyErrorText(text)
	}
}
