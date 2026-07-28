package com.lsyl.tunnel.mobile.tunnel

import com.lsyl.tunnel.mobile.profile.ForwardConfig
import com.lsyl.tunnel.mobile.profile.LoadedProfile
import com.lsyl.tunnel.mobile.profile.MobileProfile
import com.lsyl.tunnel.mobile.protocol.ProtocolClient
import com.lsyl.tunnel.mobile.protocol.ProtocolException
import com.lsyl.tunnel.mobile.protocol.TunnelProtocol
import com.lsyl.tunnel.mobile.runtime.FailureClassifier
import com.lsyl.tunnel.mobile.runtime.IssueDisposition
import com.lsyl.tunnel.mobile.service.ManagedTunnel
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicBoolean

class TunnelManager(
    private val loaded: LoadedProfile,
    protocolOverride: TunnelProtocol? = null,
    onStatsChanged: () -> Unit = {}
) : ManagedTunnel {
    private val profile: MobileProfile = loaded.profile
    private val protocol: TunnelProtocol = protocolOverride ?: ProtocolClient(profile, loaded.serverCertBytes)
    private val registry = RuntimeRegistry(profile.forwards, onStatsChanged)
    private val executors = TunnelExecutors()
    private val lifecycle = TunnelLifecycleGate()
    private val forwards = ConcurrentHashMap<String, LocalForward>()
    private val running = AtomicBoolean(false)
    @Volatile private var message: String = "未连接"

    fun start() {
        startListeners()
        checkRemote()
    }

    override fun startListeners() {
        if (!running.compareAndSet(false, true)) return
        message = "正在启动本地监听"
        profile.forwards.forEach { forward ->
            startForward(forward)
        }
        message = if (hasIssue()) "部分连接不可用，请联系管理员" else "已连接"
    }

    override fun checkRemote(onStage: (String) -> Unit) {
        check(running.get()) { "tunnel is not running" }
        onStage("连接服务端并验证账号")
        protocol.health()
        if (!running.get()) return
        onStage("检查转发规则")
        profile.forwards.forEach { forward ->
            val runtime = registry.runtime(forward)
            try {
                protocol.forwardCheck(forward)
                if (!running.get()) return
                runtime.clearIssue()
                val existing = forwards[forward.displayName()]
                if (existing == null || !existing.isRunning()) {
                    startForward(forward)
                }
            } catch (err: ProtocolException) {
                val endpoint = forward.localEndpoint()
                val issue = FailureClassifier.classify(err, forward.displayName(), endpoint.port)
                when (issue.disposition) {
                    IssueDisposition.RULE_DISABLED -> {
                        forwards.remove(forward.displayName())?.stop(issue.message, ForwardState.REJECTED, issue)
                        runtime.setState(ForwardState.REJECTED, issue.message)
                        runtime.reportIssue(issue)
                    }
                    IssueDisposition.CONNECTION_ONLY -> runtime.reportIssue(issue)
                    IssueDisposition.TRANSIENT,
                    IssueDisposition.FATAL -> throw err
                }
            }
        }
        message = if (hasIssue()) "部分连接不可用，请联系管理员" else "已连接"
    }

    fun refresh() = checkRemote {}

    override fun stop() {
        val stopped = lifecycle.stopIfRunning(running) {
            protocol.cancelPending()
            forwards.values.forEach { it.stop() }
            forwards.clear()
        }
        if (!stopped) return
        executors.shutdown()
        message = "已断开"
    }

    override fun stats(): TunnelStats = TunnelStats(running.get(), message, registry.snapshot())

    private fun hasIssue(): Boolean = registry.snapshot().any {
        it.issue != null || it.state == ForwardState.REJECTED || it.state == ForwardState.LISTEN_FAILED || it.state == ForwardState.ERROR
    }

    private fun startForward(forward: ForwardConfig) {
        lifecycle.runIfRunning(running) {
            val runtime = registry.runtime(forward)
            val local = LocalForward(forward, protocol, runtime, executors.listeners, executors.connections, executors.copies)
            forwards[forward.displayName()] = local
            local.start()
            if (!local.isRunning()) forwards.remove(forward.displayName(), local)
        }
    }
}
