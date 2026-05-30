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
		"右键导出移动端配置",
		"/api/mobile/export",
		"function exportMobileProfile",
		"var savedPasswordMask = '********';",
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
	}
	for _, text := range forbidden {
		if strings.Contains(clientHTML, text) {
			t.Fatalf("client HTML contains corrupted text or markup: %q", text)
		}
	}
}
