package com.lsyl.tunnel.mobile.service

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicReference

class RecoverySignalGate {
    private val queued = AtomicBoolean(false)

    fun tryQueue(): Boolean = queued.compareAndSet(false, true)

    fun complete() {
        queued.set(false)
    }
}

class DefaultNetworkMonitor(
    context: Context,
    private val onAvailable: () -> Unit,
    private val onUnavailable: () -> Unit
) {
    private val connectivity = context.getSystemService(ConnectivityManager::class.java)
    private val registered = AtomicBoolean(false)
    private val lastAvailable = AtomicReference<Boolean?>(null)
    private val callback = object : ConnectivityManager.NetworkCallback() {
        override fun onAvailable(network: Network) {
            publish(true)
        }

        override fun onLost(network: Network) {
            publish(connectivity.activeNetwork != null)
        }
    }

    fun start() {
        if (!registered.compareAndSet(false, true)) return
        connectivity.registerDefaultNetworkCallback(callback)
        publish(connectivity.activeNetwork != null)
    }

    fun stop() {
        if (!registered.compareAndSet(true, false)) return
        try {
            connectivity.unregisterNetworkCallback(callback)
        } catch (_: IllegalArgumentException) {
        }
        lastAvailable.set(null)
    }

    private fun publish(available: Boolean) {
        if (lastAvailable.getAndSet(available) == available) return
        if (available) onAvailable() else onUnavailable()
    }
}
