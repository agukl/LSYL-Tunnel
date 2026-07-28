package com.lsyl.tunnel.mobile.service

import com.lsyl.tunnel.mobile.runtime.FailureClassifier
import com.lsyl.tunnel.mobile.runtime.IssueDisposition
import com.lsyl.tunnel.mobile.runtime.RuntimeIssue
import com.lsyl.tunnel.mobile.tunnel.ForwardState
import com.lsyl.tunnel.mobile.tunnel.ForwardStatus
import com.lsyl.tunnel.mobile.tunnel.TunnelStats

interface ManagedTunnel {
    fun startListeners()
    fun checkRemote(onStage: (String) -> Unit = {})
    fun stats(): TunnelStats
    fun stop()
}

interface RuntimeStateSink {
    fun desiredRunning(): Boolean
    fun setDesiredRunning(value: Boolean)
    fun publish(snapshot: RuntimeSnapshot)
}

class TunnelRuntimeController(
    private val runtimeFactory: () -> ManagedTunnel,
    private val stateSink: RuntimeStateSink
) {
    private val lifecycleLock = Any()
    @Volatile private var runtime: ManagedTunnel? = null
    @Volatile private var released = false

    fun connect(userInitiated: Boolean) {
        val shouldConnect = synchronized(lifecycleLock) {
            if (released) return@synchronized false
            if (userInitiated) stateSink.setDesiredRunning(true)
            stateSink.desiredRunning()
        }
        if (!shouldConnect) return
        runtime?.let {
            publishStats(it.stats(), it)
            return
        }

        if (!publishIfNotReleased(RuntimeSnapshot(TunnelPhase.STARTING, "读取配置"))) return
        try {
            val created = runtimeFactory()
            val installed = synchronized(lifecycleLock) {
                if (released || runtime != null) {
                    false
                } else {
                    runtime = created
                    stateSink.publish(RuntimeSnapshot(TunnelPhase.STARTING, "启动本地监听"))
                    created.startListeners()
                    true
                }
            }
            if (!installed) {
                created.stop()
                return
            }
            if (released) return
            if (!hasUsableListener(created.stats())) {
                failWithoutListener(created.stats())
                return
            }
            checkRuntime(created)
        } catch (failure: Throwable) {
            if (!released) handleFailure(failure)
        }
    }

    fun refresh() {
        if (released) return
        val current = runtime
        if (current == null) {
            if (stateSink.desiredRunning()) connect(userInitiated = false)
            else publishIfNotReleased(RuntimeSnapshot(TunnelPhase.DISCONNECTED, "未连接"))
            return
        }
        checkRuntime(current)
    }

    fun networkUnavailable() {
        if (released) return
        val current = runtime ?: return
        if (!stateSink.desiredRunning()) return
        val stats = current.stats()
        val issue = RuntimeIssue(
            code = "network_unavailable",
            message = "网络暂不可用，等待恢复",
            disposition = IssueDisposition.TRANSIENT
        )
        publishIfActive(
            current,
            RuntimeSnapshot(
                phase = TunnelPhase.WAITING_NETWORK,
                summary = issue.message,
                listenerCount = listenerCount(stats),
                issues = listOf(issue) + safeIssues(stats)
            )
        )
    }

    fun runtimeChanged() {
        if (released) return
        val current = runtime ?: return
        if (!stateSink.desiredRunning()) return
        val stats = current.stats()
        val fatalIssue = safeIssues(stats).firstOrNull { it.disposition == IssueDisposition.FATAL }
        if (fatalIssue != null) {
            failRuntime(listOf(fatalIssue))
            return
        }
        publishStats(stats, current)
    }

    fun disconnect() {
        synchronized(lifecycleLock) {
            if (released) return
            stateSink.setDesiredRunning(false)
            stateSink.publish(RuntimeSnapshot(TunnelPhase.STOPPING, "正在断开"))
            runtime?.stop()
            runtime = null
            stateSink.publish(RuntimeSnapshot(TunnelPhase.DISCONNECTED, "已断开"))
        }
    }

    fun releaseForSystem(completePendingDisconnect: Boolean = false) {
        val current = synchronized(lifecycleLock) {
            released = true
            val detached = runtime
            runtime = null
            if (stateSink.desiredRunning()) {
                stateSink.publish(RuntimeSnapshot(TunnelPhase.STARTING, "等待系统恢复连接"))
            } else if (completePendingDisconnect) {
                stateSink.publish(RuntimeSnapshot(TunnelPhase.DISCONNECTED, "已断开"))
            }
            detached
        }
        current?.stop()
    }

    private fun checkRuntime(current: ManagedTunnel) {
        if (released || runtime !== current) return
        try {
            current.checkRemote { stage ->
                if (released || runtime !== current) return@checkRemote
                val stats = current.stats()
                publishIfActive(
                    current,
                    RuntimeSnapshot(
                        phase = TunnelPhase.STARTING,
                        summary = stage,
                        listenerCount = listenerCount(stats),
                        issues = safeIssues(stats)
                    )
                )
            }
            if (released || runtime !== current) return
            publishStats(current.stats(), current)
        } catch (failure: Throwable) {
            if (!released && runtime === current) handleFailure(failure, current)
        }
    }

    private fun handleFailure(failure: Throwable, expectedRuntime: ManagedTunnel? = runtime) {
        if (released) return
        val issue = FailureClassifier.classify(failure)
        if (issue.disposition == IssueDisposition.FATAL) {
            failRuntime(listOf(issue))
            return
        }

        val stats = expectedRuntime?.stats()
        val snapshot =
            RuntimeSnapshot(
                phase = TunnelPhase.WAITING_NETWORK,
                summary = issue.message,
                listenerCount = stats?.let(::listenerCount) ?: 0,
                issues = listOf(issue) + (stats?.let(::safeIssues) ?: emptyList())
            )
        if (expectedRuntime == null) publishIfNotReleased(snapshot) else publishIfActive(expectedRuntime, snapshot)
    }

    private fun publishStats(stats: TunnelStats, current: ManagedTunnel) {
        if (!hasUsableListener(stats)) {
            failWithoutListener(stats)
            return
        }
        val issues = safeIssues(stats)
        publishIfActive(
            current,
            RuntimeSnapshot(
                phase = if (issues.isEmpty()) TunnelPhase.CONNECTED else TunnelPhase.DEGRADED,
                summary = if (issues.isEmpty()) "已连接" else "部分端口不可用",
                listenerCount = listenerCount(stats),
                issues = issues
            )
        )
    }

    private fun failWithoutListener(stats: TunnelStats) {
        val issues = safeIssues(stats).ifEmpty {
            listOf(
                RuntimeIssue(
                    code = "no_local_listener",
                    message = "没有可用的本地监听端口",
                    disposition = IssueDisposition.FATAL
                )
            )
        }
        failRuntime(issues)
    }

    private fun failRuntime(issues: List<RuntimeIssue>) {
        val current = synchronized(lifecycleLock) {
            if (released) return
            val detached = runtime
            runtime = null
            stateSink.setDesiredRunning(false)
            stateSink.publish(RuntimeSnapshot(TunnelPhase.FAILED, issues.first().message, issues = issues))
            detached
        }
        current?.stop()
    }

    private fun publishIfNotReleased(snapshot: RuntimeSnapshot): Boolean = synchronized(lifecycleLock) {
        if (released) return@synchronized false
        stateSink.publish(snapshot)
        true
    }

    private fun publishIfActive(current: ManagedTunnel, snapshot: RuntimeSnapshot): Boolean =
        synchronized(lifecycleLock) {
            if (released || runtime !== current) return@synchronized false
            stateSink.publish(snapshot)
            true
        }

    private fun hasUsableListener(stats: TunnelStats): Boolean = listenerCount(stats) > 0

    private fun listenerCount(stats: TunnelStats): Int =
        stats.forwards.count { it.state == ForwardState.LISTENING }

    private fun safeIssues(stats: TunnelStats): List<RuntimeIssue> =
        stats.forwards.mapNotNull { status -> status.issue ?: listenerIssue(status) }

    private fun listenerIssue(status: ForwardStatus): RuntimeIssue? {
        if (status.state != ForwardState.LISTEN_FAILED && status.state != ForwardState.ERROR) return null
        val port = status.listenAddr.substringAfterLast(':').toIntOrNull()
        return RuntimeIssue(
            code = if (status.state == ForwardState.LISTEN_FAILED) "local_listen_failed" else "local_listener_interrupted",
            message = if (port != null) "${status.name}：本地端口 $port 监听失败" else "${status.name}：本地监听失败",
            disposition = IssueDisposition.RULE_DISABLED,
            ruleName = status.name,
            localPort = port
        )
    }
}
