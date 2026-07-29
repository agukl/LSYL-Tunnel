package com.lsyl.tunnel.mobile.service

import java.util.concurrent.atomic.AtomicBoolean

object ServiceRecoveryPolicy {
    fun shouldRecover(desiredRunning: Boolean, serviceActive: Boolean): Boolean =
        desiredRunning && !serviceActive
}

class ServiceProcessState {
    private val active = AtomicBoolean(false)

    fun markActive() {
        active.set(true)
    }

    fun markInactive() {
        active.set(false)
    }

    fun isActive(): Boolean = active.get()
}

object TunnelServiceProcessState {
    private val state = ServiceProcessState()

    fun markActive() = state.markActive()

    fun markInactive() = state.markInactive()

    fun isActive(): Boolean = state.isActive()
}
