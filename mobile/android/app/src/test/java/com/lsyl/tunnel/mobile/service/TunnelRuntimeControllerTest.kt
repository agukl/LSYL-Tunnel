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
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

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

    @Test
    fun runtimeChangePublishesStreamFailureAndLaterRecovery() {
        val runtime = FakeRuntime(healthyStats())
        val store = FakeStateSink()
        val controller = TunnelRuntimeController({ runtime }, store)
        controller.connect(userInitiated = true)
        runtime.currentStats = TunnelStats(
            running = true,
            message = "部分连接不可用",
            forwards = listOf(
                forwardStatus(
                    "rdp",
                    13389,
                    ForwardState.LISTENING,
                    RuntimeIssue(
                        "target_unreachable",
                        "目标服务暂不可达，请联系管理员检查目标服务",
                        IssueDisposition.CONNECTION_ONLY,
                        "rdp",
                        13389
                    )
                )
            )
        )

        controller.runtimeChanged()

        assertEquals(TunnelPhase.DEGRADED, store.latest.phase)
        assertEquals("target_unreachable", store.latest.issues.single().code)

        runtime.currentStats = healthyStats()
        controller.runtimeChanged()

        assertEquals(TunnelPhase.CONNECTED, store.latest.phase)
        assertTrue(store.latest.issues.isEmpty())
    }

    @Test
    fun fatalStreamFailureStopsRuntimeAndClearsRestoreIntent() {
        val runtime = FakeRuntime(healthyStats())
        val store = FakeStateSink()
        val controller = TunnelRuntimeController({ runtime }, store)
        controller.connect(userInitiated = true)
        runtime.currentStats = TunnelStats(
            running = true,
            message = "认证失败",
            forwards = listOf(
                forwardStatus(
                    "rdp",
                    13389,
                    ForwardState.LISTENING,
                    RuntimeIssue(
                        "auth_failed",
                        "账号认证失败，请重新导入配置或联系管理员",
                        IssueDisposition.FATAL,
                        "rdp",
                        13389
                    )
                )
            )
        )

        controller.runtimeChanged()

        assertTrue(runtime.stopped)
        assertFalse(store.desiredRunning())
        assertEquals(TunnelPhase.FAILED, store.latest.phase)
        assertEquals("auth_failed", store.latest.issues.single().code)
    }

    @Test
    fun systemReleaseStopsRuntimeWithoutWaitingForRemoteCheck() {
        val checkStarted = CountDownLatch(1)
        val allowCheckToFinish = CountDownLatch(1)
        val runtimeStopped = CountDownLatch(1)
        val runtime = object : ManagedTunnel {
            override fun startListeners() = Unit

            override fun checkRemote(onStage: (String) -> Unit) {
                checkStarted.countDown()
                allowCheckToFinish.await(2, TimeUnit.SECONDS)
            }

            override fun stats(): TunnelStats = healthyStats()

            override fun stop() {
                runtimeStopped.countDown()
            }
        }
        val store = FakeStateSink()
        val controller = TunnelRuntimeController({ runtime }, store)
        val workers = Executors.newFixedThreadPool(2)
        val connect = workers.submit { controller.connect(userInitiated = true) }
        assertTrue(checkStarted.await(1, TimeUnit.SECONDS))
        val release = workers.submit { controller.releaseForSystem() }

        try {
            assertTrue("system release waited for remote I/O", runtimeStopped.await(300, TimeUnit.MILLISECONDS))
        } finally {
            allowCheckToFinish.countDown()
            release.get(2, TimeUnit.SECONDS)
            connect.get(2, TimeUnit.SECONDS)
            workers.shutdownNow()
        }

        assertTrue(store.desiredRunning())
        assertEquals(TunnelPhase.STARTING, store.latest.phase)
        assertEquals("等待系统恢复连接", store.latest.summary)
    }

    @Test
    fun systemReleaseCompletesUserStopWhenDisconnectCommandWasCancelled() {
        val runtime = FakeRuntime(healthyStats())
        val store = FakeStateSink()
        val controller = TunnelRuntimeController({ runtime }, store)
        controller.connect(userInitiated = true)
        store.setDesiredRunning(false)
        store.publish(RuntimeSnapshot(TunnelPhase.STOPPING, "正在断开"))

        controller.releaseForSystem(completePendingDisconnect = true)

        assertEquals(TunnelPhase.DISCONNECTED, store.latest.phase)
        assertEquals("已断开", store.latest.summary)
    }

    @Test
    fun systemReleaseCannotBeOverwrittenByConcurrentStoppingSnapshot() {
        val runtime = FakeRuntime(healthyStats())
        val store = BlockingStopStateSink()
        val controller = TunnelRuntimeController({ runtime }, store)
        controller.connect(userInitiated = true)
        val workers = Executors.newFixedThreadPool(2)
        val disconnect = workers.submit { controller.disconnect() }
        assertTrue(store.stopTransitionStarted.await(1, TimeUnit.SECONDS))
        val releaseAttempted = CountDownLatch(1)
        val release = workers.submit {
            releaseAttempted.countDown()
            controller.releaseForSystem(completePendingDisconnect = true)
        }
        assertTrue(releaseAttempted.await(1, TimeUnit.SECONDS))

        try {
            Thread.sleep(100)
        } finally {
            store.allowStopTransition.countDown()
            disconnect.get(2, TimeUnit.SECONDS)
            release.get(2, TimeUnit.SECONDS)
            workers.shutdownNow()
        }

        assertEquals(TunnelPhase.DISCONNECTED, store.latest.phase)
    }

    @Test
    fun systemReleaseCannotBeOverwrittenByInFlightRuntimeSnapshot() {
        val runtime = BlockingStatsRuntime(healthyStats())
        val store = FakeStateSink()
        val controller = TunnelRuntimeController({ runtime }, store)
        controller.connect(userInitiated = true)
        runtime.blockStats = true
        val worker = Executors.newSingleThreadExecutor()
        val networkLoss = worker.submit { controller.networkUnavailable() }
        assertTrue(runtime.statsStarted.await(1, TimeUnit.SECONDS))

        controller.releaseForSystem()
        runtime.allowStats.countDown()
        networkLoss.get(2, TimeUnit.SECONDS)
        worker.shutdownNow()

        assertEquals(TunnelPhase.STARTING, store.latest.phase)
        assertEquals("等待系统恢复连接", store.latest.summary)
    }

    @Test
    fun systemReleaseSerializesRestoreIntentWithConcurrentConnect() {
        val runtime = FakeRuntime(healthyStats())
        val store = BlockingStartStateSink()
        val controller = TunnelRuntimeController({ runtime }, store)
        val workers = Executors.newFixedThreadPool(2)
        val connect = workers.submit { controller.connect(userInitiated = true) }
        assertTrue(store.startTransitionStarted.await(1, TimeUnit.SECONDS))
        val releaseAttempted = CountDownLatch(1)
        val release = workers.submit {
            releaseAttempted.countDown()
            controller.releaseForSystem(completePendingDisconnect = true)
        }
        assertTrue(releaseAttempted.await(1, TimeUnit.SECONDS))

        try {
            Thread.sleep(100)
        } finally {
            store.allowStartTransition.countDown()
            connect.get(2, TimeUnit.SECONDS)
            release.get(2, TimeUnit.SECONDS)
            workers.shutdownNow()
        }

        assertTrue(store.desiredRunning())
        assertEquals(TunnelPhase.STARTING, store.latest.phase)
        assertEquals("等待系统恢复连接", store.latest.summary)
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

    private class FakeRuntime(var currentStats: TunnelStats) : ManagedTunnel {
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

    private class BlockingStopStateSink : RuntimeStateSink {
        @Volatile private var desired = false
        @Volatile var latest = RuntimeSnapshot(TunnelPhase.DISCONNECTED, "未连接")
        val stopTransitionStarted = CountDownLatch(1)
        val allowStopTransition = CountDownLatch(1)

        override fun desiredRunning(): Boolean = desired

        override fun setDesiredRunning(value: Boolean) {
            desired = value
            if (!value) {
                stopTransitionStarted.countDown()
                allowStopTransition.await(2, TimeUnit.SECONDS)
            }
        }

        override fun publish(snapshot: RuntimeSnapshot) {
            latest = snapshot
        }
    }

    private class BlockingStatsRuntime(private val currentStats: TunnelStats) : ManagedTunnel {
        @Volatile var blockStats = false
        val statsStarted = CountDownLatch(1)
        val allowStats = CountDownLatch(1)

        override fun startListeners() = Unit

        override fun checkRemote(onStage: (String) -> Unit) = Unit

        override fun stats(): TunnelStats {
            if (blockStats) {
                statsStarted.countDown()
                allowStats.await(2, TimeUnit.SECONDS)
            }
            return currentStats
        }

        override fun stop() = Unit
    }

    private class BlockingStartStateSink : RuntimeStateSink {
        @Volatile private var desired = false
        @Volatile var latest = RuntimeSnapshot(TunnelPhase.DISCONNECTED, "未连接")
        val startTransitionStarted = CountDownLatch(1)
        val allowStartTransition = CountDownLatch(1)

        override fun desiredRunning(): Boolean = desired

        override fun setDesiredRunning(value: Boolean) {
            if (value) {
                startTransitionStarted.countDown()
                allowStartTransition.await(2, TimeUnit.SECONDS)
            }
            desired = value
        }

        override fun publish(snapshot: RuntimeSnapshot) {
            latest = snapshot
        }
    }
}
