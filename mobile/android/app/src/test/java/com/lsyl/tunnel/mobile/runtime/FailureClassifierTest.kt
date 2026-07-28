package com.lsyl.tunnel.mobile.runtime

import com.lsyl.tunnel.mobile.protocol.OpenResponse
import com.lsyl.tunnel.mobile.protocol.ProtocolException
import org.junit.Assert.assertEquals
import org.junit.Test
import java.net.ConnectException
import java.net.BindException
import java.net.NoRouteToHostException
import java.net.SocketTimeoutException
import java.net.UnknownHostException
import java.security.cert.CertificateException
import javax.net.ssl.SSLHandshakeException

class FailureClassifierTest {
    @Test
    fun occupiedLocalPortDisablesOnlyThatRule() {
        val issue = FailureClassifier.classify(
            BindException("Address already in use"),
            ruleName = "rdp",
            localPort = 13389
        )

        assertEquals("local_port_in_use", issue.code)
        assertEquals("本地端口 13389 已被占用", issue.message)
        assertEquals(IssueDisposition.RULE_DISABLED, issue.disposition)
    }

    @Test
    fun networkFailuresRemainRecoverable() {
        val cases = listOf(
            FailureCase(UnknownHostException("missing"), "network_dns", "无法解析服务端地址，请检查当前网络"),
            FailureCase(SocketTimeoutException("timed out"), "network_timeout", "连接服务端超时，等待网络恢复"),
            FailureCase(ConnectException("Connection refused"), "connection_refused", "服务端端口拒绝连接，请联系管理员检查服务状态"),
            FailureCase(NoRouteToHostException("no route"), "network_unavailable", "网络暂不可用，等待恢复")
        )

        cases.forEach { case ->
            val issue = FailureClassifier.classify(case.failure)
            assertEquals(case.code, issue.code)
            assertEquals(case.message, issue.message)
            assertEquals(IssueDisposition.TRANSIENT, issue.disposition)
        }
    }

    @Test
    fun certificateFailureRequiresNewProfile() {
        val failure = SSLHandshakeException("PKIX path building failed").apply {
            initCause(CertificateException("certificate mismatch"))
        }

        val issue = FailureClassifier.classify(failure)

        assertEquals("certificate_mismatch", issue.code)
        assertEquals("服务端证书不匹配，请重新导入管理员下发的配置", issue.message)
        assertEquals(IssueDisposition.FATAL, issue.disposition)
    }

    @Test
    fun protocolCodesHaveExplicitRecoveryPolicies() {
        val cases = listOf(
            ProtocolCase("credential_expired", "连接凭据已过期，请重新导入配置", IssueDisposition.FATAL),
            ProtocolCase("auth_failed", "账号认证失败，请重新导入配置或联系管理员", IssueDisposition.FATAL),
            ProtocolCase("auth_blocked", "登录失败次数过多，当前网络来源已被临时封禁，请稍后重试", IssueDisposition.TRANSIENT),
            ProtocolCase("client_version_unsupported", "客户端版本不在服务端允许范围内，请联系管理员", IssueDisposition.FATAL),
            ProtocolCase("protocol_version_unsupported", "客户端与服务端协议版本不一致，请联系管理员升级", IssueDisposition.FATAL),
            ProtocolCase("user_stream_limit", "账号并发连接已达上限，请稍后重试", IssueDisposition.CONNECTION_ONLY),
            ProtocolCase("target_denied", "当前账号没有访问该端口的权限", IssueDisposition.RULE_DISABLED),
            ProtocolCase("target_unreachable", "目标服务暂不可达，请联系管理员检查目标服务", IssueDisposition.CONNECTION_ONLY)
        )

        cases.forEach { case ->
            val failure = ProtocolException(OpenResponse(false, case.code, "server detail"))
            val issue = FailureClassifier.classify(failure, ruleName = "rdp", localPort = 13389)
            assertEquals(case.code, issue.code)
            assertEquals(case.message, issue.message)
            assertEquals(case.disposition, issue.disposition)
            assertEquals("rdp", issue.ruleName)
            assertEquals(13389, issue.localPort)
        }
    }

    private data class FailureCase(val failure: Throwable, val code: String, val message: String)

    private data class ProtocolCase(
        val code: String,
        val message: String,
        val disposition: IssueDisposition
    )
}
