package tunnel

import (
	"fmt"
	"strings"
)

// VirtualForwardErrorText returns the user-facing message for a virtual
// forwarding error. The boolean is false when the error is unrelated.
func VirtualForwardErrorText(raw string) (string, bool) {
	message, _, ok := classifyVirtualForwardError(raw)
	return message, ok
}

func isVirtualAuthorizationCancelled(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return containsAny(
		text,
		"administrator authorization for virtual forwarding was cancelled",
		"administrator authorization for virtual forwarding was canceled",
	)
}

func classifyVirtualForwardError(raw string) (message string, permanent bool, ok bool) {
	text := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case text == "":
		return "", false, false
	case containsAny(text, "virtual listen_addr must use :port or ipv4:port"):
		return "虚拟入口格式不正确，请填写 :端口 或 IPv4:端口", true, true
	case containsAny(text, "virtual listen_addr port must be between 1 and 65535"):
		return "虚拟入口端口必须在 1 到 65535 之间", true, true
	case containsAny(text, "virtual listen_addr does not support domain names"):
		return "虚拟入口不支持域名，请填写 :端口，或填写服务端证书中的 IPv4:端口", true, true
	case containsAny(text, "virtual listen_addr host must be an ipv4 address"):
		return "虚拟入口不支持 IPv6，请填写 :端口，或填写服务端证书中的 IPv4:端口", true, true
	case containsAny(text, "virtual listen_addr host must be a usable non-local ipv4 address"):
		return "虚拟入口必须使用可用的非本地 IPv4，不能使用回环、链路本地或保留地址", true, true
	case containsAny(text, "server_addr must include a valid port before virtual forwarding"):
		return "服务端地址必须包含有效端口，配置虚拟转发前请先修正 server_addr", true, true
	case containsAny(text, "virtual listen_addr cannot use the server_addr port"):
		return "虚拟入口端口不能与客户端连接服务端的端口相同，请更换业务端口", true, true
	case strings.Contains(text, "virtual endpoint") && strings.Contains(text, "duplicates forward"):
		return "存在重复的虚拟入口 IP:端口，请为每条虚拟转发配置不同端点", true, true
	case containsAny(text, "virtual forwarding supports at most"):
		return fmt.Sprintf("虚拟转发最多支持 %d 个端点，请减少配置数量", maxVirtualRedirectEndpoints), true, true
	case containsAny(text, "multiple usable ipv4 sans"):
		return "服务端证书包含多个可用 IPv4，请将虚拟 listen_addr 填写为完整的 IPv4:端口", true, true
	case containsAny(text, "no usable non-local ipv4 san"):
		return "服务端证书没有可用于虚拟入口的非本地 IPv4 SAN，请重新下发证书", true, true
	case containsAny(text, "no ipv4 san for virtual forwarding"):
		return "服务端证书没有 IPv4 SAN，无法配置虚拟入口，请重新下发证书", true, true
	case containsAny(text, "not authorized by the server certificate ipv4 san"):
		return "填写的虚拟 IP 不在服务端证书 IPv4 SAN 中，请修正 listen_addr 或重新下发证书", true, true
	case containsAny(text, "tls.ca_cert_file is required to authorize virtual forwarding"):
		return "缺少服务端信任证书，无法校验虚拟 IP，请重新下发客户端安装包", true, true
	case strings.Contains(text, "read virtual forwarding certificate policy") && containsAny(text, "no such file", "cannot find", "找不到", "系统找不到"):
		return "缺少服务端信任证书 server.crt，无法校验虚拟 IP，请重新下发客户端安装包", true, true
	case containsAny(text, "read virtual forwarding certificate policy"):
		return "无法读取服务端信任证书，无法校验虚拟 IP，请检查证书文件和权限", true, true
	case containsAny(text, "parse virtual forwarding certificate policy", "no certificate found in tls.ca_cert_file for virtual forwarding"):
		return "服务端信任证书格式无效，无法校验虚拟 IP，请重新下发证书", true, true
	case containsAny(text, "virtual address resolver is unavailable"):
		return "虚拟入口地址解析失败，请重新连接；若持续出现请重新安装客户端", true, true
	case containsAny(text, "virtual forwarding is only supported by the standard 64-bit windows client"):
		return "虚拟 IP 接管仅支持标准 64 位 Windows 客户端", true, true
	case containsAny(text, "administrator authorization for virtual forwarding was cancelled"):
		return "IP 接管需要管理员授权，本次授权已取消", false, true
	case containsAny(text, "windivert.dll is missing"):
		return "虚拟端点接管组件缺失，请修复或重新安装标准客户端", true, true
	case containsAny(text, "unsupported windivert address layout", "load windivert.dll", " from windivert.dll"):
		return "虚拟端点接管组件加载失败，请修复或重新安装标准客户端", true, true
	case containsAny(text, "windivertopen failed"):
		return "无法启动虚拟端点接管，请检查管理员权限、安全软件和 WinDivert 驱动状态", false, true
	case containsAny(text, "windivertrecv failed", "windivertsend failed", "windivert checksum update failed"):
		return "虚拟流量接管已中断，请重新连接；若持续出现请检查 WinDivert 驱动状态", false, true
	case containsAny(text, "allocate virtual local redirect listener", "did not allocate a valid local redirect port"):
		return "无法分配虚拟转发的本地桥接端口，请关闭占用大量端口的程序后重新连接", false, true
	case strings.Contains(text, "virtual forward") && strings.Contains(text, "listener already exists"):
		return "虚拟转发名称重复，请为每条转发配置不同的 name", true, true
	case containsAny(
		text,
		"parse virtual redirect endpoint",
		"virtual redirect port must be between",
		"local virtual redirect port must be between",
		"virtual redirect ports must be between",
		"duplicate virtual redirect endpoint",
		"duplicate local virtual redirect port",
		"at least one virtual redirect rule is required",
		"decode virtual redirect rules",
		"parse virtual redirect rules",
	):
		return "虚拟端点接管规则无效，请检查虚拟转发配置并重新连接", true, true
	case containsAny(text, "virtual redirect"):
		return "虚拟端点接管进程异常，请重新连接；若持续出现请检查管理员权限和 WinDivert 驱动状态", false, true
	case containsAny(text, "windivert"):
		return "虚拟端点接管异常，请重新连接；若持续出现请检查 WinDivert 组件和驱动状态", false, true
	case containsAny(text, "virtual listen_addr", "virtual endpoint", "virtual forwarding", "virtual ip", "virtual-ip"):
		return "虚拟转发配置无效，请检查 listen_addr 和证书配置", true, true
	default:
		return "", false, false
	}
}
