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
		"id=\"profileList\" class=\"profile-list\" role=\"menu\"",
		"item.setAttribute('role', 'menuitem');",
		"function switchProfileOption",
		"/api/mobile/export",
		"function exportMobileProfile",
		"id=\"versionBadge\"",
		"function setVersion",
		"reverse ? ' ← ' : ' → '",
		"var clientEndpoint = reverse",
		"var serverEndpoint = reverse",
		"server_version",
		"var savedPasswordMask = '********';",
		"var lastProfilesSignature = '';",
		"function profileListSignature",
		"if(signature !== lastProfilesSignature)",
		"max-height:260px;",
		"if(profile.current) return '当前使用';",
		"if(!profile.available) return profile.message || '不可用';",
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
		"正常监听",
		"被动已激活",
		"服务端被动端口 ",
		" -> 服务端 ",
		"('当前使用 · ' + profile.server_addr)",
		"return profile.server_addr || '可切换配置';",
		"return '可切换';",
		"profileDropdown",
		"profile-dropdown",
		"profileHint",
		"profile-hint",
		"toggleProfileDropdown",
	}
	for _, text := range forbidden {
		if strings.Contains(clientHTML, text) {
			t.Fatalf("client HTML contains corrupted text or markup: %q", text)
		}
	}
}
