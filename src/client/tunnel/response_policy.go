package tunnel

import (
	"errors"
	"strings"
	"time"

	"lsyltunnel/src/internal/protocol"
)

type responsePolicy struct {
	healthState    string
	healthMessage  string
	forwardMessage string
	permanent      bool
	retryDelay     time.Duration
}

// Known wire codes take precedence over diagnostic text, including wrapped errors.
// reverse_failed still groups several causes in the existing protocol, so it and
// unknown codes use the legacy text classifier until the protocol distinguishes them.
func policyForResponse(err error) (responsePolicy, bool) {
	var response *protocol.ResponseError
	if !errors.As(err, &response) || response == nil {
		return responsePolicy{}, false
	}
	policy := responsePolicy{healthState: HealthAuthError, permanent: true}
	switch response.Code {
	case "client_version_unsupported":
		policy.healthMessage = "客户端版本不在服务端允许范围内，请联系管理员确认两端版本"
	case "protocol_version_unsupported", "bad_request":
		policy.healthMessage = "客户端与服务端协议版本不一致，请联系管理员升级对应一端"
		// bad_request was not a terminal forward error in the existing protocol.
		policy.permanent = response.Code != "bad_request"
	case "auth_failed":
		policy.healthMessage = "账号或密码不正确，需要重新登录"
		policy.forwardMessage = "账号或密码不正确，请重新登录"
	case "credential_expired":
		policy.healthMessage = "保存的登录凭据已过期，需要重新登录"
		policy.forwardMessage = "保存的登录凭据已过期，请重新输入密码"
	case "auth_blocked":
		policy.healthMessage = "登录失败次数过多，当前来源 IP 已被临时封禁"
		policy.forwardMessage = "登录失败次数过多，当前来源 IP 已被临时封禁，请稍后再试"
		policy.permanent = false
		policy.retryDelay = 5 * time.Minute
	case "target_denied":
		policy.healthState = HealthServerUnavailable
		policy.healthMessage = "服务端暂时不可达"
		policy.forwardMessage = "当前账号没有访问该端口的权限，请联系管理员检查端口授权"
		text := strings.ToLower(response.Message)
		if strings.Contains(text, "reverse listen address is not configured on server") {
			policy.forwardMessage = "反向监听地址未在服务端配置，请联系管理员检查保留端口"
		} else if strings.Contains(text, "not allowed to activate this reverse listen address") {
			policy.forwardMessage = "当前账号无权使用该反向端口，请联系管理员检查端口授权"
		}
	case "user_stream_limit", "target_unreachable":
		policy.healthState = HealthServerUnavailable
		policy.healthMessage = "服务端暂时不可达"
		policy.permanent = false
		if response.Code == "user_stream_limit" {
			policy.forwardMessage = "当前账号并发连接数已达到上限，请关闭部分连接后重试"
		} else {
			policy.forwardMessage = "服务端无法访问目标服务，请联系管理员检查目标服务或防火墙"
		}
	default:
		return responsePolicy{}, false
	}
	if policy.forwardMessage == "" {
		policy.forwardMessage = policy.healthMessage
	}
	return policy, true
}
