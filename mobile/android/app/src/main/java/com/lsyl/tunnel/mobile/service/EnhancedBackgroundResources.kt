package com.lsyl.tunnel.mobile.service

import android.content.Context
import android.net.wifi.WifiManager
import android.os.Build
import android.os.PowerManager

internal interface BackgroundResourceLock {
    val isHeld: Boolean
    fun acquire()
    fun release()
}

internal class EnhancedBackgroundResourceController(
    private val cpuLock: BackgroundResourceLock,
    private val wifiLock: BackgroundResourceLock?,
    private val statusSink: (EnhancedResourceStatus) -> Unit
) {
    @Synchronized
    fun update(enabled: Boolean, desiredRunning: Boolean, wifiAvailable: Boolean) {
        if (!enabled || !desiredRunning) {
            release()
            return
        }

        val warnings = mutableListOf<String>()
        acquireQuietly(cpuLock, "CPU 保持失败", warnings)
        if (wifiAvailable && wifiLock != null) {
            acquireQuietly(wifiLock, "Wi-Fi 保持失败", warnings)
        } else {
            releaseQuietly(wifiLock, "Wi-Fi 释放失败", warnings)
        }
        publish(warnings)
    }

    @Synchronized
    fun release() {
        val warnings = mutableListOf<String>()
        releaseQuietly(wifiLock, "Wi-Fi 释放失败", warnings)
        releaseQuietly(cpuLock, "CPU 释放失败", warnings)
        publish(warnings)
    }

    private fun acquireQuietly(
        lock: BackgroundResourceLock,
        warning: String,
        warnings: MutableList<String>
    ) {
        if (lock.isHeld) return
        try {
            lock.acquire()
        } catch (_: Exception) {
            warnings += warning
        }
    }

    private fun releaseQuietly(
        lock: BackgroundResourceLock?,
        warning: String,
        warnings: MutableList<String>
    ) {
        if (lock == null || !lock.isHeld) return
        try {
            lock.release()
        } catch (_: Exception) {
            warnings += warning
        }
    }

    private fun publish(warnings: List<String>) {
        statusSink(
            EnhancedResourceStatus(
                cpuHeld = cpuLock.isHeld,
                wifiHeld = wifiLock?.isHeld == true,
                warning = warnings.joinToString("，")
            )
        )
    }

    companion object {
        fun create(
            context: Context,
            statusSink: (EnhancedResourceStatus) -> Unit
        ): EnhancedBackgroundResourceController {
            val appContext = context.applicationContext
            val powerManager = appContext.getSystemService(PowerManager::class.java)
            val cpuWakeLock = powerManager.newWakeLock(
                PowerManager.PARTIAL_WAKE_LOCK,
                "LSYL Tunnel:enhanced-background"
            ).apply {
                setReferenceCounted(false)
            }
            return EnhancedBackgroundResourceController(
                cpuLock = AndroidWakeLock(cpuWakeLock),
                wifiLock = createWifiLock(appContext),
                statusSink = statusSink
            )
        }

        @Suppress("DEPRECATION")
        private fun createWifiLock(context: Context): BackgroundResourceLock? {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) return null
            val wifiManager = context.getSystemService(WifiManager::class.java) ?: return null
            val lock = wifiManager.createWifiLock(
                WifiManager.WIFI_MODE_FULL_HIGH_PERF,
                "LSYL Tunnel:enhanced-background-wifi"
            ).apply {
                setReferenceCounted(false)
            }
            return AndroidWifiLock(lock)
        }
    }
}

private class AndroidWakeLock(
    private val lock: PowerManager.WakeLock
) : BackgroundResourceLock {
    override val isHeld: Boolean
        get() = lock.isHeld

    override fun acquire() {
        lock.acquire()
    }

    override fun release() {
        lock.release()
    }
}

private class AndroidWifiLock(
    private val lock: WifiManager.WifiLock
) : BackgroundResourceLock {
    override val isHeld: Boolean
        get() = lock.isHeld

    override fun acquire() {
        lock.acquire()
    }

    override fun release() {
        lock.release()
    }
}

