package com.lsyl.tunnel.mobile.service

import android.content.Context

class RuntimeStateStore(
    context: Context,
    private val onPublish: (RuntimeSnapshot) -> Unit = {}
) : RuntimeStateSink {
    private val preferences = context.getSharedPreferences(PREFERENCES, Context.MODE_PRIVATE)

    override fun desiredRunning(): Boolean = preferences.getBoolean(KEY_DESIRED_RUNNING, false)

    override fun setDesiredRunning(value: Boolean) {
        preferences.edit().putBoolean(KEY_DESIRED_RUNNING, value).commit()
    }

    override fun publish(snapshot: RuntimeSnapshot) {
        preferences.edit().putString(KEY_SNAPSHOT, snapshot.toJson()).apply()
        onPublish(snapshot)
    }

    fun loadSnapshot(): RuntimeSnapshot {
        val raw = preferences.getString(KEY_SNAPSHOT, null)
            ?: return RuntimeSnapshot(TunnelPhase.DISCONNECTED, "未连接")
        return try {
            RuntimeSnapshot.fromJson(raw)
        } catch (_: Exception) {
            RuntimeSnapshot(TunnelPhase.FAILED, "运行状态不可读，请重新连接")
        }
    }

    private companion object {
        const val PREFERENCES = "lsyl_tunnel_runtime"
        const val KEY_DESIRED_RUNNING = "desired_running"
        const val KEY_SNAPSHOT = "snapshot"
    }
}
