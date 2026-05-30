package com.lsyl.tunnel.mobile.tunnel

import com.lsyl.tunnel.mobile.profile.ForwardConfig
import java.time.Instant
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicLong

enum class ForwardState {
    STARTING,
    LISTENING,
    REJECTED,
    LISTEN_FAILED,
    ERROR,
    STOPPED
}

data class ForwardStatus(
    val name: String,
    val listenAddr: String,
    val serverTarget: String,
    val state: ForwardState,
    val message: String,
    val active: Int,
    val total: Long,
    val lastChanged: Instant
)

class ForwardRuntime(forward: ForwardConfig) {
    private val active = AtomicInteger(0)
    private val total = AtomicLong(0)
    @Volatile private var state: ForwardState = ForwardState.STARTING
    @Volatile private var message: String = "正在启动"
    @Volatile private var lastChanged: Instant = Instant.now()

    val name: String = forward.displayName()
    val listenAddr: String = forward.listenAddr
    val serverTarget: String = forward.serverTarget

    fun setState(next: ForwardState, text: String) {
        state = next
        message = text
        lastChanged = Instant.now()
    }

    fun beginStream(): () -> Unit {
        active.incrementAndGet()
        total.incrementAndGet()
        return { active.decrementAndGet() }
    }

    fun snapshot(): ForwardStatus = ForwardStatus(
        name = name,
        listenAddr = listenAddr,
        serverTarget = serverTarget,
        state = state,
        message = message,
        active = active.get(),
        total = total.get(),
        lastChanged = lastChanged
    )
}

data class TunnelStats(
    val running: Boolean,
    val message: String,
    val forwards: List<ForwardStatus>
)

class RuntimeRegistry(forwards: List<ForwardConfig>) {
    private val items = ConcurrentHashMap<String, ForwardRuntime>()

    init {
        forwards.forEach { items[it.displayName()] = ForwardRuntime(it) }
    }

    fun runtime(forward: ForwardConfig): ForwardRuntime =
        items.getValue(forward.displayName())

    fun snapshot(): List<ForwardStatus> = items.values.map { it.snapshot() }.sortedBy { it.name }
}
