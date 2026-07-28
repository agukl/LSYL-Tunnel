package com.lsyl.tunnel.mobile.runtime

import com.lsyl.tunnel.mobile.protocol.ProtocolException
import java.net.ConnectException
import java.net.NoRouteToHostException
import java.net.SocketTimeoutException
import java.net.UnknownHostException
import java.security.cert.CertificateException
import javax.net.ssl.SSLHandshakeException
import javax.net.ssl.SSLPeerUnverifiedException

object FailureClassifier {
    fun classify(failure: Throwable, ruleName: String? = null, localPort: Int? = null): RuntimeIssue {
        val protocolFailure = failure.findCause<ProtocolException>()
        if (protocolFailure != null) {
            return protocolIssue(protocolFailure.response.code, ruleName, localPort)
        }

        val classified = when {
            failure.isCertificateFailure() -> Classified(
                "certificate_mismatch",
                "服务端证书不匹配，请重新导入管理员下发的配置",
                IssueDisposition.FATAL
            )
            failure.findCause<UnknownHostException>() != null -> Classified(
                "network_dns",
                "无法解析服务端地址，请检查当前网络",
                IssueDisposition.TRANSIENT
            )
            failure.findCause<SocketTimeoutException>() != null -> Classified(
                "network_timeout",
                "连接服务端超时，等待网络恢复",
                IssueDisposition.TRANSIENT
            )
            failure.findCause<NoRouteToHostException>() != null -> Classified(
                "network_unavailable",
                "网络暂不可用，等待恢复",
                IssueDisposition.TRANSIENT
            )
            failure.findCause<ConnectException>() != null -> Classified(
                "connection_refused",
                "服务端端口拒绝连接，请联系管理员检查服务状态",
                IssueDisposition.TRANSIENT
            )
            failure.findCause<SSLHandshakeException>() != null -> Classified(
                "tls_handshake",
                "TLS 连接失败，请联系管理员检查服务端配置",
                IssueDisposition.FATAL
            )
            else -> Classified(
                "network_error",
                "服务端暂不可达，等待网络恢复",
                IssueDisposition.TRANSIENT
            )
        }
        return RuntimeIssue(classified.code, classified.message, classified.disposition, ruleName, localPort)
    }

    private fun protocolIssue(code: String, ruleName: String?, localPort: Int?): RuntimeIssue {
        val classified = when (code.lowercase()) {
            "credential_expired" -> Classified(
                code,
                "连接凭据已过期，请重新导入配置",
                IssueDisposition.FATAL
            )
            "auth_failed" -> Classified(
                code,
                "账号认证失败，请重新导入配置或联系管理员",
                IssueDisposition.FATAL
            )
            "auth_blocked" -> Classified(
                code,
                "登录失败次数过多，当前网络来源已被临时封禁，请稍后重试",
                IssueDisposition.TRANSIENT
            )
            "client_version_unsupported" -> Classified(
                code,
                "客户端版本不在服务端允许范围内，请联系管理员",
                IssueDisposition.FATAL
            )
            "protocol_version_unsupported" -> Classified(
                code,
                "客户端与服务端协议版本不一致，请联系管理员升级",
                IssueDisposition.FATAL
            )
            "user_stream_limit" -> Classified(
                code,
                "账号并发连接已达上限，请稍后重试",
                IssueDisposition.CONNECTION_ONLY
            )
            "target_denied" -> Classified(
                code,
                "当前账号没有访问该端口的权限",
                IssueDisposition.RULE_DISABLED
            )
            "target_unreachable" -> Classified(
                code,
                "目标服务暂不可达，请联系管理员检查目标服务",
                IssueDisposition.CONNECTION_ONLY
            )
            else -> Classified(
                code.ifBlank { "protocol_error" },
                "服务端返回无法识别的错误，请稍后重试",
                IssueDisposition.TRANSIENT
            )
        }
        return RuntimeIssue(classified.code, classified.message, classified.disposition, ruleName, localPort)
    }

    private fun Throwable.isCertificateFailure(): Boolean {
        if (findCause<CertificateException>() != null || findCause<SSLPeerUnverifiedException>() != null) return true
        return causeSequence().any { cause ->
            val text = cause.message.orEmpty().lowercase()
            "certificate pin mismatch" in text || "certpath" in text || "pkix" in text
        }
    }

    private inline fun <reified T : Throwable> Throwable.findCause(): T? =
        causeSequence().filterIsInstance<T>().firstOrNull()

    private fun Throwable.causeSequence(): Sequence<Throwable> = sequence {
        var current: Throwable? = this@causeSequence
        val visited = mutableSetOf<Throwable>()
        while (current != null && visited.add(current)) {
            yield(current)
            current = current.cause
        }
    }

    private data class Classified(
        val code: String,
        val message: String,
        val disposition: IssueDisposition
    )
}
