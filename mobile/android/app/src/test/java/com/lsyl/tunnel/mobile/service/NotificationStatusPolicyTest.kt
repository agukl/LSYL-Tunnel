package com.lsyl.tunnel.mobile.service

import org.junit.Assert.assertEquals
import org.junit.Test

class NotificationStatusPolicyTest {
    @Test
    fun inactiveFailureKeepsAReadableFinalNotification() {
        val disposition = NotificationStatusPolicy.inactiveDisposition(
            RuntimeSnapshot(TunnelPhase.FAILED, "认证失败")
        )

        assertEquals(InactiveNotificationDisposition.SHOW_FAILURE, disposition)
    }

    @Test
    fun inactiveNonFailureRemovesTheNotification() {
        TunnelPhase.entries
            .filterNot { it == TunnelPhase.FAILED }
            .forEach { phase ->
                assertEquals(
                    phase.name,
                    InactiveNotificationDisposition.HIDE,
                    NotificationStatusPolicy.inactiveDisposition(RuntimeSnapshot(phase, phase.name))
                )
            }
    }

    @Test
    fun refreshDoesNotReuseAStaleConnectedSnapshot() {
        val entry = NotificationStatusPolicy.refreshEntry(
            RuntimeSnapshot(TunnelPhase.CONNECTED, "已连接", listenerCount = 2)
        )

        assertEquals(TunnelPhase.STARTING, entry.phase)
        assertEquals("正在检查连接", entry.summary)
        assertEquals(2, entry.listenerCount)
    }

    @Test
    fun callerProvidedStartingStateKeepsItsPreciseReason() {
        val entry = NotificationStatusPolicy.refreshEntry(
            RuntimeSnapshot(TunnelPhase.STARTING, "正在恢复连接", listenerCount = 1)
        )

        assertEquals(TunnelPhase.STARTING, entry.phase)
        assertEquals("正在恢复连接", entry.summary)
        assertEquals(1, entry.listenerCount)
    }
}
