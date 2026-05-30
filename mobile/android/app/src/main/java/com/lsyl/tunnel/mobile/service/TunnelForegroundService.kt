package com.lsyl.tunnel.mobile.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.IBinder
import android.os.Handler
import android.os.Looper
import com.lsyl.tunnel.mobile.MainActivity
import com.lsyl.tunnel.mobile.R
import com.lsyl.tunnel.mobile.profile.ProfileStore
import com.lsyl.tunnel.mobile.tunnel.TunnelManager
import java.util.concurrent.Executors
import java.util.concurrent.RejectedExecutionException

class TunnelForegroundService : Service() {
    private val executor = Executors.newSingleThreadExecutor()
    private val monitorHandler = Handler(Looper.getMainLooper())
    private val monitorRunnable = object : Runnable {
        override fun run() {
            runMonitorCheck()
        }
    }
    @Volatile private var manager: TunnelManager? = null
    @Volatile private var foregroundActive: Boolean = false
    @Volatile private var monitorActive: Boolean = false

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        ensureChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        return when (intent?.action ?: ACTION_START) {
            ACTION_START -> {
                startTunnel()
                START_STICKY
            }
            ACTION_REFRESH -> {
                refreshTunnel()
                START_STICKY
            }
            ACTION_STOP -> {
                stopTunnelAndSelf()
                START_NOT_STICKY
            }
            else -> START_NOT_STICKY
        }
    }

    override fun onDestroy() {
        stopRuntimeMonitor()
        manager?.stop()
        manager = null
        removeForegroundNotification()
        foregroundActive = false
        executor.shutdownNow()
        super.onDestroy()
    }

    private fun startTunnel() {
        startForeground(NOTIFICATION_ID, notification("正在连接"))
        foregroundActive = true
        notifyStatus("正在连接")
        executor.execute {
            try {
                val current = manager
                if (current != null && current.stats().running) {
                    current.refresh()
                    notifyStatus(current.stats().message)
                    startRuntimeMonitor()
                    return@execute
                }
                val loaded = ProfileStore(this).load() ?: throw IllegalStateException("未导入连接配置")
                val next = TunnelManager(loaded)
                next.start()
                manager = next
                notifyStatus(next.stats().message)
                startRuntimeMonitor()
            } catch (err: Exception) {
                stopDueToFailure("连接失败：${failureMessage(err)}", keepFinalNotification = false)
            }
        }
    }

    private fun refreshTunnel() {
        notifyStatus("正在刷新")
        executor.execute {
            try {
                val current = manager
                if (current == null || !current.stats().running) {
                    notifyStatus("已断开")
                    removeForegroundNotification()
                    stopSelf()
                    return@execute
                }
                current.refresh()
                notifyStatus(current.stats().message)
            } catch (err: Exception) {
                stopDueToFailure("运行异常：${failureMessage(err)}，已断开", keepFinalNotification = true)
            }
        }
    }

    private fun stopTunnelAndSelf() {
        stopRuntimeMonitor()
        notifyStatus("正在断开")
        executor.execute {
            manager?.stop()
            manager = null
            notifyStatus("已断开")
            removeForegroundNotification()
            stopSelf()
        }
    }

    private fun startRuntimeMonitor() {
        monitorActive = true
        monitorHandler.removeCallbacks(monitorRunnable)
        monitorHandler.postDelayed(monitorRunnable, MONITOR_INTERVAL_MS)
    }

    private fun stopRuntimeMonitor() {
        monitorActive = false
        monitorHandler.removeCallbacks(monitorRunnable)
    }

    private fun runMonitorCheck() {
        if (!monitorActive) return
        try {
            executor.execute {
                if (!monitorActive) return@execute
                try {
                    val current = manager
                    if (current == null || !current.stats().running) {
                        stopRuntimeMonitor()
                        return@execute
                    }
                    current.refresh()
                    notifyStatus(current.stats().message)
                    if (monitorActive) {
                        monitorHandler.postDelayed(monitorRunnable, MONITOR_INTERVAL_MS)
                    }
                } catch (err: Exception) {
                    stopDueToFailure("运行异常：${failureMessage(err)}，已断开", keepFinalNotification = true)
                }
            }
        } catch (_: RejectedExecutionException) {
        }
    }

    private fun stopDueToFailure(text: String, keepFinalNotification: Boolean) {
        stopRuntimeMonitor()
        manager?.stop()
        manager = null
        notifyStatus(text)
        removeForegroundNotification()
        if (keepFinalNotification) {
            showFinalNotification(text)
        }
        stopSelf()
    }

    private fun notifyStatus(text: String) {
        saveStatus(this, text)
        if (foregroundActive) {
            val nm = getSystemService(NotificationManager::class.java)
            nm.notify(NOTIFICATION_ID, notification(text))
        }
        sendBroadcast(Intent(ACTION_STATUS).setPackage(packageName).putExtra(EXTRA_STATUS, text))
    }

    private fun removeForegroundNotification() {
        try {
            stopForeground(STOP_FOREGROUND_REMOVE)
        } catch (_: Exception) {
        }
        foregroundActive = false
        getSystemService(NotificationManager::class.java).cancel(NOTIFICATION_ID)
    }

    private fun showFinalNotification(text: String) {
        val nm = getSystemService(NotificationManager::class.java)
        nm.notify(NOTIFICATION_ID, notification(text, ongoing = false))
    }

    private fun notification(text: String, ongoing: Boolean = true): Notification {
        val openIntent = Intent(this, MainActivity::class.java)
        val pending = PendingIntent.getActivity(
            this,
            0,
            openIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        return Notification.Builder(this, CHANNEL_ID)
            .setContentTitle("LSYL Tunnel")
            .setContentText(text)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentIntent(pending)
            .setOngoing(ongoing)
            .setAutoCancel(!ongoing)
            .build()
    }

    private fun ensureChannel() {
        val nm = getSystemService(NotificationManager::class.java)
        val channel = NotificationChannel(CHANNEL_ID, "LSYL Tunnel", NotificationManager.IMPORTANCE_LOW)
        nm.createNotificationChannel(channel)
    }

    companion object {
        private const val CHANNEL_ID = "lsyl_tunnel"
        private const val NOTIFICATION_ID = 3443
        private const val MONITOR_INTERVAL_MS = 15_000L
        const val ACTION_START = "com.lsyl.tunnel.mobile.START"
        const val ACTION_REFRESH = "com.lsyl.tunnel.mobile.REFRESH"
        const val ACTION_STOP = "com.lsyl.tunnel.mobile.STOP"
        const val ACTION_STATUS = "com.lsyl.tunnel.mobile.STATUS"
        const val EXTRA_STATUS = "status"
        private const val PREFS = "lsyl_tunnel_status"
        private const val KEY_STATUS = "status"

        fun startIntent(context: Context): Intent = Intent(context, TunnelForegroundService::class.java).setAction(ACTION_START)
        fun refreshIntent(context: Context): Intent = Intent(context, TunnelForegroundService::class.java).setAction(ACTION_REFRESH)
        fun stopIntent(context: Context): Intent = Intent(context, TunnelForegroundService::class.java).setAction(ACTION_STOP)

        fun currentStatus(context: Context): String? =
            context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getString(KEY_STATUS, null)

        private fun saveStatus(context: Context, text: String) {
            context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
                .edit()
                .putString(KEY_STATUS, text)
                .apply()
        }

        private fun failureMessage(err: Exception): String {
            val raw = err.message?.takeIf { it.isNotBlank() } ?: err.javaClass.simpleName.ifBlank { "未知错误" }
            val text = raw.lowercase()
            return when {
                "credential_expired" in text || "saved login has expired" in text -> "连接凭证已过期"
                "auth_failed" in text || "username or password" in text -> "认证失败"
                "certificate pin mismatch" in text || "certpath" in text -> "服务端证书不匹配"
                "tls" in text || "handshake" in text -> "TLS 连接失败"
                "enetunreach" in text || "network is unreachable" in text || "no route" in text -> "网络不可用"
                "timed out" in text || "timeout" in text -> "服务端连接超时"
                "connection refused" in text || "econnrefused" in text -> "服务端拒绝连接"
                "failed to connect" in text || "unable to connect" in text -> "服务端不可达"
                else -> raw
            }
        }
    }
}
