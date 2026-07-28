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
    private var runtime: ManagedTunnel? = null

    @Synchronized
    fun connect(userInitiated: Boolean) {
        if (userInitiated) stateSink.setDesiredRunning(true)
        if (!stateSink.desiredRunning()) return
        runtime?.let {
            publishStats(it.stats())
            return
        }

        stateSink.publish(RuntimeSnapshot(TunnelPhase.STARTING, "读取配置"))
        try {
            val created = runtimeFactory()
            runtime = created
            stateSink.publish(RuntimeSnapshot(TunnelPhase.STARTING, "启动本地监听"))
            created.startListeners()
            if (!hasUsableListener(created.stats())) {
                failWithoutListener(created.stats())
                return
            }
            checkRuntime(created)
        } catch (failure: Throwable) {
            handleFailure(failure)
        }
    }

    @Synchronized
    fun refresh() {
        val current = runtime
        if (current == null) {
            if (stateSink.desiredRunning()) connect(userInitiated = false)
            else stateSink.publish(RuntimeSnapshot(TunnelPhase.DISCONNECTED, "未连接"))
            return
        }
        checkRuntime(current)
    }

    @Synchronized
    fun networkUnavailable() {
        val current = runtime ?: return
        if (!stateSink.desiredRunning()) return
        val stats = current.stats()
        val issue = RuntimeIssue(
            code = "network_unavailable",
            message = "网络暂不可用，等待恢复",
            disposition = IssueDisposition.TRANSIENT
        )
        stateSink.publish(
            RuntimeSnapshot(
                phase = TunnelPhase.WAITING_NETWORK,
                summary = issue.message,
                listenerCount = listenerCount(stats),
                issues = listOf(issue) + safeIssues(stats)
            )
        )
    }

    @Synchronized
    fun disconnect() {
        stateSink.setDesiredRunning(false)
        stateSink.publish(RuntimeSnapshot(TunnelPhase.STOPPING, "正在断开"))
        runtime?.stop()
        runtime = null
        stateSink.publish(RuntimeSnapshot(TunnelPhase.DISCONNECTED, "已断开"))
    }

    @Synchronized
    fun releaseForSystem() {
        runtime?.stop()
        runtime = null
    }

    private fun checkRuntime(current: ManagedTunnel) {
        try {
            current.checkRemote { stage ->
                val stats = current.stats()
                stateSink.publish(
                    RuntimeSnapshot(
                        phase = TunnelPhase.STARTING,
                        summary = stage,
                        listenerCount = listenerCount(stats),
                        issues = safeIssues(stats)
                    )
                )
            }
            publishStats(current.stats())
        } catch (failure: Throwable) {
            handleFailure(failure)
        }
    }

    private fun handleFailure(failure: Throwable) {
        val issue = FailureClassifier.classify(failure)
        if (issue.disposition == IssueDisposition.FATAL) {
            runtime?.stop()
            runtime = null
            stateSink.setDesiredRunning(false)
            stateSink.publish(RuntimeSnapshot(TunnelPhase.FAILED, issue.message, issues = listOf(issue)))
            return
        }

        val stats = runtime?.stats()
        stateSink.publish(
            RuntimeSnapshot(
                phase = TunnelPhase.WAITING_NETWORK,
                summary = issue.message,
                listenerCount = stats?.let(::listenerCount) ?: 0,
                issues = listOf(issue) + (stats?.let(::safeIssues) ?: emptyList())
            )
        )
    }

    private fun publishStats(stats: TunnelStats) {
        if (!hasUsableListener(stats)) {
            failWithoutListener(stats)
            return
        }
        val issues = safeIssues(stats)
        stateSink.publish(
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
        runtime?.stop()
        runtime = null
        stateSink.setDesiredRunning(false)
        stateSink.publish(RuntimeSnapshot(TunnelPhase.FAILED, issues.first().message, issues = issues))
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
