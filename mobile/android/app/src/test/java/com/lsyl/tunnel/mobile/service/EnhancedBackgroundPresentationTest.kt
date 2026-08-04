package com.lsyl.tunnel.mobile.service

import org.junit.Assert.assertEquals
import org.junit.Test

class EnhancedBackgroundPresentationTest {
    private class FakeStorage : EnhancedBackgroundStorage {
        var enabled = false
        var status = EnhancedResourceStatus()

        override fun loadEnabled(): Boolean = enabled

        override fun saveEnabled(enabled: Boolean) {
            this.enabled = enabled
        }

        override fun loadResourceStatus(): EnhancedResourceStatus = status

        override fun saveResourceStatus(status: EnhancedResourceStatus) {
            this.status = status
        }
    }

    @Test
    fun settingsDefaultToDisabledAndEmptyStatus() {
        val settings = EnhancedBackgroundSettings(FakeStorage())

        assertEquals(false, settings.enabled())
        assertEquals(EnhancedResourceStatus(), settings.resourceStatus())
    }

    @Test
    fun settingsPersistEnabledAndResourceStatus() {
        val storage = FakeStorage()
        val settings = EnhancedBackgroundSettings(storage)
        val status = EnhancedResourceStatus(cpuHeld = true, warning = "Wi-Fi 保持失败")

        settings.setEnabled(true)
        settings.publishResourceStatus(status)

        assertEquals(true, storage.enabled)
        assertEquals(status, storage.status)
    }
    @Test
    fun disabledHasNoDetail() {
        assertEquals(
            EnhancedBackgroundPresentation(checked = false, detail = ""),
            EnhancedBackgroundPresenter.present(
                enabled = false,
                batteryExempt = false,
                desiredRunning = false,
                resourceStatus = EnhancedResourceStatus()
            )
        )
    }

    @Test
    fun enabledWithoutBatteryExemptionShowsIncompleteRestriction() {
        assertEquals(
            "增强后台已开启，系统限制未完全解除",
            EnhancedBackgroundPresenter.present(
                enabled = true,
                batteryExempt = false,
                desiredRunning = true,
                resourceStatus = EnhancedResourceStatus(cpuHeld = true)
            ).detail
        )
    }

    @Test
    fun resourceFailureTakesPriorityWhileRunning() {
        assertEquals(
            "增强后台已开启，CPU 保持失败",
            EnhancedBackgroundPresenter.present(
                enabled = true,
                batteryExempt = true,
                desiredRunning = true,
                resourceStatus = EnhancedResourceStatus(warning = "CPU 保持失败")
            ).detail
        )
    }

    @Test
    fun enabledAndAuthorizedShowsActiveState() {
        assertEquals(
            "增强后台已开启",
            EnhancedBackgroundPresenter.present(
                enabled = true,
                batteryExempt = true,
                desiredRunning = true,
                resourceStatus = EnhancedResourceStatus(cpuHeld = true)
            ).detail
        )
    }

    @Test
    fun disconnectedDoesNotReportStaleResourceFailure() {
        assertEquals(
            "增强后台已开启",
            EnhancedBackgroundPresenter.present(
                enabled = true,
                batteryExempt = true,
                desiredRunning = false,
                resourceStatus = EnhancedResourceStatus(warning = "CPU 保持失败")
            ).detail
        )
    }
}

