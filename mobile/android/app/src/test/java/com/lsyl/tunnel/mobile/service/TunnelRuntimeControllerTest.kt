package com.lsyl.tunnel.mobile.service

import com.lsyl.tunnel.mobile.profile.ProfileImportException
import com.lsyl.tunnel.mobile.protocol.OpenResponse
import com.lsyl.tunnel.mobile.protocol.ProtocolException
import com.lsyl.tunnel.mobile.runtime.IssueDisposition
import com.lsyl.tunnel.mobile.runtime.RuntimeIssue
import com.lsyl.tunnel.mobile.tunnel.ForwardState
import com.lsyl.tunnel.mobile.tunnel.ForwardStatus
import com.lsyl.tunnel.mobile.tunnel.TunnelStats
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.SocketTimeoutException
import java.time.Instant

class TunnelRuntimeControllerTest {
    @Test
    fun transientRefreshFailureKeepsRuntimeAndDesiredIntent() {
        val runtime = FakeRuntime(healthyStats())
        val store = FakeStateSink()
        val controller = TunnelRuntimeController({ runtime }, store)
        controller.connect(userInitiated = true)
        runtime.checkFailure = SocketTimeoutException("timed out")

        controller.refresh()

        assertFalse(runtime.stopped)
        assertTrue(store.desiredRunning())
        assertEquals(TunnelPhase.WAITING_NETWORK, store.latest.phase)
        assertEquals("network_timeout", store.latest.issues.single().code)
    }

    @Test
    fun fatalCredentialFailureStopsRuntimeAndDisablesRestore() {
        val runtime = FakeRuntime(healthyStats()).apply {
            checkFailure = ProtocolException(OpenResponse(false, "credential_expired", "expired"))
        }
        val store = FakeStateSink()
        val controller = TunnelRuntimeController({ runtime }, store)

        controller.connect(userInitiated = true)

        assertTrue(runtime.stopped)
        assertFalse(store.desiredRunning())
        assertEquals(TunnelPhase.FAILED, store.latest.phase)
        assertEquals("credential_expired", store.latest.issues.single().code)
    }

    @Test
    fun invalidStoredProfileIsFatalBeforeRuntimeExists() {
        val store = FakeStateSink()
        val controller = TunnelRuntimeController(
            runtimeFactory = { throw ProfileImportException("配置损坏") },
            stateSink = store
        )

        controller.connect(userInitiated = true)

        assertFalse(store.desiredRunning())
        assertEquals(TunnelPhase.FAILED, store.latest.phase)
        assertEquals("invalid_profile", store.latest.issues.single().code)
    }

    @Test
    fun systemReleaseStopsSocketsButKeepsRestoreIntent() {
        val runtime = FakeRuntime(healthyStats())
        val store = FakeStateSink()
        val controller = TunnelRuntimeController({ runtime }, store)
        controller.connect(userInitiated = true)

        controller.releaseForSystem()

        assertTrue(runtime.stopped)
        assertTrue(store.desiredRunning())
    }

    @Test
    fun userDisconnectStopsRuntimeAndClearsRestoreIntent() {
        val runtime = FakeRuntime(healthyStats())
        val store = FakeStateSink()
        val controller = TunnelRuntimeController({ runtime }, store)
        controller.connect(userInitiated = true)

        controller.disconnect()

        assertTrue(runtime.stopped)
        assertFalse(store.desiredRunning())
        assertEquals(TunnelPhase.DISCONNECTED, store.latest.phase)
    }

    @Test
    fun duplicateConnectDoesNotCreateSecondRuntime() {
        val runtime = FakeRuntime(healthyStats())
        val store = FakeStateSink()
        var factoryCalls = 0
        val controller = TunnelRuntimeController(
            runtimeFactory = { factoryCalls++; runtime },
            stateSink = store
        )

        controller.connect(userInitiated = true)
        controller.connect(userInitiated = true)

        assertEquals(1, factoryCalls)
        assertEquals(1, runtime.startCalls)
        assertTrue(store.desiredRunning())
    }

    @Test
    fun oneFailedListenerProducesDegradedSnapshotWithoutStoppingHealthyRule() {
        val occupied = RuntimeIssue(
            "local_port_in_use",
            "本地端口 13390 已被占用",
            IssueDisposition.RULE_DISABLED,
            "database",
            13390
        )
        val stats = TunnelStats(
            running = true,
            message = "部分连接不可用",
            forwards = listOf(
                forwardStatus("rdp", 13389, ForwardState.LISTENING),
                forwardStatus("database", 13390, ForwardState.LISTEN_FAILED, occupied)
            )
        )
        val runtime = FakeRuntime(stats)
        val store = FakeStateSink()
        val controller = TunnelRuntimeController({ runtime }, store)

        controller.connect(userInitiated = true)

        assertFalse(runtime.stopped)
        assertEquals(TunnelPhase.DEGRADED, store.latest.phase)
        assertEquals(1, store.latest.listenerCount)
        assertEquals("local_port_in_use", store.latest.issues.single().code)
    }

    @Test
    fun networkLossPublishesWaitingStateWithoutCallingRemoteOrStopping() {
        val runtime = FakeRuntime(healthyStats())
        val store = FakeStateSink()
        val controller = TunnelRuntimeController({ runtime }, store)
        controller.connect(userInitiated = true)
        val checksBeforeLoss = runtime.checkCalls

        controller.networkUnavailable()

        assertEquals(checksBeforeLoss, runtime.checkCalls)
        assertFalse(runtime.stopped)
        assertEquals(TunnelPhase.WAITING_NETWORK, store.latest.phase)
    }

    private fun healthyStats(): TunnelStats = TunnelStats(
        running = true,
        message = "已连接",
        forwards = listOf(forwardStatus("rdp", 13389, ForwardState.LISTENING))
    )

    private fun forwardStatus(
        name: String,
        port: Int,
        state: ForwardState,
        issue: RuntimeIssue? = null
    ): ForwardStatus = ForwardStatus(
        name = name,
        listenAddr = "127.0.0.1:$port",
        serverTarget = "192.168.1.7:3389",
        state = state,
        message = issue?.message ?: "本地端口监听中",
        issue = issue,
        active = 0,
        total = 0,
        lastChanged = Instant.EPOCH
    )

    private class FakeRuntime(private var currentStats: TunnelStats) : ManagedTunnel {
        var checkFailure: Throwable? = null
        var startCalls = 0
        var checkCalls = 0
        var stopped = false

        override fun startListeners() {
            startCalls++
        }

        override fun checkRemote(onStage: (String) -> Unit) {
            checkCalls++
            onStage("连接服务端并验证账号")
            checkFailure?.let { throw it }
            onStage("检查转发规则")
        }

        override fun stats(): TunnelStats = currentStats

        override fun stop() {
            stopped = true
        }
    }

    private class FakeStateSink : RuntimeStateSink {
        private var desired = false
        var latest = RuntimeSnapshot(TunnelPhase.DISCONNECTED, "未连接")

        override fun desiredRunning(): Boolean = desired

        override fun setDesiredRunning(value: Boolean) {
            desired = value
        }

        override fun publish(snapshot: RuntimeSnapshot) {
            latest = snapshot
        }
    }
}
