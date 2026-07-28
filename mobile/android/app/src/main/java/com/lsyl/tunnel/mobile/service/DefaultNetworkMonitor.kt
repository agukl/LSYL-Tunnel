package com.lsyl.tunnel.mobile.service

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import java.util.concurrent.atomic.AtomicBoolean

class RecoverySignalGate {
    private val queued = AtomicBoolean(false)
    private val pending = AtomicBoolean(false)

    fun tryQueue(): Boolean {
        pending.set(true)
        return queued.compareAndSet(false, true)
    }

    fun drain(action: () -> Unit) {
        try {
            do {
                pending.set(false)
                action()
            } while (releaseOrReclaim())
        } catch (failure: Throwable) {
            cancel()
            throw failure
        }
    }

    fun cancel() {
        pending.set(false)
        queued.set(false)
    }

    private fun releaseOrReclaim(): Boolean {
        queued.set(false)
        return pending.get() && queued.compareAndSet(false, true)
    }
}

internal class NetworkChangeTracker<T> {
    private var initialized = false
    private var current: T? = null

    @Synchronized
    fun update(value: T?): Boolean {
        if (initialized && current == value) return false
        initialized = true
        current = value
        return true
    }
}

class DefaultNetworkMonitor(
    context: Context,
    private val onAvailable: () -> Unit,
    private val onUnavailable: () -> Unit
) {
    private val connectivity = context.getSystemService(ConnectivityManager::class.java)
    private val registered = AtomicBoolean(false)
    private val networkChanges = NetworkChangeTracker<Network>()
    private val callback = object : ConnectivityManager.NetworkCallback() {
        override fun onAvailable(network: Network) {
            publish(network)
        }

        override fun onLost(network: Network) {
            publish(connectivity.activeNetwork?.takeUnless { it == network })
        }
    }

    fun start() {
        if (!registered.compareAndSet(false, true)) return
        connectivity.registerDefaultNetworkCallback(callback)
        publish(connectivity.activeNetwork)
    }

    fun stop() {
        if (!registered.compareAndSet(true, false)) return
        try {
            connectivity.unregisterNetworkCallback(callback)
        } catch (_: IllegalArgumentException) {
        }
    }

    private fun publish(network: Network?) {
        if (!networkChanges.update(network)) return
        if (network != null) onAvailable() else onUnavailable()
    }
}
