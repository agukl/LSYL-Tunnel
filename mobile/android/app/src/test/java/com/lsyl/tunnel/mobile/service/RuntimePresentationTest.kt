package com.lsyl.tunnel.mobile.service

import com.lsyl.tunnel.mobile.runtime.IssueDisposition
import com.lsyl.tunnel.mobile.runtime.RuntimeIssue
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class RuntimePresentationTest {
    @Test
    fun connectedStateOffersDisconnectAndRefreshOnly() {
        val presentation = RuntimePresenter.present(
            RuntimeSnapshot(TunnelPhase.CONNECTED, "已连接", listenerCount = 1),
            hasProfile = true,
            desiredRunning = true
        )

        assertFalse(presentation.canConnect)
        assertTrue(presentation.canDisconnect)
        assertTrue(presentation.canRefresh)
        assertFalse(presentation.canEditConfig)
        assertEquals("", presentation.details)
    }

    @Test
    fun degradedStateFormatsOnlyRuleNameLocalPortAndSafeReason() {
        val presentation = RuntimePresenter.present(
            RuntimeSnapshot(
                phase = TunnelPhase.DEGRADED,
                summary = "部分端口不可用",
                listenerCount = 1,
                issues = listOf(
                    RuntimeIssue(
                        code = "target_denied",
                        message = "当前账号没有访问该端口的权限",
                        disposition = IssueDisposition.RULE_DISABLED,
                        ruleName = "rdp",
                        localPort = 13389
                    )
                )
            ),
            hasProfile = true,
            desiredRunning = true
        )

        assertEquals("rdp（本地端口 13389）：当前账号没有访问该端口的权限", presentation.details)
        assertFalse(presentation.details.contains("server_target"))
        assertFalse(presentation.details.contains("192.168."))
    }

    @Test
    fun localPortFailureDoesNotRepeatPortInDetails() {
        val presentation = RuntimePresenter.present(
            RuntimeSnapshot(
                phase = TunnelPhase.FAILED,
                summary = "本地端口 13389 已被占用",
                issues = listOf(
                    RuntimeIssue(
                        code = "local_port_in_use",
                        message = "本地端口 13389 已被占用",
                        disposition = IssueDisposition.RULE_DISABLED,
                        ruleName = "rdp",
                        localPort = 13389
                    )
                )
            ),
            hasProfile = true,
            desiredRunning = false
        )

        assertEquals("rdp：本地端口 13389 已被占用", presentation.details)
    }

    @Test
    fun waitingNetworkKeepsDisconnectAndRefreshAvailable() {
        val presentation = RuntimePresenter.present(
            RuntimeSnapshot(
                TunnelPhase.WAITING_NETWORK,
                "网络暂不可用，等待恢复",
                listenerCount = 1,
                issues = listOf(
                    RuntimeIssue("network_unavailable", "网络暂不可用，等待恢复", IssueDisposition.TRANSIENT)
                )
            ),
            hasProfile = true,
            desiredRunning = true
        )

        assertTrue(presentation.canDisconnect)
        assertTrue(presentation.canRefresh)
        assertFalse(presentation.canEditConfig)
        assertEquals("网络暂不可用，等待恢复", presentation.details)
    }

    @Test
    fun failedInactiveStateAllowsProfileRepairAndReconnect() {
        val presentation = RuntimePresenter.present(
            RuntimeSnapshot(TunnelPhase.FAILED, "连接凭据已过期，请重新导入配置"),
            hasProfile = true,
            desiredRunning = false
        )

        assertTrue(presentation.canConnect)
        assertFalse(presentation.canDisconnect)
        assertFalse(presentation.canRefresh)
        assertTrue(presentation.canEditConfig)
        assertTrue(presentation.canReplaceProfile)
    }

    @Test
    fun missingProfileCannotConnectOrEdit() {
        val presentation = RuntimePresenter.present(
            RuntimeSnapshot(TunnelPhase.DISCONNECTED, "未连接"),
            hasProfile = false,
            desiredRunning = false
        )

        assertFalse(presentation.canConnect)
        assertFalse(presentation.canEditConfig)
        assertTrue(presentation.canReplaceProfile)
    }

    @Test
    fun staleConnectedSnapshotCannotBlockReconnectAfterRuntimeIntentWasCleared() {
        val presentation = RuntimePresenter.present(
            RuntimeSnapshot(TunnelPhase.CONNECTED, "已连接", listenerCount = 1),
            hasProfile = true,
            desiredRunning = false
        )

        assertEquals("未连接", presentation.status)
        assertTrue(presentation.canConnect)
        assertTrue(presentation.canEditConfig)
        assertFalse(presentation.canDisconnect)
    }

    @Test
    fun stoppingStateCannotReplaceProfileUntilServiceHasDisconnected() {
        val presentation = RuntimePresenter.present(
            RuntimeSnapshot(TunnelPhase.STOPPING, "正在断开", listenerCount = 1),
            hasProfile = true,
            desiredRunning = false
        )

        assertFalse(presentation.canReplaceProfile)
        assertFalse(presentation.canEditConfig)
        assertFalse(presentation.canConnect)
    }
}
