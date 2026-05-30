package com.lsyl.tunnel.mobile.tunnel

import com.lsyl.tunnel.mobile.profile.LoadedProfile
import com.lsyl.tunnel.mobile.profile.MobileProfile
import com.lsyl.tunnel.mobile.protocol.ProtocolClient
import com.lsyl.tunnel.mobile.protocol.ProtocolException
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

class TunnelManager(private val loaded: LoadedProfile) {
    private val profile: MobileProfile = loaded.profile
    private val protocol = ProtocolClient(profile, loaded.serverCertBytes)
    private val registry = RuntimeRegistry(profile.forwards)
    private val executor: ExecutorService = Executors.newCachedThreadPool()
    private val forwards = ConcurrentHashMap<String, LocalForward>()
    private val running = AtomicBoolean(false)
    @Volatile private var message: String = "未连接"

    fun start() {
        if (!running.compareAndSet(false, true)) return
        message = "正在连接"
        protocol.health()
        profile.forwards.forEach { forward ->
            val runtime = registry.runtime(forward)
            try {
                protocol.forwardCheck(forward)
                val local = LocalForward(forward, protocol, runtime, executor)
                forwards[forward.displayName()] = local
                local.start()
            } catch (err: ProtocolException) {
                if (err.response.code == "target_denied") {
                    runtime.setState(ForwardState.REJECTED, "当前账号没有访问该端口的权限")
                } else {
                    runtime.setState(ForwardState.ERROR, err.message ?: "端口检查失败")
                }
            } catch (err: Exception) {
                runtime.setState(ForwardState.ERROR, err.message ?: "端口检查失败")
            }
        }
        message = if (hasIssue()) "部分连接不可用，请联系管理员" else "已连接"
    }

    fun refresh() {
        protocol.health()
        profile.forwards.forEach { forward ->
            val runtime = registry.runtime(forward)
            try {
                protocol.forwardCheck(forward)
                val existing = forwards[forward.displayName()]
                if (existing == null || !existing.isRunning()) {
                    val local = LocalForward(forward, protocol, runtime, executor)
                    forwards[forward.displayName()] = local
                    local.start()
                }
            } catch (err: ProtocolException) {
                val existing = forwards.remove(forward.displayName())
                if (err.response.code == "target_denied") {
                    existing?.stop("当前账号没有访问该端口的权限", ForwardState.REJECTED)
                    runtime.setState(ForwardState.REJECTED, "当前账号没有访问该端口的权限")
                } else {
                    existing?.stop(err.message ?: "端口检查失败", ForwardState.ERROR)
                    runtime.setState(ForwardState.ERROR, err.message ?: "端口检查失败")
                }
            } catch (err: Exception) {
                runtime.setState(ForwardState.ERROR, err.message ?: "端口检查失败")
            }
        }
        message = if (hasIssue()) "部分连接不可用，请联系管理员" else "已连接"
    }

    fun stop() {
        if (!running.compareAndSet(true, false)) return
        forwards.values.forEach { it.stop() }
        forwards.clear()
        executor.shutdownNow()
        message = "已断开"
    }

    fun stats(): TunnelStats = TunnelStats(running.get(), message, registry.snapshot())

    private fun hasIssue(): Boolean = registry.snapshot().any {
        it.state == ForwardState.REJECTED || it.state == ForwardState.LISTEN_FAILED || it.state == ForwardState.ERROR
    }
}
