package tunnel

import (
	"strings"
)

func classifyHealthError(err error) (string, string) {
	if err == nil {
		return HealthOK, "服务端连接正常"
	}
	if policy, ok := policyForResponse(err); ok {
		return policy.healthState, policy.healthMessage
	}
	text := strings.ToLower(err.Error())
	switch {
	case containsAny(text, "client_version_unsupported"):
		return HealthAuthError, "客户端版本不在服务端允许范围内，请联系管理员确认两端版本"
	case containsAny(text, "protocol_version_unsupported", "invalid tunnel request", "unsupported request", "bad_request"):
		return HealthAuthError, "客户端与服务端协议版本不一致，请联系管理员升级对应一端"
	case containsAny(text, "auth_failed", "username or password"):
		return HealthAuthError, "账号或密码不正确，需要重新登录"
	case containsAny(text, "credential_expired", "saved login has expired"):
		return HealthAuthError, "保存的登录凭据已过期，需要重新登录"
	case isTemporaryAuthBlockText(text):
		return HealthAuthError, "登录失败次数过多，当前来源 IP 已被临时封禁"
	case containsAny(text, "no server tls trust data", "appendcertsfrompem"):
		return HealthAuthError, "服务端信任证书无效，请联系管理员重新下发"
	case isMissingServerCertError(text):
		return HealthAuthError, "缺少服务端信任证书 server.crt，请联系管理员重新下发客户端安装包"
	case containsAny(text, "ca_cert_file", "server verification"):
		return HealthAuthError, "缺少服务端信任证书 server.crt，请联系管理员重新下发客户端安装包"
	case containsAny(text, "certificate is valid for", "cannot validate certificate", "doesn't contain any ip sans"):
		return HealthAuthError, "服务端证书和当前地址不匹配，请检查服务端地址或重新下发证书"
	case containsAny(text, "certificate has expired", "certificate is not yet valid", "has expired or is not yet valid"):
		return HealthAuthError, "服务端证书已过期或尚未生效，请联系管理员重新下发有效证书"
	case containsAny(text, "unknown authority", "not trusted"):
		return HealthAuthError, "服务端证书不受信任，请联系管理员重新下发证书"
	case containsAny(text, "missing port in address", "too many colons in address", "unknown port"):
		return HealthAuthError, "服务端地址格式不正确，请填写 地址:端口"
	case containsAny(text, "no such host"):
		return HealthServerUnavailable, "服务端地址无法解析，请检查域名或网络"
	case containsAny(text, "connection refused", "actively refused", "connectex"):
		return HealthServerUnavailable, "连接不上服务端，请检查服务端是否启动或地址端口是否正确"
	case containsAny(text, "timeout", "deadline", "i/o timeout"):
		return HealthServerUnavailable, "连接超时，请检查网络或服务端防火墙"
	case containsAny(text, "network is unreachable", "no route to host"):
		return HealthServerUnavailable, "网络不可达，请检查本机网络"
	case containsAny(text, "connection reset", "forcibly closed", "wsarecv", "eof"):
		return HealthServerUnavailable, "连接被服务端断开，请稍后重试或联系管理员"
	case containsAny(text, "connection aborted", "broken pipe", "closed network connection"):
		return HealthServerUnavailable, "连接已中断，请稍后重试或联系管理员"
	case containsAny(text, "x509"):
		return HealthAuthError, "服务端证书校验失败，请联系管理员检查证书"
	case containsAny(text, "tls handshake after certificate possession gate"):
		return HealthServerUnavailable, "TLS 握手失败，请检查服务端状态及两端版本"
	case containsAny(text, "certificate"):
		return HealthAuthError, "服务端证书校验失败，请联系管理员检查证书"
	case containsAny(text, "tls"):
		return HealthServerUnavailable, "TLS 握手失败，请检查服务端状态及两端版本"
	default:
		return HealthServerUnavailable, "服务端暂时不可达"
	}
}

func ForwardErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if policy, ok := policyForResponse(err); ok {
		return policy.forwardMessage
	}
	text := strings.ToLower(err.Error())
	if message, _, ok := classifyVirtualForwardError(text); ok {
		return message
	}
	switch {
	case containsAny(text, "client_version_unsupported"):
		return "客户端版本不在服务端允许范围内，请联系管理员确认两端版本"
	case containsAny(text, "protocol_version_unsupported", "invalid tunnel request", "unsupported request", "bad_request"):
		return "客户端与服务端协议版本不一致，请联系管理员升级对应一端"
	case containsAny(text, "auth_failed", "username or password"):
		return "账号或密码不正确，请重新登录"
	case containsAny(text, "credential_expired", "saved login has expired"):
		return "保存的登录凭据已过期，请重新输入密码"
	case containsAny(text, "user_stream_limit", "too many concurrent streams"):
		return "当前账号并发连接数已达到上限，请关闭部分连接后重试"
	case isTemporaryAuthBlockText(text):
		return "登录失败次数过多，当前来源 IP 已被临时封禁，请稍后再试"
	case containsAny(text, "certificate has expired", "certificate is not yet valid", "has expired or is not yet valid"):
		return "服务端证书已过期或尚未生效，请联系管理员重新下发有效证书"
	case containsAny(text, "certificate is valid for", "cannot validate certificate", "doesn't contain any ip sans"):
		return "服务端证书和当前地址不匹配，请检查服务端地址或重新下发证书"
	case containsAny(text, "unknown authority", "not trusted"):
		return "服务端证书不受信任，请联系管理员重新下发证书"
	case containsAny(text, "reverse listen address is not configured on server"):
		return "反向监听地址未在服务端配置，请联系管理员检查保留端口"
	case containsAny(text, "not allowed to activate this reverse listen address"):
		return "当前账号无权使用该反向端口，请联系管理员检查端口授权"
	case containsAny(text, "target_denied", "not allowed"):
		return "当前账号没有访问该端口的权限，请联系管理员检查端口授权"
	case containsAny(text, "already activated"):
		return "该被动端口已被其他客户端占用"
	case containsAny(text, "server passive port is unavailable", "listen reverse"):
		return "服务端被动端口不可用，客户端会自动重试，请联系管理员检查服务端端口占用"
	case containsAny(text, "already in use", "only one usage", "bind:"):
		return "本地端口已被占用，请关闭占用程序或调整端口"
	case containsAny(text, "connect client target"):
		return "客户端目标服务不可达，请确认对应服务已启动并监听配置端口"
	case containsAny(text, "target_unreachable", "target service is unreachable"):
		return "服务端无法访问目标服务，请联系管理员检查目标服务或防火墙"
	case containsAny(text, "connection refused", "actively refused", "connectex"):
		return "服务端暂时不可达，客户端会自动重试"
	case containsAny(text, "timeout", "deadline", "i/o timeout"):
		return "连接超时，客户端会自动重试"
	case containsAny(text, "network is unreachable", "no route to host", "no such host"):
		return "网络不可达，请检查本机网络或服务端地址"
	case containsAny(text, "connection reset", "forcibly closed", "wsarecv", "eof"):
		return "连接被断开，客户端会自动重试"
	case containsAny(text, "connection aborted", "broken pipe", "closed network connection"):
		return "连接已中断，客户端会自动重试"
	case containsAny(text, "x509"):
		return "服务端证书校验失败，请联系管理员检查证书"
	case containsAny(text, "tls handshake after certificate possession gate"):
		return "TLS 握手失败，客户端会自动重试；若持续出现请确认两端版本"
	case containsAny(text, "certificate"):
		return "服务端证书校验失败，请联系管理员检查证书"
	case containsAny(text, "tls"):
		return "TLS 握手失败，客户端会自动重试；若持续出现请确认两端版本"
	default:
		return "规则连接异常，客户端会自动重试；若持续出现请查看客户端日志"
	}
}

func IsPermanentForwardError(err error) bool {
	if err == nil {
		return false
	}
	if policy, ok := policyForResponse(err); ok {
		return policy.permanent
	}
	text := strings.ToLower(err.Error())
	if _, permanent, ok := classifyVirtualForwardError(text); ok {
		return permanent
	}
	if containsAny(
		text,
		"connection refused",
		"actively refused",
		"connectex",
		"timeout",
		"deadline",
		"i/o timeout",
		"network is unreachable",
		"no route to host",
		"no such host",
		"connection reset",
		"forcibly closed",
		"wsarecv",
		"eof",
		"connection aborted",
		"broken pipe",
		"closed network connection",
	) {
		return false
	}
	if containsAny(text, "tls handshake after certificate possession gate") && !containsAny(text, "x509") {
		return false
	}
	return containsAny(
		text,
		"auth_failed",
		"username or password",
		"credential_expired",
		"saved login has expired",
		"client_version_unsupported",
		"protocol_version_unsupported",
		"certificate",
		"x509",
		"target_denied",
		"not allowed",
		"not configured on server",
	)
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func isTemporaryAuthBlockText(text string) bool {
	return containsAny(text, "auth_blocked", "too many login failures")
}

func isMissingServerCertError(text string) bool {
	if !containsAny(text, "server.crt", "ca_cert_file") {
		return false
	}
	return containsAny(text, "no such file", "cannot find", "找不到", "系统找不到")
}
