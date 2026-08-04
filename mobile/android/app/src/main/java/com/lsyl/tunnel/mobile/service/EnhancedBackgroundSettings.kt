package com.lsyl.tunnel.mobile.service

import android.content.Context

data class EnhancedResourceStatus(
    val cpuHeld: Boolean = false,
    val wifiHeld: Boolean = false,
    val warning: String = ""
)

data class EnhancedBackgroundPresentation(
    val checked: Boolean,
    val detail: String
)

object EnhancedBackgroundPresenter {
    fun present(
        enabled: Boolean,
        batteryExempt: Boolean,
        desiredRunning: Boolean,
        resourceStatus: EnhancedResourceStatus
    ): EnhancedBackgroundPresentation {
        if (!enabled) return EnhancedBackgroundPresentation(checked = false, detail = "")
        val detail = when {
            desiredRunning && resourceStatus.warning.isNotBlank() ->
                "增强后台已开启，${resourceStatus.warning}"
            !batteryExempt -> "增强后台已开启，系统限制未完全解除"
            else -> "增强后台已开启"
        }
        return EnhancedBackgroundPresentation(checked = true, detail = detail)
    }
}

internal interface EnhancedBackgroundStorage {
    fun loadEnabled(): Boolean
    fun saveEnabled(enabled: Boolean)
    fun loadResourceStatus(): EnhancedResourceStatus
    fun saveResourceStatus(status: EnhancedResourceStatus)
}

class EnhancedBackgroundSettings internal constructor(
    private val storage: EnhancedBackgroundStorage
) {
    constructor(context: Context) : this(SharedPreferencesEnhancedBackgroundStorage(context))

    fun enabled(): Boolean = storage.loadEnabled()

    fun setEnabled(enabled: Boolean) {
        storage.saveEnabled(enabled)
    }

    fun resourceStatus(): EnhancedResourceStatus = storage.loadResourceStatus()

    fun publishResourceStatus(status: EnhancedResourceStatus) {
        storage.saveResourceStatus(status)
    }
}

private class SharedPreferencesEnhancedBackgroundStorage(context: Context) : EnhancedBackgroundStorage {
    private val preferences = context.applicationContext.getSharedPreferences(PREFERENCES, Context.MODE_PRIVATE)

    override fun loadEnabled(): Boolean = preferences.getBoolean(KEY_ENABLED, false)

    override fun saveEnabled(enabled: Boolean) {
        preferences.edit().putBoolean(KEY_ENABLED, enabled).commit()
    }

    override fun loadResourceStatus(): EnhancedResourceStatus = EnhancedResourceStatus(
        cpuHeld = preferences.getBoolean(KEY_CPU_HELD, false),
        wifiHeld = preferences.getBoolean(KEY_WIFI_HELD, false),
        warning = preferences.getString(KEY_WARNING, "").orEmpty()
    )

    override fun saveResourceStatus(status: EnhancedResourceStatus) {
        preferences.edit()
            .putBoolean(KEY_CPU_HELD, status.cpuHeld)
            .putBoolean(KEY_WIFI_HELD, status.wifiHeld)
            .putString(KEY_WARNING, status.warning)
            .apply()
    }

    private companion object {
        const val PREFERENCES = "lsyl_tunnel_background"
        const val KEY_ENABLED = "enhanced_background_enabled"
        const val KEY_CPU_HELD = "enhanced_cpu_held"
        const val KEY_WIFI_HELD = "enhanced_wifi_held"
        const val KEY_WARNING = "enhanced_resource_warning"
    }
}
