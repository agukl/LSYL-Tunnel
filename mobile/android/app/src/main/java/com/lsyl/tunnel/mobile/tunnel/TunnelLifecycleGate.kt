package com.lsyl.tunnel.mobile.tunnel

import java.util.concurrent.atomic.AtomicBoolean

internal class TunnelLifecycleGate {
    private val monitor = Any()

    fun runIfRunning(running: AtomicBoolean, action: () -> Unit): Boolean = synchronized(monitor) {
        if (!running.get()) return@synchronized false
        action()
        true
    }

    fun stopIfRunning(running: AtomicBoolean, action: () -> Unit): Boolean = synchronized(monitor) {
        if (!running.compareAndSet(true, false)) return@synchronized false
        action()
        true
    }
}
