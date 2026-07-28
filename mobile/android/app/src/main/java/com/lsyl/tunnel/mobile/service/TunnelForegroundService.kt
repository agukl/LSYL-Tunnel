package com.lsyl.tunnel.mobile.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.IBinder
import com.lsyl.tunnel.mobile.MainActivity
import com.lsyl.tunnel.mobile.R
import com.lsyl.tunnel.mobile.profile.ProfileImportException
import com.lsyl.tunnel.mobile.profile.ProfileStore
import com.lsyl.tunnel.mobile.tunnel.TunnelManager
import java.util.concurrent.Executors
import java.util.concurrent.RejectedExecutionException

class TunnelForegroundService : Service() {
    private val executor = Executors.newSingleThreadExecutor()
    private val recoveryGate = RecoverySignalGate()
    private lateinit var stateStore: RuntimeStateStore
    private lateinit var controller: TunnelRuntimeController
    private lateinit var networkMonitor: DefaultNetworkMonitor
    @Volatile private var foregroundActive = false
    @Volatile private var finalNotificationVisible = false

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        ensureChannel()
        stateStore = RuntimeStateStore(this, ::publishSnapshot)
        controller = TunnelRuntimeController(
            runtimeFactory = {
                val loaded = ProfileStore(this).load() ?: throw ProfileImportException("未导入连接配置")
                TunnelManager(loaded)
            },
            stateSink = stateStore
        )
        networkMonitor = DefaultNetworkMonitor(
            context = this,
            onAvailable = ::queueNetworkRecovery,
            onUnavailable = ::publishNetworkUnavailable
        )
        networkMonitor.start()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent == null) {
            if (!stateStore.desiredRunning()) {
                stopSelf()
                return START_NOT_STICKY
            }
            ensureForeground(RuntimeSnapshot(TunnelPhase.STARTING, "正在恢复连接"))
            executeCommand { controller.connect(userInitiated = false) }
            return START_STICKY
        }

        return when (intent.action) {
            ACTION_START -> {
                ensureForeground(RuntimeSnapshot(TunnelPhase.STARTING, "正在连接"))
                executeCommand { controller.connect(userInitiated = true) }
                START_STICKY
            }
            ACTION_REFRESH -> {
                if (stateStore.desiredRunning()) {
                    ensureForeground(stateStore.loadSnapshot())
                }
                executeCommand { controller.refresh() }
                if (stateStore.desiredRunning()) START_STICKY else START_NOT_STICKY
            }
            ACTION_STOP -> {
                executeCommand { controller.disconnect() }
                START_NOT_STICKY
            }
            else -> START_NOT_STICKY
        }
    }

    override fun onDestroy() {
        networkMonitor.stop()
        controller.releaseForSystem()
        if (foregroundActive) {
            try {
                stopForeground(STOP_FOREGROUND_REMOVE)
            } catch (_: Exception) {
            }
        }
        if (!finalNotificationVisible) {
            getSystemService(NotificationManager::class.java).cancel(NOTIFICATION_ID)
        }
        foregroundActive = false
        executor.shutdownNow()
        super.onDestroy()
    }

    private fun queueNetworkRecovery() {
        if (!foregroundActive || !stateStore.desiredRunning() || !recoveryGate.tryQueue()) return
        try {
            executor.execute {
                try {
                    controller.refresh()
                    finishIfInactive()
                } finally {
                    recoveryGate.complete()
                }
            }
        } catch (_: RejectedExecutionException) {
            recoveryGate.complete()
        }
    }

    private fun publishNetworkUnavailable() {
        if (!stateStore.desiredRunning()) return
        executeCommand { controller.networkUnavailable() }
    }

    private fun executeCommand(command: () -> Unit) {
        try {
            executor.execute {
                command()
                finishIfInactive()
            }
        } catch (_: RejectedExecutionException) {
        }
    }

    private fun finishIfInactive() {
        if (stateStore.desiredRunning()) return
        val snapshot = stateStore.loadSnapshot()
        removeForegroundNotification()
        if (snapshot.phase == TunnelPhase.FAILED) showFinalNotification(snapshot)
        stopSelf()
    }

    private fun publishSnapshot(snapshot: RuntimeSnapshot) {
        if (foregroundActive) {
            getSystemService(NotificationManager::class.java).notify(NOTIFICATION_ID, notification(snapshot))
        }
        sendBroadcast(
            Intent(ACTION_STATUS)
                .setPackage(packageName)
                .putExtra(EXTRA_STATUS, snapshot.summary)
                .putExtra(EXTRA_SNAPSHOT, snapshot.toJson())
        )
    }

    private fun ensureForeground(snapshot: RuntimeSnapshot) {
        finalNotificationVisible = false
        if (!foregroundActive) {
            startForeground(NOTIFICATION_ID, notification(snapshot))
            foregroundActive = true
        } else {
            getSystemService(NotificationManager::class.java).notify(NOTIFICATION_ID, notification(snapshot))
        }
    }

    private fun removeForegroundNotification() {
        try {
            stopForeground(STOP_FOREGROUND_REMOVE)
        } catch (_: Exception) {
        }
        foregroundActive = false
        finalNotificationVisible = false
        getSystemService(NotificationManager::class.java).cancel(NOTIFICATION_ID)
    }

    private fun showFinalNotification(snapshot: RuntimeSnapshot) {
        finalNotificationVisible = true
        getSystemService(NotificationManager::class.java).notify(
            NOTIFICATION_ID,
            notification(snapshot, ongoing = false)
        )
    }

    private fun notification(snapshot: RuntimeSnapshot, ongoing: Boolean = true): Notification {
        val openIntent = Intent(this, MainActivity::class.java)
        val pending = PendingIntent.getActivity(
            this,
            0,
            openIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        return Notification.Builder(this, CHANNEL_ID)
            .setContentTitle("LSYL Tunnel")
            .setContentText(snapshot.summary)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentIntent(pending)
            .setOngoing(ongoing)
            .setAutoCancel(!ongoing)
            .build()
    }

    private fun ensureChannel() {
        val channel = NotificationChannel(CHANNEL_ID, "LSYL Tunnel", NotificationManager.IMPORTANCE_LOW)
        getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
    }

    companion object {
        private const val CHANNEL_ID = "lsyl_tunnel"
        private const val NOTIFICATION_ID = 3443
        const val ACTION_START = "com.lsyl.tunnel.mobile.START"
        const val ACTION_REFRESH = "com.lsyl.tunnel.mobile.REFRESH"
        const val ACTION_STOP = "com.lsyl.tunnel.mobile.STOP"
        const val ACTION_STATUS = "com.lsyl.tunnel.mobile.STATUS"
        const val EXTRA_STATUS = "status"
        const val EXTRA_SNAPSHOT = "snapshot"

        fun startIntent(context: Context): Intent =
            Intent(context, TunnelForegroundService::class.java).setAction(ACTION_START)

        fun refreshIntent(context: Context): Intent =
            Intent(context, TunnelForegroundService::class.java).setAction(ACTION_REFRESH)

        fun stopIntent(context: Context): Intent =
            Intent(context, TunnelForegroundService::class.java).setAction(ACTION_STOP)

        fun currentSnapshot(context: Context): RuntimeSnapshot = RuntimeStateStore(context).loadSnapshot()

        fun currentStatus(context: Context): String = currentSnapshot(context).summary
    }
}
