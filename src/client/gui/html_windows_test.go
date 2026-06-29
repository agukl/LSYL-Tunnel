package gui

import (
	"strings"
	"testing"
)

func TestClientHTMLTextAndScriptAreIntact(t *testing.T) {
	required := []string{
		"未连接",
		"服务端地址",
		"用户名",
		"请输入密码",
		"连接",
		"正在后台值守",
		"正在重连服务端",
		"连续重连失败",
		"隐藏到托盘",
		"退出客户端",
		"右键打开配置菜单",
		"id=\"logoMenu\"",
		"function showLogoMenu",
		"function installLogoMenuDismiss",
		"id=\"mobileExportMenuBtn\"",
		"id=\"profileDropdownBtn\"",
		"id=\"profileDropdownList\"",
		"function toggleProfileDropdown",
		"function switchProfileOption",
		"/api/mobile/export",
		"function exportMobileProfile",
		"id=\"versionBadge\"",
		"function setVersion",
		"server_version",
		"var savedPasswordMask = '********';",
		"var lastProfilesSignature = '';",
		"function profileListSignature",
		"if(signature !== lastProfilesSignature)",
	}
	for _, text := range required {
		if !strings.Contains(clientHTML, text) {
			t.Fatalf("client HTML is missing required text: %q", text)
		}
	}

	forbidden := []string{
		"\u93c8",
		"\u9422",
		"\u7035",
		"\u5bb8",
		"\u95ab",
		"\u95c5",
		"?/" + "span",
		"?/" + "label",
		"?/" + "button",
		"?/" + "div",
		"id=\"profileSwitchBtn\"",
		"updateProfileSwitchButton",
	}
	for _, text := range forbidden {
		if strings.Contains(clientHTML, text) {
			t.Fatalf("client HTML contains corrupted text or markup: %q", text)
		}
	}
}
